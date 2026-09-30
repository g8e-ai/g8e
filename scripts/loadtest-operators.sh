#!/bin/bash
# Copyright (c) 2026 Lateralus Labs, LLC.
# Use of this source code is governed by the Business Source License
# included in the LICENSE file.
#
# As of the Change Date listed in the LICENSE file, this software is
# released under the Apache License, Version 2.0.
#
# Stand up N data Operators on this host, enroll + approve them, confirm
# they're online, bind the CLI to each one, fire one shell command at all of
# them through `g8e operator run`, and print a copy/paste-able summary.
#
# Everything goes through the real CLI: `g8e operator deploy --background`
# installs the binary into each Operator's own directory over SSH and starts
# `g8e operator start` there, `g8e auth enroll` approves it, and `g8e operator
# bind` / `run` drive it. The script does not copy, link, or launch binaries
# itself. Requires sshd on DEPLOY_HOST with key auth for the current user.
#
# Usage:
#   scripts/loadtest-operators.sh [COUNT]        # provision + run (default COUNT=5000)
#   scripts/loadtest-operators.sh cleanup        # kill + revoke + remove everything this script created
#
# Cleanup safety: every operator this script enrolls gets its platform
# enrollment request ID appended to $SANDBOX_ROOT/_manifest.tsv the moment
# it's approved. `cleanup` revokes ONLY the exact IDs in that file -- it
# never pattern-matches against the live `auth enroll list`, which is a
# gateway-wide view that includes other people's/other tools' operators
# (dashboard, ensemble, your own long-running data operators, etc). An
# earlier version of this script matched broadly and revoked a real,
# unrelated operator enrollment by accident; don't reintroduce that.
#
# Tuning (env vars, all optional):
#   SANDBOX_ROOT           Where operator dirs live (default /home/bob/sandbox/loadtest-operators)
#   G8E_BIN                Binary that drives the CLI and is deployed (default bin/g8e-<os>-<arch>, the compressed build)
#   OWNER_DIR              Dir holding the enrolled-owner CLI identity used for auth/list/run (default repo root)
#   ENDPOINT               Gateway discovery endpoint (default localhost)
#   DEPLOY_HOST            SSH host `operator deploy` targets (default localhost)
#   SSH_PORT / SSH_IDENTITY  Optional SSH port / identity file passed to `operator deploy`
#   RUN_CMD                One-shot command sent to every operator (default: hostname && date && echo LOADTEST_OK)
#   RUN_TIMEOUT             Per-operator dispatch timeout in seconds, max 300 (default 60)
#   PER_OP_RAM_MB           RAM budget per live operator process, MB (default 55; measured RSS ~40MB, padded)
#   RAM_RESERVE_MB          RAM left untouched for the Gateway/OS/other work (default 4096)
#   PER_OP_DISK_MB          Disk budget per operator beyond the binary copy, MB (default 10)
#   DISK_RESERVE_MB         Free disk left untouched on the sandbox filesystem (default 2048)
#   FORCE=1                 Skip the RAM safety cap and launch exactly COUNT regardless of risk
#   ENROLL_POLL_TIMEOUT_S   Max seconds to wait for one operator's CSR to show up as pending (default 15)
#   ENROLL_MAX_ATTEMPTS     Retries per operator on enrollment failure/timeout (default 3)
#   CONNECT_WAIT_S          Max seconds to wait for all approved operators to show status=active (default: count, min 60)
#
# Hard platform constraint this script is built around (see
# internal/constants/platform_enrollment.go): the Gateway allows at most 3
# concurrent, non-terminal "operator" enrollment requests at once, platform
# wide. Burst past that and new `operator start` processes get rejected with
# HTTP 429 and exit immediately. That is why provisioning below is a tight
# per-operator launch -> wait-for-CSR -> approve loop instead of "start
# everything, then approve everything" -- the latter reliably fails a chunk
# of the fleet. This is the one place this script is not a raw unthrottled
# loop, and it isn't optional pacing -- it's what the protocol requires.

set -uo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"

