package codex

import (
	"encoding/json"
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

func Test_parseUsage_normalizes_rate_limit_reset_credits(t *testing.T) {
	tests := []struct {
		name     string
		metadata string
		want     *int64
	}{
		{name: "positive", metadata: `{"available_count":3}`, want: int64Pointer(3)},
		{name: "known zero", metadata: `{"available_count":0}`, want: int64Pointer(0)},
		{name: "largest desktop safe integer", metadata: `{"available_count":9007199254740991}`, want: int64Pointer(9007199254740991)},
		{name: "missing", metadata: ""},
		{name: "null", metadata: `null`},
		{name: "missing count", metadata: `{}`},
		{name: "null count", metadata: `{"available_count":null}`},
		{name: "wrong shape", metadata: `[]`},
		{name: "negative", metadata: `{"available_count":-1}`},
		{name: "fractional", metadata: `{"available_count":1.5}`},
		{name: "numeric string", metadata: `{"available_count":"2"}`},
		{name: "beyond desktop safe integer", metadata: `{"available_count":9007199254740992}`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			root := map[string]json.RawMessage{
				"rate_limit": json.RawMessage(`{"primary_window":{"used_percent":12.5}}`),
			}
			if tt.metadata != "" {
				root["rate_limit_reset_credits"] = json.RawMessage(tt.metadata)
			}
			body, err := json.Marshal(root)
			if err != nil {
				t.Fatalf("marshal fixture: %v", err)
			}
			data, err := parseUsage(body, baseUsage())
			if err != nil {
				t.Fatalf("parseUsage returned error: %v", err)
			}
			if tt.want == nil {
				if data.RateLimitResetCredits != nil {
					t.Fatalf("RateLimitResetCredits = %+v, want nil", data.RateLimitResetCredits)
				}
				return
			}
			if data.RateLimitResetCredits == nil || data.RateLimitResetCredits.AvailableCount != *tt.want {
				t.Fatalf("RateLimitResetCredits = %+v, want available_count %d", data.RateLimitResetCredits, *tt.want)
			}
		})
	}
}

func int64Pointer(value int64) *int64 { return &value }
