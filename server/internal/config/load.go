package config

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

type rawConfig struct {
	ListenAddr   *string                      `yaml:"listen_addr"`
	AuthToken    *string                      `yaml:"auth_token"`
	PollInterval *string                      `yaml:"poll_interval"`
	Providers    map[string]rawProviderConfig `yaml:"providers"`
}

type rawProviderConfig struct {
	Enabled         *bool   `yaml:"enabled"`
	CredentialsPath *string `yaml:"credentials_path"`
}

type flagValues struct {
	ConfigPath   string
	ListenAddr   string
	AuthToken    string
	PollInterval string
	Set          map[string]bool
}

func Load(ctx context.Context, opts LoadOptions) (Config, error) {
	select {
	case <-ctx.Done():
		return Config{}, fmt.Errorf("load config canceled: %w", ctx.Err())
	default:
	}

	flags, err := parseFlags(opts.Args)
	if err != nil {
		return Config{}, err
	}
	env := parseEnv(opts.Env)
	path, err := resolveConfigPath(flags, opts.Env, env)
	if err != nil {
		return Config{}, err
	}

	cfg := Defaults()
	if err := applyYAML(path, &cfg); err != nil {
		return Config{}, err
	}
	if err := applyEnv(env, &cfg); err != nil {
		return Config{}, err
	}
	if err := applyFlags(flags, &cfg); err != nil {
		return Config{}, err
	}
	return cfg, nil
}

func parseFlags(args []string) (flagValues, error) {
	values := flagValues{Set: map[string]bool{}}
	fs := flag.NewFlagSet("usage-server", flag.ContinueOnError)
	fs.StringVar(&values.ConfigPath, "config", "", "path to config.yaml")
	fs.StringVar(&values.ListenAddr, "listen-addr", "", "address for the HTTP server")
	fs.StringVar(&values.AuthToken, "auth-token", "", "optional bearer token")
	fs.StringVar(&values.PollInterval, "poll-interval", "", "provider poll interval")
	if err := fs.Parse(args); err != nil {
		return flagValues{}, fmt.Errorf("parse flags: %w", err)
	}
	fs.Visit(func(f *flag.Flag) { values.Set[f.Name] = true })
	return values, nil
}

func resolveConfigPath(flags flagValues, envList []string, env map[string]string) (string, error) {
	if flags.Set["config"] {
		return flags.ConfigPath, nil
	}
	if path := env["USAGE_CONFIG"]; path != "" {
		return path, nil
	}
	path, err := defaultPath(envList)
	if err != nil {
		return "", err
	}
	return path, nil
}

func applyYAML(path string, cfg *Config) error {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("read config %s: %w", path, err)
	}
	var raw rawConfig
	if err := yaml.Unmarshal(data, &raw); err != nil {
		return fmt.Errorf("parse config %s: %w", path, errors.Join(ErrMalformedConfig, err))
	}
	return applyRaw(raw, cfg)
}

func applyRaw(raw rawConfig, cfg *Config) error {
	if raw.ListenAddr != nil {
		cfg.ListenAddr = *raw.ListenAddr
	}
	if raw.AuthToken != nil {
		cfg.AuthToken = *raw.AuthToken
	}
	if raw.PollInterval != nil {
		duration, err := parseDuration("poll_interval", *raw.PollInterval)
		if err != nil {
			return err
		}
		cfg.PollInterval = duration
	}
	for name, provider := range raw.Providers {
		applyProvider(strings.ToLower(name), provider, cfg)
	}
	return nil
}

func applyProvider(name string, raw rawProviderConfig, cfg *Config) {
	current := cfg.Providers[name]
	if raw.Enabled != nil {
		current.Enabled = *raw.Enabled
	}
	if raw.CredentialsPath != nil {
		current.CredentialsPath = *raw.CredentialsPath
	}
	cfg.Providers[name] = current
}

func applyEnv(env map[string]string, cfg *Config) error {
	if value := env["USAGE_LISTEN_ADDR"]; value != "" {
		cfg.ListenAddr = value
	}
	if value := env["USAGE_AUTH_TOKEN"]; value != "" {
		cfg.AuthToken = value
	}
	if value := env["USAGE_POLL_INTERVAL"]; value != "" {
		duration, err := parseDuration("USAGE_POLL_INTERVAL", value)
		if err != nil {
			return err
		}
		cfg.PollInterval = duration
	}
	return applyProviderEnv(env, cfg)
}

func applyProviderEnv(env map[string]string, cfg *Config) error {
	for key, value := range env {
		if strings.HasPrefix(key, "USAGE_PROVIDER_") && strings.HasSuffix(key, "_ENABLED") {
			name := providerNameFromEnv(key, "USAGE_PROVIDER_", "_ENABLED")
			enabled, err := strconv.ParseBool(value)
			if err != nil {
				return fmt.Errorf("%s %q: %w", key, value, errors.Join(ErrInvalidConfig, err))
			}
			current := cfg.Providers[name]
			current.Enabled = enabled
			cfg.Providers[name] = current
		}
		if strings.HasPrefix(key, "USAGE_PROVIDER_") && strings.HasSuffix(key, "_CREDENTIALS_PATH") {
			name := providerNameFromEnv(key, "USAGE_PROVIDER_", "_CREDENTIALS_PATH")
			current := cfg.Providers[name]
			current.CredentialsPath = value
			cfg.Providers[name] = current
		}
	}
	return nil
}

func applyFlags(flags flagValues, cfg *Config) error {
	if flags.Set["listen-addr"] {
		cfg.ListenAddr = flags.ListenAddr
	}
	if flags.Set["auth-token"] {
		cfg.AuthToken = flags.AuthToken
	}
	if flags.Set["poll-interval"] {
		duration, err := parseDuration("poll-interval", flags.PollInterval)
		if err != nil {
			return err
		}
		cfg.PollInterval = duration
	}
	return nil
}

func parseDuration(name string, value string) (time.Duration, error) {
	duration, err := time.ParseDuration(value)
	if err != nil {
		return 0, fmt.Errorf("%s %q: %w", name, value, errors.Join(ErrInvalidConfig, err))
	}
	return duration, nil
}

func parseEnv(env []string) map[string]string {
	vars := make(map[string]string, len(env))
	for _, entry := range env {
		key, value, found := strings.Cut(entry, "=")
		if found {
			vars[key] = value
		}
	}
	return vars
}

func providerNameFromEnv(key string, prefix string, suffix string) string {
	name := strings.TrimSuffix(strings.TrimPrefix(key, prefix), suffix)
	return strings.ToLower(name)
}