SANDBOX_ROOT="${SANDBOX_ROOT:-/home/bob/sandbox/loadtest-operators}"
OWNER_DIR="${OWNER_DIR:-$REPO_ROOT}"
ENDPOINT="${ENDPOINT:-localhost}"
DEPLOY_HOST="${DEPLOY_HOST:-localhost}"
SSH_PORT="${SSH_PORT:-}"
SSH_IDENTITY="${SSH_IDENTITY:-}"
RUN_CMD="${RUN_CMD:-hostname && date && echo LOADTEST_OK}"
RUN_TIMEOUT="${RUN_TIMEOUT:-60}"
PER_OP_RAM_MB="${PER_OP_RAM_MB:-55}"
RAM_RESERVE_MB="${RAM_RESERVE_MB:-4096}"
PER_OP_DISK_MB="${PER_OP_DISK_MB:-10}"
DISK_RESERVE_MB="${DISK_RESERVE_MB:-2048}"
FORCE="${FORCE:-0}"
ENROLL_POLL_TIMEOUT_S="${ENROLL_POLL_TIMEOUT_S:-15}"
ENROLL_MAX_ATTEMPTS="${ENROLL_MAX_ATTEMPTS:-3}"

# Deploy the compressed per-platform build (bin/g8e-<os>-<arch>, ~12MB) rather
# than the ~42MB repo-root ./g8e: `operator deploy` copies the running binary
# into every operator directory.
case "$(uname -m)" in x86_64) HOST_ARCH=amd64 ;; aarch64|arm64) HOST_ARCH=arm64 ;; *) HOST_ARCH="$(uname -m)" ;; esac
G8E_BIN="${G8E_BIN:-$REPO_ROOT/bin/g8e-$(uname -s | tr '[:upper:]' '[:lower:]')-$HOST_ARCH}"
OWNER_G8E() { (cd "$OWNER_DIR" && "$G8E_BIN" "$@"); }

# The real deploy path: install the binary into $1 on DEPLOY_HOST and start
# `operator start --working-dir $1` there, pointed at ENDPOINT.
DEPLOY_ARGS=(operator deploy --hosts "$DEPLOY_HOST" --background --endpoint "$ENDPOINT")
[ -n "$SSH_PORT" ] && DEPLOY_ARGS+=(--port "$SSH_PORT")
[ -n "$SSH_IDENTITY" ] && DEPLOY_ARGS+=(--identity "$SSH_IDENTITY")
deploy_operator() { OWNER_G8E "${DEPLOY_ARGS[@]}" --remote-dir "$1"; }
stop_operator_at() { pkill -f -- "--working-dir $1( |\$)" 2>/dev/null; }

# ---------------------------------------------------------------------------
# cleanup mode
# ---------------------------------------------------------------------------
if [ "${1:-}" = "cleanup" ]; then
  echo "Killing operator processes under $SANDBOX_ROOT ..."
  # Match on the distinctive --working-dir value, not command word order --
  # cmdline is "g8e operator start ... --working-dir <dir> ...", so a
  # pattern requiring "operator start" before the dir never matches.
  pkill -f -- "--working-dir ${SANDBOX_ROOT}/op-" 2>/dev/null
  sleep 1

  MANIFEST="$SANDBOX_ROOT/_manifest.tsv"
  if [ -f "$MANIFEST" ]; then
    n=$(wc -l < "$MANIFEST" | tr -d ' ')
    echo "Revoking $n platform enrollment(s) recorded in $MANIFEST ..."
    while IFS=$'\t' read -r rid _dir _run; do
      [ -z "$rid" ] && continue
      OWNER_G8E auth refresh >/dev/null 2>&1
      OWNER_G8E auth enroll revoke "$rid" --yes >/dev/null 2>&1
    done < "$MANIFEST"
  else
    echo "No manifest at $MANIFEST -- nothing recorded to revoke (directories will still be removed)."
  fi

  echo "Removing $SANDBOX_ROOT ..."
  rm -rf "$SANDBOX_ROOT"
  echo "Done."
  exit 0
fi

COUNT="${1:-5000}"
case "$COUNT" in
  ''|*[!0-9]*) echo "Usage: $0 [COUNT|cleanup]" >&2; exit 1 ;;
esac

if [ ! -x "$G8E_BIN" ]; then
  echo "ERROR: $G8E_BIN not found or not executable. Run 'make build' first." >&2
  exit 1
