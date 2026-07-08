package cli

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/spf13/cobra"
	gitlab "gitlab.com/gitlab-org/api/client-go/v2"
)

var errNoUpdateFlags = errors.New("no update flags provided")

type mrUpdateOptions struct {
	title              string
	description        string
	descriptionFile    string
	targetBranch       string
	labels             []string
	addLabels          []string
	removeLabels       []string
	assignees          []string
	reviewers          []string
	milestoneID        int64
	removeSourceBranch bool
	squash             bool
	allowCollaboration bool
	discussionLocked   bool
	draft              bool
	ready              bool

	titleSet              bool
	descriptionSet        bool
	targetBranchSet       bool
	labelsSet             bool
	addLabelsSet          bool
	removeLabelsSet       bool
	assigneesSet          bool
	reviewersSet          bool
	milestoneIDSet        bool
	removeSourceBranchSet bool
	squashSet             bool
	allowCollaborationSet bool
	discussionLockedSet   bool
}

// hasUpdateFlags reports whether the invocation requested any change. Draft
// and ready count only when true: --draft=false asks for nothing.
func (opts *mrUpdateOptions) hasUpdateFlags() bool {
	return opts.titleSet || opts.descriptionSet || opts.targetBranchSet ||
		opts.labelsSet || opts.addLabelsSet || opts.removeLabelsSet ||
		opts.assigneesSet || opts.reviewersSet || opts.milestoneIDSet ||
		opts.removeSourceBranchSet || opts.squashSet ||
		opts.allowCollaborationSet || opts.discussionLockedSet ||
		opts.draft || opts.ready
}

func newMRUpdateCommand(rootOpts *rootOptions, projOpts *projectOptions) *cobra.Command {
	opts := &mrUpdateOptions{}

	cmd := &cobra.Command{
		Use:   "update <!iid|iid>",
		Short: "Update a merge request",
		Long: `Update an existing merge request in the current project.

A field is sent to GitLab only when its flag is passed, so unset fields keep
their current values. Explicitly empty values clear: --description "" clears
the description, --assignee "" unassigns everyone, --label "" removes all
labels, and --milestone-id 0 unassigns the milestone.`,
		Args: wrapArgsValidator(cobra.ExactArgs(1)),
		RunE: func(cmd *cobra.Command, args []string) error {
			iid, err := parseMergeRequestRef(args[0])
			if err != nil {
				return err
			}

			flags := cmd.Flags()
			opts.titleSet = flags.Changed("title")
			opts.descriptionSet = flags.Changed("description") || flags.Changed("description-file")
			opts.targetBranchSet = flags.Changed("target-branch")
			opts.labelsSet = flags.Changed("label")
			opts.addLabelsSet = flags.Changed("add-label")
			opts.removeLabelsSet = flags.Changed("remove-label")
			opts.assigneesSet = flags.Changed("assignee")
			opts.reviewersSet = flags.Changed("reviewer")
			opts.milestoneIDSet = flags.Changed("milestone-id")
			opts.removeSourceBranchSet = flags.Changed("remove-source-branch")
			opts.squashSet = flags.Changed("squash")
			opts.allowCollaborationSet = flags.Changed("allow-collaboration")
			opts.discussionLockedSet = flags.Changed("discussion-locked")

			return runMRUpdate(cmd, rootOpts, projOpts, opts, iid)
		},
	}

	flags := cmd.Flags()
	flags.StringVar(&opts.title, "title", "", "New merge request title")
	flags.StringVar(&opts.description, "description", "", "New description as inline text (empty value clears it)")
	flags.StringVar(&opts.descriptionFile, "description-file", "", "Read the description from a file, or from stdin with -")
	flags.StringVar(&opts.targetBranch, "target-branch", "", "New target branch")
	flags.StringSliceVar(&opts.labels, "label", nil, "Replace all labels (repeatable or comma-separated; empty value removes all)")
	flags.StringSliceVar(&opts.addLabels, "add-label", nil, "Label to add, keeping existing ones (repeatable or comma-separated)")
	flags.StringSliceVar(&opts.removeLabels, "remove-label", nil, "Label to remove (repeatable or comma-separated)")
	flags.StringSliceVar(&opts.assignees, "assignee", nil, "Assignee username or numeric user ID (repeatable; empty value unassigns all)")
	flags.StringSliceVar(&opts.reviewers, "reviewer", nil, "Reviewer username or numeric user ID (repeatable; empty value removes all)")
	flags.Int64Var(&opts.milestoneID, "milestone-id", 0, "Milestone ID to assign (0 unassigns)")
	flags.BoolVar(&opts.removeSourceBranch, "remove-source-branch", false, "Delete the source branch when the merge request is merged")
	flags.BoolVar(&opts.squash, "squash", false, "Squash commits when merging")
	flags.BoolVar(&opts.allowCollaboration, "allow-collaboration", false, "Allow commits from members who can merge to the target branch")
	flags.BoolVar(&opts.discussionLocked, "discussion-locked", false, "Lock the merge request discussion")
	flags.BoolVar(&opts.draft, "draft", false, "Mark the merge request as a draft (prefixes the title with Draft:)")
	flags.BoolVar(&opts.ready, "ready", false, "Mark the merge request as ready (removes the Draft: title prefix)")

	return cmd
}

