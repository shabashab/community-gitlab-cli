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
gl-axi mr 123 --full                  # all fields + complete description
gl-axi mr list --state all --author octocat
gl-axi mr list --fields source_branch,updated_at   # add columns
gl-axi mr list --search "auth" --label bug --page 2
```

- List rows default to `iid,title,state,author`; `--fields` adds
  `draft,source_branch,target_branch,updated_at,web_url`.
- Detail views truncate the description at 500 chars with an explicit size
  marker; rerun with `--full` when the output says it was truncated.
- Every list ends with `count: N of M total` — the definitive result size.

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
