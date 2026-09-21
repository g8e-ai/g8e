---
title: Public Spectator Architecture and Threat Model
parent: Architecture
---

# Public Spectator Architecture and Threat Model

Last Updated: 2026-09-20
Version: v2.1.11

## Purpose

This document freezes the architecture, trust boundary, data classification, network path, export binding, threat model, and availability policy for public spectator observation of g8e evaluation campaigns. It defines what a public visitor can see, how safe data reaches them, and what can never cross the projection boundary. The credentialed owner-local observe mode and the anonymous public-spectator mode are separate, non-interchangeable browser modes with distinct authentication, network paths, endpoint allowlists, and disclosure policies. They must never be conflated or combined.

This document is the frozen reference for O0-boundary. Downstream packets (O1-supervisor through O7-readiness) implement against the architecture defined here. The [Generator-Neutral Builder Guide](../guides/build_observe_frontend.md) is extended separately by O5-builder-handoff to cover the public spectator mode in builder-facing terms.

## Two Browser Modes

g8e supports two browser observation modes. Each mode has its own authentication, transport, endpoint allowlist, state store, and disclosure policy. No code path shares state, configuration, credentials, or endpoints between the two modes.

### Owner-local observe mode

The existing audited adapter connects from a top-level browser directly to a reachable Gateway. The browser authenticates with WebAuthn, sends credentials on every request through the HttpOnly session cookie, and reads user-scoped observe and SSE routes. The Gateway derives user identity from the authenticated session and scopes every read to that user's projections. This mode is credentialed, user-scoped, and reaches the private Gateway directly. See [Dashboard (g8ed)](./dashboard.md) and [Generator-Neutral Builder Guide](../guides/build_observe_frontend.md) for the existing owner-local adapter and contract pack.

### Public spectator mode

Arbitrary visitors connect only to a public-safe mirror operated with the OpenDevOps.ai deployment. The local `g8e public` publisher exports allowlisted, signed g8e projections, canonical events, and public proof artifacts to the mirror through a durable host-runtime outbox. Public visitors never authenticate to, discover, or connect directly to the private Gateway. The mirror exposes no mutation, eval-launch, approval, producer, audit, filesystem, pub/sub, MCP, A2A, or tool route. Public reads are anonymous, read-only, and bounded by the allowlist and rate policy defined in this document.

### Non-interchangeability

The two modes are non-interchangeable. The public adapter contains no passkey, Gateway, credential, or session fields. It sends no credentials and exposes only public reads against the mirror origin. The owner-local adapter contains no mirror origin and never falls back to a public endpoint. No mode auto-detection, public-to-Gateway fallback, or generic path helper exists. A public visitor cannot reach the private Gateway through the mirror, and an owner cannot reach the mirror through the Gateway's credentialed routes. The two modes share no endpoint allowlist, no fetch implementation, no SSE client, and no state store.

## Network Path

The g8e-owned network path is outbound-only from the private deployment to the hosted mirror. Public browsers connect to the mirror, not to the Gateway.

