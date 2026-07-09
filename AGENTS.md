# Agentic Project Documentation

## What Is This Project?

This project is a community GitLab CLI that works through GitLab's personal access token API. It is built mainly for agentic workflows, so the project should prioritize an agentic experience: predictable command behavior, script-friendly output, clear failure modes, and workflows that are easy for coding agents to inspect and automate.

The project includes another binary named `gl-axi`. That binary is intended to be based on the `axi` standard introduced by Kun Chen: https://github.com/kunchenguid/axi.

## Project Stack

- Language: Go
- Module: `github.com/shabashab/community-gitlab-cli`
- CLI framework: Cobra (`github.com/spf13/cobra`)
- GitLab API client: `gitlab.com/gitlab-org/api/client-go/v2`
- TOON encoding: `github.com/toon-format/toon-go` — all axi TOON output goes through `toon.MarshalString` on structs tagged `toon:"..."`; never hand-format TOON
- Table rendering: `github.com/jedib0t/go-pretty/v6`, imported only by `internal/cli/output/table.go` so the library can be swapped in one place
- Task runner: Taskfile v3 (`Taskfile.yml`)
- Primary binary: `gl`, built into `bin/gl`
- Additional binary: `gl-axi`, built into `bin/gl-axi`

## Project Structure

- `cmd/gl/main.go`: `gl` application entry point; calls `cli.Execute()`.
- `cmd/gl-axi/main.go`: `gl-axi` application entry point; calls `cli.ExecuteAxi()`.
- `internal/cli/root.go`: shared Cobra root command definition, CLI initialization, exit-code handling, and the gl-axi home view (`runAxiHome`).
- `internal/cli/errors.go`: `usageError` (exit 2), cobra flag/args error wrapping, `classifyError` code mapping, and GitLab API error translation.
- `internal/cli/mr.go`: the `mr` parent command and its `!<iid>` action dispatch, plus registration of all `mr` subcommands.
- `internal/cli/mr_ref.go`: merge request ref resolution — `parseMergeRequestRef`, the `current` ref (`resolveMergeRequestRef` → `resolveCurrentMergeRequestIID`), and the `currentBranchFunc` test seam.
- `internal/cli/mr_list.go`: `mr list` — list options, `--fields` parsing, `runMRList`, and `fetchMergeRequestList` (shared with the gl-axi home view and `context`).
- `internal/cli/mr_view.go`: `mr view` — view options and `runMRView`.
- `internal/cli/mr_create.go`: `mr create` command — flag set, branch defaulting, username→ID resolution, and `CreateMergeRequestOptions` construction.
- `internal/cli/mr_update.go`: `mr update` command — Changed-gated flag sending, draft/ready title rewriting, and `UpdateMergeRequestOptions` construction.
- `internal/cli/mr_finalize.go`: `mr merge`, `mr close`, and `mr reopen` — merge endpoint options, state-event close/reopen, verified no-op state reads, and finalization output.
- `internal/cli/mr_approvals.go`: `mr approvals`, `mr approve`, and `mr unapprove` — approval status reads, approve-with-optional-SHA, unapprove plus status refresh, and approval command output.
- `internal/cli/mr_discussions.go`: `mr discussions` (thread list), `mr discussion` (single-thread view), and `mr discussion resolve|unresolve` command definitions and run functions.
- `internal/cli/mr_discussions_pipeline.go`: discussion pipeline helpers — fetch-all pagination over the discussions API, discussion short-ID prefix resolution (`resolveDiscussionRef`), and the client-side filter/sort/page pipeline over `output.DiscussionSummary` values (thread summarization itself lives in `internal/cli/output` as `output.SummarizeDiscussion`).
- `internal/cli/mr_comment.go`: `mr comment` command — the flag validation matrix (body/position/draft/reply/note/resolve combinations) and variant dispatch across the discussions, notes, and draft-notes APIs.
- `internal/cli/mr_diff.go`: `mr diff` and `mr diff patch` — changed-file summaries, list filtering/paging, and raw patch output.
- `internal/cli/mr_diff_export.go`: `mr diff export` — the filesystem review-bundle export (`manifest.toon`, `files.toon`, `patch.diff`, `diffs/`, `old/`, `new/`) and its path-safety helpers.
- `internal/diffpos/`: smart diff positioning — a pure unified-diff hunk parser mirroring GitLab's cursor semantics (`ParseFileDiff`), `LineCode` fabrication, and `ResolvePosition` building full GitLab position objects from merge request diff refs and file diffs; its `Err*` sentinels are mapped to error codes by `internal/cli/errors.go`. (`parseLineSpec` for `N`/`A:B` flag values lives in `internal/cli/mr_comment.go`.)
- `internal/cli/mr_drafts.go`: `mr drafts` command suite (list, `publish <id>`/`--all`, `delete`) — fetch-all pagination over the draft-notes API, client-side paging, and verified-noop idempotency for publish-all/delete.
- `internal/cli/flags.go`: `resolveContentFlag`, the shared helper behind the repo-wide `--<thing>` / `--<thing>-file` dual-input convention for content-bearing flags, and `parseExtraFields`, the shared `--fields` validator.
- `internal/cli/output/`: the presentation package — per-resource output structs (tagged `json` + `toon`) and text/json/toon writers split into per-resource files (`format.go`, `common.go`, `error.go`, `user.go`, `auth.go`, `project.go`, `mr.go`, `approvals.go`, `discussions.go`, `diff.go`, `comment.go`, `drafts.go`, `home.go`), plus `table.go`, the only file importing go-pretty. Owns `output.Mode` and `output.UsageError` (aliased by cli).
- `internal/cli/auth.go`: `auth` command suite (`login`, `logout`, `status`) for stored credentials.
- `internal/cli/setup.go`: gl-axi-only `setup hooks` command installing agent session integrations.
- `internal/cli/context.go`: gl-axi-only `context` command printing session-start ambient context; silent (exit 0, no output) on any failure so hooks never spam sessions.
- `internal/agenthooks/`: SessionStart hook installer targeting Claude Code (`~/.claude/settings.json`), Codex (`~/.codex/hooks.json` + `config.toml`), and OpenCode (managed plugin); idempotent, path-repairing, never touches unmanaged config.
- `.agents/skills/gl-axi/SKILL.md`: installable Agent Skill describing gl-axi usage; keep it in sync when the command surface changes.
- `docs/axi-output.md`: detailed gl-axi output contract — per-command shapes, `--fields`, truncation, count lines, help-hint rules; update when changing axi output.
- `docs/errors-and-exit-codes.md`: error model reference — structured error shape, error-code table, exit codes, API error translation; update when adding error codes.
- `docs/agent-integrations.md`: session-integration reference — `setup hooks` per-app behavior, `context` command contract, Agent Skill; update when changing `internal/agenthooks` or the context output.
- `internal/gitlabclient/config.go`: shared GitLab client-go configuration and client construction.
- `internal/repo/discovery.go`: shared git origin discovery and remote URL parsing.
- `internal/credstore/`: hybrid credential store — OS keychain via `github.com/zalando/go-keyring` with an encrypted-file fallback at `~/.gl/credentials.json`; `store.go` is the hybrid entry point, `domain.go` canonicalizes base URLs into credential keys, `crypto.go`/`file.go` implement the encrypted file backend, `keyring.go` wraps the OS keychain.
- `go.mod`: Go module metadata and dependency declarations.
- `go.sum`: Go dependency checksums.
- `Taskfile.yml`: project task definitions for building and running the CLI.
- `bin/`: local build output directory. `task build` writes `bin/gl` and `bin/gl-axi` here.
- `AGENTS.md`: agent-facing project documentation and workflow notes.
- `README.md`: user-facing project overview and command reference.

