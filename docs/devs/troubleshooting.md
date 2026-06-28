# Developer Troubleshooting

This page covers common setup failures for contributors working on g8e from a
fresh checkout. The platform runs host-native.

## First checks

Run commands from the repository root:

```bash
pwd
ls README.md g8e Makefile
```

Use a POSIX shell such as Linux, macOS Terminal, WSL, or Git Bash. The
`Makefile` uses Bash, `sed`, and `curl`. The `g8e` binary at the repository
root is a compiled Go executable; it does not depend on shell utilities beyond
the system libc.

At minimum, install the tools for the component you are touching:

- Go 1.26.4 or later for the g8e Operator and protocol work.
- Python 3.10 or later for protocol generation and demo scripts.

## `make` targets fail with missing `curl`

The `Makefile` uses `curl` to download Buf during `make buf-install` and in
demo scripts. If `curl` is not installed, install it and retry:

```bash
command -v curl
make proto
```

If the command exists in one terminal but not another, fix the shell `PATH`
before changing project files.

> Note: To support sovereign, agnostic, and air-gapped deployments, `jq` is
> completely eliminated as a host dependency. All JSON parsing and request
> assembly are handled internally by the Go CLI, allowing g8e to run on
> virtually any modern Linux environment without extra system package requirements.

## `make proto` fails before generating files

`make proto` runs `make buf-install`, then calls Buf to generate Go Protobuf
code from the schema definitions in `protocol/proto/`.

Check the local prerequisites first:

```bash
command -v go
command -v buf
```

The `buf-install` target attempts to provision Buf in the following order:

1. If Go is present on the host, it installs Buf via
   `go install github.com/bufbuild/buf/cmd/buf@v1.70.0`.
2. If Go is absent, it attempts to download the pre-compiled binary from Buf
   releases using `curl`.
3. If neither succeeds, `make proto` exits with an error. The pre-generated
   `.pb.go` files committed under `protocol/proto/` allow `go build` to
   succeed without running `make proto`, but schema changes require a working
   Buf installation.

If you are modifying `.proto` files in an offline environment, ensure that
`buf` is installed globally on your path before running `make proto`.

For Python protocol generation, use the separate target:

```bash
make proto-python
```

## `./g8e gw start` does not become healthy

The `gw start` command launches the g8e Gateway as a background process via
`gateway serve`, then waits for the process to become healthy. Start with the
status command and the log:

```bash
./g8e gw status
./g8e gw logs
```

Common causes:

- One of the local ports from `protocol/constants/ports.json` (HTTP 8080, HTTPS 8443) is already in use. The process manager in `internal/cli/platform/process.go` performs a preflight port check and reports conflicting PIDs.
- The Go toolchain is missing or below the version expected by the current Developer Guidelines (Go 1.26.4).
- Runtime PKI or secrets were created by an older incompatible checkout.

Stop the managed process before retrying. Use `gw restart` as a shortcut, or
stop and start manually:

```bash
./g8e gw restart
```

```bash
./g8e gw stop
./g8e gw start
```

Use `./g8e gw reset` or `./g8e gw clean` only for disposable local
state. They intentionally remove runtime data under `.g8e/`.

## Tests fail because the gateway is not running

The test suite uses a tiered structure with different infrastructure requirements:

- **Tier 1 (Unit tests)**: Run immediately without external dependencies via `make test-unit`.
- **Tier 2 (In-Process Integration)**: No external dependencies. Integration tests use in-process gateway fixtures (`test/fixtures/gateway_fixture.go`) that spin up the gateway within the test process. Run via `make test-integration`.
- **Tier 3 (Docker E2E)**: Requires Docker. Run via `make test-docker`.

Tier 2 integration tests do not require a running external gateway. They
construct the gateway in-process via `GatewayFixture`, which handles PKI
enrollment and mTLS configuration automatically. If these tests fail, the cause
is typically a port conflict or missing build dependencies, not a missing
gateway process.

If a test failure mentions missing trust bundles or client certificates,
confirm that the test fixture has not been modified to skip enrollment. The
`EnrollClientIdentity` helper in `test/fixtures/gateway_fixture.go` generates
test PKI material at runtime.

## Authentication failures after gateway start

The gateway requires explicit authentication before it can be used. After
starting the gateway, enroll to bootstrap your credentials:

```bash
./g8e gw start
./g8e auth enroll
```

If authentication fails, check the following:
- Ensure the gateway is running via `./g8e gw status`.
- Verify the external IP displayed during gateway start matches your network interface.
- For passkey authentication, ensure your hardware security key or platform authenticator is available.
- For certificate-based authentication, ensure `.g8e/cli.crt` and `.g8e/cli.key` exist. The `auth enroll` command generates these files via CSR-based enrollment with the gateway CA. On Windows, enrollment uses the Windows Certificate Store automatically.

## Path resolution problems

The CLI resolves the project root using `config.FindProjectRoot()` in
`internal/config/config.go`, which returns the current working directory
(`os.Getwd()`). Run commands from the project root directory to ensure
correct path resolution:

```bash
cd /path/to/g8e
./g8e gw status
```

## `./g8e` command not found

The `g8e` file at the repository root is a compiled Go binary. If you receive
"command not found", ensure you are running from the repository root and the
binary has execute permissions:

```bash
ls -l g8e
chmod +x g8e
./g8e gw status
```

If the binary is missing or outdated, rebuild it:

```bash
make build
```

The `make build` target compiles `cmd/operator` and copies the resulting
binary to the repository root as `g8e`. The target handles Windows builds
natively, producing `g8e.exe` when run on Windows.
