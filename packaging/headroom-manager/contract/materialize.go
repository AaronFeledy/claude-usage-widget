package contract

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// MaterializeLinks replaces in-tree file symlinks with regular copies. Linux
// Qt installs use SONAME links; portable archives forbid links and retain each
// DT_NEEDED filename as a real file.
func MaterializeLinks(root string) error {
	root, err := filepath.Abs(root)
	if err != nil {
		return err
	}
	var links []string
	err = filepath.Walk(root, func(name string, info os.FileInfo, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if info.Mode()&os.ModeSymlink != 0 {
			links = append(links, name)
		}
		return nil
	})
	if err != nil {
		return err
	}
	for _, link := range links {
		info, err := os.Lstat(link)
		if err != nil {
			return err
		}
		if info.IsDir() {
			return fmt.Errorf("directory link is forbidden: %s", link)
		}
		resolved, err := filepath.EvalSymlinks(link)
		if err != nil {
			return err
		}
		resolved, err = filepath.Abs(resolved)
		if err != nil {
			return err
		}
		prefix := root + string(os.PathSeparator)
		if resolved != root && !strings.HasPrefix(resolved, prefix) {
			return fmt.Errorf("link escapes package root: %s", link)
		}
		target, err := os.Stat(resolved)
		if err != nil {
			return err
		}
		if !target.Mode().IsRegular() {
			return fmt.Errorf("link target is not a regular file: %s", link)
		}
		temporary := link + ".materialized"
		if err = copyFile(resolved, temporary, target.Mode().Perm()); err != nil {
			return err
		}
		if err = os.Remove(link); err != nil {
			os.Remove(temporary)
			return err
		}
		if err = os.Rename(temporary, link); err != nil {
			return err
		}
	}
	return nil
}
