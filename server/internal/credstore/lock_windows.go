//go:build windows

package credstore

import (
	"context"
	"errors"
	"fmt"
	"os"
	"syscall"
	"time"
	"unsafe"
)

const (
	lockfileExclusiveLock   = 0x00000002
	lockfileFailImmediately = 0x00000001
	errorLockViolation      = syscall.Errno(33)
	errorSharingViolation   = syscall.Errno(32)
)

type fileLock struct{ file *os.File }

var (
	kernel32         = syscall.NewLazyDLL("kernel32.dll")
	procLockFileEx   = kernel32.NewProc("LockFileEx")
	procUnlockFileEx = kernel32.NewProc("UnlockFileEx")
)

func acquireFileLock(ctx context.Context, path string) (fileLock, error) {
	file, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return fileLock{}, fmt.Errorf("open credential lock: %w", err)
	}
	overlapped := &syscall.Overlapped{}
	for {
		err := lockFileEx(syscall.Handle(file.Fd()), lockfileExclusiveLock|lockfileFailImmediately, overlapped)
		if err == nil {
			return fileLock{file: file}, nil
		}
		if !isLockContention(err) {
			closeErr := file.Close()
			return fileLock{}, fmt.Errorf("lock credentials: %w", errorsJoin(err, closeErr))
		}
		timer := time.NewTimer(10 * time.Millisecond)
		select {
		case <-ctx.Done():
			if !timer.Stop() {
				select {
				case <-timer.C:
				default:
				}
			}
			closeErr := file.Close()
			return fileLock{}, fmt.Errorf("lock credentials canceled: %w", errorsJoin(ctx.Err(), closeErr))
		case <-timer.C:
		}
	}
}

func (l fileLock) Close() error {
	overlapped := &syscall.Overlapped{}
	if err := unlockFileEx(syscall.Handle(l.file.Fd()), overlapped); err != nil {
		closeErr := l.file.Close()
		return fmt.Errorf("unlock credentials: %w", errorsJoin(err, closeErr))
	}
	if err := l.file.Close(); err != nil {
		return fmt.Errorf("close credential lock: %w", err)
	}
	return nil
}

func lockFileEx(handle syscall.Handle, flags uint32, overlapped *syscall.Overlapped) error {
	ret, _, err := procLockFileEx.Call(
		uintptr(handle),
		uintptr(flags),
		0,
		1,
		0,
		uintptr(unsafe.Pointer(overlapped)),
	)
	if ret == 0 {
		return err
	}
	return nil
}

func isLockContention(err error) bool {
	return errors.Is(err, errorLockViolation) || errors.Is(err, errorSharingViolation)
}

func unlockFileEx(handle syscall.Handle, overlapped *syscall.Overlapped) error {
	ret, _, err := procUnlockFileEx.Call(
		uintptr(handle),
		0,
		1,
		0,
		uintptr(unsafe.Pointer(overlapped)),
	)
	if ret == 0 {
		return err
	}
	return nil
}
