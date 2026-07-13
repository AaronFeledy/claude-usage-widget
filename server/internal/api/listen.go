package api

import (
	"errors"
	"fmt"
	"net"
	"strconv"
	"strings"
)

var ErrUnsafeBind = errors.New("api: unsafe unauthenticated bind")

func ValidateStartup(listenAddr string, authToken string) error {
	loopback, err := IsLoopbackListenAddr(listenAddr)
	if err != nil {
		return err
	}
	if strings.TrimSpace(authToken) != "" {
		return nil
	}
	if loopback {
		return nil
	}
	return fmt.Errorf("%s without auth token: %w", listenAddr, ErrUnsafeBind)
}

func IsLoopbackListenAddr(listenAddr string) (bool, error) {
	host, port, err := net.SplitHostPort(listenAddr)
	if err != nil {
		return false, fmt.Errorf("parse listen address: %w", err)
	}
	if _, err := strconv.ParseUint(port, 10, 16); err != nil {
		return false, fmt.Errorf("parse listen port: %w", err)
	}
	return isLoopbackHost(host), nil
}

func isLoopbackHost(host string) bool {
	trimmed := strings.TrimSpace(strings.Trim(host, "[]"))
	if strings.EqualFold(trimmed, "localhost") {
		return true
	}
	ip := net.ParseIP(trimmed)
	return ip != nil && ip.IsLoopback()
}