fi
if ! curl -sk --max-time 3 "http://${ENDPOINT}:8080/.well-known/g8e/pki/ca-bundle" -o /dev/null; then
  echo "ERROR: Gateway discovery endpoint not reachable at http://${ENDPOINT}:8080" >&2
  exit 1
fi

if ! ssh ${SSH_PORT:+-p "$SSH_PORT"} ${SSH_IDENTITY:+-i "$SSH_IDENTITY"} -o BatchMode=yes -o ConnectTimeout=5 "$DEPLOY_HOST" true 2>/dev/null; then
  echo "ERROR: non-interactive SSH to DEPLOY_HOST=$DEPLOY_HOST failed; 'g8e operator deploy' needs it." >&2
  echo "       Check sshd, your key (SSH_IDENTITY), and that the host is in known_hosts, e.g.:" >&2
  echo "         ssh-keyscan -H $DEPLOY_HOST >> ~/.ssh/known_hosts" >&2
  exit 1
fi

# ---------------------------------------------------------------------------
# owner CLI preflight -- every phase below drives the Gateway through the
# enrolled-owner CLI identity in $OWNER_DIR. If that identity can't talk to
# the Gateway (untrusted CA, expired cert, not enrolled), nothing downstream
# can work, and the per-call `2>/dev/null` below would hide why: the enroll
# loop would just poll an empty pending list for ~15s x attempts x COUNT.
# Prove the CLI works once, up front, and show its real error if it doesn't.
# ---------------------------------------------------------------------------
PREFLIGHT_ERR="$(mktemp)"
trap 'rm -f "$PREFLIGHT_ERR"' EXIT
if ! PREFLIGHT_LIST_JSON="$(OWNER_G8E operator list --json 2>"$PREFLIGHT_ERR")" \
   || ! jq -e . >/dev/null 2>&1 <<< "$PREFLIGHT_LIST_JSON"; then
  echo "ERROR: the owner CLI in $OWNER_DIR cannot query the Gateway, so no operators can be enrolled or approved." >&2
  echo "       'g8e operator list --json' said:" >&2
  { grep -m1 -E '^Error:' "$PREFLIGHT_ERR" || { head -3 "$PREFLIGHT_ERR"; head -3 <<< "$PREFLIGHT_LIST_JSON"; }; } | sed 's/^/         /' >&2
  if grep -q 'unknown authority' "$PREFLIGHT_ERR"; then
    echo "       The CLI's trust bundle ($OWNER_DIR/.g8e/pki/trust/g8eg-ca-bundle.pem) does not contain the running Gateway's CA." >&2
    echo "       Refresh it from the Gateway (or re-run 'g8e auth enroll user'):" >&2
    echo "         curl -s http://${ENDPOINT}:8080/.well-known/g8e/pki/ca-bundle -o $OWNER_DIR/.g8e/pki/trust/g8eg-ca-bundle.pem" >&2
  else
    echo "       Check that OWNER_DIR ($OWNER_DIR) holds an enrolled CLI identity (g8e auth status / g8e auth enroll user)." >&2
  fi
  exit 1
fi

# ---------------------------------------------------------------------------
# capacity guard -- this is the number one failure mode for this script.
# Each live operator process runs ~$PER_OP_RAM_MB of RSS. Refuse to
# knowingly overcommit RAM on a host that's running other people's work
# (the Gateway, other Operators, whatever else is live right now).
# ---------------------------------------------------------------------------
mem_avail_mb=$(awk '/MemAvailable/{print int($2/1024)}' /proc/meminfo)
safe_max=$(( (mem_avail_mb - RAM_RESERVE_MB) / PER_OP_RAM_MB ))
[ "$safe_max" -lt 0 ] && safe_max=0

ACTUAL_COUNT="$COUNT"
if [ "$FORCE" != "1" ] && [ "$COUNT" -gt "$safe_max" ]; then
  ACTUAL_COUNT="$safe_max"
  echo "WARNING: requested $COUNT operators, but only ~${mem_avail_mb}MB RAM is available right now."
  echo "         Budgeting ${PER_OP_RAM_MB}MB/operator and reserving ${RAM_RESERVE_MB}MB headroom => safe cap ${safe_max}."
  echo "         Capping this run to $ACTUAL_COUNT. Re-run with FORCE=1 to ignore this and launch $COUNT anyway."
