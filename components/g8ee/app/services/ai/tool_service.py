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

from __future__ import annotations

import logging
from collections.abc import Callable
from contextvars import ContextVar, Token as ContextVarToken
from app.services.operator.command_service import OperatorCommandService

import app.llm.llm_types as types
from app.constants.status import (
    CommandErrorType,
    ComponentName,
    FileOperation,
    OperatorToolName,
)
from app.constants.prompts import AgentMode, PromptFile
from app.constants.settings import FORBIDDEN_COMMAND_PATTERNS
from app.models.http_context import VSOHttpContext
from app.models.investigations import EnrichedInvestigationContext
from app.models.settings import G8eeUserSettings
from app.llm.prompts import load_prompt
from app.errors import ExternalServiceError, ValidationError, ConfigurationError
from app.llm.llm_types import schema_from_model
from app.models.agent import OperatorCommandArgs
from app.models.model_configs import get_model_config
from app.models.tool_results import (
    CommandExecutionResult,
    FetchFileHistoryToolResult,
    FetchFileDiffToolResult,
    FileEditResult,
    FsListToolResult,
    FsReadToolResult,
    IntentPermissionResult,
    PortCheckToolResult,
    ToolResult,
)
from app.models.command_payloads import (
    CheckPortArgs,
    FileCreateArgs,
    FileEditPayload,
    FileReadArgs,
    FileUpdateArgs,
    FileWriteArgs,
    FetchFileHistoryArgs,
    FetchFileDiffArgs,
    FsListArgs,
    FsReadArgs,
    GrantIntentArgs,
    RevokeIntentArgs,
    SearchWebArgs,
)

from .grounding.web_search_provider import WebSearchProvider
from ..investigation.investigation_service import InvestigationService

logger = logging.getLogger(__name__)


