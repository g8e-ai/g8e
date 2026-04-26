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

"""Per-tool modules for the AI tool surface.

Each module owns exactly one tool and exports two callables:

- ``build()`` -> ``(types.ToolDeclaration, executor_stub)`` used at
  ``AIToolService`` construction time to register the tool with the LLM.
- ``handle(svc, tool_args, investigation, g8e_context, request_settings,
  execution_id) -> ToolResult`` invoked from ``execute_tool_call`` to
  dispatch a single tool call.

The ``ToolSpec`` entries in :mod:`app.services.ai.tool_registry` reference
these callables directly. Adding a new tool means creating one module here
plus one ``ToolSpec`` entry; nothing on ``AIToolService`` itself changes.
"""
