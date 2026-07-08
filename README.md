# Community GitLab CLI

Community GitLab CLI is a GitLab command-line tool that works through GitLab's personal access token API. It is built mainly for agentic workflows, so the project focuses on an agentic experience: predictable command behavior, script-friendly output, clear failure modes, and workflows that are easy for coding agents to inspect and automate.

The project also includes another binary, `gl-axi`, based on the `axi` standard introduced by Kun Chen: https://github.com/kunchenguid/axi.

## Documentation

Detailed reference pages live under `docs/`:

- [gl-axi Output Reference](docs/axi-output.md) — TOON format, per-command output shapes, `--fields`, truncation, counts, and help-hint rules.
- [Errors and Exit Codes](docs/errors-and-exit-codes.md) — the structured error shape, error-code table, exit codes, and GitLab API error translation.
- [Agent Session Integrations](docs/agent-integrations.md) — `setup hooks`, the `context` command contract, and the installable Agent Skill.

## Project Stack

- Language: Go
- Module: `github.com/shabashab/community-gitlab-cli`
- CLI framework: Cobra (`github.com/spf13/cobra`)
- GitLab API client: `gitlab.com/gitlab-org/api/client-go/v2`
- TOON encoding: `github.com/toon-format/toon-go`
- Task runner: Taskfile v3 (`Taskfile.yml`)
- Primary binary: `gl`, built into `bin/gl`
- Additional binary: `gl-axi`, built into `bin/gl-axi`

## Project Structure

- `cmd/gl/main.go`: `gl` application entry point; calls `cli.Execute()`.
- `cmd/gl-axi/main.go`: `gl-axi` application entry point; calls `cli.ExecuteAxi()`.
- `internal/cli/root.go`: shared Cobra root command definition, CLI initialization, and the gl-axi home view.
- `internal/cli/errors.go`: usage-error classification, exit codes, flag-error rendering, and GitLab API error translation.
- `internal/cli/setup.go` / `internal/cli/context.go`: gl-axi session-integration setup and the ambient-context command it installs.
- `internal/agenthooks/`: SessionStart hook installer for Claude Code, Codex, and OpenCode.
- `internal/gitlabclient/config.go`: shared GitLab client-go configuration and client construction.
- `internal/repo/discovery.go`: shared git origin discovery and remote URL parsing.
- `internal/credstore/`: persistent credential storage (OS keychain with an encrypted-file fallback).
- `.agents/skills/gl-axi/SKILL.md`: installable Agent Skill describing gl-axi usage for agents.
- `docs/`: detailed reference pages (axi output, errors and exit codes, agent integrations).
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

## GitLab Configuration

Commands that call GitLab use the official `gitlab.com/gitlab-org/api/client-go/v2` package through the shared configuration in `internal/gitlabclient`.

- Token precedence: `--gitlab-token`, then `GITLAB_TOKEN`, then `GL_TOKEN`, then the credential stored by `gl auth login` for the resolved host.
- Instance URL: set `GITLAB_BASE_URL` or pass `--gitlab-base-url`; defaults to `https://gitlab.com`.
- `gl` output: pass `--output text` or `--output json`; default is `text`.
- `gl-axi` output: pass `--output toon` or `--output json`; default is `toon`.

Project-aware commands can discover the current GitLab project from the local git repository by reading only `remote.origin.url`. The discovered origin supplies both the project path and, when no explicit instance URL is configured, the GitLab host. Instance URL precedence is:

1. `--gitlab-base-url`
2. `GITLAB_BASE_URL`
3. discovered `origin` host
4. `https://gitlab.com`

Pass `--project` to select a project explicitly when running outside that project's directory. It accepts either a numeric GitLab project ID or a full path such as `group/subgroup/project`.

