package grok

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/AaronFeledy/claude-usage-widget/server/internal/credstore"
)

type credentials struct {
	accessToken  string
	refreshToken string
	expiresAt    time.Time
	entryKey     string
}

type credentialStore struct {
	mu       sync.RWMutex
	refresh  sync.Mutex
	snapshot credstore.Snapshot
	creds    credentials
	loaded   bool
}

type refreshReason int

const (
	refreshReasonProactive refreshReason = iota
	refreshReasonUnauthorized
)

func (c credentials) needsRefresh(now time.Time) bool {
	if c.refreshToken == "" || c.expiresAt.IsZero() {
		return false
	}
	return !now.Before(c.expiresAt.Add(-refreshBuffer))
}

func (p *Provider) credentials(ctx context.Context) (credentials, error) {
	p.store.mu.RLock()
	if p.store.loaded {
		creds := p.store.creds
		snapshot := p.store.snapshot
		p.store.mu.RUnlock()
		info, err := os.Stat(p.credentialsPath)
		if err != nil {
			return credentials{}, fmt.Errorf("stat Grok auth: %w", err)
		}
		state := credstore.RefreshState{Snapshot: snapshot, CurrentModTime: info.ModTime(), ExpiresAt: creds.expiresAt.Add(-refreshBuffer)}
		if credstore.ShouldRefresh(p.now(), state) != credstore.RefreshDecisionReload {
			return creds, nil
		}
	} else {
		p.store.mu.RUnlock()
	}
	return p.loadCredentials(ctx)
}

func (p *Provider) loadCredentials(ctx context.Context) (credentials, error) {
	p.store.mu.Lock()
	defer p.store.mu.Unlock()
	snapshot, err := credstore.LoadSnapshot(ctx, p.credentialsPath)
	if err != nil {
		return credentials{}, fmt.Errorf("load Grok auth: %w", err)
	}
	creds, err := parseCredentials(snapshot.Data)
	if err != nil {
		return credentials{}, err
	}
	p.store.snapshot = snapshot
	p.store.creds = creds
	p.store.loaded = true
	return creds, nil
}

func parseCredentials(data []byte) (credentials, error) {
	var root map[string]json.RawMessage
	if err := json.Unmarshal(data, &root); err != nil {
		return credentials{}, fmt.Errorf("parse Grok auth: %w", err)
	}
	for entryKey, raw := range root {
		if !strings.Contains(strings.ToLower(entryKey), "auth.x.ai") {
			continue
		}
		creds, err := parseCredentialEntry(entryKey, raw)
		if err != nil {
			return credentials{}, err
		}
		return creds, nil
	}
	return credentials{}, errMissingAuthEntry
}

func parseCredentialEntry(entryKey string, raw json.RawMessage) (credentials, error) {
	var entry map[string]json.RawMessage
	if err := json.Unmarshal(raw, &entry); err != nil {
		return credentials{}, fmt.Errorf("parse Grok auth entry: %w", err)
	}
	accessToken := jsonString(entry["key"])
	if strings.TrimSpace(accessToken) == "" {
		return credentials{}, errMissingAuthEntry
	}
	expiresAt, err := parseExpiresAt(entry["expires_at"])
	if err != nil {
		return credentials{}, err
	}
	return credentials{
		accessToken:  strings.TrimSpace(accessToken),
		refreshToken: strings.TrimSpace(firstString(entry["refresh_token"], entry["refresh"])),
		expiresAt:    expiresAt,
		entryKey:     entryKey,
	}, nil
}

func parseExpiresAt(raw json.RawMessage) (time.Time, error) {
	if len(raw) == 0 || bytes.Equal(raw, []byte("null")) {
		return time.Time{}, nil
	}
	var millis int64
	if err := json.Unmarshal(raw, &millis); err == nil {
		return time.UnixMilli(millis).UTC(), nil
	}
	value := jsonString(raw)
	if value == "" {
		return time.Time{}, nil
	}
	parsed, err := time.Parse(time.RFC3339, value)
	if err != nil {
		return time.Time{}, fmt.Errorf("parse Grok auth expires_at: %w", err)
	}
	return parsed.UTC(), nil
}

func (p *Provider) refresh(ctx context.Context, stale credentials, reason refreshReason) (credentials, error) {
	p.store.refresh.Lock()
	defer p.store.refresh.Unlock()
	creds, err := p.credentials(ctx)
	if err != nil {
		return credentials{}, err
	}
	if creds.accessToken != stale.accessToken || creds.refreshToken != stale.refreshToken {
		return creds, nil
	}
	if reason == refreshReasonProactive && !creds.needsRefresh(p.now()) {
		return creds, nil
	}
	if creds.refreshToken == "" {
		return credentials{}, errInvalidGrant
	}
	refreshed, err := p.requestRefresh(ctx, creds.refreshToken, creds.expiresAt)
	if err != nil {
		return credentials{}, err
	}
	refreshed.entryKey = creds.entryKey
	if err := p.saveCredentials(ctx, refreshed); err != nil {
		return credentials{}, err
	}
	return p.loadCredentials(ctx)
}

func (p *Provider) saveCredentials(ctx context.Context, creds credentials) error {
	return credstore.AtomicUpdate(ctx, p.credentialsPath, func(data []byte) ([]byte, error) {
		return updateCredentialJSON(data, creds)
	})
}
