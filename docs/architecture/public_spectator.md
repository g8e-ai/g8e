---
title: Public Spectator Architecture and Threat Model
parent: Architecture
---

# Public Spectator Architecture and Threat Model

Last Updated: 2026-09-10
Version: v2.1.8

## Purpose

This document freezes the architecture, trust boundary, data classification, network path, export binding, threat model, and availability policy for public spectator observation of g8e evaluation campaigns. It defines what a public visitor can see, how safe data reaches them, and what can never cross the projection boundary. The credentialed owner-local observe mode and the anonymous public-spectator mode are separate, non-interchangeable browser modes with distinct authentication, network paths, endpoint allowlists, and disclosure policies. They must never be conflated or combined.

This document is the frozen reference for O0-boundary. Downstream packets (O1-supervisor through O7-readiness) implement against the architecture defined here. The [Generator-Neutral Builder Guide](../guides/build_observe_frontend.md) is extended separately by O5-builder-handoff to cover the public spectator mode in builder-facing terms.

## Two Browser Modes

g8e supports two browser observation modes. Each mode has its own authentication, transport, endpoint allowlist, state store, and disclosure policy. No code path shares state, configuration, credentials, or endpoints between the two modes.

### Owner-local observe mode

The existing audited adapter connects from a top-level browser directly to a reachable Gateway. The browser authenticates with WebAuthn, sends credentials on every request through the HttpOnly session cookie, and reads user-scoped observe and SSE routes. The Gateway derives user identity from the authenticated session and scopes every read to that user's projections. This mode is credentialed, user-scoped, and reaches the private Gateway directly. See [Dashboard (g8ed)](./dashboard.md) and [Generator-Neutral Builder Guide](../guides/build_observe_frontend.md) for the existing owner-local adapter and contract pack.

### Public spectator mode

Arbitrary visitors connect only to a public-safe mirror operated with the OpenDevOps.ai deployment. The local g8e Gateway owns an outbound publisher that exports allowlisted, signed g8e projections, canonical events, and public proof artifacts to the mirror. Public visitors never authenticate to, discover, or connect directly to the private Gateway. The mirror exposes no mutation, eval-launch, approval, producer, audit, filesystem, pub/sub, MCP, A2A, or tool route. Public reads are anonymous, read-only, and bounded by the allowlist and rate policy defined in this document.

### Non-interchangeability

The two modes are non-interchangeable. The public adapter contains no passkey, Gateway, credential, or session fields. It sends no credentials and exposes only public reads against the mirror origin. The owner-local adapter contains no mirror origin and never falls back to a public endpoint. No mode auto-detection, public-to-Gateway fallback, or generic path helper exists. A public visitor cannot reach the private Gateway through the mirror, and an owner cannot reach the mirror through the Gateway's credentialed routes. The two modes share no endpoint allowlist, no fetch implementation, no SSE client, and no state store.

## Network Path

The g8e-owned network path is outbound-only from the private Gateway to the hosted mirror. Public browsers connect to the mirror, not to the Gateway.

```
Private Gateway                        Hosted Mirror                    Public Browser
─────────────────                       ──────────────                  ──────────────
                                          │
  Safe projection store                   │
  (persisted, allowlisted)                │
        │                                 │
        ▼                                 │
  Outbound publisher                      │
  (signs batches, writes outbox)          │
        │                                 │
        ▼ HTTPS (outbound only)           │
  Hosted ingest endpoint ────────────────►│ Ingest (authenticated)
                                          │   ├─ ordered outbox
                                          │   ├─ hash chain
                                          │   ├─ signature verify
                                          │   ├─ snapshot store
                                          │   ├─ proof catalog
                                          │   ├─ SSE relay
                                          │   └─ anonymous read API
                                          │
                                          │ ◄──── GET bootstrap (anonymous)
                                          │ ◄──── GET cycles/runs/evals (anonymous, paginated)
                                          │ ◄──── GET SSE stream (anonymous, replayable)
                                          │ ◄──── GET proof download (anonymous, content-addressed)
                                          │
                                          ▼
                                     Public Browser
```

