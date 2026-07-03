package cli

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"

	gitlab "gitlab.com/gitlab-org/api/client-go/v2"
)

type userOutput struct {
	ID       int64  `json:"id"`
	Username string `json:"username"`
	Name     string `json:"name"`
	State    string `json:"state"`
	WebURL   string `json:"web_url"`
}

func writeUser(w io.Writer, format string, user *gitlab.User) error {
	if user == nil {
		return errors.New("gitlab api returned an empty current user response")
	}

	out := userOutput{
		ID:       user.ID,
		Username: user.Username,
		Name:     user.Name,
		State:    user.State,
		WebURL:   user.WebURL,
	}

	switch strings.ToLower(strings.TrimSpace(format)) {
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
