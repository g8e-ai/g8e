# Copyright (c) 2026 Lateralus Labs, LLC.
# Use of this source code is governed by the Business Source License
# included in the LICENSE file.
#
# As of the Change Date listed in the LICENSE file, this software is
# released under the Apache License, Version 2.0.

"""Typed preflight validation and source/build provenance (P2-05).

Preflight runs before any task executes and fails closed when a required
identity, hash, capability, or provenance field is unavailable. The
runner never runs ad hoc Git commands to populate source/build
provenance; the values come from environment variables set by the
trusted build system or CI pipeline.

All preflight failures raise ``PreflightError`` with a stable typed
``PreflightFailureCode`` so callers never parse error prose. Each check
is single-purpose: credential presence, provider/model/endpoint
capabilities, sampling/seed support, stack/image identity, network mode,
OS/runtime/hardware metadata, redacted configuration, content hashes,
preregistration, provider budget, and source/build provenance.
"""

from __future__ import annotations

import hashlib
import os
import platform
from dataclasses import dataclass
from enum import StrEnum

from g8e_evals.schema import (
    ContentHash,
    ProviderBudget,
    SamplingSettings,
    SourceBuildProvenance,
    StackEnvironment,
)


class PreflightFailureCode(StrEnum):
    """Centralized stable failure codes for preflight validation."""

    CREDENTIAL_MISSING = "credential_missing"
    PROVIDER_MODEL_MISSING = "provider_model_missing"
    ENDPOINT_INVALID = "endpoint_invalid"
    SAMPLING_INVALID = "sampling_invalid"
    SEED_UNSUPPORTED = "seed_unsupported"
    STACK_IMAGE_MISSING = "stack_image_missing"
    NETWORK_MODE_INVALID = "network_mode_invalid"
    OS_METADATA_MISSING = "os_metadata_missing"
    RUNTIME_VERSION_MISSING = "runtime_version_missing"
    HARDWARE_METADATA_MISSING = "hardware_metadata_missing"
    REDACTED_CONFIG_LEAK = "redacted_config_leak"
    CONTENT_HASH_MISSING = "content_hash_missing"
    CONTENT_HASH_INVALID = "content_hash_invalid"
    PREREGISTRATION_MISSING = "preregistration_missing"
    PREREGISTRATION_INVALID = "preregistration_invalid"
    PROVIDER_BUDGET_INVALID = "provider_budget_invalid"
    SOURCE_REVISION_MISSING = "source_revision_missing"
    SOURCE_TREE_STATE_HASH_MISSING = "source_tree_state_hash_missing"
    SOURCE_TREE_STATE_HASH_INVALID = "source_tree_state_hash_invalid"
    BUILD_ID_MISSING = "build_id_missing"


class PreflightError(Exception):
    """Typed preflight failure with a stable ``PreflightFailureCode``."""

    def __init__(self, code: PreflightFailureCode, message: str) -> None:
        super().__init__(f"{code.value}: {message}")
        self.code = code
        self.message = message


# Environment variable names for source/build provenance. The trusted
# build system or CI pipeline sets these before invoking the runner. The
# runner never runs ad hoc Git commands.
ENV_SOURCE_REVISION = "G8E_EVALS_SOURCE_REVISION"
ENV_SOURCE_TREE_STATE_HASH = "G8E_EVALS_SOURCE_TREE_STATE_HASH"
ENV_BUILD_ID = "G8E_EVALS_BUILD_ID"
ENV_BUILD_SYSTEM = "G8E_EVALS_BUILD_SYSTEM"
ENV_CI_RUN_ID = "G8E_EVALS_CI_RUN_ID"
ENV_CI_URL = "G8E_EVALS_CI_URL"

# Environment variable names for provider budget.
ENV_PROVIDER_BUDGET_MAX_USD = "G8E_EVALS_PROVIDER_BUDGET_MAX_USD"
ENV_PROVIDER_BUDGET_MAX_TOKENS = "G8E_EVALS_PROVIDER_BUDGET_MAX_TOKENS"
ENV_PROVIDER_BUDGET_MAX_REQUESTS = "G8E_EVALS_PROVIDER_BUDGET_MAX_REQUESTS"

# Sensitive configuration key fragments that must never appear in the
# redacted config. The redacted config is the only configuration surface
# written to the run manifest.
_SENSITIVE_KEY_FRAGMENTS = (
    "api_key",
    "apikey",
    "secret",
    "password",
    "passwd",
    "token",
    "credential",
    "private_key",
    "l2_private_key",
)

