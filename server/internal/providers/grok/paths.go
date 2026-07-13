package grok

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
)

func resolveCredentialsPath(override string) (string, error) {
	if override != "" {
		return expandHome(override)
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("resolve home directory: %w", err)
	}
	primary := filepath.Join(home, ".grok", "auth.json")
	if _, err := os.Stat(primary); err == nil {
		return primary, nil
	}
	if fallback := resolveWindowsWSLGrokAuthPath(); fallback != "" {
		if _, err := os.Stat(fallback); err == nil {
			return fallback, nil
		}
	}
	return primary, nil
}

func expandHome(path string) (string, error) {
	if path == "~" || strings.HasPrefix(path, "~/") {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", fmt.Errorf("resolve home directory: %w", err)
		}
		if path == "~" {
			return home, nil
		}
		return filepath.Join(home, strings.TrimPrefix(path, "~/")), nil
	}
	return path, nil
}

func resolveWindowsWSLGrokAuthPath() string {
	if runtime.GOOS != "windows" {
		return ""
	}
	cmd := exec.Command("wsl.exe", "sh", "-lc", `printf '%s\n%s' "$WSL_DISTRO_NAME" "$HOME"`)
	var stdout bytes.Buffer
	cmd.Stdout = &stdout
	if err := cmd.Run(); err != nil {
		return ""
	}
	lines := strings.Split(strings.ReplaceAll(stdout.String(), "\r", ""), "\n")
	if len(lines) < 2 || strings.TrimSpace(lines[0]) == "" || strings.TrimSpace(lines[1]) == "" {
		return ""
	}
	base := strings.TrimRight(strings.ReplaceAll(strings.TrimSpace(lines[1]), "/", `\`), `\`)
	if !strings.HasPrefix(base, `\`) {
		base = `\` + base
	}
	return filepath.Join(`\\wsl.localhost\`+strings.TrimSpace(lines[0])+base, ".grok", "auth.json")
}
