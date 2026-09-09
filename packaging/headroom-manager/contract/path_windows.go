//go:build windows

package contract

import (
	"os"
	"syscall"
	"unsafe"
)

var procGetFileAttributesW = kernel32.NewProc("GetFileAttributesW")

func pathIsLinkOrReparse(path string, info os.FileInfo) bool {
	if info.Mode()&os.ModeSymlink != 0 {
		return true
	}
	name, err := syscall.UTF16PtrFromString(path)
	if err != nil {
		return true
	}
	attributes, _, _ := procGetFileAttributesW.Call(uintptr(unsafe.Pointer(name)))
	return attributes == 0xffffffff || attributes&0x400 != 0
}

func validBackupMode(mode os.FileMode) bool         { return mode.Perm() == 0o444 || mode.Perm() == 0o666 }
func backupModesEqual(left, right os.FileMode) bool { return left.Perm()&0o200 == right.Perm()&0o200 }
