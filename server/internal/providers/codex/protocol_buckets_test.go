package codex

import (
	"testing"
	"time"
)

func Test_parseUsage_emits_buckets_matching_legacy_header(t *testing.T) {
	// Given
	body := []byte(`{"rate_limit":{"primary_window":{"used_percent":12.5,"reset_at":1783872000},"secondary_window":{"used_percent":80,"reset_at":1784476800}}}`)

	// When
	data, err := parseUsage(body, baseUsage())

	// Then
	if err != nil {
		t.Fatalf("parseUsage returned error: %v", err)
	}
	if len(data.Buckets) != 2 {
		t.Fatalf("Buckets = %#v, want session and weekly", data.Buckets)
	}
	session, weekly := data.Buckets[0], data.Buckets[1]
	if session.ID != "session" || session.Label != data.PrimaryLabel || session.Utilization != data.Current.Utilization {
		t.Fatalf("session bucket = %#v, current = %#v label = %q", session, data.Current, data.PrimaryLabel)
	}
	if weekly.ID != "weekly" || weekly.Label != data.SecondaryLabel || weekly.Utilization != data.Weekly.Utilization {
		t.Fatalf("weekly bucket = %#v, weekly header = %#v label = %q", weekly, data.Weekly, data.SecondaryLabel)
	}
	if session.ResetsAt == nil || !session.ResetsAt.Equal(time.Unix(1783872000, 0).UTC()) {
		t.Fatalf("session ResetsAt = %v", session.ResetsAt)
	}
}
