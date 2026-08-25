# Copyright (c) 2026 Lateralus Labs, LLC.
# Use of this source code is governed by the Business Source License
# included in the LICENSE file.
#
# As of the Change Date listed in the LICENSE file, this software is
# released under the Apache License, Version 2.0.

"""Regression tests for Phase 11 — Prompt section/mode values sourced from g8e.constants.PROMPTS."""

import pytest

from g8e.constants import prompt as _g8e_prompt

from app.constants.prompts import (
    AGENT_MODE_PROMPT_FILES,
    AgentMode,
    InvestigationContextLabel,
    PromptFile,
    PromptSection,
)

pytestmark = pytest.mark.unit


class TestPromptSectionFromG8e:
    """Verify shared PromptSection values match g8e protocol constants."""

    @pytest.mark.parametrize(
        "member,g8e_key",
        [
            (PromptSection.CAPABILITIES, "SectionCapabilities"),
            (PromptSection.EXECUTION, "SectionExecution"),
            (PromptSection.TOOLS, "SectionTools"),
            (PromptSection.RESPONSE_CONSTRAINTS, "SectionResponseConstraints"),
            (PromptSection.AGENT_PERSONA, "SectionAgentPersona"),
            (PromptSection.IDENTITY, "SectionIdentity"),
            (PromptSection.SYSTEM_CONTEXT, "SectionSystemContext"),
            (PromptSection.TRIAGE_CONTEXT, "SectionTriageContext"),
            (PromptSection.INVESTIGATION_CONTEXT, "SectionInvestigationContext"),
            (PromptSection.LEARNED_CONTEXT, "SectionLearnedContext"),
            (PromptSection.SAFETY, "SectionSafety"),
            (PromptSection.LOYALTY, "SectionLoyalty"),
            (PromptSection.DISSENT, "SectionDissent"),
            (PromptSection.VAULT_MODE, "SectionVaultMode"),
        ],
    )
    def test_section_matches_g8e(self, member: PromptSection, g8e_key: str):
        assert member.value == _g8e_prompt(g8e_key)


class TestG8eeSpecificPromptSections:
    """Verify g8ee-specific sections are local strings not in g8e protocol."""

    def test_sentinel_mode_is_local(self):
        assert PromptSection.SENTINEL_MODE == "sentinel_mode"


class TestAgentModeFromG8e:
    """Verify AgentMode values match g8e protocol constants."""

    @pytest.mark.parametrize(
        "member,g8e_key,expected",
        [
            (AgentMode.G8E_BOUND, "AgentModeG8eBound", "g8e.bound"),
            (AgentMode.G8E_NOT_BOUND, "AgentModeG8eNotBound", "g8e.not.bound"),
            (AgentMode.CLOUD_OPERATOR_BOUND, "AgentModeCloudOperatorBound", "g8e.cloud.bound"),
        ],
    )
    def test_mode_matches_g8e(self, member: AgentMode, g8e_key: str, expected: str):
        assert member.value == _g8e_prompt(g8e_key)
        assert member.value == expected

    def test_agent_mode_count(self):
        assert len(list(AgentMode)) == 3


class TestAgentModePromptFiles:
    """Verify AGENT_MODE_PROMPT_FILES keys match AgentMode member names."""

    def test_all_modes_have_prompt_files(self):
        for mode in AgentMode:
            assert mode in AGENT_MODE_PROMPT_FILES, f"{mode.name} missing from AGENT_MODE_PROMPT_FILES"

    def test_prompt_files_keys_are_agent_mode_members(self):
        assert set(AGENT_MODE_PROMPT_FILES.keys()) == set(AgentMode)

    @pytest.mark.parametrize(
        "mode,expected_sections",
        [
            (AgentMode.G8E_BOUND, {PromptSection.CAPABILITIES, PromptSection.EXECUTION, PromptSection.TOOLS}),
            (AgentMode.G8E_NOT_BOUND, {PromptSection.CAPABILITIES, PromptSection.EXECUTION, PromptSection.TOOLS}),
            (AgentMode.CLOUD_OPERATOR_BOUND, {PromptSection.CAPABILITIES, PromptSection.EXECUTION, PromptSection.TOOLS}),
        ],
    )
    def test_each_mode_has_capabilities_execution_tools(
        self, mode: AgentMode, expected_sections: set
    ):
        sections = set(AGENT_MODE_PROMPT_FILES[mode].keys())
        assert sections == expected_sections

    def test_all_prompt_file_values_are_prompt_file_members(self):
        for mode, sections in AGENT_MODE_PROMPT_FILES.items():
            for section, prompt_file in sections.items():
                assert isinstance(prompt_file, PromptFile), (
                    f"{mode.name}.{section.name} value is not a PromptFile member"
                )


class TestPromptFile:
    """Verify PromptFile enum values are valid file paths."""

    def test_prompt_file_path_property(self):
        assert PromptFile.CORE_SAFETY.path == "core/safety.txt"

    def test_all_prompt_files_have_path(self):
        for member in PromptFile:
            assert member.path == member.value
            assert "/" in member.path or "." in member.path


class TestInvestigationContextLabel:
    """Verify InvestigationContextLabel values (g8ee-specific)."""

    @pytest.mark.parametrize(
        "member,expected",
        [
            (InvestigationContextLabel.CASE, "Case"),
            (InvestigationContextLabel.DESCRIPTION, "Description"),
            (InvestigationContextLabel.STATUS, "Status"),
            (InvestigationContextLabel.PRIORITY, "Priority"),
            (InvestigationContextLabel.SEVERITY, "Severity"),
        ],
    )
    def test_label_values(self, member: InvestigationContextLabel, expected: str):
        assert member.value == expected
