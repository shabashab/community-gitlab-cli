# E2E / UAT Testing

The `e2e/` package is a black-box regression net above the unit suite: [testscript](https://pkg.go.dev/github.com/rogpeppe/go-internal/testscript) scripts under `e2e/testdata/` run the real `gl` and `gl-axi` commands (in-process, current source — no stale binaries) against a **live GitLab instance** and assert on output fragments and exit codes. Unit tests prove each command against stubbed HTTP; the E2E scripts prove the same contracts against real GitLab behavior, so refactors and optimizations can be verified end to end.

Run it before and after invasive refactors, before releases, and on the nightly CI schedule.

## Provisioning

The suite needs a GitLab instance it may create and delete projects on. Any instance works — a self-hosted/homelab deployment (recommended: no rate limits, full admin, zero blast radius) or a dedicated gitlab.com group. Scripts are instance-agnostic; the target comes entirely from environment variables.

One-time setup on the instance:

1. Create a group for fixtures, e.g. `gl-e2e`. Everything the suite creates lives under it and is named `gl-e2e-*`; treat the group as disposable.
2. Create a primary test account (a service account, not your personal one) with a personal access token with `api` scope, and make it at least Maintainer of the group (it must create and delete projects).
3. Optional, for approve/unapprove coverage: a second account with its own `api` token, invited to the group as Developer. GitLab does not let an author approve their own merge request, so approval scripts run as this second user and are skipped without it.

Environment variables:

| Variable | Required | Meaning |
| --- | --- | --- |
| `GL_E2E_HOST` | yes | Instance base URL, e.g. `https://gitlab.example.com` |
| `GL_E2E_TOKEN` | yes | `api`-scope PAT of the primary test account |
| `GL_E2E_GROUP` | yes | Group path fixture projects are created under, e.g. `gl-e2e` |
| `GL_E2E_TOKEN_ALT` | no | Second account's PAT; enables `[second-token]` scripts |
| `GL_E2E_PREMIUM` | no | `1` enables `[premium]` scripts (approval-rule assertions) |

Missing required variables fail the run immediately — running with `-tags e2e` is an explicit opt-in, and a silent skip would fake green.

Instance notes:

- **TLS:** the CLI has no `--insecure` flag. If the instance uses a self-signed or private-CA certificate, the CA must be in the system trust store of the machine running the tests. Plain `http://` base URLs work.
- **Edition:** on CE/Free the approvals API returns no rule data; `[premium]` scripts stay skipped unless the instance has an EE license.
- **Keychain safety:** the harness sets `GL_CREDSTORE=file` and redirects `HOME` into each script's workdir, so `auth` scripts never touch the real OS keychain or `~/.gl`, and `setup hooks` scripts never touch real agent configs.

## Running

```sh
task e2e                                  # whole suite
task e2e -- -run TestMR                   # one command family
task e2e -- -run 'TestMR/mr-lifecycle'    # one script
task e2e -- -v -run 'TestMR/mr-lifecycle' # verbose: full command/output log
task e2e:clean                            # sweep leaked gl-e2e-* projects (>1h old)
```

Instead of exporting the variables, you can put them in a `.test.env` file at the repository root — `task e2e` and `task e2e:clean` load it automatically when present (it is gitignored; never commit it):

```sh
# .test.env
GL_E2E_HOST=https://gitlab.example.com
GL_E2E_TOKEN=glpat-...
GL_E2E_GROUP=gl-e2e
# GL_E2E_TOKEN_ALT=glpat-...
# GL_E2E_PREMIUM=1
```

Variables already set in the shell take precedence over the file.

`task e2e` runs `go test -tags e2e -count=1 -parallel 4 -timeout 20m ./e2e`. `-count=1` defeats the test cache (a cached "pass" proves nothing about the live instance); `-parallel 4` bounds concurrent scripts — raise it if the instance handles it. Without `-tags e2e`, `go test ./...` never compiles the package, so the unit suite stays hermetic.

On failure testscript prints the executed script with each command's stdout/stderr and the failing assertion. Pass `-testwork` to keep the script workdir (`$WORK`) for inspection.

## How a script works

Each `.txtar` file under `e2e/testdata/<family>/` is one isolated scenario. Before it runs, the harness (see `e2e/params.go`):

- extracts embedded files into a fresh workdir `$WORK` and chdirs there;
- redirects `HOME` to `$WORK/home` and writes a `.gitconfig` with a test identity, `init.defaultBranch = main`, and a credential helper that supplies `$GL_E2E_TOKEN` for git pushes (the token never appears in remote URLs, which end up in logs);
- exports `GITLAB_BASE_URL`/`GITLAB_TOKEN` (so commands need no flags), `GL_E2E_*`, `SCRIPT_NAME` (the file name), and `RANDOM_STRING` (fresh per run).

Scripts run in parallel; every script must own its fixtures — never reference a project another script made. Quoting rule worth knowing: single quotes suppress `$VAR` expansion, so write `stdout 'iid: '$IID`, not `stdout 'iid: $IID'`.

### Custom commands

Beyond the [testscript built-ins](https://pkg.go.dev/github.com/rogpeppe/go-internal/testscript#hdr-Script_Language) (`exec`, `stdout`, `stderr`, `cd`, `exists`, `env`, ...):

| Command | Purpose |
| --- | --- |
| `exitcode <n> <prog> [args...]` | Run and assert the exact exit code (contract: 0 success/no-op, 1 runtime, 2 usage). `stdout`/`stderr` assertions still apply afterwards. |
| `stdout2env VAR PATTERN` | Capture a regex group from the last stdout into `$VAR` (e.g. `stdout2env IID 'iid: (\d+)'`). |
| `defer <prog> [args...]` | Run a command when the script finishes, pass or fail. |
| `retry <prog> [args...]` | Run a command, retrying up to 10× at 1s intervals until it succeeds. For eventually-consistent state right after a mutation: use it on `mr create` after a fresh push (the branch may not be visible to the API yet) and on the first `mr diff` after a create (diff refs compute asynchronously). |
| `mkproject <suffix>` | Create fixture project `gl-e2e-<suffix>` under `$GL_E2E_GROUP`; exports `$PROJECT` and `$PROJECT_URL`; deletion is auto-deferred. |
| `rmproject <suffix>` | Delete a fixture project explicitly (already-gone is fine). |

### Condition markers

Prefix a line with a condition to run it only when the environment supports it:

- `[second-token]` — `GL_E2E_TOKEN_ALT` is set.
- `[premium]` — `GL_E2E_PREMIUM=1`.

### Canonical merge request prologue

Scripts that need a merge request build the whole fixture themselves — project, repo, branches — so project discovery, branch defaulting, and the `current` ref are exercised for real:

```
mkproject $SCRIPT_NAME-$RANDOM_STRING

exec git init repo
cd repo
exec git remote add origin $GL_E2E_HOST/$PROJECT.git
exec git commit --allow-empty -m init
exec git push -u origin main
exec git checkout -b feature
exec git add change.txt
exec git commit -m 'add change'
exec git push -u origin feature

retry gl-axi mr create --title 'My scenario'
stdout2env IID 'iid: (\d+)'

-- repo/change.txt --
fixture content
```

### Writing assertions

Assert fragments, not whole documents — live output contains instance-specific iids, timestamps, and URLs. The contract sources of truth are [axi-output.md](axi-output.md) (shapes, counts, hints) and [errors-and-exit-codes.md](errors-and-exit-codes.md) (codes, exit classes, no-op rules). House patterns:

- shape headers: `stdout 'merge_requests\[1\]\{iid,title,state,author\}:'`
- definitive counts: `stdout 'count: 1 of 1 total'`
- error contract: `exitcode 2 gl-axi ...` + `stdout 'code: <error_code>'` + `! stderr .`
- no-ops: repeat the mutation, `stdout 'noop: true'` with exit 0
- leak checks: `! stdout 'api/v4'`
- TOON quotes strings that look like other types — an all-digit discussion id renders as `discussion_id: "42991560"` — so make id captures quote-tolerant: `stdout2env DISC 'discussion_id: "?([0-9a-f]{8})'`

## Cleanup

Three layers keep the instance clean: scripts' own deferred deletes, the auto-defer inside `mkproject`, and the janitor (`task e2e:clean` → `e2e/janitor`), which deletes group projects named `gl-e2e-*` older than one hour (`-max-age`, `-dry-run` to preview). Run the janitor after killing a run mid-flight; CI runs it as an always-on post-step.

## CI

`.github/workflows/e2e.yml` runs the suite nightly and on manual dispatch, on GitHub-hosted runners. The instance must be reachable from the public internet.

- **Everything instance-related is a secret** (`GL_E2E_HOST`, `GL_E2E_GROUP`, `GL_E2E_TOKEN`, `GL_E2E_TOKEN_ALT`), stored in the `e2e` environment. Host and group are secrets, not variables, on purpose: this is a public repository, Actions logs are public, and testscript failure output echoes commands containing `$GL_E2E_HOST` — secret values are masked in logs, variables are not.
- **Fork safety**: the workflow has no `pull_request` trigger, so fork code never runs against the instance with upstream secrets (GitHub never passes secrets to fork PRs anyway, and scheduled workflows are disabled in forks by default). A fork dispatching the workflow uses its own empty secrets and dies at the harness env gate. Never add `pull_request` or `pull_request_target` triggers to this file.
- **Approval gate (optional)**: add required reviewers to the `e2e` environment in repo settings to make every run — including manual dispatches by collaborators — require explicit approval.
- The janitor runs as an always-on post-step, so an aborted CI run cannot leave fixture projects behind for longer than the next run.

## UAT checklist

Release verification = the whole suite green against the canonical instance. Current coverage by family:

| Family | Scripts | Covers |
| --- | --- | --- |
| `whoami` | whoami | both binaries, text/toon/json modes, missing-token failure |
| `auth` | auth-roundtrip | login/status/logout, stored-credential use, logout no-op, file backend |
| `project` | project-info | explicit `--project`, origin discovery, json parity |
| `mr` | mr-lifecycle | create/list/view/update/close/close-noop/reopen/merge/merge-noop |
| `mr` | mr-current | `current` ref resolution, `no_current_merge_request` |
| `mr` | mr-discussions | comment thread, short-id prefix, resolve/unresolve + no-op, state filter |
| `mr` | mr-comment-positioned | DiffNote positioning, file-level comment, reply-to, `line_not_in_diff`, `file_not_in_diff` |
| `mr` | mr-diff | changed-file summary, raw patch, export bundle + `--force` guard |
| `axi` | axi-error-contract | structured errors on stdout, exit 0/1/2, URL-leak check, gl stderr counterpart |
| `axi` | axi-home | content-first home view in and out of a repo |

Not yet covered (planned P1): list filters and paging exactness, update clear-semantics and usage errors, create conflict (409), drafts lifecycle, approvals via `[second-token]`, `setup hooks`/`context`, json error parity, ref-error codes.

## Pointing at a different instance

Nothing in the scripts names an instance. To run the same suite against gitlab.com, create a group + service account there and swap the three env vars. A future docker-compose GitLab CE tier can plug in the same way.
