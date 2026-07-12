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
	host, port, err := net.SplitHostPort(listenAddr)
	if err != nil {
		return fmt.Errorf("parse listen address: %w", err)
	}
	if _, err := strconv.ParseUint(port, 10, 16); err != nil {
		return fmt.Errorf("parse listen port: %w", err)
	}
	if strings.TrimSpace(authToken) != "" {
		return nil
	}
	if isLoopbackHost(host) {
		return nil
	}
	return fmt.Errorf("%s without auth token: %w", listenAddr, ErrUnsafeBind)
}

func isLoopbackHost(host string) bool {
	trimmed := strings.TrimSpace(strings.Trim(host, "[]"))
	if strings.EqualFold(trimmed, "localhost") {
		return true
	}
	ip := net.ParseIP(trimmed)
	return ip != nil && ip.IsLoopback()
}
