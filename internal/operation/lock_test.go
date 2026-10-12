package operation

import (
	"errors"
	"testing"
)

func TestLockExcludesOtherHandlesAndReleases(t *testing.T) {
	t.Setenv("LOCALAPPDATA", t.TempDir())
	t.Setenv("XDG_CACHE_HOME", t.TempDir())
	t.Setenv("HOME", t.TempDir())
	release, err := Acquire("mutation")
	if err != nil {
		t.Fatal(err)
	}
	if second, err := Acquire("mutation"); err == nil {
		second()
		release()
		t.Fatal("a second handle acquired the lock")
	}
	release()
	again, err := Acquire("mutation")
	if err != nil {
		t.Fatalf("lock survived close: %v", err)
	}
	again()
}

func TestSharedUILocksExcludeUpdatesUntilEveryInstanceStops(t *testing.T) {
	t.Setenv("LOCALAPPDATA", t.TempDir())
	t.Setenv("XDG_CACHE_HOME", t.TempDir())
	t.Setenv("HOME", t.TempDir())
	first, err := AcquireShared("ui")
	if err != nil {
		t.Fatal(err)
	}
	second, err := AcquireShared("ui")
	if err != nil {
		first()
		t.Fatal("second UI instance was rejected", err)
	}
	for _, stop := range []func(){first, second} {
		if release, err := Acquire("ui"); !errors.Is(err, ErrBusy) {
			if release != nil {
				release()
			}
			t.Fatal("exclusive lock ignored a running UI", err)
		}
		stop()
	}
	update, err := Acquire("ui")
	if err != nil {
		t.Fatal("stopped UIs still block update", err)
	}
	if release, err := AcquireShared("ui"); !errors.Is(err, ErrBusy) {
		if release != nil {
			release()
		}
		update()
		t.Fatal("UI started during update", err)
	}
	update()
}
