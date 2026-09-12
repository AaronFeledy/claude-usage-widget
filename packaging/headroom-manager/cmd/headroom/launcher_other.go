//go:build !windows

package main

import "context"

func launcherContext(ctx context.Context) (context.Context, context.CancelFunc, error) {
	return ctx, func() {}, nil
}
