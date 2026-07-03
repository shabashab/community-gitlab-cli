package cli

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"

	"github.com/shabashab/community-gitlab-cli/internal/gitlabclient"
	gitlab "gitlab.com/gitlab-org/api/client-go/v2"
)

type userOutput struct {
	ID       int64  `json:"id"`
	Username string `json:"username"`
	Name     string `json:"name"`
	State    string `json:"state"`
	WebURL   string `json:"web_url"`
}

type axiUserOutput struct {
	ID       int64  `json:"id"`
	Username string `json:"username"`
	Name     string `json:"name"`
	WebURL   string `json:"web_url"`
}

type axiWhoamiOutput struct {
	User axiUserOutput `json:"user"`
	Next string        `json:"next"`
}

func defaultOutputFormat(mode commandMode) string {
	if mode == commandModeAxi {
		return "toon"
	}

	return "text"
}

func outputFormats(mode commandMode) string {
	if mode == commandModeAxi {
		return "toon, json"
	}

	return "text, json"
}

func writeUser(w io.Writer, format string, mode commandMode, user *gitlab.User) error {
	if user == nil {
		return errors.New("gitlab api returned an empty current user response")
	}

	format = strings.ToLower(strings.TrimSpace(format))
	if format == "" {
		format = defaultOutputFormat(mode)
	}

	if mode == commandModeAxi {
		return writeAxiUser(w, format, user)
	}

	out := userOutput{
		ID:       user.ID,
		Username: user.Username,
		Name:     user.Name,
		State:    user.State,
		WebURL:   user.WebURL,
	}

	switch format {
	case "json":
		encoder := json.NewEncoder(w)
		encoder.SetIndent("", "  ")
		return encoder.Encode(out)
	case "text":
		_, err := fmt.Fprintf(
			w,
			"id: %d\nusername: %s\nname: %s\nstate: %s\nweb_url: %s\n",
			out.ID,
			out.Username,
			out.Name,
			out.State,
			out.WebURL,
		)
		return err
	default:
		return fmt.Errorf("unsupported output format %q: use text or json", format)
	}
}

func writeAxiUser(w io.Writer, format string, user *gitlab.User) error {
	out := axiUserOutput{
		ID:       user.ID,
		Username: user.Username,
		Name:     user.Name,
		WebURL:   user.WebURL,
	}

	switch format {
	case "json":
		encoder := json.NewEncoder(w)
		encoder.SetIndent("", "  ")
		return encoder.Encode(axiWhoamiOutput{
			User: out,
			Next: "Use project list when available to inspect accessible projects.",
		})
	case "toon":
		_, err := fmt.Fprintf(
			w,
			"user{id,username,name,web_url}:\n  %d,%s,%s,%s\nnext: %s\n",
			out.ID,
			toonValue(out.Username),
			toonValue(out.Name),
			toonValue(out.WebURL),
			toonValue("Use project list when available to inspect accessible projects."),
		)
		return err
	default:
		return fmt.Errorf("unsupported output format %q: use toon or json", format)
	}
}

func writeCommandError(w io.Writer, mode commandMode, err error) {
	if mode != commandModeAxi {
		fmt.Fprintln(w, err)
		return
	}

	code := "command_failed"
	next := "Inspect the error message, fix the input or GitLab configuration, then retry."
	if errors.Is(err, gitlabclient.ErrMissingToken) {
		code = "missing_gitlab_token"
		next = "Set GITLAB_TOKEN or pass --gitlab-token, then retry."
	}

	fmt.Fprintf(
		w,
		"error{code,message}:\n  %s,%s\nnext: %s\n",
		code,
		toonValue(err.Error()),
		toonValue(next),
	)
}

func toonValue(value string) string {
	if value == "" {
		return `""`
	}

	if strings.ContainsAny(value, ",\n\r\t\"{}[]:") || strings.Contains(value, " ") {
		return strconv.Quote(value)
	}

	return value
}
