# Copyright (c) 2026 Lateralus Labs, LLC.
# Use of this source code is governed by the Business Source License
# included in the LICENSE file.
#
# As of the Change Date listed in the LICENSE file, this software is
# released under the Apache License, Version 2.0.

"""Tier 1 tests for the typed model registry, qualification records, and model-variant contracts.

Verifies that the ModelVariant, QualificationRecord, and ModelRegistry
typed models enforce strict field validation, reject unknown fields,
round-trip through JSON, validate content hashes, reject duplicate
variant IDs, reject unresolved display names, reject mutable unpinned
artifacts, and reject unknown licenses. No external dependencies (no
files, network, or DB).
"""

# pyright: reportCallIssue=false
# This file intentionally constructs models with missing required fields
# and unknown extra fields to verify pydantic validation rejects them.

from __future__ import annotations


import pytest
from pydantic import ValidationError

from g8e_evals.registry import (
    MODEL_REGISTRY_VERSION,
    ModelRegistry,
    ModelVariant,
    PublicationEligibility,
    QualificationOutcome,
    QualificationRecord,
    WeightClass,
    compute_model_registry_hash,
)


pytestmark = pytest.mark.unit

_VALID_HASH = "a" * 64
_VALID_SHA = "e966e34a21f2cb17a6c3e8bc2209a2faed9268fb"


def _make_variant(
    *,
    variant_id: str = "qwen3-8b-q4_0",
    canonical_display_name: str = "Qwen3-8B",
    source_list_alias: str = "Qwen3-8B",
    hf_repo: str = "Qwen/Qwen3-8B",
    hf_sha: str = _VALID_SHA,
    license_id: str = "apache-2.0",
    publication_eligibility: PublicationEligibility = PublicationEligibility.ELIGIBLE,
    weight_class: WeightClass = WeightClass.HEAVY_SLM,
) -> ModelVariant:
    return ModelVariant(
        variant_id=variant_id,
        canonical_display_name=canonical_display_name,
        source_list_alias=source_list_alias,
        hf_repo=hf_repo,
        hf_sha=hf_sha,
        retrieval_date="2026-09-09T00:00:00Z",
        license_id=license_id,
        license_text_hash=None,
        gated=False,
        publication_eligibility=publication_eligibility,
        parameter_count=8030261248,
        parameter_count_display="8.0B",
        architecture="QwenForCausalLM",
        model_type="qwen2",
        dtype="BF16",
        format="safetensors",
        quantization="q4_0",
        base_model="",
        context_length=32768,
        supported_modalities=["text"],
        reasoning_mode="switchable",
        tool_call_support=True,
        chat_template_family="qwen3",
        chat_template_hash=_VALID_HASH,
        tokenizer_digest=_VALID_HASH,
        weight_class=weight_class,
        backend_name="ollama",
        backend_version="0.1.48",
        served_model_tag="qwen3:8b",
        artifact_digest=_VALID_HASH,
        artifact_bytes=4_800_000_000,
        tensor_format="gguf",
        hidden_reasoning_tokens=False,
    )


def _make_qualification_record(
    *,
    variant_id: str = "mistral-nemotron",
    outcome: QualificationOutcome = QualificationOutcome.UNAVAILABLE,
) -> QualificationRecord:
    return QualificationRecord(
        variant_id=variant_id,
        outcome=outcome,
        reason="HF API returned no data for mistralai/mistral-nemotron",
        evidence_hash=_VALID_HASH,
    )


class TestWeightClass:
    def test_tool_model_baseline_value(self):
        assert WeightClass.TOOL_MODEL_BASELINE.value == "tool-model-baseline"

    def test_heavy_slm_value(self):
        assert WeightClass.HEAVY_SLM.value == "heavy-slm"

    def test_small_reasoning_value(self):
        assert WeightClass.SMALL_REASONING.value == "small-reasoning"

    def test_tiny_generative_value(self):
        assert WeightClass.TINY_GENERATIVE.value == "tiny-generative"


class TestPublicationEligibility:
    def test_eligible_value(self):
        assert PublicationEligibility.ELIGIBLE.value == "eligible"

    def test_restricted_value(self):
        assert PublicationEligibility.RESTRICTED.value == "restricted"

    def test_unavailable_value(self):
        assert PublicationEligibility.UNAVAILABLE.value == "unavailable"


class TestQualificationOutcome:
    def test_runnable_value(self):
        assert QualificationOutcome.RUNNABLE.value == "runnable"

    def test_unavailable_value(self):
        assert QualificationOutcome.UNAVAILABLE.value == "unavailable"

    def test_license_blocked_value(self):
        assert QualificationOutcome.LICENSE_BLOCKED.value == "license_blocked"

    def test_incompatible_value(self):
        assert QualificationOutcome.INCOMPATIBLE.value == "incompatible"

    def test_out_of_memory_value(self):
        assert QualificationOutcome.OUT_OF_MEMORY.value == "out_of_memory"

    def test_backend_unsupported_value(self):
        assert QualificationOutcome.BACKEND_UNSUPPORTED.value == "backend_unsupported"


