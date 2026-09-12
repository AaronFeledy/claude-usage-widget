package contract

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestServiceRegistrationDistinguishesLiveAndReplacedProcess(t *testing.T) {
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	executable, err = filepath.EvalSymlinks(executable)
	if err != nil {
		t.Fatal(err)
	}
	token, err := captureProcessToken(os.Getpid(), executable)
	if err != nil {
		t.Fatal(err)
	}
	record := ManagedServiceRecord{PID: os.Getpid(), Executable: executable, ProcessToken: token}
	if err := managedServiceMayRegister(record); err == nil {
		t.Fatal("live service receipt could be replaced")
	}
	record.ProcessToken = "different-kernel-start-identity"
	if err := managedServiceMayRegister(record); err != nil {
		t.Fatalf("reused PID refused: %v", err)
	}
	record.ProcessToken = token
	record.Executable = filepath.Join(t.TempDir(), "other-executable")
	if _, err := captureProcessToken(record.PID, record.Executable); !errors.Is(err, errProcessExecutableMismatch) {
		t.Fatalf("wrong image must be a definite mismatch: %v", err)
	}
	if err := managedServiceMayRegister(record); err != nil {
		t.Fatalf("different image refused: %v", err)
	}
}