## Task Commands

The project uses Taskfile v3. Run these commands from the repository root.

### `task build`

Builds both CLI binaries.

What it does:

- Runs `go build -o bin/gl ./cmd/gl`.
- Runs `go build -o bin/gl-axi ./cmd/gl-axi`.
- Marks both built binaries as executable with `chmod +x bin/gl bin/gl-axi`.

When to run it:

- After changing Go source files.
- Before manually testing either compiled CLI.
- Before handing off work that should include locally verified binary builds.

### `task run -- <args>`

Builds and runs the CLI binary with optional arguments.

What it does:

- Runs the `build` task first.
- Executes `bin/gl {{.CLI_ARGS}}`.

When to run it:

- During CLI development when checking command behavior.
- When testing help output or new flags/subcommands.
- When validating an agentic workflow end to end through the actual built binary.

Examples:

```sh
task run -- --help
task run -- <subcommand> <args>
```

### `task install`

Installs both CLI binaries into `GOBIN` (defaults to `~/go/bin`) via `go install ./cmd/gl ./cmd/gl-axi`.

When to run it:

- When you want `gl` and `gl-axi` available on `PATH` outside the repository.
- After changes you want reflected in the locally installed binaries.

### `task test`

Runs Go tests with `go test ./...`.

When to run it:

