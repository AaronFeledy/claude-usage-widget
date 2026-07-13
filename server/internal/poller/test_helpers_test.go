package poller

import (
	"context"
	"runtime"
	"sync"
	"sync/atomic"
	"time"

	"github.com/AaronFeledy/claude-usage-widget/server/internal/usage"
)

type providerResult struct {
	data       usage.UsageData
	err        error
	panicValue any
}

type sequenceProvider struct {
	name      string
	responses []providerResult
	calls     atomic.Int32
	mu        sync.Mutex
}

func (p *sequenceProvider) Name() string { return p.name }

func (p *sequenceProvider) Fetch(ctx context.Context) (usage.UsageData, error) {
	if err := ctx.Err(); err != nil {
		return usage.UsageData{ProviderName: p.name}, err
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	idx := int(p.calls.Add(1)) - 1
	if idx >= len(p.responses) {
		idx = len(p.responses) - 1
	}
	result := p.responses[idx]
	if result.panicValue != nil {
		panic(result.panicValue)
	}
	if result.data.ProviderName == "" {
		result.data.ProviderName = p.name
	}
	return result.data, result.err
}

type barrierProvider struct {
	name        string
	started     chan<- string
	release     <-chan struct{}
	utilization float64
}

func (p *barrierProvider) Name() string { return p.name }

func (p *barrierProvider) Fetch(ctx context.Context) (usage.UsageData, error) {
	p.started <- p.name
	select {
	case <-p.release:
	case <-ctx.Done():
		return usage.UsageData{ProviderName: p.name}, ctx.Err()
	}
	return usage.UsageData{ProviderName: p.name, Current: usage.UsageBucket{Utilization: p.utilization}}, nil
}

type serializedProvider struct {
	name    string
	started chan<- struct{}
	release <-chan struct{}
	calls   atomic.Int32
}

func (p *serializedProvider) Name() string { return p.name }

func (p *serializedProvider) Fetch(ctx context.Context) (usage.UsageData, error) {
	call := p.calls.Add(1)
	p.started <- struct{}{}
	select {
	case <-p.release:
	case <-ctx.Done():
		return usage.UsageData{ProviderName: p.name}, ctx.Err()
	}
	return usage.UsageData{ProviderName: p.name, Current: usage.UsageBucket{Utilization: float64(call)}}, nil
}

type cancelProvider struct {
	name    string
	started chan struct{}
}

func (p *cancelProvider) Name() string { return p.name }

func (p *cancelProvider) Fetch(ctx context.Context) (usage.UsageData, error) {
	close(p.started)
	<-ctx.Done()
	return usage.UsageData{ProviderName: p.name}, ctx.Err()
}

type blockingUpdateProvider struct {
	name    string
	started chan struct{}
	release chan struct{}
	calls   atomic.Int32
}

type namePanicProvider struct {
	name      string
	nameCalls atomic.Int32
}

func newNamePanicProvider(name string) *namePanicProvider {
	return &namePanicProvider{name: name}
}

func (p *namePanicProvider) Name() string {
	if p.nameCalls.Add(1) > 1 {
		panic("provider name should not be called after registration")
	}
	return p.name
}

func (p *namePanicProvider) Fetch(context.Context) (usage.UsageData, error) {
	panic("fetch panic with secret credential material")
}

func (p *blockingUpdateProvider) Name() string { return p.name }

func (p *blockingUpdateProvider) Fetch(ctx context.Context) (usage.UsageData, error) {
	call := p.calls.Add(1)
	if call > 1 {
		p.started <- struct{}{}
		select {
		case <-p.release:
		case <-ctx.Done():
			return usage.UsageData{ProviderName: p.name}, ctx.Err()
		}
	}
	return usage.UsageData{ProviderName: p.name, Current: usage.UsageBucket{Utilization: float64(call * 10)}}, nil
}

type fixedClock struct{ now time.Time }

func (c fixedClock) Now() time.Time { return c.now.UTC() }

type stepClock struct {
	values []time.Time
	next   atomic.Int32
}

func (c *stepClock) Now() time.Time {
	idx := int(c.next.Add(1)) - 1
	if idx >= len(c.values) {
		idx = len(c.values) - 1
	}
	return c.values[idx].UTC()
}

type advanceClock struct {
	mu  sync.Mutex
	now time.Time
}

func (c *advanceClock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.now.UTC()
}

func (c *advanceClock) next(delta time.Duration) time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.now = c.now.Add(delta)
	return c.now
}

type manualTicker struct {
	c       chan time.Time
	stopped atomic.Bool
}

func newManualTicker() *manualTicker { return &manualTicker{c: make(chan time.Time, 1)} }

func (t *manualTicker) C() <-chan time.Time { return t.c }

func (t *manualTicker) Stop() { t.stopped.Store(true) }

func (t *manualTicker) tick(now time.Time) { t.c <- now }

func entryNames(entries []Entry) []string {
	names := make([]string, 0, len(entries))
	for _, entry := range entries {
		names = append(names, entry.Data.ProviderName)
	}
	return names
}

func strPtr(value string) *string { return &value }

func deref(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}

func runtimeGosched() { runtime.Gosched() }

var _ usage.Provider = (*sequenceProvider)(nil)
var _ usage.Provider = (*barrierProvider)(nil)
var _ usage.Provider = (*serializedProvider)(nil)
var _ usage.Provider = (*blockingUpdateProvider)(nil)
var _ usage.Provider = (*namePanicProvider)(nil)
