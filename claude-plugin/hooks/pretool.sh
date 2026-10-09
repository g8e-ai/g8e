#!/bin/sh
# Copyright (c) 2026 Lateralus Labs, LLC.
# Use of this source code is governed by the Business Source License
# included in the LICENSE file.
#
# As of the Change Date listed in the LICENSE file, this software is
# released under the Apache License, Version 2.0.
#
# g8e PreToolUse gate.
#
# Passes the plugin's g8e Gateway tools and Claude Code's conversation-only
# tools. Denies everything else, including tools a future Claude Code release
# adds. Passing means "no decision": the call continues through Claude Code's
# normal permission flow. Denying beats any other hook's allow and every
# permission mode, including bypassPermissions.
#
# Claude Code lets a call through when its hook crashes, times out, or exits
# with any status other than 2. Every deny path therefore ends in `exit 2`, and
# nothing after reading stdin forks a process.

deny() {
  printf 'g8e: %s is blocked. This session is governed by g8e; act only through the g8e Gateway tools (mcp__plugin_g8e_g8e__*). Do not retry this tool.\n' "$1" >&2
  exit 2
}

input=$(cat) || deny "(unreadable hook input)"

# Collect every value keyed "tool_name". Inside a JSON string a quote is
# escaped, so the literal `"tool_name"` followed by `:` only appears as a key.
# The top-level key is the real tool. If tool_input also carries a
# "tool_name" key with a different value, the input is ambiguous and denied.
tool=
seen=0
rest=$input
while :; do
  case $rest in
    *'"tool_name"'*) ;;
    *) break ;;
  esac
  rest=${rest#*'"tool_name"'}
  after=${rest#"${rest%%[! 	]*}"}
  case $after in
    :*) ;;
    *) continue ;;
  esac
  after=${after#*'"'}
  name=${after%%'"'*}
  if [ "$seen" -eq 0 ]; then
    tool=$name
    seen=1
  elif [ "$name" != "$tool" ]; then
    deny "(ambiguous tool name)"
  fi
done

case $tool in
  '' | *[!A-Za-z0-9_.:-]*) deny "(unrecognized tool)" ;;
esac

case $tool in
  # The governed path. Plugin MCP tools are mcp__plugin_<plugin>_<server>__<tool>.
  mcp__plugin_g8e_g8e__*) exit 0 ;;
  # Load and wait for deferred MCP tools; without these the g8e tools are unreachable.
  ToolSearch | WaitForMcpServers) exit 0 ;;
  # Conversation-only tools. None of them reads, writes, runs, or fetches anything.
  Skill | TodoWrite | AskUserQuestion | EnterPlanMode | ExitPlanMode) exit 0 ;;
esac

deny "$tool"
