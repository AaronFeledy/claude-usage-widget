package usage_test

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/AaronFeledy/claude-usage-widget/server/internal/usage"
)

func Test_UsageData_manual_JSON_surface_QA(t *testing.T) {
	// Given
	reset := time.Date(2026, 7, 12, 22, 30, 0, 0, time.UTC)
	errorText := "reauth required"
	populated := usage.UsageData{
		ProviderName:   "Claude",
		PrimaryLabel:   "Current Session",
		SecondaryLabel: "Weekly",
		ShowSecondary:  true,
		Current:        usage.UsageBucket{Utilization: 42, ResetsAt: &reset},
		Weekly:         usage.UsageBucket{Utilization: 71},
	}
	failed := usage.UsageData{ProviderName: "Claude", Error: &errorText, NeedsReauth: true}

	// When
	populatedJSON, populatedErr := json.Marshal(populated)
	failedJSON, failedErr := json.Marshal(failed)

	// Then
	if populatedErr != nil {
		t.Fatalf("marshal populated usage: %v", populatedErr)
	}
	if failedErr != nil {
		t.Fatalf("marshal failed usage: %v", failedErr)
	}
	assertUsageKeys(t, populatedJSON)
	assertUsageKeys(t, failedJSON)
	assertJSONField(t, populatedJSON, "is_success", `true`)
	assertJSONField(t, failedJSON, "is_success", `false`)
	t.Logf("POPULATED_JSON: %s", populatedJSON)
	t.Logf("ERROR_JSON: %s", failedJSON)
}