# Providers that do not require an API key.
KEYLESS_PROVIDERS = frozenset({"ollama", "llamacpp", "fake"})


@dataclass(frozen=True)
class PreflightRequest:
    """Typed preflight request bundling every input the checks consume."""

    provider: str | None
    model: str | None
    api_key: str | None
    endpoint: str | None
    sampling: SamplingSettings
    content_hashes: list[ContentHash]
    required_content_hash_names: frozenset[str]
    preregistration_hash: str | None
    redacted_config: dict[str, object]
    stack_environment: StackEnvironment
    source_build_provenance: SourceBuildProvenance | None
    provider_budget: ProviderBudget | None
    seed_support: str = "unknown"


def _check_credential_presence(request: PreflightRequest) -> None:
    """Fail closed when a non-keyless provider is missing its API key."""
    provider = request.provider
    if not provider:
        return
    if provider in KEYLESS_PROVIDERS:
        return
    if not request.api_key:
        raise PreflightError(
            PreflightFailureCode.CREDENTIAL_MISSING,
            f"provider '{provider}' requires an API key but none was supplied",
        )


def _check_provider_model(request: PreflightRequest) -> None:
    """Fail closed when provider is set but model is missing (or vice versa)."""
    provider = request.provider
    model = request.model
    if provider and not model:
        raise PreflightError(
            PreflightFailureCode.PROVIDER_MODEL_MISSING,
            f"provider '{provider}' is set but model is empty",
        )
    if model and not provider:
        raise PreflightError(
            PreflightFailureCode.PROVIDER_MODEL_MISSING,
            f"model '{model}' is set but provider is empty",
        )


def _check_endpoint(request: PreflightRequest) -> None:
    """Fail closed when an endpoint is supplied but not a valid http(s) URL."""
    endpoint = request.endpoint
    if not endpoint:
        return
    if not endpoint.startswith(("http://", "https://")):
        raise PreflightError(
            PreflightFailureCode.ENDPOINT_INVALID,
            f"endpoint '{endpoint}' is not an http(s) URL",
        )


def _check_sampling(request: PreflightRequest) -> None:
    """Fail closed when sampling parameters are out of valid ranges."""
    sampling = request.sampling
    if sampling.temperature is not None and not (0.0 <= sampling.temperature <= 2.0):
        raise PreflightError(
            PreflightFailureCode.SAMPLING_INVALID,
            f"temperature {sampling.temperature} is outside [0.0, 2.0]",
        )
    if sampling.top_p is not None and not (0.0 < sampling.top_p <= 1.0):
        raise PreflightError(
            PreflightFailureCode.SAMPLING_INVALID,
            f"top_p {sampling.top_p} is outside (0.0, 1.0]",
        )
    if sampling.top_k is not None and sampling.top_k < 1:
        raise PreflightError(
            PreflightFailureCode.SAMPLING_INVALID,
            f"top_k {sampling.top_k} is less than 1",
        )
    if sampling.max_output_tokens is not None and sampling.max_output_tokens < 1:
        raise PreflightError(
            PreflightFailureCode.SAMPLING_INVALID,
            f"max_output_tokens {sampling.max_output_tokens} is less than 1",
        )


def _check_seed_support(request: PreflightRequest) -> None:
    """Fail closed when a seed is requested but the provider declares no seed support."""
    if request.sampling.seed is None:
        return
    if request.seed_support == "none":
        raise PreflightError(
            PreflightFailureCode.SEED_UNSUPPORTED,
            f"seed {request.sampling.seed} requested but provider declares no seed support",
        )


def _check_stack_image(request: PreflightRequest) -> None:
    """Fail closed when stack image digests are declared but empty for a production posture.

    The check is permissive for non-production runs: an empty
    ``stack_image_digests`` map is allowed when no production posture is
    declared. Production-posture runs require at least one image digest.
    """
    env = request.stack_environment
    if not env.stack_image_digests:
        return
    for name, digest in env.stack_image_digests.items():
        if not digest:
            raise PreflightError(
                PreflightFailureCode.STACK_IMAGE_MISSING,
                f"stack image '{name}' has an empty digest",
            )


def _check_network_mode(request: PreflightRequest) -> None:
    """Fail closed when network mode is declared but not one of the allowed values."""
    mode = request.stack_environment.network_mode
    if not mode:
        return
    allowed = {"online", "offline", "airgap"}
    if mode not in allowed:
        raise PreflightError(
            PreflightFailureCode.NETWORK_MODE_INVALID,
            f"network_mode '{mode}' is not one of {sorted(allowed)}",
        )


