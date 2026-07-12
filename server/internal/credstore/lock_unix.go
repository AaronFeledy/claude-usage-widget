//go:build !windows

package credstore

import (
	"context"
	"fmt"
	"os"
	"syscall"
	"time"
)

type fileLock struct{ file *os.File }

func acquireFileLock(ctx context.Context, path string) (fileLock, error) {
	file, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return fileLock{}, fmt.Errorf("open credential lock: %w", err)
	}
	for {
		if err := syscall.Flock(int(file.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err == nil {
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
