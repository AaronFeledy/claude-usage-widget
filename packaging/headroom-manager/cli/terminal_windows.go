package cli

import (
	"io"
	"os"

	"golang.org/x/sys/windows"
)

func prepareDashboardTerminal(writer io.Writer, terminal bool) (bool, func()) {
	if !terminal {
		return false, func() {}
	}
	file, ok := writer.(*os.File)
	if !ok {
		// Tests and embedders may provide their own terminal implementation.
		return true, func() {}
	}
	handle := windows.Handle(file.Fd())
	var original uint32
	if err := windows.GetConsoleMode(handle, &original); err != nil {
		return false, func() {}
	}
	required := original | windows.ENABLE_PROCESSED_OUTPUT | windows.ENABLE_VIRTUAL_TERMINAL_PROCESSING
	if err := windows.SetConsoleMode(handle, required); err != nil {
		return false, func() {}
	}
	return true, func() {
		_ = windows.SetConsoleMode(handle, original)
	}
}
