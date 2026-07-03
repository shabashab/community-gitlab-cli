/*
Copyright © 2026 Artem Tarasenko <shabashab.04@gmail.com>
*/
package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/shabashab/community-gitlab-cli/internal/credstore"
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

// axiDescription is the one-sentence tool identification shown in the gl-axi
// home view (axi guide §10).
const axiDescription = "Agent-ergonomic GitLab CLI — merge requests, projects, and auth over the personal access token API."

type rootOptions struct {
	gitlabToken   string
	gitlabBaseURL string
	output        string
	mode          commandMode
	binName       string
}

type commandMode string

const (
	commandModeStandard commandMode = "standard"
	commandModeAxi      commandMode = "axi"
)

func newRootCommand(use, short, long string, mode commandMode) (*cobra.Command, *rootOptions) {
	opts := &rootOptions{
		output:  defaultOutputFormat(mode),
		mode:    mode,
		binName: use,
	}

	rootCmd := &cobra.Command{
		Use:           use,
		Short:         short,
		Long:          long,
		Args:          wrapArgsValidator(cobra.NoArgs),
		SilenceUsage:  true,
		SilenceErrors: true,
		// Validate shared flags before any command work so a bad --output
		// fails fast instead of after an API round trip.
		PersistentPreRunE: func(_ *cobra.Command, _ []string) error {
			normalized, err := normalizeOutputFormat(opts.output, mode)
			if err != nil {
				return err
			}
			opts.output = normalized

			return nil
		},
		RunE: func(cmd *cobra.Command, _ []string) error {
			if opts.mode == commandModeAxi {
				return runAxiHome(cmd, opts)
			}

			return cmd.Help()
		},
	}

	rootCmd.SetFlagErrorFunc(flagErrorFunc)

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
	rootCmd.AddCommand(newAuthCommand(opts))
	rootCmd.AddCommand(newProjectCommand(opts))
	rootCmd.AddCommand(newMRCommand(opts))

	return rootCmd, opts
}

// runAxiHome renders the gl-axi no-args home view: tool identity first, then
// the most relevant live content — open merge requests inside a GitLab repo,
// the authenticated user otherwise.
func runAxiHome(cmd *cobra.Command, opts *rootOptions) error {
	bin := currentBinPath()

	resolved, projectErr := resolveProject(cmd, opts, nil)
	if projectErr == nil {
		listOpts := newMRListOptions()
		listOpts.limit = homeMergeRequestLimit

		mergeRequests, paging, err := fetchMergeRequestList(cmd, opts, resolved, listOpts)
		if err == nil {
			rows := make([]axiMergeRequestRow, 0, len(mergeRequests))
			for _, mergeRequest := range mergeRequests {
				if mergeRequest == nil {
					continue
				}
				rows = append(rows, axiMergeRequestRowFor(mergeRequest, nil))
			}

			help := []string{fmt.Sprintf("Run `%s mr view <iid>` for details", opts.binName)}
			if paging.totalItems > int64(len(rows)) {
				help = append(help, fmt.Sprintf("Run `%s mr` for all %d open merge requests", opts.binName, paging.totalItems))
			}
			if len(rows) == 0 {
				help = []string{fmt.Sprintf("Run `%s mr list --state all` to include merged and closed merge requests", opts.binName)}
			}

			return writeAxi(cmd.OutOrStdout(), opts.output, axiHomeRepoOutput{
				Bin:           bin,
				Description:   axiDescription,
				Project:       resolved.ref,
				MergeRequests: rows,
				Count:         fmt.Sprintf("%s open", mrListCountLine(len(rows), paging)),
				Help:          help,
			})
		}

		return err
	}

	client, err := opts.newGitLabClient()
	if err != nil {
		return err
	}

	user, _, err := client.Users.CurrentUser(gitlab.WithContext(commandContext(cmd)))
	if err != nil {
		return fmt.Errorf("get current GitLab user: %w", err)
	}

	return writeAxi(cmd.OutOrStdout(), opts.output, axiHomeUserOutput{
		Bin:         bin,
		Description: axiDescription,
		User:        axiUserFromAPI(user),
		Help: []string{
			fmt.Sprintf("Run `%s mr --project <id-or-path>` to list merge requests of a project", opts.binName),
			fmt.Sprintf("Run `%s` inside a GitLab repository for a project dashboard", opts.binName),
		},
	})
}

const homeMergeRequestLimit int64 = 5

// currentBinPath returns the executable path with the home directory
// collapsed to ~ for the home view identity line.
func currentBinPath() string {
	exe, err := os.Executable()
	if err != nil {
		return ""
	}
	if resolved, err := filepath.EvalSymlinks(exe); err == nil {
		exe = resolved
	}

	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return exe
	}
	if rel, err := filepath.Rel(home, exe); err == nil && !strings.HasPrefix(rel, "..") {
		return filepath.Join("~", rel)
	}

	return exe
}

func (o *rootOptions) newGitLabClient() (*gitlab.Client, error) {
	return o.newGitLabClientWithBaseURLFallback(gitlabclient.DefaultBaseURL)
}

func (o *rootOptions) newGitLabClientWithBaseURLFallback(baseURL string) (*gitlab.Client, error) {
	cfg := gitlabclient.NewConfigWithBaseURLFallback(o.gitlabToken, o.gitlabBaseURL, baseURL)

	// Stored credentials are the last token source, keyed by the resolved
	// host; lookup failures fall through to the missing-token error.
	if cfg.Token == "" {
		if domain, err := credstore.CanonicalDomain(cfg.BaseURL); err == nil {
			if token, _, err := credstore.New().Get(domain); err == nil {
				cfg.Token = token
			}
		}
	}

	return cfg.NewClient()
}

func execute(cmd *cobra.Command, opts *rootOptions) {
	if err := cmd.Execute(); err != nil {
		if opts.mode == commandModeAxi {
			// Structured errors belong on stdout in the requested format so
			// the agent reads them like any other output (axi guide §6).
			writeCommandError(cmd.OutOrStdout(), opts.mode, opts.output, opts.binName, err)
		} else {
			writeCommandError(cmd.ErrOrStderr(), opts.mode, opts.output, opts.binName, err)
		}
		os.Exit(exitCodeForError(err))
	}
}

// Execute runs the primary gl binary entry point.
func Execute() {
	cmd, opts := newRootCommand(
		"gl",
		"Community GitLab CLI for agentic workflows",
		glLongDescription,
		commandModeStandard,
	)
	execute(cmd, opts)
}

// ExecuteAxi runs the axi-oriented gl-axi binary entry point.
func ExecuteAxi() {
	cmd, opts := newRootCommand(
		"gl-axi",
		"Axi-oriented Community GitLab CLI for agentic workflows",
		glAxiLongDescription,
		commandModeAxi,
	)
	execute(cmd, opts)
}
