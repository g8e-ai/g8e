# Copyright (c) 2026 Lateralus Labs, LLC.
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#     http://www.apache.org/licenses/LICENSE-2.0

import time
from pathlib import Path

import pytest

from app.errors import ConfigurationError
from app.services.ai.ssh_inventory_service import (
    SshInventoryService,
    default_ssh_inventory_service,
    _load_cached,
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


def test_load_returns_cached_result_on_repeated_calls(tmp_path: Path) -> None:
    """Verify that repeated calls to load() return the same cached result."""
    cfg = _write(tmp_path, "Host alpha\n    HostName alpha.example.com\n")
    svc = SshInventoryService(ssh_config_path=str(cfg))

    inv1 = svc.load()
    inv2 = svc.load()

    assert inv1 is inv2


def test_load_invalidates_cache_when_file_mtime_changes(tmp_path: Path) -> None:
    """Verify that modifying the file invalidates the cache."""
    cfg = _write(tmp_path, "Host alpha\n    HostName alpha.example.com\n")
    svc = SshInventoryService(ssh_config_path=str(cfg))

    inv1 = svc.load()
    assert inv1.hosts[0].host == "alpha"

    time.sleep(0.01)
    cfg.write_text("Host beta\n    HostName beta.example.com\n", encoding="utf-8")

    inv2 = svc.load()
    assert inv2 is not inv1
    assert inv2.hosts[0].host == "beta"


def test_load_cache_keyed_by_path_and_mtime(tmp_path: Path) -> None:
    """Verify that cache is keyed by both path and mtime."""
    cfg1 = tmp_path / "config1"
    cfg1.write_text("Host alpha\n", encoding="utf-8")

    cfg2 = tmp_path / "config2"
    cfg2.write_text("Host beta\n", encoding="utf-8")

    mtime = cfg1.stat().st_mtime

    inv1 = _load_cached(str(cfg1), mtime)
    inv2 = _load_cached(str(cfg2), mtime)

    assert inv1 is not inv2
    assert inv1.hosts[0].host == "alpha"
    assert inv2.hosts[0].host == "beta"


def test_include_single_file(tmp_path: Path) -> None:
    """Test Include directive with a single file."""
    included = tmp_path / "included_config"
    included.write_text(
        "Host web-1\n    HostName 10.0.0.1\n    User deploy\n",
        encoding="utf-8",
    )

    cfg = _write(
        tmp_path,
        f"""
        Include {included}
        
        Host db-1
            HostName 10.0.0.2
        """,
    )
    inv = SshInventoryService(ssh_config_path=str(cfg)).load()
    
    hosts = {h.host: h for h in inv.hosts}
    assert "web-1" in hosts
    assert hosts["web-1"].hostname == "10.0.0.1"
    assert hosts["web-1"].user == "deploy"
    assert "db-1" in hosts
    assert hosts["db-1"].hostname == "10.0.0.2"


def test_include_with_glob_pattern(tmp_path: Path) -> None:
    """Test Include directive with glob pattern."""
    config_dir = tmp_path / "config.d"
    config_dir.mkdir()
    
    (config_dir / "01-web.conf").write_text(
        "Host web-1\n    HostName 10.0.1.1\n",
        encoding="utf-8",
    )
    (config_dir / "02-db.conf").write_text(
        "Host db-1\n    HostName 10.0.2.1\n",
        encoding="utf-8",
    )
    
    cfg = _write(
        tmp_path,
        f"""
        Include {config_dir}/*
        
        Host local
            HostName localhost
        """,
    )
    inv = SshInventoryService(ssh_config_path=str(cfg)).load()
    
    hosts = {h.host: h for h in inv.hosts}
    assert "web-1" in hosts
    assert "db-1" in hosts
    assert "local" in hosts


def test_include_with_relative_path(tmp_path: Path) -> None:
    """Test Include directive with relative path."""
    subdir = tmp_path / "subdir"
    subdir.mkdir()
    
    included = subdir / "config"
    included.write_text("Host remote\n    HostName 10.0.0.1\n", encoding="utf-8")
    
    cfg = _write(tmp_path, f"Include subdir/config\n")
    inv = SshInventoryService(ssh_config_path=str(cfg)).load()
    
    assert len(inv.hosts) == 1
    assert inv.hosts[0].host == "remote"
    assert inv.hosts[0].hostname == "10.0.0.1"


def test_include_with_absolute_path(tmp_path: Path, monkeypatch: pytest.MonkeyPatch) -> None:
    """Test Include directive with absolute path."""
    included = tmp_path / "included"
    included.write_text("Host absolute\n    HostName 10.0.0.1\n", encoding="utf-8")
    
    cfg = _write(tmp_path, f"Include {included}\n")
    inv = SshInventoryService(ssh_config_path=str(cfg)).load()
    
    assert len(inv.hosts) == 1
    assert inv.hosts[0].host == "absolute"


def test_include_nonexistent_path_is_ignored(tmp_path: Path) -> None:
    """Test that Include directive with non-existent path is ignored gracefully."""
    cfg = _write(
        tmp_path,
        """
        Include /nonexistent/path/config
        
        Host fallback
            HostName 10.0.0.1
        """,
    )
    inv = SshInventoryService(ssh_config_path=str(cfg)).load()
    
    assert len(inv.hosts) == 1
    assert inv.hosts[0].host == "fallback"


def test_include_circular_detection(tmp_path: Path) -> None:
    """Test that circular includes are detected and skipped."""
    cfg1 = tmp_path / "config1"
    cfg2 = tmp_path / "config2"
    
    cfg1.write_text(f"Include {cfg2}\nHost host1\n    HostName 10.0.0.1\n", encoding="utf-8")
    cfg2.write_text(f"Include {cfg1}\nHost host2\n    HostName 10.0.0.2\n", encoding="utf-8")
    
    inv = SshInventoryService(ssh_config_path=str(cfg1)).load()
    
    # Should parse both hosts without infinite recursion
    hosts = {h.host: h for h in inv.hosts}
    assert "host1" in hosts
    assert "host2" in hosts


def test_match_block_with_hosts(tmp_path: Path) -> None:
    """Test that Host directives within Match blocks are parsed."""
    cfg = _write(
        tmp_path,
        """
        Match host "bastion"
            Host bastion
                HostName 10.0.0.1
                User ops
        
        Host regular
            HostName 10.0.0.2
        """,
    )
    inv = SshInventoryService(ssh_config_path=str(cfg)).load()
    
    hosts = {h.host: h for h in inv.hosts}
    assert "bastion" in hosts
    assert hosts["bastion"].hostname == "10.0.0.1"
    assert hosts["bastion"].user == "ops"
    assert "regular" in hosts
    assert hosts["regular"].hostname == "10.0.0.2"


def test_match_block_multiple_hosts(tmp_path: Path) -> None:
    """Test Match block with multiple Host directives."""
    cfg = _write(
        tmp_path,
        """
        Match user "deploy"
            Host web-1
                HostName 10.0.1.1
            Host web-2
                HostName 10.0.1.2
        """,
    )
    inv = SshInventoryService(ssh_config_path=str(cfg)).load()
    
    hosts = {h.host: h for h in inv.hosts}
    assert "web-1" in hosts
    assert "web-2" in hosts
    assert hosts["web-1"].hostname == "10.0.1.1"
    assert hosts["web-2"].hostname == "10.0.1.2"


def test_match_block_with_port(tmp_path: Path) -> None:
    """Test Match block with Port directive."""
    cfg = _write(
        tmp_path,
        """
        Match host "special"
            Host special
                HostName 10.0.0.1
                Port 2222
        """,
    )
    inv = SshInventoryService(ssh_config_path=str(cfg)).load()
    
    assert inv.hosts[0].host == "special"
    assert inv.hosts[0].port == 2222


def test_nested_includes(tmp_path: Path) -> None:
    """Test nested Include directives."""
    level2 = tmp_path / "level2.conf"
    level2.write_text("Host level2-host\n    HostName 10.0.2.1\n", encoding="utf-8")
    
    level1 = tmp_path / "level1.conf"
    level1.write_text(
        f"Include {level2}\nHost level1-host\n    HostName 10.0.1.1\n",
        encoding="utf-8",
    )
    
    cfg = _write(tmp_path, f"Include {level1}\nHost main\n    HostName 10.0.0.1\n")
    inv = SshInventoryService(ssh_config_path=str(cfg)).load()
    
    hosts = {h.host: h for h in inv.hosts}
    assert "main" in hosts
    assert "level1-host" in hosts
    assert "level2-host" in hosts


def test_include_with_equals_separator(tmp_path: Path) -> None:
    """Test Include directive with equals separator."""
    included = tmp_path / "included"
    included.write_text("Host included-host\n    HostName 10.0.0.1\n", encoding="utf-8")
    
    cfg = _write(tmp_path, f"Include={included}\n")
    inv = SshInventoryService(ssh_config_path=str(cfg)).load()
    
    assert len(inv.hosts) == 1
    assert inv.hosts[0].host == "included-host"