fi
if [ "$ACTUAL_COUNT" -le 0 ]; then
  echo "ERROR: computed safe operator count is 0 (not enough free RAM). Free up memory or set FORCE=1." >&2
  exit 1
fi

# disk guard -- `operator deploy` puts a real copy of the binary in every
# operator directory (no hardlinks), so the fleet costs COUNT x binary size.
mkdir -p "$SANDBOX_ROOT"
bin_mb=$(( ($(stat -c %s "$G8E_BIN") + 1048575) / 1048576 ))
disk_avail_mb=$(df -Pm "$SANDBOX_ROOT" | awk 'NR==2{print $4}')
disk_safe_max=$(( (disk_avail_mb - DISK_RESERVE_MB) / (bin_mb + PER_OP_DISK_MB) ))
[ "$disk_safe_max" -lt 0 ] && disk_safe_max=0
if [ "$FORCE" != "1" ] && [ "$ACTUAL_COUNT" -gt "$disk_safe_max" ]; then
  echo "WARNING: ${disk_avail_mb}MB free on the sandbox filesystem; each operator needs ~$((bin_mb + PER_OP_DISK_MB))MB (${bin_mb}MB binary copy)."
  echo "         Reserving ${DISK_RESERVE_MB}MB => disk cap ${disk_safe_max}. Capping this run to $disk_safe_max."
  ACTUAL_COUNT="$disk_safe_max"
fi
if [ "$ACTUAL_COUNT" -le 0 ]; then
  echo "ERROR: computed safe operator count is 0 (not enough free disk). Free up space or set FORCE=1." >&2
  exit 1
fi

RUN_ID="$(date +%Y%m%d-%H%M%S)"
RUN_DIR="$SANDBOX_ROOT/_run-$RUN_ID"
mkdir -p "$RUN_DIR"
FAIL_LOG="$RUN_DIR/failures.log"
SESSION_FILE="$RUN_DIR/session_ids.txt"
SUMMARY_FILE="$RUN_DIR/summary.txt"
MANIFEST="$SANDBOX_ROOT/_manifest.tsv"
BASELINE_FILE="$RUN_DIR/baseline_sessions.txt"
: > "$FAIL_LOG"
: > "$SESSION_FILE"
touch "$MANIFEST"

CONNECT_WAIT_S="${CONNECT_WAIT_S:-$(( ACTUAL_COUNT > 60 ? ACTUAL_COUNT : 60 ))}"

hr() { printf '=%.0s' $(seq 1 78); echo; }
now() { date +%s.%N; }
elapsed() { echo "$(echo "$(now) - $1" | bc)"; }

hr
echo "g8e Operator Load Test -- run $RUN_ID"
echo "Requested: $COUNT   Actual: $ACTUAL_COUNT   Endpoint: $ENDPOINT"
echo "Sandbox:   $SANDBOX_ROOT"
echo "Run dir:   $RUN_DIR"
hr

T_START=$(now)

# baseline: operator sessions that exist BEFORE we start, so we can diff
# our new fleet out later without guessing. Written to a file (not a
# space-joined variable) because operator_session_ids are UUIDs and a
# shell-glob membership test over a newline-separated string silently never
# matches -- that bug previously let pre-existing operators (including
# witness-only ones that reject generic commands) leak into "our" fleet.
jq -r '.operators[]?.operator_session_id // empty' <<< "$PREFLIGHT_LIST_JSON" > "$BASELINE_FILE"

# ---------------------------------------------------------------------------
# Phase 1 -- nothing to prepare up front. `operator deploy` creates each
# per-operator directory and installs the binary into it in phase 2; this
# script does not copy or link anything itself.
# ---------------------------------------------------------------------------
echo "[1/6] Operator directories will be created by 'operator deploy' under $SANDBOX_ROOT"
T1=$(now)
PHASE1_S=$(elapsed "$T1")

