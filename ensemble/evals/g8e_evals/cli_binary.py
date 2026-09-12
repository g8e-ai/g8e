# Copyright (c) 2026 Lateralus Labs, LLC.
# Use of this source code is governed by the Business Source License
# included in the LICENSE file.
#
# As of the Change Date listed in the LICENSE file, this software is
# released under the Apache License, Version 2.0.

"""Resolution of the g8e platform CLI binary for eval commands.

The evals never build the platform binary; they resolve one the trusted
build already produced. Resolution order for ``--g8e-cli``/``G8E_CLI_BIN``:

1. An explicit path (must exist and be executable).
2. ``auto`` — fetch the platform binary served by the running Gateway at
   ``/.well-known/g8e/bin/g8e-<os>-<arch>`` (unauthenticated discovery
   surface; the artifact's self-stamp and recorded sha256 provide the
   evidence binding). This is the ``docker compose up -d`` path: the image
   bakes the binaries via ``make build-all`` and the Gateway serves them,
   so evals need no local Go toolchain.
3. Implicit candidates in order: the repo-root ``./g8e`` produced by
   ``make build``, the platform binary under ``bin/``, then ``g8e`` on
   ``PATH``. Implicit resolution returns ``None`` when nothing exists so
   callers keep the environment-variable provenance fallback.

The resolved binary is self-describing: ``g8e version --json`` reports
the Makefile provenance stamp consumed by ``provenance_bridge``.
"""

from __future__ import annotations

import os
import platform
import shutil
import sys
from email.utils import formatdate, parsedate_to_datetime
from pathlib import Path

import httpx

from g8e.constants import NETWORK


class CLIBinaryError(Exception):
    """An explicitly requested g8e CLI binary cannot be resolved."""


AUTO_SENTINEL = "auto"
ENV_GATEWAY_HTTP_URL = "G8E_GATEWAY_HTTP_URL"
DEFAULT_GATEWAY_HTTP_BASE = NETWORK["network"]["GatewayHTTPBase"]["value"]

# ensemble/evals/g8e_evals/cli_binary.py -> repository root.
REPO_ROOT = Path(__file__).resolve().parents[3]

DEFAULT_CACHE_DIR = Path.home() / ".cache" / "g8e-evals" / "bin"

_GOOS_BY_SYS_PLATFORM = {
    "linux": "linux",
    "darwin": "darwin",
    "win32": "windows",
    "cygwin": "windows",
}
_GOARCH_BY_MACHINE = {
    "x86_64": "amd64",
    "amd64": "amd64",
    "aarch64": "arm64",
    "arm64": "arm64",
    "i386": "386",
    "i686": "386",
    "x86": "386",
}


def platform_binary_name() -> str:
    """Return the canonical ``g8e-<goos>-<goarch>`` artifact name for this host."""
    goos = _GOOS_BY_SYS_PLATFORM.get(sys.platform)
    if goos is None:
        raise CLIBinaryError(f"unsupported host OS '{sys.platform}'")
    goarch = _GOARCH_BY_MACHINE.get(platform.machine().lower())
    if goarch is None:
        raise CLIBinaryError(f"unsupported host architecture '{platform.machine()}'")
    name = f"g8e-{goos}-{goarch}"
    if goos == "windows":
        name += ".exe"
    return name


def _candidate_paths() -> list[Path]:
    """Implicit resolution candidates, in precedence order."""
    root_copy = "g8e.exe" if sys.platform in ("win32", "cygwin") else "g8e"
    candidates = [REPO_ROOT / root_copy]
    try:
        candidates.append(REPO_ROOT / "bin" / platform_binary_name())
    except CLIBinaryError:
        pass
    on_path = shutil.which("g8e")
    if on_path:
        candidates.append(Path(on_path))
    return candidates


def _is_usable_binary(path: Path) -> bool:
    return path.is_file() and (sys.platform in ("win32", "cygwin") or os.access(path, os.X_OK))


def fetch_g8e_binary(
    gateway_base_url: str,
    cache_dir: Path,
    *,
    timeout_s: float = 120.0,
) -> Path:
    """Fetch the host platform binary from the Gateway's well-known endpoint.

    Uses a conditional GET: when a cached binary exists its mtime is sent
    as ``If-Modified-Since`` so an unchanged artifact returns 304 and the
    cache is reused. On 200 the body is written, marked executable, and
    stamped with the response ``Last-Modified`` time.
    """
    name = platform_binary_name()
    url = f"{gateway_base_url.rstrip('/')}/.well-known/g8e/bin/{name}"
    cache_dir.mkdir(parents=True, exist_ok=True)
    cached = cache_dir / name

    headers: dict[str, str] = {}
    if cached.is_file():
        headers["If-Modified-Since"] = formatdate(cached.stat().st_mtime, usegmt=True)

    try:
        response = httpx.get(url, headers=headers, timeout=timeout_s, follow_redirects=True)
    except httpx.HTTPError as error:
        raise CLIBinaryError(f"could not fetch {url}: {error}") from error

    if response.status_code == 304 and cached.is_file():
        return cached
    if response.status_code != 200:
        raise CLIBinaryError(
            f"gateway returned HTTP {response.status_code} for {url}; "
            "the Gateway serves platform binaries only when built via 'make build-all' (docker compose image) or 'make build' on the host"
        )

    cached.write_bytes(response.content)
    cached.chmod(0o755)
    last_modified = response.headers.get("Last-Modified")
    if last_modified:
        try:
            mtime = parsedate_to_datetime(last_modified).timestamp()
            os.utime(cached, (mtime, mtime))
        except (TypeError, ValueError):
            pass
    return cached


def resolve_g8e_cli(
    requested: str | None,
    *,
    gateway_base_url: str | None = None,
    cache_dir: Path | None = None,
) -> str | None:
    """Resolve the g8e CLI binary path for eval commands.

    ``requested`` is the ``--g8e-cli``/``G8E_CLI_BIN`` value. ``None`` or
    empty triggers implicit candidate resolution, which returns ``None``
    when no binary exists (callers then fall back to environment-variable
    provenance). An explicit path must exist and be executable. The
    ``auto`` sentinel fetches the binary from the running Gateway.

    Raises ``CLIBinaryError`` for explicit requests that cannot resolve.
    """
    if requested is None or not requested.strip():
        for candidate in _candidate_paths():
            if _is_usable_binary(candidate):
                return str(candidate)
        return None

    requested = requested.strip()
    if requested == AUTO_SENTINEL:
        base = gateway_base_url or os.environ.get(ENV_GATEWAY_HTTP_URL, "").strip() or DEFAULT_GATEWAY_HTTP_BASE
        return str(fetch_g8e_binary(base, cache_dir or DEFAULT_CACHE_DIR))

    path = Path(requested).expanduser()
    if not path.is_file():
        raise CLIBinaryError(f"g8e CLI binary not found: {requested}")
    if sys.platform not in ("win32", "cygwin") and not os.access(path, os.X_OK):
        raise CLIBinaryError(f"g8e CLI binary is not executable: {requested}")
    return str(path)
