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
from typing import Any, Awaitable, TypeVar
from app.services.operator.command_service import OperatorCommandService

import app.llm.llm_types as types
from app.constants.status import (
    CommandErrorType,
    ComponentName,
    FileOperation,
    OperatorToolName,
)
from app.services.ai.tool_registry import (
    AI_UNIVERSAL_TOOLS,
    OPERATOR_TOOLS,
    TOOL_SPECS,
)
from app.constants.prompts import AgentMode, PromptFile
from app.constants.settings import FORBIDDEN_COMMAND_PATTERNS
from app.models.http_context import G8eHttpContext
from app.models.investigations import EnrichedInvestigationContext
from app.models.settings import G8eePlatformSettings, G8eeUserSettings
from app.llm.prompts import load_prompt
from app.errors import ExternalServiceError, ValidationError, ConfigurationError
from app.llm.llm_types import schema_from_model
from app.models.agent import ExecutorCommandArgs, SageOperatorRequest
from app.models.model_configs import get_model_config
from app.models.tool_results import (
    CommandConstraintsResult,
    CommandExecutionResult,
    FetchFileHistoryToolResult,
    FetchFileDiffToolResult,
    FileEditResult,
    FsListToolResult,
    FsReadToolResult,
    IntentPermissionResult,
    InvestigationContextResult,
    PortCheckToolResult,
    ToolResult,
)
from app.models.tool_args import (
    CheckPortArgs,
    FileCreateArgs,
    FileReadArgs,
    FileUpdateArgs,
    FileWriteArgs,
    FetchFileHistoryArgs,
    FetchFileDiffArgs,
    FsListArgs,
    FsReadArgs,
    GrantIntentArgs,
    QueryInvestigationContextArgs,
    RevokeIntentArgs,
    SearchWebArgs,
)
from app.models.command_request_payloads import (
    CheckPortRequestPayload,
    CommandRequestPayload,
    FileEditRequestPayload,
    FetchFileHistoryRequestPayload,
    FetchFileDiffRequestPayload,
    FsListRequestPayload,
)

from app.utils.blacklist_validator import CommandBlacklistValidator
from app.utils.whitelist_validator import CommandWhitelistValidator
from app.utils.safety import map_os_string_to_platform
from .grounding.web_search_provider import WebSearchProvider
from ..data.reputation_data_service import ReputationDataService
from ..investigation.investigation_service import InvestigationService

T = TypeVar("T")

logger = logging.getLogger(__name__)