# ---------------------------------------------------------------------------
# Phase 2 -- deploy + enroll + approve, one operator at a time.
# Sequential because of the platform-wide 3-live-enrollment-request cap
# (see header comment). Each operator gets up to ENROLL_MAX_ATTEMPTS tries;
# a redeploy into the same directory resumes its persisted pending request.
# ---------------------------------------------------------------------------
echo "[2/6] Deploying + enrolling + approving $ACTUAL_COUNT operators (sequential, protocol-capped) ..."
T2=$(now)
enrolled=0
enroll_failed=0
seen_ids=""
pending_errs=0

for i in $(seq 1 "$ACTUAL_COUNT"); do
  d=$(printf "%s/op-%05d" "$SANDBOX_ROOT" "$i")
  ok=0
  for attempt in $(seq 1 "$ENROLL_MAX_ATTEMPTS"); do
    # A previous attempt's worker may still be running (it just hasn't shown
    # up as pending within our poll window yet). Stop it before redeploying
    # on top of the same directory -- two processes racing over one .g8e/
    # dir is a correctness hazard, and scp cannot overwrite a running binary.
    if [ "$attempt" -gt 1 ]; then
      stop_operator_at "$d"
      sleep 0.3
    fi
    if ! deploy_out=$(deploy_operator "$d" 2>&1); then
      echo "op-$(printf '%05d' "$i"): attempt $attempt: operator deploy failed: $(tr '\n' ' ' <<< "$deploy_out" | cut -c1-300)" >> "$FAIL_LOG"
      continue
    fi
    newid=""
    deadline=$(( $(date +%s) + ENROLL_POLL_TIMEOUT_S ))
    while [ "$(date +%s)" -lt "$deadline" ]; do
      # An empty pending list means "not there yet"; a non-zero exit means the
      # control plane is unusable. Keep those apart -- conflating them is how
      # this loop used to burn minutes per operator on a broken CLI.
      if ! pending_out=$(OWNER_G8E auth enroll pending 2>&1); then
        pending_errs=$((pending_errs + 1))
        if [ "$pending_errs" -ge 5 ]; then
          echo "ERROR: 'g8e auth enroll pending' failed $pending_errs times in a row; aborting. Last error:" >&2
          echo "$pending_out" | grep -m1 -E '^Error:' >&2 || echo "$pending_out" | head -3 >&2
          echo "Operators started so far are still running; tear down with: $0 cleanup" >&2
          exit 1
        fi
        sleep 1
        continue
      fi
      pending_errs=0
      ids=$(grep -oP 'Request ID:\s+\K[0-9a-f-]{36}' <<< "$pending_out")
      for id in $ids; do
        case " $seen_ids " in
          *" $id "*) ;;
          *) newid="$id" ;;
        esac
      done
      [ -n "$newid" ] && break
      sleep 0.2
    done
    if [ -z "$newid" ]; then
      echo "op-$(printf '%05d' "$i"): attempt $attempt: no enrollment request appeared" >> "$FAIL_LOG"
      continue
    fi
    seen_ids="$seen_ids $newid"
    if OWNER_G8E auth enroll approve "$newid" --yes >/dev/null 2>&1; then
      printf '%s\t%s\t%s\n' "$newid" "op-$(printf '%05d' "$i")" "$RUN_ID" >> "$MANIFEST"
      ok=1
      break
    else
      echo "op-$(printf '%05d' "$i"): attempt $attempt: approve failed for $newid" >> "$FAIL_LOG"
    fi
  done
  if [ "$ok" -eq 1 ]; then
    enrolled=$((enrolled + 1))
  else
    enroll_failed=$((enroll_failed + 1))
    echo "op-$(printf '%05d' "$i"): GAVE UP after $ENROLL_MAX_ATTEMPTS attempts" >> "$FAIL_LOG"
    # Don't leave a dangling process behind for an operator we're giving
    # up on -- it would otherwise sit there consuming RAM indefinitely.
    stop_operator_at "$d"
  fi
  if [ $((i % 100)) -eq 0 ] || [ "$i" -eq "$ACTUAL_COUNT" ]; then
    echo "      progress: $i/$ACTUAL_COUNT launched, $enrolled enrolled, $enroll_failed failed ($(elapsed "$T2")s elapsed)"
  fi
done
PHASE2_S=$(elapsed "$T2")
echo "      enroll phase done in ${PHASE2_S}s -- enrolled=$enrolled failed=$enroll_failed"

