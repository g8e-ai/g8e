# Copyright (c) 2026 Lateralus Labs, LLC.
# Use of this source code is governed by the Business Source License
# included in the LICENSE file.
#
# As of the Change Date listed in the LICENSE file, this software is
# released under the Apache License, Version 2.0.

from app.models.base import G8eBaseModel, Field
from app.constants import AuditorReason


class AuditorRequest(G8eBaseModel):
    """Request model for the Auditor agent."""

    intent: str = Field(description="The original user intent.")
    os: str = Field(description="The operating system of the target.")
    candidate_command: str = Field(description="The command string to be verified.")


class AuditorResult(G8eBaseModel):
    """Result from the Tribunal auditor evaluation."""

    passed: bool = Field(description="True if the auditor approves the candidate.")
    revision: str | None = Field(
        default=None, description="The revised command string if the auditor rejects the candidate."
    )
    reason: str = Field(description="Reasoning for the approval or rejection.")
    reason_enum: AuditorReason = Field(description="Canonical reason for the auditor result.")
