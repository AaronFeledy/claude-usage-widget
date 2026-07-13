package grok

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"net/http"
	"time"

	"github.com/AaronFeledy/claude-usage-widget/server/internal/usage"
)

type billingResponse struct {
	Config billingConfig `json:"config"`
}

type billingConfig struct {
	Used             billingValue `json:"used"`
	MonthlyLimit     billingValue `json:"monthlyLimit"`
	OnDemandCap      billingValue `json:"onDemandCap"`
	BillingPeriodEnd string       `json:"billingPeriodEnd"`
}

type billingValue struct {
	Value *float64 `json:"val"`
}

type settingsResponse struct {
	SubscriptionTierDisplay string `json:"subscription_tier_display"`
}

func (p *Provider) sendGet(ctx context.Context, url string, token string) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("create Grok request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("X-XAI-Token-Auth", tokenAuthHeader)
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", userAgent)
	resp, err := p.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("send Grok request: %w", err)
	}
	return resp, nil
}

func mapBilling(body io.Reader, data *usage.UsageData, now time.Time) error {
	var decoded billingResponse
	if err := json.NewDecoder(body).Decode(&decoded); err != nil {
		return fmt.Errorf("decode billing: %w", err)
	}
	used := decoded.Config.Used.Value
	limit := decoded.Config.MonthlyLimit.Value
	onDemand := decoded.Config.OnDemandCap.Value
	if used == nil || limit == nil || *limit <= 0 || onDemand == nil || decoded.Config.BillingPeriodEnd == "" {
		return errorsChangedResponse()
	}
	reset, err := time.Parse(time.RFC3339, decoded.Config.BillingPeriodEnd)
	if err != nil {
		return fmt.Errorf("parse billing period end: %w", err)
	}
	reset = reset.UTC()
	percent := math.Max(0, math.Min(100, (*used / *limit * 100)))
	data.Current = usage.UsageBucket{Utilization: percent, ResetsAt: &reset}
	data.Weekly = usage.UsageBucket{Utilization: 0, ResetsAt: &reset}
	data.PrimaryStatusText = strPtr(fmt.Sprintf("%s / %s credits · %s", formatWholeNumber(*used), formatWholeNumber(*limit), formatResetDate(reset, now)))
	if *onDemand > 0 {
		data.SecondaryStatusText = strPtr(fmt.Sprintf("%s pay-as-you-go cap", formatWholeNumber(*onDemand)))
	} else {
		data.SecondaryStatusText = strPtr("Pay as you go disabled")
	}
	data.ShowSecondary = false
	return nil
}

func (p *Provider) populateSettings(ctx context.Context, token string, data *usage.UsageData) {
	resp, err := p.sendGet(ctx, p.settingsURL, token)
	if err != nil {
		return
	}
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		if err := drainAndClose(resp); err != nil {
			return
		}
		return
	}
	var decoded settingsResponse
	if err := json.NewDecoder(resp.Body).Decode(&decoded); err != nil {
		if closeErr := drainAndClose(resp); closeErr != nil {
			return
		}
		return
	}
	if err := drainAndClose(resp); err != nil {
		return
	}
	if decoded.SubscriptionTierDisplay != "" {
		data.Subtitle = strPtr(decoded.SubscriptionTierDisplay)
	}
}

func formatResetDate(reset time.Time, now time.Time) string {
	localReset := reset.Local()
	format := "Jan 2"
	if localReset.Year() != now.Local().Year() {
		format = "Jan 2, 2006"
	}
	return "Resets " + localReset.Format(format)
}

func drainAndClose(resp *http.Response) error {
	_, copyErr := io.Copy(io.Discard, resp.Body)
	closeErr := resp.Body.Close()
	if copyErr != nil || closeErr != nil {
		return errors.Join(wrapIfErr("drain Grok response body", copyErr), wrapIfErr("close Grok response body", closeErr))
	}
	return nil
}

func wrapIfErr(message string, err error) error {
	if err == nil {
		return nil
	}
	return fmt.Errorf("%s: %w", message, err)
}

func formatWholeNumber(value float64) string {
	raw := fmt.Sprintf("%.0f", value)
	start := 0
	if raw[0] == '-' {
		start = 1
	}
	for i := len(raw) - 3; i > start; i -= 3 {
		raw = raw[:i] + "," + raw[i:]
	}
	return raw
}

func errorsChangedResponse() error {
	return fmt.Errorf("Grok billing response changed")
}
