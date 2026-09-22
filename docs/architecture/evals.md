---
title: Evaluations
parent: Architecture
---

# Evaluations

Last Updated: 2026-09-22
Version: v2.1.12

## Scope

g8e owns the platform evaluation programs: the Go-native **execution-boundary** suite and **model campaign** scoring that exercises real models through governed inference, tool dispatch, provider-boundary hardware observation, and storage-side model provenance attestation. These programs are the reference proof of the Gateway + Operator topology described in [Beyond evaluations](#beyond-evaluations); evaluations exercise that topology in a read-only telemetry mode, but the same provenance, observation, PDP, and outbound-mutation separation applies to state-altering workflows well outside model scoring. The evaluator, campaign controller, evidence store, and verifier live in the `g8e` binary under `internal/services/evaluation/` and the `g8e eval` CLI.

g8ee participates in model campaigns as the production chat path (`POST /api/v1/chat`) but does not own platform evidence, verification, or campaign orchestration. See [Ensemble Evaluations](../ensemble/evals.md) for how g8ee uses these programs.

Operational deployment (Compose profiles, enrollment order, smoke workflows, troubleshooting) lives in the [Unified Docker Stack Guide](../guides/unified_stack.md). Runtime campaign inventories and rollout queues live under `.g8e/eval/` (gitignored); checked-in templates and the public/private boundary are documented in [eval/examples/README.md](../../eval/examples/README.md).

---

## Beyond evaluations

g8e evaluation programs exercise one instance of a broader architecture: **Confidential Edge Execution with Governed State Mutation**. The same Gateway + Operator topology combines local artifact provenance, runtime telemetry observation, a central Policy Decision Point (Gateway), and outbound-only state-mutation Operators at the data owner's boundary.

Evaluations are predominantly a **read-only telemetry use case**. They prove that:

- allowed governed mutations succeed through the real Gateway and remote Operator;
- doctrine-prohibited equivalents fail closed without another effect;
- independent observers can attest terminal state without network access or write authority; and
- Provenance and Observer Operators can witness model weights and provider-boundary hardware separately from the inference executor.

When AI **actively alters system state** from local inference — not merely scores model output — the value of zero-trust provenance and outbound-only telemetry scales with the consequence of each mutation. The evaluation topology is the reference proof of that separation; the domain mirrors below show where the same pattern applies in production workflows.

### Common topology

Every mirror deployment separates four roles:

| Role | Evaluation instance | General responsibility |
| --- | --- | --- |
| **Provenance Operator** | Storage-side model weight attestation | Hashes local artifacts inference depends on (code, firmware, scans, feeds, weights) |
| **Observer** | Provider-boundary GPU/RAM sampling; networkless fixture reader | Captures runtime telemetry at the execution boundary without mutating target state |
| **Data Operator** | Campaign host tool/filesystem/process boundary | Mutates authoritative state at the owner's boundary (PRs, actuators, EHR, trades) |
| **Gateway (PDP)** | Campaign and boundary-suite policy admission | Enforces central policy from provenance proofs, observer telemetry, and posture-required authorization |

All Operators connect outbound-only over mTLS. The Gateway admits work; each Operator independently verifies envelopes before any local side effect or witness publication.

### Domain mirrors

The following workflows reuse the same topology with different artifacts, observers, mutators, and policy gates. They are architectural mappings — not claims that g8e ships turnkey sector products — but they explain why evaluation witness separation generalizes.

#### 1. Autonomous zero-trust DevSecOps and auto-remediation

Instead of scoring models, the system acts as an autonomous tier-3 DevSecOps engineer remediating critical vulnerabilities in air-gapped repositories.

```text
[Local Code Repo] ---> (Provenance Operator: hashes Git tree and SAST binaries)
                              | outbound
                              v
   [Inference Engine] ---> [g8e Gateway] <--- (Data Operator: pushes verified Git PRs)
   (runs local codegen)       ^
                              | outbound
[Compiler / Sandbox] ---> (Observer: captures test and memory telemetry)
```

- **Workflow:** A scanner flags a zero-day. A local coding agent generates a patch, compiles it, and runs unit tests.
- **Provenance Operator:** Hashes the codebase, compiler binary, and local LLM so the patch environment has not been tampered with.
- **Observer:** Captures execution telemetry, memory behavior, and test pass/fail metrics inside a local sandbox.
- **Data Operator:** Mutates state by opening a cryptographically signed pull request or merging into the enterprise repository.
- **Gateway value:** SecOps policy blocks code mutation on the primary branch unless provenance confirms pristine source hashes and the observer confirms 100% test pass without unauthorized network egress.

#### 2. Critical infrastructure and OT/SCADA cyber-physical response

In operational technology, SCADA, and industrial IoT, cloud latency and internet ingress are often unacceptable due to safety and physical security constraints.

```text
[PLC Firmware / Logic] ---> (Provenance Operator: hashes sensor baselines and edge weights)
                                   | outbound
                                   v
    [Edge GPU Inference] ---> [g8e Gateway] <--- (Data Operator: actuates PLC valves/breakers)
    (runs anomaly model)           ^
                                   | outbound
  [Physical OT Sensors] ---> (Observer: monitors voltage, pressure, temp telemetry)
```

- **Workflow:** Local sensors detect vibration anomalies in a hydro turbine. An edge model decides whether to drop load or adjust valve pressure.
- **Provenance Operator:** Hashes PLC configuration, sensor calibrations, and edge model weights against supply-chain or firmware tampering.
- **Observer:** Monitors GPU load, physical sensor telemetry, and reaction latency.
- **Data Operator:** Mutates physical state through low-level actuation commands to the SCADA controller.
- **Gateway value:** Central industrial safety policy verifies automated physical changes stay within pre-approved engineering envelopes while the SOC receives outbound plant telemetry.

#### 3. Confidential healthcare AI and local EHR state mutation

Healthcare deployments must keep PHI on premises while still using AI on DICOM images and clinical notes.

```text
[DICOM Medical Scans] ---> (Provenance Operator: hashes patient data headers and model identity)
                                   | outbound
                                   v
  [Hospital Edge GPU]  ---> [g8e Gateway] <--- (Data Operator: mutates central EHR record)
  (runs diagnostic AI)             ^
                                   | outbound
   [Local Inference]   ---> (Observer: monitors confidence and memory isolation)
```

- **Workflow:** A hospital GPU runs an oncology diagnostic model on fresh CT scans, drafting notes and flagging urgent cases.
- **Provenance Operator:** Proves DICOM scan header integrity and confirms the diagnostic model matches FDA/compliance certification.
- **Observer:** Tracks confidence thresholds, processing speed, and verifies PHI did not leak into ephemeral temp directories.
- **Data Operator:** Appends approved diagnostic notes to the hospital's central EHR.
- **Gateway value:** CISOs and HIPAA auditors receive an immutable audit trail that inference used certified models without patient data traversing external networks.

#### 4. Sovereign financial risk management and autonomous ledger execution

High-frequency trading and treasury management execute automated trades or rebalance exposure from local model outputs over confidential order books.

```text
[Confidential Order Book] ---> (Provenance Operator: hashes financial feeds and model weights)
                                      | outbound
                                      v
     [High-Spec GPU Host] ---> [g8e Gateway] <--- (Data Operator: executes ledger/broker trade)
     (runs risk engine)               ^
                                      | outbound
     [Execution Runtime]  ---> (Observer: tracks order slippage and execution speed)
```

- **Workflow:** A trading firm runs local risk models that rebalance portfolios or issue margin calls.
- **Provenance Operator:** Verifies integrity of market data feeds and proprietary model weights before order generation.
- **Observer:** Captures execution latency, slippage, and node stability during volatile markets.
- **Data Operator:** Executes trade orders against exchange APIs or blockchain smart contracts.
- **Gateway value:** Regulatory and risk policy blocks state-mutating transactions that exceed pre-set dollar limits or risk ratios even if a local agent misbehaves during market panic.

### Pattern across domains

| Layer | DevSecOps | OT / SCADA | Healthcare | FinTech |
| --- | --- | --- | --- | --- |
| **Provenance** | Git tree and compiler | PLC configs and firmware | DICOM scans and FDA weights | Order book and risk weights |
| **Observer** | Test execution and build | Sensor heat, pressure, latency | PHI isolation and confidence | Latency and slippage |
| **Data Operator** | Merge pull requests | Actuate valves / breakers | Mutate central EHR | Execute trades |
| **Gateway (PDP)** | Code release policy | Physical safety policy | HIPAA / FDA compliance | Treasury / risk limits |

The evaluation programs below instantiate this topology on the reference stack. Model campaigns add Provenance and Observer witness Operators; the native execution-boundary suite adds a networkless target observer. See the [position paper](../core/position_paper.md#57-confidential-edge-execution-with-governed-state-mutation) for the research framing and claim boundaries.

---

## Evaluation programs

| Program | Suite / catalog | Proves | Does not use |
| --- | --- | --- | --- |
| **Execution-boundary** | `core-execution-boundary@1.0.0` | One allowed governed mutation and one doctrine-prohibited equivalent through the real Gateway and remote Operator | g8ee, model providers, campaigns, synthetic simulators |
| **Model campaign** | `north-star-25@1.0.0` (standard scenario catalog) | Governed model scoring through production inference, tool scenarios, provider-boundary hardware telemetry, and storage-side model weight attestation | Direct Ollama calls from the campaign CLI or g8ee |

Both programs persist canonical, content-addressed evidence beneath `.g8e/data/eval/runs/`. Verification is independent of execution: `g8e eval boundary verify` and `g8e eval campaign verify` recompute bindings and signatures without mutating the platform.

---

## CLI surface

The `g8e eval` command tree (alias `g8e evals`) groups platform evaluation commands:

| Command group | Purpose |
| --- | --- |
| `g8e eval boundary run` | Run the native execution-boundary suite |
| `g8e eval boundary verify <run-id>` / `g8e eval boundary show <run-id>` | Verify or inspect a native run |
| `g8e eval campaign …` | Initialize, schedule, execute, publish, verify, and export evaluation model campaigns |
| `g8e eval gate inference …` | Inference-operator status, probe, and acceptance gates |
| `g8e eval gate chat run` | Chat-path vertical acceptance through production `POST /api/v1/chat` |
| `g8e eval models …` | Model inventory freeze, list, and materialize helpers |
| `g8e eval rollout …` | Campaign rollout queue init, run, list, next, and mark helpers |
| `g8e eval dev provider-observer run` | Legacy co-located dev observer only; production uses the enrolled Observer Operator |

Campaign lifecycle commands include `init`, `schedule`, `execute`, `publish`, `verify`, `status`, `show`, `export`, `mirror restore`, and repair helpers. Use `./g8e eval campaign --help` as the command-surface reference.

---

## Native execution-boundary suite

The Phase 1 suite is `core-execution-boundary@1.0.0`. It requires doctrine posture and does not use g8ee, a model provider, model judges, campaigns, scheduling, or synthetic simulators.

### Run

Start and enroll the unified stack with one active remote Operator, then run:

```bash
./g8e eval boundary run
```

Pin an exact active remote Operator session when more than one remote Operator is available:

```bash
./g8e eval boundary run --operator-session <session-id>
```

The suite performs two attempts. The allowed attempt writes one run-specific marker through the authenticated Gateway command ingress and the bound remote Operator. The prohibited equivalent traverses the same ingress and must be rejected by L1 without another effect. Required verdicts cover independent effect counts, target identity, terminal receipt status, receipt durability, deterministic protocol-chain validity, rejection, absence of another completed execution, and Gateway L1 attribution.

### Verify and inspect

Each run persists `report.json`, `verification.json`, and digest-named evidence files under `.g8e/data/eval/runs/<run-id>/`. Re-run verification without executing another mutation:

```bash
./g8e eval boundary verify <run-id>
./g8e eval boundary show <run-id>
```

Add `--json` to emit canonical protojson. Verification resolves every declared artifact, recomputes content addresses, validates report and attempt bindings, verifies receipt and persistence signatures, validates deterministic stage evidence, reconstructs typed observations, recomputes verdicts and metrics, and rejects missing, substituted, contradictory, misbound, or undeclared evidence.

### Trust boundaries

The Gateway is the Policy Decision Point and owns ingress authentication, envelope construction, L1-L3 decisions, and coordination state. The selected remote Operator is the Policy Execution Point and owns L4-L5 execution and authoritative local evidence. The Gateway receipt query is a verified mirror of Operator-authored evidence, not an independent read of the Operator database.

The native suite reads terminal fixture state through a short-lived `g8e-native-target-reader` process defined only in `eval/native-boundary-compose.yml`. It is not an Observer Operator and is not a unified-stack service. The evaluator invokes it explicitly with `docker compose run --rm`; it has no network, workload identity, runtime volume, credentials, or writeable target mount. It mounts only the shared controlled fixture volume read-only and runs with all Linux capabilities dropped so it can read the root-owned, owner-only fixture without mutating the volume or crossing a network boundary.

---

## Model campaign evaluations

Evaluation model campaigns score real models through the production g8ee `POST /api/v1/chat` path, governed inference dispatch, and a frozen scenario catalog. They use one campaign Gateway with **two** core enrolled remote Operator sessions on the campaign host (Data and Inference) plus **one or two** provider-side witness sessions when hardware observation and model provenance are enabled:

| Session | Capability flag | Host | Role |
| --- | --- | --- | --- |
| **Data Operator** | `inference_enabled=false` | Campaign host (Docker) | Governed tool/filesystem/process boundary for model-originated host actions |
| **Inference Operator** | `inference_enabled=true` | Campaign host (Docker) | Governed L4/L5 inference PEP; sole scored path to the approved Ollama provider |
| **Observer Operator** | `provider_boundary_observer_enabled=true` | Provider host (where Ollama/GPU runs) | Read-only GPU and system RAM sampling at the provider execution boundary; no provider lifecycle or generic command authority |
| **Provenance Operator** | `provenance_operator_enabled=true` | Model storage site (where weight blobs live) | Independent SHA-256 attestation of model manifests and weight blobs |

Scored inference never calls Ollama directly from g8ee or the campaign CLI. The Gateway routes inference envelopes to the exact Inference Operator session, tool intents to the exact Data Operator session, `ProviderBoundaryObservationCommand` (BEGIN/FINALIZE) to the exact Observer Operator session, and `ModelProvenanceObservationCommand` (BEGIN/FINALIZE) to the exact Provenance Operator session when provenance is enabled.

See [Model Provenance](model-provenance.md) for the zero-trust weight attestation architecture.

### Why witness operators are separate from the Inference Operator

The Inference Operator, Observer Operator, and Provenance Operator all use the same `g8e operator` binary but enroll as **different remote sessions** with different capability flags. Production witness enrollment on the provider host uses `--provider-boundary-observer-enabled` and/or `--provenance-operator-enabled` only; do **not** pass `--inference-enabled` on the provider host. Witness sessions have no generic command or provider-lifecycle authority; the Gateway and outbound Operator reject command execution on those boundaries.

**1. Provider-host placement.** The Inference Operator runs on the campaign host and calls the remote Ollama HTTP API. GPU VRAM, utilization, temperature, power, clocks, and host RAM must be sampled on the machine where inference actually runs. Model weight blobs must be hashed at the storage site where Ollama keeps content-addressed blobs (for example `~/.ollama/models`). A Linux Docker container on the campaign host cannot authoritatively witness either signal.

**2. Independent witness evidence.** Hardware-efficiency metrics and model provenance are witness evidence, not executor self-report. Model campaigns require typed witnesses at the **provider execution boundary** (Observer) and **model storage boundary** (Provenance Operator) that bind evidence to `provider_attempt_id`. The campaign controller verifies coverage separately from inference receipts.

**3. Least privilege and evidence ownership.** The Observer has no inference backend, no model-management authority, no prompt access, and no access to Inference Operator attempt files. The Provenance Operator reads model manifests and blobs locally but does not run inference, sample GPU state, or mutate weights. Merging either witness into the Inference Operator would let the inference executor attest its own GPU usage or model integrity.

**4. Gateway coordination, separate execution.** When scored inference starts and ends, the Gateway fans out fire-and-forget BEGIN/FINALIZE commands in parallel with inference dispatch:

- `ProviderBoundaryObservationCoordinator` → Observer Operator → `ProviderBoundaryObservationWindow`
- `ModelProvenanceObservationCoordinator` → Provenance Operator → `ModelProvenanceAttestationWindow`

Campaigns without observer coverage remain explicitly incomplete for hardware-efficiency claims until `g8e eval campaign verify --require-provider-observation` passes. Provenance coverage follows the same fail-closed delivery contract when the Provenance Operator is enrolled and campaign bindings carry a model digest.

**5. Same pattern as tool scenarios.** Governed tool assignments require an independent observer that cannot mutate the target. Provider-boundary observation and model provenance apply the same separation to inference-side hardware telemetry and storage-side weight attestation.

### Evaluation witness roles

g8e uses distinct witness mechanisms for native boundary tests and model campaigns:

| Name | Where it runs | Purpose |
| --- | --- | --- |
| **Native target reader** | Ephemeral campaign-host process invoked from `eval/native-boundary-compose.yml` | Networkless target-state read for the native `core-execution-boundary@1.0.0` suite only; not a unified-stack service or Operator |
| **Observer Operator** | Provider host (`g8e operator start --provider-boundary-observer-enabled`) | Enrolled read-only remote Operator for provider-boundary GPU/RAM telemetry during scored model campaigns |
| **Provenance Operator** | Model storage site (`g8e operator start --provenance-operator-enabled --model-storage-root <path>`) | Enrolled remote Operator for storage-side model weight hashing and digest attestation during scored model campaigns |

Only the remote Observer Operator supplies provider-boundary observation for model campaigns. The native target reader cannot satisfy hardware-efficiency requirements, and the Provenance Operator attests model files rather than GPU state.

### Provider-boundary Observer Operator

Deploy the Observer Operator **on the machine that runs Ollama** (for example a remote Windows GPU host), not on the Linux campaign host. Enrollment and startup procedure live in the [Unified Docker Stack Guide](../guides/unified_stack.md#provider-boundary-observer-operator-windows-ollama-host).

What the Observer does:

1. Gateway sends `ProviderBoundaryObservationCommand` (BEGIN/FINALIZE) on the observer's pub/sub cmd channel when scored inference starts and ends.
2. Observer samples GPU VRAM, utilization, temperature, power, clocks, and system RAM between BEGIN and FINALIZE.
3. Observer publishes `ProviderBoundaryObservationCompleted` on its results channel.
4. Gateway ingests windows for `g8e eval campaign verify --require-provider-observation`.

The Observer has no Ollama management capability. Ollama owns its daemon and runner processes. Consecutive scored assignments keep the daemon resident; after a completed queue, the controller reads typed `/api/ps` residency and dispatches the image-baked `/g8e operator model release <served-tag>` command through the exact Inference Operator. That governed command uses the existing `OllamaBackend` HTTP client to send an empty `/api/generate` request with `keep_alive: 0` to the Operator's approved endpoint, after which the controller confirms the campaign-owned tags are absent. No external Ollama CLI is installed or required. The Observer continues to provide only independent BEGIN/FINALIZE telemetry.

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

### Storage-side Provenance Operator

Deploy the Provenance Operator **at the model storage site** — the directory tree that holds Ollama manifests and content-addressed weight blobs. On a typical Ollama deployment this is the same physical host as the Observer (`~/.ollama/models`), but it enrolls as a **separate** governed session with `--provenance-operator-enabled`. Enrollment and startup procedure live in the [Unified Docker Stack Guide](../guides/unified_stack.md#storage-side-provenance-operator).

What the Provenance Operator does:

1. Gateway sends `ModelProvenanceObservationCommand` (BEGIN/FINALIZE) on the provenance operator's pub/sub cmd channel when scored inference starts and ends. BEGIN carries `served_model_tag`, `expected_model_digest`, `model_registry_digest`, and `campaign_id` from the frozen campaign registry.
2. On FINALIZE, the operator reads the Ollama manifest for the served model tag, hashes every referenced blob under `--model-storage-root`, and compares the manifest digest to the expected campaign model digest.
3. The operator publishes `ModelProvenanceObservationCompleted` on its results channel.
4. Gateway ingests attestation windows under `data/inference/model-provenance/windows/` for campaign verification.

**Fail closed:** If observed and expected model digests do not match, FINALIZE fails and the attestation window is not published. This is independent of the Inference Operator's own digest checks — the Provenance Operator is a storage-side witness, not a self-report from the inference executor.

**Timing rule:** Assignments that reached a terminal state before the Provenance Operator was enrolled and pub/sub-connected will lack attestation windows. Enroll the provenance operator before `execute` when chain-of-custody claims are required.

#### Example operator output (provenance)

While a scored campaign runs, the Provenance Operator console shows paired BEGIN/FINALIZE cycles for `MODEL_PROVENANCE_OBSERVATION`:

```text
INFO: Model provenance observation started
  - provider_attempt_id: f7607d6c-6804-4df9-af89-abfe16404212
  - served_model_tag: gemma4:e4b
  - expected_model_digest: 9ce994a6bd3c0985667bac5cdeddbdbba3d1553570a52fe6cad2225ab159d318
INFO: Model provenance observation completed
  - provider_attempt_id: f7607d6c-6804-4df9-af89-abfe16404212
  - served_model_tag: gemma4:e4b
  - observed_model_digest: 9ce994a6bd3c0985667bac5cdeddbdbba3d1553570a52fe6cad2225ab159d318
  - attestation_digest: a1b2c3...
INFO: Model provenance observation completion transmitted
  - event_type: g8e.v1.operator.model.provenance.observation.completed
```

What to verify:

- **BEGIN** lines include the correct `served_model_tag` and `expected_model_digest` for the frozen campaign model.
- **FINALIZE** lines show `observed_model_digest` matching `expected_model_digest` and a non-empty `attestation_digest`.
- `Publishing result` uses event type `g8e.v1.operator.model.provenance.observation.completed` on the session `results:` channel.

See [Model Provenance](model-provenance.md) for the full zero-trust weight attestation architecture.

---

## Evidence and verification

### Storage layout

| Artifact | Path | Owner |
| --- | --- | --- |
| Native run report and verification | `.g8e/data/eval/runs/<run-id>/report.json`, `verification.json`, digest-named evidence | `g8e eval boundary run` / `g8e eval boundary verify` |
| Campaign run state and results | `.g8e/data/eval/runs/<run-id>/` (campaign-scoped lifecycle, assignment, trace, and aggregate records) | `g8e eval campaign …` |
| Provider observation windows (ingested) | Gateway volume under `data/inference/provider-observer/windows/` | Gateway ingest from Observer Operator results |
| Model provenance attestation windows (ingested) | Gateway volume under `data/inference/model-provenance/windows/` | Gateway ingest from Provenance Operator results |

Native verification is owned by `g8e eval boundary verify`. Campaign verification is owned by `g8e eval campaign verify`, with `--require-provider-observation` enforcing hardware-window coverage through the Gateway read API when local evidence is missing. Model provenance attestation windows are verified through the campaign assignment verifier when strict provenance policy is enabled (see [Model Provenance](model-provenance.md)).

### Public spectator projection

The checked-in evaluation explorer reads canonical native and campaign projections from persisted runs. A public-safe projector omits principal, Operator, session, credential, endpoint, path, raw target, envelope, receipt, audit, and evidence body fields before records enter the signed public feed. Native verification remains on the owner path; mirror availability is not verification evidence. See [Public Spectator Architecture](./public_spectator.md).

Campaign assignment results use the enriched campaign projection envelope (`1.1.0`) for new records while historical result envelopes (`1.0.0`) remain readable. The enriched assignment record combines canonical protobuf JSON with named public extensions: scenario context, deterministic and semantic grade summaries, typed activity families, bounded resource metrics, verification metadata, and lowercase SHA-256 evidence bindings. Scenario descriptions and criterion labels come from the exact persisted campaign/catalog bindings; private prompts, gold answers, trace text, execution identifiers, receipt bodies, and artifact locations do not cross the boundary.

Assignment activity preserves the distinction between an observed empty list, unavailable source capture, and scenario-not-applicable. Resource metrics preserve an observed zero and identify unavailable token, retry, or latency values explicitly. Reported tool and policy outcomes are application evidence, not independent protocol authorization or schema-validation claims. Evidence bindings identify approved content-addressed artifacts only; a binding does not prove that the artifact is publicly accessible or individually verified.

Current campaign aggregate records use Evaluation Explorer view schema `1.5.0`. The `evaluation_summary` projection emits typed pass-rate, scored-inference latency p50, and output-throughput p50 metrics with units and observed, eligible, and unavailable contributor counts. Model-role campaign summaries identify `evaluation_unit: "model"`, omit heterogeneous system-only metrics, begin with `verifier_state: "not_run"`, and carry bound report metadata only after an applicable persisted verification result is published. A fresh campaign export writes the same projector-owned record to `evaluation_summary.json`, preserving headline values, coverage, unavailable reasons, verifier state, and report and population bindings without recomputation. Historical view schemas remain readable by the explorer, but new Go projections use only the current schema.

A passing campaign verification report is applicable only when its run, campaign, catalog, model registry, completed population, and verified population match the persisted evidence. When applicable, publication emits report-scoped `exploratory_verified` model-summary revisions for each eligible dataset/variant/role aggregate and republishes verified assignment results. Existing runs can be backfilled through verified catch-up even when the run-level verification summary idempotency key already exists; distinct report-scoped model keys prevent that summary from suppressing model revisions. The catch-up path probes the gateway-owned mirror and resets stale host publication idempotency when the canonical dataset is absent, so a wiped mirror can be restored without editing host state. Failed or inapplicable verification never promotes model quality.

### Public assignment result and quality vocabulary

New terminal assignment results use campaign projection envelope `1.1.0` with `message_type: "PublicAssignmentResultProjection"`. The envelope keeps canonical protobuf JSON in `record` and adds named public extensions for `scenario_summary`, `semantic_grade_summaries`, `activity_summary`, `benchmark_observations`, `resource_summary`, `verification_metadata`, and `evidence_bindings`. Lifecycle envelopes remain `1.0.0`, and readers continue to accept historical assignment-result envelopes from `1.0.0` without enriching or rewriting them.

The public assignment projection contains approved scenario identity and description, closed criterion and grade metadata, grouped model/tool/policy/governed-action activity, bounded scored-inference resource observations, verification disposition, and approved lowercase SHA-256 content bindings. It does not contain prompts, outputs, reasoning, private grade detail, call or transaction identifiers, receipt bodies, provider identities, filesystem paths, or private artifact locations. Reported application tool and policy outcomes do not prove protocol authorization, schema validity, or prevented disclosure.

Activity families distinguish `observed` with an empty record list, `unavailable` with a typed reason, and `not_applicable` for a scenario that does not define that family. The closed public unavailable-reason vocabulary is `historical_not_captured`, `source_not_captured`, `source_unavailable`, `scenario_not_applicable`, `incomplete_contributor_evidence`, and `no_scored_calls`. Resource metrics preserve observed zero and use the same vocabulary when a contributing value is unavailable; no scored calls are unavailable rather than zero. The latency value is the validated elapsed span across the scored inference population, not a lifecycle duration or a sum of call durations.

Model quality verification is run-scoped and bucket-scoped. An applicable passing report publishes `quality_state: exploratory_verified` only for the exact run-derived dataset and eligible `(variant_id, role)` aggregate covered by the verified population. Other datasets, roles, incomplete populations, failed reports, and mismatched bindings remain at their existing quality state, normally `exploratory_partial` or `unavailable`. Historical measurements whose older verifier result does not satisfy the current evidence and verification contract use `legacy_unverified`; they remain visible but never appear as currently verified. `exploratory_verified` means that the named run evidence passed the verifier's scope; it does not mean universal model quality, complete optional telemetry, or `verified_public`. The browser and exports consume the stored revision and never infer or promote quality from an assignment row, run ID, or another dataset.

---

## Implementation

Platform evaluation logic lives in `internal/services/evaluation/`:

- Native suite registry, governed command lane, and deterministic grading
- Independent target observer for the execution-boundary suite
- Campaign controller, publication coordinator, provider-boundary observation integration, and model provenance verification
- Canonical evidence storage and fail-closed verification

Provider-boundary sampling runs in the Observer Operator through `internal/services/operatorcapability/provider_boundary_observer.go` and `internal/services/inference/provider_observer/`. Storage-side model weight attestation runs in the Provenance Operator through `internal/services/operatorcapability/provenance_operator.go` and `internal/services/inference/model_provenance/`. Campaign-owned model release uses typed residency reads and governed model commands through the exact Inference Operator in `internal/services/evaluation/ollama_service.go`. Protocol contracts are defined under `protocol/proto/g8e/eval/v1/`.

---

## Related documentation

- [Position Paper](../core/position_paper.md) — Research framing for confidential edge execution and governed state mutation
- [Unified Docker Stack Guide](../guides/unified_stack.md) — Compose profiles, enrollment order, campaign workflows, and troubleshooting
- [Ensemble Evaluations](../ensemble/evals.md) — How g8ee uses g8e evals through the production chat path
- [Ensemble (g8ee)](./ensemble.md) — g8ee's role in the platform and trust boundaries
- [Model Provenance](./model-provenance.md) — Zero-trust weight attestation, Provenance Operator enrollment, and chain of custody
- [Gateway Architecture](./gateway.md) — Inference dispatch, provider-boundary coordination, and pub/sub
- [Operator Architecture](./operator.md) — L4 Warden, L5 Actuator, and capability flags
- [Public Spectator Architecture](./public_spectator.md) — Public-safe evaluation projections and explorer contract
- [Sovereignty Gauntlet](../guides/sovereignty_gauntlet.md) — Evidence-oriented demonstration and claim-scoping workflow
