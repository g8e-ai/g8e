---
title: AI Agents and the g8e Governance Boundary
parent: Architecture
---

# AI Agents and the g8e Governance Boundary

Last Updated: 2026-09-08
Version: v2.1.7

## Scope

g8e uses the word "agent" for three related but distinct concepts:

- **External AI clients** are coding agents and other applications that call the Gateway through MCP or A2A.
- **The agent launcher** is the `g8e mcp agent run` workflow that configures and starts a supported coding agent with g8e as its MCP server.
- **The g8ee agentic ensemble** is the optional first-party application that performs triage, model reasoning, tool loops, command generation, memory management, and event publication.

All three remain outside the trusted execution boundary. Model reasoning, prompts, internal voting, and application memory do not authorize host mutation. A governed operation becomes executable only after it enters the platform as typed intent or a canonical `GovernanceEnvelope` and passes the verification required by the active posture.

---

## Trust and Execution Boundaries

The Governance Gateway is the Policy Decision Point. It authenticates clients, constructs envelopes for public client protocols, binds current state and posture, coordinates protocol L2 consensus when required, suspends transactions that need human approval, and exposes pub/sub channels for remote Operators.

The Governed Operator is the Policy Execution Point on a managed host. It opens an outbound mTLS connection to the Gateway, subscribes to its session-specific command channel, verifies each received envelope locally, and executes accepted operations through its L5 Actuator. It opens no inbound management port.

The Gateway also contains an in-process Operator substrate. MCP and A2A calls received by the Gateway execute through this local L4/L5 path, which can invoke built-in tools or configured downstream MCP and A2A services. Host commands sent by g8ee as `CommandIntent` use the outbound Operator path instead, so the Operator on the target host performs L4/L5 verification and execution.

This boundary governs only operations that traverse a g8e ingress. It does not sandbox an AI process or automatically govern native tools, network access, or other side channels that remain enabled in the client itself.

---

## Agent Integration Paths

| Path | Input | Governance behavior | Execution location |
| --- | --- | --- | --- |
| **Gateway MCP** | JSON-RPC methods at `/mcp` | Tool calls, resource reads, prompt retrieval, and A2A calls are translated into typed envelopes and processed through L1-L5. Discovery methods do not execute tools. | Gateway in-process Operator, built-in tool, or configured downstream MCP/A2A service |
| **Gateway A2A** | JSON-RPC `a2a/call` at `/api/v1/a2a/call` | The Gateway constructs an `A2A_CALL` envelope, coordinates required proofs, and processes it through L1-L5. | Configured downstream A2A service through the Gateway Actuator path |
| **Operator command relay** | Typed `CommandIntent` on the target Operator command channel | The Gateway validates the target session and adds identity, state, nonce, expiry, hash, and posture. The relay does not perform L2 deliberation or L3 suspension. | Bound outbound Operator |
| **Direct envelope** | Complete canonical `GovernanceEnvelope` | The Gateway verifies the supplied envelope but does not add missing L2 or L3 proofs. This privileged route rejects app certificates. | Gateway in-process Operator |
| **External MCP wrapper** | Stdio requests forwarded to an MCP subprocess or HTTP server | Only `tools/call` arguments receive inline L1 threat screening. No envelope, L2/L3/L4/L5 execution, signed receipt, or Gateway audit is added. | Wrapped external MCP server |

MCP and A2A are the normal client-facing surfaces when the Gateway must construct the envelope, coordinate L2, or manage L3 approval. `CommandIntent` is suitable only when its proof-free relay satisfies the active posture. Direct envelope submission is reserved for clients that already possess an authorized CLI or Operator transport identity and can supply every required proof.

See [Build Apps](../guides/build_apps.md) for choosing among these integration paths.

---

## MCP Agent Launcher

`g8e mcp agent run <agent>` provides a managed local launch flow for Claude, Codex, Devin, Gemini, and Goose. It starts a local Gateway in `doctrine` posture when one is not already running, ensures the human CLI identity and passkey are enrolled, obtains a short-lived delegated app certificate for the selected agent, configures `g8e mcp stdio` as the agent's MCP server, verifies the generated interception configuration by default, and starts the agent.

The delegated certificate binds the app identity and requesting human identity in its SPIFFE URI SANs. The stdio bridge presents that certificate to the Gateway, and the Gateway records both identities in the governed transaction. This creates per-agent attribution without trusting caller-supplied identity headers.

