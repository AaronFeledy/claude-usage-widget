package api_test

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/AaronFeledy/claude-usage-widget/server/internal/api"
)

func Test_RequestLogging_redacts_authorization_and_body_when_request_completes(t *testing.T) {
	// Given
	logger := &captureLogger{}
	handler := api.NewHandler(api.Options{Cache: newFakeCache(), Logger: logger, ProviderNames: []string{"Claude"}})
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/health", strings.NewReader("body-secret"))
	req.Header.Set("Authorization", "Bearer token-secret")

	// When
	handler.ServeHTTP(rec, req)

	// Then
	log := logger.joined()
	for _, value := range []string{"body-secret", "token-secret", "Authorization"} {
		if strings.Contains(log, value) {
			t.Fatalf("log leaked %q: %s", value, log)
		}
	}
	for _, value := range []string{"method", "path", "status", "duration"} {
		if !strings.Contains(log, value) {
			t.Fatalf("log missing %q: %s", value, log)
		}
	}
}

type captureLogger struct {
	mu      sync.Mutex
	entries []string
}

func (l *captureLogger) InfoContext(_ context.Context, message string, args ...any) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.entries = append(l.entries, message, fmt.Sprint(args...))
}

func (l *captureLogger) ErrorContext(ctx context.Context, message string, args ...any) {
	l.InfoContext(ctx, message, args...)
}

func (l *captureLogger) joined() string {
	l.mu.Lock()
	defer l.mu.Unlock()
	return strings.Join(l.entries, " ")
}
