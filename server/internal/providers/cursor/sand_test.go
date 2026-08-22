package cursor

import (
	"testing"

	"github.com/AaronFeledy/claude-usage-widget/server/internal/usage"
)

func Test_grokBotBucket_emits_weekly_meter_when_included_limit_exists(t *testing.T) {
	// Given
	sand := &cursorSandUsage{
		NextResetTimestampUtc:   "2026-08-22T23:57:03.809Z",
		UsagePercent:            98.343013,
		HasNonZeroIncludedLimit: true,
	}

	// When
	bucket, ok := grokBotBucket(sand)

	// Then
	if !ok || bucket.ID != grokBotBucketID || bucket.Label != "Grok Bot" {
		t.Fatalf("bucket = %#v ok=%v", bucket, ok)
	}
	if bucket.Utilization != 98.343013 {
		t.Fatalf("utilization = %v, want 98.343013", bucket.Utilization)
	}
	if bucket.ResetsAt == nil || bucket.ResetsAt.UTC().Format("2006-01-02T15:04:05Z") != "2026-08-22T23:57:03Z" {
		t.Fatalf("resets_at = %v", bucket.ResetsAt)
	}
}

func Test_grokBotBucket_hides_when_nil(t *testing.T) {
	// When
	_, ok := grokBotBucket(nil)

	// Then
	if ok {
		t.Fatal("expected hidden Grok Bot meter for nil sand usage")
	}
}

func Test_grokBotBucket_hides_when_no_included_limit_and_zero_usage(t *testing.T) {
	// Given
	sand := &cursorSandUsage{HasNonZeroIncludedLimit: false, UsagePercent: 0}

	// When
	_, ok := grokBotBucket(sand)

	// Then
	if ok {
		t.Fatal("expected hidden Grok Bot meter without included limit or usage")
	}
}

func Test_grokBotBucket_shows_nonzero_usage_without_included_limit(t *testing.T) {
	// Given
	sand := &cursorSandUsage{HasNonZeroIncludedLimit: false, UsagePercent: 12}

	// When
	bucket, ok := grokBotBucket(sand)

	// Then
	if !ok || bucket.ID != grokBotBucketID || bucket.Utilization != 12 {
		t.Fatalf("bucket = %#v ok=%v", bucket, ok)
	}
}

func Test_populateUsageData_emits_grok_bot_between_plan_meters_and_on_demand(t *testing.T) {
	// Given
	autoPercent, apiPercent := 16.885, 100.0
	onDemandUsed, onDemandLimit := 500, 2000
	summary := cursorUsageSummary{IndividualUsage: &cursorIndividualUsage{
		Plan: &cursorPlanUsage{
			AutoPercentUsed: &autoPercent,
			APIPercentUsed:  &apiPercent,
		},
		OnDemand: &cursorOnDemandUsage{Used: &onDemandUsed, Limit: &onDemandLimit},
	}}
	sand := &cursorSandUsage{
		NextResetTimestampUtc:   "2026-08-22T23:57:03.809Z",
		UsagePercent:            98.343013,
		HasNonZeroIncludedLimit: true,
	}
	data := baseUsageData()

	// When
	populateUsageData(&data, summary, nil, sand)

	// Then
	if len(data.Buckets) != 4 {
		t.Fatalf("Buckets = %#v, want auto, api, weekly_grok_bot, on_demand", data.Buckets)
	}
	wantIDs := []string{usage.BucketAuto, usage.BucketAPI, grokBotBucketID, usage.BucketOnDemand}
	wantLabels := []string{"Cursor Models", "Other Models", "Grok Bot", "On-Demand"}
	for index, bucket := range data.Buckets {
		if bucket.ID != wantIDs[index] || bucket.Label != wantLabels[index] {
			t.Fatalf("Buckets[%d] = %#v, want id=%q label=%q", index, bucket, wantIDs[index], wantLabels[index])
		}
	}
}
