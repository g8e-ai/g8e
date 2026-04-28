# Copyright (c) 2026 Lateralus Labs, LLC.
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#     http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.

"""Read-only SSH inventory parser.

Parser of the OpenSSH ``ssh_config`` format that extracts the subset
relevant to operator streaming: ``Host``, ``HostName``, ``User``, ``Port``.

Supports ``Include`` directives for modular config files and ``Match`` blocks
for conditional configuration. Token expansion (``%h``, ``%p``) and
canonicalisation are not supported; the AI operates against the literal aliases
the user wrote in their config.
"""

from __future__ import annotations

import functools
import glob
import logging
import os
from pathlib import Path

from app.errors import ConfigurationError
from app.models.ssh_inventory import SshHost, SshInventory

logger = logging.getLogger(__name__)


_WILDCARD_CHARS = frozenset("*?!")


class SshInventoryService:
    """Parses the SSH config mounted into the g8ee container."""

    def __init__(self, ssh_config_path: str) -> None:
        self._ssh_config_path = ssh_config_path

    @property
    def source_path(self) -> str:
        return self._ssh_config_path

    def load(self) -> SshInventory:
        """Parse the configured SSH config file and return an :class:`SshInventory`.

        Raises :class:`ConfigurationError` when the file cannot be read. An
        empty (zero-byte) file is valid and yields an empty inventory -- this
        is the expected steady-state for users who have not yet populated
        ``~/.ssh/config`` on their host.

        Results are cached based on file modification time to avoid re-parsing
        on every call during Phase 2c turn-by-turn invocation.
        """
        path = Path(self._ssh_config_path)
        if not path.exists():
            raise ConfigurationError(
                f"SSH config not found at {self._ssh_config_path}. Mount the host's "
                f"~/.ssh/config into the g8ee container before invoking ssh_inventory."
            )

        try:
            mtime = path.stat().st_mtime
            # Track hash of all mtimes (main file + included files)
            mtime_hash = self._compute_mtime_hash(path)
        except OSError as exc:
            raise ConfigurationError(
                f"Failed to stat SSH config or includes at {self._ssh_config_path}: {exc}"
            ) from exc

        return _load_cached(self._ssh_config_path, mtime_hash)

    def _compute_mtime_hash(self, path: Path, seen: set[str] | None = None) -> float:
        """Recursively compute a combined mtime 'hash' (sum) for the config and its includes."""
        if seen is None:
            seen = set()
            
        canonical = str(path.resolve())
        if canonical in seen:
            return 0.0
        seen.add(canonical)
        
        try:
            total_mtime = path.stat().st_mtime
            raw = path.read_text(encoding="utf-8", errors="replace")
            
            for line in raw.splitlines():
                kv = _split_kv(line)
                if kv and kv[0] == "include":
                    include_paths = _resolve_include_path(kv[1], path.parent)
                    for include_path in include_paths:
                        if include_path.exists():
                            total_mtime += self._compute_mtime_hash(include_path, seen)
            return total_mtime
        except (OSError, ValueError):
            return 0.0


@functools.lru_cache(maxsize=32)
def _load_cached(ssh_config_path: str, mtime: float) -> SshInventory:
    """Cached SSH config loader keyed by path and modification time."""
    path = Path(ssh_config_path)

    try:
        raw = path.read_text(encoding="utf-8", errors="replace")
    except OSError as exc:
        raise ConfigurationError(
            f"Failed to read SSH config at {ssh_config_path}: {exc}"
        ) from exc

    hosts = list(_parse_ssh_config(raw, config_dir=path.parent))
    logger.info(
        "[SSH_INVENTORY] Parsed %d host blocks from %s",
        len(hosts), ssh_config_path,
    )
    return SshInventory(source_path=ssh_config_path, hosts=hosts)


def _is_wildcard(pattern: str) -> bool:
    return any(ch in _WILDCARD_CHARS for ch in pattern)


def _split_kv(line: str) -> tuple[str, str] | None:
    """Split an OpenSSH config line into (lower-cased key, value).

    Supports both ``Key Value`` and ``Key=Value`` forms. Returns ``None`` for
    blank lines, comments, or malformed entries.
    """
    stripped = line.strip()
    if not stripped or stripped.startswith("#"):
        return None
    # Strip trailing '# ...' comments. OpenSSH itself does not document
    # trailing comments, but tolerating them keeps hand-edited configs from
    # silently producing phantom aliases on Host lines.
    hash_idx = stripped.find("#")
    if hash_idx != -1:
        stripped = stripped[:hash_idx].rstrip()
        if not stripped:
            return None
    # Normalise '=' separators to whitespace.
    normalised = stripped.replace("=", " ", 1)
    parts = normalised.split(None, 1)
    if len(parts) != 2:
        return None
    key, value = parts
    return key.lower(), value.strip()


