# Copyright (c) 2026 Lateralus Labs, LLC.
# Use of this source code is governed by the Business Source License
# included in the LICENSE file.
#
# As of the Change Date listed in the LICENSE file, this software is
# released under the Apache License, Version 2.0.

"""Evaluation constants and mappings."""

from app.constants import ReasoningAgent

DesignatedRoleToAgent = {
    "primary": ReasoningAgent.SAGE,
    "assistant": ReasoningAgent.DASH,
    "lite": ReasoningAgent.DASH,
}