# ---------------------------------------------------------------------------
# Phase 3 -- confirm all enrolled operators are online (status=active).
# Approval doesn't mean connected: the worker polls for its issued
# credentials and then opens the pub/sub WebSocket, ~30s typical.
# ---------------------------------------------------------------------------
echo "[3/6] Waiting for approved operators to come online (up to ${CONNECT_WAIT_S}s) ..."
T3=$(now)
target_active=$enrolled
active_new=0
deadline=$(( $(date +%s) + CONNECT_WAIT_S ))
while [ "$(date +%s)" -lt "$deadline" ]; do
  current_sessions=$(OWNER_G8E operator list --json 2>/dev/null | jq -r '.operators[]? | select(.status=="active") | .operator_session_id')
  : > "$SESSION_FILE"
  active_new=0
  while IFS= read -r sid; do
    [ -z "$sid" ] && continue
    if ! grep -Fxq "$sid" "$BASELINE_FILE"; then
      echo "$sid" >> "$SESSION_FILE"
      active_new=$((active_new + 1))
    fi
  done <<< "$current_sessions"
  [ "$active_new" -ge "$target_active" ] && break
  sleep 2
done
PHASE3_S=$(elapsed "$T3")
echo "      $active_new/$target_active newly-enrolled operators confirmed active in ${PHASE3_S}s"

# ---------------------------------------------------------------------------
# Phase 4 -- bind the CLI session to each online operator via
# `g8e operator bind`, then clear the binding so the owner CLI identity is
# left as we found it. Binding reissues the CLI session server-side each time.
# ---------------------------------------------------------------------------
bind_ok=0
bind_failed=0
PHASE4B_S=0
if [ "$active_new" -gt 0 ]; then
  echo "[4/6] Binding the CLI session to each of $active_new online operators ..."
  T4B=$(now)
  while IFS= read -r sid; do
    [ -z "$sid" ] && continue
    if bind_out=$(OWNER_G8E operator bind "$sid" --yes 2>&1); then
      bind_ok=$((bind_ok + 1))
    else
      bind_failed=$((bind_failed + 1))
      echo "bind $sid: $(tr '\n' ' ' <<< "$bind_out" | cut -c1-300)" >> "$FAIL_LOG"
    fi
  done < "$SESSION_FILE"
  OWNER_G8E operator bind unbind --yes >/dev/null 2>&1 || echo "WARNING: could not clear the CLI operator binding; run '$G8E_BIN operator bind unbind --yes' in $OWNER_DIR" >&2
  PHASE4B_S=$(elapsed "$T4B")
  echo "      bind done in ${PHASE4B_S}s -- success=$bind_ok failed=$bind_failed"
else
  echo "[4/6] Skipped: no operators came online."
fi

# ---------------------------------------------------------------------------
# Phase 5 -- one-shot command fan-out via `g8e operator run`, which already
# dispatches to every listed session in parallel and waits for a terminal
# result from each. No custom batching added here on purpose.
# ---------------------------------------------------------------------------
RUN_RESULT_JSON="$RUN_DIR/run_result.json"
cmd_success=0
cmd_failed=0
PHASE4_S=0
if [ "$active_new" -gt 0 ]; then
  echo "[5/6] Dispatching one-shot command to $active_new operators: $RUN_CMD"
  mapfile -t session_ids < "$SESSION_FILE"
  T4=$(now)
  OWNER_G8E operator run "${session_ids[@]}" --cmd "$RUN_CMD" --timeout "$RUN_TIMEOUT" --json > "$RUN_RESULT_JSON" 2>"$RUN_DIR/run_stderr.log"
  PHASE4_S=$(elapsed "$T4")
  cmd_success=$(jq '[.results[] | select(.success==true)] | length' "$RUN_RESULT_JSON" 2>/dev/null || echo 0)
  cmd_failed=$(jq '[.results[] | select(.success==false)] | length' "$RUN_RESULT_JSON" 2>/dev/null || echo 0)
  echo "      dispatch done in ${PHASE4_S}s -- success=$cmd_success failed=$cmd_failed"
else
  echo "[5/6] Skipped: no operators came online."
fi

