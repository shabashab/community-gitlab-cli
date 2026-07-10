# Authentication and configuration

Community GitLab CLI authenticates to GitLab's personal access token API. It
can store a token for each GitLab host or read one from the environment for
ephemeral use.

## Create a personal access token

Create a token from your GitLab profile's
[Personal access tokens](https://docs.gitlab.com/user/profile/personal_access_tokens/)
page.

- Use the `api` scope for the complete CLI, including merge request mutations,
  approvals, comments, and reactions.
- A `read_api` token is enough only when you intend to use read-only commands.
- Give the token the shortest practical lifetime and store it in a password
  manager.

Your GitLab role and project permissions still control which resources the
token can access.

## Store a credential

`auth login` verifies the token against the selected GitLab host before storing
it:

```sh
gl auth login glpat-your-token --gitlab-base-url https://gitlab.com
gl auth status
gl whoami
```

`--gitlab-base-url` is required for login and must be the root URL of the
instance, not a project URL. For GitLab Self-Managed:

```sh
gl auth login glpat-your-token --gitlab-base-url https://gitlab.example.com
```

Passing the token directly can leave it in shell history. Prefer substituting
it from your password manager, for example:

```sh
gl auth login "$(pass show gitlab/token)" --gitlab-base-url https://gitlab.com
```

`gl-axi auth login` stores the same credential and differs only in its output
format.

### Where credentials are stored

The CLI stores credentials in the operating system keychain when one is
available:

- macOS Keychain
- Windows Credential Manager
- Linux Secret Service

On systems without a usable keychain, it falls back to an encrypted file at
`~/.gl/credentials.json`. The directory is created with mode `0700` and the
file with mode `0600`. The fallback protects tokens from casual plaintext
scraping, but it is not a replacement for host security.

Set `GL_CREDSTORE=file` to bypass the keychain and use the encrypted file
directly. This is useful on headless machines where keychain access prompts or
hangs:

```sh
GL_CREDSTORE=file gl auth login glpat-your-token \
  --gitlab-base-url https://gitlab.example.com
```

Remove the credential selected for the current host with:

```sh
gl auth logout
```

Logging out when no credential is stored is a successful no-op.

## Use a token without storing it

For a single shell or CI job, provide a token through the environment:

```sh
export GITLAB_TOKEN=glpat-your-token
gl whoami
```

`GL_TOKEN` is also supported. A token can be passed to one command with
`--gitlab-token`, although command-line arguments may be visible to other local
processes or recorded in shell history.

Token sources are checked in this order:

1. `--gitlab-token`
2. `GITLAB_TOKEN`
3. `GL_TOKEN`
4. The stored credential for the resolved GitLab host

## Select a GitLab host

Project-aware commands resolve the GitLab instance in this order:

1. `--gitlab-base-url`
2. `GITLAB_BASE_URL`
3. The host from the current repository's `origin` remote
4. `https://gitlab.com`

Examples:

```sh
gl whoami --gitlab-base-url https://gitlab.example.com

GITLAB_BASE_URL=https://gitlab.example.com gl mr
```

The root instance URL and an `/api/v4` URL are both accepted. Authentication
failures are translated into concise errors without printing the request URL or
response body.

## Select a project

Inside a Git repository, project-aware commands inspect only the remote named
`origin` and derive the GitLab project path from its SSH or HTTPS URL:

```sh
git remote get-url origin
gl project info
gl mr
```

Outside the repository, or to override discovery, use `--project` with a
numeric project ID or full path:

```sh
gl project info --project 12345
gl mr --project group/subgroup/project
```

An explicit `--project` is also useful when the repository has no `origin` or
the desired GitLab project differs from it.

## Choose an output format

`gl` defaults to human-readable text:

```sh
gl mr
gl mr --output json
```

`gl-axi` defaults to compact TOON for agent workflows:

```sh
gl-axi mr
gl-axi mr --output json
```

The two binaries share API behavior. Their presentation and error channels
differ; see [Errors and exit codes](errors-and-exit-codes.md) and the
[gl-axi output reference](axi-output.md).

## Troubleshooting

```sh
gl auth status
gl whoami
gl project info
```

- If `auth status` cannot find a credential, confirm that the resolved host
  matches the host used during `auth login`.
- If a project-aware command selects the wrong project, inspect `origin` or
  pass `--project` explicitly.
- If a self-managed instance is not selected, pass `--gitlab-base-url` or set
  `GITLAB_BASE_URL`.
- For automation, use JSON or `gl-axi` and branch on the documented exit codes
  instead of parsing human-readable errors.
