package main

import (
	"errors"
	"io"
	"log/slog"
	"path/filepath"
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

func discardLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}
