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

from app.constants import InvestigationStatus

from .base import Field, G8eBaseModel, G8eIdentifiableModel


class InvestigationMemory(G8eIdentifiableModel):
    case_id: str = Field(..., description="Associated case ID")
    investigation_id: str = Field(..., description="Investigation this memory represents")
    user_id: str = Field(..., description="User ID who owns this investigation")
    status: InvestigationStatus = Field(..., description="Current investigation status")
    case_title: str = Field(..., description="Title of the case")
    investigation_summary: str = Field(default="", description="High-level summary of what the conversation is about, without system names, IPs, or sensitive details")
    communication_preferences: str = Field(default="", description="How the user prefers to communicate and receive information")
    technical_background: str = Field(default="", description="User's technical experience, skills, and comfort levels")
    response_style: str = Field(default="", description="Preferred format, detail level, and structure of responses")
    problem_solving_approach: str = Field(default="", description="How the user likes to debug, investigate, and solve problems")
    interaction_style: str = Field(default="", description="Meta-preferences about questions, context, and follow-ups")


class MemoryAnalysis(G8eBaseModel):
    investigation_summary: str = Field(default="", description="High-level summary of the conversation. No system names, hostnames, IPs, or sensitive identifiers. Use generic terms: 'a Linux system', 'their Docker setup'.")
    communication_preferences: str = Field(default="", description="How the user prefers to communicate: verbosity, tone, format.")
    technical_background: str = Field(default="", description="User's technical experience level and areas of expertise.")
    response_style: str = Field(default="", description="How the user wants information presented: code completeness, comments, alternatives.")
    problem_solving_approach: str = Field(default="", description="How the user approaches debugging and problem-solving.")
    interaction_style: str = Field(default="", description="Meta-preferences about questions, context, and follow-ups.")


__all__ = ["InvestigationMemory", "MemoryAnalysis"]
