---
doc_id: scripts
title: Automation Scripts
audience: maintainers and coding agents
status: current
last_updated: 2026-10-02
version: v2.3.0
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

Entry points: [Development bootstrap](#procedures) (`linux-setup.sh`, `macos-setup.sh`, `windows-setup.ps1`), [Smoke tests](#procedures) (`smoke-test-go.sh`, `smoke-test-python.sh`), [Validation](#procedures) (`make cosais-validate`, `audit-dev-guidelines.py`), [Gateway deploy](#procedures) (`g8e-deploy.sh`, `g8e-deploy.ps1`), [Image transfer](#procedures) (`g8e demos pull/export/import/images`).

## Invariants

| ID | Rule |
| --- | --- |
| INV-SCRIPTS-01 | Setup scripts MUST check for required dependencies (git, make, curl, go, node, npm) before building. Go version MUST be read from `go.mod` and compared including the patch component, so Go 1.26.0 does not satisfy a `go 1.26.6` directive. Node.js 22+ is required for `make build`. Every other tool pin (uv, Python, buf, protoc plugins, golangci-lint) MUST be read from the root `Makefile`, which is its single source of truth; setup scripts MUST NOT hardcode a second copy. |
| INV-SCRIPTS-02 | Setup scripts on Linux and macOS MUST update shell profiles (`~/.zshrc`, `~/.bashrc`, or `~/.profile`) for persistent PATH modification. Windows scripts MUST update user-level `Path` through the registry. These are environment mutations; invoking users MUST open a new shell or source the profile before `g8e` is available. |
| INV-SCRIPTS-03 | Validator scripts (`cosais_validator`, `audit-dev-guidelines.py`) are triage tools; they identify review candidates but do not prove violations or replace semantic lint and test commands. |
| INV-SCRIPTS-04 | Gateway-served deploy scripts (`g8e-deploy.sh`, `g8e-deploy.ps1`) are bootstrap conveniences for trusted networks only. They fetch artifacts over plain HTTP, lack TLS protection, and delete `~/.g8e/pki` before startup. Users MUST fetch and inspect the rendered script before execution. |
| INV-SCRIPTS-05 | Image transfer commands (`g8e demos pull/export/import/images`) require reading `demos/images.json` relative to the current working directory and MUST be run from the repository root. Export MUST NOT include locally built images; import MUST NOT validate tars against the manifest. |
| INV-SCRIPTS-06 | Unless `--build-only` is passed, the Linux and macOS setup scripts MUST leave a machine that passes `make dev-check`: they check or install `python3`, `uv`, `rg` (ripgrep), `bc`, and a C compiler (required by `go test -race`), then run `make dev-setup` for the Go dev tools, the repo-root `.venv`, and the Node dependencies. `make ci`, `make ci-platform`, `make ci-ensemble`, and `make ci-console` depend on `make dev-check`, so a missing tool fails once with an actionable list instead of partway through the pipeline. |
| INV-SCRIPTS-07 | A prerequisite that resolves under `/mnt/<drive>/` MUST NOT count as installed. On WSL the Windows `PATH` is appended to the Linux one and exposes Windows shims such as `npm` and `npx` that cannot build or test this repository. |

## Owned surfaces

| Claim | Path | Verify |
| --- | --- | --- |
| Linux development bootstrap | `scripts/linux-setup.sh` | Dependency checks, OS package, Go, Node.js, and uv installs, contributor toolchain, shell profile updates |
| macOS development bootstrap | `scripts/macos-setup.sh` | Dependency checks, Homebrew and uv installs, contributor toolchain, shell profile updates |
| Windows development bootstrap | `scripts/windows-setup.ps1` | PowerShell 7+, build-toolchain checks (git, make, go, node), user PATH updates; no contributor toolchain |
| Shared setup helpers | `scripts/lib/dev-setup-common.sh` | Version comparison, prerequisite checks, uv install, profile PATH edits, `Makefile` pin lookup |
| Toolchain preflight | `scripts/dev-check.sh` | Run by `make dev-check`; lists every missing or mismatched tool and exits non-zero |
| Contributor toolchain targets | `Makefile` (`dev-setup`, `dev-tools`, `dev-python`, `dev-node`, `dev-check`) | `make help` lists them; pins live in the `BUF_VERSION`, `GOLANGCI_LINT_VERSION`, `UV_VERSION`, and `PYTHON_VERSION` variables |
| Go package smoke test | `scripts/smoke-test-go.sh` | Temporary module creation, `replace` directive, import verification |
| Python package smoke test | `scripts/smoke-test-python.sh` | Virtual environment, editable install, import and example verification |
| COSAiS validation | `internal/tools/cosais_validator` | Finalized overlay ID coverage check; invoked by `make cosais-validate` |
| Developer-guideline audit | `scripts/audit-dev-guidelines.py` | Go/Python guideline checks, function clones, and protocol alignment |
| Gateway-embedded deploy (Bash) | `internal/services/gateway/scripts/g8e-deploy.sh` | Embedded by `make build`; served at `/g8e-deploy.sh` |
| Gateway-embedded deploy (PowerShell) | `internal/services/gateway/scripts/g8e-deploy.ps1` | Embedded by `make build`; served at `/g8e-deploy.ps1` |
| Gateway binary download manifest | `g8e-binaries.json` (validated at build time) | Validated platform matrix: Linux amd64/arm64/386, Windows amd64/arm64, Darwin amd64/arm64 |
| Image transfer manifest | `demos/images.json` | Digest-pinned external base and service images used across demo environments |

## Procedures

### Development Bootstrap

Run the platform setup script from the repository root. The root `Makefile` must be visible to the invoked `make` targets:

```bash
bash scripts/linux-setup.sh
bash scripts/macos-setup.sh
pwsh scripts/windows-setup.ps1
```

The Linux and macOS scripts accept `-y` / `--yes` (or `G8E_SETUP_YES=1`) to install without prompting, `--build-only` to stop after `make build`, and `-h` / `--help`. By default each script runs six steps:

1. Checks `git`, `make`, `curl`, `go`, `node`, `npm`, `python3`, `uv`, `rg`, `bc`, and a C compiler. The required Go version is read from `go.mod` (currently 1.26.6) and compared including the patch component. Node.js 22+ is required because `make build` embeds the evaluation-explorer frontend and the console is built with Node. A tool that resolves under `/mnt/<drive>/` does not count (INV-SCRIPTS-07). If a prerequisite is missing or too old, the script prompts before installing it, then re-checks. Linux installs `git`, `make`, `curl`, `python3`, `ripgrep`, `bc`, and a compiler (`build-essential` or `gcc`) in one call through `apt`, `dnf`, `pacman`, or `zypper`; installs Go from the official tarball into `/usr/local/go` and Node.js from the latest 22.x tarball into `/usr/local/lib/nodejs` (links in `/usr/local/bin`), verifying the published SHA-256 of each; and installs the `uv` version pinned in the `Makefile` into `~/.local/bin`. The tarball route works on WSL, where `snap` frequently does not run. macOS uses Homebrew for packages, points to `xcode-select --install` for the compiler, and uses the same pinned `uv` installer. Windows prefers `winget` with Chocolatey fallback.
2. Builds the evaluation-explorer asset when `evaluation-explorer/dist/index.html` is absent.
3. Runs `make build`, which embeds the evaluation explorer and the console (the committed console embed is used when `console/dist` is absent). The build writes the platform binary and SHA-256 sidecar under `bin/` and copies the host executable to the repository root.
4. Runs `make dev-setup` (see [Contributor Toolchain](#contributor-toolchain)).
5. Updates the user PATH persistently: Linux and macOS append the repository root, the Go install directory (`GOBIN` or `GOPATH/bin`), `~/.local/bin`, and `/usr/local/go/bin` (when Go lives there) to `~/.zshrc`, `~/.bashrc`, or `~/.profile` based on detected shell, skipping any directory the profile already mentions; profiles are appended to and never rewritten. Windows updates user-level `Path`. The scripts also export PATH in the current shell, but that change is lost when the shell exits, so users MUST open a new terminal or source the profile before `g8e` is available in other shells.
6. Runs `make dev-check` to confirm the machine can run `make ci`.

With `--build-only`, the scripts check only `git`, `make`, `curl`, `go`, `node`, and `npm`, then run steps 2, 3, and 5 as a four-step flow (the script labels them 1 through 4: prerequisites, explorer, build, PATH). Use it when only the `g8e` binary is needed.

After setup, scripts print next steps: `./g8e --version`, `make dev-check` and `make ci` (full flow only), `g8e docker start`, `g8e gw start`, and `g8e auth enroll user -e localhost`.

`windows-setup.ps1` requires PowerShell 7+ (`pwsh`). It builds `g8e` natively but does not install the contributor toolchain, because the `Makefile` targets assume a POSIX shell and `.venv/bin`. Windows contributors run `bash scripts/linux-setup.sh` inside WSL 2. It is not a substitute for WSL when native Windows build tooling is incomplete; use Docker quick-start when local compiler toolchain is unavailable.

### Contributor Toolchain

The root `Makefile` installs everything `make ci` needs beyond the operating-system prerequisites, so the setup scripts and a manual install share one definition:

```bash
make dev-setup    # dev-tools + dev-python + dev-node
make dev-check    # verify; also runs automatically before every ci target
```

| Target | Installs |
| --- | --- |
| `make dev-tools` | `buf`, `protoc-gen-go`, `protoc-gen-go-grpc`, `protoc-gen-doc`, `golangci-lint`, `govulncheck`, and `swag` through `go install` into `GOBIN` or `GOPATH/bin`. Pinned tools reinstall at the `Makefile` versions on every run; `govulncheck` and `swag` track `@latest`, matching CI. |
| `make dev-python` | Repo-root `.venv` created by `uv` with `PYTHON_VERSION` (uv downloads the interpreter when the system has none), the in-tree `protocol/python` package, and `ensemble[test]` (pytest, ruff, pyright). An existing `.venv` is reused. |
| `make dev-node` | `npm ci` in `protocol/node`, `console`, and `g8e-adapter`, then builds the adapter. |

The `Makefile` puts the Go install directory and `~/.local/bin` on `PATH` for its own recipes, so these tools resolve under `make` even in a shell that has not sourced the updated profile. `make proto-generate` installs any missing protoc plugin itself through `make proto-tools-install`; buf runs them as `local:` plugins from `PATH`, so a missing plugin otherwise fails with `executable file not found in $PATH`. The CI workflow `.github/workflows/build-and-test.yml` pins the same versions separately and must change together with the `Makefile` pins.

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
make cosais-validate
```

`go run ./internal/tools/cosais_validator` requires `docs/reference/cosais-overlays.json` and at least one `demos/*/doctrine/` directory. It reads overlays with `status` exactly matching `finalized`, collects `overlay_ids` from top-level `doctrines` arrays in every JSON file directly under each demo doctrine directory, and fails if a finalized overlay ID is absent from that collected set. When the catalog contains no finalized overlays, it exits successfully and reports that no coverage check is active. The validator does not query NIST or determine finalization independently.

`make lint` includes `make cosais-validate`, and the primary CI workflow invokes it directly. Schema and broader doctrine-reference validation are owned by `make doctrines-validate`.

#### Developer-Guideline Audit

Run from the repository root:

```bash
python3 scripts/audit-dev-guidelines.py
```

The standard-library-only audit maps findings to `docs/devs/devs.md` invariants and the Ensemble coding guide. Go checks use a comment/string-aware lexer; Python checks use ASTs. It reports substantial exact function duplicates and renamed clones, possible unused private helpers, copied public protocol constants in g8ee, disagreement between literal Go declarations and explicit `_go_const` registry entries, semantic drift between `protocol/constants/*.json` and bundled Python registries, and missing literal owners in `docs/devs/codemap.md`. Additional rules cover runtime I/O, errors, environment keys, untyped maps, hidden-creation helper names, Pydantic imports, discarded exceptions, and local subprocess execution in Ensemble.

Default output shows the 25 files ranked by confidence-weighted finding count, with guideline references and related locations. `--list-rules` shows the rule catalog. `--rule` is repeatable; `--confidence high` filters weaker signals. Existing `--limit 0`, `--min-violations N`, `--include-generated`, and `--json` options remain available. `--root` must identify the repository root so ownership exemptions retain their meaning; the default guidelines path follows that root.

```bash
# Review duplicate paths and their existing owners.
python3 scripts/audit-dev-guidelines.py --rule duplicate-function --rule renamed-function-clone --limit 0
# Inspect g8e/g8ee shared-contract alignment.
python3 scripts/audit-dev-guidelines.py --rule protocol-constant-copy --rule protocol-go-constant-drift --rule protocol-registry-drift --json
# Optional automation gate for selected rules; existing debt can fail this command.
python3 scripts/audit-dev-guidelines.py --rule protocol-registry-drift --fail-on high --json
# Isolated audit regression tests; no platform services or third-party packages required.
python3 -m unittest discover -s scripts/tests -p 'test_audit_dev_guidelines.py'
```

Exit status is 0 for a completed report, 1 when selected findings meet `--fail-on`, and 2 for invalid inputs or incomplete coverage caused by read/Python parse failures. Gating uses all selected findings before display limits and minimum file-count filters. JSON includes untruncated totals by rule and coverage errors even when those errors are excluded from displayed rules.

Vendor trees, agent worktrees, runtime directories, dependencies, generated files, and embedded frontend builds are excluded by default. Tests contribute symbol references for unused-helper checks, but production checks skip injected test failures; test-specific checks still inspect integration/E2E parallelism and working-directory changes. Exact clones retain literal values; Python renamed clones preserve global names, call targets, and attributes, while Go renamed clones use a lexical fingerprint. `--min-clone-tokens` adjusts the default 60-token threshold. Private-helper checks include small functions and exclude methods, decorated Python functions, and Go entry points.

Alignment checks read generated Go constant owners and bundled JSON even when generated files are excluded from smell checks. Computed constants, aliases, struct-backed registries, and identifiers without explicit ownership metadata remain the owning conformance tests’ responsibility.

These findings are review candidates. Generic maps may be appropriate for open-ended data, duplicate bodies may implement distinct contracts or build variants, and dynamic imports, reflection, registration, and generated consumers can make a helper appear unused. The audit does not prove call-graph reachability, governance enforcement, Go type correctness, or semantic equivalence across languages; TypeScript/JavaScript do not receive structural checks. It performs no deletions and does not replace the owning conformance checks, semantic lint, or tests.

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

The manifest covers digest-pinned external base and service images. Export does not include locally built Gateway, Operator, demo, or Ensemble images; import does not validate tars against `demos/images.json`. Follow [Air-Gapped Deployment Guide](../guides/air_gap.md) for source staging, locally built image transfer, offline checks, and runtime egress verification. The [Demos README](../../demos/README.md) owns per-demo topology and operating workflow.

## Anti-patterns

- Invoking `g8e` immediately after setup without opening a new shell or sourcing the profile; PATH updates are not visible to the current shell (INV-SCRIPTS-02).
- Running setup scripts from a different working directory; `make build` will use the wrong Makefile (INV-SCRIPTS-01).
- Running `make ci` on a machine that skipped the setup script or used `--build-only`; `make dev-check` stops it and lists what is missing (INV-SCRIPTS-06).
- Treating `command -v npm` as proof of a Linux Node.js toolchain on WSL; the match can be a Windows shim (INV-SCRIPTS-07).
- Interpreting validator and audit script findings as definitive proof without semantic verification (INV-SCRIPTS-03).
- Running Gateway deploy scripts on hosts whose `.g8e/pki` state must be retained (INV-SCRIPTS-04).
- Running `g8e demos` commands from a directory other than the repository root; the tool will not find `demos/images.json` (INV-SCRIPTS-05).
- Assuming exported images or tars are validated against the manifest; validation is the caller's responsibility (INV-SCRIPTS-05).

## Links out

- [Developer Guidelines](../devs/devs.md): coding invariants and repository standards.
- [Documentation Guide](../devs/docs.md): how to audit, write, and review documentation.
- [Getting Started](../guides/getting_started.md): user-facing setup and first-run steps.
- [Build and Run a g8e Operator](../guides/build_operator.md): Operator enrollment and configuration.
- [Air-Gapped Deployment](../guides/air_gap.md): offline source staging and image transfer.
- [Demo Environments](../../demos/README.md): per-demo topology and operating workflow.
