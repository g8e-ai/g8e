# Architecture

## Scope

g8ee is the optional first-party agentic ensemble for g8e. It owns conversational triage, model selection, streaming tool loops, command generation, cases, investigations, memory, model telemetry, and user-facing progress events. The Gateway does not require g8ee; other clients can use g8e through MCP, A2A, or compatible native integrations.

g8ee remains outside the trusted execution boundary. Model output, Tribunal agreement, application memory, and application-level approval do not authorize a host mutation. The Gateway and the target Operator apply the active governance posture before an operation executes.

## Platform Relationships

| Relationship | Purpose | Trust boundary |
| --- | --- | --- |
| Client to g8ee | Starts or resumes an investigation, sends chat turns and attachments, answers clarification questions, and responds to application approval prompts. | Browser traffic relies on identity context from the dashboard or another trusted proxy. CLI traffic presents an Operator session that g8ee validates with the Gateway. |
| g8ee to Gateway data services | Reads settings and application records, stores attachments, and uses short-lived cached state. | g8ee authenticates with its enrolled app workload certificate. |
| g8ee to Gateway event bridge | Publishes typed progress, question, approval, result, and reputation events for browser and CLI sessions. | The Gateway authenticates g8ee and routes each event to its declared session. Events report application activity and do not authorize execution. |
| g8ee to target Operator | Sends typed host-operation intent and receives correlated results through Gateway pub/sub. | The Gateway validates the publisher and exact Operator session, constructs the governance envelope, and relays it. The target Operator independently verifies and executes accepted operations. |
| g8ee to Gateway governance | Mutates designated application records such as cases, investigations, conversation history, memories, activity telemetry, and reputation records. | Direct envelope submission requires an authorized Operator transport identity and every proof required by the active posture. |

The ensemble does not use the Gateway MCP or A2A surface for its internal host-operation or application-record paths. MCP and A2A remain separate client integration surfaces described in [AI Agents and the g8e Governance Boundary](../architecture/agents.md).

## Identity and Startup

g8ee starts after the Gateway has a first owner and the bootstrapped workload profile is enabled. It enrolls as the reserved `g8ee` app through the owner-approved platform enrollment flow. On first startup, it creates a P-256 key and certificate request, waits for approval of the exact enrollment request, proves possession of the private key, validates the issued identity, and stores the certificate chain and trust bundle in its own runtime volume.

A valid installed app identity is reused until it approaches expiry. An interrupted pending enrollment resumes with the same request and key material. Enrollment completes before g8ee opens authenticated Gateway transports and finishes application startup; denial, expiry, invalid certificate material, or an unavailable Gateway prevents readiness.

The app certificate authenticates settings, document reads, key-value access, blob access, pub/sub, session validation, health, and event traffic. In the unified deployment, the Operator runtime is also mounted read-only so g8ee can use the enrolled Operator certificate for privileged direct-envelope submissions. This credential sharing is a first-party deployment mechanism, not part of the public app enrollment contract, and the app certificate remains unauthorized for the privileged governance route.

## Request Identity

Browser requests arrive through the dashboard or another trusted proxy that supplies the authenticated user and session context. CLI requests present an Operator session together with the CLI session and user identity. g8ee asks the Gateway to validate that exact tuple and accepts it only when the response is valid and matches the requested user.

Request context carries the current case, investigation, session, and bound Operator set. g8ee checks that context against the authenticated caller before using it. Local Operator and session records support application workflows, but they do not replace Gateway-authoritative authentication.

## Conversation and Tool Flow

A conversation turn follows this flow:

1. g8ee authenticates the caller, validates the request context, loads user settings, and reads the relevant case, investigation history, attachments, Operator state, and memories.
2. Triage classifies the turn. Simple turns use the assistant-tier Dash role, while complex turns and triage failures use the primary-tier Sage role. Attachments also select the complex path.
3. The selected model streams a response and may request tools through a sequential ReAct loop. Each tool result returns to the model as context for the next turn until the model stops requesting tools.
4. A host-command request enters the Tribunal command-generation pipeline. The default configuration runs five independent persona passes, clusters and votes on candidates, and starts an anonymized second round when the first round lacks sufficient agreement. The application Warden assesses risk, the optional Auditor reviews the selected command, and deterministic command constraints run before dispatch.
5. Commands and state-changing Operator tools pass the g8ee approval workflow. Configured auto-approved commands skip the application prompt only after the command passes the deterministic safety gates.
6. g8ee sends typed intent to each exact target Operator session. Execution results return on the corresponding result channel, become client events, and return to the model when the tool loop continues.
7. g8ee persists the conversation and associated application records. Memory generation runs after the response path, while model calls contribute usage, timing, retry, finish, artifact-hash, and privacy metadata.

Clarification questions pause tool execution until the caller answers, skips, or times out. Reaching the configured tool-turn limit similarly pauses the loop for an explicit continuation decision. Approval resets the tool-turn counter, while denial stops the loop.

See [Agents](agents.md) for the persona roster, Tribunal stages, and application Warden behavior.

## Governance Paths

g8ee uses two distinct mutation paths. They share the platform governance model but differ in who constructs the canonical envelope and where the operation executes.

### Host operations

