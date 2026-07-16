package grok

import (
	"strings"
	"testing"
	"time"

	"github.com/AaronFeledy/claude-usage-widget/server/internal/usage"
)

const onDemandTestReset = "2026-08-01T00:00:00Z"

func mapGrokBilling(t *testing.T, body string) usage.UsageData {
	t.Helper()
	data := baseUsageData()
	if err := mapBilling(strings.NewReader(body), &data, time.Unix(1700000000, 0).UTC()); err != nil {
		t.Fatalf("mapBilling: %v", err)
	}
	return data
}

func findGrokBucket(data usage.UsageData, id string) (usage.Bucket, bool) {
	for _, bucket := range data.Buckets {
		if bucket.ID == id {
			return bucket, true
		}
	}
	return usage.Bucket{}, false
}

func Test_mapBilling_includes_on_demand_when_enabled_even_without_cap_or_used(t *testing.T) {
	// Given onDemandEnabled true but neither cap nor used is present
	body := `{"onDemandEnabled":true,"config":{"used":{"val":25},"monthlyLimit":{"val":100},"billingPeriodEnd":"` + onDemandTestReset + `"}}`

	// When
	data := mapGrokBilling(t, body)

	// Then the meter appears as status-only (zero utilization, enabled status)
	bucket, ok := findGrokBucket(data, usage.BucketOnDemand)
	if !ok {
		t.Fatalf("on_demand bucket = missing, want present when enabled")
	}
	if bucket.Utilization != 0 {
		t.Fatalf("on_demand Utilization = %v, want 0 (no truthful percent)", bucket.Utilization)
	}
	if bucket.StatusText == nil || *bucket.StatusText != "Pay as you go enabled" {
		t.Fatalf("on_demand StatusText = %v, want \"Pay as you go enabled\"", bucket.StatusText)
	}
	if data.SecondaryStatusText == nil || *data.SecondaryStatusText != "Pay as you go enabled" {
		t.Fatalf("SecondaryStatusText = %v, want enabled status", data.SecondaryStatusText)
	}
}

func Test_mapBilling_includes_on_demand_when_disabled_but_usage_is_non_zero(t *testing.T) {
	// Given onDemandEnabled false with non-zero used and no cap
	body := `{"onDemandEnabled":false,"config":{"used":{"val":25},"monthlyLimit":{"val":100},"billingPeriodEnd":"` + onDemandTestReset + `","onDemandUsed":{"val":15}}}`

	// When
	data := mapGrokBilling(t, body)

	// Then non-zero usage forces inclusion, status-only (no cap to plot)
	bucket, ok := findGrokBucket(data, usage.BucketOnDemand)
	if !ok {
		t.Fatalf("on_demand bucket = missing, want present when used > 0")
	}
	if bucket.Utilization != 0 {
		t.Fatalf("on_demand Utilization = %v, want 0 (no cap for truthful percent)", bucket.Utilization)
	}
	if bucket.StatusText == nil || *bucket.StatusText != "15 pay-as-you-go used" {
		t.Fatalf("on_demand StatusText = %v, want \"15 pay-as-you-go used\"", bucket.StatusText)
	}
}

func Test_mapBilling_hides_on_demand_when_explicitly_disabled_with_cap_and_zero_used(t *testing.T) {
	// Given onDemandEnabled explicitly false, positive cap, zero used
	body := `{"onDemandEnabled":false,"config":{"used":{"val":25},"monthlyLimit":{"val":100},"billingPeriodEnd":"` + onDemandTestReset + `","onDemandCap":{"val":40}}}`

	// When
	data := mapGrokBilling(t, body)

	// Then the cap alone must not resurrect the meter
	if _, ok := findGrokBucket(data, usage.BucketOnDemand); ok {
		t.Fatalf("on_demand bucket = present, want hidden when disabled with zero used")
	}
	if data.SecondaryStatusText == nil || *data.SecondaryStatusText != "Pay as you go disabled" {
		t.Fatalf("SecondaryStatusText = %v, want disabled status", data.SecondaryStatusText)
	}
}

