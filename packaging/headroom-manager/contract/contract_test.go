package contract

import (
	"archive/tar"
	"compress/gzip"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"testing"
)

func TestAssetNamesAndStrictVersions(t *testing.T) {
	cases := map[string]string{"windows/x86_64": "Headroom-v2.3.4-windows-x64.zip", "windows/arm64": "Headroom-v2.3.4-windows-arm64.zip", "linux/x86_64": "Headroom-v2.3.4-linux-x86_64.tar.gz"}
	for target, want := range cases {
		parts := strings.Split(target, "/")
		got, err := AssetName("2.3.4", parts[0], parts[1])
		if err != nil || got != want {
			t.Fatalf("%s: got %q, %v", target, got, err)
		}
	}
	for _, bad := range []string{"v1.2.3", "01.2.3", "1.02.3", "1.2", "1.2.3-01", "1.2.3-", "1.2.3+", "1.2.3+bad..id"} {
		if validVersion(bad) {
			t.Errorf("accepted invalid version %q", bad)
		}
	}
}

func TestCanonicalPathsRejectPortableAliases(t *testing.T) {
	for _, bad := range []string{"../x", "a/../x", "/root", "C:/x", "a\\b", "a//b", "bundle/CON.txt", "bundle/name. ", "bundle/name.", "bundle/a\x00b"} {
		if canonicalRelative(bad) {
			t.Errorf("accepted unsafe path %q", bad)
		}
	}
}

