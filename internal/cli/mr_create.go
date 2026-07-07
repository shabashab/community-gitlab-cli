package cli

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/shabashab/community-gitlab-cli/internal/repo"
	"github.com/spf13/cobra"
	gitlab "gitlab.com/gitlab-org/api/client-go/v2"
)

var (
	errUserNotFound        = errors.New("gitlab user not found")
	errMissingSourceBranch = errors.New("cannot determine source branch")
	errMissingTargetBranch = errors.New("cannot determine target branch")
)

type mrCreateOptions struct {
	title              string
	description        string
	descriptionFile    string
	sourceBranch       string
	targetBranch       string
	labels             []string
	assignees          []string
	reviewers          []string
	milestoneID        int64
	targetProjectID    int64
	removeSourceBranch bool
	squash             bool
	allowCollaboration bool
	draft              bool

	removeSourceBranchSet bool
	squashSet             bool
	allowCollaborationSet bool
}

func newMRCreateCommand(rootOpts *rootOptions, projOpts *projectOptions) *cobra.Command {
	opts := &mrCreateOptions{}

	cmd := &cobra.Command{
		Use:   "create",
		Short: "Create a merge request",
		Long: `Create a merge request in the current project.

Only --title is required. The source branch defaults to the currently checked
out git branch and the target branch defaults to the project's default branch.`,
		Args: wrapArgsValidator(cobra.NoArgs),
		RunE: func(cmd *cobra.Command, _ []string) error {
			opts.removeSourceBranchSet = cmd.Flags().Changed("remove-source-branch")
			opts.squashSet = cmd.Flags().Changed("squash")
			opts.allowCollaborationSet = cmd.Flags().Changed("allow-collaboration")

			return runMRCreate(cmd, rootOpts, projOpts, opts)
		},
	}

	flags := cmd.Flags()
	flags.StringVar(&opts.title, "title", "", "Merge request title (required)")
	flags.StringVar(&opts.description, "description", "", "Merge request description as inline text")
	flags.StringVar(&opts.descriptionFile, "description-file", "", "Read the description from a file, or from stdin with -")
	flags.StringVar(&opts.sourceBranch, "source-branch", "", "Source branch (defaults to the current git branch)")
	flags.StringVar(&opts.targetBranch, "target-branch", "", "Target branch (defaults to the project's default branch)")
	flags.StringSliceVar(&opts.labels, "label", nil, "Label to apply (repeatable or comma-separated)")
	flags.StringSliceVar(&opts.assignees, "assignee", nil, "Assignee username or numeric user ID (repeatable)")
	flags.StringSliceVar(&opts.reviewers, "reviewer", nil, "Reviewer username or numeric user ID (repeatable)")
	flags.Int64Var(&opts.milestoneID, "milestone-id", 0, "Milestone ID to assign")
	flags.Int64Var(&opts.targetProjectID, "target-project-id", 0, "Numeric target project ID for cross-fork merge requests")
	flags.BoolVar(&opts.removeSourceBranch, "remove-source-branch", false, "Delete the source branch when the merge request is merged")
	flags.BoolVar(&opts.squash, "squash", false, "Squash commits when merging")
	flags.BoolVar(&opts.allowCollaboration, "allow-collaboration", false, "Allow commits from members who can merge to the target branch")
	flags.BoolVar(&opts.draft, "draft", false, "Mark the merge request as a draft (prefixes the title with Draft:)")

	return cmd
}

