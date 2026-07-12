package codex

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

func parseCredentials(data []byte, path string, source CredentialSource) (Credentials, error) {
	var root map[string]json.RawMessage
	if err := json.Unmarshal(data, &root); err != nil {
		return Credentials{}, fmt.Errorf("parse codex credentials: %w", ErrCredentialsMalformed)
	}
	if source == CredentialSourceOpenCode {
		return parseOpenCodeCredentials(root, path)
	}
	return parseCodexCredentials(root, path)
}

func parseCodexCredentials(root map[string]json.RawMessage, path string) (Credentials, error) {
	if apiKey := stringField(root["OPENAI_API_KEY"]); apiKey != "" {
		return Credentials{AccessToken: apiKey, Source: CredentialSourceCodex, Path: path}, nil
	}
	var tokens struct {
		AccessToken  string `json:"access_token"`
		RefreshToken string `json:"refresh_token"`
		AccountID    string `json:"account_id"`
	}
	if err := json.Unmarshal(root["tokens"], &tokens); err != nil || strings.TrimSpace(tokens.AccessToken) == "" {
		return Credentials{}, fmt.Errorf("parse codex token credentials: %w", ErrCredentialsMalformed)
	}
	lastRefresh := parseRFC3339Field(root["last_refresh"])
	return Credentials{AccessToken: tokens.AccessToken, RefreshToken: tokens.RefreshToken, AccountID: tokens.AccountID, Source: CredentialSourceCodex, Path: path, LastRefresh: lastRefresh}, nil
}

func parseOpenCodeCredentials(root map[string]json.RawMessage, path string) (Credentials, error) {
	var doc struct {
		OpenAI struct {
			Access    string `json:"access"`
			Refresh   string `json:"refresh"`
			AccountID string `json:"accountId"`
			Expires   *int64 `json:"expires"`
		} `json:"openai"`
	}
	if err := json.Unmarshal(mustMarshalRoot(root), &doc); err != nil || strings.TrimSpace(doc.OpenAI.Access) == "" {
		return Credentials{}, fmt.Errorf("parse opencode credentials: %w", ErrCredentialsMalformed)
	}
	var expiresAt *time.Time
	if doc.OpenAI.Expires != nil {
		parsed := time.UnixMilli(*doc.OpenAI.Expires).UTC()
		expiresAt = &parsed
	}
	return Credentials{AccessToken: doc.OpenAI.Access, RefreshToken: doc.OpenAI.Refresh, AccountID: doc.OpenAI.AccountID, Source: CredentialSourceOpenCode, Path: path, ExpiresAt: expiresAt}, nil
}

func mutateCredentials(data []byte, source CredentialSource, tokens RefreshedTokens) ([]byte, error) {
	var root map[string]json.RawMessage
	if err := json.Unmarshal(data, &root); err != nil {
		return nil, fmt.Errorf("decode current credentials: %w", ErrCredentialsMalformed)
	}
	if source == CredentialSourceOpenCode {
		return mutateOpenCode(root, tokens)
	}
	return mutateCodex(root, tokens)
}

func mutateCodex(root map[string]json.RawMessage, tokens RefreshedTokens) ([]byte, error) {
	tokenFields := map[string]json.RawMessage{}
	if raw := root["tokens"]; len(raw) != 0 {
		if err := json.Unmarshal(raw, &tokenFields); err != nil {
			return nil, fmt.Errorf("decode codex tokens: %w", ErrCredentialsMalformed)
		}
	}
	tokenFields["access_token"] = marshalRaw(tokens.AccessToken)
	if strings.TrimSpace(tokens.RefreshToken) != "" {
		tokenFields["refresh_token"] = marshalRaw(tokens.RefreshToken)
	}
	if strings.TrimSpace(tokens.AccountID) != "" {
		tokenFields["account_id"] = marshalRaw(tokens.AccountID)
	}
	root["tokens"] = mustMarshalRaw(tokenFields)
	root["last_refresh"] = marshalRaw(tokens.RefreshedAt.UTC().Format(time.RFC3339))
	return json.Marshal(root)
}

func mutateOpenCode(root map[string]json.RawMessage, tokens RefreshedTokens) ([]byte, error) {
	openAI := map[string]json.RawMessage{}
	if raw := root["openai"]; len(raw) != 0 {
		if err := json.Unmarshal(raw, &openAI); err != nil {
			return nil, fmt.Errorf("decode opencode tokens: %w", ErrCredentialsMalformed)
		}
	}
	openAI["access"] = marshalRaw(tokens.AccessToken)
	if strings.TrimSpace(tokens.RefreshToken) != "" {
		openAI["refresh"] = marshalRaw(tokens.RefreshToken)
	}
	if strings.TrimSpace(tokens.AccountID) != "" {
		openAI["accountId"] = marshalRaw(tokens.AccountID)
	}
	if !tokens.ExpiresAt.IsZero() {
		openAI["expires"] = marshalRaw(tokens.ExpiresAt.UnixMilli())
	}
	root["openai"] = mustMarshalRaw(openAI)
	return json.Marshal(root)
}

func stringField(raw json.RawMessage) string {
	var value string
	if err := json.Unmarshal(raw, &value); err != nil {
		return ""
	}
	return strings.TrimSpace(value)
}

func parseRFC3339Field(raw json.RawMessage) *time.Time {
	value := stringField(raw)
	if value == "" {
		return nil
	}
	parsed, err := time.Parse(time.RFC3339, value)
	if err != nil {
		return nil
	}
	utc := parsed.UTC()
	return &utc
}

func marshalRaw[T string | int64](value T) json.RawMessage {
	data, err := json.Marshal(value)
	if err != nil {
		return nil
	}
	return data
}

func mustMarshalRaw(value map[string]json.RawMessage) json.RawMessage {
	data, err := json.Marshal(value)
	if err != nil {
		return nil
	}
	return data
}

func mustMarshalRoot(value map[string]json.RawMessage) []byte {
	data, err := json.Marshal(value)
	if err != nil {
		return nil
	}
	return data
}
