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

"""Pending restoration tools for the tool registry.

Tools in this set are OperatorToolName enum values that have not yet been
restored to the active tool registry after a prior refactoring. Each entry
represents a tool that needs a ``ToolSpec`` in ``tool_registry.TOOL_SPECS``
and corresponding ``_build_*`` / ``_handle_*`` methods on ``AIToolService``.

**Do not add to this set without explicit, documented reason.** Removing
an entry requires adding the full implementation. See
``docs/architecture/ai_agents.md`` for tool restoration guidance.
"""

from app.constants.status import OperatorToolName

PENDING_RESTORATION: frozenset[str] = frozenset({
    OperatorToolName.READ_FILE_CONTENT.value,
    OperatorToolName.FETCH_EXECUTION_OUTPUT.value,
    OperatorToolName.FETCH_SESSION_HISTORY.value,
    OperatorToolName.RESTORE_FILE.value,
})