- After changing Go source files.
- Before handing off behavior that should be covered by automated tests.

### `task client-go-source`

Downloads the official GitLab client-go module if needed and prints its local module-cache metadata, including the exact `Dir` path agents can inspect with `rg`, `sed`, or `go doc`.

When to run it:

- Before adding a command that needs a GitLab API method you have not used before.
- When debugging how `client-go` builds requests, handles pagination, retries, or models an API response.

### `task run-axi -- <args>`

Builds and runs the `gl-axi` binary with optional arguments.

What it does:

- Runs the `build` task first.
- Executes `bin/gl-axi {{.CLI_ARGS}}`.

When to run it:

- During `gl-axi` development when checking command behavior.
- When validating axi-oriented agentic workflows.
- When testing help output or future `gl-axi` flags/subcommands.

Examples:

```sh
task run-axi -- --help
task run-axi -- <subcommand> <args>
```

## Git Worktree Workflow For Agents

Use repository-local worktrees when you need an isolated checkout for parallel agent work, release checks, or branch comparisons.

- Create worktrees under `.worktrees/<name>` from the repository root. Do not create sibling repository directories or worktrees outside the repo unless the user explicitly asks.
- Use short, descriptive names that match the branch or task, for example `.worktrees/mr-drafts-fix`.
- For a new branch, run `git worktree add .worktrees/<name> -b <branch> <base-ref>`. For an existing branch, run `git worktree add .worktrees/<name> <branch>`.
- Run commands from inside the worktree when testing that branch: `cd .worktrees/<name>` and then use the normal `task build`, `task test`, or CLI commands.
- Keep `.worktrees/` ignored. Never commit files from inside `.worktrees/`; commit only the intended source changes from the worktree's own Git checkout.
- Remove a worktree after it is no longer needed with `git worktree remove .worktrees/<name>`, then prune stale metadata with `git worktree prune` if Git reports leftover records.

## GitLab Client-Go Workflow For Agents

All GitLab communication should go through `internal/gitlabclient`, which creates the official `gitlab.com/gitlab-org/api/client-go/v2` client. Do not add parallel hand-written REST callers unless there is a documented client-go gap.

Configuration contract:

- Token precedence: `--gitlab-token`, then `GITLAB_TOKEN`, then `GL_TOKEN`, then the stored credential for the resolved host (see `auth` below).
- Instance URL: use `GITLAB_BASE_URL` or `--gitlab-base-url`; default is `https://gitlab.com`.
- `client-go` accepts either the GitLab root URL or an `/api/v4` URL and normalizes the API path internally.

Stored credentials (`auth` command suite):

- `auth login <token> --gitlab-base-url <url>` verifies the token via CurrentUser, then stores it keyed by the canonical host. `--gitlab-base-url` must be passed explicitly for login; the env var is not accepted there.
- `auth logout` and `auth status` resolve the host through the normal chain (`--gitlab-base-url`, `GITLAB_BASE_URL`, discovered origin, default), so `auth status` answers "would a command run from here find a credential?".
- Storage is hybrid: OS keychain (service `community-gitlab-cli`) when available, otherwise an encrypted JSON file at `~/.gl/credentials.json` (dir `0700`, file `0600`). The file never contains the domain or token in plaintext: entries are located by a salted SHA-256 domain hash and tokens are AES-256-GCM sealed with an Argon2id key derived from the domain, so a credential is only recoverable by a caller that already knows the host. This is obfuscation plus domain-binding against casual file scraping, not brute-force resistance against a targeted attacker.
- Keep all credential persistence inside `internal/credstore`; CLI code composes it in `rootOptions.newGitLabClientWithBaseURLFallback` as the last token source, and lookup failures degrade silently to the missing-token error.

