//go:build !windows

package main

import "syscall"

func showLaunchError(string) {}

func startApplication(executable string, arguments, environment []string) error {
	return syscall.Exec(executable, append([]string{executable}, arguments...), environment)
}
