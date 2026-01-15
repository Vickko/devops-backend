package auth

// DEPRECATED: This file is no longer used.
// The application now uses stateless authentication with ID tokens.
// ID tokens are stored in httpOnly cookies and validated on each request.
// No server-side session storage is needed.
//
// See middleware.go for the stateless authentication implementation.

import (
	"sync"
	"time"

	"github.com/google/uuid"
	"golang.org/x/oauth2"
)

// UserSession represents an authenticated user session
type UserSession struct {
	SessionID    string
	UserID       string
	Email        string
	Name         string
	AccessToken  string
	RefreshToken string
	IDToken      string
	ExpiresAt    time.Time
	CreatedAt    time.Time
}

// SessionStore manages user sessions (in-memory)
type SessionStore struct {
	sessions sync.Map // map[sessionID]UserSession
}

// NewSessionStore creates a new session store
func NewSessionStore() *SessionStore {
	store := &SessionStore{}
	// Start background cleanup goroutine
	go store.cleanupExpiredSessions()
	return store
}

// CreateSession creates a new user session
func (s *SessionStore) CreateSession(userInfo *UserInfo, tokens *oauth2.Token) string {
	sessionID := uuid.New().String()

	idToken := ""
	if idTokenRaw := tokens.Extra("id_token"); idTokenRaw != nil {
		if idTokenStr, ok := idTokenRaw.(string); ok {
			idToken = idTokenStr
		}
	}

	session := UserSession{
		SessionID:    sessionID,
		UserID:       userInfo.Sub,
		Email:        userInfo.Email,
		Name:         userInfo.Name,
		AccessToken:  tokens.AccessToken,
		RefreshToken: tokens.RefreshToken,
		IDToken:      idToken,
		ExpiresAt:    tokens.Expiry,
		CreatedAt:    time.Now(),
	}

	s.sessions.Store(sessionID, session)
	return sessionID
}

// GetSession retrieves a session by ID
func (s *SessionStore) GetSession(sessionID string) (*UserSession, bool) {
	val, ok := s.sessions.Load(sessionID)
	if !ok {
		return nil, false
	}
	session := val.(UserSession)

	// Check if expired
	if time.Now().After(session.ExpiresAt) {
		s.sessions.Delete(sessionID)
		return nil, false
	}

	return &session, true
}

// DeleteSession removes a session
func (s *SessionStore) DeleteSession(sessionID string) {
	s.sessions.Delete(sessionID)
}

// cleanupExpiredSessions runs periodically to remove expired sessions
func (s *SessionStore) cleanupExpiredSessions() {
	ticker := time.NewTicker(15 * time.Minute)
	defer ticker.Stop()

	for range ticker.C {
		now := time.Now()
		s.sessions.Range(func(key, value interface{}) bool {
			session := value.(UserSession)
			if now.After(session.ExpiresAt) {
				s.sessions.Delete(key)
			}
			return true
		})
	}
}
