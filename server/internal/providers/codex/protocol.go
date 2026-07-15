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
	current, weekly := normalizeWindows(decoded.RateLimit.PrimaryWindow, decoded.RateLimit.SecondaryWindow)
	currentUsage := usageBucket(current)
	weeklyUsage := usageBucket(weekly)
	data.ProviderName = providerName
	return data.WithBuckets([]usage.Bucket{
		{ID: usage.BucketSession, Label: "5-Hour", Utilization: currentUsage.Utilization, ResetsAt: currentUsage.ResetsAt},
		{ID: usage.BucketWeekly, Label: "Weekly", Utilization: weeklyUsage.Utilization, ResetsAt: weeklyUsage.ResetsAt},
	}), nil
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

func usageBucket(window *whamWindow) usage.UsageBucket {
	if window == nil {
		return usage.UsageBucket{}
	}
	var resetsAt *time.Time
	if window.ResetAt != nil {
		parsed := time.Unix(*window.ResetAt, 0).UTC()
		resetsAt = &parsed
	}
	return usage.UsageBucket{Utilization: window.UsedPercent, ResetsAt: resetsAt}
}

func isInvalidGrantBody(body []byte) bool {
	var response struct {
		Error json.RawMessage `json:"error"`
		Code  string          `json:"code"`
	}
	if err := json.Unmarshal(body, &response); err == nil {
		code := response.Code
		var nested struct {
			Code string `json:"code"`
		}
		if json.Unmarshal(response.Error, &nested) == nil && nested.Code != "" {
			code = nested.Code
		} else {
			var direct string
			if json.Unmarshal(response.Error, &direct) == nil {
				code = direct
			}
		}
		return isTerminalRefreshCode(code)
	}
	return isTerminalRefreshCode(string(body))
}

func isTerminalRefreshCode(code string) bool {
	lower := strings.ToLower(code)
	return strings.Contains(lower, "refresh_token_expired") || strings.Contains(lower, "refresh_token_reused") || strings.Contains(lower, "invalid_grant") || strings.Contains(lower, "refresh_token_invalidated")
}

func normalizeWindows(primary, secondary *whamWindow) (*whamWindow, *whamWindow) {
	if primary == nil {
		if windowRoleOf(secondary) == windowRoleWeekly {
			return nil, secondary
		}
		return secondary, nil
	}
	if secondary == nil {
		if windowRoleOf(primary) == windowRoleWeekly {
			return nil, primary
		}
		return primary, nil
	}
	if windowRoleOf(primary) == windowRoleWeekly && windowRoleOf(secondary) != windowRoleWeekly {
		return secondary, primary
	}
	return primary, secondary
}

type windowRole uint8

const (
	windowRoleUnknown windowRole = iota
	windowRoleSession
	windowRoleWeekly
)

func windowRoleOf(window *whamWindow) windowRole {
	if window == nil || window.LimitWindowSeconds == nil {
		return windowRoleUnknown
	}
	switch *window.LimitWindowSeconds {
	case 18000:
		return windowRoleSession
	case 604800:
		return windowRoleWeekly
	default:
		return windowRoleUnknown
	}
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
		PrimaryWindow   *whamWindow `json:"primary_window"`
		SecondaryWindow *whamWindow `json:"secondary_window"`
	} `json:"rate_limit"`
}

type whamWindow struct {
	UsedPercent        float64 `json:"used_percent"`
	ResetAt            *int64  `json:"reset_at"`
	LimitWindowSeconds *int64  `json:"limit_window_seconds"`
}