class AIToolService:
    """Service for AI tool registration and execution on bound operators."""

    def __init__(
        self,
        operator_command_service: "OperatorCommandService",
        investigation_service: InvestigationService,
        web_search_provider: WebSearchProvider | None,
    ):
        self.operator_command_service = operator_command_service
        self.investigation_service = investigation_service
        self._web_search_provider: WebSearchProvider | None = web_search_provider
        logger.info("AIToolService initialized")

        self._tool_context: ContextVar[VSOHttpContext | None] = ContextVar(
            "g8ee_tool_context",
            default=None
        )

        self._tool_declarations: dict[str, types.ToolDeclaration] = {}
        self._tool_executors: dict[str, Callable[..., ToolResult]] = {}

        (self._tool_declarations[OperatorToolName.RUN_COMMANDS],
         self._tool_executors[OperatorToolName.RUN_COMMANDS]) = self._build_run_operator_commands_tool()

        (self._tool_declarations[OperatorToolName.FILE_CREATE],
         self._tool_executors[OperatorToolName.FILE_CREATE]) = self._build_file_create_tool()

        (self._tool_declarations[OperatorToolName.FILE_WRITE],
         self._tool_executors[OperatorToolName.FILE_WRITE]) = self._build_file_write_tool()

        (self._tool_declarations[OperatorToolName.FILE_READ],
         self._tool_executors[OperatorToolName.FILE_READ]) = self._build_file_read_tool()

        (self._tool_declarations[OperatorToolName.FILE_UPDATE],
         self._tool_executors[OperatorToolName.FILE_UPDATE]) = self._build_file_update_tool()

        if self.web_search_provider is not None:
            (self._tool_declarations[OperatorToolName.G8E_SEARCH_WEB],
             self._tool_executors[OperatorToolName.G8E_SEARCH_WEB]) = self._build_search_web_tool()
            logger.info("[TOOLS] g8e_web_search enabled (Vertex AI Search configured)")
        else:
            logger.info("[TOOLS] g8e_web_search disabled (VERTEX_SEARCH_ENABLED not set or credentials missing)")

        (self._tool_declarations[OperatorToolName.CHECK_PORT],
         self._tool_executors[OperatorToolName.CHECK_PORT]) = self._build_port_check_tool()

        (self._tool_declarations[OperatorToolName.LIST_FILES],
         self._tool_executors[OperatorToolName.LIST_FILES]) = self._build_list_directory_tool()

        (self._tool_declarations[OperatorToolName.GRANT_INTENT],
         self._tool_executors[OperatorToolName.GRANT_INTENT]) = self._build_grant_intent_permission_tool()

        (self._tool_declarations[OperatorToolName.REVOKE_INTENT],
         self._tool_executors[OperatorToolName.REVOKE_INTENT]) = self._build_revoke_intent_permission_tool()

        (self._tool_declarations[OperatorToolName.FETCH_FILE_HISTORY],
         self._tool_executors[OperatorToolName.FETCH_FILE_HISTORY]) = self._build_fetch_file_history_tool()

        (self._tool_declarations[OperatorToolName.FETCH_FILE_DIFF],
         self._tool_executors[OperatorToolName.FETCH_FILE_DIFF]) = self._build_fetch_file_diff_tool()

    def start_invocation_context(
        self,
        vso_context: VSOHttpContext,
    ) -> ContextVarToken[VSOHttpContext | None]:
        """Set the vso_context used by tool calls for the active request."""
        token = self._tool_context.set(vso_context)
        bound_count = len(vso_context.bound_operators) if vso_context else 0
        case_id = vso_context.case_id if vso_context else None
        user_id = vso_context.user_id if vso_context else None
        logger.info(
            "[TOOL_CONTEXT] Context initialized: case_id=%s user_id=%s bound_operators=%d",
            case_id, user_id, bound_count
        )
        return token

    def reset_invocation_context(self, token: ContextVarToken[VSOHttpContext | None]) -> None:
        """Reset invocation context after request completion."""
        self._tool_context.reset(token)
        logger.info("[TOOL_CONTEXT] Context reset")

    @property
    def web_search_provider(self) -> WebSearchProvider | None:
        """The configured WebSearchProvider, or None if g8e_web_search is not enabled."""
        return self._web_search_provider

    @property
    def g8e_web_search_available(self) -> bool:
        """True when the g8e_web_search tool is registered (Google Custom Search configured)."""
        return OperatorToolName.G8E_SEARCH_WEB in self._tool_declarations

    def get_tools(
        self,
        agent_mode: AgentMode,
        model_to_use: str | None
    ) -> list[types.ToolGroup]:
        """Build tool declarations based on Operator workflow.

        - OPERATOR_NOT_BOUND: g8e_web_search only (when configured)
        - OPERATOR_BOUND: All tool declarations (g8e_web_search when configured + operator tools)

        Returns empty list if the model does not support tools.
        """
        if model_to_use:
            config = get_model_config(model_to_use)
            if not config.supports_tools:
                logger.info("[TOOLS] Model %s does not support tools, skipping tool declarations", model_to_use)
                return []

        g8e_web_search_available = OperatorToolName.G8E_SEARCH_WEB in self._tool_declarations
        resolved_workflow = agent_mode or AgentMode.OPERATOR_NOT_BOUND

        if resolved_workflow == AgentMode.OPERATOR_NOT_BOUND:
            if not g8e_web_search_available:
                logger.info("[TOOLS] OPERATOR_NOT_BOUND: no tools available (g8e_web_search not configured)")
                return []
            return [
                types.ToolGroup(
                    tools=[
                        self._tool_declarations[OperatorToolName.G8E_SEARCH_WEB],
                    ]
                )
            ]

        operator_declarations = [
            self._tool_declarations[OperatorToolName.RUN_COMMANDS],
            self._tool_declarations[OperatorToolName.FILE_CREATE],
            self._tool_declarations[OperatorToolName.FILE_WRITE],
            self._tool_declarations[OperatorToolName.FILE_READ],
            self._tool_declarations[OperatorToolName.FILE_UPDATE],
            self._tool_declarations[OperatorToolName.LIST_FILES],
            self._tool_declarations[OperatorToolName.FETCH_FILE_HISTORY],
            self._tool_declarations[OperatorToolName.FETCH_FILE_DIFF],
            self._tool_declarations[OperatorToolName.GRANT_INTENT],
            self._tool_declarations[OperatorToolName.REVOKE_INTENT],
            self._tool_declarations[OperatorToolName.CHECK_PORT],
        ]
        if g8e_web_search_available:
            operator_declarations.insert(0, self._tool_declarations[OperatorToolName.G8E_SEARCH_WEB])
        return [
            types.ToolGroup(tools=operator_declarations)
        ]

    def _build_run_operator_commands_tool(self) -> tuple[types.ToolDeclaration, Callable[..., ToolResult]]:
        """Register tool metadata and executor for Operator command execution."""

        def run_commands_with_operator(args: OperatorCommandArgs) -> ToolResult:
            raise NotImplementedError("run_commands_with_operator should be called via execute_tool_call")

        declaration = types.ToolDeclaration(
            name=OperatorToolName.RUN_COMMANDS,
            description=load_prompt(PromptFile.TOOL_RUN_COMMANDS),
            parameters=schema_from_model(
                OperatorCommandArgs,
                required_override=["command", "justification"],
            ),
        )

        return declaration, run_commands_with_operator

    def _build_file_create_tool(self) -> tuple[types.ToolDeclarations, Callable[..., ToolResult]]:
        """Register tool metadata and executor for file creation operations."""

        def file_create_on_operator(args: FileEditPayload) -> ToolResult:
            raise NotImplementedError("file_create_on_operator should be called via execute_tool_call")

        declaration = types.ToolDeclaration(
            name=OperatorToolName.FILE_CREATE,
            description=load_prompt(PromptFile.TOOL_FILE_CREATE),
            parameters=schema_from_model(FileCreateArgs),
        )

        return declaration, file_create_on_operator

    def _build_file_write_tool(self) -> tuple[types.ToolDeclaration, Callable[..., ToolResult]]:
        """Register tool metadata and executor for file write (overwrite) operations."""

        def file_write_on_operator(args: FileEditPayload) -> ToolResult:
            raise NotImplementedError("file_write_on_operator should be called via execute_tool_call")

        declaration = types.ToolDeclaration(
            name=OperatorToolName.FILE_WRITE,
            description=load_prompt(PromptFile.TOOL_FILE_WRITE),
            parameters=schema_from_model(FileWriteArgs),
        )

        return declaration, file_write_on_operator

    def _build_file_read_tool(self) -> tuple[types.ToolDeclaration, Callable[..., ToolResult]]:
        """Register tool metadata and executor for file read operations."""

        def file_read_on_operator(args: FileEditPayload) -> ToolResult:
            raise NotImplementedError("file_read_on_operator should be called via execute_tool_call")

        declaration = types.ToolDeclaration(
            name=OperatorToolName.FILE_READ,
            description=load_prompt(PromptFile.TOOL_FILE_READ),
            parameters=schema_from_model(FileReadArgs),
        )

        return declaration, file_read_on_operator

    def _build_file_update_tool(self) -> tuple[types.ToolDeclaration, Callable[..., ToolResult]]:
        """Register tool metadata and executor for file update (find-and-replace) operations."""

        def file_update_on_operator(args: FileEditPayload) -> ToolResult:
            raise NotImplementedError("file_update_on_operator should be called via execute_tool_call")

        declaration = types.ToolDeclaration(
            name=OperatorToolName.FILE_UPDATE,
            description=load_prompt(PromptFile.TOOL_FILE_UPDATE),
            parameters=schema_from_model(FileUpdateArgs),
        )

        return declaration, file_update_on_operator

    def _build_search_web_tool(self) -> tuple[types.ToolDeclaration, Callable[..., ToolResult]]:
        """Register tool metadata and executor for web search via WebSearchProvider."""
        assert self.web_search_provider is not None, "_build_search_web_tool called before WebSearchProvider was initialised"

        def g8e_web_search(args: SearchWebArgs) -> ToolResult:
            raise NotImplementedError("g8e_web_search should be called via execute_tool_call")

        declaration = types.ToolDeclaration(
            name=OperatorToolName.G8E_SEARCH_WEB,
            description=load_prompt(PromptFile.TOOL_SEARCH_WEB),
            parameters=schema_from_model(SearchWebArgs),
        )

        return declaration, g8e_web_search

    def _build_port_check_tool(self) -> tuple[types.ToolDeclaration, Callable[..., ToolResult]]:
        """Register tool metadata and executor for port check operations."""

        def check_port_status(args: CheckPortArgs) -> ToolResult:
            raise NotImplementedError("check_port_status should be called via execute_tool_call")

        declaration = types.ToolDeclaration(
            name=OperatorToolName.CHECK_PORT,
            description=load_prompt(PromptFile.TOOL_CHECK_PORT),
            parameters=schema_from_model(CheckPortArgs, required_override=["port"]),
        )

        return declaration, check_port_status

    def _build_list_directory_tool(self) -> tuple[types.ToolDeclaration, Callable[..., ToolResult]]:
        """Register tool metadata and executor for directory listing operations."""

        def list_files_and_directories_with_detailed_metadata(args: FsListArgs) -> ToolResult:
            raise NotImplementedError("list_files_and_directories_with_detailed_metadata should be called via execute_tool_call")

        declaration = types.ToolDeclaration(
            name=OperatorToolName.LIST_FILES,
            description=load_prompt(PromptFile.TOOL_LIST_FILES),
            parameters=schema_from_model(FsListArgs),
        )

        return declaration, list_files_and_directories_with_detailed_metadata

    def _build_grant_intent_permission_tool(self) -> tuple[types.ToolDeclaration, Callable[..., ToolResult]]:
        """Register tool metadata and executor for requesting AWS intent permissions."""

        def grant_intent_permission(args: GrantIntentArgs) -> ToolResult:
            raise NotImplementedError("grant_intent_permission should be called via execute_tool_call")

        declaration = types.ToolDeclaration(
            name=OperatorToolName.GRANT_INTENT,
            description=load_prompt(PromptFile.TOOL_GRANT_INTENT),
            parameters=schema_from_model(GrantIntentArgs),
        )

        return declaration, grant_intent_permission

    def _build_revoke_intent_permission_tool(self) -> tuple[types.ToolDeclaration, Callable[..., ToolResult]]:
        """Register tool metadata and executor for revoking AWS intent permissions."""

        def revoke_intent_permission(args: RevokeIntentArgs) -> ToolResult:
            raise NotImplementedError("revoke_intent_permission should be called via execute_tool_call")

        declaration = types.ToolDeclaration(
            name=OperatorToolName.REVOKE_INTENT,
            description=load_prompt(PromptFile.TOOL_REVOKE_INTENT),
            parameters=schema_from_model(RevokeIntentArgs),
        )

        return declaration, revoke_intent_permission

    def _build_fetch_file_history_tool(self) -> tuple[types.ToolDeclaration, Callable[..., ToolResult]]:
        """Register tool metadata and executor for file history operations."""

        def fetch_file_history(args: FetchFileHistoryArgs) -> ToolResult:
            raise NotImplementedError("fetch_file_history should be called via execute_tool_call")

        declaration = types.ToolDeclaration(
            name=OperatorToolName.FETCH_FILE_HISTORY,
            description=load_prompt(PromptFile.TOOL_FETCH_FILE_HISTORY),
            parameters=schema_from_model(FetchFileHistoryArgs),
        )

        return declaration, fetch_file_history

    def _build_fetch_file_diff_tool(self) -> tuple[types.ToolDeclaration, Callable[..., ToolResult]]:
        """Register tool metadata and executor for file diff operations."""

        def fetch_file_diff(args: FetchFileDiffArgs) -> ToolResult:
            raise NotImplementedError("fetch_file_diff should be called via execute_tool_call")

        declaration = types.ToolDeclaration(
            name=OperatorToolName.FETCH_FILE_DIFF,
            description=load_prompt(PromptFile.TOOL_FETCH_FILE_DIFF),
            parameters=schema_from_model(FetchFileDiffArgs),
        )

        return declaration, fetch_file_diff

    async def execute_tool_call(
        self,
        tool_name: str,
        tool_args: dict[str, object],
        investigation: EnrichedInvestigationContext,
        vso_context: VSOHttpContext,
        request_settings: G8eeUserSettings,
    ) -> ToolResult:
        """Validate, dispatch, and execute a single AI tool call by name."""
        if tool_name == OperatorToolName.RUN_COMMANDS:
            raw_command = tool_args.get("command", "")
            command_lower = raw_command.lower() if isinstance(raw_command, str) else ""
            for pattern in FORBIDDEN_COMMAND_PATTERNS:
                if pattern in command_lower:
                    error_msg = (
                        f"SECURITY VIOLATION: Command contains forbidden pattern '{pattern}'. "
                        f"Privilege escalation commands (sudo, su, pkexec, doas, etc.) are strictly prohibited. "
                        f"If root privileges are required, ask the user to restart the Operator with sudo "
                        f"(e.g., 'sudo ./g8eo' or 'sudo g8eo'). Do NOT attempt to use sudo in commands."
                    )
                    logger.error("[SECURITY] Blocked forbidden command pattern '%s' in: %s", pattern, raw_command)
                    return CommandExecutionResult(
                        success=False,
                        error=error_msg,
                        error_type=CommandErrorType.SECURITY_VIOLATION,
                        blocked_pattern=pattern
                    )

        # CRITICAL: Validate operator binding from VSOHttpContext before any operator-bound tool execution.
        operator_tools = {
            OperatorToolName.RUN_COMMANDS,
            OperatorToolName.FILE_CREATE,
            OperatorToolName.FILE_WRITE,
            OperatorToolName.FILE_READ,
            OperatorToolName.FILE_UPDATE,
            OperatorToolName.CHECK_PORT,
            OperatorToolName.LIST_FILES,
            OperatorToolName.FETCH_FILE_HISTORY,
            OperatorToolName.FETCH_FILE_DIFF,
            OperatorToolName.GRANT_INTENT,
            OperatorToolName.REVOKE_INTENT,
        }

        if tool_name in operator_tools:
            if not vso_context or not vso_context.has_bound_operator():
                error_msg = (
                    "No operators are currently BOUND to this session. "
                    "Operator commands can only be executed when an operator is explicitly bound in the g8e UI."
                )
                logger.error("[TOOL_CALL] Execution blocked: No bound operators in VSOHttpContext")
                return CommandExecutionResult(
                    success=False,
                    error=error_msg,
                    error_type=CommandErrorType.NO_OPERATORS_AVAILABLE
                )

        logger.info("[TOOL_CALL] Starting execution: %s", tool_name)
        logger.info("[TOOL_CALL] Args: %s", tool_args)
        logger.info("[TOOL_CALL] Context - case_id: %s, user_id: %s",
            vso_context.case_id if vso_context else None,
            vso_context.user_id if vso_context else None,
        )
        logger.info("[TOOL_CALL] Investigation ID: %s", investigation.id if investigation else "None")

        try:
            if tool_name == OperatorToolName.RUN_COMMANDS:
                args = OperatorCommandArgs.model_validate(tool_args)
                logger.info("[RUN_OPERATOR_COMMANDS] Executing command: %s", args.command)
                result = await self.execute_command(
                    args, vso_context, investigation, request_settings=request_settings
                )
                logger.info("[RUN_OPERATOR_COMMANDS] Result: %s", result)
                return result

            if tool_name == OperatorToolName.FILE_CREATE:
                args = FileEditPayload.model_validate({**tool_args, "operation": FileOperation.WRITE, "create_if_missing": True})
                logger.info("[FILE_CREATE] File path: %s", args.file_path)
                result = await self._execute_file_edit(
                    args, investigation, vso_context
                )
                logger.info("[FILE_CREATE] Result: %s", result)
                return result

            if tool_name == OperatorToolName.FILE_WRITE:
                args = FileEditPayload.model_validate({**tool_args, "operation": FileOperation.WRITE})
                logger.info("[FILE_WRITE] File path: %s", args.file_path)
                result = await self._execute_file_edit(
                    args, investigation, vso_context
                )
                logger.info("[FILE_WRITE] Result: %s", result)
                return result

            if tool_name == OperatorToolName.FILE_READ:
                args = FileEditPayload.model_validate({**tool_args, "operation": FileOperation.READ})
                logger.info("[FILE_READ] File path: %s", args.file_path)
                result = await self._execute_file_edit(
                    args, investigation, vso_context
                )
                logger.info("[FILE_READ] Result: %s", result)
                return result

            if tool_name == OperatorToolName.FILE_UPDATE:
                args = FileEditPayload.model_validate({**tool_args, "operation": FileOperation.REPLACE})
                logger.info("[FILE_UPDATE] File path: %s", args.file_path)
                result = await self._execute_file_edit(
                    args, investigation, vso_context
                )
                logger.info("[FILE_UPDATE] Result: %s", result)
                return result

            if tool_name == OperatorToolName.FETCH_FILE_HISTORY:
                args = FetchFileHistoryArgs.model_validate(tool_args)
                logger.info("[FETCH_FILE_HISTORY] File path: %s", args.file_path)
                result = await self._execute_fetch_file_history(
                    args, investigation, vso_context
                )
                logger.info("[FETCH_FILE_HISTORY] Result: %s", result)
                return result

            if tool_name == OperatorToolName.FETCH_FILE_DIFF:
                args = FetchFileDiffArgs.model_validate(tool_args)
                logger.info("[FETCH_FILE_DIFF] File path: %s", args.file_path)
                result = await self._execute_fetch_file_diff(
                    args, investigation, vso_context
                )
                logger.info("[FETCH_FILE_DIFF] Result: %s", result)
                return result

            if tool_name == OperatorToolName.G8E_SEARCH_WEB:
                if self.web_search_provider is None:
                    raise ConfigurationError("g8e_web_search called but WebSearchProvider is not configured")
                args = SearchWebArgs.model_validate(tool_args)
                logger.info("[G8E_WEB_SEARCH] Query: %s", args.query)
                result: ToolResult = await self.web_search_provider.search(query=args.query, num=args.num)
                logger.info("[G8E_WEB_SEARCH] Result: %s", result)
                return result

            if tool_name == OperatorToolName.CHECK_PORT:
                if vso_context is None:
                    raise ValidationError("vso_context is required for CHECK_PORT", field="vso_context", constraint="required")
                args = CheckPortArgs.model_validate(tool_args)
                logger.info("[CHECK_PORT_STATUS] Host: %s Port: %s Protocol: %s",
                    args.host, args.port, args.protocol)
                result = await self._execute_port_check(
                    args, investigation, vso_context
                )
                logger.info("[CHECK_PORT_STATUS] Result: %s", result)
                return result

            if tool_name == OperatorToolName.LIST_FILES:
                args = FsListArgs.model_validate(tool_args)
                logger.info("[LIST_DIRECTORY] Path: %s max_depth: %s max_entries: %s",
                    args.path, args.max_depth, args.max_entries)
                result = await self._execute_fs_list(
                    args, investigation, vso_context
                )
                logger.info("[LIST_DIRECTORY] entries=%d truncated=%s",
                    result.total_count, result.truncated)
                return result

            if tool_name == OperatorToolName.GRANT_INTENT:
                args = GrantIntentArgs.model_validate(tool_args)
                logger.info("[REQUEST_INTENT] Intent: %s", args.intent_name)
                result = await self._execute_intent_permission_request(
                    args=args, investigation=investigation, vso_context=vso_context
                )
                logger.info("[REQUEST_INTENT] approved=%s", result.approved)
                return result

            if tool_name == OperatorToolName.REVOKE_INTENT:
                args = RevokeIntentArgs.model_validate(tool_args)
                logger.info("[REVOKE_INTENT] Intent: %s", args.intent_name)
                result = await self._execute_intent_revocation(
                    args=args, investigation=investigation, vso_context=vso_context
                )
                logger.info("[REVOKE_INTENT] success=%s", result.success)
                return result

            error_msg = (
                f"Unknown function: {tool_name}. "
                f"Registered functions: {', '.join(self._tool_executors.keys())}"
            )
            logger.error("[TOOL_CALL] Unregistered function called: %s", tool_name)
            logger.error("[TOOL_CALL] Available functions: %s", list(self._tool_executors.keys()))
            return CommandExecutionResult(
                success=False,
                error=error_msg,
                error_type=CommandErrorType.UNKNOWN_TOOL
            )

        except (ValidationError, ExternalServiceError):
            raise
        except Exception as e:
            logger.error("[TOOL_CALL] Execution failed for %s: %s", tool_name, e)
            raise ExternalServiceError(f"Tool execution failed for {tool_name}: {e}", service_name=tool_name, component=ComponentName.G8EE) from e

    async def execute_command(
        self,
        args: OperatorCommandArgs,
        vso_context: VSOHttpContext,
        investigation: EnrichedInvestigationContext,
        request_settings: G8eeUserSettings | None = None,
    ) -> CommandExecutionResult:
        """Delegate command execution to the OperatorCommandService."""
        return await self.operator_command_service.execute_command(
            args=args,
            vso_context=vso_context,
            investigation=investigation,
            request_settings=request_settings,
        )

    async def _execute_file_edit(
        self,
        args: FileEditPayload,
        investigation: EnrichedInvestigationContext,
        vso_context: VSOHttpContext,
    ) -> FileEditResult:
        """Delegate file edit operation to the OperatorCommandService."""
        return await self.operator_command_service.execute_file_edit(
            args=args,
            vso_context=vso_context,
            investigation=investigation,
            execution_id=args.execution_id if hasattr(args, "execution_id") and args.execution_id else (vso_context.execution_id if vso_context else "unknown"),
        )

    async def _execute_port_check(
        self,
        args: CheckPortArgs,
        investigation: EnrichedInvestigationContext,
        vso_context: VSOHttpContext,
    ) -> PortCheckToolResult:
        """Delegate port check operation to the G8eoOperatorService."""
        return await self.operator_command_service.execute_port_check(
            args=args,
            investigation=investigation,
            vso_context=vso_context,
        )

    async def _execute_fs_list(
        self,
        args: FsListArgs,
        investigation: EnrichedInvestigationContext,
        vso_context: VSOHttpContext,
    ) -> FsListToolResult:
        """Delegate file system list operation to the G8eoOperatorService."""
        return await self.operator_command_service.execute_fs_list(
            args=args,
            investigation=investigation,
            vso_context=vso_context,
        )

    async def _execute_fs_read(
        self,
        args: FsReadArgs,
        investigation: EnrichedInvestigationContext,
        vso_context: VSOHttpContext,
    ) -> FsReadToolResult:
        """Delegate file system read operation to the G8eoOperatorService."""
        return await self.operator_command_service.execute_fs_read(
            args=args,
            investigation=investigation,
            vso_context=vso_context,
        )

    async def _execute_intent_permission_request(
        self,
        *,
        args: GrantIntentArgs,
        investigation: EnrichedInvestigationContext,
        vso_context: VSOHttpContext,
    ) -> IntentPermissionResult:
        """Delegate intent permission request to the G8eoOperatorService."""
        if vso_context is None:
            raise ValidationError("vso_context is required for execute_intent_permission_request", component=ComponentName.G8EE)
        return await self.operator_command_service.execute_intent_permission_request(
            args=args,
            vso_context=vso_context,
            investigation=investigation,
        )

    async def _execute_intent_revocation(
        self,
        *,
        args: RevokeIntentArgs,
        investigation: EnrichedInvestigationContext,
        vso_context: VSOHttpContext,
    ) -> IntentPermissionResult:
        """Delegate intent permission revocation to the G8eoOperatorService."""
        if vso_context is None:
            raise ValidationError("vso_context is required for execute_intent_revocation", component=ComponentName.G8EE)
        return await self.operator_command_service.execute_intent_revocation(
            args=args,
            vso_context=vso_context,
            investigation=investigation,
        )

    async def _execute_fetch_file_history(
        self,
        args: FetchFileHistoryArgs,
        investigation: EnrichedInvestigationContext,
        vso_context: VSOHttpContext,
    ) -> FetchFileHistoryToolResult:
        """Delegate file history fetch operation to the G8eoOperatorService."""
        return await self.operator_command_service.execute_fetch_file_history(
            args=args,
            investigation=investigation,
            vso_context=vso_context,
        )

    async def _execute_fetch_file_diff(
        self,
        args: FetchFileDiffArgs,
        investigation: EnrichedInvestigationContext,
        vso_context: VSOHttpContext,
    ) -> FetchFileDiffToolResult:
        """Delegate file diff fetch operation to the G8eoOperatorService."""
        return await self.operator_command_service.execute_fetch_file_diff(
            args=args,
            investigation=investigation,
            vso_context=vso_context,
        )
