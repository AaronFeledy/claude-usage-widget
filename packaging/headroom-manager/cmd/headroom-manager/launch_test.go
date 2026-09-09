package main

import (
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestAuthoritativeEnvironmentReplacesDirtyLauncherMetadata(t *testing.T) {
	base := []string{"PATH=/bin", "HEADROOM_INSTALL_ROOT=/old", "HEADROOM_LAUNCHER_PATH=/old/headroom", "HEADROOM_PACKAGE_VERSION=0.1.0"}
	if runtime.GOOS == "windows" {
		base = append(base, "headroom_package_version=also-old")
	}
	values := map[string]string{"HEADROOM_INSTALL_ROOT": "/new", "HEADROOM_LAUNCHER_PATH": "/new/headroom", "HEADROOM_PACKAGE_VERSION": "2.0.0"}
	got := authoritativeEnvironment(base, values)
	for key, want := range values {
		count := 0
		for _, item := range got {
			name, value, found := strings.Cut(item, "=")
			if found && (name == key || runtime.GOOS == "windows" && strings.EqualFold(name, key)) {
				count++
				if value != want {
					t.Fatalf("%s = %q", key, value)
				}
			}
		}
		if count != 1 {
			t.Fatalf("%s occurs %d times in %q", key, count, got)
		}
	}
}

func TestConfiguredInstallRootMustBeAbsoluteAndClean(t *testing.T) {
	t.Setenv("HEADROOM_INSTALL_ROOT", filepath.Join("relative", "headroom"))
	if _, err := associatedInstallRoot(); err == nil {
		t.Fatal("relative configured install root accepted")
	}
}
