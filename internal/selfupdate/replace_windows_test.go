//go:build windows

package selfupdate

import (
	"bytes"
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"golang.org/x/sys/windows"
)

func TestUpdateInstallsWindowsReleaseBeforeReturning(t *testing.T) {
	assetName := "agentup_0.6.1_windows_amd64.zip"
	archiveData := makeZipArchive(t, "agentup.exe", []byte("new executable"))
	server := newReleaseServer(t, "v0.6.1", assetName, archiveData, checksumFile(assetName, archiveData))
	defer server.Close()

	target := filepath.Join(t.TempDir(), "agentup.exe")
	if err := os.WriteFile(target, []byte("old executable"), 0o755); err != nil {
		t.Fatal(err)
	}
	updater := New("0.6.0")
	updater.apiURL = server.URL + "/latest"
	updater.client = server.Client()
	updater.executablePath = func() (string, error) { return target, nil }
	result, err := updater.Update(context.Background(), false)
	if err != nil || !result.Updated || result.PendingRestart {
		t.Fatalf("update must finish synchronously: result=%+v, err=%v", result, err)
	}
	if data, err := os.ReadFile(target); err != nil || string(data) != "new executable" {
		t.Fatalf("update reported success without installing: %q, %v", data, err)
	}
	staged, err := filepath.Glob(filepath.Join(filepath.Dir(target), ".agentup-update-*"))
	if err != nil || len(staged) != 0 {
		t.Fatalf("completed update left staged executables: %v, %v", staged, err)
	}
}

func TestReplaceExecutableInstallsImmediately(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "agentup.exe")
	staged := filepath.Join(dir, "update.exe")
	for path, content := range map[string]string{
		target: "old", target + ".old": "previous backup", staged: "new",
	} {
		if err := os.WriteFile(path, []byte(content), 0o755); err != nil {
			t.Fatal(err)
		}
	}

	pending, err := replaceExecutable(staged, target)
	if err != nil || pending {
		t.Fatalf("replacement must finish before returning: pending=%v, err=%v", pending, err)
	}
	if data, err := os.ReadFile(target); err != nil || string(data) != "new" {
		t.Fatalf("new executable was not installed: %q, %v", data, err)
	}
	for _, path := range []string{staged, target + ".old"} {
		if _, err := os.Stat(path); !os.IsNotExist(err) {
			t.Fatalf("replacement left %s: %v", path, err)
		}
	}
}

func TestReplaceExecutableRestoresOriginalWhenInstallFails(t *testing.T) {
	target := filepath.Join(t.TempDir(), "agentup.exe")
	if err := os.WriteFile(target, []byte("original"), 0o755); err != nil {
		t.Fatal(err)
	}
	pending, err := replaceExecutable(filepath.Join(t.TempDir(), "missing.exe"), target)
	if err == nil || pending || !strings.Contains(err.Error(), "original restored") {
		t.Fatalf("failed installation must report rollback: pending=%v, err=%v", pending, err)
	}
	if data, err := os.ReadFile(target); err != nil || string(data) != "original" {
		t.Fatalf("original executable was not restored: %q, %v", data, err)
	}
	if _, err := os.Stat(target + ".old"); !os.IsNotExist(err) {
		t.Fatalf("rollback left a backup: %v", err)
	}
}

func TestReplaceExecutableDoesNotModifyLockedFiles(t *testing.T) {
	for _, locked := range []string{"target", "backup"} {
		t.Run(locked, func(t *testing.T) {
			dir := t.TempDir()
			target := filepath.Join(dir, "agentup.exe")
			staged := filepath.Join(dir, "update.exe")
			lockedPath := target
			if locked == "backup" {
				lockedPath += ".old"
			}
			for _, path := range []string{target, staged, lockedPath} {
				if err := os.WriteFile(path, []byte("unchanged"), 0o755); err != nil {
					t.Fatal(err)
				}
			}
			path, err := windows.UTF16PtrFromString(lockedPath)
			if err != nil {
				t.Fatal(err)
			}
			handle, err := windows.CreateFile(path, windows.GENERIC_READ, windows.FILE_SHARE_READ,
				nil, windows.OPEN_EXISTING, windows.FILE_ATTRIBUTE_NORMAL, 0)
			if err != nil {
				t.Fatal(err)
			}
			defer windows.CloseHandle(handle)

			if pending, err := replaceExecutable(staged, target); err == nil || pending {
				t.Fatalf("locked %s must fail synchronously: pending=%v, err=%v", locked, pending, err)
			}
			for _, path := range []string{target, staged, lockedPath} {
				if data, err := os.ReadFile(path); err != nil || string(data) != "unchanged" {
					t.Fatalf("locked replacement modified %s: %q, %v", path, data, err)
				}
			}
		})
	}
}

func TestReplaceExecutableWhileRunning(t *testing.T) {
	if os.Getenv("AGENTUP_SELFUPDATE_TEST_CHILD") == "1" {
		target, err := os.Executable()
		if err != nil {
			t.Fatal(err)
		}
		pending, err := replaceExecutable(target+".staged", target)
		if err != nil || pending {
			t.Fatalf("running executable must be replaced immediately: pending=%v, err=%v", pending, err)
		}
		if data, err := os.ReadFile(target); err != nil || string(data) != "new executable" {
			t.Fatalf("replacement is not visible while original is running: %q, %v", data, err)
		}
		return
	}

	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	original, err := os.ReadFile(executable)
	if err != nil {
		t.Fatal(err)
	}
	dir := filepath.Join(t.TempDir(), "\u8def\u5f84 with spaces")
	if err := os.Mkdir(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(dir, "agentup.exe")
	if err := os.WriteFile(target, original, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(target+".staged", []byte("new executable"), 0o755); err != nil {
		t.Fatal(err)
	}
	command := exec.Command(target, "-test.run=^TestReplaceExecutableWhileRunning$", "-test.v")
	command.Env = append(os.Environ(), "AGENTUP_SELFUPDATE_TEST_CHILD=1")
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("running executable replacement failed: %v\n%s", err, output)
	}
	if data, err := os.ReadFile(target + ".old"); err != nil || !bytes.Equal(data, original) {
		t.Fatalf("running original was not preserved until exit: err=%v", err)
	}
	// The next update must remove the backup left by the running original.
	if err := os.WriteFile(target+".staged", []byte("next executable"), 0o755); err != nil {
		t.Fatal(err)
	}
	if pending, err := replaceExecutable(target+".staged", target); err != nil || pending {
		t.Fatalf("update with a previous running backup failed: pending=%v, err=%v", pending, err)
	}
	if _, err := os.Stat(target + ".old"); !os.IsNotExist(err) {
		t.Fatalf("previous backup was not cleaned up: %v", err)
	}
}
