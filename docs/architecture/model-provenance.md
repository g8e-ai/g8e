---
title: Model Provenance
parent: Architecture
---

# Model Provenance

Last Updated: 2026-09-24
Version: v2.1.13

## Scope

Model campaigns bind scored inference records to the frozen model variant selected for the campaign. When a campaign model has a served tag and expected SHA-256 digest, the Gateway can coordinate an independent **Provenance Operator** at the model storage site. That Operator reads the Ollama manifest and referenced content-addressed blobs, hashes them locally, and returns a typed attestation window bound to the inference `provider_attempt_id`.

This is evaluation witness evidence, not an authorization mechanism and not a claim that the model itself is safe or that every client-side inference path is governed. The Inference Operator remains the only scored path to the approved Ollama endpoint. The Provenance Operator does not run inference, sample provider hardware, manage Ollama, or authorize execution. The Observer Operator is a separate optional witness for provider-boundary GPU and RAM telemetry; it is described in [Evaluations](evals.md).

The current implementation compares the SHA-256 digest of the local Ollama manifest with the expected campaign model digest and hashes every referenced blob. It records `MODEL_MANIFEST_VERIFICATION_STATUS_UNSIGNED`; Sigstore, Cosign, OpenSSF Model Signing, and SPIFFE/SPIRE-style short-lived attestation tokens are not implemented by this path.

## Operator topology and boundaries

A campaign has two core remote Operators on the campaign host. It may add one or both provider-side witness Operators, depending on the evidence requirements:

| Session | Capability flag | Placement | Role |
| --- | --- | --- | --- |
| **Data Operator** | `inference_enabled=false` | Campaign host | Governed tool, filesystem, and process boundary for model-originated host actions |
| **Inference Operator** | `inference_enabled=true` | Campaign host | Governed L4/L5 inference path to the approved Ollama provider |
| **Observer Operator** | `provider_boundary_observer_enabled=true` | Provider host where Ollama and the GPU run | Read-only GPU and system RAM witness; no provider lifecycle or generic command authority |
| **Provenance Operator** | `provenance_operator_enabled=true` | Model storage site where Ollama manifests and blobs live | Independent manifest and model-weight hashing and digest attestation |

Provenance does not imply that all four sessions are present: the Data and Inference Operators are the core campaign sessions, while the Observer and Provenance Operators are independently enrolled witness sessions. The Observer and Provenance Operator can run on the same physical host, but they use separate `g8e operator start` processes and governed sessions. A container on the campaign host cannot authoritatively attest files or hardware on a different provider host.

All remote Operators connect outbound-only to the Gateway over the governed mTLS/pub-sub path. The Gateway targets the exact Provenance Operator session selected from active remote operators whose runtime configuration has `provenance_operator_enabled=true`. Zero or multiple matching sessions cause preflight or observation setup to fail rather than selecting an arbitrary operator.

## Storage-side attestation

The Provenance Operator is configured with a model storage root, such as `~/.ollama/models`. On `FINALIZE`, its `OllamaStorageAttestor` performs these checks:

1. It resolves `served_model_tag` with Ollama's canonical name parser (`model.ParseName`). Untagged aliases such as `glm-5.3-flash` default to `library/<name>/latest`. Tagged library models such as `smollm3:3b-q4_k_m` resolve under `registry.ollama.ai/library/`. Registry namespaces such as `Impulse2000/smollm3:3b-q4_k_m` keep the supplied namespace. Hugging Face deep pulls such as `huggingface.co/unsloth/Qwen3.8-27B-GGUF:UD-Q4_K_M` resolve under `manifests/huggingface.co/...` on disk.
2. It reads the manifest and computes its SHA-256 digest. The digest is compared with `expected_model_digest` from the campaign binding.
3. It parses the manifest's config and layer digest references, opens each corresponding `blobs/sha256-<digest>` file, hashes the file, and verifies its declared size when present. A missing blob, invalid digest reference, content hash mismatch, or size mismatch fails the attestation.
4. It returns a `ModelProvenanceAttestationWindow` containing the served tag, expected and observed digests, manifest digest, unsigned manifest status, per-blob `ModelWeightAttestation` entries, timestamps, and `digest_match`.
5. The tracker computes `attestation_digest` as a deterministic SHA-256 digest of the window with `attestation_digest` cleared. The Gateway accepts and persists the window only after its content-addressed digest validates.

The operator removes the in-memory BEGIN context when FINALIZE is processed. FINALIZE can use the model binding carried by the command if the BEGIN context is unavailable, but it still requires a served tag and expected digest. A digest mismatch returns an error and does not publish a completion window.

### Enrollment

Run this on the host that owns the Ollama model storage, using a separate session from any Observer Operator:

