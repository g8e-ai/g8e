---
title: Air Gap
parent: Architecture
---

# Air-Gap Architecture

g8e supports air-gapped deployment with zero runtime internet dependencies when configured with a local LLM provider. Container images must be built on an internet-connected machine, then transferred to the air-gapped environment.

---

## Runtime Dependencies

The platform has **no runtime internet dependencies** when configured correctly:

- **LLM Provider:** Use Ollama (local), llama.cpp (g8el), or any OpenAI-compatible local endpoint (vLLM, LM Studio, text-generation-webui). Cloud providers (Gemini, OpenAI, Anthropic) are available but only active when explicitly configured.
- **Web Search Grounding:** Disabled by default. Uses Google Discovery Engine when enabled — leave disabled for air-gap. Note: the web search provider includes a hardcoded Google favicon URL (`www.google.com/s2/favicons`) for source display. This only triggers when web search is actively used.
- **Frontend Assets:** All fonts, icons, CSS, and vendor JS (markdown-it, highlight.js) are self-hosted. No CDN references.
- **TLS:** Self-signed CA generated at startup by g8es (operator in listen mode). No external certificate authority dependency.

---

## Build-Time Dependencies

All external fetches occur exclusively at `docker build` time. Once images are built, no internet access is required.

### Docker Base Images

| Image | Used By |
|---|---|
| `node:22-alpine3.23` | g8ed, g8ed-test-runner |
| `python:3.12-slim` | g8ee, g8ee-test-runner |
| `python:3.13-alpine` | g8ep |
| `golang:1.26-alpine3.23` | g8es builder stage, g8eo-test-runner |
| `alpine:3.23` | g8es final stage |
| `ghcr.io/ggml-org/llama.cpp:server` | g8el (llama.cpp inference server) |

**Air-gap path:** Pre-pull images on an internet-connected machine, transfer via `docker save` / `docker load` or push to a local registry (Harbor, registry:2).

### OS Packages

- **apk (Alpine):** `curl`, `ca-certificates`, `bash`, `openssl`, `jq`, `git`, `make`, `gcc`, `musl-dev`, `upx`, and network tools (g8ep only)
- **apt-get (Debian slim):** `curl`, `procps`, `net-tools`, `ca-certificates`, `openssl`, `jq`

**Air-gap path:** Pre-bake packages into custom base images, or configure a local APK/APT mirror.

### Language Package Managers

**Python (pip):**
- `components/g8ee/requirements.txt` — production and test dependencies from PyPI
- `components/g8ep/Dockerfile` — `pip install requests aiohttp`

**Air-gap path:** Bundle a vendored wheelhouse (`pip download`) or use a local PyPI mirror (devpi, bandersnatch).

**Node.js (npm):**
- `components/g8ed/package.json` — runtime dependencies from npmjs.org
- `node_modules` and `package-lock.json` are committed to the repo
- The builder stage runs `npm ci` to install from the lockfile

**Air-gap path:** Use the committed lockfile with a local npm registry (Verdaccio), or pre-populate `node_modules` in the build context.

**Go:**
- `components/g8eo/vendor/` — all operator dependencies vendored, builds use `-mod=vendor`
- `components/g8eo/tools/vendor/` — gotestsum and its dependencies vendored for test runner

---

## Vendoring State

| Component | Package Manager | Vendored |
|---|---|---|
| g8eo (operator) | Go modules | Yes |
| g8eo (test tools) | Go modules | Yes |
| g8ed (terminal) | npm | Partial — lockfile committed |
| g8ee (engine) | pip | No |
| g8ep (node) | pip | No |
| g8el (llama.cpp) | External image | N/A — uses pre-built image |

---

## External UI Links

The terminal contains informational links to external sites. These are not functional dependencies and will be dead links in an air-gapped environment:

- GitHub repository links in navigation menus
- Google AI Studio link in the setup wizard
- License and contact URLs in settings constants

---

## Air-Gap Deployment

### Standard Deployment

1. **Build images on an internet-connected machine** using `./g8e platform setup`
2. **Export images:** `docker save g8es g8ee g8ed g8ep g8el | gzip > g8e-images.tar.gz`
3. **Transfer** the archive to the air-gapped host
4. **Load images:** `docker load < g8e-images.tar.gz`
5. **Configure LLM provider** to use Ollama, llama.cpp (g8el), or an OpenAI-compatible local endpoint
6. **For g8el (llama.cpp):** Pre-download model files from HuggingFace on an internet-connected machine and transfer to `/components/g8ee/models/` on the air-gapped host
7. **Disable web search grounding** in user settings
8. **Start the platform:** `./g8e platform start`

### Fully Offline Build

For environments that cannot run `docker build` with internet access, set up the following infrastructure:

1. **Local container registry** (Harbor, registry:2) — pre-populate with base images
2. **Local PyPI mirror** (devpi, bandersnatch) — host Python packages
3. **Local npm registry** (Verdaccio) — host Node.js packages
4. **Local APK/APT mirror** — host OS packages
5. **Configure Dockerfile build args** to point to local mirrors

Once local mirrors are configured, build images on an air-gapped machine and deploy normally.
