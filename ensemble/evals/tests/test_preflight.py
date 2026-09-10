# Copyright (c) 2026 Lateralus Labs, LLC.
# Use of this source code is governed by the Business Source License
# included in the LICENSE file.
#
# As of the Change Date listed in the LICENSE file, this software is
# released under the Apache License, Version 2.0.

"""Tier 1 tests for typed preflight validation and source/build provenance (P2-05).

Verifies that preflight fails closed with a typed ``PreflightFailureCode``
for each missing identity, hash, capability, or provenance field. The
runner never runs ad hoc Git commands; source/build provenance comes
from environment variables set by the trusted build system or CI
pipeline.
"""

from __future__ import annotations

from pathlib import Path

import pytest


from g8e_evals.preflight import (
    ENV_BUILD_ID,
    ENV_BUILD_SYSTEM,
    ENV_CI_RUN_ID,
    ENV_CI_URL,
    ENV_PROVIDER_BUDGET_MAX_REQUESTS,
    ENV_PROVIDER_BUDGET_MAX_TOKENS,
    ENV_PROVIDER_BUDGET_MAX_USD,
    ENV_SOURCE_REVISION,
    ENV_SOURCE_TREE_STATE_HASH,
    PreflightError,
    PreflightFailureCode,
    PreflightRequest,
    compute_source_tree_state_hash,
    detect_stack_environment,
    load_provider_budget_from_env,
    load_source_build_provenance_from_env,
    run_preflight,
)
from g8e_evals.schema import (
    ContentHash,
    ProviderBudget,
    SamplingSettings,
    SourceBuildProvenance,
    StackEnvironment,
)


_VALID_SHA256 = "a" * 64


def _valid_provenance() -> SourceBuildProvenance:
    return SourceBuildProvenance(
        source_revision="abc123",
        source_tree_state_hash=_VALID_SHA256,
        build_id="build-1",
        build_system="github-actions",
        ci_run_id="run-1",
        ci_url="https://example.com/run/1",
    )


def _valid_stack_env() -> StackEnvironment:
    return StackEnvironment(
        os="Linux 6.8.0",
        arch="x86_64",
        cpu="x86",
        runtime_version="3.12.0",
    )


def _valid_content_hashes() -> list[ContentHash]:
    return [
        ContentHash(name="dataset", sha256=_VALID_SHA256, byte_length=100),
        ContentHash(name="prompt_bundle", sha256=_VALID_SHA256, byte_length=100),
        ContentHash(name="grader_bundle", sha256=_VALID_SHA256, byte_length=100),
    ]


def _valid_request(**overrides: object) -> PreflightRequest:
    defaults: dict[str, object] = {
        "provider": "ollama",
        "model": "test-model",
        "api_key": None,
        "endpoint": None,
        "sampling": SamplingSettings(),
        "content_hashes": _valid_content_hashes(),
        "required_content_hash_names": frozenset({"dataset", "prompt_bundle", "grader_bundle"}),
        "preregistration_hash": None,
        "redacted_config": {},
        "stack_environment": _valid_stack_env(),
        "source_build_provenance": _valid_provenance(),
        "provider_budget": None,
    }
    defaults.update(overrides)
    return PreflightRequest(**defaults)  # type: ignore[arg-type]


# ---------------------------------------------------------------------------
# PreflightRequest construction
# ---------------------------------------------------------------------------


@pytest.mark.unit
class TestPreflightRequestConstruction:
    """Verify PreflightRequest is a frozen dataclass with typed fields."""

    def test_request_is_frozen(self) -> None:
        from dataclasses import FrozenInstanceError

        request = _valid_request()
        with pytest.raises(FrozenInstanceError):
            request.provider = "openai"  # type: ignore[misc]

    def test_request_carries_all_fields(self) -> None:
        request = _valid_request()
        assert request.provider == "ollama"
        assert request.model == "test-model"
        assert request.sampling is not None
        assert request.content_hashes is not None
        assert request.source_build_provenance is not None


# ---------------------------------------------------------------------------
# Credential presence
# ---------------------------------------------------------------------------


