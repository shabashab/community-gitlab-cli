# Merge request workflows

The `mr` command covers the merge request lifecycle: discovery, creation,
updates, approvals, finalization, diffs, discussions, reactions, comments, and
draft reviews.

Examples use `gl` for human-readable output. The same command surface is
available through `gl-axi` with TOON output by default.

## Choose a project and merge request

Inside a Git repository, the CLI discovers the project from `remote.origin.url`:

```sh
gl mr
```

Pass `--project` when running elsewhere or targeting another project:

```sh
gl mr --project group/subgroup/project
gl mr --project 12345
```

A merge request can be referenced by plain IID, bang-prefixed IID, or
`current`:

```sh
gl mr 123
gl mr '!123'
gl mr current
```

Quote `'!123'` in shells that treat `!` as history expansion. `current` means
the open merge request whose source branch matches the checked-out branch. It
fails instead of guessing when there are no matches, multiple matches, or no
resolvable current branch.

## Find and inspect merge requests

Running `mr` without arguments lists open merge requests:

```sh
gl mr
gl mr list --state all --author octocat --search "search endpoint"
gl mr list --label backend --reviewer alice --target-branch main
gl mr list --order-by updated_at --sort desc --limit 50 --page 2
```

View one merge request with compact fields, or request the complete description
and all available fields:

```sh
gl mr 123
gl mr current
gl mr 123 --full
```

`gl` renders lists as tables and supports `--output json`. `gl-axi` keeps list
rows narrow by default and accepts `--fields` for optional columns:

```sh
gl-axi mr list --fields source_branch,target_branch,updated_at,web_url
```

Every list reports an exact count. Use command help for all filters:

```sh
gl mr list --help
```

## Create a merge request

Only the title is required when the current branch and project can be
discovered:

```sh
gl mr create --title "Add search endpoint"
```

The source defaults to the checked-out branch and the target defaults to the
project's default branch.

```sh
gl mr create \
  --title "Fix authentication" \
  --draft \
  --label bug \
  --assignee mona \
  --reviewer alice
```

Long content supports either an inline value or a file:

```sh
gl mr create --title "Document the API" --description "Short description"
gl mr create --title "Document the API" --description-file notes.md
git log -1 --format=%b | gl mr create --title "Use commit body" --description-file -
```

The inline and file forms are mutually exclusive. `--assignee` and
`--reviewer` accept usernames, optional `@` prefixes, or numeric user IDs and
can be repeated.

For forks or explicit branch selection:

```sh
gl mr create \
  --title "Backport fix" \
  --source-branch fix/auth \
  --target-branch release \
  --target-project-id 12345
```

GitLab conflicts, including an existing open merge request for the branch pair,
remain errors so the CLI never claims that unknown existing content is the
requested result.

## Update a merge request

Only explicitly passed fields are sent to GitLab:

```sh
gl mr update 123 --title "Rework search endpoint" --ready
gl mr update current --add-label backend --remove-label triage
gl mr update 123 --assignee mona --reviewer alice
```

Running `mr update` without field flags is a usage error. Explicitly empty
values clear supported fields:

```sh
gl mr update 123 --description ""
gl mr update 123 --assignee "" --reviewer ""
gl mr update 123 --label ""
gl mr update 123 --milestone-id 0
```

`--label` replaces all labels. `--add-label` and `--remove-label` adjust them
incrementally and can be combined, but cannot be mixed with `--label`.

`--draft` and `--ready` toggle the `Draft:` title prefix. Boolean fields can be
explicitly disabled with forms such as `--squash=false`. Source branch and
cross-fork target project cannot be changed after creation.

## Approve and finalize

Inspect approval readiness before approving:

```sh
gl mr approvals 123
gl mr approvals 123 --full
gl mr approve 123
gl mr approve 123 --sha f5b0c3d2e1
gl mr unapprove 123
```

Use `--sha` when an automation has already inspected a specific head commit and
must not approve a newer one.

Merge, close, or reopen:

```sh
gl mr merge 123 --sha f5b0c3d2e1
gl mr merge 123 --auto-merge --squash=false
gl mr merge current --remove-source-branch
gl mr close 123
gl mr reopen 123
```

Custom merge and squash commit messages follow the same inline/file input
convention:

```sh
gl mr merge 123 --merge-commit-message-file message.md
gl mr merge 123 --squash-commit-message "Squashed change"
```

Already merged, closed, or open states are verified before they are reported as
successful no-ops. Incompatible states still surface a GitLab error.

## Inspect diffs