### Tool Interception Limits

The launcher disables or excludes native tools where the supported agent exposes a reliable control:

- Claude and Codex receive a strict MCP configuration and native-tool exclusions.
- Goose starts without profile extensions and loads g8e as the session extension.
- Gemini receives an empty built-in tool allowlist and a g8e MCP server entry.
- Devin receives g8e as its configured MCP server, but the launcher cannot disable Devin's native tools.

For Devin, only operations sent through the g8e MCP server cross the governance boundary. The same limitation applies to any client that retains native tools, another MCP server, direct filesystem access, shell access, or unrestricted network access.

### Wrapping an MCP Server

When `g8e mcp agent run` receives `--url` or an arbitrary command instead of a supported agent name, it runs the external MCP wrapper. This mode screens `tools/call` arguments with L1 doctrine and forwards accepted requests directly to the downstream server. It is not equivalent to the named-agent launch path and does not provide L2-L5 governance.

---

## MCP Stdio Bridge and Credentials

`g8e mcp stdio` is a credential-consuming stdio-to-HTTPS bridge. It answers the MCP initialization handshake locally, drops notifications that require no response, and proxies other requests to the Gateway over TLS 1.3. It does not enroll a user, install trust, open a passkey enrollment ceremony, or start the Gateway when invoked directly.

Certificate and key pairs resolve in this order:

1. Delegated app certificate and key flags.
2. Delegated app certificate and key environment variables.
3. CLI certificate and key flags.
4. CLI certificate and key environment variables.
5. Enrolled CLI credentials in the local runtime tree.

Each tier must provide a complete certificate and key pair. An incomplete pair fails closed rather than mixing credentials across tiers. The CA bundle resolves from its flag, then its environment variable, then the enrolled trust bundle; the Gateway URL resolves from its flag, then its environment variable, then the default HTTPS MCP URL.

When L3 approval is required, the stdio bridge opens the approval page, waits for the matching `approval.completed` event over the authenticated SSE stream, and retries the original request. There is no polling fallback, so this automatic flow requires enrolled CLI credentials and a CLI session even when the MCP request itself uses delegated app credentials.

---

## The Five-Layer Interlock

Every governed operation reaches the same L4/L5 verification and execution boundary. The active posture determines whether L2 and L3 are required gates or non-gating evidence.

### L1 Doctrine

L1 decodes the typed payload and applies protobuf field constraints, forbidden-pattern rules, and MITRE ATT&CK-oriented threat detection. L1 is mandatory in every posture. The executing Warden performs this validation before dispatch.

### L2 Consensus

L2 verifies Ed25519 votes over the transaction hash against an enabled consensus policy and its trusted member keys. Required postures enforce the configured quorum of distinct affirmative signers. The Gateway-owned MCP and A2A construction paths request deliberation under `consensus` and `notary`; direct envelopes and the `CommandIntent` relay do not receive missing votes automatically.

The protocol L2 service is separate from application-level multi-model voting. Internal model agreement is advisory unless enrolled consensus members emit valid Ed25519 votes that satisfy the Gateway policy. See [Consensus](./consensus.md).

### L3 Notary

L3 authorizes mutations under `ratify` and `notary`. Gateway MCP and A2A requests without the required proof suspend for a WebAuthn approval ceremony and resume with the resulting proof. Read-only actions do not require L3, and paths that do not implement suspension must arrive with the required proof already attached.

### L4 Warden

L4 reserves the nonce for durable replay prevention, checks expiry, decodes and validates the typed payload, recomputes the transaction hash, verifies the current state root, and evaluates posture-required L2 and L3 evidence. The posture travels in the envelope so a remote Operator applies the Gateway-selected policy. Any universal check or required proof failure prevents dispatch and produces signed rejection evidence when the Actuator is available.

### L5 Actuator

L5 signs and persists an `EXECUTING` receipt before invoking the handler. It appends a signed commitment when the SQL commitment ledger is available, rehydrates scrubbed payload data at the execution site, mints a transaction-bound just-in-time capability, invokes the selected handler, dissolves the capability, and signs and persists the final result with its durable-persistence attestation.

