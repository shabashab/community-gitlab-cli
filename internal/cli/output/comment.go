package output

import (
	"fmt"
	"io"
	"strings"

	gitlab "gitlab.com/gitlab-org/api/client-go/v2"
)

// CommentCreatedOutput is the compact created-comment view. There is no body
// echo — the caller knows what it wrote; File/Line come from the response
// position so agents see what GitLab actually anchored.
type CommentCreatedOutput struct {
	DiscussionID string `json:"discussion_id,omitempty" toon:"discussion_id,omitempty"`
	NoteID       int64  `json:"note_id" toon:"note_id"`
	Author       string `json:"author" toon:"author"`
	Type         string `json:"type" toon:"type"`
	Resolvable   bool   `json:"resolvable" toon:"resolvable"`
	File         string `json:"file,omitempty" toon:"file,omitempty"`
	Line         int64  `json:"line,omitempty" toon:"line,omitempty"`
	CreatedAt    string `json:"created_at" toon:"created_at"`
}

type axiCommentCreatedOutput struct {
	Comment CommentCreatedOutput `json:"comment" toon:"comment"`
	Help    []string             `json:"help,omitempty" toon:"help,omitempty"`
}

// CommentCreatedFromNote builds the created-comment view from the response
// note. discussionID is empty for plain notes created via the notes API.
func CommentCreatedFromNote(discussionID string, note *gitlab.Note) CommentCreatedOutput {
	out := CommentCreatedOutput{
		DiscussionID: strings.ToLower(discussionID),
		NoteID:       note.ID,
		Author:       note.Author.Username,
		Type:         string(note.Type),
		Resolvable:   note.Resolvable,
		CreatedAt:    formatTimeValue(note.CreatedAt),
	}
	if out.Type == "" {
		out.Type = string(gitlab.GenericNote)
	}
	if position := note.Position; position != nil {
		out.File = position.NewPath
		out.Line = position.NewLine
		if out.File == "" {
			out.File = position.OldPath
		}
		if out.Line == 0 {
			out.Line = position.OldLine
		}
	}

	return out
}

func WriteCommentCreated(w io.Writer, format string, mode Mode, out CommentCreatedOutput, iid int64, positionRequested bool, hints *MRHintContext) error {
	if mode == ModeAxi {
		axiOut := out
		axiOut.DiscussionID = ShortDiscussionID(out.DiscussionID)

		var help []string
		if axiOut.DiscussionID != "" {
			help = append(help, fmt.Sprintf(
				"Run `%s %d %s%s` for the full thread",
				MRDiscussionViewCommandName,
				iid,
				axiOut.DiscussionID,
				hints.ProjectSuffix(),
			))
		} else {
			help = append(help, fmt.Sprintf(
				"Run `mr discussions %d --state all%s` to list comments on the merge request",
				iid,
				hints.ProjectSuffix(),
			))
		}
		if downgraded := commentPositionDowngradeHint(out, iid, positionRequested, hints); downgraded != "" {
			help = append(help, downgraded)
		}

		return WriteAxi(w, format, axiCommentCreatedOutput{Comment: axiOut, Help: help})
	}

	format, err := NormalizeFormat(format, mode)
	if err != nil {
		return err
	}

	if format == "json" {
		return WriteJSON(w, out)
	}

	return writeCommentCreatedText(w, out)
}

// commentPositionDowngradeHint surfaces GitLab's silent position drop: the
// API can answer 201 yet attach the comment to the merge request instead of
// the requested diff line. The mutation succeeded, so this stays a hint —
// never an error an agent would retry into a duplicate.
func commentPositionDowngradeHint(out CommentCreatedOutput, iid int64, positionRequested bool, hints *MRHintContext) string {
	if !positionRequested || out.Type == string(gitlab.DiffNote) {
		return ""
	}

	return fmt.Sprintf(
		"GitLab did not anchor the comment to the requested diff position (type %s) — run `%s %d %s%s` to verify",
		out.Type,
		MRDiscussionViewCommandName,
		iid,
		ShortDiscussionID(out.DiscussionID),
		hints.ProjectSuffix(),
	)
}

func writeCommentCreatedText(w io.Writer, out CommentCreatedOutput) error {
	if out.DiscussionID != "" {
		if _, err := fmt.Fprintf(w, "discussion: %s\n", out.DiscussionID); err != nil {
			return err
		}
	}
	if _, err := fmt.Fprintf(
		w,
		"note: %d\nauthor: %s\ntype: %s\nresolvable: %t\n",
		out.NoteID,
		out.Author,
		out.Type,
		out.Resolvable,
	); err != nil {
		return err
	}
	if out.File != "" {
		location := out.File
		if out.Line > 0 {
			location = fmt.Sprintf("%s:%d", out.File, out.Line)
		}
		if _, err := fmt.Fprintf(w, "file: %s\n", location); err != nil {
			return err
		}
	}
	_, err := fmt.Fprintf(w, "created_at: %s\n", out.CreatedAt)

	return err
}
