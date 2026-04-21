# Copyright (c) 2026 Lateralus Labs, LLC.
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#     http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.

"""Prompt alignment drift-prevention tests.

Stage 2 of the prompt-alignment work established a contract between three
surfaces:

1. ``shared/constants/agents.json`` carries pure persona voice. Scaffolding
   placeholders (``{request}``, ``{guidelines}``, ``{os}``, etc.) live in
   the *consumer* templates (``TRIBUNAL_PROMPT_TEMPLATE`` and
   ``TRIBUNAL_VERIFIER_TEMPLATE`` in ``command_generator.py``), not in the
   persona text itself.

2. ``components/g8ee/app/prompts_data/modes/<mode>/tools.txt`` carries
   mode-specific rules only. Per-tool parameter descriptions live in
   ``prompts_data/tools/<name>.txt``; mode files must never re-embed
   ``<parameters>`` / ``<returns>`` / ``<behavior>`` blocks or
   parameter-bullet specs (``- request:`` / ``- guidelines:``).

3. Every active ``OperatorToolName`` value has a corresponding
   ``prompts_data/tools/<name>.txt`` file (or is explicitly listed in the
   pending-restoration allowlist).

These tests pin that contract. If any one of them fails, a future refactor
is about to silently regress to the pre-Stage-2 shape (placeholder drift
back into personas, duplicated tool descriptions across mode files and
tool files, or an enum value with no tool description file).
"""

from __future__ import annotations

import re
from pathlib import Path

import pytest

from app.constants.status import OperatorToolName
from app.utils.agent_persona_loader import (
    get_agent_persona,
    get_tribunal_member,
)

pytestmark = pytest.mark.unit


_PROMPTS_DATA_DIR: Path = (
    Path(__file__).parent.parent.parent / "app" / "prompts_data"
)
_MODES_DIR: Path = _PROMPTS_DATA_DIR / "modes"
_TOOLS_DIR: Path = _PROMPTS_DATA_DIR / "tools"


# Personas that carry ``str.format`` placeholders by design because their
# single caller renders them with substituted values. The prompt-alignment
# doctrine is explicit that scaffolding belongs in the consumer, not the
# persona — this allowlist is the opt-out for personas whose migration to
# template-rendered scaffolding has not yet happened. Shrinking this set is
# the goal; do not grow it without a documented owner.
_PERSONA_PLACEHOLDER_ALLOWLIST: frozenset[str] = frozenset()


# Personas whose persona text must be pure voice — no ``{...}``
# placeholders. Every Tribunal member plus the Auditor (Verifier) is
# rendered through ``TRIBUNAL_PROMPT_TEMPLATE`` / ``TRIBUNAL_VERIFIER_TEMPLATE``
# in ``command_generator.py``; the primary / fast-path agents (Sage, Dash)
# are rendered through ``build_modular_system_prompt``. Codex and Judge
# are rendered from their own dedicated paths and do not embed template
# scaffolding either. None of them carry scaffolding in the persona text
# itself.
_PURE_VOICE_PERSONAS: tuple[str, ...] = (
    "sage",
    "dash",
    "triage",
    "auditor",
    "scribe",
    "axiom",
    "concord",
    "variance",
    "pragma",
    "nemesis",
    "codex",
    "judge",
)


_PLACEHOLDER_RE: re.Pattern[str] = re.compile(r"\{[a-z_][a-z0-9_]*\}")


