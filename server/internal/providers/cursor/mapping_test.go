package cursor

import (
	"testing"

	"github.com/AaronFeledy/claude-usage-widget/server/internal/usage"
)

func Test_percentFromPlan_uses_fractional_total_percent_as_percent_units(t *testing.T) {
	percent := 0.36
	summary := cursorUsageSummary{IndividualUsage: &cursorIndividualUsage{Plan: &cursorPlanUsage{TotalPercentUsed: &percent}}}

	got := percentFromPlan(summary, 0, 0)

	if got != 0.36 {
		t.Fatalf("percent = %v, want 0.36", got)
	}
}

func Test_percentFromPlan_prefers_total_percent_over_plan_ratio(t *testing.T) {
	percent := 7.5
	summary := cursorUsageSummary{IndividualUsage: &cursorIndividualUsage{Plan: &cursorPlanUsage{TotalPercentUsed: &percent}}}

	got := percentFromPlan(summary, 50, 100)

	if got != 7.5 {
		t.Fatalf("percent = %v, want 7.5", got)
	}
}

func Test_percentFromPlan_averages_auto_and_api_percent(t *testing.T) {
	autoPercent, apiPercent := 20.0, 40.0
	summary := cursorUsageSummary{IndividualUsage: &cursorIndividualUsage{Plan: &cursorPlanUsage{
		AutoPercentUsed: &autoPercent,
		APIPercentUsed:  &apiPercent,
	}}}

	got := percentFromPlan(summary, 0, 0)

	if got != 30 {
		t.Fatalf("percent = %v, want 30", got)
	}
}

func Test_populateUsageData_uses_overall_fallback_for_percent_and_status(t *testing.T) {
	used, limit := 2500, 10000
	summary := cursorUsageSummary{IndividualUsage: &cursorIndividualUsage{Overall: &cursorOverallUsage{Used: &used, Limit: &limit}}}
	data := usage.UsageData{}

	populateUsageData(&data, summary, nil)

	if data.Current.Utilization != 25 {
		t.Fatalf("utilization = %v, want 25", data.Current.Utilization)
	}
	assertStringPtr(t, data.PrimaryStatusText, "$25 / $100 this cycle")
}

func Test_populateUsageData_uses_pooled_fallback_for_percent_and_status(t *testing.T) {
	used, limit := 3000, 12000
	summary := cursorUsageSummary{TeamUsage: &cursorTeamUsage{Pooled: &cursorPooledUsage{Used: &used, Limit: &limit}}}
	data := usage.UsageData{}

	populateUsageData(&data, summary, nil)

	if data.Current.Utilization != 25 {
		t.Fatalf("utilization = %v, want 25", data.Current.Utilization)
	}
	assertStringPtr(t, data.PrimaryStatusText, "$30 / $120 this cycle")
}

func Test_populateOnDemand_falls_back_to_team_cap(t *testing.T) {
	personalUsed, teamUsed, teamLimit := 100, 5000, 20000
	summary := cursorUsageSummary{
		IndividualUsage: &cursorIndividualUsage{OnDemand: &cursorOnDemandUsage{Used: &personalUsed}},
		TeamUsage:       &cursorTeamUsage{OnDemand: &cursorOnDemandUsage{Used: &teamUsed, Limit: &teamLimit}},
	}
	data := usage.UsageData{ShowSecondary: true}

	populateOnDemand(&data, summary, nil)

	if data.Weekly.Utilization != 25 {
		t.Fatalf("utilization = %v, want 25", data.Weekly.Utilization)
	}
	assertStringPtr(t, data.SecondaryStatusText, "$50 / $200 team on-demand")
}
