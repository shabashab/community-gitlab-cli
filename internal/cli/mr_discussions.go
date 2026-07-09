package cli

import (
	"errors"
	"fmt"
	"strings"

	"github.com/shabashab/community-gitlab-cli/internal/cli/output"
	"github.com/spf13/cobra"
	gitlab "gitlab.com/gitlab-org/api/client-go/v2"
)

var errDiscussionNotResolvable = errors.New("discussion is not resolvable")

// mrDiscussionListExtraFields are the optional axi list columns that --fields
// can add to the compact default schema.
var mrDiscussionListExtraFields = []string{"type", "file", "line", "created_at", "id_full"}

var mrDiscussionListDefaultFields = []string{"id", "author", "state", "notes", "updated_at", "preview"}

type mrDiscussionListOptions struct {
	state   string
	author  string
	system  bool
	orderBy string
	sortDir string
	limit   int64
	page    int64
	fields  []string
}

func newMRDiscussionListOptions() *mrDiscussionListOptions {
	return &mrDiscussionListOptions{
		state:   output.DefaultDiscussionStateFilter,
		orderBy: output.DefaultDiscussionOrderBy,
		sortDir: output.DefaultDiscussionSortDirection,
		limit:   output.DefaultMergeRequestListLimit,
		page:    1,
	}
}

func newMRDiscussionsCommand(rootOpts *rootOptions, projOpts *projectOptions) *cobra.Command {
	opts := newMRDiscussionListOptions()
	var fieldsFlag string

	cmd := &cobra.Command{
		Use:   "discussions <!iid|iid|current>",
		Short: "List discussion threads on a merge request",
		Long: `List discussion threads on a merge request in the current project.

The GitLab discussions API has no server-side filters, so filtering, sorting,
and paging happen client-side over the complete thread list; totals are always
exact. By default only unresolved threads are shown and system-generated
activity is hidden. The literal reference "current" resolves to the open merge
request of the currently checked out git branch. In bash and zsh, quote the
bang form ('!123') to avoid shell history expansion.`,
		Args: wrapArgsValidator(cobra.ExactArgs(1)),
		RunE: func(cmd *cobra.Command, args []string) error {
			iid, err := resolveMergeRequestRef(cmd, rootOpts, projOpts, args[0])
			if err != nil {
				return err
			}

			fields, err := parseMRDiscussionFields(fieldsFlag)
			if err != nil {
				return err
			}
			opts.fields = fields

			if err := normalizeMRDiscussionListOptions(opts); err != nil {
				return err
			}

			return runMRDiscussionList(cmd, rootOpts, projOpts, opts, iid)
		},
	}

	flags := cmd.Flags()
	flags.StringVar(&opts.state, "state", opts.state, "Filter by resolution state: all, resolved, unresolved (non-resolvable threads match only all)")
	flags.StringVar(&opts.author, "author", "", "Filter by thread starter username (optional @ prefix)")
	flags.BoolVar(&opts.system, "system", false, "Include system-generated discussions (excluded by default)")
	flags.StringVar(&opts.orderBy, "order-by", opts.orderBy, "Order threads by: created_at or updated_at (thread updated_at = newest note update)")
	flags.StringVar(&opts.sortDir, "sort", opts.sortDir, "Sort direction: asc or desc")
	flags.Int64Var(&opts.limit, "limit", opts.limit, "Threads per page, applied after filtering")
	flags.Int64Var(&opts.page, "page", opts.page, "Page of the filtered result to show")
	flags.StringVar(
		&fieldsFlag,
		"fields",
		"",
		fmt.Sprintf("Comma-separated extra columns to add to the compact schema: %s", strings.Join(mrDiscussionListExtraFields, ", ")),
	)

	return cmd
}

