package cli

import (
	"errors"
	"fmt"
	"strings"

	"github.com/shabashab/community-gitlab-cli/internal/cli/output"
	"github.com/spf13/cobra"
	gitlab "gitlab.com/gitlab-org/api/client-go/v2"
)

type mrCommentOptions struct {
	body     string
	bodyFile string
	file     string
	line     string
	oldLine  string
	draft    bool
	replyTo  string
	note     bool
	resolve  bool

	fileSet    bool
	lineSet    bool
	oldLineSet bool
	replySet   bool
}

func newMRCommentCommand(rootOpts *rootOptions, projOpts *projectOptions) *cobra.Command {
	opts := &mrCommentOptions{}

	cmd := &cobra.Command{
		Use:   "comment <!iid|iid|current>",
		Short: "Add a review comment to a merge request",
		Long: `Add a review comment to a merge request in the current project.

Without position flags the comment starts a new resolvable discussion thread
(--note posts a plain non-resolvable note instead). Pass --file to anchor the
comment to a changed file, and --line (new file version) or --old-line (old
version, for removed lines) to anchor it to a diff line; --line 10:15 covers
a range. The CLI resolves GitLab's diff position (SHAs, old/new line pairing,
renames) from the merge request itself, so plain line numbers are enough.

With --draft the comment becomes a pending draft note that only you can see
until "mr drafts publish" turns the whole set into one published review.
--reply-to <discussion-id> answers an existing thread instead of opening a
new one.

The literal reference "current" resolves to the open merge request of the
currently checked out git branch. In bash and zsh, quote the bang form
('!123') to avoid shell history expansion.`,
		Args: wrapArgsValidator(cobra.ExactArgs(1)),
		RunE: func(cmd *cobra.Command, args []string) error {
			iid, err := resolveMergeRequestRef(cmd, rootOpts, projOpts, args[0])
			if err != nil {
				return err
			}

			opts.fileSet = cmd.Flags().Changed("file")
			opts.lineSet = cmd.Flags().Changed("line")
			opts.oldLineSet = cmd.Flags().Changed("old-line")
			opts.replySet = cmd.Flags().Changed("reply-to")

			return runMRComment(cmd, rootOpts, projOpts, opts, iid)
		},
	}

	flags := cmd.Flags()
	flags.StringVar(&opts.body, "body", "", "Comment body as inline text")
	flags.StringVar(&opts.bodyFile, "body-file", "", "Read the comment body from a file, or from stdin with -")
	flags.StringVar(&opts.file, "file", "", "File path to anchor the comment to (alone: a file-level comment)")
	flags.StringVar(&opts.line, "line", "", "Line in the new file version: <line> or <start>:<end> (requires --file)")
	flags.StringVar(&opts.oldLine, "old-line", "", "Line in the old file version, for removed lines: <line> or <start>:<end> (requires --file)")
	flags.BoolVar(&opts.draft, "draft", false, "Create a pending draft note instead of an immediate comment")
	flags.StringVar(&opts.replyTo, "reply-to", "", "Reply to an existing discussion: full 40-hex ID or unique prefix")
	flags.BoolVar(&opts.note, "note", false, "Post a plain non-resolvable note instead of a discussion thread")
	flags.BoolVar(&opts.resolve, "resolve", false, "Resolve the replied-to thread when the draft publishes (requires --draft --reply-to)")

	return cmd
}

// validateMRCommentOptions enforces the flag-combination matrix before any
// API call and turns the position flags into a comment anchor.
func validateMRCommentOptions(opts *mrCommentOptions) (commentAnchor, error) {
	if opts.lineSet && opts.oldLineSet {
		return commentAnchor{}, newUsageError(
			errors.New("--line and --old-line are mutually exclusive"),
			"--line addresses the new file version, --old-line the old one — for unchanged lines either side works and the CLI pairs them automatically",
		)
	}
	if (opts.lineSet || opts.oldLineSet) && !opts.fileSet {
		return commentAnchor{}, newUsageError(
			errors.New("--line/--old-line require --file"),
			"Pass --file <path> with the file the line belongs to",
		)
	}
	if opts.replySet && (opts.fileSet || opts.lineSet || opts.oldLineSet) {
		return commentAnchor{}, newUsageError(
			errors.New("--reply-to cannot be combined with --file/--line/--old-line"),
			"A reply attaches to the thread's existing position — drop the position flags",
		)
	}
	if opts.replySet && opts.note {
		return commentAnchor{}, newUsageError(
			errors.New("--reply-to cannot be combined with --note"),
			"Replies always join the referenced discussion thread — drop --note",
		)
	}
	if opts.note && opts.draft {
		return commentAnchor{}, newUsageError(
			errors.New("--note cannot be combined with --draft"),
			"Draft notes always publish into discussions — drop --note, or drop --draft for an immediate plain note",
		)
	}
	if opts.note && opts.fileSet {
		return commentAnchor{}, newUsageError(
			errors.New("--note cannot be combined with --file/--line/--old-line"),
			"Plain notes cannot anchor to the diff — drop --note to create a diff thread",
		)
	}
	if opts.resolve && !opts.replySet {
		return commentAnchor{}, newUsageError(
			errors.New("--resolve requires --reply-to"),
			"--resolve marks the replied-to thread resolved when the draft publishes — pass --reply-to <discussion-id>",
		)
	}
	if opts.resolve && !opts.draft {
		return commentAnchor{}, newUsageError(
			errors.New("--resolve requires --draft"),
			"Only draft replies can resolve a thread on publish — add --draft",
		)
	}

	anchor := commentAnchor{
		file: strings.TrimPrefix(strings.TrimSpace(opts.file), "./"),
		side: sideNew,
	}
	if opts.fileSet && anchor.file == "" {
		return commentAnchor{}, newUsageError(
			errors.New("--file requires a non-empty path"),
			"Pass the repository-relative path of a file changed by the merge request",
		)
	}

	var err error
	switch {
	case opts.lineSet:
		anchor.start, anchor.end, err = parseLineSpec("line", opts.line)
	case opts.oldLineSet:
		anchor.side = sideOld
		anchor.start, anchor.end, err = parseLineSpec("old-line", opts.oldLine)
	}
	if err != nil {
		return commentAnchor{}, err
	}

	return anchor, nil
}

