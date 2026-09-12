//go:build !darwin

package app

import "context"

// Windows job objects and Linux PR_SET_PDEATHSIG are established by the client.
func desktopParentContext(parent context.Context) (context.Context, context.CancelFunc) {
	return parent, func() {}
}
