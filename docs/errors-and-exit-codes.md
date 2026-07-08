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
| `invalid_merge_request_ref` | `mr` reference that is not `!<iid>` / `<iid>` / `current` | 2 |
| `unknown_merge_request_action` | unsupported per-MR action (supported: `view` alias `info`, `diff`, `update` as `mr update !<iid>`, `discussions` as `mr discussions !<iid>`) | 2 |
| `no_update_flags` | `mr update` with no field flags — nothing to change | 2 |
| `invalid_discussion_ref` | discussion reference that is not a 40-character hex ID or a prefix of one | 2 |
| `ambiguous_discussion_ref` | discussion ID prefix matching more than one thread (match count in the message) | 2 |
| `discussion_not_found` | discussion ID prefix matching no thread on the merge request (a full 40-character ID that does not exist surfaces as `gitlab_not_found` instead — the API answers that lookup) | 1 |
| `invalid_draft_note_id` | `mr drafts publish`/`delete` draft-id argument that is not a positive integer | 2 |
| `merge_request_diff_not_ready` | diff-backed commands while GitLab is still preparing the merge request diff (`diff_refs` empty — populates asynchronously after creation; retry shortly) | 1 |
| `file_not_in_diff` | `mr diff --file`/`mr comment --file` path matching no file changed by the merge request (hint lists changed paths) | 1 |
| `line_not_in_diff` | `mr comment --line`/`--old-line` addressing a line that is not visible in the merge request diff on the requested side (hint lists the commentable ranges and suggests the other side when the line exists there) | 1 |
| `diff_too_large` | positioned `mr comment` on a file whose diff GitLab returns collapsed/too large — line resolution is impossible; comment file-level instead | 1 |
| `unsafe_export_path` | `mr diff export` encountered a repository path that would escape the bundle directory | 1 |
| `export_dir_not_empty` | `mr diff export --dir` points at a non-empty existing path without `--force` | 2 |
| `missing_gitlab_token` | no token from flag, env, or credential store | 1 |
| `missing_gitlab_base_url` | `auth login` without explicit `--gitlab-base-url` | 1 |
| `invalid_gitlab_token` | token verification failed during `auth login` | 1 |
| `no_stored_credential` | credential lookup found nothing | 1 |
| `credential_store_unreadable` | corrupt or unsupported `~/.gl/credentials.json` | 1 |
| `missing_gitlab_project` | no `--project` and no discoverable git origin | 1 |
| `user_not_found` | `--assignee`/`--reviewer` username with no GitLab match | 1 |
| `missing_source_branch` | `mr create` without `--source-branch` and no current git branch (detached HEAD, not a repository) | 1 |
| `missing_target_branch` | `mr create` without `--target-branch` and no readable project default branch | 1 |
| `missing_current_branch` | `current` ref with no current git branch (detached HEAD, not a repository) | 1 |
| `no_current_merge_request` | `current` ref: the current branch has no open merge request | 1 |
| `ambiguous_current_merge_request` | `current` ref: several open merge requests share the current source branch (candidates listed in the message) | 1 |
| `gitlab_auth_failed` | GitLab returned 401 | 1 |
| `gitlab_forbidden` | GitLab returned 403 | 1 |
| `gitlab_not_found` | GitLab returned 404 | 1 |
| `gitlab_conflict` | GitLab returned 409 (e.g. an open merge request already exists for the branch pair) | 1 |
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

Request URLs and methods never leak into agent-facing messages. GitLab's semantic response message (e.g. `404 Project Not Found`) is kept — it is the meaning, not the noise. Response detail longer than 200 runes is truncated with an ellipsis so raw bodies (HTML error pages from proxies or wrong hosts) never flood the message.

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

The boundary of the rule is verifiability. `mr create` hitting a 409 because an open merge request already exists for the branch pair is **not** a no-op: the existing merge request may not match the requested title, description, or reviewers, so the intent cannot be confirmed as satisfied. It fails with `gitlab_conflict` (exit 1) and a hint to inspect the existing merge request. Report `noop` only when the observed state provably equals the requested state.

The draft-note commands apply the same boundary:

- `mr drafts publish <iid> --all` with nothing pending is a **no-op** (exit 0, `published: {all: true, count: 0, noop: true}`): the CLI lists first, and an observed-empty set provably satisfies "all my drafts are published".
- `mr drafts delete <iid> <id>` hitting a 404 is a **verified no-op** (exit 0, `deleted: {id, noop: true}`) only when a successful follow-up list proves the ID is absent; if the list itself fails, the original 404 error stands (exit 1).
- `mr drafts publish <iid> <id>` hitting a 404 is **not** a no-op: publishing a specific missing draft cannot be confirmed as satisfied — it fails as `gitlab_not_found` (exit 1) with a hint to list the pending drafts.
