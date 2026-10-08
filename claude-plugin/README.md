# g8e plugin for Claude Code

Puts a Claude Code session on the g8e governed path. The g8e Gateway MCP server becomes the only way for Claude to
read, change, or run anything. Every other tool is denied.

It works in every Claude Code surface that loads plugins, including the terminal CLI and the VS Code and JetBrains
extensions.

## What it does

| Component | Behavior |
| --- | --- |
| MCP server `g8e` ([.mcp.json](.mcp.json)) | Runs `g8e mcp stdio --app <app_name>` from your g8e root. Every tool call is proxied over mTLS to the Gateway and becomes a `GovernanceEnvelope` that passes L1–L5. Tools appear as `mcp__plugin_g8e_g8e__<tool>`. |
| PreToolUse hook ([hooks/pretool.sh](hooks/pretool.sh)) | Denies every tool except the g8e tools, `ToolSearch`/`WaitForMcpServers` (needed to load MCP tools), and conversation-only tools (`Skill`, `TodoWrite`, `AskUserQuestion`, `EnterPlanMode`, `ExitPlanMode`). Unknown and future tools are denied. Fires in subagents too. |
| SessionStart hook ([hooks/session-start.sh](hooks/session-start.sh)) | Tells Claude it is governed, which tools to use, and the project path to pass to them. Flags missing setup. |
| `/g8e:setup` skill | Setup steps for the user to run in their own terminal. |

The hook does not grant anything. g8e tool calls still go through Claude Code's normal permission prompts. To skip
those prompts and rely on the Gateway's posture instead, add `"mcp__plugin_g8e_g8e"` to `permissions.allow` in your
settings.

## Requirements

- A running g8e Gateway, and in its g8e root: an enrolled CLI user (`g8e auth enroll user`) and an owner-approved
  application identity (`g8e auth enroll app claude`). `/g8e:setup` lists the steps.
- `/bin/sh` (Linux or macOS). See [Limitations](#limitations).

## Install

From GitHub:

```text
/plugin marketplace add g8e-ai/g8e
/plugin install g8e@g8e
```

From a local checkout:

```text
/plugin marketplace add /path/to/g8e
/plugin install g8e@g8e
```

For development, load it for one session only:

```bash
claude --plugin-dir /path/to/g8e/claude-plugin
```

When the plugin is enabled, Claude Code asks for its options. `/config` changes them later:

| Option | Default | Meaning |
| --- | --- | --- |
| `g8e_root` | (required) | Directory that contains `.g8e/`, where you ran `g8e gw start` and enrolled. g8e resolves credentials from its working directory, so the bridge runs from here. |
| `g8e_binary` | `g8e` | Path to the `g8e` binary, or a name on `PATH`. |
| `app_name` | `claude` | Enrolled application identity. Receipts record it as `spiffe://g8e.local/app/<app_name>`. |

Install the plugin at project scope (`claude plugin install g8e@g8e --scope project`) when only some repositories should
be governed. Installed at user scope, it governs every session.

## Plugin or launcher

`g8e mcp agent run claude` is the other way to govern Claude Code. It starts the Gateway if needed, enrolls, and
launches Claude with `--strict-mcp-config` and `--disallowed-tools`.

| | Plugin | `g8e mcp agent run claude` |
| --- | --- | --- |
| Start | Any Claude Code surface, including IDE extensions | The `g8e` command, terminal only |
| Native tools | Denied by a hook. Still listed to the model, which is told not to use them. | Partly removed from the model's tool list: Bash, Read, Write, Edit, Glob, Grep, WebSearch, WebFetch |
| Tools not in that list | Denied (allowlist) | Not disabled |
| Other MCP servers | Their tools are denied by the hook | Not loaded (`--strict-mcp-config`) |
| Setup | You run `/g8e:setup` steps once | Automatic |

Use one or the other in a session. The plugin allows only its own server's tools, so with the plugin enabled the
launcher's `mcp__g8e__*` tools are denied. A server named `g8e` in a project's `.mcp.json` could run anything, so the
plugin does not trust that name.

## Limitations

- The governance boundary is the g8e tools. The hook stops Claude Code's own tools. It does not stop other processes on
  the machine, and it does not stop you: Claude Code's `!` shell commands and the IDE are outside it.
- Claude Code lets a tool call through if the hook cannot run: `sh` missing, a crash, or a timeout. The hook uses only
  `sh` builtins after reading its input. It does not run on Windows without a POSIX `sh`.
- Native tools remain visible to the model. Claude may try one; the hook denies it and Claude is told not to retry.
  Claude Code does not let a plugin remove built-in tools from the model's list. For that, launch with the launcher, or
  add the tools to `permissions.deny` in your own settings.
- `Skill` is allowed, so Claude can load skills from other plugins. A skill cannot act except through tools, and every
  tool call it makes passes through this hook.
- `Agent` is denied. A remote subagent would run where this plugin may not be installed.

## Develop

```bash
make claude-plugin-test   # hook and bridge-wrapper tests, shellcheck, claude plugin validate
```
