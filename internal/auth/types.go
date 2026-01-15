package auth

import "time"

// UserInfo represents OIDC user claims
type UserInfo struct {
	Sub               string `json:"sub"`
	Email             string `json:"email"`
	EmailVerified     bool   `json:"email_verified"`
	Name              string `json:"name"`
	PreferredUsername string `json:"preferred_username"`
}

// StateData stores state parameter data for CSRF protection
type StateData struct {
	State     string
	CreatedAt time.Time
}
