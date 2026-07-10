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

var (
	errInvalidNoteID       = errors.New("invalid note id")
	errNoteNotInDiscussion = errors.New("note is not part of the discussion")
	errInvalidEmojiName    = errors.New("invalid emoji name")
)

const awardEmojiFetchPageSize int64 = 100

func newMRDiscussionReactCommand(rootOpts *rootOptions, projOpts *projectOptions, action string, remove bool) *cobra.Command {
	short := "Add an emoji reaction to a note in a discussion thread"
	semantics := "Reacting again with an emoji you already awarded is a verified no-op"
	if remove {
		short = "Remove your emoji reaction from a note in a discussion thread"
		semantics = "Removing a reaction you have not awarded is a verified no-op"
	}

	cmd := &cobra.Command{
		Use:   action + " <!iid|iid|current> <discussion-id> <note-id> <emoji>",
		Short: short,
		Long: fmt.Sprintf(`%s of a merge request in the current project.

<discussion-id> is the full 40-character hex ID or any unique prefix of one,
and <note-id> is the numeric note id, both as shown by "mr discussion".
<emoji> is a GitLab emoji name such as thumbsup, with or without surrounding
colons. %s and exits 0. The literal reference "current" resolves to the open
merge request of the currently checked out git branch.`, short, semantics),
		Args: wrapArgsValidator(cobra.ExactArgs(4)),
		RunE: func(cmd *cobra.Command, args []string) error {
			iid, err := resolveMergeRequestRef(cmd, rootOpts, projOpts, args[0])
			if err != nil {
				return err
			}

			noteID, err := parseNoteID(args[2])
			if err != nil {
				return err
			}

			emoji, err := normalizeEmojiName(args[3])
			if err != nil {
				return err
			}

			return runMRDiscussionReact(cmd, rootOpts, projOpts, iid, args[1], noteID, emoji, action, remove)
		},
	}

	return cmd
}

func parseNoteID(ref string) (int64, error) {
	id, err := strconv.ParseInt(strings.TrimSpace(ref), 10, 64)
	if err != nil || id <= 0 {
		return 0, newUsageError(fmt.Errorf("%w %q: expected a positive numeric note id", errInvalidNoteID, ref))
	}

	return id, nil
}

// normalizeEmojiName accepts both the bare GitLab emoji name and the
// colon-wrapped chat form, since agents copy either from surrounding context.
func normalizeEmojiName(raw string) (string, error) {
	name := strings.TrimSpace(raw)
	if len(name) >= 2 && strings.HasPrefix(name, ":") && strings.HasSuffix(name, ":") {
		name = name[1 : len(name)-1]
	}
	if name == "" || strings.ContainsAny(name, ": \t\n") {
		return "", newUsageError(fmt.Errorf("%w %q: expected a GitLab emoji name such as thumbsup or :thumbsup:", errInvalidEmojiName, raw))
	}

	return name, nil
}

// ensureNoteInDiscussion guards react/unreact against note ids from other
// threads, so a typo fails with the thread's real note ids instead of a raw
// GitLab 404.
func ensureNoteInDiscussion(discussion *gitlab.Discussion, noteID, iid int64, hints *output.MRHintContext) error {
	ids := make([]string, 0, len(discussion.Notes))
	for _, note := range discussion.Notes {
		if note == nil {
			continue
		}
		if note.ID == noteID {
			return nil
		}
		ids = append(ids, strconv.FormatInt(note.ID, 10))
	}

	shortID := output.ShortDiscussionID(discussion.ID)

	return newHelpError(
		fmt.Errorf("%w: note %d is not part of discussion %s on merge request !%d", errNoteNotInDiscussion, noteID, shortID, iid),
		fmt.Sprintf(
			"This thread's note ids: %s — run `%s %d %s%s` for the full conversation",
			strings.Join(ids, ", "),
			output.MRDiscussionViewCommandName,
			iid,
			shortID,
			hints.ProjectSuffix(),
		),
	)
}

func fetchAllNoteAwardEmoji(ctx context.Context, client *gitlab.Client, projectRef any, iid, noteID int64) ([]*gitlab.AwardEmoji, error) {
	opt := &gitlab.ListAwardEmojiOptions{
		ListOptions: gitlab.ListOptions{PerPage: awardEmojiFetchPageSize, Page: 1},
	}

	var all []*gitlab.AwardEmoji
	for {
		awards, resp, err := client.AwardEmoji.ListMergeRequestAwardEmojiOnNote(projectRef, iid, noteID, opt, gitlab.WithContext(ctx))
		if err != nil {
			return nil, fmt.Errorf("list reactions on note %d of merge request !%d in project %q: %w", noteID, iid, projectRef, err)
		}
		all = append(all, awards...)

		if resp == nil || resp.NextPage == 0 {
			break
		}
		opt.Page = resp.NextPage
	}

	return all, nil
}

