package claude

import (
	"fmt"
	"math"
	"strconv"
	"strings"

	"github.com/AaronFeledy/claude-usage-widget/server/internal/usage"
)

type extraUsageResponse struct {
	IsEnabled    *bool    `json:"is_enabled"`
	MonthlyLimit *float64 `json:"monthly_limit"`
	UsedCredits  *float64 `json:"used_credits"`
	Utilization  *float64 `json:"utilization"`
}

func (e *extraUsageResponse) toBucket() (usage.Bucket, bool) {
	if e == nil {
		return usage.Bucket{}, false
	}
	enabled := e.IsEnabled != nil && *e.IsEnabled
	used := floatOrZero(e.UsedCredits)
	limit := floatOrZero(e.MonthlyLimit)
	if !usage.ShouldShowCreditMeter(enabled, used, e.Utilization) {
		return usage.Bucket{}, false
	}
	util := usage.CreditUtilization(used, limit, e.Utilization)
	status := formatExtraUsageStatus(used, limit)
	return usage.Bucket{
		ID:          usage.BucketExtra,
		Label:       "Extra usage",
		Utilization: util,
		StatusText:  &status,
	}, true
}

func floatOrZero(value *float64) float64 {
	if value == nil {
		return 0
	}
	return *value
}

func formatExtraUsageStatus(used, limit float64) string {
	if limit > 0 {
		return fmt.Sprintf("%s / %s credits", formatCredits(used), formatCredits(limit))
	}
	if used > 0 {
		return fmt.Sprintf("%s credits used", formatCredits(used))
	}
	return "Extra usage enabled"
}

func formatCredits(value float64) string {
	if math.Abs(value-math.Round(value)) < 1e-9 {
		return strconv.FormatInt(int64(math.Round(value)), 10)
	}
	formatted := strconv.FormatFloat(value, 'f', 2, 64)
	return strings.TrimRight(strings.TrimRight(formatted, "0"), ".")
}