@pytest.mark.unit
class TestCredentialPresence:
    """Fail closed when a non-keyless provider is missing its API key."""

    def test_keyless_provider_without_api_key_passes(self) -> None:
        request = _valid_request(provider="ollama", api_key=None)
        run_preflight(request)

    def test_keyed_provider_without_api_key_fails(self) -> None:
        request = _valid_request(provider="openai", api_key=None)
        with pytest.raises(PreflightError) as exc_info:
            run_preflight(request)
        assert exc_info.value.code == PreflightFailureCode.CREDENTIAL_MISSING

    def test_keyed_provider_with_api_key_passes(self) -> None:
        request = _valid_request(provider="openai", api_key="sk-test")
        run_preflight(request)

    def test_no_provider_passes(self) -> None:
        request = _valid_request(provider=None, model=None, api_key=None)
        run_preflight(request)


# ---------------------------------------------------------------------------
# Provider/model
# ---------------------------------------------------------------------------


@pytest.mark.unit
class TestProviderModel:
    """Fail closed when provider is set but model is missing (or vice versa)."""

    def test_provider_without_model_fails(self) -> None:
        request = _valid_request(provider="openai", model=None, api_key="sk-test")
        with pytest.raises(PreflightError) as exc_info:
            run_preflight(request)
        assert exc_info.value.code == PreflightFailureCode.PROVIDER_MODEL_MISSING

    def test_model_without_provider_fails(self) -> None:
        request = _valid_request(provider=None, model="gpt-4o", api_key=None)
        with pytest.raises(PreflightError) as exc_info:
            run_preflight(request)
        assert exc_info.value.code == PreflightFailureCode.PROVIDER_MODEL_MISSING


# ---------------------------------------------------------------------------
# Endpoint validation
# ---------------------------------------------------------------------------


@pytest.mark.unit
class TestEndpointValidation:
    """Fail closed when an endpoint is not a valid http(s) URL."""

    def test_valid_http_endpoint_passes(self) -> None:
        request = _valid_request(endpoint="http://localhost:8080")
        run_preflight(request)

    def test_valid_https_endpoint_passes(self) -> None:
        request = _valid_request(endpoint="https://api.openai.com")
        run_preflight(request)

    def test_invalid_endpoint_fails(self) -> None:
        request = _valid_request(endpoint="ftp://bad")
        with pytest.raises(PreflightError) as exc_info:
            run_preflight(request)
        assert exc_info.value.code == PreflightFailureCode.ENDPOINT_INVALID


# ---------------------------------------------------------------------------
# Sampling validation
# ---------------------------------------------------------------------------


@pytest.mark.unit
class TestSamplingValidation:
    """Fail closed when sampling parameters are out of valid ranges."""

    def test_valid_sampling_passes(self) -> None:
        request = _valid_request(sampling=SamplingSettings(temperature=0.7, top_p=0.9, top_k=40, max_output_tokens=100))
        run_preflight(request)

    def test_temperature_too_high_fails(self) -> None:
        request = _valid_request(sampling=SamplingSettings(temperature=3.0))
        with pytest.raises(PreflightError) as exc_info:
            run_preflight(request)
        assert exc_info.value.code == PreflightFailureCode.SAMPLING_INVALID

    def test_temperature_negative_fails(self) -> None:
        request = _valid_request(sampling=SamplingSettings(temperature=-0.1))
        with pytest.raises(PreflightError) as exc_info:
            run_preflight(request)
        assert exc_info.value.code == PreflightFailureCode.SAMPLING_INVALID

    def test_top_p_zero_fails(self) -> None:
        request = _valid_request(sampling=SamplingSettings(top_p=0.0))
        with pytest.raises(PreflightError) as exc_info:
            run_preflight(request)
        assert exc_info.value.code == PreflightFailureCode.SAMPLING_INVALID

    def test_top_p_too_high_fails(self) -> None:
        request = _valid_request(sampling=SamplingSettings(top_p=1.5))
        with pytest.raises(PreflightError) as exc_info:
            run_preflight(request)
        assert exc_info.value.code == PreflightFailureCode.SAMPLING_INVALID

    def test_top_k_zero_fails(self) -> None:
        request = _valid_request(sampling=SamplingSettings(top_k=0))
        with pytest.raises(PreflightError) as exc_info:
            run_preflight(request)
        assert exc_info.value.code == PreflightFailureCode.SAMPLING_INVALID

    def test_max_output_tokens_zero_fails(self) -> None:
        request = _valid_request(sampling=SamplingSettings(max_output_tokens=0))
        with pytest.raises(PreflightError) as exc_info:
            run_preflight(request)
        assert exc_info.value.code == PreflightFailureCode.SAMPLING_INVALID


