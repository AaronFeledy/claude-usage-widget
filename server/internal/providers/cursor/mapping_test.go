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

	populateUsageData(&data, summary, nil, nil)

	if data.Current.Utilization != 25 {
		t.Fatalf("utilization = %v, want 25", data.Current.Utilization)
	}
	assertStringPtr(t, data.PrimaryStatusText, "$25 / $100 this cycle")
}

func Test_populateUsageData_uses_pooled_fallback_for_percent_and_status(t *testing.T) {
	used, limit := 3000, 12000
	summary := cursorUsageSummary{TeamUsage: &cursorTeamUsage{Pooled: &cursorPooledUsage{Used: &used, Limit: &limit}}}
	data := usage.UsageData{}

	populateUsageData(&data, summary, nil, nil)

	if data.Current.Utilization != 25 {
		t.Fatalf("utilization = %v, want 25", data.Current.Utilization)
	}
	assertStringPtr(t, data.PrimaryStatusText, "$30 / $120 this cycle")
}

func Test_resolveOnDemandBucket_falls_back_to_team_cap(t *testing.T) {
	personalUsed, teamUsed, teamLimit := 100, 5000, 20000
	summary := cursorUsageSummary{
		IndividualUsage: &cursorIndividualUsage{OnDemand: &cursorOnDemandUsage{Used: &personalUsed}},
		TeamUsage:       &cursorTeamUsage{OnDemand: &cursorOnDemandUsage{Used: &teamUsed, Limit: &teamLimit}},
	}

	bucket, status, show := resolveOnDemandBucket(summary, nil)

	if !show || bucket.Utilization != 25 || bucket.ID != usage.BucketOnDemand {
		t.Fatalf("bucket = %#v show=%v", bucket, show)
	}
	assertStringPtr(t, status, "$50 / $200 team on-demand")
}

func Test_resolveOnDemandBucket_shows_when_spend_without_cap(t *testing.T) {
	used := 1500
	summary := cursorUsageSummary{
		IndividualUsage: &cursorIndividualUsage{OnDemand: &cursorOnDemandUsage{Used: &used}},
	}

	bucket, status, show := resolveOnDemandBucket(summary, nil)

	if !show || bucket.Utilization != 0 || bucket.ID != usage.BucketOnDemand {
		t.Fatalf("bucket = %#v show=%v", bucket, show)
	}
	assertStringPtr(t, status, "$15 on-demand this cycle")
}

func Test_resolveOnDemandBucket_shows_when_enabled_without_spend(t *testing.T) {
	// Given
	enabled := true
	summary := cursorUsageSummary{
		IndividualUsage: &cursorIndividualUsage{OnDemand: &cursorOnDemandUsage{Enabled: &enabled}},
	}

	// When
	bucket, status, show := resolveOnDemandBucket(summary, nil)

	// Then
	if !show || bucket.Utilization != 0 || bucket.ID != usage.BucketOnDemand {
		t.Fatalf("bucket = %#v show=%v", bucket, show)
	}
	assertStringPtr(t, status, "On-demand enabled")
}

func Test_populateUsageData_emits_buckets_matching_visible_header(t *testing.T) {
	// Given
	planUsed, planLimit, onDemandUsed, onDemandLimit := 2500, 10000, 500, 2000
	summary := cursorUsageSummary{IndividualUsage: &cursorIndividualUsage{
		Plan:     &cursorPlanUsage{Used: &planUsed, Limit: &planLimit},
		OnDemand: &cursorOnDemandUsage{Used: &onDemandUsed, Limit: &onDemandLimit},
	}}
	data := baseUsageData()

	// When
	populateUsageData(&data, summary, nil, nil)

	// Then
	if len(data.Buckets) != 2 {
		t.Fatalf("Buckets = %#v, want plan and on_demand", data.Buckets)
	}
	primary, secondary := data.Buckets[0], data.Buckets[1]
	if primary.ID != usage.BucketPlan || primary.Label != data.PrimaryLabel || primary.Utilization != data.Current.Utilization {
		t.Fatalf("primary bucket = %#v, current = %#v label = %q", primary, data.Current, data.PrimaryLabel)
	}
	if secondary.ID != usage.BucketOnDemand || secondary.Label != "On-Demand" || secondary.Utilization != data.Weekly.Utilization {
		t.Fatalf("secondary bucket = %#v, weekly = %#v label = %q", secondary, data.Weekly, data.SecondaryLabel)
	}
}

