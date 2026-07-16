package api_test

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/AaronFeledy/claude-usage-widget/server/internal/api"
)

func Test_Routing_returns_json_405_with_allow_for_known_routes_when_method_wrong(t *testing.T) {
	// Given
	tests := []struct {
		name   string
		method string
		path   string
		allow  string
	}{
		{"collection", http.MethodPost, "/api/v1/usage", http.MethodGet},
		{"provider", http.MethodPost, "/api/v1/usage/claude", http.MethodGet},
		{"health", http.MethodPost, "/api/v1/health", http.MethodGet},
		{"cursor credentials", http.MethodGet, "/api/v1/providers/cursor/credentials", http.MethodPut},
		{"Grok credentials", http.MethodGet, "/api/v1/providers/grok/credentials", http.MethodPut},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			handler := api.NewHandler(api.Options{Cache: newFakeCache(), Cursor: &fakeCursor{}, Grok: &fakeGrok{}, Poller: newFakeCache(), ProviderNames: []string{"Claude", "Cursor", "Grok"}})
			rec := httptest.NewRecorder()
			req := httptest.NewRequest(tt.method, tt.path, nil)

			// When
			handler.ServeHTTP(rec, req)

			// Then
			assertStatus(t, rec, http.StatusMethodNotAllowed)
			assertJSON(t, rec, `{"error":"method not allowed"}`)
			if allow := rec.Header().Get("Allow"); allow != tt.allow {
				t.Fatalf("Allow = %q, want %q", allow, tt.allow)
			}
		})
	}
}

func Test_Routing_returns_json_404_for_unknown_route(t *testing.T) {
	// Given
	handler := api.NewHandler(api.Options{Cache: newFakeCache(), ProviderNames: []string{"Claude"}})
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/not-found", nil)

	// When
	handler.ServeHTTP(rec, req)

	// Then
	assertStatus(t, rec, http.StatusNotFound)
	assertJSON(t, rec, `{"error":"not found"}`)
}

func Test_Auth_rejects_unknown_and_method_mismatch_before_routing_when_token_configured(t *testing.T) {
	// Given
	handler := api.NewHandler(api.Options{Cache: newFakeCache(), AuthToken: "secret", ProviderNames: []string{"Claude"}})
	for _, req := range []*http.Request{
		httptest.NewRequest(http.MethodGet, "/api/v1/not-found", nil),
		httptest.NewRequest(http.MethodPost, "/api/v1/health", strings.NewReader("{}")),
	} {
		rec := httptest.NewRecorder()

		// When
		handler.ServeHTTP(rec, req)

		// Then
		assertStatus(t, rec, http.StatusUnauthorized)
		assertJSON(t, rec, `{"error":"unauthorized"}`)
		if allow := rec.Header().Get("Allow"); allow != "" {
			t.Fatalf("unauthorized response exposed Allow = %q", allow)
		}
	}
}
