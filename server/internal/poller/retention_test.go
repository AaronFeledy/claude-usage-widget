package poller

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/AaronFeledy/claude-usage-widget/server/internal/usage"
)

func Test_Poller_PollAll_retains_last_good_usage_when_later_provider_returns_error_or_panics(t *testing.T) {
	// Given
	reset := time.Date(2026, 7, 12, 15, 0, 0, 0, time.UTC)
	title := "pro"
	status := "retrying"
	reauth := "cursor login"
	poller := New(Options{Clock: &stepClock{values: []time.Time{reset, reset.Add(time.Minute), reset.Add(2 * time.Minute)}}})
	provider := &sequenceProvider{name: "Cursor", responses: []providerResult{
		{data: usage.UsageData{ProviderName: "Cursor", PrimaryLabel: "Requests", SecondaryLabel: "Month", ShowSecondary: true, Subtitle: &title, Current: usage.UsageBucket{Utilization: 55, ResetsAt: &reset}, Weekly: usage.UsageBucket{Utilization: 66}}},
		{data: usage.UsageData{ProviderName: "Cursor", PrimaryLabel: "Requests", Error: strPtr("AUTH_EXPIRED"), NeedsReauth: true, ReauthCommand: &reauth, PrimaryStatusText: &status}},
		{panicValue: "secret token abc123"},
	}}
	if err := poller.Register(provider, true); err != nil {
		t.Fatalf("Register returned error: %v", err)
	}

	// When
	first := poller.PollAll(context.Background())[0]
	second := poller.PollAll(context.Background())[0]
	third := poller.PollAll(context.Background())[0]

	// Then
	if first.Data.Error != nil || first.Data.Current.Utilization != 55 {
		t.Fatalf("first entry = %+v", first)
	}
	if second.Data.Current.Utilization != 55 || second.Data.Weekly.Utilization != 66 || second.Data.Subtitle == nil || *second.Data.Subtitle != "pro" {
		t.Fatalf("expected error retained last-good usage, got %+v", second.Data)
	}
	if second.Data.Error == nil || *second.Data.Error != "AUTH_EXPIRED" || !second.Data.NeedsReauth || second.Data.ReauthCommand == nil || *second.Data.ReauthCommand != reauth || second.Data.PrimaryStatusText == nil || *second.Data.PrimaryStatusText != status {
		t.Fatalf("expected error overlay, got %+v", second.Data)
	}
	if third.Data.Current.Utilization != 55 || third.Data.Error == nil || strings.Contains(*third.Data.Error, "secret") || strings.Contains(*third.Data.Error, "token") {
		t.Fatalf("panic overlay leaked or wiped last-good data: %+v", third.Data)
	}
}

func Test_Poller_PollAll_keeps_base_error_data_when_provider_has_no_prior_success(t *testing.T) {
	// Given
	reauth := "codex"
	provider := &sequenceProvider{name: "Codex", responses: []providerResult{{data: usage.UsageData{ProviderName: "Codex", PrimaryLabel: "5-Hour", SecondaryLabel: "Weekly", ReauthCommand: &reauth, Error: strPtr("Run `codex` to sign in."), NeedsReauth: true}}}}
	poller := New(Options{Clock: fixedClock{now: time.Date(2026, 7, 12, 10, 0, 0, 0, time.UTC)}})
	if err := poller.Register(provider, true); err != nil {
		t.Fatalf("Register returned error: %v", err)
	}

	// When
	entry := poller.PollAll(context.Background())[0]

	// Then
	if entry.Data.ProviderName != "Codex" || entry.Data.Error == nil || *entry.Data.Error != "Run `codex` to sign in." || !entry.Data.NeedsReauth {
		t.Fatalf("entry = %+v", entry)
	}
}

func Test_Poller_PollAll_recovers_provider_panic_and_other_providers_succeed(t *testing.T) {
	// Given
	poller := New(Options{Clock: fixedClock{now: time.Date(2026, 7, 12, 10, 0, 0, 0, time.UTC)}})
	if err := poller.Register(&sequenceProvider{name: "Panic", responses: []providerResult{{panicValue: "boom with credential path"}}}, true); err != nil {
		t.Fatalf("Register panic provider returned error: %v", err)
	}
	if err := poller.Register(&sequenceProvider{name: "Claude", responses: []providerResult{{data: usage.UsageData{ProviderName: "Claude", Current: usage.UsageBucket{Utilization: 12}}}}}, true); err != nil {
		t.Fatalf("Register claude returned error: %v", err)
	}

	// When
	entries := poller.PollAll(context.Background())

	// Then
	got := map[string]Entry{}
	for _, entry := range entries {
		got[entry.Data.ProviderName] = entry
	}
	if got["Claude"].Data.Current.Utilization != 12 {
		t.Fatalf("Claude entry = %+v", got["Claude"])
	}
	panicErr := got["Panic"].Data.Error
	if panicErr == nil || *panicErr != "Provider fetch failed. Will retry." {
		t.Fatalf("panic error = %v", panicErr)
	}
}

