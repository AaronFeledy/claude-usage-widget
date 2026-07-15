package poller

import (
	"testing"
	"time"

	"github.com/AaronFeledy/claude-usage-widget/server/internal/usage"
)

func Test_copyUsageData_deep_copies_buckets_and_reset_times(t *testing.T) {
	// Given
	reset := time.Date(2026, 7, 12, 22, 30, 0, 0, time.UTC)
	wantReset := reset
	source := usage.UsageData{Buckets: []usage.Bucket{{ID: "weekly", Label: "Weekly", Utilization: 71, ResetsAt: &reset}}}

	// When
	copied := copyUsageData(source)
	source.Buckets[0].Label = "changed"
	*source.Buckets[0].ResetsAt = reset.Add(time.Hour)

	// Then
	if len(copied.Buckets) != 1 {
		t.Fatalf("len(Buckets) = %d, want 1", len(copied.Buckets))
	}
	if copied.Buckets[0].Label != "Weekly" || !copied.Buckets[0].ResetsAt.Equal(wantReset) {
		t.Fatalf("copied bucket = %+v, want independent Weekly bucket resetting at %s", copied.Buckets[0], wantReset)
	}
}
