//go:build linux

package contract

import (
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"testing"
)

func TestSystemdManagedServiceRequiresFixedUserUnitPID(t *testing.T) {
	previous := userSystemctlCommand
	t.Cleanup(func() { userSystemctlCommand = previous })
	var calls [][]string
	userSystemctlCommand = func(arguments ...string) ([]byte, error) {
		calls = append(calls, append([]string(nil), arguments...))
		return []byte("1234\n"), nil
	}
	supervisor, err := managedServiceSupervisor([]string{"INVOCATION_ID=fixture"}, 1234)
	if err != nil || supervisor != managedSystemdUser {
		t.Fatalf("supervisor = %q, %v", supervisor, err)
	}
	if _, err = managedServiceSupervisor([]string{"SYSTEMD_EXEC_PID=1235"}, 1235); err == nil {
		t.Fatal("different fixed-unit MainPID was accepted")
	}
	callCount := len(calls)
	if supervisor, err = managedServiceSupervisor([]string{"SYSTEMD_EXEC_PID=1234", "INVOCATION_ID=inherited"}, 9999); err != nil || supervisor != "" {
		t.Fatalf("inherited supervisor marker = %q, %v", supervisor, err)
	}
	if len(calls) != callCount {
		t.Fatal("inherited mismatched SYSTEMD_EXEC_PID queried the fixed unit")
	}
	if supervisor, err = managedServiceSupervisor([]string{"PATH=/usr/bin"}, 1234); err != nil || supervisor != "" {
		t.Fatalf("ordinary service supervisor = %q, %v", supervisor, err)
	}
	if !reflect.DeepEqual(calls[0], []string{"show", "--property", "MainPID", "--value", managedUserUnit}) {
		t.Fatalf("systemctl arguments = %#v", calls[0])
	}
}

func TestSupervisedPublicLauncherHandoffIsBoundToIntent(t *testing.T) {
	previous := userSystemctlCommand
	t.Cleanup(func() { userSystemctlCommand = previous })
	userSystemctlCommand = func(arguments ...string) ([]byte, error) {
		return []byte(strconv.Itoa(os.Getpid()) + "\n"), nil
	}
	parent := t.TempDir()
	root := filepath.Join(parent, "install")
	application := filepath.Join(parent, "bin", "headroom-gui")
	launcher := filepath.Join(parent, "bin", "headroom")
	archive := makePackage(t, parent, "19.0.0", false)
	if _, err := InstallArchiveWithCLIEntry(archive, root, application, launcher, Expectations{}); err != nil {
		t.Fatal(err)
	}
	executable, _, err := ActiveExecutableForRole(root, RoleCLI)
	if err != nil {
		t.Fatal(err)
	}
	arguments := []string{"--config", filepath.Join(parent, "config.yaml")}
	if err = authorizeSupervisedManagedServiceRestart(root, executable, arguments, nil, parent); err != nil {
		t.Fatal(err)
	}
	if err = AuthorizeSupervisedLauncherHandoff(root, launcher, os.Getpid(), append([]string{"serve"}, arguments...)); err != nil {
		t.Fatalf("exact supervised handoff rejected: %v", err)
	}
	if err = AuthorizeSupervisedLauncherHandoff(root, launcher, os.Getpid(), []string{"serve", "--config", "/different"}); err == nil {
		t.Fatal("supervised handoff accepted changed service arguments")
	}
	if err = AuthorizeSupervisedLauncherHandoff(root, application, os.Getpid(), append([]string{"serve"}, arguments...)); err == nil {
		t.Fatal("supervised handoff accepted a non-CLI launcher")
	}
}
