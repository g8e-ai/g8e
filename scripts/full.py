# Copyright (c) 2026 Lateralus Labs, LLC.
# Use of this source code is governed by the Business Source License
# included in the LICENSE file.
#
# As of the Change Date listed in the LICENSE file, this software is
# released under the Apache License, Version 2.0.

"""Interactive host stack launcher, invoked after `make up` by `make full`."""

import argparse
import os
import shlex
import subprocess
import sys
import time
from pathlib import Path

ROOT = Path(__file__).resolve().parents[1]
ROLES = ("provenance", "observer", "inference")


def model_root():
    """Consult configuration/storage only; never start or probe Ollama."""
    configured = os.environ.get("OLLAMA_MODELS")
    if configured:
        return str(Path(configured).expanduser())
    # Snap's configured model location takes precedence over its default.
    if Path("/snap/ollama/current").exists():
        try:
            result = subprocess.run(
                ["snap", "get", "ollama", "models"],
                capture_output=True,
                text=True,
                timeout=5,
                check=False,
            )
            if result.returncode == 0 and result.stdout.strip():
                return result.stdout.strip()
        except (OSError, subprocess.TimeoutExpired):
            pass
    for path in (
        Path("/var/snap/ollama/common/models"),
        Path("/usr/share/ollama/.ollama/models"),
        Path.home() / ".ollama/models",
    ):
        if path.is_dir():
            return str(path)
    return str(Path.home() / ".ollama/models")


def prompt(label, default):
    try:
        return input(f"{label} [{default}]: ").strip() or default
    except EOFError as exc:
        raise ValueError(
            "make full requires interactive input; use --dry-run to preview commands"
        ) from exc


def is_local(system):
    return system.lower() in ("localhost", "127.0.0.1", "::1")


def operator_args(role, gateway, directory, storage, ollama):
    args = ["operator", "start", "--endpoint", gateway, "--working-dir", directory]
    if role == "provenance":
        args += [
            "--provenance-operator-enabled",
            "--provenance-operator-id",
            "g8e-model-provenance-operator",
            "--model-storage-root",
            storage,
        ]
    elif role == "inference":
        args += ["--inference-enabled", "--inference-ollama-endpoint", ollama]
    else:
        args += [
            "--provider-boundary-observer-enabled",
            "--provider-boundary-observer-id",
            "g8e-provider-boundary-observer",
        ]
    return args


def powershell_quote(value):
    return "'" + value.replace("'", "''") + "'"


def remote_commands(system, directory, args):
    print(f"\nRun on {system} with a g8e binary built for that host.")
    print(
        "Use a Gateway hostname/IP reachable from that host and matching its TLS certificate."
    )
    print("POSIX shell (place g8e in the working directory):")

    def shell_path(value):
        if value.startswith("~/"):
            return '"$HOME"/' + shlex.quote(value[2:])
        return shlex.quote(value)

    print(f"mkdir -p {shell_path(directory)} && cd {shell_path(directory)}")
    shell_args = [shlex.quote(arg) for arg in args]
    shell_args[args.index("--working-dir") + 1] = '"$PWD"'
    if "--model-storage-root" in args:
        index = args.index("--model-storage-root") + 1
        shell_args[index] = shell_path(args[index])
    print("./g8e " + " ".join(shell_args))
    print("Windows PowerShell (place g8e.exe in the working directory):")
    quote = powershell_quote
    print(f"New-Item -ItemType Directory -Force -Path {quote(directory)} | Out-Null")
    print(f"Set-Location -LiteralPath {quote(directory)}")
    ps_args = [quote(arg) for arg in args]
    ps_args[args.index("--working-dir") + 1] = "(Get-Location).Path"
    if "--model-storage-root" in args:
        index = args.index("--model-storage-root") + 1
        if args[index].startswith("~/"):
            ps_args[index] = f"(Join-Path $HOME {quote(args[index][2:])})"
    print("& .\\g8e.exe " + " ".join(ps_args))


