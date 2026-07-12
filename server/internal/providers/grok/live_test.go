package grok

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func Test_Provider_live_temp_copy_QA_when_enabled(t *testing.T) {
	if os.Getenv("GROK_LIVE_TEST") != "1" {
		t.Skip("set GROK_LIVE_TEST=1 to run live Grok QA against a temp credential copy")
	}
	// Given
	home, err := os.UserHomeDir()
	if err != nil {
		t.Fatalf("home dir: %v", err)
	}
	original := filepath.Join(home, ".grok", "auth.json")
	data, err := os.ReadFile(original)
	if err != nil {
		t.Fatalf("read original Grok auth: %v", err)
	}
	copyPath := filepath.Join(t.TempDir(), "auth.json")
	if err := os.WriteFile(copyPath, data, 0o600); err != nil {
		t.Fatalf("write temp Grok auth copy: %v", err)
	}
	provider, err := NewProvider(Options{CredentialsPath: copyPath})
	if err != nil {
		t.Fatalf("NewProvider: %v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	// When
	usage, err := provider.Fetch(ctx)

	// Then
	if err != nil {
		t.Fatalf("Fetch live temp copy: %v", err)
	}
	if usage.Error != nil {
		t.Fatalf("live Grok usage error: %s", *usage.Error)
	}
	t.Logf("LIVE_GROK_STATUS: provider=%s current=%.0f secondary_hidden=%v reauth=%v", usage.ProviderName, usage.Current.Utilization, !usage.ShowSecondary, usage.NeedsReauth)
	if usage.Current.ResetsAt != nil {
		t.Logf("LIVE_GROK_RESET_RANGE: current_reset_utc=%s", usage.Current.ResetsAt.UTC().Format(time.RFC3339))
	}
}
