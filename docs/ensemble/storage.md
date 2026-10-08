---
doc_id: ensemble-storage
title: Ensemble Storage
audience: maintainers and coding agents
status: current
last_updated: 2026-10-06
version: v2.3.2
owners:
  - ensemble/app/clients/
  - ensemble/app/services/data/
  - internal/services/gateway/
  - internal/constants/collections.go
related:
  - ../architecture/storage.md
  - ../architecture/governance.md
  - architecture.md
  - governance.md
  - pki.md
  - llm-providers.md
when_to_read: Understanding how g8ee interacts with Gateway storage services, collection access policies, cache strategies, attachment handling, data protection boundaries, and survival across restarts.
do_not_use_for:
  - Platform storage topology and encryption (docs/architecture/storage.md)
  - Governance verification layers (docs/architecture/governance.md)
  - Operator substrate and local stores (docs/architecture/operator.md)
  - PKI and enrollment (ensemble/pki.md)
---

# Ensemble Storage

## Purpose

g8ee stores durable application records and attachment objects through the Gateway's authenticated document and blob services. The Gateway persists these in its local `g8e.db`; active model turns, pending application approvals, background task tracking, and result correlations remain in g8ee process memory and do not survive a restart. g8ee also persists its enrolled certificate, private key, trust bundle, and resumable enrollment state in its runtime volume, which are process identity credentials rather than application records. A target Operator retains its authoritative receipts, audit events, execution output, file-mutation evidence, replay state, and optional file ledger on that host. See [Storage Architecture](../architecture/storage.md) for the platform storage topology and protection applied to each store.

## Quick index

