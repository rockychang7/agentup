package cmd

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/spf13/cobra"

	"agentup/pkg/table"
)

func commandContext(cmd *cobra.Command) context.Context {
	if ctx := cmd.Context(); ctx != nil {
		return ctx
	}
	return context.Background()
}

var outputFlags = struct {
	table.Options
	JSON    bool
	Offline bool
}{Options: table.Options{Color: "auto"}}

// reportedError preserves a failure exit code when a structured summary already explains it.
type reportedError struct{ message string }

func (e reportedError) Error() string { return e.message }

func writeJSON(cmd *cobra.Command, value any) error {
	encoder := json.NewEncoder(cmd.OutOrStdout())
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(value); err != nil {
		return fmt.Errorf("write JSON: %w", err)
	}
	return nil
}

func printer(cmd *cobra.Command) *table.Printer {
	return table.New(cmd.OutOrStdout(), presentationOptions())
}

func notices(cmd *cobra.Command) *table.Printer {
	return table.New(cmd.ErrOrStderr(), presentationOptions())
}

func presentationOptions() table.Options {
	opts := outputFlags.Options
	if outputFlags.JSON {
		opts.Color = "never"
	}
	return opts
}
