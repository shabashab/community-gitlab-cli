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

`gl-axi mr <iid>` returns a compact detail view: `iid, title, state, draft, author, source_branch, target_branch, detailed_merge_status, has_conflicts, pipeline_status, user_notes_count, updated_at, web_url, approval, description`.

- The description is truncated at 500 runes with an explicit size marker: `… (truncated, 8432 chars total)`.
- The nested `approval:` summary is fetched by default and includes `approved`, `approvals_required`, `approvals_left`, `user_has_approved`, `user_can_approve`, and `approved_by[]`.
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

## Merge request approvals

`gl-axi mr approvals <!iid|iid|current>` returns compact approval status:

```
approval:
  iid: 42
  approved: false
  approvals_required: 2
  approvals_left: 1
  user_has_approved: true
  user_can_approve: false
  approved_by[1]{username,approved_at}:
    alice,"2026-07-04T10:00:00Z"
```

- `mr <iid> approvals` is a parent-dispatch alias for `mr approvals <iid>`; `mr approval <iid>` is also accepted.
- `--full` adds approval configuration metadata, suggested approvers, configured approvers/groups, and approval rules still left.
- `mr approve <iid>` approves as the authenticated user and returns compact approval status plus a `mr view <iid>` next-step hint.
- `mr approve <iid> --sha <sha>` sends GitLab's optimistic guard and approves only when the merge request head still matches that SHA. Empty `--sha` is a usage error.
- `mr unapprove <iid>` removes your approval and then refreshes approval status because GitLab's unapprove endpoint has no response body.

## Merge request discussions

`gl-axi mr discussions <!iid|iid|current>` lists the discussion threads of one merge request as compact rows with the default schema `id,author,state,notes,updated_at,preview`:

```
discussions[2]{id,author,state,notes,updated_at,preview}:
  6f9a1c2d,alice,unresolved,2,"2026-07-03T12:00:00Z",This branch check looks inverted. See the loop above.
  aa11bb22,mona,resolved,1,"2026-07-01T08:00:00Z",Typo in the docstring
count: 2 of 2 total
help[1]: Run `mr discussion 123 <id>` for the full conversation
```

- The default schema is six columns — a documented deviation from the 3–4 guideline: threads have no title, so the flattened 80-rune `preview` of the first note is the title-equivalent, and `updated_at` (the newest note update in the thread) is the whole point of the command: spotting threads with news.
- `id` is the lowercase 8-character prefix of the 40-character discussion ID; every command that takes a discussion ID accepts any unique prefix. `--fields type,file,line,created_at,id_full` adds columns (comma-separated, any subset), including the full ID.
- `state` is `resolved`, `unresolved`, or `none` for threads that are not resolvable (standalone comments, system activity).
- **The GitLab discussions API has no server-side filters or sorting**, so the CLI fetches the complete thread list and filters, sorts, and pages client-side. Totals in `count: N of M total` are therefore always exact (`M` = threads matching the filters).
- Filters: `--state all|resolved|unresolved` (**default `unresolved`** — the actionable set; `unresolved` means resolvable and not resolved, so `none` threads match only `all`), `--author <username>` (thread starter, optional `@`, case-insensitive), and `--system` to include system-generated discussions (hidden by default).
- Sorting: `--order-by created_at|updated_at` plus `--sort asc|desc` (default `created_at asc`, the API order). `--order-by updated_at --sort desc` answers "what changed most recently?".
- Paging: `--limit` (default 20) and `--page` apply to the filtered result. Next-page hints re-emit every non-default filter flag so the suggested command is runnable as-is.
- Empty results are explicit (`discussions[0]:` + `count: 0 of 0 total`) with a filter-relaxation hint; when system discussions were excluded, a second hint states how many.

## Discussion thread view

`gl-axi mr discussion <!iid|iid|current> <discussion-id>` prints one full conversation: a `discussion:` object followed by a `notes[N]:` tabular array with **complete note bodies** — this command is the escape hatch, so nothing is truncated and no hints are emitted (the view is self-contained).

```
discussion:
  id: 6f9a1c2d0e5b7a9c1d2e3f4a5b6c7d8e9f0a1b2c
  state: unresolved
  resolvable: true
  file: internal/cli/mr.go
  line: 42
  updated_at: "2026-07-03T12:00:00Z"
  notes: 2
notes[2]{id,author,created_at,updated_at,system,body}:
  901,alice,"2026-07-01T08:00:00Z","2026-07-03T12:00:00Z",false,"This branch check looks inverted.\nSee the loop above."
  902,bob,"2026-07-02T09:00:00Z","2026-07-02T09:00:00Z",false,Agreed - needs a fix
```

- `<discussion-id>` is the full 40-character hex ID (fetched directly) or any unique prefix (resolved against the thread list, case-insensitive). An ambiguous prefix is a usage error (`ambiguous_discussion_ref`, exit 2) stating the match count; a prefix matching nothing is `discussion_not_found` (exit 1). A full ID that does not exist surfaces as the API's `gitlab_not_found`.
- `file`/`line` appear only for diff threads; `resolved_by`/`resolved_at` only for resolved threads.

## Merge request diff

`gl-axi mr diff <!iid|iid|current>` returns a compact changed-file summary, not a patch dump:

