<p align="center">
  <img src="assets/gl-logo.png" alt="Community GitLab CLI logo" width="220">
</p>

<h1 align="center">Community GitLab CLI</h1>

<p align="center">
  A GitLab CLI built for coding agents.
</p>

`glab` is built for people at a terminal. Community GitLab CLI is built for
coding agents.

`gl-axi` is the agent-native entry point. It follows Kun Chen's
[axi standard](https://github.com/kunchenguid/axi) and returns compact TOON,
exact counts, coded errors, and useful next commands. The CLI handles project
discovery, pagination, diff positions, and no-op checks instead of leaving that
work to the agent.

An agent can carry a merge request review end to end: find the MR for the
current branch, inspect the diff, read and resolve discussions, leave line-level
comments, react to notes, build a draft review, and publish it. The same command
set is available through `gl` with human-readable output.

## Install from source

Prebuilt packages are not available yet. For now, build the CLI from source
with [Task](https://taskfile.dev/).

### Requirements

- [Git](https://git-scm.com/)
- [Go 1.26.4 or newer](https://go.dev/doc/install)
- [Task v3](https://taskfile.dev/docs/installation)

Clone the repository and build both binaries:

```sh
git clone https://github.com/shabashab/community-gitlab-cli.git
cd community-gitlab-cli
task build
```

The binaries are written to `bin/gl` and `bin/gl-axi`. Try them in place:

```sh
./bin/gl --help
./bin/gl-axi --help
```

To install both commands into `GOBIN`, or `$(go env GOPATH)/bin` when `GOBIN`
is not set:

```sh
task install
gl --help
gl-axi --help
```

If the commands are not found after installation, add the Go binary directory
to your `PATH`. For the default Go configuration on macOS and Linux:

```sh
export PATH="$(go env GOPATH)/bin:$PATH"
```

## Choose a command

Both binaries use the same GitLab API implementation and support the same core
workflows.

| Command | Default output | Best for |
| --- | --- | --- |
| `gl` | Human-readable text | Interactive terminal use and conventional scripts |
| `gl-axi` | Compact TOON | Coding agents and token-conscious automation |

Both commands also support JSON output with `--output json`. Start with `gl`
unless you specifically want the agent-oriented output contract.

## Authenticate

Create a [GitLab personal access token](https://docs.gitlab.com/user/profile/personal_access_tokens/)
with the `api` scope for full read and write functionality, then verify and
store it for your GitLab host:

```sh
gl auth login glpat-your-token --gitlab-base-url https://gitlab.com
gl auth status
gl whoami
```

Passing a token directly can leave it in shell history. Prefer substituting it
from your password manager. You can also skip credential storage and provide
`GITLAB_TOKEN` or `GL_TOKEN` in the environment.

For a self-managed instance, replace `https://gitlab.com` with the root URL of
your GitLab instance. See [Authentication and configuration](docs/authentication.md)
for credential storage, token precedence, project selection, and host
resolution.

## Your first workflow

Run project-aware commands inside a repository whose `origin` points to GitLab:

```sh
cd my-gitlab-project

gl mr                         # list open merge requests
gl mr current                 # view the MR for the current branch
gl mr diff current            # summarize its changed files
gl mr discussions current     # list unresolved review threads
```

Create a merge request from the current branch. The target defaults to the
project's default branch:

```sh
gl mr create --title "Add search endpoint"
```

Review and comment without calculating GitLab diff positions yourself:

```sh
gl mr diff current --file internal/search.go --fields new_ranges
gl mr comment current --file internal/search.go --line 42 --body "Can we simplify this?"
```

Outside a repository, or when you want another project, pass a numeric project
ID or full path:

```sh
gl mr --project group/subgroup/project
```

See [Merge request workflows](docs/merge-requests.md) for creating, updating,
approving, merging, diff review, discussions, reactions, and draft reviews.
Every command also has focused help:

```sh
gl mr --help
gl mr comment --help
```

## Agent workflows

`gl-axi` returns compact TOON by default, structured errors on stdout, exact
list counts, and runnable `help[]` suggestions:

```sh
gl-axi mr
gl-axi mr current
gl-axi mr discussions current --order-by updated_at --sort desc
gl-axi mr diff export current --dir .gl-axi/current-review
```

Optional session integrations can add ambient merge request context to Claude
Code, Codex, and OpenCode:

```sh
gl-axi setup hooks
```

See [Agent session integrations](docs/agent-integrations.md) and the
[gl-axi output reference](docs/axi-output.md) for the complete agent contract.

## Documentation

- [Documentation index](docs/README.md)
- [Authentication and configuration](docs/authentication.md)
- [Merge request workflows](docs/merge-requests.md)
- [Agent session integrations](docs/agent-integrations.md)
- [gl-axi output reference](docs/axi-output.md)
- [Errors and exit codes](docs/errors-and-exit-codes.md)

## Contributing

Contributions and feedback are welcome. See [CONTRIBUTING.md](CONTRIBUTING.md)
for the development setup, tests, and pull request expectations.
