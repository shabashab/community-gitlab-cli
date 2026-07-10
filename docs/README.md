# Community GitLab CLI documentation

Start with the [project README](../README.md) to build the CLI, choose between
`gl` and `gl-axi`, authenticate, and run your first merge request commands.

## Use the CLI

- [Authentication and configuration](authentication.md) — personal access
  tokens, stored credentials, GitLab hosts, project discovery, and output
  formats.
- [Merge request workflows](merge-requests.md) — create and update merge
  requests, inspect diffs, review discussions, react to notes, and publish
  draft reviews.
- [Errors and exit codes](errors-and-exit-codes.md) — failure behavior for
  scripts and agent workflows.

The built-in command reference is always available from the CLI:

```sh
gl --help
gl mr --help
gl mr comment --help
```

## Use gl-axi with agents

- [Agent session integrations](agent-integrations.md) — install ambient context
  hooks for Claude Code, Codex, and OpenCode, or install the Agent Skill.
- [gl-axi output reference](axi-output.md) — TOON and JSON shapes, compact
  fields, truncation, counts, and next-step hints.
- [Errors and exit codes](errors-and-exit-codes.md) — structured errors,
  machine-readable codes, and the exit-code contract.

## Contribute

- [Contributing guide](../CONTRIBUTING.md) — local setup, validation, and pull
  request expectations.
- [E2E / UAT testing](e2e-testing.md) — live GitLab test environment,
  testscript scenarios, and release verification.
- [Agent project guide](../AGENTS.md) — detailed repository architecture and
  implementation contracts for coding agents.
