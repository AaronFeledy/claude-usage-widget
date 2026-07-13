package credstore

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

func AtomicUpdate(ctx context.Context, path string, mutator Mutator) (err error) {
	if mutator == nil {
		return ErrNilMutator
	}
	lock, err := acquireFileLock(ctx, path+".lock")
	if err != nil {
		return err
	}
	defer func() {
		err = errors.Join(err, lock.Close())
	}()

	snapshot, err := LoadSnapshot(ctx, path)
	if err != nil {
		return err
	}
	if !json.Valid(snapshot.Data) {
		return fmt.Errorf("current credentials %s: %w", path, ErrInvalidJSON)
	}
	updated, err := mutator(append([]byte(nil), snapshot.Data...))
	if err != nil {
		return fmt.Errorf("mutate credentials %s: %w", path, err)
	}
	if !json.Valid(updated) {
		return fmt.Errorf("updated credentials %s: %w", path, ErrInvalidJSON)
	}
	select {
	case <-ctx.Done():
		return fmt.Errorf("commit credentials canceled: %w", ctx.Err())
	default:
	}
	return replaceAtomically(path, updated, snapshot.Mode)
}

func replaceAtomically(path string, data []byte, mode os.FileMode) (err error) {
	dir := filepath.Dir(path)
	tmp, err := os.CreateTemp(dir, "."+filepath.Base(path)+".tmp-*")
	if err != nil {
		return fmt.Errorf("create temp credential file: %w", err)
	}
	tmpName := tmp.Name()
	tmpOpen := true
	defer func() {
		if err != nil {
			err = errors.Join(err, os.Remove(tmpName))
		}
	}()
	defer func() {
		if tmpOpen {
			err = errors.Join(err, tmp.Close())
		}
	}()

	if err := tmp.Chmod(mode); err != nil {
		return fmt.Errorf("chmod temp credential file: %w", err)
	}
	if _, err := tmp.Write(data); err != nil {
		return fmt.Errorf("write temp credential file: %w", err)
	}
	if err := tmp.Sync(); err != nil {
		return fmt.Errorf("sync temp credential file: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("close temp credential file: %w", err)
	}
	tmpOpen = false
	if err := replaceExisting(tmpName, path); err != nil {
		return fmt.Errorf("replace credential file: %w", err)
	}
	if err := syncDirectory(dir); err != nil {
		return fmt.Errorf("sync credential directory: %w", err)
	}
	return nil
}
