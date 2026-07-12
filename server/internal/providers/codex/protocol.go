package codex

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/AaronFeledy/claude-usage-widget/server/internal/usage"
)

func parseUsage(body []byte, data usage.UsageData) (usage.UsageData, error) {
	var decoded whamResponse
	if err := json.Unmarshal(body, &decoded); err != nil {
		return data, fmt.Errorf("decode codex usage response: %w", err)
	}
	if decoded.PlanType != nil {
		data.Subtitle = decoded.PlanType
	}
	data.Current = usageBucket(decoded.RateLimit.PrimaryWindow)
	data.Weekly = usageBucket(decoded.RateLimit.SecondaryWindow)
	return data, nil
}

func baseUsage() usage.UsageData {
	reauth := "codex"
	return usage.UsageData{ProviderName: providerName, PrimaryLabel: "5-Hour", SecondaryLabel: "Weekly", ShowSecondary: true, ReauthCommand: &reauth}
}

func reauthUsage() usage.UsageData {
	data := baseUsage()
	message := "AUTH_EXPIRED"
	data.Error = &message
	data.NeedsReauth = true
	return data
}

func usageBucket(window whamWindow) usage.UsageBucket {
	var resetsAt *time.Time
	if window.ResetAt != nil {
		parsed := time.Unix(*window.ResetAt, 0).UTC()
		resetsAt = &parsed
	}
	return usage.UsageBucket{Utilization: window.UsedPercent, ResetsAt: resetsAt}
}

func isInvalidGrantBody(body string) bool {
	lower := strings.ToLower(body)
	return strings.Contains(lower, "refresh_token_expired") || strings.Contains(lower, "refresh_token_reused") || strings.Contains(lower, "refresh_token_invalidated")
}

func credsExpiresAt(creds Credentials) time.Time {
	if creds.ExpiresAt != nil {
		return *creds.ExpiresAt
	}
	return time.Time{}
}

type refreshRequest struct {
	ClientID     string `json:"client_id"`
	GrantType    string `json:"grant_type"`
	RefreshToken string `json:"refresh_token"`
	Scope        string `json:"scope"`
}

type refreshResponse struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	ExpiresIn    *int   `json:"expires_in"`
}

type whamResponse struct {
	PlanType  *string `json:"plan_type"`
	RateLimit struct {
		PrimaryWindow   whamWindow `json:"primary_window"`
		SecondaryWindow whamWindow `json:"secondary_window"`
	} `json:"rate_limit"`
}

type whamWindow struct {
	UsedPercent float64 `json:"used_percent"`
	ResetAt     *int64  `json:"reset_at"`
}
