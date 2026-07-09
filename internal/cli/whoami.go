package cli

import (
	"context"
	"fmt"

	"github.com/shabashab/community-gitlab-cli/internal/cli/output"
	"github.com/spf13/cobra"
	gitlab "gitlab.com/gitlab-org/api/client-go/v2"
)

func newWhoamiCommand(opts *rootOptions) *cobra.Command {
	return &cobra.Command{
		Use:   "whoami",
		Short: "Show the authenticated GitLab user",
		Args:  wrapArgsValidator(cobra.NoArgs),
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runWhoami(cmd, opts)
		},
	}
}

func runWhoami(cmd *cobra.Command, opts *rootOptions) error {
	client, err := opts.newGitLabClient()
	if err != nil {
		return err
	}

	ctx := cmd.Context()
	if ctx == nil {
		ctx = context.Background()
	}

	user, _, err := client.Users.CurrentUser(gitlab.WithContext(ctx))
	if err != nil {
		return fmt.Errorf("get current GitLab user: %w", err)
	}

	return output.WriteUser(cmd.OutOrStdout(), opts.output, opts.mode, user)
}
