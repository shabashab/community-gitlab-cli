package cli

import (
	"errors"
	"fmt"
	"io"

	gitlab "gitlab.com/gitlab-org/api/client-go/v2"
)

type userOutput struct {
	ID       int64  `json:"id" toon:"id"`
	Username string `json:"username" toon:"username"`
	Name     string `json:"name" toon:"name"`
	State    string `json:"state" toon:"state"`
	WebURL   string `json:"web_url" toon:"web_url"`
}

type axiUserOutput struct {
	ID       int64  `json:"id" toon:"id"`
	Username string `json:"username" toon:"username"`
	Name     string `json:"name" toon:"name"`
	WebURL   string `json:"web_url" toon:"web_url"`
}

type axiWhoamiOutput struct {
	User axiUserOutput `json:"user" toon:"user"`
	Help []string      `json:"help,omitempty" toon:"help,omitempty"`
}

func writeUser(w io.Writer, format string, mode commandMode, user *gitlab.User) error {
	if user == nil {
		return errors.New("gitlab api returned an empty current user response")
	}

	if mode == commandModeAxi {
		return writeAxi(w, format, axiWhoamiOutput{
			User: axiUserFromAPI(user),
			Help: whoamiHelp(),
		})
	}

	format, err := normalizeOutputFormat(format, mode)
	if err != nil {
		return err
	}

	out := userOutput{
		ID:       user.ID,
		Username: user.Username,
		Name:     user.Name,
		State:    user.State,
		WebURL:   user.WebURL,
	}

	if format == "json" {
		return writeJSON(w, out)
	}

	_, err = fmt.Fprintf(
		w,
		"id: %d\nusername: %s\nname: %s\nstate: %s\nweb_url: %s\n",
		out.ID,
		out.Username,
		out.Name,
		out.State,
		out.WebURL,
	)

	return err
}

func axiUserFromAPI(user *gitlab.User) axiUserOutput {
	return axiUserOutput{
		ID:       user.ID,
		Username: user.Username,
		Name:     user.Name,
		WebURL:   user.WebURL,
	}
}

func whoamiHelp() []string {
	return []string{
		"Run `project info` to inspect the current project",
		"Run `mr` to list open merge requests",
	}
}
