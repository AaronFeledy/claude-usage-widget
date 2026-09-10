package contract

import "time"

type processWatch interface {
	Wait(time.Duration) error
	KillWait(time.Duration) error
	Close() error
}

type installLock interface{ Close() error }

func stopRecordedProcess(pid int, executable, token string, timeout time.Duration) error {
	watch, err := watchProcess(pid, executable, token)
	if err != nil {
		if processGone(pid) {
			return nil
		}
		return err
	}
	defer watch.Close()
	return watch.KillWait(timeout)
}
