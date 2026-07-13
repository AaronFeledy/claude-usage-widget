package codex

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/AaronFeledy/claude-usage-widget/server/internal/usage"
)

func Test_Client_implements_usage_provider_and_maps_wham_response(t *testing.T) {
	// Given
	path := writeAuth(t, `{"tokens":{"access_token":"access","refresh_token":"refresh","account_id":"acct"},"last_refresh":"2026-07-12T00:00:00Z"}`)
	var sawRequest bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		sawRequest = true
		assertWhamRequest(t, r, "access", "acct")
		_, _ = w.Write([]byte(`{"plan_type":"pro","rate_limit":{"primary_window":{"used_percent":12.5,"reset_at":1783872000},"secondary_window":{"used_percent":80,"reset_at":1784476800}}}`))
	}))
	defer server.Close()
	provider := New(Options{CredentialsPath: path, UsageURL: server.URL, HTTPClient: server.Client()})

	// When
	var typed usage.Provider = provider
	data, err := typed.Fetch(context.Background())

	// Then
	if err != nil {
		t.Fatalf("Fetch returned error: %v", err)
	}
	if typed.Name() != "Codex" || data.ProviderName != "Codex" || data.PrimaryLabel != "5-Hour" || data.SecondaryLabel != "Weekly" || data.ReauthCommand == nil || *data.ReauthCommand != "codex" {
		t.Fatalf("usage contract = %+v name=%q", data, typed.Name())
	}
	if !sawRequest || data.Subtitle == nil || *data.Subtitle != "pro" || data.Current.Utilization != 12.5 || data.Weekly.Utilization != 80 {
		t.Fatalf("mapped usage = %+v sawRequest=%v", data, sawRequest)
	}
	if data.Current.ResetsAt == nil || !data.Current.ResetsAt.Equal(time.Unix(1783872000, 0).UTC()) {
		t.Fatalf("Current.ResetsAt = %v", data.Current.ResetsAt)
	}
}

func Test_Client_refreshes_once_on_401_and_retries_with_new_token(t *testing.T) {
	// Given
	path := writeAuth(t, `{"tokens":{"access_token":"old","refresh_token":"refresh","account_id":"acct"},"last_refresh":"2026-07-12T00:00:00Z"}`)
	var usageCalls atomic.Int32
	refreshCalls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/usage":
			call := usageCalls.Add(1)
			if call == 1 {
				w.WriteHeader(http.StatusUnauthorized)
				return
			}
			assertWhamRequest(t, r, "new-access", "acct")
			_, _ = w.Write([]byte(`{"rate_limit":{"primary_window":{"used_percent":1},"secondary_window":{"used_percent":2}}}`))
		case "/token":
			refreshCalls++
			assertRefreshRequest(t, r, "refresh")
			_, _ = w.Write([]byte(`{"access_token":"new-access","refresh_token":"new-refresh","expires_in":3600}`))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()
	provider := New(Options{CredentialsPath: path, UsageURL: server.URL + "/usage", TokenURL: server.URL + "/token", HTTPClient: server.Client()})

	// When
	data, err := provider.Fetch(context.Background())

	// Then
	if err != nil || data.Error != nil {
		t.Fatalf("Fetch data=%+v err=%v", data, err)
	}
	if usageCalls.Load() != 2 || refreshCalls != 1 {
		t.Fatalf("usageCalls=%d refreshCalls=%d, want 2 and 1", usageCalls.Load(), refreshCalls)
	}
	if got := string(readFile(t, path)); !strings.Contains(got, `"access_token":"new-access"`) || !strings.Contains(got, `"refresh_token":"new-refresh"`) {
		t.Fatalf("auth file not updated: %s", got)
	}
}

