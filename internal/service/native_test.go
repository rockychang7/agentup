package service

import (
	"reflect"
	"testing"

	"agentup/internal/model"
	"agentup/internal/runner"
)

func TestNativeNpm12PreviewMatchesExecution(t *testing.T) {
	for _, tt := range []struct {
		name        model.ToolName
		binary, pkg string
	}{
		{model.ToolClaudeCode, "claude", "@anthropic-ai/claude-code"},
		{model.ToolOpenCode, "opencode", "@opencode/cli"},
	} {
		t.Run(string(tt.name), func(t *testing.T) {
			s, mr := newTestService(t)
			binary := "/mock/" + tt.binary
			mr.SetLookPath(tt.binary, binary, nil)
			mr.SetResult(binary, []string{"--version"}, runner.MockResult{Stdout: "2.2.6"})
			mr.SetResult("npm", []string{"--version"}, runner.MockResult{Stdout: "12.2.2"})
			mr.SetResult("npm", []string{"list", "-g", tt.pkg, "--depth=0"}, runner.MockResult{Stdout: tt.pkg + "@2.2.6"})
			mr.SetResult("npm", []string{"view", tt.pkg, "version"}, runner.MockResult{Stdout: "2.2.6"})
			args := []string{"install", "-g", "--include=optional", "--ignore-scripts=false", "--allow-scripts=" + tt.pkg, tt.pkg + "@latest"}
			command := append([]string{"npm"}, args...)
			mr.SetResult("npm", args, runner.MockResult{Stdout: "postinstall completed\n"})
			if err := s.Refresh(false); err != nil {
				t.Fatal(err)
			}
			state := waitState(t, s, func(state State) bool { return !state.Scanning })
			for _, agent := range state.Agents {
				if agent.Name == tt.name && !reflect.DeepEqual(agent.Command, command) {
					t.Fatalf("unsafe preview: %+v", agent)
				}
			}
			if _, err := s.Upgrade([]model.ToolName{tt.name}, state.Revision); err != nil {
				t.Fatal(err)
			}
			state = waitState(t, s, func(state State) bool { return len(state.ActiveTasks) == 0 && len(state.Tasks) > 0 })
			result := state.Tasks[0].Tools[0].Result
			if state.Tasks[0].Status != "completed" || result == nil || !reflect.DeepEqual(result.Command, command) {
				t.Fatalf("preview/execution diverged: %+v", state.Tasks[0])
			}
		})
	}
}
