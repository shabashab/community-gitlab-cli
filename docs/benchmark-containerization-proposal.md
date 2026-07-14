# Proposal: containerized benchmark trials

- Status: implemented for the CLI-adapter MVP
- Goal: give every benchmark trial a fresh, repeatable filesystem and process
  environment
- Related design: [LLM agent benchmarking](llm-benchmarking.md)

## Decision

Run each trial in one new Docker container and remove it when the trial ends.
Keep fixture provisioning, readiness checks, grading, result aggregation, and
GitLab project cleanup in the host `benchctl` process.

Pass the trial's remote GitLab URL, model, and selected adapter as non-secret
configuration. Stage GitLab and provider credentials in ephemeral read-only
mounts that the trusted entrypoint loads into the trial process. This is
appropriate for the intended environment: a dedicated GitLab instance and
tokens scoped to disposable benchmark projects.

The implementation does not need credential brokers, restricted egress
proxies, custom RPC protocols, or a separate adapter container. Docker is being
used to prevent one trial's files, agent configuration, sessions, and processes
from affecting another trial—not as a hostile-code security boundary.

## Architecture

```text
host benchctl
  provision disposable GitLab fixture
  create neutral trial repository
  build prompt and helper condition
             |
             v
  fresh agent container
    exact Claude Code or Codex version
    gl, gl-axi, and pinned glab
    fresh HOME and workspace
    GitLab/provider credentials from ephemeral secret mounts
    normal network access to GitLab and the model provider
             |
             v
  capture stdout, stderr, events, exit state
  remove container and temporary workspace
             |
             v
  host oracle grades remote state
  delete GitLab fixture
```

The isolation unit is one task repetition. Never reuse a container between
tasks, trials, models, adapters, or helper conditions.

## What should be isolated

Each trial should receive only:

- the neutral Git repository created for its fixture;
- its prompt and selected helper material;
- the selected agent and model configuration;
- the benchmark GitLab host and scoped token;
- the provider authentication required by the agent CLI; and
- the benchmark adapters installed in the image.

Each trial should have a new:

- container writable layer;
- `HOME` directory;
- agent session and cache directory;
- workspace directory;
- generated agent configuration; and
- container name and ID.

Do not mount the project source checkout, host home, global Claude/Codex config,
SSH directory, Docker socket, result directory, or another trial's workspace.
The generated neutral repository is the only host directory that needs to be
mounted, and it is deleted after the trial.

It is acceptable for the agent process to receive benchmark tokens in its
environment after the trusted entrypoint reads them. Secret values must not be
stored in Docker's configured environment, labels, names, argv, traces, or run
manifest.

## Images

Build two agent images:

```text
bench/docker/Dockerfile.codex
bench/docker/Dockerfile.claude
```

Each image should contain:

- one exact agent CLI version;
- Git and CA certificates;
- `gl` and `gl-axi` built from the repository revision under test;
- one exact `glab` release; and
- a small entrypoint that invokes the existing agent command.

The images can contain all three CLI adapters. The prompt and Claude tool
permissions select the requested adapter, while the existing trajectory audit
marks use of a different CLI or raw HTTP as a policy failure. This avoids an
unnecessary agent-by-adapter image matrix.

Build images once before a benchmark run, not during a measured trial. Record:

- image ID and, when available, registry digest;
- Dockerfile hash;
- agent version;
- adapter versions and binary hashes;
- repository commit and dirty-worktree state; and
- image OS and architecture.

Exact package versions matter for repeatability. Digest pinning is useful for
published or long-lived comparisons, but a locally built image ID is sufficient
for development runs.

## Docker runner

Add an execution abstraction around the current `ExecuteAgent` seam:

```go
type AgentRunner interface {
	Run(context.Context, AgentConfig) (AgentResult, RuntimeMetadata, error)
}
```

Implement:

- `LocalRunner`, containing the current host-process behavior; and
- `DockerRunner`, which creates one container, attaches stdin/stdout/stderr,
  waits, inspects the exit state, and removes the container.

Keep the Claude/Codex argv construction and event parsers shared between both
runners. The Docker runner changes where the command executes, not how its
events are interpreted.

Use the installed Docker CLI through `exec.CommandContext` initially. It avoids
adding the Docker Go SDK to this opt-in harness and automatically respects the
active Docker context. Construct commands as argv slices rather than shell
strings, and wrap Docker command execution behind a small interface for unit
tests.

## Trial container configuration

The container configuration should be roughly equivalent to:

