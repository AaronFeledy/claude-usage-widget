package poller

import (
	"errors"
	"fmt"
)

var (
	ErrInvalidInterval = errors.New("poller: invalid interval")
	ErrInvalidProvider = errors.New("poller: invalid provider")
)

type invalidIntervalError struct {
	interval string
}

func (e invalidIntervalError) Error() string {
	return fmt.Sprintf("%s: %s", ErrInvalidInterval, e.interval)
}

func (e invalidIntervalError) Unwrap() error { return ErrInvalidInterval }

type invalidProviderError struct {
	reason string
}

func (e invalidProviderError) Error() string {
	return fmt.Sprintf("%s: %s", ErrInvalidProvider, e.reason)
}

func (e invalidProviderError) Unwrap() error { return ErrInvalidProvider }
