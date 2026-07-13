package claude

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
)

type credentialFixture struct {
	Access       string
	Refresh      string
	ExpiresAt    int64
	Subscription string
	Extra        string
}

func writeClaudeCredentials(t *testing.T, fixture credentialFixture) string {
	t.Helper()
	return writeClaudeCredentialsIn(t, filepath.Join(t.TempDir(), ".credentials.json"), fixture)
}

func writeClaudeCredentialsAt(t *testing.T, path string, fixture credentialFixture) {
	t.Helper()
	writeFile(t, path, []byte(claudeCredentialsJSON(fixture)))
}

func writeClaudeCredentialsIn(t *testing.T, path string, fixture credentialFixture) string {
	t.Helper()
	writeFile(t, path, []byte(claudeCredentialsJSON(fixture)))
	return path
}

func writeOpenCodeCredentials(t *testing.T, fixture credentialFixture) string {
	t.Helper()
	return writeOpenCodeCredentialsIn(t, filepath.Join(t.TempDir(), "auth.json"), fixture)
}

func writeOpenCodeCredentialsIn(t *testing.T, path string, fixture credentialFixture) string {
	t.Helper()
	writeFile(t, path, []byte(openCodeCredentialsJSON(fixture)))
	return path
}

func claudeCredentialsJSON(fixture credentialFixture) string {
	subscription := ""
	if fixture.Subscription != "" {
		subscription = `,"subscriptionType":"` + fixture.Subscription + `"`
	}
	return `{"claudeAiOauth":{"accessToken":"` + fixture.Access + `","refreshToken":"` + fixture.Refresh + `","expiresAt":` + intString(fixture.ExpiresAt) + subscription + fixture.Extra + `}}`
}

func openCodeCredentialsJSON(fixture credentialFixture) string {
	return `{"anthropic":{"access":"` + fixture.Access + `","refresh":"` + fixture.Refresh + `","expires":` + intString(fixture.ExpiresAt) + fixture.Extra + `}}`
}

func writeFile(t *testing.T, path string, data []byte) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatalf("mkdir fixture dir: %v", err)
	}
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatalf("write fixture: %v", err)
	}
}

func readFile(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read file: %v", err)
	}
	return string(data)
}

func compactJSON(t *testing.T, raw string) string {
	t.Helper()
	var value json.RawMessage
	if err := json.Unmarshal([]byte(raw), &value); err != nil {
		t.Fatalf("compact json unmarshal: %v", err)
	}
	encoded, err := json.Marshal(value)
	if err != nil {
		t.Fatalf("compact json marshal: %v", err)
	}
	return string(encoded)
}

func assertRefreshRequest(t *testing.T, r *http.Request, refreshToken string) {
	t.Helper()
	if r.Method != http.MethodPost {
		t.Fatalf("refresh method = %s", r.Method)
	}
	if got := r.Header.Get("Content-Type"); !strings.HasPrefix(got, "application/x-www-form-urlencoded") {
		t.Fatalf("Content-Type = %q", got)
	}
	if err := r.ParseForm(); err != nil {
		t.Fatalf("parse form: %v", err)
	}
	if r.Form.Get("grant_type") != "refresh_token" || r.Form.Get("refresh_token") != refreshToken || r.Form.Get("client_id") != oauthClientID {
		t.Fatalf("refresh form = %v", r.Form)
	}
}

func writeResponse(t *testing.T, w http.ResponseWriter, body string) {
	t.Helper()
	if _, err := w.Write([]byte(body)); err != nil {
		t.Fatalf("write response: %v", err)
	}
}

func futureMillis(t *testing.T) int64 {
	t.Helper()
	return time.Now().Add(time.Hour).UnixMilli()
}

func pastMillis(t *testing.T) int64 {
	t.Helper()
	return time.Now().Add(-time.Hour).UnixMilli()
}

func intString(value int64) string {
	return strconv.FormatInt(value, 10)
}
