package grok

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"
)

func (p *Provider) requestRefresh(ctx context.Context, refreshToken string, fallbackExpires time.Time) (credentials, error) {
	form := url.Values{}
	form.Set("grant_type", "refresh_token")
	form.Set("refresh_token", refreshToken)
	form.Set("client_id", oauthClientID)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, p.tokenURL, strings.NewReader(form.Encode()))
	if err != nil {
		return credentials{}, fmt.Errorf("create Grok refresh request: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, err := p.httpClient.Do(req)
	if err != nil {
		return credentials{}, fmt.Errorf("refresh Grok token: %w", err)
	}
	var body map[string]json.RawMessage
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		if closeErr := drainAndClose(resp); closeErr != nil {
			return credentials{}, closeErr
		}
		return credentials{}, fmt.Errorf("decode Grok refresh response: %w", err)
	}
	if err := drainAndClose(resp); err != nil {
		return credentials{}, err
	}
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		if isInvalidGrantStatus(resp.StatusCode, body) {
			return credentials{}, errInvalidGrant
		}
		return credentials{}, errRefreshFailed
	}
	return refreshedCredentials(body, refreshToken, fallbackExpires, p.now())
}

func isInvalidGrantStatus(status int, body map[string]json.RawMessage) bool {
	if status != http.StatusBadRequest && status != http.StatusUnauthorized {
		return false
	}
	return strings.Contains(strings.ToLower(jsonString(body["error"])), "invalid_grant")
}

func refreshedCredentials(body map[string]json.RawMessage, refreshToken string, fallbackExpires time.Time, now time.Time) (credentials, error) {
	accessToken := strings.TrimSpace(jsonString(body["access_token"]))
	if accessToken == "" {
		return credentials{}, errRefreshFailed
	}
	expiresAt, err := refreshExpiresAt(body, fallbackExpires, now)
	if err != nil {
		return credentials{}, err
	}
	newRefresh := strings.TrimSpace(jsonString(body["refresh_token"]))
	if newRefresh == "" {
		newRefresh = refreshToken
	}
	return credentials{accessToken: accessToken, refreshToken: newRefresh, expiresAt: expiresAt}, nil
}

func refreshExpiresAt(body map[string]json.RawMessage, fallbackExpires time.Time, now time.Time) (time.Time, error) {
	if raw := body["expires_in"]; len(raw) > 0 {
		var seconds int64
		if err := json.Unmarshal(raw, &seconds); err == nil {
			return now.Add(time.Duration(seconds) * time.Second).UTC(), nil
		}
	}
	if raw := body["expires_at"]; len(raw) > 0 {
		return parseExpiresAt(raw)
	}
	return fallbackExpires, nil
}
