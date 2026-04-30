---
title: Air Gap
parent: Architecture
---

# Air-Gap Architecture

g8e supports fully air-gapped deployments with **zero runtime internet dependencies**. The platform achieves this by self-hosting all core infrastructure, vendoring critical dependencies, and providing a local inference server for LLMs.

---

## Zero-Trust Privacy Principle

The air-gap configuration is the "Canonical Truth" of g8e's privacy model. In this mode, the platform operates as a completely sealed unit:
- **No Telemetry:** No outbound usage or health data is sent to Lateralus Labs.
- **No CDNs:** All frontend assets (fonts, icons, JS libraries) are served from the local `g8ed` container.
- **Local Persistence:** Data is stored in a local SQLite database managed by the `g8es` service.
- **Local Intelligence:** LLM inference is handled by `g8el` or a local Ollama instance.

---

## Runtime Backbone: g8es

In air-gapped environments, the standard cloud-backed `g8es` is replaced by the **Operator Listen Mode**. The `g8eo` binary is launched with the `--listen` flag, assuming the role of the platform's central nervous system.

### Responsibilities
- **Persistence:** Provides a high-performance SQLite-backed KV store for `g8ee` and `g8ed`.
- **Messaging:** Acts as the Pub/Sub broker (WSS on port 9001) for all component communication.
- **Security:** Serves as the internal Certificate Authority (CA), auto-generating self-signed TLS certificates for all inter-container traffic.
- **Auth Proxy:** Handles internal authentication between services on port 9000.

---

## Local Inference: g8el

The `g8el` service provides a local `llama.cpp` inference server. It is the recommended LLM provider for air-gapped deployments.

- **Port:** `11444`
- **Default Model:** `google_gemma-4-E2B-it-Q4_K_M.gguf`
- **Air-Gap Requirement:** Model files must be pre-downloaded and placed in the `components/g8ee/models/` directory on the host, which is mapped to `/models` inside the container.
- **Entrypoint Logic:** If the model file is missing at startup, the container will attempt to download it from HuggingFace. In an air-gapped environment, this will fail unless the model is pre-staged.

---

## Build-Time Dependencies

While runtime has no dependencies, the build process (`docker build`) requires internet access or a local mirror.

### Docker Base Images

| Image | Component | Purpose |
|---|---|---|
| `golang:1.26-alpine3.23` | `g8es` (builder) | Compiling the Operator binary |
| `alpine:3.23` | `g8es` (final) | Runtime environment for the backbone |
| `node:22-alpine3.23` | `g8ed` | Frontend and Terminal backend |
| `python:3.12-slim` | `g8ee` | AI Engine and reasoning logic |
| `python:3.13-alpine` | `g8ep` | Node host and test runner |
| `ghcr.io/ggml-org/llama.cpp:server` | `g8el` | Local LLM inference |

### Language Vendoring

- **Go (g8eo):** 100% vendored in `components/g8eo/vendor/`. Builds use `-mod=vendor`.
- **Node.js (g8ed):** `package-lock.json` is committed. The build uses `npm ci` for deterministic installs.
- **Python (g8ee/g8ep):** Uses standard `pip` requirements. For air-gap builds, use a local PyPI mirror or pre-download wheels into the build context.

---

## Vault & Secret Management

Secrets in an air-gapped environment (API keys, session tokens, HMAC keys) are managed by the `g8eo` SecretManager and stored in the local Vault.

### Offline Vault Operations
The Operator provides a CLI for managing the encrypted vault without internet access:
- **Rekey:** `g8e.operator --rekey-vault --old-key <old> -k <new>`
- **Verify:** `g8e.operator --verify-vault -k <key>`
- **Reset:** `g8e.operator --reset-vault` (Destructive)

---

## Deployment Workflow

### 1. Build & Stage
On an internet-connected machine:
1. Build all images: `./g8e platform setup`
2. Download the default model: `google_gemma-4-E2B-it-Q4_K_M.gguf`
3. Export images: `docker save g8es g8ee g8ed g8ep g8el | gzip > g8e-images.tar.gz`

### 2. Transfer
Move `g8e-images.tar.gz` and the model file to the air-gapped host via physical media or secure transfer.

### 3. Deploy
On the air-gapped host:
1. Load images: `docker load < g8e-images.tar.gz`
2. Place the model file in `components/g8ee/models/`
3. Configure `LLMProvider.G8EL` as the primary provider in settings.
4. Ensure `SearchSettings.enabled` is `false` (default).
5. Start the platform: `./g8e platform start`
