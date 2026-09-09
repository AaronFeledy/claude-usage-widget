package contract

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strconv"
	"strings"
)

const (
	SchemaVersion       = 1
	PackageManifestName = "package-manifest.json"
	maxManifestBytes    = 4 << 20
	MaxArchiveBytes     = int64(2 << 30)
)

var hashPattern = regexp.MustCompile(`^[0-9a-f]{64}$`)

type Component struct {
	Path    string `json:"path"`
	Version string `json:"version"`
}

type Components struct {
	Application      Component  `json:"application"`
	Server           Component  `json:"server"`
	CredentialHelper *Component `json:"credential_helper"`
	Launcher         Component  `json:"launcher"`
	Manager          Component  `json:"manager"`
}

type File struct {
	Path   string `json:"path"`
	Size   int64  `json:"size"`
	SHA256 string `json:"sha256"`
	Mode   string `json:"mode,omitempty"`
}

type PackageManifest struct {
	Schema       int        `json:"schema"`
	Product      string     `json:"product"`
	Version      string     `json:"version"`
	Platform     string     `json:"platform"`
	Architecture string     `json:"architecture"`
	AssetName    string     `json:"asset_name"`
	QtVersion    string     `json:"qt_version"`
	Baseline     string     `json:"runtime_baseline"`
	Components   Components `json:"components"`
	Files        []File     `json:"files"`
}

type ReleasePackage struct {
	Platform            string     `json:"platform"`
	Architecture        string     `json:"architecture"`
	AssetName           string     `json:"asset_name"`
	Size                int64      `json:"size"`
	SHA256              string     `json:"sha256"`
	PackageManifestPath string     `json:"package_manifest_path"`
	Components          Components `json:"components"`
}

type ReleaseManifest struct {
	Schema   int              `json:"schema"`
	Product  string           `json:"product"`
	Version  string           `json:"version"`
	Packages []ReleasePackage `json:"packages"`
}

type Expectations struct {
	Version, Platform, Architecture, AssetName string
}

func AssetName(version, platform, architecture string) (string, error) {
	if !validVersion(version) {
		return "", fmt.Errorf("invalid version %q", version)
	}
	switch platform + "/" + architecture {
	case "windows/x86_64":
		return "Headroom-v" + version + "-windows-x64.zip", nil
	case "windows/arm64":
		return "Headroom-v" + version + "-windows-arm64.zip", nil
	case "linux/x86_64":
		return "Headroom-v" + version + "-linux-x86_64.tar.gz", nil
	default:
		return "", fmt.Errorf("unsupported package target %s/%s", platform, architecture)
	}
}

func ArchiveRoot(version, platform, architecture string) (string, error) {
	asset, err := AssetName(version, platform, architecture)
	if err != nil {
		return "", err
	}
	if strings.HasSuffix(asset, ".tar.gz") {
		return strings.TrimSuffix(asset, ".tar.gz"), nil
	}
	return strings.TrimSuffix(asset, ".zip"), nil
}

func NativeTarget() (string, string, error) {
	platform := runtime.GOOS
	arch := runtime.GOARCH
	if arch == "amd64" {
		arch = "x86_64"
	}
	if _, err := AssetName("0.0.0", platform, arch); err != nil {
		return "", "", err
	}
	return platform, arch, nil
}

func validVersion(version string) bool {
	mainAndBuild := strings.SplitN(version, "+", 2)
	if len(mainAndBuild) == 2 && !validIdentifiers(mainAndBuild[1], false) {
		return false
	}
	mainAndPre := strings.SplitN(mainAndBuild[0], "-", 2)
	parts := strings.Split(mainAndPre[0], ".")
	if len(parts) != 3 {
		return false
	}
	for _, part := range parts {
		if !validNumeric(part) {
			return false
		}
	}
	return len(mainAndPre) == 1 || validIdentifiers(mainAndPre[1], true)
}

func validIdentifiers(value string, rejectLeadingZero bool) bool {
	if value == "" {
		return false
	}
	for _, identifier := range strings.Split(value, ".") {
		if identifier == "" {
			return false
		}
		numeric := true
		for _, r := range identifier {
			if !(r >= '0' && r <= '9') {
				numeric = false
			}
			if !((r >= '0' && r <= '9') || (r >= 'A' && r <= 'Z') || (r >= 'a' && r <= 'z') || r == '-') {
				return false
			}
		}
		if rejectLeadingZero && numeric && len(identifier) > 1 && identifier[0] == '0' {
			return false
		}
	}
	return true
}

