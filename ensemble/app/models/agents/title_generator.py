# Copyright (c) 2026 Lateralus Labs, LLC.
# Use of this source code is governed by the Business Source License
# included in the LICENSE file.
#
# As of the Change Date listed in the LICENSE file, this software is
# released under the Apache License, Version 2.0.

from app.models.base import G8eBaseModel, Field


class CaseTitleRequest(G8eBaseModel):
    """Request model for case title generation."""

    description: str = Field(description="The case description or initial message.")
    max_length: int = Field(default=80, ge=10, le=200, description="Maximum title length.")


class CaseTitleResult(G8eBaseModel):
    """The generated title from the utility."""

    generated_title: str = Field(description="The concise, meaningful generated title.")
    fallback: bool = Field(default=False, description="True if the title is a non-AI fallback.")
