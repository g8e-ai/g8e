#!/bin/sh
# shellcheck disable=SC2016 # single-quoted '${user_config.*}' is a literal placeholder input.
# Tests for the g8e Claude Code plugin scripts. POSIX sh; no Claude Code,
# Gateway, or network required.
#
# Usage: sh claude-plugin/test/run.sh

here=$(CDPATH='' cd -- "$(dirname -- "$0")" && pwd) || exit 1
plugin=$(dirname -- "$here")
pretool=$plugin/hooks/pretool.sh
session=$plugin/hooks/session-start.sh
bridge=$plugin/scripts/mcp-stdio.sh

tmp=$(mktemp -d) || exit 1
trap 'rm -rf "$tmp"' EXIT INT TERM

failures=0
count=0

fail() {
  failures=$((failures + 1))
  printf 'FAIL: %s\n' "$1"
}

# expect_gate <want-status> <label> <hook-input-json>
expect_gate() {
  count=$((count + 1))
  printf '%s' "$3" | sh "$pretool" >"$tmp/out" 2>"$tmp/err"
  got=$?
  [ "$got" -eq "$1" ] || fail "$2: exit $got, want $1 (stderr: $(cat "$tmp/err"))"
  if [ "$1" -eq 0 ] && { [ -s "$tmp/out" ] || [ -s "$tmp/err" ]; }; then
    fail "$2: pass should print nothing"
  fi
  if [ "$1" -eq 2 ] && ! grep -q 'is blocked' "$tmp/err"; then
    fail "$2: deny should explain itself on stderr"
  fi
}

# call <tool_name> [tool_input-json]: a PreToolUse input in Claude Code's field order.
call() {
  input_json=${2:-'{}'}
  printf '{"session_id":"s","cwd":"/p","permission_mode":"default","hook_event_name":"PreToolUse","tool_name":"%s","tool_input":%s,"tool_use_id":"t"}' "$1" "$input_json"
}

# --- pretool.sh: the governed path passes
expect_gate 0 "g8e gateway tool" "$(call mcp__plugin_g8e_g8e__run_shell_command '{"command":"ls"}')"
expect_gate 0 "g8e read_file" "$(call mcp__plugin_g8e_g8e__read_file '{"path":"/p/a.go"}')"
expect_gate 0 "ToolSearch" "$(call ToolSearch '{"query":"g8e"}')"
expect_gate 0 "WaitForMcpServers" "$(call WaitForMcpServers)"
for t in Skill TodoWrite AskUserQuestion EnterPlanMode ExitPlanMode; do
  expect_gate 0 "$t" "$(call "$t")"
done
expect_gate 0 "spaced JSON" '{ "tool_name" : "mcp__plugin_g8e_g8e__sys_info", "tool_input": {} }'

# --- pretool.sh: everything else is denied
for t in Bash Read Edit Write Glob Grep NotebookEdit WebFetch WebSearch Agent Task \
  PowerShell Monitor SendMessage ListMcpResourcesTool ReadMcpResourceTool SomeFutureTool; do
  expect_gate 2 "$t" "$(call "$t")"
done
expect_gate 2 "launcher server name" "$(call mcp__g8e__run_shell_command)"
expect_gate 2 "other MCP server" "$(call mcp__filesystem__write_file)"
expect_gate 2 "other plugin's g8e server" "$(call mcp__plugin_evil_g8e__run_shell_command)"
expect_gate 2 "g8e plugin, other server" "$(call mcp__plugin_g8e_other__run)"
expect_gate 2 "prefix without separator" "$(call mcp__plugin_g8e_g8e_extra)"
expect_gate 2 "bare server name" "$(call mcp__plugin_g8e_g8e)"
expect_gate 2 "empty input" ''
expect_gate 2 "malformed input" '{'
expect_gate 2 "no tool_name" '{"tool_input":{}}'
expect_gate 2 "null tool_name" '{"tool_name":null,"tool_input":{}}'
expect_gate 2 "empty tool_name" "$(call '')"
expect_gate 2 "tool_name with odd characters" '{"tool_name":"Bash Read","tool_input":{}}'

