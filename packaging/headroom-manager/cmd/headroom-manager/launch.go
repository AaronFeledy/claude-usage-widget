package main

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/AaronFeledy/claude-usage-widget/packaging/headroom-manager/contract"
)

func launch(arguments []string) error { return launchRole(contract.RoleApplication, arguments) }

func launchRole(role string, arguments []string) error {
	root, err := associatedInstallRoot()
	if err != nil {
		return err
	}
	if err = contract.RecoverInstall(root); err != nil {
		return err
	}
	executable, inspection, err := contract.ActiveExecutableForRole(root, role)
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
	values := map[string]string{
		"HEADROOM_INSTALL_ROOT":    root,
		"HEADROOM_LAUNCHER_PATH":   launcher,
		"HEADROOM_PACKAGE_VERSION": inspection.Version,
	}
	if role == contract.RoleCLI {
		values["HEADROOM_PUBLIC_LAUNCHER_PID"] = fmt.Sprintf("%d", os.Getpid())
		values["HEADROOM_PUBLIC_LAUNCHER_PATH"] = launcher
	}
	environment := authoritativeEnvironment(os.Environ(), values)
	return startApplication(executable, arguments, environment, role == contract.RoleApplication)
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
	data, readErr := readAssociation(launcher + ".root")
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
	canonical, err := contract.ValidateInstallRoot(root)
	if err != nil {
		return "", fmt.Errorf("Headroom launcher association is invalid")
	}
	return canonical, nil
}

func readAssociation(path string) ([]byte, error) {
	linked, err := os.Lstat(path)
	if err != nil || !linked.Mode().IsRegular() || linked.Mode()&os.ModeSymlink != 0 {
		return nil, fmt.Errorf("Headroom launcher association is invalid")
	}
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil || !info.Mode().IsRegular() || !os.SameFile(linked, info) || info.Size() <= 1 || info.Size() > 4096 {
		return nil, fmt.Errorf("Headroom launcher association is invalid")
	}
	data := make([]byte, info.Size())
	if _, err = io.ReadFull(file, data); err != nil {
		return nil, fmt.Errorf("Headroom launcher association is invalid")
	}
	if len(data) < 2 || data[len(data)-1] != '\n' || strings.ContainsAny(string(data[:len(data)-1]), "\r\n\x00") {
		return nil, fmt.Errorf("Headroom launcher association is invalid")
	}
	return data, nil
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
	for _, key := range []string{"HEADROOM_INSTALL_ROOT", "HEADROOM_LAUNCHER_PATH", "HEADROOM_PACKAGE_VERSION", "HEADROOM_PUBLIC_LAUNCHER_PID", "HEADROOM_PUBLIC_LAUNCHER_PATH"} {
		if value, ok := values[key]; ok {
			result = append(result, key+"="+value)
		}
	}
	return result
}
