/*
Copyright © 2026 Artem Tarasenko <shabashab.04@gmail.com>
*/
package cli

import (
	"fmt"
	"os"

	"github.com/shabashab/community-gitlab-cli/internal/gitlabclient"
	"github.com/spf13/cobra"
	gitlab "gitlab.com/gitlab-org/api/client-go/v2"
)

const glLongDescription = `Community GitLab CLI works with GitLab through the personal access token API.

It is designed for predictable command behavior, script-friendly output,
clear failure modes, and workflows that coding agents can inspect and automate.`

const glAxiLongDescription = `gl-axi is the axi-oriented Community GitLab CLI entry point.

It works with GitLab through the personal access token API and is intended for
agentic workflows based on the axi standard introduced by Kun Chen.`

type rootOptions struct {
	gitlabToken   string
	gitlabBaseURL string
	output        string
	mode          commandMode
}

type commandMode string

const (
	commandModeStandard commandMode = "standard"
	commandModeAxi      commandMode = "axi"
)

func newRootCommand(use, short, long string, mode commandMode) *cobra.Command {
	opts := &rootOptions{
		output: defaultOutputFormat(mode),
		mode:   mode,
	}

	rootCmd := &cobra.Command{
		Use:           use,
		Short:         short,
		Long:          long,
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if opts.mode == commandModeAxi {
				return runWhoami(cmd, opts)
			}

			return cmd.Help()
		},
	}

	rootCmd.PersistentFlags().StringVar(
		&opts.gitlabToken,
		"gitlab-token",
		"",
		fmt.Sprintf("GitLab personal access token (env %s or %s)", gitlabclient.TokenEnv, gitlabclient.AlternateTokenEnv),
	)
	rootCmd.PersistentFlags().StringVar(
		&opts.gitlabBaseURL,
		"gitlab-base-url",
		"",
		fmt.Sprintf("GitLab instance URL (env %s, default %s)", gitlabclient.BaseURLEnv, gitlabclient.DefaultBaseURL),
	)
	rootCmd.PersistentFlags().StringVarP(
		&opts.output,
		"output",
		"o",
		opts.output,
		fmt.Sprintf("Output format: %s", outputFormats(mode)),
	)

	rootCmd.AddCommand(newWhoamiCommand(opts))
	rootCmd.AddCommand(newProjectCommand(opts))

	return rootCmd
}

func (o *rootOptions) newGitLabClient() (*gitlab.Client, error) {
	return gitlabclient.NewConfig(o.gitlabToken, o.gitlabBaseURL).NewClient()
}

func (o *rootOptions) newGitLabClientWithBaseURLFallback(baseURL string) (*gitlab.Client, error) {
	return gitlabclient.NewConfigWithBaseURLFallback(o.gitlabToken, o.gitlabBaseURL, baseURL).NewClient()
}

func execute(cmd *cobra.Command) {
	if err := cmd.Execute(); err != nil {
		writeCommandError(cmd.ErrOrStderr(), commandModeStandard, err)
		os.Exit(1)
	}
}

func executeAxi(cmd *cobra.Command) {
	if err := cmd.Execute(); err != nil {
		writeCommandError(cmd.ErrOrStderr(), commandModeAxi, err)
		os.Exit(1)
	}
}

// Execute runs the primary gl binary entry point.
func Execute() {
	execute(newRootCommand(
		"gl",
		"Community GitLab CLI for agentic workflows",
		glLongDescription,
		commandModeStandard,
	))
}

// ExecuteAxi runs the axi-oriented gl-axi binary entry point.
func ExecuteAxi() {
	executeAxi(newRootCommand(
		"gl-axi",
		"Axi-oriented Community GitLab CLI for agentic workflows",
		glAxiLongDescription,
		commandModeAxi,
	))
}
