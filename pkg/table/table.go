package table

import (
	"fmt"
	"io"
	"os"
	"text/tabwriter"

	"agentup/internal/model"
)

// PrintList prints the list of tools and their status as a formatted table.
func PrintList(tools []model.ToolInfo) {
	PrintListTo(os.Stdout, tools)
}

// PrintListTo prints the list of tools to the given writer.
func PrintListTo(w io.Writer, tools []model.ToolInfo) {
	tw := tabwriter.NewWriter(w, 0, 0, 3, ' ', 0)
	defer tw.Flush()

	fmt.Fprintln(tw, "Name\tInstalled\tVersion\tLatest\tUpdate\tInstall Method\tPath\tUpgrade Supported")
	fmt.Fprintln(tw, "----\t---------\t-------\t------\t------\t--------------\t----\t-----------------")

	for _, t := range tools {
		installed := "no"
		if t.Installed {
			installed = "yes"
		}

		version := "-"
		if t.Installed {
			version = t.Version
		}

		latest := "-"
		if t.Installed && t.LatestVersion != "" {
			latest = t.LatestVersion
		}

		update := "-"
		if t.Installed {
			if t.LatestVersion == "" {
				update = "unknown"
			} else if t.UpgradeAvailable {
				update = "yes"
			} else {
				update = "no"
			}
		}

		method := "-"
		if t.Installed {
			method = string(t.InstallMethod)
		}

		path := "-"
		if t.Installed {
			path = t.Path
		}

		upgrade := "-"
		if t.Installed {
			if t.UpgradeSupported {
				upgrade = "yes"
			} else {
				upgrade = "no"
			}
		}

		fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\n", t.Name, installed, version, latest, update, method, path, upgrade)
	}
}

// PrintUpgradeResults prints upgrade results as a formatted summary table.
// Progress messages are printed by the Upgrader during execution;
// this function only prints the final summary.
func PrintUpgradeResults(results []model.UpgradeResult) {
	PrintUpgradeResultsTo(os.Stdout, results)
}

// PrintUpgradeResultsTo prints upgrade results to the given writer.
func PrintUpgradeResultsTo(w io.Writer, results []model.UpgradeResult) {
	fmt.Fprintln(w, "\nSummary:")

	tw := tabwriter.NewWriter(w, 0, 0, 3, ' ', 0)

	for _, r := range results {
		versionChange := ""
		switch r.Status {
		case model.UpgradeStatusSuccess:
			if r.NewVersion != "" && r.NewVersion != "unknown" {
				if r.OldVersion != r.NewVersion && r.OldVersion != "" && r.OldVersion != "unknown" {
					versionChange = fmt.Sprintf("%s -> %s", r.OldVersion, r.NewVersion)
				} else {
					versionChange = fmt.Sprintf("%s (already latest)", r.NewVersion)
				}
			} else if r.Message != "" {
				versionChange = r.Message
			}
		case model.UpgradeStatusSkipped:
			versionChange = r.Message
		case model.UpgradeStatusFailed:
			versionChange = r.Message
		}

		fmt.Fprintf(tw, "%s\t%s\t%s\n", r.Name, r.Status, versionChange)
	}
	tw.Flush()
}

// PrintDoctor prints the doctor diagnosis result.
func PrintDoctor(result model.DoctorResult) {
	PrintDoctorTo(os.Stdout, result)
}

// PrintDoctorTo prints the doctor diagnosis result to the given writer.
func PrintDoctorTo(w io.Writer, result model.DoctorResult) {
	fmt.Fprintf(w, "Operating System: %s\n", result.OS)
	fmt.Fprintln(w)

	fmt.Fprintln(w, "Package Managers:")
	tw := tabwriter.NewWriter(w, 0, 0, 3, ' ', 0)
	fmt.Fprintln(tw, "  Name\tFound\tVersion\tPath")
	fmt.Fprintln(tw, "  ----\t-----\t-------\t----")

	for _, m := range result.Managers {
		found := "no"
		version := "-"
		path := "-"
		if m.Found {
			found = "yes"
			version = m.Version
			path = m.Path
		}
		fmt.Fprintf(tw, "  %s\t%s\t%s\t%s\n", m.Name, found, version, path)
	}
	tw.Flush()

	fmt.Fprintln(w)
	fmt.Fprintln(w, "Agent CLI Tools:")
	tw2 := tabwriter.NewWriter(w, 0, 0, 3, ' ', 0)
	fmt.Fprintln(tw2, "  Name\tFound\tPath")
	fmt.Fprintln(tw2, "  ----\t-----\t----")

	for _, t := range result.Tools {
		found := "no"
		path := "-"
		if t.Found {
			found = "yes"
			path = t.Path
		}
		fmt.Fprintf(tw2, "  %s\t%s\t%s\n", t.Name, found, path)
	}
	tw2.Flush()
}