# ---------------------------------------------------------------------------
# Seed support
# ---------------------------------------------------------------------------


@pytest.mark.unit
class TestSeedSupport:
    """Fail closed when a seed is requested but the provider declares no seed support."""

    def test_seed_with_unknown_support_passes(self) -> None:
        request = _valid_request(sampling=SamplingSettings(seed=42), seed_support="unknown")
        run_preflight(request)

    def test_seed_with_deterministic_support_passes(self) -> None:
        request = _valid_request(sampling=SamplingSettings(seed=42), seed_support="deterministic")
        run_preflight(request)

    def test_seed_with_none_support_fails(self) -> None:
        request = _valid_request(sampling=SamplingSettings(seed=42), seed_support="none")
        with pytest.raises(PreflightError) as exc_info:
            run_preflight(request)
        assert exc_info.value.code == PreflightFailureCode.SEED_UNSUPPORTED

    def test_no_seed_passes_regardless_of_support(self) -> None:
        request = _valid_request(sampling=SamplingSettings(seed=None), seed_support="none")
        run_preflight(request)


# ---------------------------------------------------------------------------
# Stack image
# ---------------------------------------------------------------------------


@pytest.mark.unit
class TestStackImage:
    """Fail closed when stack image digests are declared but empty."""

    def test_empty_stack_image_digests_passes(self) -> None:
        request = _valid_request(stack_environment=_valid_stack_env())
        run_preflight(request)

    def test_valid_stack_image_digests_passes(self) -> None:
        env = _valid_stack_env().model_copy(update={"stack_image_digests": {"g8e": "sha256:abc"}})
        request = _valid_request(stack_environment=env)
        run_preflight(request)

    def test_empty_digest_value_fails(self) -> None:
        env = _valid_stack_env().model_copy(update={"stack_image_digests": {"g8e": ""}})
        request = _valid_request(stack_environment=env)
        with pytest.raises(PreflightError) as exc_info:
            run_preflight(request)
        assert exc_info.value.code == PreflightFailureCode.STACK_IMAGE_MISSING


# ---------------------------------------------------------------------------
# Network mode
# ---------------------------------------------------------------------------


@pytest.mark.unit
class TestNetworkMode:
    """Fail closed when network mode is not one of the allowed values."""

    @pytest.mark.parametrize("mode", ["online", "offline", "airgap"])
    def test_valid_network_mode_passes(self, mode: str) -> None:
        env = _valid_stack_env().model_copy(update={"network_mode": mode})
        request = _valid_request(stack_environment=env)
        run_preflight(request)

    def test_empty_network_mode_passes(self) -> None:
        env = _valid_stack_env().model_copy(update={"network_mode": ""})
        request = _valid_request(stack_environment=env)
        run_preflight(request)

    def test_invalid_network_mode_fails(self) -> None:
        env = _valid_stack_env().model_copy(update={"network_mode": "modem"})
        request = _valid_request(stack_environment=env)
        with pytest.raises(PreflightError) as exc_info:
            run_preflight(request)
        assert exc_info.value.code == PreflightFailureCode.NETWORK_MODE_INVALID


# ---------------------------------------------------------------------------
# OS/runtime/hardware metadata
# ---------------------------------------------------------------------------


@pytest.mark.unit
class TestStackEnvironmentMetadata:
    """Fail closed when OS, runtime version, or arch metadata is missing."""

    def test_missing_os_fails(self) -> None:
        env = _valid_stack_env().model_copy(update={"os": ""})
        request = _valid_request(stack_environment=env)
        with pytest.raises(PreflightError) as exc_info:
            run_preflight(request)
        assert exc_info.value.code == PreflightFailureCode.OS_METADATA_MISSING

    def test_missing_runtime_version_fails(self) -> None:
        env = _valid_stack_env().model_copy(update={"runtime_version": ""})
        request = _valid_request(stack_environment=env)
        with pytest.raises(PreflightError) as exc_info:
            run_preflight(request)
        assert exc_info.value.code == PreflightFailureCode.RUNTIME_VERSION_MISSING

    def test_missing_arch_fails(self) -> None:
        env = _valid_stack_env().model_copy(update={"arch": ""})
        request = _valid_request(stack_environment=env)
        with pytest.raises(PreflightError) as exc_info:
            run_preflight(request)
        assert exc_info.value.code == PreflightFailureCode.HARDWARE_METADATA_MISSING