```bash
g8e operator start \
  --provenance-operator-enabled \
  --provenance-operator-id g8e-model-provenance-1 \
  --model-storage-root /path/to/ollama/models
```

The process submits a platform enrollment request. Approve that request through the owner enrollment workflow before relying on the witness. Do not pass `--inference-enabled` for a storage-only witness. The [Unified Docker Stack Guide](../guides/unified_stack.md#storage-side-provenance-operator) contains provider-host prerequisites, Windows examples, and the operational enrollment sequence.

## Gateway coordination and inference handoff

For each scored inference with a non-empty served tag and expected model digest, the Gateway provenance coordinator targets the active Provenance Operator and sends typed commands on its session-specific command channel:

1. `BEGIN` carries `provider_attempt_id`, start time, retry count, `served_model_tag`, `expected_model_digest`, `model_registry_digest`, and `campaign_id`.
2. The Inference Operator performs the governed inference dispatch. The Provenance Operator does not observe prompts or inference execution.
3. `FINALIZE` carries the attempt and inference transaction identifiers, terminal attempt status, timestamps, retry count, and the same model binding.
4. The Provenance Operator publishes `ModelProvenanceObservationCompleted` on its results channel. The Gateway validates and stores the included window under `.g8e/data/inference/model-provenance/windows/` in the Gateway runtime volume, keyed by `provider_attempt_id`.

The command and result are relayed through the normal governed Operator path. Results are evidence; pub/sub delivery is not itself durable governance evidence. The Gateway's stored window is a verified mirror of the Provenance Operator's attestation, while the storage-side Operator remains the authority for the local files it hashed.

Before execution, strict campaign workflows can run two preflights: one verifies that exactly one active Provenance Operator is enrolled and subscribed to its command channel, and the other sends a probe BEGIN/FINALIZE pair for each frozen served-tag/digest binding and waits for a matching attestation. A failed preflight stops the workflow before scored assignments are consumed. Commands with no provider attempt, served tag, or expected digest are not sent by the coordinator.

## Persistence, API, and verification

Gateway-local windows are canonical protojson files owned by the Gateway's runtime file service. They are private runtime evidence, not public spectator data. An authenticated owner mTLS client can read one window with:

```text
GET /api/v1/inference/model-provenance/attestations/{provider_attempt_id}
```

The response wraps the canonical window in a `window` field. The same route exposes owner-authenticated preflight operations used by campaign execution; these operations return readiness or an error and do not replace verification of the persisted window.

Campaign verification loads windows locally and can fall back to the Gateway read API when local evidence is unavailable. It always validates the window's `attestation_digest` and checks the expected campaign digest binding. Missing windows are recorded as unavailable under interim policy. `g8e eval campaign verify --require-model-provenance` selects strict policy, which requires a valid window for every scored inference and requires `digest_match=true`. On `g8e eval rollout run`, `--require-witness` defaults true and enables strict provider observation and model provenance together; on `g8e eval campaign start` it remains opt-in. Without strict policy, missing provenance is incomplete witness telemetry rather than an automatic assignment-verification failure.

This produces a bounded evidence chain from the frozen campaign model binding to the storage-side manifest and blob hashes and then to the governed inference attempt. It does not prove that an unsigned manifest came from a trusted supply chain, that the Ollama process loaded only those bytes, or that inference performed through a native client or another side channel was governed.

## Protocol surface

| Artifact | Type |
| --- | --- |
| Command | `g8e.eval.v1.ModelProvenanceObservationCommand` |
| Result | `g8e.eval.v1.ModelProvenanceObservationCompleted` |
| Evidence | `g8e.eval.v1.ModelProvenanceAttestationWindow` |
| Blob evidence | `g8e.eval.v1.ModelWeightAttestation` |
| Action | `MODEL_PROVENANCE_OBSERVATION` |

The command has `BEGIN` and `FINALIZE` phases and carries attempt status values for completed or failed provider attempts. The attestation window includes `schema_version` `1.0.0`, the model binding, manifest verification status, per-blob evidence, timestamps, `digest_match`, and `attestation_digest`. See the generated [evaluation API reference](../../protocol/docs/reference/api/g8e/eval/v1/index.md) for the complete wire contract.

## Related documentation

- [Evaluations](evals.md) — campaign topology, witness roles, strict verification, and evidence ownership
- [Unified Docker Stack Guide](../guides/unified_stack.md) — enrollment prerequisites, provider-host deployment, preflight workflow, and troubleshooting
- [Ensemble Evaluations](../ensemble/evals.md) — how g8ee participates in the production chat path during campaigns
- [Operator Architecture](operator.md) — outbound Operator transport and capability boundaries
- [Protocol](protocol.md) — canonical wire types and serialization
