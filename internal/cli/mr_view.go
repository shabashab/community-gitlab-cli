package cli

import (
	"fmt"

	"github.com/shabashab/community-gitlab-cli/internal/cli/output"
	"github.com/spf13/cobra"
	gitlab "gitlab.com/gitlab-org/api/client-go/v2"
)

type mrViewOptions struct {
	full bool
}

func newMRViewCommand(rootOpts *rootOptions, projOpts *projectOptions) *cobra.Command {
	opts := &mrViewOptions{}

	cmd := &cobra.Command{
		Use:     "view <!iid|iid|current>",
		Aliases: []string{"info"},
		Short:   "Show merge request information",
		Args:    wrapArgsValidator(cobra.ExactArgs(1)),
		RunE: func(cmd *cobra.Command, args []string) error {
			iid, err := resolveMergeRequestRef(cmd, rootOpts, projOpts, args[0])
			if err != nil {
				return err
			}

			return runMRView(cmd, rootOpts, projOpts, opts, iid)
		},
	}

	cmd.Flags().BoolVar(
		&opts.full,
		"full",
		false,
		"Show all merge request fields and the complete description",
	)

	return cmd
}

func runMRView(cmd *cobra.Command, rootOpts *rootOptions, projOpts *projectOptions, opts *mrViewOptions, iid int64) error {
	resolved, err := resolveProject(cmd, rootOpts, projOpts)
	if err != nil {
		return err
	}

	client, err := rootOpts.newGitLabClientWithBaseURLFallback(resolved.baseURL)
	if err != nil {
		return err
	}

	mergeRequest, _, err := client.MergeRequests.GetMergeRequest(resolved.ref, iid, nil, gitlab.WithContext(commandContext(cmd)))
	if err != nil {
		return fmt.Errorf("get merge request !%d in project %q: %w", iid, resolved.ref, err)
	}
	approvals, err := fetchMergeRequestApprovals(cmd, client, resolved.ref, iid)
	if err != nil {
		return err
	}

	hints := &output.MRHintContext{Project: explicitProjectRef(projOpts)}

	return output.WriteMergeRequestWithApprovals(cmd.OutOrStdout(), rootOpts.output, rootOpts.mode, mergeRequest, approvals, opts.full, hints)
}
