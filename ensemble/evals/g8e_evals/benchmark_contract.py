# Copyright (c) 2026 Lateralus Labs, LLC.
# Use of this source code is governed by the Business Source License
# included in the LICENSE file.
#
# As of the Change Date listed in the LICENSE file, this software is
# released under the Apache License, Version 2.0.

"""Typed benchmark audit report and model-sensitive benchmark
population contracts for the campaign.

The audit report classifies every registered suite as REAL_MODEL,
REAL_SYSTEM, or DETERMINISTIC_SIMULATION, records whether each suite
measures a candidate model, a live system, or only deterministic
simulation, and ensures simulator-only suites are excluded from model
comparisons. Only REAL_MODEL and REAL_SYSTEM suites are exposed through
the model campaign registry.

The benchmark population contract defines deterministic model-sensitive
populations for instruction following, structured output/tool selection,
tier exercise, and binary decisions with source provenance,
redistribution licenses, content hashes, deduplication/contamination
checks, and tests.

Every model is frozen with ``extra="forbid"``. Content hashes are
SHA-256 over canonical JSON (sorted keys, no extra whitespace).
"""

from __future__ import annotations

import hashlib
import json
from enum import StrEnum
from typing import Self

from pydantic import BaseModel, ConfigDict, Field, model_validator

from g8e_evals.suites import SUITE_REGISTRY, SuiteExecutionClass


BENCHMARK_AUDIT_VERSION = "1.0.0"
BENCHMARK_POPULATION_VERSION = "1.0.0"


def _sha256(data: str) -> str:
    return hashlib.sha256(data.encode()).hexdigest()


class AuditExecutionClass(StrEnum):
    """Execution class for a benchmark suite in the audit report.

    Mirrors ``SuiteExecutionClass`` from ``g8e_evals.suites`` but is a
    separate type so the audit report is self-contained and does not
    leak the suite registry's internal enum identity.

    ``REAL_MODEL``: The selected model receives the task and matching
    model-boundary telemetry is recorded. Eligible for model comparisons.
    ``REAL_SYSTEM``: The live g8ee/g8e path receives the task and the
    configured model-tier calls are observed. Eligible for model
    comparisons.
    ``DETERMINISTIC_SIMULATION``: A local simulator produces observations
    without calling the candidate model. Excluded from model rankings.
    """

    REAL_MODEL = "real_model"
    REAL_SYSTEM = "real_system"
    DETERMINISTIC_SIMULATION = "deterministic_simulation"


def _from_suite_execution_class(cls: SuiteExecutionClass) -> AuditExecutionClass:
    """Convert a ``SuiteExecutionClass`` to an ``AuditExecutionClass``."""
    if cls is SuiteExecutionClass.REAL_MODEL:
        return AuditExecutionClass.REAL_MODEL
    if cls is SuiteExecutionClass.REAL_SYSTEM:
        return AuditExecutionClass.REAL_SYSTEM
    if cls is SuiteExecutionClass.DETERMINISTIC_SIMULATION:
        return AuditExecutionClass.DETERMINISTIC_SIMULATION
    raise ValueError(f"unknown SuiteExecutionClass: {cls!r}")