Project-aware commands use `internal/repo` to discover the current project from `remote.origin.url`. Only the remote named `origin` is read by default. Instance URL precedence for project-aware commands is `--gitlab-base-url`, then `GITLAB_BASE_URL`, then the discovered origin host, then `https://gitlab.com`. Use the shared `--project` flag for commands that need an explicit project outside the current repository; it accepts either a numeric GitLab project ID or a full path such as `group/subgroup/project`.

`gl` and `gl-axi` share GitLab API behavior. Keep GitLab API calls in shared command code and branch only at the presentation/ergonomics layer through the root command mode:

- `gl`: default `--output text`; supports `text` and `json`; plain stderr errors.
- `gl-axi`: default `--output toon`; supports `toon` and `json`; compact fields, `help[]` next-step suggestions, and content-first root behavior. All TOON output is encoded through `toon-go` — build a tagged struct and call `output.WriteAxi`; never format TOON by hand.
- Errors and exit codes (axi contract, enforced in `internal/cli/errors.go` + `root.go`): gl-axi errors are structured output (`error`, `code`, `help[]`) on **stdout** in the requested format; exit `0` for success including no-ops (idempotent mutations such as `auth logout` with nothing stored), `1` for runtime errors, `2` for usage errors. Wrap invalid-invocation errors in `newUsageError` so they exit 2; unknown flags automatically list the command's valid flags inline. Raw client-go API errors are translated by `translateGitLabAPIError` — never let request URLs or raw bodies leak into messages.
- Running `gl-axi` with no subcommand shows live data, not help. The home view prints `bin` + `description` first (axi §10), then the open merge requests of the current repo, or `whoami` data outside one.
- Merge request commands live under `mr`: bare `mr` lists open merge requests (content-first in both modes), `mr !<iid>` / `mr <iid>` shows one, `--full` expands the truncated description and adds all fields. The view fetches approval status by default and nests a compact `approval` summary (`approved`, `approvals_required`, `approvals_left`, `user_has_approved`, `user_can_approve`, `approved_by`). The `!<iid>` dispatch happens in the `mr` parent command's `RunE`; extend its action switch when adding per-merge-request actions such as `diff` or `notes`. Extra positional args are rejected (fail loud).
- The literal ref `current` (or `!current`) is accepted wherever an iid is (`mr current`, `mr view current`, `mr update current`, `mr merge current`, `mr close current`, `mr reopen current`) and resolves via `resolveMergeRequestRef` → `resolveCurrentMergeRequestIID`: current git branch (`currentBranchFunc`, a test seam over `repo.CurrentBranch`) → `ListProjectMergeRequests` filtered by `SourceBranch` + `State=opened`. Open merge requests only; failures are loud, exit 1: no branch → `errMissingCurrentBranch` (`missing_current_branch`), zero matches → `errNoCurrentMergeRequest` (`no_current_merge_request`), several matches → `errAmbiguousCurrentMergeRequest` (`ambiguous_current_merge_request`, candidate iids in the message). Hints for the last two are built at the error site with `newHelpError` (a runtime analog of `usageError` that keeps exit 1) so they can embed the branch and carry `--project` forward. Only lowercase `current` is special; anything else falls through to the numeric parser.
- `mr create` posts a new merge request; only `--title` is required (missing title is a usage error, exit 2). `--source-branch` defaults to the current git branch via `repo.CurrentBranch`; `--target-branch` defaults to the target project's default branch via `GetProject` (the `--target-project-id` fork target when set). `--assignee`/`--reviewer` accept usernames (optional `@`, resolved through `Users.ListUsers`) or all-digits numeric IDs. `--draft` prepends `Draft:` to the title client-side without doubling an existing prefix. Boolean API flags (`--squash`, `--remove-source-branch`, `--allow-collaboration`) are sent only when explicitly passed. A GitLab 409 (open merge request already exists for the branch pair) maps to `gitlab_conflict`, exit 1 — not a no-op, because the existing merge request may not match the requested content. Output reuses the compact `merge_request:` view shape plus a next-step `mr view <iid>` hint (`output.WriteMergeRequestCreated`).
- `mr update <!iid|iid|current>` edits an existing merge request. A field is sent iff its flag was explicitly passed (`cmd.Flags().Changed` on every flag, not just booleans), so explicitly empty values clear: `--description ""`, `--assignee ""`, `--reviewer ""`, `--label ""`, `--milestone-id 0`; `--title ""`/`--target-branch ""` are usage errors (not clearable). No field flags at all is a usage error (`errNoUpdateFlags` → `no_update_flags`, exit 2) — fail loud, not a silent no-op. `--label` (replace) is mutually exclusive with `--add-label`/`--remove-label` (which combine). `--draft`/`--ready` are mutually exclusive and rewrite the title client-side (`applyDraftTitle`/`stripDraftTitle`); without `--title` the current title is fetched via `GetMergeRequest` first. Idempotent prefix no-ops (`--ready` on a non-draft) still send the PUT and exit 0. Source branch and cross-fork target project are immutable — the update API has no such fields. Output reuses `output.WriteMergeRequestCreated`. The bare `mr !<iid> update` parent-dispatch form returns a usage error redirecting to the subcommand (flags cannot parse on the parent).
- `mr merge <!iid|iid|current>` finalizes through `MergeRequests.AcceptMergeRequest`; `--sha` is an optional optimistic head check and cannot be empty, `--auto-merge`, `--squash`, and `--remove-source-branch` are sent only when explicitly passed, and commit messages use the content-flag convention (`--merge-commit-message[-file]`, `--squash-commit-message[-file]`). `mr close`/`mr reopen` use `UpdateMergeRequestOptions.StateEvent`. Already merged/closed/open states are verified by `GetMergeRequest` and reported as `noop: true`, exit 0; incompatible states surface GitLab errors. Output is `merge_request` + `action` + optional `noop` through `output.WriteMergeRequestAction`. The bare parent-dispatch forms `mr !<iid> merge|close|reopen` redirect to the subcommands like `update` does.
- `mr approvals <!iid|iid|current>` reads merge request approval status through `MergeRequests.GetMergeRequestApprovals`; `mr <iid> approvals` and alias `mr approval` are accepted. Compact output is `approval:` with approval readiness and approvers; `--full` adds suggested approvers, configured approvers/groups, rules left, and availability/configuration booleans. `mr approve <!iid|iid|current> [--sha <sha>]` calls `MergeRequestApprovals.ApproveMergeRequest` and returns compact approval status plus a `mr view <iid>` hint; empty `--sha` is a usage error. `mr unapprove <!iid|iid|current>` calls `UnapproveMergeRequest`, then refreshes with `GetMergeRequestApprovals` because the API returns no body. The bare parent-dispatch forms `mr !<iid> approve|unapprove` redirect to the subcommands like `update` does.
- `mr discussions <!iid|iid|current>` lists a merge request's discussion threads; `mr discussion <!iid|iid|current> <discussion-id>` prints one thread's full conversation (complete note bodies, no hints — the view is self-contained). The GitLab discussions API has **no server-side filters or sorting** (`ListMergeRequestDiscussionsOptions` embeds only `ListOptions`), so `fetchAllMergeRequestDiscussions` pages through everything (`per_page=100`, follow `resp.NextPage`) and filtering (`--state all|resolved|unresolved` default **unresolved**, `--author` = thread starter, `--system` opt-in for system threads), sorting (`--order-by created_at|updated_at`, `--sort`), and paging (`--limit`/`--page`) run client-side — totals in `count: N of M total` are always exact. Thread state semantics: resolvable = any note `Resolvable`; resolved = all resolvable notes `Resolved`; non-resolvable threads are state `none` and match only `--state all`; a thread's `updated_at` is the max note `UpdatedAt` (fallback `CreatedAt`). Discussion IDs are 40-char hex; lists show the lowercase 8-char prefix (`output.ShortDiscussionID`) and `resolveDiscussionRef` accepts a full ID (direct `GetMergeRequestDiscussion`) or any unique prefix (list + match; ambiguous → `ambiguous_discussion_ref` exit 2, no match → `discussion_not_found` exit 1, non-hex → `invalid_discussion_ref` exit 2). The parent-dispatch forms `mr !<iid> discussions|discussion|threads` redirect to the subcommands like `update` does.
- `mr diff <!iid|iid|current>` lists changed files in a compact shape (`path,status,additions,deletions,hunks`; `--fields old_path,generated,collapsed,too_large,new_ranges,old_ranges`; `--file` accepts old or new path). It fetches `GetMergeRequest` for diff refs, then paginates `ListMergeRequestDiffs`; `--limit`/`--page` are client-side and totals are exact. `mr diff patch <ref>` streams raw unified diff bytes from `ShowMergeRequestRawDiffs` and intentionally bypasses structured output. `mr diff export <ref> --dir <path>` creates an agent review bundle (`manifest.toon`, `files.toon`, `patch.diff`, `diffs/`, `old/`, `new/`) pinned to `base_sha`/`head_sha`; repository paths must stay inside the bundle (`unsafe_export_path`), and non-empty dirs require `--force` (`export_dir_not_empty`).
- `mr comment <!iid|iid|current>` creates review comments; the flag combination picks the API. Default → `Discussions.CreateMergeRequestDiscussion` (resolvable thread); `--note` → `Notes.CreateMergeRequestNote` (plain note; excludes `--draft` and position flags); `--draft` → `DraftNotes.CreateDraftNote` (pending review note); `--reply-to <discussion-ref>` (unique prefix ok, resolved via `resolveDiscussionRef`; excludes position flags and `--note`) → `AddMergeRequestDiscussionNote`, or a draft reply through `InReplyToDiscussionID`; `--resolve` (requires `--draft --reply-to`) sets `ResolveDiscussion`, applied by GitLab at publish time. Body comes from `resolveContentFlag` (`--body`/`--body-file`) and is required. All combination violations are usage errors (exit 2) checked before any API call.
- Positioning (`mr comment --file` [+ `--line`/`--old-line`, single `N` or range `A:B`; the two line flags are mutually exclusive and require `--file`]) goes through `diffpos.ResolvePosition` in `internal/diffpos`: `GetMergeRequest` → `DiffRefs` (empty SHAs → `merge_request_diff_not_ready`, exit 1, retry hint), `diffpos.FetchAllDiffs` (paginated `ListMergeRequestDiffs`), file match by new or old path (`file_not_in_diff` with a changed-paths hint), then a pure hunk parse that mirrors GitLab's diff cursors exactly: added lines send `new_line` only, removed lines `old_line` only, context lines **both** (numbers may differ). `--file` alone is a `position_type=file` comment. Ranges resolve both endpoints and fabricate `line_code = sha1(path)_old_new` per endpoint (never guess cursors — GitLab validates them against its own parse). A line not visible in the diff is `line_not_in_diff` (exit 1) with the commentable ranges and a cross-side suggestion; collapsed/too-large diffs are `diff_too_large`. GitLab can answer 201 yet silently drop the position — the writers surface that as a verify hint (response `type` ≠ `DiffNote`), never as an error, so agents do not retry into duplicates.
- `mr drafts <!iid|iid|current>` lists your pending draft notes (`fetchAllDraftNotes`, `per_page=100`; no API filters, so `--limit`/`--page` run client-side; axi rows `id,file,line,preview` + `--fields discussion_id,resolve_discussion`). `mr drafts publish <ref> <id>` publishes one (`PublishDraftNote`, PUT); `--all` lists first and calls `PublishAllDraftNotes` only when drafts exist — an observed-empty set is a verified no-op (exit 0, `noop: true`), while publishing a specific missing ID stays `gitlab_not_found` (exit 1) with a drafts-list hint (`newHelpError` at the call site; `classifyError`'s 404 branch honors site hints via `helpFromError`). `mr drafts delete <ref> <id>` treats a 404 as a no-op only when a follow-up list proves the ID absent. Draft IDs are numeric (`invalid_draft_note_id`, exit 2). The parent-dispatch forms `mr !<iid> comment|drafts` redirect to the subcommands like `update` does.
- **Content flags convention:** every flag that accepts long-form text comes as a pair — `--<thing>` for inline text and `--<thing>-file <path>` for file input, where `-` reads stdin. The pair is mutually exclusive (usage error, exit 2); file read failures are runtime errors (exit 1); content is passed through verbatim, without trimming. Implement new content inputs through `resolveContentFlag` in `internal/cli/flags.go`; never invent a different dual-input shape (`mr create --description`/`--description-file` is the reference).
- Token-frugal defaults for agents: axi list rows are 4 columns (`iid,title,state,author`) with a `--fields` escape hatch for extra columns and a definitive `count: N of M total` line; merge request view is a compact field set with the description truncated at 500 runes and an explicit size marker, and the `--full` hint appears only when something was actually truncated. Help hints must be real runnable commands, parameterize dynamic values (`<iid>`), and carry an explicit `--project` forward. Keep new axi output narrow by default and put escape hatches behind flags like `--full`.
- Ambient context: `gl-axi setup hooks` installs SessionStart integrations via `internal/agenthooks`; the hook command is `gl-axi context`, which must stay silent (exit 0, no output) whenever it cannot produce useful context.

To inspect the actual upstream implementation:

```sh
task client-go-source
```

Use the printed `Dir` value to inspect source directly, for example:

```sh
rg "func NewClient|func WithBaseURL|type Client struct" <Dir>
rg "type UsersServiceInterface|func .*CurrentUser" <Dir>/users.go
```

Helpful starting points inside the client-go source:

- `gitlab.go`: client construction, retry behavior, base URL normalization, request execution.
- `client_options.go`: `WithBaseURL`, `WithUserAgent`, HTTP client options, retry options.
- `<resource>.go`: service interfaces, request option structs, and API methods for a GitLab resource.
- `request_options.go`: per-request options such as `gitlab.WithContext`.
- `testing/`: generated service mocks from the upstream project when a future command needs interface-driven tests.

## Testing Guide For Agents

Run tests with `task test` (`go test ./...`). Tests live next to their package (`internal/cli/*_test.go`, `internal/repo/discovery_test.go`). There are no interface mocks and no golden files — read this section before writing a test so you match the house style on the first attempt.

### CLI tests stub GitLab with httptest

Every GitLab interaction is tested against a real HTTP server:

- Use `httptest.NewServer(http.HandlerFunc(...))` with a **single handler and a `switch` on `r.URL.EscapedPath()`**. Do not use `http.ServeMux` route patterns: project paths are URL-encoded (`projects/group%2Fproject`), and mux matching operates on the unescaped path, so patterns will not behave as expected. `EscapedPath()` keeps the `%2F`.
- Assert requests via `r.URL.EscapedPath()`, `r.URL.Query()`, `r.Header.Get("Private-Token")`, and `r.Method`.
- client-go sends **JSON** request bodies. Decode with `json.NewDecoder(r.Body).Decode(&map[string]any{})`. Numbers decode as `float64`; `gitlab.LabelOptions` marshals to a single comma-joined string (`"bug,backend"`), not an array.
- Reuse the fixtures in `helpers_test.go`: `testMergeRequest(iid, description)` builds a `*gitlab.MergeRequest`, `mergeRequestJSON(iid, description)` the matching canned response.

### Two invocation levels — pick deliberately

1. **Direct run-function calls** (`runMRView(cmd, &rootOptions{...}, &projectOptions{...}, ...)`) with a bare `&cobra.Command{}` and a hand-filled options struct. Fast and focused, but a bare command has **no registered flags**: anything using `cmd.Flags().Changed(...)` (set-tracking bools, `resolveContentFlag`) silently sees "not set". Use this level only when the run function does not inspect flag state.
2. **Full root-command execution**: `cmd, _ := newRootCommand("gl", "test", "test", commandModeStandard)`, then `cmd.SetOut(&buf)`, `cmd.SetErr(&bytes.Buffer{})`, `cmd.SetArgs([]string{"mr", ..., "--gitlab-token", "test-token", "--gitlab-base-url", server.URL, "--project", "group/project", "-o", "json"})`, then `cmd.Execute()`. Required for flag parsing, `Changed` detection, mutual exclusion, usage errors, and stdin (`cmd.SetIn(strings.NewReader(...))`).

Helper gotchas learned the hard way:

- `executeMRRootCommand` (helpers_test.go) **fatals on error** — for error-path tests write a variant that returns `(string, error)`.
- Capture output **after** executing: `err := cmd.Execute(); return out.String(), err`. Writing `return out.String(), cmd.Execute()` evaluates the buffer before the command runs and returns an empty string.
- Always pass `--project` and explicit branch flags in CLI tests: the test process runs inside this repository, so code paths that shell out to git (origin discovery, current-branch defaulting) would otherwise read the real repo and make the test environment-dependent.
- Tests exercising the `current` ref must stub the branch lookup through the `currentBranchFunc` seam via `stubCurrentBranch(t, branch, err)` (helpers_test.go), which restores the real `repo.CurrentBranch` in `t.Cleanup` — never let a cli test shell out to git for the current branch.

### Assertions

- Output assertions are **substring checks** (`strings.Contains`) on TOON/JSON fragments — no golden files. Examples of house fragments: `merge_requests[2]{iid,title,state,author}:`, `"iid": 123`, `count: 2 of 57 total`.
- TOON quotes strings containing special characters: URLs and timestamps render as `web_url: "https://..."` — include the quotes in the expected fragment or the assertion fails.
- Usage errors: assert `errors.Is(err, <sentinel>)` and `exitCodeForError(err) == 2` (runtime errors: `== 1`).
- Error codes: render with `writeCommandError(&buf, commandModeAxi, "toon", "gl-axi", err)` and assert `code: <expected_code>`; also assert the absence of leaks (`strings.Contains(got, server.URL)` must be false).
- Writers are unit-testable directly: call `output.WriteMergeRequest(&buf, "toon", commandModeAxi, ...)` etc. without any server.

### internal/repo tests use real git

- Guard with `if _, err := exec.LookPath("git"); err != nil { t.Skip(...) }`.
- Build repos in `t.TempDir()` via the `runGit(t, dir, args...)` helper (prepends `-C dir`).
- Commits need an identity; pass it inline instead of touching global config: `runGit(t, dir, "-c", "user.name=test", "-c", "user.email=test@example.com", "commit", "--allow-empty", "-m", "init")`.
- Detached HEAD state: `runGit(t, dir, "checkout", "--detach")` (requires at least one commit).

## Commit Message Policy

Use Conventional Commits for every commit. The required format is:

```text
<type>(optional-scope)!: <description>
```

Allowed commit types are strict:

- `feat`: user-visible feature or new capability.
- `fix`: bug fix or incorrect behavior correction.
- `docs`: documentation-only change.
- `style`: formatting-only change with no behavior impact.
- `refactor`: code restructuring that does not add features or fix bugs.
- `perf`: performance improvement.
- `test`: adding or changing tests only.
- `build`: build system, dependency, module, or packaging change.
- `ci`: continuous integration or automation workflow change.
- `chore`: maintenance task that does not fit another type.
- `revert`: revert of a previous commit.

Do not use vague or non-standard types such as `update`, `change`, `misc`, `wip`, `cleanup`, or `improve`. Use a scope when it adds useful routing context, for example `feat(cli): add project list command`, `fix(gitlabclient): validate missing token`, or `docs(agents): document commit policy`.

Use `!` after the type or scope for breaking changes, and include a `BREAKING CHANGE:` footer when the impact needs explanation:

```text
feat(cli)!: rename output flag

BREAKING CHANGE: --format replaces --output for all commands.
```

Keep the description imperative, concise, and specific. Prefer one logical change per commit.

## Notes For Agents

- Keep generated binaries and transient local outputs out of source changes unless explicitly requested.
- Prefer adding task commands to `Taskfile.yml` when a workflow becomes repeated or important for agentic development.
- Run `task test` after changing Go source files.
- Preserve scriptability when adding CLI behavior: stable flags, clear stdout/stderr separation, useful exit codes, and machine-readable output where appropriate.