# ---------------------------------------------------------------------------
# Phase 6 -- verify + resource snapshot + copy/paste summary.
# ---------------------------------------------------------------------------
echo "[6/6] Building summary ..."
proc_pids=$(pgrep -f -- "--working-dir ${SANDBOX_ROOT}/op-" || true)
proc_count=$(echo -n "$proc_pids" | grep -c . || true)
rss_total_mb=0
if [ -n "$proc_pids" ]; then
  rss_total_mb=$(ps -o rss= -p $(echo "$proc_pids" | tr '\n' ',' | sed 's/,$//') 2>/dev/null | awk '{s+=$1} END {printf "%.0f", s/1024}')
fi
disk_used=$(du -sh "$SANDBOX_ROOT" 2>/dev/null | awk '{print $1}')
TOTAL_S=$(elapsed "$T_START")

{
  hr
  echo "g8e Operator Load Test -- Summary (run $RUN_ID)"
  hr
  echo "Host:              $(hostname)  ($(nproc) vCPU, $(awk '/MemTotal/{printf "%.0fGB", $2/1024/1024}' /proc/meminfo) RAM)"
  echo "g8e version:       $(OWNER_G8E version 2>/dev/null | head -1)"
  echo "Gateway endpoint:  $ENDPOINT"
  echo
  echo "Requested operators:  $COUNT"
  echo "Actual operators:     $ACTUAL_COUNT $( [ "$ACTUAL_COUNT" -lt "$COUNT" ] && echo "(capped by available RAM/disk -- see below)" )"
  echo
  echo "-- Enrollment --"
  echo "  Enrolled (approved):     $enrolled / $ACTUAL_COUNT"
  echo "  Failed after retries:    $enroll_failed"
  echo "  Confirmed online:        $active_new / $enrolled"
  echo "  Bound via operator bind: $bind_ok / $active_new"
  echo
  echo "-- One-shot command dispatch --"
  echo "  Command:  $RUN_CMD"
  echo "  Success:  $cmd_success / $active_new"
  echo "  Failed:   $cmd_failed / $active_new"
  echo
  echo "-- Timing --"
  printf "  %-30s %10.1fs\n" "Deploy + enroll + approve ($enrolled ops)" "$PHASE2_S"
  printf "  %-30s %10.1fs\n" "Online confirmation" "$PHASE3_S"
  printf "  %-30s %10.1fs\n" "CLI bind ($active_new ops)" "$PHASE4B_S"
  printf "  %-30s %10.1fs\n" "Command dispatch ($active_new ops)" "$PHASE4_S"
  printf "  %-30s %10.1fs\n" "Total wall clock" "$TOTAL_S"
  if [ "$enrolled" -gt 0 ]; then
    printf "  %-30s %10.2fs\n" "Avg deploy+enroll / operator" "$(echo "$PHASE2_S / $enrolled" | bc -l)"
  fi
  echo
  echo "-- Resource footprint --"
  echo "  Live operator processes:  $proc_count"
  echo "  Aggregate RSS:            ${rss_total_mb}MB"
  echo "  Sandbox disk used:        ${disk_used:-unknown}"
  echo "  RAM available at start:   ${mem_avail_mb}MB (safety cap computed: $safe_max operators)"
  echo "  Disk available at start:  ${disk_avail_mb}MB (safety cap computed: $disk_safe_max operators)"
  echo
  if [ "$enroll_failed" -gt 0 ] || [ "$cmd_failed" -gt 0 ] || [ "$bind_failed" -gt 0 ]; then
    echo "-- Failures (see $FAIL_LOG and $RUN_RESULT_JSON for detail) --"
    [ "$enroll_failed" -gt 0 ] && echo "  $enroll_failed operator(s) never enrolled."
    [ "$bind_failed" -gt 0 ] && echo "  $bind_failed operator(s) failed to bind."
    [ "$cmd_failed" -gt 0 ] && jq -r '.results[] | select(.success==false) | "  \(.operator_session_id): \(.error)"' "$RUN_RESULT_JSON" 2>/dev/null | head -20
    echo
  fi
  echo "Artifacts: $RUN_DIR"
  echo "Teardown:  $0 cleanup"
  hr
} | tee "$SUMMARY_FILE"
