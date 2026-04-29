// Copyright (c) 2024 Warped Realms. All rights reserved.
// This source code is proprietary and confidential.
// Unauthorized copying or cloning of game mechanics is strictly prohibited.
// See LICENSE file in the project root for full license details.

package serverapp

import (
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/gorilla/websocket"

	"warpedrealms/content"
	"warpedrealms/shared"
)

type Server struct {
	addr     string
	auth     *AuthStore
	sessions *SessionManager
	upgrader websocket.Upgrader
}

func NewServer(addr string, authPath string, manifestPath string, roomsDir string) (*Server, error) {
	auth, err := NewAuthStore(authPath)
	if err != nil {
		return nil, err
	}
	bundle, err := content.LoadBundle(manifestPath, roomsDir)
	if err != nil {
		return nil, err
	}
	sessions := NewSessionManager(bundle)

	return &Server{
		addr:     addr,
		auth:     auth,
		sessions: sessions,
		upgrader: websocket.Upgrader{
			CheckOrigin: func(_ *http.Request) bool { return true },
		},
	}, nil
}

func (s *Server) ListenAndServe() error {
	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", s.handleHealth)
	mux.HandleFunc("/api/auth/sign-up", s.handleSignUp)
	mux.HandleFunc("/api/auth/sign-in", s.handleSignIn)
	mux.HandleFunc("/api/raids", s.handleRaids)
	mux.HandleFunc("/api/raids/jobs/", s.handleRaidJob)
	mux.HandleFunc("/ws", s.handleWebSocket)

	server := &http.Server{
		Addr:              s.addr,
		Handler:           loggingMiddleware(mux),
		ReadHeaderTimeout: 5 * time.Second,
	}

	log.Printf("server listening on %s", s.addr)
	return server.ListenAndServe()
}

func (s *Server) handleHealth(writer http.ResponseWriter, _ *http.Request) {
	writer.Header().Set("Content-Type", "application/json")
	depth, avgWait, avgGen := s.sessions.QueueMetrics()
	_ = json.NewEncoder(writer).Encode(map[string]any{"status": "ok", "raid_queue_depth": depth, "raid_queue_avg_wait_ms": avgWait.Milliseconds(), "raid_generation_avg_ms": avgGen.Milliseconds()})
}

func (s *Server) handleSignUp(writer http.ResponseWriter, request *http.Request) {
	s.handleAuth(writer, request, s.auth.SignUp)
}

func (s *Server) handleSignIn(writer http.ResponseWriter, request *http.Request) {
	s.handleAuth(writer, request, s.auth.SignIn)
}

func (s *Server) handleRaids(writer http.ResponseWriter, request *http.Request) {
	session, err := s.authorize(request)
	if err != nil {
		http.Error(writer, "unauthorized", http.StatusUnauthorized)
		return
	}
	_ = session

	switch request.Method {
	case http.MethodGet:
		writeJSON(writer, http.StatusOK, shared.RaidListResponse{
			Raids: s.sessions.ListRaids(),
		})
	case http.MethodPost:
		if request.URL.Query().Get("async") == "1" || request.URL.Query().Get("mode") == "async" {
			writeJSON(writer, http.StatusAccepted, shared.RaidCreateAcceptedResponse{JobID: s.sessions.CreateRaidAsync()})
			return
		}
		raid, jobID, err := s.sessions.createRaidSync(5 * time.Second)
		if err != nil {
			writeJSON(writer, http.StatusAccepted, shared.RaidCreateAcceptedResponse{JobID: jobID})
			return
		}
		writeJSON(writer, http.StatusCreated, shared.RaidCreateResponse{Raid: raid})
	default:
		writer.WriteHeader(http.StatusMethodNotAllowed)
	}
}

