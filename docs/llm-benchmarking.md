# LLM agent benchmarking

This document specifies how to compare GitLab interfaces when they are used by
coding agents. The benchmark evaluates an **agent harness + exact model + tool
adapter** as one system. It does not claim to isolate the underlying language
model from Claude Code, Codex, prompts, permissions, helper material, or tool
configuration.

The repository contains a small executable MVP under `bench/`. It validates the
fixture, agent-driver, trace, and outcome-grading design before the complete
matrix is built.

## Questions the benchmark answers

The benchmark has two independent result families:

1. **Agent effectiveness:** did the agent complete the intended GitLab workflow
   and leave the correct remote state?
2. **Interface efficiency:** how much wall time, context, money, output, and
   tool traffic did the interface consume?

Do not collapse these into one primary score. A fast failure is not an
efficient success, and a weighted score can hide whether a result came from
correctness, latency, or price.

This follows the outcome-oriented structure used by agent benchmarks such as
[SWE-bench](https://github.com/SWE-bench/SWE-bench),
[Terminal-Bench](https://www.tbench.ai/news/announcement), and
[tau-bench](https://arxiv.org/abs/2406.12045): provision isolated state, give
an agent a task and tools, retain the trajectory, and grade the resulting
environment.

## Full experiment matrix

### Agent and model configurations

Use exact model IDs and record the actual model reported by the agent event
stream. Never rely on a moving alias and never enable an automatic fallback in
a model-comparison run.

| Agent harness | Model configuration |
| --- | --- |
| Claude Code | `claude-fable-5` |
| Claude Code | `claude-opus-4-8` |
| Codex CLI | `gpt-5.6-sol` |
| Codex CLI | `gpt-5.6-terra` |
| Codex CLI | `gpt-5.6-luna` |

The primary matrix fixes reasoning effort at `high`. Effort is a separate
factor. If it becomes important, run a follow-up sweep over the effort levels
supported by every model rather than quietly changing effort in the primary
matrix.

### GitLab adapters

| Adapter | Pinning requirement | Authentication |
| --- | --- | --- |
| `glab` | exact release and binary hash | dedicated benchmark user |
| official GitLab MCP | GitLab version plus `get_mcp_server_version` | OAuth/Dynamic Client Registration |
| community GitLab MCP | exact package release and lockfile/hash | PAT held by the MCP process |
| `gl` | repository commit and binary hash | PAT |
| `gl-axi` | repository commit and binary hash | PAT |

Use `@zereight/mcp-gitlab` as the initial community MCP implementation, but pin
an exact version in the run manifest. Changing community implementations is a
new adapter, not an in-place upgrade of an old result.

The official GitLab MCP is a beta Premium/Ultimate feature and its tool surface
depends on the GitLab release and enabled features. Run every adapter against
the same licensed GitLab instance. Capture `tools/list` before a run and derive
the common-denominator task set from the tools actually available on that
instance.

Five model configurations crossed with five adapters produce 25 primary
cells. Every reported row must include the agent CLI version, requested and
actual model, effort, adapter version, GitLab version, task suite version, and
helper condition.

## Helper-material conditions

Helper skills are part of the agent-facing product, but silently enabling them
would mix interface discoverability with documentation quality. Run explicit
conditions:

| Condition | Material available | Interpretation |
| --- | --- | --- |
| `discovery` | tool schemas, command help, and error output only | inherent discoverability |
| `native` | the helper skill or guide shipped/recommended by the adapter | supported product experience |
| `normalized` | an evaluator-authored guide with the same template and token budget for every adapter | interface comparison after controlling for documentation |

For normalized guides, use the same sections and approximately 1,200–1,500
tokens: authentication/project selection, discovery, list/view/diff, mutations,
comments/replies, error recovery, verification, and three short examples. A
guide must not contain task values, fixture identifiers, grader details, or
claims that one adapter is preferred.

Record the helper path, byte count, SHA-256 hash, whether the agent consulted
it, and the tokens added to the run. Report helper uplift as the pass-rate
difference between otherwise identical cells.

`gl-axi setup hooks` is not a helper-only condition: it injects live ambient
state. Measure hooks in a separate `native+ambient-context` condition.

The MVP supports `--helper none`, `--helper native` for `gl-axi`, and an
arbitrary `--helper-file`. It appends helper content inside a clearly delimited
prompt block. Installing skills through each agent's native skill loader is a
future adapter-level refinement.

## Task suites

### Common-denominator suite

Use this suite for comparisons across all five adapters. Include only actions
that the preflight capability probe proves every adapter can perform.

Start with 20–30 tasks distributed across:

- atomic reads: locate an MR, report state, count filtered results;
- atomic writes: create an MR or exact top-level note;
- multi-step workflows: locate, inspect, decide, mutate, and verify;
- pagination and high-cardinality lists;
- large descriptions, discussions, and diffs;
- idempotency and duplicate prevention;
- invalid inputs and recoverable errors;
- eventual consistency, only in tasks explicitly labeled as recovery tests.

### Capability suite

This measures useful coverage rather than a common surface. Include operations
such as:

- MR update, field clearing, close, reopen, and merge;
- approvals and unapprovals;
- discussion resolution and reactions;
- positioned diff comments and ranges;
- draft reviews and publication;
- diff export bundles;
- `current` reference resolution;
- structured errors, help hints, and verified no-ops.

Publish capability coverage separately. If unsupported tasks count as failures
in a workload-utility result, publish the workload weights and retain the
common-denominator leaderboard beside it.

### Stress suite for agent-oriented output

Small happy paths will not expose the intended differences between `gl` and
`gl-axi`. Seed fixtures with 50–200 MRs, page-spanning discussions, long
descriptions, a large multi-file diff with one relevant marker, definitive
empty results, and coded recoverable errors. These tasks measure whether compact
TOON, exact counts, narrow defaults, and runnable hints reduce tool calls and
context consumption.

## Trial lifecycle

Each trial owns all of its state:

```text
task specification
      |
      v
fixture provisioner ---------> fresh GitLab project, branches, MR, notes
      |                                      |
      v                                      |
agent driver -> selected CLI or MCP adapter -+
      |
      +-----------------------> complete JSONL trajectory

oracle grader -> direct GitLab API read -> assertions and score
```

1. Create a randomly named private project under a disposable benchmark group.
2. Seed repository branches, content, MRs, discussions, and distractors through
   the official GitLab API client.
3. Wait for all state required by the main task to become observable. Fixture
   setup and readiness time is not agent time.
4. Create a neutral local repository with an `origin` URL and appropriate
   current branch. Do not expose this repository's `AGENTS.md` to trials.
5. Start a fresh, nonpersistent agent session with one GitLab adapter.
6. Retain every agent event, command/tool call, stderr event, final message,
   provider usage field, and duration.
7. Grade remote state with the host-side oracle client. Do not trust the
   agent's claim that it succeeded.
8. Delete the project. A janitor separately removes leaked `gl-bench-*`
   projects after interrupted runs.

Use a dedicated benchmark account and a token scoped to the disposable
benchmark group. For the intended dedicated GitLab instance, the same token may
provision fixtures, run the selected adapter, grade results, and delete
projects. Pass `GL_BENCH_HOST` into the trial as `GITLAB_BASE_URL` and pass the
token through the environment. Separate agent and oracle tokens remain an
optional refinement when attribution or grader independence matters.

## Required container isolation

A repository-specific implementation proposal, including the Docker runner,
image layout, trial lifecycle, and delivery slices, is in
[Containerized benchmark trials](benchmark-containerization-proposal.md).

The complete harness must run **every trial in a new, disposable Docker
container**. A trial—not a model/adapter cell or a complete benchmark run—is
the isolation unit. Containers must never be reused between repetitions,
tasks, models, adapters, or helper conditions. The current host-process runner
is a development MVP and is not sufficient for publishable results.

The host orchestrator remains outside the container and owns fixture
provisioning, readiness checks, oracle grading, result aggregation, and cleanup.
The agent CLI and GitLab adapters run inside the trial container:

```text
host orchestrator
  provision fixture with oracle credential
  build minimal trial specification
            |
            v
  fresh isolated container
    neutral temporary HOME
    exact Claude/Codex CLI and model configuration
    pinned CLI or MCP adapters
    task workspace + prompt + selected helper condition only
    scoped GitLab and provider credentials from ephemeral secret mounts
    no host repository, host user config, or other trial state
            |
            v
  capture stdout/stderr/events -> destroy container
            |
            v
  host oracle grades GitLab state -> fixture cleanup
```

### Minimal context contract

The container should receive only:

- the task prompt and trial identifiers required by the task;
- a clean task repository containing the neutral Git state and files needed by
  that task;
- the pinned benchmark adapters and their runtime dependencies;
- the exact helper material for the declared `discovery`, `native`, or
  `normalized` condition;
- generated neutral agent configuration containing no user preferences,
  memories, hooks, plugins, unrelated skills, or ambient project instructions;
- the scoped GitLab token and provider authentication through ephemeral,
  read-only credential mounts loaded by the trusted image entrypoint;
  and
- normal outbound network access to the remote GitLab URL and model provider.

Do not mount the benchmark source checkout, host `HOME`, `.gitconfig`, SSH
directory, complete agent configuration directories, Docker socket, result
directories, or another trial's filesystem. Mount only the generated temporary
trial repository. Capture event streams through the container process's
stdout/stderr rather than a writable result mount.

### Container profile

Give each trial a fresh writable layer, temporary home, temporary workspace,
fixed CPU/memory/PID limits, and hard wall-time limit. Run as the non-root
host user's numeric UID/GID so files created through the workspace bind mount
remain removable on native Linux. Root hosts and non-POSIX identities are
rejected. Never use `--privileged`, host PID namespaces, or a Docker socket
mount.
Custom seccomp profiles, credential brokers, and deny-by-default egress are not
required for the dedicated benchmark environment.

Use Docker's normal bridge networking so the container can reach both the
remote GitLab URL and the model provider. Pass the configured host through as
`GITLAB_BASE_URL`; host networking, `host.docker.internal`, and Docker
service-name discovery are not needed.

### Image and execution design

Build one pinned image for each agent harness and install `gl`, `gl-axi`, and a
pinned `glab` in both. The selected adapter is enforced through the prompt,
Claude tool permissions, and the existing trajectory audit. Build or pull
images before measured trials. Record the image ID/digest, agent version,
adapter hashes, OS, and architecture.

`benchctl` supports `--isolation local|docker`. Docker is the default; `local`
remains available for fast harness development. The Docker runner:

1. validate the selected image and adapter/helper hashes;
2. create a new temporary repository and uniquely named container;
3. mount the GitLab/provider credentials and pass generated agent configuration;
4. start the container with fixed resource limits and stream stdout/stderr;
5. enforce the timeout and inspect exit/OOM state;
6. remove the container and temporary workspace on every exit path; and
7. invoke the host-side grader and fixture cleanup.

For Codex trials, the container is the sandbox boundary. `DockerRunner` adds
`codex exec --dangerously-bypass-approvals-and-sandbox` so Codex does not try
to create a nested `bwrap` namespace. Local-mode runs keep Codex's normal
`workspace-write` sandbox.

The run manifest should include the image ID/digest, Docker platform, numeric
container identity, agent and adapter versions, helper condition, resource
limits, network name, isolation mode, container startup duration, exit/OOM
state, and whether cleanup succeeded. Do not record credential values or raw
Docker environment output.

### Isolation acceptance tests

Container isolation is complete when integration tests prove:

- a task succeeds through the selected adapter and its trace is graded normally;
- every trial gets a different container, home, workspace, and fixture;
- the agent cannot see the benchmark checkout, host home, Docker socket,
  result directory, another trial, or undeclared helper material;
- filesystem changes, processes, caches, and agent sessions do not survive
  into a second trial;
- timeout, cancellation, OOM, and agent crash paths still remove every
  container, temporary workspace, and fixture; and
- host cleanup can remove container-created mode-0700 workspace content on
  native Linux, and retained containers expose no token through
  `docker inspect .Config.Env`; and
- the manifest identifies the image, adapter, task, helper condition, resource
  policy, and exit state without including tokens.

Treat failure of an isolation assertion as a harness failure, never as a model
or adapter benchmark result.

## Grading

Prefer deterministic graders:

- direct GitLab object/state equality;
- exact comment counts and bodies;
- existence and absence checks;
- expected MR state, labels, assignees, or branches;
- repository artifact hashes;
- negative assertions for extra MRs, notes, or mutations.

Read-only tasks may require a structured final answer. Give both agent drivers
the same JSON schema and grade exact fields. The MVP uses simple required-string
checks for its two read-only tasks; replacing those with schema-constrained
answers is part of the next iteration.

Use an LLM grader only for genuinely subjective review quality. Blind it to the
adapter/model identity, calibrate it against human reviewers, and never let it
replace deterministic state checks.

## Metrics and statistics

Primary metrics:

- pass@1 / per-trial task success;
- pass^3 or pass^5 for repeated-run reliability;
- macro-average across tasks;
- task-level bootstrap 95% confidence intervals;
- unsupported-task rate, reported separately.

Efficiency metrics:

- end-to-end agent wall time;
- provider/API time when reported;
- adapter initialization and tool-call time;
- input, cached-input, cache-creation, output, and reasoning tokens;
- provider cost;
- total and failed tool calls;
- recovery attempts;
- bytes returned to the model;
- MCP tool count and `tools/list` schema bytes;
- cost per successful task and wall time per successful task.

Report efficiency across all trials and conditional on success. Otherwise a
cell that fails quickly looks artificially cheap. Prefer accuracy/cost and
accuracy/latency Pareto charts over a single composite leaderboard.

Run multiple trials because agent behavior is nondeterministic. Begin with 12
tasks x 3 trials x 25 cells (900 trials) to validate the harness. A publishable
run can use 20–30 tasks and five trials. Randomize or block-interleave cells by
task and trial so time-of-day, GitLab load, and provider rate limits do not
systematically favor one adapter.

Classify failures:

- invalid parameters, wrong tool selection, repeated bad calls, and task
  timeout are agent/interface failures;
- provider 5xx, GitLab 5xx, and transport loss are infrastructure-invalid
  trials and may be rerun under one uniform policy;
- rate limiting is reported separately and must not be silently retried;
- model refusal is its own outcome and must not trigger a fallback model.

## Mechanical interface benchmark

Run a non-agent microbenchmark separately:

- cold process or MCP startup;
- MCP initialize and `tools/list` size;
- cold and warm authenticated reads;
- output bytes for equivalent list, view, and diff operations;
- peak RSS and CPU if useful;
- model-facing round trips and error-payload size.

Keep an MCP server alive for a trial, matching normal usage. CLI startup occurs
per invocation. Report startup and steady-state numbers separately.

## MVP implementation

The MVP validates the architecture for Claude Code and Codex against `gl`,
`gl-axi`, and `glab`. It provides three tasks:

| Task | Setup and grading |
| --- | --- |
| `find-mr` | find a uniquely titled MR; final answer must contain IID/source/target |
| `inspect-diff` | inspect the MR; final answer must contain path and exact seeded marker |
| `comment-mr` | create one exact comment; direct API grader requires exactly one copy |

Every task gets a fresh project and MR. The fixture is seeded through
`client-go`, the local repo contains only project-discovery metadata, and raw
Claude/Codex event streams plus normalized `trials.jsonl`, `manifest.json`, and
`summary.json` are written under `bench/results/<run-id>/`. The manifest records
the Docker runtime, image, resource profile, source revision, dirty state, and
prompt/helper hashes without recording credential values.

Before traces and normalized results are written, the harness replaces the
GitLab token, selected provider credential, and token values from Codex's auth
file with `[REDACTED]`. A final typed redaction pass covers policy violations,
grade failures, execution errors, cleanup errors, and terminal diagnostics so
late error construction cannot reintroduce a credential.

`summary.json` aggregates normalized input, cached input, cache-creation,
output, reasoning, and turn counts. It records both summed agent execution
time (`agent_duration_ms`) and end-to-end run time (`wall_duration_ms`). Claude
cost is summed from the CLI's provider-reported values. Codex CLI does not
report USD cost, so the harness estimates it from the recorded token counters
and a versioned API list-price table for `gpt-5.6-sol`, `gpt-5.6-terra`, and
`gpt-5.6-luna`; `cost_source` distinguishes those cases. The embedded
`pricing` object makes the estimate auditable. It is an API-list-price
equivalent, not necessarily an incremental charge when Codex is authenticated
through a subscription. Reasoning tokens are already included in output
tokens and are not charged twice.

The static Codex estimator cannot reconstruct per-request long-context
surcharges from the CLI's run-level token totals. The pricing metadata marks
this limitation explicitly. Runs using an unknown Codex model retain token and
duration totals but use `cost_source: "unavailable"` until its pricing is added.
Their `cost_usd` value is `null` rather than a misleading zero.

### Environment

Use a disposable group and account. Dedicated benchmark variables take
precedence; the MVP falls back to the equivalent `GL_E2E_*` variables for a
quick validation against the existing test instance.

```sh
export GL_BENCH_HOST=https://gitlab.example.com
export GL_BENCH_TOKEN=glpat-...
export GL_BENCH_GROUP=gl-benchmark
```

Alternatively, place the values in the gitignored `.benchmark.env` file at the
repository root. `task benchmark:preflight` and `task benchmark:run` load it
automatically:

```sh
# .benchmark.env
GL_BENCH_HOST=https://gitlab.example.com
GL_BENCH_TOKEN=glpat-...
GL_BENCH_GROUP=gl-benchmark
```

The token needs permission to create and delete projects in the group. Docker
trials use account authentication by default:

```sh
# Codex: either export a trusted automation token, or allow the default
# ~/.codex/auth.json file-backed ChatGPT login to be copied per trial.
export CODEX_ACCESS_TOKEN=...

# Claude: generate with `claude setup-token` from a subscribed account.
export CLAUDE_CODE_OAUTH_TOKEN=...
```

`CODEX_API_KEY` and `ANTHROPIC_API_KEY` remain compatibility fallbacks. Only
the selected agent's provider credential is passed to a container. Credential
values are staged in mode-0600 files, mounted read-only outside `/workspace`,
loaded by an allowlisted entrypoint without shell evaluation, and deleted when
the container stops. They are never stored in Docker's configured environment.

### Commands

```sh
task benchmark:list
task benchmark:images

task benchmark:preflight -- --agent claude --model claude-opus-4-8 --tool gl-axi
task benchmark:preflight -- --agent codex --model gpt-5.6-sol --tool gl-axi

task benchmark:run -- \
  --agent claude \
  --model claude-opus-4-8 \
  --tool gl-axi \
  --tasks find-mr,inspect-diff,comment-mr \
  --trials 1 \
  --helper native

task benchmark:run -- \
  --agent codex \
  --model gpt-5.6-sol \
  --tool gl-axi \
  --tasks find-mr,inspect-diff,comment-mr \
  --trials 1 \
  --helper none

task benchmark:isolation-test
task benchmark:clean -- --dry-run
```

Add `--fail-on-task-failure` for a smoke run that should fail the shell command
when any grader fails. By default, a completed evaluation exits successfully
and records task failures as benchmark data.

### MVP limitations before publishing results

- Docker is the default and creates one container, home, and workspace per
  trial. `--isolation local` is retained only for harness development and is
  not valid for published comparisons.
- Official and community MCP adapters are documented but not wired into the
  executable runner yet.
- Claude command permissions restrict GitLab operations to the selected CLI.
  Docker-isolated Codex runs rely on the container boundary plus trajectory
  auditing to detect raw HTTP or another GitLab CLI; local-mode Codex retains
  its workspace sandbox with network enabled.
- Claude runs in safe mode and Codex ignores user config. Both use fresh
  container homes and pinned image installations recorded in the run manifest.
- GitLab and provider credentials are loaded into the trial process from
  short-lived read-only mounts. Running untrusted tasks against shared
  infrastructure would still require a different credential policy.
- Read-only graders use exact string containment instead of a shared structured
  final-answer schema.
- Trials are sequential and reports contain raw results rather than confidence
  intervals or charts.
- `task benchmark:clean` sweeps old labeled containers, benchmark-owned
  temporary workspaces, and `gl-bench-*` projects after interrupted runs.
- Native helper material is prompt-delivered rather than installed through the
  agent's actual skill loader.
- Raw tool traces can contain the configured GitLab host, group, project URLs,
  usernames, and repository content. Results are gitignored, but a redaction
  pass is required before publishing them.

These limitations are intentionally visible in every result. The MVP is for
validating fixtures, prompts, traces, usage parsing, and graders—not for making
final comparative claims.
