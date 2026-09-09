package contract

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

const StateName = "install-state.json"

type InstallState struct {
	Schema         int    `json:"schema"`
	Product        string `json:"product"`
	Platform       string `json:"platform"`
	Architecture   string `json:"architecture"`
	ActiveVersion  string `json:"active_version"`
	VersionPath    string `json:"version_path"`
	ManifestSHA256 string `json:"manifest_sha256"`
	PackageAsset   string `json:"package_asset"`
}

type StageResult struct {
	Schema         int    `json:"schema"`
	Product        string `json:"product"`
	Version        string `json:"version"`
	Platform       string `json:"platform"`
	Architecture   string `json:"architecture"`
	PackageAsset   string `json:"package_asset"`
	ManifestSHA256 string `json:"manifest_sha256"`
	PackageRoot    string `json:"package_root"`
}

type Inspection struct {
	Installed       bool     `json:"installed"`
	TrustedIdentity bool     `json:"trusted_identity"`
	Complete        bool     `json:"complete"`
	Version         string   `json:"version,omitempty"`
	Platform        string   `json:"platform,omitempty"`
	Architecture    string   `json:"architecture,omitempty"`
	PackageAsset    string   `json:"package_asset,omitempty"`
	LauncherPath    string   `json:"launcher_path,omitempty"`
	Missing         []string `json:"missing,omitempty"`
}

func StageArchive(archive, installRoot string, expected Expectations) (StageResult, error) {
	var err error
	installRoot, err = NormalizeInstallRoot(installRoot)
	if err != nil {
		return StageResult{}, err
	}
	if err := validateInstallTargets(installRoot, ""); err != nil {
		return StageResult{}, err
	}
	platform, architecture, err := NativeTarget()
	if err != nil {
		return StageResult{}, err
	}
	if expected.Platform != "" && expected.Platform != platform {
		return StageResult{}, fmt.Errorf("cannot stage %s package on %s", expected.Platform, platform)
	}
	if expected.Architecture != "" && expected.Architecture != architecture {
		return StageResult{}, fmt.Errorf("cannot stage %s package on %s", expected.Architecture, architecture)
	}
	inspected, _, err := InspectArchive(archive)
	if err != nil {
		return StageResult{}, err
	}
	if err = CheckExpectations(inspected, expected); err != nil {
		return StageResult{}, err
	}
	if inspected.Platform != platform || inspected.Architecture != architecture {
		return StageResult{}, fmt.Errorf("cannot stage %s/%s package on %s/%s", inspected.Platform, inspected.Architecture, platform, architecture)
	}
	expected.Platform = platform
	expected.Architecture = architecture
	stagingRoot := filepath.Join(installRoot, "staging")
	if err = os.MkdirAll(stagingRoot, 0o700); err != nil {
		return StageResult{}, err
	}
	token := make([]byte, 8)
	if _, err = rand.Read(token); err != nil {
		return StageResult{}, err
	}
	stage := filepath.Join(stagingRoot, "package-"+hex.EncodeToString(token))
	extractRoot := filepath.Join(stage, "contents")
	if err = os.Mkdir(stage, 0o700); err != nil {
		return StageResult{}, err
	}
	manifest, packageRoot, err := ExtractAndVerify(archive, extractRoot, expected)
	if err != nil {
		os.RemoveAll(stage)
		return StageResult{}, err
	}
	manifestHash, err := digestFile(filepath.Join(packageRoot, PackageManifestName))
	if err != nil {
		os.RemoveAll(stage)
		return StageResult{}, err
	}
	result := StageResult{Schema: SchemaVersion, Product: "Headroom", Version: manifest.Version, Platform: manifest.Platform, Architecture: manifest.Architecture, PackageAsset: manifest.AssetName, ManifestSHA256: manifestHash, PackageRoot: packageRoot}
	if err = WriteJSON(filepath.Join(stage, "verified-stage.json"), result); err != nil {
		os.RemoveAll(stage)
		return StageResult{}, err
	}
	return result, nil
}