List changed files before requesting the full patch:

```sh
gl mr diff 123
gl mr diff current --file internal/search.go
gl-axi mr diff current --fields old_path,new_ranges,old_ranges
```

The summary includes path, status, additions, deletions, and hunk count. Optional
fields expose renames, generated/collapsed/too-large state, and commentable line
ranges.

Stream the unified diff when another tool needs patch bytes:

```sh
gl mr diff patch 123 > review.patch
```

For a deep agent review, export a filesystem bundle:

```sh
gl-axi mr diff export 123 --dir .gl-axi/mr-123
```

The bundle contains a manifest, changed-file metadata, the full patch,
per-file diffs, and old/new changed-file snapshots pinned to the merge request
diff refs. A non-empty destination is rejected unless `--force` is passed.

## Review discussions

Discussion lists show unresolved threads by default:

```sh
gl mr discussions 123
gl mr discussions current --state all --system
gl mr discussions 123 --order-by updated_at --sort desc
gl mr discussions 123 --author alice --fields file,line,id_full
```

`--state resolved` selects resolved threads; `--state all` also includes
non-resolvable comments and, with `--system`, system activity. Filtering,
sorting, and paging happen after the CLI fetches every discussion, so totals are
exact.

Lists show an eight-character discussion ID. Any unique hexadecimal prefix or
the full ID is accepted:

```sh
gl mr discussion 123 6f9a1c2d
gl mr discussion resolve 123 6f9a1c2d
gl mr discussion unresolve current aa11bb22
```

The thread view includes every note's full body, numeric note ID, author,
timestamps, position, and reactions.

### React to a note

Use the discussion ID and numeric note ID from the thread view:

```sh
gl mr discussion react 123 6f9a1c2d 901 thumbsup
gl mr discussion unreact 123 6f9a1c2d 901 :thumbsup:
```

Emoji names work with or without surrounding colons. Adding an emoji you
already awarded or removing one you did not award is a verified no-op.

Thread views always show per-note reactions. Add an aggregate reaction column
to a discussion list only when needed:

```sh
gl mr discussions 123 --reactions
```

This option performs additional requests because GitLab's discussion listing
does not include award data.

## Add review comments

Without position flags, `mr comment` starts a resolvable discussion:

```sh
gl mr comment 123 --body "LGTM overall"
gl mr comment 123 --note --body "FYI: deploy scheduled"
```

`--note` creates a plain, non-resolvable note instead. Comment bodies also
support `--body-file <path>` and `--body-file -` for stdin.

Anchor comments to a changed file, line, or range:

```sh
gl mr comment 123 --file internal/search.go --body "Can we rename this file?"
gl mr comment 123 --file internal/search.go --line 42 --body "Can we simplify this?"
gl mr comment 123 --file internal/search.go --line 10:15 --body "Extract this block"
gl mr comment 123 --file internal/search.go --old-line 40 --body "Why remove this?"
```

The CLI fetches the merge request diff and constructs GitLab's complete
position object. You provide the repository path and visible line numbers, not
diff SHAs or line codes.

Position errors are reported before a comment is posted and include useful
changed paths or line ranges. A file-level comment remains possible when a diff
is too large for line positioning.

Reply to an existing thread with its unique ID prefix:

```sh
gl mr comment 123 --reply-to 6f9a1c2d --body "Agreed"
```

Replies cannot be combined with new position flags.

## Publish a draft review

Add `--draft` to keep comments private until the review is ready:

```sh
gl mr comment 123 --draft --file internal/search.go --line 42 \
  --body "This condition looks inverted"
gl mr comment 123 --draft --reply-to 6f9a1c2d --resolve \
  --body "Fixed in the latest push"
```

Inspect and publish pending drafts:

```sh
gl mr drafts 123
gl mr drafts publish 123 --all
gl mr drafts publish 123 77
gl mr drafts delete 123 77
```

The recommended review flow is to create all draft comments and then publish
them together with `--all`. Publishing an observed-empty draft set succeeds as
a verified no-op.

## Script and agent output

Use JSON when a conventional script needs structured output:

```sh
gl mr current --output json
```

Use `gl-axi` for token-efficient agent output, structured errors, exact counts,
and next-step hints:

```sh
gl-axi mr current
gl-axi mr discussions current --order-by updated_at --sort desc
```

See the [gl-axi output reference](axi-output.md) and
[Errors and exit codes](errors-and-exit-codes.md) for stable output and failure
contracts. Use `gl mr <command> --help` as the exhaustive flag reference.
