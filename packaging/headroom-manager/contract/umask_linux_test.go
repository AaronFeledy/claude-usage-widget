package contract

import (
	"syscall"
	"testing"
)

func TestRestrictiveUmaskStillExtractsRecordedModes(t *testing.T) {
	root := t.TempDir()
	archive := makePackage(t, root, "4.0.0", false)
	old := syscall.Umask(0o077)
	defer syscall.Umask(old)
	if _, _, err := VerifyArchive(archive, Expectations{}); err != nil {
		t.Fatal(err)
	}
}
