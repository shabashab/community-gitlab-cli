# Agentic Project Documentation

## What Is This Project?

This project is a community GitLab CLI that works through GitLab's personal access token API. It is built mainly for agentic workflows, so the project should prioritize an agentic experience: predictable command behavior, script-friendly output, clear failure modes, and workflows that are easy for coding agents to inspect and automate.

The project includes another binary named `gl-axi`. That binary is intended to be based on the `axi` standard introduced by Kun Chen: https://github.com/kunchenguid/axi.

## Project Stack

- Language: Go
- Module: `github.com/shabashab/community-gitlab-cli`
- CLI framework: Cobra (`github.com/spf13/cobra`)
- GitLab API client: `gitlab.com/gitlab-org/api/client-go/v2`
- Task runner: Taskfile v3 (`Taskfile.yml`)
- Primary binary: `gl`, built into `bin/gl`
- Additional binary: `gl-axi`, built into `bin/gl-axi`

## Project Structure

- `cmd/gl/main.go`: `gl` application entry point; calls `cli.Execute()`.
- `cmd/gl-axi/main.go`: `gl-axi` application entry point; calls `cli.ExecuteAxi()`.
- `internal/cli/root.go`: shared Cobra root command definition and CLI initialization.
- `internal/gitlabclient/config.go`: shared GitLab client-go configuration and client construction.
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

## GitLab Client-Go Workflow For Agents

All GitLab communication should go through `internal/gitlabclient`, which creates the official `gitlab.com/gitlab-org/api/client-go/v2` client. Do not add parallel hand-written REST callers unless there is a documented client-go gap.

Configuration contract:

- Token: prefer `GITLAB_TOKEN`; `GL_TOKEN` and `--gitlab-token` are also supported.
- Instance URL: use `GITLAB_BASE_URL` or `--gitlab-base-url`; default is `https://gitlab.com`.
- `client-go` accepts either the GitLab root URL or an `/api/v4` URL and normalizes the API path internally.

`gl` and `gl-axi` share GitLab API behavior. Keep GitLab API calls in shared command code and branch only at the presentation/ergonomics layer through the root command mode:

- `gl`: default `--output text`; supports `text` and `json`; normal stderr errors.
- `gl-axi`: default `--output toon`; supports `toon` and `json`; compact fields, contextual `next` hints, structured TOON-style errors, and content-first root behavior.
- Running `gl-axi` with no subcommand should show live data, not help. While `whoami` is the only GitLab-backed command, the root dashboard delegates to `whoami`.

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
