# Copyright (c) 2026 Lateralus Labs, LLC.
# Use of this source code is governed by the Business Source License
# included in the LICENSE file.
#
# As of the Change Date listed in the LICENSE file, this software is
# released under the Apache License, Version 2.0.

"""INV-ENV-04 ratchet for the ensemble's environment variable registry."""

import ast
from pathlib import Path

import pytest

from app.constants.env_vars import ENV_VAR_CATEGORY, VIOLATION, EnvVar

pytestmark = [pytest.mark.unit]

_VALID_CATEGORIES = {"secret", "user_endpoint", "host", VIOLATION}
_APP_DIR = Path(__file__).resolve().parents[3] / "app"

# The exact set of keys allowed to be platform configuration read from the
# environment (the env-config-purge follow-on inventory). Removing an entry
# requires removing the env read; adding one is not allowed, so new platform
# configuration cannot enter through the environment.
_ALLOWED_VIOLATIONS = frozenset(
    {
        "G8E_ALLOWED_ORIGINS",
        "G8E_DATA_DIR",
        "G8E_G8EE_HTTPS_PORT",
        "G8E_LLM_JEV_MODEL",
        "G8E_OPERATOR_SESSION_ID",
        "G8E_PASSKEY_ORIGIN",
        "G8E_PASSKEY_RP_ID",
        "G8E_PASSKEY_RP_NAME",
        "G8E_PROJECT_ROOT",
        "G8E_STRICT_CONSTANTS_LINT",
        "G8E_TEST_LLM_ASSISTANT_MODEL",
        "G8E_TEST_LLM_ASSISTANT_PROVIDER",
        "G8E_TEST_LLM_LITE_MODEL",
        "G8E_TEST_LLM_LITE_PROVIDER",
        "G8E_TEST_LLM_PRIMARY_MODEL",
        "G8E_TEST_LLM_PRIMARY_PROVIDER",
        "G8E_TEST_TMP_DIR",
    }
)

# Platform configuration that moved to typed bootstrap settings or Gateway-backed
# platform settings. They must never reappear as environment variables.
_RETIRED = frozenset(
    {
        "G8E_GATEWAY_URL",
        "G8E_GATEWAY_HTTP_URL",
        "G8E_GATEWAY_HTTPS_URL",
        "G8E_GATEWAY_PUBSUB_URL",
        "G8E_RUNTIME_DIR",
        "G8E_PKI_DIR",
        "G8E_SECRETS_DIR",
        "G8E_CA_CERT_PATH",
        "G8E_LLM_PRIMARY_PROVIDER",
        "G8E_LLM_PRIMARY_MODEL",
        "G8E_LLM_ASSISTANT_PROVIDER",
        "G8E_LLM_ASSISTANT_MODEL",
        "G8E_LLM_LITE_PROVIDER",
        "G8E_LLM_LITE_MODEL",
        "G8E_LLM_MAX_TOKENS",
        "G8E_LLM_COMMAND_GEN_ENABLED",
        "G8E_LLM_COMMAND_GEN_AUDITOR",
        "G8E_LLM_COMMAND_GEN_PASSES",
    }
)


def _registered_names() -> set[str]:
    return {value for attr, value in vars(EnvVar).items() if attr.isupper()}


def test_every_env_var_has_a_valid_category():
    names = _registered_names()

    assert set(ENV_VAR_CATEGORY) == names
    assert set(ENV_VAR_CATEGORY.values()) <= _VALID_CATEGORIES


def test_violation_set_only_shrinks():
    actual = {name for name, category in ENV_VAR_CATEGORY.items() if category == VIOLATION}

    added = actual - _ALLOWED_VIOLATIONS
    assert not added, (
        f"{sorted(added)} are platform configuration read from the environment; "
        "use a typed default or an explicit launch argument (INV-ENV-04)"
    )
    stale = _ALLOWED_VIOLATIONS - actual
    assert not stale, (
        f"{sorted(stale)} are no longer violations: remove them from _ALLOWED_VIOLATIONS"
    )


def test_retired_platform_config_is_not_registered():
    assert _registered_names().isdisjoint(_RETIRED)


def _env_read_arguments(tree: ast.AST) -> list[ast.expr]:
    """First argument of every os.environ.get / os.getenv / os.environ[...] read."""
    reads: list[ast.expr] = []
    for node in ast.walk(tree):
        if isinstance(node, ast.Call) and node.args:
            func = node.func
            is_getenv = (
                isinstance(func, ast.Attribute)
                and func.attr == "getenv"
                and isinstance(func.value, ast.Name)
                and func.value.id == "os"
            )
            is_environ_get = (
                isinstance(func, ast.Attribute)
                and func.attr == "get"
                and isinstance(func.value, ast.Attribute)
                and func.value.attr == "environ"
            )
            if is_getenv or is_environ_get:
                reads.append(node.args[0])
        elif (
            isinstance(node, ast.Subscript)
            and isinstance(node.value, ast.Attribute)
            and node.value.attr == "environ"
        ):
            reads.append(node.slice)
    return reads


def test_production_env_reads_use_registry_keys():
    offenders: list[str] = []
    py_files = [
        p
        for p in _APP_DIR.rglob("*.py")
        if not any(part.startswith(".") for part in p.parts)
        and "venv" not in p.parts
        and "__pycache__" not in p.parts
    ]
    for path in sorted(py_files):
        tree = ast.parse(path.read_text(), filename=str(path))
        for arg in _env_read_arguments(tree):
            if isinstance(arg, ast.Constant) and isinstance(arg.value, str):
                offenders.append(f"{path.relative_to(_APP_DIR.parent)}:{arg.lineno} {arg.value!r}")

    assert not offenders, f"env reads must use EnvVar keys, not string literals: {offenders}"
