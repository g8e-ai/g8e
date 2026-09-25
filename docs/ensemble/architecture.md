# Architecture

## Scope

g8ee is the optional first-party FastAPI application for conversational interaction with g8e. It owns request authentication and context validation, triage, model selection, streaming tool loops, command generation, cases, investigations, memories, model telemetry, Operator workflow services, and user-facing progress events. The Gateway and standalone Operator do not require g8ee; other clients can use g8e through MCP, A2A, or compatible native integrations.

g8ee remains outside the trusted execution boundary. Model output, Tribunal agreement, application memory, and application-level approval express application intent or telemetry; they do not authorize a host or platform mutation. The Gateway and the executing Operator apply the active governance posture before an operation executes.

## Platform Relationships

| Relationship | Purpose | Trust boundary |
| --- | --- | --- |
| Client to g8ee | Starts or resumes an investigation, sends chat turns and supported attachment references, answers triage questions, and responds to application approval prompts. | Browser requests use authenticated proxy context. CLI requests use a bearer Operator session together with the CLI session and user context. `AuthService` validates the resulting identity and context; g8ee does not trust arbitrary identity headers by themselves. |
| g8ee to Gateway data services | Reads settings and application records, stores attachments, and uses cache-aside state. | g8ee uses its enrolled app workload certificate over mTLS. Gateway-backed document, KV, and blob services own the durable application data. |
| g8ee to Gateway event bridge | Publishes typed progress, question, approval, result, reputation, and background events for browser and CLI sessions. | `EventService` sends events to the Gateway SSE push API with the session routing fields. Events without a web or CLI target are skipped. Events are delivery telemetry, not governance state or durable execution evidence. |
| g8ee to target Operator | Sends typed host-operation intent and receives correlated results through Gateway pub/sub. | g8ee publishes a protojson `CommandIntent` to the exact `cmd:<operator_id>:<operator_session_id>` channel using its app workload connection. The Gateway authorizes the publisher and target session, constructs the governed envelope, and relays it to the matching Operator. |
| g8ee to Gateway governance | Mutates designated application records such as cases, investigations, conversation history, memories, activity telemetry, reputation records, and stake resolutions. | `GovernanceClient` submits canonical envelopes over the Gateway HTTPS endpoint using the enrolled g8ee app certificate. The envelope carries the delegated Operator/session and application identity fields; the Gateway binds them to the transport identity and applies the governance checks. |

The ensemble does not use the Gateway MCP or A2A surface for its internal host-operation or application-record paths. MCP and A2A remain separate client integration surfaces described in [AI Agents and the g8e Governance Boundary](../architecture/agents.md).

## Identity and Startup

g8ee starts after the Gateway has been bootstrapped and the `bootstrapped` deployment profile is enabled. The FastAPI lifespan loads local settings, then loads a valid enrolled app identity or begins owner-approved platform enrollment for component `g8ee` and kind `ensemble`. Enrollment uses the Gateway's plain-HTTP discovery surface. It generates a P-256 key and CSR, persists resumable pending state with restrictive permissions, polls approval with bounded retries, signs the completion transcript, validates the issued chain, SANs, public key, and component kind, and atomically installs the certificate, key, and trust bundle. Existing credentials are renewed when they are within one day of expiry. The process does not become ready while enrollment is pending or invalid.

The enrolled app certificate is g8ee's normal Gateway identity for the DB, KV, pub/sub, blob, event, and HTTP clients. The configured Gateway and Operator URLs identify the same Gateway-facing HTTPS and WebSocket surfaces from the deployment's network perspective. An Operator session is separate delegated authority: g8ee discovers session data through its application workflow and validates caller-supplied sessions with the Gateway. The app certificate is not an Operator certificate and does not grant unrestricted host access.

After identity resolution, the lifespan creates the mTLS transport configuration and connects the DB, KV, pub/sub, and blob clients. It loads platform settings through the cache-aside service, overlays them on local bootstrap settings, constructs `GovernanceClient` and all domain, Operator, chat, and provider services, then starts certificate initialization, command pub/sub listeners, and the HTTP client service. Operator heartbeat persistence and freshness evaluation are Gateway-owned; g8ee does not subscribe to heartbeat channels or run stale-heartbeat monitors. FastAPI yields readiness only after these startup hooks complete. Shutdown waits up to five seconds for tracked chat tasks before stopping background services and transports.