func runMRUpdate(cmd *cobra.Command, rootOpts *rootOptions, projOpts *projectOptions, opts *mrUpdateOptions, iid int64) error {
	if !opts.hasUpdateFlags() {
		return newUsageError(
			fmt.Errorf("%w for merge request !%d", errNoUpdateFlags, iid),
			"Pass at least one field flag, e.g. --title, --description/--description-file, --target-branch, --assignee, --reviewer, --label/--add-label/--remove-label, --milestone-id, --draft/--ready, --squash, --remove-source-branch, --discussion-locked, --allow-collaboration",
		)
	}
	if opts.labelsSet && (opts.addLabelsSet || opts.removeLabelsSet) {
		return newUsageError(
			errors.New("--label replaces all labels and is mutually exclusive with --add-label/--remove-label"),
			"Use --label to replace the full label set, or --add-label/--remove-label (combinable) for incremental changes",
		)
	}
	if opts.draft && opts.ready {
		return newUsageError(
			errors.New("--draft and --ready are mutually exclusive"),
			"Pass --draft to mark the merge request as a draft, or --ready to mark it ready",
		)
	}
	if opts.titleSet && strings.TrimSpace(opts.title) == "" {
		return newUsageError(
			errors.New("merge request title cannot be empty"),
			"Pass a non-empty value to --title, or omit the flag to keep the current title",
		)
	}
	if opts.targetBranchSet && strings.TrimSpace(opts.targetBranch) == "" {
		return newUsageError(
			errors.New("target branch cannot be empty"),
			"Pass a non-empty value to --target-branch, or omit the flag to keep the current target branch",
		)
	}

	description, err := resolveContentFlag(cmd, opts.description, opts.descriptionFile, "description", "description-file")
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

	assigneeIDs, err := resolveUserIDs(ctx, client, "assignee", opts.assignees)
	if err != nil {
		return err
	}
	reviewerIDs, err := resolveUserIDs(ctx, client, "reviewer", opts.reviewers)
	if err != nil {
		return err
	}

	title, sendTitle, err := resolveUpdateTitle(ctx, client, resolved.ref, iid, opts)
	if err != nil {
		return err
	}

	updateOpts := buildUpdateMergeRequestOptions(opts, title, sendTitle, description, assigneeIDs, reviewerIDs)

	updated, _, err := client.MergeRequests.UpdateMergeRequest(resolved.ref, iid, updateOpts, gitlab.WithContext(ctx))
	if err != nil {
		return fmt.Errorf("update merge request !%d in project %q: %w", iid, resolved.ref, err)
	}

	hints := &mrHintContext{project: explicitProjectRef(projOpts)}

	return writeMergeRequestCreated(cmd.OutOrStdout(), rootOpts.output, rootOpts.mode, updated, hints)
}

