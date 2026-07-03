# Agent Session Integrations

`gl-axi` can register itself into an agent's session lifecycle so every conversation starts with relevant GitLab state already visible — the AXI standard's "ambient context" pattern. Two complementary paths exist: SessionStart hooks (live state, loads every session) and an installable Agent Skill (static guidance, loads on demand). One of them is enough; installing both is fine.

## `gl-axi setup hooks`

Installs or repairs SessionStart integrations for the three default agent apps. Implementation lives in `internal/agenthooks`.

```
$ gl-axi setup hooks
hooks[4]{app,path,status}:
  claude-code,~/.claude/settings.json,installed
  codex,~/.codex/hooks.json,installed
  codex-config,~/.codex/config.toml,updated
  opencode,~/.config/opencode/plugins/axi-gl-axi.js,installed
help[2]: Restart your agent session to receive gl-axi ambient context,...
```

Per-target behavior:

| App | File | What is written |
| --- | ---- | ---------------- |
| Claude Code | `~/.claude/settings.json` | a `hooks.SessionStart` group with one `{type: "command", command, timeout: 10}` entry |
| Codex | `~/.codex/hooks.json` | same SessionStart JSON shape |
| Codex | `~/.codex/config.toml` | `hooks = true` under `[features]` (line-based edit, rest of file untouched) |
| OpenCode | `~/.config/opencode/plugins/axi-gl-axi.js` | a generated plugin that runs the hook command once per session and injects its stdout into system context |

Guarantees:

- **Portable command.** The hook command is the bare binary name (`gl-axi context`) when PATH resolves it to the current executable, otherwise the absolute path. Global installs stay portable; ad-hoc builds still point at the right binary.
- **Idempotent.** Rerunning with the same path reports `unchanged` for every target and writes nothing.
- **Path repair.** Managed entries are recognized by the `gl-axi` marker inside the command; if the binary moved or was reinstalled, the entry is updated in place (`updated`).
- **Unmanaged config is never touched.** Unrelated settings keys, other hooks, and other TOML sections are preserved byte-for-byte in meaning. An existing OpenCode plugin file without the managed marker is refused (`error: refusing to overwrite unmanaged plugin file`), not overwritten.
- **Per-target errors are reported, not fatal.** Each row carries its own status; one broken target does not abort the others.

## `gl-axi context`

The command the hooks run. Prints a compact digest of the GitLab repository in the current working directory:

```
project: group/app
merge_requests[3]{iid,title,state,author}:
  ...
count: 3 of 3 total open
help[1]: Run `gl-axi mr view <iid>` for merge request details
```

Contract — ambient context must never break or spam a session:

- **Directory-scoped**: shows only the current repo's state; caps at 5 merge requests (this output loads on every session, so it stays ruthlessly minimal).
- **Silent on any failure**: no GitLab origin, no resolvable credential, unreachable host — all produce empty stdout and exit 0. A session hook that errors would inject noise into every conversation; silence is the correct failure mode here.
- Hints inside context output always include the binary name (`gl-axi mr ...`), because the agent reads them outside any CLI invocation.

## Agent Skill

A static, installable skill lives at [`.agents/skills/gl-axi/SKILL.md`](../.agents/skills/gl-axi/SKILL.md):

```sh
npx skills add shabashab/community-gitlab-cli --skill gl-axi
```

It documents the command surface, output contract, and auth flow with trigger-shaped frontmatter so agents load it when a GitLab task appears. Compared to the hook it costs no per-session tokens and works in any agent supporting the skill format, but it cannot show live state. Keep it in sync when the command surface changes.

## Choosing a path

- Working repeatedly in GitLab repos with a hook-capable agent (Claude Code, Codex, OpenCode): install the hooks — the agent starts every session already knowing the open merge requests.
- Occasional GitLab work, or an agent without hook support: install the skill.
- Both is fine; they do not conflict.