class BenchmarkAuditEntry(BaseModel):
    """One benchmark suite's classification in the audit report.

    Records whether the suite measures a candidate model, a live system,
    or only deterministic simulation, whether it is eligible for model
    comparisons, whether it has a deterministic grader, and the grader
    and provenance loader factory names.

    A DETERMINISTIC_SIMULATION suite cannot be eligible for model
    comparisons. A REAL_MODEL suite must have ``measures_candidate_model``
    set to True. A DETERMINISTIC_SIMULATION suite must have
    ``measures_candidate_model`` set to False.
    """

    model_config = ConfigDict(extra="forbid", frozen=True)

    suite_id: str = Field(min_length=1, description="Unique suite identifier.")
    execution_class: AuditExecutionClass = Field(description="Execution class of the suite.")
    description: str = Field(min_length=1, description="Human-readable suite description.")
    measures_candidate_model: bool = Field(description="Whether the suite measures a candidate model directly.")
    measures_live_system: bool = Field(description="Whether the suite measures a live g8ee/g8e system.")
    is_deterministic_simulation: bool = Field(description="Whether the suite is a deterministic simulation.")
    eligible_for_model_comparison: bool = Field(description="Whether the suite is eligible for model comparisons.")
    has_deterministic_grader: bool = Field(description="Whether the suite has a deterministic grader.")
    grader_factory_name: str | None = Field(
        default=None,
        description="Name of the grader factory class, or None for synthetic suites.",
    )
    provenance_loader_name: str = Field(
        min_length=1,
        description="Name of the provenance loader function.",
    )

    @model_validator(mode="after")
    def _validate_entry(self) -> Self:
        if self.execution_class == AuditExecutionClass.DETERMINISTIC_SIMULATION:
            if self.eligible_for_model_comparison:
                raise ValueError(
                    f"DETERMINISTIC_SIMULATION suite {self.suite_id!r} cannot be "
                    f"eligible_for_model_comparison"
                )
            if self.measures_candidate_model:
                raise ValueError(
                    f"DETERMINISTIC_SIMULATION suite {self.suite_id!r} must have "
                    f"measures_candidate_model=False"
                )
            if self.measures_live_system:
                raise ValueError(
                    f"DETERMINISTIC_SIMULATION suite {self.suite_id!r} must have "
                    f"measures_live_system=False"
                )
        elif self.execution_class == AuditExecutionClass.REAL_MODEL:
            if not self.measures_candidate_model:
                raise ValueError(
                    f"REAL_MODEL suite {self.suite_id!r} must have "
                    f"measures_candidate_model=True"
                )
            if not self.eligible_for_model_comparison:
                raise ValueError(
                    f"REAL_MODEL suite {self.suite_id!r} must be "
                    f"eligible_for_model_comparison"
                )
        elif self.execution_class == AuditExecutionClass.REAL_SYSTEM:
            if not self.measures_live_system:
                raise ValueError(
                    f"REAL_SYSTEM suite {self.suite_id!r} must have "
                    f"measures_live_system=True"
                )
            if not self.eligible_for_model_comparison:
                raise ValueError(
                    f"REAL_SYSTEM suite {self.suite_id!r} must be "
                    f"eligible_for_model_comparison"
                )
        return self


class BenchmarkAuditReport(BaseModel):
    """Typed audit report classifying every registered benchmark suite.

    The report's suite counts must match the actual entries. Suite IDs
    must be unique. The report is the frozen record of which suites are
    eligible for model comparisons and which are simulator-only.
    """

    model_config = ConfigDict(extra="forbid", frozen=True)

    audit_id: str = Field(min_length=1, description="Unique audit identifier.")
    audit_version: str = Field(min_length=1, description="Audit schema version.")
    created_at: str = Field(min_length=1, description="ISO 8601 creation timestamp.")
    entries: list[BenchmarkAuditEntry] = Field(
        min_length=1,
        description="Audit entries, one per registered suite.",
    )
    total_suite_count: int = Field(ge=1, description="Total number of suites in the audit.")
    model_comparison_suite_count: int = Field(
        ge=0,
        description="Number of suites eligible for model comparisons.",
    )
    simulation_suite_count: int = Field(
        ge=0,
        description="Number of deterministic simulation suites.",
    )

    @model_validator(mode="after")
    def _validate_report(self) -> Self:
        if len(self.entries) != self.total_suite_count:
            raise ValueError(
                f"total_suite_count ({self.total_suite_count}) != "
                f"len(entries) ({len(self.entries)})"
            )

        suite_ids = [e.suite_id for e in self.entries]
        if len(suite_ids) != len(set(suite_ids)):
            seen: set[str] = set()
            dupes: list[str] = []
            for sid in suite_ids:
                if sid in seen:
                    dupes.append(sid)
                seen.add(sid)
            raise ValueError(f"duplicate suite_id in audit: {sorted(set(dupes))}")

        actual_model_comparison = sum(
            1 for e in self.entries if e.eligible_for_model_comparison
        )
        if actual_model_comparison != self.model_comparison_suite_count:
            raise ValueError(
                f"model_comparison_suite_count ({self.model_comparison_suite_count}) "
                f"!= actual ({actual_model_comparison})"
            )

        actual_simulation = sum(
            1 for e in self.entries
            if e.execution_class == AuditExecutionClass.DETERMINISTIC_SIMULATION
        )
        if actual_simulation != self.simulation_suite_count:
            raise ValueError(
                f"simulation_suite_count ({self.simulation_suite_count}) "
                f"!= actual ({actual_simulation})"
            )

        if self.model_comparison_suite_count + self.simulation_suite_count != self.total_suite_count:
            raise ValueError(
                f"model_comparison_suite_count ({self.model_comparison_suite_count}) + "
                f"simulation_suite_count ({self.simulation_suite_count}) != "
                f"total_suite_count ({self.total_suite_count})"
            )

        return self