The private Gateway receives no public connection and gains no anonymous route. The outbound publisher initiates the only connection from the Gateway to the mirror. The mirror accepts authenticated ingest from the publisher and anonymous reads from public browsers. The two surfaces are separate allowlists on the mirror.

SSE connection direction is explicit. A browser initiates an SSE connection to the server it can reach. A private local Gateway cannot initiate an SSE response into a static hosted page. The Gateway therefore pushes signed batches over outbound HTTPS to the hosted mirror, and public browsers open anonymous read-only SSE connections to that mirror. The mirror preserves g8e event schemas, sequence and replay semantics, verification labels, and disclosure boundaries. The mirror does not become a second campaign or governance implementation; it relays signed projections and proofs produced by the Gateway.

The public mirror is a required data-plane companion to the static frontend. A presentation-only site generated by Lovable or another builder is insufficient for an unbounded live stream. The handoff (O5-builder-handoff) defines the mirror ingest, snapshot, SSE, history, and proof APIs without requiring a particular visual framework. The OpenDevOps.ai implementation may use Lovable Cloud/Supabase, Cloudflare Workers with Durable Objects and R2, or equivalent services as long as it passes the generated g8e conformance suite.

## Closed Allowlist

The public spectator surface exposes only a closed allowlist of fields, event types, and artifact classes. Everything not on the allowlist is prohibited. The allowlist is defined here and enforced by the projector, validator, and promoter in S8-publication and by the outbound publisher and mirror in O3-public-feed.

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

The projection function in `ensemble/evals/g8e_evals/projection.py` enforces this allowlist at the eval layer. The outbound publisher and mirror enforce it at the transport layer. Unknown fields, prohibited patterns, non-finite values, path traversal, and symlinks fail closed at every layer.

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

A snapshot binds its high-water sequence (the last accepted sequence) and the feed-chain hash (the hash of the last accepted batch). A public client reconciles against the snapshot to establish a consistent cursor.

### Key rotation and revocation

Signing keys are owner-only secrets stored in the Gateway runtime tree. They never appear in frontend runtime JSON, logs, events, reports, proofs, or contract packs. Key rotation proceeds as follows:

1. The owner generates a new Ed25519 key pair and records the new key ID in the Gateway configuration.
2. The outbound publisher signs subsequent batches with the new key. The batch carries the new signing key ID.
3. The mirror accepts batches signed by either the old or new key during a configurable overlap window.
4. After the overlap window, the old key is revoked. The mirror rejects batches signed with the revoked key ID.
5. A key revocation event is published as a signed batch record before the old key is deactivated. The revocation record carries the revoked key ID, revocation time, and revocation signature.

Key compromise requires immediate revocation. The owner marks the compromised key ID as revoked in the Gateway configuration. The outbound publisher emits a revocation record and switches to the backup key. The mirror rejects any batch signed with the compromised key after the revocation record is accepted. Historical batches signed with the compromised key remain verifiable but carry a compromised-key flag in the mirror's trust metadata.

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

An attacker constructs a fake event payload and injects it into the SSE stream or bootstrap response. Mitigation: every event carries a sequence, content hash, and signature. The mirror verifies the signature before serving. A public client verifies the signature against the published signing key. A forged event fails signature verification and is rejected.

### Mirror tampering

A compromised mirror modifies stored projections, proofs, or event history. Mitigation: every projection and proof is content-addressed by SHA-256. Every batch is signed. A public client or offline verifier recomputes the content hash and verifies the signature. Tampering changes the hash and breaks the chain. The root manifest in each proof package binds the full tree, and a network-disabled verifier reproduces the root.

### Cache poisoning

An attacker injects a poisoned cache entry between the mirror and public browsers. Mitigation: proof downloads are served with fixed safe headers including `Cache-Control: immutable` and `Content-Type` matching the artifact media type. Content-addressed URLs make a poisoned cache entry produce a hash mismatch. The client verifies the SHA-256 of downloaded bytes against the catalog metadata before accepting.

### Artifact substitution

An attacker replaces a proof artifact at a given URL with a different file. Mitigation: proof artifacts are served under immutable content-addressed identities. The URL is derived from the SHA-256 of the artifact bytes. A substituted artifact produces a different URL or a hash mismatch. The client verifies bytes before accepting.

