package contract

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// MaterializeLinks replaces ordinary in-tree file and directory symlinks with
// regular copies. Native macOS framework links are preserved for code signing
// and are authenticated separately by the package manifest.
func MaterializeLinks(root string) error {
	root, err := filepath.Abs(root)
	if err != nil {
		return err
	}
	root, err = filepath.EvalSymlinks(root)
	if err != nil {
		return err
	}
	var links []string
	err = filepath.Walk(root, func(name string, info os.FileInfo, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if info.Mode()&os.ModeSymlink != 0 {
			if strings.Contains(filepath.ToSlash(name), ".framework/") {
				return nil
			}
			links = append(links, name)
		}
		return nil
	})
	if err != nil {
		return err
	}
	sort.Slice(links, func(i, j int) bool {
		depthI := strings.Count(links[i], string(os.PathSeparator))
		depthJ := strings.Count(links[j], string(os.PathSeparator))
		if depthI != depthJ {
			return depthI > depthJ
		}
		return links[i] < links[j]
	})
	budget := materializeBudget{}
	for _, link := range links {
		info, err := os.Lstat(link)
		if errors.Is(err, os.ErrNotExist) {
			continue
		}
		if err != nil {
			return err
		}
		if info.Mode()&os.ModeSymlink == 0 {
			continue
		}
		resolved, err := resolveInRoot(root, link)
		if err != nil {
			return err
		}
		temporary := link + ".materialized"
		if _, err = os.Lstat(temporary); !errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("materialization target already exists: %s", temporary)
		}
		if err = copyMaterialized(root, resolved, temporary, map[string]bool{}, &budget); err != nil {
			_ = os.RemoveAll(temporary)
			return err
		}
		if err = os.Remove(link); err != nil {
			_ = os.RemoveAll(temporary)
			return err
		}
		if err = os.Rename(temporary, link); err != nil {
			return err
		}
	}
	return nil
}

type materializeBudget struct {
	entries int
	bytes   int64
}

func resolveInRoot(root, name string) (string, error) {
	resolved, err := filepath.EvalSymlinks(name)
	if err != nil {
		return "", err
	}
	resolved, err = filepath.Abs(resolved)
	if err != nil {
		return "", err
	}
	if resolved != root && !strings.HasPrefix(resolved, root+string(os.PathSeparator)) {
		return "", fmt.Errorf("link escapes package root: %s", name)
	}
	return resolved, nil
}

func copyMaterialized(root, source, destination string, stack map[string]bool, budget *materializeBudget) error {
	info, err := os.Lstat(source)
	if err != nil {
		return err
	}
	if info.Mode()&os.ModeSymlink != 0 {
		resolved, err := resolveInRoot(root, source)
		if err != nil {
			return err
		}
		return copyMaterialized(root, resolved, destination, stack, budget)
	}
	budget.entries++
	if budget.entries > maxPackageEntries {
		return errors.New("materialized tree has too many entries")
	}
	if info.Mode().IsRegular() {
		budget.bytes += info.Size()
		if info.Size() > maxPackageFileBytes || budget.bytes > maxPackageBytes {
			return errors.New("materialized tree exceeds package size limits")
		}
		return copyFile(source, destination, info.Mode().Perm())
	}
	if !info.IsDir() {
		return fmt.Errorf("link target is not a regular file or directory: %s", source)
	}
	real, err := filepath.EvalSymlinks(source)
	if err != nil {
		return err
	}
	if stack[real] {
		return fmt.Errorf("directory link cycle: %s", source)
	}
	stack[real] = true
	defer delete(stack, real)
	if err = os.Mkdir(destination, info.Mode().Perm()); err != nil {
		return err
	}
	entries, err := os.ReadDir(source)
	if err != nil {
		return err
	}
	for _, entry := range entries {
		if err = copyMaterialized(root, filepath.Join(source, entry.Name()), filepath.Join(destination, entry.Name()), stack, budget); err != nil {
			return err
		}
	}
	return os.Chmod(destination, info.Mode().Perm())
}