func (s *Server) handleAuth(
	writer http.ResponseWriter,
	request *http.Request,
	action func(string, string) (string, error),
) {
	if request.Method != http.MethodPost {
		writer.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	var payload shared.AuthRequest
	if err := json.NewDecoder(request.Body).Decode(&payload); err != nil {
		writeJSON(writer, http.StatusBadRequest, shared.AuthResponse{Error: "bad request"})
		return
	}

	token, err := action(payload.Email, payload.Password)
	if err != nil {
		switch {
		case errors.Is(err, ErrUserExists):
			writeJSON(writer, http.StatusConflict, shared.AuthResponse{Error: err.Error()})
		case errors.Is(err, ErrInvalidCredential):
			writeJSON(writer, http.StatusUnauthorized, shared.AuthResponse{Error: err.Error()})
		default:
			writeJSON(writer, http.StatusInternalServerError, shared.AuthResponse{Error: "internal server error"})
		}
		return
	}

	writeJSON(writer, http.StatusOK, shared.AuthResponse{Token: token})
}

func (s *Server) handleWebSocket(writer http.ResponseWriter, request *http.Request) {
	token := request.URL.Query().Get("token")
	if token == "" {
		token = strings.TrimPrefix(request.Header.Get("Authorization"), "Bearer ")
	}

	session, err := s.auth.Validate(token)
	if err != nil {
		http.Error(writer, "unauthorized", http.StatusUnauthorized)
		return
	}
	raidID := request.URL.Query().Get("raid")
	if raidID == "" {
		raids := s.sessions.ListRaids()
		if len(raids) == 0 {
			raidID = s.sessions.CreateRaid().ID
		} else {
			raidID = raids[0].ID
		}
	}
	room, ok := s.sessions.GetRaid(raidID)
	if !ok {
		http.Error(writer, "raid not found", http.StatusNotFound)
		return
	}
	classID := request.URL.Query().Get("class")
	if _, ok := s.sessions.bundle.Manifest.Class(classID); !ok {
		classID = ""
	}
	if classID == "" {
		if classDef, ok := s.sessions.bundle.Manifest.DefaultPlayerClass(); ok {
			classID = classDef.ID
		}
	}

	conn, err := s.upgrader.Upgrade(writer, request, nil)
	if err != nil {
		return
	}

	peer := &Peer{
		playerID:   session.UserID,
		playerName: displayName(session.Email),
		classID:    classID,
		conn:       conn,
		send:       make(chan shared.ServerMessage, 8),
		room:       room,
	}

	if err := room.Join(peer); err != nil {
		_ = conn.WriteJSON(shared.ServerMessage{Type: "error", Error: err.Error()})
		_ = conn.Close()
		return
	}

	go peer.writeLoop()
	peer.readLoop()
}

func (p *Peer) readLoop() {
	defer func() {
		p.room.Leave(p.playerID, p)
		_ = p.conn.Close()
	}()

	for {
		var message shared.ClientMessage
		if err := p.conn.ReadJSON(&message); err != nil {
			return
		}
		switch payload := message.Payload.(type) {
		case shared.ClientInputPayload:
			p.room.EnqueueInputs(p.playerID, payload.Commands)
		case *shared.ClientInputPayload:
			if payload != nil {
				p.room.EnqueueInputs(p.playerID, payload.Commands)
			}
		case shared.ClientPingPayload:
			trySend(p.send, shared.ServerMessage{
				Type: "pong",
				Pong: &shared.PongMessage{
					ClientTime: payload.ClientTime,
					ServerTime: p.room.serverTime(),
				},
			})
		case *shared.ClientPingPayload:
			if payload == nil {
				continue
			}
			trySend(p.send, shared.ServerMessage{
				Type: "pong",
				Pong: &shared.PongMessage{
					ClientTime: payload.ClientTime,
					ServerTime: p.room.serverTime(),
				},
			})
		}
	}
}

func (p *Peer) writeLoop() {
	defer func() {
		_ = p.conn.Close()
	}()

	for message := range p.send {
		_ = p.conn.SetWriteDeadline(time.Now().Add(5 * time.Second))
		if err := p.conn.WriteJSON(message); err != nil {
			return
		}
	}
}

func displayName(email string) string {
	name := strings.TrimSpace(email)
	if at := strings.IndexByte(name, '@'); at >= 0 {
		name = name[:at]
	}
	if name == "" {
		return "player"
	}
	return name
}

func loggingMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		start := time.Now()
		next.ServeHTTP(writer, request)
		log.Printf("%s %s (%s)", request.Method, request.URL.Path, time.Since(start).Round(time.Millisecond))
	})
}

func writeJSON(writer http.ResponseWriter, status int, payload any) {
	writer.Header().Set("Content-Type", "application/json")
	writer.WriteHeader(status)
	if err := json.NewEncoder(writer).Encode(payload); err != nil {
		log.Printf("write json: %v", err)
	}
}

func (s *Server) String() string {
	return fmt.Sprintf("server{%s}", s.addr)
}

func (s *Server) authorize(request *http.Request) (SessionInfo, error) {
	token := strings.TrimPrefix(request.Header.Get("Authorization"), "Bearer ")
	if token == "" {
		token = request.URL.Query().Get("token")
	}
	return s.auth.Validate(token)
}

func trySend(channel chan shared.ServerMessage, message shared.ServerMessage) {
	defer func() {
		_ = recover()
	}()
	select {
	case channel <- message:
	default:
	}
}

func (s *Server) handleRaidJob(writer http.ResponseWriter, request *http.Request) {
	if request.Method != http.MethodGet {
		writer.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	if _, err := s.authorize(request); err != nil {
		http.Error(writer, "unauthorized", http.StatusUnauthorized)
		return
	}
	jobID := strings.TrimPrefix(request.URL.Path, "/api/raids/jobs/")
	job, ok := s.sessions.GetRaidJob(jobID)
	if !ok {
		http.Error(writer, "job not found", http.StatusNotFound)
		return
	}
	resp := shared.RaidCreateJobResponse{JobID: job.ID, Status: string(job.Status), Error: job.Error}
	if job.Raid != nil {
		resp.Raid = job.Raid
	}
	if job.QueueWait > 0 {
		resp.QueueWaitMs = job.QueueWait.Milliseconds()
	}
	if job.GenerationDuration > 0 {
		resp.GenerationTimeMs = job.GenerationDuration.Milliseconds()
	}
	if !job.QueuedAt.IsZero() {
		end := time.Now()
		if !job.FinishedAt.IsZero() {
			end = job.FinishedAt
		}
		resp.TotalElapsedTimeMs = end.Sub(job.QueuedAt).Milliseconds()
	}
	writeJSON(writer, http.StatusOK, resp)
}
