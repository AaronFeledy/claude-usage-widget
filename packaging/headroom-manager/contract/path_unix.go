//go:build !windows

package contract

import "os"

func pathIsLinkOrReparse(_ string, info os.FileInfo) bool {
	return info.Mode()&os.ModeSymlink != 0
}

func validBackupMode(mode os.FileMode) bool         { return mode != 0 && mode <= 0o777 && mode&0o022 == 0 }
func backupModesEqual(left, right os.FileMode) bool { return left.Perm() == right.Perm() }
