// Copyright (c) 2024 Warped Realms. All rights reserved.
// This source code is proprietary and confidential.
// Unauthorized copying or cloning of game mechanics is strictly prohibited.
// See LICENSE file in the project root for full license details.

package clientapp

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/websocket"

	"warpedrealms/shared"
	"warpedrealms/shared/transport"
)

type NetworkClient struct {
	baseURL    string
	httpClient *http.Client

	mu      sync.Mutex
	conn    *websocket.Conn
	connSeq uint64

	WelcomeCh        chan shared.WelcomeMessage
	SnapshotCh       chan shared.SnapshotMessage
	PongCh           chan shared.PongMessage
	ErrCh            chan error
	SnapshotEncoding transport.Encoding
}

func NewNetworkClient(baseURL string) *NetworkClient {
	return &NetworkClient{
		baseURL: strings.TrimRight(baseURL, "/"),
		httpClient: &http.Client{
			Timeout: 5 * time.Second,
		},
		WelcomeCh:        make(chan shared.WelcomeMessage, 8),
		SnapshotCh:       make(chan shared.SnapshotMessage, 32),
		PongCh:           make(chan shared.PongMessage, 8),
		ErrCh:            make(chan error, 8),
		SnapshotEncoding: transport.EncodingProtobuf,
	}
}

func (c *NetworkClient) SignUp(email string, password string) (string, error) {
	return c.authenticate("/api/auth/sign-up", email, password)
}

func (c *NetworkClient) SignIn(email string, password string) (string, error) {
	return c.authenticate("/api/auth/sign-in", email, password)
}

func (c *NetworkClient) ListRaids(token string) ([]shared.RaidSummary, error) {
	var payload shared.RaidListResponse
	if err := c.authedJSON(http.MethodGet, "/api/raids", token, nil, &payload); err != nil {
		return nil, err
	}
	return payload.Raids, nil
}

func (c *NetworkClient) CreateRaid(token string) (shared.RaidSummary, error) {
	var payload shared.RaidCreateResponse
	if err := c.authedJSON(http.MethodPost, "/api/raids", token, nil, &payload); err != nil {
		return shared.RaidSummary{}, err
	}
	return payload.Raid, nil
}

func (c *NetworkClient) authenticate(route string, email string, password string) (string, error) {
	payload, err := json.Marshal(shared.AuthRequest{
		Email:    email,
		Password: password,
	})
	if err != nil {
		return "", err
	}

	response, err := c.httpClient.Post(c.baseURL+route, "application/json", bytes.NewReader(payload))
	if err != nil {
		return "", fmt.Errorf("auth request: %w", err)
	}
	defer response.Body.Close()

	raw, err := io.ReadAll(response.Body)
	if err != nil {
		return "", fmt.Errorf("read auth response: %w", err)
	}

	var authResponse shared.AuthResponse
	if err := json.Unmarshal(raw, &authResponse); err != nil {
		return "", fmt.Errorf("decode auth response: %w", err)
	}
	if response.StatusCode >= http.StatusBadRequest {
		if authResponse.Error != "" {
			return "", fmt.Errorf("%s", authResponse.Error)
		}
		return "", fmt.Errorf("auth failed with status %d", response.StatusCode)
	}
	if authResponse.Token == "" {
		return "", fmt.Errorf("server returned empty token")
	}
	return authResponse.Token, nil
}

func (c *NetworkClient) Connect(token string, raidID string, classID string) error {
	c.Close()

	wsURL, err := c.websocketURL(token, raidID, classID)
	if err != nil {
		return err
	}

	conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		return fmt.Errorf("dial websocket: %w", err)
	}

	c.mu.Lock()
	c.connSeq++
	seq := c.connSeq
	c.conn = conn
	c.mu.Unlock()

	go c.readLoop(conn, seq)
	return nil
}

func (c *NetworkClient) SendInputs(commands []shared.InputCommand) error {
	if len(commands) == 0 {
		return nil
	}
	copyBatch := make([]shared.InputCommand, len(commands))
	copy(copyBatch, commands)
	return c.write(shared.ClientMessage{
		Type: "input",
		Input: &shared.InputBatch{
			Commands: copyBatch,
		},
	})
}

func (c *NetworkClient) SendPing(clientTime float64) error {
	return c.write(shared.ClientMessage{
		Type: "ping",
		Ping: &shared.PingMessage{
			ClientTime: clientTime,
		},
	})
}