def start_local(name, directory, command, dry_run=False):
    directory = Path(directory).expanduser().resolve()
    print(f"\n{name}: {shlex.join(command)}\n  working directory: {directory}")
    if dry_run:
        return
    directory.mkdir(parents=True, exist_ok=True)
    pid_file = directory / "full.pid"
    if pid_file.exists():
        try:
            os.kill(int(pid_file.read_text().strip()), 0)
        except (ProcessLookupError, ValueError):
            pass
        else:
            print(
                f"  Already running (PID {pid_file.read_text().strip()}); keeping existing process."
            )
            return
    log_path = directory / "full.log"
    with log_path.open("ab") as log:
        process = subprocess.Popen(
            command,
            cwd=directory,
            stdin=subprocess.DEVNULL,
            stdout=log,
            stderr=subprocess.STDOUT,
            start_new_session=True,
        )
    pid_file.write_text(f"{process.pid}\n")
    # Detect immediate launch/import failures without waiting for owner approval.
    time.sleep(0.5)
    if process.poll() is not None:
        pid_file.unlink(missing_ok=True)
        raise RuntimeError(
            f"{name} exited with status {process.returncode}; see {log_path}"
        )
    print(
        f"  Started PID {process.pid}; log: {log_path} (may be awaiting enrollment approval)"
    )


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument(
        "--dry-run",
        action="store_true",
        help="Prompt and print without starting processes",
    )
    args = parser.parse_args()
    print(
        "Each operator uses its own identity directory. Provenance reads model files;"
    )
    print(
        "Observer should run on the Ollama/GPU host. Remote roles print launch commands."
    )
    plan = []
    storage = ollama = None
    for role in ROLES:
        system = prompt(
            f"{role.title()} system (localhost or remote host)", "localhost"
        )
        default_dir = (
            str(Path.home() / ".ollama/g8e" / role)
            if is_local(system)
            else f"~/.ollama/g8e/{role}"
        )
        directory = prompt(
            f"{role.title()} working directory (on {system})", default_dir
        )
        if is_local(system):
            directory = str(Path(directory).expanduser().resolve())
        plan.append((role, system, directory))
        if role == "provenance":
            storage_default = (
                model_root() if is_local(system) else "~/.ollama/models"
            )
            storage = prompt(
                f"Provenance Ollama models directory (on {system})", storage_default
            )
            if is_local(system):
                storage = str(Path(storage).expanduser().resolve())
        elif role == "inference":
            ollama = prompt(
                "Inference Ollama URL (Windows provider example: http://192.168.1.2:11434)",
                "http://localhost:11434",
            )
    # Sharing a cwd would share the operator's .g8e runtime and credentials.
    locations = [(system.lower(), directory) for _, system, directory in plan]
    local_dirs = [directory for _, system, directory in plan if is_local(system)]
    if len(set(locations)) != len(locations) or len(set(local_dirs)) != len(local_dirs):
        raise ValueError("Each operator needs a separate working directory")
    remote_gateway = "localhost"
    if any(not is_local(system) for _, system, _ in plan):
        remote_gateway = prompt(
            "Gateway hostname reachable from remote operators (TLS certificate name)",
            "g8e.local",
        )
    if not args.dry_run:
        try:
            subprocess.run(
                [sys.executable, "-c", "import app.serve"],
                cwd=ROOT / "ensemble",
                check=True,
            )
        except subprocess.CalledProcessError as exc:
            raise RuntimeError(
                "g8ee dependencies are missing; run make dev-python and retry"
            ) from exc
    for role, system, directory in plan:
        gateway = "localhost" if is_local(system) else remote_gateway
        command_args = operator_args(role, gateway, directory, storage, ollama)
        if is_local(system):
            start_local(
                role, directory, [str(ROOT / "g8e"), *command_args], args.dry_run
            )
        else:
            remote_commands(system, directory, command_args)
    ensemble_dir = ROOT / ".local.dev/full/ensemble"
    start_local(
        "g8ee",
        ensemble_dir,
        [
            sys.executable,
            "-m",
            "app.serve",
            "--host",
            "127.0.0.1",
            "--runtime-dir",
            str(ensemble_dir / ".g8e"),
            "--gateway-http-url",
            "http://localhost:8080",
            "--gateway-url",
            "https://localhost:8443",
            "--gateway-https-url",
            "https://localhost:8443",
            "--gateway-pubsub-url",
            "wss://localhost:8443",
        ],
        args.dry_run,
    )
    print("\nBootstrap owner if needed: ./g8e auth enroll user -e localhost")
    print("Review workload requests: ./g8e auth enroll pending")
    print("Approve each intended request: ./g8e auth enroll approve <request-id> --yes")
    print("g8ee becomes ready after approval: http://127.0.0.1:8000/health")
    print("To stop a local workload: kill $(cat <working-directory>/full.pid)")
    print("make down stops the Gateway; workload processes are separate.")


if __name__ == "__main__":
    # app.serve must remain importable when its cwd is the isolated runtime directory.
    os.environ["PYTHONPATH"] = (
        str(ROOT / "ensemble") + os.pathsep + os.environ.get("PYTHONPATH", "")
    )
    try:
        main()
    except (OSError, ValueError, RuntimeError, subprocess.SubprocessError) as exc:
        print(f"ERROR: {exc}", file=sys.stderr)
        sys.exit(1)
    except KeyboardInterrupt:
        sys.exit(130)
