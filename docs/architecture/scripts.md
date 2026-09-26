# g8e Scripts

Last Updated: 2026-09-26
Version: v2.1.13

This document catalogs the executable automation under `scripts/`, the deploy-script templates embedded in the Gateway, and the script-backed `g8e demos` image-transfer workflow. The root `Makefile` owns build and validation orchestration.

## Inventory

| Category | Entry point | Purpose |
| --- | --- | --- |
| Development bootstrap | `scripts/linux-setup.sh` | Checks Linux build prerequisites, builds evaluation-explorer when needed, runs `make build`, and adds the repository root to the user’s shell profile |
| Development bootstrap | `scripts/macos-setup.sh` | Checks macOS build prerequisites, builds evaluation-explorer when needed, runs `make build`, and adds the repository root to the user’s shell profile |
| Development bootstrap | `scripts/windows-setup.ps1` | Checks Windows build prerequisites (PowerShell 7+), builds evaluation-explorer when needed, runs `make build`, and adds the repository root to the user PATH |
| Package smoke test | `scripts/smoke-test-go.sh` | Builds a clean temporary Go module against the local g8e module |
| Package smoke test | `scripts/smoke-test-python.sh` | Installs the local Python protocol package in a virtual environment and runs its public imports and examples |
| Data validation | `make validate-cosais` (`go run ./internal/tools/cosais_validator`) | Checks detector references for overlays marked finalized in the checked-in COSAiS catalog |
| Developer-guideline audit | `scripts/audit-dev-guidelines.py` | Ranks files by statically detectable violations of `docs/devs/devs.md` |
| Remote Operator bootstrap | `/g8e-deploy.sh` and `/g8e-deploy.ps1` | Public Gateway HTTP routes that render embedded scripts for binary download and Operator startup |
| Air-gap image transfer | `g8e demos pull`, `export`, `import`, and `images` | Transfers the digest-pinned external images declared in `demos/images.json` |

## Development Bootstrap

Run the platform script from the repository root so its `make build` invocation uses the root Makefile:

```bash
bash scripts/linux-setup.sh
bash scripts/macos-setup.sh
pwsh scripts/windows-setup.ps1
```

Each script performs four steps:

1. It checks for `git`, `make`, `go`, `node`, and `npm`. The required Go version is read from the root `go.mod` (currently Go 1.26.6). Node.js 22+ is required because `make build` embeds the evaluation-explorer frontend.
2. If a checked dependency is missing or too old, it asks before invoking a supported package manager. Linux prefers `snap` for Go and Node when available because distro packages are often too old; macOS uses Homebrew; Windows prefers `winget` and otherwise uses Chocolatey. Pass `-y` or set `G8E_SETUP_YES=1` for non-interactive installs.
3. It builds the evaluation-explorer asset when `dashboard/g8e-adapter/evaluation-explorer/dist/index.html` is absent (`npm ci` or `npm install`, then `npm run build`), then runs `make build`. The build writes the platform binary and SHA-256 file under `bin/` and copies the host executable to the repository root.
4. It appends the repository root to the user PATH. Linux and macOS update `~/.zshrc`, `~/.bashrc`, or `~/.profile` based on the current shell. Windows updates the user-level `Path`. These are persistent user-environment mutations. The scripts also update their own process environment, but that child-process update does not alter the invoking shell.

After setup, the scripts print the recommended next steps from the current getting-started flow: `./g8e --version`, `g8e docker start --full`, `g8e gw start`, and `g8e auth enroll user -e localhost`. Open a new terminal or source the selected profile before using `g8e` from other shells.

`windows-setup.ps1` requires PowerShell 7+ (`pwsh`). It is not a substitute for WSL when native Windows build tooling is incomplete; use the Docker quick-start path when a local compiler toolchain is unavailable.

## Package Smoke Tests

Run the smoke tests from any directory:

```bash
bash scripts/smoke-test-go.sh
bash scripts/smoke-test-python.sh
```

`smoke-test-go.sh` creates a temporary module, adds a `replace` directive to the current repository, imports `github.com/g8e-ai/g8e/v2/protocol`, runs `go mod tidy`, and builds a minimal executable. Its exit trap removes the temporary module on success or failure. The test can access the configured Go module network while resolving dependencies.

`smoke-test-python.sh` creates `protocol/python/.smoke-env`, upgrades `pip`, installs `protocol/python` in editable mode, checks the public constants, models, and package-version imports, and runs `constants_example.py` and `models_example.py`. It removes the virtual environment after a successful run. Because it has no exit trap, a failed command can leave `.smoke-env` behind for manual removal. The test uses package indexes while upgrading `pip` and resolving dependencies.

The primary `build-and-test.yml` workflow runs both scripts in its Smoke Tests job for pushes to `main`, pull requests targeting `main`, and manual workflow dispatches. These scripts validate the local working tree rather than an already-published Go module or Python distribution.

## Validation and Developer Audit Scripts

### COSAiS Overlay Coverage

Run the COSAiS guard through its owning Make target:

```bash
make validate-cosais
```

`go run ./internal/tools/cosais_validator` requires `docs/reference/cosais-overlays.json` and at least one `demos/*/doctrine/` directory. It reads overlays whose checked-in `status` is exactly `finalized`, collects `overlay_ids` from the top-level `doctrines` arrays in every JSON file directly under each demo doctrine directory, and fails when a finalized overlay ID is absent from that collected set. If the catalog contains no finalized overlays, it exits successfully and reports that no coverage check is active. The validator does not query NIST or determine finalization independently.

