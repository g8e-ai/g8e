---
title: Terminal-Bench Integration
parent: Benchmarking
---

# Terminal-Bench Integration — Project Plan

Benchmarking the g8e platform against
[terminal-bench](https://github.com/laude-institute/terminal-bench).

Status: **Proposed.** No code yet. This document defines the architecture,
phased plan, and per-step recommendations. Every claim about either side
of the integration was verified against live source before being written
down.

---

## 1. Executive Summary

Terminal-bench (tb) benchmarks autonomous agents on real terminal tasks.
Each task runs in its own Docker container, tb hands the agent a live
`TmuxSession`, the agent completes the task, and a test script inside the
container grades the result.

The g8e integration plugs in at the agent layer. We ship a `BaseAgent`
subclass that, for each tb task:

1. Injects the existing `g8e.operator` binary into the tb task container.
2. Starts the operator and binds it to a benchmark user.
3. POSTs the task prompt to g8ee's chat pipeline — the same entry point
   the dashboard uses — triggering the full governance pipeline
   (Tribunal, approval gate, Sentinel, LFAA, mTLS).
4. Responds programmatically to every approval prompt the agent
   generates, using the same HTTP endpoint a human would.
5. Returns control to tb when the agent run completes. tb scores the
   container's final state.

**Four verified facts anchor the design.** Each is load-bearing; if any
turns out wrong, the design changes.

| # | Fact | Evidence |
|---|------|----------|
| 1 | `TmuxSession.container` is a `docker.models.containers.Container`. The agent has full Docker API on the tb task container (`exec_run`, `put_archive`, `get_archive`). | `terminal_bench/terminal/tmux_session.py` |
| 2 | `g8e.operator` is a self-contained static binary, already produced by `./g8e operator build`, living at `/home/g8e/g8e.operator` in g8ep. | `@/home/bob/g8e/docs/components/g8ep.md:14-18` |
| 3 | g8ep has Python 3.13, `docker-cli`, docker socket, and resolves `g8e.local` — everything tb's harness needs to run. | `@/home/bob/g8e/docs/components/g8ep.md:44-165` |
| 4 | The approval-respond endpoint is session-cookie gated (not per-request WebAuthn). The existing `./g8e` CLI already converts an API key into a session via `POST /api/auth/operator`. | `@/home/bob/g8e/components/g8ed/routes/operator/operator_approval_routes.js:40`; `@/home/bob/g8e/g8e:212-281` |

---

## 2. Non-Goals

- **No autonomy-bypass flag.** Approval cannot be disabled in shipping
  code. The benchmark harness authenticates as a scoped identity and
  responds to approvals via the public endpoint, exactly as a human
  would.
- **No reimplementation of the operator protocol.** We inject the
  existing Go binary, we do not write a Python operator.
- **No reimplementation of the chat pipeline.** The adapter talks to the
  same HTTP API the dashboard talks to.
- **No modifications to shipping components** (`g8ee`, `g8ed`, `g8eo`,
  `g8ep`). Everything lives under `benchmarks/terminal-bench/`.
- **No intent-grant coverage** in v1. terminal-bench-core tasks are
  local-shell; intent-grant is AWS/GCP. Agent will never request one.

---

## 3. Architecture

### 3.1 Topology

```
     Host (docker compose)
  ┌─────────────────────────────────────────────────────────────┐
  │                                                             │
  │   g8es  ←  g8ee  ←  g8ed       (control plane — unchanged)  │
  │                       ▲                                     │
  │                       │  HTTPS (session cookie)             │
  │                       │                                     │
  │        ┌──────────────┴──────────────────────┐              │
  │        │                                     │              │
  │        │        tb harness (in g8ep)         │              │
  │        │                                     │              │
  │        │   tb CLI  →  G8eAgent (BaseAgent)   │              │
  │        │                    │                │              │
  │        │                    ├─ chat POST ────┘              │
  │        │                    ├─ SSE listen                   │
  │        │                    └─ approval POST                │
  │        │                                                    │
  │        └────────────────┬───────────────────────────────────┘
  │                         │ docker API (exec_run, put_archive)│
  │                         ▼                                   │
  │          ┌──────────────────────────────────┐               │
  │          │  tb task container  (per task)   │               │
  │          │  ┌──────────────────────────┐    │               │
  │          │  │ /tmp/g8e.operator        │    │               │
  │          │  │  ──mTLS WSS──► g8es      │    │               │
  │          │  └──────────────────────────┘    │               │
  │          │  tmux session + task fixtures    │               │
  │          │  attached to g8e-network         │               │
  │          └──────────────────────────────────┘               │
  └─────────────────────────────────────────────────────────────┘
```

### 3.2 Component inventory

| Component | Role | Status |
|-----------|------|--------|
| `g8e.operator` binary | Real execution in the tb task container. Speaks the existing operator pub/sub protocol. | **Reused as-is.** |
| g8ee chat pipeline (`POST /api/chat`-equivalent) | Accepts the task prompt, runs Sage/Dash + Tribunal, emits approval requests, returns the final assistant message. | **Reused as-is.** |
| `POST /api/auth/operator` (API-key mode) | Converts a benchmark API key into a web session cookie. | **Reused as-is.** |
| `POST /operator/approval/respond` | Approves/denies pending approvals. | **Reused as-is.** |
| g8ed SSE stream | Signals approval-requested events so we do not poll. | **Reused as-is.** |
| g8ep container | Host for the tb harness. Provides Python, docker-cli, docker socket, `g8e.local` DNS. | **Reused as-is.** |
| `G8eAgent` (new) | tb `BaseAgent` subclass wiring the above together. | **New, ~300 LOC.** |
| `OperatorInjector` (new) | Small helper that copies the operator binary into a tb task container and starts it. | **New, ~50 LOC.** |
| `ApprovalBridge` (new) | SSE client that auto-responds to approvals for a given investigation. | **New, ~100 LOC.** |
| `G8eChatClient` (new) | Thin HTTP client for chat-pipeline + operator-bind endpoints. | **New, ~100 LOC.** |

Total new code: ~550 lines of Python plus a Dockerfile fragment for
installing `terminal-bench` into g8ep (or a derived image).

### 3.3 Why this design is right

- Operator-in-task-container is the **faithful production shape**. tb
  tasks measure real execution outcomes; the governed binary must land
  where the test script grades.
- tb harness in g8ep keeps the whole run inside the platform's own
  network and avoids cross-boundary auth issues.
- Chat endpoint (not MCP) because MCP has a 30-second per-tool timeout
  that would fail every approval-gated command
  (`@/home/bob/g8e/docs/architecture/mcp.md:428-430`).
- Per-task container isolation is preserved (tb's default). No shared
  state between tasks, no snapshot/reset hack.

---

## 4. Phased Plan

Phases are **independently shippable**. Each has a clear deliverable, a
test of done, and per-step recommendations.

### Phase 0 — Decisions & prerequisites (no code)

**Deliverable:** This document merged, open questions resolved, one-line
agreement on §5 decisions.

| Step | Recommendation |
|------|----------------|
| 0.1 Confirm topology | Operator-in-tb-container (§3.1). Do not pursue "g8ep-as-task-host". |
| 0.2 Confirm transport | Chat-pipeline endpoint, not MCP. |
| 0.3 Pick a benchmark user identity | New dedicated user `bench@lateraluslabs.local`, created once via existing user-management script. Scopes all bench activity to one identity for audit. |
| 0.4 Pick primary model | Anthropic `claude-sonnet-4` (current tb leaderboard default) through g8ee's provider abstraction. |
| 0.5 Pick Tribunal config | Default three passes for the headline submission; document a five-pass variant as a separate label. |
| 0.6 Pick target dataset | `terminal-bench-core==0.1.1` for the first leaderboard submission; `terminal-bench-core==head` for dev iterations. |
| 0.7 Resolve open questions | See §6. |

### Phase 1 — tb harness in g8ep

**Deliverable:** `./g8e bench tb --help` lists tb's help text. `tb datasets list` works from inside g8ep.

| Step | Recommendation |
|------|----------------|
| 1.1 Install strategy | Add `terminal-bench` + its deps to a dedicated `benchmarks/terminal-bench/pyproject.toml`. Do **not** add to g8ep's base image — install at runtime into a venv so image churn stays out of shipping. |
| 1.2 Invocation | Add a single entry in the `./g8e` CLI: `./g8e bench tb <args>`. Implementation is an `exec` into g8ep that activates the bench venv and forwards to `tb`. Pattern matches existing `./g8e test g8ee` dispatch. |
| 1.3 Docker socket check | Confirm g8ep can `docker ps` against the host daemon — already true per `@/home/bob/g8e/docs/components/g8ep.md:116-124`. |
| 1.4 Smoke test | `./g8e bench tb run --agent terminus --task-id hello-world` end-to-end using tb's stock agent. Proves g8ep can host tb without any g8e-specific code. |

### Phase 2 — Benchmark user + API key bootstrap

**Deliverable:** A script that, given a fresh platform, provisions the
benchmark user and stashes a G8eKey where the harness can find it.

| Step | Recommendation |
|------|----------------|
| 2.1 User creation | Reuse `scripts/data/manage-g8es.py users create`. Idempotent. |
| 2.2 G8eKey generation | Reuse existing API-key creation path. Store in `~/.g8e/bench-credentials` (mode 600), mirroring `./g8e login` conventions (`@/home/bob/g8e/g8e:100-113`). |
| 2.3 Session mint | Adapter calls `POST /api/auth/operator` with `Authorization: Bearer <G8eKey>` at startup to convert to a session cookie. Cookie held in-memory for the run. |
| 2.4 Passkey | **No passkey.** API-key auth returns a session directly for CLI identities (`@/home/bob/g8e/components/g8ed/services/operator/operator_auth_service.js:160-283`); no WebAuthn automation needed. |

### Phase 3 — Operator injection primitive

**Deliverable:** A standalone script that, given a tb container handle,
puts the operator binary in it, starts it, waits for bind, and returns
the `operator_id` + `operator_session_id`.

| Step | Recommendation |
|------|----------------|
| 3.1 Binary selection | Detect tb container arch via `container.attrs['Platform']` or `uname -m` via `exec_run`. Select `linux-amd64` or `linux-arm64` binary from g8ep's `/home/g8e/binaries/` (produced by `./g8e operator build-all`). |
| 3.2 Copy into container | Use `container.put_archive("/tmp", <tar>)`. Do **not** shell out to `docker cp`. |
| 3.3 Network attachment | `docker network connect g8e-network <container_id>` post-launch so the operator can resolve `g8e.local`. One subprocess call. |
| 3.4 Start the operator | `container.exec_run([...], detach=True)`. Flags: `--endpoint g8e.local --api-key <key> --no-git --log info`. The `--no-git` flag skips LFAA git init which is unnecessary noise for short-lived bench containers. |
| 3.5 Bind detection | Poll `GET /api/operators` (existing dashboard API) until the new slot is `ACTIVE`. Budget ~15s; fail task if exceeded. |
| 3.6 Bind to session | `POST /api/operators/bind` (existing). Ties the operator to our bench session so chat-pipeline tool calls route to it. |
| 3.7 Teardown | On task end, send a graceful shutdown command via pub/sub, then `operator terminate` via the API. Do NOT rely on tb's container stop to cleanly unwind — unbind on our side first to avoid stale slots. |

### Phase 4 — Approval bridge

**Deliverable:** A background task that, for a given `investigation_id`,
subscribes to SSE and approves every incoming approval request within
the configured policy.

| Step | Recommendation |
|------|----------------|
| 4.1 SSE subscription | Use existing SSE endpoint. Filter events by `investigation_id`. |
| 4.2 Approval policy | Approve: `OPERATOR_COMMAND_APPROVAL_REQUESTED`, `OPERATOR_FILE_EDIT_APPROVAL_REQUESTED`, `AI_AGENT_CONTINUE_APPROVAL_REQUESTED`. Deny: `OPERATOR_INTENT_APPROVAL_REQUESTED` (unreachable in v1 but defensive). |
| 4.3 Response mechanics | `POST /operator/approval/respond` with `approved: true`, `reason: "benchmark harness auto-approval"`, using the session cookie from Phase 2. |
| 4.4 Latency budget | Approve within a bounded window (e.g. 500ms) of receiving the SSE event. Tribunal + approval already add seconds per command; do not add more. |
| 4.5 Audit trail | Log every auto-approval to a per-task `approvals.jsonl` under the task's logging dir. Include `approval_id`, `command` (or `file_path`), `approved_at`, `reason`. Non-negotiable for submission honesty (§7). |
| 4.6 Fault handling | If SSE drops, reconnect with backoff; if approvals cannot be responded to, fail the task explicitly — never silently succeed. |

### Phase 5 — `G8eAgent` (single-task end-to-end)

**Deliverable:** `tb run --agent-import-path g8e_tb_adapter.agent:G8eAgent
--dataset terminal-bench-core==head --task-id hello-world` scores.

| Step | Recommendation |
|------|----------------|
| 5.1 Subclass | Implement `BaseAgent` with `perform_task(task_description, session, logging_dir) -> AgentResult`. |
| 5.2 Lifecycle order inside `perform_task` | (a) Inject operator (Phase 3). (b) Open chat client session (Phase 2 cookie). (c) Start approval bridge (Phase 4). (d) `POST` the prompt. (e) Stream the chat response to completion. (f) Close bridge. (g) Teardown operator. (h) Return `AgentResult`. |
| 5.3 Completion detection | "Run done" = chat pipeline returns a terminal assistant message with no further tool calls AND no pending approvals. Do **not** register a harness-only "I'm done" tool — unnecessary. |
| 5.4 Failure handling | Any exception in the lifecycle → return `AgentResult(failed=True)` with the error; do not raise. tb treats failure as a task fail, which is the correct semantics. |
| 5.5 Timeouts | Hard wallclock cap per task, conservatively 20 minutes. Tribunal × tool-loop can burn time at default pass counts. |
| 5.6 `logging_dir` | Write: `chat.jsonl` (full conversation), `approvals.jsonl` (from Phase 4), `operator.log` (from `exec_run` output), `timing.json` (phase timings). All under `logging_dir`. |

### Phase 6 — Robustness for full-suite runs

**Deliverable:** Clean 50-task run with `--n-concurrent 4`. No zombie
operators. No leaked containers. No stale sessions.

| Step | Recommendation |
|------|----------------|
| 6.1 Concurrency model | One benchmark user + one session cookie per tb *run*. One operator slot per tb *task* (claimed at task start, released at task end). tb's `--n-concurrent` controls parallelism. |
| 6.2 Slot hygiene | On `tb run` start, enumerate stale bench operator slots and terminate them — recovers from prior crashes. |
| 6.3 Network cleanup | On task end, `docker network disconnect g8e-network <container>` before tb kills it. Avoids network-name orphans. |
| 6.4 Retry on transient errors | Single retry per task on: operator bind timeout, SSE disconnect during first 10s, chat pipeline 5xx. No retry on: grading failures, real tool errors, exceeded wallclock. |
| 6.5 Parallel-safe logging | Every log path namespaced by `investigation_id`. |

### Phase 7 — Reporting & metrics

**Deliverable:** A single `run-summary.json` per tb run with
leaderboard-quality numbers plus g8e-specific metrics.

| Step | Recommendation |
|------|----------------|
| 7.1 tb-native metrics | Pass/fail per task (tb's output). Pass rate. |
| 7.2 Governance metrics | Per task: Tribunal invocations, Tribunal consensus/revisions, approval counts by type, approval latencies, token spend (primary + assistant). Aggregate across run. |
| 7.3 Cost | Per-task wallclock and token cost. Aggregate. Required for the submission disclosures in §7. |
| 7.4 Report format | Plain JSON. tb already writes per-task artifacts under `logging_dir`; we aggregate on `tb run` exit. |
| 7.5 Human summary | A short markdown table emitted to stdout at run end, for operator sanity-check. |

### Phase 8 — CI integration

**Deliverable:** Nightly GitHub Actions job that runs a fixed 10-task
smoke subset. Failure blocks nothing but paging a channel.

| Step | Recommendation |
|------|----------------|
| 8.1 Smoke subset | 10 tasks covering: easy (hello-world), file-ops, network, long-running, multi-step. Picked once during Phase 5 validation. |
| 8.2 Frequency | Nightly, not per-PR. Real LLM calls cost money. |
| 8.3 Alerting | Surface Tribunal-delta regressions (e.g. average pass-rate drop > 5pp for 2 consecutive runs) as the primary signal — this catches platform regressions that unit tests cannot. |
| 8.4 Budget guardrail | Hard token budget per nightly run. Abort if exceeded. |

### Phase 9 — Leaderboard submission

**Deliverable:** PR to terminal-bench with adapter code + parity results.

| Step | Recommendation |
|------|----------------|
| 9.1 Full run | `terminal-bench-core==0.1.1`, default Tribunal (3 passes), Anthropic Sonnet 4. Capture everything. |
| 9.2 Variant run | Same dataset, Tribunal 5 passes. Submit as a second labeled entry. |
| 9.3 README | Full disclosures per §7. |
| 9.4 Parity experiment | Run tb's stock `terminus` agent on the same dataset from inside g8ep to prove the harness location does not perturb scores. |
| 9.5 Submission label | `g8e-tribunal-3` and `g8e-tribunal-5`. Clear, comparable. |

---

## 5. Code layout

```
benchmarks/
  terminal-bench/
    README.md                # user-facing how-to-run
    pyproject.toml           # installs terminal-bench + adapter
    g8e_tb_adapter/
      __init__.py
      agent.py               # G8eAgent(BaseAgent)
      chat_client.py         # G8eChatClient: auth, post prompt, stream response
      approval_bridge.py     # SSE subscriber + auto-responder
      operator_injector.py   # put_archive + exec_run + bind poll
      reporter.py            # per-task + run-summary writers
      policy.py              # approval policy (what to approve/deny)
      config.py              # env vars, timeouts, budgets
      _internal/
        api_paths.py         # mirror of shared api_paths.json (single source)
        events.py            # mirror of shared events.json (single source)
    scripts/
      bootstrap-user.sh      # Phase 2: create bench user + key
      run.sh                 # `tb run` wrapper with default flags
```

No changes to: `components/`, `shared/`, `scripts/` (top-level), `g8e`
CLI (other than a single `bench` dispatch entry).

---

## 6. Open questions — with recommendations

Each question has a default recommendation so the project can move
without blocking. Overrides welcome.

| # | Question | Default recommendation |
|---|----------|------------------------|
| Q1 | Does `g8e.operator` bind successfully when started with `--api-key` only (no device link)? | Expected yes per `@/home/bob/g8e/components/g8ed/services/operator/operator_auth_service.js:160-283`. **Verify in Phase 3.1 smoke.** |
| Q2 | Does `POST /operator/approval/respond` accept a session minted by `POST /api/auth/operator` with API-key auth? | Expected yes — both are standard session cookies. **Verify in Phase 4.3 smoke** with a single synthetic approval. |
| Q3 | Will tb task containers come up with `/tmp` writable and `tar` present (needed for `put_archive`)? | Expected yes — tb's base images are full Ubuntu/Alpine. **If a task violates this, skip it and log.** |
| Q4 | Is it acceptable to attach tb's task container to `g8e-network` post-launch? | Docker supports this natively. No task in tb-core is known to bind specific networks. **Proceed.** |
| Q5 | Single benchmark user for all tb users, or one per variant? | One user per variant label (`bench-tribunal-3`, `bench-tribunal-5`). Clean audit trails, no cross-variant interference. |
| Q6 | MCP endpoint — should we still wire it up as a secondary entry? | No. 30s timeout is disqualifying. Chat endpoint only. |
| Q7 | Do we need a `benchmark_signal_done` tool? | No. Agent natural termination is reliable. Revisit only if Phase 5 validation shows otherwise. |

---

## 7. Submission honesty

Any leaderboard submission MUST document:

- **Governance pipeline ran end-to-end.** Tribunal, approval gate,
  Sentinel, LFAA — all real.
- **Approvals were auto-responded** by a benchmark harness authenticated
  as `bench-tribunal-3@lateraluslabs.local` (or variant). Every
  auto-approval is in the audit ledger *and* in the exported
  `approvals.jsonl`.
- **Tribunal pass count** (3 for the headline label; 5 for the variant
  label).
- **Models** (primary + assistant).
- **Wallclock and token cost** per task.
- **Parity** against tb's stock `terminus` agent run from the same
  g8ep host, proving the harness location is neutral.

This is load-bearing for the platform's credibility. A submission that
quietly elides the auto-approver or the Tribunal config is worse than no
submission.

---

## 8. Risks

| Risk | Likelihood | Mitigation |
|------|-----------|------------|
| `g8e.operator` fails to bind in an unusual tb container (alpine, rootless, read-only tmpfs). | Medium | Skip + log; quantify how many tasks this affects in Phase 5. If > 5%, invest in alternative injection paths. |
| Network-connect race: operator tries to resolve `g8e.local` before network attachment completes. | Low | Attach network *before* starting the operator; add 1s settle. |
| Chat-pipeline endpoint shape is not stable for external callers. | Unknown | Verify during Phase 2.3. If unstable, fall back to the internal relay the dashboard JS uses — still no shipping-code change, just a different endpoint choice. |
| tb updates break adapter contract. | Low | Pin tb version in `pyproject.toml`; adopt new versions deliberately. |
| Tribunal × tb task length → runaway cost. | Medium | Per-task wallclock cap (20 min) + per-run token budget guardrail (§6.4, §8.4). |
| Parallel bench-ops step on each other's operator slots. | Low | Slot-per-task isolation (§6.1) + slot-hygiene sweep (§6.2). |

---

## 9. Effort estimate

| Phase | Estimate |
|-------|----------|
| Phase 0 | 0.5 day (decisions + this doc review) |
| Phase 1 | 0.5 day |
| Phase 2 | 0.5 day |
| Phase 3 | 1 day |
| Phase 4 | 1 day |
| Phase 5 | 1–2 days |
| Phase 6 | 1 day |
| Phase 7 | 0.5 day |
| Phase 8 | 0.5 day |
| Phase 9 | 1 day + full-run wallclock |
| **Total** | **~7.5 dev days + full-run time** |

Real LLM cost for one full `terminal-bench-core` run at default Tribunal
is the biggest non-engineering variable. Estimate and budget before
Phase 9.

---

## 10. Related documentation

| Document | Why it matters |
|----------|----------------|
| `@/home/bob/g8e/docs/components/g8ep.md` | Harness host; operator binary location; docker access. |
| `@/home/bob/g8e/docs/architecture/ai_agents.md` | Chat pipeline, Tribunal, agent loop. |
| `@/home/bob/g8e/docs/architecture/operator.md` | Operator lifecycle, `--api-key` mode, binding. |
| `@/home/bob/g8e/docs/architecture/security.md` | Approval gate invariants we preserve. |
| `@/home/bob/g8e/docs/architecture/mcp.md` | Why MCP is not the right transport for this integration. |
| `@/home/bob/g8e/docs/testing.md` | Existing eval infrastructure (payload-grading benchmarks; not used here, but explains the project's existing testing philosophy). |
| `@/home/bob/g8e/g8e` | Existing CLI patterns for auth, session minting, container dispatch. |
| [terminal-bench first steps](https://www.tbench.ai/docs/first-steps) | `BaseAgent` interface we implement. |
| [terminal-bench task overview](https://www.tbench.ai/docs/task-overview) | Task format we are grading against. |
