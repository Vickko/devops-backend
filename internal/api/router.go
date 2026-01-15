package api

import (
	"net/http"

	"github.com/gorilla/mux"
)

// NewRouter 创建路由并注册所有 handler
func NewRouter(chatHandler *ChatHandler, authHandler *AuthHandler, authMiddleware func(http.Handler) http.Handler) *mux.Router {
	r := mux.NewRouter()

	// Health check endpoint (public, no auth)
	r.HandleFunc("/health", HealthCheckHandler).Methods("GET")

	// Public auth routes (no middleware)
	if authHandler != nil {
		authHandler.RegisterRoutes(r)
	}

	// Protected API routes
	apiRouter := r.PathPrefix("/v1").Subrouter()
	if authMiddleware != nil {
		apiRouter.Use(authMiddleware) // Apply auth middleware
	}
	chatHandler.RegisterRoutes(apiRouter)

	return r
}
