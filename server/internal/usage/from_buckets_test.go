package usage_test

import (
	"reflect"
	"testing"

	"github.com/AaronFeledy/claude-usage-widget/server/internal/usage"
)

func Test_FromBuckets_sets_provider_and_derives_header(t *testing.T) {
	// Given
	buckets := []usage.Bucket{
		{ID: usage.BucketSession, Label: "Current Session", Utilization: 10},
		{ID: usage.BucketWeekly, Label: "Weekly", Utilization: 20},
	}

	// When
	got := usage.FromBuckets("Claude", buckets)

	// Then
	if got.ProviderName != "Claude" || got.PrimaryLabel != "Current Session" || got.SecondaryLabel != "Weekly" || !got.ShowSecondary {
		t.Fatalf("FromBuckets header = %+v", got)
	}
	if got.Current.Utilization != 10 || got.Weekly.Utilization != 20 || len(got.Buckets) != 2 {
		t.Fatalf("FromBuckets meters = %+v", got)
	}
}

func Test_NormalizeBuckets_orders_dedupes_and_caps(t *testing.T) {
	// Given
	in := []usage.Bucket{
		{ID: usage.BucketExtra, Label: "Extra", Utilization: 1},
		{ID: "weekly_fable", Label: "Fable", Utilization: 2},
		{ID: usage.BucketWeekly, Label: "Weekly", Utilization: 3},
		{ID: usage.BucketSession, Label: "Session", Utilization: 4},
		{ID: usage.BucketSession, Label: "Dup", Utilization: 99},
		{ID: "", Label: "skip", Utilization: 5},
		{ID: "other", Label: "Other", Utilization: 6},
	}

	// When
	got := usage.NormalizeBuckets(in)

	// Then
	wantIDs := []string{usage.BucketSession, usage.BucketWeekly, "weekly_fable", usage.BucketExtra, "other"}
	if len(got) != len(wantIDs) {
		t.Fatalf("len = %d, want %d (%+v)", len(got), len(wantIDs), got)
	}
	for i, id := range wantIDs {
		if got[i].ID != id {
			t.Fatalf("got[%d].ID = %q, want %q (full=%+v)", i, got[i].ID, id, got)
		}
	}
	if got[0].Utilization != 4 {
		t.Fatalf("first-wins dedupe failed: %+v", got[0])
	}
}

func Test_NormalizeBuckets_caps_at_MaxBuckets(t *testing.T) {
	// Given
	in := make([]usage.Bucket, usage.MaxBuckets+3)
	for i := range in {
		in[i] = usage.Bucket{ID: "id_" + itoa(i), Label: "x", Utilization: float64(i)}
	}

	// When
	got := usage.NormalizeBuckets(in)

	// Then
	if len(got) != usage.MaxBuckets {
		t.Fatalf("len = %d, want %d", len(got), usage.MaxBuckets)
	}
}

func itoa(value int) string {
	if value == 0 {
		return "0"
	}
	digits := make([]byte, 0, 4)
	for value > 0 {
		digits = append([]byte{byte('0' + value%10)}, digits...)
		value /= 10
	}
	return string(digits)
}

func Test_ShouldShowCreditMeter_enabled_or_usage(t *testing.T) {
	util := 12.0
	zero := 0.0
	tests := []struct {
		name    string
		enabled bool
		used    float64
		util    *float64
		want    bool
	}{
		{name: "disabled zero", want: false},
		{name: "enabled zero", enabled: true, want: true},
		{name: "disabled with spend", used: 1.5, want: true},
		{name: "disabled with util", util: &util, want: true},
		{name: "disabled zero util", util: &zero, want: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := usage.ShouldShowCreditMeter(tt.enabled, tt.used, tt.util); got != tt.want {
				t.Fatalf("ShouldShowCreditMeter = %v, want %v", got, tt.want)
			}
		})
	}
}

func Test_CreditUtilization_prefers_explicit_then_ratio(t *testing.T) {
	explicit := 77.0
	if got := usage.CreditUtilization(10, 100, &explicit); got != 77 {
		t.Fatalf("explicit = %v, want 77", got)
	}
	if got := usage.CreditUtilization(25, 100, nil); got != 25 {
		t.Fatalf("ratio = %v, want 25", got)
	}
	if got := usage.CreditUtilization(10, 0, nil); got != 0 {
		t.Fatalf("no limit = %v, want 0", got)
	}
	if got := usage.CreditUtilization(200, 100, nil); got != 100 {
		t.Fatalf("clamp = %v, want 100", got)
	}
}

func Test_WithBuckets_normalizes_order(t *testing.T) {
	// Given
	data := usage.UsageData{ProviderName: "Claude"}
	in := []usage.Bucket{
		{ID: usage.BucketExtra, Label: "Extra", Utilization: 1},
		{ID: usage.BucketSession, Label: "Session", Utilization: 2},
	}

	// When
	got := data.WithBuckets(in)

	// Then
	if !reflect.DeepEqual([]string{got.Buckets[0].ID, got.Buckets[1].ID}, []string{usage.BucketSession, usage.BucketExtra}) {
		t.Fatalf("order = %+v", got.Buckets)
	}
}
