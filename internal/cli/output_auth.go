package cli

import (
	"fmt"
	"io"
	"strings"
)

type authLoginResult struct {
	Username string `json:"username" toon:"username"`
	Domain   string `json:"domain" toon:"domain"`
	Backend  string `json:"backend" toon:"backend"`
}

type axiAuthLoginOutput struct {
	Login authLoginResult `json:"login" toon:"login"`
	Help  []string        `json:"help,omitempty" toon:"help,omitempty"`
}

type authLogoutResult struct {
	Domain   string   `json:"domain" toon:"domain"`
	Backends []string `json:"backends" toon:"backends"`
	Noop     bool     `json:"noop,omitempty" toon:"noop,omitempty"`
}

type axiAuthLogoutOutput struct {
	Logout authLogoutResult `json:"logout" toon:"logout"`
	Help   []string         `json:"help,omitempty" toon:"help,omitempty"`
}

type authStatusResult struct {
	Domain        string   `json:"domain" toon:"domain"`
	Authenticated bool     `json:"authenticated" toon:"authenticated"`
	Backends      []string `json:"backends" toon:"backends"`
	Warnings      []string `json:"warnings,omitempty" toon:"warnings,omitempty"`
}

type axiAuthStatusOutput struct {
	Status authStatusResult `json:"status" toon:"status"`
	Help   []string         `json:"help,omitempty" toon:"help,omitempty"`
}

func writeAuthLogin(w io.Writer, format string, mode commandMode, result authLoginResult) error {
	if mode == commandModeAxi {
		return writeAxi(w, format, axiAuthLoginOutput{
			Login: result,
			Help: []string{
				fmt.Sprintf("Credential stored for %s — run `whoami` to verify API access", result.Domain),
				"Run `mr` to list open merge requests",
			},
		})
	}

	format, err := normalizeOutputFormat(format, mode)
	if err != nil {
		return err
	}

	if format == "json" {
		return writeJSON(w, result)
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

func writeAuthLogout(w io.Writer, format string, mode commandMode, result authLogoutResult) error {
	if result.Backends == nil {
		result.Backends = []string{}
	}

	if mode == commandModeAxi {
		return writeAxi(w, format, axiAuthLogoutOutput{
			Logout: result,
			Help: []string{
				"Run `auth login <token> --gitlab-base-url <url>` to authenticate again",
			},
		})
	}

	format, err := normalizeOutputFormat(format, mode)
	if err != nil {
		return err
	}

	if format == "json" {
		return writeJSON(w, result)
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

func writeAuthStatus(w io.Writer, format string, mode commandMode, result authStatusResult) error {
	if result.Backends == nil {
		result.Backends = []string{}
	}

	if mode == commandModeAxi {
		help := []string{"Run `auth login <token> --gitlab-base-url <url>` to store a credential"}
		if result.Authenticated {
			help = []string{"Run `whoami` to verify the stored token still works"}
		}

		return writeAxi(w, format, axiAuthStatusOutput{Status: result, Help: help})
	}

	format, err := normalizeOutputFormat(format, mode)
	if err != nil {
		return err
	}

	if format == "json" {
		return writeJSON(w, result)
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
