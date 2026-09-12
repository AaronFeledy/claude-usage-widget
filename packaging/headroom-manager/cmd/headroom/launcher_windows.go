//go:build windows

package main

import (
	"context"
	"errors"
	"os"
	"strconv"

	"github.com/AaronFeledy/claude-usage-widget/packaging/headroom-manager/contract"
)

func launcherContext(ctx context.Context) (context.Context, context.CancelFunc, error) {
	value, path, token := os.Getenv("HEADROOM_PUBLIC_LAUNCHER_PID"), os.Getenv("HEADROOM_PUBLIC_LAUNCHER_PATH"), os.Getenv("HEADROOM_PUBLIC_LAUNCHER_TOKEN")
	if value == "" && path == "" && token == "" {
		return ctx, func() {}, nil
	}
	pid, err := strconv.Atoi(value)
	expected, _, identityErr := contract.ActiveExecutableForRole(os.Getenv("HEADROOM_INSTALL_ROOT"), contract.RolePublicLauncher)
	if err != nil || pid <= 0 || pid != os.Getppid() || identityErr != nil || !sameExecutable(expected, path) {
		return nil, nil, errors.New("Windows CLI requires its verified public launcher parent")
	}
	return contract.LauncherLifetimeContext(ctx, pid, path, token)
}
