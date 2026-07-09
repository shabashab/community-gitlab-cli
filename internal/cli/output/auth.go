package output

import (
	"fmt"
	"io"
	"strings"
)

type AuthLoginResult struct {
	Username string `json:"username" toon:"username"`
	Domain   string `json:"domain" toon:"domain"`
	Backend  string `json:"backend" toon:"backend"`
}

type axiAuthLoginOutput struct {
	Login AuthLoginResult `json:"login" toon:"login"`
	Help  []string        `json:"help,omitempty" toon:"help,omitempty"`
}

type AuthLogoutResult struct {
	Domain   string   `json:"domain" toon:"domain"`
	Backends []string `json:"backends" toon:"backends"`
	Noop     bool     `json:"noop,omitempty" toon:"noop,omitempty"`
}

type axiAuthLogoutOutput struct {
	Logout AuthLogoutResult `json:"logout" toon:"logout"`
	Help   []string         `json:"help,omitempty" toon:"help,omitempty"`
}

type AuthStatusResult struct {
	Domain        string   `json:"domain" toon:"domain"`
	Authenticated bool     `json:"authenticated" toon:"authenticated"`
	Backends      []string `json:"backends" toon:"backends"`
	Warnings      []string `json:"warnings,omitempty" toon:"warnings,omitempty"`
}

type axiAuthStatusOutput struct {
	Status AuthStatusResult `json:"status" toon:"status"`
	Help   []string         `json:"help,omitempty" toon:"help,omitempty"`
}

func WriteAuthLogin(w io.Writer, format string, mode Mode, result AuthLoginResult) error {
	if mode == ModeAxi {
		return WriteAxi(w, format, axiAuthLoginOutput{
			Login: result,
			Help: []string{
				fmt.Sprintf("Credential stored for %s — run `whoami` to verify API access", result.Domain),
				"Run `mr` to list open merge requests",
			},
		})
	}

	format, err := NormalizeFormat(format, mode)
	if err != nil {
		return err
	}

	if format == "json" {
		return WriteJSON(w, result)
	}

	_, err = fmt.Fprintf(
		w,
		"username: %s\ndomain: %s\nbackend: %s\n",
		result.Username,
		result.Domain,
		result.Backend,
	)

	return err
}

func WriteAuthLogout(w io.Writer, format string, mode Mode, result AuthLogoutResult) error {
	if result.Backends == nil {
		result.Backends = []string{}
	}

	if mode == ModeAxi {
		return WriteAxi(w, format, axiAuthLogoutOutput{
			Logout: result,
			Help: []string{
				"Run `auth login <token> --gitlab-base-url <url>` to authenticate again",
			},
		})
	}

	format, err := NormalizeFormat(format, mode)
	if err != nil {
		return err
	}

	if format == "json" {
		return WriteJSON(w, result)
	}

	if result.Noop {
		_, err = fmt.Fprintf(w, "domain: %s\nno stored credential (no-op)\n", result.Domain)
		return err
	}

	_, err = fmt.Fprintf(
		w,
		"domain: %s\nremoved_from: %s\n",
		result.Domain,
		strings.Join(result.Backends, ", "),
	)

	return err
}

func WriteAuthStatus(w io.Writer, format string, mode Mode, result AuthStatusResult) error {
	if result.Backends == nil {
		result.Backends = []string{}
	}

	if mode == ModeAxi {
		help := []string{"Run `auth login <token> --gitlab-base-url <url>` to store a credential"}
		if result.Authenticated {
			help = []string{"Run `whoami` to verify the stored token still works"}
		}

		return WriteAxi(w, format, axiAuthStatusOutput{Status: result, Help: help})
	}

	format, err := NormalizeFormat(format, mode)
	if err != nil {
		return err
	}

	if format == "json" {
		return WriteJSON(w, result)
	}

	if _, err := fmt.Fprintf(
		w,
		"domain: %s\nauthenticated: %t\nbackends: %s\n",
		result.Domain,
		result.Authenticated,
		strings.Join(result.Backends, ", "),
	); err != nil {
		return err
	}
	for _, warning := range result.Warnings {
		if _, err := fmt.Fprintf(w, "warning: %s\n", warning); err != nil {
			return err
		}
	}

	return nil
}
