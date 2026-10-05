# Copyright (c) 2026 Lateralus Labs, LLC.
# Use of this source code is governed by the Business Source License
# included in the LICENSE file.
#
# As of the Change Date listed in the LICENSE file, this software is
# released under the Apache License, Version 2.0.

"""Host stack launcher: unattended `make full`, interactive `make full-setup`."""

import argparse
import hashlib
import ipaddress
import json
import os
import re
import shlex
import signal
import ssl
import subprocess
import sys
import time
from pathlib import Path
from urllib.parse import urlsplit

ROOT = Path(__file__).resolve().parents[1]
ROLES = ("provenance", "observer", "inference", "data")
TRUST_PATH = Path(".g8e/pki/trust/g8eg-ca-bundle.pem")
WORKLOADS_PATH = Path(".local.dev/full/workloads.json")
LAUNCHES = {}
GATEWAY_HOST = "g8e.local"


# Names come from the canonical registry; unrelated .env settings never reach
# the launcher or get exported to its subprocesses.
ENV_REGISTRY = ROOT / "protocol/constants/env_vars.json"
LAUNCHER_ENV_FIELDS = (
    "Hostname",
    "OllamaEndpoint",
    *(role.title() + "Host" for role in ROLES),
)


def load_environment(path):
    try:
        from dotenv import dotenv_values
    except ImportError as exc:
        raise RuntimeError("python-dotenv is missing; run make dev-python") from exc
    registry = json.loads(ENV_REGISTRY.read_text())["env_vars"]
    values = dotenv_values(path, interpolate=False) if path.exists() else {}
    result = {}
    for field in LAUNCHER_ENV_FIELDS:
        entry = registry[field]
        if entry["category"] != "user_endpoint":
            raise ValueError(f"Invalid launcher env category for {field}")
        key = entry["value"]
        if key in os.environ:
            result[field] = os.environ[key].strip()
        elif key in values:
            result[field] = (values[key] or "").strip()
    return result


def validate_host(host):
    # A host identity, never a URL, port, shell expression, or arbitrary option.
    try:
        ipaddress.ip_address(host)
    except ValueError:
        if not re.fullmatch(r"[A-Za-z0-9](?:[A-Za-z0-9.-]*[A-Za-z0-9])?", host):
            raise ValueError(f"Expected a hostname or IP address, got {host!r}")
    return host


def url_host(host):
    return f"[{host}]" if ":" in host else host


def validate_ollama(endpoint):
    url = urlsplit(endpoint)
    if (
        url.scheme not in ("http", "https")
        or not url.hostname
        or url.username
        or url.password
        or url.query
        or url.fragment
    ):
        raise ValueError("G8E_OLLAMA_ENDPOINT must be an HTTP(S) provider URL")
    # Accessing port also validates its syntax and range.
    _ = url.port
    return endpoint


def certificate_fingerprints(pem):
    blocks = re.findall(
        r"-----BEGIN CERTIFICATE-----.*?-----END CERTIFICATE-----", pem, re.DOTALL
    )
    if not blocks:
        raise ValueError("CA file contains no certificates")
    return {
        hashlib.sha256(ssl.PEM_cert_to_DER_cert(block)).hexdigest() for block in blocks
    }


def local_workloads(plan):
    return [
        (role, Path(directory)) for role, system, directory in plan if is_local(system)
    ] + [("g8ee", ROOT / ".local.dev/full/ensemble")]