func fetchCurrentUserID(ctx context.Context, client *gitlab.Client) (int64, error) {
	user, _, err := client.Users.CurrentUser(gitlab.WithContext(ctx))
	if err != nil {
		return 0, fmt.Errorf("get current GitLab user: %w", err)
	}

	return user.ID, nil
}

func findOwnAward(awards []*gitlab.AwardEmoji, userID int64, emoji string) *gitlab.AwardEmoji {
	for _, award := range awards {
		if award != nil && award.User.ID == userID && award.Name == emoji {
			return award
		}
	}

	return nil
}

func runMRDiscussionReact(cmd *cobra.Command, rootOpts *rootOptions, projOpts *projectOptions, iid int64, ref string, noteID int64, emoji, action string, remove bool) error {
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
	if err := ensureNoteInDiscussion(discussion, noteID, iid, hints); err != nil {
		return err
	}

	var noop bool
	if remove {
		noop, err = removeNoteReaction(ctx, client, resolved.ref, iid, noteID, emoji)
	} else {
		noop, err = addNoteReaction(ctx, client, resolved.ref, iid, noteID, emoji, discussion.ID, hints)
	}
	if err != nil {
		return err
	}

	out := output.DiscussionReactionOutput{
		DiscussionID: strings.ToLower(discussion.ID),
		NoteID:       noteID,
		Emoji:        emoji,
	}

	return output.WriteDiscussionReaction(cmd.OutOrStdout(), rootOpts.output, rootOpts.mode, out, action, noop, iid, hints)
}

// addNoteReaction posts the award optimistically. GitLab answers 404 both for
// a duplicate award and for an unknown emoji name, so the 404 branch lists the
// note's awards: an own award with that name proves the duplicate (verified
// no-op), anything else keeps the original error.
func addNoteReaction(ctx context.Context, client *gitlab.Client, projectRef any, iid, noteID int64, emoji, discussionID string, hints *output.MRHintContext) (noop bool, err error) {
	_, _, err = client.AwardEmoji.CreateMergeRequestAwardEmojiOnNote(
		projectRef,
		iid,
		noteID,
		&gitlab.CreateAwardEmojiOptions{Name: emoji},
		gitlab.WithContext(ctx),
	)
	if err == nil {
		return false, nil
	}

	wrapped := fmt.Errorf("react with :%s: to note %d on merge request !%d in project %q: %w", emoji, noteID, iid, projectRef, err)
	if !isGitLabNotFound(err) {
		return false, wrapped
	}

	userID, userErr := fetchCurrentUserID(ctx, client)
	if userErr != nil {
		return false, wrapped
	}
	awards, listErr := fetchAllNoteAwardEmoji(ctx, client, projectRef, iid, noteID)
	if listErr != nil {
		return false, wrapped
	}
	if findOwnAward(awards, userID, emoji) != nil {
		return true, nil
	}

	return false, newHelpError(wrapped, fmt.Sprintf(
		"GitLab answers 404 for unknown emoji names too — check the emoji name, and run `%s %d %s%s` to confirm the note still exists",
		output.MRDiscussionViewCommandName,
		iid,
		output.ShortDiscussionID(discussionID),
		hints.ProjectSuffix(),
	))
}

// removeNoteReaction lists first because the delete endpoint needs the award
// id; no own award with the requested name is a verified no-op.
func removeNoteReaction(ctx context.Context, client *gitlab.Client, projectRef any, iid, noteID int64, emoji string) (noop bool, err error) {
	userID, err := fetchCurrentUserID(ctx, client)
	if err != nil {
		return false, err
	}
	awards, err := fetchAllNoteAwardEmoji(ctx, client, projectRef, iid, noteID)
	if err != nil {
		return false, err
	}

	own := findOwnAward(awards, userID, emoji)
	if own == nil {
		return true, nil
	}

	_, err = client.AwardEmoji.DeleteMergeRequestAwardEmojiOnNote(projectRef, iid, noteID, own.ID, gitlab.WithContext(ctx))
	if err == nil {
		return false, nil
	}

	wrapped := fmt.Errorf("remove :%s: reaction from note %d on merge request !%d in project %q: %w", emoji, noteID, iid, projectRef, err)
	if !isGitLabNotFound(err) {
		return false, wrapped
	}

	// The award vanished between list and delete; treat as a no-op only when a
	// fresh list proves it is gone.
	awards, listErr := fetchAllNoteAwardEmoji(ctx, client, projectRef, iid, noteID)
	if listErr != nil || findOwnAward(awards, userID, emoji) != nil {
		return false, wrapped
	}

	return true, nil
}
