//go:build windows

package main

import (
	"os"
	"syscall"
	"unsafe"
)

func showLaunchError(message string) {
	user32 := syscall.NewLazyDLL("user32.dll")
	messageBox := user32.NewProc("MessageBoxW")
	text, _ := syscall.UTF16PtrFromString(message)
	title, _ := syscall.UTF16PtrFromString("Headroom could not start")
	_, _, _ = messageBox.Call(0, uintptr(unsafe.Pointer(text)), uintptr(unsafe.Pointer(title)), 0x10)
}

func startApplication(executable string, arguments, environment []string, detach bool) error {
	files := []*os.File{nil, nil, nil}
	if !detach {
		files = []*os.File{os.Stdin, os.Stdout, os.Stderr}
	}
	process, err := os.StartProcess(executable, append([]string{executable}, arguments...), &os.ProcAttr{Env: environment, Files: files})
	if err != nil {
		return err
	}
	if detach {
		return process.Release()
	}
	state, err := process.Wait()
	if err != nil {
		return err
	}
	if !state.Success() {
		code := state.ExitCode()
		if code <= 0 {
			code = 2
		}
		return exitStatusError{code: code}
	}
	return nil
}
