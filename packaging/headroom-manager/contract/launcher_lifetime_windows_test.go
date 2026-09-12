//go:build windows

package contract

import (
	"context"
	"os"
	"os/exec"
	"testing"
	"time"
)

func TestLauncherLossCancelsCLIWithoutKillingIndependentHelper(t *testing.T) {
	t.Setenv("HEADROOM_FIXTURE_SLEEP_MS", "60000")
	launcher := exec.Command(fixtureExecutable)
	if err := launcher.Start(); err != nil {
		t.Fatal(err)
	}
	defer func() { launcher.Process.Kill(); launcher.Wait() }()
	helper := exec.Command(fixtureExecutable)
	if err := helper.Start(); err != nil {
		t.Fatal(err)
	}
	defer func() { helper.Process.Kill(); helper.Wait() }()
	token, err := captureProcessToken(launcher.Process.Pid, fixtureExecutable)
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := LauncherLifetimeContext(context.Background(), launcher.Process.Pid, fixtureExecutable, "wrong"); err == nil {
		t.Fatal("accepted wrong launcher creation identity")
	}
	ctx, stop, err := LauncherLifetimeContext(context.Background(), launcher.Process.Pid, fixtureExecutable, token)
	if err != nil {
		t.Fatal(err)
	}
	defer stop()
	launcher.Process.Kill()
	select {
	case <-ctx.Done():
	case <-time.After(2 * time.Second):
		t.Fatal("launcher loss did not cancel CLI context")
	}
	if processGone(helper.Process.Pid) {
		t.Fatal("independent updater helper was killed")
	}
}

func TestLauncherWatchCleanupDoesNotTerminateLauncher(t *testing.T) {
	executable, _ := os.Executable()
	token, err := CurrentProcessToken()
	if err != nil {
		t.Fatal(err)
	}
	ctx, stop, err := LauncherLifetimeContext(context.Background(), os.Getpid(), executable, token)
	if err != nil {
		t.Fatal(err)
	}
	stop()
	if ctx.Err() != context.Canceled {
		t.Fatal("monitor cleanup did not cancel")
	}
	if processGone(os.Getpid()) {
		t.Fatal("monitor terminated launcher")
	}
}
