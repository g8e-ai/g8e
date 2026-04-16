# Air-Gap Architecture

g8e is designed to run with zero cloud dependencies at **runtime** when configured with a local LLM provider. This document catalogs all external dependencies, categorizes them by lifecycle phase, and describes the air-gap deployment path.

> **Current state — not yet a fully offline build.** Container images must be built on an internet-connected machine today. The `docker build` pipeline still fetches from Docker Hub (base images), PyPI (`g8ee` and `g8ep` Python packages), npmjs (`npm ci` for `g8ed`), and the Alpine/Debian package mirrors. See the **Vendoring Status** table below for what has been vendored and what has not. The intended air-gap deployment flow is: build images on a connected host, then `docker save` / `docker load` onto the target.

---

## Runtime Dependencies (Zero External)

The platform has **no runtime internet dependencies** when configured correctly:

- **LLM Provider:** Use Ollama (local) or any OpenAI-compatible local endpoint (vLLM, LM Studio, text-generation-webui). Cloud providers (Gemini, OpenAI, Anthropic) are available but only active when explicitly configured.
- **Web Search Grounding:** Disabled by default. Uses Google Discovery Engine when enabled — leave disabled for air-gap.
- **Frontend Assets:** All fonts, icons, CSS, and vendor JS (Mermaid, highlight.js) are self-hosted. No CDN references.
- **TLS:** Self-signed CA generated at startup by g8es. No external certificate authority dependency.

---

## Build-Time Dependencies

All external fetches occur exclusively at `docker build` time. Once images are built, no internet access is required.

### Docker Base Images

| Image | Used By |
|---|---|
| `node:22-alpine3.23` | g8ed, g8ed-test-runner |
| `python:3.13-slim` | g8ee, g8ee-test-runner |
| `python:3.13-alpine` | g8ep |
| `golang:1.26-alpine3.23` | g8es builder stage, g8eo-test-runner |
| `alpine:3.23` | g8es final stage |

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
- `node_modules` and `package-lock.json` are committed to the repo (partially air-gap ready)
- The builder stage runs `npm ci` to install from the lockfile

**Air-gap path:** Use the committed lockfile with a local npm registry (Verdaccio), or pre-populate `node_modules` in the build context.

**Go:**
- `components/g8eo/vendor/` — all operator dependencies vendored, builds use `-mod=vendor` (air-gap ready)
- `components/g8eo/tools/vendor/` — gotestsum and its dependencies vendored for test runner (air-gap ready)

---

## Vendoring Status

| Component | Package Manager | Vendored | Notes |
|---|---|---|---|
| g8eo (operator) | Go modules | Yes | `vendor/` directory, `-mod=vendor` builds |
| g8eo (test tools) | Go modules | Yes | `tools/vendor/` directory, gotestsum built from source |
| g8ed (dashboard) | npm | Partial | `package-lock.json` committed; `npm ci` still fetches from registry |
| g8ee (engine) | pip | No | `requirements.txt` fetches from PyPI at build time |
| g8ep (node) | pip | No | Two packages installed at build time |

---

## Eliminated Internet Fetches

The following internet-dependent patterns have been removed from the build:

1. **g8ed/Dockerfile** — Removed `curl -fsSL https://www.npmjs.com/install.sh | sh` npm upgrade from the final stage. npm is only used in the builder stage; the runtime image runs `node server.js` directly.
2. **g8ed/Dockerfile.test** — Same npm upgrade pattern removed.
3. **g8eo/Dockerfile.test** — Replaced `go install gotest.tools/gotestsum@v1.12.1` (fetches from proxy.golang.org) with a vendored build from `components/g8eo/tools/`.

---

## UI Links to External Sites

These are informational `<a href>` links in the dashboard, not functional dependencies. They will be dead links in an air-gapped environment but do not affect platform operation:

- GitHub repository links in navigation menus
- Google AI Studio link in the setup wizard
- License and contact URLs in settings constants

---

## Air-Gap Deployment Checklist

1. **Build images on an internet-connected machine** using `./g8e platform setup`
2. **Export images:** `docker save g8es g8ee g8ed g8ep | gzip > g8e-images.tar.gz`
3. **Transfer** the archive to the air-gapped host
4. **Load images:** `docker load < g8e-images.tar.gz`
5. **Configure LLM provider** to use Ollama or an OpenAI-compatible local endpoint
6. **Disable web search grounding** in user settings
7. **Start the platform:** `./g8e platform start`
