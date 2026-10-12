//go:build !windows && !linux && !darwin

package operation

import (
	"fmt"
	"os"
)

func lock(file *os.File, shared bool) error {
	return fmt.Errorf("operation locking is not supported on this platform")
}

func isBusy(err error) bool { return false }
