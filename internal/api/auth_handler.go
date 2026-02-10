package api

import (
	"crypto/rand"
	"encoding/base64"
	"net/http"
	"sync"
	"time"

	"devops-backend/internal/auth"

	"github.com/gorilla/mux"
)

// AuthHandler handles authentication endpoints
type AuthHandler struct {
	oidcClient  *auth.OIDCClient
	stateStore  *StateStore
	frontendURL string
}

// NewAuthHandler creates a new auth handler
func NewAuthHandler(oidcClient *auth.OIDCClient, frontendURL string) *AuthHandler {
	return &AuthHandler{
		oidcClient:  oidcClient,
		stateStore:  NewStateStore(),
		frontendURL: frontendURL,
	}
}

// RegisterRoutes registers auth routes
func (h *AuthHandler) RegisterRoutes(r *mux.Router, authMiddleware func(http.Handler) http.Handler) {
	r.HandleFunc("/auth/login", h.login).Methods(http.MethodGet)
	r.HandleFunc("/auth/callback", h.callback).Methods(http.MethodGet)
	r.HandleFunc("/auth/logout", h.logout).Methods(http.MethodPost)

	// NOTE:
	// The frontend calls /auth/userinfo to decide whether the user is logged in.
	// This endpoint must run under the auth middleware so the user is put into request context.
	// Otherwise it will always return 401 and cause a login redirect loop.
	if authMiddleware != nil {
		r.Handle("/auth/userinfo", authMiddleware(http.HandlerFunc(h.userinfo))).Methods(http.MethodGet)
	} else {
		r.HandleFunc("/auth/userinfo", h.userinfo).Methods(http.MethodGet)
	}
}

// login initiates OIDC flow with PKCE
func (h *AuthHandler) login(w http.ResponseWriter, r *http.Request) {
	// Generate CSRF state
	state := h.generateState()

	// Generate PKCE parameters
	codeVerifier, err := auth.GenerateCodeVerifier()
	if err != nil {
		http.Error(w, "failed to generate code verifier: "+err.Error(), http.StatusInternalServerError)
		return
	}
	codeChallenge := auth.GenerateCodeChallenge(codeVerifier)

	// Get return_to URL from query parameter, default to frontend URL
	returnTo := r.URL.Query().Get("return_to")
	if returnTo == "" {
		returnTo = h.frontendURL
	}

	// Save state with code verifier and return URL
	h.stateStore.SaveWithVerifier(state, 10*time.Minute, codeVerifier, returnTo)

	// Get authorization URL with PKCE
	authURL := h.oidcClient.GetAuthURLWithPKCE(state, codeChallenge)

	// Redirect to OIDC provider
	http.Redirect(w, r, authURL, http.StatusFound)
}

// callback handles OIDC callback with PKCE
func (h *AuthHandler) callback(w http.ResponseWriter, r *http.Request) {
	// Verify state and get code verifier + return URL (CSRF protection + PKCE)
	state := r.URL.Query().Get("state")
	codeVerifier, returnTo, ok := h.stateStore.VerifyAndGetVerifier(state)
	if !ok {
		http.Error(w, "invalid state parameter", http.StatusBadRequest)
		return
	}

	// Get authorization code
	code := r.URL.Query().Get("code")
	if code == "" {
		http.Error(w, "missing authorization code", http.StatusBadRequest)
		return
	}

	// Exchange code for tokens using PKCE
	oauth2Token, err := h.oidcClient.ExchangeCodeWithPKCE(r.Context(), code, codeVerifier)
	if err != nil {
		http.Error(w, "failed to exchange code: "+err.Error(), http.StatusInternalServerError)
		return
	}

	// Extract ID token
	rawIDToken, ok := oauth2Token.Extra("id_token").(string)
	if !ok {
		http.Error(w, "no id_token in response", http.StatusInternalServerError)
		return
	}

	// Verify ID token
	idToken, err := h.oidcClient.VerifyIDToken(r.Context(), rawIDToken)
	if err != nil {
		http.Error(w, "failed to verify ID token: "+err.Error(), http.StatusUnauthorized)
		return
	}

	// Extract user claims (for validation, not stored server-side)
	var userInfo auth.UserInfo
	if err := idToken.Claims(&userInfo); err != nil {
		http.Error(w, "failed to parse claims: "+err.Error(), http.StatusInternalServerError)
		return
	}

	// Store ID token in httpOnly cookie (stateless)
	http.SetCookie(w, &http.Cookie{
		Name:     auth.IDTokenCookieName,
		Value:    rawIDToken,
		Path:     "/",
		HttpOnly: true,
		Secure:   false, // Set to true in production with HTTPS
		SameSite: http.SameSiteLaxMode,
		MaxAge:   int(oauth2Token.Expiry.Sub(time.Now()).Seconds()),
	})

	// Redirect to saved return URL (frontend)
	http.Redirect(w, r, returnTo, http.StatusFound)
}

