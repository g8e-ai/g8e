# g8e Evals — Real-Operator Evaluation Framework

> **Status:** design. This document supersedes the current `components/g8ee/tests/evals/real_operator_fixture.py` and related pytest plumbing for real-operator evals.

## 1. Why we are rewriting this

The existing `components/g8ee/tests/evals/real_operator_fixture.py` and its `real_operator` conftest fixture are structurally broken and must be removed, not patched.

Concrete findings:

- `conftest.py` calls `fixture.bind_to_session(...)` which is **not defined** on `RealOperatorFixture`. Any test using the fixture fails on import-free runtime. That alone proves nobody is running this code.
- The operator is launched **inside `g8ee-test-runner`** via `subprocess.Popen`. That container:
  - has no operator binary baked in (the fixture downloads it fresh over HTTPS every run),
  - runs as uid 1001 with `cap_drop: ALL` and no privileged shell,
  - is not the container the platform expects to see as a managed node,
  - shares its network namespace with pytest, so the operator's `--http-port 443`/`--wss-port 9001` flags collide with or falsely imply things about the test runner's own interface.
- Operator identity is scraped with a regex from stdout (`operator_id=... operator_session_id=...`). The operator does not emit that line format. The fixture times out 100% of the time.
- Readiness = "two regexes matched". There is no confirmation that the operator reached `ACTIVE`, is receiving heartbeats, or is reachable via pubsub.
- Binary verification downloads happen through `verify=False` fallback even though the same binary already lives at `/home/g8e/g8e.operator` inside `g8ep`.
- Meanwhile a **correct** real-operator pattern already exists at `components/g8ee/tests/e2e/conftest.py` (provisions slots via the g8ed internal API, reads api_key from the g8es document store, subscribes to the real PubSub heartbeat channel). The evals suite duplicated and downgraded that pattern instead of reusing it — and even that pattern is not the right shape for evals, because evals are supposed to exercise **the product**, not the internal fixture plumbing.

The demo (`demo/Makefile`, `demo/docker-compose.yml`, `demo/containers/web-node/entrypoint.sh`) gives us the pattern we actually want: a fleet of real Linux containers, each running a real operator that auth'd against the platform with a **user-generated device link token** — the exact end-user workflow.

## 2. Design goals

1. **No bandaids.** Delete `real_operator_fixture.py` and the `real_operator` fixture from the evals conftest.
2. **Real product surface.** Evals hit the same `g8ed` HTTPS API a browser hits, using a device link token the user generates from the dashboard — no internal auth tokens, no slot-provisioning short-circuits, no fake operator documents.
3. **Real operator containers.** Each scenario runs against one (or more) dedicated operator containers on the `g8e-network`, exactly like `demo/web-node`. They are disposable and isolated from pytest's test runner.
4. **Host-driven harness.** Evals are launched as `./g8e evals run`. They run on the Docker host, not inside `g8ee-test-runner`. They do not use pytest. Pytest stays dedicated to unit/integration/e2e.
5. **Deterministic baseline, generative flexibility.** Keep the existing gold-set JSON shape (`gold_set.json`, `benchmark_gold_set.json`, `privacy_gold_set.json`) as scenario inputs. Reuse the existing `reporter.py` + `metrics.py` artifact format.
6. **Cheap to iterate.** Fleet stands up in seconds; scenarios run in parallel; failures produce the exact container logs and a reproduction command.

## 3. Target architecture

```
Host
 ├── ./g8e evals run
 │     └── scripts/evals/runner.py         (Python, runs on host, orchestrator)
 │           ├── docker compose -f components/g8ee/evals/docker-compose.evals.yml up
 │           │     ├── eval-node-01..NN    (real operator containers, device-token auth)
 │           │     └── (joins g8e-network, reaches g8e.local == g8ed)
 │           ├── g8ed HTTPS API            (chat + investigations + approvals)
 │           └── writes components/g8ee/reports/evals/<ts>/…
 │
 └── g8e platform (docker-compose.yml)     (g8ed, g8ee, g8es, g8ep — unchanged)
```

