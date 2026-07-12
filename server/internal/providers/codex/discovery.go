package codex

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

type WSLAuthPathFunc func(context.Context) (string, error)

type DiscoveryOptions struct {
	ConfiguredPath   string
	HomeDir          string
	OpenCodeAuthPath string
	Env              map[string]string
	WSLAuthPath      WSLAuthPathFunc
}

type DiscoveredAuth struct {
	Path   string
	Source CredentialSource
}

func DiscoverAuthPath(ctx context.Context, opts DiscoveryOptions) (DiscoveredAuth, error) {
	if opts.ConfiguredPath != "" {
		return DiscoveredAuth{Path: opts.ConfiguredPath, Source: CredentialSourceCodex}, nil
	}
	if codexHome := envValue(opts.Env, "CODEX_HOME"); codexHome != "" {
		path := filepath.Join(codexHome, "auth.json")
		if fileExists(path) {
			return DiscoveredAuth{Path: path, Source: CredentialSourceCodex}, nil
		}
	}
	home, err := homeDir(opts.HomeDir)
	if err != nil {
		return DiscoveredAuth{}, err
	}
	homeAuth := filepath.Join(home, ".codex", "auth.json")
	if fileExists(homeAuth) {
		return DiscoveredAuth{Path: homeAuth, Source: CredentialSourceCodex}, nil
	}
	resolveWSL := opts.WSLAuthPath
	if resolveWSL == nil {
		resolveWSL = defaultWSLAuthPath
	}
	if wslPath, err := resolveWSL(ctx); err == nil && wslPath != "" && fileExists(wslPath) {
		return DiscoveredAuth{Path: wslPath, Source: CredentialSourceCodex}, nil
	}
	openCodePath := opts.OpenCodeAuthPath
	if openCodePath == "" {
		openCodePath = filepath.Join(home, ".local", "share", "opencode", "auth.json")
	}
	if fileExists(openCodePath) {
		return DiscoveredAuth{Path: openCodePath, Source: CredentialSourceOpenCode}, nil
	}
	return DiscoveredAuth{Path: homeAuth, Source: CredentialSourceCodex}, nil
}

func envValue(env map[string]string, key string) string {
	if env != nil {
		return strings.TrimSpace(env[key])
	}
	return strings.TrimSpace(os.Getenv(key))
}

func homeDir(configured string) (string, error) {
	if configured != "" {
		return configured, nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("resolve home directory: %w", err)
	}
	return home, nil
}

func defaultWSLAuthPath(ctx context.Context) (string, error) {
	if runtime.GOOS != "windows" {
		return "", nil
	}
	ctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "wsl.exe", "sh", "-lc", "printf '%s\n%s\n%s' \"$WSL_DISTRO_NAME\" \"$HOME\" \"$CODEX_HOME\"")
	output, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("resolve WSL codex auth: %w", err)
	}
	lines := strings.Split(strings.ReplaceAll(string(output), "\r", ""), "\n")
	if len(lines) < 2 || strings.TrimSpace(lines[0]) == "" || strings.TrimSpace(lines[1]) == "" {
		return "", nil
	}
	distro := strings.TrimSpace(lines[0])
	codexHome := ""
	if len(lines) > 2 {
		codexHome = strings.TrimSpace(lines[2])
	}
	base := strings.TrimSpace(lines[1])
	segments := []string{".codex", "auth.json"}
	if codexHome != "" {
		base = codexHome
		segments = []string{"auth.json"}
	}
	return combineWSLPath(distro, base, segments...), nil
}

func combineWSLPath(distro string, linuxBase string, segments ...string) string {
	normalized := strings.TrimRight(strings.ReplaceAll(linuxBase, "/", `\`), `\`)
	if !strings.HasPrefix(normalized, `\`) {
		normalized = `\` + normalized
	}
	parts := append([]string{`\\wsl.localhost\` + distro + normalized}, segments...)
	return filepath.Join(parts...)
}

func fileExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && !info.IsDir()
}
