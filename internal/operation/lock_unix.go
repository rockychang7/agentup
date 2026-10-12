//go:build linux || darwin

package operation

import (
	"errors"
	"os"

	"golang.org/x/sys/unix"
)

func lock(file *os.File, shared bool) error {
	mode := unix.LOCK_EX
	if shared {
		mode = unix.LOCK_SH
	}
	return unix.Flock(int(file.Fd()), mode|unix.LOCK_NB)
}

func isBusy(err error) bool { return errors.Is(err, unix.EWOULDBLOCK) }
