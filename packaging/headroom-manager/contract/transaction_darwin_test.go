//go:build darwin

package contract

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
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
