package api_test

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/AaronFeledy/claude-usage-widget/server/internal/api"
)

func Test_GrokCredentials_rejects_invalid_requests(t *testing.T) {
	tests := []struct {
		name   string
		method string
		body   string
		status int
	}{
		{"method", http.MethodPost, `{}`, http.StatusMethodNotAllowed},
		{"malformed", http.MethodPut, `{`, http.StatusBadRequest},
		{"unknown field", http.MethodPut, `{"cookie":"x","extra":true}`, http.StatusBadRequest},
		{"empty", http.MethodPut, `{}`, http.StatusBadRequest},
		{"access token", http.MethodPut, `{"access_token":"token"}`, http.StatusBadRequest},
		{"both", http.MethodPut, `{"cookie":"x","access_token":"token"}`, http.StatusBadRequest},
		{"empty cookie", http.MethodPut, `{"cookie":"   "}`, http.StatusBadRequest},
		{"oversized", http.MethodPut, `{"cookie":"` + strings.Repeat("x", 1<<20) + `"}`, http.StatusBadRequest},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cache := newFakeCache(entry("Grok", 10, 20, nil))
			handler := api.NewHandler(api.Options{Cache: cache, Grok: &fakeGrok{}, Poller: cache, ProviderNames: []string{"Grok"}})
			recorder := httptest.NewRecorder()
			request := httptest.NewRequest(tt.method, "/api/v1/providers/grok/credentials", strings.NewReader(tt.body))

			handler.ServeHTTP(recorder, request)

			assertStatus(t, recorder, tt.status)
		})
	}
}

func Test_GrokCredentials_requires_auth_and_reports_refetch_failure(t *testing.T) {
	t.Run("authentication", func(t *testing.T) {
		cache := newFakeCache(entry("Grok", 10, 20, nil))
		grok := &fakeGrok{}
		handler := api.NewHandler(api.Options{Cache: cache, Grok: grok, Poller: cache, AuthToken: "secret", ProviderNames: []string{"Grok"}})
		recorder := httptest.NewRecorder()
		request := httptest.NewRequest(http.MethodPut, "/api/v1/providers/grok/credentials", strings.NewReader(`{"cookie":"sso=session"}`))

		handler.ServeHTTP(recorder, request)

		assertStatus(t, recorder, http.StatusUnauthorized)
		if grok.cookie != "" || cache.refetches != 0 {
			t.Fatalf("unauthenticated request changed credentials or refetched: cookie = %q refetches = %d", grok.cookie, cache.refetches)
		}
	})

	t.Run("refetch failure", func(t *testing.T) {
		cache := newFakeCacheWithError()
		handler := api.NewHandler(api.Options{Cache: cache, Grok: &fakeGrok{}, Poller: cache, ProviderNames: []string{"Grok"}})
		recorder := httptest.NewRecorder()
		request := httptest.NewRequest(http.MethodPut, "/api/v1/providers/grok/credentials", strings.NewReader(`{"cookie":"sso=session"}`))

		handler.ServeHTTP(recorder, request)

		assertStatus(t, recorder, http.StatusBadGateway)
	})
}

func Test_CredentialRoutes_serialize_cursor_and_grok_updates(t *testing.T) {
	transaction := newBlockingCredentialTransaction()
	handler := api.NewHandler(api.Options{
		Cache:         transaction,
		Cursor:        transaction,
		Grok:          transaction,
		Poller:        transaction,
		ProviderNames: []string{"Cursor", "Grok"},
	})
	firstStatus := make(chan int, 1)
	go func() {
		recorder := httptest.NewRecorder()
		request := httptest.NewRequest(http.MethodPut, "/api/v1/providers/cursor/credentials", strings.NewReader(`{"cookie":"first"}`))
		handler.ServeHTTP(recorder, request)
		firstStatus <- recorder.Code
	}()
	<-transaction.firstPollStarted

	secondStatus := make(chan int, 1)
	go func() {
		recorder := httptest.NewRecorder()
		request := httptest.NewRequest(http.MethodPut, "/api/v1/providers/grok/credentials", strings.NewReader(`{"cookie":"second"}`))
		handler.ServeHTTP(recorder, request)
		secondStatus <- recorder.Code
	}()

	select {
	case <-transaction.secondSetterCalled:
		t.Fatal("Grok credential setter ran before Cursor refetch completed")
	case <-transaction.readyToReleaseFirst:
	}
	close(transaction.releaseFirstPoll)
	if status := <-firstStatus; status != http.StatusOK {
		t.Fatalf("Cursor credential status = %d, want %d", status, http.StatusOK)
	}
	if status := <-secondStatus; status != http.StatusOK {
		t.Fatalf("Grok credential status = %d, want %d", status, http.StatusOK)
	}
	transaction.assertOrder(t)
}