# ---------------------------------------------------------------------------
# Redacted config leak
# ---------------------------------------------------------------------------


@pytest.mark.unit
class TestRedactedConfigLeak:
    """Fail closed when the redacted config leaks a sensitive key fragment."""

    def test_clean_redacted_config_passes(self) -> None:
        request = _valid_request(redacted_config={"model": "gpt-4o", "provider": "openai"})
        run_preflight(request)

    @pytest.mark.parametrize("key", ["api_key", "apikey", "secret", "password", "passwd", "token", "credential", "private_key", "l2_private_key"])
    def test_sensitive_key_fails(self, key: str) -> None:
        request = _valid_request(redacted_config={key: "leaked"})
        with pytest.raises(PreflightError) as exc_info:
            run_preflight(request)
        assert exc_info.value.code == PreflightFailureCode.REDACTED_CONFIG_LEAK

    def test_sensitive_key_case_insensitive_fails(self) -> None:
        request = _valid_request(redacted_config={"API_KEY": "leaked"})
        with pytest.raises(PreflightError) as exc_info:
            run_preflight(request)
        assert exc_info.value.code == PreflightFailureCode.REDACTED_CONFIG_LEAK


# ---------------------------------------------------------------------------
# Content hashes
# ---------------------------------------------------------------------------


@pytest.mark.unit
class TestContentHashes:
    """Fail closed when a required content hash is missing or invalid."""

    def test_all_required_hashes_present_passes(self) -> None:
        request = _valid_request()
        run_preflight(request)

    def test_missing_required_hash_fails(self) -> None:
        hashes = [
            ContentHash(name="dataset", sha256=_VALID_SHA256),
            ContentHash(name="prompt_bundle", sha256=_VALID_SHA256),
        ]
        request = _valid_request(content_hashes=hashes)
        with pytest.raises(PreflightError) as exc_info:
            run_preflight(request)
        assert exc_info.value.code == PreflightFailureCode.CONTENT_HASH_MISSING

    def test_empty_sha256_fails(self) -> None:
        hashes = [
            ContentHash(name="dataset", sha256=_VALID_SHA256),
            ContentHash(name="prompt_bundle", sha256=_VALID_SHA256),
            ContentHash(name="grader_bundle", sha256=""),
        ]
        request = _valid_request(content_hashes=hashes)
        with pytest.raises(PreflightError) as exc_info:
            run_preflight(request)
        assert exc_info.value.code == PreflightFailureCode.CONTENT_HASH_INVALID

    def test_invalid_sha256_fails(self) -> None:
        hashes = [
            ContentHash(name="dataset", sha256=_VALID_SHA256),
            ContentHash(name="prompt_bundle", sha256=_VALID_SHA256),
            ContentHash(name="grader_bundle", sha256="not-hex"),
        ]
        request = _valid_request(content_hashes=hashes)
        with pytest.raises(PreflightError) as exc_info:
            run_preflight(request)
        assert exc_info.value.code == PreflightFailureCode.CONTENT_HASH_INVALID


# ---------------------------------------------------------------------------
# Preregistration
# ---------------------------------------------------------------------------


@pytest.mark.unit
class TestPreregistration:
    """Fail closed when a preregistration hash is declared but invalid."""

    def test_none_preregistration_passes(self) -> None:
        request = _valid_request(preregistration_hash=None)
        run_preflight(request)

    def test_valid_preregistration_passes(self) -> None:
        request = _valid_request(preregistration_hash=_VALID_SHA256)
        run_preflight(request)

    def test_empty_preregistration_fails(self) -> None:
        request = _valid_request(preregistration_hash="")
        with pytest.raises(PreflightError) as exc_info:
            run_preflight(request)
        assert exc_info.value.code == PreflightFailureCode.PREREGISTRATION_INVALID

    def test_invalid_preregistration_fails(self) -> None:
        request = _valid_request(preregistration_hash="not-hex")
        with pytest.raises(PreflightError) as exc_info:
            run_preflight(request)
        assert exc_info.value.code == PreflightFailureCode.PREREGISTRATION_INVALID


