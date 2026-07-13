package claude

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/AaronFeledy/claude-usage-widget/server/internal/usage"
)

type usageResponse struct {
	FiveHour usageBucketResponse `json:"five_hour"`
	SevenDay usageBucketResponse `json:"seven_day"`
}

type usageBucketResponse struct {
	Utilization float64 `json:"utilization"`
	ResetsAt    string  `json:"resets_at"`
}

type statusError struct {
	statusCode int
	body       string
}

func (e statusError) message() string {
	if e.statusCode == http.StatusTooManyRequests {
		return "Rate limited. Will retry."
	}
	return fmt.Sprintf("API error (%d): %s", e.statusCode, e.body)
}

func (c *Client) fetchUsage(ctx context.Context, creds *credentials) (usage.UsageData, *statusError, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.usageURL, nil)
	if err != nil {
		return usage.UsageData{}, nil, fmt.Errorf("build usage request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+creds.accessToken)
	req.Header.Set("anthropic-beta", betaHeader)
	req.Header.Set("User-Agent", userAgent)
	resp, err := c.client.Do(req)
	if err != nil {
		return usage.UsageData{}, nil, err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return usage.UsageData{}, nil, fmt.Errorf("read usage response: %w", err)
	}
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return usage.UsageData{}, &statusError{statusCode: resp.StatusCode, body: string(body)}, nil
	}
	parsed, err := parseUsageResponse(body, creds.subscriptionType)
	if err != nil {
		return usage.UsageData{}, nil, err
	}
	return parsed, nil, nil
}

func parseUsageResponse(body []byte, subscription *string) (usage.UsageData, error) {
	var response usageResponse
	if err := json.Unmarshal(body, &response); err != nil {
		return usage.UsageData{}, fmt.Errorf("parse usage response: %w", err)
	}
	current, err := response.FiveHour.toUsageBucket()
	if err != nil {
		return usage.UsageData{}, fmt.Errorf("five_hour: %w", err)
	}
	weekly, err := response.SevenDay.toUsageBucket()
	if err != nil {
		return usage.UsageData{}, fmt.Errorf("seven_day: %w", err)
	}
	return usage.UsageData{ProviderName: providerName, PrimaryLabel: primaryLabel, SecondaryLabel: secondaryLabel, ShowSecondary: true, Subtitle: subscription, Current: current, Weekly: weekly}, nil
}

func (b usageBucketResponse) toUsageBucket() (usage.UsageBucket, error) {
	if b.Utilization < 0 || b.Utilization > 100 {
		return usage.UsageBucket{}, fmt.Errorf("utilization %.2f out of range", b.Utilization)
	}
	var reset *time.Time
	if strings.TrimSpace(b.ResetsAt) != "" {
		parsed, err := time.Parse(time.RFC3339, b.ResetsAt)
		if err != nil {
			return usage.UsageBucket{}, fmt.Errorf("resets_at: %w", err)
		}
		utc := parsed.UTC()
		reset = &utc
	}
	return usage.UsageBucket{Utilization: b.Utilization, ResetsAt: reset}, nil
}

func formatCredentialError(err error) string {
	if err == nil {
		return ""
	}
	if strings.Contains(err.Error(), ErrCredentialsMissing.Error()) {
		return "Credentials not found. Run claude to authenticate."
	}
	return "Credentials are invalid. Run claude to authenticate."
}

func formatFetchError(err error) string {
	if err == nil {
		return ""
	}
	if strings.Contains(err.Error(), "Client.Timeout") || strings.Contains(err.Error(), "context deadline exceeded") {
		return "Request timed out"
	}
	return "Network error: " + err.Error()
}
