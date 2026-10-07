---
doc_id: model-provenance
title: Model Provenance
audience: engineering, campaign operators
status: current
last_updated: 2026-10-06
version: v2.3.2
owners:
  - internal/services/inference/model_provenance/
  - protocol/proto/g8e/eval/v1/eval.proto
related:
  - architecture/operator.md
  - architecture/evals.md
  - architecture/protocol.md
when_to_read: Designing campaign workflows that require model integrity evidence, coordinating storage-side provenance operators, or verifying model weight hashes against expected digests.
do_not_use_for:
  - Inference governance and capability boundaries (see operator.md)
  - Campaign topology and witness roles (see evals.md)
  - Protocol wire contracts (see protocol.md)
---

# Model Provenance

## Purpose

Model campaigns bind scored inference records to the frozen model variant selected for the campaign. When a campaign model has a served tag and expected SHA-256 digest, the Gateway coordinates an independent **Provenance Operator** at the model storage site to attest model integrity. That Operator reads the Ollama manifest and referenced content-addressed blobs, hashes them locally, and returns a typed attestation window bound to the inference `provider_attempt_id`.

This is evaluation witness evidence, not an authorization mechanism and not a claim that the model itself is safe or that every client-side inference path is governed. The Inference Operator remains the only scored path to the approved Ollama endpoint. The Provenance Operator does not run inference, sample provider hardware, manage Ollama, or authorize execution. The Observer Operator is a separate optional witness for provider-boundary GPU and RAM telemetry; it is described in [Evaluations](evals.md).

The current implementation compares the SHA-256 digest of the local Ollama manifest with the expected campaign model digest and hashes every referenced blob, recording `MODEL_MANIFEST_VERIFICATION_STATUS_UNSIGNED` in the attestation window. Sigstore, Cosign, OpenSSF Model Signing, and SPIFFE/SPIRE-style short-lived attestation tokens are not implemented.

## Quick index