func InstallArchive(archive, installRoot, entryPath string, expected Expectations) (InstallState, error) {
	var err error
	installRoot, err = NormalizeInstallRoot(installRoot)
	if err != nil {
		return InstallState{}, err
	}
	if entryPath != "" {
		entryPath, err = normalizeAbsolutePath(entryPath, "entry path")
		if err != nil {
			return InstallState{}, err
		}
	}
	if err := validateInstallTargets(installRoot, entryPath); err != nil {
		return InstallState{}, err
	}
	stage, err := StageArchive(archive, installRoot, expected)
	if err != nil {
		return InstallState{}, err
	}
	stageContainer := filepath.Dir(stage.PackageRoot)
	defer os.RemoveAll(stageContainer)
	manifestFile, err := os.Open(filepath.Join(stage.PackageRoot, PackageManifestName))
	if err != nil {
		return InstallState{}, err
	}
	manifest, err := DecodePackageManifest(manifestFile)
	manifestFile.Close()
	if err != nil {
		return InstallState{}, err
	}
	versions := filepath.Join(installRoot, "versions")
	if err = os.MkdirAll(versions, 0o755); err != nil {
		return InstallState{}, err
	}
	versionDir := filepath.Join(versions, manifest.Version)
	if _, err = os.Stat(versionDir); err != nil && !errors.Is(err, os.ErrNotExist) {
		return InstallState{}, err
	}
	installTemp := versionDir + ".installing"
	versionBackup := versionDir + ".replaced"
	os.RemoveAll(installTemp)
	os.RemoveAll(versionBackup)
	if err = copyTree(filepath.Join(stage.PackageRoot, "bundle"), installTemp); err != nil {
		os.RemoveAll(installTemp)
		return InstallState{}, err
	}
	if err = copyFile(filepath.Join(stage.PackageRoot, PackageManifestName), filepath.Join(installTemp, PackageManifestName), 0o644); err != nil {
		os.RemoveAll(installTemp)
		return InstallState{}, err
	}
	if _, statErr := os.Stat(versionDir); statErr == nil {
		if err = os.Rename(versionDir, versionBackup); err != nil {
			os.RemoveAll(installTemp)
			return InstallState{}, err
		}
	}
	if err = os.Rename(installTemp, versionDir); err != nil {
		if _, backupErr := os.Stat(versionBackup); backupErr == nil {
			_ = os.Rename(versionBackup, versionDir)
		}
		os.RemoveAll(installTemp)
		return InstallState{}, err
	}
	os.RemoveAll(versionBackup)
	ext := ""
	if runtime.GOOS == "windows" {
		ext = ".exe"
	}
	launcherSource := filepath.Join(stage.PackageRoot, "bootstrap", "headroom"+ext)
	managerSource := filepath.Join(stage.PackageRoot, "bootstrap", "headroom-package"+ext)
	launcherDest := filepath.Join(installRoot, "headroom-launcher"+ext)
	managerDest := filepath.Join(installRoot, "headroom-package"+ext)
	if runtime.GOOS == "windows" {
		launcherDest = filepath.Join(installRoot, "headroom.exe")
	}
	if err = replaceFile(launcherSource, launcherDest, 0o755); err != nil {
		return InstallState{}, err
	}
	if err = replaceFile(managerSource, managerDest, 0o755); err != nil {
		return InstallState{}, err
	}
	if err = replaceBytes([]byte(installRoot+"\n"), launcherDest+".root", 0o600); err != nil {
		return InstallState{}, err
	}
	if entryPath != "" {
		if !filepath.IsAbs(entryPath) {
			return InstallState{}, errors.New("entry path must be absolute")
		}
		if filepath.Clean(entryPath) != filepath.Clean(launcherDest) {
			if err = os.MkdirAll(filepath.Dir(entryPath), 0o755); err != nil {
				return InstallState{}, err
			}
			if err = replaceFile(launcherSource, entryPath, 0o755); err != nil {
				return InstallState{}, err
			}
		}
		if err = replaceBytes([]byte(installRoot+"\n"), entryPath+".root", 0o600); err != nil {
			return InstallState{}, err
		}
	}
	state := InstallState{Schema: SchemaVersion, Product: "Headroom", Platform: manifest.Platform, Architecture: manifest.Architecture, ActiveVersion: manifest.Version, VersionPath: filepath.ToSlash(filepath.Join("versions", manifest.Version)), ManifestSHA256: stage.ManifestSHA256, PackageAsset: manifest.AssetName}
	if err = writeAtomicJSON(filepath.Join(installRoot, StateName), state); err != nil {
		return InstallState{}, err
	}
	return state, nil
}

