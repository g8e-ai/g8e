---
title: Air Gap
parent: Guides
---

# Air-Gapped Deployment

Last Updated: 2026-09-08
Version: v2.1.7

g8e runs without internet access when its binaries, container images, configuration, and any optional downstream services are staged inside the isolated environment. The Gateway and Operator do not require a hosted g8e service, and the repository vendors the Go modules required to build the `g8e` binary.

Air-gap isolation is an infrastructure property, not a g8e runtime mode. The CLI has no `--air-gap` switch, and the default Docker bridge networks do not block internet egress. Enforce the boundary with host, firewall, container-runtime, and network controls, and configure every optional integration to use an approved internal endpoint or remain disabled.

## Supported Deployment Forms

| Deployment form | Stage on the connected host | Requirements in the air gap |
| --- | --- | --- |
| Native binary | A `g8e` binary and its `.sha256` file for the target OS and architecture; custom doctrine files when used | The binary and access to the private hosts on which the Gateway and Operators run |
| Offline source build | The complete source tree, including `vendor/` | Go 1.26.6 and local build tools; set `GOTOOLCHAIN=local` and `GOFLAGS=-mod=vendor` |
| Containers | Fully built application images, every digest-pinned external image, Compose files, and bind-mounted configuration or demo data | Docker Engine and Docker Compose v2; start with pulling and building disabled |

A normal native Gateway deployment does not need the `protocol/` source tree at runtime. The Go protocol types and built-in compliance catalogs are compiled into the binary. Transfer external doctrine files passed through `--doctrine-dir` and any reference data required by a specific compliance command.

## Runtime Network Behavior

The Gateway listens on two ports by default:

- **8080 HTTP** provides health and state checks, initial bootstrap, CA bundle and fingerprint discovery, token-scoped CLI recovery, token-scoped platform enrollment, deploy scripts, and binary downloads. Other paths redirect to HTTPS.
- **8443 HTTPS** provides authenticated APIs, the embedded browser console, WebAuthn ceremonies, governance envelopes, MCP and A2A ingress, WebSocket pub/sub, SSE, audit APIs, and data services. Authentication varies by route: mTLS, a browser web session, JWT when JWKS is configured, or a scoped bootstrap or enrollment token.

The Operator opens no inbound service port for normal operation. It initiates an mTLS WebSocket connection to the Gateway and pulls work from its operator-specific channel. Gateway, Operator, CLI, dashboard, ensemble, consensus, and downstream traffic can cross a private network; air-gapped does not mean localhost-only.

The platform does not send product analytics or error reports to a hosted g8e service. It does record local operational events, SSE events, audit records, and model-call telemetry. Optional features can initiate network connections and must be reviewed before deployment:

- `--jwks-url` fetches keys from the configured identity provider.
- `--mcp-downstream-url` and `--a2a-downstream-url` proxy to configured services.
- Consensus and Operator endpoints connect to their configured private addresses.
- The ensemble calls the selected LLM provider. Use the built-in `fake` provider for deterministic operation without a model server, or point Ollama, llama.cpp, or another supported provider at an internal endpoint. Do not configure public OpenAI, Anthropic, Gemini, or other internet endpoints inside the air gap.
- Native network, HTTP probe, DNS, cloud metadata, SSH, copy, deploy, and streaming tools access destinations requested by admitted operations. Governance controls admission; network policy controls reachability.

WebAuthn passkeys do not require an external identity provider. Leave JWKS configuration empty when local passkeys and mTLS are the only identity mechanisms.

## Local Assets and Persistence

The Gateway browser console is embedded in the `g8e` binary and is served locally. The Gateway creates its runtime tree under `.g8e/` by default. Runtime state is not confined to one SQLite database:

- `data/` contains the canonical SQLite database, audit database, commitment data, and other persisted records.
- `pki/` contains generated authorities, serving certificates, workload identities, revocation data, and trust bundles.
- `secrets/` contains platform secret material managed by the local keystore.
- `vault/` contains vault state and the local vault key.
- `logs/`, `pids/`, and other runtime directories contain operational state.

The vault is mandatory for the audit store. Sensitive audit content fields, including content text and command standard output and error, are encrypted before storage. The SQLite files as a whole are not SQLCipher-encrypted, and not every field or file in `.g8e/` is encrypted. Protect the runtime directory with restrictive filesystem permissions, full-disk or volume encryption, controlled backups, and physical access controls. Back up the vault key with the encrypted data; storing only one makes the backup unusable.

