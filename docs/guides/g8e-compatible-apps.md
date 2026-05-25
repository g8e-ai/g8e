# g8e-Compatible Applications

Architecturally, a g8e-compatible application functions strictly as a `GovernanceEnvelope` producer and a receipt consumer. It maintains no privileged communication channels, never interacts directly with the host system, and communicates with the Operator exclusively through public ingress paths. 

Security operations including Doctrine (L1Doctrine), Consensus (L2Consensus), and Notary (L3Notary) verification gates, replay defense, state binding, cryptographic audit, and human-in-the-loop authorization are fully delegated to the Operator substrate. The application provides only the components the protocol cannot intrinsically supply: the mutation intent and optionally, consensus evidence.

## Application Architecture Spectrum

The architecture of a g8e application varies based on how it satisfies the Consensus (L2Consensus) consensus requirement. All applications produce the canonical `GovernanceEnvelope` wire format.

### Minimal Applications
A minimal application constructs the mutation intent and builds a valid `GovernanceEnvelope`. This requires formatting the typed payload, generating a deterministic transaction hash, and appending a nonce, expiry, and fetched state root. The application submits the envelope and consumes the signed receipt. Minimal applications do not produce L2 consensus evidence natively. They rely on the Operator's protocol-agnostic MCP/A2A translation layer or a trusted upstream producer to fulfill the L2 requirement.

### Maximal Applications
A maximal application performs the identical intent formulation and envelope construction, while additionally producing its own Consensus (L2Consensus) consensus evidence. It executes an internal consensus mechanism and signs the envelope directly. A g8e-compatible agentic ensemble represents the reference implementation of a maximal application, generating the required consensus signatures.

## Structural Invariants

Two invariants apply to all g8e applications:

1. **Identity and Authentication**: Application identity is established via an mTLS/SPIFFE certificate. The application authenticates cryptographically and receives no ambient trust. The Operator evaluates its envelope with identical rigor to any external client.
2. **State Management**: Application-internal state remains the exclusive responsibility of the application. The g8e protocol governs and audits mutations to host reality; it does not manage or persist the application working memory.