def _resolve_include_path(pattern: str, config_dir: Path) -> list[Path]:
    """Resolve an Include directive pattern to a list of file paths.

    Supports both absolute paths and paths relative to the config directory.
    Supports glob patterns (e.g., ``config.d/*`` or ``~/.ssh/config.d/*``).
    Expands ``~`` to the user's home directory.

    Returns an empty list if no files match the pattern.
    """
    # Expand ~ to home directory
    expanded = os.path.expanduser(pattern)
    
    # If the path is absolute, use it as-is; otherwise, make it relative to config_dir
    if os.path.isabs(expanded):
        glob_pattern = expanded
    else:
        glob_pattern = str(config_dir / expanded)
    
    # Resolve the glob pattern
    matched = glob.glob(glob_pattern)
    
    # Sort for deterministic ordering
    return sorted(Path(p) for p in matched)


def _parse_ssh_config(
    raw: str,
    config_dir: Path,
    included_files: set[str] | None = None,
):
    """Yield :class:`SshHost` instances for every ``Host`` block in *raw*.

    A ``Host`` line may declare multiple aliases (e.g. ``Host web-1 web-2``);
    each alias becomes a separate :class:`SshHost` entry sharing the block's
    ``HostName``/``User``/``Port`` values. The first non-wildcard alias drives
    ``hostname`` resolution; wildcard aliases are emitted with
    ``is_wildcard=True`` so the AI can surface them but never target them.

    Supports ``Include`` directives to recursively parse additional config files.
    Supports ``Match`` blocks for conditional configuration; hosts within Match
    blocks are included in the inventory.

    Args:
        raw: The SSH config file content as a string.
        config_dir: Directory containing the SSH config file (for resolving Include paths).
        included_files: Set of already-included file paths to prevent circular includes.
    """
    if included_files is None:
        included_files = set()

    current_aliases: list[str] = []
    current_hostname: str | None = None
    current_user: str | None = None
    current_port: int | None = None
    in_match_block = False

    def flush():
        for alias in current_aliases:
            wild = _is_wildcard(alias)
            yield SshHost(
                host=alias,
                hostname=current_hostname or alias,
                user=current_user,
                port=current_port,
                is_wildcard=wild,
            )

    for line in raw.splitlines():
        kv = _split_kv(line)
        if kv is None:
            continue
        key, value = kv

        if key == "include":
            # Handle Include directive
            include_paths = _resolve_include_path(value, config_dir)
            for include_path in include_paths:
                # Prevent circular includes
                canonical_path = str(include_path.resolve())
                if canonical_path in included_files:
                    logger.debug(
                        "[SSH_INVENTORY] Skipping circular include: %s",
                        canonical_path,
                    )
                    continue
                
                included_files.add(canonical_path)
                
                try:
                    included_raw = include_path.read_text(encoding="utf-8", errors="replace")
                    yield from _parse_ssh_config(
                        included_raw,
                        config_dir=include_path.parent,
                        included_files=included_files,
                    )
                except OSError as exc:
                    logger.warning(
                        "[SSH_INVENTORY] Failed to read included file %s: %s",
                        include_path, exc,
                    )
            continue

        if key == "match":
            # Start of a Match block - flush any pending Host block
            yield from flush()
            current_aliases = []
            current_hostname = None
            current_user = None
            current_port = None
            in_match_block = True
            continue

        if key == "host":
            # Flush the previous block before starting a new one.
            yield from flush()
            current_aliases = value.split()
            current_hostname = None
            current_user = None
            current_port = None
            in_match_block = False
            continue

        if not current_aliases:
            # Directive outside any Host block (global defaults). We deliberately
            # ignore these because they apply to wildcard '*' anyway and have no
            # bearing on alias enumeration.
            continue

        if key == "hostname":
            current_hostname = value
        elif key == "user":
            current_user = value
        elif key == "port":
            try:
                current_port = int(value)
            except ValueError:
                # Malformed port entries are tolerated; leave port unset rather
                # than failing the whole inventory parse.
                logger.warning(
                    "[SSH_INVENTORY] Ignoring malformed Port=%r in block %s",
                    value, current_aliases,
                )

    yield from flush()


def default_ssh_inventory_service() -> SshInventoryService:
    """Construct an :class:`SshInventoryService` using the canonical mount path.

    The path is overridable via the ``G8E_SSH_CONFIG_PATH`` env var so tests
    can point the service at a fixture without mocking.
    """
    path = os.environ.get("G8E_SSH_CONFIG_PATH", "/etc/g8e/ssh_config")
    return SshInventoryService(ssh_config_path=path)
