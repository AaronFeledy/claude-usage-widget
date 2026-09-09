package contract

import (
	"archive/tar"
	"compress/gzip"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"
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

func TestRestrictiveUmaskStillExtractsRecordedModes(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("umask is Unix-specific")
	}
	root := t.TempDir()
	archive := makePackage(t, root, "4.0.0", false)
	old := syscall.Umask(0o077)
	defer syscall.Umask(old)
	if _, _, err := VerifyArchive(archive, Expectations{}); err != nil {
		t.Fatal(err)
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
		if err = os.WriteFile(filepath.Join(root, "bundle", "share", "headroom", "THIRD_PARTY_NOTICES.txt"), []byte("changed"), 0644); err != nil {
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