def _check_os_metadata(request: PreflightRequest) -> None:
    """Fail closed when OS metadata is missing."""
    if not request.stack_environment.os:
        raise PreflightError(
            PreflightFailureCode.OS_METADATA_MISSING,
            "stack_environment.os is empty",
        )


def _check_runtime_version(request: PreflightRequest) -> None:
    """Fail closed when runtime version is missing."""
    if not request.stack_environment.runtime_version:
        raise PreflightError(
            PreflightFailureCode.RUNTIME_VERSION_MISSING,
            "stack_environment.runtime_version is empty",
        )


def _check_hardware_metadata(request: PreflightRequest) -> None:
    """Fail closed when arch metadata is missing."""
    if not request.stack_environment.arch:
        raise PreflightError(
            PreflightFailureCode.HARDWARE_METADATA_MISSING,
            "stack_environment.arch is empty",
        )


def _check_redacted_config(request: PreflightRequest) -> None:
    """Fail closed when the redacted config leaks a sensitive key fragment."""
    leaked: list[str] = []
    for key in request.redacted_config:
        lowered = key.lower()
        if any(frag in lowered for frag in _SENSITIVE_KEY_FRAGMENTS):
            leaked.append(key)
    if leaked:
        raise PreflightError(
            PreflightFailureCode.REDACTED_CONFIG_LEAK,
            f"redacted_config leaks sensitive keys: {sorted(leaked)}",
        )


def _check_content_hashes(request: PreflightRequest) -> None:
    """Fail closed when a required content hash is missing or invalid."""
    by_name = {h.name: h for h in request.content_hashes}
    for required in request.required_content_hash_names:
        if required not in by_name:
            raise PreflightError(
                PreflightFailureCode.CONTENT_HASH_MISSING,
                f"required content hash '{required}' is missing",
            )
        h = by_name[required]
        if not h.sha256:
            raise PreflightError(
                PreflightFailureCode.CONTENT_HASH_INVALID,
                f"content hash '{required}' has an empty sha256",
            )
        if len(h.sha256) != 64 or not all(c in "0123456789abcdef" for c in h.sha256):
            raise PreflightError(
                PreflightFailureCode.CONTENT_HASH_INVALID,
                f"content hash '{required}' sha256 is not a 64-char hex string",
            )


def _check_preregistration(request: PreflightRequest) -> None:
    """Fail closed when a preregistration hash is declared but invalid."""
    h = request.preregistration_hash
    if h is None:
        return
    if not h:
        raise PreflightError(
            PreflightFailureCode.PREREGISTRATION_INVALID,
            "preregistration hash is empty",
        )
    if len(h) != 64 or not all(c in "0123456789abcdef" for c in h):
        raise PreflightError(
            PreflightFailureCode.PREREGISTRATION_INVALID,
            "preregistration hash is not a 64-char hex string",
        )


def _check_provider_budget(request: PreflightRequest) -> None:
    """Fail closed when a provider budget is declared but invalid."""
    budget = request.provider_budget
    if budget is None:
        return
    if budget.max_usd < 0:
        raise PreflightError(
            PreflightFailureCode.PROVIDER_BUDGET_INVALID,
            f"provider_budget.max_usd {budget.max_usd} is negative",
        )
    if budget.max_tokens is not None and budget.max_tokens < 0:
        raise PreflightError(
            PreflightFailureCode.PROVIDER_BUDGET_INVALID,
            f"provider_budget.max_tokens {budget.max_tokens} is negative",
        )
    if budget.max_requests is not None and budget.max_requests < 0:
        raise PreflightError(
            PreflightFailureCode.PROVIDER_BUDGET_INVALID,
            f"provider_budget.max_requests {budget.max_requests} is negative",
        )


