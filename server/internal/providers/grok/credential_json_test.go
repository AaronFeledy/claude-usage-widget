package grok

import (
	"encoding/json"
	"testing"
	"time"
)

func Test_updateCredentialJSON_writes_expires_at_as_rfc3339_string_for_grok_cli_compat(t *testing.T) {
	// Given: grok CLI writes expires_at as an RFC3339 string; the server must
	// preserve that type on refresh or the grok CLI rejects auth.json as corrupt.
	const entryKey = "https://auth.x.ai/oauth2/token"
	original := []byte(`{
		"https://auth.x.ai/oauth2/token": {
			"key": "old-token",
			"refresh_token": "old-refresh",
			"expires_at": "2026-07-13T11:35:22.481004194Z",
			"create_time": "2026-07-13T05:35:22.481004194Z"
		}
	}`)
	expires := time.Date(2026, 7, 13, 17, 35, 22, 481004194, time.UTC)

	// When
	updated, err := updateCredentialJSON(original, credentials{
		accessToken:  "new-token",
		refreshToken: "new-refresh",
		expiresAt:    expires,
		entryKey:     entryKey,
	})
	if err != nil {
		t.Fatalf("updateCredentialJSON returned error: %v", err)
	}

	// Then: expires_at is a JSON string (not a bare number) so the grok CLI can parse it.
	var root map[string]map[string]json.RawMessage
	if err := json.Unmarshal(updated, &root); err != nil {
		t.Fatalf("unmarshal updated credentials: %v", err)
	}
	raw := root[entryKey]["expires_at"]
	var asString string
	if err := json.Unmarshal(raw, &asString); err != nil {
		t.Fatalf("expires_at is not a JSON string (got %s): %v", raw, err)
	}
	if want := "2026-07-13T17:35:22.481004194Z"; asString != want {
		t.Fatalf("expires_at = %q, want %q", asString, want)
	}

	// And: the server can still read what it wrote back.
	reparsed, err := parseCredentials(updated)
	if err != nil {
		t.Fatalf("parseCredentials round-trip: %v", err)
	}
	if !reparsed.expiresAt.Equal(expires) {
		t.Fatalf("round-trip expiresAt = %v, want %v", reparsed.expiresAt, expires)
	}
	if reparsed.accessToken != "new-token" || reparsed.refreshToken != "new-refresh" {
		t.Fatalf("round-trip tokens = %q/%q", reparsed.accessToken, reparsed.refreshToken)
	}

	// And: create_time is preserved byte-for-byte (type and value), proving no unrelated fields changed.
	createTime, err := stringField(root[entryKey]["create_time"])
	if err != nil {
		t.Fatalf("create_time no longer a string: %v", err)
	}
	if want := "2026-07-13T05:35:22.481004194Z"; createTime != want {
		t.Fatalf("create_time = %q, want %q", createTime, want)
	}
}

func stringField(raw json.RawMessage) (string, error) {
	var value string
	err := json.Unmarshal(raw, &value)
	return value, err
}
