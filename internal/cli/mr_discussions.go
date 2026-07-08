package cli

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/spf13/cobra"
	gitlab "gitlab.com/gitlab-org/api/client-go/v2"
)

var (
	errInvalidDiscussionRef   = errors.New("invalid discussion reference")
	errAmbiguousDiscussionRef = errors.New("ambiguous discussion reference")
	errDiscussionNotFound     = errors.New("discussion not found")
)

const (
	defaultDiscussionStateFilter       = "unresolved"
	defaultDiscussionOrderBy           = "created_at"
	defaultDiscussionSortDirection     = "asc"
	discussionPreviewLimit             = 80
	discussionShortIDLength            = 8
	discussionFetchPageSize      int64 = 100
	// mrDiscussionViewCommandName is how help hints reference the
	// single-thread view command; change here if it is ever renamed.
	mrDiscussionViewCommandName = "mr discussion"
)

// mrDiscussionListExtraFields are the optional axi list columns that --fields
// can add to the compact default schema.
var mrDiscussionListExtraFields = []string{"type", "file", "line", "created_at", "id_full"}

var mrDiscussionListDefaultFields = []string{"id", "author", "state", "notes", "updated_at", "preview"}

// discussionRefPattern matches a full 40-character hex discussion ID or any
// prefix of one.
var discussionRefPattern = regexp.MustCompile(`^[0-9a-f]{1,40}$`)

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
		state:   defaultDiscussionStateFilter,
		orderBy: defaultDiscussionOrderBy,
		sortDir: defaultDiscussionSortDirection,
		limit:   defaultMergeRequestListLimit,
		page:    1,
	}
}

