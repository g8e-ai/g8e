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

from pydantic import BaseModel, Field, ConfigDict


class AgentPersonaModel(BaseModel):
    """Base model for all AI agent personas.

    This replaces the JSON-based persona definitions with structured code models,
    ensuring consistency, validation, and strong alignment with the architecture.
    """
    id: str
    display_name: str
    icon: str
    description: str
    role: str
    model_tier: str
    tools: list[str] = Field(default_factory=list)
    identity: str
    purpose: str
    autonomy: str
    output_contract: str | None = None

    model_config = ConfigDict(frozen=True)

    def format_xml_tag(self, tag_name: str, content: str) -> str:
        """Format content within XML tags with consistent structure."""
        return f"<{tag_name}>\n{content.strip()}\n</{tag_name}>"

    def get_system_prompt(self) -> str:
        """Build a system prompt from persona fields following the canonical layout."""
        parts = [self.format_xml_tag("role", self.role)]

        if self.output_contract:
            parts.append(self.format_xml_tag("output_contract", self.output_contract))

        parts.append(self.format_xml_tag("identity", self.identity))

        if self.purpose:
            parts.append(self.format_xml_tag("purpose", self.purpose))

        if self.autonomy:
            parts.append(self.format_xml_tag("autonomy", self.autonomy))

        return "\n\n".join(parts)
