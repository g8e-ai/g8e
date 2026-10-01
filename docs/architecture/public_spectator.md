---
doc_id: public_spectator
title: Public Spectator Architecture and Threat Model
audience: architects and security reviewers
status: current
last_updated: 2026-09-29
version: v2.2.4
owners:
  - internal/services/gateway/public_mirror.go
  - internal/services/publicdisclosure/
  - internal/cli/cmd/public/
  - docs/architecture/
related:
  - docs/architecture/sse.md
  - docs/architecture/dashboard.md
  - docs/architecture/gateway.md
  - docs/architecture/network.md
  - docs/guides/public_spectator.md
  - docs/guides/build_observe_frontend.md
when_to_read: Designing, implementing, or reviewing public spectator feed export, mirror infrastructure, disclosure policies, threat mitigations, or browser-facing read endpoints.
do_not_use_for:
  - Deployment configuration procedures (docs/guides/public_spectator.md)
  - Owner-local observe frontend contracts (docs/guides/build_observe_frontend.md)
  - Event bridge implementation (docs/architecture/sse.md)
  - Gateway trust boundaries (docs/architecture/gateway.md)
---

# Public Spectator Architecture and Threat Model

## Purpose

This document defines the architecture, trust boundary, data classification, network path, export binding, threat model, and availability policy for public spectator observation of g8e evaluation campaigns. It specifies what anonymous visitors can see, how safe data reaches them, and what cannot cross the projection boundary. The credentialed owner-local observe mode and the anonymous public-spectator mode are separate, non-interchangeable browser modes with distinct authentication, network paths, endpoint allowlists, and disclosure policies. They must never be conflated or combined.

This is the canonical reference for public spectator architecture. The [Public Spectator Operations Guide](../guides/public_spectator.md) describes deployment and publication procedures. The [Generator-Neutral Builder Guide](../guides/build_observe_frontend.md) describes the separate owner-local observe integration for generated frontends.

## Quick index

