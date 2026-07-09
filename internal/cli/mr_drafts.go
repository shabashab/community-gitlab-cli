package cli

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/shabashab/community-gitlab-cli/internal/cli/output"
	"github.com/spf13/cobra"
	gitlab "gitlab.com/gitlab-org/api/client-go/v2"
)

var errInvalidDraftNoteID = errors.New("invalid draft note id")

const draftNoteFetchPageSize int64 = 100

// mrDraftListExtraFields are the optional axi list columns that --fields can
// add to the compact default schema (id, file, line, preview).
var mrDraftListExtraFields = []string{"discussion_id", "resolve_discussion"}

var mrDraftListDefaultFields = []string{"id", "file", "line", "preview"}

type mrDraftListOptions struct {
	limit  int64
	page   int64
	fields []string
}

func newMRDraftsCommand(rootOpts *rootOptions, projOpts *projectOptions) *cobra.Command {
	opts := &mrDraftListOptions{limit: output.DefaultMergeRequestListLimit, page: 1}
	var fieldsFlag string

	cmd := &cobra.Command{
		Use:   "drafts <!iid|iid|current>",
		Short: "List and manage your pending draft notes on a merge request",
		Long: `List your pending draft notes on a merge request.

Draft notes are review comments only you can see until they are published.
Create them with "mr comment <iid> --draft ...", then publish the pending
review with "mr drafts publish <iid> --all" (or a single draft by ID). The
draft-notes API has no filters, so paging happens client-side and totals are
always exact. The literal reference "current" resolves to the open merge
request of the currently checked out git branch.`,
		Args: wrapArgsValidator(cobra.ExactArgs(1)),
		RunE: func(cmd *cobra.Command, args []string) error {
			iid, err := resolveMergeRequestRef(cmd, rootOpts, projOpts, args[0])
			if err != nil {
				return err
			}

			fields, err := parseExtraFields(fieldsFlag, mrDraftListExtraFields, mrDraftListDefaultFields)
			if err != nil {
				return err
			}
			opts.fields = fields

			if opts.limit < 1 {
				return newUsageError(fmt.Errorf("--limit must be at least 1, got %d", opts.limit))
			}
			if opts.page < 1 {
				return newUsageError(fmt.Errorf("--page must be at least 1, got %d", opts.page))
			}

			return runMRDraftList(cmd, rootOpts, projOpts, opts, iid)
		},
	}

	flags := cmd.Flags()
	flags.Int64Var(&opts.limit, "limit", opts.limit, "Draft notes per page")
	flags.Int64Var(&opts.page, "page", opts.page, "Page of the draft list to show")
	flags.StringVar(
		&fieldsFlag,
		"fields",
		"",
		fmt.Sprintf("Comma-separated extra columns to add to the compact schema: %s", strings.Join(mrDraftListExtraFields, ", ")),
	)

	cmd.AddCommand(newMRDraftsPublishCommand(rootOpts, projOpts))
	cmd.AddCommand(newMRDraftsDeleteCommand(rootOpts, projOpts))

	return cmd
}

func newMRDraftsPublishCommand(rootOpts *rootOptions, projOpts *projectOptions) *cobra.Command {
	var all bool

	cmd := &cobra.Command{
		Use:   "publish <!iid|iid|current> [<draft-id>]",
		Short: "Publish pending draft notes",
		Long: `Publish one pending draft note by ID, or every pending draft note with
--all, turning them into comments visible to everyone on the merge request.

Publishing with --all when nothing is pending is a no-op that exits 0.`,
		Args: wrapArgsValidator(cobra.RangeArgs(1, 2)),
		RunE: func(cmd *cobra.Command, args []string) error {
			iid, err := resolveMergeRequestRef(cmd, rootOpts, projOpts, args[0])
			if err != nil {
				return err
			}

			if all && len(args) == 2 {
				return newUsageError(
					errors.New("--all and an explicit draft-id are mutually exclusive"),
					"Pass --all to publish every pending draft, or a single <draft-id> from `mr drafts <iid>`",
				)
			}
			if !all && len(args) < 2 {
				return newUsageError(
					errors.New("missing draft note id"),
					"Pass a <draft-id> from `mr drafts <iid>`, or --all to publish every pending draft",
				)
			}

			if all {
				return runMRDraftsPublishAll(cmd, rootOpts, projOpts, iid)
			}

			id, err := parseDraftNoteID(args[1])
			if err != nil {
				return err
			}

			return runMRDraftsPublishOne(cmd, rootOpts, projOpts, iid, id)
		},
	}

	cmd.Flags().BoolVar(&all, "all", false, "Publish every pending draft note on the merge request")

	return cmd
}