func Test_Client_returns_reauth_usage_when_refresh_token_reused_or_invalidated(t *testing.T) {
	tests := []struct {
		name string
		body string
	}{
		{name: "reused", body: `{"error":"refresh_token_reused"}`},
		{name: "invalidated", body: `{"error":"refresh_token_invalidated"}`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Given
			path := writeAuth(t, `{"tokens":{"access_token":"old","refresh_token":"refresh"},"last_refresh":"2026-07-12T00:00:00Z"}`)
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path == "/usage" {
					w.WriteHeader(http.StatusForbidden)
					return
				}
				w.WriteHeader(http.StatusUnauthorized)
				_, _ = w.Write([]byte(tt.body))
			}))
			defer server.Close()
			provider := New(Options{CredentialsPath: path, UsageURL: server.URL + "/usage", TokenURL: server.URL + "/token", HTTPClient: server.Client()})

			// When
			data, err := provider.Fetch(context.Background())

			// Then
			if err != nil {
				t.Fatalf("Fetch returned error: %v", err)
			}
			if data.Error == nil || *data.Error != "AUTH_EXPIRED" || !data.NeedsReauth || data.ReauthCommand == nil || *data.ReauthCommand != "codex" {
				t.Fatalf("data = %+v, want reauth", data)
			}
		})
	}
}

func Test_Client_handles_api_key_missing_account_rate_limit_server_error_and_canceled_context(t *testing.T) {
	tests := []struct {
		name       string
		auth       string
		statusCode int
		cancel     bool
		wantError  string
	}{
		{name: "api key skips refresh", auth: `{"OPENAI_API_KEY":"sk-test"}`, statusCode: http.StatusOK},
		{name: "missing account id omits header", auth: `{"tokens":{"access_token":"access","refresh_token":"refresh"},"last_refresh":"2026-07-12T00:00:00Z"}`, statusCode: http.StatusOK},
		{name: "rate limited", auth: `{"tokens":{"access_token":"access","refresh_token":"refresh"},"last_refresh":"2026-07-12T00:00:00Z"}`, statusCode: http.StatusTooManyRequests, wantError: "API error (429)"},
		{name: "server error", auth: `{"tokens":{"access_token":"access","refresh_token":"refresh"},"last_refresh":"2026-07-12T00:00:00Z"}`, statusCode: http.StatusInternalServerError, wantError: "API error (500)"},
		{name: "canceled context", auth: `{"tokens":{"access_token":"access","refresh_token":"refresh"},"last_refresh":"2026-07-12T00:00:00Z"}`, statusCode: http.StatusOK, cancel: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Given
			path := writeAuth(t, tt.auth)
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if got := r.Header.Get("ChatGPT-Account-Id"); got != "" {
					t.Fatalf("ChatGPT-Account-Id = %q, want omitted", got)
				}
				w.WriteHeader(tt.statusCode)
				_, _ = w.Write([]byte(`{"rate_limit":{"primary_window":{"used_percent":1},"secondary_window":{"used_percent":2}}}`))
			}))
			defer server.Close()
			ctx, cancel := context.WithCancel(context.Background())
			if tt.cancel {
				cancel()
			} else {
				defer cancel()
			}

			// When
			data, err := New(Options{CredentialsPath: path, UsageURL: server.URL, HTTPClient: server.Client()}).Fetch(ctx)

			// Then
			if tt.cancel {
				if !errors.Is(err, context.Canceled) {
					t.Fatalf("Fetch err = %v, want context canceled", err)
				}
				return
			}
			if tt.wantError == "" && (err != nil || data.Error != nil) {
				t.Fatalf("Fetch data=%+v err=%v", data, err)
			}
			if tt.wantError != "" && (data.Error == nil || *data.Error != tt.wantError) {
				t.Fatalf("data.Error = %v, want %q", data.Error, tt.wantError)
			}
		})
	}
}

