# Evaluations

## Overview

g8e supports two evaluation programs documented on this page: the Go-native **execution-boundary** suite (`core-execution-boundary@1.0.0`) and **model campaign** evaluations that score real models through g8ee and governed inference. The sections below cover each program's topology, trust boundaries, and operator roles.

## Native execution-boundary suite

g8e provides a Go-native evaluation command that proves the core remote execution boundary against the real unified Docker stack. The evaluator runs on the Docker host, submits typed intent to the Gateway, binds execution to one exact remote Operator session, observes the controlled target through a separate networkless Compose observer, and persists canonical content-addressed evidence beneath `.g8e/data/eval/runs/`.

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

## Model campaign evaluations

North Star model campaigns score real models through the production g8ee `POST /api/v1/chat` path, governed inference dispatch, and a frozen scenario catalog. They use one campaign Gateway with **three** distinct enrolled remote Operator sessions:

| Session | Capability flag | Host | Role |
| --- | --- | --- | --- |
| **Data Operator** | `inference_enabled=false` | Campaign host (Docker) | Governed tool/filesystem/process boundary for model-originated host actions |
| **Inference Operator** | `inference_enabled=true` | Campaign host (Docker) | Governed L4/L5 inference PEP; sole scored path to the approved Ollama provider |
| **Observer Operator** | `provider_boundary_observer_enabled=true` | Provider host (where Ollama/GPU runs) | Read-only GPU and system RAM sampling at the provider execution boundary |

Scored inference never calls Ollama directly from g8ee or the campaign CLI. The Gateway routes inference envelopes to the exact Inference Operator session, tool intents to the exact Data Operator session, and `ProviderBoundaryObservationCommand` (BEGIN/FINALIZE) to the exact Observer Operator session.

Day-to-day campaign operations (Compose profiles, enrollment order, mini smoke, observer checklist) live in [Unified Docker Stack Guide](../guides/unified_stack.md).

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
