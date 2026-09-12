//go:build !windows

package contract

import (
	"os"
	"path/filepath"
	"testing"
)

func TestPrivateJSONRejectsLoosePermissionsAndDuplicateKeys(t *testing.T) {
	root := t.TempDir()
	value := struct {
		Name string `json:"name"`
	}{Name: "fixture"}
	if err := WritePrivateJSON(root, filepath.Join("runtime", "fixture.json"), value); err != nil {
		t.Fatal(err)
	}
	var decoded struct {
		Name string `json:"name"`
	}
	if err := ReadPrivateJSON(root, filepath.Join("runtime", "fixture.json"), &decoded); err != nil || decoded.Name != "fixture" {
		t.Fatalf("decoded = %+v, %v", decoded, err)
	}
	path := filepath.Join(root, "runtime", "fixture.json")
	if err := os.WriteFile(path, []byte(`{"name":"a","name":"b"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := ReadPrivateJSON(root, filepath.Join("runtime", "fixture.json"), &decoded); err == nil {
		t.Fatal("duplicate private JSON keys accepted")
	}
	if err := os.Chmod(path, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := ReadPrivateJSON(root, filepath.Join("runtime", "fixture.json"), &decoded); err == nil {
		t.Fatal("group-readable private JSON accepted")
	}
}