In the root Compose deployment, the `ensemble` service is built from `ensemble/Dockerfile` in the `bootstrapped` profile, exposes container port 8000, and publishes it as `${G8E_ENSEMBLE_PORT:-8000}`. The `g8e-ensemble-data` volume is mounted at `/root/.g8e`; `g8e-operator-data` is mounted read-only at `/operator-state` for bootstrap secrets and manifests; and `g8e-shared-tmp` is mounted at `/tmp`. The read-only mount does not expose the Docker host or provide the Operator certificate. Host operations still reach a separately enrolled Operator through the Gateway command channel.

See [Authentication and Authorization](../architecture/auth.md) and [Unified Docker Stack](../guides/unified_stack.md) for platform enrollment and deployment procedures.

## Request Identity and HTTP Surface

The application registers health, chat, and internal routers in `ensemble/app/main.py`. The canonical full application paths are generated from `ensemble/app/constants/api_paths.json`; the routers serve the `/api/v1` prefix through `InternalAPIPaths`. The health router exposes `/health`, `/health/live`, and `/health/details`. These health probes do not require application authentication; `/health` is the Compose healthcheck, while `/health/details` reports whether key services are initialized.

The chat router exposes chat triage actions and chat-session queries. The internal router exposes chat start/stop, cases, investigations, settings, evaluation traces, application approvals, Operator lifecycle and session workflows, and credential operations. Most application routes depend on `require_authenticated_context`. `AuthService` authenticates either a bearer Operator session after Gateway validation or a trusted proxy context containing the required user identity fields, then g8ee checks request context ownership and session bindings. Operator-authentication relay routes are explicit workflow exceptions; they do not make arbitrary unauthenticated internal routes valid.

## Conversation and Tool Flow

A conversation turn authenticates the caller and validates its context before g8ee loads user settings, investigation history, memories, and referenced attachments. Triage routes simple turns to Dash and complex turns or triage failures to Sage. Attachments select the complex path. The selected provider streams a response through the sequential ReAct loop, and typed tool results return to the model for subsequent turns. Provider adapters include OpenAI, Anthropic, Gemini, Ollama, llama.cpp, a fake provider for tests, and the governed `g8e` inference provider when configured.

Host-command requests pass through Tribunal command generation, candidate clustering, optional Auditor review, Marshal risk analysis, deterministic command validation, and the g8ee application approval service. The default Tribunal uses five independent persona passes. Auto-approved commands skip only the g8ee approval prompt after passing the hard command gates; auto-approval is not protocol L3. Tribunal agreement is application reasoning and does not create protocol L2 signatures.

The command service creates a typed internal `G8eMessage`. At the pub/sub boundary, `PubSubClient.publish_command` serializes the typed payload to protobuf bytes and emits canonical protojson `CommandIntent`, including the exact Operator and session, action type, request context, and routing identifiers. g8ee does not construct the governed envelope for this path and does not fetch the Gateway state root.

The Gateway command relay rejects malformed intents, channel and target mismatches, invalid Operator sessions, and disallowed witness dispatches. It applies L1 screening, obtains the current state root, adds identity, replay, expiry, and posture data, constructs the canonical `GovernanceEnvelope`, and forwards it only to the matching Operator session. The Operator independently performs its verification and execution stages. Results return on the matching result channel and g8ee publishes the application result event.

The agent stops for an application continuation decision after the configured `AGENT_MAX_TOOL_TURNS` limit, currently 25. Approval resets the loop counter; denial, timeout, or an unsuccessful approval request stops the loop. Triage clarification similarly pauses the application workflow until the caller answers, skips, or times out.

See [Agents](agents.md) for persona responsibilities and Tribunal stages.

## Governed Application-Record Path

Protected application data is read through the Gateway document, KV, and blob clients. The owning g8ee data services submit protected writes through `GovernanceClient` at `POST /api/v1/governance/envelopes`. The client maps internal payload names to canonical protocol types, serializes typed payload bytes, binds requestor, acting app, Operator/session, case and investigation identifiers, nonce, expiry, and state root into the transaction hash, and submits canonical protojson. It serializes submissions with a lock and retries `TX_STATE_MISMATCH` by fetching a fresh state root and rebuilding the envelope, up to three retries.

