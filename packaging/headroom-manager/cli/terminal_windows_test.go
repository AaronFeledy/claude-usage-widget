package cli

import (
	"errors"
	"os"
	"testing"

	"golang.org/x/sys/windows"
)

var (
	kernel32                  = windows.NewLazySystemDLL("kernel32.dll")
	createConsoleScreenBuffer = kernel32.NewProc("CreateConsoleScreenBuffer")
	allocConsole              = kernel32.NewProc("AllocConsole")
	freeConsole               = kernel32.NewProc("FreeConsole")
)

func newTestConsoleScreenBuffer() (windows.Handle, error) {
	handleValue, _, callErr := createConsoleScreenBuffer.Call(
		uintptr(windows.GENERIC_READ|windows.GENERIC_WRITE),
		uintptr(windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE),
		0,
		uintptr(1), // CONSOLE_TEXTMODE_BUFFER
		0,
	)
	handle := windows.Handle(handleValue)
	if handle == windows.InvalidHandle {
		return windows.InvalidHandle, callErr
	}
	return handle, nil
}

func TestPrepareDashboardTerminalEnablesAndRestoresConsoleMode(t *testing.T) {
	handle, err := newTestConsoleScreenBuffer()
	if errors.Is(err, windows.ERROR_INVALID_HANDLE) {
		result, _, allocErr := allocConsole.Call()
		if result == 0 {
			t.Fatalf("allocate owned test console: %v", allocErr)
		}
		t.Cleanup(func() {
			if result, _, freeErr := freeConsole.Call(); result == 0 {
				t.Errorf("free owned test console: %v", freeErr)
			}
		})
		handle, err = newTestConsoleScreenBuffer()
	}
	if err != nil {
		t.Fatalf("create disposable console screen buffer: %v", err)
	}
	file := os.NewFile(uintptr(handle), "Headroom test console screen buffer")
	if file == nil {
		windows.CloseHandle(handle)
		t.Fatal("create console screen buffer file")
	}
	defer file.Close()

	const initial = windows.ENABLE_PROCESSED_OUTPUT | windows.ENABLE_WRAP_AT_EOL_OUTPUT
	if err := windows.SetConsoleMode(handle, initial); err != nil {
		t.Fatalf("set disposable console mode: %v", err)
	}
	terminal, restore := prepareDashboardTerminal(file, true)
	if !terminal {
		t.Fatal("disposable console was not accepted")
	}
	var enabled uint32
	if err := windows.GetConsoleMode(handle, &enabled); err != nil {
		t.Fatal(err)
	}
	if enabled&windows.ENABLE_PROCESSED_OUTPUT == 0 || enabled&windows.ENABLE_VIRTUAL_TERMINAL_PROCESSING == 0 {
		t.Fatalf("enabled console mode = %#x", enabled)
	}
	restore()
	var restored uint32
	if err := windows.GetConsoleMode(handle, &restored); err != nil {
		t.Fatal(err)
	}
	if restored != initial {
		t.Fatalf("restored console mode = %#x, want %#x", restored, initial)
	}
}

func TestPrepareDashboardTerminalRejectsNonConsoleFile(t *testing.T) {
	file, err := os.CreateTemp(t.TempDir(), "stdout")
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	terminal, restore := prepareDashboardTerminal(file, true)
	defer restore()
	if terminal {
		t.Fatal("ordinary file was accepted as a console")
	}
}
