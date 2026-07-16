package grok

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptrace"
	"net/url"
	"strings"
	"sync/atomic"
	"time"
)

const (
	refreshMaxAttempts  = 3
	refreshMaxBodyBytes = 1 << 20
	refreshRetryDelay   = 25 * time.Millisecond
)

func (p *Provider) requestRefresh(ctx context.Context, current credentials) (credentials, error) {
	var lastErr error
	for attempt := 0; attempt < refreshMaxAttempts; attempt++ {
		refreshed, retry, err := p.requestRefreshAttempt(ctx, current)
		if err == nil {
			return refreshed, nil
		}
		lastErr = err
		if !retry || attempt == refreshMaxAttempts-1 {
			return credentials{}, err
		}
		timer := time.NewTimer(time.Duration(attempt+1) * refreshRetryDelay)
		select {
		case <-ctx.Done():
			if !timer.Stop() {
				select {
				case <-timer.C:
				default:
				}
			}
			return credentials{}, fmt.Errorf("wait to retry Grok token refresh: %w", ctx.Err())
		case <-timer.C:
		}
	}
	return credentials{}, lastErr
}

func (p *Provider) requestRefreshAttempt(ctx context.Context, current credentials) (credentials, bool, error) {
	form := url.Values{}
	form.Set("grant_type", "refresh_token")
	form.Set("refresh_token", current.refreshToken)
	form.Set("client_id", defaultString(current.clientID, oauthClientID))
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, p.tokenURL, strings.NewReader(form.Encode()))
	if err != nil {
		return credentials{}, false, fmt.Errorf("create Grok refresh request: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	standardTransport := p.httpClient.Transport == nil
	if _, ok := p.httpClient.Transport.(*http.Transport); ok {
		standardTransport = true
	}
	var wroteRequest atomic.Bool
	if standardTransport {
		req = req.WithContext(httptrace.WithClientTrace(req.Context(), &httptrace.ClientTrace{
			WroteHeaders: func() {
				wroteRequest.Store(true)
			},
			WroteRequest: func(httptrace.WroteRequestInfo) {
				wroteRequest.Store(true)
			},
		}))
	}
	resp, err := p.httpClient.Do(req)
	if err != nil {
		if resp != nil && resp.Body != nil {
			err = errors.Join(err, resp.Body.Close())
		}
		if ctx.Err() != nil {
			return credentials{}, false, fmt.Errorf("refresh Grok token: %w", ctx.Err())
		}
		return credentials{}, standardTransport && !wroteRequest.Load(), fmt.Errorf("refresh Grok token: %w", err)
	}
	bodyBytes, readErr := io.ReadAll(io.LimitReader(resp.Body, refreshMaxBodyBytes+1))
	closeErr := resp.Body.Close()
	if readErr != nil {
		if ctx.Err() != nil {
			return credentials{}, false, fmt.Errorf("read Grok refresh response: %w", errors.Join(ctx.Err(), readErr, closeErr))
		}
		return credentials{}, false, fmt.Errorf("read Grok refresh response: %w", errors.Join(readErr, closeErr))
	}
	if len(bodyBytes) > refreshMaxBodyBytes {
		return credentials{}, isRetryableRefreshStatus(resp.StatusCode), fmt.Errorf("Grok refresh response exceeds %d bytes: %w", refreshMaxBodyBytes, errRefreshFailed)
	}
	var body map[string]json.RawMessage
	decodeErr := json.Unmarshal(bodyBytes, &body)
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		if decodeErr != nil {
			return credentials{}, isRetryableRefreshStatus(resp.StatusCode), fmt.Errorf("decode Grok refresh response: %w", decodeErr)
		}
		if resp.StatusCode == http.StatusBadRequest || resp.StatusCode == http.StatusUnauthorized {
			switch oauthErrorCode(body) {
			case "invalid_grant", "invalid_client":
				return credentials{}, false, fmt.Errorf("Grok refresh rejected credentials: %w", errInvalidGrant)
			}
		}
		return credentials{}, isRetryableRefreshStatus(resp.StatusCode), fmt.Errorf("Grok refresh returned %d: %w", resp.StatusCode, errRefreshFailed)
	}
	if decodeErr != nil {
		return credentials{}, false, fmt.Errorf("decode Grok refresh response: %w", decodeErr)
	}
	refreshed, err := refreshedCredentials(body, current, p.now())
	return refreshed, false, err
}

func oauthErrorCode(body map[string]json.RawMessage) string {
	return strings.ToLower(strings.TrimSpace(jsonString(body["error"])))
}

func isRetryableRefreshStatus(status int) bool {
	return status == http.StatusBadRequest || status == http.StatusUnauthorized || status == http.StatusTooManyRequests
}

func refreshedCredentials(body map[string]json.RawMessage, current credentials, now time.Time) (credentials, error) {
	accessToken := strings.TrimSpace(jsonString(body["access_token"]))
	if accessToken == "" {
		return credentials{}, errRefreshFailed
	}
	expiresAt, err := refreshExpiresAt(body, now)
	if err != nil {
		return credentials{}, err
	}
	refreshToken := strings.TrimSpace(jsonString(body["refresh_token"]))
	if refreshToken == "" {
		refreshToken = current.refreshToken
	}
	return credentials{
		accessToken:  accessToken,
		refreshToken: refreshToken,
		userID:       current.userID,
		clientID:     defaultString(current.clientID, oauthClientID),
		expiresAt:    expiresAt,
		createTime:   now.UTC(),
	}, nil
}

func refreshExpiresAt(body map[string]json.RawMessage, now time.Time) (time.Time, error) {
	if raw := body["expires_in"]; len(raw) > 0 {
		var seconds int64
		if err := json.Unmarshal(raw, &seconds); err == nil {
			return now.Add(time.Duration(seconds) * time.Second).UTC(), nil
		}
	}
	if raw := body["expires_at"]; len(raw) > 0 {
		return parseExpiresAt(raw)
	}
	return time.Time{}, nil
}