- [Purpose](#purpose)
- [Invariants](#invariants)
- [Owned surfaces](#owned-surfaces)
- [Procedures](#procedures)
- [Anti-patterns](#anti-patterns)
- [Links out](#links-out)

Invariant groups: [Document collections and access](#document-collections-and-access-inv-collections), [Document access without an app cache](#document-access-without-an-app-cache-inv-cache), [Attachment handling](#attachment-handling-inv-attachments), [State roots and governance](#state-roots-and-governance-inv-governance), [Data protection and persistence](#data-protection-and-persistence-inv-protection).

## Invariants

### Document collections and access (`INV-COLLECTIONS`)

| ID | Rule |
| --- | --- |
| INV-COLLECTIONS-01 | The document store organizes JSON records by collection and identifier. g8ee uses it for cases, investigations, tasks, memories, settings, users, Operator workflow records, certificate-revocation records, agent activity metadata, reputation state and commitments, and stake resolutions. Conversation entries are embedded in investigation documents and linked by entry hashes. Case and investigation `status` fields are typed protocol enums (`CaseStatus`, `InvestigationStatus`); the ensemble models validate them on construction and reject unknown values. |
| INV-COLLECTIONS-02 | Protected application collections (cases, investigations, tasks, memories, agent_activity_metadata, reputation_state, reputation_commitments, stake_resolutions) require GovernanceEnvelope submission; the Gateway rejects direct mutations with a 409 Conflict redirect to `/api/v1/governance/envelopes`. |
| INV-COLLECTIONS-03 | Direct-mutation allowed collections (settings, users, operators, operator_sessions, bound_sessions, passkey_challenges, revoked_certificates, trusted_signers, console_audit) bypass governance validation. The api_keys collection is not on this allowlist and requests are rejected. |
| INV-COLLECTIONS-04 | Authenticated reads retrieve individual records or filter a collection with comparison operators, one ordering field, and a result limit. g8ee applies field projection after the Gateway returns a query and always retains the document `id`. Document replacement overwrites the full stored record; merge updates are applied by the Gateway. |
| INV-COLLECTIONS-05 | Client-side array helpers use read-modify-write cycles and batch helpers send operations one at a time. These operations are not atomic across concurrent writers or across a batch. |

### Document access without an app cache (`INV-CACHE`)

| ID | Rule |
| --- | --- |
| INV-CACHE-01 | g8ee keeps no document or query cache. `DocumentService` (`ensemble/app/db/document_service.py`) is the single owner of Gateway document access, and every read is an authoritative read through the Gateway document API. g8ee has no key-value client and the Gateway exposes no `/api/v1/kv/` surface. |
| INV-CACHE-02 | Gateway document writes touch no `kv_store` row. Every `kv_store` row is committed state in the bound or observed tier, and the legacy `g8e:cache:*` rows are removed when the Gateway opens an existing database. |
| INV-CACHE-03 | A hot read that profiling justifies is cached in process at its Gateway owner, as cache-aside on read and write-through on write. No write invalidates a whole collection or flushes all query results. |
| INV-CACHE-04 | The DB and Blob clients perform startup health checks via HTTP GET to the health endpoint, but g8ee does not fail startup solely because one check returns false. Document and blob failures propagate as errors, and failed document writes raise `DatabaseError`. |

### Attachment handling (`INV-ATTACHMENTS`)

| ID | Rule |
| --- | --- |
| INV-ATTACHMENTS-01 | g8ee receives attachment metadata containing blob references in the form `att:{investigation_id}/{attachment_id}`. The attachment flow retrieves referenced objects only; it does not call the blob client's write or delete methods. Retrieved objects are parsed as serialized `AttachmentData` records containing content metadata and a base64 payload, then classified as text, image, PDF, or another type for the selected model provider. |
| INV-ATTACHMENTS-02 | Text content smaller than 5 MiB is decoded as UTF-8 with replacement for invalid sequences. The Gateway limits one blob request body to 50 MiB. Blob reads return only active, unexpired objects. The attachment flow does not assign TTLs and has no cleanup task; attachment expiration or deletion depends on the writer and Gateway blob lifecycle. |
| INV-ATTACHMENTS-03 | Direct blob mutation is allowed only in the `temp`, `uploads`, `cache`, and `scratch` namespaces, plus caller-owned application or user namespaces under identity rules. The `att:` namespace used by g8ee references is not on the direct-mutation allowlist. The attachment service retrieves referenced objects only and does not upload or delete attachments. |

### State roots and governance (`INV-GOVERNANCE`)

| ID | Rule |
| --- | --- |
| INV-GOVERNANCE-01 | All document content contributes to the Gateway's bound state root. Bound key-value entries contribute; bound blobs contribute with metadata and content. Expired values and blobs are excluded; observed-state entries use a separate commitment. |
| INV-GOVERNANCE-02 | A protected application-record mutation follows the platform five-layer interlock: L1 Doctrine (typed payload validation, hard gates, threat detection), L2 Consensus (Ed25519 vote verification), L3 Notary (WebAuthn or signed CLI proof), L4 Warden (signature, expiry, nonce, transaction hash, state root, identity, and L2/L3 evidence verification), and L5 Actuator (transaction dispatch with capability token and signed receipt). |
| INV-GOVERNANCE-03 | g8ee serializes direct-envelope submissions with a submission lock and retries a state-root mismatch with a fresh root up to three times. The client submits over the Gateway HTTPS endpoint using the enrolled g8ee app mTLS certificate. Delegated Operator and session identity fields are carried in the envelope. The route verifies supplied evidence but does not create missing L2 votes or suspend for L3; a mutation fails when the active posture requires evidence that g8ee did not supply. |

### Data protection and persistence (`INV-PROTECTION`)

| ID | Rule |
| --- | --- |
| INV-PROTECTION-01 | Document, key-value, blob, health, and related Gateway requests use g8ee's enrolled app workload certificate over mTLS. The GovernanceClient uses the same configured TLS identity for direct envelope submission and supplies the configured Operator session context when building the envelope. The app certificate does not grant unrestricted host access; host operations use the separate command-relay path to the target Operator. |
| INV-PROTECTION-02 | Gateway application documents, key-value data, blobs, and structured metadata are not protected by the Operator vault's field-level encryption. At-rest protection depends on the Gateway runtime and database access controls. Operator-local stores apply their own selective encryption and retention rules as described in [Storage Architecture](../architecture/storage.md). |
| INV-PROTECTION-03 | Returned host output and user-provided attachments can enter conversation records or model-provider context. The fact that execution evidence is authoritative on the Operator does not mean every copy of returned content remains on that host. Provider selection, application retention, and attachment handling must account for that data flow. |
| INV-PROTECTION-04 | Gateway maintenance removes expired key-value entries and blobs. g8ee does not apply one retention policy to durable documents and does not schedule document or attachment deletion. Cases, investigations, conversation history, memories, activity records, and reputation records remain until an application workflow deletes or replaces them. |
| INV-PROTECTION-05 | Durable Gateway records and g8ee identity files survive a g8ee restart when their runtime volume is retained. In-flight model work, pending application approvals, result correlations, and tracked background tasks do not survive. Shutdown waits up to five seconds for tracked chat tasks before closing g8ee's Gateway transports. |

## Owned surfaces

| Claim | Path | Verify |
| --- | --- | --- |
| Document collections | ensemble/app/services/data/ | Define application-record types and serialize/deserialize operations |
| Protected collection policy | internal/constants/collections.go | Collection allowlist enforcement |
| Document access | ensemble/app/db/document_service.py | Uncached reads, raise-on-failure writes |
| Gateway client initialization | ensemble/app/main.py, ensemble/app/clients/ | Health checks and connection lifecycle |
| Attachment reference and parsing | ensemble/app/models/attachments.py, ensemble/app/services/data/attachment_store_service.py | `att:` format and `AttachmentData` deserialization |
| Blob mutation allowlist | internal/services/gateway/data_controller.go | Direct-mutation namespace enforcement |
| Governance envelope submission | ensemble/app/services/governance/, internal/services/gateway/ | Five-layer verification and state-root retry logic |
| mTLS identity and transport | ensemble/app/clients/, internal/services/gateway/platform_enrollment_service.go | Workload certificate configuration and usage |

## Procedures

### Verify collection access policy

Identify whether a collection requires GovernanceEnvelope submission:

1. Locate the collection name in `internal/constants/collections.go`.
2. Check if it appears in the protected collection set (cases, investigations, tasks, memories, agent_activity_metadata, reputation_state, reputation_commitments, stake_resolutions).
3. If protected, all writes must submit to `/api/v1/governance/envelopes` with a `GovernanceEnvelope` payload.
4. If not protected, direct document mutations are allowed via the document service.

### Trace attachment retrieval flow

To understand how an attachment is retrieved and processed:

1. Receive attachment metadata with blob reference: `att:{investigation_id}/{attachment_id}`.
2. Call the blob client's `get()` method with the reference.
3. The Gateway returns only active, unexpired blob content.
4. Parse the returned bytes as a serialized `AttachmentData` record.
5. Deserialize the base64 payload and classify the content type.
6. For text content under 5 MiB, decode as UTF-8 with replacement sequences.
7. Pass to the selected model provider or store in application records.

### Submit a protected document mutation

To submit a mutation to a protected collection:

1. Construct a typed document operation (create, update, or delete) for the target collection.
2. Wrap the operation in a `GovernanceEnvelope` with delegated Operator and session identity context.
3. Call the GovernanceClient's envelope submission method.
4. The client serializes the submission with a submission lock.
5. Submit via HTTPS to the Gateway using the g8ee app mTLS certificate.
6. If the Gateway returns a state-root mismatch, retry up to three times with a fresh root.
7. On success, the L4 Warden and L5 Actuator verify and execute the transaction.

## Anti-patterns

- Bypassing GovernanceEnvelope for mutations to protected collections (INV-COLLECTIONS-02). The Gateway rejects direct mutations and no document store method accepts them.
- Adding a g8ee-side document or query cache (INV-CACHE-01). A copy outside the Gateway can only be stale; cache hot reads in process at their Gateway owner (INV-CACHE-03).
- Uploading or deleting attachments via the blob client (INV-ATTACHMENTS-03). The attachment service retrieves objects only; direct blob mutations use separate namespaces.
- Assuming startup failure if a Gateway health check fails (INV-CACHE-04). g8ee continues startup; later storage operations fail with explicit errors.
- Assuming in-flight model work survives a restart (INV-PROTECTION-05). Active model turns and pending approvals are memory-only and are lost on shutdown.

## Links out

- [Ensemble Architecture](architecture.md): Trust boundaries, startup identity, application records, events, and restart behavior.
- [Ensemble Governance](governance.md): Mutation paths, envelope validation, and posture behavior.
- [PKI and Trust](pki.md): g8ee app enrollment and transport credentials.
- [LLM Providers](llm-providers.md): Provider configuration and model-facing data flow.
- [Storage Architecture](../architecture/storage.md): Gateway and Operator persistence, encryption scope, retention, and state-root semantics.
- [Platform Governance](../architecture/governance.md): Five-layer verification, postures, and receipts.
