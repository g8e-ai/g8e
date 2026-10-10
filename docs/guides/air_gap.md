---
doc_id: air_gap
title: Air-Gapped Deployment
audience: platform operators and infrastructure teams
status: current
last_updated: 2026-10-06
version: v2.3.2
owners:
  - docs/guides/air_gap.md
  - Dockerfile
  - docker-compose.yml
  - Makefile
  - go.mod
  - protocol/python/pyproject.toml
related:
  - docs/guides/unified_stack.md
  - docs/guides/docker_gateway.md
  - docs/guides/connect_operator_to_gateway.md
  - docs/architecture/network.md
  - docs/architecture/encryption.md
when_to_read: Deploying g8e inside a network-isolated environment; staging binaries, containers, and protocol libraries for offline transfer.
do_not_use_for:
  - Runtime encryption and vault mechanics (docs/architecture/encryption.md)
  - Network identity, PKI, and TLS configuration (docs/architecture/network.md)
  - Docker image and container port configuration (docs/guides/docker_gateway.md)
  - Operator enrollment and cross-gateway connectivity (docs/guides/connect_operator_to_gateway.md)
---

# Air-Gapped Deployment

## Purpose

Documents how to stage g8e for deployment in network-isolated environments using native binaries, source builds, or prebuilt container images. Covers media transfer, offline build procedures, workload enrollment, and air-gap verification.

## Quick index

