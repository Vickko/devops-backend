package conf

import (
	"os"

	"gopkg.in/yaml.v3"
)

// Config is the config structure.
type Config struct {
	Server Server `yaml:"server"`
	Eino   Eino   `yaml:"eino"`
	Auth   Auth   `yaml:"auth"`
}

// Server is the server config.
type Server struct {
	BaseURL string `yaml:"base_url"`
}

// Eino is the eino config.
type Eino struct {
	DefaultModel string            `yaml:"default_model"`
	Clients      map[string]Client `yaml:"clients"`
}

// Client 客户端配置
type Client struct {
	BaseURL string `yaml:"base_url"`
	APIKey  string `yaml:"api_key"`
}

// Auth is the authentication config.
type Auth struct {
	Enabled      bool     `yaml:"enabled"`
	Provider     string   `yaml:"provider"`
	ClientID     string   `yaml:"client_id"`
	ClientSecret string   `yaml:"client_secret"`
	RedirectURL  string   `yaml:"redirect_url"` // Optional: if not set, auto-constructed from server.base_url
	FrontendURL  string   `yaml:"frontend_url"`
	Scopes       []string `yaml:"scopes"`
}

// GetRedirectURL returns the OIDC callback URL
// If RedirectURL is explicitly configured, use it
// Otherwise, construct from server base_url + hardcoded callback path
func (a *Auth) GetRedirectURL(serverBaseURL string) string {
	if a.RedirectURL != "" {
		return a.RedirectURL
	}
	return serverBaseURL + "/api/auth/callback"
}

// Load loads config from file.
func Load(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, err
	}

	// Set default server base URL if not configured
	if cfg.Server.BaseURL == "" {
		cfg.Server.BaseURL = "http://localhost:52538"
	}

	// Override server config from env vars if present
	if baseURL := os.Getenv("SERVER_BASE_URL"); baseURL != "" {
		cfg.Server.BaseURL = baseURL
	}

	// Override auth config from env vars if present
	if secret := os.Getenv("OIDC_CLIENT_SECRET"); secret != "" {
		cfg.Auth.ClientSecret = secret
	}
	if provider := os.Getenv("OIDC_PROVIDER"); provider != "" {
		cfg.Auth.Provider = provider
	}
	if redirectURL := os.Getenv("OIDC_REDIRECT_URL"); redirectURL != "" {
		cfg.Auth.RedirectURL = redirectURL
	}
	if frontendURL := os.Getenv("OIDC_FRONTEND_URL"); frontendURL != "" {
		cfg.Auth.FrontendURL = frontendURL
	}

	return &cfg, nil
}
