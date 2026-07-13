package codex

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func Test_Client_allows_only_one_refresh_in_flight_when_parallel_fetches_get_401(t *testing.T) {
	// Given
	path := writeAuth(t, `{"tokens":{"access_token":"old","refresh_token":"refresh"},"last_refresh":"2026-07-12T00:00:00Z"}`)
	var refreshCalls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/token" {
			refreshCalls.Add(1)
			time.Sleep(25 * time.Millisecond)
			_, _ = w.Write([]byte(`{"access_token":"new","refresh_token":"new-refresh","expires_in":3600}`))
			return
		}
		if r.Header.Get("Authorization") == "Bearer old" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		_, _ = w.Write([]byte(`{"rate_limit":{"primary_window":{"used_percent":1},"secondary_window":{"used_percent":2}}}`))
	}))
	defer server.Close()
	provider := New(Options{CredentialsPath: path, UsageURL: server.URL + "/usage", TokenURL: server.URL + "/token", HTTPClient: server.Client()})

	// When
	var wg sync.WaitGroup
	errs := make(chan error, 2)
	for range 2 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			data, err := provider.Fetch(context.Background())
			if err != nil {
				errs <- err
				return
			}
			if data.Error != nil {
				errs <- errors.New(*data.Error)
			}
		}()
	}
	wg.Wait()
	close(errs)

	// Then
	for err := range errs {
		if err != nil {
			t.Fatalf("parallel Fetch error: %v", err)
		}
	}
	if refreshCalls.Load() != 1 {
		t.Fatalf("refreshCalls = %d, want 1", refreshCalls.Load())
	}
}

func Test_Client_returns_context_error_when_HTTP_timeout_expires(t *testing.T) {
	// Given
	path := writeAuth(t, `{"tokens":{"access_token":"access","refresh_token":"refresh"},"last_refresh":"2026-07-12T00:00:00Z"}`)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(100 * time.Millisecond)
		_, _ = w.Write([]byte(`{}`))
	}))
	defer server.Close()
	client := server.Client()
	client.Timeout = 10 * time.Millisecond

	// When
	_, err := New(Options{CredentialsPath: path, UsageURL: server.URL, HTTPClient: client}).Fetch(context.Background())

	// Then
	if err == nil {
		t.Fatal("Fetch err = nil, want timeout error")
	}
}