```text
docker create
  --name gl-bench-<run>-<task>-<trial>
  --label community-gitlab-cli.benchmark=true
  --label community-gitlab-cli.run=<run-id>
  --label community-gitlab-cli.trial=<trial-id>
  --workdir /workspace
  --user <host-uid>:<host-gid>
  --init
  --memory 2g
  --memory-swap 2g
  --cpus 2
  --pids-limit 256
  --tmpfs /home/bench:rw,size=256m,uid=<host-uid>,gid=<host-gid>,mode=0700
  --env HOME=/home/bench
  --env GITLAB_BASE_URL=<GL_BENCH_HOST>
  --env GL_TOKEN=
  --env DISABLE_AUTOUPDATER=1
  --mount type=bind,src=<temporary-trial-repo>,dst=/workspace
  --mount type=bind,src=<mode-0600-secret-file>,dst=/run/secrets/benchmark.env,readonly
  --network bridge
  <agent-image>
  <agent-command...>
```

Put `CODEX_API_KEY`, `ANTHROPIC_API_KEY`, or other provider credentials only in
the matching agent's secret file. The entrypoint accepts a fixed credential-key
allowlist and never evaluates file content as shell code. For account-based
auth, mount the minimum mode-0600 auth file and copy it into the private tmpfs
home rather than mounting the complete host agent directory. Delete all staged
secret sources after the container stops.

Resolve the host's numeric POSIX UID/GID once and use it at every trial and
preflight callsite. Reject UID 0 and non-numeric identities. Matching the bind
mount owner keeps container-created files host-removable without recursive
world-writable permissions or privileged cleanup helpers.

The fixed CPU, memory, and PID limits are primarily for comparable measurements
and runaway-process protection. Calibrate them with smoke runs, then keep the
same values for every cell in a comparison.

Codex must not start its internal `bwrap` sandbox inside the trial container:
the nested user namespace is unavailable on common Docker runtimes. The Docker
runner therefore adds `--dangerously-bypass-approvals-and-sandbox` to `codex
exec`. This is scoped to `DockerRunner`; `LocalRunner` retains Codex's
`workspace-write` sandbox. The fresh container, host-matched unprivileged UID,
resource limits, tmpfs home, and single workspace mount remain the outer
isolation boundary.

A read-only root filesystem, capability changes, custom seccomp, and egress
firewalls are deliberately out of scope unless a later deployment requires
them. Docker's normal container namespaces plus fresh per-trial storage are
enough for this benchmark environment.

## Remote GitLab connectivity

Pass `GL_BENCH_HOST` unchanged into the container as `GITLAB_BASE_URL`. The
remote GitLab URL should resolve and be reachable through Docker's normal
outbound bridge networking, just like the model-provider endpoint. No host
networking, `host.docker.internal` alias, or Docker service-name discovery is
needed.

Preflight should run GitLab authentication and one lightweight provider request
from the actual agent image. This verifies container DNS, outbound networking,
TLS, the configured remote URL, and both credentials before fixture creation.

For a self-managed instance using a private CA, mount that CA file read-only or
include it in the local image and record its hash.

## Exact trial lifecycle

For every task repetition:

1. Provision and stabilize the GitLab fixture on the host.
2. Create a new host temporary directory and neutral Git repository with only
   the fixture's origin and current branch.
3. Build the prompt and declared helper condition.
4. Create a uniquely named container with labels, resource limits, fresh home,
   trial repository, non-secret configuration, ephemeral credential mounts,
   and the selected image.
5. Start it attached, send the prompt on stdin, and stream stdout/stderr into
   the existing event capture.
6. Enforce the trial timeout. On cancellation or timeout, kill the container.
7. Inspect the container before removal to record exit code, timestamps, and
   whether it was OOM-killed.
8. Remove the container and temporary repository using a cleanup context that
   is independent of the canceled trial context.
9. Grade through the host GitLab client and delete the fixture.

Do not use `docker run --rm` for the first implementation because the runner
needs to inspect exit and OOM state before removal. Use `docker create`,
`docker start --attach --interactive`, `docker inspect`, and `docker rm -f`.

Container startup and teardown belong to infrastructure duration. Preserve the
existing agent duration and wall duration, and add `container_startup_ms` plus
the inspected exit metadata.

## Configuration

Add these flags:

```text
--isolation local|docker     default docker; local is for harness development
--codex-image <ref>          default local benchmark image
--claude-image <ref>         default local benchmark image
--codex-auth-file <path>     default $CODEX_HOME/auth.json
--keep-container             debugging only
```