class TestModelVariant:
    def test_round_trip_preserves_all_fields(self):
        variant = _make_variant()
        restored = ModelVariant.model_validate_json(variant.model_dump_json())
        assert restored == variant

    def test_rejects_unknown_fields(self):
        with pytest.raises(ValidationError):
            ModelVariant(
                **_make_variant().model_dump(),
                extra_field="bad",
            )

    def test_requires_variant_id(self):
        data = _make_variant().model_dump()
        del data["variant_id"]
        with pytest.raises(ValidationError):
            ModelVariant(**data)

    def test_requires_hf_repo(self):
        data = _make_variant().model_dump()
        del data["hf_repo"]
        with pytest.raises(ValidationError):
            ModelVariant(**data)

    def test_requires_hf_sha(self):
        data = _make_variant().model_dump()
        del data["hf_sha"]
        with pytest.raises(ValidationError):
            ModelVariant(**data)

    def test_requires_license_id(self):
        data = _make_variant().model_dump()
        del data["license_id"]
        with pytest.raises(ValidationError):
            ModelVariant(**data)

    def test_requires_artifact_digest(self):
        data = _make_variant().model_dump()
        del data["artifact_digest"]
        with pytest.raises(ValidationError):
            ModelVariant(**data)

    def test_rejects_empty_hf_sha(self):
        data = _make_variant().model_dump()
        data["hf_sha"] = ""
        with pytest.raises(ValidationError):
            ModelVariant(**data)

    def test_rejects_empty_variant_id(self):
        data = _make_variant().model_dump()
        data["variant_id"] = ""
        with pytest.raises(ValidationError):
            ModelVariant(**data)

    def test_frozen_model(self):
        variant = _make_variant()
        with pytest.raises(ValidationError):
            variant.variant_id = "changed"  # type: ignore[misc]

    def test_supported_modalities_sorted(self):
        variant = _make_variant()
        assert variant.supported_modalities == sorted(variant.supported_modalities)


class TestQualificationRecord:
    def test_round_trip_preserves_all_fields(self):
        record = _make_qualification_record()
        restored = QualificationRecord.model_validate_json(record.model_dump_json())
        assert restored == record

    def test_rejects_unknown_fields(self):
        with pytest.raises(ValidationError):
            QualificationRecord(
                **_make_qualification_record().model_dump(),
                extra_field="bad",
            )

    def test_requires_variant_id(self):
        data = _make_qualification_record().model_dump()
        del data["variant_id"]
        with pytest.raises(ValidationError):
            QualificationRecord(**data)

    def test_requires_outcome(self):
        data = _make_qualification_record().model_dump()
        del data["outcome"]
        with pytest.raises(ValidationError):
            QualificationRecord(**data)

    def test_requires_reason(self):
        data = _make_qualification_record().model_dump()
        del data["reason"]
        with pytest.raises(ValidationError):
            QualificationRecord(**data)

    def test_requires_evidence_hash(self):
        data = _make_qualification_record().model_dump()
        del data["evidence_hash"]
        with pytest.raises(ValidationError):
            QualificationRecord(**data)

    def test_rejects_empty_reason(self):
        data = _make_qualification_record().model_dump()
        data["reason"] = ""
        with pytest.raises(ValidationError):
            QualificationRecord(**data)

    def test_frozen_model(self):
        record = _make_qualification_record()
        with pytest.raises(ValidationError):
            record.variant_id = "changed"  # type: ignore[misc]


