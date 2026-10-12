//go:build windows

package operation

import (
	"errors"
	"os"

	"golang.org/x/sys/windows"
)

func lock(file *os.File, shared bool) error {
	flags := uint32(windows.LOCKFILE_FAIL_IMMEDIATELY)
	if !shared {
		flags |= windows.LOCKFILE_EXCLUSIVE_LOCK
	}
	return windows.LockFileEx(windows.Handle(file.Fd()), flags, 0, 1, 0, &windows.Overlapped{})
}

func isBusy(err error) bool { return errors.Is(err, windows.ERROR_LOCK_VIOLATION) }
