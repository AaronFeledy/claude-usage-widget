package contract

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func makeCLIPackage(t *testing.T, parent, version string) string {
	t.Helper()
	platform, arch, err := NativeTarget()
	if err != nil {
		t.Fatal(err)
	}
	name, _ := ArchiveRootForKind(PackageKindCLI, version, platform, arch)
	root := filepath.Join(parent, name)
	ext := nativeExtension()
	for _, name := range []string{"bootstrap/headroom", "bootstrap/headroom-package", "bundle/bin/headroom", "bundle/bin/headroom-package"} {
		copyFixture(t, fixtureExecutable, filepath.Join(root, filepath.FromSlash(name+ext)))
	}
	for _, name := range []string{"bundle/share/headroom/THIRD_PARTY_NOTICES.txt", "bundle/share/licenses/headroom/LICENSE"} {
		path := filepath.Join(root, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte("fixture license\n"), 0644); err != nil {
			t.Fatal(err)
		}
	}
	manifest, err := BuildManifestForKind(PackageKindCLI, root, version, platform, arch, "", "Go native CLI")
	if err != nil {
		t.Fatal(err)
	}
	if err = WriteJSON(filepath.Join(root, PackageManifestName), manifest); err != nil {
		t.Fatal(err)
	}
	archive := filepath.Join(parent, manifest.AssetName)
	if err = WriteArchive(root, archive); err != nil {
		t.Fatal(err)
	}
	return archive
}

func TestCLIPackageUsesSharedValidationAndInstallation(t *testing.T) {
	parent := t.TempDir()
	archive := makeCLIPackage(t, parent, "2.3.4")
	manifest, _, err := VerifyArchive(archive, Expectations{})
	if err != nil {
		t.Fatal(err)
	}
	if manifest.QtVersion != "" || manifest.Components.Application != manifest.Components.Server {
		t.Fatal("CLI unexpectedly requires Qt or a second server runtime")
	}
	root, entry := filepath.Join(parent, "install"), filepath.Join(parent, "bin", "headroom"+nativeExtension())
	state, err := InstallArchive(archive, root, entry, Expectations{})
	if err != nil {
		t.Fatal(err)
	}
	inspection := InspectInstall(root)
	if state.PackageKind != PackageKindCLI || inspection.PackageKind != PackageKindCLI || !inspection.Complete || !inspection.TrustedIdentity {
		t.Fatalf("CLI identity was lost: %+v", inspection)
	}
	// A desktop-shaped manifest must not be able to masquerade as this profile.
	manifest.PackageKind = ""
	if ValidateManifest(manifest) == nil {
		t.Fatal("CLI archive accepted as a desktop package")
	}
}

func TestCLICatalogKeepsDesktopPackagesSeparate(t *testing.T) {
	parent := t.TempDir()
	root := filepath.Join(parent, "install")
	current := makeCLIPackage(t, parent, "2.3.4")
	if _, err := InstallArchive(current, root, filepath.Join(parent, "bin", "headroom"+nativeExtension()), Expectations{}); err != nil {
		t.Fatal(err)
	}
	next := makeCLIPackage(t, parent, "2.4.0")
	fixture := newUpdateFixture(t, "2.4.0", next)
	fixture.kind = PackageKindCLI
	result, err := fixture.client().StageLatest(context.Background(), root)
	if err != nil {
		t.Fatal(err)
	}
	if result.Status != "staged" || result.Stage.PackageKind != PackageKindCLI || !strings.HasPrefix(result.AssetName, "Headroom-CLI-") {
		t.Fatalf("wrong update package: %+v", result)
	}
	catalog, _ := fixture.release()
	encoded, _ := json.Marshal(catalog)
	if _, err := DecodeReleaseManifest(strings.NewReader(string(encoded))); err != nil {
		t.Fatal(err)
	}
	catalog.Packages = catalog.Packages[:5]
	encoded, _ = json.Marshal(catalog)
	if _, err := DecodeReleaseManifest(strings.NewReader(string(encoded))); err == nil {
		t.Fatal("incomplete CLI catalog accepted")
	}
}

func TestDesktopManifestOmitsNewKindField(t *testing.T) {
	data, err := json.Marshal(macOSManifestFixture())
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(data), "package_kind") {
		t.Fatal("desktop manifest breaks old strict schema readers")
	}
	if _, err := AssetNameForKind(PackageKindCLI, "2.3.4", "linux", "arm64"); err != nil {
		t.Fatal(err)
	}
	if _, err := AssetName("2.3.4", "linux", "arm64"); err == nil {
		t.Fatal("legacy desktop target contract changed")
	}
	if _, err := AssetNameForKind("unknown", "2.3.4", "linux", "x86_64"); err == nil {
		t.Fatal("unknown package kind accepted")
	}
}
