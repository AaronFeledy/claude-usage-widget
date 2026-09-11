//go:build darwin

package main

import (
	"context"
	"os"
	"time"
)

// Darwin has no Linux PR_SET_PDEATHSIG. A private desktop server stops when
// its parent exits and the kernel reparents it. This is lifecycle cleanup;
// the TLS session certificate remains the authority for connection trust.
var desktopInitialParent = os.Getppid()

func desktopParentContext(parent context.Context) (context.Context, context.CancelFunc) {
	ctx, cancel := context.WithCancel(parent)
	if desktopInitialParent <= 1 || os.Getppid() != desktopInitialParent {
		cancel()
		return ctx, cancel
	}
	go func() {
		ticker := time.NewTicker(250 * time.Millisecond)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				if os.Getppid() != desktopInitialParent {
					cancel()
					return
				}
			}
		}
	}()
	return ctx, cancel
}