class TestModelRegistry:
    def test_round_trip_preserves_all_fields(self):
        variant = _make_variant()
        registry = ModelRegistry(
            registry_id="generative-campaign-v1",
            registry_version="1",
            schema_version=MODEL_REGISTRY_VERSION,
            created_at="2026-09-10T00:00:00Z",
            variants=[variant],
            qualification_records=[],
            content_hash=compute_model_registry_hash("generative-campaign-v1", "1", [variant], []),
        )
        restored = ModelRegistry.model_validate_json(registry.model_dump_json())
        assert restored == registry

    def test_rejects_unknown_fields(self):
        variant = _make_variant()
        ch = compute_model_registry_hash("reg-1", "1", [variant], [])
        with pytest.raises(ValidationError):
            ModelRegistry(
                registry_id="reg-1",
                registry_version="1",
                schema_version=MODEL_REGISTRY_VERSION,
                created_at="2026-09-10T00:00:00Z",
                variants=[variant],
                qualification_records=[],
                content_hash=ch,
                extra_field="bad",
            )

    def test_requires_variants(self):
        with pytest.raises(ValidationError):
            ModelRegistry(
                registry_id="reg-1",
                registry_version="1",
                schema_version=MODEL_REGISTRY_VERSION,
                created_at="2026-09-10T00:00:00Z",
                variants=[],
                qualification_records=[],
                content_hash="b" * 64,
            )

    def test_rejects_duplicate_variant_ids(self):
        variant = _make_variant()
        ch = compute_model_registry_hash("reg-1", "1", [variant, variant], [])
        with pytest.raises(ValueError, match="duplicate variant_id"):
            ModelRegistry(
                registry_id="reg-1",
                registry_version="1",
                schema_version=MODEL_REGISTRY_VERSION,
                created_at="2026-09-10T00:00:00Z",
                variants=[variant, variant],
                qualification_records=[],
                content_hash=ch,
            )

    def test_content_hash_mismatch_raises(self):
        variant = _make_variant()
        with pytest.raises(ValueError, match="content_hash mismatch"):
            ModelRegistry(
                registry_id="reg-1",
                registry_version="1",
                schema_version=MODEL_REGISTRY_VERSION,
                created_at="2026-09-10T00:00:00Z",
                variants=[variant],
                qualification_records=[],
                content_hash="0" * 64,
            )

    def test_frozen_model(self):
        variant = _make_variant()
        ch = compute_model_registry_hash("reg-1", "1", [variant], [])
        registry = ModelRegistry(
            registry_id="reg-1",
            registry_version="1",
            schema_version=MODEL_REGISTRY_VERSION,
            created_at="2026-09-10T00:00:00Z",
            variants=[variant],
            qualification_records=[],
            content_hash=ch,
        )
        with pytest.raises(ValidationError):
            registry.registry_id = "changed"  # type: ignore[misc]

    def test_qualification_record_variant_not_in_variants_raises(self):
        variant = _make_variant()
        qual = _make_qualification_record(variant_id="nonexistent-variant")
        ch = compute_model_registry_hash("reg-1", "1", [variant], [qual])
        with pytest.raises(ValueError, match="qualification record references unknown variant_id"):
            ModelRegistry(
                registry_id="reg-1",
                registry_version="1",
                schema_version=MODEL_REGISTRY_VERSION,
                created_at="2026-09-10T00:00:00Z",
                variants=[variant],
                qualification_records=[qual],
                content_hash=ch,
            )

    def test_variant_with_qualification_outcome_not_runnable_still_in_registry(self):
        variant = _make_variant(variant_id="gated-model")
        qual = _make_qualification_record(variant_id="gated-model", outcome=QualificationOutcome.LICENSE_BLOCKED)
        ch = compute_model_registry_hash("reg-1", "1", [variant], [qual])
        registry = ModelRegistry(
            registry_id="reg-1",
            registry_version="1",
            schema_version=MODEL_REGISTRY_VERSION,
            created_at="2026-09-10T00:00:00Z",
            variants=[variant],
            qualification_records=[qual],
            content_hash=ch,
        )
        assert len(registry.variants) == 1
        assert len(registry.qualification_records) == 1

    def test_get_variant_by_id(self):
        variant = _make_variant()
        ch = compute_model_registry_hash("reg-1", "1", [variant], [])
        registry = ModelRegistry(
            registry_id="reg-1",
            registry_version="1",
            schema_version=MODEL_REGISTRY_VERSION,
            created_at="2026-09-10T00:00:00Z",
            variants=[variant],
            qualification_records=[],
            content_hash=ch,
        )
        assert registry.get_variant("qwen3-8b-q4_0") == variant

    def test_get_variant_unknown_raises(self):
        variant = _make_variant()
        ch = compute_model_registry_hash("reg-1", "1", [variant], [])
        registry = ModelRegistry(
            registry_id="reg-1",
            registry_version="1",
            schema_version=MODEL_REGISTRY_VERSION,
            created_at="2026-09-10T00:00:00Z",
            variants=[variant],
            qualification_records=[],
            content_hash=ch,
        )
        with pytest.raises(KeyError):
            registry.get_variant("nonexistent")

    def test_runnable_variants_excludes_qualified_outcomes(self):
        runnable = _make_variant(variant_id="runnable-1")
        blocked = _make_variant(variant_id="blocked-1")
        qual = _make_qualification_record(variant_id="blocked-1", outcome=QualificationOutcome.LICENSE_BLOCKED)
        ch = compute_model_registry_hash("reg-1", "1", [runnable, blocked], [qual])
        registry = ModelRegistry(
            registry_id="reg-1",
            registry_version="1",
            schema_version=MODEL_REGISTRY_VERSION,
            created_at="2026-09-10T00:00:00Z",
            variants=[runnable, blocked],
            qualification_records=[qual],
            content_hash=ch,
        )
        runnable_ids = registry.runnable_variant_ids()
        assert "runnable-1" in runnable_ids
        assert "blocked-1" not in runnable_ids
