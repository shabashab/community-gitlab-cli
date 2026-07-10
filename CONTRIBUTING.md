# Contributing

Thanks for helping improve Community GitLab CLI. Contributions should preserve
the project's predictable command behavior, useful failure modes, and stable
automation contracts.

## Development setup

Install:

- [Git](https://git-scm.com/)
- [Go 1.26.4 or newer](https://go.dev/doc/install)
- [Task v3](https://taskfile.dev/docs/installation)

Clone the repository, build both binaries, and run the unit tests:

```sh
git clone https://github.com/shabashab/community-gitlab-cli.git
cd community-gitlab-cli
task build
task test
```

Run either locally built command while developing:

```sh
task run -- --help
task run-axi -- --help
```

## Before opening a pull request

Run the standard checks:

```sh
task test
task build
```

When a change touches the CLI command surface, update the relevant user
documentation and the live E2E scenarios. See [E2E / UAT testing](docs/e2e-testing.md)
for environment setup, script conventions, and the UAT checklist.

Keep pull requests focused and include:

- A concise description of the user-visible outcome.
- Tests for new or changed behavior.
- Documentation updates for commands, flags, output shapes, errors, or exit
  codes.
- The validation commands you ran and whether live E2E verification remains
  pending.

## Project guidance

[AGENTS.md](AGENTS.md) is the detailed repository guide. It documents the
architecture, command contracts, testing patterns, worktree workflow, and
GitLab client conventions used by this project.

Useful focused references include:

- [gl-axi output reference](docs/axi-output.md)
- [Errors and exit codes](docs/errors-and-exit-codes.md)
- [Agent session integrations](docs/agent-integrations.md)
- [E2E / UAT testing](docs/e2e-testing.md)