### Path traversal

An attacker crafts a projection or proof path containing `..` or absolute path segments to escape the public artifact root. Mitigation: the projection function rejects traversal in evidence links. The mirror normalizes paths and rejects any path containing `..` or absolute segments. Proof catalog entries are validated against the public artifact root. The `project_to_public` function in `ensemble/evals/g8e_evals/projection.py` checks traversal using `PurePosixPath` normalization.

### Symlinks

An attacker places a symlink in the public artifact tree to escape the root or reference private evidence. Mitigation: the mirror serves only regular files. Symlinks are rejected at ingest and at serve time. The file safety checks in the campaign verifier and source provenance verifier reject symlinks. The mirror's proof catalog does not list symlinked entries.

### Oversized artifacts

An attacker submits an oversized proof or projection to exhaust mirror storage or client bandwidth. Mitigation: the mirror enforces a maximum artifact size at ingest. Proof packages carry a byte length in the catalog metadata. The outbound publisher enforces a maximum batch size. Public downloads are bounded by the catalog-declared byte length, and the mirror rejects a download whose actual size exceeds the declared length.

### Denial of service

An attacker floods the mirror's anonymous read endpoints or SSE stream to exhaust resources. Mitigation: the mirror enforces anonymous-read rate limits, SSE connection limits, and bootstrap response size limits. Anonymous reads are bounded by the rate and stream limits defined below. The mirror may degrade gracefully by serving stale snapshots with a stale freshness label rather than failing completely.

### Correlation leakage

A public visitor attempts to correlate campaign projections or proof metadata with private user identity, session, or deployment information. Mitigation: the public surface carries no user identity, session identity, or real deployment identity. The source deployment pseudonym is a stable but non-reversible identifier. Campaign and run IDs are public-safe identifiers that do not encode user or session information. The projection allowlist excludes any field that could enable correlation.

### Restricted-field publication

A projection or proof accidentally includes a prohibited field. Mitigation: the projection function enforces a closed allowlist and rejects unknown fields. The outbound publisher validates every record against the allowlist before signing. The mirror validates every record against the allowlist before serving. A restricted-field publication fails closed at the first layer that detects it. The `PublicProjection` model in `ensemble/evals/g8e_evals/projection.py` uses `extra="forbid"` and a prohibited-pattern check.

## Anonymous Read Limits, Retention, and Availability

### Anonymous read limits

| Surface | Limit | Default |
| --- | --- | --- |
| Bootstrap response | One bounded snapshot per request | Fixed size, no pagination |
| Cursor-paginated cycles/runs/evals | Page size 1-100 | Default 20 |
| SSE live stream | One connection per client, bounded queue | 100-event in-memory buffer |
| Proof download | One download per request, byte-counted | Maximum artifact size enforced |
| Anonymous read rate | Requests per minute per client IP | Configured by mirror operator |

Anonymous reads never expose mutation, producer, audit, filesystem, pub/sub, MCP, A2A, or tool routes. The public contract contains no mutation or producer operation.

### Stream limits

The SSE stream has a bounded in-memory queue. If a connected consumer falls behind, the mirror drops the oldest queued event rather than blocking the publisher. The persisted copy remains eligible for a later cursor-based replay until retention cleanup removes it. The mirror sends a `truncated` sentinel when the replay limit is reached, so the consumer knows more history may remain.

### Retention

| Data class | Retention | Cleanup |
| --- | --- | --- |
| Event history (SSE rows) | Bounded window | Periodic cleanup removes rows older than the retention window |
| Proof artifacts | Append-only with tombstones | Public retention uses append-only tombstones rather than rewriting history |
| Snapshots | Current and recent | Old snapshots are superseded but not deleted; the high-water sequence advances |
| Outbox (Gateway side) | Until mirror acknowledgment | Acknowledged batches are removed after a confirmation window |

The Gateway never silently deletes reports, encrypted evidence, indexes, or proofs. Storage pressure is reported and requires an explicit owner retention operation.

### Cache policy