func Test_Poller_PollAll_preserves_partial_auth_overlay_when_fetch_returns_error(t *testing.T) {
	// Given
	reset := time.Date(2026, 7, 12, 15, 0, 0, 0, time.UTC)
	reauth := "claude"
	primaryStatus := "credentials need refresh"
	secondaryStatus := "weekly retained"
	provider := &sequenceProvider{name: "Claude", responses: []providerResult{
		{data: usage.UsageData{ProviderName: "Claude", Current: usage.UsageBucket{Utilization: 71, ResetsAt: &reset}, Weekly: usage.UsageBucket{Utilization: 22}}},
		{data: usage.UsageData{ProviderName: "Claude", NeedsReauth: true, ReauthCommand: &reauth, PrimaryStatusText: &primaryStatus, SecondaryStatusText: &secondaryStatus}, err: transientProviderError{message: "secret token expired"}},
	}}
	poller := New(Options{Clock: fixedClock{now: reset}})
	if err := poller.Register(provider, true); err != nil {
		t.Fatalf("Register returned error: %v", err)
	}
	poller.PollAll(context.Background())

	// When
	entry := poller.PollAll(context.Background())[0]

	// Then
	if entry.Data.Current.Utilization != 71 || entry.Data.Weekly.Utilization != 22 {
		t.Fatalf("last-good utilization was not retained: %+v", entry.Data)
	}
	if !entry.Data.NeedsReauth || entry.Data.ReauthCommand == nil || *entry.Data.ReauthCommand != reauth {
		t.Fatalf("reauth overlay was not preserved: %+v", entry.Data)
	}
	if entry.Data.PrimaryStatusText == nil || *entry.Data.PrimaryStatusText != primaryStatus || entry.Data.SecondaryStatusText == nil || *entry.Data.SecondaryStatusText != secondaryStatus {
		t.Fatalf("status overlay was not preserved: %+v", entry.Data)
	}
	if entry.Data.Error == nil || *entry.Data.Error != "Provider fetch failed. Will retry." || strings.Contains(*entry.Data.Error, "secret") {
		t.Fatalf("error was not sanitized: %+v", entry.Data)
	}
}

func Test_Poller_PollAll_preserves_partial_auth_overlay_without_prior_success(t *testing.T) {
	// Given
	reauth := "claude"
	primaryStatus := "credentials missing"
	provider := &sequenceProvider{name: "Claude", responses: []providerResult{
		{data: usage.UsageData{ProviderName: "Claude", PrimaryLabel: "Current Session", SecondaryLabel: "Weekly", ShowSecondary: true, NeedsReauth: true, ReauthCommand: &reauth, PrimaryStatusText: &primaryStatus}, err: transientProviderError{message: "credential path /home/user/.claude"}},
	}}
	poller := New(Options{Clock: fixedClock{now: time.Date(2026, 7, 12, 15, 0, 0, 0, time.UTC)}})
	if err := poller.Register(provider, true); err != nil {
		t.Fatalf("Register returned error: %v", err)
	}

	// When
	entry := poller.PollAll(context.Background())[0]

	// Then
	if !entry.Data.NeedsReauth || entry.Data.ReauthCommand == nil || *entry.Data.ReauthCommand != reauth || entry.Data.PrimaryStatusText == nil || *entry.Data.PrimaryStatusText != primaryStatus {
		t.Fatalf("partial auth overlay was not preserved: %+v", entry.Data)
	}
	if entry.Data.Error == nil || *entry.Data.Error != "Provider fetch failed. Will retry." || strings.Contains(*entry.Data.Error, "/home/user") {
		t.Fatalf("error was not sanitized: %+v", entry.Data)
	}
}

func Test_Poller_PollAll_clears_stale_reauth_when_fetch_error_partial_has_no_reauth(t *testing.T) {
	// Given
	reauth := "claude"
	provider := &sequenceProvider{name: "Claude", responses: []providerResult{
		{data: usage.UsageData{ProviderName: "Claude", Current: usage.UsageBucket{Utilization: 71}, NeedsReauth: true, ReauthCommand: &reauth}},
		{data: usage.UsageData{ProviderName: "Claude", NeedsReauth: false}, err: transientProviderError{message: "network failure"}},
	}}
	poller := New(Options{Clock: fixedClock{now: time.Date(2026, 7, 12, 15, 0, 0, 0, time.UTC)}})
	if err := poller.Register(provider, true); err != nil {
		t.Fatalf("Register returned error: %v", err)
	}
	poller.PollAll(context.Background())

	// When
	entry := poller.PollAll(context.Background())[0]

	// Then
	if entry.Data.NeedsReauth || entry.Data.ReauthCommand != nil {
		t.Fatalf("stale reauth was not cleared: %+v", entry.Data)
	}
	if entry.Data.Current.Utilization != 71 {
		t.Fatalf("last-good utilization was not retained: %+v", entry.Data)
	}
}

func Test_Poller_PollAll_panic_recovery_uses_registered_name_without_calling_provider_name(t *testing.T) {
	// Given
	poller := New(Options{Clock: fixedClock{now: time.Date(2026, 7, 12, 10, 0, 0, 0, time.UTC)}})
	provider := newNamePanicProvider("PanicName")
	if err := poller.Register(provider, true); err != nil {
		t.Fatalf("Register panic-name provider returned error: %v", err)
	}
	if err := poller.Register(&sequenceProvider{name: "Claude", responses: []providerResult{{data: usage.UsageData{ProviderName: "Claude", Current: usage.UsageBucket{Utilization: 12}}}}}, true); err != nil {
		t.Fatalf("Register claude returned error: %v", err)
	}

	// When
	entries := poller.PollAll(context.Background())

	// Then
	got := map[string]Entry{}
	for _, entry := range entries {
		got[entry.Data.ProviderName] = entry
	}
	if got["Claude"].Data.Current.Utilization != 12 {
		t.Fatalf("Claude entry = %+v", got["Claude"])
	}
	panicErr := got["PanicName"].Data.Error
	if panicErr == nil || *panicErr != "Provider fetch failed. Will retry." {
		t.Fatalf("panic entry = %+v", got["PanicName"])
	}
	if provider.nameCalls.Load() != 1 {
		t.Fatalf("Name calls = %d, want registration-only lookup", provider.nameCalls.Load())
	}
}

type transientProviderError struct{ message string }

func (e transientProviderError) Error() string { return e.message }

var _ error = transientProviderError{}
