//go:build windows

package credstore

import (
	"fmt"
	"syscall"
	"unsafe"
)

const (
	movefileReplaceExisting = 0x00000001
	movefileWriteThrough    = 0x00000008
)

var procMoveFileExW = kernel32.NewProc("MoveFileExW")

func replaceExisting(tmpName string, path string) error {
	tmpPtr, err := syscall.UTF16PtrFromString(tmpName)
	if err != nil {
		return fmt.Errorf("encode temp path: %w", err)
	}
	pathPtr, err := syscall.UTF16PtrFromString(path)
	if err != nil {
		return fmt.Errorf("encode destination path: %w", err)
	}
	if err := moveFileExW(tmpPtr, pathPtr, movefileReplaceExisting|movefileWriteThrough); err != nil {
		return fmt.Errorf("movefileex replace existing: %w", err)
	}
	return nil
}

func syncDirectory(_ string) error { return nil }

func moveFileExW(existingName *uint16, newName *uint16, flags uint32) error {
	// SAFETY: existingName and newName point to Go-owned UTF-16 buffers created by
	// replaceExisting and kept live for this synchronous MoveFileExW call. Windows
	// does not retain either pointer after the call returns.
	ret, _, err := procMoveFileExW.Call(
		uintptr(unsafe.Pointer(existingName)),
		uintptr(unsafe.Pointer(newName)),
		uintptr(flags),
	)
	if ret == 0 {
		return err
	}
	return nil
}