def _check_source_build_provenance(request: PreflightRequest) -> None:
    """Fail closed when source/build provenance is required but unavailable.

    Source/build provenance is required for release-facing runs. The
    runner never runs ad hoc Git commands; the values come from
    environment variables set by the trusted build system or CI
    pipeline. When ``source_build_provenance`` is None, preflight reads
    the environment variables directly.
    """
    provenance = request.source_build_provenance
    if provenance is not None:
        if not provenance.source_revision:
            raise PreflightError(
                PreflightFailureCode.SOURCE_REVISION_MISSING,
                "source_build_provenance.source_revision is empty",
            )
        if not provenance.source_tree_state_hash:
            raise PreflightError(
                PreflightFailureCode.SOURCE_TREE_STATE_HASH_MISSING,
                "source_build_provenance.source_tree_state_hash is empty",
            )
        if len(provenance.source_tree_state_hash) != 64 or not all(
            c in "0123456789abcdef" for c in provenance.source_tree_state_hash
        ):
            raise PreflightError(
                PreflightFailureCode.SOURCE_TREE_STATE_HASH_INVALID,
                "source_build_provenance.source_tree_state_hash is not a 64-char hex string",
            )
        return

    # Fall back to environment variables.
    source_revision = os.environ.get(ENV_SOURCE_REVISION, "").strip()
    if not source_revision:
        raise PreflightError(
            PreflightFailureCode.SOURCE_REVISION_MISSING,
            f"environment variable {ENV_SOURCE_REVISION} is not set; "
            "trusted build or CI metadata must supply the source revision",
        )
    source_tree_state_hash = os.environ.get(ENV_SOURCE_TREE_STATE_HASH, "").strip()
    if not source_tree_state_hash:
        raise PreflightError(
            PreflightFailureCode.SOURCE_TREE_STATE_HASH_MISSING,
            f"environment variable {ENV_SOURCE_TREE_STATE_HASH} is not set; "
            "trusted build or CI metadata must supply the source tree state hash",
        )
    if len(source_tree_state_hash) != 64 or not all(
        c in "0123456789abcdef" for c in source_tree_state_hash
    ):
        raise PreflightError(
            PreflightFailureCode.SOURCE_TREE_STATE_HASH_INVALID,
            f"environment variable {ENV_SOURCE_TREE_STATE_HASH} is not a 64-char hex string",
        )


# Ordered preflight checks. Each check is single-purpose and raises
# ``PreflightError`` on failure. The order is deliberate: credential and
# provider checks run first because they are the cheapest to verify and
# the most common failure mode; provenance runs last because it depends
# on environment variables that the build system sets.
_PREFLIGHT_CHECKS = (
    _check_provider_model,
    _check_credential_presence,
    _check_endpoint,
    _check_sampling,
    _check_seed_support,
    _check_stack_image,
    _check_network_mode,
    _check_os_metadata,
    _check_runtime_version,
    _check_hardware_metadata,
    _check_redacted_config,
    _check_content_hashes,
    _check_preregistration,
    _check_provider_budget,
    _check_source_build_provenance,
)


def run_preflight(request: PreflightRequest) -> None:
    """Run every preflight check in order. Raises ``PreflightError`` on the first failure."""
    for check in _PREFLIGHT_CHECKS:
        check(request)


def load_source_build_provenance_from_env() -> SourceBuildProvenance:
    """Load source/build provenance from environment variables set by the trusted build system.

    Raises ``PreflightError`` when a required variable is missing or invalid.
    """
    source_revision = os.environ.get(ENV_SOURCE_REVISION, "").strip()
    if not source_revision:
        raise PreflightError(
            PreflightFailureCode.SOURCE_REVISION_MISSING,
            f"environment variable {ENV_SOURCE_REVISION} is not set",
        )
    source_tree_state_hash = os.environ.get(ENV_SOURCE_TREE_STATE_HASH, "").strip()
    if not source_tree_state_hash:
        raise PreflightError(
            PreflightFailureCode.SOURCE_TREE_STATE_HASH_MISSING,
            f"environment variable {ENV_SOURCE_TREE_STATE_HASH} is not set",
        )
    if len(source_tree_state_hash) != 64 or not all(
        c in "0123456789abcdef" for c in source_tree_state_hash
    ):
        raise PreflightError(
            PreflightFailureCode.SOURCE_TREE_STATE_HASH_INVALID,
            f"environment variable {ENV_SOURCE_TREE_STATE_HASH} is not a 64-char hex string",
        )
    build_id = os.environ.get(ENV_BUILD_ID, "").strip()
    build_system = os.environ.get(ENV_BUILD_SYSTEM, "").strip()
    ci_run_id = os.environ.get(ENV_CI_RUN_ID, "").strip()
    ci_url = os.environ.get(ENV_CI_URL, "").strip()
    return SourceBuildProvenance(
        source_revision=source_revision,
        source_tree_state_hash=source_tree_state_hash,
        build_id=build_id,
        build_system=build_system,
        ci_run_id=ci_run_id,
        ci_url=ci_url,
    )


