package main

import (
	"fmt"
	"os"
	"path/filepath"
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
	environment := append(os.Environ(), "HEADROOM_INSTALL_ROOT="+root, "HEADROOM_LAUNCHER_PATH="+launcher, "HEADROOM_PACKAGE_VERSION="+inspection.Version)
	return startApplication(executable, arguments, environment)
}

func associatedInstallRoot() (string, error) {
	if configured := os.Getenv("HEADROOM_INSTALL_ROOT"); configured != "" {
		return configured, nil
	}
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
		root := strings.TrimSuffix(string(data), "\n")
		if !filepath.IsAbs(root) || filepath.Clean(root) != root || strings.ContainsAny(root, "\r\n\x00") {
			return "", fmt.Errorf("Headroom launcher association is invalid")
		}
		return root, nil
	}
	if !os.IsNotExist(readErr) {
		return "", readErr
	}
	if _, stateErr := os.Stat(filepath.Join(filepath.Dir(launcher), contract.StateName)); stateErr == nil {
		return filepath.Dir(launcher), nil
	}
	return defaultInstallRoot(), nil
}
