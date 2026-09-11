//go:build linux

package pairing

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
	"strings"
)

func NativeRunner(config Config) (*Runner, error) {
	if err := config.Validate(); err != nil {
		return nil, err
	}
	kernel, err := os.ReadFile("/proc/sys/kernel/osrelease")
	if err != nil || !strings.Contains(strings.ToLower(string(kernel)), "microsoft") || os.Getenv("WSL_DISTRO_NAME") != config.Distribution {
		return nil, errors.New("pairing must run inside the explicitly selected WSL distribution")
	}
	account, err := user.Current()
	if err != nil || account.Username != config.User {
		return nil, errors.New("pairing must run as the explicitly selected WSL user")
	}
	return &Runner{Config: config, Platform: "linux", TranslateWindows: func(ctx context.Context, path string) (string, error) {
		// wslpath is a native argument-based path conversion, never a shell.
		command := exec.CommandContext(ctx, "/usr/bin/wslpath", "-u", "--", path)
		var output messageBuffer
		command.Stdout = &output
		if err := command.Run(); err != nil {
			return "", errors.New("Windows path translation failed")
		}
		translated := strings.TrimSuffix(output.String(), "\n")
		if !safeText(translated, 4096) || !filepath.IsAbs(translated) || filepath.Clean(translated) != translated {
			return "", errors.New("Windows interop path is invalid")
		}
		return translated, nil
	}}, nil
}
