#!/bin/sh
# Copyright (c) 2026 Lateralus Labs, LLC.
# Use of this source code is governed by the Business Source License
# included in the LICENSE file.
#
# As of the Change Date listed in the LICENSE file, this software is
# released under the Apache License, Version 2.0.
#
# g8e SessionStart context. Plain stdout from a SessionStart hook is added to
# Claude's context. Enforcement does not depend on this script; pretool.sh
# denies non-gateway tools whether or not Claude has read this.

root=${CLAUDE_PLUGIN_OPTION_G8E_ROOT:-}
app=${CLAUDE_PLUGIN_OPTION_APP_NAME:-claude}
project=${CLAUDE_PROJECT_DIR:-$(pwd)}

cat <<EOF
This Claude Code session is governed by g8e (plugin "g8e").

- The only tools that can act are the g8e Gateway MCP tools, named mcp__plugin_g8e_g8e__<tool>. Load them with ToolSearch (query "g8e") before first use.
- Bash, Read, Edit, Write, Glob, Grep, NotebookEdit, WebFetch, WebSearch, Agent, and every other non-g8e tool are denied by a PreToolUse hook. A denial is final: do not retry the same tool, and do not look for another native tool that does the same thing.
- Every g8e tool call becomes a GovernanceEnvelope that passes L1-L5 at the Gateway and the executing Operator, and is recorded as application identity spiffe://g8e.local/app/${app}.
- g8e tools run on the Operator host, not inside this conversation. The project directory is ${project}: give read_file absolute paths, and pass working_dir "${project}" to run_shell_command.
- Read files with read_file. Change files with run_shell_command; there is no separate edit tool.
- Depending on the Gateway posture, a call may wait for a human to approve it in the g8e Console (L3). If a call is rejected by doctrine or policy, report the reason to the user instead of working around it.
- If no mcp__plugin_g8e_g8e__ tools are available, say so and ask the user to run /g8e:setup. Do not fall back to native tools.
EOF

# shellcheck disable=SC2016 # '${' matches an unsubstituted ${user_config.*} placeholder.
case $root in
  '' | '${'*)
    printf '\nSetup needed: the plugin option g8e_root is not set, so the g8e MCP server cannot start. Tell the user to run /g8e:setup.\n'
    ;;
  *)
    if [ ! -d "$root/.g8e" ]; then
      printf '\nSetup needed: %s has no .g8e directory, so the g8e MCP server cannot authenticate. Tell the user to run /g8e:setup.\n' "$root"
    fi
    ;;
esac
exit 0
