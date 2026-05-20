# Developer Troubleshooting

This page covers common setup failures for contributors working on g8e from a
fresh checkout. The platform itself runs host-native; Docker is only needed for
the demo fleet and a few data-inspection helpers.

## First checks

Run commands from the repository root:

```bash
pwd
ls README.md g8e Makefile
```

Use a POSIX shell such as Linux, macOS Terminal, WSL, or Git Bash. The root
`./g8e` launcher and `Makefile` use Bash, `find`, `sed`, `curl`, and `jq`.

Check the current version requirements in [Developer Guidelines](../devs.md).
At minimum, install the tools for the component you are touching:

- Go for `g8eo` and protocol work.
- Python for the optional `g8ee` adapter and evals.
- Node only for dashboard or GUI work.
- Docker only for demo fleet workflows.

## `./g8e` fails with missing `curl` or `jq`

The launcher performs a dependency check before dispatching subcommands. Install
the missing command and retry from the repository root.

```bash
command -v curl
command -v jq
./g8e platform status
```

If the command exists in one terminal but not another, fix the shell `PATH`
before changing project files.

## `make proto` fails before generating files

`make proto` runs `make buf-install`, then calls Buf and post-processes the
generated Python files with `find` and `sed`.

Check the local prerequisites first:

```bash
command -v curl
command -v chmod
command -v find
command -v sed
make buf-install
```

If Buf is not installed globally, the Makefile downloads a local `./buf`
binary. That step needs network access to GitHub releases and permission to
write an executable file in the repo root.

If generation succeeds but Python imports fail later, rerun the full target
instead of only calling Buf:

```bash
make proto
```

The full target also creates `__init__.py` files and rewrites generated Python
imports for package-relative use.

## Docker socket not found

Core component development does not use Docker. If you see a Docker socket
error while starting the main platform, check that you are not accidentally
running a demo or data-inspection helper.

For normal development, use host-native commands:

```bash
./g8e platform start
./g8e test g8eo
```

For demo fleet work, start Docker Desktop or the local Docker daemon, then check
that the current user can reach the socket:

```bash
docker ps
./g8e demo status
```

On Linux, a permission error usually means the user is not allowed to access the
Docker socket. Fix the host Docker setup before changing g8e code.

## `./g8e platform start` does not become healthy

The platform start path builds and launches the Operator, then waits for the
health endpoint. Start with the status command and the Operator log:

```bash
./g8e platform status
tail -n 80 .g8e/logs/operator-listen.log
```

Common causes:

- A previous Operator process is still running.
- One of the local ports from `protocol/constants/paths.json` is already in use.
- The Go toolchain is missing or below the version expected by the current
  Developer Guidelines.
- Runtime PKI or secrets were created by an older incompatible checkout.

Stop the managed process before retrying:

```bash
./g8e platform down
./g8e platform start
```

Use `./g8e platform reset` or `./g8e platform clean` only for disposable local
state. They intentionally remove runtime data under `.g8e/`.

## `g8ee` virtualenv is missing

The Python engine is optional. Engine and eval commands expect the local
virtualenv under `services/g8ee/.venv`.

Create it through the platform scripts so the expected environment variables
and trust material are wired consistently:

```bash
./g8e platform start --with-apps
```

If you only need the Operator, skip `g8ee` and run:

```bash
./g8e test g8eo
```

## Tests fail because the platform is not running

The test runner uses real infrastructure. Start the platform before tests that
need the Operator, and start optional apps only when the test target requires
them.

```bash
./g8e platform start
./g8e test g8eo

./g8e platform start --with-apps
./g8e test g8ee
```

If a test failure mentions missing trust bundles or client certificates, confirm
that `.g8e/pki/` exists and that `./g8e platform status` reports the Operator as
running.

## Path resolution problems

Scripts resolve `G8E_PROJECT_ROOT` from their own location. Avoid invoking
subscripts directly until the root launcher works:

```bash
./g8e platform status
```

If you have exported `G8E_PROJECT_ROOT` manually, make sure it points at the
same checkout you are editing. A stale value can make scripts read constants,
logs, or generated files from another clone.
