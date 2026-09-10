# Copyright (c) 2026 Lateralus Labs, LLC.
# Use of this source code is governed by the Business Source License
# included in the LICENSE file.
#
# As of the Change Date listed in the LICENSE file, this software is
# released under the Apache License, Version 2.0.

"""Typed, versioned private campaign model registry.

Each generative variant records its stable campaign model-variant ID,
corrected canonical display name, source-list alias, upstream
organization/repository, immutable revision, retrieval date, license,
access restrictions, redistribution constraints, publication
eligibility, parameter count, architecture, supported modalities,
context limit, verified versus declared capabilities, backend
provider/version, served model tag, artifact digest, format,
quantization, artifact byte length, tokenizer digest, chat template
hash, reasoning mode, hidden reasoning tokens, and weight class.

The registry rejects unresolved display names, mutable unpinned
artifacts, duplicate variant IDs, and qualification records that
reference unknown variant IDs. Gated checkpoints are retained with a
recorded qualification outcome and reason rather than silently dropped.

Every model is frozen with ``extra="forbid"``. Content hashes are
SHA-256 over canonical JSON (sorted keys, no extra whitespace). Secrets
never appear in these records; the registry covers identity, license,
artifact, and capability disposition, not credentials.
"""

from __future__ import annotations

import hashlib
import json
from enum import StrEnum
from typing import Self

from pydantic import BaseModel, ConfigDict, Field, model_validator


MODEL_REGISTRY_VERSION = "1.0.0"


def _sha256(data: str) -> str:
    return hashlib.sha256(data.encode()).hexdigest()


class WeightClass(StrEnum):
    """Weight class for grouping models in the comparison campaign."""

    TOOL_MODEL_BASELINE = "tool-model-baseline"
    HEAVY_SLM = "heavy-slm"
    SMALL_REASONING = "small-reasoning"
    TINY_GENERATIVE = "tiny-generative"


class PublicationEligibility(StrEnum):
    """Publication eligibility for a model variant.

    ``ELIGIBLE``: License permits commercial use and redistribution.
    ``RESTRICTED``: License or access restricts publication to safe
    metadata projections (hashes, license, access status) without
    redistributing weights or restricted prompts.
    ``UNAVAILABLE``: Model could not be resolved or is not accessible.
    """

    ELIGIBLE = "eligible"
    RESTRICTED = "restricted"
    UNAVAILABLE = "unavailable"


class QualificationOutcome(StrEnum):
    """Typed qualification outcome for a model variant.

    ``RUNNABLE``: The variant passed all qualification smokes and is
    eligible for measured campaign cells.
    ``UNAVAILABLE``: The variant could not be resolved or accessed.
    ``LICENSE_BLOCKED``: The license or access policy blocks use.
    ``INCOMPATIBLE``: The variant cannot satisfy the tier's typed-output
    or tool-call contracts.
    ``OUT_OF_MEMORY``: The variant does not fit the available hardware.
    ``BACKEND_UNSUPPORTED``: The backend does not support the variant's
    format, quantization, or architecture.
    """

    RUNNABLE = "runnable"
    UNAVAILABLE = "unavailable"
    LICENSE_BLOCKED = "license_blocked"
    INCOMPATIBLE = "incompatible"
    OUT_OF_MEMORY = "out_of_memory"
    BACKEND_UNSUPPORTED = "backend_unsupported"


