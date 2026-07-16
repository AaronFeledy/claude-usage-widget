package grok

import (
	"encoding/json"
	"fmt"
	"time"
)

func updateCredentialJSON(data []byte, creds credentials) ([]byte, error) {
	var root map[string]map[string]json.RawMessage
	if err := json.Unmarshal(data, &root); err != nil {
		return nil, fmt.Errorf("parse Grok auth for update: %w", err)
	}
	entry, ok := root[creds.entryKey]
	if !ok {
		return nil, errMissingAuthEntry
	}
	entry["key"] = jsonStringRaw(creds.accessToken)
	if _, ok := entry["refresh_token"]; ok {
		entry["refresh_token"] = jsonStringRaw(creds.refreshToken)
	} else if _, ok := entry["refresh"]; ok {
		entry["refresh"] = jsonStringRaw(creds.refreshToken)
	} else {
		entry["refresh_token"] = jsonStringRaw(creds.refreshToken)
	}
	if !creds.expiresAt.IsZero() {
		entry["expires_at"] = jsonStringRaw(creds.expiresAt.UTC().Format(time.RFC3339Nano))
	} else {
		delete(entry, "expires_at")
	}
	if !creds.createTime.IsZero() {
		entry["create_time"] = jsonStringRaw(creds.createTime.UTC().Format(time.RFC3339Nano))
	}
	if creds.clientID != "" {
		entry["oidc_client_id"] = jsonStringRaw(creds.clientID)
	}
	updated, err := json.MarshalIndent(root, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("encode Grok auth update: %w", err)
	}
	return updated, nil
}

func jsonString(raw json.RawMessage) string {
	var value string
	if err := json.Unmarshal(raw, &value); err != nil {
		return ""
	}
	return value
}

func firstString(values ...json.RawMessage) string {
	for _, raw := range values {
		if value := jsonString(raw); value != "" {
			return value
		}
	}
	return ""
}

func jsonStringRaw(value string) json.RawMessage {
	encoded, err := json.Marshal(value)
	if err != nil {
		return nil
	}
	return encoded
}
