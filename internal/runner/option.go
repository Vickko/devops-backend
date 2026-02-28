package runner

import "time"

// Image pull policy constants.
const (
	PullIfNotPresent = "if-not-present"
	PullAlways       = "always"
	PullNever        = "never"
)

// Config holds the configuration for a Docker container runner.
type Config struct {
	MemoryLimit     int64
	NanoCPUs        int64
	PidsLimit       int64
	TaskTimeout     time.Duration
	ImagePullPolicy string
	WorkDir         string
	NetworkMode     string // empty = Docker default (bridge); set "none" to isolate
	MaxOutputBytes  int    // cap on stdout/stderr per exec; 0 = use default
	CapDrop         []string
	SecurityOpt     []string
}

// Option is a functional option for configuring the runner.
type Option func(*Config)

func defaultConfig() *Config {
	return &Config{
		MemoryLimit:     512 * 1024 * 1024, // 512 MB
		NanoCPUs:        500_000_000,        // 0.5 core
		PidsLimit:       512,
		TaskTimeout:     60 * time.Second,
		ImagePullPolicy: PullIfNotPresent,
		WorkDir:         "/workspace",
		MaxOutputBytes:  16 << 20, // 16 MB
	}
}

func WithMemoryLimit(limit int64) Option {
	return func(c *Config) { c.MemoryLimit = limit }
}

func WithNanoCPUs(n int64) Option {
	return func(c *Config) { c.NanoCPUs = n }
}

func WithPidsLimit(limit int64) Option {
	return func(c *Config) { c.PidsLimit = limit }
}

func WithTaskTimeout(d time.Duration) Option {
	return func(c *Config) { c.TaskTimeout = d }
}

func WithImagePullPolicy(policy string) Option {
	return func(c *Config) { c.ImagePullPolicy = policy }
}

func WithWorkDir(dir string) Option {
	return func(c *Config) { c.WorkDir = dir }
}

func WithCapDrop(caps []string) Option {
	return func(c *Config) { c.CapDrop = caps }
}

func WithSecurityOpt(opts []string) Option {
	return func(c *Config) { c.SecurityOpt = opts }
}

func WithNetworkMode(mode string) Option {
	return func(c *Config) { c.NetworkMode = mode }
}

func WithMaxOutputBytes(n int) Option {
	return func(c *Config) { c.MaxOutputBytes = n }
}
