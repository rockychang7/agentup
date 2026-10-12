package uisession

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"agentup/internal/operation"
)

const testToken = "AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA"

func isolate(t *testing.T) {
	t.Helper()
	t.Setenv("LOCALAPPDATA", t.TempDir())
	t.Setenv("XDG_CACHE_HOME", t.TempDir())
	t.Setenv("HOME", t.TempDir())
}

func TestRegistryListsMultipleInstancesAndHidesJSONTokens(t *testing.T) {
	isolate(t)
	first, closeFirst, err := Register("http://127.0.0.1:49101/#token="+testToken, "test")
	if err != nil {
		t.Fatal(err)
	}
	defer closeFirst()
	_, closeSecond, err := Register("http://127.0.0.1:49102/#token="+testToken, "test")
	if err != nil {
		t.Fatal(err)
	}
	defer closeSecond()
	if _, cleanup, err := Register(first.URL(), "test"); err == nil {
		cleanup()
		t.Fatal("registered the same port twice")
	}
	instances, err := List()
	if err != nil || len(instances) != 2 || instances[0].URL() != first.URL() {
		t.Fatalf("incorrect registry: %+v, %v", instances, err)
	}
	data, _ := json.Marshal(instances)
	if strings.Contains(string(data), testToken) || strings.Contains(string(data), `"token"`) {
		t.Fatal("status JSON disclosed the session token")
	}
	closeSecond()
	instances, err = List()
	if err != nil || len(instances) != 1 || instances[0].Port != first.Port {
		t.Fatalf("stopping one instance affected another: %+v, %v", instances, err)
	}
}

func TestStaleRecordIsPrunedWithoutContactingReusedPort(t *testing.T) {
	isolate(t)
	dir, err := directory()
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "49103.json")
	if err := os.WriteFile(path, []byte(`{"pid":1,"port":49103,"token":"`+testToken+`"}`), 0600); err != nil {
		t.Fatal(err)
	}
	instances, err := List()
	if err != nil || len(instances) != 0 {
		t.Fatal("stale record was reported as running", instances, err)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatal("stale record was not removed", err)
	}
}

func TestInvalidRecordsAndLaunchURLsAreRejected(t *testing.T) {
	isolate(t)
	for _, launchURL := range []string{
		"http://evil.example:49101/#token=" + testToken,
		"http://127.0.0.1:0/#token=" + testToken,
		"http://127.0.0.1:99999/#token=" + testToken,
		"http://127.0.0.1:49101/#token=short",
		"http://127.0.0.1:49101/#token=" + strings.Repeat("!", 43),
	} {
		if _, cleanup, err := Register(launchURL, "test"); err == nil {
			cleanup()
			t.Fatal("unsafe launch URL accepted", launchURL)
		}
	}
	dir, _ := directory()
	release, err := operation.Acquire(lockName(49104))
	if err != nil {
		t.Fatal(err)
	}
	defer release()
	path := filepath.Join(dir, "49104.json")
	if err := os.WriteFile(path, []byte(`{"pid":1,"port":49105,"token":"`+testToken+`"}`), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := List(); err == nil {
		t.Fatal("record could redirect control to another port")
	}
}