- [Purpose](#purpose)
- [Quick index](#quick-index)
- [Invariants](#invariants)
- [Owned surfaces](#owned-surfaces)
- [Procedures](#procedures)
  - [Supported deployment forms](#supported-deployment-forms)
  - [Runtime network behavior in air-gap](#runtime-network-behavior-in-air-gap)
  - [Local assets and persistence](#local-assets-and-persistence)
  - [Prepare a native binary](#prepare-a-native-binary)
  - [Prepare container images](#prepare-container-images)
  - [Protocol libraries and generation](#protocol-libraries-and-generation)
  - [Verification and isolation checklist](#verification-and-isolation-checklist)
- [Anti-patterns](#anti-patterns)
- [Links out](#links-out)

Key invariant groups: [Isolation Properties](#isolation-properties-inv-air-iso), [Build and Transfer](#build-and-transfer-inv-air-build), [Runtime Behavior](#runtime-behavior-inv-air-runtime).

## Invariants

Ids are stable. Append the next free number in a group. Do not renumber.

### Isolation Properties (`INV-AIR-ISO`)

| ID | Rule |
| --- | --- |
| INV-AIR-ISO-01 | Air-gap isolation is an infrastructure property enforced by host, firewall, container-runtime, and network controls. The CLI has no `--air-gap` switch and the default Docker bridge networks do not block internet egress. |
| INV-AIR-ISO-02 | Every optional integration (JWKS, MCP/A2A proxies, consensus, LLM providers) MUST use an approved internal endpoint or remain unset (empty configuration). Public endpoints such as OpenAI, Anthropic, Gemini, or other internet-facing services MUST NOT be configured inside the air gap. |
| INV-AIR-ISO-03 | Governance admits tool operations; network policy controls reachability. Operations that perform DNS, HTTP, SSH, cloud metadata, or other network operations require governance authorization. Network controls do not supplement governance — they enforce the boundary that governance defines. |

### Build and Transfer (`INV-AIR-BUILD`)

| ID | Rule |
| --- | --- |
| INV-AIR-BUILD-01 | Native binary builds MUST use `GOTOOLCHAIN=local GOFLAGS=-mod=vendor` to ensure the checked-in vendor tree provides all dependencies. Go 1.26.9 must already be installed; automatic toolchain selection is prohibited. |
| INV-AIR-BUILD-02 | Container image builds on a connected host MUST complete fully (including `apt-get`, `pip`, and `npm` install steps) before transfer. Pre-pulling base images alone does not make Dockerfile builds offline. Final images MUST preserve exact repository names and tags for offline loading. |
| INV-AIR-BUILD-03 | A source archive MUST include the committed Gateway frontend embeds. `make build` uses those embeds when local `dist/` trees are absent; release preparation rebuilds them explicitly. |
| INV-AIR-BUILD-04 | `make test-airgap` verifies vendor tree presence and a vendored Go build, but does NOT verify container build offline-capability, image completeness, runtime egress blocking, or endpoint configuration. |

### Runtime Behavior (`INV-AIR-RUNTIME`)

| ID | Rule |
| --- | --- |
| INV-AIR-RUNTIME-01 | The Gateway listens on HTTP port 8080 (bootstrap, CA discovery, token-scoped enrollment) and HTTPS port 8443 (authenticated APIs, console, WebSocket pub/sub). Remaining unhandled HTTP paths redirect to HTTPS. Port values are derived from `internal/constants/ports.go`. |
| INV-AIR-RUNTIME-02 | The Operator opens no inbound service port. It initiates an outbound-only mTLS WebSocket connection to the Gateway's pub/sub endpoint and pulls work from its operator-specific channel. |
| INV-AIR-RUNTIME-03 | The platform does not send product analytics or error reports to any hosted service. It records local operational events, SSE events, audit records, and model-call telemetry. |
| INV-AIR-RUNTIME-04 | Gateway and Operator each maintain separate `.g8e/` runtime trees on their host or container volume. The Gateway is not a central database server for remote Operator execution. Operator state is local and authoritative to that host. |

## Owned surfaces

| Claim | Path | Verify |
| --- | --- | --- |
| Makefile air-gap target | `Makefile` | `make test-airgap` |
| Go vendor build support | `go.mod`, `vendor/` | `go build -mod=vendor ./...` succeeds |
| Container image build | `Dockerfile`, `docker-compose.yml` | `docker compose build` completes without external registry access on connected host |
| Python wheel build | `protocol/python/pyproject.toml`, `Makefile` | `make protocol-python-build` produces `protocol/python/dist/g8e-2.3.2-py3-none-any.whl` |
| Gateway port defaults | `internal/constants/ports.go` | HTTP 8080, HTTPS 8443 |

## Procedures

### Supported deployment forms

Three offline transfer models are supported:

1. **Native binary**: Transfer a prebuilt `g8e` binary and its `.sha256` checksum sidecar for the target OS and architecture. No runtime compiler or build tools required. Simplest form factor for isolated hosts.

2. **Offline source build**: Transfer the complete source tree including `vendor/`, the built evaluation-explorer asset, and a connected host where Go 1.26.9 is available. Allows rebuilds inside the air gap without external network access.

3. **Container images**: Transfer fully built Docker images, `docker-compose.yml`, and the host-side `g8e` CLI. Enables multi-service deployments with automatic workload orchestration.

For native deployments, the `protocol/` source tree, Dockerfiles, and developer scripts are not required at runtime. Transfer external doctrine files passed through `--doctrine-dir`, consensus bootstrap files via `--consensus-bootstrap`, model assets, and any reference data required by compliance commands. Keep the host-side `g8e` CLI available for enrollment and operational commands.

### Runtime network behavior in air-gap

The Gateway exposes a consolidated two-port topology as default:

- **HTTP port 8080**: Health checks, initial bootstrap, CA bundle and fingerprint discovery, token-scoped CLI recovery, token-scoped platform enrollment, and binary downloads. All other paths return HTTP 301 redirect to HTTPS.
- **HTTPS port 8443**: Authenticated APIs, embedded browser console, WebAuthn ceremonies, governance envelopes, MCP and A2A ingress, WebSocket pub/sub, SSE, audit APIs, and data services. Authentication per route: mTLS, browser web session, JWT when JWKS is configured, or scoped bootstrap or enrollment token.

Port configuration is derived from [internal/constants/ports.go](../../internal/constants/ports.go). Modify with `--http-port` and `--https-port` flags to `gw start`.

The Operator initiates an outbound-only mTLS WebSocket connection to the Gateway and pulls work from its operator-specific channel. No inbound service port is opened. Gateway, Operator, CLI, console, ensemble, consensus, and downstream service traffic can cross private networks; air-gapped does not mean localhost-only.

Outbound network integrations that require review before deployment:

- If JWT authentication uses an external JWKS provider, configure its endpoint to an approved internal service. Leave JWT/JWKS integration unset when using local passkeys and mTLS only.
- `--mcp-downstream-url` and `--a2a-downstream-url`: Proxy to configured services. Must point to approved internal endpoints.
- Consensus and Operator endpoints: Connect to their configured private addresses.
- LLM provider: The ensemble calls the selected provider. Use the built-in `fake` provider for deterministic operation without a model server, or configure an internal Ollama, llama.cpp, or compatible endpoint. Do not configure public OpenAI, Anthropic, Gemini, or internet-facing services.
- Tool operations: Native network, HTTP, DNS, cloud metadata, SSH, copy, deploy, and streaming tools access destinations requested by admitted operations. Governance controls admission; network policy controls reachability.

### Local assets and persistence

The Gateway browser console is embedded in the `g8e` binary and served locally without external CDN access. The Gateway creates its runtime tree under `.g8e/` by default. Runtime state is organized as follows:

- **data/**: Contains `g8e.db` (main state), suspended-transaction databases, and Operator-specific execution and replay stores. Gateway and Operator databases are local to their runtime; the Gateway is not a central database for remote Operator execution.
- **data/ledger/**: Optional file-ledger repositories. Evaluation and component-specific artifacts use subdirectories under `data/`.
- **pki/**: Generated Root CA, intermediates, serving certificates, workload identities, revocation data, and trust bundles.
- **secrets/**: Platform secret material managed by the local keystore.
- **vault/**: Vault encryption key and related state. Mandatory for the audit and execution-vault services.
- **logs/**, **pids/**: Operational runtime state.

The vault is mandatory for audit and execution-vault services. Sensitive audit fields (event content, command stdout/stderr) are encrypted before storage. The suspended-transaction database, structured metadata, and SQLite files as a whole are not uniformly encrypted; only vault-covered fields are encrypted. The file ledger can contain plaintext copies when the vault is locked.

Protect the runtime directory with restrictive filesystem permissions, full-disk or volume encryption, controlled backups, and physical access controls. Back up the vault key alongside encrypted data; storing only one makes the backup unusable.

Gateway and Operator have separate runtime trees. Operators are authoritative for their host-local audit and execution state. The Gateway stores platform, identity, routing, and mirrored audit state. Container deployments persist these trees in separate named Docker volumes, defined in [docker-compose.yml](../../docker-compose.yml).

### Prepare a native binary

Run these commands on a connected build host from the repository root:

```bash
# Verify the checked-in vendor tree and run static air-gap checks.
GOTOOLCHAIN=local GOFLAGS=-mod=vendor make test-airgap

# If the evaluation-explorer asset is not already staged, build it while connected.
cd evaluation-explorer && npm run build
cd ..

# Build for the connected host's OS and architecture.
GOTOOLCHAIN=local GOFLAGS=-mod=vendor make build

# Or build all supported platforms (Linux, Windows, Darwin with multiple architectures).
GOTOOLCHAIN=local GOFLAGS=-mod=vendor make build-all
```

Build commands produce:
- `make build`: Writes `bin/g8e-<os>-<arch>` (with `.exe` on Windows), a neighboring `.sha256` file, and copies the host binary to `./g8e`.
- `make build-all`: Writes binaries and checksums for Linux (amd64, arm64, 386), Windows (amd64, arm64), and Darwin (amd64, arm64). Publishes `bin/g8e-binaries.json` only after the complete matrix validates.

Go 1.26.9 must already be installed before running with `GOTOOLCHAIN=local`; this prevents Go from automatically downloading another toolchain version.

Transfer the target binary, its checksum, the complete manifest when using a full matrix, and any custom doctrine directory through the approved media-transfer process. Verify checksums from the directory containing the transferred artifacts:

```bash
cd bin
sha256sum -c g8e-linux-amd64.sha256
install -m 0755 g8e-linux-amd64 ../g8e
```

Start a local-only Gateway:

```bash
./g8e gw start --cert-mode localhost
./g8e auth enroll user
```

For private-network deployments, omit `--cert-mode localhost`. The default `full` certificate mode detects network hostnames and IPs; ensure the selected private hostname or address is present in the serving certificate and pass the Gateway endpoint to enrollment and Operator commands using `-e <hostname>` or `-e <hostname:port>`.

For a CLI-only owner when no browser is available:

```bash
./g8e auth enroll user --headless
```

Headless enrollment produces an mTLS-only identity without a passkey, preventing authentication to the browser console. Recovery or approval of platform workloads must be delegated to an already-enrolled CLI via `./g8e auth approve-recovery <token>`.

### Prepare container images

Build container images on the connected host. The repository Dockerfiles are not offline build recipes:

- **Dockerfile**: Runs `apt-get` in both build and runtime stages to install g8e, Ollama integration tools, and runtime dependencies.
- **Ensemble Dockerfile**: Installs Python packages with `pip`.

Pre-pulling only base images does not make these Dockerfiles offline-capable. Build the final images completely while connected, then preserve their exact repository names and tags in the exported archive.

#### Unified stack

Build all images for the default stack (Gateway, Operator, Ensemble, Inference Operator):

```bash
docker compose build

# Record exact image names.
docker compose config --images

# Export each unique image.
docker save -o /tmp/g8e-unified-images.tar <image-ref> [<image-ref> ...]
```

Transfer the image archive, `docker-compose.yml`, the host-side `g8e` CLI, and any `.env` or Compose override for private endpoints. On the isolated host:

```bash
docker load -i /media/g8e-unified-images.tar

# Start only the Gateway.
docker compose up -d --no-build --pull never
./g8e auth enroll user

# Start all workloads.
docker compose up -d --no-build --pull never

# List and approve pending enrollment requests.
./g8e auth enroll pending
./g8e auth enroll approve <operator-request-id> --yes
./g8e auth enroll approve <ensemble-request-id> --yes
./g8e auth enroll approve <inference-operator-request-id> --yes
```

Configure model roles before sending model requests. For deterministic operation without a model server, set `G8E_LLM_PRIMARY_PROVIDER=fake` and `G8E_LLM_PRIMARY_MODEL=fake` in the environment or `.env` file. The root Compose file must include these in the ensemble service's `environment` list. Otherwise configure a supported provider with an approved internal endpoint. Model weights and Ollama runtime must be staged separately; do not assume that g8e images include model files or Ollama.

See [Unified Docker Stack](unified_stack.md) for identity, volume, hostname, and port configuration.

### Protocol libraries and generation

The root `vendor/` directory makes the repository buildable with `-mod=vendor`. It does not enable `go get github.com/g8e-ai/g8e/v2@<version>` to work offline in an unrelated Go module. Downstream Go applications need their own staged source tree, vendor tree, pre-populated module cache, or internal Go module proxy.

Build the Python protocol wheel and collect its transitive dependencies on the connected host:

```bash
make protocol-python-build
pip download --dest /tmp/g8e-python-wheels protocol/python/dist/g8e-2.3.2-py3-none-any.whl
```

Transfer the complete wheel directory, then install without an index:

```bash
pip install --no-index --find-links /media/g8e-python-wheels g8e==2.3.2
```

The Python package includes JSON constants at `g8e/_data`; there is no `G8E_PROTOCOL_DIR` runtime setting.

Generated Go, Python, and Node protocol sources are already present in the repository. Protocol regeneration is not required at runtime. `make proto-generate` is not inherently offline; its setup paths can install Buf, Python, and Node tooling and update lock files. To regenerate inside an isolated build environment, stage the pinned generator binaries and all Python and Node dependencies first.

Cross-platform scripts under `scripts/` bootstrap developer workspaces and invoke operating-system package managers; they are not air-gap installers.

### Verification and isolation checklist

Run `make test-airgap` in the staged source tree before transfer. This target verifies:

1. `vendor/` exists.
2. `go build -mod=vendor ./...` succeeds.

This is a build and static-reference check. It does NOT verify container build offline-capability, image completeness, runtime egress blocking, or endpoint configuration. Verify those properties separately:

- Deny outbound traffic at the host firewall and network perimeter, then test the denial.
- Apply container-network firewall policy if containers must have no egress beyond approved peers. (Repository bridge networks are not egress-deny boundaries.)
- Start containers with `--no-build --pull never` and confirm all services become ready without registry or package-repository access.
- Keep any external JWT/JWKS integration, public LLM endpoints, and public downstream MCP/A2A URLs unset.
- Point consensus, Gateway, Operator, model, DNS, NTP, and other required services at approved private endpoints.
- Verify transferred binary and image digests through the organization's media-transfer process.
- Persist and back up each `.g8e/` runtime tree or named Docker volume according to local retention policy.
- Monitor firewall and DNS logs for attempted external connections during acceptance testing.

## Anti-patterns

- Starting container builds with only base images pre-pulled, expecting Dockerfile package installs to work offline (INV-AIR-BUILD-02).
- Building `make build` without first building the evaluation-explorer asset (INV-AIR-BUILD-03).
- Using `GOTOOLCHAIN=auto` or installing Go 1.26.9 on-demand during offline builds; always use `GOTOOLCHAIN=local` with pre-installed toolchain (INV-AIR-BUILD-01).
- Configuring public LLM endpoints, identity providers, or MCP/A2A proxies inside the air gap (INV-AIR-ISO-02).
- Relying on `make test-airgap` to prove offline runtime operation; verify network controls and endpoint configuration separately (INV-AIR-BUILD-04).
- Storing vault backup keys separately from encrypted data; restore usability requires both (INV-AIR-RUNTIME-04).
- Sharing `.g8e/` runtime trees between Gateway and Operator or between multiple Operator instances (INV-AIR-RUNTIME-04).

## Links out

- **[Unified Docker Stack](unified_stack.md)**: Multi-service Compose deployment, bootstrap flow, and environment configuration.
- **[Docker Gateway](docker_gateway.md)**: Container image details, port mapping, identity configuration, and volume persistence.
- **[Connect Operator to Gateway](connect_operator_to_gateway.md)**: Operator enrollment and private-network connectivity.
- **[Network Architecture](../architecture/network.md)**: PKI hierarchy, SPIFFE identities, TLS/mTLS enforcement, and port topology.
- **[Encryption](../architecture/encryption.md)**: Vault encryption, sensitive field scrubbing, and rehydration boundaries.
