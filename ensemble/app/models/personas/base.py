# Copyright (c) 2026 Lateralus Labs, LLC.
# Use of this source code is governed by the Business Source License
# included in the LICENSE file.
#
# As of the Change Date listed in the LICENSE file, this software is
# released under the Apache License, Version 2.0.

from app.models.base import BaseModel, Field, ConfigDict


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
