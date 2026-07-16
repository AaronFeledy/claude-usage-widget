package credstore

import (
	"fmt"
	"io"
	"os"
	"strconv"
	"time"
)

func writeLockMetadata(file *os.File) error {
	if err := file.Truncate(0); err != nil {
		return fmt.Errorf("truncate credential lock metadata: %w", err)
	}
	if _, err := file.Seek(0, io.SeekStart); err != nil {
		return fmt.Errorf("seek credential lock metadata: %w", err)
	}
	metadata := strconv.Itoa(os.Getpid()) + ":" + strconv.FormatInt(time.Now().Unix(), 10)
	if _, err := io.WriteString(file, metadata); err != nil {
		return fmt.Errorf("write credential lock metadata: %w", err)
	}
	return nil
}