- [Purpose](#purpose)
- [Two Browser Modes](#two-browser-modes)
- [Network Path](#network-path)
- [Closed Allowlist](#closed-allowlist)
- [Export Binding](#export-binding)
- [Threat Model](#threat-model)
- [Anonymous Read Limits, Retention, and Availability](#anonymous-read-limits-retention-and-availability)
- [Relationship to Existing Architecture](#relationship-to-existing-architecture)
- [Acceptance Criteria](#acceptance-criteria)

## Invariants

### Authentication and endpoint isolation

Each browser mode has its own authentication mechanism, transport security, endpoint allowlist, state store, and disclosure policy. No code path shares state, configuration, credentials, or endpoints between the two modes.

### Public-safe projection

Every exported record passes through a closed allowlist before becoming browser-visible. The campaign projector, disclosure validator, publisher, and mirror enforce this allowlist at every layer, and validation fails closed.

### Signed export binding

Every exported batch is a cryptographically signed, append-only record bound to source deployment, schema version, monotonic sequence, prior-batch hash, timestamp, content hash, and signature. The chain cannot be reordered, duplicated, or equivocated.

### Network isolation

Public visitors never discover, authenticate to, or connect to the private Gateway. The public mirror exposes only read and streaming endpoints. No mutation, governance, audit, filesystem, credential, or producer route reaches the public listener.

## Two Browser Modes

g8e supports two separate browser observation modes.

### Owner-local observe mode

An authenticated browser connects directly to the private Gateway. The browser authenticates with WebAuthn and receives an HttpOnly session cookie. The Gateway derives user identity from the authenticated session and scopes every read to that user's credentials. This mode is credentialed, user-scoped, and reaches the private Gateway directly. See [Dashboard (g8ed)](./dashboard.md) and [Generator-Neutral Builder Guide](../guides/build_observe_frontend.md) for the existing owner-local adapter and contract pack.

### Public spectator mode

Arbitrary visitors connect only to an anonymous public mirror exposed by the Gateway-owned `PublicSpectatorRuntime` in [internal/services/gateway/public_spectator_runtime.go](../../internal/services/gateway/public_spectator_runtime.go) or by a separately operated compatible mirror. In the unified Compose deployment, the Gateway initializes the publisher and mirror in the Gateway process and persists their state in the Gateway runtime volume. The host `g8e public` commands use the Gateway publication route when the local Gateway is healthy; when a configured remote mirror is used instead, the CLI owns the publisher and its durable outbox. Both paths export allowlisted, signed public-feed projections and public proof artifacts. Public visitors never authenticate to, discover, or connect directly to the private Gateway. The mirror exposes no mutation, eval-launch, approval, producer, audit, filesystem, pub/sub, MCP, A2A, or tool route. Public reads are anonymous, read-only, and bounded by the allowlist and rate policy defined in this document.

### Non-interchangeability

The two modes are non-interchangeable. The public adapter contains no passkey, Gateway, credential, or session fields. It sends no credentials and exposes only public reads against the configured mirror origin. The owner-local adapter contains no mirror origin and never falls back to a public endpoint. No mode auto-detection, public-to-Gateway fallback, or generic path helper exists. A public visitor cannot reach the private Gateway through the mirror, and an owner cannot reach the mirror through the Gateway's credentialed routes. The modes share no endpoint allowlist, fetch implementation, SSE client, or persisted browser state; each mode owns its own state, if any.

## Network Path

The supported unified deployment runs the public mirror inside the Gateway process on listeners and route tables that are separate from the private Gateway API. The campaign publisher enters through the owner-authenticated `POST /api/v1/public-feed/batches` route; the Gateway publisher then sends the signed batch to the private mirror listener. Public browsers reach only the anonymous read listener through the host cloudflared connector and Cloudflare edge; they never connect to the private Gateway API. A remote-mirror deployment instead runs the publisher in the host CLI and sends it to the configured mirror origin.

```
Campaign host                         Gateway container                    Public path
─────────────                         ─────────────────                    ───────────
Canonical campaign records
       │
       ▼ owner mTLS
Gateway publication route ──────────► PublicSpectatorRuntime
                                      ├─ signed ordered batches
                                      ├─ durable mirror state
                                      ├─ hash chain and snapshots
                                      ├─ proof catalog and manifest
                                      ├─ private ingest listener
                                      └─ anonymous read/SSE listener
                                                   │
                                                   ▼ Docker host publication 127.0.0.1:8082
                                             host cloudflared ──outbound tunnel──► Cloudflare edge
                                                                                         │
                                                                                         ▼ HTTPS
                                                                                   Public browser
```

The private Gateway API gains no anonymous route. The in-process public spectator runtime exposes separate private-ingest and anonymous-read allowlists, listeners, and host publications over the same gateway-owned durable volume. Compose publishes ports 8081, 8082, and 5173 on host loopback only; the container listeners remain reachable on the Compose bridge where required. Cloudflared targets the host's loopback-only public port and cannot route public traffic to private ingest or the Gateway API.

SSE connection direction is explicit. A browser opens `GET /stream` through Cloudflare to the mirror's public listener. The mirror preserves public-feed record schemas, sequence and replay semantics, freshness labels, and disclosure boundaries. It does not become a second campaign or governance implementation; it verifies and stores signed batches and proofs admitted from the private publication path, then serves their public projections.

The public mirror is a required data-plane companion to the static frontend. A presentation-only site generated by Lovable or another builder is insufficient for a replayable live stream. The mirror owns bootstrap, snapshot, history, SSE, and proof APIs without requiring a particular visual framework.

### Listener contracts

The private listener accepts authenticated `POST /ingest`, `POST /keys/register`, and `POST /proof-ingest` requests from the publisher. The public listener exposes only `GET /bootstrap`, `GET /snapshot`, `GET /history`, `GET /stream`, `GET /proof-catalog`, `GET /proof-manifest`, and `GET /proofs/<artifact-id>`. Its CORS policy permits anonymous `GET` and `OPTIONS` requests and the `Last-Event-ID` header. Unknown paths and the private operations are not registered on the public listener. The Gateway's primary HTTP and HTTPS routers do not expose these mirror paths.

The runtime is enabled by `--public-spectator`, which is enabled by default in the root Compose service. Its listener flags are `--public-spectator-private-listen`, `--public-spectator-public-listen`, `--eval-explorer-listen`, `--eval-explorer-root`, and `--public-spectator-trusted-proxy-cidr`. In Compose, the Gateway container binds `0.0.0.0:8081`, `0.0.0.0:8082`, and `0.0.0.0:5173`; Docker publishes those listeners to host loopback ports `8081`, `8082`, and `5173`. Outside Compose, listener validation permits loopback addresses only.

The Gateway-owned mirror stores its state at `public-mirror/state.json`; the Gateway publisher stores export configuration, the signing key, ingest token, snapshot, outbox, rotation state, and proof package under the Gateway runtime tree's `public-feed/` and `public-proofs/` paths. These are component-local runtime files in the Gateway volume, not Docker-host files. The remote publisher uses the same relative state paths in the runtime of the process that owns it.

## Closed Allowlist

The public spectator surface exposes only a closed allowlist of fields, event types, and artifact classes. Everything not on the allowlist is prohibited. The campaign projector, disclosure validator, publisher, and mirror enforce this allowlist before a record becomes browser-visible.

### Public live projections

Public live projections carry only the following field families:

- Campaign identity: campaign ID, campaign revision, cycle ID.
- Run and assignment identity: run ID, assignment ID, variant ID, task ID, arm ID, repetition index.
- Exact model-role identity: Primary variant ID, Assistant variant ID, Lite variant ID, role-combination ID. Each is a stable identifier bound to an immutable `ModelVariant` in the frozen model registry.
- Metric identity and values: metric ID, numerator, denominator, rate or value, unit, aggregation method, uncertainty interval when valid, claim status.
- Verification status: verification label, verification boundary, verified index-generation hash.
- Disposition: effective, superseded, qualification, unavailable. Superseded and unavailable dispositions are visible with their typed reason; the underlying private evidence is not.
- Environment class: hardware class label, backend label, quantization policy label. These are categorical labels, not machine-specific identifiers.
- Publication status: cycle status, publication status, source freshness label.
- Evidence link: relative path to a public proof artifact. The link is a content-addressed relative path, never an absolute URL, machine path, or private download location.
- Assignment detail: approved scenario context, typed grade summaries, grouped model/tool/policy/governed-action activity, bounded resource metrics, verification metadata, lowercase SHA-256 content bindings, and the optional model-text extensions `model_response`, `failure_output`, and `role_transcripts`. Activity and resource families retain explicit missingness semantics rather than treating unavailable values as zero.

Assignment detail also carries a bounded, value-free account of why an assignment failed (INV-EVAL-EVID-04). These fields exist only in campaign result envelope `1.2.0`; the Gateway and browser validators reject a `1.1.0` envelope that carries any of them. Per assignment: `trajectory_outcome` (closed vocabulary such as `DIRECT`, `RECOVERED`, `NO_TOOL_CALL`, `WRONG_TOOL`, `WRONG_ARGUMENTS`, `IGNORED_GUIDANCE`, `ABANDONED_AFTER_ERROR`, `CIRCUMVENTED_DENIAL`, `LOOP_EXHAUSTED`, `PROVIDER_REJECTED_TOOL_DECLARATION`), `guided_retry_count`, `failure_reason` (the bounded public sentence, projected from `public_failure_reason` only), and `tools_declared` (the tool names sent to the provider on the scored agent call; triage and memory calls are never the source). Per tool call: `loop_turn`, `error_type`, and `guidance_shown`. Per scenario summary: `trajectory_policy` and `prompt_hint` (hinted tool names and where each argument comes from: prompt, seed, workspace, operator context, or model; never the argument values). The model-visible error, suggestion, and error-analysis text, the hint values, and the start of the model output stay in the private, digest-bound trace and the private `failure_reason`; none of them is projected.

Assignment detail records are public-safe projections, not traces. They may identify approved model variants, roles, tool labels, closed explanation codes, reported outcomes, and content-addressed bindings. They never contain prompts, reasoning, call or transaction identifiers, receipt bodies, filesystem paths, provider identities, private artifact locations, or unrestricted free text, with one bounded exception: the optional `model_response` extension carries the designated-role output the evaluated model produced for that assignment, and `failure_output` carries the recorded error, failure output, or non-`stop` finish reason for an assignment that did not complete. Both come from the run's assignment trace, are published as recorded rather than rewritten, and are not evidence-bound. The public validator rejects a record whose text exceeds 256 KiB or contains `BEGIN PRIVATE KEY` or `spiffe://`; it does not otherwise classify or redact model text, so operators must treat evaluation scenario inputs and provider outputs as public before publication.

The optional `role_transcripts` extension shows what each model role did, lifted from its digest-bound g8ee trace. It is one entry per role (one for homogeneous assignments, three in `lite → assistant → primary` order for g8ee-routed formations). Each entry has `role`, `response`, `finish_reason`, `trace_digest`, and `tool_calls`. Each tool call has `tool_name`, `arguments_json`, `arguments_hash`, `command`, `success`, `error_type`, `result_json`, and `result_redaction`. `arguments_json` is the model's exact arguments as canonical JSON, and `arguments_hash` is `sha256(arguments_json)` as bound into the trace, so a reader can check the arguments shown against the trace. Tool results are capped at 16 KiB (`result_redaction: truncated`). Any response or result that matches the restricted-text rule is withheld (`result_redaction: restricted`, or an empty response) rather than failing the whole publish batch. The gateway validator enforces a closed field set, a closed role and redaction vocabulary, and lowercase SHA-256 digests. Operator, session, transaction and receipt identifiers, provider endpoints, and timestamps are never included. Tool results can contain synthetic scenario fixture content and command output from the evaluation Data Operator, so that Operator must hold only public-safe content. A reported tool or policy outcome is not a protocol authorization decision, and an evidence binding is not proof that a public visitor can retrieve or independently verify the referenced artifact.

### Explorer view schema

The public spectator explorer normalizes accepted snapshot and live records to view schema `1.5.0`. The common view envelope carries `schema_version`, `kind`, `dataset_id`, `quality_state`, and `observed_at`; the browser continues to read historical view records from `1.0.0` through `1.4.0`. The `assignment_result` view keeps required `task_id` and accepts optional `scenario_id`; new records set both to the exact scenario identity, while a record that supplies both with different values is rejected.

View schema `1.5.0` retains the public assignment families `scenario_summary`, `semantic_grade_summaries`, `activity_summary`, `evidence_bindings`, `resource_summary`, and `verification_metadata` and adds typed run-level `headline_metrics` for pass rate, latency p50, and output-throughput p50. Each applicable run metric carries its unit and observed, eligible, and unavailable contributor counts. Evaluation summaries also carry `evaluation_unit`, use `not_run` until a bound verification result exists, and include bound verification metadata when the result is `passed` or `failed`. The view adapter converts canonical campaign enum names and decimal protobuf integers into bounded view values. It preserves explicit zero, distinguishes observed-empty from unavailable and not-applicable activity, and refuses unknown fields, unsupported enums, conflicting identities, malformed hashes, negative or non-finite values, duplicate criterion or evidence identities, and out-of-bounds arrays or strings.

The view quality state `exploratory_verified` is only a stored publication result for the exact dataset, variant, and role aggregate covered by an applicable passing run report. The explorer does not infer this state from an assignment, run ID, badge, or the highest quality state seen in history, and it never changes `exploratory_verified` into `verified_public`. The closed assignment unavailable vocabulary is `historical_not_captured`, `source_not_captured`, `source_unavailable`, `scenario_not_applicable`, `incomplete_contributor_evidence`, and `no_scored_calls`. A missing value is never rendered as zero.

### Disclosure enforcement and offline reproduction

Disclosure enforcement is layered. The Go projector builds only typed approved fields and rejects extension collisions; the producer-side public disclosure validator rejects unknown or prohibited assignment fields and validates the canonical protobuf projection; the publisher and mirror reject malformed records, prohibited patterns, traversal, symlinks, and invalid proof packages; and the frontend validates raw campaign envelopes before adaptation and validates adapted view records before indexing. The receiver remains a fail-closed boundary rather than trusting the producer or the contract description. Private prompts, outputs, reasoning, identities, credentials, endpoints, paths, envelopes, receipt internals, audit data, and evidence bodies remain prohibited at every layer.

Offline reproduction uses the signed public package, not the mirror's availability. A verifier downloads the proof manifest, proof catalog, and content-addressed artifact bytes, records the source pseudonym and feed high-water metadata, then disables network access. It checks that every artifact's SHA-256 matches its catalog entry, that catalog entries match the manifest, that the manifest root recomputes from the declared artifact hashes and metadata, and that the Ed25519 signature verifies against the published signing key. It then validates the exported projection and event records against the frozen `1.5.0` view contract, replays records in sequence order, and compares the resulting high-water and feed-chain hashes with the snapshot. A failed hash, signature, schema, disclosure, binding, or sequence check stops reproduction; it does not produce a partial verified result. The browser explorer performs structural and disclosure validation and does not replace the offline cryptographic verifier. Mirror recovery can republish the same signed package, but it never recomputes evaluation results or upgrades their quality state.

### Public immutable proofs

Public proof packages contain only:

- Safe campaign profile projection and hash.
- Safe model registry projection with immutable model and backend artifact identities and quantization.
- Campaign verification report and exact verified index-generation hash.
- One safe projection for every effective included run plus typed dispositions for unavailable and superseded executions.
- Per-task, repetition-aware comparison rows.
- Typed efficiency observations (resource measurements from the S6 typed observer).
- Statistical analysis record.
- Provenance for the explicit source inclusion manifest.
- Caveats from a closed vocabulary.

Proofs are served under immutable content-addressed identities. Catalog metadata includes filename, media type, bytes, SHA-256, signature and trust metadata, campaign and run source, generated time, classification, verification command, and immutable URL.

### Public metadata

Public metadata includes:

- Source deployment pseudonym (not the real hostname, IP, or PKI identity).
- Protocol and schema version.
- Feed sequence range (first and last sequence in a batch).
- Prior-batch hash.
- Generated timestamp.
- Signing key ID.
- Content hash.
- Signature.
- Source freshness label: active, delayed, stale, intentionally stopped, safety stopped, source offline.

### Prohibited fields

The following never cross the projection boundary:

- Raw prompts and model outputs.
- Chain-of-thought, reasoning traces, and intermediate generation tokens.
- Private evidence, encrypted evidence envelopes, and evidence-key metadata.
- Owner-only projections and user-scoped observe data.
- User identity, session identity, CLI session ID, web session ID, and passkey material.
- Credentials, API keys, private keys, tokens, and passwords.
- Private endpoints, Gateway URLs, machine-specific filesystem paths, and local PKI identity.
- Private download locations and encrypted artifact locations.
- Producer endpoints, audit records, governance envelopes, and receipt internals.
- Mutation, approval, eval-launch, pub/sub, MCP, A2A, and tool route surfaces.
- Any field not in the explicit allowlist above.

The public publisher and mirror enforce the transport allowlist and reject unknown fields, prohibited patterns, non-finite values, path traversal, and symlinks. The evaluation explorer is connected to Go-native evaluation output through a minimal public-safe projector that reads canonical `report.json` and `verification.json` from persisted native runs and emits only the public-safe typed records required by the existing explorer contract.

## Export Binding

Every exported batch is a signed, append-only record bound to the following fields:

| Field | Description |
| --- | --- |
| Source deployment pseudonym | A stable pseudonymous identifier for the source Gateway deployment. Not the real hostname, IP, or SPIFFE identity. |
| Protocol version | The public observer protocol version. |
| Schema version | The projection schema version. |
| First sequence | Monotonic sequence number of the first record in the batch. |
| Last sequence | Monotonic sequence number of the last record in the batch. |
| Previous batch hash | SHA-256 of the previous accepted batch. The first batch carries a fixed zero hash. |
| Record hashes | SHA-256 of each record in the batch, in order. |
| Generated time | Timestamp when the batch was produced, in UTC with monotonic source ordering. |
| Content hash | SHA-256 over the canonical encoding of all record hashes and batch metadata. |
| Signing key ID | Identifier of the signing key used to produce the signature. |
| Signature | Ed25519 signature over the content hash. |

A snapshot binds its high-water sequence (the last accepted sequence) and the feed-chain hash (the hash of the last accepted batch). A public client reconciles against the snapshot to establish a consistent cursor. The snapshot read accepts an optional retained batch-end sequence so a returning browser can validate that its cached snapshot remains on the retained chain before resuming from the cached cursor.

A source transition never splices, renumbers, or deletes an existing accepted chain. The owner stops the publisher, archives its current configuration, signing key, ingest token, outbox, snapshot, key-rotation record, and local proof package, then creates a distinct source pseudonym with a fresh key, token, empty outbox, zero-hash predecessor, and sequence beginning at one. The mirror retains the archived source under its original source identity and marks the newly registered source as active for anonymous reads that omit an explicit source. Explicit source queries continue to reproduce the archived chain. A source transition does not upgrade, relabel, or republish historical datasets.

### Key rotation and revocation

Signing keys are owner-only secrets stored in the host g8e runtime tree. They never appear in frontend runtime JSON, logs, events, reports, proofs, or contract packs. Key rotation proceeds as follows:

Key rotation is coordinated by `g8e public rotate-key` or the publisher's `RotateKey` operation:

1. The owner generates a new Ed25519 key pair and persists rotation state in the runtime tree.
2. The publisher registers the new public key with the private mirror, authenticated by a request signed with the current private key.
3. The publisher emits a key-revocation record signed by the old key, then switches to the new key after the batch is acknowledged.
4. The mirror records the revocation and rejects later batches signed with the revoked key. The CLI finalizes the new key only after the revocation batch is acknowledged; an interrupted rotation remains pending and blocks unrelated publisher commands.

There is no configurable overlap window in the current implementation. Historical batches signed with the old key remain verifiable through the retained key registry, while subsequent batches use the new key. A compromised key requires the owner to complete this rotation workflow; the mirror does not independently infer compromise or revoke a key without a signed registration/revocation path.

## Threat Model

The public spectator surface is threat-modeled against the following attack vectors. Each vector has a defined mitigation, and every mitigation fails closed.

### Replay and reordering

An attacker captures a signed batch and resubmits it to the mirror ingest endpoint or replays it to a public SSE consumer. Mitigation: the mirror rejects any batch whose first sequence is less than or equal to the last accepted sequence. The monotonic sequence and prior-batch hash chain make reordering detectable. A public consumer rejects any event whose sequence is less than or equal to the last consumed sequence. The `Last-Event-ID` header on SSE reconnect prevents replaying already-handled events.

### Duplicate sequence numbers

An attacker submits a batch with a sequence number already accepted. Mitigation: the mirror rejects duplicate sequence numbers by checking the first sequence against the high-water mark. The content hash distinguishes a legitimate retransmission (same hash, idempotent accept) from a forged batch (different hash, rejected).

### Equivocation

A compromised source sends different batches with the same sequence number to different mirrors. Mitigation: the prior-batch hash chain and content hash bind each batch to its position. A public consumer or cross-mirror auditor compares content hashes for the same sequence range. Equivocation produces different content hashes and is detectable. The signing key ID identifies the source, and a detected equivocation triggers key revocation.

### Stale snapshots

A public client receives a stale snapshot that does not reflect the current high-water mark. Mitigation: the snapshot binds its high-water sequence and feed-chain hash. A client compares the snapshot's high-water against the mirror's current high-water. A stale snapshot is labeled as stale, not served as current. The freshness label distinguishes active, delayed, stale, intentionally stopped, safety stopped, and source offline.

### Forged events

An attacker constructs a fake event payload and injects it into the SSE stream or bootstrap response. Mitigation: each feed batch carries ordered records, record hashes, a content hash, and an Ed25519 signature. The mirror verifies the signature and record/hash-chain invariants before persisting or serving the batch. The browser validates the received contract and sequence state; the signed package's offline verifier performs the cryptographic signature check. A forged batch fails mirror or offline verification.

### Mirror tampering

A compromised mirror modifies stored projections, proofs, or event history. Mitigation: every projection and proof is content-addressed by SHA-256. Every batch is signed. A public client or offline verifier recomputes the content hash and verifies the signature. Tampering changes the hash and breaks the chain. The root manifest in each proof package binds the full tree, and a network-disabled verifier reproduces the root.

### Cache poisoning

An attacker injects a poisoned cache entry between the mirror and public browsers. Mitigation: proof downloads are served with fixed safe headers including `Cache-Control: immutable` and `Content-Type` matching the artifact media type. Content-addressed URLs make a poisoned cache entry produce a hash mismatch. The client verifies the SHA-256 of downloaded bytes against the catalog metadata before accepting.

### Artifact substitution

An attacker replaces a proof artifact at a given URL with a different file. Mitigation: proof artifacts are served under immutable content-addressed identities. The URL is derived from the SHA-256 of the artifact bytes. A substituted artifact produces a different URL or a hash mismatch. The client verifies bytes before accepting.

### Path traversal

An attacker crafts a projection or proof path containing `..` or absolute path segments to escape the public artifact root. Mitigation: the publisher rejects traversal in evidence links. The mirror normalizes paths and rejects any path containing `..` or absolute segments. Proof catalog entries are validated against the public artifact root.

### Symlinks

An attacker places a symlink in the public artifact tree to escape the root or reference private evidence. Mitigation: the mirror serves only regular files. Symlinks are rejected at ingest and at serve time. The file safety checks in the campaign verifier and source provenance verifier reject symlinks. The mirror's proof catalog does not list symlinked entries.

### Oversized artifacts

An attacker submits an oversized proof or projection to exhaust mirror storage or client bandwidth. Mitigation: the mirror enforces a maximum artifact size at ingest. Proof packages carry a byte length in the catalog metadata. The outbound publisher enforces a maximum batch size. Public downloads are bounded by the catalog-declared byte length, and the mirror rejects a download whose actual size exceeds the declared length.

### Denial of service

An attacker floods the mirror's anonymous read endpoints or SSE stream to exhaust resources. Mitigation: the mirror enforces anonymous-read rate limits, SSE connection limits, and bootstrap response size limits. Anonymous reads are bounded by the rate and stream limits defined below. The mirror may degrade gracefully by serving stale snapshots with a stale freshness label rather than failing completely.

### Correlation leakage

A public visitor attempts to correlate campaign projections or proof metadata with private user identity, session, or deployment information. Mitigation: the public surface carries no user identity, session identity, or real deployment identity. The source deployment pseudonym is a stable but non-reversible identifier. Campaign and run IDs are public-safe identifiers that do not encode user or session information. The projection allowlist excludes any field that could enable correlation.

### Restricted-field publication

A projection or proof accidentally includes a prohibited field. Mitigation: the outbound publisher validates every record against the closed allowlist before signing. The mirror validates every record against the allowlist before serving. A restricted-field publication fails closed at the first layer that detects it.

## Anonymous Read Limits, Retention, and Availability

### Anonymous read limits

| Surface | Limit | Default |
| --- | --- | --- |
| Bootstrap response | One bounded snapshot per request | Fixed size, no pagination |
| Cursor-paginated cycles/runs/evals | Page size 1-500 | Default 20 |
| SSE live stream | Globally bounded concurrent connections with one bounded queue per connection | 1,000 connections; 100-event in-memory buffer per connection |
| Proof download | One download per request, byte-counted | Maximum artifact size enforced |
| Anonymous read rate | Requests per minute per client IP | Configured by mirror operator; exactly one valid unicast `CF-Connecting-IP` is accepted only when the socket peer belongs to an explicit trusted-proxy CIDR |

Anonymous reads never expose mutation, producer, audit, filesystem, pub/sub, MCP, A2A, or tool routes. The public contract contains no mutation or producer operation.

### Stream limits

The SSE stream has a bounded in-memory queue. If a connected consumer falls behind, the mirror drops the oldest queued event rather than blocking the publisher. The persisted copy remains eligible for a later cursor-based replay until retention cleanup removes it. The mirror sends a `truncated` sentinel when the replay limit is reached, so the consumer knows more history may remain.

### Retention

| Data class | Retention | Cleanup |
| --- | --- | --- |
| Event history (feed records and batches) | Bounded retained prefix | The mirror retains up to 25,000 batches by default and advances the retained predecessor hash when older batches are pruned |
| Proof artifacts and metadata | Append-only in the mirror state | Content-addressed artifacts remain available while their catalog and manifest remain valid; the current implementation has no tombstone cleanup job |
| Snapshots | Current publisher and mirror checkpoints | A snapshot records the current high-water sequence and feed-chain hash; clients reconcile against the retained chain |
| Publisher outbox | Until mirror acknowledgment | The publisher prunes acknowledged entries after its configured acknowledgment window; failed entries remain retryable |

The publisher never silently deletes reports, encrypted evidence, indexes, or proofs. Storage pressure is reported and requires an explicit owner retention operation.

### Cache policy

Proof downloads are served with `Cache-Control: immutable` because they are content-addressed. Bootstrap and paginated reads are served with short cache durations because they reflect live state. History responses use negotiated gzip compression. SSE streams are never cached. A deployment may place a CDN in front of proof artifacts, but that is outside the Gateway runtime; live projections and SSE streams should bypass it to preserve freshness. The Evaluation Explorer persists accepted public records and their sealed snapshot in browser IndexedDB. On reload it resumes from that cursor only after the mirror confirms the cached source, protocol, sequence, and feed-chain checkpoint; an incompatible or pruned checkpoint is discarded and replay starts from retained history.

### Stale and offline semantics

| State | Meaning | Display |
| --- | --- | --- |
| Active | Source is publishing and the mirror is current | Live data, current freshness label |
| Delayed | Source is publishing but the mirror is behind | Last known data with a delayed label |
| Stale | Source has not published within the freshness window | Last known data with a stale label |
| Intentionally stopped | Owner issued a graceful stop | Final state with an intentionally-stopped label |
| Safety stopped | A safety stop fired | Final state with a safety-stop label and typed stop reason |
| Source offline | No heartbeat within the liveness window | Last known data with a source-offline label |

A heartbeat proves source liveness only. The absence of a heartbeat does not mean the source is compromised; it means the source is unreachable or stopped. The mirror displays the last known state with the appropriate freshness label rather than fabricating data or hiding the state.

Mirror availability is not verification evidence. A mirror outage does not invalidate signed proofs, and a mirror recovery does not recompute eval results. The proof root is self-contained and verifiable offline. A public client that downloads a proof and verifies it offline does not depend on mirror availability for verification.

### Availability boundaries

The mirror is a read-only data plane. It does not participate in governance, execution, or campaign decisions. A mirror outage pauses public visibility but does not stop the private campaign. The publisher writes to a durable ordered outbox and retries idempotently when the mirror recovers. Mirror retry never reruns valid evaluation work.

### Gateway-owned listener separation

The production mirror runs inside the Gateway process with `--public-spectator` enabled (the default) and exposes separate private and public listeners over the same durable state in the gateway volume. The listeners bind to the container network interfaces; Compose publishes their host ports on loopback only. The private listener serves authenticated batch ingest, proof ingest, replacement-key registration, and local read diagnostics. The public listener mounts only bootstrap, snapshot, history, SSE, proof catalog, proof manifest, and content-addressed proof downloads. Cloudflared targets only the host-published public listener through an explicit plain-HTTP service URL, so the public hostname has no route to ingest, key registration, proof ingest, the Gateway console, or another private service. The mirror accepts `CF-Connecting-IP` only from the configured Docker bridge peer, and the Gateway container runs with a 16,384 `nofile` soft and hard limit plus `unless-stopped` recovery. See [Public Spectator Operations Guide](../guides/public_spectator.md) for verification and tunnel procedure.

## Relationship to Existing Architecture

The public spectator architecture extends the existing observe and SSE infrastructure without modifying the private Gateway's trust boundary:

- The owner-local observe API (`GET /api/v1/observe/*`) remains credentialed and user-scoped. The public spectator surface does not add anonymous routes to the Gateway.
- The SSE event bridge (`GET /api/v1/sse/stream`, `GET /api/v1/sse/events`) remains session-scoped. The public SSE stream is served by the mirror, not by the Gateway.
- The remaining observe producer endpoints (`POST /api/v1/observe/producer/*`) remain mTLS-authenticated and ensemble-only. The CLI-local public publisher consumes only reviewed public-safe records, signs durable batches, and exports them through the private mirror listener; it does not expose a producer route to browsers.
- The checked-in evaluation explorer in [dashboard/g8e-adapter/evaluation-explorer/](../../dashboard/g8e-adapter/evaluation-explorer/) connects to Go-native evaluation output. A minimal public-safe projector reads canonical native and campaign records from persisted runs under `.g8e/data/eval/runs/<run-id>/` and emits only the public-safe typed records required by the explorer contract. Enriched campaign assignment records use the `1.2.0` campaign result envelope and combine canonical protobuf JSON with named extensions for scenario context, grades, activity, resources, verification metadata, evidence bindings, and the optional bounded `model_response`, `failure_output`, and `role_transcripts` extensions. Historical `1.0.0` assignment result envelopes remain readable.
- Public assignment activity groups by model, tool decision, tool call, policy decision, and governed action families. Each family carries availability semantics so observed empty, unavailable capture, and scenario-not-applicable remain distinguishable. Resource summaries preserve explicit zero and expose bounded latency, token, cache, and retry observations only when their source capture supports them.
- A passing, run-applicable campaign verification publishes report-scoped `exploratory_verified` model-summary revisions for eligible variant/role aggregates and can backfill existing runs through verified catch-up. Catch-up probes the gateway-owned dataset and clears stale host publication idempotency only when the canonical dataset is missing, allowing mirror-volume recovery without manual state edits. The browser displays the stored quality state; it does not infer verification from assignment records or promote a partial model row itself.
- The projected records pass disclosure and contract validation, enter the real `g8e public publish` flow, advance its durable high-water sequence, reach the local mirror, appear under the exact run ID in anonymous mirror history, and are delivered over the real SSE stream. Native evaluation verification is owned by `g8e eval boundary verify`, and campaign verification is owned by `g8e eval runs verify`; mirror availability is not verification evidence.
- The public-safe projection omits all principal, Operator, session, credential, endpoint, path, raw target, envelope, receipt, audit, execution identifier, and evidence body fields.

See [SSE Streaming](./sse.md) for the existing event bridge, [Dashboard (g8ed)](./dashboard.md) for the owner-local browser interface, [Generator-Neutral Builder Guide](../guides/build_observe_frontend.md) for the audited adapter and contract pack, [Public Spectator Operations Guide](../guides/public_spectator.md) for gateway-owned deployment procedure, and [Network Architecture](./network.md) for private platform PKI and transport boundaries.

## Anti-patterns

- Treating public-spectator mode and owner-local observe mode as interchangeable or auto-detecting between them.
- Publishing records that contain prompts, reasoning, identities, credentials, endpoints, or paths without explicit operator review and approval.
- Hand-editing mirror state or proof artifacts instead of using publisher commands.
- Relying on mirror availability as evidence of verification validity.
- Enabling public endpoints without first validating the closed allowlist against the actual publication records.
- Allowing authenticated and anonymous connections to share route handlers or state.

## Owned surfaces

| Claim | Path | Verify |
| --- | --- | --- |
| Public mirror state storage | public-mirror/state.json | Runtime file exists and validates against PublicMirrorStoreState |
| Public feed configuration | public-feed/ directory | Export config, signing key, ingest token, snapshot persist with correct permissions |
| Public publisher commands | `g8e public init`, `g8e public config set`, `g8e public source transition`, `g8e public publish`, `g8e public push`, `g8e public repair-outbox`, `g8e public rotate-key`, `g8e public restore`, `g8e public status`, `g8e public verify-assignment` | Each command implements its documented operation without fallback paths |
| Verification commands | `g8e eval boundary verify`, `g8e eval runs verify` | Verification output reports quality state without mirror involvement |
| Disclosure validation | [internal/services/publicdisclosure/validator.go](../../internal/services/publicdisclosure/validator.go) | Validator rejects prohibited fields and validates against closed allowlist |
| Mirror listener separation | [internal/services/gateway/public_spectator_runtime.go](../../internal/services/gateway/public_spectator_runtime.go) | Private and public listeners are separate; private routes never exposed on public listener |

## Procedures

### Publishing campaign records

1. Run `g8e public init --source-id=<source-id> --mirror-origin=<mirror-url>` on the host to initialize the publisher.
2. The publisher consumes public-safe campaign records supplied to `g8e public publish <records.jsonl>`.
3. The publisher validates each record against the closed allowlist using [internal/services/publicdisclosure/validator.go](../../internal/services/publicdisclosure/validator.go).
4. Valid records are signed, batched, and persisted in the publisher's durable outbox.
5. `g8e public publish <records.jsonl>` sends the batch to the private mirror listener on the authenticated `/api/v1/public-feed/batches` route.
6. The mirror validates the batch signature and compliance, then stores it in public-mirror/state.json.
7. The mirror acknowledges the batch back to the publisher.
8. `g8e public push` retries the durable outbox against the mirror; acknowledged entries are pruned and failed entries remain retryable.

### Key rotation

1. Run `g8e public rotate-key` to generate a new Ed25519 key pair and emit a key-revocation record signed by the old key.
2. The publisher registers the new public key with the mirror, authenticated by the old private key.
3. The mirror records both the revocation and the new key registration.
4. The CLI finalizes the key rotation only after the revocation batch is acknowledged.
5. Subsequent batches are signed with the new key.
6. Historical batches remain verifiable through the mirror's retained key registry.

### Source transitions

1. Run `g8e public source transition --source-id=<new-id> --yes` to archive the current source and create a new one. Without `--yes` the command does not archive the current publisher state.
2. The current publisher configuration, signing key, and outbox are archived.
3. A new source is created with a fresh key, token, empty outbox, zero-hash predecessor, and sequence beginning at one.
4. The mirror retains the old source's full history under its original identity.
5. Anonymous reads that omit an explicit source ID receive the new active source.
6. Explicit source queries continue to reproduce the archived chain.

### Offline verification

1. Download the proof manifest, proof catalog, and all content-addressed artifact bytes.
2. Record the source pseudonym and feed high-water metadata.
3. Disable network access.
4. Verify each artifact's SHA-256 against the catalog entry.
5. Verify that catalog entries match the manifest.
6. Verify that the manifest root recomputes from declared artifact hashes and metadata.
7. Verify the Ed25519 signature against the published signing key.
8. Validate the exported projection and event records against the frozen `1.5.0` view contract.
9. Replay records in sequence order.
10. Compare the resulting high-water and feed-chain hashes with the snapshot.
11. If any check fails, stop and report the failure; do not produce a partial result.

## Links out

- [SSE Streaming](./sse.md): Gateway event publication and browser delivery surfaces.
- [Dashboard (g8ed)](./dashboard.md): Owner-local browser interface and runtime boundaries.
- [Generator-Neutral Builder Guide](../guides/build_observe_frontend.md): Audited adapter and contract pack for generated observe frontends.
- [Public Spectator Operations Guide](../guides/public_spectator.md): Private ingest, anonymous public listener, tunnel, restart, and publication procedure.
- [Network Architecture](./network.md): PKI, mTLS, and transport surfaces.
- [Gateway Architecture](./gateway.md): Gateway services, protocol surfaces, and trust boundaries.
- [Evaluations](./evals.md): Go-native execution-boundary commands, model campaign evidence, Observer and Provenance Operator witness roles, verification, and the connected evaluation explorer projection.
- [Model Provenance](./model-provenance.md): Storage-side weight attestation and chain-of-custody for scored inference.