func newMRDiscussionCommand(rootOpts *rootOptions, projOpts *projectOptions) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "discussion <!iid|iid|current> <discussion-id>",
		Short: "Show the full conversation of one discussion thread",
		Long: `Show every note of one discussion thread on a merge request, with
complete bodies.

<discussion-id> is the full 40-character hex ID or any unique prefix of one,
as shown by "mr discussions". The literal reference "current" resolves to the
open merge request of the currently checked out git branch.

Use "mr discussion resolve" or "mr discussion unresolve" to change the
resolution state of a resolvable thread.`,
		Args: wrapArgsValidator(cobra.ExactArgs(2)),
		RunE: func(cmd *cobra.Command, args []string) error {
			iid, err := resolveMergeRequestRef(cmd, rootOpts, projOpts, args[0])
			if err != nil {
				return err
			}

			return runMRDiscussionView(cmd, rootOpts, projOpts, iid, args[1])
		},
	}

	cmd.AddCommand(newMRDiscussionResolveCommand(rootOpts, projOpts, "resolve", true))
	cmd.AddCommand(newMRDiscussionResolveCommand(rootOpts, projOpts, "unresolve", false))

	return cmd
}

func newMRDiscussionResolveCommand(rootOpts *rootOptions, projOpts *projectOptions, action string, desired bool) *cobra.Command {
	short := "Resolve a merge request discussion thread"
	if !desired {
		short = "Unresolve a merge request discussion thread"
	}

	cmd := &cobra.Command{
		Use:   action + " <!iid|iid|current> <discussion-id>",
		Short: short,
		Long: fmt.Sprintf(`%s a resolvable discussion thread on a merge request in the current project.

<discussion-id> is the full 40-character hex ID or any unique prefix of one,
as shown by "mr discussions". Already-%sd threads are reported as no-ops and
exit 0. The literal reference "current" resolves to the open merge request of
the currently checked out git branch.`, strings.ToUpper(action[:1])+action[1:], action),
		Args: wrapArgsValidator(cobra.ExactArgs(2)),
		RunE: func(cmd *cobra.Command, args []string) error {
			iid, err := resolveMergeRequestRef(cmd, rootOpts, projOpts, args[0])
			if err != nil {
				return err
			}

			return runMRDiscussionResolve(cmd, rootOpts, projOpts, iid, args[1], action, desired)
		},
	}

	return cmd
}

func parseMRDiscussionFields(value string) ([]string, error) {
	return parseExtraFields(value, mrDiscussionListExtraFields, mrDiscussionListDefaultFields)
}

// normalizeMRDiscussionListOptions validates the enum and range flags before
// any API call so bad invocations fail fast with the valid set inline.
func normalizeMRDiscussionListOptions(opts *mrDiscussionListOptions) error {
	opts.state = strings.ToLower(strings.TrimSpace(opts.state))
	switch opts.state {
	case "all", "resolved", "unresolved":
	default:
		return newUsageError(
			fmt.Errorf("unsupported --state %q", opts.state),
			"Valid --state values: all, resolved, unresolved",
		)
	}

	opts.orderBy = strings.ToLower(strings.TrimSpace(opts.orderBy))
	switch opts.orderBy {
	case "created_at", "updated_at":
	default:
		return newUsageError(
			fmt.Errorf("unsupported --order-by %q", opts.orderBy),
			"Valid --order-by values: created_at, updated_at",
		)
	}

	opts.sortDir = strings.ToLower(strings.TrimSpace(opts.sortDir))
	switch opts.sortDir {
	case "asc", "desc":
	default:
		return newUsageError(
			fmt.Errorf("unsupported --sort %q", opts.sortDir),
			"Valid --sort values: asc, desc",
		)
	}

	if opts.limit < 1 {
		return newUsageError(fmt.Errorf("--limit must be at least 1, got %d", opts.limit))
	}
	if opts.page < 1 {
		return newUsageError(fmt.Errorf("--page must be at least 1, got %d", opts.page))
	}

	return nil
}