`make lint` includes `make validate-cosais`, and the primary CI workflow also invokes the target directly. Schema and broader doctrine-reference validation remain owned by `make validate-doctrines`.

### Developer-Guideline Triage

Run `python3 scripts/audit-dev-guidelines.py` from the repository root to rank files by statically detectable patterns associated with the rules in `docs/devs/devs.md`. The default output shows the 25 files with the most findings and includes each matching line and rule identifier. Use `--limit 0` to print every matching file, `--min-violations N` to focus on higher-count files, `--include-generated` to include generated and Swagger/OpenAPI paths, or `--json` for machine-readable output. The audit is intentionally heuristic: it identifies review candidates and does not prove that a line violates a guideline or replace the repository’s semantic lint and test commands.

## Gateway-Served Operator Bootstrap

The Gateway embeds `internal/services/gateway/scripts/g8e-deploy.sh` and `g8e-deploy.ps1` at build time. Gateway HTTP handler construction parses both templates and fails if initialization fails. The plain HTTP router exposes these unauthenticated GET routes on the configured Gateway HTTP port, which defaults to 8080:

| Route | Content | Authentication |
| --- | --- | --- |
| `/g8e-deploy.sh` | Rendered Bash script for Linux and macOS | None |
| `/g8e-deploy.ps1` | Rendered PowerShell script for Windows | None |
| `/.well-known/g8e/bin/{filename}` | Pre-built platform executable | None |

The binary route accepts only names listed in the validated `g8e-binaries.json` manifest. It serves from the image-baked `/opt/g8e/bin` or, for a source-built Gateway, a validated `bin/` directory beside the running executable. The standard matrix contains Linux amd64, arm64, and 386; Windows amd64 and arm64; and Darwin amd64 and arm64. Unsupported names and unvalidated roots fail closed.

The rendered scripts derive the Gateway host and HTTP port from the request and configuration. The Windows handler prefers `X-Forwarded-Host`; both templates allow `GATEWAY_HOST` and `GATEWAY_PORT` environment variables to override the rendered values at execution time.

On a target host, the script performs this sequence:

1. It recursively removes the target user’s `~/.g8e/pki` directory before validating the target platform or downloading a binary.
2. It detects the operating system and architecture, downloads the selected executable over plain HTTP into the current directory as `g8e` or `g8e.exe`, and replaces an existing file with that name.
3. It starts `g8e operator start -e <gateway-host>` in the foreground. When no Operator credentials remain, Operator startup enters the owner-approved platform enrollment flow described in [Build and Run a g8e Operator](../guides/build_operator.md).

The deploy scripts fetch the matching `.sha256` sidecar and verify the downloaded temporary file before replacing the local executable. They do not verify a signature or protect the script and artifact downloads with TLS. The routes are bootstrap conveniences for a trusted network, not an authenticated software-distribution boundary. Fetch and inspect the rendered script before execution, and do not run it on a host whose existing `.g8e/pki` state must be retained.

After the Gateway is reachable and the required platform binaries are present, the current direct execution forms are:

```bash
curl -fsSL http://<gateway-host>:8080/g8e-deploy.sh | bash
```

```powershell
iwr http://<gateway-host>:8080/g8e-deploy.ps1 -UseBasicParsing | iex
```

Linux and macOS require `curl` or `wget`; Windows requires PowerShell. Standard Gateway container images ship only the runtime binary at `/g8e`; the `/.well-known/g8e/bin/` download surface is available when a validated `bin/` mirror exists beside a source-built Gateway or when `/opt/g8e/bin` contains a full manifest from a custom image build. Build the complete matrix on the host with `make build-all`.

## Air-Gap Image Transfer

The `g8e demos` image commands read `demos/images.json` relative to the current working directory, so run them from the repository root. `g8e demos pull`, `export`, and `import` require the Docker CLI and a reachable Docker daemon; `g8e demos images` only reads the manifest.

- `g8e demos images` prints each manifest image, digest, informational tag, and associated demos.
- `g8e demos pull` pulls every entry as `<image>@<digest>`.
- `g8e demos export [output-dir]` saves each manifest image to a separate tar file and skips a tar path that already exists. The default directory is `demos/images-export/`.
- `g8e demos import [input-dir]` loads every `.tar` file directly under the selected directory. The default directory is `demos/images-export/`.

The manifest covers digest-pinned external base and service images. Export does not include locally built Gateway, Operator, demo, Ensemble, or Dashboard images, and import does not validate the tar files against `demos/images.json`. Follow the [Air-Gapped Deployment Guide](../guides/air_gap.md) for source staging, locally built image transfer, offline checks, and runtime egress verification. The [Demos README](../../demos/README.md) owns the per-demo topology and operating workflow.

## Related Documentation

- [Documentation Guide](../devs/docs.md)
- [Release Process](../devs/release_process.md)
- [Getting Started](../guides/getting_started.md)
- [Build and Run a g8e Operator](../guides/build_operator.md)
- [Connect an Operator to a Gateway](../guides/connect_operator_to_gateway.md)
- [Air-Gapped Deployment](../guides/air_gap.md)
- [Demo Environments](../../demos/README.md)
