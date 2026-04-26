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

"""AI tool orchestration surface.

``AIToolService`` is intentionally thin: it owns the per-request context
var, the dependency wiring shared by every tool, and the pre-dispatch
guards (forbidden-pattern scan, bound-operator auth gate, web-search
configuration check). Each tool's declaration build and execution body
lives in its own module under :mod:`app.services.ai.tools`, referenced
directly by callable on the corresponding :class:`ToolSpec`.

Adding a new tool means writing one module under ``tools/`` plus one
``ToolSpec`` entry — no method ever needs to be added here.
"""

from __future__ import annotations

import logging
from collections.abc import Awaitable, Callable
from contextvars import ContextVar, Token as ContextVarToken
from typing import TYPE_CHECKING

import app.llm.llm_types as types
from app.constants.status import (
    CommandErrorType,
    ComponentName,
    OperatorToolName,
)
from app.constants.prompts import AgentMode
from app.constants.settings import FORBIDDEN_COMMAND_PATTERNS
from app.errors import ConfigurationError, ExternalServiceError, ValidationError
from app.models.http_context import G8eHttpContext
from app.models.investigations import EnrichedInvestigationContext
from app.models.model_configs import get_model_config
from app.models.settings import G8eePlatformSettings, G8eeUserSettings
from app.models.tool_results import CommandExecutionResult, ToolResult
from app.services.ai.tool_registry import (
    AI_UNIVERSAL_TOOLS,
    OPERATOR_TOOLS,
    TOOL_SPECS,
)
from app.services.investigation.investigation_service import InvestigationService
from app.services.operator.command_service import OperatorCommandService
from app.utils.blacklist_validator import CommandBlacklistValidator
from app.utils.whitelist_validator import CommandWhitelistValidator

from .grounding.web_search_provider import WebSearchProvider
from .ssh_inventory_service import SshInventoryService
from ..data.reputation_data_service import ReputationDataService

if TYPE_CHECKING:
    from .reputation_service import ReputationService
    from .chat_task_manager import BackgroundTaskManager
    from ..data.stake_resolution_data_service import StakeResolutionDataService

logger = logging.getLogger(__name__)


class AIToolService:
    """Service for AI tool registration and execution on bound operators."""

    def __init__(
        self,
        operator_command_service: "OperatorCommandService",
        investigation_service: InvestigationService,
        reputation_data_service: ReputationDataService,
        reputation_service: ReputationService,
        stake_resolution_data_service: StakeResolutionDataService,
        chat_task_manager: BackgroundTaskManager,
        ssh_inventory_service: SshInventoryService,
        web_search_provider: WebSearchProvider | None,
        platform_settings: G8eePlatformSettings | None = None,
        user_settings: G8eeUserSettings | None = None,
        whitelist_validator: CommandWhitelistValidator | None = None,
        blacklist_validator: CommandBlacklistValidator | None = None,
    ):
        self.operator_command_service = operator_command_service
        self.investigation_service = investigation_service
        self._web_search_provider: WebSearchProvider | None = web_search_provider
        self._platform_settings = platform_settings
        self._user_settings = user_settings
        self._reputation_data_service = reputation_data_service
        self._reputation_service = reputation_service
        self._stake_resolution_data_service = stake_resolution_data_service
        self._chat_task_manager = chat_task_manager
        self._ssh_inventory_service = ssh_inventory_service

        from app.utils.validators import get_blacklist_validator, get_whitelist_validator
        self._whitelist_validator = (
            whitelist_validator if whitelist_validator is not None else get_whitelist_validator()
        )
        self._blacklist_validator = (
            blacklist_validator if blacklist_validator is not None else get_blacklist_validator()
        )

        logger.info("AIToolService initialized")

        self._tool_context: ContextVar[G8eHttpContext | None] = ContextVar(
            "g8ee_tool_context",
            default=None,
        )

        self._tool_declarations: dict[str, types.ToolDeclaration] = {}
        self._tool_handlers: dict[str, Callable[..., Awaitable[ToolResult]]] = {}

        for spec in TOOL_SPECS:
            if spec.requires_web_search and self.web_search_provider is None:
                logger.info(
                    "[TOOLS] %s disabled (VERTEX_SEARCH_ENABLED not set or credentials missing)",
                    spec.name.value,
                )
                continue
            self._tool_declarations[spec.name] = spec.builder()
            self._tool_handlers[spec.name] = spec.handler
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
            case_id, user_id, bound_count,
        )
        return token

    def reset_invocation_context(self, token: ContextVarToken[G8eHttpContext | None]) -> None:
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

    @property
    def user_settings(self) -> G8eeUserSettings | None:
        return self._user_settings

    @property
    def reputation_data_service(self) -> ReputationDataService:
        """The configured ``ReputationDataService`` (required)."""
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
    def reputation_service(self) -> ReputationService:
        """The configured ``ReputationService`` (required)."""
        return self._reputation_service

    @property
    def stake_resolution_data_service(self) -> StakeResolutionDataService:
        """The configured ``StakeResolutionDataService`` (required)."""
        return self._stake_resolution_data_service

    @property
    def chat_task_manager(self) -> BackgroundTaskManager:
        """The configured ``BackgroundTaskManager`` (required)."""
        return self._chat_task_manager

    @property
    def ssh_inventory_service(self) -> SshInventoryService:
        """The configured ``SshInventoryService`` (required)."""
        return self._ssh_inventory_service

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
        model_to_use: str | None,
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
                logger.info(
                    "[TOOLS] Model %s does not support tools, skipping tool declarations",
                    model_to_use,
                )
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
                    logger.error(
                        "[SECURITY] Blocked forbidden command pattern '%s' in: %s",
                        pattern, raw_command,
                    )
                    return CommandExecutionResult(
                        success=False,
                        error=error_msg,
                        error_type=CommandErrorType.SECURITY_VIOLATION,
                        blocked_pattern=pattern,
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
                    error_type=CommandErrorType.NO_OPERATORS_AVAILABLE,
                )

        if tool_name == OperatorToolName.G8E_SEARCH_WEB and self.web_search_provider is None:
            raise ExternalServiceError(
                "g8e_web_search called but WebSearchProvider is not configured"
            )

        logger.info("[TOOL_CALL] Starting execution: %s", tool_name)
        logger.info("[TOOL_CALL] Args: %s", tool_args)
        logger.info(
            "[TOOL_CALL] Context - case_id: %s, user_id: %s",
            g8e_context.case_id if g8e_context else None,
            g8e_context.user_id if g8e_context else None,
        )
        logger.info(
            "[TOOL_CALL] Investigation ID: %s",
            investigation.id if investigation else "None",
        )

        handler = self._tool_handlers.get(tool_name)
        if not handler:
            error_msg = (
                f"Unknown function: {tool_name}. "
                f"Registered functions: {', '.join(self._tool_handlers.keys())}"
            )
            logger.error("[TOOL_CALL] Unregistered function called: %s", tool_name)
            logger.error("[TOOL_CALL] Available functions: %s", list(self._tool_handlers.keys()))
            return CommandExecutionResult(
                success=False,
                error=error_msg,
                error_type=CommandErrorType.UNKNOWN_TOOL,
            )

        try:
            return await handler(self, tool_args, investigation, g8e_context, request_settings, execution_id)
        except (ValidationError, ExternalServiceError):
            raise
        except Exception as e:
            logger.error("[TOOL_CALL] Execution failed for %s: %s", tool_name, e)
            raise ExternalServiceError(
                f"Tool execution failed for {tool_name}: {e}",
                service_name=tool_name,
                component=ComponentName.G8EE,
            ) from e
