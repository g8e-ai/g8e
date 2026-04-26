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

Minimal parser of the OpenSSH ``ssh_config`` format that extracts the subset
relevant to operator streaming: ``Host``, ``HostName``, ``User``, ``Port``.

Intentionally does NOT support: ``Match`` blocks, ``Include`` directives,
token expansion (``%h``, ``%p``), or canonicalisation. The AI is expected to
operate against the literal aliases the user wrote in their config.
"""

from __future__ import annotations

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
        """
        path = Path(self._ssh_config_path)
        if not path.exists():
            raise ConfigurationError(
                f"SSH config not found at {self._ssh_config_path}. Mount the host's "
                f"~/.ssh/config into the g8ee container before invoking ssh_inventory."
            )

        try:
            raw = path.read_text(encoding="utf-8", errors="replace")
        except OSError as exc:
            raise ConfigurationError(
                f"Failed to read SSH config at {self._ssh_config_path}: {exc}"
            ) from exc

        hosts = list(_parse_ssh_config(raw))
        logger.info(
            "[SSH_INVENTORY] Parsed %d host blocks from %s",
            len(hosts), self._ssh_config_path,
        )
        return SshInventory(source_path=self._ssh_config_path, hosts=hosts)


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


def _parse_ssh_config(raw: str):
    """Yield :class:`SshHost` instances for every ``Host`` block in *raw*.

    A ``Host`` line may declare multiple aliases (e.g. ``Host web-1 web-2``);
    each alias becomes a separate :class:`SshHost` entry sharing the block's
    ``HostName``/``User``/``Port`` values. The first non-wildcard alias drives
    ``hostname`` resolution; wildcard aliases are emitted with
    ``is_wildcard=True`` so the AI can surface them but never target them.
    """

    current_aliases: list[str] = []
    current_hostname: str | None = None
    current_user: str | None = None
    current_port: int | None = None

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

        if key == "host":
            # Flush the previous block before starting a new one.
            yield from flush()
            current_aliases = value.split()
            current_hostname = None
            current_user = None
            current_port = None
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
