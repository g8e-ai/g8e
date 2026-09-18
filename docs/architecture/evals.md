---
title: Evaluations
parent: Architecture
---

# Evaluations

Last Updated: 2026-09-18  
Version: v2.1.8

## Scope

g8e owns the platform evaluation programs: the Go-native **execution-boundary** suite and **model campaign** scoring that exercises real models through governed inference, tool dispatch, and provider-boundary hardware observation. The evaluator, campaign controller, evidence store, and verifier live in the `g8e` binary under `internal/services/evaluation/` and the `g8e eval` CLI.

g8ee participates in model campaigns as the production chat path (`POST /api/v1/chat`) but does not own platform evidence, verification, or campaign orchestration. See [Ensemble Evaluations](../ensemble/evals.md) for how g8ee uses these programs.

Operational deployment (Compose profiles, enrollment order, smoke workflows, troubleshooting) lives in the [Unified Docker Stack Guide](../guides/unified_stack.md).

---

## Evaluation programs

| Program | Suite / catalog | Proves | Does not use |
| --- | --- | --- | --- |
| **Execution-boundary** | `core-execution-boundary@1.0.0` | One allowed governed mutation and one doctrine-prohibited equivalent through the real Gateway and remote Operator | g8ee, model providers, campaigns, synthetic simulators |
| **Model campaign** | `north-star-25@1.0.0` scenario catalog | Governed model scoring through production inference, tool scenarios, and provider-boundary hardware telemetry | Direct Ollama calls from the campaign CLI or g8ee |

Both programs persist canonical, content-addressed evidence beneath `.g8e/data/eval/runs/`. Verification is independent of execution: `g8e eval verify` and `g8e eval campaign verify` recompute bindings and signatures without mutating the platform.

---

## CLI surface

The `g8e eval` command tree (alias `g8e evals`) groups platform evaluation commands:

| Command group | Purpose |
| --- | --- |
| `g8e eval run core-execution-boundary` | Run the native execution-boundary suite |
| `g8e eval verify <run-id>` / `g8e eval show <run-id>` | Verify or inspect a native run |
| `g8e eval campaign …` | Initialize, schedule, execute, publish, verify, and export North Star model campaigns |
| `g8e eval inference …` | Inference-operator status, registry freeze, probe, and acceptance gates |
| `g8e eval chat accept` | Phase 1A chat-path vertical acceptance through production `POST /api/v1/chat` |
| `g8e eval inventory …` | Model inventory and registry helpers |
| `g8e eval queue …` | Campaign queue inspection and orchestration |
| `g8e eval provider-observer run` | Legacy co-located dev observer only; production uses the enrolled Observer Operator |

Campaign lifecycle commands include `init`, `schedule`, `execute`, `publish`, `verify`, `status`, `show`, `export`, `mirror restore`, and repair helpers. Use `./g8e eval campaign --help` as the command-surface reference.

---

## Native execution-boundary suite

The Phase 1 suite is `core-execution-boundary@1.0.0`. It requires doctrine posture and does not use g8ee, a model provider, model judges, campaigns, scheduling, or synthetic simulators.

### Run

Start and enroll the unified stack with one active remote Operator, then run:

```bash
./g8e eval run core-execution-boundary
```

Pin an exact active remote Operator session when more than one remote Operator is available:

```bash
./g8e eval run core-execution-boundary --operator-session <session-id>
```

The suite performs two attempts. The allowed attempt writes one run-specific marker through the authenticated Gateway command ingress and the bound remote Operator. The prohibited equivalent traverses the same ingress and must be rejected by L1 without another effect. Required verdicts cover independent effect counts, target identity, terminal receipt status, receipt durability, deterministic protocol-chain validity, rejection, absence of another completed execution, and Gateway L1 attribution.

### Verify and inspect

Each run persists `report.json`, `verification.json`, and digest-named evidence files under `.g8e/data/eval/runs/<run-id>/`. Re-run verification without executing another mutation:

```bash
./g8e eval verify <run-id>
./g8e eval show <run-id>
```

Add `--json` to emit canonical protojson. Verification resolves every declared artifact, recomputes content addresses, validates report and attempt bindings, verifies receipt and persistence signatures, validates deterministic stage evidence, reconstructs typed observations, recomputes verdicts and metrics, and rejects missing, substituted, contradictory, misbound, or undeclared evidence.

