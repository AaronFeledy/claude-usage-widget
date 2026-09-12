//go:build !windows

package contract

import (
	"os"
	"strconv"
)

func currentOwnerIdentity() (string, error) { return strconv.Itoa(os.Getuid()), nil }