The Gateway verifies the supplied envelope, binds app transport identity to the envelope's acting app and delegated session fields, injects its active posture when the client omits one, and executes the operation through the governance pipeline. The g8ee client may include an mTLS certificate fingerprint in L3 metadata, but that fingerprint is not a WebAuthn assertion or signed CLI proof. The direct application-record path does not create missing L2 votes or suspend for supported L3 approval. Protected mutations therefore fail closed when the active posture requires protocol evidence that the envelope does not contain. This path is not equivalent to Gateway MCP or A2A paths that can coordinate configured L2 deliberation or supported L3 approval.

## Persistence and Ownership

g8ee has no local durable application database. Gateway-backed document, KV, and blob stores hold cases, investigations, conversation history, memories, settings, Operator workflow records, agent activity, reputation, stake resolutions, and attachments. g8ee owns the service logic and cache coordination, but the Gateway-backed stores own these durable records.

The process owns ephemeral coordination such as active model turns, background task tracking, pending application approvals, and command-result correlations. These do not survive an ensemble restart. Its certificate, key, trust bundle, and resumable enrollment state persist in the ensemble runtime volume. The executing Operator owns authoritative receipts, audit, replay, execution-vault, command output, and file-mutation evidence in its own runtime. g8ee may copy selected results into application records or model context, but it does not replace that evidence.

Conversation content, attachments, prompts, model outputs, and returned host output are application data and may contain sensitive user content. Model telemetry records provider and model identity, timing, usage when available, retry and finish metadata, canonical input/output hashes, and privacy-analysis metadata. It does not attest to provider behavior outside the governed g8e path.

## Five-Layer Boundary and Limits

For the command-relay path, the Gateway constructs the envelope and the target Operator verifies it. For protected application-record writes, g8ee supplies the envelope and the Gateway verifies it. In either case the active posture determines the required gates:

1. **L1 Doctrine** validates typed payloads and hard safety and threat rules.
2. **L2 Consensus** verifies enrolled-member Ed25519 votes when the posture requires consensus. Tribunal model agreement is not L2.
3. **L3 Notary** verifies WebAuthn or signed CLI authorization for mutation actions when the posture requires ratification. g8ee application approval and an mTLS fingerprint are not substitutes.
4. **L4 Warden** checks expiry, nonce replay, transaction hash, state root, target identity, and required evidence before dispatch.
5. **L5 Actuator** invokes the isolated handler with a transaction-bound capability and produces signed execution or rejection evidence.

SSE events, application approvals, memories, reputation, and model telemetry do not authorize execution or change the Gateway state root. g8ee does not open an inbound management path to a target host. The governance boundary covers only operations that traverse g8e; it does not govern native client tools, provider behavior, unrestricted network access, or other side channels.

See [Governance](./governance.md) for canonical posture semantics and [AI Agents and the g8e Governance Boundary](../architecture/agents.md) for the distinctions among MCP, A2A, direct envelopes, and command relay.

## Related Documentation

- [AI Agents and the g8e Governance Boundary](../architecture/agents.md): Client integration paths and the limits of application agents.
- [Gateway Architecture](../architecture/gateway.md): Gateway services, authentication, governance coordination, and local execution.
- [Operator Architecture](../architecture/operator.md): Outbound transport, independent verification, L5 execution, and authoritative audit.
- [Governance](../architecture/governance.md): Five-layer verification and posture behavior.
- [Authentication and Authorization](../architecture/auth.md): Workload enrollment, mTLS identities, and human authorization.
- [SSE Streaming](../architecture/sse.md): Gateway event delivery and session targeting.
- [g8ee Agents](agents.md): Persona roster, Tribunal, Auditor, and Marshal.
- [Ensemble Storage](storage.md): Gateway-backed application storage and restart behavior.
- [Unified Docker Stack](../guides/unified_stack.md): Compose topology, ports, profiles, and approval workflow.
- [Documentation Guide](../devs/docs.md): Documentation audit and ownership rules.
