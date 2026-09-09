//go:build windows

package contract

import (
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
	"testing"
	"time"
)

func TestWindowsJunctionInstallRootIsRejected(t *testing.T) {
	root := t.TempDir()
	target := filepath.Join(root, "real")
	junction := filepath.Join(root, "junction")
	if err := os.Mkdir(target, 0o700); err != nil {
		t.Fatal(err)
	}
	if output, err := exec.Command("cmd.exe", "/d", "/c", "mklink", "/J", junction, target).CombinedOutput(); err != nil {
		t.Fatalf("create native junction fixture: %v: %s", err, output)
	}
	if _, err := ValidateInstallRoot(junction); err == nil {
		t.Fatal("Windows junction install root was accepted")
	}
}

func TestWindowsProcessQueryDenialIsNotTreatedAsExit(t *testing.T) {
	original := openSynchronizeProcess
	openSynchronizeProcess = func(int) (uintptr, error) { return 0, syscall.ERROR_ACCESS_DENIED }
	defer func() { openSynchronizeProcess = original }()
	if processGone(1234) {
		t.Fatal("access-denied process query was treated as confirmed exit")
	}
}

func TestWindowsAbsentProcessIsTreatedAsExit(t *testing.T) {
	if !processGone(2147483647) {
		t.Fatal("an absent process was not treated as exited")
	}
}

func TestWindowsWritableBootstrapBackupRoundTrip(t *testing.T) {
	root := t.TempDir()
	entry := filepath.Join(root, "shortcuts", "Headroom.exe")
	if err := os.MkdirAll(filepath.Dir(entry), 0o700); err != nil {
		t.Fatal(err)
	}
	targets := stableBootstrapTargets(root, entry)
	for index, target := range targets {
		if err := os.WriteFile(target, []byte{byte(index + 1)}, 0o666); err != nil {
			t.Fatal(err)
		}
	}
	backup := filepath.Join(root, "transactions", "backup")
	if err := os.MkdirAll(backup, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := prepareBootstrapBackup(root, entry, backup); err != nil {
		t.Fatal(err)
	}
	for _, target := range targets {
		if err := os.WriteFile(target, []byte("changed"), 0o666); err != nil {
			t.Fatal(err)
		}
	}
	if err := restoreBootstrap(root, entry, backup); err != nil {
		t.Fatal(err)
	}
	if err := verifyBootstrapRestore(root, entry, backup); err != nil {
		t.Fatal(err)
	}
}

func TestWindowsExactWatchStopsOnlyVerifiedProcess(t *testing.T) {
	t.Setenv("HEADROOM_FIXTURE_SLEEP_MS", "60000")
	target := startFixturePath(t, fixtureExecutable)
	defer func() { _ = target.Process.Kill(); _, _ = target.Process.Wait() }()
	unrelated := startFixturePath(t, fixtureExecutable)
	defer func() { _ = unrelated.Process.Kill(); _, _ = unrelated.Process.Wait() }()
	token, err := captureProcessToken(target.Process.Pid, fixtureExecutable)
	if err != nil {
		t.Fatal(err)
	}
	watch, err := watchProcess(target.Process.Pid, fixtureExecutable, token)
	if err != nil {
		t.Fatal(err)
	}
	if err = watch.KillWait(3 * time.Second); err != nil {
		watch.Close()
		t.Fatal(err)
	}
	if err = watch.KillWait(time.Second); err != nil {
		watch.Close()
		t.Fatalf("already-signaled retained process handle was unsafe: %v", err)
	}
	watch.Close()
	_, _ = target.Process.Wait()
	if _, err = captureProcessToken(unrelated.Process.Pid, fixtureExecutable); err != nil {
		t.Fatalf("unrelated process was disturbed: %v", err)
	}
	if err = stopRecordedProcess(target.Process.Pid, fixtureExecutable, token, time.Second); err != nil {
		t.Fatalf("already-exited exact process was unsafe: %v", err)
	}
}