def prepare_identities(plan, reset=False, dry_run=False, interactive=True):
    """Trust changes are authorized from local disk, never from a TLS failure."""
    workloads = local_workloads(plan)
    root_path = ROOT / ".g8e/pki/root/root_ca.crt"
    if dry_run:
        print(
            "\nIdentity preflight: compare saved trust with the local Gateway CA before launch."
        )
        return
    current_root = certificate_fingerprints(root_path.read_text())
    bundle = (ROOT / TRUST_PATH).read_text()
    if not current_root.issubset(certificate_fingerprints(bundle)):
        raise RuntimeError("Local Gateway trust bundle does not contain its root CA")
    stale = []
    for role, directory in workloads:
        if (directory / ".g8e/pki/root/root_ca.crt").exists():
            raise RuntimeError(
                f"Workload directory contains a Gateway runtime: {directory}"
            )
        saved = directory / TRUST_PATH
        for path in [saved, *list(saved.parents)[:3]]:
            if path.is_symlink():
                raise RuntimeError(f"Refusing symlink trust path: {path}")
        identity = directory / (
            ".g8e/pki/issued/apps/g8ee.crt"
            if role == "g8ee"
            else ".g8e/pki/operator.crt"
        )
        pending = directory / (
            ".g8e/pki/pending-enrollment/g8ee.json"
            if role == "g8ee"
            else ".g8e/pki/pending-enrollment/g8eo.json"
        )
        if saved.exists():
            try:
                matches = current_root.issubset(
                    certificate_fingerprints(saved.read_text())
                )
            except ValueError:
                matches = False
            if not matches:
                stale.append((role, directory))
        elif identity.exists() or pending.exists():
            stale.append((role, directory))
    if stale:
        print(
            "\nGateway identity has changed or saved workload trust is missing/invalid."
        )
        print("These local workloads need fresh enrollment:")
        for role, directory in stale:
            print(f"  {role:<12} {directory}")
        print(
            "Working data, model files, vault keys, configuration, and logs will be preserved."
        )
        if not reset and not interactive:
            raise RuntimeError(
                "Stale workload identities require an explicit reset; "
                "retry with make full RESET_IDENTITIES=1 or use make full-setup"
            )
        if not reset and prompt(
            "Reset their identities and request fresh enrollment? (y/N)", "n"
        ).lower() not in ("y", "yes"):
            raise RuntimeError("Identity reset declined; no workloads launched")
        for role, directory in stale:
            command = [str(ROOT / "g8e")]
            if role == "g8ee":
                command += ["ensemble", "reset-identity", "--yes"]
            else:
                command += [
                    "operator",
                    "reset-identity",
                    "--working-dir",
                    str(directory),
                    "--yes",
                ]
            subprocess.run(command, cwd=ROOT, check=True)
    for _, directory in workloads:
        # Refuse symlinked runtime paths before installing local authoritative trust.
        target = directory / TRUST_PATH
        for path in [target, *list(target.parents)[:3]]:
            if path.is_symlink():
                raise RuntimeError(f"Refusing symlink trust path: {path}")
        target.parent.mkdir(parents=True, exist_ok=True)
        temporary = target.with_name("g8eg-ca-bundle.pem.new")
        fd = os.open(temporary, os.O_WRONLY | os.O_CREAT | os.O_EXCL, 0o600)
        try:
            with os.fdopen(fd, "w") as output:
                output.write(bundle)
            temporary.replace(target)
        finally:
            temporary.unlink(missing_ok=True)
    registry = ROOT / WORKLOADS_PATH
    registry.parent.mkdir(parents=True, exist_ok=True)
    if plan or not registry.exists():
        registry.write_text(
            json.dumps(
                [
                    {"role": role, "directory": str(directory)}
                    for role, directory in workloads
                ],
                indent=2,
            )
            + "\n"
        )


def workload_state(name, directory, offset=0):
    pid_file = directory / "full.pid"
    try:
        pid = int(pid_file.read_text().strip())
        running = pid > 0 and ensemble_running(pid)
    except (FileNotFoundError, ValueError):
        running = False
    log_path = directory / "full.log"
    if not running:
        return "failed", f"see {log_path}"
    with log_path.open("rb") as log:
        log.seek(offset)
        recent = log.read().decode(errors="replace")
    if "operator pub/sub WebSocket connected" in recent:
        return "connected", ""
    if "Application startup complete" in recent:
        return "ready", ""
    if (
        "request submitted" in recent
        or "approval url" in recent.lower()
        or "resuming pending" in recent
        or "polling for approval" in recent
    ):
        return "awaiting approval", "run ./g8e auth enroll pending"
    if "gateway not yet bootstrapped" in recent:
        return "awaiting enrollment", "run ./g8e auth enroll user -e g8e.local"
    return "starting", f"see {log_path}"


def report_workloads(timeout=20):
    deadline = time.monotonic() + timeout
    while True:
        states = [
            (name, *workload_state(name, directory, offset))
            for name, (directory, offset) in LAUNCHES.items()
        ]
        if (
            all(state != "starting" for _, state, _ in states)
            or time.monotonic() >= deadline
        ):
            break
        time.sleep(0.2)
    print("\nWorkload startup")
    for name, state, detail in states:
        print(f"  {name:<12} {state}" + (f"; {detail}" if detail else ""))
    return any(state == "failed" for _, state, _ in states)