Proof downloads are served with `Cache-Control: immutable` because they are content-addressed. Bootstrap and paginated reads are served with short cache durations because they reflect live state. SSE streams are never cached. The mirror may use a CDN for proof artifacts only; live projections and SSE streams bypass the CDN to preserve freshness.

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

The mirror is a read-only data plane. It does not participate in governance, execution, or campaign decisions. A mirror outage pauses public visibility but does not stop the private campaign. The Gateway's outbound publisher writes to a durable ordered outbox and retries idempotently when the mirror recovers. A prolonged mirror outage pauses before the next cycle (O1-supervisor safety stop) while the outbox retains exports. Mirror retry never reruns valid eval work.

## Relationship to Existing Architecture

The public spectator architecture extends the existing observe and SSE infrastructure without modifying the private Gateway's trust boundary:

- The owner-local observe API (`GET /api/v1/observe/*`) remains credentialed and user-scoped. The public spectator surface does not add anonymous routes to the Gateway.
- The SSE event bridge (`GET /api/v1/sse/stream`, `GET /api/v1/sse/events`) remains session-scoped. The public SSE stream is served by the mirror, not by the Gateway.
- The observe producer endpoints (`POST /api/v1/observe/producer/*`) remain mTLS-authenticated and ensemble-only. The outbound publisher is a new Gateway-internal component that reads persisted safe projections and exports signed batches; it does not expose a new producer route to browsers.
- The `PublicProjection` model in `ensemble/evals/g8e_evals/projection.py` defines the eval-layer allowlist. The outbound publisher and mirror enforce the same allowlist at the transport layer.
- The campaign verifier (`ensemble/evals/g8e_evals/campaign_verify.py`) and source provenance verifier (`ensemble/evals/g8e_evals/provenance.py`) produce the verification reports and provenance records that public proofs carry. A public proof is built only from a passing verification report and exact index generation.

See [SSE Streaming](./sse.md) for the existing event bridge, [Dashboard (g8ed)](./dashboard.md) for the owner-local browser interface, [Generator-Neutral Builder Guide](../guides/build_observe_frontend.md) for the audited adapter and contract pack, and [Network Architecture](./network.md) for the PKI and mTLS infrastructure that the outbound publisher uses to authenticate to the mirror.

## Acceptance Criteria

This document is accepted when:

1. Security review approves the allowlist and outbound-only architecture. Anonymous private-Gateway requests remain unauthorized, and the public contract contains no mutation or producer operation.
2. The two browser modes are documented as separate and non-interchangeable, with distinct authentication, transport, endpoint allowlists, and disclosure policies.
3. The closed allowlist covers public live projections, public immutable proofs, and public metadata, with every prohibited field enumerated.
4. The network path is frozen: the Gateway persists a safe projection, the outbound publisher signs and sends it to a hosted ingest endpoint, and public browsers connect to the hosted mirror's snapshot and SSE read endpoints. The private Gateway receives no public connection.
5. Every export is bound to pseudonymous source deployment, schema version, monotonic sequence, prior-batch hash, timestamp, content hash, signing key ID, and signature. Key rotation and revocation are defined.
6. Every threat vector (replay, reordering, duplicate sequence numbers, equivocation, stale snapshots, forged events, mirror tampering, cache poisoning, artifact substitution, traversal, symlinks, oversized artifacts, denial of service, correlation leakage, restricted-field publication) has a defined mitigation that fails closed.
7. Anonymous read limits, stream limits, retention, cache policy, stale and offline semantics, and availability boundaries are defined. Mirror availability is explicitly not verification evidence.

## See Also

- [SSE Streaming](./sse.md): Gateway event publication and browser delivery surfaces.
- [Dashboard (g8ed)](./dashboard.md): Owner-local browser interface and runtime boundaries.
- [Generator-Neutral Builder Guide](../guides/build_observe_frontend.md): Audited adapter and contract pack for generated observe frontends.
- [Network Architecture](./network.md): PKI, mTLS, and transport surfaces.
- [Gateway Architecture](./gateway.md): Gateway services, protocol surfaces, and trust boundaries.
- [Ensemble Evaluation](../ensemble/evals.md): Campaign commands, verification, and evidence semantics.
