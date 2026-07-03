# Agentic Project Documentation

## What Is This Project?

This project is a community GitLab CLI that works through GitLab's personal access token API. It is built mainly for agentic workflows, so the project should prioritize an agentic experience: predictable command behavior, script-friendly output, clear failure modes, and workflows that are easy for coding agents to inspect and automate.

The project includes another binary named `gl-axi`. That binary is intended to be based on the `axi` standard introduced by Kun Chen: https://github.com/kunchenguid/axi.

## Project Stack

- Language: Go
- Module: `github.com/shabashab/community-gitlab-cli`
- CLI framework: Cobra (`github.com/spf13/cobra`)
- Task runner: Taskfile v3 (`Taskfile.yml`)
- Primary binary: `gl`, built into `bin/gl`
- Additional binary: `gl-axi`, built into `bin/gl-axi`

## Project Structure

- `cmd/gl/main.go`: `gl` application entry point; calls `cli.Execute()`.
- `cmd/gl-axi/main.go`: `gl-axi` application entry point; calls `cli.ExecuteAxi()`.
- `internal/cli/root.go`: shared Cobra root command definition and CLI initialization.
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

## Notes For Agents

- Keep generated binaries and transient local outputs out of source changes unless explicitly requested.
- Prefer adding task commands to `Taskfile.yml` when a workflow becomes repeated or important for agentic development.
- If tests are added, document the test command here and in `README.md`.
- Preserve scriptability when adding CLI behavior: stable flags, clear stdout/stderr separation, useful exit codes, and machine-readable output where appropriate.