| Posture | L1 | L2 | L3 for mutations |
| --- | --- | --- | --- |
| `doctrine` | Required | Not required | Not required |
| `consensus` | Required | Required | Not required |
| `ratify` | Required | Not required | Required |
| `notary` | Required | Required | Required |

---

## First-Party Agentic Ensemble

g8ee is an optional first-party client, not part of the Gateway or Operator trust boundary. It owns the conversational and model-facing concerns that the platform protocol intentionally leaves to applications:

1. Triage classifies a turn and selects the fast Dash path or the primary Sage path.
2. The selected model streams a response and may request tools through a sequential ReAct loop.
3. Host-command requests pass through the five-member Tribunal, candidate clustering and audit, and application-level Warden risk analysis before dispatch.
4. The tool result returns to the model for another turn until the model stops requesting tools.
5. Reaching the configured tool-turn limit requires an explicit continuation decision before the loop can continue.
6. The ensemble publishes typed progress and result events through the Gateway SSE event bridge.

The Tribunal improves command generation but does not produce protocol L2 signatures. For host operations, g8ee publishes `CommandIntent` to the bound Operator channel. For governed platform records, it constructs direct envelopes and uses the dedicated Operator-credential transport available in the unified deployment. It does not use the Gateway MCP endpoint for these internal paths.

Conversation history, investigation context, generated memories, model telemetry, and retry state remain application-owned. g8e governs host and platform operations; the Gateway is not implicit agent memory. See [g8ee Agents](../ensemble/agents.md) for the persona roster and [Ensemble Architecture](./ensemble.md) for its connection model.

---

## Results, Receipts, and Events

A completed Gateway MCP tool call returns tool content plus an authoritative signed receipt reference. Calls rejected after signed L1 or L2 stage evidence return that reference in structured JSON-RPC error data. Caller-supplied execution and investigation identifiers are preserved when present; the Gateway generates them when absent.

The executing Actuator stores the complete receipt in its local SQL audit store. A remote Operator also publishes signed receipts to the Gateway receipt channel; the Gateway verifies the signer and mirrors accepted receipts for centralized access. That mirror is best-effort and does not replace the Operator's local record.

Governed file mutations record file-mutation evidence and ledger hashes when the file ledger is active. The SQL commitment chain covers admitted executions independently of the file ledger.

Two SSE mechanisms serve different purposes:

- The MCP endpoint supports MCP's streaming transport.
- The platform SSE event bridge carries approval notifications and typed application events, including g8ee chat progress, questions, and results.

SSE events are delivery telemetry, not governance state, and do not alter the state root. See [SSE Streaming](./sse.md).

---

## Security Properties and Limits

- The AI client supplies intent but has no authority to bypass doctrine or required proofs.
- Client identity comes from authenticated transport context, including SPIFFE identities in mTLS certificates, rather than trusted identity headers.
- The transaction hash binds executable intent, target, state, replay controls, and requestor and app attribution.
- A remote Operator independently verifies the envelope before changing its host.
- L2 requires trusted signer keys and policy quorum; model agreement alone has no protocol authority.
- L3 applies to mutation-classified actions under `ratify` and `notary`, not to every read operation.
- Signed receipts prove the governed path's outcome, but they do not attest to activity performed through client-native tools or other bypass channels.
- Operator command channels are scoped to an exact Operator and session. The Gateway rejects mismatched target identity and does not broadcast commands.

---

## Related Documentation

- [Gateway Architecture](./gateway.md): Gateway services, client ingress, governance coordination, and local execution.
- [Operator Architecture](./operator.md): Outbound Operator transport, native tools, L4/L5 execution, and local audit.
- [Governance](./governance.md): Five-layer verification and posture behavior.
- [Consensus](./consensus.md): L2 policies, enrolled members, and vote verification.
- [Authentication and Authorization](./auth.md): mTLS identities, delegated app credentials, CLI sessions, and WebAuthn.
- [Network Architecture](./network.md): TLS surfaces, pub/sub transport, and Operator channels.
- [SSE Streaming](./sse.md): Approval and application event delivery.
- [Ensemble Architecture](./ensemble.md): The first-party g8ee deployment and connection model.
- [g8ee Agents](../ensemble/agents.md): Persona hierarchy, Tribunal, Warden, and support agents.
- [Build Apps](../guides/build_apps.md): Public integration paths and application-owned state.