class TestPersonaPlaceholderHygiene:
    """Personas that are rendered through a template must not embed
    ``str.format`` placeholders themselves. The template owns the
    scaffolding; the persona owns the voice."""

    @pytest.mark.parametrize("agent_id", _PURE_VOICE_PERSONAS)
    def test_persona_has_no_format_placeholders(self, agent_id: str) -> None:
        persona = get_agent_persona(agent_id)
        matches = _PLACEHOLDER_RE.findall(persona.persona)
        assert not matches, (
            f"Persona '{agent_id}' leaked str.format placeholders back into "
            f"its persona text: {sorted(set(matches))}. Scaffolding lives in "
            f"the consumer template (see command_generator.TRIBUNAL_PROMPT_TEMPLATE "
            f"/ TRIBUNAL_VERIFIER_TEMPLATE), not in agents.json."
        )

    def test_placeholder_allowlist_is_shrinking(self) -> None:
        """The allowlist must be empty - all scaffolding must be in consumer-side templates."""
        assert len(_PERSONA_PLACEHOLDER_ALLOWLIST) == 0, (
            f"Persona placeholder allowlist must be empty. "
            f"Move scaffolding to consumer-side templates and strip "
            f"placeholders from persona text. "
            f"Current entries: {sorted(_PERSONA_PLACEHOLDER_ALLOWLIST)}"
        )

    def test_allowlist_and_pure_voice_sets_are_disjoint(self) -> None:
        overlap = set(_PURE_VOICE_PERSONAS) & _PERSONA_PLACEHOLDER_ALLOWLIST
        assert not overlap, (
            f"Personas cannot be both pure-voice and placeholder-allowlisted: "
            f"{sorted(overlap)}"
        )


class TestTribunalPersonaOutputContract:
    """Each Tribunal member's persona must still carry the terse
    shell-only output contract that ``_normalise_command`` depends on. If
    a rewrite softens this into prose ("return the command with a brief
    explanation"), the pipeline silently starts receiving markdown-wrapped
    commands."""

    @pytest.mark.parametrize(
        "member_id",
        ("axiom", "concord", "variance", "pragma", "nemesis"),
    )
    def test_member_persona_enforces_shell_only_output(self, member_id: str) -> None:
        persona = get_tribunal_member(member_id)
        text = persona.persona
        assert "<output_contract>" in text, (
            f"{member_id}: persona lost its <output_contract> section"
        )
        assert "shell command" in text.lower(), (
            f"{member_id}: persona no longer names the shell-command contract"
        )
        assert "no markdown" in text.lower() or "no markdown fences" in text.lower(), (
            f"{member_id}: persona no longer forbids markdown fences"
        )


class TestModeToolsFilesAreRulesOnly:
    """``modes/<mode>/tools.txt`` files carry mode-specific rules and
    tool-advertisement lists. Per-tool parameter descriptions live in
    ``prompts_data/tools/<name>.txt`` and are loaded separately into each
    ``ToolDeclaration.description`` by ``AIToolService``. The two surfaces
    must not drift back into each other."""

    # Tags that belong to per-tool description files, never to mode rules.
    _TOOL_DESCRIPTION_TAGS: tuple[str, ...] = (
        "<parameters>",
        "</parameters>",
        "<returns>",
        "</returns>",
        "<behavior>",
        "</behavior>",
    )

    # Bullet-style parameter specs (``- request:`` / ``- guidelines:``) are
    # the most common shape that tool descriptions take. If these leak back
    # into a mode tools.txt, the mode file has re-absorbed a tool description.
    _PARAM_BULLET_RE: re.Pattern[str] = re.compile(
        r"^\s*-\s+(request|guidelines|path|file_path|command|query|port|"
        r"target_operator|target_operators|old_content|new_content|content|"
        r"data_type|intent_name|pending_command)\s*:",
        re.MULTILINE,
    )

    def _mode_tools_files(self) -> list[Path]:
        assert _MODES_DIR.is_dir(), f"modes dir missing: {_MODES_DIR}"
        return sorted(_MODES_DIR.glob("*/tools.txt"))

    def test_at_least_one_mode_tools_file_exists(self) -> None:
        files = self._mode_tools_files()
        assert files, (
            f"No modes/*/tools.txt files found under {_MODES_DIR}. "
            f"The prompt pipeline needs at least one."
        )

    def test_mode_tools_files_carry_no_tool_description_tags(self) -> None:
        offenders: list[str] = []
        for path in self._mode_tools_files():
            text = path.read_text(encoding="utf-8")
            for tag in self._TOOL_DESCRIPTION_TAGS:
                if tag in text:
                    offenders.append(f"{path.relative_to(_PROMPTS_DATA_DIR)}: {tag}")
        assert not offenders, (
            "Mode tools.txt files must not embed per-tool description tags "
            "(those live in prompts_data/tools/<name>.txt). Offenders: "
            + ", ".join(offenders)
        )

    def test_mode_tools_files_carry_no_parameter_bullet_specs(self) -> None:
        offenders: list[str] = []
        for path in self._mode_tools_files():
            text = path.read_text(encoding="utf-8")
            for match in self._PARAM_BULLET_RE.finditer(text):
                offenders.append(
                    f"{path.relative_to(_PROMPTS_DATA_DIR)}: "
                    f"'{match.group(0).strip()}'"
                )
        assert not offenders, (
            "Mode tools.txt files must not carry parameter-bullet specs "
            "(those live in prompts_data/tools/<name>.txt). Offenders: "
            + "; ".join(offenders)
        )