func Test_Client_reloads_credentials_instead_of_refreshing_when_mtime_changes_before_401_refresh(t *testing.T) {
	// Given
	path := writeAuth(t, `{"tokens":{"access_token":"old","refresh_token":"old-refresh"},"last_refresh":"2026-07-12T00:00:00Z"}`)
	var usageCalls atomic.Int32
	var refreshCalls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/token" {
			refreshCalls.Add(1)
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		call := usageCalls.Add(1)
		if call == 1 {
			writeFile(t, path, `{"tokens":{"access_token":"external","refresh_token":"external-refresh"}}`)
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		if r.Header.Get("Authorization") != "Bearer external" {
			t.Fatalf("Authorization = %q, want external", r.Header.Get("Authorization"))
		}
		_, _ = w.Write([]byte(`{"rate_limit":{"primary_window":{"used_percent":3},"secondary_window":{"used_percent":4}}}`))
	}))
	defer server.Close()

	// When
	data, err := New(Options{CredentialsPath: path, UsageURL: server.URL + "/usage", TokenURL: server.URL + "/token", HTTPClient: server.Client()}).Fetch(context.Background())

	// Then
	if err != nil || data.Error != nil {
		t.Fatalf("Fetch data=%+v err=%v", data, err)
	}
	if refreshCalls.Load() != 0 || usageCalls.Load() != 2 {
		t.Fatalf("refreshCalls=%d usageCalls=%d, want 0 and 2", refreshCalls.Load(), usageCalls.Load())
	}
}

func Test_Live_CodexProvider_temp_copy_QA(t *testing.T) {
	if os.Getenv("CODEX_LIVE_TEST") != "1" {
		t.Skip("set CODEX_LIVE_TEST=1 to run live Codex QA")
	}
	// Given
	home, err := os.UserHomeDir()
	if err != nil {
		t.Fatalf("home dir: %v", err)
	}
	source := filepath.Join(home, ".codex", "auth.json")
	bytes, err := os.ReadFile(source)
	if err != nil {
		t.Fatalf("read live codex auth: %v", err)
	}
	copyPath := filepath.Join(t.TempDir(), "auth.json")
	if err := os.WriteFile(copyPath, bytes, 0o600); err != nil {
		t.Fatalf("write temp auth copy: %v", err)
	}

	// When
	data, err := New(Options{CredentialsPath: copyPath}).Fetch(context.Background())

	// Then
	if err != nil {
		t.Fatalf("Fetch returned error: %v", err)
	}
	if data.ProviderName != "Codex" || data.Error != nil {
		t.Fatalf("live data provider=%q error=%v", data.ProviderName, data.Error)
	}
	if data.Current.Utilization < 0 || data.Current.Utilization > 100 || data.Weekly.Utilization < 0 || data.Weekly.Utilization > 100 {
		t.Fatalf("live utilization out of range: %+v", data)
	}
	if data.Subtitle != nil && *data.Subtitle == "" {
		t.Fatal("subtitle supplied but empty")
	}
}

func assertWhamRequest(t *testing.T, r *http.Request, token string, accountID string) {
	t.Helper()
	if r.Method != http.MethodGet {
		t.Fatalf("method = %s, want GET", r.Method)
	}
	if got := r.Header.Get("Authorization"); got != "Bearer "+token {
		t.Fatalf("Authorization = %q", got)
	}
	if r.Header.Get("Accept") != "application/json" || r.Header.Get("User-Agent") != "ClaudeUsageWidget" {
		t.Fatalf("headers = %#v", r.Header)
	}
	if accountID != "" && r.Header.Get("ChatGPT-Account-Id") != accountID {
		t.Fatalf("ChatGPT-Account-Id = %q", r.Header.Get("ChatGPT-Account-Id"))
	}
}

func assertRefreshRequest(t *testing.T, r *http.Request, refreshToken string) {
	t.Helper()
	if r.Method != http.MethodPost || r.Header.Get("Content-Type") != "application/json" {
		t.Fatalf("refresh request method/header = %s %#v", r.Method, r.Header)
	}
	var body struct {
		ClientID     string `json:"client_id"`
		GrantType    string `json:"grant_type"`
		RefreshToken string `json:"refresh_token"`
		Scope        string `json:"scope"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		t.Fatalf("decode refresh request: %v", err)
	}
	if body.ClientID != "app_EMoamEEZ73f0CkXaXp7hrann" || body.GrantType != "refresh_token" || body.RefreshToken != refreshToken || body.Scope != "openid profile email" {
		t.Fatalf("refresh body = %+v", body)
	}
}
