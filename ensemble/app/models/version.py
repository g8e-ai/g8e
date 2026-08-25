# Copyright (c) 2026 Lateralus Labs, LLC.
# Use of this source code is governed by the Business Source License
# included in the LICENSE file.
#
# As of the Change Date listed in the LICENSE file, this software is
# released under the Apache License, Version 2.0.

from app.models.base import G8eBaseModel, Field


class VersionInfo(G8eBaseModel):
    """Version information for a g8e component."""

    version: str = Field(description="Semver version string (e.g. v0.1.3)")
