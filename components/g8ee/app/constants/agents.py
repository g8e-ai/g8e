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

from enum import StrEnum
from app.constants.shared import _AGENTS


class ReasoningAgent(StrEnum):
    """Canonical agent identity used to route system-prompt assembly.

    Each value matches the top-level id under `agent.metadata` in
    `shared/constants/agents.json`. `SAGE` is the deep-reasoning primary
    agent; `DASH` is the fast-path assistant agent. The chat pipeline
    resolves a ReasoningAgent from the tier it selected (primary -> SAGE,
    assistant -> DASH) and threads it into `build_modular_system_prompt`
    so the assembled prompt prepends the agent's persona.
    """
    SAGE = _AGENTS["agent.names"]["sage"]
    DASH = _AGENTS["agent.names"]["dash"]


class TriageComplexityClassification(StrEnum):
    SIMPLE  = _AGENTS["triage.complexity"]["simple"]
    COMPLEX = _AGENTS["triage.complexity"]["complex"]


class TriageConfidence(StrEnum):
    """Confidence level for triage classifications (both complexity and intent)."""
    HIGH    = _AGENTS["triage.confidence"]["high"]
    LOW     = _AGENTS["triage.confidence"]["low"]


class TriageIntentClassification(StrEnum):
    INFORMATION = _AGENTS["triage.intent"]["information"]
    ACTION      = _AGENTS["triage.intent"]["action"]
    UNKNOWN     = _AGENTS["triage.intent"]["unknown"]


class TriageRequestPosture(StrEnum):
    """Triage's read of the user's state for this turn.

    Downstream agents (Primary, Assistant) calibrate their dissent and
    denial-memory behavior based on this value — see `core/dissent.txt`.

    - normal:      Standard engaged request. Default.
    - escalated:   User is frustrated / time-pressured. Tighten prose, do not
                   weaken safety work.
    - adversarial: User appears to be bypassing prior refusals. Apply denial
                   memory with full force, warn on every tool call.
    - confused:    User's request contradicts their stated goal. Name the
                   contradiction before acting.
    """
    NORMAL      = _AGENTS["triage.posture"]["normal"]
    ESCALATED   = _AGENTS["triage.posture"]["escalated"]
    ADVERSARIAL = _AGENTS["triage.posture"]["adversarial"]
    CONFUSED    = _AGENTS["triage.posture"]["confused"]


class TribunalMember(StrEnum):
    """The five permanent members of the Tribunal.

    Each member has a fixed name and role that defines its reasoning profile.
    The ordering is canonical: Axiom (pass 0), Concord (pass 1), Variance (pass 2),
    Pragma (pass 3), Nemesis (pass 4). When more than 5 passes are configured the members cycle.
    """
    AXIOM   = _AGENTS["tribunal.members"]["axiom"]
    CONCORD = _AGENTS["tribunal.members"]["concord"]
    VARIANCE = _AGENTS["tribunal.members"]["variance"]
    PRAGMA = _AGENTS["tribunal.members"]["pragma"]
    NEMESIS = _AGENTS["tribunal.members"]["nemesis"]


class AuditorReason(StrEnum):
    """The Auditor's stated reason for its verdict.

    These values are emitted in Tribunal SSE payloads and must match
    the shared constants in shared/constants/agents.json.
    """
    OK                   = _AGENTS["tribunal.auditor_reason"]["ok"]
    REVISED              = _AGENTS["tribunal.auditor_reason"]["revised"]
    EMPTY_RESPONSE       = _AGENTS["tribunal.auditor_reason"]["empty_response"]
    NO_VALID_REVISION    = _AGENTS["tribunal.auditor_reason"]["no_valid_revision"]
    AUDITOR_ERROR        = _AGENTS["tribunal.auditor_reason"]["auditor_error"]
    SWAPPED_TO_DISSENTER = _AGENTS["tribunal.auditor_reason"]["swapped_to_dissenter"]
    REVISED_FROM_DISSENT = _AGENTS["tribunal.auditor_reason"]["revised_from_dissent"]
    WHITELIST_VIOLATION  = _AGENTS["tribunal.auditor_reason"]["whitelist_violation"]


class TieBreakReason(StrEnum):
    """How a tie at the top of the uniform vote was resolved.

    Populated on VoteBreakdown only when more than one command cluster
    held the highest vote count and one of the deterministic tie-break
    rules resolved it.
    """
    SHORTEST         = _AGENTS["tribunal.tie_break_reason"]["shortest"]
    EXCLUDED_NEMESIS = _AGENTS["tribunal.tie_break_reason"]["excluded_nemesis"]
    ALPHABETICAL     = _AGENTS["tribunal.tie_break_reason"]["alphabetical"]