- [Purpose](#purpose)
- [Invariants](#invariants)
- [Owned surfaces](#owned-surfaces)
- [Procedures](#procedures)
- [Anti-patterns](#anti-patterns)
- [Links out](#links-out)

Invariant groups: [Operator topology](#operator-topology-inv-prov-topo), [Attestation flow](#attestation-flow-inv-prov-attest), [Evidence persistence and verification](#evidence-persistence-and-verification-inv-prov-persist).

## Invariants

### Operator topology (`INV-PROV-TOPO`)

| ID | Rule |
| --- | --- |
| INV-PROV-TOPO-01 | The Provenance Operator does not run inference, sample provider hardware, manage Ollama, or authorize execution. It attests model weight integrity only. |
| INV-PROV-TOPO-02 | The Gateway selects exactly one active Provenance Operator whose runtime configuration has `provenance_operator_enabled=true`. Zero or multiple matching sessions cause preflight or observation setup to fail. |
| INV-PROV-TOPO-03 | The Observer Operator and Provenance Operator can coexist on the same physical host only when they use separate `g8e operator start` processes and governed sessions. |
| INV-PROV-TOPO-04 | A container on the campaign host cannot authoritatively attest files or hardware on a different provider host. The Provenance Operator remains the storage-side authority. |

### Attestation flow (`INV-PROV-ATTEST`)

| ID | Rule |
| --- | --- |
| INV-PROV-ATTEST-01 | Attestation is triggered only when a campaign model has a non-empty served tag and expected model digest. Commands with missing attempt ID, served tag, or expected digest are not sent. |
| INV-PROV-ATTEST-02 | On FINALIZE, `OllamaStorageAttestor` reads the manifest via `ollama.ParseName`, computes its SHA-256 digest, and hashes every referenced blob under `blobs/sha256-<digest>`. |
| INV-PROV-ATTEST-03 | Digest mismatch, missing blob, invalid digest reference, content hash mismatch, or size mismatch fails attestation and prevents publishing a completion window. |
| INV-PROV-ATTEST-04 | The attestation window records `MODEL_MANIFEST_VERIFICATION_STATUS_UNSIGNED`. Sigstore, Cosign, and SPIFFE/SPIRE attestation tokens are not implemented. |

### Evidence persistence and verification (`INV-PROV-PERSIST`)

| ID | Rule |
| --- | --- |
| INV-PROV-PERSIST-01 | Gateway-local windows are canonical protojson files owned by the Gateway's runtime file service under `.g8e/data/inference/model-provenance/windows/`, keyed by `provider_attempt_id`, which MUST be 1 to 128 ASCII letters, digits, `-`, `_`, or `.` (`models.ValidateProviderAttemptID`) because it names the file. They are private runtime evidence, not public spectator data. |
| INV-PROV-PERSIST-02 | Authenticated owner mTLS clients read windows via GET `/api/v1/inference/model-provenance/attestations/{provider_attempt_id}`, which returns the window wrapped in a `window` field. |
| INV-PROV-PERSIST-03 | Campaign verification validates the window's `attestation_digest` and checks the expected campaign digest binding. `g8e eval runs verify --require-provenance` enforces strict policy, requiring a valid window and `digest_match=true` for every scored inference. |
| INV-PROV-PERSIST-04 | Missing windows are recorded as unavailable telemetry under interim policy. Strict policy requires a valid window for every scored inference. |

## Owned surfaces

| Claim | Path | Verify |
| --- | --- | --- |
| Provenance Operator capability | [internal/services/operatorcapability/provenance_operator.go](../../internal/services/operatorcapability/provenance_operator.go) | `SelectProvenanceOperator` and `ActiveProvenanceOperators` implementations |
| Ollama manifest attestation | [internal/services/inference/model_provenance/attestor.go](../../internal/services/inference/model_provenance/attestor.go) | `OllamaStorageAttestor.Attest` and `attestBlob` implementations |
| Protocol surface | [protocol/proto/g8e/eval/v1/eval.proto](../../protocol/proto/g8e/eval/v1/eval.proto) | `ModelProvenanceAttestationWindow`, `ModelProvenanceObservationCommand`, `ModelProvenanceObservationCompleted`, `ModelWeightAttestation` definitions |
| Attestation window persistence | [internal/services/inference/model_provenance/window_store.go](../../internal/services/inference/model_provenance/window_store.go) | `WindowStore.Load` and `WindowStore.Save` implementations |
| Campaign verification | [internal/services/evaluation/campaign_model_provenance.go](../../internal/services/evaluation/campaign_model_provenance.go) | `CampaignModelProvenanceReader.VerifyAssignmentModelProvenance` implementation |
| Gateway API controller | [internal/services/gateway/model_provenance_controller.go](../../internal/services/gateway/model_provenance_controller.go) | Swagger annotations and route handlers |

## Procedures

### Enroll a Provenance Operator

Run this on the host that owns the Ollama model storage, using a separate session from any Observer Operator:

```bash
g8e operator start \
  --provenance-operator-enabled \
  --provenance-operator-id g8e-model-provenance-1 \
  --model-storage-root /path/to/ollama/models
```

The process submits a platform enrollment request. Approve that request through the owner enrollment workflow before relying on the witness. Do not pass `--inference-enabled` for a storage-only witness. The [Unified Docker Stack Guide](../guides/unified_stack.md#storage-side-provenance-operator) contains provider-host prerequisites, Windows examples, and the operational enrollment sequence.

### Attestation workflow

The Provenance Operator is configured with a model storage root, such as `~/.ollama/models`. On FINALIZE, its `OllamaStorageAttestor` performs these steps:

1. Resolves `served_model_tag` using `ollama.ParseName`. Untagged aliases such as `glm-5.3-flash` default to `library/<name>/latest`. Tagged library models such as `smollm3:3b-q4_k_m` resolve under `registry.ollama.ai/library/`. Registry namespaces such as `Impulse2000/smollm3:3b-q4_k_m` keep the supplied namespace. Hugging Face deep pulls such as `huggingface.co/unsloth/Qwen3.8-27B-GGUF:UD-Q4_K_M` resolve under `manifests/huggingface.co/...` on disk.
2. Reads the manifest and computes its SHA-256 digest, comparing it with `expected_model_digest` from the campaign binding.
3. Parses the manifest's config and layer digest references, opens each corresponding `blobs/sha256-<digest>` file, hashes the file, and verifies its declared size when present.
4. Returns a `ModelProvenanceAttestationWindow` containing the served tag, expected and observed digests, manifest digest, unsigned manifest status, per-blob `ModelWeightAttestation` entries, timestamps, and `digest_match`.
5. The tracker computes `attestation_digest` as a deterministic SHA-256 digest of the window with `attestation_digest` cleared. The Gateway accepts and persists the window only after its content-addressed digest validates.

The operator removes the in-memory BEGIN context when FINALIZE is processed. FINALIZE can use the model binding carried by the command if the BEGIN context is unavailable, but it still requires a served tag and expected digest. A digest mismatch returns an error and does not publish a completion window.

### Gateway coordination and inference handoff

For each scored inference with a non-empty served tag and expected model digest, the Gateway provenance coordinator targets the active Provenance Operator and sends typed commands on its session-specific command channel:

1. `BEGIN` carries `provider_attempt_id`, start time, retry count, `served_model_tag`, `expected_model_digest`, `model_registry_digest`, and `campaign_id`.
2. The Inference Operator performs the governed inference dispatch. The Provenance Operator does not observe prompts or inference execution.
3. `FINALIZE` carries the attempt and inference transaction identifiers, terminal attempt status, timestamps, retry count, and the same model binding.
4. The Provenance Operator publishes `ModelProvenanceObservationCompleted` on its results channel. The Gateway validates and stores the included window under `.g8e/data/inference/model-provenance/windows/` in the Gateway runtime volume, keyed by `provider_attempt_id`.

The command and result are relayed through the normal governed Operator path. Results are evidence; pub/sub delivery is not itself durable governance evidence. The Gateway's stored window is a verified mirror of the Provenance Operator's attestation, while the storage-side Operator remains the authority for the local files it hashed.

Before execution, strict campaign workflows can run two preflights: one verifies that exactly one active Provenance Operator is enrolled and subscribed to its command channel, and the other sends a probe BEGIN/FINALIZE pair for each frozen served-tag/digest binding. The Gateway registers for the exact Operator receipt and completion event before dispatch, verifies the BEGIN and FINALIZE outcomes without polling the window store, and reports progress through the ephemeral `g8e.v1.inference.model.provenance.preflight.updated` SSE event (see [SSE](sse.md#model-provenance-preflight-progress)). A failed preflight stops the workflow before scored assignments are consumed.

The Provenance Operator permits at most two concurrent storage attestations. Additional FINALIZE commands wait for a hashing slot and honor cancellation. Blob hashing checks cancellation between filesystem reads. Every attempt still hashes its own manifest and blobs; hashes are not reused across attempts.

### Read attestation evidence

Campaign verification loads windows locally and can fall back to the Gateway read API when local evidence is unavailable. It always validates the window's `attestation_digest` and checks the expected campaign digest binding.

Before strict provenance verification, `runs verify`, `runs start --require-provenance`, and strict rollout execution wait up to five minutes for the scored attempts' durable completion windows. They check only pending attempt IDs every two seconds and proceed immediately when coverage is complete. This wait is shared across the run; it is not five minutes per inference. A timeout reports the missing-window count and a sample of attempt IDs, and prevents qualification. Read or persistence failures return immediately. The wait neither reruns scoring nor creates an attestation for a missing attempt. Interim verification retains its immediate evidence capture behavior.

```text
GET /api/v1/inference/model-provenance/attestations/{provider_attempt_id}
```

The response wraps the canonical window in a `window` field. The same route exposes owner-authenticated preflight operations used by campaign execution; these operations return readiness or an error and do not replace verification of the persisted window.

Missing windows are recorded as unavailable under interim policy. `g8e eval runs verify --require-provenance` selects strict policy, which requires a valid window for every scored inference and requires `digest_match=true`. On `g8e eval rollout run`, `--require-witness` defaults true and enables strict provider observation and model provenance together; on `g8e eval runs start` it remains opt-in. Without strict policy, missing provenance is incomplete witness telemetry rather than an automatic assignment-verification failure.

## Anti-patterns

- Authoritatively attesting files or hardware on a different host than the Provenance Operator's storage root (INV-PROV-TOPO-04).
- Running the Provenance Operator with `--inference-enabled` on the storage host — the storage witness must be separate from inference (INV-PROV-TOPO-03).
- Signing or validating signatures on unsigned manifests — the attestation records `MODEL_MANIFEST_VERIFICATION_STATUS_UNSIGNED` and does not claim supply chain provenance (INV-PROV-ATTEST-04).
- Relying on missing windows as attestation success — interim policy records unavailable telemetry; strict policy requires a valid window for verification (INV-PROV-PERSIST-04).
- Sending BEGIN or FINALIZE commands when the served tag or expected digest is empty (INV-PROV-ATTEST-01).

## Links out

- [Evaluations](evals.md) — campaign topology, witness roles, strict verification, and evidence ownership
- [Operator Architecture](operator.md) — outbound Operator transport and capability boundaries
- [Protocol](protocol.md) — canonical wire types and serialization
- [Unified Docker Stack Guide](../guides/unified_stack.md) — enrollment prerequisites, provider-host deployment, preflight workflow, and troubleshooting
- [Ensemble Evaluations](../ensemble/evals.md) — how g8ee participates in the production chat path during campaigns
- Generated [evaluation API reference](../../protocol/docs/reference/api/g8e/eval/v1/index.md) — complete wire contract for `ModelProvenanceAttestationWindow`, `ModelProvenanceObservationCommand`, and related types
