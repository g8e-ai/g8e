---
title: Model Provenance
parent: Architecture
---

# Zero Trust Model Provenance

Last Updated: 2026-09-18

## Scope

Model campaigns already bind scored inference to a frozen `ModelRegistryFreeze` at campaign init and witness provider-boundary hardware telemetry through the **Observer Operator**. The **Provenance Operator** adds a storage-side attestor that independently hashes model weight blobs at the site where `.gguf` / Ollama blobs live, producing a cryptographic chain-of-custody record for every scored inference attempt.

This document describes the evaluation-time architecture. Signed manifest verification (Sigstore, Cosign, OpenSSF Model Signing) and short-lived SPIFFE/SPIRE-style attestation tokens are planned follow-ons; the current implementation performs fail-closed digest binding against the frozen campaign model digest.

---

## Operator topology

North Star model campaigns use **four** distinct remote operator sessions when provenance is enabled:

| Session | Capability flag | Host | Role |
| --- | --- | --- | --- |
| **Data Operator** | default | Campaign host | Governed tool/filesystem boundary |
| **Inference Operator** | `inference_enabled=true` | Campaign host | Governed path to Ollama |
| **Observer Operator** | `provider_boundary_observer_enabled=true`; optional `provider_boundary_observer_ollama_enabled=true` (`--ollama`) | Provider host (GPU/Ollama runtime) | Read-only GPU/RAM witness telemetry; optional governed Ollama service restart on the provider host |
| **Provenance Operator** | `provenance_operator_enabled=true` | Model storage site (blob store) | Independent model weight hashing and digest attestation |

The Observer and Provenance Operator may run on the same physical host (for example a Windows Ollama box where `~/.ollama/models` is local), but they are enrolled as separate governed operator sessions with separate capability flags.

---

## Storage-side operations (Provenance Operator)

When a model is present in the enterprise registry, the Provenance Operator acts as gatekeeper and attestor for model weights:

1. **Cryptographic hashing** — On `FINALIZE`, the operator reads the Ollama manifest for the served model tag, hashes every referenced content-addressed blob under `--model-storage-root`, and records per-blob `ModelWeightAttestation` entries.
2. **Digest verification** — The manifest digest (SHA-256 of manifest bytes) is compared to the `expected_model_digest` carried in the governed `ModelProvenanceObservationCommand` from the frozen campaign registry. Mismatch fails closed.
3. **Attestation window** — A `ModelProvenanceAttestationWindow` is minted, content-addressed by `attestation_digest`, and published on the operator results channel for gateway ingest.

### Enrollment

```bash
g8e operator start \
  --provenance-operator-enabled \
  --provenance-operator-id g8e-model-provenance-1 \
  --model-storage-root /path/to/ollama/models
```

---

## Inference handoff (zero trust handshake)

For every scored inference dispatch in campaign mode:

1. Gateway sends `ModelProvenanceObservationCommand` `BEGIN` with `served_model_tag`, `expected_model_digest`, `model_registry_digest`, and `campaign_id`.
2. Governed inference proceeds through the Inference Operator.
3. Gateway sends `FINALIZE`; the Provenance Operator attests blobs and publishes `ModelProvenanceObservationCompleted`.
4. Gateway ingests attestation windows under `.g8e/data/inference/model-provenance/windows/`.

If the Provenance Operator is enrolled and campaign bindings include a model digest, provenance command delivery failures fail closed the same way as provider-boundary observation gaps.

---

## Governance audit trail

Campaign verification can load attestation windows by `provider_attempt_id` and enforce:

- `attestation_digest` integrity
- `digest_match == true` under strict policy
- `observed_model_digest` equals the frozen campaign model digest

This yields an auditable chain of custody from frozen registry → storage-side weight hashes → governed inference record.

---

## Protocol surface

| Artifact | Type |
| --- | --- |
| Command | `g8e.eval.v1.ModelProvenanceObservationCommand` |
| Result | `g8e.eval.v1.ModelProvenanceObservationCompleted` |
| Evidence | `g8e.eval.v1.ModelProvenanceAttestationWindow` |
| Action | `MODEL_PROVENANCE_OBSERVATION` |

---

## Related docs

- [Evaluations](evals.md) — campaign operator topology, Observer Operator, and verification
- [Unified Docker Stack Guide](../guides/unified_stack.md) — enrollment order, provider-host deployment, and troubleshooting
- [Ensemble Evaluations](../ensemble/evals.md) — how g8ee participates in the production chat path during campaigns
- [Operator architecture](operator.md) — governed PEP pattern shared by all remote operators
