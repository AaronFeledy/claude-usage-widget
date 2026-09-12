//go:build !windows && !linux

package pairing

import "errors"

func NativeRunner(Config) (*Runner, error) {
	return nil, errors.New("Windows/WSL pairing is available only on Windows and WSL")
}
