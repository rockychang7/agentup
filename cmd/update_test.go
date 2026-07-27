package cmd

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/spf13/cobra"

	"agentup/internal/selfupdate"
)

type fakeSelfUpdater struct {
	result selfupdate.Result
	err    error
	called bool
	force  bool
}

func (u *fakeSelfUpdater) Update(_ context.Context, force bool) (selfupdate.Result, error) {
	u.called = true
	u.force = force
	return u.result, u.err
}

func TestRunSelfUpdateSuccess(t *testing.T) {
	originalUpdater := appSelfUpdater
	originalVersion := Version
	t.Cleanup(func() {
		appSelfUpdater = originalUpdater
		Version = originalVersion
	})

	Version = "0.1.0"
	fake := &fakeSelfUpdater{
		result: selfupdate.Result{
			CurrentVersion: "0.1.0",
			LatestVersion:  "v0.2.0",
			Updated:        true,
		},
	}
	appSelfUpdater = fake

	var output bytes.Buffer
	command := &cobra.Command{}
	command.SetOut(&output)

	if err := runSelfUpdate(command, true); err != nil {
		t.Fatalf("run self-update: %v", err)
	}
	if !fake.force {
		t.Fatal("expected force flag to be forwarded")
	}
	if !strings.Contains(output.String(), "Updated agentup from 0.1.0 to v0.2.0") {
		t.Fatalf("unexpected output %q", output.String())
	}
}

func TestUpgradeCommandRoutesAgentupToSelfUpdater(t *testing.T) {
	originalUpdater := appSelfUpdater
	originalVersion := Version
	t.Cleanup(func() {
		appSelfUpdater = originalUpdater
		Version = originalVersion
	})

	Version = "0.1.0"
	fake := &fakeSelfUpdater{
		result: selfupdate.Result{
			CurrentVersion: "0.1.0",
			LatestVersion:  "v0.2.0",
			Updated:        true,
		},
	}
	appSelfUpdater = fake

	var output bytes.Buffer
	command := &cobra.Command{}
	command.SetOut(&output)

	if err := upgradeCmd.RunE(command, []string{"agentup"}); err != nil {
		t.Fatalf("run agentup upgrade route: %v", err)
	}
	if !fake.called {
		t.Fatal("expected agentup upgrade route to call the self-updater")
	}
	if fake.force {
		t.Fatal("expected agentup upgrade route to use normal version checks")
	}
	if !strings.Contains(output.String(), "Updated agentup from 0.1.0 to v0.2.0") {
		t.Fatalf("unexpected output %q", output.String())
	}
}

func TestRunSelfUpdateAlreadyCurrent(t *testing.T) {
	originalUpdater := appSelfUpdater
	t.Cleanup(func() {
		appSelfUpdater = originalUpdater
	})

	appSelfUpdater = &fakeSelfUpdater{
		result: selfupdate.Result{
			CurrentVersion: "0.2.0",
			LatestVersion:  "v0.2.0",
			SkippedReason:  "already up to date",
		},
	}

	var output bytes.Buffer
	command := &cobra.Command{}
	command.SetOut(&output)

	if err := runSelfUpdate(command, false); err != nil {
		t.Fatalf("run self-update: %v", err)
	}
	if !strings.Contains(output.String(), "already up to date") {
		t.Fatalf("unexpected output %q", output.String())
	}
}

func TestRunSelfUpdateError(t *testing.T) {
	originalUpdater := appSelfUpdater
	t.Cleanup(func() {
		appSelfUpdater = originalUpdater
	})

	appSelfUpdater = &fakeSelfUpdater{err: errors.New("network unavailable")}
	command := &cobra.Command{}
	command.SetOut(&bytes.Buffer{})

	err := runSelfUpdate(command, false)
	if err == nil || !strings.Contains(err.Error(), "network unavailable") {
		t.Fatalf("expected update error, got %v", err)
	}
}
