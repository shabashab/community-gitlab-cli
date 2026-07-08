# gl-axi Output Reference

`gl-axi` implements the [AXI standard](https://github.com/kunchenguid/axi) — ergonomic conventions for CLIs that autonomous agents drive through shell execution. This page describes the output contract in detail. For errors and exit codes see [errors-and-exit-codes.md](errors-and-exit-codes.md); for session integrations see [agent-integrations.md](agent-integrations.md).

## Format

Default output is [TOON](https://toonformat.dev/) (Token-Oriented Object Notation), encoded with the spec-compliant [`toon-go`](https://github.com/toon-format/toon-go) library. Pass `-o json` for JSON with the same structure. Every output is one document: single entities render as nested objects, collections as tabular arrays with declared lengths.

```
merge_requests[2]{iid,title,state,author}:
  41,Fix auth bug,opened,alice
  38,Add pagination,opened,bob
count: 2 of 2 total
help[1]: Run `mr view <iid>` for details
```

Implementation rule: all TOON is produced by `toon.MarshalString` over structs tagged `toon:"..."` (see `internal/cli/output.go`). Hand-formatted TOON is not allowed anywhere in the codebase — quoting, escaping, and header rules come from the library, so output always strict-decodes.

The document itself is spec-compliant (no trailing newline); the CLI appends a single POSIX trailing newline when writing to stdout.

## Help hints

Outputs that have a non-obvious next step end with a `help[N]:` array of complete, runnable commands:

- Dynamic values are parameterized (`mr view <iid>`), never guessed.
- Disambiguating flags carry forward: `mr list --project group/app` suggests `mr view <iid> --project group/app`.
- Self-contained outputs (detail views, confirmations) carry no hints.
- Pagination hints appear only when a next page can exist: `--page 2` when `total_pages > page`, or a "more results may exist" hint when GitLab omits the total and the page came back full.

## Home view

Running `gl-axi` with no arguments prints tool identity first, then the most relevant live content:

```
bin: ~/go/bin/gl-axi
description: Agent-ergonomic GitLab CLI — merge requests, projects, and auth over the personal access token API.
project: group/app
merge_requests[3]{iid,title,state,author}:
  ...
count: 3 of 3 total open
help[1]: Run `gl-axi mr view <iid>` for details
```

Inside a GitLab repository the content is the project's open merge requests (up to 5); outside one it is the authenticated user (`whoami` data) with hints for pointing the CLI at a project.

## Merge request list

`gl-axi mr` / `gl-axi mr list` returns compact rows with the minimal default schema `iid,title,state,author`.

- `--fields draft,source_branch,target_branch,updated_at,web_url` adds columns (comma-separated, any subset). Unknown names are rejected with the valid set inline. Requesting a default column is a no-op.
- `count: N of M total` states the definitive result size. Empty results are explicit: `merge_requests[0]:` plus `count: 0 of 0 total` plus a filter-relaxation hint — the absence of results is the answer, not a failure.
- When GitLab omits the `X-Total` header (very large lists), the line reads `count: N of unknown total`.
- JSON output additionally carries numeric `total`, `page`, `total_pages` fields.

## Merge request view

`gl-axi mr <iid>` returns a compact detail view: `iid, title, state, draft, author, source_branch, target_branch, detailed_merge_status, has_conflicts, pipeline_status, user_notes_count, updated_at, web_url, description`.

- The description is truncated at 500 runes with an explicit size marker: `… (truncated, 8432 chars total)`.
- When (and only when) truncation happened, a hint suggests the escape hatch: `help[1]: Run `mr view 42 --full` for the complete description and all fields`.
- `--full` returns every field (assignees, reviewers, labels, milestone, changes_count, sha, timestamps, ...) and the complete description, with no hints — the view is self-contained.
- String lists (assignees, reviewers, labels) are TOON inline arrays: `labels[2]: backend,search`.
- The literal ref `current` (`gl-axi mr current` / `mr view current`) resolves to the open merge request whose source branch is the currently checked out git branch. Only open merge requests match. Zero matches fail loud with `no_current_merge_request`, several matches with `ambiguous_current_merge_request` (candidates listed), and an unresolvable branch (detached HEAD, not a repository) with `missing_current_branch` — all exit 1 with runnable hints.

## Merge request create

`gl-axi mr create --title <title>` posts a new merge request and returns the same compact `merge_request:` object as the view, so agents parse one shape for both. Unlike the self-contained view, a create always carries a next-step hint:

```
merge_request:
  iid: 42
  title: Add search endpoint
  ...
  web_url: "https://gitlab.example/group/app/-/merge_requests/42"
help[1]: Run `mr view 42` to check merge status and pipeline results
```

- Only `--title` is required. `--source-branch` defaults to the current git branch; `--target-branch` defaults to the project's default branch (both lookups are skipped when the flag is passed).
- Description is dual-input per the content-flags convention: `--description <text>` inline, or `--description-file <path>` (`-` reads stdin). Passing both is a usage error.
- `--assignee`/`--reviewer` accept a username (optional `@` prefix, resolved via the users API) or an all-digits numeric user ID; both are repeatable.
- `--draft` prefixes the title with `Draft:` client-side (GitLab derives draft status from the title). An existing `draft:` prefix is not doubled.
- `--label` (repeatable), `--milestone-id`, `--target-project-id` (cross-fork), `--remove-source-branch`, `--squash`, and `--allow-collaboration` map directly to the API; the booleans are sent only when explicitly passed.
- If the description exceeds the truncation limit, the output truncates it as usual and adds the `mr view <iid> --full` escape-hatch hint.

## Merge request update

`gl-axi mr update <!iid|iid|current> --<flag> <value>` updates an existing merge request and returns the same compact `merge_request:` object plus the `mr view <iid>` next-step hint as create. The `current` ref resolves exactly as in the view command.

- **A field is sent iff its flag was passed.** Unset fields keep their current values on GitLab; there are no implicit defaults. Calling `mr update` with no field flags at all is a usage error (`no_update_flags`, exit 2) — nothing was requested.
- Explicitly empty values clear: `--description ""` clears the description, `--assignee ""` unassigns everyone, `--reviewer ""` removes all reviewers, `--label ""` removes all labels, `--milestone-id 0` unassigns the milestone. `--title ""` and `--target-branch ""` are usage errors — those fields cannot be cleared.
- Labels come in two modes: `--label` replaces the full set; `--add-label`/`--remove-label` adjust it incrementally and may combine. `--label` together with either incremental flag is a usage error.
- `--draft` and `--ready` (mutually exclusive) rewrite the title client-side: `--draft` prepends `Draft:`, `--ready` strips a leading `Draft:` prefix (case-insensitive). When `--title` is not passed alongside, the current title is fetched first so the prefix change applies to what is actually on the merge request. Applying an already-satisfied state (`--ready` on a non-draft) still succeeds with exit 0.
- Booleans (`--squash`, `--remove-source-branch`, `--allow-collaboration`, `--discussion-locked`) are sent only when passed; `--squash=false` sends an explicit `false`.
- Description is dual-input per the content-flags convention: `--description <text>` inline, or `--description-file <path>` (`-` reads stdin).
- The source branch and cross-fork target project cannot be changed after creation; the update API has no such fields.

## Auth and project outputs

- `auth login` → `login:` object plus hints for the next step.
- `auth logout` → `logout:` object with a `backends[N]:` array; logging out with nothing stored is acknowledged with `noop: true` and exit 0.
- `auth status` → `status:` object; probe problems surface as a `warnings[N]:` array inside it.
- `project info` → `project:` object (nested `namespace:` when present), no hints — a detail view fully answers the query.

## Token-frugality rules for new commands

When extending the axi surface, keep defaults narrow and information definitive:

- List schemas: 3–4 default columns, `--fields` for more.
- Long-form content: truncate with a size marker, suggest `--full` only when truncated.
- Always state totals and explicit zeros.
- Put every escape hatch behind a flag, never in the default output.
- Mutations return the compact detail view of the resulting resource plus a single next-step hint — never the full field set by default.
- Content-bearing inputs follow the dual-flag convention: `--<thing>` for inline text, `--<thing>-file <path>` for file input with `-` meaning stdin (see `resolveContentFlag` in `internal/cli/flags.go`).
