package claude

import (
	"context"
	"net/http"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func Test_Live_Fetch_uses_temp_copy_of_real_credentials(t *testing.T) {
	if os.Getenv("CLAUDE_PROVIDER_LIVE") != "1" {
		t.Skip("set CLAUDE_PROVIDER_LIVE=1 to run guarded live test")
	}
	// Given
	home, err := os.UserHomeDir()
	if err != nil {
		t.Fatalf("home dir: %v", err)
	}
	original := filepath.Join(home, ".claude", ".credentials.json")
	data, err := os.ReadFile(original)
	if err != nil {
		t.Fatalf("read original credentials metadata: %v", err)
	}
	copyPath := filepath.Join(t.TempDir(), ".credentials.json")
	if err := os.WriteFile(copyPath, data, 0o600); err != nil {
		t.Fatalf("write temp credentials copy: %v", err)
	}
	client := New(Options{CredentialsPath: copyPath, HTTPClient: &httpClientWithTimeout})

	// When
	got, err := client.Fetch(context.Background())

	// Then
	if err != nil {
		t.Fatalf("live fetch returned error: %v", err)
	}
	if got.Current.Utilization < 0 || got.Current.Utilization > 100 || got.Weekly.Utilization < 0 || got.Weekly.Utilization > 100 {
		t.Fatalf("utilization out of range: current=%.1f weekly=%.1f", got.Current.Utilization, got.Weekly.Utilization)
	}
	if got.ProviderName != "Claude" {
		t.Fatalf("live status provider=%q", got.ProviderName)
	}
	t.Logf("live status provider=%q current_utilization_in_range=%v weekly_utilization_in_range=%v current_reset_present=%v weekly_reset_present=%v", got.ProviderName, true, true, got.Current.ResetsAt != nil, got.Weekly.ResetsAt != nil)
}

var httpClientWithTimeout = http.Client{Timeout: 15 * time.Second}