```
diff:
  iid: 123
  base_sha: base000
  start_sha: start000
  head_sha: head000
  files: 2
files[2]{path,status,additions,deletions,hunks}:
  src/app.go,modified,2,1,1
  new/name.go,renamed,1,1,1
count: 2 of 2 total
help[1]: Run `mr diff 123 --file <path> --fields new_ranges,old_ranges` for one file
help[2]: Run `mr diff export 123 --dir .gl-axi/mr-123` to create a filesystem review bundle
help[3]: Run `mr comment 123 --file <path> --line <line> --body <text>` to comment on a diff line
```

- Default rows are `path,status,additions,deletions,hunks`; `--fields old_path,generated,collapsed,too_large,new_ranges,old_ranges` adds columns. `status` is `added`, `deleted`, `renamed`, or `modified`.
- `--file <path>` accepts the old or new repository-relative path and narrows the result to one changed file. `--limit`/`--page` are client-side over the full changed-file list; totals are exact.
- `mr diff patch <iid>` writes GitLab's raw unified diff bytes directly to stdout and intentionally bypasses structured TOON output.
- `mr diff export <iid> --dir <path>` writes an agent review bundle: `manifest.toon`, `files.toon`, `patch.diff`, `diffs/<path>.diff`, and changed-file snapshots under `old/` and `new/`, pinned to the merge request diff refs. Non-empty export directories are refused unless `--force` is passed.

## Merge request comment

`gl-axi mr comment <!iid|iid|current> --body <text>` creates a review comment and returns a compact `comment:` object — the created note's identifiers and what GitLab actually anchored, never a body echo (the caller knows what it wrote):

```
comment:
  discussion_id: 6f9a1c2d
  note_id: 9001
  author: reviewer
  type: DiffNote
  resolvable: true
  file: src/app.go
  line: 42
  created_at: "2026-07-08T10:00:00Z"
help[1]: Run `mr discussion 123 6f9a1c2d` for the full thread
```

- The body is dual-input per the content-flags convention: `--body <text>` inline, or `--body-file <path>` (`-` reads stdin, the primary agent path). Passing both, or neither, is a usage error.
- Without position flags the comment starts a resolvable discussion thread. `--note` posts a plain non-resolvable note via the notes API instead (its output carries no `discussion_id`).
- **Positioning is resolved by the CLI, not the caller.** `--file <path>` anchors to a changed file (alone: a file-level comment); `--line <n>` addresses the new file version, `--old-line <n>` the old one (removed lines); `<start>:<end>` covers a range. The CLI fetches the merge request diff, classifies the target line (added / removed / unchanged), and builds GitLab's position object — SHAs, `old_path`/`new_path` (renames included), the old/new line pairing, and fabricated `line_code`s for ranges. Callers never supply SHAs.
- Position failures are loud and specific: `merge_request_diff_not_ready` (diff refs still being prepared — retry), `file_not_in_diff` (hint lists changed paths), `line_not_in_diff` (hint lists the commentable line ranges, plus a cross-side suggestion when the line exists on the other side), `diff_too_large` (comment file-level instead). All exit 1 before anything is posted.
- `--draft` creates the comment as a pending draft note instead (see below). `--reply-to <discussion-id>` (full ID or unique prefix) answers an existing thread; it cannot combine with position flags or `--note`. `--resolve` (draft replies only) resolves the thread when the draft publishes.
- **Position downgrade surfacing:** GitLab can answer 201 yet silently attach the comment to the merge request instead of the requested diff line. The output's `type`/`file`/`line` reflect the response, and when a requested position did not stick a hint says so — the mutation still succeeded (exit 0), so agents must not retry.

## Draft notes

`gl-axi mr comment <iid> --draft ...` returns a compact `draft_note:` object. GitLab's draft-note response is thin (no author name, no timestamps), and the preview is the flattened 80-rune head of the body:

```
draft_note:
  id: 77
  preview: This nil check is inverted
  file: src/app.go
  line: 42
help[1]: Run `mr drafts publish 123 77` to publish it, or `mr drafts publish 123 --all` for the whole pending review
help[2]: Run `mr drafts 123` to list pending drafts
```

The intended agent review flow is N × `mr comment <iid> --draft ...` followed by one `mr drafts publish <iid> --all` — the review lands atomically and is gentler on rate limits than N immediate comments.

`gl-axi mr drafts <!iid|iid|current>` lists your pending drafts with the default schema `id,file,line,preview` (`--fields discussion_id,resolve_discussion` adds columns; `discussion_id` is set only on reply drafts). The draft-notes API has no filters, so paging (`--limit`/`--page`) is client-side and `count: N of M total` is always exact:

```
draft_notes[2]{id,file,line,preview}:
  77,src/app.go,42,This nil check is inverted
  78,,0,Overall direction looks good
count: 2 of 2 total
help[1]: Run `mr drafts publish 123 --all` to publish the pending review, or `mr drafts publish 123 <id>` for a single draft
```

`gl-axi mr drafts publish <iid> <draft-id>` publishes one draft; `--all` bulk-publishes every pending draft (the two forms are mutually exclusive, and one must be given). The output is a `published:` object with the count of drafts observed at publish time:

```
published:
  all: true
  count: 3
help[1]: Run `mr discussions 123` to see the published threads
```

`gl-axi mr drafts delete <iid> <draft-id>` deletes one pending draft and returns a `deleted:` object. See the idempotency notes in [errors-and-exit-codes.md](errors-and-exit-codes.md) for the no-op rules (`publish --all` on an empty set, delete of a verified-absent draft).

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