For shell commands, file operations, filesystem inspection, history access, port checks, and other Operator tools, g8ee publishes a typed `CommandIntent` for an exact Operator and session. The Gateway rejects an unauthorized publisher, malformed intent, a channel and target mismatch, or an invalid Operator session. It binds the current state, replay controls, identity, target, and active posture into a `GovernanceEnvelope`, then relays the envelope to that Operator.

The relay does not create missing protocol L2 votes or L3 authorization proofs. Tribunal voting is application reasoning, not Ed25519 protocol consensus, and the g8ee approval prompt is not WebAuthn or a signed CLI proof. A relayed operation therefore fails closed when the active posture requires evidence that the envelope does not contain.

The target Operator performs the authoritative pre-dispatch verification and executes an accepted operation through its Actuator. It returns a typed result correlated to the original execution. g8ee never reads or mutates Operator disk directly.

### Governed application records

For designated application records, g8ee constructs a canonical `GovernanceEnvelope`, binds the current Gateway state root, identity, nonce, expiry, and typed payload into its transaction hash, and submits it through the privileged governance surface. Concurrent state changes can make the bound root stale; g8ee serializes submissions and retries a state-mismatch rejection with a fresh root up to three times.

The unified deployment uses the Operator certificate for this transport because app certificates cannot access the privileged route. The Gateway binds the envelope to the authenticated Operator identity, supplies the active posture when needed, and processes the operation through its local Warden and Actuator. The route verifies supplied evidence but does not create missing L2 votes or human L3 authorization.

The certificate fingerprint carried by g8ee is transport evidence, not a complete L3 proof. Postures requiring CLI authorization also require a signature over the transaction, while browser authorization uses WebAuthn. Direct application-record mutations therefore fail under a posture whose required proof is absent.

## Five-Layer Interlock

Both mutation paths terminate at the same five-layer governance model. The active posture determines whether L2 and L3 evidence is mandatory:

1. **L1 Doctrine** applies typed payload validation, hard gates, forbidden-pattern rules, and MITRE ATT&CK-oriented threat detection.
2. **L2 Consensus** verifies Ed25519 votes from enrolled consensus members against the configured policy and quorum when the posture requires consensus.
3. **L3 Notary** verifies human authorization through WebAuthn or a signed CLI proof for mutations when the posture requires ratification.
4. **L4 Warden** verifies signatures, expiry, nonce replay protection, transaction hash, current state root, target identity, and all posture-required evidence before dispatch.
5. **L5 Actuator** invokes the isolated handler with a transaction-bound just-in-time capability and produces signed execution or rejection receipts.

See [Governance](../architecture/governance.md) for posture behavior and receipt semantics.

## Application State and Events

g8ee owns cases, investigations, conversation history, generated memories, attachments, retry state, Tribunal and reputation records, and model telemetry. Structured records, blobs, and cached values use Gateway-backed data services. Designated record mutations use the direct-envelope path, while reads use the authenticated data transports.

Host execution results can return to g8ee and become model context or conversation data. The Operator retains its authoritative local receipts and audit evidence; g8ee does not replace that record. The Gateway is not implicit agent memory, and g8ee cannot inspect host state outside governed Operator operations.

Progress, clarification, approval, response, result, and reputation updates use the Gateway event bridge for delivery to a web or CLI session. These events are delivery telemetry, not governance state, and do not change the platform state root. Events without a routing session are not delivered.

Durable application records survive a g8ee restart, but active model turns, pending application approvals, and result correlations are process-local. Restarting the service interrupts that in-flight work. Shutdown waits briefly for tracked chat tasks before closing listeners and transports.

## Security Properties and Limits

- g8ee supplies intent but cannot bypass doctrine, posture-required proofs, or Operator verification.
- g8ee has no direct management connection to a target host; host operations execute only through a bound Operator.
- Gateway routing binds host intent to an exact Operator and session rather than broadcasting it.
- Tribunal agreement, Auditor output, application Warden risk, reputation, and auto-approval remain advisory to protocol governance.
- Application approval and auto-approval do not satisfy protocol L3.
- Direct-envelope and command-relay paths fail when the active posture requires proofs they do not supply.
- Model-provider calls occur outside the g8e execution boundary. Governance receipts do not attest to provider behavior or to activity through side channels outside g8e transports.
- Returned host results and user content can contain sensitive data and become part of model context or application records. Operators must configure providers and retention accordingly.

## Related

- [Platform Ensemble Architecture](../architecture/ensemble.md): Platform relationships, governance paths, deployment, and security limits.
- [AI Agents and the g8e Governance Boundary](../architecture/agents.md): MCP, A2A, command relay, direct envelope, and agent trust boundaries.
- [Gateway Architecture](../architecture/gateway.md): Gateway services, identity, governance coordination, and local execution.
- [Operator Architecture](../architecture/operator.md): Remote verification, host execution, results, and local audit.
- [Governance](../architecture/governance.md): Five-layer verification, receipts, and posture behavior.
- [Authentication and Authorization](../architecture/auth.md): Workload enrollment, mTLS identities, CLI sessions, and WebAuthn.
- [Server-Sent Events](../architecture/sse.md): Approval and application event delivery.
- [Agents](agents.md): Triage, Dash, Sage, Tribunal, Auditor, Warden, and support agents.
- [LLM Providers](llm-providers.md): Provider configuration and model tiers.
- [Testing](tests.md): Ensemble test tiers and commands.