def generate_audit_report(
    *,
    audit_id: str,
    audit_version: str,
    created_at: str,
) -> BenchmarkAuditReport:
    """Generate a benchmark audit report from the current suite registry.

    Classifies every registered suite by its execution class, records
    whether it measures a candidate model, a live system, or only
    deterministic simulation, and computes the model-comparison and
    simulation suite counts.
    """
    entries: list[BenchmarkAuditEntry] = []
    for suite_id in sorted(SUITE_REGISTRY.keys()):
        spec = SUITE_REGISTRY[suite_id]
        audit_class = _from_suite_execution_class(spec.execution_class)
        is_sim = audit_class == AuditExecutionClass.DETERMINISTIC_SIMULATION
        is_real_model = audit_class == AuditExecutionClass.REAL_MODEL
        is_real_system = audit_class == AuditExecutionClass.REAL_SYSTEM
        grader_name = spec.grader_factory.__name__ if spec.grader_factory is not None else None
        provenance_name = spec.provenance_loader.__name__
        entry = BenchmarkAuditEntry(
            suite_id=spec.suite_id,
            execution_class=audit_class,
            description=spec.description,
            measures_candidate_model=is_real_model,
            measures_live_system=is_real_system,
            is_deterministic_simulation=is_sim,
            eligible_for_model_comparison=spec.is_model_comparison,
            has_deterministic_grader=spec.grader_factory is not None,
            grader_factory_name=grader_name,
            provenance_loader_name=provenance_name,
        )
        entries.append(entry)

    model_comparison_count = sum(1 for e in entries if e.eligible_for_model_comparison)
    simulation_count = sum(
        1 for e in entries
        if e.execution_class == AuditExecutionClass.DETERMINISTIC_SIMULATION
    )

    return BenchmarkAuditReport(
        audit_id=audit_id,
        audit_version=audit_version,
        created_at=created_at,
        entries=entries,
        total_suite_count=len(entries),
        model_comparison_suite_count=model_comparison_count,
        simulation_suite_count=simulation_count,
    )


class BenchmarkPopulationKind(StrEnum):
    """Kind of model-sensitive benchmark population.

    ``INSTRUCTION_FOLLOWING``: Instruction-following tasks with
    deterministic graders.
    ``STRUCTURED_OUTPUT``: Structured output and tool selection tasks
    with deterministic format verification.
    ``TIER_EXERCISE``: Tasks that exercise a specific g8ee model tier
    with provider-boundary telemetry.
    ``BINARY_DECISION``: Binary decision tasks with a typed predicate
    and annotation protocol.
    """

    INSTRUCTION_FOLLOWING = "instruction_following"
    STRUCTURED_OUTPUT = "structured_output"
    TIER_EXERCISE = "tier_exercise"
    BINARY_DECISION = "binary_decision"


class DeduplicationCheck(BaseModel):
    """Deduplication and contamination check for a benchmark population.

    Records the deduplication method, the contamination check method,
    the number of duplicate tasks removed, the number of contaminated
    tasks removed, and the content hash of the deduplication/contamination
    report. The check is frozen before measured collection.
    """

    model_config = ConfigDict(extra="forbid", frozen=True)

    deduplication_method: str = Field(
        min_length=1,
        description="Deduplication method (e.g. exact_hash, semantic_hash).",
    )
    contamination_check_method: str = Field(
        min_length=1,
        description="Contamination check method (e.g. ngram_overlap, prompt_hash).",
    )
    duplicate_tasks_removed: int = Field(ge=0, description="Number of duplicate tasks removed.")
    contaminated_tasks_removed: int = Field(ge=0, description="Number of contaminated tasks removed.")
    check_report_hash: str = Field(
        min_length=64, max_length=64,
        description="SHA-256 of the deduplication/contamination check report.",
    )


