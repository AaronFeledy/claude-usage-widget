//go:build windows

package contract

import (
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
	"testing"
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