func TestInstallFreshUpgradeAndAuxiliaryRepair(t *testing.T) {
	root := t.TempDir()
	archive1 := makePackage(t, root, "1.2.3", false)
	installRoot := filepath.Join(root, "install with spaces")
	entry := filepath.Join(root, "bin with spaces", "headroom")
	if runtime.GOOS == "windows" {
		entry += ".exe"
	}
	platform, architecture, _ := NativeTarget()
	state, err := InstallArchive(archive1, installRoot, entry, Expectations{Version: "1.2.3", Platform: platform, Architecture: architecture})
	if err != nil {
		t.Fatal(err)
	}
	if state.ActiveVersion != "1.2.3" {
		t.Fatal(state)
	}
	if _, err = InstallArchive(archive1, installRoot, entry, Expectations{}); err != nil {
		t.Fatalf("matching-version repair failed: %v", err)
	}
	association, err := os.ReadFile(entry + ".root")
	if err != nil || string(association) != installRoot+"\n" {
		t.Fatalf("launcher association = %q, %v", association, err)
	}
	inspection := InspectInstall(installRoot)
	if !inspection.TrustedIdentity || !inspection.Complete {
		t.Fatalf("unexpected inspection: %+v", inspection)
	}
	archive2 := makePackage(t, root, "1.2.4", false)
	state, err = InstallArchive(archive2, installRoot, entry, Expectations{})
	if err != nil {
		t.Fatal(err)
	}
	if state.ActiveVersion != "1.2.4" {
		t.Fatal(state)
	}
	server := filepath.Join(installRoot, "versions", "1.2.4", "bin", "usage-server"+nativeExtension())
	if err = os.Remove(server); err != nil {
		t.Fatal(err)
	}
	inspection = InspectInstall(installRoot)
	if !inspection.TrustedIdentity || inspection.Complete || len(inspection.Missing) == 0 {
		t.Fatalf("repair identity lost: %+v", inspection)
	}
	executable, _, err := ActiveExecutable(installRoot)
	if err != nil || filepath.Base(executable) != "headroom"+nativeExtension() {
		t.Fatalf("desktop should remain launchable: %q %v", executable, err)
	}
	runtimePath := "lib/qt6/plugins/platforms/libqoffscreen.so"
	if runtime.GOOS == "windows" {
		runtimePath = "plugins/platforms/qoffscreen.dll"
	}
	runtimeFile := filepath.Join(installRoot, "versions", "1.2.4", filepath.FromSlash(runtimePath))
	if err = os.WriteFile(runtimeFile, []byte("corrupt runtime"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, _, err = ActiveExecutable(installRoot); err == nil {
		t.Fatal("corrupt required runtime was launchable")
	}
	if _, err = InstallArchive(archive2, installRoot, entry, Expectations{}); err != nil {
		t.Fatalf("matching-version runtime repair failed: %v", err)
	}
	app := filepath.Join(installRoot, "versions", "1.2.4", "bin", "headroom"+nativeExtension())
	if err = os.WriteFile(app, []byte("corrupt"), 0o755); err != nil {
		t.Fatal(err)
	}
	if _, _, err = ActiveExecutable(installRoot); err == nil {
		t.Fatal("corrupt desktop was launchable")
	}
}

func TestInstallStateCannotRedirectVersionPath(t *testing.T) {
	root := t.TempDir()
	archive := makePackage(t, root, "2.0.0", false)
	installRoot := filepath.Join(root, "install")
	if _, err := InstallArchive(archive, installRoot, filepath.Join(root, "headroom"+nativeExtension()), Expectations{}); err != nil {
		t.Fatal(err)
	}
	statePath := filepath.Join(installRoot, StateName)
	var state InstallState
	data, _ := os.ReadFile(statePath)
	if err := json.Unmarshal(data, &state); err != nil {
		t.Fatal(err)
	}
	state.VersionPath = "versions/../foreign"
	if err := WriteJSON(statePath, state); err != nil {
		t.Fatal(err)
	}
	if InspectInstall(installRoot).TrustedIdentity {
		t.Fatal("noncanonical version path was trusted")
	}
}

func TestMaterializeLinksKeepsSonameAsRegularFile(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlinks are a Linux package concern")
	}
	root := t.TempDir()
	target := filepath.Join(root, "libthing.so.1.2")
	link := filepath.Join(root, "libthing.so.1")
	if err := os.WriteFile(target, []byte("library"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Base(target), link); err != nil {
		t.Fatal(err)
	}
	if err := MaterializeLinks(root); err != nil {
		t.Fatal(err)
	}
	info, err := os.Lstat(link)
	if err != nil || !info.Mode().IsRegular() {
		t.Fatalf("SONAME was not materialized: %v %v", info, err)
	}
}

func TestValidationFailureDoesNotChangeActiveVersion(t *testing.T) {
	root := t.TempDir()
	installRoot := filepath.Join(root, "install")
	first := makePackage(t, root, "3.0.0", false)
	if _, err := InstallArchive(first, installRoot, filepath.Join(root, "headroom"+nativeExtension()), Expectations{}); err != nil {
		t.Fatal(err)
	}
	broken := makePackage(t, root, "3.1.0", true)
	if _, err := InstallArchive(broken, installRoot, filepath.Join(root, "headroom"+nativeExtension()), Expectations{}); err == nil {
		t.Fatal("broken package installed")
	}
	if got := InspectInstall(installRoot).Version; got != "3.0.0" {
		t.Fatalf("active version changed to %q", got)
	}
}

func TestStageResultIsOwnedAndRecorded(t *testing.T) {
	root := t.TempDir()
	archive := makePackage(t, root, "3.2.1", false)
	installRoot := filepath.Join(root, "install")
	stage, err := StageArchive(archive, installRoot, Expectations{})
	if err != nil {
		t.Fatal(err)
	}
	stagingRoot := filepath.Join(installRoot, "staging") + string(os.PathSeparator)
	if !strings.HasPrefix(stage.PackageRoot, stagingRoot) || filepath.Base(stage.PackageRoot) == "" {
		t.Fatalf("stage escaped owned root: %+v", stage)
	}
	recordPath := filepath.Join(filepath.Dir(filepath.Dir(stage.PackageRoot)), "verified-stage.json")
	data, err := os.ReadFile(recordPath)
	if err != nil {
		t.Fatal(err)
	}
	var recorded StageResult
	if err = json.Unmarshal(data, &recorded); err != nil || recorded != stage {
		t.Fatalf("verified stage record mismatch: %+v %v", recorded, err)
	}
}

func TestArchiveRejectsLinksAndCaseCollisions(t *testing.T) {
	if runtime.GOOS != "linux" || runtime.GOARCH != "amd64" {
		t.Skip("tar mutation fixture runs on Linux x86_64")
	}
	parent := t.TempDir()
	_ = makePackage(t, parent, "1.0.0", false)
	rootName, _ := ArchiveRoot("1.0.0", "linux", "x86_64")
	packageRoot := filepath.Join(parent, rootName)
	linkArchive := filepath.Join(parent, "link.tar.gz")
	writeMutatedTar(t, packageRoot, linkArchive, &tar.Header{Name: rootName + "/link", Typeflag: tar.TypeSymlink, Linkname: "/tmp/x"}, "")
	if _, _, err := InspectArchive(linkArchive); err == nil || !strings.Contains(err.Error(), "links") {
		t.Fatalf("link rejection missing: %v", err)
	}
	caseArchive := filepath.Join(parent, "case.tar.gz")
	source := filepath.Join(packageRoot, "bundle", "bin", "headroom")
	writeMutatedTar(t, packageRoot, caseArchive, &tar.Header{Name: rootName + "/BUNDLE/bin/headroom", Mode: 0755, Typeflag: tar.TypeReg}, source)
	if _, _, err := InspectArchive(caseArchive); err == nil || !strings.Contains(err.Error(), "case-colliding") {
		t.Fatalf("case collision rejection missing: %v", err)
	}
}

func TestMalformedAndOversizedManifest(t *testing.T) {
	valid := PackageManifest{}
	data, _ := json.Marshal(valid)
	data = append(data, []byte(" trailing")...)
	if _, err := DecodePackageManifest(strings.NewReader(string(data))); err == nil {
		t.Fatal("trailing content accepted")
	}
	if _, err := DecodePackageManifest(strings.NewReader(strings.Repeat(" ", maxManifestBytes+1))); err == nil {
		t.Fatal("oversized manifest accepted")
	}
}

func TestReleaseManifestRejectsInvalidArchiveSizes(t *testing.T) {
	release := validReleaseManifest(t, "5.6.7")
	for name, size := range map[string]int64{"zero": 0, "negative": -1, "oversized": MaxArchiveBytes + 1} {
		t.Run(name, func(t *testing.T) {
			changed := release
			changed.Packages = append([]ReleasePackage(nil), release.Packages...)
			changed.Packages[0].Size = size
			data, _ := json.Marshal(changed)
			if _, err := DecodeReleaseManifest(strings.NewReader(string(data))); err == nil {
				t.Fatalf("accepted archive size %d", size)
			}
		})
	}
	data, _ := json.Marshal(release)
	var raw map[string]any
	_ = json.Unmarshal(data, &raw)
	raw["packages"].([]any)[0].(map[string]any)["size"] = true
	data, _ = json.Marshal(raw)
	if _, err := DecodeReleaseManifest(strings.NewReader(string(data))); err == nil {
		t.Fatal("accepted boolean archive size")
	}
}

func TestInspectRejectsArchiveSizeBounds(t *testing.T) {
	for name, size := range map[string]int64{"empty": 0, "oversized": MaxArchiveBytes + 1} {
		t.Run(name, func(t *testing.T) {
			archive := filepath.Join(t.TempDir(), "Headroom-v1.0.0-windows-x64.zip")
			file, err := os.Create(archive)
			if err != nil {
				t.Fatal(err)
			}
			if err = file.Truncate(size); err != nil {
				file.Close()
				t.Skipf("filesystem cannot create sparse archive fixture: %v", err)
			}
			file.Close()
			if _, _, err = InspectArchive(archive); err == nil || !strings.Contains(err.Error(), "size") {
				t.Fatalf("archive size %d rejection = %v", size, err)
			}
		})
	}
}

func TestManifestRejectsMissingRuntimeCaseCollisionAndUnsafeMode(t *testing.T) {
	root := t.TempDir()
	archive := makePackage(t, root, "6.0.0", false)
	manifest, _, err := InspectArchive(archive)
	if err != nil {
		t.Fatal(err)
	}
	required := "bundle/qml/QtQuick/Controls/Basic/qmldir"
	if manifest.Platform == "linux" {
		required = "bundle/lib/qt6/qml/QtQuick/Controls/Basic/qmldir"
	}
	missing := manifest
	missing.Files = removeFileRecord(missing.Files, required)
	if err = ValidateManifest(missing); err == nil || !strings.Contains(err.Error(), "runtime file missing") {
		t.Fatalf("missing runtime rejection = %v", err)
	}
	collision := manifest
	collision.Files = append([]File(nil), manifest.Files...)
	copyRecord := collision.Files[0]
	copyRecord.Path = strings.ToUpper(copyRecord.Path)
	collision.Files = append(collision.Files, copyRecord)
	// Sorting makes the fixture reach portable case-collision validation rather
	// than failing only because file records must be ordered.
	sort.Slice(collision.Files, func(i, j int) bool { return collision.Files[i].Path < collision.Files[j].Path })
	if err = ValidateManifest(collision); err == nil {
		t.Fatal("case-colliding manifest records accepted")
	}
	if manifest.Platform == "linux" {
		unsafe := manifest
		unsafe.Files = append([]File(nil), manifest.Files...)
		unsafe.Files[0].Mode = "0777"
		if err = ValidateManifest(unsafe); err == nil {
			t.Fatal("world-writable package mode accepted")
		}
	}
}

func TestVerifyArchiveHashesPayloadAndForeignExpectationDoesNotMutate(t *testing.T) {
	root := t.TempDir()
	corrupt := makePackage(t, root, "7.0.0", true)
	if _, _, err := VerifyArchive(corrupt, Expectations{}); err == nil || !strings.Contains(err.Error(), "hash mismatch") {
		t.Fatalf("corrupt archive verification = %v", err)
	}
	valid := makePackage(t, root, "7.0.1", false)
	platform, _, _ := NativeTarget()
	foreign := "windows"
	if platform == "windows" {
		foreign = "linux"
	}
	installRoot := filepath.Join(root, "foreign-install")
	if _, err := StageArchive(valid, installRoot, Expectations{Platform: foreign}); err == nil {
		t.Fatal("foreign platform expectation was accepted")
	}
	if _, err := os.Stat(installRoot); !os.IsNotExist(err) {
		t.Fatalf("foreign stage mutated install root: %v", err)
	}
}

func removeFileRecord(files []File, name string) []File {
	result := make([]File, 0, len(files)-1)
	for _, file := range files {
		if file.Path != name {
			result = append(result, file)
		}
	}
	return result
}

func validReleaseManifest(t *testing.T, version string) ReleaseManifest {
	t.Helper()
	manifest := ReleaseManifest{Schema: SchemaVersion, Product: "Headroom", Version: version}
	for _, target := range [][2]string{{"windows", "x86_64"}, {"windows", "arm64"}, {"linux", "x86_64"}} {
		platform, arch := target[0], target[1]
		asset, _ := AssetName(version, platform, arch)
		root, _ := ArchiveRoot(version, platform, arch)
		ext := ""
		if platform == "windows" {
			ext = ".exe"
		}
		components := Components{
			Application: Component{Path: "bundle/bin/headroom" + ext, Version: version},
			Server:      Component{Path: "bundle/bin/usage-server" + ext, Version: version},
			Launcher:    Component{Path: "bootstrap/headroom" + ext, Version: version},
			Manager:     Component{Path: "bootstrap/headroom-package" + ext, Version: version},
		}
		if platform == "windows" {
			components.CredentialHelper = &Component{Path: "bundle/bin/headroom-credential-helper.exe", Version: version}
		}
		manifest.Packages = append(manifest.Packages, ReleasePackage{Platform: platform, Architecture: arch, AssetName: asset, Size: 1, SHA256: strings.Repeat("a", 64), PackageManifestPath: root + "/" + PackageManifestName, Components: components})
	}
	return manifest
}

func makePackage(t *testing.T, parent, version string, corrupt bool) string {
	t.Helper()
	platform, arch, err := NativeTarget()
	if err != nil {
		t.Fatal(err)
	}
	asset, err := AssetName(version, platform, arch)
	if err != nil {
		t.Fatal(err)
	}
	rootName, err := ArchiveRoot(version, platform, arch)
	if err != nil {
		t.Fatal(err)
	}
	root := filepath.Join(parent, rootName)
	if err = os.MkdirAll(filepath.Join(root, "bundle", "bin"), 0755); err != nil {
		t.Fatal(err)
	}
	if err = os.MkdirAll(filepath.Join(root, "bundle", "share", "headroom"), 0755); err != nil {
		t.Fatal(err)
	}
	if err = os.MkdirAll(filepath.Join(root, "bundle", "share", "licenses", "headroom"), 0755); err != nil {
		t.Fatal(err)
	}
	if err = os.MkdirAll(filepath.Join(root, "bootstrap"), 0755); err != nil {
		t.Fatal(err)
	}
	self, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	ext := nativeExtension()
	names := []string{"bundle/bin/headroom" + ext, "bundle/bin/usage-server" + ext, "bootstrap/headroom" + ext, "bootstrap/headroom-package" + ext}
	if platform == "windows" {
		names = append(names, "bundle/bin/headroom-credential-helper.exe")
	}
	for _, name := range names {
		copyFixture(t, self, filepath.Join(root, filepath.FromSlash(name)))
	}
	runtimeFiles := []string{"bundle/qml/QtQuick/Controls/Basic/qmldir"}
	if platform == "windows" {
		runtimeFiles = append(runtimeFiles, "bundle/bin/msvcp140.dll", "bundle/bin/vcruntime140.dll", "bundle/plugins/platforms/qwindows.dll", "bundle/plugins/platforms/qoffscreen.dll", "bundle/plugins/tls/qschannelbackend.dll", "bundle/plugins/imageformats/qsvg.dll", "bundle/plugins/iconengines/qsvgicon.dll")
	} else {
		runtimeFiles = []string{"bundle/lib/qt6/qml/QtQuick/Controls/Basic/qmldir", "bundle/lib/qt6/plugins/platforms/libqxcb.so", "bundle/lib/qt6/plugins/platforms/libqwayland-generic.so", "bundle/lib/qt6/plugins/platforms/libqoffscreen.so", "bundle/lib/qt6/plugins/tls/libqopensslbackend.so", "bundle/lib/qt6/plugins/imageformats/libqsvg.so", "bundle/lib/qt6/plugins/iconengines/libqsvgicon.so"}
	}
	for _, name := range runtimeFiles {
		full := filepath.Join(root, filepath.FromSlash(name))
		if err = os.MkdirAll(filepath.Dir(full), 0755); err != nil {
			t.Fatal(err)
		}
		if err = os.WriteFile(full, []byte("synthetic runtime\n"), 0644); err != nil {
			t.Fatal(err)
		}
	}
	os.WriteFile(filepath.Join(root, "bundle", "share", "headroom", "THIRD_PARTY_NOTICES.txt"), []byte("synthetic notice\n"), 0644)
	os.WriteFile(filepath.Join(root, "bundle", "share", "licenses", "headroom", "LICENSE"), []byte("synthetic license\n"), 0644)
	manifest, err := BuildManifest(root, version, platform, arch, "6.8.3", "fixture")
	if err != nil {
		t.Fatal(err)
	}
	if err = WriteJSON(filepath.Join(root, PackageManifestName), manifest); err != nil {
		t.Fatal(err)
	}
	if corrupt {
		if err = os.WriteFile(filepath.Join(root, "bundle", "share", "headroom", "THIRD_PARTY_NOTICES.txt"), []byte("corrupted notice\n"), 0644); err != nil {
			t.Fatal(err)
		}
	}
	archive := filepath.Join(parent, asset)
	if err = WriteArchive(root, archive); err != nil {
		t.Fatal(err)
	}
	return archive
}

func nativeExtension() string {
	if runtime.GOOS == "windows" {
		return ".exe"
	}
	return ""
}

func writeMutatedTar(t *testing.T, root, output string, extra *tar.Header, extraSource string) {
	t.Helper()
	file, err := os.Create(output)
	if err != nil {
		t.Fatal(err)
	}
	gz := gzip.NewWriter(file)
	writer := tar.NewWriter(gz)
	base := filepath.Base(root)
	err = filepath.Walk(root, func(name string, info os.FileInfo, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if info.IsDir() {
			return nil
		}
		header, err := tar.FileInfoHeader(info, "")
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(root, name)
		header.Name = filepath.ToSlash(filepath.Join(base, rel))
		if err = writer.WriteHeader(header); err != nil {
			return err
		}
		source, err := os.Open(name)
		if err != nil {
			return err
		}
		_, err = io.Copy(writer, source)
		source.Close()
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	if extraSource != "" {
		info, err := os.Stat(extraSource)
		if err != nil {
			t.Fatal(err)
		}
		extra.Size = info.Size()
	}
	if err = writer.WriteHeader(extra); err != nil {
		t.Fatal(err)
	}
	if extraSource != "" {
		source, err := os.Open(extraSource)
		if err != nil {
			t.Fatal(err)
		}
		_, err = io.Copy(writer, source)
		source.Close()
		if err != nil {
			t.Fatal(err)
		}
	}
	if err = writer.Close(); err != nil {
		t.Fatal(err)
	}
	if err = gz.Close(); err != nil {
		t.Fatal(err)
	}
	if err = file.Close(); err != nil {
		t.Fatal(err)
	}
}

func copyFixture(t *testing.T, source, destination string) {
	t.Helper()
	src, err := os.Open(source)
	if err != nil {
		t.Fatal(err)
	}
	defer src.Close()
	dst, err := os.OpenFile(destination, os.O_CREATE|os.O_WRONLY|os.O_EXCL, 0755)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = io.Copy(dst, src); err != nil {
		t.Fatal(err)
	}
	if err = dst.Close(); err != nil {
		t.Fatal(err)
	}
}
