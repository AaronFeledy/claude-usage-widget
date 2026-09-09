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
	"strings"
	"time"
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
	Installed        bool     `json:"installed"`
	TrustedIdentity  bool     `json:"trusted_identity"`
	Complete         bool     `json:"complete"`
	Version          string   `json:"version,omitempty"`
	VersionPath      string   `json:"version_path,omitempty"`
	Platform         string   `json:"platform,omitempty"`
	Architecture     string   `json:"architecture,omitempty"`
	PackageAsset     string   `json:"package_asset,omitempty"`
	LauncherPath     string   `json:"launcher_path,omitempty"`
	ActiveExecutable string   `json:"active_executable,omitempty"`
	Missing          []string `json:"missing,omitempty"`
	ApplyStatus      string   `json:"apply_status,omitempty"`
	ApplyMessage     string   `json:"apply_message,omitempty"`
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
	stagingRoot, err := ensureOwnedDirectory(installRoot, "staging", 0o700)
	if err != nil {
		return StageResult{}, err
	}
	if err = validateInstallTargets(stagingRoot, ""); err != nil {
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
	if err = validateInstallTargets(installRoot, entryPath); err != nil {
		return InstallState{}, err
	}
	stage, err := StageArchive(archive, installRoot, expected)
	if err != nil {
		return InstallState{}, err
	}
	stageContainer := filepath.Dir(filepath.Dir(stage.PackageRoot))
	defer os.RemoveAll(stageContainer)
	recoveryErr := RecoverInstall(installRoot)
	record := filepath.Join(stageContainer, "verified-stage.json")
	stage, manifest, err := LoadVerifiedStage(installRoot, record)
	if err != nil {
		return InstallState{}, err
	}
	lock, err := acquireInstallLock(installRoot, 10*time.Second)
	if err != nil {
		return InstallState{}, err
	}
	defer lock.Close()
	stage, manifest, err = LoadVerifiedStage(installRoot, record)
	if err != nil {
		return InstallState{}, err
	}
	var superseded []string
	if recoveryErr != nil {
		superseded, err = supersedeUnrecoverableTransactions(installRoot)
		if err != nil {
			return InstallState{}, errors.Join(recoveryErr, err)
		}
	} else if err = rejectIncompleteTransactions(installRoot); err != nil {
		return InstallState{}, err
	}
	transactions, err := ensureOwnedDirectory(installRoot, "transactions", 0o700)
	if err != nil {
		return InstallState{}, err
	}
	token, err := randomHex(8)
	if err != nil {
		return InstallState{}, err
	}
	backup := filepath.Join(transactions, "install-"+token)
	if err = os.Mkdir(backup, 0o700); err != nil {
		return InstallState{}, err
	}
	state, generation, err := createGeneration(installRoot, stage, manifest)
	if err != nil {
		_ = os.RemoveAll(backup)
		return InstallState{}, err
	}
	journal := InstallJournal{Schema: SchemaVersion, Product: "Headroom", Phase: "generation-ready", InstallRoot: installRoot,
		EntryPath: entryPath, GenerationDir: generation, Candidate: state}
	for _, directory := range superseded {
		journal.Supersedes = append(journal.Supersedes, filepath.Base(directory))
	}
	statePath := filepath.Join(installRoot, StateName)
	if info, stateErr := os.Lstat(statePath); stateErr == nil {
		if !info.Mode().IsRegular() {
			_ = os.RemoveAll(generation)
			_ = os.RemoveAll(backup)
			return InstallState{}, errors.New("installed state path is unsafe")
		}
		journal.PriorStateExisted = true
		journal.PriorStateSHA256, err = digestFile(statePath)
		if err != nil {
			_ = os.RemoveAll(generation)
			_ = os.RemoveAll(backup)
			return InstallState{}, err
		}
		if err = copyFile(statePath, filepath.Join(backup, "prior-install-state.json"), 0o600); err != nil {
			_ = os.RemoveAll(generation)
			_ = os.RemoveAll(backup)
			return InstallState{}, err
		}
	} else if !errors.Is(stateErr, os.ErrNotExist) {
		_ = os.RemoveAll(generation)
		_ = os.RemoveAll(backup)
		return InstallState{}, stateErr
	}
	journalPath := filepath.Join(backup, InstallJournalName)
	if err = writeDurableJSON(journalPath, journal); err != nil {
		return InstallState{}, err
	}
	if err = prepareBootstrapBackup(installRoot, entryPath, backup); err != nil {
		_ = os.RemoveAll(generation)
		return InstallState{}, err
	}
	journal.Phase = "bootstrap-intent"
	if err = writeDurableJSON(journalPath, journal); err != nil {
		return InstallState{}, err
	}
	if transactionTestHook != nil {
		if hookErr := transactionTestHook("install-bootstrap-intent"); hookErr != nil {
			return InstallState{}, errors.Join(hookErr, recoverInstallJournal(journal, backup))
		}
	}
	if err = applyBootstrap(stage.PackageRoot, installRoot, entryPath, manifest); err != nil {
		return InstallState{}, errors.Join(err, recoverInstallJournal(journal, backup))
	}
	journal.Phase = "state-intent"
	if err = writeDurableJSON(journalPath, journal); err != nil {
		return InstallState{}, errors.Join(err, recoverInstallJournal(journal, backup))
	}
	if err = writeDurableJSON(statePath, state); err != nil {
		return InstallState{}, errors.Join(err, recoverInstallJournal(journal, backup))
	}
	journal.Phase = "complete"
	if err = writeDurableJSON(journalPath, journal); err != nil {
		return InstallState{}, err
	}
	_ = os.Remove(filepath.Join(installRoot, "last-apply-result.json"))
	_ = os.RemoveAll(backup)
	for _, directory := range superseded {
		_ = os.RemoveAll(directory)
	}
	return state, nil
}

