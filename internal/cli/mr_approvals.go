package cli

import (
	"errors"
	"fmt"
	"strings"

	"github.com/shabashab/community-gitlab-cli/internal/cli/output"
	"github.com/spf13/cobra"
	gitlab "gitlab.com/gitlab-org/api/client-go/v2"
)

type mrApprovalsOptions struct {
	full bool
}

type mrApproveOptions struct {
	sha string
}

func newMRApprovalsCommand(rootOpts *rootOptions, projOpts *projectOptions) *cobra.Command {
	opts := &mrApprovalsOptions{}

	cmd := &cobra.Command{
		Use:     "approvals <!iid|iid|current>",
		Aliases: []string{"approval"},
		Short:   "Show merge request approval status",
		Long: `Show approval status for a merge request.

The compact view shows whether the merge request is approved, how many
approvals remain, whether the current user has approved, and who has already
approved. Pass --full for approval configuration metadata and rule status.`,
		Args: wrapArgsValidator(cobra.ExactArgs(1)),
		RunE: func(cmd *cobra.Command, args []string) error {
			iid, err := resolveMergeRequestRef(cmd, rootOpts, projOpts, args[0])
			if err != nil {
				return err
			}

			return runMRApprovals(cmd, rootOpts, projOpts, opts, iid)
		},
	}

	cmd.Flags().BoolVar(&opts.full, "full", false, "Show approval rules, approver groups, and configuration metadata")

	return cmd
}

func newMRApproveCommand(rootOpts *rootOptions, projOpts *projectOptions) *cobra.Command {
	opts := &mrApproveOptions{}

	cmd := &cobra.Command{
		Use:   "approve <!iid|iid|current>",
		Short: "Approve a merge request",
		Long: `Approve a merge request as the authenticated user.

Pass --sha to require GitLab to approve only if the merge request head still
matches that commit SHA.`,
		Args: wrapArgsValidator(cobra.ExactArgs(1)),
		RunE: func(cmd *cobra.Command, args []string) error {
			iid, err := resolveMergeRequestRef(cmd, rootOpts, projOpts, args[0])
			if err != nil {
				return err
			}
			if cmd.Flags().Changed("sha") && strings.TrimSpace(opts.sha) == "" {
				return newUsageError(
					errors.New("--sha cannot be empty"),
					"Pass the current merge request head SHA, or omit --sha to approve the current head without an optimistic guard",
				)
			}

			return runMRApprove(cmd, rootOpts, projOpts, opts, iid)
		},
	}

	cmd.Flags().StringVar(&opts.sha, "sha", "", "Require this merge request head SHA when approving")

	return cmd
}

func newMRUnapproveCommand(rootOpts *rootOptions, projOpts *projectOptions) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "unapprove <!iid|iid|current>",
		Short: "Remove your approval from a merge request",
		Args:  wrapArgsValidator(cobra.ExactArgs(1)),
		RunE: func(cmd *cobra.Command, args []string) error {
			iid, err := resolveMergeRequestRef(cmd, rootOpts, projOpts, args[0])
			if err != nil {
				return err
			}

			return runMRUnapprove(cmd, rootOpts, projOpts, iid)
		},
	}

	return cmd
}

func runMRApprovals(cmd *cobra.Command, rootOpts *rootOptions, projOpts *projectOptions, opts *mrApprovalsOptions, iid int64) error {
	resolved, err := resolveProject(cmd, rootOpts, projOpts)
	if err != nil {
		return err
	}

	client, err := rootOpts.newGitLabClientWithBaseURLFallback(resolved.baseURL)
	if err != nil {
		return err
	}

	approvals, err := fetchMergeRequestApprovals(cmd, client, resolved.ref, iid)
	if err != nil {
		return err
	}

	return output.WriteMergeRequestApproval(cmd.OutOrStdout(), rootOpts.output, rootOpts.mode, approvals, opts.full, nil)
}

func runMRApprove(cmd *cobra.Command, rootOpts *rootOptions, projOpts *projectOptions, opts *mrApproveOptions, iid int64) error {
	resolved, err := resolveProject(cmd, rootOpts, projOpts)
	if err != nil {
		return err
	}

	client, err := rootOpts.newGitLabClientWithBaseURLFallback(resolved.baseURL)
	if err != nil {
		return err
	}

	approveOpts := &gitlab.ApproveMergeRequestOptions{}
	if strings.TrimSpace(opts.sha) != "" {
		approveOpts.SHA = gitlab.Ptr(strings.TrimSpace(opts.sha))
	}

	approvals, _, err := client.MergeRequestApprovals.ApproveMergeRequest(
		resolved.ref,
		iid,
		approveOpts,
		gitlab.WithContext(commandContext(cmd)),
	)
	if err != nil {
		return fmt.Errorf("approve merge request !%d in project %q: %w", iid, resolved.ref, err)
	}

	hints := &output.MRHintContext{Project: explicitProjectRef(projOpts)}
	help := []string{fmt.Sprintf("Run `mr view %d%s` to check merge status and pipeline results", iid, hints.ProjectSuffix())}

	return output.WriteMergeRequestApproval(cmd.OutOrStdout(), rootOpts.output, rootOpts.mode, approvals, false, help)
}

func runMRUnapprove(cmd *cobra.Command, rootOpts *rootOptions, projOpts *projectOptions, iid int64) error {
	resolved, err := resolveProject(cmd, rootOpts, projOpts)
	if err != nil {
		return err
	}

	client, err := rootOpts.newGitLabClientWithBaseURLFallback(resolved.baseURL)
	if err != nil {
		return err
	}

	if _, err := client.MergeRequestApprovals.UnapproveMergeRequest(resolved.ref, iid, gitlab.WithContext(commandContext(cmd))); err != nil {
		return fmt.Errorf("unapprove merge request !%d in project %q: %w", iid, resolved.ref, err)
	}

	approvals, err := fetchMergeRequestApprovals(cmd, client, resolved.ref, iid)
	if err != nil {
		return err
	}

	hints := &output.MRHintContext{Project: explicitProjectRef(projOpts)}
	help := []string{fmt.Sprintf("Run `mr view %d%s` to check merge status and pipeline results", iid, hints.ProjectSuffix())}

	return output.WriteMergeRequestApproval(cmd.OutOrStdout(), rootOpts.output, rootOpts.mode, approvals, false, help)
}

func fetchMergeRequestApprovals(cmd *cobra.Command, client *gitlab.Client, projectRef any, iid int64) (*gitlab.MergeRequestApprovals, error) {
	approvals, _, err := client.MergeRequests.GetMergeRequestApprovals(
		projectRef,
		iid,
		gitlab.WithContext(commandContext(cmd)),
	)
	if err != nil {
		return nil, fmt.Errorf("get approval status for merge request !%d in project %q: %w", iid, projectRef, err)
	}

	return approvals, nil
}
