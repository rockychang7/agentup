package service

import (
	"path/filepath"
	"reflect"
	"testing"

	"agentup/internal/model"
	"agentup/internal/runner"
)

func setupOpenCodeNpm(mr *runner.MockRunner, version, pkg string) {
	mr.SetLookPath("opencode", "/mock/opencode", nil)
	mr.SetResult("/mock/opencode", []string{"--version"}, runner.MockResult{Stdout: version})
	mr.SetResult("npm", []string{"--version"}, runner.MockResult{Stdout: "10.0.0"})
	mr.SetResult("npm", []string{"list", "-g", pkg, "--depth=0"}, runner.MockResult{Stdout: pkg + "@" + version})
	mr.SetResult("npm", []string{"view", pkg, "version"}, runner.MockResult{Stdout: version})
	mr.SetResult("npm", []string{"install", "-g", pkg + "@latest"}, runner.MockResult{})
}

func TestOpenCodePreviewMatchesReleaseLine(t *testing.T) {
	for _, tt := range []struct {
		version, pkg string
		native       bool
	}{
		{"1.19.0", "opencode-ai", false},
		{"2.0.29", "@opencode/cli", false},
		{"1.19.0", "", true},
		{"2.0.29", "", true},
		{"unknown", "", false},
		{"2.1.0-beta.1", "", false},
	} {
		name := tt.version + "/npm"
		if tt.native {
			name = tt.version + "/native"
		}
		t.Run(name, func(t *testing.T) {
			s, mr := newTestService(t)
			binary := "/mock/opencode"
			var want []string
			if tt.pkg != "" {
				setupOpenCodeNpm(mr, tt.version, tt.pkg)
				want = []string{"npm", "install", "-g", tt.pkg + "@latest"}
			} else {
				if tt.native {
					home := t.TempDir()
					t.Setenv("HOME", home)
					t.Setenv("USERPROFILE", home)
					binary = filepath.Join(home, ".opencode", "bin", "opencode")
					want = []string{binary, "upgrade", "--method", "curl"}
				}
				mr.SetLookPath("opencode", binary, nil)
				mr.SetResult(binary, []string{"--version"}, runner.MockResult{Stdout: tt.version})
			}
			if err := s.Refresh(true); err != nil {
				t.Fatal(err)
			}
			state := waitState(t, s, func(state State) bool { return !state.Scanning })
			var agent Agent
			for _, a := range state.Agents {
				if a.Name == model.ToolOpenCode {
					agent = a
				}
			}
			if !agent.Installed || agent.Path != binary || !reflect.DeepEqual(agent.Command, want) ||
				agent.UpgradeSupported != (want != nil) {
				t.Fatalf("incorrect service preview: %+v, want %v", agent, want)
			}
		})
	}
}

func TestOpenCodeServiceUsesConfirmedPackage(t *testing.T) {
	for _, tt := range []struct{ version, pkg string }{
		{"1.19.0", "opencode-ai"},
		{"2.0.29", "@opencode/cli"},
	} {
		t.Run(tt.version, func(t *testing.T) {
			s, mr := newTestService(t)
			setupOpenCodeNpm(mr, tt.version, tt.pkg)
			if err := s.Refresh(false); err != nil {
				t.Fatal(err)
			}
			state := waitState(t, s, func(state State) bool { return !state.Scanning })
			if _, err := s.Upgrade([]model.ToolName{model.ToolOpenCode}, state.Revision); err != nil {
				t.Fatal(err)
			}
			state = waitState(t, s, func(state State) bool {
				return !state.Scanning && state.ActiveTask == "" && len(state.Tasks) > 0
			})
			result := state.Tasks[0].Tools[0].Result
			want := []string{"npm", "install", "-g", tt.pkg + "@latest"}
			if state.Tasks[0].Status != "completed" || result == nil || !reflect.DeepEqual(result.Command, want) {
				t.Fatalf("incorrect service upgrade: %+v", state.Tasks[0])
			}
		})
	}
}

func TestOpenCodeServiceRejectsChangedReleaseLine(t *testing.T) {
	s, mr := newTestService(t)
	setupOpenCodeNpm(mr, "1.19.0", "opencode-ai")
	if err := s.Refresh(false); err != nil {
		t.Fatal(err)
	}
	state := waitState(t, s, func(state State) bool { return !state.Scanning })
	setupOpenCodeNpm(mr, "2.0.29", "@opencode/cli")
	if _, err := s.Upgrade([]model.ToolName{model.ToolOpenCode}, state.Revision); err != nil {
		t.Fatal(err)
	}
	state = waitState(t, s, func(state State) bool {
		return !state.Scanning && state.ActiveTask == "" && len(state.Tasks) > 0
	})
	result := state.Tasks[0].Tools[0].Result
	if state.Tasks[0].Status != "failed" || result == nil || result.Outcome != model.OutcomeFailed {
		t.Fatalf("stale release line was accepted: %+v", state.Tasks[0])
	}
	for _, call := range mr.Calls {
		if call.Name == "npm" && len(call.Args) > 0 && call.Args[0] == "install" {
			t.Fatalf("stale confirmation ran a mutation: %+v", call)
		}
	}
}
