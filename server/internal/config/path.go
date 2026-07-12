package config

import (
	"fmt"
	"path/filepath"
	"runtime"
)

func DefaultPath(goos string, env []string) (string, error) {
	vars := parseEnv(env)
	if goos == "windows" {
		appData := vars["APPDATA"]
		if appData == "" {
			return "", fmt.Errorf("APPDATA missing: %w", ErrInvalidConfig)
		}
		return filepath.Join(appData, "ClaudeUsageWidget", "config.yaml"), nil
	}

	if xdg := vars["XDG_CONFIG_HOME"]; xdg != "" {
		return filepath.Join(xdg, "claude-usage-widget", "config.yaml"), nil
	}
	home := vars["HOME"]
	if home == "" {
		return "", fmt.Errorf("HOME missing: %w", ErrInvalidConfig)
	}
	return filepath.Join(home, ".config", "claude-usage-widget", "config.yaml"), nil
}

func defaultPath(env []string) (string, error) {
	return DefaultPath(runtime.GOOS, env)
}
