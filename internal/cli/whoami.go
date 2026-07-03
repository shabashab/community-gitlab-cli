package cli

import (
	"fmt"

	"github.com/spf13/cobra"
	gitlab "gitlab.com/gitlab-org/api/client-go/v2"
)

func newWhoamiCommand(opts *rootOptions) *cobra.Command {
	return &cobra.Command{
		Use:   "whoami",
		Short: "Show the authenticated GitLab user",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			client, err := opts.newGitLabClient()
			if err != nil {
				return err
			}

			user, _, err := client.Users.CurrentUser(gitlab.WithContext(cmd.Context()))
			if err != nil {
				return fmt.Errorf("get current GitLab user: %w", err)
			}

			return writeUser(cmd.OutOrStdout(), opts.output, user)
		},
	}
}