The Gateway and each Operator have separate runtime trees. An Operator is authoritative for its host-local audit and execution state, while the Gateway stores platform, identity, routing, and mirrored audit state. Container deployments persist these trees in separate named volumes.

## Prepare a Native Binary

Run these commands on a connected build host from the repository root:

```bash
# Verify that the checked-in vendor tree supports a vendored build and run the repository's static air-gap checks.
make test-airgap

# Build for the connected host's OS and architecture without toolchain or module downloads.
GOTOOLCHAIN=local GOFLAGS=-mod=vendor make build

# For other supported targets, build all platform binaries instead.
GOTOOLCHAIN=local GOFLAGS=-mod=vendor make build-all
```

`make build` writes `bin/g8e-<os>-<arch>` (with `.exe` on Windows), writes a neighboring `.sha256` file, and copies the host binary to `./g8e`. `make build-all` writes binaries and checksums for all supported targets. Go 1.26.6 must already be installed when `GOTOOLCHAIN=local` is set; this prevents Go's automatic toolchain selection from downloading another toolchain.

Transfer the target binary, its checksum, and any custom doctrine directory through the approved media-transfer process. Preserve the generated `bin/` path because the checksum file records that relative filename. Verify the checksum on the target before installation:

```bash
sha256sum -c bin/g8e-linux-amd64.sha256
install -m 0755 bin/g8e-linux-amd64 ./g8e
```

Start and enroll a local-only Gateway with:

```bash
./g8e gw start --cert-mode localhost
./g8e auth enroll user -e localhost
```

Omit `--cert-mode localhost` when clients or Operators connect over a private network. The default `full` certificate mode detects host identities; ensure the selected private hostname or address is present in the serving certificate and pass the Gateway endpoint to enrollment and Operator commands. Use `./g8e auth enroll user --headless -e <gateway>` for a CLI-only owner when no browser is available. Headless enrollment does not create a passkey and therefore cannot authenticate to the browser console or provide WebAuthn approval.

## Prepare Container Images

Build container images on the connected host. The repository Dockerfiles are not offline build recipes:

- The Gateway/Operator Dockerfile runs `apt-get` in both build and runtime stages.
- The ensemble Dockerfile installs Python packages with `pip`.
- The dashboard Dockerfile installs packages with `npm` and `apk`.

Pre-pulling only the base images does not make these Docker builds offline. Build the final images before transfer, then preserve their exact repository names and tags in the exported archive.

### Unified stack

```bash
# Build the Gateway/Operator, ensemble, and dashboard images while connected.
docker compose --profile bootstrapped build

# Record the exact image names that Compose expects.
docker compose --profile bootstrapped config --images

# Save each unique image name shown by the preceding command.
docker save -o /tmp/g8e-unified-images.tar <image-ref> [<image-ref> ...]
```

Transfer the image archive and the repository's `docker-compose.yml`. On the isolated host:

```bash
docker load -i /media/g8e-unified-images.tar

# Start only the Gateway without pulling or building.
docker compose up -d --no-build --pull never
./g8e auth enroll user -e localhost

# Start the enrolled workloads, then approve their pending requests.
docker compose --profile bootstrapped up -d --no-build --pull never
./g8e auth pending-platform-enrollments
./g8e auth approve-platform-enrollment <operator-request-id> --yes
./g8e auth approve-platform-enrollment <ensemble-request-id> --yes
./g8e auth approve-platform-enrollment <dashboard-request-id> --yes
```

Before sending model requests, configure every model role in use through platform settings or a Compose override. For deterministic operation without a model server, pass both `G8E_LLM_PRIMARY_PROVIDER=fake` and a primary model name such as `G8E_LLM_PRIMARY_MODEL=fake`; the root Compose file does not forward these host variables unless they are added to the ensemble service's `environment` list. Otherwise configure a supported provider with an approved internal endpoint. See [Unified Docker Stack](unified_stack.md) for identity, volume, hostname, and port configuration.

### Demo stacks

The demo image manifest contains the digest-pinned external images referenced by the four per-demo Compose files plus the Gateway/Operator build-stage and runtime bases. On the connected host:

```bash
./g8e demos pull
./g8e demos export /tmp/g8e-external-images

# Build the source-based images for each demo that will be transferred.
docker compose -f demos/<org>/compose.yml build
docker compose -f demos/<org>/compose.yml config --images

# Save the unique source-built image names from the list; digest references are already in the external-image export.
docker save -o /tmp/g8e-<org>-built-images.tar <built-image-ref> [<built-image-ref> ...]
```

