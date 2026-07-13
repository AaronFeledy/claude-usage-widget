package cursor

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/AaronFeledy/claude-usage-widget/server/internal/usage"
)

const (
	providerName     = "Cursor"
	defaultBaseURL   = "https://cursor.com"
	usageSummaryPath = "/api/usage-summary"
	authMePath       = "/api/auth/me"
	legacyUsagePath  = "/api/usage"
)

var (
	ErrInvalidToken = errors.New("cursor: invalid access token")
	ErrUnauthorized = errors.New("cursor: unauthorized")
)

type Options struct {
	BaseURL             string
	AuthPath            string
	HTTPClient          *http.Client
	AllowLocalDiscovery bool
}

type Client struct {
	baseURL             string
	authPath            string
	httpClient          *http.Client
	allowLocalDiscovery bool
	secret              secretStore
}

type secretStore struct {
	mu           sync.RWMutex
	cookieHeader string
}

func NewClient(opts Options) *Client {
	baseURL := strings.TrimRight(opts.BaseURL, "/")
	if baseURL == "" {
		baseURL = defaultBaseURL
	}
	httpClient := opts.HTTPClient
	if httpClient == nil {
		httpClient = &http.Client{Timeout: 15 * time.Second}
	}
	return &Client{
		baseURL:             baseURL,
		authPath:            opts.AuthPath,
		httpClient:          httpClient,
		allowLocalDiscovery: opts.AllowLocalDiscovery,
	}
}

func (c *Client) Name() string { return providerName }

func (c *Client) SetCookieHeader(cookieHeader string) {
	c.secret.set(strings.TrimSpace(cookieHeader))
}

func (c *Client) SetAccessToken(accessToken string) error {
	cookieHeader, err := cookieFromAccessToken(accessToken)
	if err != nil {
		return err
	}
	c.secret.set(cookieHeader)
	return nil
}

func (c *Client) Fetch(ctx context.Context) (usage.UsageData, error) {
	data := baseUsageData()
	cookieHeader, err := c.cookieHeader(ctx)
	if err != nil {
		message := "Log in to cursor.com, or push Cursor credentials from the tray."
		data.Error = &message
		return data, nil
	}
	summary, status, err := c.fetchUsageSummary(ctx, cookieHeader)
	if err != nil {
		data = c.dataForFetchError(data, err)
		if ctx.Err() != nil {
			return data, err
		}
		return data, nil
	}
	if status == http.StatusUnauthorized {
		c.secret.clear()
		message := "Cursor session expired. Log in to cursor.com again."
		data.Error = &message
		data.NeedsReauth = true
		return data, nil
	}
	if status != http.StatusOK {
		message := fmt.Sprintf("Cursor usage request failed with HTTP %d.", status)
		data.Error = &message
		return data, nil
	}
	userInfo := c.fetchUserInfo(ctx, cookieHeader)
	legacyUsage := c.fetchLegacyUsage(ctx, cookieHeader, userInfo.Sub)
	populateUsageData(&data, summary, legacyUsage)
	data.Subtitle = summary.MembershipType
	return data, nil
}

func (c *Client) dataForFetchError(data usage.UsageData, err error) usage.UsageData {
	message := err.Error()
	data.Error = &message
	if errors.Is(err, ErrUnauthorized) {
		data.NeedsReauth = true
		c.secret.clear()
	}
	return data
}

func (c *Client) cookieHeader(ctx context.Context) (string, error) {
	if cookieHeader := c.secret.get(); cookieHeader != "" {
		return cookieHeader, nil
	}
	if !c.allowLocalDiscovery {
		return "", ErrUnauthorized
	}
	cookieHeader, err := readLocalCookieHeader(ctx, c.authPath)
	if err != nil {
		return "", err
	}
	c.secret.set(cookieHeader)
	return cookieHeader, nil
}

func (c *Client) fetchUsageSummary(ctx context.Context, cookieHeader string) (cursorUsageSummary, int, error) {
	resp, err := c.get(ctx, usageSummaryPath, cookieHeader)
	if err != nil {
		return cursorUsageSummary{}, 0, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return cursorUsageSummary{}, resp.StatusCode, nil
	}
	var summary cursorUsageSummary
	if err := json.NewDecoder(resp.Body).Decode(&summary); err != nil {
		return cursorUsageSummary{}, resp.StatusCode, fmt.Errorf("decode Cursor usage summary: %w", err)
	}
	return summary, resp.StatusCode, nil
}

func (c *Client) fetchUserInfo(ctx context.Context, cookieHeader string) cursorUserInfo {
	resp, err := c.get(ctx, authMePath, cookieHeader)
	if err != nil {
		return cursorUserInfo{}
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return cursorUserInfo{}
	}
	var userInfo cursorUserInfo
	if err := json.NewDecoder(resp.Body).Decode(&userInfo); err != nil {
		return cursorUserInfo{}
	}
	return userInfo
}

func (c *Client) fetchLegacyUsage(ctx context.Context, cookieHeader string, userSub string) *cursorUsageResponse {
	if strings.TrimSpace(userSub) == "" {
		return nil
	}
	path := legacyUsagePath + "?user=" + url.QueryEscape(userSub)
	resp, err := c.get(ctx, path, cookieHeader)
	if err != nil {
		return nil
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil
	}
	var legacyUsage cursorUsageResponse
	if err := json.NewDecoder(resp.Body).Decode(&legacyUsage); err != nil {
		return nil
	}
	return &legacyUsage
}

func (c *Client) get(ctx context.Context, path string, cookieHeader string) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+path, nil)
	if err != nil {
		return nil, fmt.Errorf("build Cursor request: %w", err)
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Cookie", cookieHeader)
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("send Cursor request: %w", err)
	}
	return resp, nil
}

func (s *secretStore) set(cookieHeader string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.cookieHeader = cookieHeader
}

func (s *secretStore) get() string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.cookieHeader
}

func (s *secretStore) clear() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.cookieHeader = ""
}

func (c *Client) hasSecret() bool {
	return c.secret.get() != ""
}

func baseUsageData() usage.UsageData {
	return usage.UsageData{
		ProviderName:   providerName,
		PrimaryLabel:   "Included Plan",
		SecondaryLabel: "On-Demand",
		ShowSecondary:  true,
		ReauthCommand:  nil,
		NeedsReauth:    false,
		Current:        usage.UsageBucket{},
		Weekly:         usage.UsageBucket{},
	}
}