### Trust boundaries

The Gateway is the Policy Decision Point and owns ingress authentication, envelope construction, L1-L3 decisions, and coordination state. The selected remote Operator is the Policy Execution Point and owns L4-L5 execution and authoritative local evidence. The Gateway receipt query is a verified mirror of Operator-authored evidence, not an independent read of the Operator database.

The independent observer is a short-lived `g8e-eval-observer` Compose process. It has no network, workload identity, runtime volume, credentials, or writeable target mount. It mounts only the shared controlled fixture volume read-only. The observer runs as the root UID with all Linux capabilities dropped so it can read the root-owned, owner-only fixture created by the Operator while remaining unable to mutate the read-only volume or cross a network boundary.

---

## Model campaign evaluations

North Star model campaigns score real models through the production g8ee `POST /api/v1/chat` path, governed inference dispatch, and a frozen scenario catalog. They use one campaign Gateway with **three** distinct enrolled remote Operator sessions:

| Session | Capability flag | Host | Role |
| --- | --- | --- | --- |
| **Data Operator** | `inference_enabled=false` | Campaign host (Docker) | Governed tool/filesystem/process boundary for model-originated host actions |
| **Inference Operator** | `inference_enabled=true` | Campaign host (Docker) | Governed L4/L5 inference PEP; sole scored path to the approved Ollama provider |
| **Observer Operator** | `provider_boundary_observer_enabled=true` | Provider host (where Ollama/GPU runs) | Read-only GPU and system RAM sampling at the provider execution boundary |

Scored inference never calls Ollama directly from g8ee or the campaign CLI. The Gateway routes inference envelopes to the exact Inference Operator session, tool intents to the exact Data Operator session, and `ProviderBoundaryObservationCommand` (BEGIN/FINALIZE) to the exact Observer Operator session.

### Why the Inference Operator and Observer Operator are separate

Both roles use the same `g8e operator` binary but enroll as **different remote sessions** with different capability flags. Production Observer enrollment uses `--provider-boundary-observer-enabled` only; do **not** pass `--inference-enabled` on the provider host.

**1. Provider-host placement.** The Inference Operator runs on the campaign host and calls the remote Ollama HTTP API. GPU VRAM, utilization, temperature, power, clocks, and host RAM must be sampled on the machine where inference actually runs. A Linux Docker container on the campaign host cannot authoritatively observe hardware on a remote Windows GPU host.

**2. Independent witness evidence.** Hardware-efficiency metrics are witness evidence, not executor self-report. North Star requires a typed observer at the **provider execution boundary** that binds sample windows to `provider_attempt_id`. The campaign controller verifies observation coverage separately from inference receipts and never infers provider hardware state from model latency or host-side process observations.

**3. Least privilege and evidence ownership.** The Observer has no inference backend, no model-management authority, no prompt access, and no access to Inference Operator attempt files. Merging observation into the Inference Operator would let the inference executor attest its own GPU usage and blur which session owns which evidence chain.

**4. Gateway coordination, separate execution.** When scored inference starts and ends, the Gateway `ProviderBoundaryObservationCoordinator` sends fire-and-forget BEGIN/FINALIZE commands to the Observer over pub/sub in parallel with inference dispatch. Verification binds completed `ProviderBoundaryObservationWindow` records to governed attempts. Campaigns without observer coverage may still publish timing and token fields but remain explicitly incomplete for hardware-efficiency claims until `g8e eval campaign verify --require-provider-observation` passes.

**5. Same pattern as tool scenarios.** Governed tool assignments require an independent observer that cannot mutate the target. Provider-boundary observation applies the same separation to inference-side hardware telemetry.

### Observer naming: do not confuse the two observers

g8e uses two different “observer” concepts:

| Name | Where it runs | Purpose |
| --- | --- | --- |
| **`g8e-eval-observer`** | Campaign host Compose (`evaluation` profile) | Networkless target-state reader for the native `core-execution-boundary@1.0.0` suite only |
| **Observer Operator** | Provider host (`g8e operator start --provider-boundary-observer-enabled`) | Enrolled remote Operator for provider-boundary GPU/RAM telemetry during scored model campaigns |