`g8e demos export` saves only the images listed in `demos/images.json`; it does not save locally built Gateway/Operator images. Transfer both archives, the selected `demos/<org>/` tree, and `demos/images.json`. On the isolated host:

```bash
./g8e demos import /media/g8e-external-images
docker load -i /media/g8e-<org>-built-images.tar
docker compose -f demos/<org>/compose.yml up -d --no-build --pull never
```

Use direct Compose commands with `--no-build --pull never` for strict offline startup. `./g8e demos start <org>` runs `docker compose up -d` without those explicit controls. Complete owner and Operator enrollment using the Gateway ports printed for the selected demo; see [Demos README](../../demos/README.md) for the per-demo port map and bootstrap sequence.

## Protocol Libraries and Generation

The root `vendor/` directory makes this repository buildable with `-mod=vendor`. It does not make `go get github.com/g8e-ai/g8e/v2@<version>` work offline in an unrelated Go module. Downstream Go applications need their own staged source and vendor tree, a pre-populated module cache, or an internal Go module proxy.

Build a Python wheel and collect its transitive dependencies on the connected host:

```bash
make python-build
pip download --dest /tmp/g8e-python-wheels protocol/python/dist/g8e-2.1.7-py3-none-any.whl
```

Transfer the complete wheel directory, then install without an index:

```bash
pip install --no-index --find-links /media/g8e-python-wheels g8e==2.1.7
```

The Python package includes its JSON constants under `g8e/_data`; there is no `G8E_PROTOCOL_DIR` runtime setting.

Generated Go, Python, and Node protocol sources are already present in the repository. Protocol regeneration is not required on a runtime host. `make proto` is not inherently offline because its setup paths can install Buf, Python, and Node tooling and can update lock files. To regenerate inside an isolated build environment, stage the pinned generator binaries and all Python and Node package dependencies first. The cross-platform scripts under `scripts/` bootstrap developer workspaces and can invoke operating-system package managers; they are not air-gap installers.

## Verification and Isolation Checklist

Run `make test-airgap` in the staged source tree before transfer. The target currently verifies:

1. `vendor/` exists.
2. `go build -mod=vendor ./...` succeeds.
3. `demos/images.json` exists.
4. Demo Compose files contain none of the tag patterns checked as unpinned by the target.
5. Demo Python files contain no `pip install` or `import requests` references.

This target is a build and static-reference check. It does not prove that container builds work offline, that every required image was exported, that runtime egress is blocked, or that configured integrations use only internal endpoints. Verify those properties separately:

- Deny outbound traffic at the host firewall and network perimeter, and test the denial.
- If containers must have no egress beyond approved peers, apply container-network firewall policy; the repository's bridge networks are not egress-deny boundaries.
- Start containers with `--no-build --pull never` and confirm all services become ready without registry or package-repository access.
- Keep `--jwks-url`, public LLM endpoints, and public downstream MCP or A2A URLs unset.
- Point consensus, Gateway, Operator, model, DNS, NTP, and other required services at approved private endpoints.
- Verify transferred binary and image digests through the organization's media-transfer process.
- Persist and back up each `.g8e/` runtime tree or named volume according to local retention policy.
- Monitor firewall and DNS logs for attempted external connections during acceptance testing.

## Security Boundaries

- g8e authenticates Gateway-to-Operator and privileged client traffic with locally issued certificates, but bootstrap HTTP and browser web-session routes are not mTLS traffic.
- The Gateway can call configured JWKS, downstream MCP/A2A, consensus, and private service endpoints. Network controls, not the Gateway process, enforce destination reachability.
- Governance decides whether a requested tool operation is admitted. It does not replace firewall rules for tools that perform DNS, HTTP, SSH, cloud metadata, or other network operations.
- Sensitive audit fields use the local vault, while host or volume encryption protects complete databases, PKI files, logs, and other runtime artifacts.
- Missing posture-required proofs and unavailable required local security services fail closed. Optional network integrations remain disabled when their endpoint settings are empty.

## See Also

- **[Connect Operator to Gateway](connect_operator_to_gateway.md)**: Operator enrollment and private-network management.
- **[Docker Gateway](docker_gateway.md)**: Gateway and Operator image, ports, identity, and persistence.
- **[Unified Docker Stack](unified_stack.md)**: Four-service Compose deployment and bootstrap flow.
- **[Demos README](../../demos/README.md)**: Demo image manifest, export/import commands, and per-demo startup.
- **[Network Architecture](../architecture/network.md)**: Listener, mTLS, certificate, and network identity design.
- **[Encryption](../architecture/encryption.md)**: Vault encryption, scrubbing, and rehydration boundaries.
