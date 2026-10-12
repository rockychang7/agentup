package operation

import "testing"

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