The Compose `g8e-eval-observer` service is **not** the provider-boundary Observer Operator and does not satisfy North Star hardware-efficiency requirements for model campaigns.

### Provider-boundary Observer Operator

Deploy the Observer Operator **on the machine that runs Ollama** (for example a remote Windows GPU host), not on the Linux campaign host. Enrollment and startup procedure live in the [Unified Docker Stack Guide](../guides/unified_stack.md#provider-boundary-observer-operator-windows-ollama-host).

What the Observer does:

1. Gateway sends `ProviderBoundaryObservationCommand` (BEGIN/FINALIZE) on the observer's pub/sub cmd channel when scored inference starts and ends.
2. Observer samples GPU VRAM, utilization, temperature, power, clocks, and system RAM between BEGIN and FINALIZE.
3. Observer publishes `ProviderBoundaryObservationCompleted` on its results channel.
4. Gateway ingests windows for `g8e eval campaign verify --require-provider-observation`.

**Timing rule:** Assignments that reached a terminal state before the Observer Operator was enrolled and pub/sub-connected will fail `--require-provider-observation`. Enroll the observer before `execute`, or accept that early assignments lack hardware windows.

#### Example operator output (Windows Ollama host)

While a scored campaign runs, the Observer Operator console on the provider host shows paired BEGIN/FINALIZE cycles for `PROVIDER_BOUNDARY_OBSERVATION`. Each cycle verifies the incoming `GovernanceEnvelope`, mints and dissolves a JIT capability, appends a ledger commitment, samples GPU and system RAM, and publishes completion on the session results channel.

Healthy output looks like this (excerpt from a live run against remote Ollama):

```text
2026-09-18T08:49:23.763-07:00 INFO: Minted JIT capability
  - message_id: 29ea0607ec447a953984ea7255867b5a386e68ade4eac2577331fba9552b33f0
  - action_type: PROVIDER_BOUNDARY_OBSERVATION
  - target_resource:
  - expires_at: 2026-09-18T15:54:24Z
2026-09-18T08:49:23.763-07:00 INFO: Executing verified transaction through Actuator
  - event_type: g8e.v1.operator.provider.boundary.observation.requested
2026-09-18T08:49:23.763-07:00 INFO: Provider-boundary observation started
  - provider_attempt_id: f7607d6c-6804-4df9-af89-abfe16404212
  - inference_transaction_id:
2026-09-18T08:49:23.763-07:00 INFO: Dissolved JIT capability
  - message_id: 29ea0607ec447a953984ea7255867b5a386e68ade4eac2577331fba9552b33f0
  - action_type: PROVIDER_BOUNDARY_OBSERVATION
2026-09-18T08:49:23.779-07:00 INFO: ActionReceipt recorded
  - transaction_id: 29ea0607ec447a953984ea7255867b5a386e68ade4eac2577331fba9552b33f0
  - status: EXECUTION_STATUS_COMPLETED
...
2026-09-18T08:49:24.185-07:00 INFO: Publishing result
  - channel: results:02da84b2-ec05-421d-8b50-778dbbcad68f:cb0fc6ba-52d9-4fcf-a854-dde9f99592ff
  - event_type: g8e.v1.operator.provider.boundary.observation.completed
  - id: e9f93e9bb84e7f9d0a3743958b8048ef09292009c10d1297c3c19fa36003c9a4
2026-09-18T08:49:24.185-07:00 INFO: Provider-boundary observation completion transmitted
  - operator_session_id: cb0fc6ba-52d9-4fcf-a854-dde9f99592ff
  - provider_attempt_id: f7607d6c-6804-4df9-af89-abfe16404212
  - transaction_id: e9f93e9bb84e7f9d0a3743958b8048ef09292009c10d1297c3c19fa36003c9a4
2026-09-18T08:49:24.185-07:00 INFO: Provider-boundary observation completed
  - provider_attempt_id: f7607d6c-6804-4df9-af89-abfe16404212
  - sample_count: 2
  - observation_digest: 6ccbf0a05dd150f8ff97b74ee03f6cbc9ac9dc9ae7f288fb5381cc73af7c7361
...
2026-09-18T08:49:24.275-07:00 INFO: Provider-boundary observation started
  - provider_attempt_id: d8e52fc2-2f43-41ab-94a6-50a0c171147c
  - inference_transaction_id:
...
2026-09-18T08:49:26.953-07:00 INFO: Provider-boundary observation completed
  - provider_attempt_id: d8e52fc2-2f43-41ab-94a6-50a0c171147c
  - sample_count: 6
  - observation_digest: cb3ed7e0c112021ad22bb3bf01c9f203a0ae2cb67005507a8d0f3185bf6baf3f
...
2026-09-18T08:49:27.002-07:00 INFO: Provider-boundary observation started
  - provider_attempt_id: c497dfc0-55ab-4bb8-a10a-5d0e5c210393
  - inference_transaction_id:
2026-09-18T08:49:27.008-07:00 INFO: Actuator execution succeeded
  - message_id: 115392cc60037b05166607eb97ab571acdbca1ace1258d2886cac5061dc50d81
  - receipt_status: EXECUTION_STATUS_COMPLETED
```

What to verify:

- **BEGIN** lines (`Provider-boundary observation started`) arrive when scored inference starts for a `provider_attempt_id`.
- **FINALIZE** lines (`Provider-boundary observation completed`) include `sample_count` and `observation_digest` for the same `provider_attempt_id`.
- Every transaction ends with `Actuator execution succeeded` and `receipt_status: EXECUTION_STATUS_COMPLETED`.
- `Publishing result` uses event type `g8e.v1.operator.provider.boundary.observation.completed` on the session `results:` channel.

If you see only BEGIN without matching FINALIZE for an attempt, or repeated `EXECUTION_STATUS_EXECUTING` without `COMPLETED`, check Gateway connectivity and that the campaign `execute` process is still running.

---

## Evidence and verification

### Storage layout

| Artifact | Path | Owner |
| --- | --- | --- |
| Native run report and verification | `.g8e/data/eval/runs/<run-id>/report.json`, `verification.json`, digest-named evidence | `g8e eval run` / `g8e eval verify` |
| Campaign run state and results | `.g8e/data/eval/runs/<run-id>/` (campaign-scoped lifecycle, assignment, trace, and aggregate records) | `g8e eval campaign …` |
| Provider observation windows (ingested) | Gateway volume under `data/inference/provider-observer/windows/` | Gateway ingest from Observer Operator results |

Native verification is owned by `g8e eval verify`. Campaign verification is owned by `g8e eval campaign verify`, with `--require-provider-observation` enforcing hardware-window coverage through the Gateway read API when local evidence is missing.

### Public spectator projection

The checked-in evaluation explorer reads canonical native and campaign projections from persisted runs. A public-safe projector omits principal, Operator, session, credential, endpoint, path, raw target, envelope, receipt, audit, and evidence body fields before records enter the signed public feed. Native verification remains on the owner path; mirror availability is not verification evidence. See [Public Spectator Architecture](./public_spectator.md).

---

## Implementation

Platform evaluation logic lives in `internal/services/evaluation/`:

- Native suite registry, governed command lane, and deterministic grading
- Independent target observer for the execution-boundary suite
- Campaign controller, publication coordinator, and provider-boundary observation integration
- Canonical evidence storage and fail-closed verification

Provider-boundary sampling runs in the Observer Operator through `internal/services/operatorcapability/provider_boundary_observer.go` and `internal/services/inference/provider_observer/`. Protocol contracts are defined under `protocol/proto/g8e/eval/v1/`.

---

## Related documentation

- [Unified Docker Stack Guide](../guides/unified_stack.md) — Compose profiles, enrollment order, campaign workflows, and troubleshooting
- [Ensemble Evaluations](../ensemble/evals.md) — How g8ee uses g8e evals through the production chat path
- [Ensemble (g8ee)](./ensemble.md) — g8ee's role in the platform and trust boundaries
- [Gateway Architecture](./gateway.md) — Inference dispatch, provider-boundary coordination, and pub/sub
- [Operator Architecture](./operator.md) — L4 Warden, L5 Actuator, and capability flags
- [Public Spectator Architecture](./public_spectator.md) — Public-safe evaluation projections and explorer contract
- [Sovereignty Gauntlet](../guides/sovereignty_gauntlet.md) — Evidence-oriented demonstration and claim-scoping workflow