# ---------------------------------------------------------------------------
# Provider budget
# ---------------------------------------------------------------------------


@pytest.mark.unit
class TestProviderBudget:
    """Fail closed when a provider budget is declared but invalid.

    The schema rejects negative values at construction, so the preflight
    check is a defense-in-depth secondary check. These tests verify the
    preflight check passes for valid budgets and that the env-loading
    path rejects invalid values.
    """

    def test_none_budget_passes(self) -> None:
        request = _valid_request(provider_budget=None)
        run_preflight(request)

    def test_valid_budget_passes(self) -> None:
        request = _valid_request(provider_budget=ProviderBudget(max_usd=10.0, max_tokens=1000, max_requests=100))
        run_preflight(request)

    def test_zero_usd_budget_passes(self) -> None:
        request = _valid_request(provider_budget=ProviderBudget(max_usd=0.0))
        run_preflight(request)

    def test_budget_with_none_limits_passes(self) -> None:
        request = _valid_request(provider_budget=ProviderBudget(max_usd=10.0, max_tokens=None, max_requests=None))
        run_preflight(request)


# ---------------------------------------------------------------------------
# Source/build provenance
# ---------------------------------------------------------------------------


@pytest.mark.unit
class TestSourceBuildProvenance:
    """Fail closed when source/build provenance is required but unavailable."""

    def test_valid_provenance_passes(self) -> None:
        request = _valid_request(source_build_provenance=_valid_provenance())
        run_preflight(request)

    def test_missing_source_revision_fails(self) -> None:
        provenance = _valid_provenance().model_copy(update={"source_revision": ""})
        request = _valid_request(source_build_provenance=provenance)
        with pytest.raises(PreflightError) as exc_info:
            run_preflight(request)
        assert exc_info.value.code == PreflightFailureCode.SOURCE_REVISION_MISSING

    def test_missing_source_tree_state_hash_fails(self) -> None:
        provenance = _valid_provenance().model_copy(update={"source_tree_state_hash": ""})
        request = _valid_request(source_build_provenance=provenance)
        with pytest.raises(PreflightError) as exc_info:
            run_preflight(request)
        assert exc_info.value.code == PreflightFailureCode.SOURCE_TREE_STATE_HASH_MISSING

    def test_invalid_source_tree_state_hash_fails(self) -> None:
        provenance = _valid_provenance().model_copy(update={"source_tree_state_hash": "not-hex"})
        request = _valid_request(source_build_provenance=provenance)
        with pytest.raises(PreflightError) as exc_info:
            run_preflight(request)
        assert exc_info.value.code == PreflightFailureCode.SOURCE_TREE_STATE_HASH_INVALID


# ---------------------------------------------------------------------------
# Production-posture gating of source/build provenance
# ---------------------------------------------------------------------------


@pytest.mark.unit
class TestSourceBuildProvenanceProductionPostureGating:
    """Production-posture runs require provenance; non-production runs skip the env fallback.

    The synthetic suite is a non-production path. When
    ``is_production_posture`` is False and no provenance is supplied
    (neither on the request nor via environment variables), preflight
    must not fail on the provenance check. Production-posture runs
    (the default) still fail closed when provenance is absent.
    """

    def test_production_posture_without_provenance_fails(self, monkeypatch: pytest.MonkeyPatch) -> None:
        monkeypatch.delenv(ENV_SOURCE_REVISION, raising=False)
        monkeypatch.delenv(ENV_SOURCE_TREE_STATE_HASH, raising=False)
        request = _valid_request(source_build_provenance=None, is_production_posture=True)
        with pytest.raises(PreflightError) as exc_info:
            run_preflight(request)
        assert exc_info.value.code == PreflightFailureCode.SOURCE_REVISION_MISSING

    def test_non_production_posture_without_provenance_passes(self, monkeypatch: pytest.MonkeyPatch) -> None:
        monkeypatch.delenv(ENV_SOURCE_REVISION, raising=False)
        monkeypatch.delenv(ENV_SOURCE_TREE_STATE_HASH, raising=False)
        request = _valid_request(source_build_provenance=None, is_production_posture=False)
        run_preflight(request)

    def test_non_production_posture_with_provenance_still_validates(self) -> None:
        provenance = _valid_provenance().model_copy(update={"source_revision": ""})
        request = _valid_request(source_build_provenance=provenance, is_production_posture=False)
        with pytest.raises(PreflightError) as exc_info:
            run_preflight(request)
        assert exc_info.value.code == PreflightFailureCode.SOURCE_REVISION_MISSING

    def test_default_is_production_posture(self) -> None:
        request = _valid_request(source_build_provenance=None)
        assert request.is_production_posture is True


