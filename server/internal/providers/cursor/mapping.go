package cursor

import (
	"fmt"
	"math"
	"strconv"
	"strings"
	"time"

	"github.com/AaronFeledy/claude-usage-widget/server/internal/usage"
)

func populateUsageData(data *usage.UsageData, summary cursorUsageSummary, legacyUsage *cursorUsageResponse) {
	billingCycleEnd := parseDate(summary.BillingCycleEnd)
	planUsedRaw := intValue(planUsage(summary).Used)
	planLimitRaw := intValue(planUsage(summary).Limit)
	planPercent := percentFromPlan(summary, planUsedRaw, planLimitRaw)

	requestsUsed, requestsLimit, hasRequests := requestUsage(legacyUsage)
	if hasRequests {
		data.PrimaryLabel = "Requests"
		planPercent = float64(requestsUsed) / float64(requestsLimit) * 100
		primaryStatus := fmt.Sprintf("%d / %d requests this cycle", requestsUsed, requestsLimit)
		data.PrimaryStatusText = &primaryStatus
	} else {
		primaryStatus := fmt.Sprintf("$%s / $%s this cycle", money(planUsedRaw), money(planLimitRaw))
		data.PrimaryStatusText = &primaryStatus
	}

	data.Current = usage.UsageBucket{Utilization: clampPercent(planPercent), ResetsAt: billingCycleEnd}
	populateOnDemand(data, summary, billingCycleEnd)
}

func populateOnDemand(data *usage.UsageData, summary cursorUsageSummary, billingCycleEnd *time.Time) {
	onDemand := onDemandUsage(summary)
	onDemandUsedRaw := intValue(onDemand.Used)
	if onDemand.Limit != nil && *onDemand.Limit > 0 {
		onDemandLimitRaw := *onDemand.Limit
		onDemandPercent := float64(onDemandUsedRaw) / float64(onDemandLimitRaw) * 100
		data.Weekly = usage.UsageBucket{Utilization: clampPercent(onDemandPercent), ResetsAt: billingCycleEnd}
		secondaryStatus := fmt.Sprintf("$%s / $%s on-demand", money(onDemandUsedRaw), money(onDemandLimitRaw))
		data.SecondaryStatusText = &secondaryStatus
		return
	}
	data.ShowSecondary = false
	secondaryStatus := "No on-demand cap exposed by Cursor"
	if onDemandUsedRaw > 0 {
		secondaryStatus = fmt.Sprintf("$%s on-demand this cycle", money(onDemandUsedRaw))
	}
	data.SecondaryStatusText = &secondaryStatus
}

func planUsage(summary cursorUsageSummary) cursorPlanUsage {
	if summary.IndividualUsage == nil || summary.IndividualUsage.Plan == nil {
		return cursorPlanUsage{}
	}
	return *summary.IndividualUsage.Plan
}

func onDemandUsage(summary cursorUsageSummary) cursorOnDemandUsage {
	if summary.IndividualUsage == nil || summary.IndividualUsage.OnDemand == nil {
		return cursorOnDemandUsage{}
	}
	return *summary.IndividualUsage.OnDemand
}

func requestUsage(legacyUsage *cursorUsageResponse) (int, int, bool) {
	if legacyUsage == nil || legacyUsage.GPT4 == nil || legacyUsage.GPT4.MaxRequestUsage == nil {
		return 0, 0, false
	}
	requestsLimit := *legacyUsage.GPT4.MaxRequestUsage
	if requestsLimit <= 0 {
		return 0, 0, false
	}
	if legacyUsage.GPT4.NumRequestsTotal != nil {
		return *legacyUsage.GPT4.NumRequestsTotal, requestsLimit, true
	}
	if legacyUsage.GPT4.NumRequests != nil {
		return *legacyUsage.GPT4.NumRequests, requestsLimit, true
	}
	return 0, 0, false
}

func percentFromPlan(summary cursorUsageSummary, planUsedRaw int, planLimitRaw int) float64 {
	if planLimitRaw > 0 {
		return float64(planUsedRaw) / float64(planLimitRaw) * 100
	}
	plan := planUsage(summary)
	if plan.TotalPercentUsed == nil {
		return 0
	}
	if *plan.TotalPercentUsed <= 1 {
		return *plan.TotalPercentUsed * 100
	}
	return *plan.TotalPercentUsed
}

func parseDate(value *string) *time.Time {
	if value == nil || *value == "" {
		return nil
	}
	parsed, err := time.Parse(time.RFC3339, *value)
	if err != nil {
		return nil
	}
	utc := parsed.UTC()
	return &utc
}

func clampPercent(value float64) float64 {
	return math.Min(math.Max(value, 0), 100)
}

func intValue(value *int) int {
	if value == nil {
		return 0
	}
	return *value
}

func money(cents int) string {
	dollars := float64(cents) / 100
	formatted := strconv.FormatFloat(dollars, 'f', 2, 64)
	return strings.TrimRight(strings.TrimRight(formatted, "0"), ".")
}
