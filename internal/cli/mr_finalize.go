package cli

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/spf13/cobra"
	gitlab "gitlab.com/gitlab-org/api/client-go/v2"
)

const (
	mrFinalizeActionMerge  = "merge"
	mrFinalizeActionClose  = "close"
	mrFinalizeActionReopen = "reopen"

	mergeRequestStateOpened = "opened"
	mergeRequestStateClosed = "closed"
	mergeRequestStateMerged = "merged"
)

type mrMergeOptions struct {
	sha                     string
	autoMerge               bool
	squash                  bool
	removeSourceBranch      bool
	mergeCommitMessage      string
	mergeCommitMessageFile  string
	squashCommitMessage     string
	squashCommitMessageFile string

	autoMergeSet          bool
	squashSet             bool
	removeSourceBranchSet bool
	mergeMessageSet       bool
	squashMessageSet      bool
}

func newMRMergeCommand(rootOpts *rootOptions, projOpts *projectOptions) *cobra.Command {
	opts := &mrMergeOptions{}

	cmd := &cobra.Command{
		Use:   "merge <!iid|iid|current>",
		Short: "Merge a merge request",
		Long: `Merge a merge request in the current project.

Pass --sha to require GitLab to merge only if the merge request head still
matches that commit SHA. Use --auto-merge to ask GitLab to merge after the
pipeline succeeds when the merge request is not mergeable immediately.`,
		Args: wrapArgsValidator(cobra.ExactArgs(1)),
		RunE: func(cmd *cobra.Command, args []string) error {
			iid, err := resolveMergeRequestRef(cmd, rootOpts, projOpts, args[0])
			if err != nil {
				return err
			}

			flags := cmd.Flags()
			if flags.Changed("sha") && strings.TrimSpace(opts.sha) == "" {
				return newUsageError(
					errors.New("--sha cannot be empty"),
					"Pass the current merge request head SHA, or omit --sha to merge the current head without an optimistic guard",
				)
			}

			opts.autoMergeSet = flags.Changed("auto-merge")
			opts.squashSet = flags.Changed("squash")
			opts.removeSourceBranchSet = flags.Changed("remove-source-branch")
			opts.mergeMessageSet = flags.Changed("merge-commit-message") || flags.Changed("merge-commit-message-file")
			opts.squashMessageSet = flags.Changed("squash-commit-message") || flags.Changed("squash-commit-message-file")

			return runMRMerge(cmd, rootOpts, projOpts, opts, iid)
		},
	}

	flags := cmd.Flags()
	flags.StringVar(&opts.sha, "sha", "", "Require this merge request head SHA when merging")
	flags.BoolVar(&opts.autoMerge, "auto-merge", false, "Merge when the pipeline succeeds if the merge request is not mergeable immediately")
	flags.BoolVar(&opts.squash, "squash", false, "Squash commits when merging")
	flags.BoolVar(&opts.removeSourceBranch, "remove-source-branch", false, "Delete the source branch when the merge request is merged")
	flags.StringVar(&opts.mergeCommitMessage, "merge-commit-message", "", "Merge commit message as inline text")
	flags.StringVar(&opts.mergeCommitMessageFile, "merge-commit-message-file", "", "Read the merge commit message from a file, or from stdin with -")
	flags.StringVar(&opts.squashCommitMessage, "squash-commit-message", "", "Squash commit message as inline text")
	flags.StringVar(&opts.squashCommitMessageFile, "squash-commit-message-file", "", "Read the squash commit message from a file, or from stdin with -")

	return cmd
}

func newMRCloseCommand(rootOpts *rootOptions, projOpts *projectOptions) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "close <!iid|iid|current>",
		Short: "Close a merge request",
		Long: `Close a merge request in the current project.

Closing an already closed merge request is a verified no-op and exits 0.`,
		Args: wrapArgsValidator(cobra.ExactArgs(1)),
		RunE: func(cmd *cobra.Command, args []string) error {
			iid, err := resolveMergeRequestRef(cmd, rootOpts, projOpts, args[0])
			if err != nil {
				return err
			}

			return runMRStateEvent(cmd, rootOpts, projOpts, iid, mrFinalizeActionClose, mergeRequestStateClosed)
		},
	}

	return cmd
}

func newMRReopenCommand(rootOpts *rootOptions, projOpts *projectOptions) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "reopen <!iid|iid|current>",
		Short: "Reopen a merge request",
		Long: `Reopen a merge request in the current project.

Reopening an already open merge request is a verified no-op and exits 0.`,
		Args: wrapArgsValidator(cobra.ExactArgs(1)),
		RunE: func(cmd *cobra.Command, args []string) error {
			iid, err := resolveMergeRequestRef(cmd, rootOpts, projOpts, args[0])
			if err != nil {
				return err
			}

			return runMRStateEvent(cmd, rootOpts, projOpts, iid, mrFinalizeActionReopen, mergeRequestStateOpened)
		},
	}

	return cmd
}

