/*
Copyright © 2026 Artem Tarasenko <shabashab.04@gmail.com>
*/
package cli

import (
	"os"

	"github.com/spf13/cobra"
)

const glLongDescription = `Community GitLab CLI works with GitLab through the personal access token API.

It is designed for predictable command behavior, script-friendly output,
clear failure modes, and workflows that coding agents can inspect and automate.`

const glAxiLongDescription = `gl-axi is the axi-oriented Community GitLab CLI entry point.

It works with GitLab through the personal access token API and is intended for
agentic workflows based on the axi standard introduced by Kun Chen.`

func newRootCommand(use, short, long string) *cobra.Command {
	rootCmd := &cobra.Command{
		Use:   use,
		Short: short,
		Long:  long,
	}

	rootCmd.Flags().BoolP("toggle", "t", false, "Help message for toggle")

	return rootCmd
}

// Execute runs the primary gl binary entry point.
func Execute() {
	err := newRootCommand(
		"gl",
		"Community GitLab CLI for agentic workflows",
		glLongDescription,
	).Execute()
	if err != nil {
		os.Exit(1)
	}
}

// ExecuteAxi runs the axi-oriented gl-axi binary entry point.
func ExecuteAxi() {
	err := newRootCommand(
		"gl-axi",
		"Axi-oriented Community GitLab CLI for agentic workflows",
		glAxiLongDescription,
	).Execute()
	if err != nil {
		os.Exit(1)
	}
}