func newMRDraftsDeleteCommand(rootOpts *rootOptions, projOpts *projectOptions) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "delete <!iid|iid|current> <draft-id>",
		Short: "Delete a pending draft note",
		Long: `Delete one of your pending draft notes before it is published.

Deleting a draft that is already gone is a verified no-op: the CLI confirms
the ID is absent from the pending list and exits 0.`,
		Args: wrapArgsValidator(cobra.ExactArgs(2)),
		RunE: func(cmd *cobra.Command, args []string) error {
			iid, err := resolveMergeRequestRef(cmd, rootOpts, projOpts, args[0])
			if err != nil {
				return err
			}

			id, err := parseDraftNoteID(args[1])
			if err != nil {
				return err
			}

			return runMRDraftsDelete(cmd, rootOpts, projOpts, iid, id)
		},
	}

	return cmd
}

func parseDraftNoteID(ref string) (int64, error) {
	id, err := strconv.ParseInt(strings.TrimSpace(ref), 10, 64)
	if err != nil || id <= 0 {
		return 0, newUsageError(
			fmt.Errorf("%w %q: expected a numeric draft note id", errInvalidDraftNoteID, ref),
		)
	}

	return id, nil
}

func runMRDraftList(cmd *cobra.Command, rootOpts *rootOptions, projOpts *projectOptions, opts *mrDraftListOptions, iid int64) error {
	resolved, err := resolveProject(cmd, rootOpts, projOpts)
	if err != nil {
		return err
	}

	client, err := rootOpts.newGitLabClientWithBaseURLFallback(resolved.baseURL)
	if err != nil {
		return err
	}

	drafts, err := fetchAllDraftNotes(commandContext(cmd), client, resolved.ref, iid)
	if err != nil {
		return err
	}

	outputs := make([]output.DraftNoteOutput, 0, len(drafts))
	for _, draft := range drafts {
		if draft == nil {
			continue
		}
		outputs = append(outputs, output.DraftNoteToOutput(draft))
	}

	rows, paging := pageDraftNotes(outputs, opts.page, opts.limit)
	hints := &output.MRHintContext{Project: explicitProjectRef(projOpts), Limit: opts.limit}

	return output.WriteDraftNoteList(cmd.OutOrStdout(), rootOpts.output, rootOpts.mode, rows, paging, opts.fields, iid, hints)
}

func runMRDraftsPublishOne(cmd *cobra.Command, rootOpts *rootOptions, projOpts *projectOptions, iid, id int64) error {
	resolved, err := resolveProject(cmd, rootOpts, projOpts)
	if err != nil {
		return err
	}

	client, err := rootOpts.newGitLabClientWithBaseURLFallback(resolved.baseURL)
	if err != nil {
		return err
	}

	hints := &output.MRHintContext{Project: explicitProjectRef(projOpts)}

	if _, err := client.DraftNotes.PublishDraftNote(resolved.ref, iid, id, gitlab.WithContext(commandContext(cmd))); err != nil {
		return draftNoteAPIError(
			fmt.Errorf("publish draft note %d on merge request !%d in project %q: %w", id, iid, resolved.ref, err),
			err,
			rootOpts.binName,
			iid,
			hints,
		)
	}

	return output.WriteDraftNotesPublished(cmd.OutOrStdout(), rootOpts.output, rootOpts.mode, output.DraftPublishResult{ID: &id, Count: 1}, iid, hints)
}

func runMRDraftsPublishAll(cmd *cobra.Command, rootOpts *rootOptions, projOpts *projectOptions, iid int64) error {
	resolved, err := resolveProject(cmd, rootOpts, projOpts)
	if err != nil {
		return err
	}

	client, err := rootOpts.newGitLabClientWithBaseURLFallback(resolved.baseURL)
	if err != nil {
		return err
	}

	ctx := commandContext(cmd)
	hints := &output.MRHintContext{Project: explicitProjectRef(projOpts)}

	// List first: publishing all of an observed-empty set is a provable
	// no-op (exit 0), and the count reflects the drafts seen at publish time.
	drafts, err := fetchAllDraftNotes(ctx, client, resolved.ref, iid)
	if err != nil {
		return err
	}
	if len(drafts) == 0 {
		return output.WriteDraftNotesPublished(cmd.OutOrStdout(), rootOpts.output, rootOpts.mode, output.DraftPublishResult{All: true, Noop: true}, iid, hints)
	}

	if _, err := client.DraftNotes.PublishAllDraftNotes(resolved.ref, iid, gitlab.WithContext(ctx)); err != nil {
		return fmt.Errorf("publish all draft notes on merge request !%d in project %q: %w", iid, resolved.ref, err)
	}

	return output.WriteDraftNotesPublished(cmd.OutOrStdout(), rootOpts.output, rootOpts.mode, output.DraftPublishResult{All: true, Count: len(drafts)}, iid, hints)
}

