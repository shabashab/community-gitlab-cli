package output

import (
	"fmt"
	"io"
	"sort"
	"strings"

	gitlab "gitlab.com/gitlab-org/api/client-go/v2"
)

type reactionGroup struct {
	name  string
	users []string
}

// groupReactions buckets awards by emoji name with a deterministic order:
// most-awarded group first, ties by name, usernames ascending.
func groupReactions(awards []*gitlab.AwardEmoji) []reactionGroup {
	byName := map[string][]string{}
	for _, award := range awards {
		if award == nil {
			continue
		}
		byName[award.Name] = append(byName[award.Name], award.User.Username)
	}

	groups := make([]reactionGroup, 0, len(byName))
	for name, users := range byName {
		sort.Strings(users)
		groups = append(groups, reactionGroup{name: name, users: users})
	}
	sort.Slice(groups, func(i, j int) bool {
		if len(groups[i].users) != len(groups[j].users) {
			return len(groups[i].users) > len(groups[j].users)
		}

		return groups[i].name < groups[j].name
	})

	return groups
}

// FormatNoteReactions renders one note's reactions compactly as space-joined
// "name:count(user1,user2)" groups. Empty input renders as "".
func FormatNoteReactions(awards []*gitlab.AwardEmoji) string {
	groups := groupReactions(awards)
	parts := make([]string, 0, len(groups))
	for _, group := range groups {
		parts = append(parts, fmt.Sprintf("%s:%d(%s)", group.name, len(group.users), strings.Join(group.users, ",")))
	}

	return strings.Join(parts, " ")
}

// AggregateReactions renders thread-level totals as space-joined "name:count"
// groups, keeping list rows frugal.
func AggregateReactions(awards []*gitlab.AwardEmoji) string {
	groups := groupReactions(awards)
	parts := make([]string, 0, len(groups))
	for _, group := range groups {
		parts = append(parts, fmt.Sprintf("%s:%d", group.name, len(group.users)))
	}

	return strings.Join(parts, " ")
}

// DiscussionReactionOutput identifies the reaction a react/unreact command
// acted on. There is no award echo — the note view is the source of truth.
type DiscussionReactionOutput struct {
	DiscussionID string `json:"discussion_id" toon:"discussion_id"`
	NoteID       int64  `json:"note_id" toon:"note_id"`
	Emoji        string `json:"emoji" toon:"emoji"`
}

type discussionReactionActionOutput struct {
	Reaction DiscussionReactionOutput `json:"reaction" toon:"reaction"`
	Action   string                   `json:"action" toon:"action"`
	Noop     bool                     `json:"noop,omitempty" toon:"noop,omitempty"`
	Help     []string                 `json:"help,omitempty" toon:"help,omitempty"`
}

func WriteDiscussionReaction(w io.Writer, format string, mode Mode, out DiscussionReactionOutput, action string, noop bool, iid int64, hints *MRHintContext) error {
	if mode == ModeAxi {
		axiOut := out
		axiOut.DiscussionID = ShortDiscussionID(out.DiscussionID)

		return WriteAxi(w, format, discussionReactionActionOutput{
			Reaction: axiOut,
			Action:   action,
			Noop:     noop,
			Help:     discussionReactionHelp(action, iid, axiOut.DiscussionID, out.NoteID, out.Emoji, hints),
		})
	}

	format, err := NormalizeFormat(format, mode)
	if err != nil {
		return err
	}

	if format == "json" {
		return WriteJSON(w, discussionReactionActionOutput{Reaction: out, Action: action, Noop: noop})
	}

	return writeDiscussionReactionText(w, out, action, noop)
}

func discussionReactionHelp(action string, iid int64, discussionID string, noteID int64, emoji string, hints *MRHintContext) []string {
	suffix := hints.ProjectSuffix()
	viewHint := fmt.Sprintf("Run `%s %d %s%s` for the full thread", MRDiscussionViewCommandName, iid, discussionID, suffix)

	if action == "react" {
		return []string{
			viewHint,
			fmt.Sprintf("Run `mr discussion unreact %d %s %d %s%s` to remove the reaction", iid, discussionID, noteID, emoji, suffix),
		}
	}

	return []string{
		viewHint,
		fmt.Sprintf("Run `mr discussion react %d %s %d %s%s` to add the reaction back", iid, discussionID, noteID, emoji, suffix),
	}
}

func writeDiscussionReactionText(w io.Writer, out DiscussionReactionOutput, action string, noop bool) error {
	var line string
	switch {
	case action == "react" && noop:
		line = fmt.Sprintf("note %d already has your :%s: reaction (no-op)", out.NoteID, out.Emoji)
	case action == "react":
		line = fmt.Sprintf("reacted with :%s: to note %d", out.Emoji, out.NoteID)
	case noop:
		line = fmt.Sprintf("note %d has no :%s: reaction from you (no-op)", out.NoteID, out.Emoji)
	default:
		line = fmt.Sprintf("removed :%s: reaction from note %d", out.Emoji, out.NoteID)
	}

	_, err := fmt.Fprintf(w, "%s\ndiscussion: %s\n", line, out.DiscussionID)

	return err
}
