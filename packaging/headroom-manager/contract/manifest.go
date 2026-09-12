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
	// Empty kind is the original desktop contract; omit it from desktop JSON
	// so installed schema-1 managers can continue reading new desktop packages.
	PackageKindCLI = "cli"
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
	Path       string `json:"path"`
	Size       int64  `json:"size"`
	SHA256     string `json:"sha256"`
	Mode       string `json:"mode,omitempty"`
	LinkTarget string `json:"link_target,omitempty"`
}

type PackageManifest struct {
	PackageKind  string     `json:"package_kind,omitempty"`
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
	PackageKind string           `json:"package_kind,omitempty"`
	Schema      int              `json:"schema"`
	Product     string           `json:"product"`
	Version     string           `json:"version"`
	Packages    []ReleasePackage `json:"packages"`
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
	case "macos/x86_64":
		return "Headroom-v" + version + "-macos-x86_64.tar.gz", nil
	case "macos/arm64":
		return "Headroom-v" + version + "-macos-arm64.tar.gz", nil
	default:
		return "", fmt.Errorf("unsupported package target %s/%s", platform, architecture)
	}
}

func AssetNameForKind(kind, version, platform, architecture string) (string, error) {
	if kind == "" {
		return AssetName(version, platform, architecture)
	}
	if kind != PackageKindCLI || !validVersion(version) {
		return "", errors.New("invalid package kind or version")
	}
	suffix := platform + "-" + architecture + ".tar.gz"
	switch platform + "/" + architecture {
	case "windows/x86_64":
		suffix = "windows-x64.zip"
	case "windows/arm64":
		suffix = "windows-arm64.zip"
	case "linux/x86_64", "linux/arm64", "macos/x86_64", "macos/arm64":
	default:
		return "", fmt.Errorf("unsupported CLI package target %s/%s", platform, architecture)
	}
	return "Headroom-CLI-v" + version + "-" + suffix, nil
}

func ArchiveRoot(version, platform, architecture string) (string, error) {
	return ArchiveRootForKind("", version, platform, architecture)
}

