package main

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/AaronFeledy/claude-usage-widget/packaging/headroom-manager/contract"
)

func launch(arguments []string) error {
	root, err := associatedInstallRoot()
	if err != nil {
		return err
	}
	executable, inspection, err := contract.ActiveExecutable(root)
	if err != nil {
		return err
	}
	launcher, err := os.Executable()
	if err != nil {
		return err
	}
	launcher, err = filepath.Abs(launcher)
	if err != nil {
		return err
	}
	environment := authoritativeEnvironment(os.Environ(), map[string]string{
		"HEADROOM_INSTALL_ROOT":    root,
		"HEADROOM_LAUNCHER_PATH":   launcher,
		"HEADROOM_PACKAGE_VERSION": inspection.Version,
	})
	return startApplication(executable, arguments, environment)
}

func associatedInstallRoot() (string, error) {
	launcher, err := os.Executable()
	if err != nil {
		return "", err
	}
	launcher, err = filepath.Abs(launcher)
	if err != nil {
		return "", err
	}
	data, readErr := os.ReadFile(launcher + ".root")
	if readErr == nil {
		return validatedAssociation(strings.TrimSuffix(string(data), "\n"))
	}
	if !os.IsNotExist(readErr) {
		return "", readErr
	}
	if configured := os.Getenv("HEADROOM_INSTALL_ROOT"); configured != "" {
		return validatedAssociation(configured)
	}
	if _, stateErr := os.Stat(filepath.Join(filepath.Dir(launcher), contract.StateName)); stateErr == nil {
		return filepath.Dir(launcher), nil
	}
	return defaultInstallRoot(), nil
}

func validatedAssociation(root string) (string, error) {
	if !filepath.IsAbs(root) || filepath.Clean(root) != root || strings.ContainsAny(root, "\r\n\x00") {
		return "", fmt.Errorf("Headroom launcher association is invalid")
	}
	return root, nil
}

func authoritativeEnvironment(base []string, values map[string]string) []string {
	result := make([]string, 0, len(base)+len(values))
	for _, item := range base {
		key, _, found := strings.Cut(item, "=")
		if !found {
			continue
		}
		managed := false
		for replacement := range values {
			if key == replacement || runtime.GOOS == "windows" && strings.EqualFold(key, replacement) {
				managed = true
				break
			}
		}
		if !managed {
			result = append(result, item)
		}
	}
	for _, key := range []string{"HEADROOM_INSTALL_ROOT", "HEADROOM_LAUNCHER_PATH", "HEADROOM_PACKAGE_VERSION"} {
		result = append(result, key+"="+values[key])
	}
	return result
}
