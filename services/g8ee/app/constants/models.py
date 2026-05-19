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

from typing import Dict, Any
from pydantic import BaseModel, Field, ConfigDict

class G8eBaseModel(BaseModel):
    """Base model for protocol constants to avoid circular imports."""
    model_config = ConfigDict(
        populate_by_name=True,
        extra="ignore",
    )

class ProtocolConstantValue(G8eBaseModel):
    """A single constant value with its Go and Python names."""
    value: str
    go_const: str = Field(alias="_go_const")
    python_const: str = Field(alias="_python_const")

class CollectionsConstants(G8eBaseModel):
    """Canonical database collection names."""
    collections: Dict[str, ProtocolConstantValue]

class DocumentIdsConstants(G8eBaseModel):
    """Canonical document IDs and sentinel IDs."""
    document_ids: Dict[str, ProtocolConstantValue]
    sentinel_id: Dict[str, ProtocolConstantValue]

class ApiPathsConstants(G8eBaseModel):
    """Internal API paths for g8ee and client."""
    internal_prefix: str
    g8ee: Dict[str, str]
    client: Dict[str, str]

class StatusConstants(G8eBaseModel):
    """Canonical status values."""
    # We use Dict[str, Any] for nested status groups to keep the model flexible
    # while still providing a container for validation.
    model_config = ConfigDict(extra="allow")

class AgentsConstants(G8eBaseModel):
    """Canonical AI persona definitions and metadata."""
    model_config = ConfigDict(extra="allow")

class EventsConstants(G8eBaseModel):
    """Canonical event types."""
    model_config = ConfigDict(extra="allow")

class ChannelsConstants(G8eBaseModel):
    """Canonical database channel names."""
    model_config = ConfigDict(extra="allow")

class PubSubConstants(G8eBaseModel):
    """Canonical pubsub topics and actions."""
    model_config = ConfigDict(extra="allow")

class IntentsConstants(G8eBaseModel):
    """Canonical cloud intents."""
    model_config = ConfigDict(extra="allow")

class PromptsConstants(G8eBaseModel):
    """Canonical prompt file paths and AI mode constants."""
    model_config = ConfigDict(extra="allow")

class HeadersConstants(G8eBaseModel):
    """Canonical HTTP headers."""
    model_config = ConfigDict(extra="allow")

class PlatformConstants(G8eBaseModel):
    """Canonical platform constants."""
    model_config = ConfigDict(extra="allow")

class ErrorsConstants(G8eBaseModel):
    """Canonical error codes and categories."""
    model_config = ConfigDict(extra="allow")

class SendersConstants(G8eBaseModel):
    """Canonical message senders."""
    model_config = ConfigDict(extra="allow")

class KVKeysConstants(G8eBaseModel):
    """Canonical KV key schema."""
    cache_prefix: str = Field(alias="cache.prefix")
    key_schema: Dict[str, str] = Field(alias="key.schema")
    session_type: Dict[str, str] = Field(alias="session.type")

class SecurityConstraintsConstants(G8eBaseModel):
    """Canonical security constraints model."""
    system_path_prefixes: Dict[str, Any] = Field(default_factory=dict)
    high_risk_system_files: Dict[str, Any] = Field(default_factory=dict)
    forbidden_directories: Dict[str, Any] = Field(default_factory=dict)
    forbidden_command_patterns: Dict[str, Any] = Field(default_factory=dict)

    @classmethod
    def get_default_forbidden_patterns(cls) -> list[str]:
        return [
            "sudo",
            "su ",
            "su\t",
            "su -",
            "pkexec",
            "doas",
            "runas",
            "chmod +s",
            "chmod u+s",
            "chmod g+s",
            "setuid",
            "setgid",
            "eval ",
            "exec ",
            "$(",
            "`",
            "> /dev/sd",
            "> /dev/nvme",
            "> /dev/vd",
            "dd if=",
            "nsenter",
            "unshare",
            ":(){ :|:& };:",
            "| bash",
            "| sh",
            "| zsh",
            "| python",
            "| perl",
            "| php",
            "| ruby",
            "| node",
            "base64 -d",
            "base64 --decode",
            "rm -rf /",
            "rm -rf /etc",
            "rm -rf /var",
            "rm -rf /usr",
            "rm -rf /bin",
            "chmod 777",
            "chmod -R 777",
            "chmod a+rwx",
            "chown -R",
            "> /etc/passwd",
            "> /etc/shadow",
            "> /etc/sudoers",
            ">> /etc/passwd",
            ">> /etc/shadow",
            ">> /etc/sudoers",
            "iptables -F",
            "ufw disable",
            "history -c",
            "unset HISTFILE",
            "rm $0",
            "rm $BASH_SOURCE"
        ]

class InfraPaths(G8eBaseModel):
    """Infrastructure-related paths."""
    db_path: str
    ca_cert_path: str
    app_cert_dir: str
    pki_dir: str
    secrets_dir: str
    docs_dir: str
    protocol_dir: str
    protocol_constants_dir: str
    protocol_models_dir: str
    ssh_config_path: str

class G8eePaths(G8eBaseModel):
    """Paths specific to the g8ee service."""
    app_dir: str | None = None
    config_dir: str | None = None
    tests_dir: str | None = None
    cert_name: str
    evals: Dict[str, str] | None = None

class PathsConstants(G8eBaseModel):
    """Complete paths configuration from paths.json."""
    infra: InfraPaths
    g8ee: G8eePaths
    ports: Dict[str, int] | None = None
