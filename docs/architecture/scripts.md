---
doc_id: scripts
title: Automation Scripts
audience: maintainers and coding agents
status: current
last_updated: 2026-09-28
version: v2.2.3
owners:
  - scripts/
  - internal/services/gateway/scripts/
  - demos/images.json
related:
  - docs/devs/docs.md
  - docs/devs/devs.md
  - docs/guides/getting_started.md
  - docs/guides/build_operator.md
  - docs/guides/air_gap.md
when_to_read: Understanding the automation under scripts/, the Gateway's embedded deploy templates, the air-gap image-transfer workflow, or the validation toolchain.
do_not_use_for:
  - Developer coding invariants (docs/devs/devs.md)
  - Getting-started user steps (docs/guides/getting_started.md)
  - Operator enrollment and configuration (docs/guides/build_operator.md)
---

# Automation Scripts

## Purpose

Catalogs the executable automation under `scripts/`, the deploy-script templates embedded in the Gateway, the validation toolchain, and the script-backed `g8e demos` image-transfer workflow. The root `Makefile` owns build and validation orchestration.

## Quick index

- [Purpose](#purpose)
- [Invariants](#invariants)
- [Owned surfaces](#owned-surfaces)
- [Procedures](#procedures)
- [Anti-patterns](#anti-patterns)
- [Links out](#links-out)

Entry points: [Development bootstrap](#procedures) (`linux-setup.sh`, `macos-setup.sh`, `windows-setup.ps1`), [Smoke tests](#procedures) (`smoke-test-go.sh`, `smoke-test-python.sh`), [Validation](#procedures) (`make validate-cosais`, `audit-dev-guidelines.py`), [Gateway deploy](#procedures) (`g8e-deploy.sh`, `g8e-deploy.ps1`), [Image transfer](#procedures) (`g8e demos pull/export/import/images`).

## Invariants

| ID | Rule |
| --- | --- |
| INV-SCRIPTS-01 | Setup scripts MUST check for required dependencies (git, make, go, node, npm) before building. Go version MUST be read from `go.mod`. Node.js 22+ is required for `make build`. |
| INV-SCRIPTS-02 | Setup scripts on Linux and macOS MUST update shell profiles (`~/.zshrc`, `~/.bashrc`, or `~/.profile`) for persistent PATH modification. Windows scripts MUST update user-level `Path` through the registry. These are environment mutations; invoking users MUST open a new shell or source the profile before `g8e` is available. |
| INV-SCRIPTS-03 | Validator scripts (`cosais_validator`, `audit-dev-guidelines.py`) are triage tools; they identify review candidates but do not prove violations or replace semantic lint and test commands. |
| INV-SCRIPTS-04 | Gateway-served deploy scripts (`g8e-deploy.sh`, `g8e-deploy.ps1`) are bootstrap conveniences for trusted networks only. They fetch artifacts over plain HTTP, lack TLS protection, and delete `~/.g8e/pki` before startup. Users MUST fetch and inspect the rendered script before execution. |
| INV-SCRIPTS-05 | Image transfer commands (`g8e demos pull/export/import/images`) require reading `demos/images.json` relative to the current working directory and MUST be run from the repository root. Export MUST NOT include locally built images; import MUST NOT validate tars against the manifest. |

## Owned surfaces

| Claim | Path | Verify |
| --- | --- | --- |
| Linux development bootstrap | `scripts/linux-setup.sh` | Dependency checks, build prerequisites, shell profile updates |
| macOS development bootstrap | `scripts/macos-setup.sh` | Dependency checks, build prerequisites, shell profile updates |
| Windows development bootstrap | `scripts/windows-setup.ps1` | PowerShell 7+, dependency checks, build prerequisites, user PATH updates |
| Go package smoke test | `scripts/smoke-test-go.sh` | Temporary module creation, `replace` directive, import verification |
| Python package smoke test | `scripts/smoke-test-python.sh` | Virtual environment, editable install, import and example verification |
| COSAiS validation | `internal/tools/cosais_validator` | Finalized overlay ID coverage check; invoked by `make validate-cosais` |
| Developer-guideline audit | `scripts/audit-dev-guidelines.py` | Static pattern matching against `docs/devs/devs.md` rules |
| Gateway-embedded deploy (Bash) | `internal/services/gateway/scripts/g8e-deploy.sh` | Embedded by `make build`; served at `/g8e-deploy.sh` |
| Gateway-embedded deploy (PowerShell) | `internal/services/gateway/scripts/g8e-deploy.ps1` | Embedded by `make build`; served at `/g8e-deploy.ps1` |
| Gateway binary download manifest | `g8e-binaries.json` (validated at build time) | Validated platform matrix: Linux amd64/arm64/386, Windows amd64/arm64, Darwin amd64/arm64 |
| Image transfer manifest | `demos/images.json` | Digest-pinned external base and service images used across demo environments |

## Procedures

### Development Bootstrap

Run the platform setup script from the repository root. The root `Makefile` must be visible to the invoked `make build`:

```bash
bash scripts/linux-setup.sh
bash scripts/macos-setup.sh
pwsh scripts/windows-setup.ps1
```

Each setup script:

1. Checks for `git`, `make`, `go`, `node`, and `npm`. Required Go version is read from `go.mod` (currently 1.26.6). Node.js 22+ is required because `make build` embeds the evaluation-explorer frontend.
2. If a dependency is missing or too old, prompts before invoking a supported package manager. Linux prefers `snap` for Go and Node when available; macOS uses Homebrew; Windows prefers `winget` with Chocolatey fallback. Pass `-y` or set `G8E_SETUP_YES=1` for non-interactive installs.
3. Builds the evaluation-explorer asset when `dashboard/g8e-adapter/evaluation-explorer/dist/index.html` is absent, then runs `make build`. The build writes the platform binary and SHA-256 sidecar under `bin/` and copies the host executable to the repository root.
4. Updates the user PATH persistently: Linux and macOS append the repository root to `~/.zshrc`, `~/.bashrc`, or `~/.profile` based on detected shell; Windows updates user-level `Path`. The scripts also export PATH in the current shell, but that change is lost when the shell exits—users MUST open a new terminal or source the profile before `g8e` is available in other shells.

After setup, scripts print next steps: `./g8e --version`, `g8e docker start --full`, `g8e gw start`, and `g8e auth enroll user -e localhost`.

`windows-setup.ps1` requires PowerShell 7+ (`pwsh`). It is not a substitute for WSL when native Windows build tooling is incomplete; use Docker quick-start when local compiler toolchain is unavailable.

### Smoke Tests

Run from any directory:

```bash
bash scripts/smoke-test-go.sh
bash scripts/smoke-test-python.sh
```

`smoke-test-go.sh` creates a temporary Go module, adds a `replace` directive pointing to the local repository, imports `github.com/g8e-ai/g8e/v2/protocol`, runs `go mod tidy`, and builds a minimal executable. The exit trap removes the temporary module on success or failure. The test accesses configured Go module networks while resolving dependencies.

`smoke-test-python.sh` creates `protocol/python/.smoke-env`, upgrades `pip`, installs `protocol/python` in editable mode, checks public constants and models imports, runs `constants_example.py` and `models_example.py`, then removes the virtual environment on success. Failed commands can leave `.smoke-env` behind for manual cleanup. The test uses package indexes while upgrading `pip` and resolving dependencies.

The primary CI workflow (`build-and-test.yml`) runs both scripts for pushes to `main`, pull requests targeting `main`, and manual workflow dispatches. These tests validate the local working tree rather than published distributions.

### Validation and Audit

#### COSAiS Overlay Coverage

Run through its Make target:

```bash
make validate-cosais
```

`go run ./internal/tools/cosais_validator` requires `docs/reference/cosais-overlays.json` and at least one `demos/*/doctrine/` directory. It reads overlays with `status` exactly matching `finalized`, collects `overlay_ids` from top-level `doctrines` arrays in every JSON file directly under each demo doctrine directory, and fails if a finalized overlay ID is absent from that collected set. When the catalog contains no finalized overlays, it exits successfully and reports that no coverage check is active. The validator does not query NIST or determine finalization independently.

`make lint` includes `make validate-cosais`, and the primary CI workflow invokes it directly. Schema and broader doctrine-reference validation are owned by `make validate-doctrines`.

#### Developer-Guideline Audit

Run from the repository root:

```bash
python3 scripts/audit-dev-guidelines.py
```

Ranks files by statically detectable patterns in `docs/devs/devs.md`. Default output shows the 25 files with the most findings, including matching lines and rule identifiers. Use `--limit 0` to print all files, `--min-violations N` to focus on high-count files, `--include-generated` to include generated and Swagger/OpenAPI paths, or `--json` for machine-readable output. This is an intentionally heuristic tool: it identifies review candidates and does not prove violations or replace semantic lint and test commands.

### Gateway-Embedded Operator Bootstrap

The Gateway embeds `internal/services/gateway/scripts/g8e-deploy.sh` and `g8e-deploy.ps1` at build time. Gateway HTTP handler construction parses both templates and fails if initialization fails. The plain HTTP router exposes these unauthenticated GET routes on the configured Gateway HTTP port (default 8080):

| Route | Content | Authentication |
| --- | --- | --- |
| `/g8e-deploy.sh` | Rendered Bash script for Linux and macOS | None |
| `/g8e-deploy.ps1` | Rendered PowerShell script for Windows | None |
| `/.well-known/g8e/bin/{filename}` | Pre-built platform executable | None |

The binary route accepts only names listed in the validated `g8e-binaries.json` manifest. It serves from `/opt/g8e/bin` (image-baked) or, for source-built Gateway, a validated `bin/` directory beside the running executable. The standard matrix is Linux amd64/arm64/386, Windows amd64/arm64, Darwin amd64/arm64. Unsupported names and unvalidated roots fail closed.

Rendered scripts derive Gateway host and HTTP port from the request and configuration. The Windows handler prefers `X-Forwarded-Host`; both templates allow `GATEWAY_HOST` and `GATEWAY_PORT` environment variables to override rendered values at execution time.

On the target host, the script:

1. Recursively removes `~/.g8e/pki` before validating the platform or downloading a binary.
2. Detects operating system and architecture, downloads the selected executable over plain HTTP into the current directory as `g8e` or `g8e.exe`, replaces any existing file with that name.
3. Starts `g8e operator start -e <gateway-host>` in the foreground. When no Operator credentials remain, startup enters the owner-approved enrollment flow in [Build and Run a g8e Operator](../guides/build_operator.md).

Scripts fetch the matching `.sha256` sidecar and verify the downloaded temporary file before replacing the local executable. They do not verify a signature or protect downloads with TLS. The routes are bootstrap conveniences for trusted networks, not authenticated software-distribution boundaries. Fetch and inspect the rendered script before execution; do not run it on a host whose `.g8e/pki` state must be retained.

After the Gateway is reachable and required binaries are present, direct execution forms are:

```bash
curl -fsSL http://<gateway-host>:8080/g8e-deploy.sh | bash
```

```powershell
iwr http://<gateway-host>:8080/g8e-deploy.ps1 -UseBasicParsing | iex
```

Linux and macOS require `curl` or `wget`; Windows requires PowerShell. Standard Gateway container images ship only the runtime binary at `/g8e`; the `/.well-known/g8e/bin/` download surface is available when a validated `bin/` mirror exists beside a source-built Gateway or when `/opt/g8e/bin` contains a full manifest from a custom image build. Build the complete matrix with `make build-all`.

### Air-Gap Image Transfer

The `g8e demos` image commands read `demos/images.json` relative to the current working directory; run them from the repository root. Commands `g8e demos pull`, `export`, and `import` require the Docker CLI and a reachable Docker daemon; `g8e demos images` only reads the manifest.

- `g8e demos images` prints each manifest image, digest, informational tag, and associated demos.
- `g8e demos pull` pulls every entry as `<image>@<digest>`.
- `g8e demos export [output-dir]` saves each manifest image to a separate tar file and skips a tar path that already exists. Default directory is `demos/images-export/`.
- `g8e demos import [input-dir]` loads every `.tar` file directly under the selected directory. Default directory is `demos/images-export/`.

The manifest covers digest-pinned external base and service images. Export does not include locally built Gateway, Operator, demo, Ensemble, or Dashboard images; import does not validate tars against `demos/images.json`. Follow [Air-Gapped Deployment Guide](../guides/air_gap.md) for source staging, locally built image transfer, offline checks, and runtime egress verification. The [Demos README](../../demos/README.md) owns per-demo topology and operating workflow.

## Anti-patterns

- Invoking `g8e` immediately after setup without opening a new shell or sourcing the profile; PATH updates are not visible to the current shell (INV-SCRIPTS-02).
- Running setup scripts from a different working directory; `make build` will use the wrong Makefile (INV-SCRIPTS-01).
- Interpreting validator and audit script findings as definitive proof without semantic verification (INV-SCRIPTS-03).
- Running Gateway deploy scripts on hosts whose `.g8e/pki` state must be retained (INV-SCRIPTS-04).
- Running `g8e demos` commands from a directory other than the repository root; the tool will not find `demos/images.json` (INV-SCRIPTS-05).
- Assuming exported images or tars are validated against the manifest; validation is the caller's responsibility (INV-SCRIPTS-05).

## Links out

- [Developer Guidelines](devs.md): coding invariants and repository standards.
- [Documentation Guide](../devs/docs.md): how to audit, write, and review documentation.
- [Getting Started](../guides/getting_started.md): user-facing setup and first-run steps.
- [Build and Run a g8e Operator](../guides/build_operator.md): Operator enrollment and configuration.
- [Air-Gapped Deployment](../guides/air_gap.md): offline source staging and image transfer.
- [Demo Environments](../../demos/README.md): per-demo topology and operating workflow.