class ModelVariant(BaseModel):
    """Immutable record for one generative model variant in the registry.

    Binds the stable campaign model-variant ID, corrected canonical
    display name, source-list alias, upstream repository and immutable
    revision, license and access disposition, parameter count and
    architecture, backend and artifact identity, tokenizer and template
    identity, reasoning mode, and weight class. A changed checkpoint
    revision, quantization, chat template, reasoning mode, or backend
    becomes a distinct variant; results from distinct variants are
    never silently merged.
    """

    model_config = ConfigDict(extra="forbid", frozen=True)

    variant_id: str = Field(min_length=1, description="Stable campaign model-variant ID.")
    canonical_display_name: str = Field(min_length=1, description="Corrected canonical display name.")
    source_list_alias: str = Field(min_length=1, description="Original source-list alias before correction.")
    hf_repo: str = Field(min_length=1, description="Upstream Hugging Face organization/repository.")
    hf_sha: str = Field(min_length=1, description="Immutable revision SHA (40 hex characters).")
    retrieval_date: str = Field(min_length=1, description="ISO 8601 retrieval date.")
    license_id: str = Field(min_length=1, description="SPDX license identifier or custom label.")
    license_text_hash: str | None = Field(default=None, description="SHA-256 of standalone LICENSE file, None when no standalone file exists.")
    gated: bool = Field(description="Whether the upstream repo requires access approval.")
    publication_eligibility: PublicationEligibility = Field(description="Publication eligibility disposition.")
    parameter_count: int = Field(ge=0, description="Total parameter count.")
    parameter_count_display: str = Field(min_length=1, description="Human-readable parameter count (e.g. 8.0B).")
    architecture: str = Field(min_length=1, description="Architecture class (e.g. QwenForCausalLM).")
    model_type: str = Field(min_length=1, description="Model type label from upstream metadata.")
    dtype: str = Field(min_length=1, description="Data type (e.g. BF16, FP16).")
    format: str = Field(min_length=1, description="Artifact format (e.g. safetensors, gguf).")
    quantization: str = Field(default="", description="Quantization policy label (e.g. q4_0, fp16). Empty for unquantized.")
    base_model: str = Field(default="", description="Base model repository, empty when not a fine-tune.")
    context_length: int | None = Field(default=None, description="Maximum context length in tokens, None when unknown.")
    supported_modalities: list[str] = Field(min_length=1, description="Supported input modalities (e.g. text, image).")
    reasoning_mode: str = Field(default="non-reasoning", description="Reasoning mode label (e.g. non-reasoning, switchable, thinking).")
    tool_call_support: bool = Field(description="Whether the model supports tool/function calling.")
    chat_template_family: str = Field(default="", description="Chat template family label (e.g. qwen3, llama-3.1).")
    chat_template_hash: str = Field(default="", description="SHA-256 of the chat template bytes. Empty when no template exists.")
    tokenizer_digest: str = Field(default="", description="SHA-256 of the tokenizer configuration. Empty when gated-unavailable.")
    weight_class: WeightClass = Field(description="Weight class for grouping in the comparison campaign.")
    backend_name: str = Field(min_length=1, description="Backend provider name (e.g. ollama, llama.cpp).")
    backend_version: str = Field(default="", description="Backend version string.")
    served_model_tag: str = Field(min_length=1, description="Exact served model tag or ID (e.g. qwen3:8b).")
    artifact_digest: str = Field(min_length=64, max_length=64, description="SHA-256 of the served model artifact.")
    artifact_bytes: int = Field(default=0, ge=0, description="Artifact file size in bytes, 0 when unmeasured.")
    tensor_format: str = Field(default="", description="Tensor format label (e.g. gguf, safetensors).")
    hidden_reasoning_tokens: bool = Field(default=False, description="Whether hidden reasoning tokens are enabled.")

    @model_validator(mode="after")
    def _validate_variant(self) -> Self:
        if self.supported_modalities != sorted(self.supported_modalities):
            raise ValueError(
                f"supported_modalities must be sorted: {self.supported_modalities}"
            )
        return self


class QualificationRecord(BaseModel):
    """Typed qualification outcome for a model variant.

    Records unavailable, license-blocked, incompatible, out-of-memory,
    or backend-unsupported outcomes with an evidence-backed reason.
    A variant with a non-runnable qualification outcome remains in the
    registry but is excluded from measured campaign cells.
    """

    model_config = ConfigDict(extra="forbid", frozen=True)

    variant_id: str = Field(min_length=1, description="Model variant ID from the registry.")
    outcome: QualificationOutcome = Field(description="Typed qualification outcome.")
    reason: str = Field(min_length=1, description="Evidence-backed reason for the outcome.")
    evidence_hash: str = Field(min_length=64, max_length=64, description="SHA-256 of the evidence supporting this outcome.")