def model_root():
    """Consult configuration/storage only; never start or probe Ollama."""
    # Consider Snap configuration first, but verify storage before choosing it.
    snap_models = None
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
                snap_models = Path(result.stdout.strip()).expanduser()
        except (OSError, subprocess.TimeoutExpired):
            # Snap query is best-effort; if unavailable or slow, continue with fallback paths.
            pass
    candidates = [
        Path("/var/snap/ollama/common/models"),
        Path("/usr/share/ollama/.ollama/models"),
        Path.home() / ".ollama/models",
    ]
    if snap_models is not None:
        candidates.insert(0, snap_models)
    # Windows Ollama storage may be exposed through WSL drive mounts.
    drives = sorted(
        drive
        for drive in Path("/mnt").glob("*")
        if len(drive.name) == 1 and drive.name in "abcdefghijklmnopqrstuvwxyz"
    )
    candidates.extend(drive / "ai/Ollama/models" for drive in drives)
    for drive in drives:
        candidates.extend(sorted(drive.glob("Users/*/.ollama/models")))
    # Empty Snap manifests/blobs directories should not hide a populated store.
    for path in candidates:
        if (path / "blobs").is_dir() and any(
            manifest.is_file() for manifest in (path / "manifests").rglob("*")
        ):
            return str(path)
    for path in candidates:
        if (path / "manifests").is_dir() and (path / "blobs").is_dir():
            return str(path)
    for path in candidates:
        if path.is_dir():
            return str(path)
    return str(Path.home() / ".ollama/models")


def prompt(label, default):
    try:
        return input(f"{label} [{default}]: ").strip() or default
    except EOFError as exc:
        raise ValueError(
            "make full-setup requires interactive input; use --dry-run to preview commands"
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
    elif role == "observer":
        args += [
            "--provider-boundary-observer-enabled",
            "--provider-boundary-observer-id",
            "g8e-provider-boundary-observer",
        ]
    elif role == "data":
        pass
    return args


def powershell_quote(value):
    return "'" + value.replace("'", "''") + "'"


def remote_commands(system, directory, args):
    args = [*args, "--trust-bundle", "gateway-ca-bundle.pem"]
    print(f"\nRun on {system} with a g8e binary built for that host.")
    print(
        "Use a Gateway hostname/IP reachable from that host and matching its TLS certificate."
    )
    print(
        "Copy the Gateway CA bundle into this working directory as gateway-ca-bundle.pem using a trusted channel; verify its fingerprint independently."
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
    if dry_run:
        print(f"\n{name}: {shlex.join(command)}\n  working directory: {directory}")
        return
    directory.mkdir(parents=True, exist_ok=True)
    pid_file = directory / "full.pid"
    if pid_file.exists():
        try:
            pid = int(pid_file.read_text().strip())
            if pid <= 0 or not ensemble_running(pid):
                raise ProcessLookupError
        except (ProcessLookupError, ValueError):
            pass
        else:
            LAUNCHES[name] = (directory, 0)
            print(f"  {name:<12} running; checking startup")
            return
    log_path = directory / "full.log"
    offset = log_path.stat().st_size if log_path.exists() else 0
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
    LAUNCHES[name] = (directory, offset)
    print(f"  {name:<12} started; checking startup")


def start_ensemble(dry_run=False, gateway=GATEWAY_HOST):
    ensemble_dir = ROOT / ".local.dev/full/ensemble"
    gateway = url_host(gateway)
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
            f"http://{gateway}:8080",
            "--gateway-url",
            f"https://{gateway}:8443",
            "--gateway-https-url",
            f"https://{gateway}:8443",
            "--gateway-pubsub-url",
            f"wss://{gateway}:8443",
        ],
        dry_run,
    )


def ensemble_running(pid):
    # Linux signal-0 also sees zombies; treat them as stopped.
    stat = Path(f"/proc/{pid}/stat")
    try:
        if stat.read_text().rsplit(")", 1)[1].split()[0] == "Z":
            return False
    except FileNotFoundError:
        # Process may have exited between checks; defer to os.kill(pid, 0) below.
        pass
    try:
        os.kill(pid, 0)
    except ProcessLookupError:
        return False
    return True


