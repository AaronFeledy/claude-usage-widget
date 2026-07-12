package poller

import (
	"context"
	"reflect"
	"testing"
	"time"

	"github.com/AaronFeledy/claude-usage-widget/server/internal/usage"
)

func Test_Poller_PollAll_fetches_enabled_providers_concurrently_and_omits_disabled(t *testing.T) {
	// Given
	clock := fixedClock{now: time.Date(2026, 7, 12, 10, 0, 0, 0, time.FixedZone("offset", -4*60*60))}
	poller := New(Options{Clock: clock})
	started := make(chan string, 3)
	release := make(chan struct{})
	providers := []*barrierProvider{
		{name: "Zulu", started: started, release: release, utilization: 30},
		{name: "Alpha", started: started, release: release, utilization: 10},
		{name: "bravo", started: started, release: release, utilization: 20},
	}
	for _, provider := range providers {
		if err := poller.Register(provider, true); err != nil {
			t.Fatalf("Register returned error: %v", err)
		}
	}
	if err := poller.Register(&barrierProvider{name: "Disabled", started: started, release: release}, false); err != nil {
		t.Fatalf("Register disabled returned error: %v", err)
	}
	done := make(chan []Entry, 1)

	// When
	go func() { done <- poller.PollAll(context.Background()) }()

	// Then
	seen := make(map[string]bool)
	for range providers {
		seen[<-started] = true
	}
	if !reflect.DeepEqual(seen, map[string]bool{"Zulu": true, "Alpha": true, "bravo": true}) {
		t.Fatalf("started providers = %v", seen)
	}
	close(release)
	entries := <-done
	if got := entryNames(entries); !reflect.DeepEqual(got, []string{"Alpha", "bravo", "Zulu"}) {
		t.Fatalf("entry names = %v, want deterministic enabled order", got)
	}
	for _, entry := range entries {
		if !entry.FetchedAt.Equal(clock.now.UTC()) {
			t.Fatalf("FetchedAt = %v, want %v", entry.FetchedAt, clock.now.UTC())
		}
	}
}

func Test_Poller_Snapshot_and_Get_return_defensive_copies_without_fetching(t *testing.T) {
	// Given
	reset := time.Date(2026, 7, 12, 12, 0, 0, 0, time.UTC)
	subtitle := "team"
	provider := &sequenceProvider{name: "Cursor", responses: []providerResult{{data: usage.UsageData{ProviderName: "Cursor", Subtitle: &subtitle, Current: usage.UsageBucket{Utilization: 42, ResetsAt: &reset}}}}}
	poller := New(Options{Clock: fixedClock{now: reset}})
	if err := poller.Register(provider, true); err != nil {
		t.Fatalf("Register returned error: %v", err)
	}
	poller.PollAll(context.Background())

	// When
	snapshot := poller.Snapshot()
	got, ok := poller.Get("cursor")
	mutatedSubtitle := "mutated"
	mutatedReset := reset.Add(time.Hour)
	snapshot[0].Data.Subtitle = &mutatedSubtitle
	snapshot[0].Data.Current.ResetsAt = &mutatedReset
	got.Data.Subtitle = &mutatedSubtitle
	got.Data.Current.ResetsAt = &mutatedReset
	second, secondOK := poller.Get("CURSOR")

	// Then
	if !ok || !secondOK {
		t.Fatalf("Get ok=%v secondOK=%v, want true", ok, secondOK)
	}
	if provider.calls.Load() != 1 {
		t.Fatalf("Fetch calls = %d, want cache reads to avoid upstream work", provider.calls.Load())
	}
	if second.Data.Subtitle == nil || *second.Data.Subtitle != "team" {
		t.Fatalf("Subtitle after mutation = %v", second.Data.Subtitle)
	}
	if second.Data.Current.ResetsAt == nil || !second.Data.Current.ResetsAt.Equal(reset) {
		t.Fatalf("ResetsAt after mutation = %v", second.Data.Current.ResetsAt)
	}
}

func Test_Poller_Snapshot_returns_last_entry_while_provider_fetch_is_blocked(t *testing.T) {
	// Given
	provider := &blockingUpdateProvider{
		name:    "Cursor",
		started: make(chan struct{}),
		release: make(chan struct{}),
	}
	poller := New(Options{Clock: fixedClock{now: time.Date(2026, 7, 12, 10, 0, 0, 0, time.UTC)}})
	if err := poller.Register(provider, true); err != nil {
		t.Fatalf("Register returned error: %v", err)
	}
	first, _, _ := poller.PollProvider(context.Background(), "cursor")
	if first.Data.Current.Utilization != 10 {
		t.Fatalf("initial utilization = %v", first.Data.Current.Utilization)
	}
	blockedDone := make(chan Entry, 1)

	// When
	go func() { entry, _, _ := poller.PollProvider(context.Background(), "cursor"); blockedDone <- entry }()
	<-provider.started
	snapshot := poller.Snapshot()
	close(provider.release)
	updated := <-blockedDone

	// Then
	if got := snapshot[0].Data.Current.Utilization; got != 10 {
		t.Fatalf("snapshot utilization while fetch blocked = %v, want last cache value", got)
	}
	if updated.Data.Current.Utilization != 20 {
		t.Fatalf("updated utilization = %v", updated.Data.Current.Utilization)
	}
}

func Test_Poller_PollProvider_coalesces_same_provider_fetches_but_allows_other_providers_parallel(t *testing.T) {
	// Given
	poller := New(Options{Clock: fixedClock{now: time.Date(2026, 7, 12, 10, 0, 0, 0, time.UTC)}})
	slowStarted := make(chan struct{}, 2)
	slowRelease := make(chan struct{})
	slow := &serializedProvider{name: "Cursor", started: slowStarted, release: slowRelease}
	fast := &sequenceProvider{name: "Claude", responses: []providerResult{{data: usage.UsageData{ProviderName: "Claude", Current: usage.UsageBucket{Utilization: 7}}}}}
	if err := poller.Register(slow, true); err != nil {
		t.Fatalf("Register slow returned error: %v", err)
	}
	if err := poller.Register(fast, true); err != nil {
		t.Fatalf("Register fast returned error: %v", err)
	}
	firstDone := make(chan Entry, 1)
	secondDone := make(chan Entry, 1)
	fastDone := make(chan Entry, 1)

	// When
	go func() { entry, _, _ := poller.PollProvider(context.Background(), "cursor"); firstDone <- entry }()
	<-slowStarted
	go func() { entry, _, _ := poller.PollProvider(context.Background(), "CURSOR"); secondDone <- entry }()
	go func() { entry, _, _ := poller.PollProvider(context.Background(), "claude"); fastDone <- entry }()

	// Then
	fastEntry := <-fastDone
	if fastEntry.Data.Current.Utilization != 7 {
		t.Fatalf("fast provider utilization = %v", fastEntry.Data.Current.Utilization)
	}
	select {
	case <-slowStarted:
		t.Fatal("same provider fetch overlapped")
	default:
	}
	close(slowRelease)
	first := <-firstDone
	second := <-secondDone
	if slow.calls.Load() != 2 {
		t.Fatalf("same provider calls = %d, want sequential fetches without overlap", slow.calls.Load())
	}
	if first.Data.Current.Utilization != 1 || second.Data.Current.Utilization != 2 {
		t.Fatalf("sequential utilizations = %v %v", first.Data.Current.Utilization, second.Data.Current.Utilization)
	}
}
