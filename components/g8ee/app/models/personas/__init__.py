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

from .base import AgentPersonaModel
from .triage import TriagePersona
from .sage import SagePersona
from .dash import DashPersona
from .auditor import AuditorPersona
from .axiom import AxiomPersona
from .concord import ConcordPersona
from .variance import VariancePersona
from .pragma import PragmaPersona
from .nemesis import NemesisPersona
from .scribe import ScribePersona
from .codex import CodexPersona
from .judge import JudgePersona
from .warden import (
    WardenPersona,
    WardenCommandRiskPersona,
    WardenErrorPersona,
    WardenFileRiskPersona,
)
from .tribunal import TribunalPersona

PERSONA_REGISTRY: dict[str, AgentPersonaModel] = {
    "triage": TriagePersona(),
    "sage": SagePersona(),
    "dash": DashPersona(),
    "auditor": AuditorPersona(),
    "axiom": AxiomPersona(),
    "concord": ConcordPersona(),
    "variance": VariancePersona(),
    "pragma": PragmaPersona(),
    "nemesis": NemesisPersona(),
    "scribe": ScribePersona(),
    "codex": CodexPersona(),
    "judge": JudgePersona(),
    "warden": WardenPersona(),
    "warden_command_risk": WardenCommandRiskPersona(),
    "warden_error": WardenErrorPersona(),
    "warden_file_risk": WardenFileRiskPersona(),
    "tribunal": TribunalPersona(),
}


def get_persona(agent_id: str) -> AgentPersonaModel:
    """Retrieve a persona by agent ID from the registry."""
    if agent_id not in PERSONA_REGISTRY:
        raise KeyError(f"Persona '{agent_id}' not found in registry.")
    return PERSONA_REGISTRY[agent_id]


def list_persona_ids() -> list[str]:
    """List all registered persona IDs."""
    return list(PERSONA_REGISTRY.keys())
