# Storage

## Scope

g8ee does not open a local application database. It stores durable application records, cached values, and attachment objects through the Gateway's authenticated document, key-value, and blob services. The Gateway persists those services in its local `g8e.db`; active model turns, pending application approvals, and result correlations remain in g8ee process memory and do not survive a restart.

Host execution evidence has a separate owner and lifecycle. A target Operator retains its authoritative receipts, audit events, execution output, file-mutation evidence, replay state, and optional file ledger on that host. g8ee can receive governed results and persist selected content in application records or send it to a configured model provider, but it does not read the Operator's local stores directly. See [Storage Architecture](../architecture/storage.md) for the platform storage topology and protection applied to each store.

## Storage Topology

| State | Owner | Persistence | g8ee use |
| --- | --- | --- | --- |
| Application documents | Gateway | `g8e.db` document store | Cases, investigations, conversation history, memories, settings, Operator workflow records, agent activity, reputation, and stake resolutions |
| Cached and short-lived values | Gateway | `g8e.db` key-value store | Optional document and query cache plus limited session and request coordination |
| Attachment objects | Gateway | `g8e.db` blob store | Retrieval and preparation of attachment content referenced by a conversation |
| In-flight work | g8ee process | Memory only | Model turns, application approvals, background task tracking, and Operator result correlation |
| Execution evidence | Executing Operator | Operator-local stores | Authoritative audit, receipt, execution-vault, replay, and file-ledger records |

## Application Documents

The document store organizes JSON records by collection and identifier. g8ee uses it for cases and tasks, investigations and their conversation history, generated memories, settings, users and Operator workflow records, certificate-revocation records, agent activity metadata, reputation state and commitments, and stake resolutions. Conversation entries are embedded in investigation documents and linked by entry hashes.

Authenticated reads retrieve individual records or filter a collection with comparison operators, one ordering field, and a result limit. Field projection is applied by the g8ee client after the Gateway returns a query. Document replacement overwrites user data, while merge updates operate on top-level fields and remove a field when its incoming value is null.

Protected application collections include cases, investigations, tasks, memories, agent activity metadata, reputation state, reputation commitments, and stake resolutions. The Gateway rejects direct document mutations for these collections. g8ee submits their writes as typed document operations in a `GovernanceEnvelope`; reads continue to use the document service.

The Gateway permits direct mutations only for a fixed set of platform support collections. The current g8ee API-key service targets an application-specific `api_keys` collection through direct document methods, but that collection is not on the Gateway allowlist, so its create and update requests are rejected rather than persisted.

The client-side list helpers use read-modify-write cycles, and the batch helper sends operations one at a time. These operations are not atomic across concurrent writers or across a batch.

## Key-Value Cache

The key-value service stores string values with optional expiration. g8ee serializes document and query cache entries as JSON and assigns collection-specific TTLs. Cache-management keys use the `g8e:cache:` prefix and do not contribute to the Gateway's bound state root.

Cache reads are disabled by default through `gateway.enable_cache_read`. Document and query reads still warm the cache after a Gateway read, but subsequent reads bypass those entries while the setting remains disabled. Current document mutation paths do not clear warmed document or query entries, so enabling cache reads can return stale data until expiration or explicit invalidation.

Hash, list, counter, and pattern operations are client-side conveniences over string values. Hash, list, and counter updates use read-modify-write sequences rather than server-side atomic operations. Pattern matching uses Gateway glob semantics.

The key-value client treats an unavailable or unhealthy service as a cache miss or unsuccessful cache write for many operations. Document and blob failures instead propagate as storage or network errors. Startup records unsuccessful connectivity checks but does not make all three data-service checks readiness gates.

## Attachments

g8ee receives attachment metadata containing a blob reference in the form `att:{investigation_id}/{attachment_id}`. Its attachment flow only retrieves referenced objects; it does not upload or delete attachment objects. The retrieved object is parsed as a serialized attachment record containing the content metadata and base64 payload, then classified as text, image, PDF, or another type for the selected model provider. Text content smaller than 5 MiB is decoded for text context.

The Gateway limits one blob request body to 50 MiB. Blob reads return only active, unexpired objects. The g8ee blob client does not assign a TTL, and g8ee has no attachment cleanup task, so attachment expiration or deletion depends on the writer and Gateway blob lifecycle.

The Gateway currently allows direct blob mutation only in its temporary, upload, cache, and scratch namespaces, subject to caller ownership. The `att:` namespace used by g8ee references is not on that direct-mutation allowlist. The current g8ee application flow therefore documents attachment retrieval only, not a working g8ee-managed upload or deletion path.

## State Roots and Governance

All document content contributes to the Gateway's bound state root. Active bound key-value entries also contribute unless they are cache-management keys, and active bound blobs contribute with their metadata and content. Expired values and blobs are excluded; observed-state entries use a separate commitment.

A protected application-record mutation follows the platform five-layer interlock:

1. **L1 Doctrine** validates the typed payload and applies hard gates, forbidden-pattern matching, and MITRE ATT&CK-oriented threat detection.
2. **L2 Consensus** verifies Ed25519 votes from enrolled consensus members when the active posture requires consensus.
3. **L3 Notary** verifies WebAuthn or a signed CLI proof for mutations when the active posture requires human authorization.
4. **L4 Warden** verifies signatures, expiry, nonce replay protection, the transaction hash, the current state root, target identity, and required L2 and L3 evidence.
5. **L5 Actuator** dispatches the accepted document operation with a transaction-bound capability and produces signed receipt evidence.

g8ee serializes direct-envelope submissions and retries a state-root mismatch with a fresh root up to three times. The route verifies supplied evidence but does not create missing L2 votes or L3 proofs, so a mutation fails when the active posture requires evidence that g8ee did not supply.

## Authentication and Data Protection

Document, key-value, blob, health, and related Gateway requests use g8ee's enrolled app workload certificate over mTLS. Protected direct-envelope submissions require an authorized Operator transport identity. In the unified deployment, g8ee uses the Operator certificate mounted read-only for that route; falling back to the app certificate causes the Gateway to reject the privileged request.

Gateway application documents, key-value data, blobs, and their structured metadata are not protected by the Operator vault's field-level encryption. Their at-rest protection depends on the Gateway runtime and database access controls. Operator-local stores apply their own selective encryption and retention rules as described in [Storage Architecture](../architecture/storage.md).

Returned host output and user-provided attachments can enter conversation records or model-provider context. The fact that execution evidence is authoritative on the Operator does not mean every copy of returned content remains on that host. Provider selection, application retention, and attachment handling must account for that data flow.

## Retention and Restart Behavior

Gateway maintenance removes expired key-value entries and blobs. g8ee supplies TTLs for cache entries, but it does not apply one retention policy to durable documents and does not schedule document or attachment deletion. Cases, investigations, conversation history, memories, activity records, and reputation records remain until an application workflow deletes or replaces them.

Durable Gateway records survive a g8ee restart. In-flight model work, pending application approvals, result correlations, and tracked background tasks do not. Shutdown waits briefly for tracked chat tasks before closing its Gateway transports.

## Related

- [Ensemble Architecture](architecture.md): Trust boundaries, startup identity, application records, events, and restart behavior
- [Storage Architecture](../architecture/storage.md): Gateway and Operator persistence, encryption scope, retention, and state-root semantics
- [Governance](governance.md): g8ee mutation paths and envelope validation
- [Platform Governance](../architecture/governance.md): Five-layer verification, postures, and receipts
- [PKI and Trust](pki.md): g8ee app enrollment and transport credentials
- [LLM Providers](llm-providers.md): Provider configuration and model-facing data flow
