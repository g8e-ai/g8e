---
title: Air Gap
parent: Architecture
---

# Air-Gap Architecture

Last Updated: 2026-06-24
Version: v1.2.0

The g8e platform operates in environments without internet connectivity. The platform supports air-gapped deployments with zero runtime external network dependencies, using the g8e Gateway, the g8e Operator, and local Go dependencies. The platform supports both binary deployment and containerized deployment via Docker.

---

## Privacy and Data Sovereignty

In an air-gapped configuration, the platform restricts all outbound communication and retains all data locally:

- **No Telemetry**: The platform disables all outbound telemetry, usage statistics, and error reporting.
- **Local Assets**: All user interface assets, fonts, icons, and libraries are served locally by platform services.
- **Local Persistence**: All platform state, including session records, configuration settings, and cryptographic keys, resides in local SQLite databases managed by the g8e Gateway. Runtime database paths are defined in `internal/paths/paths.go`, including the main database at `.g8e/data/g8e.db`, local state at `.g8e/local_state.db`, and audit vault at `.g8e/data/audit_vault.db`. Database filenames are defined in `internal/constants/paths.go`.

---

## g8e Gateway Role

In an air-gapped deployment, the g8e Gateway operates as the central Policy Decision Point (PDP). Running the g8e Node in gateway mode activates persistence and messaging services on the local host.

### Port Configuration and Communication Surfaces

The gateway exposes two logical communication surfaces. Canonical ports are defined in `internal/constants/ports.go` and `protocol/constants/ports.json`.

| Surface | Port (default) | Authentication | Purpose |
| :--- | :--- | :--- | :--- |
| **HTTP (Bootstrap + MCP)** | `8080` (plain HTTP) | None | Serves local trust bundles, handles Certificate Signing Request (CSR) enrollment, and plain HTTP MCP for development/testing. |
| **HTTPS (mTLS API + Public)** | `8443` (mTLS) | mTLS + URI SAN | Receives `GovernanceEnvelope` mutation payloads, handles `/db` persistence, runs WebSocket pub/sub streaming and SSE event streaming, and provides browser management interface. |

Surfaces with conflicting TLS client-authentication requirements do not share a network port. Sharing ports forces the use of `tls.VerifyClientCertIfGiven`, which degrades the mTLS execution boundary. The initialization sequence validates port isolation and fails if configurations overlap.

### Core Functional Capabilities

- **State Persistence**: All system state is stored locally within SQLite databases. Runtime paths are defined in `internal/paths/paths.go`, including the main database (`.g8e/data/g8e.db`), local state (`.g8e/local_state.db`), and audit vault (`.g8e/data/audit_vault.db`). Database filenames are defined in `internal/constants/paths.go`.
- **Local Public Key Infrastructure (PKI)**: The gateway generates a local Certificate Authority (CA) using ECDSA P-384 keys to issue and rotate TLS certificates for local services.
- **Secret Storage**: An internal encrypted vault stores local credentials and access tokens, removing any requirement for external key managers.
- **Event Brokerage**: A local WebSocket pub/sub broker (`internal/services/gateway/gateway_pubsub.go`) manages real-time communication between the gateway and connected clients. Server-Sent Events (SSE) streaming (`internal/services/gateway/gateway_http_sse.go`) provides event delivery to browser and CLI subscribers.
- **WebAuthn Passkey Bootstrap**: The gateway supports WebAuthn passkey-based authentication for secure local enrollment without external identity providers.

---

## Policy Execution Point: g8e Operator

The g8e Operator operates as the host-side Policy Execution Point (PEP). In an air-gapped deployment, the g8e Operator runs as a daemon on the target host and initiates a local mTLS connection to the g8e Gateway.

Every transaction or mutation payload wrapped in a `GovernanceEnvelope` undergoes sequential verification across the five-layer interlock sequence before execution on the host:

1. **L1 Doctrine**: Technical Bedrock (Hard Gates) performs threat analysis, command blacklist checks, and pattern matching, defined in `internal/services/governance/l1_doctrine.go`.
2. **L2 Consensus**: Multi-agent consensus signature verification validates the cryptographic signatures on the transaction using Ed25519. L2 verification is performed by the L4 Warden via posture-aware checks, with governance posture logic defined in `internal/services/governance/posture.go` and verification implementation in `internal/services/governance/l4_warden.go`.
3. **L3 Notary**: Human-in-the-loop authorization verifies approvals via WebAuthn passkeys (for web sessions) or cryptographically signed CLI proofs (for CLI sessions), defined in `internal/services/governance/l3_notary.go`.
4. **L4 Warden**: Pre-dispatch verification gates validate replay prevention, expiration, transaction nonces, and the state Merkle root, defined in `internal/services/governance/l4_warden.go`.
5. **L5 Actuator**: Isolated boundary tool dispatch executes the validated operation via Model Context Protocol (MCP) or Agent2Agent (A2A), producing a cryptographically signed transaction receipt, defined in `internal/services/governance/l5_actuator.go`.

