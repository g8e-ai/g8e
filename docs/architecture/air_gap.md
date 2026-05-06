---
title: Air Gap
parent: Architecture
---

# Air-Gap Architecture

Last Updated: 5-6-2026
Version: v.0.2.0

g8e is designed for high-security environments where internet connectivity is strictly prohibited. The platform supports fully air-gapped deployments with **zero runtime internet dependencies**, achieving this through self-hosted core infrastructure, vendored dependencies, and local LLM inference.

---

## Zero-Trust Privacy Principle

The air-gap configuration is the "Canonical Truth" of g8e's privacy model. In this mode, the platform operates as a completely sealed unit:

- **No Telemetry:** Zero outbound usage, health, or error data is sent to Lateralus Labs.
- **Local Assets:** All frontend assets (fonts, icons, JS libraries) are served locally from the `g8ed` container.
- **Local Persistence:** All platform state is stored in a unified SQLite database managed by the `g8es` (Operator listen mode) service.
- **Local Intelligence:** LLM inference is handled by the `g8el` component (a self-hosted `llama.cpp` server).

---

## The Platform Backbone: g8es (Listen Mode)

In an air-gapped deployment, the platform requires a local "Hub" for persistence and messaging. This is provided by running the `g8eo` (Operator) binary in **Listen Mode**, which the platform refers to as `g8es`.

### Architecture & Ports
The `g8es` backbone exposes two primary interfaces for internal component communication:

| Port | Protocol | Purpose |
|---|---|---|
| **9000** | **HTTPS** | **Document Store & Vault:** Unified SQLite-backed API for `g8ee` and `g8ed`. |
| **9001** | **WSS** | **Pub/Sub Broker:** Real-time messaging backbone for all platform events. |

### Core Responsibilities
- **Unified Persistence:** Replaces external databases with a single `g8e.db` SQLite file.
- **Internal PKI:** Acts as the platform's Certificate Authority (CA), auto-generating TLS certificates for all inter-container traffic.
- **Secret Management:** Provides an encrypted Vault for storing platform secrets (API keys, tokens) without external dependencies.
- **Blob Storage:** Hosts local binaries (like the Operator itself) for deployment to other air-gapped nodes.

---

## Local Inference: g8el

The `g8el` component provides a local `llama.cpp` inference server, enabling agentic operations without cloud LLM providers.

- **Interface:** OpenAI-compatible HTTP API on port `11444`.
- **Default Model:** `google_gemma-4-E2B-it-Q4_K_M.gguf` (2.6B parameter model).
- **Thinking Support:** Inherits `LlamaCppProvider` logic, supporting local reasoning models when configured.
- **Provisioning:** Model files must be pre-staged in `components/g8ee/models/` (mapped to `/models` in the container). If the file is missing, the container will attempt a download, which will fail in a true air-gap.

---

## Build-Time vs. Runtime

| Phase | Internet Requirement | Air-Gap Strategy |
|---|---|---|
| **Build** | Required (Default) | Use the `setup` workflow on a connected machine to cache all base images and vendor dependencies. |
| **Runtime** | **None** | All services communicate exclusively over the internal `g8e-network`. |

### Vendoring & Dependency Management
- **Go (`g8eo`):** 100% vendored in `components/g8eo/vendor/`.
- **Node.js (`g8ed`):** Locked via `package-lock.json`; all assets are bundled during the build.
- **Python (`g8ee`):** Requirements are frozen. For air-gap builds, use the pre-staged Docker image strategy.

---

## Deployment Workflow

### 1. Preparation (Connected Environment)
1. Build the platform: `./g8e platform setup`
2. Download the required model file: `google_gemma-4-E2B-it-Q4_K_M.gguf`
3. Export the platform images: `docker save g8es g8ee g8ed g8ep g8el | gzip > g8e-airgap.tar.gz`

### 2. Implementation (Air-Gapped Host)
1. Load the images: `docker load < g8e-airgap.tar.gz`
2. Stage the model: Place the `.gguf` file in `components/g8ee/models/`.
3. Configure the platform:
   - Set `primary_provider` to `g8el`.
   - Ensure `search.enabled` is `false`.
4. Launch: `./g8e platform start`

---

## Security Invariants

1. **No External Dialing:** In air-gap mode, components are forbidden from initiating connections to any address outside the Docker network.
2. **TLS Mandatory:** All internal traffic between `g8ed`, `g8ee`, and `g8es` is encrypted using the `g8es` internal CA.
3. **Data Sovereignty:** All audit logs, chat history, and telemetry remain strictly on the host's filesystem.
