//go:build !windows

package credstore

import (
	"context"
	"errors"
	"fmt"
	"os"
	"syscall"
	"time"
)

type fileLock struct{ file *os.File }

func acquireFileLock(ctx context.Context, path string) (fileLock, error) {
	fd, err := syscall.Open(path, syscall.O_CREAT|syscall.O_RDWR|syscall.O_CLOEXEC|syscall.O_NOFOLLOW, 0o600)
	if err != nil {
		return fileLock{}, fmt.Errorf("open credential lock: %w", err)
	}
	file := os.NewFile(uintptr(fd), path)
	var stat syscall.Stat_t
	if err := syscall.Fstat(fd, &stat); err != nil {
		closeErr := file.Close()
		return fileLock{}, fmt.Errorf("inspect credential lock: %w", errorsJoin(err, closeErr))
	}
	if stat.Mode&syscall.S_IFMT != syscall.S_IFREG || stat.Nlink != 1 {
		closeErr := file.Close()
		validationErr := errors.New("credential lock must be a regular single-link file")
		return fileLock{}, fmt.Errorf("inspect credential lock: %w", errorsJoin(validationErr, closeErr))
	}
	for {
		if err := syscall.Flock(int(file.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err == nil {
			if err := writeLockMetadata(file); err != nil {
				unlockErr := syscall.Flock(int(file.Fd()), syscall.LOCK_UN)
				closeErr := file.Close()
				return fileLock{}, fmt.Errorf("initialize credential lock: %w", errorsJoin(err, errorsJoin(unlockErr, closeErr)))
			}
			return fileLock{file: file}, nil
		} else if err != syscall.EWOULDBLOCK && err != syscall.EAGAIN {
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
	if err := syscall.Flock(int(l.file.Fd()), syscall.LOCK_UN); err != nil {
		closeErr := l.file.Close()
		return fmt.Errorf("unlock credentials: %w", errorsJoin(err, closeErr))
	}
	if err := l.file.Close(); err != nil {
		return fmt.Errorf("close credential lock: %w", err)
	}
	return nil
}