func runMRComment(cmd *cobra.Command, rootOpts *rootOptions, projOpts *projectOptions, opts *mrCommentOptions, iid int64) error {
	anchor, err := validateMRCommentOptions(opts)
	if err != nil {
		return err
	}

	body, err := resolveContentFlag(cmd, opts.body, opts.bodyFile, "body", "body-file")
	if err != nil {
		return err
	}
	if strings.TrimSpace(body) == "" {
		return newUsageError(
			errors.New("missing required flag --body"),
			"Pass the comment text inline with --body, or from a file (or stdin via -) with --body-file",
		)
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
	hints := &output.MRHintContext{Project: explicitProjectRef(projOpts)}

	if opts.replySet {
		return runMRCommentReply(cmd, rootOpts, opts, client, resolved, iid, body, hints)
	}

	var position *gitlab.PositionOptions
	if opts.fileSet {
		position, err = resolveCommentPosition(ctx, client, resolved.ref, iid, anchor)
		if err != nil {
			return err
		}
	}

	if opts.draft {
		draft, _, err := client.DraftNotes.CreateDraftNote(resolved.ref, iid, &gitlab.CreateDraftNoteOptions{
			Note:     gitlab.Ptr(body),
			Position: position,
		}, gitlab.WithContext(ctx))
		if err != nil {
			return fmt.Errorf("create draft note on merge request !%d in project %q: %w", iid, resolved.ref, err)
		}

		return output.WriteDraftNoteCreated(cmd.OutOrStdout(), rootOpts.output, rootOpts.mode, draft, iid, opts.fileSet, hints)
	}

	if opts.note {
		note, _, err := client.Notes.CreateMergeRequestNote(resolved.ref, iid, &gitlab.CreateMergeRequestNoteOptions{
			Body: gitlab.Ptr(body),
		}, gitlab.WithContext(ctx))
		if err != nil {
			return fmt.Errorf("comment on merge request !%d in project %q: %w", iid, resolved.ref, err)
		}
		if note == nil {
			return errors.New("gitlab api returned an empty note response")
		}

		return output.WriteCommentCreated(cmd.OutOrStdout(), rootOpts.output, rootOpts.mode, output.CommentCreatedFromNote("", note), iid, false, hints)
	}

	discussion, _, err := client.Discussions.CreateMergeRequestDiscussion(resolved.ref, iid, &gitlab.CreateMergeRequestDiscussionOptions{
		Body:     gitlab.Ptr(body),
		Position: position,
	}, gitlab.WithContext(ctx))
	if err != nil {
		return fmt.Errorf("comment on merge request !%d in project %q: %w", iid, resolved.ref, err)
	}
	note := firstDiscussionNote(discussion)
	if note == nil {
		return errors.New("gitlab api returned a discussion without notes")
	}

	return output.WriteCommentCreated(cmd.OutOrStdout(), rootOpts.output, rootOpts.mode, output.CommentCreatedFromNote(discussion.ID, note), iid, opts.fileSet, hints)
}

func runMRCommentReply(cmd *cobra.Command, rootOpts *rootOptions, opts *mrCommentOptions, client *gitlab.Client, resolved resolvedProject, iid int64, body string, hints *output.MRHintContext) error {
	ctx := commandContext(cmd)

	discussion, err := resolveDiscussionRef(ctx, client, resolved.ref, iid, opts.replyTo)
	if err != nil {
		return err
	}

	if opts.draft {
		createOpts := &gitlab.CreateDraftNoteOptions{
			Note:                  gitlab.Ptr(body),
			InReplyToDiscussionID: gitlab.Ptr(discussion.ID),
		}
		if opts.resolve {
			createOpts.ResolveDiscussion = gitlab.Ptr(true)
		}

		draft, _, err := client.DraftNotes.CreateDraftNote(resolved.ref, iid, createOpts, gitlab.WithContext(ctx))
		if err != nil {
			return fmt.Errorf("create draft reply to discussion %s on merge request !%d in project %q: %w", output.ShortDiscussionID(discussion.ID), iid, resolved.ref, err)
		}

		return output.WriteDraftNoteCreated(cmd.OutOrStdout(), rootOpts.output, rootOpts.mode, draft, iid, false, hints)
	}

	note, _, err := client.Discussions.AddMergeRequestDiscussionNote(resolved.ref, iid, discussion.ID, &gitlab.AddMergeRequestDiscussionNoteOptions{
		Body: gitlab.Ptr(body),
	}, gitlab.WithContext(ctx))
	if err != nil {
		return fmt.Errorf("reply to discussion %s on merge request !%d in project %q: %w", output.ShortDiscussionID(discussion.ID), iid, resolved.ref, err)
	}
	if note == nil {
		return errors.New("gitlab api returned an empty note response")
	}

	return output.WriteCommentCreated(cmd.OutOrStdout(), rootOpts.output, rootOpts.mode, output.CommentCreatedFromNote(discussion.ID, note), iid, false, hints)
}

func firstDiscussionNote(discussion *gitlab.Discussion) *gitlab.Note {
	if discussion == nil {
		return nil
	}
	for _, note := range discussion.Notes {
		if note != nil {
			return note
		}
	}

	return nil
}
