//go:build !windows

package main

import (
	"os"
	"os/exec"
	"syscall"
)

func startApplyManager(path, request string) error {
	command := exec.Command(path, "apply", "--request", request)
	command.Env = os.Environ()
	command.Stdin, command.Stdout, command.Stderr = nil, nil, nil
	command.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	if err := command.Start(); err != nil {
		return err
	}
	return command.Process.Release()
}