func runMRMerge(cmd *cobra.Command, rootOpts *rootOptions, projOpts *projectOptions, opts *mrMergeOptions, iid int64) error {
	mergeMessage, err := resolveContentFlag(cmd, opts.mergeCommitMessage, opts.mergeCommitMessageFile, "merge-commit-message", "merge-commit-message-file")
	if err != nil {
		return err
	}
	squashMessage, err := resolveContentFlag(cmd, opts.squashCommitMessage, opts.squashCommitMessageFile, "squash-commit-message", "squash-commit-message-file")
	if err != nil {
		return err
	}

	resolved, err := resolveProject(cmd, rootOpts, projOpts)
	if err != nil {
		return err
	}

	client, err := rootOpts.newGitLabClientWithBaseURLFallback(resolved.baseURL)
	if err != nil {
		return err
	}

	ctx := commandContext(cmd)
	hints := &mrHintContext{project: explicitProjectRef(projOpts)}

	current, err := getMergeRequestForFinalization(ctx, client, resolved.ref, iid)
	if err != nil {
		return err
	}
	if current.State == mergeRequestStateMerged {
		return writeMergeRequestAction(cmd.OutOrStdout(), rootOpts.output, rootOpts.mode, current, mrFinalizeActionMerge, true, hints)
	}

	acceptOpts := buildAcceptMergeRequestOptions(opts, mergeMessage, squashMessage)
	merged, _, err := client.MergeRequests.AcceptMergeRequest(resolved.ref, iid, acceptOpts, gitlab.WithContext(ctx))
	if err != nil {
		wrapped := fmt.Errorf("merge request !%d in project %q: %w", iid, resolved.ref, err)
		if !isRetryableFinalizationRace(err) {
			return wrapped
		}

		verified, verifyErr := getMergeRequestForFinalization(ctx, client, resolved.ref, iid)
		if verifyErr != nil || verified.State != mergeRequestStateMerged {
			return wrapped
		}

		return writeMergeRequestAction(cmd.OutOrStdout(), rootOpts.output, rootOpts.mode, verified, mrFinalizeActionMerge, true, hints)
	}

	return writeMergeRequestAction(cmd.OutOrStdout(), rootOpts.output, rootOpts.mode, merged, mrFinalizeActionMerge, false, hints)
}

func runMRStateEvent(cmd *cobra.Command, rootOpts *rootOptions, projOpts *projectOptions, iid int64, action, desiredState string) error {
	resolved, err := resolveProject(cmd, rootOpts, projOpts)
	if err != nil {
		return err
	}

	client, err := rootOpts.newGitLabClientWithBaseURLFallback(resolved.baseURL)
	if err != nil {
		return err
	}

	ctx := commandContext(cmd)
	hints := &mrHintContext{project: explicitProjectRef(projOpts)}

	current, err := getMergeRequestForFinalization(ctx, client, resolved.ref, iid)
	if err != nil {
		return err
	}
	if current.State == desiredState {
		return writeMergeRequestAction(cmd.OutOrStdout(), rootOpts.output, rootOpts.mode, current, action, true, hints)
	}

	stateEvent := action
	updated, _, err := client.MergeRequests.UpdateMergeRequest(
		resolved.ref,
		iid,
		&gitlab.UpdateMergeRequestOptions{StateEvent: gitlab.Ptr(stateEvent)},
		gitlab.WithContext(ctx),
	)
	if err != nil {
		wrapped := fmt.Errorf("%s merge request !%d in project %q: %w", action, iid, resolved.ref, err)
		if !isRetryableFinalizationRace(err) {
			return wrapped
		}

		verified, verifyErr := getMergeRequestForFinalization(ctx, client, resolved.ref, iid)
		if verifyErr != nil || verified.State != desiredState {
			return wrapped
		}

		return writeMergeRequestAction(cmd.OutOrStdout(), rootOpts.output, rootOpts.mode, verified, action, true, hints)
	}

	return writeMergeRequestAction(cmd.OutOrStdout(), rootOpts.output, rootOpts.mode, updated, action, false, hints)
}

func buildAcceptMergeRequestOptions(opts *mrMergeOptions, mergeMessage, squashMessage string) *gitlab.AcceptMergeRequestOptions {
	acceptOpts := &gitlab.AcceptMergeRequestOptions{}

	if strings.TrimSpace(opts.sha) != "" {
		acceptOpts.SHA = gitlab.Ptr(strings.TrimSpace(opts.sha))
	}
	if opts.autoMergeSet {
		acceptOpts.AutoMerge = gitlab.Ptr(opts.autoMerge)
	}
	if opts.squashSet {
		acceptOpts.Squash = gitlab.Ptr(opts.squash)
	}
	if opts.removeSourceBranchSet {
		acceptOpts.ShouldRemoveSourceBranch = gitlab.Ptr(opts.removeSourceBranch)
	}
	if opts.mergeMessageSet {
		acceptOpts.MergeCommitMessage = gitlab.Ptr(mergeMessage)
	}
	if opts.squashMessageSet {
		acceptOpts.SquashCommitMessage = gitlab.Ptr(squashMessage)
	}

	return acceptOpts
}

func getMergeRequestForFinalization(ctx context.Context, client *gitlab.Client, projectRef any, iid int64) (*gitlab.MergeRequest, error) {
	mergeRequest, _, err := client.MergeRequests.GetMergeRequest(projectRef, iid, nil, gitlab.WithContext(ctx))
	if err != nil {
		return nil, fmt.Errorf("get merge request !%d in project %q: %w", iid, projectRef, err)
	}
	if mergeRequest == nil {
		return nil, errors.New("gitlab api returned an empty merge request response")
	}

	return mergeRequest, nil
}

func isRetryableFinalizationRace(err error) bool {
	var respErr *gitlab.ErrorResponse
	if !errors.As(err, &respErr) {
		return false
	}

	switch respErr.StatusCode {
	case 405, 409:
		return true
	default:
		return false
	}
}
