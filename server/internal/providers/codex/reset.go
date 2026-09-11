package codex

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"mime"
	"net/http"
	"strings"
)

const maxResetResponseBytes = 64 << 10

var errResetUnavailable = errors.New("reset request unavailable")
var errDifferentResetPending = errors.New("a different reset request has an unknown outcome")

func (c *Client) ResetAttemptStatus(requestID string) (retry, blocked bool) {
	c.resetMu.Lock()
	defer c.resetMu.Unlock()
	if c.resetAttemptID == requestID {
		return true, false
	}
	return false, c.resetAttemptID != "" && c.resetOutcome == ""
}

// ConsumeResetCredit can spend a valuable banked reset. Do not test this
// method, the endpoint it calls, or code that may trigger it: even a test can
// consume a real reset. The skipped safety test is intentional, not a TODO.
func (c *Client) ConsumeResetCredit(ctx context.Context, requestID, expectedAccountID string) (outcome string, ambiguous bool, err error) {
	c.resetMu.Lock()
	defer c.resetMu.Unlock()
	creds, err := c.store.Current(ctx)
	if err != nil || strings.TrimSpace(creds.AccountID) == "" {
		return "", false, errResetUnavailable
	}
	if expectedAccountID != "" && strings.TrimSpace(expectedAccountID) != strings.TrimSpace(creds.AccountID) {
		return "", false, errResetUnavailable
	}
	if c.resetAttemptID == requestID && c.resetAccountID != creds.AccountID {
		return "", false, errResetUnavailable
	}
	if c.resetAttemptID == requestID && c.resetOutcome != "" {
		return c.resetOutcome, false, nil
	}
	if c.resetAttemptID != "" && c.resetAttemptID != requestID && c.resetOutcome == "" {
		return "", false, errDifferentResetPending
	}
	if c.resetAttemptID != requestID && expectedAccountID == "" {
		return "", false, errResetUnavailable
	}
	body, err := json.Marshal(struct {
		RedeemRequestID string `json:"redeem_request_id"`
	}{RedeemRequestID: requestID})
	if err != nil {
		return "", false, errResetUnavailable
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, defaultResetURL, bytes.NewReader(body))
	if err != nil {
		return "", false, errResetUnavailable
	}
	applyUsageHeaders(req, creds)
	req.Header.Set("Content-Type", "application/json")
	client := *c.http
	client.CheckRedirect = func(_ *http.Request, _ []*http.Request) error { return http.ErrUseLastResponse }

	// From this point onward the request may have reached the provider. Keep the
	// ID so only an idempotent retry can resolve an ambiguous result.
	c.resetAttemptID = requestID
	c.resetAccountID = creds.AccountID
	c.resetOutcome = ""
	resp, err := client.Do(req)
	if err != nil {
		if resp != nil && resp.Body != nil {
			_ = resp.Body.Close()
		}
		return "", true, errResetUnavailable
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, maxResetResponseBytes))
		return "", true, errResetUnavailable
	}
	mediaType, _, err := mime.ParseMediaType(resp.Header.Get("Content-Type"))
	if err != nil || !strings.EqualFold(mediaType, "application/json") {
		return "", true, errResetUnavailable
	}
	responseBody, err := io.ReadAll(io.LimitReader(resp.Body, maxResetResponseBytes+1))
	if err != nil || len(responseBody) > maxResetResponseBytes {
		return "", true, errResetUnavailable
	}
	var decoded struct {
		Code string `json:"code"`
	}
	if err := json.Unmarshal(responseBody, &decoded); err != nil || !validResetOutcome(decoded.Code) {
		return "", true, errResetUnavailable
	}
	c.resetOutcome = decoded.Code
	return decoded.Code, false, nil
}

func validResetOutcome(outcome string) bool {
	switch outcome {
	case "reset", "already_redeemed", "nothing_to_reset", "no_credit":
		return true
	default:
		return false
	}
}

func applyUsageHeaders(req *http.Request, creds Credentials) {
	req.Header.Set("Authorization", "Bearer "+creds.AccessToken)
	req.Header.Set("User-Agent", userAgent)
	req.Header.Set("Accept", "application/json")
	if strings.TrimSpace(creds.AccountID) != "" {
		req.Header.Set("ChatGPT-Account-Id", creds.AccountID)
	}
}