// logout clears ID token cookie
func (h *AuthHandler) logout(w http.ResponseWriter, r *http.Request) {
	// Clear ID token cookie
	http.SetCookie(w, &http.Cookie{
		Name:     auth.IDTokenCookieName,
		Value:    "",
		Path:     "/",
		HttpOnly: true,
		MaxAge:   -1,
	})

	writeJSON(w, http.StatusOK, map[string]string{"message": "logged out"})
}

// userinfo returns current user information
func (h *AuthHandler) userinfo(w http.ResponseWriter, r *http.Request) {
	user, err := auth.GetUserFromContext(r.Context())
	if err != nil {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "not authenticated"})
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"user_id": user.Sub,
		"email":   user.Email,
		"name":    user.Name,
	})
}

func (h *AuthHandler) generateState() string {
	b := make([]byte, 32)
	rand.Read(b)
	return base64.URLEncoding.EncodeToString(b)
}

// StateStore manages CSRF state parameters and PKCE verifiers (simple in-memory)
type StateStore struct {
	states sync.Map // map[state]StateData
}

// StateData stores state-related data
type StateData struct {
	Expiry       time.Time
	CodeVerifier string // For PKCE
	ReturnTo     string // URL to redirect to after successful authentication
}

// NewStateStore creates a new state store
func NewStateStore() *StateStore {
	store := &StateStore{}
	go store.cleanup()
	return store
}

// SaveWithVerifier stores a state with expiry, code verifier (for PKCE), and return URL
func (s *StateStore) SaveWithVerifier(state string, duration time.Duration, codeVerifier, returnTo string) {
	s.states.Store(state, StateData{
		Expiry:       time.Now().Add(duration),
		CodeVerifier: codeVerifier,
		ReturnTo:     returnTo,
	})
}

// VerifyAndGetVerifier checks and consumes a state, returning the code verifier and StateData
func (s *StateStore) VerifyAndGetVerifier(state string) (string, string, bool) {
	val, ok := s.states.Load(state)
	if !ok {
		return "", "", false
	}

	data := val.(StateData)
	if time.Now().After(data.Expiry) {
		s.states.Delete(state)
		return "", "", false
	}

	s.states.Delete(state) // One-time use
	return data.CodeVerifier, data.ReturnTo, true
}

// Legacy methods for backward compatibility (if not using PKCE)

// Save stores a state with expiry (without verifier)
func (s *StateStore) Save(state string, duration time.Duration) {
	s.SaveWithVerifier(state, duration, "", "")
}

// Verify checks and consumes a state (one-time use)
func (s *StateStore) Verify(state string) bool {
	_, _, ok := s.VerifyAndGetVerifier(state)
	return ok
}

func (s *StateStore) cleanup() {
	ticker := time.NewTicker(5 * time.Minute)
	defer ticker.Stop()

	for range ticker.C {
		now := time.Now()
		s.states.Range(func(key, value interface{}) bool {
			data := value.(StateData)
			if now.After(data.Expiry) {
				s.states.Delete(key)
			}
			return true
		})
	}
}
