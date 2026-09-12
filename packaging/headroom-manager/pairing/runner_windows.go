//go:build windows

package pairing

import (
	"path/filepath"

	"golang.org/x/sys/windows"
)

func NativeRunner(config Config) (*Runner, error) {
	if err := config.Validate(); err != nil {
		return nil, err
	}
	system, err := windows.GetSystemDirectory()
	if err != nil {
		return nil, err
	}
	return &Runner{Config: config, Platform: "windows", WSLExecutable: filepath.Join(system, "wsl.exe")}, nil
}
