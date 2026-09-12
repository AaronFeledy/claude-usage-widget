package contract

import "os"

// CurrentProcessToken identifies this kernel process, not merely its PID.
func CurrentProcessToken() (string, error) {
	executable, err := os.Executable()
	if err != nil {
		return "", err
	}
	return captureProcessToken(os.Getpid(), executable)
}
