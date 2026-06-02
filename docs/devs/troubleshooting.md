# Developer Troubleshooting

This page covers common setup failures for contributors working on g8e from a
fresh checkout. The platform runs host-native; Docker is not required for
platform development.

## First checks

Run commands from the repository root:

```bash
pwd
ls README.md g8e Makefile
```

Use a POSIX shell such as Linux, macOS Terminal, WSL, or Git Bash. The root
`./g8e` launcher and `Makefile` use Bash, `find`, `sed`, and `curl`.

At minimum, install the tools for the component you are touching:

- Go for `g8eo` and protocol work.
- Python for optional g8e-compatible agentic ensembles and evals.

## `./g8e` fails with missing `curl`

The launcher performs a dependency check before dispatching subcommands. Install
the missing command and retry from the repository root.

```bash
command -v curl
./g8e gw status
```

If the command exists in one terminal but not another, fix the shell `PATH`
before changing project files.

> Note: To support sovereign, agnostic, and air-gapped deployments, `jq` is
> completely eliminated as a host dependency. All JSON parsing and request
> assembly are handled internally by the unified Go CLI, allowing g8e to run on
> virtually any modern Linux environment without extra system package requirements.

## `make proto` fails before generating files

`make proto` runs `make buf-install`, then calls Buf and post-processes the
generated Python files with `find` and `sed`.

Check the local prerequisites first:

```bash
command -v curl
command -v chmod
command -v find
command -v sed
```

For air-gapped and sovereign setups, the Makefile is highly resilient:
1. If Go is present on the host, `make proto` natively compiles Buf from source
   on your machine to ensure a flawless, host-agnostic binary fit.
2. If Go is absent, it attempts to download the pre-compiled binary from Buf releases.
3. If both are absent or if the network is offline, the build gracefully succeeds
   and utilizes the pre-generated protocol files already committed under
   `protocol/proto/` rather than failing the compilation.

If you are modifying `.proto` files in an offline environment and need to recompile,
ensure that `buf` is installed globally on your path.

If generation succeeds but Python imports fail later, rerun the full target
instead of only calling Buf:

```bash
make proto
```

The full target also creates `__init__.py` files and rewrites generated Python
imports for package-relative use.

## `./g8e gw start` does not become healthy

The gateway start path builds and launches the g8e Gateway, then waits for the
health endpoint. Start with the status command and the log:

```bash
./g8e gw status
./g8e gw logs
```

Common causes:

- One of the local ports from `protocol/constants/ports.json` is already in use (the startup script performs an automatic preflight check and reports conflicting PIDs).
- The Go toolchain is missing or below the version expected by the current Developer Guidelines.
- Runtime PKI or secrets were created by an older incompatible checkout.

Stop the managed process before retrying:

```bash
./g8e gw stop
./g8e gw start
```

Use `./g8e gw reset` or `./g8e gw clean` only for disposable local
state. They intentionally remove runtime data under `.g8e/`.

## Tests fail because the gateway is not running

The test runner uses real infrastructure. Start the gateway before tests that
need the g8e Gateway, and start optional apps only when the test target requires
them.

```bash
./g8e gw start
make test
```

If a test failure mentions missing trust bundles or client certificates, confirm
that `.g8e/pki/` exists and that `./g8e gw status` reports the g8e Gateway as
running.

## Path resolution problems

Scripts resolve the project root by walking up from the current working directory to find the `.git` directory or `protocol/` directory. Avoid invoking subscripts directly until the root launcher works:

```bash
./g8e gw status
```

Run commands from the project root directory to ensure correct path resolution.

## `./g8e` command not found

The root `./g8e` launcher is a Bash script at the repository root. If you receive
"command not found", ensure you are running from the repository root and the script
has execute permissions:

```bash
ls -l g8e
chmod +x g8e
./g8e gw status
```

The launcher delegates to the compiled binary in `./bin/g8e`. If the binary is missing,
run:

```bash
make build
```
