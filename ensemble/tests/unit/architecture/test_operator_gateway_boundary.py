# Copyright (c) 2026 Lateralus Labs, LLC.
# Use of this source code is governed by the Business Source License
# included in the LICENSE file.
#
# As of the Change Date listed in the LICENSE file, this software is
# released under the Apache License, Version 2.0.

"""Static boundary checks for Gateway-owned Operator protocol extraction."""

from __future__ import annotations

import ast
from pathlib import Path

import pytest

pytestmark = pytest.mark.unit

REPO_ROOT = Path(__file__).resolve().parents[4]
ENSEMBLE_APP = REPO_ROOT / "ensemble" / "app"

FORBIDDEN_OPERATOR_WRITE_METHODS = {
    "create_operator",
    "update_operator",
    "update_operator_status",
    "add_history_entry",
    "append_command_result",
    "add_operator_activity",
    "add_operator_approval",
    "update_operator_heartbeat",
}

DELETED_OPERATOR_AUTH_MODULES = {
    "operator_auth_service.py",
    "operator_session_service.py",
    "session_auth_listener.py",
    "operator_lifecycle_service.py",
    "heartbeat_service.py",
    "heartbeat_stale_monitor.py",
}

DELETED_OPERATOR_IMPORT_PREFIXES = (
    "app.services.operator.operator_auth_service",
    "app.services.operator.operator_session_service",
    "app.services.operator.session_auth_listener",
    "app.services.operator.operator_lifecycle_service",
    "app.services.operator.heartbeat_service",
    "app.services.operator.heartbeat_stale_monitor",
)

ALLOWED_DIRECT_PUBSUB_PUBLISHERS: set[Path] = set()


def _python_files_under(path: Path) -> list[Path]:
    return sorted(path.rglob("*.py"))


def _read(path: Path) -> str:
    return path.read_text(encoding="utf-8")


class TestOperatorGatewayBoundary:
    def test_no_heartbeat_channel_subscribers_in_g8ee_app(self):
        offenders: list[str] = []
        for path in _python_files_under(ENSEMBLE_APP):
            text = _read(path)
            if "heartbeat:" in text and "OperatorChannel.heartbeat" not in text:
                if "subscribe(" in text or "on_channel_message" in text:
                    offenders.append(str(path.relative_to(REPO_ROOT)))
        assert offenders == [], f"g8ee must not subscribe to heartbeat:* channels: {offenders}"

    def test_deleted_operator_authority_modules_stay_absent(self):
        operator_dir = ENSEMBLE_APP / "services" / "operator"
        present = sorted(
            name for name in DELETED_OPERATOR_AUTH_MODULES if (operator_dir / name).exists()
        )
        assert present == [], f"Deleted operator authority modules reappeared: {present}"

    def test_local_operator_command_authority_validator_stays_absent(self):
        validator = ENSEMBLE_APP / "security" / "operator_command_validator.py"
        assert not validator.exists(), (
            "g8ee must not regain a local Operator command authority validator; "
            "Gateway dispatch owns authorization"
        )
        offenders = [
            str(path.relative_to(REPO_ROOT))
            for path in _python_files_under(ENSEMBLE_APP)
            if "operator_command_validator" in _read(path)
        ]
        assert offenders == [], f"Local Operator authority validator references remain: {offenders}"

    def test_operator_data_service_has_no_local_write_methods(self):
        path = ENSEMBLE_APP / "services" / "operator" / "operator_data_service.py"
        tree = ast.parse(_read(path))
        method_names = {
            node.name
            for node in tree.body
            if isinstance(node, ast.ClassDef) and node.name == "OperatorDataService"
            for item in node.body
            if isinstance(item, ast.FunctionDef | ast.AsyncFunctionDef)
        }
        forbidden = sorted(method_names & FORBIDDEN_OPERATOR_WRITE_METHODS)
        assert forbidden == [], f"OperatorDataService regained write methods: {forbidden}"

    def test_port_service_does_not_reference_pubsub_dispatch(self):
        path = ENSEMBLE_APP / "services" / "operator" / "port_service.py"
        text = _read(path)
        assert "pubsub_service" not in text
        assert "register_operator_session" not in text
        assert "is_ready" not in text

    def test_file_and_filesystem_services_do_not_reference_pubsub_dispatch(self):
        for name in ("file_service.py", "filesystem_service.py"):
            path = ENSEMBLE_APP / "services" / "operator" / name
            text = _read(path)
            assert "pubsub_service" not in text, f"{name} must not retain pubsub_service wiring"
            assert ".publish_command(" not in text, f"{name} must not publish commands directly"

    def test_direct_pubsub_publishers_are_explicitly_allowlisted(self):
        """Governed dispatch uses Gateway HTTP; only documented fire-and-forget paths may publish."""
        offenders: list[str] = []
        operator_dir = ENSEMBLE_APP / "services" / "operator"
        for path in _python_files_under(operator_dir):
            if path.name == "pubsub_service.py":
                continue
            text = _read(path)
            if ".publish_command(" not in text:
                continue
            if path.resolve() not in {p.resolve() for p in ALLOWED_DIRECT_PUBSUB_PUBLISHERS}:
                offenders.append(str(path.relative_to(REPO_ROOT)))
        assert offenders == [], (
            "Unexpected direct pub/sub command publishers outside allowlist: "
            f"{offenders}. Route through GatewayOperatorClient.dispatch() or extend the allowlist."
        )

    def test_deleted_operator_modules_are_not_imported_in_g8ee_app(self):
        offenders: list[str] = []
        for path in _python_files_under(ENSEMBLE_APP):
            text = _read(path)
            for prefix in DELETED_OPERATOR_IMPORT_PREFIXES:
                if prefix in text:
                    offenders.append(f"{path.relative_to(REPO_ROOT)} imports {prefix}")
        assert offenders == [], f"Deleted operator authority modules still imported: {offenders}"

    def test_operator_data_service_does_not_write_operator_documents(self):
        path = ENSEMBLE_APP / "services" / "operator" / "operator_data_service.py"
        text = _read(path)
        write_markers = (
            "cache.set_document",
            "cache.upsert",
            "cache.write",
            "cache.put",
            "cache.update",
            "cache.delete",
            "create_document",
            "update_document",
        )
        offenders = [marker for marker in write_markers if marker in text]
        assert offenders == [], (
            "OperatorDataService must not write operator documents locally: "
            f"found {offenders}"
        )
