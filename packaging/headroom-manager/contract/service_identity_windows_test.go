//go:build windows

package contract

import "testing"

func TestServiceRegistrationPreservesUnqueryableWindowsReceipt(t *testing.T) {
	// The Windows System process exists but its image/start identity cannot be
	// queried by an ordinary per-user process. Never interpret access denial as
	// evidence of a replaced service.
	if processGone(4) {
		t.Skip("System process unavailable")
	}
	if handle, _, _, err := openWindowsProcess(4, processSynchronize|processQueryLimitedInformation); err == nil {
		procCloseHandle.Call(uintptr(handle))
		t.Skip("System process is queryable in this account")
	}
	if err := managedServiceMayRegister(ManagedServiceRecord{PID: 4, Executable: `C:\unqueryable.exe`, ProcessToken: "old"}); err == nil {
		t.Fatal("unqueryable process allowed receipt replacement")
	}
}