class ModelRegistry(BaseModel):
    """Typed, versioned private campaign model registry.

    Contains all generative variants and their qualification records.
    The registry rejects duplicate variant IDs, qualification records
    that reference unknown variant IDs, and content-hash mismatches.

    The ``content_hash`` is SHA-256 over canonical JSON of the registry
    (sorted variant IDs, sorted qualification records, sorted keys, no
    extra whitespace). Changing any variant or qualification record
    changes the hash and invalidates downstream campaign profile and
    manifest hashes.
    """

    model_config = ConfigDict(extra="forbid", frozen=True)

    registry_id: str = Field(min_length=1, description="Unique registry identifier.")
    registry_version: str = Field(min_length=1, description="Registry version (incremented when variants change).")
    schema_version: str = Field(min_length=1, description="Schema version of the registry contract.")
    created_at: str = Field(min_length=1, description="ISO 8601 creation timestamp.")
    variants: list[ModelVariant] = Field(min_length=1, description="All generative model variants in the registry.")
    qualification_records: list[QualificationRecord] = Field(
        default_factory=list,
        description="Qualification records for variants that are not runnable.",
    )
    content_hash: str = Field(
        min_length=64,
        max_length=64,
        description="SHA-256 over canonical JSON of the registry.",
    )

    @model_validator(mode="after")
    def _validate_registry(self) -> Self:
        variant_ids = [v.variant_id for v in self.variants]
        if len(variant_ids) != len(set(variant_ids)):
            seen: set[str] = set()
            dupes: list[str] = []
            for vid in variant_ids:
                if vid in seen:
                    dupes.append(vid)
                seen.add(vid)
            raise ValueError(f"duplicate variant_id in registry: {sorted(set(dupes))}")

        artifact_keys = [
            (v.hf_repo, v.hf_sha, v.quantization) for v in self.variants
        ]
        if len(artifact_keys) != len(set(artifact_keys)):
            seen_a: set[tuple[str, str, str]] = set()
            dupes_a: list[tuple[str, str, str]] = []
            for key in artifact_keys:
                if key in seen_a:
                    dupes_a.append(key)
                seen_a.add(key)
            raise ValueError(
                f"duplicate artifact (hf_repo, hf_sha, quantization) in registry: "
                f"{sorted(set(dupes_a))}"
            )

        variant_id_set = set(variant_ids)
        for qual in self.qualification_records:
            if qual.variant_id not in variant_id_set:
                raise ValueError(
                    f"qualification record references unknown variant_id: {qual.variant_id!r}"
                )

        expected = compute_model_registry_hash(
            self.registry_id,
            self.registry_version,
            self.variants,
            self.qualification_records,
        )
        if self.content_hash != expected:
            raise ValueError(
                f"model registry content_hash mismatch: declared {self.content_hash!r}, "
                f"computed {expected!r}"
            )
        return self

    def get_variant(self, variant_id: str) -> ModelVariant:
        """Return the variant for ``variant_id``. Raises ``KeyError`` if not found."""
        for v in self.variants:
            if v.variant_id == variant_id:
                return v
        raise KeyError(variant_id)

    def runnable_variant_ids(self) -> list[str]:
        """Return sorted variant IDs that have no non-runnable qualification record."""
        qualified_out = {
            q.variant_id for q in self.qualification_records
            if q.outcome != QualificationOutcome.RUNNABLE
        }
        return sorted(v.variant_id for v in self.variants if v.variant_id not in qualified_out)


def compute_model_registry_hash(
    registry_id: str,
    registry_version: str,
    variants: list[ModelVariant],
    qualification_records: list[QualificationRecord],
) -> str:
    """Compute the content hash for a model registry without constructing the full model."""
    payload = json.dumps(
        {
            "registry_id": registry_id,
            "registry_version": registry_version,
            "variants": [
                json.loads(v.model_dump_json()) for v in sorted(variants, key=lambda v: v.variant_id)
            ],
            "qualification_records": [
                json.loads(q.model_dump_json())
                for q in sorted(qualification_records, key=lambda q: (q.variant_id, q.outcome.value))
            ],
        },
        allow_nan=False,
        ensure_ascii=False,
        separators=(",", ":"),
        sort_keys=True,
    )
    return _sha256(payload)


__all__ = [
    "MODEL_REGISTRY_VERSION",
    "ModelRegistry",
    "ModelVariant",
    "PublicationEligibility",
    "QualificationOutcome",
    "QualificationRecord",
    "WeightClass",
    "compute_model_registry_hash",
]
