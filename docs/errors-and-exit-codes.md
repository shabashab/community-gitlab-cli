# Errors and Exit Codes

Both binaries share one error model, implemented in `internal/cli/errors.go` and `internal/cli/root.go`. The presentation differs by mode: `gl` prints plain errors to stderr for humans; `gl-axi` prints structured errors to **stdout** in the requested output format so agents parse failures exactly like results.

## Exit codes

| Code | Meaning | Examples |
| ---- | ------- | -------- |
| 0 | Success, including no-ops | normal output; `auth logout` with nothing stored |
| 1 | Runtime error | API failures, missing token, unreachable host |
| 2 | Usage error | unknown flag or command, bad arguments, unsupported `--output`, unknown `--fields` name |

Usage errors are marked internally by wrapping in `newUsageError(err, help...)`. Cobra flag-parse failures and positional-argument failures are converted automatically (`SetFlagErrorFunc`, `wrapArgsValidator`), so every invalid invocation exits 2 without per-command effort.

## Structured error shape (gl-axi)

```
error: "unknown flag: --stat"
code: usage_error
help[1]: "Valid flags for `gl-axi mr list`: --author, --draft, ... (--help always allowed)"
```

With `-o json`:

```json
{
  "error": "unknown flag: --stat",
  "code": "usage_error",
  "help": ["Valid flags for `gl-axi mr list`: ..."]
}
```

Design rules:

- **Stdout, same format as results.** Agents read one channel; stderr is reserved for diagnostics humans might read.
- **Self-correcting in one turn.** Unknown-flag errors embed the command's full valid-flag list, so the agent does not need a follow-up `--help` call. Other errors suggest the specific command that fixes the problem.
- **Fail loud on unrecognized input.** Unknown flags, unknown commands, unknown `--fields` names, and extra positional arguments are rejected by name — never silently ignored.
- **Validate before dependencies.** Shared flags (`--output`) are validated in the root `PersistentPreRunE`, before any GitLab request is made.

## Error codes

| Code | Trigger | Exit |
| ---- | ------- | ---- |
| `usage_error` | invalid invocation not covered by a more specific code | 2 |
| `invalid_merge_request_ref` | `mr` reference that is not `!<iid>` / `<iid>` | 2 |
| `unknown_merge_request_action` | unsupported per-MR action (supported: `view`, alias `info`) | 2 |
| `missing_gitlab_token` | no token from flag, env, or credential store | 1 |
| `missing_gitlab_base_url` | `auth login` without explicit `--gitlab-base-url` | 1 |
| `invalid_gitlab_token` | token verification failed during `auth login` | 1 |
| `no_stored_credential` | credential lookup found nothing | 1 |
| `credential_store_unreadable` | corrupt or unsupported `~/.gl/credentials.json` | 1 |
| `missing_gitlab_project` | no `--project` and no discoverable git origin | 1 |
| `gitlab_auth_failed` | GitLab returned 401 | 1 |
| `gitlab_forbidden` | GitLab returned 403 | 1 |
| `gitlab_not_found` | GitLab returned 404 | 1 |
| `gitlab_rate_limited` | GitLab returned 429 | 1 |
| `gitlab_api_error` | any other GitLab HTTP error | 1 |
| `command_failed` | anything unclassified | 1 |

## GitLab API error translation

Raw client-go errors look like `GET https://host/api/v4/user: 401 {message: 401 Unauthorized}` — request method, full URL, and response body. `translateGitLabAPIError` rewrites the API portion into a short actionable message while preserving the command's own context prefix:

```
error: "get merge request !5 in project \"group/app\": GitLab resource not found (404): 404 Not Found"
code: gitlab_not_found
help[1]: Check the project path or ID and the merge request iid
```

Request URLs and methods never leak into agent-facing messages. GitLab's semantic response message (e.g. `404 Project Not Found`) is kept — it is the meaning, not the noise.

## Idempotent mutations

Mutations do not fail when the desired state already exists. `auth logout` with no stored credential reports the no-op and exits 0:

```
logout:
  domain: gitlab.example.com
  backends[0]:
  noop: true
help[1]: Run `auth login <token> --gitlab-base-url <url>` to authenticate again
```

Apply the same rule to future mutations (closing an already-closed MR, deleting an absent label): acknowledge, report `noop`, exit 0. Reserve non-zero exits for intents that genuinely cannot be satisfied.