// resolveUpdateTitle decides whether a title is sent and what it should be.
// The title is sent when --title was passed or when --draft/--ready need to
// rewrite it; in the latter case without --title, the current title is
// fetched so the prefix change applies to what is actually on the merge
// request.
func resolveUpdateTitle(ctx context.Context, client *gitlab.Client, projectRef any, iid int64, opts *mrUpdateOptions) (string, bool, error) {
	if !opts.titleSet && !opts.draft && !opts.ready {
		return "", false, nil
	}

	title := strings.TrimSpace(opts.title)
	if !opts.titleSet {
		mergeRequest, _, err := client.MergeRequests.GetMergeRequest(projectRef, iid, nil, gitlab.WithContext(ctx))
		if err != nil {
			return "", false, fmt.Errorf("get merge request !%d in project %q to update its title: %w", iid, projectRef, err)
		}
		title = mergeRequest.Title
	}

	if opts.draft {
		title = applyDraftTitle(title)
	}
	if opts.ready {
		title = stripDraftTitle(title)
	}

	return title, true, nil
}

// stripDraftTitle removes the leading Draft: marker (case-insensitive), the
// inverse of applyDraftTitle. Legacy WIP:/[Draft] markers are out of scope —
// Draft: is GitLab's canonical form and the only one applyDraftTitle emits.
func stripDraftTitle(title string) string {
	trimmed := strings.TrimSpace(title)
	if !strings.HasPrefix(strings.ToLower(trimmed), "draft:") {
		return title
	}

	return strings.TrimSpace(trimmed[len("draft:"):])
}

func buildUpdateMergeRequestOptions(
	opts *mrUpdateOptions,
	title string,
	sendTitle bool,
	description string,
	assigneeIDs, reviewerIDs []int64,
) *gitlab.UpdateMergeRequestOptions {
	updateOpts := &gitlab.UpdateMergeRequestOptions{}

	if sendTitle {
		updateOpts.Title = gitlab.Ptr(title)
	}
	if opts.descriptionSet {
		updateOpts.Description = gitlab.Ptr(description)
	}
	if opts.targetBranchSet {
		updateOpts.TargetBranch = gitlab.Ptr(strings.TrimSpace(opts.targetBranch))
	}
	if opts.labelsSet {
		labels := gitlab.LabelOptions(opts.labels)
		updateOpts.Labels = &labels
	}
	if opts.addLabelsSet {
		labels := gitlab.LabelOptions(opts.addLabels)
		updateOpts.AddLabels = &labels
	}
	if opts.removeLabelsSet {
		labels := gitlab.LabelOptions(opts.removeLabels)
		updateOpts.RemoveLabels = &labels
	}
	if opts.assigneesSet {
		if assigneeIDs == nil {
			assigneeIDs = []int64{}
		}
		updateOpts.AssigneeIDs = &assigneeIDs
	}
	if opts.reviewersSet {
		if reviewerIDs == nil {
			reviewerIDs = []int64{}
		}
		updateOpts.ReviewerIDs = &reviewerIDs
	}
	if opts.milestoneIDSet {
		updateOpts.MilestoneID = gitlab.Ptr(opts.milestoneID)
	}
	if opts.removeSourceBranchSet {
		updateOpts.RemoveSourceBranch = gitlab.Ptr(opts.removeSourceBranch)
	}
	if opts.squashSet {
		updateOpts.Squash = gitlab.Ptr(opts.squash)
	}
	if opts.allowCollaborationSet {
		updateOpts.AllowCollaboration = gitlab.Ptr(opts.allowCollaboration)
	}
	if opts.discussionLockedSet {
		updateOpts.DiscussionLocked = gitlab.Ptr(opts.discussionLocked)
	}

	return updateOpts
}
