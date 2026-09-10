//go:build !linux

package sshaccess

import (
	"context"
	"errors"
	"net"
)

var errUnsupported = errors.New("SSH access is supported only on Linux and WSL")

func ValidatePlatform() error { return errUnsupported }

func SocketPath(home string) string { return "" }

func Listen(home string) (net.Listener, error) { return nil, errUnsupported }

func dialTrustedSocket(ctx context.Context, home string) (net.Conn, error) {
	return nil, errUnsupported
}
