package claude

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func Test_Client_Fetch_returns_usage_error_for_upstream_429_and_500(t *testing.T) {
	tests := []struct {
		name       string
		statusCode int
		body       string
		want       string
	}{
		{name: "rate limited", statusCode: http.StatusTooManyRequests, body: "slow down", want: "Rate limited"},
		{name: "server error", statusCode: http.StatusInternalServerError, body: "broken", want: "API error"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Given
			credentialsPath := writeClaudeCredentials(t, credentialFixture{Access: "access", Refresh: "refresh", ExpiresAt: futureMillis(t)})
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(tt.statusCode)
				writeResponse(t, w, tt.body)
			}))
			defer server.Close()
			client := New(Options{CredentialsPath: credentialsPath, HTTPClient: server.Client(), UsageURL: server.URL, TokenURL: server.URL + "/token"})

			// When
			got, err := client.Fetch(context.Background())

			// Then
			if err != nil {
				t.Fatalf("Fetch returned error: %v", err)
			}
			if got.Error == nil || !strings.Contains(*got.Error, tt.want) {
				t.Fatalf("error = %v, want containing %q", got.Error, tt.want)
			}
		})
	}
}

func Test_Client_Fetch_returns_timeout_error_when_http_hangs(t *testing.T) {
	// Given
	credentialsPath := writeClaudeCredentials(t, credentialFixture{Access: "access", Refresh: "refresh", ExpiresAt: futureMillis(t)})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		<-r.Context().Done()
	}))
	defer server.Close()
	client := New(Options{CredentialsPath: credentialsPath, HTTPClient: &http.Client{Timeout: 20 * time.Millisecond}, UsageURL: server.URL, TokenURL: server.URL + "/token"})

	// When
	got, err := client.Fetch(context.Background())

	// Then
	if err != nil {
		t.Fatalf("Fetch returned error: %v", err)
	}
	if got.Error == nil || !strings.Contains(*got.Error, "timed out") {
		t.Fatalf("error = %v, want timeout", got.Error)
	}
}
