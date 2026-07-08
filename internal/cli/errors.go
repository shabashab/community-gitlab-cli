package cli

import (
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/shabashab/community-gitlab-cli/internal/credstore"
	"github.com/shabashab/community-gitlab-cli/internal/gitlabclient"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
	gitlab "gitlab.com/gitlab-org/api/client-go/v2"
)

// usageError marks errors caused by invalid invocation (unknown flags, bad
// arguments, unsupported formats). Commands exit with code 2 for usage errors
// and 1 for everything else, matching the axi exit-code contract.
type usageError struct {
	err  error
	help []string
}

func (e *usageError) Error() string { return e.err.Error() }

func (e *usageError) Unwrap() error { return e.err }

func newUsageError(err error, help ...string) error {
	return &usageError{err: err, help: help}
}

func exitCodeForError(err error) int {
	var usage *usageError
	if errors.As(err, &usage) {
		return 2
	}

	return 1
}

// wrapArgsValidator converts cobra positional-argument failures into usage
// errors so they exit 2 and carry a per-command help hint.
func wrapArgsValidator(validator cobra.PositionalArgs) cobra.PositionalArgs {
	return func(cmd *cobra.Command, args []string) error {
		if err := validator(cmd, args); err != nil {
			return newUsageError(err, fmt.Sprintf("Run `%s --help` for usage", cmd.CommandPath()))
		}

		return nil
	}
}

// flagErrorFunc turns cobra flag-parse failures into self-correcting usage
// errors that list the command's valid flags inline, so the agent does not
// need a follow-up --help call.
func flagErrorFunc(cmd *cobra.Command, err error) error {
	return newUsageError(
		err,
		fmt.Sprintf("Valid flags for `%s`: %s (--help always allowed)", cmd.CommandPath(), commandFlagList(cmd)),
	)
}

func commandFlagList(cmd *cobra.Command) string {
	cmd.InitDefaultHelpFlag()

	seen := map[string]bool{}
	var names []string
	collect := func(name string) {
		if !seen[name] {
			seen[name] = true
			names = append(names, "--"+name)
		}
	}
	cmd.LocalFlags().VisitAll(func(flag *pflag.Flag) { collect(flag.Name) })
	cmd.InheritedFlags().VisitAll(func(flag *pflag.Flag) { collect(flag.Name) })
	sort.Strings(names)

	return strings.Join(names, ", ")
}

// translateGitLabAPIError rewrites raw client-go request errors ("GET
// https://host/api/v4/...: 401 {message: ...}") into a short, actionable
// message while preserving the command's own context prefix.
func translateGitLabAPIError(err error) (message string, ok bool) {
	var respErr *gitlab.ErrorResponse
	if !errors.As(err, &respErr) {
		return "", false
	}

	var short string
	switch respErr.StatusCode {
	case 401:
		short = "GitLab rejected the token (401 Unauthorized)"
	case 403:
		short = "the token lacks permission for this action (403 Forbidden)"
	case 404:
		short = "GitLab resource not found (404)"
	case 409:
		short = "GitLab reports a conflict (409)"
	case 429:
		short = "GitLab API rate limit hit (429), wait and retry"
	default:
		short = fmt.Sprintf("GitLab API error (%d)", respErr.StatusCode)
	}
	if detail := strings.TrimSpace(respErr.Message); detail != "" && respErr.StatusCode != 401 {
		// Non-API responses (proxies, wrong hosts) can carry whole HTML pages
		// as the message; cap the detail so raw bodies never leak through.
		if runes := []rune(detail); len(runes) > apiErrorDetailLimit {
			detail = string(runes[:apiErrorDetailLimit]) + "…"
		}
		short = fmt.Sprintf("%s: %s", short, detail)
	}

	return strings.Replace(err.Error(), respErr.Error(), short, 1), true
}

// apiErrorDetailLimit caps how much of a GitLab error response body is echoed
// into translated messages.
const apiErrorDetailLimit = 200

