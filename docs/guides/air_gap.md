---
title: Air Gap
parent: Architecture
---

# Air-Gap Architecture

Last Updated: 2026-05-29
Version: v1.1.0

The g8e platform operates in environments without internet connectivity. The platform supports air-gapped deployments with zero runtime external network dependencies, using the Governance Gateway (`g8eg`), the g8e Operator (`g8eo`), local Go dependencies, and local model inference.

---

## Privacy and Data Sovereignty

In an air-gapped configuration, the platform restricts all outbound communication and retains all data locally:

- **No Telemetry**: The platform disables all outbound telemetry, usage statistics, and error reporting.
- **Local Assets**: All user interface assets, fonts, icons, and libraries are served locally by platform services.
- **Local Persistence**: All platform state, including session records, configuration settings, and cryptographic keys, resides in a unified local SQLite database managed by the Governance Gateway (`g8eg`). The database path is defined in `@/home/bob/g8e/internal/constants/paths.go` as `.g8e/data/g8e.db`.

---

## Governance Gateway (g8eg) Role

In an air-gapped deployment, the Governance Gateway (`g8eg`) operates as the central Policy Decision Point (PDP). Running the `g8e` binary in gateway mode activates persistence and messaging services on the local host.

### Port Configuration and Communication Surfaces

The gateway exposes three logical communication surfaces. Canonical ports are defined in `@/home/bob/g8e/internal/constants/ports.go` and `@/home/bob/g8e/protocol/constants/ports.json`.

| Surface | Port (default) | Authentication | Purpose |
| :--- | :--- | :--- | :--- |
| **Bootstrap** | `8441` (Plain HTTP) | None | Serves local trust bundles and handles Certificate Signing Request (CSR) enrollment. |
| **Public Surface** | `8443` (TLS) | `web_session_id` | Browser management interface, WebAuthn passkey registration, and PKI discovery. |
| **mTLS API & Pub/Sub** | `8440` (mTLS) | mTLS + URI SAN | Receives `GovernanceEnvelope` mutation payloads, handles `/db` persistence, and runs `/ws/pubsub` streaming. |

Surfaces with conflicting TLS client-authentication requirements do not share a network port. Sharing ports forces the use of `tls.VerifyClientCertIfGiven`, which degrades the mTLS execution boundary. The initialization sequence validates port isolation and fails if configurations overlap.

### Core Functional Capabilities

- **State Persistence**: All system state is stored locally within a single SQLite database file as defined in `@/home/bob/g8e/internal/constants/paths.go`.
- **Local Public Key Infrastructure (PKI)**: The gateway generates a local Certificate Authority (CA) using ECDSA P-384 keys to issue and rotate TLS certificates for local services.
- **Secret Storage**: An internal encrypted vault stores local credentials and access tokens, removing any requirement for external key managers.
- **Event Brokerage**: A local websocket pub/sub server manages communication between the gateway and connected clients.

---

## Policy Execution Point: g8e Operator (g8eo)

The g8e Operator (`g8eo`) operates as the host-side Policy Execution Point (PEP). In an air-gapped deployment, the operator runs as a daemon on the target host and initiates a local mTLS connection to the Governance Gateway (`g8eg`).

Every transaction or mutation payload wrapped in a `GovernanceEnvelope` undergoes sequential verification across the five-layer interlock sequence before execution on the host:

1. **L1 Doctrine**: Technical Bedrock (Hard Gates) performs threat analysis, command blacklist checks, and pattern matching, defined in `@/home/bob/g8e/internal/services/governance/l1_doctrine.go`.
2. **L2 Consensus**: Multi-agent consensus signature verification validates the cryptographic signatures on the transaction using Ed25519, defined in `@/home/bob/g8e/internal/services/governance/l2_consensus.go`.
3. **L3 Notary**: Human-in-the-loop authorization verifies approvals via WebAuthn passkeys or cryptographically signed CLI proofs, defined in `@/home/bob/g8e/internal/services/governance/l3_notary.go`.
4. **L4 Warden**: Pre-dispatch verification gates validate replay prevention, expiration, transaction nonces, and the state Merkle root, defined in `@/home/bob/g8e/internal/services/governance/l4_warden.go`.
5. **L5 Actuator**: Isolated boundary tool dispatch executes the validated operation via Model Context Protocol (MCP) or Agent2Agent (A2A), producing a cryptographically signed transaction receipt, defined in `@/home/bob/g8e/internal/services/governance/l5_actuator.go`.