def stop_ensemble():
    pid_file = ROOT / ".local.dev/full/ensemble/full.pid"
    if not pid_file.exists():
        print("  g8ee       stopped (host)")
        return
    pid = int(pid_file.read_text().strip())
    if pid <= 0:
        raise ValueError(f"Invalid ensemble PID in {pid_file}")
    if ensemble_running(pid):
        cmdline = Path(f"/proc/{pid}/cmdline")
        if cmdline.exists() and b"app.serve" not in cmdline.read_bytes().split(b"\0"):
            raise RuntimeError(f"PID {pid} is not the ensemble; refusing to stop it")
        os.kill(pid, signal.SIGTERM)
        deadline = time.monotonic() + 10
        while ensemble_running(pid):
            if time.monotonic() >= deadline:
                raise RuntimeError(
                    "g8ee did not stop within 10 seconds; check ensemble logs"
                )
            time.sleep(0.1)
    pid_file.unlink(missing_ok=True)
    print("  g8ee       stopped (host)")


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument(
        "--dry-run",
        action="store_true",
        help="Print without starting processes or writing runtime state",
    )
    parser.add_argument(
        "--setup", action="store_true", help="Prompt for operator settings"
    )
    parser.add_argument("--env-file", type=Path, default=ROOT / ".env")
    parser.add_argument("--start-gateway", action="store_true")
    parser.add_argument("--model-storage-root", help="Explicit model store path")
    for role in ROLES:
        parser.add_argument(
            f"--{role}-working-dir", help=f"Explicit {role} runtime directory"
        )
    parser.add_argument("--ensemble-action", choices=("start", "stop", "restart"))
    parser.add_argument(
        "--reset-identities",
        action="store_true",
        help="Confirm reset of stale identities in the selected local workload directories",
    )
    args = parser.parse_args()
    if args.ensemble_action:
        gateway = GATEWAY_HOST
        if args.ensemble_action in ("start", "restart"):
            gateway = validate_host(
                load_environment(args.env_file).get("Hostname", GATEWAY_HOST)
            )
        if args.dry_run and args.ensemble_action in ("stop", "restart"):
            print("g8ee: stop the local Python process")
            if args.ensemble_action == "restart":
                start_ensemble(True, gateway)
            return
        if args.ensemble_action in ("stop", "restart"):
            stop_ensemble()
        if args.ensemble_action in ("start", "restart"):
            prepare_identities([], args.reset_identities, args.dry_run)
            start_ensemble(args.dry_run, gateway)
            if not args.dry_run and report_workloads():
                raise RuntimeError("g8ee failed during startup; see its log")
        return
    environment = {} if args.setup else load_environment(args.env_file)
    gateway_host = validate_host(environment.get("Hostname", GATEWAY_HOST))
    if not args.setup:
        if not environment.get("OllamaEndpoint"):
            raise ValueError(
                "Set G8E_OLLAMA_ENDPOINT in .env or the process environment"
            )
        validate_ollama(environment["OllamaEndpoint"])
        systems = {
            role: validate_host(environment.get(role.title() + "Host", "localhost"))
            for role in ROLES
        }
    if args.setup:
        print("\nSet up operators · press Enter to use the defaults.")
        print("Choose localhost or a remote host for each role.")
    plan = []
    storage = ollama = None
    descriptions = {
        "provenance": "tracks model files; place it where models are stored",
        "observer": "observes provider activity; place it on the Ollama/GPU host",
        "inference": "runs inference",
        "data": "executes governed tools and file operations; place it on the target host",
    }
    for role in ROLES:
        if args.setup:
            print(f"\n{role.title()} · {descriptions[role]}")
            system = prompt(
                f"{role.title()} system (localhost or remote host)", "localhost"
            )
        else:
            system = systems[role]
        default_dir = (
            str(Path.home() / ".ollama/g8e" / role)
            if is_local(system)
            else f"~/.ollama/g8e/{role}"
        )
        directory = getattr(args, role + "_working_dir") or default_dir
        if args.setup:
            directory = prompt(
                f"{role.title()} working directory (on {system})", directory
            )
        if not directory.strip():
            raise ValueError(f"{role} working directory must not be empty")
        if is_local(system):
            directory = str(Path(directory).expanduser().resolve())
        plan.append((role, system, directory))
        if role == "provenance":
            storage_default = model_root() if is_local(system) else "~/.ollama/models"
            storage = args.model_storage_root or storage_default
            if args.setup:
                storage = prompt(
                    f"Provenance Ollama models directory (on {system})", storage
                )
            if is_local(system):
                storage = str(Path(storage).expanduser().resolve())
        elif role == "inference":
            ollama = (
                prompt(
                    "Inference Ollama URL (Windows provider example: http://192.168.1.2:11434)",
                    "http://localhost:11434",
                )
                if args.setup
                else environment["OllamaEndpoint"]
            )
            validate_ollama(ollama)
    # Sharing a cwd would share the operator's .g8e runtime and credentials.
    locations = [(system.lower(), directory) for _, system, directory in plan]
    local_dirs = [directory for _, system, directory in plan if is_local(system)]
    if len(set(locations)) != len(locations) or len(set(local_dirs)) != len(local_dirs):
        raise ValueError("Each operator needs a separate working directory")
    remote_gateway = gateway_host
    if args.setup and any(not is_local(system) for _, system, _ in plan):
        remote_gateway = prompt(
            "Gateway hostname reachable from remote operators (TLS certificate name)",
            GATEWAY_HOST,
        )
    if not args.dry_run:
        try:
            subprocess.run(
                [sys.executable, "-c", "import app.serve"],
                cwd=ROOT / "ensemble",
                check=True,
                capture_output=True,
                text=True,
            )
        except subprocess.CalledProcessError as exc:
            raise RuntimeError(
                "g8ee dependencies are missing; run make dev-python and retry"
            ) from exc
    if args.start_gateway:
        gateway_command = [str(ROOT / "g8e"), "gw", "start", "--quiet"]
        if not args.setup:
            gateway_command += [
                "--public-base-url",
                f"https://{url_host(gateway_host)}:8443",
                "--passkey-rp-id",
                gateway_host,
                "--passkey-rp-origin",
                f"https://{url_host(gateway_host)}:8443",
                "--cors-origin",
                f"https://{url_host(gateway_host)}:8443",
            ]
        if args.dry_run:
            print("Gateway: " + shlex.join(gateway_command))
        else:
            subprocess.run(gateway_command, cwd=ROOT, check=True)
    prepare_identities(
        plan, args.reset_identities, args.dry_run, interactive=args.setup
    )
    print("\nLaunching workloads" if not args.dry_run else "\nLaunch preview")
    for role, system, directory in plan:
        gateway = gateway_host if is_local(system) else remote_gateway
        command_args = operator_args(role, gateway, directory, storage, ollama)
        if is_local(system):
            start_local(
                role, directory, [str(ROOT / "g8e"), *command_args], args.dry_run
            )
        else:
            remote_commands(system, directory, command_args)
    start_ensemble(args.dry_run, gateway_host)
    failed = report_workloads() if not args.dry_run else False
    print_summary(
        args.dry_run, any(not is_local(system) for _, system, _ in plan), gateway_host
    )
    if failed:
        raise RuntimeError(
            "One or more workloads failed during startup; see the logs listed above"
        )


