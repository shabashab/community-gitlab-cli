package cli

import (
	"fmt"
	"io"
	"strings"

	gitlab "gitlab.com/gitlab-org/api/client-go/v2"
)

// commentCreatedOutput is the compact created-comment view. There is no body
// echo — the caller knows what it wrote; File/Line come from the response
// position so agents see what GitLab actually anchored.
type commentCreatedOutput struct {
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
	Comment commentCreatedOutput `json:"comment" toon:"comment"`
	Help    []string             `json:"help,omitempty" toon:"help,omitempty"`
}

// commentCreatedFromNote builds the created-comment view from the response
// note. discussionID is empty for plain notes created via the notes API.
func commentCreatedFromNote(discussionID string, note *gitlab.Note) commentCreatedOutput {
	out := commentCreatedOutput{
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

func writeCommentCreated(w io.Writer, format string, mode commandMode, out commentCreatedOutput, iid int64, positionRequested bool, hints *mrHintContext) error {
	if mode == commandModeAxi {
		axiOut := out
		axiOut.DiscussionID = shortDiscussionID(out.DiscussionID)

		var help []string
		if axiOut.DiscussionID != "" {
			help = append(help, fmt.Sprintf(
				"Run `%s %d %s%s` for the full thread",
				mrDiscussionViewCommandName,
				iid,
				axiOut.DiscussionID,
				hints.projectSuffix(),
			))
		} else {
			help = append(help, fmt.Sprintf(
				"Run `mr discussions %d --state all%s` to list comments on the merge request",
				iid,
				hints.projectSuffix(),
			))
		}
		if downgraded := commentPositionDowngradeHint(out, iid, positionRequested, hints); downgraded != "" {
			help = append(help, downgraded)
		}

		return writeAxi(w, format, axiCommentCreatedOutput{Comment: axiOut, Help: help})
	}

	format, err := normalizeOutputFormat(format, mode)
	if err != nil {
		return err
	}

	if format == "json" {
		return writeJSON(w, out)
	}

	return writeCommentCreatedText(w, out)
}

// commentPositionDowngradeHint surfaces GitLab's silent position drop: the
// API can answer 201 yet attach the comment to the merge request instead of
// the requested diff line. The mutation succeeded, so this stays a hint —
// never an error an agent would retry into a duplicate.
func commentPositionDowngradeHint(out commentCreatedOutput, iid int64, positionRequested bool, hints *mrHintContext) string {
	if !positionRequested || out.Type == string(gitlab.DiffNote) {
		return ""
	}

	return fmt.Sprintf(
		"GitLab did not anchor the comment to the requested diff position (type %s) — run `%s %d %s%s` to verify",
		out.Type,
		mrDiscussionViewCommandName,
		iid,
		shortDiscussionID(out.DiscussionID),
		hints.projectSuffix(),
	)
}

func writeCommentCreatedText(w io.Writer, out commentCreatedOutput) error {
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