Verified operations are logged to a host-local ledger, and the operator exposes local tools as a standalone Model Context Protocol (MCP) server.

---

## Local Model Inference

For environments without external API access, the platform integrates with local inference engines (such as `llama.cpp`) hosted on the same loopback interface or a local network segment.

- **Downstream Integration**: BYO clients connect to the inference engine using an OpenAI-compatible API.
- **Reference Model**: The platform defaults to `Gemma 4 E2B` for local transaction and payload generation.
- **Configuration Endpoint**: Connection strings point to the local server via the `llamacpp_endpoint` configuration setting, which defaults to `http://localhost:11444`.
- **Model Provisioning**: GGUF format model files are pre-staged directly on the local inference server. The platform does not download or cache model binaries at runtime.

---

## Build-Time versus Runtime Requirements

| Development Phase | Network Requirements | Air-Gap Isolation Strategy |
| :--- | :--- | :--- |
| **Build Phase** | External network access required. | Compile and resolve dependencies on a connected build host prior to deployment. |
| **Runtime Phase** | Zero external network access required. | All communications occur over localhost interfaces or private local networks. |

### Dependency Resolution and Build Tools

To ensure a self-contained installation, the build process packages all required components offline:

- **Go Dependencies**: The core platform compiles into a single binary, resolving dependencies defined in `@/home/bob/g8e/go.mod`.
- **Protocol Generation**: Protobuf compilation is performed offline using local tools without relying on the remote Buf Schema Registry (BSR). Configuration details are defined in `@/home/bob/g8e/buf.gen.yaml` and `@/home/bob/g8e/Makefile`.
- **Build-Time Tooling**: Protobuf stub generation requires `buf`, `protoc-gen-go`, and `protoc-gen-go-grpc` during the build phase. These binaries are not required on the target runtime host.

---

## Deployment and Setup Workflow

Implementing an air-gapped deployment requires a connected staging host to resolve dependencies and compile the binaries before transfer to the target host.

### 1. Preparation on a Connected Host

1. **Compile Binaries**: Execute compilation targets for the target architecture on the staging machine:
   ```bash
   make build
   ```
   Or for compressed release bundles:
   ```bash
   make build-compressed
   ```
2. **Package Runtime Configurations**: Archive the build artifacts and the protocol schemas:
   - The compiled `bin/g8e` binary.
   - The protocol configuration files under the `@/home/bob/g8e/protocol/` directory.

### 2. Implementation on the Air-Gapped Target Host

1. **Stage Binaries and Schemas**: Copy the compiled `g8e` binary and the schema directories to the target directory. Ensure the `g8e` binary is executable.
2. **Stage Downstream Servers**: If integrating with downstream servers, deploy the target MCP or A2A services on the local loopback or isolated network.
3. **Configure Downstream Endpoints**: Export environment variables or configure options to target downstream services:
   - Set `G8E_MCP_DOWNSTREAM_URL` to point to your local MCP server.
   - Set `G8E_A2A_DOWNSTREAM_URL` to point to your local Agent2Agent server.
4. **Initialize the Gateway**:
   ```bash
   ./g8e platform start
   ```
5. **Establish Local Session**: Log in to establish local credentials:
   ```bash
   ./g8e auth login
   ```

---

## Security Invariants

1. **Isolated Boundaries**: In gateway mode, the Governance Gateway (`g8eg`) does not initiate outbound connections to any external network addresses.
2. **Mutual Cryptographic Trust**: All traffic between the Governance Gateway (`g8eg`), connected clients, and the g8e Operator (`g8eo`) is encrypted and authenticated using mutual TLS (mTLS) issued by the local Certificate Authority.
3. **Local Sovereignty**: All audit logs, transactions, and state records remain strictly on the host filesystem inside the local `.g8e` directory, as defined in `@/home/bob/g8e/internal/constants/paths.go`.
4. **Fail-Closed Design**: If any component requires a missing or unavailable external resource, it terminates immediately with a clear error instead of attempting unencrypted or insecure fallbacks.

