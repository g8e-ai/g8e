# Copyright (c) 2026 Lateralus Labs, LLC.
# Use of this source code is governed by the Business Source License
# included in the LICENSE file.
#
# As of the Change Date listed in the LICENSE file, this software is
# released under the Apache License, Version 2.0.

"""Pending restoration tools for the tool registry.

Tools in this set are OperatorToolName enum values that have not yet been
restored to the active tool registry after a prior refactoring. Each entry
represents a tool that needs a ``ToolSpec`` in ``tool_registry.TOOL_SPECS``
and corresponding ``_build_*`` / ``_handle_*`` methods on ``AIToolService``.

**Do not add to this set without explicit, documented reason.** Removing
an entry requires adding the full implementation. See
``docs/architecture/ai_agents.md`` for tool restoration guidance.
"""

from app.constants.generated_status import OperatorToolName

PENDING_RESTORATION: frozenset[str] = frozenset(
    {
        OperatorToolName.READ_FILE_CONTENT.value,
        OperatorToolName.FETCH_EXECUTION_OUTPUT.value,
        OperatorToolName.FETCH_SESSION_HISTORY.value,
        OperatorToolName.RESTORE_FILE.value,
    }
)