Verified operations are logged to a host-local ledger, and the Operator exposes local tools as a standalone Model Context Protocol (MCP) server.

---

## Build-Time versus Runtime Requirements

| Development Phase | Network Requirements | Air-Gap Isolation Strategy |
| :--- | :--- | :--- |
| **Build Phase** | External network access required. | Compile and resolve dependencies on a connected build host prior to deployment. |
| **Runtime Phase** | Zero external network access required. | All communications occur over localhost interfaces or private local networks. |

### Dependency Resolution and Build Tools

To ensure a self-contained installation, the build process packages all required components offline:

- **Go Dependencies**: The core platform compiles into a single g8e Node, resolving dependencies defined in `go.mod`.
- **Protocol Generation**: Protobuf compilation is performed offline using local tools without relying on the remote Buf Schema Registry (BSR). Configuration details are defined in `buf.gen.yaml` and `Makefile`.
- **Build-Time Tooling**: Protobuf stub generation requires `buf`, `protoc-gen-go`, and `protoc-gen-go-grpc` during the build phase. These binaries are not required on the target runtime host.
- **Cross-Platform Setup Scripts**: The platform provides platform-specific setup scripts for automated installation and validation: `scripts/linux-setup.sh`, `scripts/macos-setup.sh`, and `scripts/windows-setup.ps1`.

---

## Deployment and Setup Workflow

Implementing an air-gapped deployment requires a connected staging host to resolve dependencies and compile the binaries before transfer to the target host.

### 1. Preparation on a Connected Host

1. **Compile Binaries**: Execute compilation targets for the target architecture on the staging machine:
   ```bash
   make build
   ```
2. **Package Runtime Configurations**: Archive the build artifacts and the protocol schemas:
   - The compiled `bin/g8e` g8e Node.
   - The protocol configuration files under the `protocol/` directory.
3. **Optional Container Build**: For containerized deployments, use the demo configurations in `demos/healthcare`, `demos/gov`, `demos/finance`, `demos/secure-data`, or `demos/swarm` as reference. The root `docker-compose.yml` defines both `g8e-gateway` and `g8e-operator` services using the same `Dockerfile` with different command-line flags.

### 2. Implementation on the Air-Gapped Target Host

1. **Stage Binaries and Schemas**: Copy the compiled `g8e` g8e Node and the schema directories to the target directory. Ensure the `g8e` g8e Node is executable.
2. **Initialize the Gateway**:
   ```bash
   ./g8e gw start
   ```
3. **Establish Local Session**: Log in to establish local credentials:
   ```bash
   ./g8e auth enroll
   ```
4. **Optional Remote Management**: Use operator remote management CLI commands (`cp`, `scp`, `deploy`, `stream`) to manage remote hosts within the air-gapped environment. These commands are defined in `internal/cli/cmd/operator.go`.

---

## Security Invariants

1. **Isolated Boundaries**: In gateway mode, the g8e Gateway does not initiate outbound connections to any external network addresses.
2. **Mutual Cryptographic Trust**: All traffic between the g8e Gateway, connected clients, and the g8e Operator is encrypted and authenticated using mutual TLS (mTLS) issued by the local Certificate Authority.
3. **Local Sovereignty**: All audit logs, transactions, and state records remain strictly on the host filesystem inside the local `.g8e` directory. Runtime paths are defined in `internal/paths/paths.go`, and directory name constants are defined in `internal/constants/paths.go`.
4. **Fail-Closed Design**: If any component requires a missing or unavailable external resource, it terminates immediately with a clear error instead of attempting unencrypted or insecure fallbacks.
5. **Mandatory Encryption at Rest**: All sensitive data stored in SQLite databases is encrypted using platform-managed encryption keys.

---

## See Also

- **[Connect Operator to Gateway](connect_operator_to_gateway.md)**: Remote management commands (cp, scp, stream, deploy) for operators inside the air-gapped environment.