// classifyError maps an error to a stable machine-readable code, an
// agent-facing message, and next-step suggestions.
func classifyError(err error, bin string) (code, message string, help []string) {
	message = err.Error()

	switch {
	case errors.Is(err, errMissingExplicitBaseURL):
		return "missing_gitlab_base_url", message, []string{
			fmt.Sprintf("Run `%s auth login <token> --gitlab-base-url https://<gitlab-host>`", bin),
		}
	case errors.Is(err, errTokenVerification):
		return "invalid_gitlab_token", message, []string{
			"Check the token value and scopes (read_api at minimum), then retry `auth login`",
		}
	case errors.Is(err, credstore.ErrNotFound):
		return "no_stored_credential", message, []string{
			fmt.Sprintf("Run `%s auth status` to inspect credential state", bin),
			fmt.Sprintf("Run `%s auth login <token> --gitlab-base-url <url>` to store a credential", bin),
		}
	case errors.Is(err, credstore.ErrCorruptCredentials), errors.Is(err, credstore.ErrUnsupportedVersion):
		return "credential_store_unreadable", message, []string{
			fmt.Sprintf("Inspect or remove ~/.gl/credentials.json, then run `%s auth login` again", bin),
		}
	case errors.Is(err, gitlabclient.ErrMissingToken):
		return "missing_gitlab_token", message, []string{
			fmt.Sprintf("Set GITLAB_TOKEN, pass --gitlab-token, or run `%s auth login <token> --gitlab-base-url <url>`", bin),
		}
	case errors.Is(err, errMissingProject):
		return "missing_gitlab_project", message, []string{
			"Run inside a git repository with remote origin configured, or pass --project <id-or-path>",
		}
	case errors.Is(err, errInvalidMergeRequestRef):
		return "invalid_merge_request_ref", message, []string{
			fmt.Sprintf("Reference merge requests as !<iid> or <iid>, for example `%s mr !123`", bin),
		}
	case errors.Is(err, errUnknownMergeRequestAction):
		return "unknown_merge_request_action", message, []string{
			fmt.Sprintf("Supported actions: view (alias: info), update — run `%s mr --help` for usage", bin),
		}
	case errors.Is(err, errUserNotFound):
		return "user_not_found", message, []string{
			"Check the username spelling, or pass a numeric user ID to --assignee/--reviewer",
		}
	case errors.Is(err, errMissingSourceBranch):
		return "missing_source_branch", message, []string{
			"Pass --source-branch <branch> explicitly",
		}
	case errors.Is(err, errMissingTargetBranch):
		return "missing_target_branch", message, []string{
			"Pass --target-branch <branch> explicitly",
		}
	case errors.Is(err, errNoUpdateFlags):
		return "no_update_flags", message, []string{
			fmt.Sprintf("Pass at least one field flag — run `%s mr update --help` for the list", bin),
		}
	}

	if translated, ok := translateGitLabAPIError(err); ok {
		var respErr *gitlab.ErrorResponse
		errors.As(err, &respErr)
		switch respErr.StatusCode {
		case 401:
			return "gitlab_auth_failed", translated, []string{
				fmt.Sprintf("Run `%s auth status` to check stored credentials", bin),
				fmt.Sprintf("Run `%s auth login <token> --gitlab-base-url <url>` with a valid token", bin),
			}
		case 403:
			return "gitlab_forbidden", translated, []string{
				"Use a token with sufficient scopes and project access",
			}
		case 404:
			return "gitlab_not_found", translated, []string{
				"Check the project path or ID and the merge request iid",
			}
		case 409:
			return "gitlab_conflict", translated, []string{
				fmt.Sprintf("An open merge request for this source/target branch pair may already exist — run `%s mr list --source-branch <branch>`", bin),
			}
		case 429:
			return "gitlab_rate_limited", translated, []string{
				"Wait before retrying; reduce request frequency",
			}
		default:
			return "gitlab_api_error", translated, nil
		}
	}

	var usage *usageError
	if errors.As(err, &usage) {
		return "usage_error", message, usage.help
	}

	return "command_failed", message, []string{
		"Inspect the error message, fix the input or GitLab configuration, then retry",
	}
}
