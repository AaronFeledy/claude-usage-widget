//go:build !windows

package contract

import (
	"errors"
	"os"
	"syscall"
)

func securePrivateDirectory(path string, configure bool) error {
	if configure {
		if err := os.Chmod(path, 0o700); err != nil {
			return err
		}
	}
	return validatePrivateUnix(path, true)
}
func securePrivateFile(path string, newlyCreated bool) error {
	if newlyCreated {
		if err := os.Chmod(path, 0o600); err != nil {
			return err
		}
	}
	return validatePrivateUnix(path, false)
}

func validatePrivateUnix(path string, directory bool) error {
	info, err := os.Lstat(path)
	if err != nil {
		return err
	}
	if pathIsLinkOrReparse(path, info) || directory != info.IsDir() || (!directory && !info.Mode().IsRegular()) || info.Mode().Perm()&0o077 != 0 {
		return errors.New("private storage permissions are unsafe")
	}
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok || int(stat.Uid) != os.Getuid() {
		return errors.New("private storage owner is unsafe")
	}
	return nil
}