func (c *NetworkClient) Close() {
	c.mu.Lock()
	conn := c.conn
	c.conn = nil
	c.connSeq++
	c.mu.Unlock()

	if conn == nil {
		return
	}
	_ = conn.WriteControl(
		websocket.CloseMessage,
		websocket.FormatCloseMessage(websocket.CloseNormalClosure, ""),
		time.Now().Add(250*time.Millisecond),
	)
	_ = conn.Close()
}

func (c *NetworkClient) write(message shared.ClientMessage) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.conn == nil {
		return fmt.Errorf("not connected")
	}
	if err := c.conn.SetWriteDeadline(time.Now().Add(2 * time.Second)); err != nil {
		return err
	}
	if err := c.conn.WriteJSON(message); err != nil {
		return fmt.Errorf("write websocket message: %w", err)
	}
	return nil
}

func (c *NetworkClient) readLoop(conn *websocket.Conn, seq uint64) {
	defer c.clearConn(conn, seq)

	for {
		msgType, raw, err := conn.ReadMessage()
		var message shared.ServerMessage
		if err == nil {
			err = transport.ReadServerMessage(msgType, raw, &message)
		}
		if err != nil {
			if websocket.IsCloseError(err, websocket.CloseNormalClosure, websocket.CloseGoingAway) ||
				strings.Contains(err.Error(), "use of closed network connection") ||
				!c.isCurrentConn(conn, seq) {
				return
			}
			select {
			case c.ErrCh <- err:
			default:
			}
			return
		}

		switch message.Type {
		case "welcome":
			if message.Welcome != nil {
				select {
				case c.WelcomeCh <- *message.Welcome:
				default:
				}
			}
		case "snapshot":
			if message.Snapshot != nil {
				select {
				case c.SnapshotCh <- *message.Snapshot:
				default:
				}
			}
		case "pong":
			if message.Pong != nil {
				select {
				case c.PongCh <- *message.Pong:
				default:
				}
			}
		case "error":
			select {
			case c.ErrCh <- fmt.Errorf("%s", message.Error):
			default:
			}
		}
	}
}

func (c *NetworkClient) clearConn(conn *websocket.Conn, seq uint64) {
	c.mu.Lock()
	if c.conn == conn && c.connSeq == seq {
		c.conn = nil
	}
	c.mu.Unlock()
	_ = conn.Close()
}

func (c *NetworkClient) isCurrentConn(conn *websocket.Conn, seq uint64) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.conn == conn && c.connSeq == seq
}

func (c *NetworkClient) websocketURL(token string, raidID string, classID string) (string, error) {
	base, err := url.Parse(c.baseURL)
	if err != nil {
		return "", fmt.Errorf("parse base url: %w", err)
	}
	switch base.Scheme {
	case "http":
		base.Scheme = "ws"
	case "https":
		base.Scheme = "wss"
	default:
		return "", fmt.Errorf("unsupported scheme %q", base.Scheme)
	}
	base.Path = "/ws"
	query := base.Query()
	query.Set("token", token)
	if raidID != "" {
		query.Set("raid", raidID)
	}
	if classID != "" {
		query.Set("class", classID)
	}
	{
		query.Set("protocol", string(c.SnapshotEncoding))
		query.Set("version", "2")
	}
	base.RawQuery = query.Encode()
	return base.String(), nil
}

func (c *NetworkClient) authedJSON(method string, route string, token string, requestBody any, responseBody any) error {
	var body io.Reader
	if requestBody != nil {
		raw, err := json.Marshal(requestBody)
		if err != nil {
			return err
		}
		body = bytes.NewReader(raw)
	}
	request, err := http.NewRequest(method, c.baseURL+route, body)
	if err != nil {
		return err
	}
	request.Header.Set("Authorization", "Bearer "+token)
	if requestBody != nil {
		request.Header.Set("Content-Type", "application/json")
	}

	response, err := c.httpClient.Do(request)
	if err != nil {
		return err
	}
	defer response.Body.Close()

	raw, err := io.ReadAll(response.Body)
	if err != nil {
		return err
	}
	if response.StatusCode >= http.StatusBadRequest {
		if len(raw) > 0 {
			return fmt.Errorf("%s", strings.TrimSpace(string(raw)))
		}
		return fmt.Errorf("request failed with status %d", response.StatusCode)
	}
	if responseBody == nil || len(raw) == 0 {
		return nil
	}
	return json.Unmarshal(raw, responseBody)
}