`--keep-container` retains the stopped container and workspace only after all
staged secret sources are deleted. The retained container is safe to inspect
but intentionally not restartable without re-provisioning credentials. The
workspace can still contain repository and agent-created content; remove it
with `task benchmark:clean` when debugging is complete.

The existing GitLab variables can remain unchanged:

```text
GL_BENCH_HOST
GL_BENCH_TOKEN
GL_BENCH_GROUP
```

The same scoped token may provision fixtures, run the adapter, grade results,
and clean up projects on the dedicated local instance. Supporting separate
oracle and agent tokens can remain an optional future configuration.

Suggested Taskfile commands:

```text
task benchmark:images
task benchmark:preflight -- --isolation docker --agent codex --tool gl-axi --model <exact-model-id>
task benchmark:run -- --isolation docker ...
task benchmark:isolation-test
task benchmark:clean
```

`benchmark:clean` should remove old labeled containers and leaked
`gl-bench-*` projects after interrupted runs.

## Manifest additions

Record enough runtime information to explain and repeat a result:

- isolation mode;
- Docker client/server version and active context;
- image ID/digest, OS, and architecture;
- container ID and name;
- host-matched numeric container UID/GID;
- requested resource limits;
- agent and adapter versions;
- Docker network mode and remote GitLab URL without tokens;
- start, finish, exit, timeout, and OOM state;
- helper and prompt-template hashes; and
- cleanup success.

Do not record environment-variable values, provider auth files, staged secret
paths, or Docker inspection output containing the environment.

## Isolation tests

The Docker integration tests should focus on trial independence:

1. A fake agent creates files in its home and workspace; the next trial cannot
   see them.
2. Two trials receive different repositories, fixture values, container IDs,
   and home directories.
3. The container cannot see the project source checkout, host home, Docker
   socket, result directory, or another active trial's workspace.
4. A normal trial captures stdout, stderr, exit status, and agent events.
5. Timeout, cancellation, agent crash, and OOM paths still remove the
   container and temporary workspace.
6. A live smoke task succeeds for both Claude and Codex through the selected
   adapter and is graded normally on the host.
7. Repeated and parallel trials leave no container, process, workspace, or
   GitLab fixture behind.
8. Container-created mode-0700 workspace paths remain host-removable on native
   Linux, and retained container configuration contains no credential value.

These are harness tests. Their failure invalidates the benchmark run rather
than counting as a model failure.

## Implementation sequence

### 1. Runner seam (implemented)

- Extract `LocalRunner` from `ExecuteAgent` without behavior changes.
- Add `AgentRunner`, `RuntimeMetadata`, and fake-runner unit tests.
- Add `--isolation local|docker`; Docker is the final default and local remains
  available for harness development.

### 2. Images and Docker execution (implemented)

- Add the two pinned agent Dockerfiles and image-build task.
- Implement create, attach, timeout, inspect, and cleanup.
- Stage GitLab/provider credentials through ephemeral read-only mounts and
  load them through the allowlisted entrypoint.
- Reproduce the existing JSONL traces and graders for the three MVP tasks.

### 3. Isolation verification and handoff (implemented; live matrix remains operator-run)

- Add the trial-independence tests and labeled-resource janitor.
- Add Docker-aware preflight and manifest fields.
- Make Docker isolation the required mode for complete comparative runs while
  preserving local mode for harness development.
- Add MCP servers to the images or generated agent config when that adapter
  work begins; they can use the same scoped GitLab credential mount.

## Definition of done

Docker support is complete when every trial gets a new container, home, and
workspace; Claude and Codex complete the current MVP tasks from the images;
the host grader still determines correctness; repeated and parallel trials do
not share state; timeout and crash paths clean up reliably; retained containers
contain no configured credentials; and the manifest identifies the exact
image, agent, adapter, identity, resources, and Docker runtime used.

## References

- [Docker run reference](https://docs.docker.com/reference/cli/docker/container/run)
  for environment, mounts, users, resource limits, networking, and lifecycle.
- [Docker resource constraints](https://docs.docker.com/engine/containers/resource_constraints/)
  for CPU and memory controls.
- [Docker bind mounts](https://docs.docker.com/engine/storage/bind-mounts/)
  for mounting the generated per-trial repository.
- [Codex non-interactive mode](https://learn.chatgpt.com/docs/non-interactive-mode)
  for `codex exec`, ephemeral runs, JSONL, environment authentication, and
  automation use.
- [Claude Code CLI reference](https://docs.anthropic.com/en/docs/claude-code/cli-usage)
  for print mode, stream JSON, model selection, permissions, and turn limits.