def print_summary(dry_run=False, has_remote=False, gateway=GATEWAY_HOST):
    print(
        "\nPlatform status"
        if not dry_run
        else "\nPreview complete · no workloads started"
    )
    if not dry_run:
        for command, label in (
            (["gw", "status", "--brief"], "Gateway / operators"),
            (["ensemble", "status"], "g8ee"),
        ):
            try:
                result = subprocess.run(
                    [str(ROOT / "g8e"), *command],
                    cwd=ROOT,
                    capture_output=True,
                    text=True,
                    timeout=10,
                    check=False,
                )
                if result.returncode == 0:
                    print(result.stdout.rstrip())
                else:
                    print(
                        f"  {label:<12} status unavailable; run ./g8e {shlex.join(command)}"
                    )
            except (OSError, subprocess.TimeoutExpired):
                print(
                    f"  {label:<12} status unavailable; run ./g8e {shlex.join(command)}"
                )
    if has_remote:
        print("  Remote roles need the launch commands above run on their hosts.")
    print("\nManage with g8e (from this repository)")
    print("  ./g8e gw status                  Gateway and connected operators")
    print("  ./g8e ensemble status            g8ee readiness")
    print("  ./g8e operator list              Operator IDs and sessions")
    print("  ./g8e operator show <id>         Operator details")
    print("  ./g8e ensemble logs              g8ee startup issues")
    print("\nIf workloads need approval")
    print(f"  ./g8e auth enroll user -e {gateway}   Enroll your CLI identity first")
    print("  ./g8e auth enroll pending             Review requests")
    print("  ./g8e auth enroll approve <id> --yes   Approve an intended request")
    print(f"\nConsole: https://{url_host(gateway)}:8443/console/")


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