func Test_mapBilling_shows_legacy_on_demand_when_enabled_is_absent_but_cap_is_positive(t *testing.T) {
	// Given legacy response: no onDemandEnabled, positive cap
	body := `{"config":{"used":{"val":25},"monthlyLimit":{"val":100},"billingPeriodEnd":"` + onDemandTestReset + `","onDemandCap":{"val":40}}}`

	// When
	data := mapGrokBilling(t, body)

	// Then the positive cap is an implied-enabled compatibility signal
	bucket, ok := findGrokBucket(data, usage.BucketOnDemand)
	if !ok {
		t.Fatalf("on_demand bucket = missing, want present for legacy positive cap")
	}
	if bucket.Utilization != 0 {
		t.Fatalf("on_demand Utilization = %v, want 0 (cap only, no truthful percent)", bucket.Utilization)
	}
	if bucket.StatusText == nil || *bucket.StatusText != "40 pay-as-you-go cap" {
		t.Fatalf("on_demand StatusText = %v, want \"40 pay-as-you-go cap\"", bucket.StatusText)
	}
}

func Test_mapBilling_computes_clamped_on_demand_utilization_from_used_and_cap(t *testing.T) {
	// Given used and positive cap that support a truthful percentage
	body := `{"onDemandEnabled":true,"config":{"used":{"val":25},"monthlyLimit":{"val":100},"billingPeriodEnd":"` + onDemandTestReset + `","onDemandUsed":{"val":10},"onDemandCap":{"val":40}}}`

	// When
	data := mapGrokBilling(t, body)

	// Then utilization is used/cap and status shows both figures
	bucket, ok := findGrokBucket(data, usage.BucketOnDemand)
	if !ok {
		t.Fatalf("on_demand bucket = missing, want present")
	}
	if bucket.Utilization != 25 {
		t.Fatalf("on_demand Utilization = %v, want 25", bucket.Utilization)
	}
	if bucket.StatusText == nil || *bucket.StatusText != "10 / 40 pay-as-you-go" {
		t.Fatalf("on_demand StatusText = %v, want \"10 / 40 pay-as-you-go\"", bucket.StatusText)
	}

	// And usage above the cap clamps to 100
	over := mapGrokBilling(t, `{"onDemandEnabled":true,"config":{"used":{"val":25},"monthlyLimit":{"val":100},"billingPeriodEnd":"`+onDemandTestReset+`","onDemandUsed":{"val":50},"onDemandCap":{"val":40}}}`)
	overBucket, ok := findGrokBucket(over, usage.BucketOnDemand)
	if !ok {
		t.Fatalf("on_demand bucket = missing for over-cap usage")
	}
	if overBucket.Utilization != 100 {
		t.Fatalf("on_demand Utilization = %v, want clamped 100", overBucket.Utilization)
	}
}

func Test_mapBilling_includes_on_demand_for_weekly_only_credits_payload(t *testing.T) {
	// Given a credits-format payload that omits monthlyLimit/used/billingPeriodEnd
	// but supplies a weekly currentPeriod plus on-demand fields
	body := `{"onDemandEnabled":true,"config":{"onDemandUsed":{"val":10},"onDemandCap":{"val":40},"creditUsagePercent":63.5,"currentPeriod":{"type":"USAGE_PERIOD_TYPE_WEEKLY","end":"2026-07-15T00:00:00Z"}}}`

	// When
	data := mapGrokBilling(t, body)

	// Then on-demand is present with truthful utilization even without legacy fields
	bucket, ok := findGrokBucket(data, usage.BucketOnDemand)
	if !ok {
		t.Fatalf("on_demand bucket = missing, want present for weekly-only credits payload")
	}
	if bucket.Utilization != 25 {
		t.Fatalf("on_demand Utilization = %v, want 25", bucket.Utilization)
	}
	if bucket.StatusText == nil || *bucket.StatusText != "10 / 40 pay-as-you-go" {
		t.Fatalf("on_demand StatusText = %v, want \"10 / 40 pay-as-you-go\"", bucket.StatusText)
	}
	if len(data.Buckets) != 2 || data.Buckets[0].ID != usage.BucketWeekly || data.Buckets[1].ID != usage.BucketOnDemand {
		t.Fatalf("Buckets = %#v, want canonical weekly,on_demand", data.Buckets)
	}
	if data.Buckets[1].StatusText == nil || *data.Buckets[1].StatusText != "10 / 40 pay-as-you-go" {
		t.Fatalf("on_demand bucket StatusText = %v, want preserved measured status", data.Buckets[1].StatusText)
	}
	if data.SecondaryStatusText != nil {
		t.Fatalf("SecondaryStatusText = %v, want nil to match Weekly legacy secondary", data.SecondaryStatusText)
	}
	if data.SecondaryLabel != "Weekly" || !data.ShowSecondary {
		t.Fatalf("legacy header = label %q shown %t, want Weekly/true", data.SecondaryLabel, data.ShowSecondary)
	}
	if data.Weekly.Utilization != 63.5 {
		t.Fatalf("Weekly.Utilization = %v, want 63.5", data.Weekly.Utilization)
	}
}