func ArchiveRootForKind(kind, version, platform, architecture string) (string, error) {
	asset, err := AssetNameForKind(kind, version, platform, architecture)
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
	if platform == "darwin" {
		platform = "macos"
	}
	arch := runtime.GOARCH
	if arch == "amd64" {
		arch = "x86_64"
	}
	if _, err := AssetNameForKind(PackageKindCLI, "0.0.0", platform, arch); err != nil {
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
	if manifest.Schema != SchemaVersion || manifest.Product != "Headroom" || !validVersion(manifest.Version) || !validPackageKind(manifest.PackageKind) {
		return manifest, errors.New("unrecognized release manifest")
	}
	seen := map[string]bool{}
	for _, pkg := range manifest.Packages {
		expected, err := AssetNameForKind(manifest.PackageKind, manifest.Version, pkg.Platform, pkg.Architecture)
		if err != nil || pkg.AssetName != expected || pkg.Size <= 0 || pkg.Size > MaxArchiveBytes || !hashPattern.MatchString(pkg.SHA256) {
			return manifest, fmt.Errorf("invalid release package %q", pkg.AssetName)
		}
		key := pkg.Platform + "/" + pkg.Architecture
		if seen[key] {
			return manifest, fmt.Errorf("duplicate release package %s", key)
		}
		seen[key] = true
		root, _ := ArchiveRootForKind(manifest.PackageKind, manifest.Version, pkg.Platform, pkg.Architecture)
		if pkg.PackageManifestPath != root+"/"+PackageManifestName {
			return manifest, fmt.Errorf("invalid package manifest path for %s", key)
		}
		if err := validateReleaseComponentsForKind(pkg.Components, pkg.Platform, manifest.Version, manifest.PackageKind); err != nil {
			return manifest, err
		}
	}
	legacy := []string{"windows/x86_64", "windows/arm64", "linux/x86_64"}
	full := append(append([]string{}, legacy...), "macos/x86_64", "macos/arm64")
	want := full
	if len(manifest.Packages) == len(legacy) {
		want = legacy
	}
	if manifest.PackageKind == PackageKindCLI {
		want = append(full, "linux/arm64")
	}
	for _, key := range want {
		if !seen[key] {
			return manifest, fmt.Errorf("release manifest is missing %s", key)
		}
	}
	if len(manifest.Packages) != len(want) {
		return manifest, errors.New("release manifest contains an unsupported package set")
	}
	return manifest, nil
}

func validateReleaseComponents(components Components, platform, version string) error {
	return validateReleaseComponentsForKind(components, platform, version, "")
}

func validateReleaseComponentsForKind(components Components, platform, version, kind string) error {
	ext := ""
	if platform == "windows" {
		ext = ".exe"
	}
	application, server := packageApplicationPath(platform), packageServerPath(platform)
	if kind == PackageKindCLI {
		application = "bundle/bin/headroom" + ext
		server = application
	}
	checks := []struct {
		component Component
		path      string
	}{
		{components.Application, application},
		{components.Server, server},
		{components.Launcher, "bootstrap/headroom" + ext},
		{components.Manager, "bootstrap/headroom-package" + ext},
	}
	for _, check := range checks {
		if err := requireComponent(check.component, check.path, version); err != nil {
			return err
		}
	}
	if platform == "windows" && kind != PackageKindCLI {
		if components.CredentialHelper == nil {
			return errors.New("Windows release package is missing credential helper")
		}
		return requireComponent(*components.CredentialHelper, "bundle/bin/headroom-credential-helper.exe", version)
	}
	if components.CredentialHelper != nil {
		return fmt.Errorf("%s release package declares a credential helper", platform)
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
	if m.Schema != SchemaVersion || m.Product != "Headroom" || !validVersion(m.Version) || !validPackageKind(m.PackageKind) {
		return errors.New("unrecognized package manifest")
	}
	expectedAsset, err := AssetNameForKind(m.PackageKind, m.Version, m.Platform, m.Architecture)
	if err != nil || m.AssetName != expectedAsset {
		return fmt.Errorf("asset name does not match package target")
	}
	if (m.PackageKind == "" && !validVersion(m.QtVersion)) || (m.PackageKind == PackageKindCLI && m.QtVersion != "") || strings.TrimSpace(m.Baseline) == "" {
		return errors.New("package runtime metadata is incomplete")
	}
	exe := ""
	server := ""
	helper := ""
	launcher := ""
	manager := ""
	if m.Platform == "windows" {
		exe, server, helper, launcher, manager = "bundle/bin/headroom.exe", "bundle/bin/usage-server.exe", "bundle/bin/headroom-credential-helper.exe", "bootstrap/headroom.exe", "bootstrap/headroom-package.exe"
	} else if m.Platform == "macos" {
		exe, server, launcher, manager = packageApplicationPath(m.Platform), packageServerPath(m.Platform), "bootstrap/headroom", "bootstrap/headroom-package"
	} else {
		exe, server, launcher, manager = "bundle/bin/headroom", "bundle/bin/usage-server", "bootstrap/headroom", "bootstrap/headroom-package"
	}
	if m.PackageKind == PackageKindCLI {
		exe = "bundle/bin/headroom"
		if m.Platform == "windows" {
			exe += ".exe"
		}
		server, helper = exe, ""
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
	if m.Platform == "windows" && m.PackageKind != PackageKindCLI {
		if m.Components.CredentialHelper == nil {
			return errors.New("Windows package is missing credential helper metadata")
		}
		if err := requireComponent(*m.Components.CredentialHelper, helper, m.Version); err != nil {
			return err
		}
	} else if m.Components.CredentialHelper != nil {
		return fmt.Errorf("%s package must not declare a credential helper", m.Platform)
	}
	required := map[string]bool{exe: false, server: false, launcher: false, manager: false, "bundle/share/headroom/THIRD_PARTY_NOTICES.txt": false, "bundle/share/licenses/headroom/LICENSE": false, "bundle/share/licenses/qt/attributions/index.json": false}
	if m.PackageKind == PackageKindCLI {
		delete(required, "bundle/share/licenses/qt/attributions/index.json")
		required[strings.Replace(manager, "bootstrap/", "bundle/bin/", 1)] = false
	}
	if helper != "" {
		required[helper] = false
	}
	seen := map[string]bool{}
	folded := map[string]bool{}
	last := ""
	for _, file := range m.Files {
		fold := strings.ToLower(file.Path)
		if !canonicalRelative(file.Path) || file.Path == PackageManifestName || seen[file.Path] || folded[fold] || file.Path <= last || file.Size < 0 || !hashPattern.MatchString(file.SHA256) || file.LinkTarget == "" && !validMode(file.Mode, m.Platform) {
			return fmt.Errorf("invalid file record %q", file.Path)
		}
		if file.LinkTarget != "" && (m.PackageKind == PackageKindCLI || m.Platform != "macos" || file.Mode != "" || !validFrameworkLink(file.Path, file.LinkTarget) || file.Size != int64(len(file.LinkTarget)) || file.SHA256 != hashString(file.LinkTarget)) {
			return fmt.Errorf("invalid framework link record %q", file.Path)
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
	if err := validateFrameworkLinkGraph(m.Files, m.Platform); err != nil {
		return err
	}
	if m.PackageKind == PackageKindCLI {
		if m.Platform != "windows" {
			for _, name := range []string{exe, launcher, manager, "bundle/bin/headroom-package"} {
				if record, ok := fileRecord(m.Files, name); !ok || record.Mode != "0755" {
					return fmt.Errorf("CLI executable mode must be 0755: %s", name)
				}
			}
		}
		return nil
	}
	cliPath := PackageCLIPath(m.Platform, "")
	cliBootstrap := "bootstrap/headroom-cli"
	if m.Platform == "windows" {
		cliBootstrap += ".exe"
	}
	cliRecord, hasCLI := fileRecord(m.Files, cliPath)
	bootstrapRecord, hasBootstrap := fileRecord(m.Files, cliBootstrap)
	if hasCLI != hasBootstrap {
		return errors.New("desktop CLI payload and bootstrap must be packaged together")
	}
	if hasCLI && (cliRecord.LinkTarget != "" || bootstrapRecord.LinkTarget != "" ||
		(m.Platform != "windows" && (cliRecord.Mode != "0755" || bootstrapRecord.Mode != "0755"))) {
		return errors.New("desktop CLI components must be regular executables")
	}
	if m.Platform == "windows" {
		for _, name := range []string{"bundle/bin/headroom-package.exe", "bundle/bin/msvcp140.dll", "bundle/bin/vcruntime140.dll", "bundle/plugins/platforms/qwindows.dll", "bundle/plugins/platforms/qoffscreen.dll", "bundle/plugins/tls/qschannelbackend.dll", "bundle/plugins/imageformats/qsvg.dll", "bundle/plugins/iconengines/qsvgicon.dll", "bundle/qml/QtQuick/Controls/Basic/qmldir"} {
			if !seen[name] {
				return fmt.Errorf("required Windows runtime file missing: %s", name)
			}
		}
	} else if m.Platform == "linux" {
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
	} else {
		for name, target := range map[string]string{
			"QtCore": "Versions/Current/QtCore", "Resources": "Versions/Current/Resources", "Versions/Current": "A",
		} {
			full := "bundle/Headroom.app/Contents/Frameworks/QtCore.framework/" + name
			if record, ok := fileRecord(m.Files, full); !ok || record.LinkTarget != target {
				return fmt.Errorf("required macOS framework link missing or invalid: %s", full)
			}
		}
		for _, name := range []string{
			"bundle/bin/headroom-package",
			"bundle/Headroom.app/Contents/Info.plist",
			"bundle/Headroom.app/Contents/Resources/headroom.icns",
			"bundle/Headroom.app/Contents/Frameworks/QtCore.framework/Versions/A/QtCore",
			"bundle/Headroom.app/Contents/Frameworks/QtCore.framework/Versions/A/Resources/Info.plist",
			"bundle/Headroom.app/Contents/PlugIns/platforms/libqcocoa.dylib",
			"bundle/Headroom.app/Contents/PlugIns/platforms/libqoffscreen.dylib",
			"bundle/Headroom.app/Contents/PlugIns/tls/libqsecuretransportbackend.dylib",
			"bundle/Headroom.app/Contents/PlugIns/imageformats/libqsvg.dylib",
			"bundle/Headroom.app/Contents/PlugIns/iconengines/libqsvgicon.dylib",
			"bundle/Headroom.app/Contents/Resources/qml/QtQuick/Controls/Basic/qmldir",
		} {
			if !seen[name] {
				return fmt.Errorf("required macOS runtime file missing: %s", name)
			}
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
	return BuildManifestForKind("", root, version, platform, architecture, qtVersion, baseline)
}

func validPackageKind(kind string) bool { return kind == "" || kind == PackageKindCLI }

func BuildManifestForKind(kind, root, version, platform, architecture, qtVersion, baseline string) (PackageManifest, error) {
	asset, err := AssetNameForKind(kind, version, platform, architecture)
	if err != nil {
		return PackageManifest{}, err
	}
	m := PackageManifest{PackageKind: kind, Schema: SchemaVersion, Product: "Headroom", Version: version, Platform: platform, Architecture: architecture, AssetName: asset, QtVersion: qtVersion, Baseline: baseline}
	m.Components = Components{Application: Component{Path: packageApplicationPath(platform), Version: version}, Server: Component{Path: packageServerPath(platform), Version: version}, Launcher: Component{Path: "bootstrap/headroom", Version: version}, Manager: Component{Path: "bootstrap/headroom-package", Version: version}}
	if platform == "windows" {
		m.Components.Launcher.Path += ".exe"
		m.Components.Manager.Path += ".exe"
		m.Components.CredentialHelper = &Component{Path: "bundle/bin/headroom-credential-helper.exe", Version: version}
	}
	if kind == PackageKindCLI {
		m.Components.Application.Path = "bundle/bin/headroom"
		if platform == "windows" {
			m.Components.Application.Path += ".exe"
		}
		m.Components.Server = m.Components.Application
		m.Components.CredentialHelper = nil
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
		if info.Mode()&os.ModeSymlink != 0 {
			target, readErr := os.Readlink(name)
			if kind == PackageKindCLI || platform != "macos" || readErr != nil || !validFrameworkLink(filepath.ToSlash(rel), target) {
				return fmt.Errorf("unsupported package entry %s", rel)
			}
			m.Files = append(m.Files, File{Path: filepath.ToSlash(rel), Size: int64(len(target)), SHA256: hashString(target), LinkTarget: target})
			return nil
		}
		if !info.Mode().IsRegular() && !info.IsDir() {
			return fmt.Errorf("unsupported package entry %s", rel)
		}
		if info.IsDir() {
			return nil
		}
		file, err := os.Open(name)
		if err != nil {
			return err
		}
		hash := sha256.New()
		_, copyErr := io.Copy(hash, file)
		closeErr := file.Close()
		if err := errors.Join(copyErr, closeErr); err != nil {
			return err
		}
		record := File{Path: filepath.ToSlash(rel), Size: info.Size(), SHA256: hex.EncodeToString(hash.Sum(nil))}
		if platform != "windows" {
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

func packageApplicationPath(platform string) string {
	if platform == "macos" {
		return "bundle/Headroom.app/Contents/MacOS/headroom"
	}
	if platform == "windows" {
		return "bundle/bin/headroom.exe"
	}
	return "bundle/bin/headroom"
}

func packageServerPath(platform string) string {
	if platform == "macos" {
		return "bundle/Headroom.app/Contents/MacOS/usage-server"
	}
	if platform == "windows" {
		return "bundle/bin/usage-server.exe"
	}
	return "bundle/bin/usage-server"
}

func PackageCLIPath(platform, kind string) string {
	if kind == PackageKindCLI {
		if platform == "windows" {
			return "bundle/bin/headroom.exe"
		}
		return "bundle/bin/headroom"
	}
	return strings.TrimSuffix(packageApplicationPath(platform), ".exe") + "-cli" + func() string {
		if platform == "windows" {
			return ".exe"
		}
		return ""
	}()
}

func installedComponentPath(root, versionPath, packagePath string) string {
	relative := strings.TrimPrefix(packagePath, "bundle/")
	return filepath.Join(root, filepath.FromSlash(versionPath), filepath.FromSlash(relative))
}

func installedManifest(root string, state InstallState) (PackageManifest, error) {
	file, err := os.Open(filepath.Join(root, filepath.FromSlash(state.VersionPath), PackageManifestName))
	if err != nil {
		return PackageManifest{}, err
	}
	defer file.Close()
	return DecodePackageManifest(file)
}

func hashString(value string) string {
	sum := sha256.Sum256([]byte(value))
	return hex.EncodeToString(sum[:])
}

func frameworkRoot(name string) string {
	const prefix = "bundle/Headroom.app/Contents/Frameworks/"
	if !strings.HasPrefix(name, prefix) {
		return ""
	}
	rest := strings.TrimPrefix(name, prefix)
	index := strings.LastIndex(rest, ".framework/")
	if index <= 0 {
		return ""
	}
	return prefix + rest[:index+len(".framework")]
}

func validFrameworkLink(name, target string) bool {
	root := frameworkRoot(name)
	if root == "" || !canonicalRelative(target) {
		return false
	}
	resolved := path.Clean(path.Join(path.Dir(name), target))
	return resolved != root && strings.HasPrefix(resolved, root+"/")
}

func validateFrameworkLinkGraph(files []File, platform string) error {
	links := map[string]string{}
	listed := map[string]bool{}
	for _, file := range files {
		listed[file.Path] = true
		if file.LinkTarget != "" {
			links[file.Path] = file.LinkTarget
		}
	}
	if platform != "macos" && len(links) != 0 {
		return errors.New("framework links are supported only on macOS")
	}
	for name := range listed {
		for parent := path.Dir(name); parent != "."; parent = path.Dir(parent) {
			if listed[parent] {
				return fmt.Errorf("package record descends through a file or link: %s", name)
			}
		}
	}
	for name, targetValue := range links {
		target := path.Clean(path.Join(path.Dir(name), targetValue))
		seen := map[string]bool{name: true}
		for step := 0; step <= len(links); step++ {
			linkPrefix := ""
			for candidate := target; candidate != "." && candidate != "/"; candidate = path.Dir(candidate) {
				if _, ok := links[candidate]; ok {
					linkPrefix = candidate
					break
				}
			}
			if linkPrefix == "" {
				break
			}
			if seen[linkPrefix] {
				return fmt.Errorf("framework link cycle at %s", name)
			}
			seen[linkPrefix] = true
			suffix := strings.TrimPrefix(target, linkPrefix)
			target = path.Clean(path.Join(path.Dir(linkPrefix), links[linkPrefix], suffix))
		}
		root := frameworkRoot(name)
		if target == root || !strings.HasPrefix(target, root+"/") {
			return fmt.Errorf("framework link escapes its framework: %s", name)
		}
		declared := listed[target]
		if !declared {
			for item := range listed {
				if strings.HasPrefix(item, target+"/") {
					declared = true
					break
				}
			}
		}
		if !declared {
			return fmt.Errorf("framework link target is not declared: %s", name)
		}
	}
	return nil
}

func WriteJSON(name string, value any) error {
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	return os.WriteFile(name, data, 0o644)
}
