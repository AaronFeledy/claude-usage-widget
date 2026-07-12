//go:build !windows

package credstore

import (
	"fmt"
	"os"
)

func replaceExisting(tmpName string, path string) error {
	if err := os.Rename(tmpName, path); err != nil {
		return fmt.Errorf("rename over destination: %w", err)
	}
	return nil
}

func syncDirectory(path string) (err error) {
	dir, err := os.Open(path)
	if err != nil {
		return fmt.Errorf("open directory: %w", err)
	}
	defer func() {
		if closeErr := dir.Close(); closeErr != nil {
			err = errorsJoin(err, fmt.Errorf("close directory: %w", closeErr))
		}
	}()
	if err := dir.Sync(); err != nil {
		return fmt.Errorf("sync directory: %w", err)
	}
	return nil
}
