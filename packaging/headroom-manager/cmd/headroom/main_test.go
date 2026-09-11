package main

import (
	"bytes"
	"context"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestServeHelpBypassesManagedRegistration(t *testing.T) {
	root := t.TempDir()
	t.Setenv("HEADROOM_INSTALL_ROOT", root)
	t.Setenv("SYSTEMD_EXEC_PID", "1")
	var output bytes.Buffer
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	err := managedServe(context.Background(), []string{"--help"}, []string{"HEADROOM_INSTALL_ROOT=" + root, "SYSTEMD_EXEC_PID=1"}, logger, "test", strings.NewReader(""), &output)
	if err != nil {
		t.Fatal(err)
	}
	if _, statErr := os.Stat(filepath.Join(root, "runtime", "managed-serve.json")); !os.IsNotExist(statErr) {
		t.Fatalf("serve help created a managed receipt: %v", statErr)
	}
}
