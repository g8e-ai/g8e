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

    model_config = ConfigDict(populate_by_name=True)

    def get_system_prompt(self) -> str:
        """Build a system prompt from identity, purpose, role, and autonomy fields."""
        return f"<role>\n{self.role}\n</role>\n\n<identity>\n{self.identity}\n</identity>\n\n<purpose>\n{self.purpose}\n</purpose>\n\n<autonomy>\n{self.autonomy}\n</autonomy>"


def _load_agents_json() -> dict[str, Any]:
    """Load the agents.json file from shared constants."""
    try:
        with open(_AGENTS_JSON_PATH, "r") as f:
            return json.load(f)
    except FileNotFoundError:
        logger.error(f"Agents file not found at {_AGENTS_JSON_PATH}")
        raise
    except json.JSONDecodeError as e:
        logger.error(f"Failed to parse agents.json: {e}")
        raise


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
        member_id: Tribunal member identifier (atom, clio, nemesis)
        
    Returns:
        AgentPersona for the specified Tribunal member
    """
    return get_agent_persona(member_id)
