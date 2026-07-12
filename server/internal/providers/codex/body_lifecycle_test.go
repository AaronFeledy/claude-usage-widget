package codex

import (
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"sync"
	"testing"
)

func Test_Client_returns_context_when_closing_unauthorized_body_fails_before_retry(t *testing.T) {
	// Given
	path := writeAuth(t, `{"tokens":{"access_token":"old","refresh_token":"refresh"},"last_refresh":"2026-07-12T00:00:00Z"}`)
	firstBody := &trackedBody{closeErr: errors.New("close boom")}
	finalBody := &trackedBody{payload: `{"rate_limit":{"primary_window":{"used_percent":1},"secondary_window":{"used_percent":2}}}`}
	transport := &codexLifecycleTransport{firstUsageBody: firstBody, finalUsageBody: finalBody}
	provider := New(Options{CredentialsPath: path, UsageURL: "https://codex.test/usage", TokenURL: "https://codex.test/token", HTTPClient: &http.Client{Transport: transport}})

	// When
	_, err := provider.Fetch(context.Background())

	// Then
	if err == nil || !strings.Contains(err.Error(), "close codex unauthorized response body") || !strings.Contains(err.Error(), "close boom") {
		t.Fatalf("Fetch err = %v, want wrapped unauthorized close error", err)
	}
	if firstBody.closeCount() != 1 {
		t.Fatalf("first body closes = %d, want exactly 1", firstBody.closeCount())
	}
	if transport.refreshes != 0 || transport.usages != 1 {
		t.Fatalf("refreshes=%d usages=%d, want no retry after close failure", transport.refreshes, transport.usages)
	}
}

func Test_Client_closes_unauthorized_and_final_bodies_once_when_retry_succeeds(t *testing.T) {
	// Given
	path := writeAuth(t, `{"tokens":{"access_token":"old","refresh_token":"refresh"},"last_refresh":"2026-07-12T00:00:00Z"}`)
	firstBody := &trackedBody{}
	finalBody := &trackedBody{payload: `{"rate_limit":{"primary_window":{"used_percent":1},"secondary_window":{"used_percent":2}}}`}
	transport := &codexLifecycleTransport{firstUsageBody: firstBody, finalUsageBody: finalBody}
	provider := New(Options{CredentialsPath: path, UsageURL: "https://codex.test/usage", TokenURL: "https://codex.test/token", HTTPClient: &http.Client{Transport: transport}})

	// When
	data, err := provider.Fetch(context.Background())

	// Then
	if err != nil || data.Error != nil {
		t.Fatalf("Fetch data=%+v err=%v", data, err)
	}
	if firstBody.closeCount() != 1 || finalBody.closeCount() != 1 {
		t.Fatalf("body closes first=%d final=%d, want exactly 1 each", firstBody.closeCount(), finalBody.closeCount())
	}
	if transport.refreshes != 1 || transport.usages != 2 {
		t.Fatalf("refreshes=%d usages=%d, want one refresh and one retry", transport.refreshes, transport.usages)
	}
}

type codexLifecycleTransport struct {
	firstUsageBody *trackedBody
	finalUsageBody *trackedBody
	usages         int
	refreshes      int
}

func (t *codexLifecycleTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	if req.URL.Path == "/token" {
		t.refreshes++
		return responseWithBody(http.StatusOK, &trackedBody{payload: `{"access_token":"new","refresh_token":"new-refresh","expires_in":3600}`}), nil
	}
	t.usages++
	if t.usages == 1 {
		return responseWithBody(http.StatusUnauthorized, t.firstUsageBody), nil
	}
	return responseWithBody(http.StatusOK, t.finalUsageBody), nil
}

type trackedBody struct {
	mu       sync.Mutex
	payload  string
	closed   int
	read     bool
	closeErr error
}

func (b *trackedBody) Read(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.read || b.payload == "" {
		return 0, io.EOF
	}
	b.read = true
	return copy(p, b.payload), io.EOF
}

func (b *trackedBody) Close() error {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.closed++
	return b.closeErr
}

func (b *trackedBody) closeCount() int {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.closed
}

func responseWithBody(status int, body io.ReadCloser) *http.Response {
	return &http.Response{StatusCode: status, Body: body, Header: make(http.Header)}
}
