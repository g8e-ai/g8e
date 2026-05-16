---
title: Air Gap
parent: Architecture
---

# Air-Gap Architecture

Last Updated: 2026-05-12
Version: v0.2.5

g8e is designed for high-security environments where internet connectivity is strictly prohibited. The platform supports fully air-gapped deployments with **zero runtime internet dependencies**, achieving this through a self-contained **Substrate** (Operator + Protocol), vendored dependencies, and optional local LLM inference.

---

## Zero-Trust Privacy Principle

The air-gap configuration is the "Canonical Truth" of g8e's privacy model. In this mode, the platform operates as a completely sealed unit:

- **No Telemetry:** Zero outbound usage, health, or error data is sent to Lateralus Labs.
- **Local Assets:** All frontend assets (fonts, icons, JS libraries) are served locally by the application layer.
- **Local Persistence:** All platform state, including chat history, settings, and secrets, is stored in a unified SQLite database managed by the reference Operator (`g8eo`) in Listen Mode.

---

## The Platform Backbone: Operator (Listen Mode)

In an air-gapped deployment, the platform requires a local "Hub" for persistence and messaging. This is provided by running the `g8eo` (Operator) binary in **Listen Mode** (`--listen`). In this mode, the Operator acts as the platform's central persistence and messaging backbone rather than an outbound execution agent.

### Architecture & Ports
The reference Operator in Listen Mode exposes interfaces for public protocol communication from optional reference adapters (Dashboard and Engine) and BYO clients. While the binary defaults to port 443, the standard platform deployment maps these to:

| Port | Protocol | Purpose |
|---|---|---|
| **9000** | **HTTPS** | **Substrate API:** Document Store, Vault, and Protocol endpoints. |
| **9001** | **WSS** | **Pub/Sub Broker:** Real-time messaging backbone for all platform events. |

### Core Responsibilities
- **Unified Persistence:** Replaces external databases with a single `g8e.db` SQLite file in `.g8e/data`.
- **Internal PKI:** Acts as the platform's Certificate Authority (CA), auto-generating ECDSA P-384 TLS certificates for all inter-component traffic.
- **Secret Management:** Provides an encrypted Vault for storing platform secrets (API keys, internal tokens) without external dependencies.
- **Messaging:** Serves as the central Pub/Sub broker for all compliant clients.

---

## Local LLM Inference

For air-gapped reasoning, g8e integrates a local inference engine within the `g8ee` (Engine) component.

- **Engine:** `llama.cpp` integration via `LlamaCppProvider`.
- **Default Model:** `Gemma 4 E2B` (optimized for local reasoning).
- **Interface:** OpenAI-compatible internal API.
- **Provisioning:** Model GGUF files must be pre-staged in `services/g8ee/models/`. If the file is missing, the Engine will attempt a download, which will fail in a true air-gap.

---

## Build-Time vs. Runtime

| Phase | Internet Requirement | Air-Gap Strategy |
|---|---|---|
| **Build** | Required (Default) | Use the `setup` workflow on a connected machine to cache all base images and vendor dependencies. |
| **Runtime** | **None** | All components communicate exclusively over the internal `g8e-network` or localhost. |

### Vendoring & Dependency Management
- **Operator (Go):** 100% vendored in `services/g8eo/vendor/`.
- **Dashboard (Node.js):** Locked via `package-lock.json`; all assets are bundled during the build.
- **Engine (Python):** Requirements are frozen. For air-gap builds, use the pre-staged environment or Docker image strategy.

---

## Deployment Workflow

### 1. Preparation (Connected Environment)
1. **Bootstrap Platform:** Run `./g8e platform setup` on a connected machine to cache dependencies and build binaries.
2. **Download Model:** Obtain the `Gemma 4 E2B` GGUF model file.
3. **Export Assets:** Bundle the `.g8e` runtime directory and component binaries.

### 2. Implementation (Air-Gapped Host)
1. **Stage Binaries:** Place the `g8e.operator` binary and component source/images on the host.
2. **Stage Model:** Place the `.gguf` file in `services/g8ee/models/`.
3. **Configure:**
   - Set `vertex_search_enabled` to `false` in Settings.
   - Ensure `llm_primary_provider` is set to `llamacpp`.
4. **Launch:** `./g8e platform up`

---

## Security Invariants

1. **No Outbound Dialing:** In Listen Mode, the Operator is forbidden from initiating connections to any address outside the local platform.
2. **Mutual Trust:** All internal traffic between Dashboard, Engine, and Operator is encrypted using the Operator's internal CA.
3. **Data Sovereignty:** All audit logs, chat history, and telemetry remain strictly on the host's filesystem in the `.g8e` directory.
4. **Fail-Closed Privacy:** If a component requires an external resource that is unavailable, it must fail with a clear error rather than attempting a fallback to insecure or public endpoints.