Key properties:

- The runner uses **only** public surfaces: device-link auth → POST chat → stream SSE → optionally POST approval. No `X-Internal-Auth`, no direct g8es document writes.
- Each eval node is a real Linux container with a real filesystem; scenarios like "tail /var/log/auth.log" actually tail a log file. Broken/healthy profiles (reused from demo if relevant) seed the environment.
- The runner owns the device token lifetime; it never mints one itself. The user pastes a token the same way they do in the dashboard.

## 4. File layout (target state)

```
components/g8ee/evals/
├── README.md
├── docker-compose.evals.yml              # Eval fleet (N operator containers)
├── containers/
│   └── eval-node/
│       ├── Dockerfile
│       └── entrypoint.sh                 # Downloads + supervises operator (demo pattern)
├── runner/
│   ├── __init__.py
│   ├── cli.py                            # ./g8e evals run entrypoint
│   ├── fleet.py                          # compose up/down, health, discover
│   ├── client.py                         # g8ed chat/approval/SSE client
│   ├── scorer.py                         # judge + deterministic matchers
│   ├── reporter.py                       # (move from tests/evals/reporter.py)
│   └── metrics.py                        # (move from tests/evals/metrics.py)
└── gold_sets/
    ├── accuracy.json                     # (was gold_set.json)
    ├── benchmark.json                    # (was benchmark_gold_set.json)
    └── privacy.json                      # (was privacy_gold_set.json)

docs/benchmarking/evals.md                # (this file)
components/g8ee/reports/evals/            # existing artifact dir — keep
```

