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

"""Hierarchical SSH inventory: environment -> group -> host.

The OpenSSH config format has no native concept of environments or groups,
only ``Host`` blocks (which may be wildcards). To allow the AI to target
fleets at granular levels we keep ``environment`` and ``group`` as first-class
fields on every ``SshHost`` even though the SSH config parser cannot populate
them today. They are reserved for the planned future flow where users define
the hierarchy explicitly through the g8e UI on top of their existing SSH
config aliases.

Until that UI exists, parsed hosts have ``environment=None`` and ``group=None``
and the inventory exposes them only via the flat ``hosts`` list.
"""

from __future__ import annotations

from pydantic import Field

from .base import G8eBaseModel


class SshHost(G8eBaseModel):
    """A single SSH target derived from ~/.ssh/config or a future g8e-managed inventory.

    ``host`` is the user-facing alias (the ``Host`` directive value).
    ``hostname`` is the resolved DNS name or IP (the ``HostName`` directive),
    falling back to ``host`` when no explicit ``HostName`` was provided.

    ``environment`` and ``group`` are reserved for explicit user-defined
    organisation; the SSH-config parser leaves them ``None``. Wildcards in
    ``Host`` patterns are intentionally NOT inferred as groups -- the user
    must define groupings explicitly.
    """

    host: str = Field(..., description="SSH alias (Host directive)")
    hostname: str = Field(..., description="Resolved hostname or IP (HostName directive, or host if absent)")
    user: str | None = Field(default=None, description="Login user from User directive, if set")
    port: int | None = Field(default=None, description="SSH port from Port directive, if set")
    environment: str | None = Field(
        default=None,
        description="User-defined environment label (e.g. 'prod', 'staging'). Not inferred from SSH config.",
    )
    group: str | None = Field(
        default=None,
        description="User-defined group label within an environment (e.g. 'web', 'db'). Not inferred from SSH config.",
    )
    is_wildcard: bool = Field(
        default=False,
        description="True when the Host directive is a wildcard pattern (e.g. 'Host *', 'Host web-*'). "
        "Wildcards cannot be streamed to directly; they must be resolved by the user against concrete hosts.",
    )


class SshInventory(G8eBaseModel):
    """Parsed SSH inventory exposed to the AI ssh_inventory tool.

    Hosts are reported in the order they appear in the SSH config. Wildcard
    Host blocks are included with ``is_wildcard=True`` so the model can show
    the user what patterns exist, but they cannot be selected as stream targets.
    """

    source_path: str = Field(..., description="Filesystem path the inventory was parsed from")
    hosts: list[SshHost] = Field(default_factory=list)

    @property
    def streamable_hosts(self) -> list[SshHost]:
        """Concrete (non-wildcard) hosts the AI may reference as stream targets."""
        return [h for h in self.hosts if not h.is_wildcard]
