package config

import (
	"errors"
	"time"
)

var (
	ErrInvalidConfig   = errors.New("config: invalid")
	ErrMalformedConfig = errors.New("config: malformed")
)

type Config struct {
	ListenAddr   string
	AuthToken    string
	PollInterval time.Duration
	Providers    map[string]ProviderConfig
}

type ProviderConfig struct {
	Enabled         bool
	CredentialsPath string
}

type LoadOptions struct {
	Args []string
	Env  []string
}

func Defaults() Config {
	return Config{
		ListenAddr:   "127.0.0.1:7823",
		PollInterval: 60 * time.Second,
		Providers: map[string]ProviderConfig{
			"claude": {Enabled: true, CredentialsPath: "~/.claude/.credentials.json"},
			"codex":  {Enabled: true},
			"cursor": {Enabled: true},
			"grok":   {Enabled: true},
		},
	}
}
