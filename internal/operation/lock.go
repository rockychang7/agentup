package operation

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

var ErrBusy = errors.New("another AgentUp operation is active")

// Acquire uses an OS lock so a crashed process cannot leave a stale busy state.
func Acquire(name string) (func(), error) {
	return acquire(name, false)
}

// AcquireShared permits multiple readers while excluding updates and uninstall.
func AcquireShared(name string) (func(), error) {
	return acquire(name, true)
}

func Directory() (string, error) {
	root, err := os.UserCacheDir()
	if err != nil {
		return "", err
	}
	dir := filepath.Join(root, "agentup")
	if err = os.MkdirAll(dir, 0700); err != nil {
		return "", err
	}
	return dir, nil
}

func acquire(name string, shared bool) (func(), error) {
	dir, err := Directory()
	if err != nil {
		return nil, err
	}
	file, err := os.OpenFile(filepath.Join(dir, name+".lock"), os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		return nil, err
	}
	if err = lock(file, shared); err != nil {
		file.Close()
		if isBusy(err) {
			return nil, fmt.Errorf("%w: %v", ErrBusy, err)
		}
		return nil, fmt.Errorf("lock AgentUp operation: %w", err)
	}
	return func() { _ = file.Close() }, nil
}