`gl-axi` uses the same GitLab client and command behavior as `gl`, but changes presentation for agent ergonomics: spec-compliant [TOON](https://toonformat.dev/) output (encoded with `github.com/toon-format/toon-go`), minimal default schemas, `help[]` next-step suggestions, and structured errors with machine-readable codes. Running `gl-axi` with no subcommand prints a home view that identifies the tool (`bin`, `description`) followed by live content: the open merge requests of the current GitLab repository, or the authenticated user outside a repository.

Errors and exit codes follow the axi contract:

- `gl-axi` errors are structured output on **stdout** in the requested format (`error`, `code`, `help[]`); `gl` keeps plain errors on stderr.
- Exit codes: `0` success (including no-ops such as logging out with nothing stored), `1` runtime error, `2` usage error (unknown flag/command, bad arguments, unsupported `--output`).
- Unknown flags fail loud and list the command's valid flags inline, so an agent can self-correct in one turn.
- Raw GitLab API errors are translated into short actionable messages (`gitlab_auth_failed`, `gitlab_not_found`, `gitlab_rate_limited`, ...) without leaking request URLs.

Example:

```sh
GITLAB_TOKEN=... task run -- whoami --output json
GITLAB_TOKEN=... task run-axi -- whoami
GITLAB_TOKEN=... task run -- project info
GITLAB_TOKEN=... task run -- project info --project group/subgroup/project --output json
GITLAB_TOKEN=... task run-axi -- project info --project 12345
```

## Authentication

The `auth` command stores a personal access token per GitLab host so later commands work without `GITLAB_TOKEN`.

```sh
gl auth login glpat-... --gitlab-base-url https://gitlab.com   # verify and store a token
gl auth status                                                 # is a credential stored for this host?
gl auth logout                                                 # remove the stored credential
```

- `auth login` requires an explicit `--gitlab-base-url` and verifies the token against the instance (`/user`) before storing anything. Passing the token as an argument may leave it in shell history; prefer substituting it from a password manager.
- `auth logout` and `auth status` resolve the host like every other command: `--gitlab-base-url`, then `GITLAB_BASE_URL`, then the discovered git origin, then `https://gitlab.com`.
- `auth logout` is idempotent: with no stored credential it acknowledges the no-op and exits 0.
- Credentials are stored in the OS keychain (macOS Keychain, Windows Credential Manager, Linux Secret Service) when available. On headless systems the fallback is an encrypted file at `~/.gl/credentials.json` (`0700` directory, `0600` file) that contains neither the host nor the token in plaintext: hosts are stored as salted hashes and tokens are AES-256-GCM encrypted with a key derived from the host via Argon2id. This protects against opportunistic file scraping; it is not a defense against a targeted attacker who can guess common GitLab hostnames.
- Explicit `--gitlab-token` or environment tokens always take precedence over stored credentials.

## Merge Requests

The `mr` command works with merge requests in the current project (or `--project`).

Reference a specific merge request as `!<iid>` or plain `<iid>`. In bash and zsh, quote the bang form (`'!123'`) to avoid shell history expansion; plain `123` always works unquoted.

```sh
gl mr                          # list open merge requests (same as gl mr list)
gl mr list --state all --author octocat --search "search endpoint"
gl mr list --label backend --label search --target-branch main --draft=false
gl mr list --order-by updated_at --sort asc --limit 50 --page 2
gl mr '!123'                   # show one merge request (compact fields)
gl mr 123 --full               # all fields and the complete description
gl mr view '!123'              # explicit subcommand form, same behavior
```

- `gl mr list` renders a table; `--output json` returns `{merge_requests, count, total, page, total_pages}`.
- Merge request descriptions are truncated by default with an explicit size marker; pass `--full` for the complete body and all fields. The axi view suggests `--full` only when something was actually truncated.
- `gl-axi mr` prints compact TOON rows (`iid,title,state,author` by default) with a definitive `count: N of M total` line and `help[]` hints for the next step (view command, next page when one exists, filter relaxation on empty results). Hints carry an explicit `--project` forward when one was passed.
- `gl-axi mr list --fields draft,source_branch,target_branch,updated_at,web_url` adds columns to the compact schema; unknown field names are rejected with the valid set inline.
- List filters: `--state`, `--search`, `--label`, `--author`, `--reviewer`, `--source-branch`, `--target-branch`, `--draft`, `--milestone`, `--order-by`, `--sort`, `--limit`, `--page`.

### Creating merge requests

```sh
gl mr create --title "Add search endpoint"                     # source = current branch, target = default branch
gl mr create --title "Fix auth" --draft --label bug --assignee mona --reviewer @alice
gl mr create --title "Docs" --description-file notes.md --target-branch main
git log -1 --format=%b | gl mr create --title "From commit" --description-file -
```

- `--title` is the only required flag. `--source-branch` defaults to the currently checked out git branch; `--target-branch` defaults to the project's default branch. Passing either flag skips the corresponding lookup.
- The description is dual-input: `--description <text>` for inline content, `--description-file <path>` to read a file, `--description-file -` to read stdin. Passing both flags is a usage error. This `--<thing>` / `--<thing>-file` pair is the repo-wide convention for all content-bearing flags.
- `--assignee` and `--reviewer` are repeatable and accept a username (optional `@` prefix, resolved through the GitLab users API) or an all-digits numeric user ID.
- `--draft` prefixes the title with `Draft:` (GitLab derives draft status from the title); an existing `draft:` prefix is left untouched.
- Remaining creation parameters map directly to the API: `--label` (repeatable), `--milestone-id`, `--target-project-id` for cross-fork merge requests, `--remove-source-branch`, `--squash`, `--allow-collaboration`. Boolean flags are sent only when explicitly passed.
- A 409 from GitLab (an open merge request already exists for the branch pair) fails with `gitlab_conflict` and a hint to inspect the existing one.

### Updating merge requests

```sh
gl mr update 123 --title "Rework search endpoint" --ready
gl mr update '!123' --add-label backend --remove-label triage --assignee mona
gl mr update 123 --description ""              # clear the description
gl mr update 123 --squash=false                # explicitly disable squash
```

- A field is sent to GitLab only when its flag is passed — unset fields keep their current values. Running `mr update` with no field flags is a usage error (exit 2), not a silent no-op.
- Explicitly empty values clear: `--description ""`, `--assignee ""`, `--reviewer ""`, `--label ""`, and `--milestone-id 0` clear their fields. `--title` and `--target-branch` cannot be cleared; empty values are usage errors.
- `--label` replaces the full label set; `--add-label`/`--remove-label` adjust it incrementally and may combine. Mixing `--label` with the incremental flags is a usage error.
- `--draft` and `--ready` (mutually exclusive) toggle the `Draft:` title prefix. Without `--title`, the current title is fetched first so the prefix change applies to the real title. Marking an already-ready merge request `--ready` succeeds (exit 0).
- Boolean flags (`--squash`, `--remove-source-branch`, `--allow-collaboration`, `--discussion-locked`) are sent only when explicitly passed; the `=false` form sends an explicit disable.
- The source branch and cross-fork target project cannot be changed after creation.

## Agent Session Integrations (gl-axi)

`gl-axi setup hooks` installs SessionStart integrations so agent sessions start with ambient GitLab context — the open merge requests of the repository the session starts in:

- Claude Code: `~/.claude/settings.json` SessionStart hook
- Codex: `~/.codex/hooks.json` plus `hooks = true` under `[features]` in `~/.codex/config.toml`
- OpenCode: a managed plugin in `~/.config/opencode/plugins/`

The installed hook runs `gl-axi context`, which prints a compact merge request digest and stays completely silent (exit 0, no output) outside GitLab repositories, without credentials, or when GitLab is unreachable. Repeated `setup hooks` runs are no-ops; a moved or reinstalled binary path is repaired automatically; unmanaged user configuration is never touched.

As a lower-overhead alternative, an installable Agent Skill lives at `.agents/skills/gl-axi/SKILL.md` (`npx skills add shabashab/community-gitlab-cli --skill gl-axi`). It loads on demand instead of every session. The hook and the skill are complementary — one of them is enough.

## Development Notes

- Keep generated binaries and transient local outputs out of source changes unless explicitly requested.
- Prefer adding task commands to `Taskfile.yml` when a workflow becomes repeated or important for agentic development.
- Run `task test` after changing Go source files.
- Preserve scriptability when adding CLI behavior: stable flags, clear stdout/stderr separation, useful exit codes, and machine-readable output where appropriate.
