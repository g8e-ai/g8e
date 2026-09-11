# Copyright (c) 2026 Lateralus Labs, LLC.
# Use of this source code is governed by the Business Source License
# included in the LICENSE file.
#
# As of the Change Date listed in the LICENSE file, this software is
# released under the Apache License, Version 2.0.

"""Typed scenario metadata for the multi-role, heterogeneous-stack benchmark.

Each scenario declares its category (one of nine), complexity level
(Light, Assistant, Primary), required tools, expected role exercise,
and grader criteria. The scenario metadata is carried in the
``benchmark_specific`` field of ``TaskMetadata`` through a typed
``ScenarioMetadata`` model so the grader can extract it deterministically.

The nine categories span the four evaluation questions:

- Can the model do the job? (instruction adherence, tool selection, tool
  arguments, technical analysis, final response)
- Can the model behave inside the protocol? (routing/delegation,
  verification)
- Can the platform govern it safely? (security/policy)
- Is it worth running locally? (recovery exercises both protocol and
  resource behavior)

The instruction-adherence scenarios reuse the existing IFEval suite
(REAL_MODEL). The other eight categories are REAL_SYSTEM suites that
exercise the g8ee ensemble through ``G8eeChatSUT``.
"""

from __future__ import annotations

from enum import StrEnum

from pydantic import BaseModel, ConfigDict, Field, model_validator


class ScenarioCategory(StrEnum):
    """The nine benchmark categories spanning the four evaluation questions."""

    INSTRUCTION_ADHERENCE = "instruction_adherence"
    TOOL_SELECTION = "tool_selection"
    TOOL_ARGUMENTS = "tool_arguments"
    TECHNICAL_ANALYSIS = "technical_analysis"
    ROUTING_DELEGATION = "routing_delegation"
    VERIFICATION = "verification"
    SECURITY_POLICY = "security_policy"
    RECOVERY = "recovery"
    FINAL_RESPONSE = "final_response"


class ScenarioComplexity(StrEnum):
    """Task complexity level that determines which role the scenario exercises.

    ``LIGHT``: Tiny, fast, high-volume decisions. Exercises the Light tier
    (triage/classification).
    ``ASSISTANT``: Bounded technical analysis. Exercises the Assistant tier
    (focused investigation).
    ``PRIMARY``: Genuinely agentic tasks requiring planning, delegation, and
    synthesis. Exercises the Primary tier.
    """

    LIGHT = "light"
    ASSISTANT = "assistant"
    PRIMARY = "primary"


class ExpectedRole(StrEnum):
    """The role the scenario is designed to exercise.

    Maps to the g8ee ensemble tiers: Sage (primary), Dash (assistant),
    Tribunal (lite).
    """

    PRIMARY = "primary"
    ASSISTANT = "assistant"
    LIGHT = "light"


class GraderCriterion(BaseModel):
    """One deterministic grading criterion for a scenario.

    The grader checks each criterion against the response answer. A
    criterion specifies a ``kind`` (what to check), a ``value`` (what
    to check against), and whether the criterion must ``must_pass``
    (default True). Criteria with ``must_pass=False`` are informational
    and do not affect the pass/fail result.

    ``keyword_present``: The answer must contain the keyword (case-insensitive).
    ``keyword_absent``: The answer must not contain the keyword.
    ``regex_match``: The answer must match the regex pattern.
    ``json_field_equals``: The answer contains JSON with a field matching
    the expected value. ``value`` is ``{"field": "...", "expected": ...}``.
    ``json_field_present``: The answer contains JSON with the named field.
    ``min_length``: The answer must be at least ``value`` characters.
    ``max_length``: The answer must be at most ``value`` characters.
    ``starts_with``: The answer must start with the given prefix.
    ``ends_with``: The answer must end with the given suffix.
    """

    model_config = ConfigDict(extra="forbid", frozen=True)

    kind: str = Field(min_length=1, description="Criterion kind (e.g. keyword_present).")
    value: str = Field(description="Criterion value (keyword, regex, JSON spec, etc.).")
    must_pass: bool = Field(default=True, description="Whether this criterion must pass for the scenario to pass.")


class ScenarioMetadata(BaseModel):
    """Typed metadata for one benchmark scenario.

    Captures the category, complexity, required tools, expected role,
    and deterministic grader criteria. Carried in the
    ``benchmark_specific`` field of ``TaskMetadata`` so the loader and
    grader share a single typed contract.
    """

    model_config = ConfigDict(extra="forbid", frozen=True)

    category: ScenarioCategory = Field(description="Benchmark category.")
    complexity: ScenarioComplexity = Field(description="Task complexity level.")
    expected_role: ExpectedRole = Field(description="Role the scenario exercises.")
    required_tools: list[str] = Field(
        default_factory=list,
        description="Tools the scenario expects the model to use.",
    )
    criteria: list[GraderCriterion] = Field(
        min_length=1,
        description="Deterministic grading criteria.",
    )

    @model_validator(mode="after")
    def _validate_role_matches_complexity(self) -> ScenarioMetadata:
        """The expected role must match the complexity level."""
        expected_map = {
            ScenarioComplexity.LIGHT: ExpectedRole.LIGHT,
            ScenarioComplexity.ASSISTANT: ExpectedRole.ASSISTANT,
            ScenarioComplexity.PRIMARY: ExpectedRole.PRIMARY,
        }
        if expected_map[self.complexity] != self.expected_role:
            raise ValueError(
                f"expected_role {self.expected_role!r} does not match "
                f"complexity {self.complexity!r}"
            )
        return self


__all__ = [
    "ExpectedRole",
    "GraderCriterion",
    "ScenarioCategory",
    "ScenarioComplexity",
    "ScenarioMetadata",
]