Deleted:
- `components/g8ee/tests/evals/real_operator_fixture.py`
- The `real_operator` fixture in `components/g8ee/tests/evals/conftest.py`
- `components/g8ee/tests/evals/test_agent_tool_loop.py::test_orchestrate_tool_execution_with_real_operator`
  (the security-violation test in the same file stays — it's a pure integration test and has no business being in evals).

Non-operator gold-set tests (accuracy/benchmark/privacy that drive LLM+validation paths without a real operator) stay under `tests/evals/` and keep using pytest. Only the **real-operator** dimension moves out.

## 5. Container design — `eval-node`

Mirrors `demo/containers/web-node/entrypoint.sh` but stripped to the minimum needed for operator coverage.

Dockerfile:
- base: `debian:stable-slim` (closer to a real target than alpine)
- installs: `bash curl coreutils iproute2 procps file openssh-client grep sed awk findutils less` plus anything the gold-set scenarios require (logs, nginx, etc — profile-driven)
- non-root `appuser` for the operator, optional `root`-profile container for root-level scenarios
- writes a realistic filesystem fixture (`/var/log/auth.log`, `/etc/app/config.json`, etc) at entrypoint so scenarios have something real to find

Entrypoint responsibilities (copy from `demo/containers/web-node/entrypoint.sh:122-169`):

1. If `DEVICE_TOKEN` is unset, exit non-zero — evals must always have a token.
2. Download `g8e.operator` from `https://${G8E_ENDPOINT:-g8e.local}/operator/download/linux/amd64` with `Authorization: Bearer $DEVICE_TOKEN` (verify checksum; retry on failure).
3. `exec` the operator with `-e g8e.local -D $DEVICE_TOKEN --no-git`. Do **not** override `--http-port`/`--wss-port` — let the defaults match the platform.
4. Supervised restart loop, stdout prefixed with `[eval-node-NN operator]` so `docker logs` is readable.

Compose (`docker-compose.evals.yml`):
- external network `g8e-network`, external volume `g8es-ssl` (same as demo)
- N services `eval-node-01..NN` using an anchor block, each with env `DEVICE_TOKEN`, `EVAL_NODE_ID`, and optional `EVAL_PROFILE`
- `labels: io.g8e.evals=true` for discovery
- no exposed host ports; everything stays on `g8e-network`

## 6. Orchestrator — `./g8e evals run`

CLI surface (host-side):

```
./g8e evals run --device-token dlk_xxx [--suite accuracy|benchmark|privacy|all] \
                [--nodes 5] [--scenarios id1,id2] [--parallel 4] \
                [--llm-provider …] [--llm-model …] [--report-dir …]
./g8e evals up   --device-token dlk_xxx --nodes 5    # bring up fleet without running
./g8e evals down                                     # tear down fleet
./g8e evals status
./g8e evals logs eval-node-03
```

Orchestrator flow per scenario:

1. **Pick a node.** Round-robin or pin by `scenario.required_profile`. Wait until g8ed's `GET /api/internal/operators/user/{user}/status`-equivalent **public** path reports BOUND — we'll expose the minimum public status needed, or we reuse the dashboard's existing polling endpoint.
2. **Create an investigation** via the same POST the dashboard uses.
3. **Send the chat message.** Stream SSE back; capture all tool calls, approvals required, final message text, and timing. No mocking — the g8ee agent is the real agent.
4. **Auto-approve** any `approval_required` events using the same public approval endpoint the dashboard uses. (Privacy/safety scenarios should assert that certain tool calls never reach approval, or that Sentinel blocked them.)
5. **Score** using `scorer.py`:
   - deterministic matchers for `expected_tool`/`expected_payload` (reuse benchmark gold-set shape)
   - LLM judge for `expected_behavior`/`required_concepts` (reuse accuracy gold-set shape)
   - Sentinel assertions for privacy (secret must not appear in final response)
6. **Record** an `EvalRow` (same dataclass used today in `tests/evals/metrics.py`).
7. **Reset node state** between scenarios (either `docker restart eval-node-NN` or a scripted fs reset — start with docker restart; it's <2s).
8. After the last scenario, `reporter.persist_report(...)` writes `report.txt`, `results.csv`, `summary.json` to `components/g8ee/reports/evals/<ts>/`.

On failure the runner prints:
- the scenario id
- the final SSE timeline
- `docker logs eval-node-NN --tail=200`
- the exact command to re-run just that scenario

Parallelism: scenarios are independent if each owns its own node. Cap `--parallel` at `--nodes`.

## 7. Gold set consolidation

- Move `tests/evals/{gold_set,benchmark_gold_set,privacy_gold_set}.json` → `components/g8ee/evals/gold_sets/{accuracy,benchmark,privacy}.json`.
- Validate on load with the existing schema checks from `tests/evals/shared.py` (`REQUIRED_SCENARIO_FIELDS`, `REQUIRED_BENCHMARK_FIELDS`). Move those helpers to `runner/`.
- Scenarios marked `agent_mode: "OPERATOR_BOUND"` previously **skipped** in raw-model tests (`tests/evals/shared.py:load_and_validate_gold_set` with `filter_operator_bound=True`) are now first-class inputs for `./g8e evals run`. No more silent skips.

## 8. Step-by-step implementation plan

One PR per step so each is bisectable.

**Step 0 — Delete the bandaid (prereq, standalone PR)**

- Remove `components/g8ee/tests/evals/real_operator_fixture.py`.
- Remove the `real_operator` fixture from `components/g8ee/tests/evals/conftest.py`.
- Remove `test_orchestrate_tool_execution_with_real_operator` from `components/g8ee/tests/evals/test_agent_tool_loop.py` (keep the security-violation test; it's unrelated).
- Run `./g8e test g8ee -- tests/evals` to prove nothing else depended on them.
- Update `tests/evals/conftest.py` docstring to stop claiming "real operators" — fake-operator gold-set evals continue to live there.

**Step 1 — eval-node container**

- Create `components/g8ee/evals/containers/eval-node/{Dockerfile,entrypoint.sh}`.
- Entrypoint is a cleaned-up port of `demo/containers/web-node/entrypoint.sh:122-169`. No nginx, no flask, no SSH, no fake secrets — just the operator supervisor loop and any fixture files individual scenarios need.
- Validate manually: `docker build`, `docker run -e DEVICE_TOKEN=dlk_xxx --network g8e-network ...`, confirm operator appears in the g8ed dashboard.

**Step 2 — compose profile**

- Create `components/g8ee/evals/docker-compose.evals.yml` with one anchor + N services (start with N=3).
- Add `./g8e evals up|down|status|logs` to the top-level `g8e` CLI — thin wrappers around `docker compose` with the evals compose file.
- Document in `components/g8ee/evals/README.md`: prereqs (`./g8e platform setup`, login, dashboard device-token generation), quickstart.

**Step 3 — runner skeleton + g8ed client**

- Add `components/g8ee/evals/runner/` with `cli.py`, `client.py`, `fleet.py`.
- `client.py`: async `aiohttp` client for `POST /api/chat`, SSE stream consumer, `POST /api/approvals/{id}`. TLS via `g8es-ssl` CA (mounted into `g8ep` or read from the volume on the host).
- `fleet.py`: `up(n, device_token)`, `down()`, `wait_bound(timeout)`, `restart(node_id)`, `logs(node_id, tail)`.
- `cli.py`: wire `./g8e evals run --device-token … --dry-run` that just brings up fleet, runs one canned chat ("run `uname -a`"), prints the response, tears down.

**Step 4 — scorer + reporter port**

- Move `components/g8ee/tests/evals/{metrics,reporter}.py` → `components/g8ee/evals/runner/`. Adjust imports.
- `scorer.py` consolidates three scoring paths:
  - deterministic tool matcher (benchmark) — port logic from `test_agent_benchmark.py`
  - LLM judge for expected_behavior — port from `test_agent_accuracy.py`
  - privacy redaction checks — port from `test_agent_privacy.py`
- Unit test the scorers in `components/g8ee/tests/unit/` against recorded SSE fixtures (no operator needed for scorer unit tests).

**Step 5 — full runner loop**

- Implement scenario selection, round-robin node assignment, per-scenario `reset` between runs, parallelism, artifact writing.
- Run against a single gold-set file end-to-end. Prove: all OPERATOR_BOUND scenarios that used to be skipped now execute against a real operator.

**Step 6 — regression coverage**

- Add a golden "smoke" scenario that must always pass (`echo hello` style). `./g8e evals run --suite smoke` is safe to run in CI.
- Add one failure-injection test: stop `eval-node-01` mid-scenario, confirm runner reports the failure cleanly rather than hanging.
- Document expected run times and resource footprint in `components/g8ee/evals/README.md`.

**Step 7 — docs + deprecation notes**

- Point `docs/benchmarking/` and `docs/developer.md` at this file.
- Remove any stale references to `real_operator` / `TEST_DEVICE_TOKEN` in component docs.
- Update `.github/workflows/build-and-test.yml` to exclude real-operator evals from PR runs (device token required) but allow an opt-in nightly job.

## 9. What explicitly stays out of scope

- No changes to the operator binary, g8ed, g8ee, or g8es. If we hit a real bug in those during step 5, it's a separate PR with its own regression test.
- No auto-minting of device tokens inside the runner. That's a user action by design.
- No new internal auth surface. If a scoring need can't be met through public APIs, that's a product gap to fix in the product, not a shortcut in evals.
- Keep pytest-based accuracy evals (non-operator) intact during the migration. Delete them only after the new runner has at least one green run of the equivalent suite.

## 10. Open questions

- **Profiles per node.** Do accuracy/benchmark gold-set scenarios need environment-specific profiles (nginx broken, logs seeded, etc.) the way demo nodes do? If yes, we add a small `profiles/` dir under `eval-node` and map `scenario.required_profile → container`. Defer until step 5 surfaces a real need.
- **Token reuse across runs.** A dashboard-generated device token is long-lived; we should document rotation expectations and whether `./g8e evals run` should validate the token before fleet-up to fail fast.
- **Approval policy.** Auto-approve everything vs. simulate a user approving only "safe" actions. The privacy/safety suite needs the nuanced version; start with auto-approve-all and add policy hooks in step 5.