func InspectInstall(installRoot string) Inspection {
	result := Inspection{}
	installRoot, err := NormalizeInstallRoot(installRoot)
	if err != nil {
		return result
	}
	data, err := readBoundedFile(filepath.Join(installRoot, StateName), maxManifestBytes)
	if err != nil {
		return result
	}
	var state InstallState
	if json.Unmarshal(data, &state) != nil {
		return result
	}
	result, _ = inspectState(installRoot, state)
	var outcome ApplyResult
	if data, readErr := readBoundedFile(filepath.Join(installRoot, "last-apply-result.json"), maxManifestBytes); readErr == nil && json.Unmarshal(data, &outcome) == nil &&
		outcome.Schema == SchemaVersion && outcome.Product == "Headroom" && outcome.Version == state.ActiveVersion && outcome.VersionPath == state.VersionPath {
		result.ApplyStatus, result.ApplyMessage = outcome.Status, outcome.FailureReason
	}
	return result
}

func inspectState(installRoot string, state InstallState) (Inspection, error) {
	result := Inspection{Installed: true}
	if state.Schema != SchemaVersion || state.Product != "Headroom" || !validVersion(state.ActiveVersion) || !validVersionPath(state) {
		return result, errors.New("installed state identity is invalid")
	}
	expectedAsset, err := AssetName(state.ActiveVersion, state.Platform, state.Architecture)
	if err != nil || expectedAsset != state.PackageAsset || !hashPattern.MatchString(state.ManifestSHA256) {
		return result, errors.New("installed state package identity is invalid")
	}
	manifestPath := filepath.Join(installRoot, filepath.FromSlash(state.VersionPath), PackageManifestName)
	digest, err := digestFile(manifestPath)
	if err != nil || digest != state.ManifestSHA256 {
		return result, errors.New("installed manifest digest is invalid")
	}
	file, err := os.Open(manifestPath)
	if err != nil {
		return result, err
	}
	manifest, err := DecodePackageManifest(file)
	file.Close()
	if err != nil || manifest.Version != state.ActiveVersion || manifest.Platform != state.Platform || manifest.Architecture != state.Architecture || manifest.AssetName != state.PackageAsset {
		return result, errors.New("installed manifest identity is invalid")
	}
	result.TrustedIdentity = true
	result.Version, result.VersionPath = state.ActiveVersion, state.VersionPath
	result.Platform, result.Architecture, result.PackageAsset = state.Platform, state.Architecture, state.PackageAsset
	ext := ""
	if state.Platform == "windows" {
		ext = ".exe"
	}
	result.LauncherPath = filepath.Join(installRoot, "headroom-launcher"+ext)
	if state.Platform == "windows" {
		result.LauncherPath = filepath.Join(installRoot, "headroom.exe")
	}
	versionRoot := filepath.Join(installRoot, filepath.FromSlash(state.VersionPath))
	applicationName := "headroom"
	if state.Platform == "windows" {
		applicationName += ".exe"
	}
	result.ActiveExecutable = filepath.Join(versionRoot, "bin", applicationName)
	for _, record := range manifest.Files {
		if !strings.HasPrefix(record.Path, "bundle/") {
			continue
		}
		relative := strings.TrimPrefix(record.Path, "bundle/")
		name := filepath.Join(versionRoot, filepath.FromSlash(relative))
		info, statErr := os.Lstat(name)
		modeMismatch := state.Platform == "linux" && fmt.Sprintf("%04o", infoMode(info)) != record.Mode
		if statErr != nil || !info.Mode().IsRegular() || info.Size() != record.Size || modeMismatch {
			result.Missing = append(result.Missing, relative)
			continue
		}
		fileDigest, digestErr := digestFile(name)
		if digestErr != nil || fileDigest != record.SHA256 {
			result.Missing = append(result.Missing, relative)
		}
	}
	bootstrapMissing := func(recordPath, installedPath string) {
		for _, record := range manifest.Files {
			if record.Path != recordPath {
				continue
			}
			info, statErr := os.Lstat(installedPath)
			digest, digestErr := digestFile(installedPath)
			if statErr != nil || !info.Mode().IsRegular() || info.Size() != record.Size || digestErr != nil || digest != record.SHA256 {
				result.Missing = append(result.Missing, recordPath)
			}
			return
		}
	}
	if state.Platform == "windows" {
		bootstrapMissing("bootstrap/headroom.exe", filepath.Join(installRoot, "headroom.exe"))
		bootstrapMissing("bootstrap/headroom-package.exe", filepath.Join(installRoot, "headroom-package.exe"))
	} else {
		bootstrapMissing("bootstrap/headroom", filepath.Join(installRoot, "headroom-launcher"))
		bootstrapMissing("bootstrap/headroom-package", filepath.Join(installRoot, "headroom-package"))
	}
	if data, associationErr := readBoundedFile(result.LauncherPath+".root", 4096); associationErr != nil || string(data) != installRoot+"\n" {
		result.Missing = append(result.Missing, "bootstrap/association")
	}
	result.Complete = len(result.Missing) == 0
	return result, nil
}