func Test_populateUsageData_emits_separate_auto_api_and_on_demand_buckets(t *testing.T) {
	// Given
	autoPercent, apiPercent := 20.0, 40.0
	onDemandUsed, onDemandLimit := 500, 2000
	summary := cursorUsageSummary{IndividualUsage: &cursorIndividualUsage{
		Plan: &cursorPlanUsage{
			AutoPercentUsed: &autoPercent,
			APIPercentUsed:  &apiPercent,
		},
		OnDemand: &cursorOnDemandUsage{Used: &onDemandUsed, Limit: &onDemandLimit},
	}}
	data := baseUsageData()

	// When
	populateUsageData(&data, summary, nil, nil)

	// Then
	if len(data.Buckets) != 3 {
		t.Fatalf("Buckets = %#v, want auto, api, and on_demand", data.Buckets)
	}
	wantIDs := []string{"auto", "api", usage.BucketOnDemand}
	wantLabels := []string{"Cursor Models", "Other Models", "On-Demand"}
	wantUtilization := []float64{20, 40, 25}
	for index, bucket := range data.Buckets {
		if bucket.ID != wantIDs[index] || bucket.Label != wantLabels[index] || bucket.Utilization != wantUtilization[index] {
			t.Fatalf("Buckets[%d] = %#v, want id=%q label=%q utilization=%v", index, bucket, wantIDs[index], wantLabels[index], wantUtilization[index])
		}
	}
	if data.Buckets[0].StatusText == nil || data.PrimaryStatusText == nil || *data.Buckets[0].StatusText != *data.PrimaryStatusText {
		t.Fatalf("auto StatusText = %v, want primary %v", data.Buckets[0].StatusText, data.PrimaryStatusText)
	}
	if data.Buckets[1].StatusText != nil {
		t.Fatalf("api StatusText = %v, want nil so on-demand text is not reused", data.Buckets[1].StatusText)
	}
}

func Test_populateUsageData_preserves_legacy_header_when_auto_and_api_are_separate(t *testing.T) {
	// Given
	autoPercent, apiPercent, totalPercent := 20.0, 40.0, 35.0
	summary := cursorUsageSummary{IndividualUsage: &cursorIndividualUsage{Plan: &cursorPlanUsage{
		AutoPercentUsed:  &autoPercent,
		APIPercentUsed:   &apiPercent,
		TotalPercentUsed: &totalPercent,
	}}}
	data := baseUsageData()

	// When
	populateUsageData(&data, summary, nil, nil)

	// Then
	if len(data.Buckets) != 2 {
		t.Fatalf("Buckets = %#v, want Cursor Models and Other Models", data.Buckets)
	}
	if data.Current.Utilization != 35 || data.Weekly != (usage.UsageBucket{}) || data.PrimaryLabel != "Included Plan" || data.SecondaryLabel != "On-Demand" || data.ShowSecondary {
		t.Fatalf("legacy header changed: current=%#v weekly=%#v primary=%q secondary=%q show=%v", data.Current, data.Weekly, data.PrimaryLabel, data.SecondaryLabel, data.ShowSecondary)
	}
}

func Test_populateUsageData_preserves_hidden_secondary_header_with_one_bucket(t *testing.T) {
	// Given
	planUsed, planLimit := 2500, 10000
	summary := cursorUsageSummary{IndividualUsage: &cursorIndividualUsage{Plan: &cursorPlanUsage{Used: &planUsed, Limit: &planLimit}}}
	data := baseUsageData()

	// When
	populateUsageData(&data, summary, nil, nil)

	// Then
	if len(data.Buckets) != 1 || data.Buckets[0].ID != "plan" || data.Buckets[0].Label != "Included Plan" {
		t.Fatalf("Buckets = %#v, want one Included Plan bucket", data.Buckets)
	}
	if data.ShowSecondary || data.SecondaryLabel != "On-Demand" || data.Weekly != (usage.UsageBucket{}) {
		t.Fatalf("legacy header = show %v secondary %q weekly %#v", data.ShowSecondary, data.SecondaryLabel, data.Weekly)
	}
}
