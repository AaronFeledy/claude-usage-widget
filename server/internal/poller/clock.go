package poller

import "time"

type Clock interface {
	Now() time.Time
}

type realClock struct{}

func (realClock) Now() time.Time { return time.Now().UTC() }

type Ticker interface {
	C() <-chan time.Time
	Stop()
}

type tickerFunc func(time.Duration) Ticker

type realTicker struct {
	ticker *time.Ticker
}

func newRealTicker(interval time.Duration) Ticker {
	return realTicker{ticker: time.NewTicker(interval)}
}

func (t realTicker) C() <-chan time.Time { return t.ticker.C }

func (t realTicker) Stop() { t.ticker.Stop() }