# ---------------------------------------------------------------------------
# Source/build provenance from environment variables
# ---------------------------------------------------------------------------


@pytest.mark.unit
class TestSourceBuildProvenanceFromEnv:
    """Load source/build provenance from environment variables set by the trusted build system."""

    def test_load_from_env_succeeds(self, monkeypatch: pytest.MonkeyPatch) -> None:
        monkeypatch.setenv(ENV_SOURCE_REVISION, "abc123")
        monkeypatch.setenv(ENV_SOURCE_TREE_STATE_HASH, _VALID_SHA256)
        monkeypatch.setenv(ENV_BUILD_ID, "build-1")
        monkeypatch.setenv(ENV_BUILD_SYSTEM, "github-actions")
        monkeypatch.setenv(ENV_CI_RUN_ID, "run-1")
        monkeypatch.setenv(ENV_CI_URL, "https://example.com/run/1")
        provenance = load_source_build_provenance_from_env()
        assert provenance.source_revision == "abc123"
        assert provenance.source_tree_state_hash == _VALID_SHA256
        assert provenance.build_id == "build-1"
        assert provenance.build_system == "github-actions"
        assert provenance.ci_run_id == "run-1"
        assert provenance.ci_url == "https://example.com/run/1"

    def test_missing_source_revision_fails(self, monkeypatch: pytest.MonkeyPatch) -> None:
        monkeypatch.delenv(ENV_SOURCE_REVISION, raising=False)
        monkeypatch.setenv(ENV_SOURCE_TREE_STATE_HASH, _VALID_SHA256)
        with pytest.raises(PreflightError) as exc_info:
            load_source_build_provenance_from_env()
        assert exc_info.value.code == PreflightFailureCode.SOURCE_REVISION_MISSING

    def test_missing_source_tree_state_hash_fails(self, monkeypatch: pytest.MonkeyPatch) -> None:
        monkeypatch.setenv(ENV_SOURCE_REVISION, "abc123")
        monkeypatch.delenv(ENV_SOURCE_TREE_STATE_HASH, raising=False)
        with pytest.raises(PreflightError) as exc_info:
            load_source_build_provenance_from_env()
        assert exc_info.value.code == PreflightFailureCode.SOURCE_TREE_STATE_HASH_MISSING

    def test_invalid_source_tree_state_hash_fails(self, monkeypatch: pytest.MonkeyPatch) -> None:
        monkeypatch.setenv(ENV_SOURCE_REVISION, "abc123")
        monkeypatch.setenv(ENV_SOURCE_TREE_STATE_HASH, "not-hex")
        with pytest.raises(PreflightError) as exc_info:
            load_source_build_provenance_from_env()
        assert exc_info.value.code == PreflightFailureCode.SOURCE_TREE_STATE_HASH_INVALID

    def test_empty_env_vars_fails(self, monkeypatch: pytest.MonkeyPatch) -> None:
        monkeypatch.setenv(ENV_SOURCE_REVISION, "")
        monkeypatch.setenv(ENV_SOURCE_TREE_STATE_HASH, _VALID_SHA256)
        with pytest.raises(PreflightError) as exc_info:
            load_source_build_provenance_from_env()
        assert exc_info.value.code == PreflightFailureCode.SOURCE_REVISION_MISSING


# ---------------------------------------------------------------------------
# Provider budget from environment variables
# ---------------------------------------------------------------------------