func runMRCreate(cmd *cobra.Command, rootOpts *rootOptions, projOpts *projectOptions, opts *mrCreateOptions) error {
	title := strings.TrimSpace(opts.title)
	if title == "" {
		return newUsageError(
			errors.New("missing required flag --title"),
			"Pass --title <title> — the only required flag; branches default to the current branch and the project default branch",
		)
	}
	if opts.draft {
		title = applyDraftTitle(title)
	}

	description, err := resolveContentFlag(cmd, opts.description, opts.descriptionFile, "description", "description-file")
	if err != nil {
		return err
	}

	sourceBranch := strings.TrimSpace(opts.sourceBranch)
	if sourceBranch == "" {
		sourceBranch, err = repo.CurrentBranch(commandContext(cmd), "")
		if err != nil {
			return fmt.Errorf("%w (%v): pass --source-branch", errMissingSourceBranch, err)
		}
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

	targetBranch := strings.TrimSpace(opts.targetBranch)
	if targetBranch == "" {
		targetBranch, err = defaultTargetBranch(ctx, client, resolved, opts.targetProjectID)
		if err != nil {
			return err
		}
	}

	assigneeIDs, err := resolveUserIDs(ctx, client, "assignee", opts.assignees)
	if err != nil {
		return err
	}
	reviewerIDs, err := resolveUserIDs(ctx, client, "reviewer", opts.reviewers)
	if err != nil {
		return err
	}

	createOpts := buildCreateMergeRequestOptions(opts, title, description, sourceBranch, targetBranch, assigneeIDs, reviewerIDs)

	created, _, err := client.MergeRequests.CreateMergeRequest(resolved.ref, createOpts, gitlab.WithContext(ctx))
	if err != nil {
		return fmt.Errorf("create merge request in project %q: %w", resolved.ref, err)
	}

	hints := &mrHintContext{project: explicitProjectRef(projOpts)}

	return writeMergeRequestCreated(cmd.OutOrStdout(), rootOpts.output, rootOpts.mode, created, hints)
}

// defaultTargetBranch reads the default branch of the project the merge
// request will land in: the --target-project-id fork target when set,
// otherwise the resolved project itself.
func defaultTargetBranch(ctx context.Context, client *gitlab.Client, resolved resolvedProject, targetProjectID int64) (string, error) {
	var pid any = resolved.ref
	if targetProjectID > 0 {
		pid = targetProjectID
	}

	project, _, err := client.Projects.GetProject(pid, nil, gitlab.WithContext(ctx))
	if err != nil {
		return "", fmt.Errorf("%w (%v): pass --target-branch", errMissingTargetBranch, err)
	}
	if project == nil || strings.TrimSpace(project.DefaultBranch) == "" {
		return "", fmt.Errorf("%w: project has no default branch: pass --target-branch", errMissingTargetBranch)
	}

	return project.DefaultBranch, nil
}

// applyDraftTitle prepends the Draft: marker GitLab derives draft status
// from, unless the title already carries one.
func applyDraftTitle(title string) string {
	if strings.HasPrefix(strings.ToLower(strings.TrimSpace(title)), "draft:") {
		return title
	}

	return "Draft: " + title
}

// resolveUserIDs maps --assignee/--reviewer values to numeric user IDs.
// All-digit values pass through as raw IDs; anything else, optionally
// @-prefixed, is resolved with an exact-username lookup.
func resolveUserIDs(ctx context.Context, client *gitlab.Client, flagName string, values []string) ([]int64, error) {
	var ids []int64
	for _, value := range values {
		name := strings.TrimPrefix(strings.TrimSpace(value), "@")
		if name == "" {
			continue
		}

		if id, err := strconv.ParseInt(name, 10, 64); err == nil {
			ids = append(ids, id)
			continue
		}

		users, _, err := client.Users.ListUsers(
			&gitlab.ListUsersOptions{Username: gitlab.Ptr(name)},
			gitlab.WithContext(ctx),
		)
		if err != nil {
			return nil, fmt.Errorf("look up user %q for --%s: %w", name, flagName, err)
		}
		if len(users) == 0 || users[0] == nil {
			return nil, fmt.Errorf("%w: no GitLab user with username %q for --%s", errUserNotFound, name, flagName)
		}
		ids = append(ids, users[0].ID)
	}

	return ids, nil
}

func buildCreateMergeRequestOptions(
	opts *mrCreateOptions,
	title, description, sourceBranch, targetBranch string,
	assigneeIDs, reviewerIDs []int64,
) *gitlab.CreateMergeRequestOptions {
	createOpts := &gitlab.CreateMergeRequestOptions{
		Title:        gitlab.Ptr(title),
		SourceBranch: gitlab.Ptr(sourceBranch),
		TargetBranch: gitlab.Ptr(targetBranch),
	}

	if description != "" {
		createOpts.Description = gitlab.Ptr(description)
	}
	if len(opts.labels) > 0 {
		labels := gitlab.LabelOptions(opts.labels)
		createOpts.Labels = &labels
	}
	if len(assigneeIDs) > 0 {
		createOpts.AssigneeIDs = &assigneeIDs
	}
	if len(reviewerIDs) > 0 {
		createOpts.ReviewerIDs = &reviewerIDs
	}
	if opts.milestoneID > 0 {
		createOpts.MilestoneID = gitlab.Ptr(opts.milestoneID)
	}
	if opts.targetProjectID > 0 {
		createOpts.TargetProjectID = gitlab.Ptr(opts.targetProjectID)
	}
	if opts.removeSourceBranchSet {
		createOpts.RemoveSourceBranch = gitlab.Ptr(opts.removeSourceBranch)
	}
	if opts.squashSet {
		createOpts.Squash = gitlab.Ptr(opts.squash)
	}
	if opts.allowCollaborationSet {
		createOpts.AllowCollaboration = gitlab.Ptr(opts.allowCollaboration)
	}

	return createOpts
}
