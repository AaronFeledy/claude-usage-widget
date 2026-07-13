package main

import (
	"errors"
	"io"
	"log/slog"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/AaronFeledy/claude-usage-widget/server/internal/api"
	"github.com/AaronFeledy/claude-usage-widget/server/internal/config"
)

func Test_Run_rejects_off_loopback_empty_auth_before_provider_construction(t *testing.T) {
	// Given
	args := []string{"-config", filepath.Join(t.TempDir(), "missing.yaml"), "-listen-addr", "0.0.0.0:0", "-auth-token", ""}
	env := []string{"USAGE_PROVIDER_UNKNOWN_ENABLED=true"}

	// When
	err := run(args, env, discardLogger())

	// Then
	if !errors.Is(err, api.ErrUnsafeBind) {
		t.Fatalf("run error = %v, want ErrUnsafeBind", err)
	}
}

func Test_Run_allows_loopback_empty_auth_until_later_startup_error(t *testing.T) {
	// Given
	args := []string{"-config", filepath.Join(t.TempDir(), "missing.yaml"), "-listen-addr", "127.0.0.1:0", "-auth-token", ""}
	env := []string{"USAGE_PROVIDER_UNKNOWN_ENABLED=true"}

	// When
	err := run(args, env, discardLogger())

	// Then
	if !errors.Is(err, config.ErrInvalidConfig) {
		t.Fatalf("run error = %v, want ErrInvalidConfig", err)
	}
}

func Test_Run_allows_authenticated_off_loopback_until_later_startup_error(t *testing.T) {
	// Given
	args := []string{"-config", filepath.Join(t.TempDir(), "missing.yaml"), "-listen-addr", "0.0.0.0:0", "-auth-token", "secret"}
	env := []string{"USAGE_PROVIDER_UNKNOWN_ENABLED=true"}

	// When
	err := run(args, env, discardLogger())

	// Then
	if !errors.Is(err, config.ErrInvalidConfig) {
		t.Fatalf("run error = %v, want ErrInvalidConfig", err)
	}
}

func Test_BuildPoller_allows_cursor_local_discovery_only_for_loopback_listen_addresses(t *testing.T) {
	// Given
	tests := []struct {
		name          string
		listenAddr    string
		wantDiscovery bool
	}{
		{name: "ipv4 loopback", listenAddr: "127.0.0.1:7823", wantDiscovery: true},
		{name: "localhost", listenAddr: "localhost:7823", wantDiscovery: true},
		{name: "ipv6 loopback", listenAddr: "[::1]:7823", wantDiscovery: true},
		{name: "wildcard ipv4", listenAddr: "0.0.0.0:7823"},
		{name: "wildcard ipv6", listenAddr: "[::]:7823"},
		{name: "lan address", listenAddr: "192.168.1.5:7823"},
		{name: "hostname", listenAddr: "example.com:7823"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := config.Defaults()
			cfg.ListenAddr = tt.listenAddr
			cfg.Providers = map[string]config.ProviderConfig{
				"cursor": {Enabled: true, CredentialsPath: filepath.Join(t.TempDir(), "auth.json")},
			}

			// When
			_, cursorClient, _, err := buildPoller(cfg)
			if err != nil {
				t.Fatalf("buildPoller error = %v", err)
			}

			// Then
			if cursorClient == nil {
				t.Fatal("cursor client = nil")
			}
			gotDiscovery := reflect.ValueOf(cursorClient).Elem().FieldByName("allowLocalDiscovery").Bool()
			if gotDiscovery != tt.wantDiscovery {
				t.Fatalf("AllowLocalDiscovery = %v for %q, want %v", gotDiscovery, tt.listenAddr, tt.wantDiscovery)
			}
		})
	}
}

func discardLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}
