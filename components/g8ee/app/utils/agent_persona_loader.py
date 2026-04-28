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

"""Agent Persona Loader

Centralized loader for AI agent persona definitions from shared/constants/agents.json.
Provides a single source of truth for agent identities, purposes, and prompt templates.
"""

import json
import logging
from functools import lru_cache
from pathlib import Path
from typing import Any
from pydantic import BaseModel, ConfigDict, Field

logger = logging.getLogger(__name__)

_AGENTS_JSON_PATH = Path(__file__).parent.parent.parent.parent.parent / "shared" / "constants" / "agents.json"


class AgentPersona(BaseModel):
    """Represents a complete agent persona definition with Pydantic validation."""

    agent_id: str = Field(..., alias="id")
    display_name: str = Field(..., alias="display_name")
    icon: str = Field(..., alias="icon")
    description: str = Field(..., alias="description")
    role: str = Field(..., alias="role")
    model_tier: str = Field(..., alias="model_tier")
    tools: list[str] = Field(default_factory=list, alias="tools")
    identity: str = Field(..., alias="identity")
    purpose: str = Field(..., alias="purpose")
    autonomy: str = Field(..., alias="autonomy")
    output_contract: str | None = Field(default=None, alias="output_contract")

    model_config = ConfigDict(populate_by_name=True)

    @staticmethod
    def format_xml_tag(tag_name: str, content: str) -> str:
        """Format content within XML tags with consistent structure.

        Enforces the canonical XML scaffolding pattern:
        - Opening tag on its own line
        - Content on its own line
        - Closing tag on its own line

        This guarantees hard structural boundaries required by the architecture.

        Args:
            tag_name: The XML tag name (without angle brackets)
            content: The content to wrap in the tag

        Returns:
            Formatted XML string with the tag structure
        """
        return f"<{tag_name}>\n{content}\n</{tag_name}>"

    def get_system_prompt(self) -> str:
        """Build a system prompt from persona fields following the canonical layout.

        Canonical layout per docs/architecture/agent_personas.md:
        1. <role>
        2. <output_contract> (only if present as explicit field; the
           ``output_contract`` tag MUST NOT be embedded in ``identity`` —
           that contract is enforced by
           ``test_prompt_alignment::test_no_persona_embeds_output_contract_in_identity``).
        3. <identity>
        4. <purpose>
        5. <autonomy>
        """
        parts = [self.format_xml_tag("role", self.role)]

        if self.output_contract:
            parts.append(self.format_xml_tag("output_contract", self.output_contract))

        parts.append(self.format_xml_tag("identity", self.identity))

        if self.purpose:
            parts.append(self.format_xml_tag("purpose", self.purpose))

        if self.autonomy:
            parts.append(self.format_xml_tag("autonomy", self.autonomy))

        return "\n\n".join(parts)


@lru_cache(maxsize=1)
def _load_agents_json() -> dict[str, Any]:
    """Load the agents.json file from shared constants.

    Cached: agents.json is immutable at runtime. Prompt builds happen on
    every chat turn and previously re-read + re-parsed the file each time.
    """
    try:
        with open(_AGENTS_JSON_PATH, "r") as f:
            return json.load(f)
    except FileNotFoundError:
        logger.error(f"Agents file not found at {_AGENTS_JSON_PATH}")
        raise
    except json.JSONDecodeError as e:
        logger.error(f"Failed to parse agents.json: {e}")
        raise


def clear_agents_json_cache() -> None:
    """Clear the agents.json read cache. Test-only."""
    _load_agents_json.cache_clear()


def get_agent_persona(agent_id: str) -> AgentPersona:
    """Retrieve an agent persona by ID.
    
    Args:
        agent_id: The agent identifier (e.g., "triage", "primary", "assistant")
        
    Returns:
        AgentPersona object with all metadata and prompt template
        
    Raises:
        KeyError: If agent_id is not found in agents.json
        ValidationError: If agent data fails Pydantic validation
    """
    agents_data = _load_agents_json()
    agent_metadata = agents_data.get("agent.metadata", {})
    
    if agent_id not in agent_metadata:
        available = ", ".join(agent_metadata.keys())
        raise KeyError(
            f"Agent '{agent_id}' not found in agents.json. "
            f"Available agents: {available}"
        )
    
    data = agent_metadata[agent_id]
    
    return AgentPersona.model_validate(data)


def list_all_agents() -> list[str]:
    """Return a list of all available agent IDs."""
    agents_data = _load_agents_json()
    agent_metadata = agents_data.get("agent.metadata", {})
    return list(agent_metadata.keys())


def get_tribunal_member(member_id: str) -> AgentPersona:
    """Retrieve a Tribunal member persona by member ID.

    Args:
        member_id: Tribunal member identifier (axiom, concord, variance, pragma, nemesis)

    Returns:
        AgentPersona for the specified Tribunal member
    """
    return get_agent_persona(member_id)
