package cli

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/shabashab/community-gitlab-cli/internal/cli/output"
	"github.com/shabashab/community-gitlab-cli/internal/gitlabclient"
	"github.com/shabashab/community-gitlab-cli/internal/repo"
	"github.com/spf13/cobra"
	gitlab "gitlab.com/gitlab-org/api/client-go/v2"
)

var errMissingProject = errors.New("missing GitLab project")

type projectOptions struct {
	project string
}

type resolvedProject struct {
	ref     string
	baseURL string
}

func newProjectCommand(opts *rootOptions) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "project",
		Short: "Work with GitLab projects",
	}

	cmd.AddCommand(newProjectInfoCommand(opts))

	return cmd
}

func newProjectInfoCommand(rootOpts *rootOptions) *cobra.Command {
	opts := &projectOptions{}

	cmd := &cobra.Command{
		Use:   "info",
		Short: "Show GitLab project information",
		Args:  wrapArgsValidator(cobra.NoArgs),
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runProjectInfo(cmd, rootOpts, opts)
		},
	}

	addProjectFlag(cmd, opts)

	return cmd
}

func addProjectFlag(cmd *cobra.Command, opts *projectOptions) {
	cmd.Flags().StringVar(
		&opts.project,
		"project",
		"",
		"GitLab project ID or full path (defaults to the current git origin)",
	)
}

func runProjectInfo(cmd *cobra.Command, rootOpts *rootOptions, opts *projectOptions) error {
	resolved, err := resolveProject(cmd, rootOpts, opts)
	if err != nil {
		return err
	}

	client, err := rootOpts.newGitLabClientWithBaseURLFallback(resolved.baseURL)
	if err != nil {
		return err
	}

	ctx := cmd.Context()
	if ctx == nil {
		ctx = context.Background()
	}

	project, _, err := client.Projects.GetProject(resolved.ref, nil, gitlab.WithContext(ctx))
	if err != nil {
		return fmt.Errorf("get GitLab project %q: %w", resolved.ref, err)
	}

	return output.WriteProject(cmd.OutOrStdout(), rootOpts.output, rootOpts.mode, project)
}

func resolveProject(cmd *cobra.Command, rootOpts *rootOptions, opts *projectOptions) (resolvedProject, error) {
	projectRef := ""
	if opts != nil {
		projectRef = strings.TrimSpace(opts.project)
	}

	discover := projectRef == "" || !hasConfiguredBaseURL(rootOpts)
	var discovered repo.Origin
	var discoveryErr error
	if discover {
		ctx := cmd.Context()
		if ctx == nil {
			ctx = context.Background()
		}
		discovered, discoveryErr = repo.DiscoverOrigin(ctx, "")
	}

	if projectRef == "" {
		if discoveryErr != nil {
			return resolvedProject{}, fmt.Errorf(
				"%w: run inside a git repository with remote origin configured or pass --project",
				errMissingProject,
			)
		}
		projectRef = discovered.ProjectPath
	}

	return resolvedProject{
		ref:     projectRef,
		baseURL: discovered.BaseURL,
	}, nil
}

func hasConfiguredBaseURL(opts *rootOptions) bool {
	if opts != nil && strings.TrimSpace(opts.gitlabBaseURL) != "" {
		return true
	}

	return strings.TrimSpace(os.Getenv(gitlabclient.BaseURLEnv)) != ""
}

// explicitProjectRef reports the --project value when one was passed, so help
// hints can carry the flag forward into suggested commands.
func explicitProjectRef(projOpts *projectOptions) string {
	if projOpts == nil {
		return ""
	}

	return strings.TrimSpace(projOpts.project)
}
