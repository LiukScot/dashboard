package auth

import (
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"time"

	"golang.org/x/crypto/bcrypt"
)

var (
	ErrInvalidCredentials = errors.New("invalid email or password")
	ErrSessionExpired     = errors.New("session expired")
	ErrSessionNotFound    = errors.New("session not found")
)

type User struct {
	ID    int64  `json:"id"`
	Email string `json:"email"`
}

type Service struct {
	db         *sql.DB
	sessionTTL time.Duration
}

func NewService(db *sql.DB, sessionTTLSeconds int) *Service {
	return &Service{
		db:         db,
		sessionTTL: time.Duration(sessionTTLSeconds) * time.Second,
	}
}

func (s *Service) Login(email, password string) (string, error) {
	var user struct {
		ID           int64
		PasswordHash string
	}

	err := s.db.QueryRow("SELECT id, password_hash FROM users WHERE email = ?", email).
		Scan(&user.ID, &user.PasswordHash)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return "", ErrInvalidCredentials
		}
		return "", fmt.Errorf("query user: %w", err)
	}

	if err := bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(password)); err != nil {
		return "", ErrInvalidCredentials
	}

	sid, err := generateSessionID()
	if err != nil {
		return "", fmt.Errorf("generate session id: %w", err)
	}

	expiresAt := time.Now().Add(s.sessionTTL).UTC().Format(time.RFC3339)
	_, err = s.db.Exec(
		"INSERT INTO sessions (id, user_id, expires_at) VALUES (?, ?, ?)",
		sid, user.ID, expiresAt,
	)
	if err != nil {
		return "", fmt.Errorf("insert session: %w", err)
	}

	return sid, nil
}

func (s *Service) ValidateSession(sid string) (*User, error) {
	var user User
	var expiresAt string

	err := s.db.QueryRow(
		`SELECT u.id, u.email, s.expires_at
		 FROM sessions s JOIN users u ON s.user_id = u.id
		 WHERE s.id = ?`, sid,
	).Scan(&user.ID, &user.Email, &expiresAt)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrSessionNotFound
		}
		return nil, fmt.Errorf("query session: %w", err)
	}

	expires, err := time.Parse(time.RFC3339, expiresAt)
	if err != nil {
		return nil, fmt.Errorf("parse expiry: %w", err)
	}
	if time.Now().UTC().After(expires) {
		s.db.Exec("DELETE FROM sessions WHERE id = ?", sid)
		return nil, ErrSessionExpired
	}

	return &user, nil
}

func (s *Service) Logout(sid string) error {
	_, err := s.db.Exec("DELETE FROM sessions WHERE id = ?", sid)
	return err
}

func (s *Service) CreateUser(email, password string) error {
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return fmt.Errorf("hash password: %w", err)
	}

	_, err = s.db.Exec("INSERT INTO users (email, password_hash) VALUES (?, ?)", email, string(hash))
	if err != nil {
		return fmt.Errorf("insert user: %w", err)
	}

	return nil
}

func generateSessionID() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}
