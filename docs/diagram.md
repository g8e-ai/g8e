
# g8e Architecture & Protocol Visualization

This document provides a visual mapping of the g8e platform, highlighting the protocol substrate as the foundational mandatory layer.

## 1. The g8e Protocol (Foundational Substrate)
The g8e protocol is a typed, signed, state-bound transaction layer. It is the single source of truth for all system mutations and the only mandatory component for interoperability.

```mermaid
graph LR
    Start["Original MCP / A2A / User Message<br/>(Interpreted Intent)"]

    subgraph Verification ["Operator Verification - protocol-mandated"]
        direction LR
        L1{"L1: Technical Bedrock<br/>Forbidden Patterns?"}
        L2{"L2: Consensus<br/>Tribunal Signature?"}
        L3{"L3: Authorization<br/>Human Presence?"}
        State{"State Check<br/>Merkle Root Fresh?"}
        
        FailClosed["Fail Closed<br/>Error + Audit Entry"]
        Warden["Signed Action Receipt<br/>Audit Commitment"]
        LocalVault([Local Vault])

        L1 -- "Passed" --> L2
        L1 -- "Violated" ----> FailClosed
        
        L2 -- "Passed" --> L3
        L2 -- "Invalid/Missing" ---> FailClosed
        
        L3 -- "Authorized" --> State
        L3 -- "Denied" --> FailClosed
        
        State -- "Fresh" --> Warden
        State -- "Stale" --> FailClosed

        Warden --> LocalVault
        FailClosed --> LocalVault
    end

    LocalVault --> Destination["Original MCP / A2A / User Message<br/>(Audited, Signed, Recorded)"]

    Start --> L1
```

## 2. AI Reasoning Engine (Using the Protocol)
The reference AI engine (g8ee) or any BYO agentic system consumes the protocol to articulate intent and produce verifiable transactions.

```mermaid
graph TD
    classDef principal fill:#f9d0c4,stroke:#333,stroke-width:2px,color:#000;
    classDef engine fill:#e1f5fe,stroke:#0288d1,stroke-width:2px,color:#000;
    classDef protocol fill:#fff3e0,stroke:#f57c00,stroke-width:2px,color:#000;

    Principal(("Principal (Human / Agent)")):::principal

    subgraph Engine ["g8ee AI Engine (Application Layer)"]
        direction TB
        Triage["Triage Agent (Intent & Posture)"]:::engine
        Reasoner["Sage / Dash (Reasoning Path)"]:::engine
        
        subgraph Tribunal ["Tribunal (L2 Producer)"]
            direction TB
            Panel["5-Member Agent Panel"]:::engine
            Warden["Warden (Two-Strike Circuit Breaker)"]:::engine
            Auditor["Auditor (L2 Verifier)"]:::engine
            
            Panel --> Warden
            Warden --> Auditor
        end
        
        Triage --> Reasoner
        Reasoner --> Panel
        
        %% Short Circuits (Feedback Loops)
        Warden -. "Risk Feedback (Short Circuit)" .-> Reasoner
        Auditor -. "Rejection / Revision (Short Circuit)" .-> Reasoner
    end

    Principal -- "Initiates Intent" --> Triage
    Auditor -- "Produces L2 Signed Intent" --> Protocol["g8e Protocol Envelope"]:::protocol
```

### 3-Layer Governance Summary
Every mutation must pass all three layers in sequence at the substrate boundary.

| Layer | Name | Mechanism | Responsibility |
|---|---|---|---|
| **L1** | **Technical Bedrock** | Static Analysis / Reflection | Forbidden patterns, regex threat matching, and policy enforcement. |
| **L2** | **Consensus** | Ed25519 Signatures | Cryptographic proof that an independent ensemble (Tribunal) co-validated the intent. |
| **L3** | **Authorization** | WebAuthn / FIDO2 | Hardware-bound proof of human presence for mutations. |

### Implementation Reference
- **Protocol Schemas**: `protocol/proto/*.proto`
- **Governance Logic**: `services/g8eo/internal/services/governance/`
- **Audit Storage**: `services/g8eo/internal/services/storage/audit_vault.go`