func InspectInstall(installRoot string) Inspection {
	result := Inspection{}
	installRoot, err := NormalizeInstallRoot(installRoot)
	if err != nil {
		return result
	}
	data, err := os.ReadFile(filepath.Join(installRoot, StateName))
	if err != nil {
		return result
	}
	result.Installed = true
	var state InstallState
	if json.Unmarshal(data, &state) != nil || state.Schema != SchemaVersion || state.Product != "Headroom" || !validVersion(state.ActiveVersion) || state.VersionPath != filepath.ToSlash(filepath.Join("versions", state.ActiveVersion)) {
		return result
	}
	expectedAsset, err := AssetName(state.ActiveVersion, state.Platform, state.Architecture)
	if err != nil || expectedAsset != state.PackageAsset || !hashPattern.MatchString(state.ManifestSHA256) {
		return result
	}
	manifestPath := filepath.Join(installRoot, filepath.FromSlash(state.VersionPath), PackageManifestName)
	digest, err := digestFile(manifestPath)
	if err != nil || digest != state.ManifestSHA256 {
		return result
	}
	file, err := os.Open(manifestPath)
	if err != nil {
		return result
	}
	manifest, err := DecodePackageManifest(file)
	file.Close()
	if err != nil || manifest.Version != state.ActiveVersion || manifest.Platform != state.Platform || manifest.Architecture != state.Architecture || manifest.AssetName != state.PackageAsset {
		return result
	}
	result.TrustedIdentity = true
	result.Version = state.ActiveVersion
	result.Platform = state.Platform
	result.Architecture = state.Architecture
	result.PackageAsset = state.PackageAsset
	ext := ""
	if state.Platform == "windows" {
		ext = ".exe"
	}
	result.LauncherPath = filepath.Join(installRoot, "headroom-launcher"+ext)
	if state.Platform == "windows" {
		result.LauncherPath = filepath.Join(installRoot, "headroom.exe")
	}
	versionRoot := filepath.Join(installRoot, filepath.FromSlash(state.VersionPath))
	for _, record := range manifest.Files {
		if !strings.HasPrefix(record.Path, "bundle/") {
			continue
		}
		relative := strings.TrimPrefix(record.Path, "bundle/")
		name := filepath.Join(versionRoot, filepath.FromSlash(relative))
		info, err := os.Lstat(name)
		if err != nil || !info.Mode().IsRegular() || info.Size() != record.Size {
			result.Missing = append(result.Missing, relative)
			continue
		}
		digest, err := digestFile(name)
		if err != nil || digest != record.SHA256 {
			result.Missing = append(result.Missing, relative)
		}
	}
	result.Complete = len(result.Missing) == 0
	return result
}

func ActiveExecutable(installRoot string) (string, Inspection, error) {
	canonicalRoot, err := NormalizeInstallRoot(installRoot)
	if err != nil {
		return "", Inspection{}, err
	}
	installRoot = canonicalRoot
	inspection := InspectInstall(installRoot)
	if !inspection.TrustedIdentity {
		return "", inspection, errors.New("Headroom installation identity is not valid")
	}
	name := "headroom"
	if inspection.Platform == "windows" {
		name += ".exe"
	}
	executable := filepath.Join(installRoot, "versions", inspection.Version, "bin", name)
	allowedMissing := map[string]bool{"bin/usage-server": true, "bin/usage-server.exe": true, "bin/headroom-credential-helper.exe": true}
	for _, missing := range inspection.Missing {
		if !allowedMissing[filepath.ToSlash(missing)] {
			return "", inspection, fmt.Errorf("Headroom %s runtime is incomplete; reinstall %s", inspection.Version, inspection.PackageAsset)
		}
	}
	info, err := os.Lstat(executable)
	if err != nil || !info.Mode().IsRegular() {
		return "", inspection, fmt.Errorf("Headroom %s application is missing; reinstall %s", inspection.Version, inspection.PackageAsset)
	}
	return executable, inspection, nil
}

