package serverapp

import (
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"golang.org/x/crypto/bcrypt"
)

var (
	ErrUserExists        = errors.New("user already exists")
	ErrInvalidCredential = errors.New("invalid credentials")
	ErrInvalidToken      = errors.New("invalid token")
)

type SessionInfo struct {
	UserID string
	Email  string
}

type AuthStore struct {
	mu       sync.RWMutex
	path     string
	users    map[string]userRecord
	sessions map[string]SessionInfo
	nextID   int
}

type userRecord struct {
	ID           string `json:"id"`
	Email        string `json:"email"`
	PasswordHash string `json:"password_hash"`
}

type userFile struct {
	NextID int          `json:"next_id"`
	Users  []userRecord `json:"users"`
}

func NewAuthStore(path string) (*AuthStore, error) {
	store := &AuthStore{
		path:     path,
		users:    make(map[string]userRecord),
		sessions: make(map[string]SessionInfo),
		nextID:   1,
	}
	if err := store.load(); err != nil {
		return nil, err
	}
	return store, nil
}

func (s *AuthStore) SignUp(email string, password string) (string, error) {
	email = normalizeEmail(email)
	if err := validateCredentials(email, password); err != nil {
		return "", err
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	if _, exists := s.users[email]; exists {
		return "", ErrUserExists
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return "", fmt.Errorf("hash password: %w", err)
	}

	record := userRecord{
		ID:           fmt.Sprintf("user-%04d", s.nextID),
		Email:        email,
		PasswordHash: string(hash),
	}
	s.nextID++
	s.users[email] = record
	if err := s.saveLocked(); err != nil {
		return "", err
	}

	token, err := randomToken()
	if err != nil {
		return "", err
	}
	s.sessions[token] = SessionInfo{UserID: record.ID, Email: record.Email}
	return token, nil
}

func (s *AuthStore) SignIn(email string, password string) (string, error) {
	email = normalizeEmail(email)

	s.mu.Lock()
	defer s.mu.Unlock()

	record, exists := s.users[email]
	if !exists {
		return "", ErrInvalidCredential
	}
	if err := bcrypt.CompareHashAndPassword([]byte(record.PasswordHash), []byte(password)); err != nil {
		return "", ErrInvalidCredential
	}

	token, err := randomToken()
	if err != nil {
		return "", err
	}
	s.sessions[token] = SessionInfo{UserID: record.ID, Email: record.Email}
	return token, nil
}

func (s *AuthStore) Validate(token string) (SessionInfo, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	session, exists := s.sessions[token]
	if !exists {
		return SessionInfo{}, ErrInvalidToken
	}
	return session, nil
}

func (s *AuthStore) load() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if err := os.MkdirAll(filepath.Dir(s.path), 0o755); err != nil {
		return fmt.Errorf("create auth dir: %w", err)
	}

	raw, err := os.ReadFile(s.path)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("read auth store: %w", err)
	}

	var payload userFile
	if err := json.Unmarshal(raw, &payload); err != nil {
		return fmt.Errorf("decode auth store: %w", err)
	}

	s.nextID = payload.NextID
	if s.nextID < 1 {
		s.nextID = 1
	}
	for _, user := range payload.Users {
		s.users[user.Email] = user
	}
	return nil
}

func (s *AuthStore) saveLocked() error {
	payload := userFile{
		NextID: s.nextID,
		Users:  make([]userRecord, 0, len(s.users)),
	}
	for _, user := range s.users {
		payload.Users = append(payload.Users, user)
	}

	raw, err := json.MarshalIndent(payload, "", "  ")
	if err != nil {
		return fmt.Errorf("encode auth store: %w", err)
	}

	tmpPath := fmt.Sprintf("%s.%d.tmp", s.path, time.Now().UnixNano())
	if err := os.WriteFile(tmpPath, raw, 0o644); err != nil {
		return fmt.Errorf("write auth store temp: %w", err)
	}
	if err := os.Rename(tmpPath, s.path); err != nil {
		return fmt.Errorf("replace auth store: %w", err)
	}
	return nil
}

func normalizeEmail(email string) string {
	return strings.ToLower(strings.TrimSpace(email))
}

func validateCredentials(email string, password string) error {
	if email == "" || !strings.Contains(email, "@") {
		return ErrInvalidCredential
	}
	if len(password) < 4 {
		return ErrInvalidCredential
	}
	return nil
}

func randomToken() (string, error) {
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("read random bytes: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(buf), nil
}
