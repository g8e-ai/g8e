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

from enum import Enum
from app.constants.shared import _AGENTS


class TriageComplexityClassification(str, Enum):
    __str__ = lambda self: self.value
    SIMPLE  = _AGENTS["triage.complexity"]["simple"]
    COMPLEX = _AGENTS["triage.complexity"]["complex"]


class TriageConfidence(str, Enum):
    """Confidence level for triage classifications (both complexity and intent)."""
    __str__ = lambda self: self.value
    HIGH    = _AGENTS["triage.confidence"]["high"]
    LOW     = _AGENTS["triage.confidence"]["low"]


class TriageIntentClassification(str, Enum):
    __str__ = lambda self: self.value
    INFORMATION = _AGENTS["triage.intent"]["information"]
    ACTION      = _AGENTS["triage.intent"]["action"]
    UNKNOWN     = _AGENTS["triage.intent"]["unknown"]


class TriageRequestPosture(str, Enum):
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
    __str__ = lambda self: self.value
    NORMAL      = _AGENTS["triage.posture"]["normal"]
    ESCALATED   = _AGENTS["triage.posture"]["escalated"]
    ADVERSARIAL = _AGENTS["triage.posture"]["adversarial"]
    CONFUSED    = _AGENTS["triage.posture"]["confused"]


class TribunalMember(str, Enum):
    """The five permanent members of the Tribunal.

    Each member has a fixed name and role that defines its reasoning profile.
    The ordering is canonical: Axiom (pass 0), Concord (pass 1), Variance (pass 2),
    Pragma (pass 3), Nemesis (pass 4). When more than 5 passes are configured the members cycle.
    """
    __str__ = lambda self: self.value
    AXIOM   = _AGENTS["tribunal.members"]["axiom"]
    CONCORD = _AGENTS["tribunal.members"]["concord"]
    VARIANCE = _AGENTS["tribunal.members"]["variance"]
    PRAGMA = _AGENTS["tribunal.members"]["pragma"]
    NEMESIS = _AGENTS["tribunal.members"]["nemesis"]


class VerifierReason(str, Enum):
    """The Verifier's stated reason for its verdict.

    These values are emitted in Tribunal SSE payloads and must match
    the shared constants in shared/constants/agents.json.
    """
    __str__ = lambda self: self.value
    OK                = _AGENTS["tribunal.verifier_reason"]["ok"]
    REVISED           = _AGENTS["tribunal.verifier_reason"]["revised"]
    EMPTY_RESPONSE    = _AGENTS["tribunal.verifier_reason"]["empty_response"]
    NO_VALID_REVISION = _AGENTS["tribunal.verifier_reason"]["no_valid_revision"]
    VERIFIER_ERROR    = _AGENTS["tribunal.verifier_reason"]["verifier_error"]