// NormalizeInstallRoot accepts native absolute paths with either separator and
// returns the single path representation persisted in launcher associations.
func NormalizeInstallRoot(installRoot string) (string, error) {
	return normalizeAbsolutePath(installRoot, "install root")
}

func normalizeAbsolutePath(value, label string) (string, error) {
	if value == "" || strings.ContainsAny(value, "\r\n\x00") {
		return "", fmt.Errorf("%s must be an absolute clean path", label)
	}
	clean := filepath.Clean(value)
	if !filepath.IsAbs(clean) {
		return "", fmt.Errorf("%s must be an absolute clean path", label)
	}
	return clean, nil
}

func validateInstallTargets(installRoot, entryPath string) error {
	if !filepath.IsAbs(installRoot) || filepath.Clean(installRoot) != installRoot {
		return errors.New("install root must be an absolute clean path")
	}
	if entryPath != "" && (!filepath.IsAbs(entryPath) || filepath.Clean(entryPath) != entryPath) {
		return errors.New("entry path must be an absolute clean path")
	}
	for _, target := range []string{installRoot, entryPath} {
		if target == "" {
			continue
		}
		current := target
		for {
			info, err := os.Lstat(current)
			if err == nil && info.Mode()&os.ModeSymlink != 0 {
				return fmt.Errorf("install target traverses a link: %s", current)
			}
			parent := filepath.Dir(current)
			if parent == current {
				break
			}
			current = parent
		}
	}
	return nil
}

func digestFile(name string) (string, error) {
	file, err := os.Open(name)
	if err != nil {
		return "", err
	}
	defer file.Close()
	hash := sha256.New()
	if _, err = io.Copy(hash, file); err != nil {
		return "", err
	}
	return hex.EncodeToString(hash.Sum(nil)), nil
}

func copyTree(source, destination string) error {
	return filepath.Walk(source, func(name string, info os.FileInfo, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		rel, err := filepath.Rel(source, name)
		if err != nil {
			return err
		}
		target := filepath.Join(destination, rel)
		if info.Mode()&os.ModeType != 0 && !info.IsDir() {
			return fmt.Errorf("links and special files are forbidden: %s", rel)
		}
		if info.IsDir() {
			return os.MkdirAll(target, info.Mode().Perm())
		}
		return copyFile(name, target, info.Mode().Perm())
	})
}
func copyFile(source, destination string, mode os.FileMode) error {
	src, err := os.Open(source)
	if err != nil {
		return err
	}
	defer src.Close()
	if err = os.MkdirAll(filepath.Dir(destination), 0o755); err != nil {
		return err
	}
	dst, err := os.OpenFile(destination, os.O_CREATE|os.O_EXCL|os.O_WRONLY, mode)
	if err != nil {
		return err
	}
	_, copyErr := io.Copy(dst, src)
	closeErr := dst.Close()
	if copyErr != nil {
		return copyErr
	}
	if closeErr != nil {
		return closeErr
	}
	return os.Chmod(destination, mode)
}
func replaceFile(source, destination string, mode os.FileMode) error {
	src, err := os.ReadFile(source)
	if err != nil {
		return err
	}
	return replaceBytes(src, destination, mode)
}
func replaceBytes(contents []byte, destination string, mode os.FileMode) error {
	temporary := destination + ".new"
	backup := destination + ".old"
	os.Remove(temporary)
	os.Remove(backup)
	if err := os.MkdirAll(filepath.Dir(destination), 0o755); err != nil {
		return err
	}
	if err := os.WriteFile(temporary, contents, mode); err != nil {
		return err
	}
	if _, err := os.Stat(destination); err == nil {
		if err = os.Rename(destination, backup); err != nil {
			os.Remove(temporary)
			return err
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		os.Remove(temporary)
		return err
	}
	if err := os.Rename(temporary, destination); err != nil {
		os.Rename(backup, destination)
		return err
	}
	os.Remove(backup)
	return nil
}
func writeAtomicJSON(name string, value any) error {
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	if err = os.MkdirAll(filepath.Dir(name), 0o700); err != nil {
		return err
	}
	temporary := name + ".new"
	if err = os.WriteFile(temporary, data, 0o600); err != nil {
		return err
	}
	return os.Rename(temporary, name)
}
