# Copyright (c) 2026 Lateralus Labs, LLC.
# Use of this source code is governed by the Business Source License
# included in the LICENSE file.
#
# As of the Change Date listed in the LICENSE file, this software is
# released under the Apache License, Version 2.0.

"""Per-tool modules for the AI tool surface.

Each module owns exactly one tool and exports two callables:

- ``build() -> types.ToolDeclaration`` used at ``AIToolService``
  construction time to register the tool with the LLM.
- ``handle(svc, tool_args, investigation, g8e_context, request_settings,
  execution_id) -> ToolResult`` invoked from ``execute_tool_call`` to
  dispatch a single tool call.

The ``ToolSpec`` entries in :mod:`app.services.ai.tool_registry` reference
these callables directly. Adding a new tool means creating one module here
plus one ``ToolSpec`` entry; nothing on ``AIToolService`` itself changes.
"""
