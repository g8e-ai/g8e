---
title: Air Gap
parent: Architecture
---

# Air-Gap Architecture

Last Updated: 2026-05-20
Version: v0.2.7

g8e is designed for high-security environments where internet connectivity is strictly prohibited. The platform supports fully air-gapped deployments with **zero runtime internet dependencies**, achieving this through a self-contained **Substrate** (Governance Gateway, Governed Operator, and Protocol), vendored dependencies, and optional local LLM inference.

---

## Zero-Trust Privacy Principle

The air-gap configuration is the "Canonical Truth" of g8e's privacy model. In this mode, the platform operates as a completely sealed unit:

- **No Telemetry:** Zero outbound usage, health, or error data is sent to Lateralus Labs.
- **Local Assets:** All frontend assets (fonts, icons, JS libraries) are served locally by the application layer.
- **Local Persistence:** All platform state, including chat history, settings, and secrets, is stored in a unified SQLite database managed by the Governance Gateway (`g8eg`) in Listen Mode.

---

## The Platform Backbone: Governance Gateway (g8eg)

In an air-gapped deployment, the platform requires a local "Hub" for persistence and messaging. This is provided by running the Governance Gateway (`g8eg` / `g8e.gateway` binary) in **Listen Mode** (`--listen`). In this mode, the Governance Gateway acts as the platform's central persistence and messaging backbone rather than an outbound execution agent.

### Architecture & Ports
The Governance Gateway in Listen Mode exposes four logical surfaces. Defaults are sourced from `protocol/constants/paths.json`. The TLS surfaces multiplex onto a single port when configured equal; Bootstrap must remain on its own port to be served as plain HTTP.

| Surface | Port (default) | Auth | Purpose |
|---|---|---|---|
| **Bootstrap** | `8441` (plain HTTP) | None | Download trust bundles, device-link enrollment, and CSR signing. |
| **Public Port** | `8440` (TLS) | Web session | Browser login, WebAuthn challenge, and PKI discovery. |
| **mTLS API** | `8440` | mTLS + URI SAN | Central `/api/governance/envelope` mutation endpoint and `/db` persistence. |
| **Pub/Sub** | `8440` (mTLS WSS) | mTLS + URI SAN | Real-time WebSocket event fan-out. |

When the mTLS, Public, and Pub/Sub surfaces share a port, the gateway serves them through a single `MasterRouter` with `tls.VerifyClientCertIfGiven`; per-route handlers enforce mTLS and URI SAN on substrate routes. To deploy on privileged ports (for example, `443` and `80`), grant the gateway binary `CAP_NET_BIND_SERVICE` or use an external port redirect; the unprivileged defaults above require no out-of-band setup. See `docs/g8eg.md` for the full multiplex contract.

### Core Responsibilities
- **Unified Persistence:** Replaces external databases with a single `g8e.db` SQLite file in `.g8e/data`.
- **Internal PKI:** Acts as the platform's Certificate Authority (CA), auto-generating ECDSA P-384 TLS certificates for all inter-component traffic.
- **Secret Management:** Provides an encrypted Vault for storing platform secrets (API keys, internal tokens) without external dependencies.
- **Messaging:** Serves as the central Pub/Sub broker for all compliant clients.

---

## The PEP: Governed Operator (g8eo)

To execute mutations on the local air-gapped host, a **Governed Operator (`g8eo` / `g8e.operator`)** daemon runs in standard mode on the target machine:
- Connects outbound-only over local mTLS WSS to the local `g8eg` gateway.
- Subscribes to command events, processes them via local Warden boundaries, and writes history to a host-local ledger.
- Exposes tools to standard local clients as a Model Context Protocol (MCP) Server.

---

## Local LLM Inference

For air-gapped reasoning, g8e supports external local inference via the `g8ee` (Engine) component's `LlamaCppProvider`.

- **Engine:** HTTP client to external `llama.cpp` server via OpenAI-compatible API.
- **Default Model:** `Gemma 4 E2B` (optimized for local reasoning).
- **Interface:** Configured via `llamacpp_endpoint` setting (default: `http://localhost:11444`).
- **Provisioning:** Model GGUF files must be pre-staged on the external llama.cpp server. The Engine does not download models; it is a client only.

---

## Build-Time vs. Runtime

| Phase | Internet Requirement | Air-Gap Strategy |
|---|---|---|
| **Build** | Required (Default) | Use the `setup` workflow on a connected machine to cache all base images and vendor dependencies. |
| **Runtime** | **None** | All components communicate exclusively over the internal `g8e-network` or localhost. |

### Vendoring & Dependency Management

**Direct Dependency Invariant:** All package manifests must reflect only direct imports. Transitive dependencies are not explicitly listed. This invariant is enforced by header comments in `services/g8ee/requirements.txt` and auditable via `./g8e test g8ee`.

**Substrate (Go):**
- 100% vendored in `services/g8eo/vendor/`.
- Build tools declared in `services/g8eo/go.mod` (protoc-gen-go, protoc-gen-go-grpc).
- Protocol generation uses local `buf` and `protoc` (no remote BSR dependency).

**Python Runtime (g8ee):**
- Direct dependencies frozen in `services/g8ee/requirements.txt`.
- Categories: web framework (fastapi, uvicorn), LLM providers (google-genai, anthropic, openai), protocol (protobuf, grpcio), storage (sqlalchemy, alembic), utilities (tenacity, python-dateutil).
- No vendoring; use pre-staged virtual environment or Docker image.

**Protocol Package:**
- Python bindings in `protocol/python/g8e_protocol/` generated from `.proto` files.
- Build dependency: `grpcio-tools` for Python stub generation.

**Evals Suite:**
- Dependencies in `evals/pyproject.toml` (pytest, httpx, pydantic).
- Separate from runtime g8ee dependencies.

**Build-Time Tools:**
- `buf` (Buf CLI) for protobuf schema management.
- `protoc-gen-go`, `protoc-gen-go-grpc` for Go stubs.
- `grpcio-tools` for Python stubs.
- These tools are not required at runtime.

---

## Deployment Workflow

### 1. Preparation (Connected Environment)
1. **Bootstrap Platform:** Run `./g8e platform setup` on a connected machine to cache dependencies and build binaries.
2. **Download Model:** Obtain the `Gemma 4 E2B` GGUF model file.
3. **Export Assets:** Bundle the `.g8e` runtime directory and component binaries.

### 2. Implementation (Air-Gapped Host)
1. **Stage Binaries:** Place the `g8e.gateway` and `g8e.operator` binaries and component source/images on the host.
2. **Stage External LLM Server:** Deploy llama.cpp server with pre-staged `.gguf` model files on the host or adjacent network.
3. **Configure:**
   - Set `search.enabled` to `false` in `SearchSettings`.
   - Set `llamacpp_endpoint` to the external llama.cpp server URL (default: `http://localhost:11444`).
   - Ensure `llm_primary_provider` is set to `llamacpp`.
4. **Launch:** `./g8e platform up`

---

## Security Invariants

1. **No Outbound Dialing:** In Listen Mode, the Governance Gateway (`g8eg`) is forbidden from initiating connections to any address outside the local platform.
2. **Mutual Trust:** All internal traffic between Governance Gateway, Engine, and Operator is encrypted using the Operator's internal CA.
3. **Data Sovereignty:** All audit logs, chat history, and telemetry remain strictly on the host's filesystem in the `.g8e` directory.
4. **Fail-Closed Privacy:** If a component requires an external resource that is unavailable, it must fail with a clear error rather than attempting a fallback to insecure or public endpoints.
