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
- Table rendering: `github.com/jedib0t/go-pretty/v6`, imported only by `internal/cli/mr_table.go` so the library can be swapped in one place
- Task runner: Taskfile v3 (`Taskfile.yml`)
- Primary binary: `gl`, built into `bin/gl`
- Additional binary: `gl-axi`, built into `bin/gl-axi`

## Project Structure

- `cmd/gl/main.go`: `gl` application entry point; calls `cli.Execute()`.
- `cmd/gl-axi/main.go`: `gl-axi` application entry point; calls `cli.ExecuteAxi()`.
- `internal/cli/root.go`: shared Cobra root command definition, CLI initialization, exit-code handling, and the gl-axi home view (`runAxiHome`).
- `internal/cli/errors.go`: `usageError` (exit 2), cobra flag/args error wrapping, `classifyError` code mapping, and GitLab API error translation.
- `internal/cli/mr.go`: `mr` command suite (`list`, `view`, `create`, `update`, and `!<iid>` dispatch), ref resolution including the `current` ref (`resolveMergeRequestRef`), `--fields` parsing, and merge request run functions.
- `internal/cli/mr_create.go`: `mr create` command — flag set, branch defaulting, username→ID resolution, and `CreateMergeRequestOptions` construction.
- `internal/cli/mr_update.go`: `mr update` command — Changed-gated flag sending, draft/ready title rewriting, and `UpdateMergeRequestOptions` construction.
- `internal/cli/flags.go`: `resolveContentFlag`, the shared helper behind the repo-wide `--<thing>` / `--<thing>-file` dual-input convention for content-bearing flags.
- `internal/cli/mr_table.go`: standard-mode merge request table rendering; the only file importing go-pretty.
- `internal/cli/output.go`: per-resource output structs (tagged `json` + `toon`) and text/json/toon writers, plus structured error rendering.
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
- `gl-axi`: default `--output toon`; supports `toon` and `json`; compact fields, `help[]` next-step suggestions, and content-first root behavior. All TOON output is encoded through `toon-go` — build a tagged struct and call `writeAxi`; never format TOON by hand.
- Errors and exit codes (axi contract, enforced in `internal/cli/errors.go` + `root.go`): gl-axi errors are structured output (`error`, `code`, `help[]`) on **stdout** in the requested format; exit `0` for success including no-ops (idempotent mutations such as `auth logout` with nothing stored), `1` for runtime errors, `2` for usage errors. Wrap invalid-invocation errors in `newUsageError` so they exit 2; unknown flags automatically list the command's valid flags inline. Raw client-go API errors are translated by `translateGitLabAPIError` — never let request URLs or raw bodies leak into messages.
- Running `gl-axi` with no subcommand shows live data, not help. The home view prints `bin` + `description` first (axi §10), then the open merge requests of the current repo, or `whoami` data outside one.
- Merge request commands live under `mr`: bare `mr` lists open merge requests (content-first in both modes), `mr !<iid>` / `mr <iid>` shows one, `--full` expands the truncated description and adds all fields. The `!<iid>` dispatch happens in the `mr` parent command's `RunE`; extend its action switch when adding per-merge-request actions such as `diff` or `notes`. Extra positional args are rejected (fail loud).
- The literal ref `current` (or `!current`) is accepted wherever an iid is (`mr current`, `mr view current`, `mr update current`) and resolves via `resolveMergeRequestRef` → `resolveCurrentMergeRequestIID`: current git branch (`currentBranchFunc`, a test seam over `repo.CurrentBranch`) → `ListProjectMergeRequests` filtered by `SourceBranch` + `State=opened`. Open merge requests only; failures are loud, exit 1: no branch → `errMissingCurrentBranch` (`missing_current_branch`), zero matches → `errNoCurrentMergeRequest` (`no_current_merge_request`), several matches → `errAmbiguousCurrentMergeRequest` (`ambiguous_current_merge_request`, candidate iids in the message). Hints for the last two are built at the error site with `newHelpError` (a runtime analog of `usageError` that keeps exit 1) so they can embed the branch and carry `--project` forward. Only lowercase `current` is special; anything else falls through to the numeric parser.
- `mr create` posts a new merge request; only `--title` is required (missing title is a usage error, exit 2). `--source-branch` defaults to the current git branch via `repo.CurrentBranch`; `--target-branch` defaults to the target project's default branch via `GetProject` (the `--target-project-id` fork target when set). `--assignee`/`--reviewer` accept usernames (optional `@`, resolved through `Users.ListUsers`) or all-digits numeric IDs. `--draft` prepends `Draft:` to the title client-side without doubling an existing prefix. Boolean API flags (`--squash`, `--remove-source-branch`, `--allow-collaboration`) are sent only when explicitly passed. A GitLab 409 (open merge request already exists for the branch pair) maps to `gitlab_conflict`, exit 1 — not a no-op, because the existing merge request may not match the requested content. Output reuses the compact `merge_request:` view shape plus a next-step `mr view <iid>` hint (`writeMergeRequestCreated`).
- `mr update <!iid|iid|current>` edits an existing merge request. A field is sent iff its flag was explicitly passed (`cmd.Flags().Changed` on every flag, not just booleans), so explicitly empty values clear: `--description ""`, `--assignee ""`, `--reviewer ""`, `--label ""`, `--milestone-id 0`; `--title ""`/`--target-branch ""` are usage errors (not clearable). No field flags at all is a usage error (`errNoUpdateFlags` → `no_update_flags`, exit 2) — fail loud, not a silent no-op. `--label` (replace) is mutually exclusive with `--add-label`/`--remove-label` (which combine). `--draft`/`--ready` are mutually exclusive and rewrite the title client-side (`applyDraftTitle`/`stripDraftTitle`); without `--title` the current title is fetched via `GetMergeRequest` first. Idempotent prefix no-ops (`--ready` on a non-draft) still send the PUT and exit 0. Source branch and cross-fork target project are immutable — the update API has no such fields. Output reuses `writeMergeRequestCreated`. The bare `mr !<iid> update` parent-dispatch form returns a usage error redirecting to the subcommand (flags cannot parse on the parent).
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
- Reuse the fixtures in `mr_test.go`: `testMergeRequest(iid, description)` builds a `*gitlab.MergeRequest`, `mergeRequestJSON(iid, description)` the matching canned response.

