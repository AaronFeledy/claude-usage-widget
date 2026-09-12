//go:build windows

package contract

import (
	"path/filepath"
	"testing"

	"golang.org/x/sys/windows"
)

func TestPrivateWindowsJSONRoundTripAndRejectsForeignACE(t *testing.T) {
	root := t.TempDir()
	relative := filepath.Join("runtime", "windows-private.json")
	want := struct {
		Name string `json:"name"`
	}{Name: "fixture"}
	if err := WritePrivateJSON(root, relative, want); err != nil {
		t.Fatal(err)
	}
	var got struct {
		Name string `json:"name"`
	}
	if err := ReadPrivateJSON(root, relative, &got); err != nil || got != want {
		t.Fatalf("private round trip = %+v, %v", got, err)
	}
	owner, err := currentOwnerIdentity()
	if err != nil {
		t.Fatal(err)
	}
	descriptor, err := windows.SecurityDescriptorFromString("D:P(A;;FA;;;" + owner + ")(A;;FR;;;WD)")
	if err != nil {
		t.Fatal(err)
	}
	dacl, _, err := descriptor.DACL()
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(root, relative)
	if err = windows.SetNamedSecurityInfo(path, windows.SE_FILE_OBJECT, windows.DACL_SECURITY_INFORMATION|windows.PROTECTED_DACL_SECURITY_INFORMATION, nil, nil, dacl, nil); err != nil {
		t.Fatal(err)
	}
	if err = ReadPrivateJSON(root, relative, &got); err == nil {
		t.Fatal("private JSON with foreign readable ACE was accepted")
	}
}
