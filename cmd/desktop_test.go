package cmd

import (
	"encoding/json"
	"strings"
	"testing"

	"agentup/internal/desktop"
)

func TestDesktopOfflineListAndAllOutputRemainStructured(t *testing.T) {
	for _, kind := range []string{"desktop", "all"} {
		t.Run(kind, func(t *testing.T) {
			cmd, out, _, mr := setupCommandTest(t)
			cmd.Flags().String("kind", kind, "")
			outputFlags.JSON, outputFlags.Offline = true, true
			if err := runList(cmd, nil); err != nil {
				// A missing platform discovery helper is represented in structured output.
				if _, ok := err.(reportedError); !ok {
					t.Fatal(err)
				}
			}
			if kind == "desktop" {
				var snapshot desktop.Snapshot
				if err := json.Unmarshal(out.Bytes(), &snapshot); err != nil || len(snapshot.Installations) != 4 {
					t.Fatalf("invalid desktop JSON: %s (%v)", out, err)
				}
			} else {
				var result struct {
					CLI     []any            `json:"cli"`
					Desktop desktop.Snapshot `json:"desktop"`
				}
				if err := json.Unmarshal(out.Bytes(), &result); err != nil || len(result.CLI) != 4 || len(result.Desktop.Installations) != 4 {
					t.Fatalf("invalid mixed JSON: %s (%v)", out, err)
				}
			}
			for _, call := range mr.Calls {
				if strings.Contains(strings.Join(call.Args, " "), "@latest") {
					t.Fatalf("offline list attempted an upgrade: %+v", call)
				}
			}
		})
	}
}

func TestDesktopArgumentsRejectIDEAndMixedUpgrade(t *testing.T) {
	for _, tc := range []struct {
		name, kind string
		args       []string
	}{
		{"IDE", "desktop", []string{"antigravity-ide"}},
		{"mixed", "all", nil},
		{"unknown kind", "something", nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cmd, _, _, mr := setupCommandTest(t)
			cmd.Flags().String("kind", tc.kind, "")
			cmd.Flags().Bool("yes", false, "")
			cmd.Flags().String("instance", "", "")
			if err := runDesktopUpgrade(cmd, tc.args); err == nil {
				t.Fatal("invalid desktop target was accepted")
			}
			if len(mr.Calls) != 0 {
				t.Fatal("invalid arguments ran external commands")
			}
		})
	}
}

func TestDesktopOfflineUpgradeRejectsBeforeDiscovery(t *testing.T) {
	cmd, _, _, mr := setupCommandTest(t)
	cmd.Flags().String("kind", "desktop", "")
	outputFlags.Offline = true
	if err := runDesktopUpgrade(cmd, nil); err == nil || len(mr.Calls) != 0 {
		t.Fatalf("offline upgrade reached discovery: %v", err)
	}
}
