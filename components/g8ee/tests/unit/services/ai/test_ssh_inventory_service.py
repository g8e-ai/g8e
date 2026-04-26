# Copyright (c) 2026 Lateralus Labs, LLC.
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#     http://www.apache.org/licenses/LICENSE-2.0

from pathlib import Path

import pytest

from app.errors import ConfigurationError
from app.services.ai.ssh_inventory_service import (
    SshInventoryService,
    default_ssh_inventory_service,
)


def _write(tmp_path: Path, body: str) -> Path:
    cfg = tmp_path / "ssh_config"
    cfg.write_text(body, encoding="utf-8")
    return cfg


def test_load_missing_file_raises_configuration_error(tmp_path: Path) -> None:
    svc = SshInventoryService(ssh_config_path=str(tmp_path / "does-not-exist"))
    with pytest.raises(ConfigurationError, match="SSH config not found"):
        svc.load()


def test_load_empty_file_returns_empty_inventory(tmp_path: Path) -> None:
    cfg = _write(tmp_path, "")
    inv = SshInventoryService(ssh_config_path=str(cfg)).load()
    assert inv.source_path == str(cfg)
    assert inv.hosts == []
    assert inv.streamable_hosts == []


def test_basic_host_block(tmp_path: Path) -> None:
    cfg = _write(
        tmp_path,
        """
        Host bastion
            HostName 10.0.0.1
            User ops
            Port 2222
        """,
    )
    inv = SshInventoryService(ssh_config_path=str(cfg)).load()
    assert len(inv.hosts) == 1
    h = inv.hosts[0]
    assert h.host == "bastion"
    assert h.hostname == "10.0.0.1"
    assert h.user == "ops"
    assert h.port == 2222
    assert h.is_wildcard is False
    assert h.environment is None
    assert h.group is None


def test_host_with_no_explicit_hostname_falls_back_to_alias(tmp_path: Path) -> None:
    cfg = _write(tmp_path, "Host web1\n")
    inv = SshInventoryService(ssh_config_path=str(cfg)).load()
    assert inv.hosts[0].hostname == "web1"


def test_multiple_aliases_on_one_host_line_split_into_separate_entries(tmp_path: Path) -> None:
    cfg = _write(
        tmp_path,
        """
        Host web-1 web-2 web-3
            HostName 10.1.1.1
            User deploy
        """,
    )
    inv = SshInventoryService(ssh_config_path=str(cfg)).load()
    aliases = [h.host for h in inv.hosts]
    assert aliases == ["web-1", "web-2", "web-3"]
    # Block-level fields apply to every alias in the block.
    for h in inv.hosts:
        assert h.hostname == "10.1.1.1"
        assert h.user == "deploy"


def test_wildcard_aliases_marked_and_excluded_from_streamable(tmp_path: Path) -> None:
    cfg = _write(
        tmp_path,
        """
        Host *
            User defaultuser

        Host prod-*
            HostName 10.0.%h.1

        Host db1
            HostName 10.2.0.5
        """,
    )
    inv = SshInventoryService(ssh_config_path=str(cfg)).load()

    by_host = {h.host: h for h in inv.hosts}
    assert by_host["*"].is_wildcard is True
    assert by_host["prod-*"].is_wildcard is True
    assert by_host["db1"].is_wildcard is False

    streamable = [h.host for h in inv.streamable_hosts]
    assert streamable == ["db1"]


def test_equals_separator_is_supported(tmp_path: Path) -> None:
    cfg = _write(
        tmp_path,
        """
        Host edge
            HostName=edge.example.com
            User=admin
            Port=22022
        """,
    )
    inv = SshInventoryService(ssh_config_path=str(cfg)).load()
    h = inv.hosts[0]
    assert h.hostname == "edge.example.com"
    assert h.user == "admin"
    assert h.port == 22022


def test_comments_and_blank_lines_ignored(tmp_path: Path) -> None:
    cfg = _write(
        tmp_path,
        """
        # global comment
        Host alpha   # trailing comments after value are NOT supported, but a
                     # full-line comment between blocks is fine.
            HostName alpha.example.com

        # block separator

        Host beta
            HostName beta.example.com
        """,
    )
    inv = SshInventoryService(ssh_config_path=str(cfg)).load()
    assert [h.host for h in inv.hosts] == ["alpha", "beta"]


def test_global_directives_before_first_host_are_ignored(tmp_path: Path) -> None:
    """Pre-Host-block directives apply to '*' in real ssh; we ignore them so the
    inventory only reports user-named aliases."""
    cfg = _write(
        tmp_path,
        """
        ServerAliveInterval 30
        Compression yes

        Host alpha
            HostName alpha.example.com
        """,
    )
    inv = SshInventoryService(ssh_config_path=str(cfg)).load()
    assert [h.host for h in inv.hosts] == ["alpha"]


def test_malformed_port_is_tolerated(tmp_path: Path) -> None:
    cfg = _write(
        tmp_path,
        """
        Host weird
            HostName weird.example.com
            Port not-a-number
        """,
    )
    inv = SshInventoryService(ssh_config_path=str(cfg)).load()
    assert inv.hosts[0].port is None


def test_default_service_uses_env_override(tmp_path: Path, monkeypatch: pytest.MonkeyPatch) -> None:
    cfg = _write(tmp_path, "Host alpha\n    HostName alpha\n")
    monkeypatch.setenv("G8E_SSH_CONFIG_PATH", str(cfg))
    svc = default_ssh_inventory_service()
    assert svc.source_path == str(cfg)
    assert svc.load().hosts[0].host == "alpha"


def test_default_service_falls_back_to_canonical_mount_path(monkeypatch: pytest.MonkeyPatch) -> None:
    monkeypatch.delenv("G8E_SSH_CONFIG_PATH", raising=False)
    svc = default_ssh_inventory_service()
    assert svc.source_path == "/etc/g8e/ssh_config"
