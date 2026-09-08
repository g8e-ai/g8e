# g8e Scripts

Last Updated: 2026-09-08
Version: v2.1.7

This document catalogs the executable automation under `scripts/`, the deploy-script templates embedded in the Gateway, and the script-backed `g8e demos` image-transfer workflow. The root `Makefile` owns build and validation orchestration; the [Documentation Guide](../devs/docs.md) and [Release Process](../devs/release_process.md) own the generated README and public-evidence procedures.

## Inventory

| Category | Entry point | Purpose |
| --- | --- | --- |
| Development bootstrap | `scripts/linux-setup.sh` | Checks Linux build prerequisites, runs the host build, and adds the repository root to the user’s shell profile |
| Development bootstrap | `scripts/macos-setup.sh` | Checks macOS build prerequisites, runs the host build, and adds the repository root to the user’s shell profile |
| Development bootstrap | `scripts/windows-setup.ps1` | Checks Windows build prerequisites, runs the host build, and adds the repository root to the user PATH |
| Package smoke test | `scripts/smoke-test-go.sh` | Builds a clean temporary Go module against the local g8e module |
| Package smoke test | `scripts/smoke-test-python.sh` | Installs the local Python protocol package in a virtual environment and runs its public imports and examples |
| Data validation | `scripts/validate-cosais-overlays.sh` | Checks detector references for overlays marked finalized in the checked-in COSAiS catalog |
| Make target audit | `scripts/validate-make-targets.sh` | Runs selected groups of root Makefile targets and summarizes their results |
| README rendering | `scripts/generate_readme.py` | Validates the promoted public proof snapshot and renders or checks the generated root README |
| Evidence projection | `scripts/project_readme_evidence.py` | Projects a private evaluation report into a sanitized README evidence candidate |
| Provenance collection | `scripts/collect_readme_provenance.py` | Produces a canonical, checksum-bound source, component, image, and environment manifest |
| Evidence promotion | `scripts/promote_readme_evidence.py` | Validates an approved candidate tree digest and atomically replaces the promoted README evidence snapshot |
| Remote Operator bootstrap | `/g8e-deploy.sh` and `/g8e-deploy.ps1` | Public Gateway HTTP routes that render embedded scripts for binary download and Operator startup |
| Air-gap image transfer | `g8e demos pull`, `export`, `import`, and `images` | Transfers the digest-pinned external images declared in `demos/images.json` |

## Development Bootstrap

Run the platform script from the repository root so its `make build` invocation uses the root Makefile:

```bash
bash scripts/linux-setup.sh
bash scripts/macos-setup.sh
pwsh scripts/windows-setup.ps1
```

Each script performs three steps:

1. It checks for `make` and `go`. Its version check accepts Go 1.26 or any later major or minor release; the root `go.mod` is the authoritative build requirement and currently declares Go 1.26.6.
2. If a checked dependency is missing or too old, it asks before invoking a supported package manager. Linux supports `apt-get`, `dnf`, `pacman`, or `zypper`; macOS uses Homebrew; Windows prefers `winget` and otherwise uses Chocolatey. The package manager is necessary only when the script must install a dependency.
3. It runs `make build`, which writes the platform binary and SHA-256 file under `bin/`, copies the host executable to the repository root, and copies the executable to `demos/bin/g8e`.

The Linux and macOS scripts append the repository root to `~/.zshrc`, `~/.bashrc`, or `~/.profile`, based on the current shell. The Windows script updates the user-level `Path`. These are persistent user-environment mutations. The scripts also update their own process environment, but that child-process update does not alter the invoking shell. Open a new terminal or source the selected profile, then run `g8e --version`.

The scripts check only Go and Make before building. The root Makefile and host environment can impose additional command requirements. PowerShell 7 is the documented Windows entry point, but `windows-setup.ps1` does not enforce the PowerShell version and still invokes `make build`; it is not a standalone native Windows compiler workflow.

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

`validate-cosais-overlays.sh` requires `python3`, `docs/reference/cosais-overlays.json`, and at least one `demos/*/doctrine/` directory. It reads overlays whose checked-in `status` is exactly `finalized`, collects `overlay_ids` from the top-level `doctrines` arrays in every JSON file directly under each demo doctrine directory, and fails when a finalized overlay ID is absent from that collected set. If the catalog contains no finalized overlays, it exits successfully and reports that no coverage check is active. The script does not query NIST or determine finalization independently.

`make lint` includes `make validate-cosais`, and the primary CI workflow also invokes the target directly. Schema and broader doctrine-reference validation remain owned by `make validate-doctrines`.

### Make Target Audit

`validate-make-targets.sh` is a manually invoked broad audit of root Makefile targets. Run every configured phase with `bash scripts/validate-make-targets.sh`, or select one phase through the environment:

```bash
PHASE=lint bash scripts/validate-make-targets.sh
```

The script groups targets into help, build, protocol, lint, test, Python, dashboard, Docker, CI, doctrine, and cleanup phases. It suppresses each target’s output, continues after failures, and exits nonzero after printing the aggregate summary when any target failed. It always skips `release`, but an unfiltered run still invokes targets that require local services, Docker, external credentials, or network access, and its final cleanup phase runs state-removing Make targets. Review the phase definitions before using the unfiltered mode. This audit is not wired into the primary CI workflow.

## README and Public-Evidence Tooling

The README tools form one controlled pipeline rather than independent editing utilities:

1. `collect_readme_provenance.py` writes a new canonical provenance record for an explicit source-file list and supplied component, image, campaign, and environment records. It rejects an existing output path.
2. `project_readme_evidence.py` validates and sanitizes a private evaluation report into a new candidate directory. Stage 2 projection also binds a baseline candidate and collected provenance.
3. `promote_readme_evidence.py` computes the candidate tree digest, requires an exact release-owner-approved digest, validates the candidate with the README reader, and atomically replaces `docs/evidence/readme/current/`.
4. `generate_readme.py` validates the promoted snapshot and `docs/templates/README.md.tmpl`, then writes `README.md`. Its `--check` mode renders in memory and fails on drift without modifying the output.

Use `make readme`, `make readme-test`, and `make readme-check` for normal rendering and validation. Candidate creation and promotion are attended release operations; follow the exact inputs, review boundaries, and approval process in the [Release Process](../devs/release_process.md). Do not edit `README.md` or `docs/evidence/readme/current/` directly.

## Gateway-Served Operator Bootstrap

The Gateway embeds `internal/services/gateway/scripts/g8e-deploy.sh` and `g8e-deploy.ps1` at build time. Gateway HTTP handler construction parses both templates and fails if initialization fails. The plain HTTP router exposes these unauthenticated GET routes on the configured Gateway HTTP port, which defaults to 8080:

| Route | Content | Authentication |
| --- | --- | --- |
| `/g8e-deploy.sh` | Rendered Bash script for Linux and macOS | None |
| `/g8e-deploy.ps1` | Rendered PowerShell script for Windows | None |
| `/.well-known/g8e/bin/{filename}` | Pre-built platform executable | None |

The binary route accepts only names matching `g8e-(linux|darwin|windows)-(amd64|arm64|386)` with an optional `.exe` suffix. It searches the image-baked `/opt/g8e/bin`, a `bin/` directory beside the running executable, and the Gateway PKI binaries directory. The standard `make build-all` matrix produces Linux amd64, arm64, and 386; Windows amd64 and arm64; and Darwin amd64 and arm64. The scripts recognize Windows x86 and Darwin 386, but those binaries are not produced by the standard all-platform build.

The rendered scripts derive the Gateway host and HTTP port from the request and configuration. The Windows handler prefers `X-Forwarded-Host`; both templates allow `GATEWAY_HOST` and `GATEWAY_PORT` environment variables to override the rendered values at execution time.

On a target host, the script performs this sequence:

1. It recursively removes the target user’s `~/.g8e/pki` directory before validating the target platform or downloading a binary.
2. It detects the operating system and architecture, downloads the selected executable over plain HTTP into the current directory as `g8e` or `g8e.exe`, and replaces an existing file with that name.
3. It starts `g8e operator start -e <gateway-host>` in the foreground. When no Operator credentials remain, Operator startup enters the owner-approved platform enrollment flow described in [Build and Run a g8e Operator](../guides/build_operator.md).

The deploy scripts do not fetch or verify the `.sha256` files produced by the build, do not verify a signature, and do not protect the script or binary download with TLS. The routes are bootstrap conveniences for a trusted network, not an authenticated software-distribution boundary. Fetch and inspect the rendered script before execution, and do not run it on a host whose existing `.g8e/pki` state must be retained.

After the Gateway is reachable and the required platform binaries are present, the current direct execution forms are:

```bash
curl -fsSL http://<gateway-host>:8080/g8e-deploy.sh | bash
```

```powershell
iwr http://<gateway-host>:8080/g8e-deploy.ps1 -UseBasicParsing | iex
```

Linux and macOS require `curl` or `wget`; Windows requires PowerShell. Container-built Gateways include the complete standard platform matrix in `/opt/g8e/bin`; a source-built Gateway host needs the requested binaries in one of the searched locations, normally through `make build-all`.

## Air-Gap Image Transfer

The `g8e demos` image commands read `demos/images.json` relative to the current working directory, so run them from the repository root. They require the Docker CLI and a reachable Docker daemon.

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