func newMRDiscussionsCommand(rootOpts *rootOptions, projOpts *projectOptions) *cobra.Command {
	opts := newMRDiscussionListOptions()
	var fieldsFlag string

	cmd := &cobra.Command{
		Use:   "discussions <!iid|iid>",
		Short: "List discussion threads on a merge request",
		Long: `List discussion threads on a merge request in the current project.

The GitLab discussions API has no server-side filters, so filtering, sorting,
and paging happen client-side over the complete thread list; totals are always
exact. By default only unresolved threads are shown and system-generated
activity is hidden. In bash and zsh, quote the bang form ('!123') to avoid
shell history expansion.`,
		Args: wrapArgsValidator(cobra.ExactArgs(1)),
		RunE: func(cmd *cobra.Command, args []string) error {
			iid, err := parseMergeRequestRef(args[0])
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
		Use:   "discussion <!iid|iid> <discussion-id>",
		Short: "Show the full conversation of one discussion thread",
		Long: `Show every note of one discussion thread on a merge request, with
complete bodies.

<discussion-id> is the full 40-character hex ID or any unique prefix of one,
as shown by "mr discussions".`,
		Args: wrapArgsValidator(cobra.ExactArgs(2)),
		RunE: func(cmd *cobra.Command, args []string) error {
			iid, err := parseMergeRequestRef(args[0])
			if err != nil {
				return err
			}

			return runMRDiscussionView(cmd, rootOpts, projOpts, iid, args[1])
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

	summaries := make([]discussionSummary, 0, len(discussions))
	for _, discussion := range discussions {
		summary, ok := summarizeDiscussion(discussion)
		if !ok {
			continue
		}
		summaries = append(summaries, summary)
	}

	excludedSystem := 0
	if !opts.system {
		for _, summary := range summaries {
			if summary.system {
				excludedSystem++
			}
		}
	}

	filtered := filterDiscussionSummaries(summaries, opts.state, opts.author, opts.system)
	sortDiscussionSummaries(filtered, opts.orderBy, opts.sortDir)
	rows, paging := pageDiscussionSummaries(filtered, opts.page, opts.limit)

	hints := &discussionHintContext{
		mrHintContext:  mrHintContext{project: explicitProjectRef(projOpts), limit: opts.limit},
		iid:            iid,
		state:          opts.state,
		author:         opts.author,
		system:         opts.system,
		orderBy:        opts.orderBy,
		sortDir:        opts.sortDir,
		excludedSystem: excludedSystem,
	}

	return writeDiscussionList(cmd.OutOrStdout(), rootOpts.output, rootOpts.mode, rows, paging, opts.fields, hints)
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

	return writeDiscussion(cmd.OutOrStdout(), rootOpts.output, rootOpts.mode, discussion)
}

// fetchAllMergeRequestDiscussions pages through the full discussion list. The
// discussions API has no filters or sorting, so every command works from the
// complete set and totals stay exact.
func fetchAllMergeRequestDiscussions(ctx context.Context, client *gitlab.Client, projectRef any, iid int64) ([]*gitlab.Discussion, error) {
	opt := &gitlab.ListMergeRequestDiscussionsOptions{
		ListOptions: gitlab.ListOptions{PerPage: discussionFetchPageSize, Page: 1},
	}

	var all []*gitlab.Discussion
	for {
		discussions, resp, err := client.Discussions.ListMergeRequestDiscussions(projectRef, iid, opt, gitlab.WithContext(ctx))
		if err != nil {
			return nil, fmt.Errorf("list discussions on merge request !%d in project %q: %w", iid, projectRef, err)
		}
		all = append(all, discussions...)

		if resp == nil || resp.NextPage == 0 {
			break
		}
		opt.Page = resp.NextPage
	}

	return all, nil
}

// resolveDiscussionRef turns a user-supplied discussion reference — a full
// 40-character hex ID or a unique prefix — into the discussion it names.
func resolveDiscussionRef(ctx context.Context, client *gitlab.Client, projectRef any, iid int64, ref string) (*gitlab.Discussion, error) {
	normalized := strings.ToLower(strings.TrimSpace(ref))
	if !discussionRefPattern.MatchString(normalized) {
		return nil, newUsageError(
			fmt.Errorf("%w %q: expected a 40-character hex ID or a unique prefix of one", errInvalidDiscussionRef, ref),
		)
	}

	if len(normalized) == 40 {
		discussion, _, err := client.Discussions.GetMergeRequestDiscussion(projectRef, iid, normalized, gitlab.WithContext(ctx))
		if err != nil {
			return nil, fmt.Errorf("get discussion %s on merge request !%d in project %q: %w", normalized, iid, projectRef, err)
		}

		return discussion, nil
	}

	discussions, err := fetchAllMergeRequestDiscussions(ctx, client, projectRef, iid)
	if err != nil {
		return nil, err
	}

	var matches []*gitlab.Discussion
	for _, discussion := range discussions {
		if discussion != nil && strings.HasPrefix(strings.ToLower(discussion.ID), normalized) {
			matches = append(matches, discussion)
		}
	}

	switch len(matches) {
	case 1:
		return matches[0], nil
	case 0:
		return nil, fmt.Errorf("%w: no discussion on merge request !%d matches prefix %q", errDiscussionNotFound, iid, normalized)
	default:
		return nil, newUsageError(
			fmt.Errorf("%w %q: %d discussions match", errAmbiguousDiscussionRef, normalized, len(matches)),
		)
	}
}

// discussionSummary is the computed per-thread view the list pipeline works
// on: resolution state, last activity, and the compact row fields.
type discussionSummary struct {
	id         string
	author     string
	state      string
	preview    string
	noteType   string
	file       string
	line       int64
	resolvable bool
	resolved   bool
	system     bool
	notesCount int
	createdAt  time.Time
	updatedAt  time.Time
	resolvedBy string
	resolvedAt *time.Time
}

// summarizeDiscussion computes the thread-level summary. ok is false for nil
// discussions and discussions without notes, which carry nothing to show.
func summarizeDiscussion(discussion *gitlab.Discussion) (discussionSummary, bool) {
	if discussion == nil {
		return discussionSummary{}, false
	}

	notes := make([]*gitlab.Note, 0, len(discussion.Notes))
	for _, note := range discussion.Notes {
		if note != nil {
			notes = append(notes, note)
		}
	}
	if len(notes) == 0 {
		return discussionSummary{}, false
	}

	summary := discussionSummary{
		id:         strings.ToLower(discussion.ID),
		notesCount: len(notes),
		system:     true,
	}

	first := notes[0]
	summary.author = first.Author.Username
	summary.preview = discussionPreview(first.Body)
	summary.noteType = string(first.Type)
	if summary.noteType == "" {
		summary.noteType = string(gitlab.GenericNote)
	}
	if first.CreatedAt != nil {
		summary.createdAt = *first.CreatedAt
	}
	if position := first.Position; position != nil {
		summary.file = position.NewPath
		summary.line = position.NewLine
		if summary.file == "" {
			summary.file = position.OldPath
		}
		if summary.line == 0 {
			summary.line = position.OldLine
		}
	}

	resolvedAll := true
	for _, note := range notes {
		if !note.System {
			summary.system = false
		}

		updated := note.UpdatedAt
		if updated == nil {
			updated = note.CreatedAt
		}
		if updated != nil && updated.After(summary.updatedAt) {
			summary.updatedAt = *updated
		}

		if note.Resolvable {
			summary.resolvable = true
			if note.Resolved {
				if note.ResolvedBy.Username != "" {
					summary.resolvedBy = note.ResolvedBy.Username
				}
				if note.ResolvedAt != nil {
					summary.resolvedAt = note.ResolvedAt
				}
			} else {
				resolvedAll = false
			}
		}
	}
	summary.resolved = summary.resolvable && resolvedAll

	switch {
	case !summary.resolvable:
		summary.state = "none"
	case summary.resolved:
		summary.state = "resolved"
	default:
		summary.state = "unresolved"
	}

	return summary, true
}

// discussionPreview flattens a note body to one line and truncates it at
// discussionPreviewLimit runes with an explicit ellipsis.
func discussionPreview(body string) string {
	flattened := strings.Join(strings.Fields(body), " ")

	runes := []rune(flattened)
	if len(runes) <= discussionPreviewLimit {
		return flattened
	}

	return string(runes[:discussionPreviewLimit]) + "…"
}

func shortDiscussionID(id string) string {
	id = strings.ToLower(id)
	if len(id) <= discussionShortIDLength {
		return id
	}

	return id[:discussionShortIDLength]
}

func filterDiscussionSummaries(summaries []discussionSummary, state, author string, includeSystem bool) []discussionSummary {
	author = strings.TrimPrefix(strings.TrimSpace(author), "@")

	filtered := make([]discussionSummary, 0, len(summaries))
	for _, summary := range summaries {
		if summary.system && !includeSystem {
			continue
		}
		if author != "" && !strings.EqualFold(summary.author, author) {
			continue
		}

		switch state {
		case "resolved":
			if !summary.resolved {
				continue
			}
		case "unresolved":
			// glab semantics: unresolved means resolvable and not resolved;
			// non-resolvable threads match only --state all.
			if !summary.resolvable || summary.resolved {
				continue
			}
		}

		filtered = append(filtered, summary)
	}

	return filtered
}

func sortDiscussionSummaries(summaries []discussionSummary, orderBy, sortDir string) {
	key := func(summary discussionSummary) time.Time {
		if orderBy == "updated_at" {
			return summary.updatedAt
		}

		return summary.createdAt
	}

	sort.SliceStable(summaries, func(i, j int) bool {
		if sortDir == "desc" {
			return key(summaries[j]).Before(key(summaries[i]))
		}

		return key(summaries[i]).Before(key(summaries[j]))
	})
}

// pageDiscussionSummaries slices the filtered result into the requested page.
// Totals are exact because the whole set was fetched and filtered locally.
func pageDiscussionSummaries(summaries []discussionSummary, page, limit int64) ([]discussionSummary, mrListPaging) {
	total := int64(len(summaries))
	paging := mrListPaging{
		page:       page,
		totalItems: total,
		totalPages: (total + limit - 1) / limit,
	}

	start := (page - 1) * limit
	if start >= total {
		return nil, paging
	}

	end := start + limit
	if end > total {
		end = total
	}

	return summaries[start:end], paging
}
