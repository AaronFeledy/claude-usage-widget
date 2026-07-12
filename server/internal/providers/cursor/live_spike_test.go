package cursor

import (
	"context"
	"encoding/json"
	"net/http"
	"os"
	"testing"
	"time"
)

func Test_LiveSpike_auth_token_cookie_candidates(t *testing.T) {
	if os.Getenv("CURSOR_LIVE_SPIKE") != "1" {
		t.Skip("set CURSOR_LIVE_SPIKE=1 to test local Cursor auth against live endpoints")
	}
	// Given
	accessToken := readLiveAccessToken(t)
	workosCookie, err := cookieFromAccessToken(accessToken)
	if err != nil {
		t.Fatalf("build WorkOS candidate: %v", err)
	}
	candidates := []spikeCandidate{
		{name: "workos_sub_access", cookieHeader: workosCookie},
		{name: "bare_access", cookieHeader: "WorkosCursorSessionToken=" + accessToken},
	}
	runner := spikeRunner{client: &http.Client{Timeout: 10 * time.Second}}

	// When
	results := make([]spikeResult, 0, len(candidates)*2)
	for _, candidate := range candidates {
		results = append(results, runner.run(t, candidate, usageSummaryPath))
		results = append(results, runner.run(t, candidate, authMePath))
	}

	// Then
	goCandidate := false
	for _, result := range results {
		t.Logf("candidate=%s endpoint=%s status=%d recognized_schema=%v", result.candidate, result.endpoint, result.status, result.recognizedSchema)
		if result.candidate == "workos_sub_access" && result.status == http.StatusOK && result.recognizedSchema {
			goCandidate = true
		}
	}
	if !goCandidate {
		t.Fatal("no Cursor auth-token cookie candidate returned HTTP 200 with recognized JSON")
	}
}

type spikeCandidate struct {
	name         string
	cookieHeader string
}

type spikeResult struct {
	candidate        string
	endpoint         string
	status           int
	recognizedSchema bool
}

type spikeRunner struct {
	client *http.Client
}

func readLiveAccessToken(t *testing.T) string {
	t.Helper()
	data, err := os.ReadFile(defaultAuthPath())
	if err != nil {
		t.Fatalf("read local Cursor auth: %v", err)
	}
	var auth localAuthFile
	if err := json.Unmarshal(data, &auth); err != nil {
		t.Fatalf("parse local Cursor auth: %v", err)
	}
	if auth.AccessToken == "" {
		t.Fatal("local Cursor auth has no accessToken")
	}
	return auth.AccessToken
}

func (r spikeRunner) run(t *testing.T, candidate spikeCandidate, endpoint string) spikeResult {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, defaultBaseURL+endpoint, nil)
	if err != nil {
		t.Fatalf("build spike request: %v", err)
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Cookie", candidate.cookieHeader)
	resp, err := r.client.Do(req)
	if err != nil {
		return spikeResult{candidate: candidate.name, endpoint: endpoint, status: 0}
	}
	defer resp.Body.Close()
	return spikeResult{
		candidate:        candidate.name,
		endpoint:         endpoint,
		status:           resp.StatusCode,
		recognizedSchema: recognizeSpikeSchema(resp, endpoint),
	}
}

func recognizeSpikeSchema(resp *http.Response, endpoint string) bool {
	if resp.StatusCode != http.StatusOK {
		return false
	}
	if endpoint == usageSummaryPath {
		var summary cursorUsageSummary
		return json.NewDecoder(resp.Body).Decode(&summary) == nil && summary.IndividualUsage != nil
	}
	var userInfo cursorUserInfo
	return json.NewDecoder(resp.Body).Decode(&userInfo) == nil && userInfo.Sub != ""
}
