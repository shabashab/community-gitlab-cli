---
name: gl-axi
description: >
  Agent-ergonomic GitLab CLI for merge requests, projects, and auth over the
  personal access token API. Use for any GitLab operation from the shell:
  listing or inspecting merge requests, checking project info, or managing
  stored GitLab credentials. Prefer it over raw GitLab API calls.
---

# gl-axi — Agent-ergonomic GitLab CLI

`gl-axi` follows the AXI standard: token-efficient TOON output on stdout,
structured errors with machine-readable codes, definitive empty states, and
`help[]` next-step suggestions after each output. Exit codes: 0 success
(including no-ops), 1 error, 2 usage error.

If `gl-axi` is not on PATH, install it with:

```sh
go install github.com/shabashab/community-gitlab-cli/cmd/gl-axi@latest
```

## Orientation

```sh
gl-axi                 # home view: bin, description, then live data
                       # (open MRs inside a GitLab repo, your user otherwise)
gl-axi whoami          # authenticated GitLab user
gl-axi project info    # current project details (or --project <id-or-path>)
```

Project-aware commands discover the project from the git `origin` remote.
Outside a repository, pass `--project <id-or-path>` (numeric ID or
`group/subgroup/project`).

## Merge requests

```sh
gl-axi mr                             # open MRs, compact 4-column rows
gl-axi mr 123                         # one MR (also: gl-axi mr '!123')
gl-axi mr current                     # the MR of the checked-out branch
gl-axi mr 123 --full                  # all fields + complete description
gl-axi mr list --state all --author octocat
gl-axi mr list --fields source_branch,updated_at   # add columns
gl-axi mr list --search "auth" --label bug --page 2
gl-axi mr create --title "Fix auth"                # source = current branch, target = default branch
gl-axi mr create --title "Fix auth" --description-file notes.md
gl-axi mr create --title "Fix auth" --description-file - < notes.md
gl-axi mr update 123 --title "Fix auth v2" --ready
gl-axi mr update 123 --add-label bug --assignee mona
gl-axi mr update current --ready
```

- List rows default to `iid,title,state,author`; `--fields` adds
  `draft,source_branch,target_branch,updated_at,web_url`.
- Detail views truncate the description at 500 chars with an explicit size
  marker; rerun with `--full` when the output says it was truncated.
- Every list ends with `count: N of M total` — the definitive result size.
- `mr create` needs only `--title`; source/target branches default to the
  current git branch and the project default branch. `--description` takes
  inline text, `--description-file` a path (`-` = stdin) — never both.
  `--assignee`/`--reviewer` take a username or numeric user ID (repeatable);
  `--draft`, `--label`, `--milestone-id`, `--squash`,
  `--remove-source-branch` cover the remaining basics.
- `mr update <iid>` sends only the fields whose flags you pass; everything
  else keeps its current value. `--draft`/`--ready` toggle the `Draft:` title
  prefix; `--label` replaces all labels while `--add-label`/`--remove-label`
  adjust incrementally; explicitly empty values clear (`--description ""`,
  `--assignee ""`, `--milestone-id 0`). No flags at all is a usage error.
- The ref `current` (view and update) resolves via the current git branch to
  its open MR. Open MRs only; zero or multiple matches fail loud (exit 1,
  codes `no_current_merge_request` / `ambiguous_current_merge_request` with
  candidates listed), as does an unresolvable branch
  (`missing_current_branch`).

## Auth

```sh
gl-axi auth status                                  # would a command from here find a credential?
gl-axi auth login <token> --gitlab-base-url <url>   # verify + store per host
gl-axi auth logout                                  # remove (no-op exit 0 if absent)
```

Token precedence: `--gitlab-token`, `GITLAB_TOKEN`, `GL_TOKEN`, stored
credential for the resolved host. Instance URL: `--gitlab-base-url`,
`GITLAB_BASE_URL`, git origin host, `https://gitlab.com`.

## Output formats

Default output is TOON. Pass `-o json` for JSON with the same structure.
Errors are structured on stdout in the same format:

```
error: <message>
code: <machine_readable_code>
help[1]: <command that fixes the problem>
```

## Ambient context (optional)

`gl-axi setup hooks` installs SessionStart hooks (Claude Code, Codex,
OpenCode) that inject the current repository's open merge requests at session
start. This skill and the hook are complementary — one of them is enough.