func infoMode(info os.FileInfo) os.FileMode {
	if info == nil {
		return 0
	}
	return info.Mode().Perm()
}

func validVersionPath(state InstallState) bool {
	legacy := filepath.ToSlash(filepath.Join("versions", state.ActiveVersion))
	if state.VersionPath == legacy {
		return true
	}
	prefix := legacy + ".generation-"
	if !strings.HasPrefix(state.VersionPath, prefix) || strings.Contains(state.VersionPath, "\\") || filepath.ToSlash(filepath.Clean(filepath.FromSlash(state.VersionPath))) != state.VersionPath {
		return false
	}
	parts := strings.Split(strings.TrimPrefix(state.VersionPath, prefix), "-")
	if len(parts) != 2 || len(parts[0]) != 16 || len(parts[1]) != 16 {
		return false
	}
	isHex := func(value string) bool {
		_, err := hex.DecodeString(value)
		return err == nil && strings.ToLower(value) == value
	}
	return isHex(parts[0]) && isHex(parts[1]) && strings.HasPrefix(state.ManifestSHA256, parts[0])
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
	executable := filepath.Join(installRoot, filepath.FromSlash(inspection.VersionPath), "bin", name)
	for _, missing := range inspection.Missing {
		if !isAuxiliaryPath(missing) {
			return "", inspection, fmt.Errorf("Headroom %s runtime is incomplete; reinstall %s", inspection.Version, inspection.PackageAsset)
		}
	}
	info, err := os.Lstat(executable)
	if err != nil || !info.Mode().IsRegular() {
		return "", inspection, fmt.Errorf("Headroom %s application is missing; reinstall %s", inspection.Version, inspection.PackageAsset)
	}
	return executable, inspection, nil
}

func isAuxiliaryPath(path string) bool {
	switch filepath.ToSlash(path) {
	case "bin/usage-server", "bin/usage-server.exe", "bin/headroom-credential-helper.exe",
		"bootstrap/headroom", "bootstrap/headroom.exe", "bootstrap/headroom-package", "bootstrap/headroom-package.exe", "bootstrap/association":
		return true
	}
	return false
}

// NormalizeInstallRoot accepts native absolute paths with either separator and
// returns the single path representation persisted in launcher associations.
func NormalizeInstallRoot(installRoot string) (string, error) {
	return normalizeAbsolutePath(installRoot, "install root")
}

func ValidateInstallRoot(installRoot string) (string, error) {
	root, err := NormalizeInstallRoot(installRoot)
	if err != nil {
		return "", err
	}
	if err = validateInstallTargets(root, ""); err != nil {
		return "", err
	}
	return root, nil
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
			if err == nil && pathIsLinkOrReparse(current, info) {
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
	if err := os.MkdirAll(filepath.Dir(destination), 0o755); err != nil {
		return err
	}
	file, err := os.CreateTemp(filepath.Dir(destination), ".headroom-replace-*")
	if err != nil {
		return err
	}
	temporary := file.Name()
	failed := true
	defer func() {
		_ = file.Close()
		if failed {
			_ = os.Remove(temporary)
		}
	}()
	if err = file.Chmod(mode); err == nil {
		_, err = file.Write(contents)
	}
	if err == nil {
		err = file.Sync()
	}
	if closeErr := file.Close(); err == nil {
		err = closeErr
	}
	if err != nil {
		return err
	}
	if err = replaceAtomic(temporary, destination); err != nil {
		return err
	}
	failed = false
	return syncDirectory(filepath.Dir(destination))
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