func Test_mapBilling_includes_enabled_on_demand_for_weekly_only_payload(t *testing.T) {
	// Given enabled-only on-demand with a weekly currentPeriod and no cap/used
	body := `{"onDemandEnabled":true,"config":{"creditUsagePercent":10,"currentPeriod":{"type":"USAGE_PERIOD_TYPE_WEEKLY","end":"2026-07-15T00:00:00Z"}}}`

	// When
	data := mapGrokBilling(t, body)

	// Then on-demand is present as status-only and owns the secondary status line
	bucket, ok := findGrokBucket(data, usage.BucketOnDemand)
	if !ok {
		t.Fatalf("on_demand bucket = missing, want present when enabled")
	}
	if bucket.Utilization != 0 {
		t.Fatalf("on_demand Utilization = %v, want 0", bucket.Utilization)
	}
	if bucket.StatusText == nil || *bucket.StatusText != "Pay as you go enabled" {
		t.Fatalf("on_demand StatusText = %v, want \"Pay as you go enabled\"", bucket.StatusText)
	}
	if len(data.Buckets) != 2 || data.Buckets[0].ID != usage.BucketWeekly || data.Buckets[1].ID != usage.BucketOnDemand {
		t.Fatalf("Buckets = %#v, want canonical weekly,on_demand", data.Buckets)
	}
	if data.Buckets[1].StatusText == nil || *data.Buckets[1].StatusText != "Pay as you go enabled" {
		t.Fatalf("on_demand bucket StatusText = %v, want preserved enabled status", data.Buckets[1].StatusText)
	}
	if data.SecondaryStatusText != nil {
		t.Fatalf("SecondaryStatusText = %v, want nil to match Weekly legacy secondary", data.SecondaryStatusText)
	}
}

func Test_mapBilling_places_on_demand_after_credits_in_canonical_order(t *testing.T) {
	// Given a measured on-demand meter with credits and no weekly period
	body := `{"onDemandEnabled":true,"config":{"used":{"val":25},"monthlyLimit":{"val":100},"billingPeriodEnd":"` + onDemandTestReset + `","onDemandUsed":{"val":10},"onDemandCap":{"val":40}}}`

	// When
	data := mapGrokBilling(t, body)

	// Then canonical order is credits then on_demand
	if len(data.Buckets) != 2 {
		t.Fatalf("Buckets = %#v, want credits and on_demand", data.Buckets)
	}
	if data.Buckets[0].ID != usage.BucketCredits || data.Buckets[1].ID != usage.BucketOnDemand {
		t.Fatalf("Buckets order = %s,%s, want credits,on_demand", data.Buckets[0].ID, data.Buckets[1].ID)
	}
	if data.SecondaryStatusText == nil || *data.SecondaryStatusText != "10 / 40 pay-as-you-go" {
		t.Fatalf("SecondaryStatusText = %v, want measured status", data.SecondaryStatusText)
	}
}
