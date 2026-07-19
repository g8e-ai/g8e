# g8e Scripts

Last Updated: 2026-07-19
Version: v1.5.9

g8e provides platform-specific bootstrap scripts for local development, gateway-served deploy scripts for remote operator installation, and smoke test scripts that verify SDK importability in clean environments. The `g8e demos` CLI also supports air-gapped image export and import for demo environments.

## Overview

| Category | Script | Purpose |
|---|---|---|
| Dev bootstrap | `linux-setup.sh` | Fresh-clone to working binary on Linux |
| Dev bootstrap | `macos-setup.sh` | Fresh-clone to working binary on macOS |
| Dev bootstrap | `windows-setup.ps1` | Fresh-clone to working binary on Windows |
| Smoke test | `smoke-test-go.sh` | Verifies the Go module imports in a clean project |
| Smoke test | `smoke-test-python.sh` | Verifies the Python package installs and imports in a clean venv |
| Remote deploy | `g8e-deploy.sh` | Served by gateway; downloads binary on Linux/macOS hosts |
| Remote deploy | `g8e-deploy.ps1` | Served by gateway; downloads binary on Windows hosts |
| Air-gapped deploy | `g8e demos` CLI | Pull, export, import, and list Docker images for air-gapped demo deployments |

## Dev Bootstrap Scripts

Platform-specific bootstrap scripts get a developer from a fresh clone to a working `g8e` binary in one command. Each script validates required dependencies, installs missing tooling interactively, builds the project, and adds the binary to PATH.

### Usage

Run the script for your platform: `bash scripts/linux-setup.sh` on Linux, `bash scripts/macos-setup.sh` on macOS, or `pwsh scripts/windows-setup.ps1` on Windows (PowerShell 7+).

After the script completes, open a new terminal or source your profile and verify with `g8e --version`.

### What Each Script Does

1. **Validates and installs dependencies**: Checks for `make` and `go`, verifies the Go version meets the minimum required version (currently 1.26), and prompts interactively to install missing tooling via the platform-appropriate package manager (`apt-get`, `dnf`, `pacman`, `zypper`, `brew`, `winget`, or `choco`).
2. **Builds the binary**: Runs `make build` to produce the `g8e` binary.
3. **Adds to PATH**: Adds the repository root to the shell profile or Windows user PATH so `g8e` is available in new terminal sessions.

### Requirements

- **Git**: to clone the repository (not checked by the scripts).
- **Go 1.26+**: validated by the scripts; installed if missing.
- **Make**: validated by the scripts; installed if missing.
- **Homebrew**: required on macOS for automatic dependency installation.
- **winget or Chocolatey**: required on Windows for automatic dependency installation.

## Smoke Test Scripts

Two smoke test scripts verify that the published Go and Python packages work in clean environments, mirroring the README quickstart. CI runs both on every pull request.

### Usage

Run `bash scripts/smoke-test-go.sh` to verify the Go SDK and `bash scripts/smoke-test-python.sh` to verify the Python package. Both scripts create isolated temporary environments, import public packages, and clean up on exit.

### What Each Script Does

- **`smoke-test-go.sh`**: Creates a temporary Go module, adds a replace directive pointing at the local repository, imports the protocol package, and builds a minimal binary.
- **`smoke-test-python.sh`**: Creates a clean virtual environment, installs the Python package in editable mode, verifies README imports and example scripts, then removes the environment.

## Remote Deploy Scripts (Gateway-Served)

The gateway serves deploy scripts over HTTP on port 8080. A remote host fetches the script with `curl` or `Invoke-WebRequest`, and the script auto-detects OS and architecture, downloads the correct pre-built binary from the gateway, and starts the g8e operator in enrollment mode.

### Usage on a Remote Host

On Linux or macOS, run `curl -fsSL http://<gateway-ip>:8080/g8e-deploy.sh | bash`. On Windows (PowerShell), run `iwr http://<gateway-ip>:8080/g8e-deploy.ps1 -UseBasicParsing | iex`.

### Prerequisites

- The gateway must be running and reachable on its HTTP port (default 8080).
- Pre-built platform binaries must exist on the gateway (run `make build-all` on the gateway host to produce them).
- `curl` or `wget` on Linux/macOS; PowerShell on Windows.

## Air-Gapped Deployment Commands

The `g8e demos` CLI provides `pull`, `export`, `import`, and `images` commands for air-gapped demo environments. The manifest at `demos/images.json` pins all external Docker images to sha256 digests.

### Usage

On a connected machine, pull and export images with `g8e demos pull` and `g8e demos export /tmp/g8e-images`. Transfer the export directory to the air-gapped machine, then run `g8e demos import /tmp/g8e-images`. List all images in the manifest with `g8e demos images`.

See [Demos README](../../demos/README.md#air-gapped-deployment) for the full air-gapped deployment workflow.

## See Also

- [Getting Started Guide](../guides/getting_started.md)
- [Developer Guide](../devs/devs.md)
- [Build Gateway](../guides/build_gateway.md)
- [Build Operator](../guides/build_operator.md)
- [Air Gap Guide](../guides/air_gap.md)
- [Demos README](../../demos/README.md)
