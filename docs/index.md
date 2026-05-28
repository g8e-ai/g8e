# g8e: Data-Sovereign Runtime Governance for Autonomous Execution

**A self-hosted, air-gap capable substrate that enforces fail-closed authority over AI tool calls through sequential technical, consensus, and human verification layers.**

[Getting Started](guides/getting_started.md){ .md-button .md-button--primary } [Position Paper](core/position_paper.md){ .md-button }

## Core Architecture

```mermaid
sequenceDiagram
    autonumber
    participant Principal as Principal<br/>(Human / AI Agent)
    participant Ensemble as Producer<br/>(g8e-compatible agentic ensemble / BYO / MCP client)
    participant Gateway as Governance Gateway<br/>(g8eg)
    participant Operator as Governed Operator<br/>(g8eo)

    Principal->>Ensemble: Submit intent (MCP / A2A / tool call)
    Note over Ensemble: Reach Consensus (L2)<br/>Wrap in signed GovernanceEnvelope
    Ensemble->>Gateway: Submit envelope for admission

    Operator->>Gateway: Open outbound-only mTLS tunnel
    Operator->>Gateway: Fetch pending GovernanceEnvelope

    Note over Operator: Run verification layers — Doctrine, Consensus, Notary, Warden<br/>(fail-closed)<br/>Execute via Actuator<br/>Anchor to local audit vault

    Operator->>Gateway: Push Sovereignty-scrubbed signed receipt
    Gateway->>Principal: Return final safe output
```

## 5-Layer Governance Verification

Every mutation passes a fail-closed verification pipeline at the host boundary.

=== "L1: Technical Bedrock (L1Doctrine)"

Static analysis and forbidden pattern enforcement. The action violates no hard technical policy before it reaches consensus.

=== "L2: Consensus (L2Consensus)"

Multi-model Byzantine Fault Tolerant Tribunal. Independent, heterogeneous models co-sign every mutation with Ed25519 cryptographic signatures.

=== "L3: Notary (L3Notary)"

WebAuthn/FIDO2 hardware-bound proof of human presence. A human authorizes the exact transaction using its hash as the challenge.

=== "L4: Warden (L4Warden)"

Pre-dispatch verification gate. Enforces transaction integrity, freshness (expiry/nonce), and state-root matching.

=== "L5: Actuator (L5Actuator)"

Sovereign execution boundary. The single fail-closed dispatch path that issues signed Action Receipts.

## Quick Links

<div class="grid cards" markdown>

-   **Getting Started**

    [Get started with g8e](guides/getting_started.md) in minutes using the unified CLI.

-   **Architecture**

    [Architecture and protocol overview](core/about.md) for the governance substrate.

-   **Guides**

    [Operational guides](guides/getting_started.md) for building, connecting, and deploying components.

-   **Reference**

    [API documentation](reference/api/index.md), constants, and protocol references.

</div>