@pytest.mark.unit
class TestProviderBudgetFromEnv:
    """Load provider budget from environment variables."""

    def test_no_budget_returns_none(self, monkeypatch: pytest.MonkeyPatch) -> None:
        monkeypatch.delenv(ENV_PROVIDER_BUDGET_MAX_USD, raising=False)
        monkeypatch.delenv(ENV_PROVIDER_BUDGET_MAX_TOKENS, raising=False)
        monkeypatch.delenv(ENV_PROVIDER_BUDGET_MAX_REQUESTS, raising=False)
        assert load_provider_budget_from_env() is None

    def test_load_from_env_succeeds(self, monkeypatch: pytest.MonkeyPatch) -> None:
        monkeypatch.setenv(ENV_PROVIDER_BUDGET_MAX_USD, "10.0")
        monkeypatch.setenv(ENV_PROVIDER_BUDGET_MAX_TOKENS, "1000")
        monkeypatch.setenv(ENV_PROVIDER_BUDGET_MAX_REQUESTS, "100")
        budget = load_provider_budget_from_env()
        assert budget is not None
        assert budget.max_usd == 10.0
        assert budget.max_tokens == 1000
        assert budget.max_requests == 100

    def test_invalid_max_usd_fails(self, monkeypatch: pytest.MonkeyPatch) -> None:
        monkeypatch.setenv(ENV_PROVIDER_BUDGET_MAX_USD, "not-a-number")
        with pytest.raises(PreflightError) as exc_info:
            load_provider_budget_from_env()
        assert exc_info.value.code == PreflightFailureCode.PROVIDER_BUDGET_INVALID

    def test_invalid_max_tokens_fails(self, monkeypatch: pytest.MonkeyPatch) -> None:
        monkeypatch.setenv(ENV_PROVIDER_BUDGET_MAX_USD, "10.0")
        monkeypatch.setenv(ENV_PROVIDER_BUDGET_MAX_TOKENS, "not-an-int")
        with pytest.raises(PreflightError) as exc_info:
            load_provider_budget_from_env()
        assert exc_info.value.code == PreflightFailureCode.PROVIDER_BUDGET_INVALID


# ---------------------------------------------------------------------------
# Source tree state hash computation
# ---------------------------------------------------------------------------


class TestComputeSourceTreeStateHash:
    """Compute a deterministic SHA-256 over the source tree state."""

    pytestmark = pytest.mark.integration

    def test_deterministic_hash(self, tmp_path: Path) -> None:
        (tmp_path / "a.txt").write_text("hello")
        (tmp_path / "b.txt").write_text("world")
        hash1 = compute_source_tree_state_hash(tmp_path)
        hash2 = compute_source_tree_state_hash(tmp_path)
        assert hash1 == hash2
        assert len(hash1) == 64

    def test_content_change_changes_hash(self, tmp_path: Path) -> None:
        (tmp_path / "a.txt").write_text("hello")
        hash1 = compute_source_tree_state_hash(tmp_path)
        (tmp_path / "a.txt").write_text("world")
        hash2 = compute_source_tree_state_hash(tmp_path)
        assert hash1 != hash2

    def test_file_addition_changes_hash(self, tmp_path: Path) -> None:
        (tmp_path / "a.txt").write_text("hello")
        hash1 = compute_source_tree_state_hash(tmp_path)
        (tmp_path / "b.txt").write_text("world")
        hash2 = compute_source_tree_state_hash(tmp_path)
        assert hash1 != hash2

    def test_symlink_rejected(self, tmp_path: Path) -> None:
        (tmp_path / "a.txt").write_text("hello")
        (tmp_path / "link.txt").symlink_to(tmp_path / "a.txt")
        with pytest.raises(PreflightError) as exc_info:
            compute_source_tree_state_hash(tmp_path)
        assert exc_info.value.code == PreflightFailureCode.SOURCE_TREE_STATE_HASH_INVALID

    def test_non_directory_fails(self, tmp_path: Path) -> None:
        file_path = tmp_path / "not-a-dir.txt"
        file_path.write_text("hello")
        with pytest.raises(PreflightError) as exc_info:
            compute_source_tree_state_hash(file_path)
        assert exc_info.value.code == PreflightFailureCode.SOURCE_TREE_STATE_HASH_INVALID

    def test_empty_directory_produces_valid_hash(self, tmp_path: Path) -> None:
        h = compute_source_tree_state_hash(tmp_path)
        assert len(h) == 64
        assert all(c in "0123456789abcdef" for c in h)


