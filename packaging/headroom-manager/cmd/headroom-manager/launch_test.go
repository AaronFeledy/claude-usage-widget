package main

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestAssociationReaderIsBoundedRegularAndExact(t *testing.T) {
	root := t.TempDir()
	association := filepath.Join(root, "headroom.root")
	valid := filepath.Join(root, "install") + "\n"
	if err := os.WriteFile(association, []byte(valid), 0o600); err != nil {
		t.Fatal(err)
	}
	if data, err := readAssociation(association); err != nil || string(data) != valid {
		t.Fatalf("valid association = %q, %v", data, err)
	}
	for name, contents := range map[string]string{"crlf": filepath.Join(root, "install") + "\r\n", "trailing": valid + "garbage", "oversized": string(make([]byte, 4097))} {
		t.Run(name, func(t *testing.T) {
			if err := os.WriteFile(association, []byte(contents), 0o600); err != nil {
				t.Fatal(err)
			}
			if _, err := readAssociation(association); err == nil {
				t.Fatal("malformed association was accepted")
			}
		})
	}
	if runtime.GOOS != "windows" {
		target := filepath.Join(root, "target")
		if err := os.WriteFile(target, []byte(valid), 0o600); err != nil {
			t.Fatal(err)
		}
		_ = os.Remove(association)
		if err := os.Symlink(target, association); err != nil {
			t.Fatal(err)
		}
		if _, err := readAssociation(association); err == nil {
			t.Fatal("linked association was accepted")
		}
	}
}

func TestAssociationRejectsLinkedInstallRoot(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("native junction coverage runs in Windows package CI")
	}
	root := t.TempDir()
	realRoot := filepath.Join(root, "real")
	if err := os.Mkdir(realRoot, 0o700); err != nil {
		t.Fatal(err)
	}
	linked := filepath.Join(root, "linked")
	if err := os.Symlink(realRoot, linked); err != nil {
		t.Fatal(err)
	}
	if _, err := validatedAssociation(linked); err == nil {
		t.Fatal("link-backed install root was accepted")
	}
}