def load_provider_budget_from_env() -> ProviderBudget | None:
    """Load provider budget from environment variables. Returns None when no budget is declared."""
    max_usd_str = os.environ.get(ENV_PROVIDER_BUDGET_MAX_USD, "").strip()
    max_tokens_str = os.environ.get(ENV_PROVIDER_BUDGET_MAX_TOKENS, "").strip()
    max_requests_str = os.environ.get(ENV_PROVIDER_BUDGET_MAX_REQUESTS, "").strip()
    if not max_usd_str and not max_tokens_str and not max_requests_str:
        return None
    try:
        max_usd = float(max_usd_str) if max_usd_str else 0.0
    except ValueError as exc:
        raise PreflightError(
            PreflightFailureCode.PROVIDER_BUDGET_INVALID,
            f"{ENV_PROVIDER_BUDGET_MAX_USD}='{max_usd_str}' is not a number",
        ) from exc
    max_tokens: int | None = None
    if max_tokens_str:
        try:
            max_tokens = int(max_tokens_str)
        except ValueError as exc:
            raise PreflightError(
                PreflightFailureCode.PROVIDER_BUDGET_INVALID,
                f"{ENV_PROVIDER_BUDGET_MAX_TOKENS}='{max_tokens_str}' is not an integer",
            ) from exc
    max_requests: int | None = None
    if max_requests_str:
        try:
            max_requests = int(max_requests_str)
        except ValueError as exc:
            raise PreflightError(
                PreflightFailureCode.PROVIDER_BUDGET_INVALID,
                f"{ENV_PROVIDER_BUDGET_MAX_REQUESTS}='{max_requests_str}' is not an integer",
            ) from exc
    return ProviderBudget(
        max_usd=max_usd,
        max_tokens=max_tokens,
        max_requests=max_requests,
    )


def compute_source_tree_state_hash(source_root: object) -> str:
    """Compute a deterministic SHA-256 over the source tree state.

    The source root must be a path-like object pointing at the directory
    containing the source tree. The hash covers every regular file
    beneath the root, sorted by relative path, with each file's
    relative path and SHA-256 contributing to the digest. Symlinks are
    rejected. This is the only function that reads the source tree
    directly; the runner calls it only when the trusted build system
    has not already supplied a hash via environment variables.
    """
    from pathlib import Path

    root = Path(source_root)
    if not root.is_dir():
        raise PreflightError(
            PreflightFailureCode.SOURCE_TREE_STATE_HASH_INVALID,
            f"source root '{root}' is not a directory",
        )
    files: list[Path] = []
    for path in sorted(root.rglob("*")):
        if path.is_symlink():
            raise PreflightError(
                PreflightFailureCode.SOURCE_TREE_STATE_HASH_INVALID,
                f"symlink rejected in source tree: {path}",
            )
        if path.is_file():
            files.append(path)
    hasher = hashlib.sha256()
    for file_path in files:
        rel = file_path.relative_to(root).as_posix()
        file_hash = hashlib.sha256(file_path.read_bytes()).hexdigest()
        hasher.update(rel.encode("utf-8"))
        hasher.update(b"\0")
        hasher.update(file_hash.encode("utf-8"))
        hasher.update(b"\0")
    return hasher.hexdigest()


def detect_stack_environment() -> StackEnvironment:
    """Detect the current stack environment from the runtime.

    Returns a ``StackEnvironment`` with OS, arch, runtime version, and
    CPU populated from ``platform``. Network mode and stack image
    digests are left empty for the caller to populate from build metadata.
    """
    return StackEnvironment(
        os=platform.platform(),
        arch=platform.machine(),
        cpu=platform.processor() or "",
        runtime_version=platform.python_version(),
    )


__all__ = [
    "ENV_BUILD_ID",
    "ENV_BUILD_SYSTEM",
    "ENV_CI_RUN_ID",
    "ENV_CI_URL",
    "ENV_PROVIDER_BUDGET_MAX_REQUESTS",
    "ENV_PROVIDER_BUDGET_MAX_TOKENS",
    "ENV_PROVIDER_BUDGET_MAX_USD",
    "ENV_SOURCE_REVISION",
    "ENV_SOURCE_TREE_STATE_HASH",
    "KEYLESS_PROVIDERS",
    "PreflightError",
    "PreflightFailureCode",
    "PreflightRequest",
    "compute_source_tree_state_hash",
    "detect_stack_environment",
    "load_provider_budget_from_env",
    "load_source_build_provenance_from_env",
    "run_preflight",
]