class TestEveryActiveToolHasADescriptionFile:
    """Each active ``OperatorToolName`` (i.e. every enum value not listed
    in the tool-registry pending-restoration allowlist) must have a
    matching ``prompts_data/tools/<name>.txt`` file. Drift here means the
    LLM sees the tool schema but no description for it — an immediate
    quality regression."""

    def _pending_restoration(self) -> frozenset[str]:
        from tests.unit.services.ai.test_tool_registry_invariants import (
            _PENDING_RESTORATION,
        )
        return _PENDING_RESTORATION

    def test_tools_directory_exists(self) -> None:
        assert _TOOLS_DIR.is_dir(), f"tools dir missing: {_TOOLS_DIR}"

    def test_every_active_tool_has_a_description_file(self) -> None:
        pending = self._pending_restoration()
        missing: list[str] = []
        for member in OperatorToolName:
            if member.value in pending:
                continue
            tool_file = _TOOLS_DIR / f"{member.value}.txt"
            if not tool_file.is_file():
                missing.append(member.value)
        assert not missing, (
            f"OperatorToolName values with no prompts_data/tools/<name>.txt "
            f"and not in _PENDING_RESTORATION: {sorted(missing)}. Either add "
            f"the description file or add the enum value to "
            f"_PENDING_RESTORATION."
        )

    def test_no_orphan_tool_description_files(self) -> None:
        """Every ``tools/<name>.txt`` should correspond to a real
        ``OperatorToolName`` value. An orphan file means a tool was renamed
        or removed without cleaning up its description — a stale artifact
        that confuses future readers."""
        enum_values = {member.value for member in OperatorToolName}
        orphans: list[str] = []
        for path in sorted(_TOOLS_DIR.glob("*.txt")):
            if path.stem not in enum_values:
                orphans.append(path.name)
        assert not orphans, (
            f"Orphan tool description files in prompts_data/tools/ with no "
            f"matching OperatorToolName: {orphans}. Remove the file or add "
            f"the enum value."
        )


class TestAgenticReasoningLivesOnSage:
    """The ``<agentic_reasoning>`` reasoning-discipline block was moved
    from ``modes/operator_bound/capabilities.txt`` onto Sage's persona.
    Stage 2 routed Sage (complex turns) vs Dash (simple turns) through
    ``build_modular_system_prompt(agent_name=...)``, so the reasoning
    block must appear in Sage's system prompt and nowhere else."""

    def test_agentic_reasoning_is_in_sage_persona(self) -> None:
        sage = get_agent_persona("sage")
        assert "<agentic_reasoning>" in sage.persona, (
            "Sage persona lost its <agentic_reasoning> block. "
            "This is the home of agentic_reasoning after Stage 2."
        )

    def test_agentic_reasoning_not_in_any_mode_prompt_file(self) -> None:
        leaked: list[str] = []
        for path in sorted(_MODES_DIR.rglob("*.txt")):
            if "<agentic_reasoning>" in path.read_text(encoding="utf-8"):
                leaked.append(str(path.relative_to(_PROMPTS_DATA_DIR)))
        assert not leaked, (
            "agentic_reasoning block leaked back into mode prompt files: "
            f"{leaked}. It must live only on Sage's persona in agents.json."
        )

    def test_agentic_reasoning_not_in_dash_persona(self) -> None:
        """Dash is the fast-path voice — it must not carry the deep
        reasoning block or it ceases to be a fast path."""
        dash = get_agent_persona("dash")
        assert "<agentic_reasoning>" not in dash.persona, (
            "Dash persona absorbed the <agentic_reasoning> block. "
            "That block belongs to Sage only; Dash's value is speed."
        )