func validNumeric(value string) bool {
	if value == "" || len(value) > 1 && value[0] == '0' {
		return false
	}
	for _, r := range value {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

func canonicalRelative(name string) bool {
	if name == "" || strings.ContainsRune(name, 0) || strings.ContainsAny(name, "\\:") || path.IsAbs(name) {
		return false
	}
	for _, r := range name {
		if r < 0x20 || r == 0x7f {
			return false
		}
	}
	if path.Clean(name) != name || name == "." || strings.HasPrefix(name, "../") {
		return false
	}
	reserved := map[string]bool{"con": true, "prn": true, "aux": true, "nul": true, "com1": true, "com2": true, "com3": true, "com4": true, "com5": true, "com6": true, "com7": true, "com8": true, "com9": true, "lpt1": true, "lpt2": true, "lpt3": true, "lpt4": true, "lpt5": true, "lpt6": true, "lpt7": true, "lpt8": true, "lpt9": true}
	for _, segment := range strings.Split(name, "/") {
		if segment == "" || segment == "." || segment == ".." || strings.HasSuffix(segment, ".") || strings.HasSuffix(segment, " ") {
			return false
		}
		base := strings.ToLower(strings.SplitN(segment, ".", 2)[0])
		if reserved[base] {
			return false
		}
	}
	return true
}

func DecodePackageManifest(reader io.Reader) (PackageManifest, error) {
	var manifest PackageManifest
	data, err := io.ReadAll(io.LimitReader(reader, maxManifestBytes+1))
	if err != nil {
		return manifest, err
	}
	if len(data) > maxManifestBytes {
		return manifest, errors.New("package manifest is too large")
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&manifest); err != nil {
		return manifest, fmt.Errorf("decode package manifest: %w", err)
	}
	if err := ensureEOF(decoder); err != nil {
		return manifest, err
	}
	return manifest, ValidateManifest(manifest)
}

func DecodeReleaseManifest(reader io.Reader) (ReleaseManifest, error) {
	var manifest ReleaseManifest
	data, err := io.ReadAll(io.LimitReader(reader, maxManifestBytes+1))
	if err != nil {
		return manifest, err
	}
	if len(data) > maxManifestBytes {
		return manifest, errors.New("release manifest is too large")
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&manifest); err != nil {
		return manifest, fmt.Errorf("decode release manifest: %w", err)
	}
	if err := ensureEOF(decoder); err != nil {
		return manifest, err
	}
	if manifest.Schema != SchemaVersion || manifest.Product != "Headroom" || !validVersion(manifest.Version) {
		return manifest, errors.New("unrecognized release manifest")
	}
	seen := map[string]bool{}
	for _, pkg := range manifest.Packages {
		expected, err := AssetName(manifest.Version, pkg.Platform, pkg.Architecture)
		if err != nil || pkg.AssetName != expected || pkg.Size <= 0 || pkg.Size > MaxArchiveBytes || !hashPattern.MatchString(pkg.SHA256) {
			return manifest, fmt.Errorf("invalid release package %q", pkg.AssetName)
		}
		key := pkg.Platform + "/" + pkg.Architecture
		if seen[key] {
			return manifest, fmt.Errorf("duplicate release package %s", key)
		}
		seen[key] = true
		root, _ := ArchiveRoot(manifest.Version, pkg.Platform, pkg.Architecture)
		if pkg.PackageManifestPath != root+"/"+PackageManifestName {
			return manifest, fmt.Errorf("invalid package manifest path for %s", key)
		}
		if err := validateReleaseComponents(pkg.Components, pkg.Platform, manifest.Version); err != nil {
			return manifest, err
		}
	}
	for _, key := range []string{"windows/x86_64", "windows/arm64", "linux/x86_64"} {
		if !seen[key] {
			return manifest, fmt.Errorf("release manifest is missing %s", key)
		}
	}
	if len(manifest.Packages) != 3 {
		return manifest, errors.New("release manifest contains an unsupported package set")
	}
	return manifest, nil
}

func validateReleaseComponents(components Components, platform, version string) error {
	ext := ""
	if platform == "windows" {
		ext = ".exe"
	}
	checks := []struct {
		component Component
		path      string
	}{
		{components.Application, "bundle/bin/headroom" + ext},
		{components.Server, "bundle/bin/usage-server" + ext},
		{components.Launcher, "bootstrap/headroom" + ext},
		{components.Manager, "bootstrap/headroom-package" + ext},
	}
	for _, check := range checks {
		if err := requireComponent(check.component, check.path, version); err != nil {
			return err
		}
	}
	if platform == "windows" {
		if components.CredentialHelper == nil {
			return errors.New("Windows release package is missing credential helper")
		}
		return requireComponent(*components.CredentialHelper, "bundle/bin/headroom-credential-helper.exe", version)
	}
	if components.CredentialHelper != nil {
		return errors.New("Linux release package declares a credential helper")
	}
	return nil
}

func ensureEOF(decoder *json.Decoder) error {
	var extra any
	if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) {
		return errors.New("manifest contains trailing data")
	}
	return nil
}

func ValidateManifest(m PackageManifest) error {
	if m.Schema != SchemaVersion || m.Product != "Headroom" || !validVersion(m.Version) {
		return errors.New("unrecognized package manifest")
	}
	expectedAsset, err := AssetName(m.Version, m.Platform, m.Architecture)
	if err != nil || m.AssetName != expectedAsset {
		return fmt.Errorf("asset name does not match package target")
	}
	if !validVersion(m.QtVersion) || strings.TrimSpace(m.Baseline) == "" {
		return errors.New("package runtime metadata is incomplete")
	}
	exe := ""
	server := ""
	helper := ""
	launcher := ""
	manager := ""
	if m.Platform == "windows" {
		exe, server, helper, launcher, manager = "bundle/bin/headroom.exe", "bundle/bin/usage-server.exe", "bundle/bin/headroom-credential-helper.exe", "bootstrap/headroom.exe", "bootstrap/headroom-package.exe"
	} else {
		exe, server, launcher, manager = "bundle/bin/headroom", "bundle/bin/usage-server", "bootstrap/headroom", "bootstrap/headroom-package"
	}
	if err := requireComponent(m.Components.Application, exe, m.Version); err != nil {
		return err
	}
	if err := requireComponent(m.Components.Server, server, m.Version); err != nil {
		return err
	}
	if err := requireComponent(m.Components.Launcher, launcher, m.Version); err != nil {
		return err
	}
	if err := requireComponent(m.Components.Manager, manager, m.Version); err != nil {
		return err
	}
	if m.Platform == "windows" {
		if m.Components.CredentialHelper == nil {
			return errors.New("Windows package is missing credential helper metadata")
		}
		if err := requireComponent(*m.Components.CredentialHelper, helper, m.Version); err != nil {
			return err
		}
	} else if m.Components.CredentialHelper != nil {
		return errors.New("Linux package must not declare a credential helper")
	}
	required := map[string]bool{exe: false, server: false, launcher: false, manager: false, "bundle/share/headroom/THIRD_PARTY_NOTICES.txt": false, "bundle/share/licenses/headroom/LICENSE": false, "bundle/share/licenses/qt/attributions/index.json": false}
	if helper != "" {
		required[helper] = false
	}
	seen := map[string]bool{}
	folded := map[string]bool{}
	last := ""
	for _, file := range m.Files {
		fold := strings.ToLower(file.Path)
		if !canonicalRelative(file.Path) || file.Path == PackageManifestName || seen[file.Path] || folded[fold] || file.Path <= last || file.Size < 0 || !hashPattern.MatchString(file.SHA256) || !validMode(file.Mode, m.Platform) {
			return fmt.Errorf("invalid file record %q", file.Path)
		}
		seen[file.Path] = true
		folded[fold] = true
		last = file.Path
		if _, ok := required[file.Path]; ok {
			required[file.Path] = true
		}
	}
	for name, found := range required {
		if !found {
			return fmt.Errorf("required package file missing: %s", name)
		}
	}
	if m.Platform == "windows" {
		for _, name := range []string{"bundle/bin/headroom-package.exe", "bundle/bin/msvcp140.dll", "bundle/bin/vcruntime140.dll", "bundle/plugins/platforms/qwindows.dll", "bundle/plugins/platforms/qoffscreen.dll", "bundle/plugins/tls/qschannelbackend.dll", "bundle/plugins/imageformats/qsvg.dll", "bundle/plugins/iconengines/qsvgicon.dll", "bundle/qml/QtQuick/Controls/Basic/qmldir"} {
			if !seen[name] {
				return fmt.Errorf("required Windows runtime file missing: %s", name)
			}
		}
	} else {
		for _, name := range []string{"bundle/bin/headroom-package", "bundle/plugins/platforms/libqxcb.so", "bundle/plugins/platforms/libqoffscreen.so", "bundle/plugins/tls/libqopensslbackend.so", "bundle/plugins/imageformats/libqsvg.so", "bundle/plugins/iconengines/libqsvgicon.so", "bundle/qml/QtQuick/Controls/Basic/qmldir"} {
			if !seen[name] {
				return fmt.Errorf("required Linux runtime file missing: %s", name)
			}
		}
		if !seen["bundle/plugins/platforms/libqwayland-generic.so"] && !seen["bundle/plugins/platforms/libqwayland.so"] {
			return errors.New("required Linux Wayland platform plugin is missing")
		}
		for _, component := range []Component{m.Components.Application, m.Components.Server, m.Components.Launcher, m.Components.Manager} {
			if record, ok := fileRecord(m.Files, component.Path); !ok || record.Mode != "0755" {
				return fmt.Errorf("package executable mode must be 0755: %s", component.Path)
			}
		}
		if record, ok := fileRecord(m.Files, "bundle/bin/headroom-package"); !ok || record.Mode != "0755" {
			return errors.New("package runtime manager mode must be 0755")
		}
	}
	return nil
}

func fileRecord(files []File, name string) (File, bool) {
	index := sort.Search(len(files), func(i int) bool { return files[i].Path >= name })
	if index < len(files) && files[index].Path == name {
		return files[index], true
	}
	return File{}, false
}

func validMode(mode, platform string) bool {
	if platform == "windows" {
		return mode == ""
	}
	if len(mode) != 4 || mode[0] != '0' {
		return false
	}
	for _, r := range mode[1:] {
		if r < '0' || r > '7' {
			return false
		}
	}
	parsed, err := strconv.ParseUint(mode, 8, 16)
	// Package files may be readable/executable, never special or writable by group/other.
	return err == nil && parsed&0o022 == 0
}

func requireComponent(component Component, expectedPath, version string) error {
	if component.Path != expectedPath || component.Version != version {
		return fmt.Errorf("invalid component metadata for %s", expectedPath)
	}
	return nil
}

func CheckExpectations(m PackageManifest, expected Expectations) error {
	checks := []struct{ name, got, want string }{{"version", m.Version, expected.Version}, {"platform", m.Platform, expected.Platform}, {"architecture", m.Architecture, expected.Architecture}, {"asset", m.AssetName, expected.AssetName}}
	for _, check := range checks {
		if check.want != "" && check.got != check.want {
			return fmt.Errorf("package %s %q does not match expected %q", check.name, check.got, check.want)
		}
	}
	return nil
}

func BuildManifest(root, version, platform, architecture, qtVersion, baseline string) (PackageManifest, error) {
	asset, err := AssetName(version, platform, architecture)
	if err != nil {
		return PackageManifest{}, err
	}
	m := PackageManifest{Schema: SchemaVersion, Product: "Headroom", Version: version, Platform: platform, Architecture: architecture, AssetName: asset, QtVersion: qtVersion, Baseline: baseline}
	m.Components = Components{Application: Component{Path: "bundle/bin/headroom", Version: version}, Server: Component{Path: "bundle/bin/usage-server", Version: version}, Launcher: Component{Path: "bootstrap/headroom", Version: version}, Manager: Component{Path: "bootstrap/headroom-package", Version: version}}
	if platform == "windows" {
		m.Components.Application.Path += ".exe"
		m.Components.Server.Path += ".exe"
		m.Components.Launcher.Path += ".exe"
		m.Components.Manager.Path += ".exe"
		m.Components.CredentialHelper = &Component{Path: "bundle/bin/headroom-credential-helper.exe", Version: version}
	}
	err = filepath.WalkDir(root, func(name string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		rel, err := filepath.Rel(root, name)
		if err != nil {
			return err
		}
		if rel == "." || filepath.ToSlash(rel) == PackageManifestName {
			return nil
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() && !info.IsDir() {
			return fmt.Errorf("unsupported package entry %s", rel)
		}
		if info.IsDir() {
			return nil
		}
		file, err := os.Open(name)
		if err != nil {
			return err
		}
		defer file.Close()
		hash := sha256.New()
		if _, err := io.Copy(hash, file); err != nil {
			return err
		}
		record := File{Path: filepath.ToSlash(rel), Size: info.Size(), SHA256: hex.EncodeToString(hash.Sum(nil))}
		if platform == "linux" {
			record.Mode = fmt.Sprintf("%04o", info.Mode().Perm())
		}
		m.Files = append(m.Files, record)
		return nil
	})
	if err != nil {
		return PackageManifest{}, err
	}
	sort.Slice(m.Files, func(i, j int) bool { return m.Files[i].Path < m.Files[j].Path })
	return m, ValidateManifest(m)
}

func WriteJSON(name string, value any) error {
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	return os.WriteFile(name, data, 0o644)
}