# A spoofed tool_name inside tool_input must not change the decision.
expect_gate 2 "spoof after real name" \
  '{"tool_name":"Bash","tool_input":{"tool_name":"mcp__plugin_g8e_g8e__x","command":"id"}}'
expect_gate 2 "spoof before real name" \
  '{"tool_input":{"tool_name":"mcp__plugin_g8e_g8e__x"},"tool_name":"Bash"}'
# Escaped quotes inside values are not keys.
expect_gate 2 "escaped tool_name in a value" \
  '{"tool_name":"Bash","tool_input":{"command":"echo \"tool_name\": \"mcp__plugin_g8e_g8e__x\""}}'
expect_gate 0 "tool_name as a plain value" \
  '{"tool_name":"mcp__plugin_g8e_g8e__read_file","tool_input":{"path":"tool_name"}}'

# The deny decision must hold with a minimal PATH (only sh builtins after cat).
count=$((count + 1))
printf '%s' "$(call Bash)" | env -i PATH=/usr/bin:/bin sh "$pretool" >/dev/null 2>&1
[ $? -eq 2 ] || fail "deny with minimal environment"

# --- session-start.sh
count=$((count + 1))
out=$(env -i PATH=/usr/bin:/bin CLAUDE_PROJECT_DIR=/proj CLAUDE_PLUGIN_OPTION_APP_NAME=claude sh "$session") ||
  fail "session-start exits 0"
case $out in *'mcp__plugin_g8e_g8e__'*) ;; *) fail "session-start names the tool prefix" ;; esac
case $out in *'/proj'*) ;; *) fail "session-start names the project directory" ;; esac
case $out in *'g8e_root is not set'*) ;; *) fail "session-start flags unset g8e_root" ;; esac

count=$((count + 1))
mkdir -p "$tmp/root/.g8e"
out=$(env -i PATH=/usr/bin:/bin CLAUDE_PLUGIN_OPTION_G8E_ROOT="$tmp/root" sh "$session")
case $out in *'Setup needed'*) fail "session-start is quiet when .g8e exists" ;; esac

count=$((count + 1))
out=$(env -i PATH=/usr/bin:/bin CLAUDE_PLUGIN_OPTION_G8E_ROOT="$tmp/missing" sh "$session")
case $out in *'has no .g8e directory'*) ;; *) fail "session-start flags a root without .g8e" ;; esac

# --- mcp-stdio.sh
# expect_bridge <label> <stderr-substring> <args...>
expect_bridge() {
  count=$((count + 1))
  label=$1 want=$2
  shift 2
  sh "$bridge" "$@" </dev/null >"$tmp/out" 2>"$tmp/err"
  got=$?
  [ "$got" -ne 0 ] || fail "$label: should fail"
  grep -q "$want" "$tmp/err" || fail "$label: stderr lacks '$want': $(cat "$tmp/err")"
}

expect_bridge "unset root" "g8e_root option is not set" '' g8e claude
expect_bridge "placeholder root" "g8e_root option is not set" '${user_config.g8e_root}' g8e claude
expect_bridge "root without .g8e" "has no .g8e directory" "$tmp/missing" g8e claude
expect_bridge "missing binary" "was not found" "$tmp/root" "$tmp/no-such-g8e" claude

# A stub binary proves the bridge runs from the root and passes the app name.
count=$((count + 1))
cat >"$tmp/fake-g8e" <<'EOF'
#!/bin/sh
printf '%s|%s\n' "$(pwd)" "$*"
EOF
chmod +x "$tmp/fake-g8e"
got=$(cd / && sh "$bridge" "$tmp/root" "$tmp/fake-g8e" myapp)
want="$(cd "$tmp/root" && pwd)|mcp stdio --app myapp"
[ "$got" = "$want" ] || fail "bridge exec: got '$got', want '$want'"

count=$((count + 1))
got=$(cd / && sh "$bridge" "$tmp/root" "$tmp/fake-g8e" '${user_config.app_name}')
case $got in *'--app claude') ;; *) fail "bridge defaults app name to claude: got '$got'" ;; esac

if [ "$failures" -ne 0 ]; then
  printf '%d of %d checks failed\n' "$failures" "$count"
  exit 1
fi
printf 'ok: %d checks passed\n' "$count"
