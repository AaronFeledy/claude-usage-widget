//go:build darwin

package contract

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"
)

func TestDarwinKernelExecutableIdentity(t *testing.T) {
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	executable, err = filepath.EvalSymlinks(executable)
	if err != nil {
		t.Fatal(err)
	}
	actual, err := darwinProcessPath(os.Getpid())
	if err != nil || actual != executable {
		t.Fatalf("kernel executable: %q, %v; want %q", actual, err, executable)
	}
	token, err := captureProcessToken(os.Getpid(), executable)
	if err != nil || token == "" {
		t.Fatalf("capture self: %q, %v", token, err)
	}
	if _, err = captureProcessToken(os.Getpid(), executable+"-wrong"); err == nil {
		t.Fatal("wrong executable accepted")
	}
	if _, err = watchProcess(os.Getpid(), executable, token+"-wrong"); err == nil {
		t.Fatal("wrong start identity accepted")
	}
	if _, err = watchProcess(os.Getpid(), executable, token); err != nil {
		t.Fatal(err)
	}
}

func TestDarwinProcessGoneRecognizesReapedProcess(t *testing.T) {
	process := exec.Command(fixtureExecutable)
	if err := process.Start(); err != nil {
		t.Fatal(err)
	}
	pid := process.Process.Pid
	if err := process.Wait(); err != nil {
		t.Fatal(err)
	}
	if !processGone(pid) {
		t.Fatal("fully reaped process was reported as live")
	}
}

func TestDarwinWatchWaitsForVerifiedProcessExit(t *testing.T) {
	t.Setenv("HEADROOM_FIXTURE_SLEEP_MS", "60000")
	process := exec.Command(fixtureExecutable)
	process.Env = os.Environ()
	if err := process.Start(); err != nil {
		t.Fatal(err)
	}
	defer func() {
		_ = process.Process.Kill()
		_, _ = process.Process.Wait()
	}()
	token, err := captureProcessToken(process.Process.Pid, fixtureExecutable)
	if err != nil {
		t.Fatal(err)
	}
	watch, err := watchProcess(process.Process.Pid, fixtureExecutable, token)
	if err != nil {
		t.Fatal(err)
	}
	defer watch.Close()
	if err = process.Process.Kill(); err != nil {
		t.Fatal(err)
	}
	if err = watch.Wait(3 * time.Second); err != nil {
		t.Fatal(err)
	}
}

func TestDarwinUnavailablePathIsDistinctFromWrongIdentity(t *testing.T) {
	if _, err := captureProcessToken(os.Getpid(), filepath.Join(t.TempDir(), "wrong")); err == nil || errors.Is(err, errDarwinExecutableUnavailable) {
		t.Fatalf("wrong live executable was not rejected distinctly: %v", err)
	}
}