class AIToolService:
    """Service for AI tool registration and execution on bound operators."""

    def __init__(
        self,
        operator_command_service: "OperatorCommandService",
        investigation_service: InvestigationService,
        web_search_provider: WebSearchProvider | None,
        platform_settings: G8eePlatformSettings | None = None,
        user_settings: G8eeUserSettings | None = None,
        whitelist_validator: CommandWhitelistValidator | None = None,
        blacklist_validator: CommandBlacklistValidator | None = None,
        reputation_data_service: ReputationDataService | None = None,
    ):
        self.operator_command_service = operator_command_service
        self.investigation_service = investigation_service
        self._web_search_provider: WebSearchProvider | None = web_search_provider
        self._platform_settings = platform_settings
        self._user_settings = user_settings
        self._reputation_data_service = reputation_data_service

        from app.utils.validators import get_blacklist_validator, get_whitelist_validator
        self._whitelist_validator = whitelist_validator if whitelist_validator is not None else get_whitelist_validator()
        self._blacklist_validator = blacklist_validator if blacklist_validator is not None else get_blacklist_validator()

        logger.info("AIToolService initialized")

        self._tool_context: ContextVar[G8eHttpContext | None] = ContextVar(
            "g8ee_tool_context",
            default=None
        )

        self._tool_declarations: dict[str, types.ToolDeclaration] = {}
        self._tool_executors: dict[str, Callable[..., ToolResult]] = {}
        self._tool_handlers: dict[str, Callable[..., Awaitable[ToolResult]]] = {}

        for spec in TOOL_SPECS:
            if spec.requires_web_search and self.web_search_provider is None:
                logger.info(
                    "[TOOLS] %s disabled (VERTEX_SEARCH_ENABLED not set or credentials missing)",
                    spec.name.value,
                )
                continue
            builder: Callable[[], tuple[types.ToolDeclaration, Callable[..., ToolResult]]] = getattr(self, spec.builder_attr)
            declaration, executor = builder()
            self._tool_declarations[spec.name] = declaration
            self._tool_executors[spec.name] = executor
            self._tool_handlers[spec.name] = getattr(self, spec.handler_attr)
            if spec.requires_web_search:
                logger.info("[TOOLS] %s enabled (Vertex AI Search configured)", spec.name.value)

        self._assert_tool_registry_invariants()

    def _assert_tool_registry_invariants(self) -> None:
        """Enforce alignment between OPERATOR_TOOLS, AI_UNIVERSAL_TOOLS, and registered declarations.

        Every OPERATOR_TOOLS member must have a ToolDeclaration (otherwise it is a dead
        approval-gate entry). Every registered declaration must be classified as either
        operator-gated or universal; unclassified declarations indicate a future bug where
        the auth guard in execute_tool_call silently skips a bound-operator check.
        """
        declared = {
            name.value if isinstance(name, OperatorToolName) else str(name)
            for name in self._tool_declarations.keys()
        }
        missing_operator = OPERATOR_TOOLS - declared
        if missing_operator:
            raise ConfigurationError(
                f"OPERATOR_TOOLS contains tools with no ToolDeclaration: {sorted(missing_operator)}. "
                f"Either register a declaration in AIToolService.__init__ or remove the entry from OPERATOR_TOOLS."
            )
        classified = OPERATOR_TOOLS | AI_UNIVERSAL_TOOLS
        # G8E_SEARCH_WEB is conditionally registered; exclude it from the universal-required check.
        required_universal = AI_UNIVERSAL_TOOLS - {OperatorToolName.G8E_SEARCH_WEB.value}
        missing_universal = required_universal - declared
        if missing_universal:
            raise ConfigurationError(
                f"AI_UNIVERSAL_TOOLS contains tools with no ToolDeclaration: {sorted(missing_universal)}."
            )
        unclassified = declared - classified
        if unclassified:
            raise ConfigurationError(
                f"Registered tool declarations are not classified as OPERATOR_TOOLS or AI_UNIVERSAL_TOOLS: "
                f"{sorted(unclassified)}. Add each tool to the appropriate frozenset in app/constants/status.py."
            )

    def start_invocation_context(
        self,
        g8e_context: G8eHttpContext,
    ) -> ContextVarToken[G8eHttpContext | None]:
        """Set the g8e_context used by tool calls for the active request."""
        token = self._tool_context.set(g8e_context)
        bound_count = len(g8e_context.bound_operators) if g8e_context else 0
        case_id = g8e_context.case_id if g8e_context else None
        user_id = g8e_context.user_id if g8e_context else None
        logger.info(
            "[TOOL_CONTEXT] Context initialized: case_id=%s user_id=%s bound_operators=%d",
            case_id, user_id, bound_count
        )
        return token

    def reset_invocation_context(self, token: ContextVarToken[G8eHttpContext | None]) -> None:
        """Reset invocation context after request completion."""
        self._tool_context.reset(token)
        logger.info("[TOOL_CONTEXT] Context reset")

    def _convert_args_to_payload(
        self,
        args_dict: dict[str, object],
        payload_cls: type[T],
        execution_id: str,
        **extra_fields: object,
    ) -> T:
        """Convert LLM tool args to downstream Payload with execution_id injection.

        This centralizes the Args→Payload conversion pattern to ensure execution_id
        is never forgotten when adding new tools or refactoring existing ones.

        Args:
            args_dict: Raw tool arguments from the LLM tool call
            payload_cls: The Payload Pydantic model class to convert to
            execution_id: The execution_id to inject into the payload
            **extra_fields: Additional fields to merge into the payload

        Returns:
            Validated instance of the payload class with execution_id injected
        """
        return payload_cls.model_validate({**args_dict, "execution_id": execution_id, **extra_fields})

    @property
    def web_search_provider(self) -> WebSearchProvider | None:
        """The configured WebSearchProvider, or None if g8e_web_search is not enabled."""
        return self._web_search_provider

    @property
    def g8e_web_search_available(self) -> bool:
        """True when the g8e_web_search tool is registered (Google Custom Search configured)."""
        return OperatorToolName.G8E_SEARCH_WEB in self._tool_declarations

    @property
    def user_settings(self) -> G8eeUserSettings | None:
        """The configured user settings."""
        return self._user_settings

    @property
    def reputation_data_service(self) -> ReputationDataService:
        """The configured ``ReputationDataService``.

        Tribunal-path invocations require it; constructing an
        ``AIToolService`` without one and then invoking the Tribunal is
        a configuration error surfaced at the call site.
        """
        if self._reputation_data_service is None:
            raise ConfigurationError(
                "AIToolService constructed without reputation_data_service; "
                "Tribunal path requires it. Wire it in service_factory."
            )
        return self._reputation_data_service

    @property
    def auditor_hmac_key(self) -> str:
        """The auditor HMAC key from the wired platform settings.

        Tribunal-path invocations require it; absence is a configuration
        error surfaced at the call site.
        """
        key = None
        if self._platform_settings is not None:
            key = self._platform_settings.auth.auditor_hmac_key
        if not key:
            raise ConfigurationError(
                "AIToolService has no auditor_hmac_key available; the key "
                "must be seeded via g8eo SecretManager and overlaid onto "
                "AuthSettings.auditor_hmac_key."
            )
        return key

    @property
    def whitelist_validator(self) -> CommandWhitelistValidator:
        """The configured whitelist validator."""
        return self._whitelist_validator

    @property
    def blacklist_validator(self) -> CommandBlacklistValidator:
        """The configured blacklist validator."""
        return self._blacklist_validator

    def get_tools(
        self,
        agent_mode: AgentMode,
        model_to_use: str | None
    ) -> list[types.ToolGroup]:
        """Build tool declarations for the given AgentMode, driven by TOOL_SPECS.

        A tool is exposed iff ``agent_mode in spec.agent_modes`` and the tool is
        currently registered (``g8e_web_search`` is only registered when a
        ``WebSearchProvider`` was injected). Ordering follows ``TOOL_SPECS``.

        Returns an empty list if the model does not support tools.
        """
        if model_to_use:
            config = get_model_config(model_to_use)
            if not config.supports_tools:
                logger.info("[TOOLS] Model %s does not support tools, skipping tool declarations", model_to_use)
                return []

        resolved_workflow = agent_mode or AgentMode.OPERATOR_NOT_BOUND

        tools = [
            self._tool_declarations[spec.name]
            for spec in TOOL_SPECS
            if resolved_workflow in spec.agent_modes and spec.name in self._tool_declarations
        ]
        if not tools:
            return []
        return [types.ToolGroup(tools=tools)]

    def _build_run_operator_commands_tool(self) -> tuple[types.ToolDeclaration, Callable[..., ToolResult]]:
        """Register tool metadata and executor for Operator command execution."""

        def run_commands_with_operator(args: ExecutorCommandArgs) -> ToolResult:
            raise NotImplementedError("run_commands_with_operator should be called via execute_tool_call")

        declaration = types.ToolDeclaration(
            name=OperatorToolName.RUN_COMMANDS,
            description=load_prompt(PromptFile.TOOL_RUN_COMMANDS),
            parameters=schema_from_model(
                SageOperatorRequest,
                required_override=["request"],
            ),
        )

        return declaration, run_commands_with_operator

    def _build_file_create_tool(self) -> tuple[types.ToolDeclaration, Callable[..., ToolResult]]:
        """Register tool metadata and executor for file creation operations."""

        def file_create_on_operator(args: FileEditRequestPayload) -> ToolResult:
            raise NotImplementedError("file_create_on_operator should be called via execute_tool_call")

        declaration = types.ToolDeclaration(
            name=OperatorToolName.FILE_CREATE,
            description=load_prompt(PromptFile.TOOL_FILE_CREATE),
            parameters=schema_from_model(FileCreateArgs),
        )

        return declaration, file_create_on_operator

    def _build_file_write_tool(self) -> tuple[types.ToolDeclaration, Callable[..., ToolResult]]:
        """Register tool metadata and executor for file write (overwrite) operations."""

        def file_write_on_operator(args: FileEditRequestPayload) -> ToolResult:
            raise NotImplementedError("file_write_on_operator should be called via execute_tool_call")

        declaration = types.ToolDeclaration(
            name=OperatorToolName.FILE_WRITE,
            description=load_prompt(PromptFile.TOOL_FILE_WRITE),
            parameters=schema_from_model(FileWriteArgs),
        )

        return declaration, file_write_on_operator

    def _build_file_read_tool(self) -> tuple[types.ToolDeclaration, Callable[..., ToolResult]]:
        """Register tool metadata and executor for file read operations."""

        def file_read_on_operator(args: FileEditRequestPayload) -> ToolResult:
            raise NotImplementedError("file_read_on_operator should be called via execute_tool_call")

        declaration = types.ToolDeclaration(
            name=OperatorToolName.FILE_READ,
            description=load_prompt(PromptFile.TOOL_FILE_READ),
            parameters=schema_from_model(FileReadArgs),
        )

        return declaration, file_read_on_operator

    def _build_file_update_tool(self) -> tuple[types.ToolDeclaration, Callable[..., ToolResult]]:
        """Register tool metadata and executor for file update (find-and-replace) operations."""

        def file_update_on_operator(args: FileEditRequestPayload) -> ToolResult:
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

    def _build_query_investigation_context_tool(self) -> tuple[types.ToolDeclaration, Callable[..., ToolResult]]:
        """Register tool metadata and executor for investigation context queries."""

        def query_investigation_context(args: QueryInvestigationContextArgs) -> ToolResult:
            raise NotImplementedError("query_investigation_context should be called via execute_tool_call")

        declaration = types.ToolDeclaration(
            name=OperatorToolName.QUERY_INVESTIGATION_CONTEXT,
            description=load_prompt(PromptFile.TOOL_QUERY_INVESTIGATION_CONTEXT),
            parameters=schema_from_model(QueryInvestigationContextArgs),
        )

        return declaration, query_investigation_context

    def _build_get_command_constraints_tool(self) -> tuple[types.ToolDeclaration, Callable[..., ToolResult]]:
        """Register tool metadata and executor for command constraint queries."""

        def get_command_constraints() -> ToolResult:
            raise NotImplementedError("get_command_constraints should be called via execute_tool_call")

        declaration = types.ToolDeclaration(
            name=OperatorToolName.GET_COMMAND_CONSTRAINTS,
            description=load_prompt(PromptFile.TOOL_GET_COMMAND_CONSTRAINTS),
            parameters=types.Schema(type=types.Type.OBJECT, properties={}, required=None),
        )

        return declaration, get_command_constraints

    async def _handle_run_commands(
        self,
        tool_args: dict[str, object],
        investigation: EnrichedInvestigationContext,
        g8e_context: G8eHttpContext,
        request_settings: G8eeUserSettings,
        execution_id: str,
    ) -> ToolResult:
        args = ExecutorCommandArgs.model_validate(tool_args)
        logger.info("[RUN_OPERATOR_COMMANDS] Executing command: %s", args.command)
        result = await self.execute_command(
            args, g8e_context, investigation, request_settings=request_settings, execution_id=execution_id
        )
        logger.info("[RUN_OPERATOR_COMMANDS] Result: %s", result)
        return result

    async def _handle_file_create(
        self,
        tool_args: dict[str, object],
        investigation: EnrichedInvestigationContext,
        g8e_context: G8eHttpContext,
        request_settings: G8eeUserSettings,
        execution_id: str,
    ) -> ToolResult:
        args = FileEditRequestPayload.model_validate({**tool_args, "execution_id": execution_id, "operation": FileOperation.WRITE, "create_if_missing": True})
        logger.info("[FILE_CREATE] File path: %s", args.file_path)
        result = await self._execute_file_edit(
            args, investigation, g8e_context
        )
        logger.info("[FILE_CREATE] Result: %s", result)
        return result

    async def _handle_file_write(
        self,
        tool_args: dict[str, object],
        investigation: EnrichedInvestigationContext,
        g8e_context: G8eHttpContext,
        request_settings: G8eeUserSettings,
        execution_id: str,
    ) -> ToolResult:
        args = FileEditRequestPayload.model_validate({**tool_args, "execution_id": execution_id, "operation": FileOperation.WRITE})
        logger.info("[FILE_WRITE] File path: %s", args.file_path)
        result = await self._execute_file_edit(
            args, investigation, g8e_context
        )
        logger.info("[FILE_WRITE] Result: %s", result)
        return result

    async def _handle_file_read(
        self,
        tool_args: dict[str, object],
        investigation: EnrichedInvestigationContext,
        g8e_context: G8eHttpContext,
        request_settings: G8eeUserSettings,
        execution_id: str,
    ) -> ToolResult:
        args = FileEditRequestPayload.model_validate({**tool_args, "execution_id": execution_id, "operation": FileOperation.READ})
        logger.info("[FILE_READ] File path: %s", args.file_path)
        result = await self._execute_file_edit(
            args, investigation, g8e_context
        )
        logger.info("[FILE_READ] Result: %s", result)
        return result

    async def _handle_file_update(
        self,
        tool_args: dict[str, object],
        investigation: EnrichedInvestigationContext,
        g8e_context: G8eHttpContext,
        request_settings: G8eeUserSettings,
        execution_id: str,
    ) -> ToolResult:
        args = FileEditRequestPayload.model_validate({**tool_args, "execution_id": execution_id, "operation": FileOperation.REPLACE})
        logger.info("[FILE_UPDATE] File path: %s", args.file_path)
        result = await self._execute_file_edit(
            args, investigation, g8e_context
        )
        logger.info("[FILE_UPDATE] Result: %s", result)
        return result

    async def _handle_fetch_file_history(
        self,
        tool_args: dict[str, object],
        investigation: EnrichedInvestigationContext,
        g8e_context: G8eHttpContext,
        request_settings: G8eeUserSettings,
        execution_id: str,
    ) -> ToolResult:
        args = self._convert_args_to_payload(tool_args, FetchFileHistoryRequestPayload, execution_id)
        logger.info("[FETCH_FILE_HISTORY] File path: %s", args.file_path)
        result = await self._execute_fetch_file_history(
            args, investigation, g8e_context
        )
        logger.info("[FETCH_FILE_HISTORY] Result: %s", result)
        return result

    async def _handle_fetch_file_diff(
        self,
        tool_args: dict[str, object],
        investigation: EnrichedInvestigationContext,
        g8e_context: G8eHttpContext,
        request_settings: G8eeUserSettings,
        execution_id: str,
    ) -> ToolResult:
        args = self._convert_args_to_payload(tool_args, FetchFileDiffRequestPayload, execution_id)
        logger.info("[FETCH_FILE_DIFF] File path: %s", args.file_path)
        result = await self._execute_fetch_file_diff(
            args, investigation, g8e_context
        )
        logger.info("[FETCH_FILE_DIFF] Result: %s", result)
        return result

    async def _handle_search_web(
        self,
        tool_args: dict[str, object],
        investigation: EnrichedInvestigationContext,
        g8e_context: G8eHttpContext,
        request_settings: G8eeUserSettings,
        execution_id: str,
    ) -> ToolResult:
        if self.web_search_provider is None:
            raise ConfigurationError("g8e_web_search called but WebSearchProvider is not configured")
        args = SearchWebArgs.model_validate(tool_args)
        logger.info("[G8E_WEB_SEARCH] Query: %s", args.query)
        result: ToolResult = await self.web_search_provider.search(query=args.query, num=args.num)
        logger.info("[G8E_WEB_SEARCH] Result: %s", result)
        return result

    async def _handle_port_check(
        self,
        tool_args: dict[str, object],
        investigation: EnrichedInvestigationContext,
        g8e_context: G8eHttpContext,
        request_settings: G8eeUserSettings,
        execution_id: str,
    ) -> ToolResult:
        args = self._convert_args_to_payload(tool_args, CheckPortRequestPayload, execution_id)
        logger.info("[CHECK_PORT_STATUS] Host: %s Port: %s Protocol: %s",
            args.host, args.port, args.protocol)
        result = await self._execute_port_check(
            args, investigation, g8e_context
        )
        logger.info("[CHECK_PORT_STATUS] Result: %s", result)
        return result

    async def _handle_list_files(
        self,
        tool_args: dict[str, object],
        investigation: EnrichedInvestigationContext,
        g8e_context: G8eHttpContext,
        request_settings: G8eeUserSettings,
        execution_id: str,
    ) -> ToolResult:
        args = self._convert_args_to_payload(tool_args, FsListRequestPayload, execution_id)
        logger.info("[LIST_DIRECTORY] Path: %s max_depth: %s max_entries: %s",
            args.path, args.max_depth, args.max_entries)
        result = await self._execute_fs_list(
            args, investigation, g8e_context
        )
        logger.info("[LIST_DIRECTORY] entries=%d truncated=%s",
            result.total_count, result.truncated)
        return result

    async def _handle_grant_intent(
        self,
        tool_args: dict[str, object],
        investigation: EnrichedInvestigationContext,
        g8e_context: G8eHttpContext,
        request_settings: G8eeUserSettings,
        execution_id: str,
    ) -> ToolResult:
        args = GrantIntentArgs.model_validate(tool_args)
        logger.info("[REQUEST_INTENT] Intent: %s", args.intent_name)
        result = await self._execute_intent_permission_request(
            args=args, investigation=investigation, g8e_context=g8e_context
        )
        logger.info("[REQUEST_INTENT] approved=%s", result.approved)
        return result

    async def _handle_revoke_intent(
        self,
        tool_args: dict[str, object],
        investigation: EnrichedInvestigationContext,
        g8e_context: G8eHttpContext,
        request_settings: G8eeUserSettings,
        execution_id: str,
    ) -> ToolResult:
        args = RevokeIntentArgs.model_validate(tool_args)
        logger.info("[REVOKE_INTENT] Intent: %s", args.intent_name)
        result = await self._execute_intent_revocation(
            args=args, investigation=investigation, g8e_context=g8e_context
        )
        logger.info("[REVOKE_INTENT] success=%s", result.success)
        return result

    async def _get_investigation_helper(
        self,
        investigation_id: str,
        data_type: str,
    ) -> tuple[dict[str, Any] | None, InvestigationContextResult | None]:
        """Helper to fetch investigation and handle missing error response."""
        inv = await self.investigation_service.get_investigation(investigation_id)
        if inv:
            return inv.model_dump(), None
        
        return None, InvestigationContextResult(
            success=False,
            error=f"Investigation not found: {investigation_id}",
            error_type=CommandErrorType.VALIDATION_ERROR,
            data_type=data_type,
            investigation_id=investigation_id,
        )

    async def _handle_query_investigation_context(
        self,
        tool_args: dict[str, object],
        investigation: EnrichedInvestigationContext,
        g8e_context: G8eHttpContext,
        request_settings: G8eeUserSettings,
        execution_id: str,
    ) -> ToolResult:
        args = QueryInvestigationContextArgs.model_validate(tool_args)
        logger.info("[QUERY_INVESTIGATION_CONTEXT] data_type=%s limit=%s", args.data_type, args.limit)
        
        if not investigation or not investigation.id:
            logger.error("[QUERY_INVESTIGATION_CONTEXT] No investigation ID available")
            return InvestigationContextResult(
                success=False,
                error="No investigation ID available",
                error_type=CommandErrorType.VALIDATION_ERROR,
                data_type=args.data_type,
            )
        
        investigation_id = investigation.id
        data: dict[str, Any] | list[dict[str, Any]] | str | None = None
        item_count: int | None = None
        
        try:
            if args.data_type == "conversation_history":
                messages = await self.investigation_service.get_chat_messages(investigation_id)
                if args.limit:
                    messages = messages[-args.limit:] if args.limit > 0 else messages
                data = [msg.model_dump() for msg in messages]
                item_count = len(messages)
                
            elif args.data_type == "investigation_status":
                data, error_res = await self._get_investigation_helper(investigation_id, args.data_type)
                if error_res:
                    return error_res
                item_count = 1
                    
            elif args.data_type == "history_trail":
                data, error_res = await self._get_investigation_helper(investigation_id, args.data_type)
                if error_res:
                    return error_res
                item_count = 1
                    
            elif args.data_type == "operator_actions":
                data = await self.investigation_service.get_operator_actions_for_ai_context(investigation_id)
                item_count = 1
                
            else:
                return InvestigationContextResult(
                    success=False,
                    error=f"Invalid data_type: {args.data_type}. Valid values: conversation_history, investigation_status, history_trail, operator_actions",
                    error_type=CommandErrorType.VALIDATION_ERROR,
                    data_type=args.data_type,
                    investigation_id=investigation_id,
                )
                
            logger.info("[QUERY_INVESTIGATION_CONTEXT] success=True item_count=%s", item_count)
            return InvestigationContextResult(
                success=True,
                data_type=args.data_type,
                data=data,
                item_count=item_count,
                investigation_id=investigation_id,
            )
            
        except Exception as e:
            logger.error("[QUERY_INVESTIGATION_CONTEXT] Failed: %s", e, exc_info=True)
            return InvestigationContextResult(
                success=False,
                error=f"Investigation context query failed: {e}. Retry or check investigation ID.",
                error_type=CommandErrorType.EXECUTION_ERROR,
                data_type=args.data_type,
                investigation_id=investigation_id,
            )

    async def _handle_get_command_constraints(
        self,
        tool_args: dict[str, object],
        investigation: EnrichedInvestigationContext,
        g8e_context: G8eHttpContext,
        request_settings: G8eeUserSettings,
        execution_id: str,
    ) -> ToolResult:
        logger.info("[GET_COMMAND_CONSTRAINTS] Retrieving command constraints")

        cv = self._user_settings.command_validation if self._user_settings else None
        whitelisting_enabled = cv.enable_whitelisting if cv else False
        blacklisting_enabled = cv.enable_blacklisting if cv else False

        whitelisted_commands: list[dict[str, Any]] = []
        global_forbidden_patterns: list[str] = []
        global_forbidden_directories: list[str] = []
        if whitelisting_enabled:
            # Map OS string to Platform enum using centralized function
            os_name = self._user_settings.operator_context.os if self._user_settings and self._user_settings.operator_context else DEFAULT_OS_NAME
            platform = map_os_string_to_platform(os_name)

            whitelisted_commands = self._whitelist_validator.get_available_commands_with_metadata(platform)
            global_forbidden_patterns = list(self._whitelist_validator.forbidden_patterns)
            global_forbidden_directories = list(self._whitelist_validator.forbidden_directories)

        blacklisted_commands: list[dict[str, str]] = []
        blacklisted_substrings: list[dict[str, str]] = []
        blacklisted_patterns: list[dict[str, str]] = []
        if blacklisting_enabled:
            blacklisted_commands = self._blacklist_validator.get_forbidden_commands()
            blacklisted_substrings = self._blacklist_validator.get_forbidden_substrings()
            blacklisted_patterns = self._blacklist_validator.get_forbidden_patterns()

        parts: list[str] = []
        if not whitelisting_enabled and not blacklisting_enabled:
            parts.append("No command constraints are currently enforced. All commands are permitted.")
        else:
            if whitelisting_enabled:
                parts.append(
                    f"Whitelisting ENABLED: only the {len(whitelisted_commands)} listed commands are permitted. "
                    "Each command has strict 'safe_options' and 'validation' patterns that MUST be followed. "
                    "Any command or argument not explicitly allowed by these rules will be blocked by the technical (L1) validator."
                )
            if blacklisting_enabled:
                parts.append(
                    "Blacklisting ENABLED: commands matching blacklisted entries will be blocked."
                )

        result = CommandConstraintsResult(
            success=True,
            whitelisting_enabled=whitelisting_enabled,
            blacklisting_enabled=blacklisting_enabled,
            whitelisted_commands=whitelisted_commands,
            blacklisted_commands=blacklisted_commands,
            blacklisted_substrings=blacklisted_substrings,
            blacklisted_patterns=blacklisted_patterns,
            global_forbidden_patterns=global_forbidden_patterns,
            global_forbidden_directories=global_forbidden_directories,
            message=" ".join(parts),
        )
        logger.info(
            "[GET_COMMAND_CONSTRAINTS] whitelisting=%s blacklisting=%s whitelist_count=%d blacklist_count=%d",
            whitelisting_enabled, blacklisting_enabled,
            len(whitelisted_commands), len(blacklisted_commands),
        )
        return result

    async def execute_tool_call(
        self,
        tool_name: str,
        tool_args: dict[str, object],
        investigation: EnrichedInvestigationContext,
        g8e_context: G8eHttpContext,
        request_settings: G8eeUserSettings,
        execution_id: str | None = None,
    ) -> ToolResult:
        """Validate, dispatch, and execute a single AI tool call by name.

        ``execution_id`` is the caller-generated canonical id for this tool invocation.
        It is threaded to per-call operator services (port_check, fs_list, fs_read,
        file_edit) so that UI lifecycle events (STARTED/COMPLETED/FAILED) published
        by each service share a single id with the pub/sub execution registry entry
        and the native per-tool lifecycle event emitted by agent_sse (for universal
        tools, an ``LLM_TOOL_G8E_<TOOL>_REQUESTED`` chunk carrying the same id).
        """
        if tool_name == OperatorToolName.RUN_COMMANDS:
            raw_command = tool_args.get("command", "")
            command_lower = raw_command.lower() if isinstance(raw_command, str) else ""
            for pattern in FORBIDDEN_COMMAND_PATTERNS:
                if pattern in command_lower:
                    error_msg = (
                        f"SECURITY VIOLATION: Command contains forbidden pattern '{pattern}'. "
                        f"Privilege escalation commands (sudo, su, pkexec, doas, etc.) are strictly prohibited. "
                        f"Find an alternative approach that does not require elevated privileges."
                    )
                    logger.error("[SECURITY] Blocked forbidden command pattern '%s' in: %s", pattern, raw_command)
                    return CommandExecutionResult(
                        success=False,
                        error=error_msg,
                        error_type=CommandErrorType.SECURITY_VIOLATION,
                        blocked_pattern=pattern
                    )

        if tool_name in OPERATOR_TOOLS:
            if not g8e_context or not g8e_context.has_bound_operator():
                error_msg = (
                    "No operators are currently BOUND to this session. "
                    "Operator commands can only be executed when an operator is explicitly bound in the g8e UI."
                )
                logger.error("[TOOL_CALL] Execution blocked: No bound operators in G8eHttpContext")
                return CommandExecutionResult(
                    success=False,
                    error=error_msg,
                    error_type=CommandErrorType.NO_OPERATORS_AVAILABLE
                )

        if tool_name == OperatorToolName.G8E_SEARCH_WEB and self.web_search_provider is None:
            raise ExternalServiceError("g8e_web_search called but WebSearchProvider is not configured")

        logger.info("[TOOL_CALL] Starting execution: %s", tool_name)
        logger.info("[TOOL_CALL] Args: %s", tool_args)
        logger.info("[TOOL_CALL] Context - case_id: %s, user_id: %s",
            g8e_context.case_id if g8e_context else None,
            g8e_context.user_id if g8e_context else None,
        )
        logger.info("[TOOL_CALL] Investigation ID: %s", investigation.id if investigation else "None")

        handler = self._tool_handlers.get(tool_name)
        if not handler:
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

        try:
            return await handler(tool_args, investigation, g8e_context, request_settings, execution_id)
        except (ValidationError, ExternalServiceError):
            raise
        except Exception as e:
            logger.error("[TOOL_CALL] Execution failed for %s: %s", tool_name, e)
            raise ExternalServiceError(f"Tool execution failed for {tool_name}: {e}", service_name=tool_name, component=ComponentName.G8EE) from e

    async def execute_command(
        self,
        args: ExecutorCommandArgs,
        g8e_context: G8eHttpContext,
        investigation: EnrichedInvestigationContext,
        request_settings: G8eeUserSettings,
        execution_id: str | None = None,
    ) -> CommandExecutionResult:
        """Delegate command execution to the OperatorCommandService."""
        return await self.operator_command_service.execute_command(
            args=args,
            g8e_context=g8e_context,
            investigation=investigation,
            request_settings=request_settings,
            execution_id=execution_id,
        )

    async def _execute_file_edit(
        self,
        args: FileEditRequestPayload,
        investigation: EnrichedInvestigationContext,
        g8e_context: G8eHttpContext,
    ) -> FileEditResult:
        """Delegate file edit operation to the OperatorCommandService.

        ``execution_id`` is extracted from args.execution_id.
        """
        return await self.operator_command_service.execute_file_edit(
            args=args,
            g8e_context=g8e_context,
            investigation=investigation,
        )

    async def _execute_port_check(
        self,
        args: CheckPortRequestPayload,
        investigation: EnrichedInvestigationContext,
        g8e_context: G8eHttpContext,
    ) -> PortCheckToolResult:
        """Delegate port check operation to the G8eoOperatorService."""
        return await self.operator_command_service.execute_port_check(
            args=args,
            investigation=investigation,
            g8e_context=g8e_context,
        )

    async def _execute_fs_list(
        self,
        args: FsListRequestPayload,
        investigation: EnrichedInvestigationContext,
        g8e_context: G8eHttpContext,
    ) -> FsListToolResult:
        """Delegate file system list operation to the G8eoOperatorService."""
        return await self.operator_command_service.execute_fs_list(
            args=args,
            investigation=investigation,
            g8e_context=g8e_context,
        )

    async def _execute_file_read(
        self,
        args: FsReadPayload,
        investigation: EnrichedInvestigationContext,
        g8e_context: G8eHttpContext,
    ) -> FsReadToolResult:
        """Delegate file system read operation to the G8eoOperatorService."""
        return await self.operator_command_service.execute_file_read(
            args=args,
            investigation=investigation,
            g8e_context=g8e_context,
        )

    async def _execute_intent_permission_request(
        self,
        *,
        args: GrantIntentArgs,
        investigation: EnrichedInvestigationContext,
        g8e_context: G8eHttpContext,
    ) -> IntentPermissionResult:
        """Delegate intent permission request to the G8eoOperatorService."""
        return await self.operator_command_service.execute_intent_permission_request(
            args=args,
            g8e_context=g8e_context,
            investigation=investigation,
        )

    async def _execute_intent_revocation(
        self,
        *,
        args: RevokeIntentArgs,
        investigation: EnrichedInvestigationContext,
        g8e_context: G8eHttpContext,
    ) -> IntentPermissionResult:
        """Delegate intent permission revocation to the G8eoOperatorService."""
        return await self.operator_command_service.execute_intent_revocation(
            args=args,
            g8e_context=g8e_context,
            investigation=investigation,
        )

    async def _execute_fetch_file_history(
        self,
        args: FetchFileHistoryRequestPayload,
        investigation: EnrichedInvestigationContext,
        g8e_context: G8eHttpContext,
    ) -> FetchFileHistoryToolResult:
        """Delegate file history fetch operation to the G8eoOperatorService."""
        return await self.operator_command_service.execute_fetch_file_history(
            args=args,
            investigation=investigation,
            g8e_context=g8e_context,
        )

    async def _execute_fetch_file_diff(
        self,
        args: FetchFileDiffRequestPayload,
        investigation: EnrichedInvestigationContext,
        g8e_context: G8eHttpContext,
    ) -> FetchFileDiffToolResult:
        """Delegate file diff fetch operation to the G8eoOperatorService."""
        return await self.operator_command_service.execute_fetch_file_diff(
            args=args,
            investigation=investigation,
            g8e_context=g8e_context,
        )
