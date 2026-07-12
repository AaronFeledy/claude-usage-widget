package main

import (
	"bytes"
	"log/slog"
	"testing"
)

func Test_run_returns_nil_when_help_requested(t *testing.T) {
	// Given
	var logs bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&logs, nil))

	// When
	err := run([]string{"--help"}, nil, logger)

	// Then
	if err != nil {
		t.Fatalf("run returned error: %v", err)
	}
	if bytes.Contains(logs.Bytes(), []byte("ERROR")) {
		t.Fatalf("logs contain ERROR: %s", logs.String())
	}
}
