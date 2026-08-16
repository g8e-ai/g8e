---
title: Air Gap
parent: Guides
---

# Air-Gap Architecture

Last Updated: 2026-08-16
Version: v1.7.6

The g8e platform operates in environments without internet connectivity. The platform supports air-gapped deployments with zero runtime external network dependencies, using the g8e Gateway, the g8e Operator, and fully vendored Go dependencies in the root `vendor/` directory. The platform supports both binary deployment and containerized deployment via Docker.

---

## Privacy and Data Sovereignty

In an air-gapped configuration, the platform restricts all outbound communication and retains all data locally:

- **No Telemetry**: The platform disables all outbound telemetry, usage statistics, and error reporting.
- **Local Assets**: All user interface assets, fonts, icons, and libraries are served locally by platform services.
- **Local Persistence**: All platform state, including session records, configuration settings, and cryptographic keys, resides in local SQLite databases managed by the g8e Gateway within the `.g8e` directory.

---

## g8e Gateway Role

In an air-gapped deployment, the g8e Gateway operates as the central Policy Decision Point (PDP). Running the `g8e` binary in gateway mode activates persistence and messaging services on the local host.

### Communication Surfaces

The gateway exposes two logical communication surfaces:

- **HTTP (port 8080)**: Serves health checks, local trust bundles, CA discovery, CLI recovery request/status/complete (token-scoped), device and app enrollment, deploy scripts, and binary downloads. Unregistered paths redirect to the HTTPS port. OS trust installation is handled by `auth enroll` directly.
- **HTTPS (port 8443)**: Receives governance envelope mutation payloads, handles persistence, runs WebSocket pub/sub and SSE event streaming, serves MCP and A2A ingress, provides WebAuthn passkey authentication, and hosts the browser management console.

Surfaces with conflicting TLS client-authentication requirements do not share a network port. The initialization sequence validates port isolation and fails if configurations overlap.

### Core Functional Capabilities

- **State Persistence**: All system state is stored locally within SQLite databases inside the `.g8e` directory.
- **Local Public Key Infrastructure (PKI)**: The gateway generates a local Certificate Authority (CA) using ECDSA P-256 keys to issue and rotate TLS certificates for local services.
- **Secret Storage**: An internal encrypted vault stores local credentials and access tokens, removing any requirement for external key managers.
- **Event Brokerage**: A local WebSocket pub/sub broker manages real-time communication between the gateway and connected clients. Server-Sent Events (SSE) streaming provides event delivery to browser and CLI subscribers.
- **WebAuthn Passkey Bootstrap**: The gateway supports WebAuthn passkey-based authentication for secure local enrollment without external identity providers.

---

## Policy Execution Point: g8e Operator

The g8e Operator operates as the host-side Policy Execution Point (PEP). In an air-gapped deployment, the g8e Operator runs as a daemon on the target host and initiates a local mTLS connection to the g8e Gateway.

Every transaction or mutation payload wrapped in a `GovernanceEnvelope` undergoes sequential verification across the five-layer interlock sequence before execution on the host:

1. **L1 Doctrine**: Hard gates perform threat analysis, command blacklist checks, and pattern matching.
2. **L2 Consensus**: Multi-agent consensus signature verification validates the cryptographic signatures on the transaction using Ed25519.
3. **L3 Notary**: Human-in-the-loop authorization verifies approvals via WebAuthn passkeys (for web sessions) or cryptographically signed CLI proofs (for CLI sessions).
4. **L4 Warden**: Pre-dispatch verification gates validate signatures, replay prevention, expiration, transaction nonces, and the state Merkle root.
5. **L5 Actuator**: Isolated boundary tool dispatch executes the validated operation via MCP or A2A, with JIT capability minting and a cryptographically signed transaction receipt.

See [Governance](../architecture/governance.md) for the full interlock sequence details.

Verified operations are logged to a host-local ledger, and the Operator exposes local tools as a standalone Model Context Protocol (MCP) server.

---

## Build-Time versus Runtime Requirements

| Development Phase | Network Requirements | Air-Gap Isolation Strategy |
| :--- | :--- | :--- |
| **Build Phase** | Network access only needed for initial `go mod vendor`. Once vendored, builds are fully offline. | Compile and resolve dependencies on a connected build host, then transfer the vendored source tree to the air-gapped machine. |
| **Runtime Phase** | Zero external network access required. | All communications occur over localhost interfaces or private local networks. |

### Dependency Resolution and Build Tools

To ensure a self-contained installation, the build process packages all required components offline:

