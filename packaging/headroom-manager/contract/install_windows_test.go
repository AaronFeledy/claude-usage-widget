package contract

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestWindowsForwardSlashInstallPathsBecomeStableIdentity(t *testing.T) {
	root := t.TempDir()
	archive := makePackage(t, root, "4.5.6", false)
	canonicalInstall := filepath.Join(root, "mixed path", "install")
	canonicalEntry := filepath.Join(root, "mixed path", "entry", "headroom.exe")
	forwardInstall := strings.ReplaceAll(canonicalInstall, `\`, "/")
	forwardEntry := strings.ReplaceAll(canonicalEntry, `\`, "/")
	mixedEntry := strings.Replace(forwardEntry, "/mixed path/", `\mixed path/`, 1)

	if _, err := InstallArchive(archive, forwardInstall, mixedEntry, Expectations{}); err != nil {
		t.Fatal(err)
	}
	association, err := os.ReadFile(canonicalEntry + ".root")
	if err != nil || string(association) != canonicalInstall+"\n" {
		t.Fatalf("stable launcher association = %q, %v", association, err)
	}
	inspection := InspectInstall(forwardInstall)
	if !inspection.TrustedIdentity || !inspection.Complete {
		t.Fatalf("normalized installation identity = %+v", inspection)
	}
	executable, _, err := ActiveExecutable(forwardInstall)
	if err != nil || executable != filepath.Join(canonicalInstall, "versions", "4.5.6", "bin", "headroom.exe") {
		t.Fatalf("active executable = %q, %v", executable, err)
	}
	if _, err = NormalizeInstallRoot("relative/install"); err == nil {
		t.Fatal("relative install root accepted")
	}
	if _, err = NormalizeInstallRoot(canonicalInstall + "\nforeign"); err == nil {
		t.Fatal("unsafe install root accepted")
	}
}