func runMRDiscussionList(cmd *cobra.Command, rootOpts *rootOptions, projOpts *projectOptions, opts *mrDiscussionListOptions, iid int64) error {
	resolved, err := resolveProject(cmd, rootOpts, projOpts)
	if err != nil {
		return err
	}

	client, err := rootOpts.newGitLabClientWithBaseURLFallback(resolved.baseURL)
	if err != nil {
		return err
	}

	discussions, err := fetchAllMergeRequestDiscussions(commandContext(cmd), client, resolved.ref, iid)
	if err != nil {
		return err
	}

	summaries := make([]output.DiscussionSummary, 0, len(discussions))
	for _, discussion := range discussions {
		summary, ok := output.SummarizeDiscussion(discussion)
		if !ok {
			continue
		}
		summaries = append(summaries, summary)
	}

	excludedSystem := 0
	if !opts.system {
		for _, summary := range summaries {
			if summary.System {
				excludedSystem++
			}
		}
	}

	filtered := filterDiscussionSummaries(summaries, opts.state, opts.author, opts.system)
	sortDiscussionSummaries(filtered, opts.orderBy, opts.sortDir)
	rows, paging := pageDiscussionSummaries(filtered, opts.page, opts.limit)

	hints := &output.DiscussionHintContext{
		MRHintContext:  output.MRHintContext{Project: explicitProjectRef(projOpts), Limit: opts.limit},
		IID:            iid,
		State:          opts.state,
		Author:         opts.author,
		System:         opts.system,
		OrderBy:        opts.orderBy,
		SortDir:        opts.sortDir,
		ExcludedSystem: excludedSystem,
	}

	return output.WriteDiscussionList(cmd.OutOrStdout(), rootOpts.output, rootOpts.mode, rows, paging, opts.fields, hints)
}

func runMRDiscussionView(cmd *cobra.Command, rootOpts *rootOptions, projOpts *projectOptions, iid int64, ref string) error {
	resolved, err := resolveProject(cmd, rootOpts, projOpts)
	if err != nil {
		return err
	}

	client, err := rootOpts.newGitLabClientWithBaseURLFallback(resolved.baseURL)
	if err != nil {
		return err
	}

	discussion, err := resolveDiscussionRef(commandContext(cmd), client, resolved.ref, iid, ref)
	if err != nil {
		return err
	}

	return output.WriteDiscussion(cmd.OutOrStdout(), rootOpts.output, rootOpts.mode, discussion)
}

func runMRDiscussionResolve(cmd *cobra.Command, rootOpts *rootOptions, projOpts *projectOptions, iid int64, ref, action string, desired bool) error {
	resolved, err := resolveProject(cmd, rootOpts, projOpts)
	if err != nil {
		return err
	}

	client, err := rootOpts.newGitLabClientWithBaseURLFallback(resolved.baseURL)
	if err != nil {
		return err
	}

	ctx := commandContext(cmd)
	discussion, err := resolveDiscussionRef(ctx, client, resolved.ref, iid, ref)
	if err != nil {
		return err
	}

	hints := &output.MRHintContext{Project: explicitProjectRef(projOpts)}
	summary, ok := output.SummarizeDiscussion(discussion)
	if !ok || !summary.Resolvable {
		return newHelpError(
			fmt.Errorf("%w: discussion %s on merge request !%d cannot be resolved or unresolved", errDiscussionNotResolvable, output.ShortDiscussionID(discussion.ID), iid),
			fmt.Sprintf("Run `%s %d %s%s` for the full thread", output.MRDiscussionViewCommandName, iid, output.ShortDiscussionID(discussion.ID), hints.ProjectSuffix()),
		)
	}

	if summary.Resolved == desired {
		return output.WriteDiscussionAction(cmd.OutOrStdout(), rootOpts.output, rootOpts.mode, discussion, action, true, iid, hints)
	}

	updated, _, err := client.Discussions.ResolveMergeRequestDiscussion(
		resolved.ref,
		iid,
		discussion.ID,
		&gitlab.ResolveMergeRequestDiscussionOptions{Resolved: gitlab.Ptr(desired)},
		gitlab.WithContext(ctx),
	)
	if err != nil {
		return fmt.Errorf("%s discussion %s on merge request !%d in project %q: %w", action, output.ShortDiscussionID(discussion.ID), iid, resolved.ref, err)
	}

	return output.WriteDiscussionAction(cmd.OutOrStdout(), rootOpts.output, rootOpts.mode, updated, action, false, iid, hints)
}