### Two invocation levels — pick deliberately

1. **Direct run-function calls** (`runMRView(cmd, &rootOptions{...}, &projectOptions{...}, ...)`) with a bare `&cobra.Command{}` and a hand-filled options struct. Fast and focused, but a bare command has **no registered flags**: anything using `cmd.Flags().Changed(...)` (set-tracking bools, `resolveContentFlag`) silently sees "not set". Use this level only when the run function does not inspect flag state.
2. **Full root-command execution**: `cmd, _ := newRootCommand("gl", "test", "test", commandModeStandard)`, then `cmd.SetOut(&buf)`, `cmd.SetErr(&bytes.Buffer{})`, `cmd.SetArgs([]string{"mr", ..., "--gitlab-token", "test-token", "--gitlab-base-url", server.URL, "--project", "group/project", "-o", "json"})`, then `cmd.Execute()`. Required for flag parsing, `Changed` detection, mutual exclusion, usage errors, and stdin (`cmd.SetIn(strings.NewReader(...))`).

Helper gotchas learned the hard way:

- `executeMRRootCommand` (mr_test.go) **fatals on error** — for error-path tests write a variant that returns `(string, error)`.
- Capture output **after** executing: `err := cmd.Execute(); return out.String(), err`. Writing `return out.String(), cmd.Execute()` evaluates the buffer before the command runs and returns an empty string.
- Always pass `--project` and explicit branch flags in CLI tests: the test process runs inside this repository, so code paths that shell out to git (origin discovery, current-branch defaulting) would otherwise read the real repo and make the test environment-dependent.
- Tests exercising the `current` ref must stub the branch lookup through the `currentBranchFunc` seam via `stubCurrentBranch(t, branch, err)` (mr_test.go), which restores the real `repo.CurrentBranch` in `t.Cleanup` — never let a cli test shell out to git for the current branch.

### Assertions

- Output assertions are **substring checks** (`strings.Contains`) on TOON/JSON fragments — no golden files. Examples of house fragments: `merge_requests[2]{iid,title,state,author}:`, `"iid": 123`, `count: 2 of 57 total`.
- TOON quotes strings containing special characters: URLs and timestamps render as `web_url: "https://..."` — include the quotes in the expected fragment or the assertion fails.
- Usage errors: assert `errors.Is(err, <sentinel>)` and `exitCodeForError(err) == 2` (runtime errors: `== 1`).
- Error codes: render with `writeCommandError(&buf, commandModeAxi, "toon", "gl-axi", err)` and assert `code: <expected_code>`; also assert the absence of leaks (`strings.Contains(got, server.URL)` must be false).
- Writers are unit-testable directly: call `writeMergeRequest(&buf, "toon", commandModeAxi, ...)` etc. without any server.

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