- **Go Dependencies**: The core platform compiles into a single statically-linked g8e binary. All Go dependencies are vendored into the root `vendor/` directory. The build uses `-mod=vendor` and sets `GOFLAGS=-mod=vendor` in the Dockerfile, ensuring no network access is needed during compilation. Run `go mod vendor` on a connected host to populate or refresh this directory.
- **Protocol Library (Go)**: The protocol is part of the root Go module `github.com/g8e-ai/g8e`. Since all dependencies are vendored, `go get github.com/g8e-ai/g8e@v1.7.5` works offline once the vendor directory is populated. No additional downloads are required.
- **Protocol Library (Python)**: The `g8e` Python package is published to PyPI. For air-gapped environments, download the wheel on a connected host and transfer it:

  ```bash
  # On connected host:
  pip download g8e==1.7.5 -d /tmp/g8e-python-pkg

  # Transfer /tmp/g8e-python-pkg/ to the air-gapped host, then:
  pip install --no-index --find-links /tmp/g8e-python-pkg g8e==1.7.5
  ```

  The package includes bundled JSON protocol constants. Set `G8E_PROTOCOL_DIR` if the constants directory is in a non-default location. See the [Protocol Library documentation](../architecture/protocol.md) for details.
- **Protocol Generation**: Protobuf compilation is performed offline using local tools without relying on the remote Buf Schema Registry (BSR). Configuration details are defined in `buf.gen.yaml` and `Makefile`.
- **Build-Time Tooling**: Protobuf code and documentation generation requires `buf`, `protoc-gen-go`, `protoc-gen-go-grpc`, and `protoc-gen-doc` during the build phase. These binaries are not required on the target runtime host.
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
   - The compiled `g8e` binary (produced at `bin/g8e-<os>-<arch>` with a root copy at `./g8e`).
   - The protocol configuration files under the `protocol/` directory.
3. **Optional Container Build**: For containerized deployments, use the demo configurations in `demos/healthcare`, `demos/finance`, `demos/dhs`, `demos/fedramp`, or `demos/frontend` as reference. Demos build from the repo-root `Dockerfile` via `context: ../..` (the same production image, always-FIPS, compiles from source using vendored modules in-container). The root `docker-compose.yml` defines both `g8e-gateway` and `g8e-operator` services using the same root `Dockerfile` with different command-line flags.
4. **Pre-Pull Docker Images (Containerized Deployments)**: For air-gapped Docker deployments, pre-pull all external images on the connected host:
   ```bash
   g8e demos pull
   g8e demos export /tmp/g8e-images
   ```
   This pulls all images pinned by sha256 digest in `demos/images.json` and saves them as `.tar` files. Transfer the export directory to the air-gapped machine and load them with `g8e demos import /tmp/g8e-images`.

### 2. Implementation on the Air-Gapped Target Host

1. **Stage Binaries and Schemas**: Copy the compiled `g8e` binary and the schema directories to the target directory. Ensure the `g8e` binary is executable.
2. **Initialize the Gateway**:
   ```bash
   ./g8e gw start
   ```
3. **Establish Local Session**: Log in to establish local credentials:
   ```bash
   ./g8e auth enroll
   ```
4. **Optional Remote Management**: Use operator remote management CLI commands (`cp`, `scp`, `deploy`, `stream`) to manage remote hosts within the air-gapped environment. See [Connect Operator to Gateway](connect_operator_to_gateway.md) for details.
5. **Verify Air-Gap Readiness**: Run the air-gap verification target to confirm vendored builds, image pinning, and script integrity:
   ```bash
   make test-airgap
   ```
   This checks that `vendor/` exists, the vendored build compiles, `demos/images.json` is present, no unpinned image references remain in compose files, and no `pip install` or `import requests` references remain in demo Python files.

---

## Security Invariants

1. **Isolated Boundaries**: In gateway mode, the g8e Gateway does not initiate outbound connections to any external network addresses.
2. **Mutual Cryptographic Trust**: All traffic between the g8e Gateway, connected clients, and the g8e Operator is encrypted and authenticated using mutual TLS (mTLS) issued by the local Certificate Authority.
3. **Local Sovereignty**: All audit logs, transactions, and state records remain strictly on the host filesystem inside the local `.g8e` directory.
4. **Fail-Closed Design**: If any component requires a missing or unavailable external resource, it terminates immediately with a clear error instead of attempting unencrypted or insecure fallbacks.
5. **Mandatory Encryption at Rest**: All sensitive data stored in SQLite databases is encrypted using platform-managed encryption keys.

---

## See Also

- **[Connect Operator to Gateway](connect_operator_to_gateway.md)**: Remote management commands (cp, scp, stream, deploy) for operators inside the air-gapped environment.
- **[Docker Gateway](docker_gateway.md)**: Containerized deployment details for gateway and operator services.
- **[Demos README](../../demos/README.md)**: Step-by-step air-gapped Docker image export/import workflow using `g8e demos export/import` and `demos/images.json`.
- **[Scripts README](../architecture/scripts.md)**: Central reference for all g8e deployment and bootstrap scripts.

