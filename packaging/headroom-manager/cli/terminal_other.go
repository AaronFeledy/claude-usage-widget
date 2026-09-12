//go:build !windows

package cli

import "io"

func prepareDashboardTerminal(_ io.Writer, terminal bool) (bool, func()) {
	return terminal, func() {}
}
