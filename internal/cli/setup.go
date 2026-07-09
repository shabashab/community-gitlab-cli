package cli

import (
	"fmt"

	"github.com/shabashab/community-gitlab-cli/internal/agenthooks"
	"github.com/shabashab/community-gitlab-cli/internal/cli/output"
	"github.com/spf13/cobra"
)

func newSetupCommand(rootOpts *rootOptions) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "setup",
		Short: "Install agent session integrations",
	}

	cmd.AddCommand(newSetupHooksCommand(rootOpts))

	return cmd
}

func newSetupHooksCommand(rootOpts *rootOptions) *cobra.Command {
	return &cobra.Command{
		Use:   "hooks",
		Short: "Install or repair agent SessionStart hooks for gl-axi ambient context",
		Long: `Install or repair SessionStart integrations so agent sessions start with
gl-axi ambient context (open merge requests of the current repository).

Targets Claude Code (~/.claude/settings.json), Codex (~/.codex/hooks.json and
config.toml), and OpenCode (~/.config/opencode/plugins). Repeated runs are
no-ops; a moved or reinstalled binary is repaired automatically. Unmanaged
configuration is never modified.`,
		Args: wrapArgsValidator(cobra.NoArgs),
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runSetupHooks(cmd, rootOpts)
		},
	}
}

func runSetupHooks(cmd *cobra.Command, rootOpts *rootOptions) error {
	command := agenthooks.PortableCommand(rootOpts.binName, "context")

	results := agenthooks.InstallSessionStartHooks(agenthooks.Options{Command: command})

	targets := make([]output.SetupTargetOutput, 0, len(results))
	for _, result := range results {
		targets = append(targets, output.SetupTargetOutput{
			App:    result.App,
			Path:   result.Path,
			Status: result.Status,
		})
	}

	return output.WriteAxi(cmd.OutOrStdout(), rootOpts.output, output.AxiSetupHooksOutput{
		Hooks: targets,
		Help: []string{
			"Restart your agent session to receive gl-axi ambient context",
			fmt.Sprintf("Hook command: `%s` (directory-scoped, silent outside GitLab repositories)", command),
		},
	})
}
