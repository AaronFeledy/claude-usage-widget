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

func startApplication(executable string, arguments, environment []string) error {
	process, err := os.StartProcess(executable, append([]string{executable}, arguments...), &os.ProcAttr{Env: environment, Files: []*os.File{nil, nil, nil}})
	if err != nil {
		return err
	}
	return process.Release()
}