```
Private g8e deployment                 Hosted Mirror                    Public Browser
──────────────────────                  ──────────────                   ──────────────
                                          │
  Safe publication input                  │
  (persisted, allowlisted)                │
        │                                 │
        ▼                                 │
  CLI-local publisher                     │
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

The private Gateway receives no public connection and gains no anonymous route. The CLI-local publisher initiates authenticated ingest to the mirror. The mirror accepts authenticated ingest from that private publisher and anonymous reads from public browsers. The two surfaces are separate allowlists and separate listeners in the host-backed deployment.

SSE connection direction is explicit. A browser initiates an SSE connection to the server it can reach. A private local Gateway cannot initiate an SSE response into a static hosted page. The CLI-local publisher therefore pushes signed batches over an outbound connection to the mirror, and public browsers open anonymous read-only SSE connections to that mirror. The mirror preserves g8e event schemas, sequence and replay semantics, verification labels, and disclosure boundaries. The mirror does not become a second campaign or governance implementation; it relays signed projections and proofs produced by the private deployment.

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
- Assignment detail: approved scenario context, typed grade summaries, grouped model/tool/policy/governed-action activity, bounded resource metrics, verification metadata, and lowercase SHA-256 content bindings. Activity and resource families retain explicit missingness semantics rather than treating unavailable values as zero.

Assignment detail records are public-safe projections, not traces. They may identify approved model variants, roles, tool labels, closed explanation codes, reported outcomes, and content-addressed bindings. They never contain prompts, outputs, reasoning, call or transaction identifiers, receipt bodies, filesystem paths, provider identities, private artifact locations, or unrestricted free text. A reported tool or policy outcome is not a protocol authorization decision, and an evidence binding is not proof that a public visitor can retrieve or independently verify the referenced artifact.

### Explorer view 1.4.0 semantics

The public spectator explorer normalizes accepted snapshot and live records to view schema `1.4.0`. The common view envelope carries `schema_version`, `kind`, `dataset_id`, `quality_state`, and `observed_at`; the browser continues to read historical view records from `1.0.0` through `1.3.0`. The `assignment_result` view keeps required `task_id` and accepts optional `scenario_id`; new records set both to the exact scenario identity, while a record that supplies both with different values is rejected.

View schema `1.4.0` adds the public assignment families `scenario_summary`, `semantic_grade_summaries`, `activity_summary`, `evidence_bindings`, `resource_summary`, and `verification_metadata` while retaining the historical benchmark and metric fields. The view adapter converts canonical campaign enum names and decimal protobuf integers into bounded view values. It preserves explicit zero, distinguishes observed-empty from unavailable and not-applicable activity, and refuses unknown fields, unsupported enums, conflicting identities, malformed hashes, negative or non-finite values, duplicate criterion or evidence identities, and out-of-bounds arrays or strings.

The view quality state `exploratory_verified` is only a stored publication result for the exact dataset, variant, and role aggregate covered by an applicable passing run report. The explorer does not infer this state from an assignment, run ID, badge, or the highest quality state seen in history, and it never changes `exploratory_verified` into `verified_public`. The closed assignment unavailable vocabulary is `historical_not_captured`, `source_not_captured`, `source_unavailable`, `scenario_not_applicable`, `incomplete_contributor_evidence`, and `no_scored_calls`. A missing value is never rendered as zero.

### Disclosure enforcement and offline reproduction

Disclosure enforcement is layered. The Go projector builds only typed approved fields and rejects extension collisions; the producer-side public disclosure validator rejects unknown or prohibited assignment fields and validates the canonical protobuf projection; the publisher and mirror reject malformed records, prohibited patterns, traversal, symlinks, and invalid proof packages; and the frontend validates raw campaign envelopes before adaptation and validates adapted view records before indexing. The receiver remains a fail-closed boundary rather than trusting the producer or the contract description. Private prompts, outputs, reasoning, identities, credentials, endpoints, paths, envelopes, receipt internals, audit data, and evidence bodies remain prohibited at every layer.

Offline reproduction uses the signed public package, not the mirror's availability. A verifier downloads the proof manifest, proof catalog, and content-addressed artifact bytes, records the source pseudonym and feed high-water metadata, then disables network access. It checks that every artifact's SHA-256 matches its catalog entry, that catalog entries match the manifest, that the manifest root recomputes from the declared artifact hashes and metadata, and that the Ed25519 signature verifies against the published signing key. It then validates the exported projection and event records against the frozen `1.4.0` view contract, replays records in sequence order, and compares the resulting high-water and feed-chain hashes with the snapshot. A failed hash, signature, schema, disclosure, binding, or sequence check stops reproduction; it does not produce a partial verified result. Mirror recovery can republish the same signed package, but it never recomputes evaluation results or upgrades their quality state.

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

### Key rotation and revocation

Signing keys are owner-only secrets stored in the host g8e runtime tree. They never appear in frontend runtime JSON, logs, events, reports, proofs, or contract packs. Key rotation proceeds as follows:

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
| Anonymous read rate | Requests per minute per client IP | Configured by mirror operator; Cloudflare's connecting IP is accepted only from the loopback tunnel connector |

Anonymous reads never expose mutation, producer, audit, filesystem, pub/sub, MCP, A2A, or tool routes. The public contract contains no mutation or producer operation.

### Stream limits

The SSE stream has a bounded in-memory queue. If a connected consumer falls behind, the mirror drops the oldest queued event rather than blocking the publisher. The persisted copy remains eligible for a later cursor-based replay until retention cleanup removes it. The mirror sends a `truncated` sentinel when the replay limit is reached, so the consumer knows more history may remain.

### Retention

| Data class | Retention | Cleanup |
| --- | --- | --- |
| Event history (SSE rows) | Bounded window | Periodic cleanup removes rows older than the retention window |
| Proof artifacts | Append-only with tombstones | Public retention uses append-only tombstones rather than rewriting history |
| Snapshots | Current and recent | Old snapshots are superseded but not deleted; the high-water sequence advances |
| Publisher outbox | Until mirror acknowledgment | Acknowledged batches are removed after a confirmation window |

The publisher never silently deletes reports, encrypted evidence, indexes, or proofs. Storage pressure is reported and requires an explicit owner retention operation.

### Cache policy

Proof downloads are served with `Cache-Control: immutable` because they are content-addressed. Bootstrap and paginated reads are served with short cache durations because they reflect live state. History responses use negotiated gzip compression. SSE streams are never cached. The mirror may use a CDN for proof artifacts only; live projections and SSE streams bypass the CDN to preserve freshness. The Evaluation Explorer persists accepted public records and their sealed snapshot in browser IndexedDB. On reload it resumes from that cursor only after the mirror confirms the cached source, protocol, sequence, and feed-chain checkpoint; an incompatible or pruned checkpoint is discarded and replay starts from retained history.

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

The mirror is a read-only data plane. It does not participate in governance, execution, or campaign decisions. A mirror outage pauses public visibility but does not stop the private campaign. The CLI-local publisher writes to a durable ordered outbox and retries idempotently when the mirror recovers. A prolonged mirror outage pauses before the next cycle (O1-supervisor safety stop) while the outbox retains exports. Mirror retry never reruns valid eval work.

### Gateway-owned listener separation

The production mirror runs inside the Gateway process (`--public-spectator`) with two unique loopback-only listeners over the same durable state in the gateway volume. The private listener serves authenticated batch ingest, proof ingest, replacement-key registration, and local read diagnostics. The public listener mounts only bootstrap, snapshot, history, SSE, proof catalog, proof manifest, and content-addressed proof downloads. Cloudflared targets only the public listener through an explicit plain-HTTP service URL, so the public hostname has no route to ingest, key registration, proof ingest, the Gateway console, or another private service. See the [Public Spectator Operations Guide](../guides/public_spectator.md) for verification and tunnel procedure.

## Relationship to Existing Architecture

The public spectator architecture extends the existing observe and SSE infrastructure without modifying the private Gateway's trust boundary:

- The owner-local observe API (`GET /api/v1/observe/*`) remains credentialed and user-scoped. The public spectator surface does not add anonymous routes to the Gateway.
- The SSE event bridge (`GET /api/v1/sse/stream`, `GET /api/v1/sse/events`) remains session-scoped. The public SSE stream is served by the mirror, not by the Gateway.
- The remaining observe producer endpoints (`POST /api/v1/observe/producer/*`) remain mTLS-authenticated and ensemble-only. The CLI-local public publisher consumes only reviewed public-safe records, signs durable batches, and exports them through the private mirror listener; it does not expose a producer route to browsers.
- The checked-in evaluation explorer in `dashboard/g8e-adapter/evaluation-explorer/` is connected to Go-native evaluation output. A minimal public-safe projector reads canonical native and campaign records from persisted runs under `.g8e/data/eval/runs/<run-id>/` and emits only the public-safe typed records required by the explorer contract. Enriched campaign assignment records use the `1.1.0` campaign result envelope and combine canonical protobuf JSON with named extensions for scenario context, grades, activity, resources, verification metadata, and evidence bindings. Historical `1.0.0` assignment result envelopes remain readable.
- Public assignment activity is grouped by model, tool decision, tool call, policy decision, and governed action families. Each family carries availability semantics so observed empty, unavailable capture, and scenario-not-applicable remain distinguishable. Resource summaries preserve explicit zero and expose bounded latency, token, cache, and retry observations only when their source capture supports them.
- A passing, run-applicable campaign verification publishes report-scoped `exploratory_verified` model-summary revisions for eligible variant/role aggregates and can backfill existing runs through verified catch-up. Catch-up probes the gateway-owned dataset and clears stale host publication idempotency only when the canonical dataset is missing, allowing mirror-volume recovery without manual state edits. The browser displays the stored quality state; it does not infer verification from assignment records or promote a partial model row itself.
- The projected records pass disclosure and contract validation, enter the real `g8e public` publisher, advance its durable high-water sequence, reach the local mirror, appear under the exact run ID in anonymous mirror history, and are delivered over the real SSE stream. Native evaluation verification is owned by `g8e eval boundary verify`, and campaign verification is owned by `g8e eval campaign verify`; mirror availability is not verification evidence.
- The public-safe projection omits all principal, Operator, session, credential, endpoint, path, raw target, envelope, receipt, audit, execution identifier, and evidence body fields.

See [SSE Streaming](./sse.md) for the existing event bridge, [Dashboard (g8ed)](./dashboard.md) for the owner-local browser interface, [Generator-Neutral Builder Guide](../guides/build_observe_frontend.md) for the audited adapter and contract pack, [Public Spectator Operations Guide](../guides/public_spectator.md) for gateway-owned deployment procedure, and [Network Architecture](./network.md) for private platform PKI and transport boundaries.

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
- [Public Spectator Operations Guide](../guides/public_spectator.md): Private ingest, anonymous public listener, tunnel, restart, and publication procedure.
- [Network Architecture](./network.md): PKI, mTLS, and transport surfaces.
- [Gateway Architecture](./gateway.md): Gateway services, protocol surfaces, and trust boundaries.
- [Evaluations](./evals.md): Go-native execution-boundary commands, model campaign evidence, Observer and Provenance Operator witness roles, verification, and the connected evaluation explorer projection.
- [Model Provenance](./model-provenance.md): Storage-side weight attestation and chain-of-custody for scored inference.