# ---------------------------------------------------------------------------
# Stack environment detection
# ---------------------------------------------------------------------------


@pytest.mark.unit
class TestDetectStackEnvironment:
    """Detect the current stack environment from the runtime."""

    def test_detect_populates_fields(self) -> None:
        env = detect_stack_environment()
        assert env.os
        assert env.arch
        assert env.runtime_version


# ---------------------------------------------------------------------------
# Schema models
# ---------------------------------------------------------------------------


@pytest.mark.unit
class TestSourceBuildProvenanceSchema:
    """Verify SourceBuildProvenance is frozen, extra-forbid, and validates fields."""

    def test_frozen(self) -> None:
        from pydantic import ValidationError

        provenance = _valid_provenance()
        with pytest.raises(ValidationError):
            provenance.source_revision = "other"  # type: ignore[misc]

    def test_extra_field_rejected(self) -> None:
        from pydantic import ValidationError
        with pytest.raises(ValidationError):
            SourceBuildProvenance(
                source_revision="abc",
                source_tree_state_hash=_VALID_SHA256,
                extra_field="bad",  # type: ignore[call-arg]
            )

    def test_empty_source_revision_rejected(self) -> None:
        from pydantic import ValidationError
        with pytest.raises(ValidationError):
            SourceBuildProvenance(source_revision="", source_tree_state_hash=_VALID_SHA256)

    def test_invalid_source_tree_state_hash_rejected(self) -> None:
        from pydantic import ValidationError
        with pytest.raises(ValidationError):
            SourceBuildProvenance(source_revision="abc", source_tree_state_hash="not-hex")


@pytest.mark.unit
class TestProviderBudgetSchema:
    """Verify ProviderBudget is frozen, extra-forbid, and validates fields."""

    def test_frozen(self) -> None:
        from pydantic import ValidationError

        budget = ProviderBudget(max_usd=10.0)
        with pytest.raises(ValidationError):
            budget.max_usd = 20.0  # type: ignore[misc]

    def test_extra_field_rejected(self) -> None:
        from pydantic import ValidationError
        with pytest.raises(ValidationError):
            ProviderBudget(max_usd=10.0, extra_field="bad")  # type: ignore[call-arg]

    def test_negative_max_usd_rejected(self) -> None:
        from pydantic import ValidationError
        with pytest.raises(ValidationError):
            ProviderBudget(max_usd=-1.0)

    def test_negative_max_tokens_rejected(self) -> None:
        from pydantic import ValidationError
        with pytest.raises(ValidationError):
            ProviderBudget(max_usd=10.0, max_tokens=-1)


@pytest.mark.unit
class TestRunManifestProvenanceFields:
    """Verify RunManifest carries the new provenance and budget fields."""

    def test_manifest_with_provenance_and_budget(self) -> None:
        from g8e_evals.schema import RunManifest
        manifest = RunManifest(
            run_id="r1",
            suite_id="s",
            suite_version="1.0",
            source_build_provenance=_valid_provenance(),
            provider_budget=ProviderBudget(max_usd=10.0),
        )
        assert manifest.source_build_provenance is not None
        assert manifest.source_build_provenance.source_revision == "abc123"
        assert manifest.provider_budget is not None
        assert manifest.provider_budget.max_usd == 10.0

    def test_manifest_without_provenance_and_budget(self) -> None:
        from g8e_evals.schema import RunManifest
        manifest = RunManifest(run_id="r1", suite_id="s", suite_version="1.0")
        assert manifest.source_build_provenance is None
        assert manifest.provider_budget is None

    def test_manifest_serializes_provenance(self) -> None:
        from g8e_evals.schema import RunManifest
        manifest = RunManifest(
            run_id="r1",
            suite_id="s",
            suite_version="1.0",
            source_build_provenance=_valid_provenance(),
        )
        json_str = manifest.model_dump_json()
        assert '"source_build_provenance"' in json_str
        assert '"source_revision":"abc123"' in json_str

    def test_manifest_old_source_revision_field_removed(self) -> None:
        from pydantic import ValidationError
        from g8e_evals.schema import RunManifest
        with pytest.raises(ValidationError):
            RunManifest(
                run_id="r1",
                suite_id="s",
                suite_version="1.0",
                source_revision="abc",  # type: ignore[call-arg]
            )
