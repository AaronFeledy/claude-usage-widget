package server_test

import (
	"context"
	"log/slog"
	"net"
	"net/http"
	"testing"
	"time"

	"github.com/AaronFeledy/claude-usage-widget/server/internal/server"
)

func Test_Run_serves_injected_handler(t *testing.T) {
	// Given
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	ready := make(chan net.Addr, 1)
	done := make(chan error, 1)
	handler := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})
	go func() {
		done <- server.Run(ctx, server.RunOptions{Listener: listener, Handler: handler, Logger: slog.Default(), Ready: ready})
	}()
	addr := <-ready

	// When
	client := &http.Client{Timeout: time.Second}
	resp, err := client.Get("http://" + addr.String() + "/api/v1/health")

	// Then
	if err != nil {
		t.Fatalf("GET injected handler: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("status = %d, want 204", resp.StatusCode)
	}
	cancel()
	<-done
}

func Test_Run_exits_after_context_cancel(t *testing.T) {
	// Given
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	ready := make(chan net.Addr, 1)
	done := make(chan error, 1)
	go func() {
		done <- server.Run(ctx, server.RunOptions{Listener: listener, Logger: slog.Default(), Ready: ready})
	}()
	select {
	case <-ready:
	case <-time.After(time.Second):
		t.Fatal("server did not signal ready")
	}

	// When
	cancel()

	// Then
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("Run returned error: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("server did not stop after cancellation")
	}
}