func runMRDraftsDelete(cmd *cobra.Command, rootOpts *rootOptions, projOpts *projectOptions, iid, id int64) error {
	resolved, err := resolveProject(cmd, rootOpts, projOpts)
	if err != nil {
		return err
	}

	client, err := rootOpts.newGitLabClientWithBaseURLFallback(resolved.baseURL)
	if err != nil {
		return err
	}

	ctx := commandContext(cmd)
	hints := &output.MRHintContext{Project: explicitProjectRef(projOpts)}

	if _, err := client.DraftNotes.DeleteDraftNote(resolved.ref, iid, id, gitlab.WithContext(ctx)); err != nil {
		wrapped := draftNoteAPIError(
			fmt.Errorf("delete draft note %d on merge request !%d in project %q: %w", id, iid, resolved.ref, err),
			err,
			rootOpts.binName,
			iid,
			hints,
		)
		if !isGitLabNotFound(err) {
			return wrapped
		}

		// The requested state is "this draft is gone". A 404 counts as a
		// no-op only when a successful list proves the ID is really absent;
		// if the list itself fails, surface the original error.
		drafts, listErr := fetchAllDraftNotes(ctx, client, resolved.ref, iid)
		if listErr != nil {
			return wrapped
		}
		for _, draft := range drafts {
			if draft != nil && draft.ID == id {
				return wrapped
			}
		}

		return output.WriteDraftNoteDeleted(cmd.OutOrStdout(), rootOpts.output, rootOpts.mode, output.DraftDeleteResult{ID: id, Noop: true}, iid, hints)
	}

	return output.WriteDraftNoteDeleted(cmd.OutOrStdout(), rootOpts.output, rootOpts.mode, output.DraftDeleteResult{ID: id}, iid, hints)
}

// fetchAllDraftNotes pages through the caller's pending draft notes. The
// draft-notes API has no filters, so commands work from the complete set and
// totals stay exact.
func fetchAllDraftNotes(ctx context.Context, client *gitlab.Client, projectRef any, iid int64) ([]*gitlab.DraftNote, error) {
	opt := &gitlab.ListDraftNotesOptions{
		ListOptions: gitlab.ListOptions{PerPage: draftNoteFetchPageSize, Page: 1},
	}

	var all []*gitlab.DraftNote
	for {
		drafts, resp, err := client.DraftNotes.ListDraftNotes(projectRef, iid, opt, gitlab.WithContext(ctx))
		if err != nil {
			return nil, fmt.Errorf("list draft notes on merge request !%d in project %q: %w", iid, projectRef, err)
		}
		all = append(all, drafts...)

		if resp == nil || resp.NextPage == 0 {
			break
		}
		opt.Page = resp.NextPage
	}

	return all, nil
}

// pageDraftNotes slices the full draft list into the requested page. Totals
// are exact because the whole set was fetched.
func pageDraftNotes(drafts []output.DraftNoteOutput, page, limit int64) ([]output.DraftNoteOutput, output.MRListPaging) {
	total := int64(len(drafts))
	paging := output.MRListPaging{
		Page:       page,
		TotalItems: total,
		TotalPages: (total + limit - 1) / limit,
	}

	start := (page - 1) * limit
	if start >= total {
		return nil, paging
	}

	end := start + limit
	if end > total {
		end = total
	}

	return drafts[start:end], paging
}

// draftNoteAPIError attaches a drafts-list hint to 404s so the fixed
// gitlab_not_found suggestion ("check the project path") does not mislead
// when the missing resource is the draft note itself.
func draftNoteAPIError(wrapped, cause error, bin string, iid int64, hints *output.MRHintContext) error {
	if !isGitLabNotFound(cause) {
		return wrapped
	}

	return newHelpError(wrapped, fmt.Sprintf(
		"Run `%s mr drafts %d%s` to list your pending draft notes and their IDs",
		bin,
		iid,
		hints.ProjectSuffix(),
	))
}
