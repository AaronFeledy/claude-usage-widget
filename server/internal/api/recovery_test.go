package api

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func Test_Recovery_does_not_write_duplicate_header_when_panic_follows_response(t *testing.T) {
	// Given
	handler := loggingMiddleware(noopLogger{}, recoveryMiddleware(noopLogger{}, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
		panic("hidden panic")
	})))
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/panic-after-write", nil)

	// When
	handler.ServeHTTP(rec, req)

	// Then
	if rec.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want 204", rec.Code)
	}
	if rec.Body.String() != "" {
		t.Fatalf("body = %q, want empty", rec.Body.String())
	}
}

type noopLogger struct{}

func (noopLogger) InfoContext(context.Context, string, ...any)  {}
func (noopLogger) ErrorContext(context.Context, string, ...any) {}