class BenchmarkPopulation(BaseModel):
    """Frozen model-sensitive benchmark population for the campaign.

    Defines a deterministic population of tasks with source provenance,
    redistribution license, content hashes, deduplication/contamination
    checks, and a minimum population size. The population is frozen
    before measured collection.

    Each population declares its kind (instruction following, structured
    output, tier exercise, binary decision), the benchmark suite ID it
    belongs to, the task IDs, the dataset content hash, the source
    provenance hash, the redistribution license, and the
    deduplication/contamination check.
    """

    model_config = ConfigDict(extra="forbid", frozen=True)

    population_id: str = Field(min_length=1, description="Unique population identifier.")
    population_version: str = Field(min_length=1, description="Population schema version.")
    kind: BenchmarkPopulationKind = Field(description="Kind of benchmark population.")
    suite_id: str = Field(min_length=1, description="Benchmark suite ID this population belongs to.")
    description: str = Field(min_length=1, description="Human-readable population description.")
    task_ids: list[str] = Field(
        min_length=1,
        description="Sorted task IDs in the population.",
    )
    task_count: int = Field(ge=1, description="Number of tasks in the population.")
    dataset_content_hash: str = Field(
        min_length=64, max_length=64,
        description="SHA-256 of the dataset content.",
    )
    source_provenance_hash: str = Field(
        min_length=64, max_length=64,
        description="SHA-256 of the source provenance manifest.",
    )
    redistribution_license: str = Field(
        min_length=1,
        description="SPDX license identifier or custom label for redistribution.",
    )
    deduplication_check: DeduplicationCheck = Field(
        description="Deduplication and contamination check for this population.",
    )
    minimum_population: int = Field(
        ge=1,
        description="Minimum task population for this benchmark kind.",
    )
    content_hash: str = Field(
        min_length=64, max_length=64,
        description="SHA-256 over canonical JSON of the population.",
    )

    @model_validator(mode="after")
    def _validate_population(self) -> Self:
        if len(self.task_ids) != self.task_count:
            raise ValueError(
                f"task_count ({self.task_count}) != len(task_ids) ({len(self.task_ids)})"
            )
        if self.task_ids != sorted(self.task_ids):
            raise ValueError(f"task_ids must be sorted: {self.task_ids}")
        if len(self.task_ids) != len(set(self.task_ids)):
            raise ValueError(f"task_ids must not contain duplicates: {self.task_ids}")
        if self.task_count < self.minimum_population:
            raise ValueError(
                f"task_count ({self.task_count}) < minimum_population ({self.minimum_population})"
            )

        expected = compute_population_hash(
            population_id=self.population_id,
            population_version=self.population_version,
            kind=self.kind,
            suite_id=self.suite_id,
            description=self.description,
            task_ids=self.task_ids,
            dataset_content_hash=self.dataset_content_hash,
            source_provenance_hash=self.source_provenance_hash,
            redistribution_license=self.redistribution_license,
            deduplication_check=self.deduplication_check,
            minimum_population=self.minimum_population,
        )
        if self.content_hash != expected:
            raise ValueError(
                f"benchmark population content_hash mismatch: "
                f"declared {self.content_hash!r}, computed {expected!r}"
            )
        return self


def compute_population_hash(
    *,
    population_id: str,
    population_version: str,
    kind: BenchmarkPopulationKind,
    suite_id: str,
    description: str,
    task_ids: list[str],
    dataset_content_hash: str,
    source_provenance_hash: str,
    redistribution_license: str,
    deduplication_check: DeduplicationCheck,
    minimum_population: int,
) -> str:
    """Compute the content hash for a benchmark population."""
    payload = json.dumps(
        {
            "population_id": population_id,
            "population_version": population_version,
            "kind": kind.value,
            "suite_id": suite_id,
            "description": description,
            "task_ids": sorted(task_ids),
            "dataset_content_hash": dataset_content_hash,
            "source_provenance_hash": source_provenance_hash,
            "redistribution_license": redistribution_license,
            "deduplication_check": json.loads(deduplication_check.model_dump_json()),
            "minimum_population": minimum_population,
        },
        allow_nan=False,
        ensure_ascii=False,
        separators=(",", ":"),
        sort_keys=True,
    )
    return _sha256(payload)


__all__ = [
    "BENCHMARK_AUDIT_VERSION",
    "BENCHMARK_POPULATION_VERSION",
    "AuditExecutionClass",
    "BenchmarkAuditEntry",
    "BenchmarkAuditReport",
    "BenchmarkPopulation",
    "BenchmarkPopulationKind",
    "DeduplicationCheck",
    "compute_population_hash",
    "generate_audit_report",
]
