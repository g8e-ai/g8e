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


import asyncio
import logging

from app.constants import (
    INVESTIGATION_LOOKUP_MAX_RETRIES,
    INVESTIGATION_LOOKUP_RETRY_DELAYS_MS,
    ComponentName,
    EventType,
    FileOperation,
    OperatorStatus,
    OperatorType,
)
from app.errors import ExternalServiceError, ResourceNotFoundError
from app.models.agent import OperatorContext
from app.models.http_context import G8eHttpContext
from app.models.investigations import (
    EnrichedInvestigationContext,
    InvestigationModel,
    InvestigationUpdateRequest,
    InvestigationQueryRequest,
    InvestigationHistoryEntry,
    ConversationMessageMetadata,
    ConversationHistoryMessage,
    InvestigationCreateRequest,
)
from app.models.operators import CommandInternalResult, FileEditResult, OperatorDocument
from app.services.protocols import InvestigationDataServiceProtocol, OperatorDataServiceProtocol, MemoryDataServiceProtocol

logger = logging.getLogger(__name__)


class InvestigationService:

    def __init__(
        self,
        investigation_data_service: InvestigationDataServiceProtocol,
        operator_data_service: OperatorDataServiceProtocol,
        memory_data_service: MemoryDataServiceProtocol,
    ):
        self.investigation_data_service = investigation_data_service
        self.operator_data_service = operator_data_service
        self.memory_data_service = memory_data_service

    async def create_investigation(
        self,
        request: InvestigationCreateRequest,
    ) -> InvestigationModel:
        """Domain orchestration for creating a new investigation."""
        return await self.investigation_data_service.create_investigation(request)

    async def get_investigation_context(
        self,
        case_id: str | None = None,
        investigation_id: str | None = None,
        user_id: str | None = None
    ) -> EnrichedInvestigationContext:
        # SECURITY: user_id should ALWAYS be provided for user-facing queries to ensure
        # proper tenant isolation. Queries without user_id will log a security warning.
        try:
            if investigation_id:
                for attempt in range(INVESTIGATION_LOOKUP_MAX_RETRIES):
                    investigation = await self.investigation_data_service.get_investigation(
                        investigation_id
                    )
                    if investigation:
                        if attempt > 0:
                            logger.info(
                                "Investigation found on retry attempt %s/%s",
                                attempt + 1,
                                INVESTIGATION_LOOKUP_MAX_RETRIES,
                                extra={
                                    "investigation_id": investigation_id,
                                    "retry_attempt": attempt + 1,
                                    "total_wait_ms": sum(INVESTIGATION_LOOKUP_RETRY_DELAYS_MS[:attempt])
                                }
                            )
                        enriched = self._to_enriched(investigation)
                        return await self._attach_memory_context(enriched)

                    if attempt < INVESTIGATION_LOOKUP_MAX_RETRIES - 1:
                        wait_ms = INVESTIGATION_LOOKUP_RETRY_DELAYS_MS[attempt]
                        logger.info(
                            "Investigation not found, retrying in %sms (attempt %s/%s)",
                            wait_ms,
                            attempt + 1,
                            INVESTIGATION_LOOKUP_MAX_RETRIES,
                            extra={
                                "investigation_id": investigation_id,
                                "retry_attempt": attempt + 1,
                                "wait_ms": wait_ms
                            }
                        )
                        await asyncio.sleep(wait_ms / 1000)

                logger.error(
                    "Investigation lookup failed after %s retries: investigation_id=%s case_id=%s",
                    INVESTIGATION_LOOKUP_MAX_RETRIES,
                    investigation_id,
                    case_id
                )
                raise ResourceNotFoundError(
                    message=f"Investigation {investigation_id} not found after {INVESTIGATION_LOOKUP_MAX_RETRIES} retries",
                    resource_type="investigation",
                    resource_id=investigation_id,
                )

            if case_id:
                if not user_id:
                    logger.warning(
                        "get_investigation_context called without user_id for case %s",
                        case_id,
                        extra={"case_id": case_id, "security": "unscoped_query"}
                    )
                investigations = await self.investigation_data_service.get_case_investigations(
                    case_id=case_id,
                    user_id=user_id
                )
                if investigations:
                    # Sort investigations to find the one with the latest created_at timestamp
                    latest = max(investigations, key=lambda inv: inv.created_at)
                    enriched = self._to_enriched(latest)
                    return await self._attach_memory_context(enriched)
                logger.error("No investigations found for case_id=%s", case_id)
                raise ResourceNotFoundError(
                    message=f"No investigations found for case {case_id}",
                    resource_type="investigation",
                    resource_id=case_id,
                )

            logger.error("Investigation context requires an investigation_id or case_id")
            raise ResourceNotFoundError(
                message="Investigation context could not be resolved",
                resource_type="investigation",
                resource_id="unknown",
            )

        except ResourceNotFoundError:
            raise
        except Exception as e:
            logger.error("Error getting investigation context: %s", e)
            raise ExternalServiceError(
                f"Failed to get investigation context: {e}",
                service_name="investigation_service",
                component=ComponentName.G8EE
            ) from e

    @staticmethod
    def _to_enriched(investigation: InvestigationModel) -> EnrichedInvestigationContext:
        if isinstance(investigation, EnrichedInvestigationContext):
            return investigation
        return EnrichedInvestigationContext.model_validate(investigation, from_attributes=True)

    async def get_enriched_investigation_context(
        self,
        investigation: EnrichedInvestigationContext,
        user_id: str,
        g8e_context: G8eHttpContext,
    ) -> EnrichedInvestigationContext:
        logger.info(
            "Enriching investigation context with Operator details",
            extra={
                "investigation_id": investigation.id,
                "user_id": user_id,
                "bound_operator_count": len(g8e_context.bound_operators) if g8e_context else 0,
                "case_id": investigation.case_id
            }
        )

        # 1. Populate operator documents from bound operators in context
        operator_docs = []
        bound_in_context = g8e_context.bound_operators if g8e_context else []
        for bound_op in bound_in_context:
            if bound_op.status != OperatorStatus.BOUND:
                continue
            try:
                op = await self.operator_data_service.get_operator(bound_op.operator_id)
                if op:
                    operator_docs.append(op)
                else:
                    logger.warning(
                        "Bound Operator not found in cache",
                        extra={"operator_id": bound_op.operator_id}
                    )
            except Exception as e:
                logger.error(
                    "Failed to fetch Operator document from cache: %s",
                    e,
                    extra={
                        "operator_id": bound_op.operator_id,
                        "error": str(e)
                    }
                )

        has_bound = len(operator_docs) > 0
        investigation.operator_documents = operator_docs

        if has_bound:
            logger.info(
                "Operators BOUND - context enriched for AI",
                extra={
                    "operator_count": len(operator_docs),
                    "operator_ids": [op.operator_id for op in operator_docs],
                }
            )
        else:
            logger.info(
                "No operators BOUND - no Operator context for AI",
                extra={
                    "bound_operator_count_in_context": len(bound_in_context),
                }
            )

        logger.info(
            "Investigation context enrichment complete",
            extra={
                "investigation_id": investigation.id,
                "operators_bound": has_bound,
                "total_bound_operators": len(operator_docs),
            }
        )

        return investigation

    async def _attach_memory_context(self, context: EnrichedInvestigationContext) -> EnrichedInvestigationContext:
        investigation_id = context.id
        if not investigation_id:
            return context

        memory = await self.memory_data_service.get_memory(investigation_id)
        if not memory:
            logger.info(
                "No memory found for investigation",
                extra={"investigation_id": investigation_id}
            )
            return context

        context.memory = memory

        logger.info(
            "Memory context attached to investigation",
            extra={
                "investigation_id": investigation_id,
                "has_summary": bool(memory.investigation_summary),
                "has_communication_prefs": bool(memory.communication_preferences),
                "has_technical_background": bool(memory.technical_background),
                "has_response_style": bool(memory.response_style),
                "updated_at": memory.updated_at
            }
        )

        return context

    async def query_investigations(
        self, request: InvestigationQueryRequest
    ) -> list[InvestigationModel]:
        """Domain orchestration for querying investigations."""
        return await self.investigation_data_service.query_investigations(request)

    async def get_investigation(self, investigation_id: str) -> InvestigationModel | None:
        """Domain orchestration for fetching an investigation by ID."""
        return await self.investigation_data_service.get_investigation(investigation_id)

    async def get_case_investigations(
        self,
        case_id: str,
        user_id: str | None,
    ) -> list[InvestigationModel]:
        """Domain orchestration for fetching investigations by case ID."""
        return await self.investigation_data_service.get_case_investigations(case_id=case_id, user_id=user_id)

    async def get_chat_messages(self, investigation_id: str) -> list[ConversationHistoryMessage]:
        """Domain orchestration for fetching full chat history."""
        return await self.investigation_data_service.get_chat_messages(investigation_id)

    async def update_investigation_raw(
        self,
        investigation_id: str,
        updates: dict[str, object],
        merge: bool = True,
    ) -> None:
        """Domain orchestration for low-level investigation updates."""
        await self.investigation_data_service.update_investigation_raw(
            investigation_id=investigation_id,
            updates=updates,
            merge=merge
        )

    async def delete_investigation(self, investigation_id: str) -> None:
        """Domain orchestration for deleting an investigation."""
        await self.investigation_data_service.delete_investigation(investigation_id)

    async def update_investigation(
        self,
        investigation_id: str,
        request: InvestigationUpdateRequest,
        actor: ComponentName = ComponentName.G8EE,
    ) -> InvestigationModel:
        """Domain logic for updating an investigation with change tracking and history."""
        investigation = await self.investigation_data_service.get_investigation(investigation_id)
        if not investigation:
            raise ResourceNotFoundError(
                message=f"Investigation {investigation_id} not found",
                resource_type="investigation",
                resource_id=investigation_id,
            )

        changes: dict[str, object] = {}

        if request.status is not None and request.status != investigation.status:
            changes["status"] = {"old": investigation.status, "new": request.status}
            investigation.update_status(request.status, actor, f"Status updated to {request.status}")

        if request.priority is not None and request.priority != investigation.priority:
            changes["priority"] = {"old": investigation.priority, "new": request.priority}
            investigation.priority = request.priority

        if request.case_title is not None and request.case_title != investigation.case_title:
            changes["case_title"] = {"old": investigation.case_title, "new": request.case_title}
            investigation.case_title = request.case_title

        if request.customer_context is not None and request.customer_context != investigation.customer_context:
            changes["customer_context"] = True
            investigation.customer_context = request.customer_context

        if request.technical_context is not None and request.technical_context != investigation.technical_context:
            changes["technical_context"] = True
            investigation.technical_context = request.technical_context

        if request.sentinel_mode is not None and request.sentinel_mode != investigation.sentinel_mode:
            changes["sentinel_mode"] = {"old": investigation.sentinel_mode, "new": request.sentinel_mode}
            investigation.sentinel_mode = request.sentinel_mode

        if not changes:
            return investigation

        investigation.add_history_entry(
            event_type=EventType.INVESTIGATION_UPDATED,
            actor=actor,
            summary="Investigation updated",
            details=ConversationMessageMetadata(
                changes={k: v for k, v in changes.items() if v is not True}
            ),
        )

        patch: dict[str, object] = {}
        if "status" in changes:
            patch["status"] = investigation.status
        if "priority" in changes:
            patch["priority"] = investigation.priority
        if "case_title" in changes:
            patch["case_title"] = investigation.case_title
        if "customer_context" in changes:
            patch["customer_context"] = investigation.customer_context
        if "technical_context" in changes:
            patch["technical_context"] = investigation.technical_context
        if "sentinel_mode" in changes:
            patch["sentinel_mode"] = investigation.sentinel_mode
        patch["history_trail"] = [e.flatten_for_db() for e in investigation.history_trail]

        await self.investigation_data_service.update_investigation_raw(investigation_id, patch)
        logger.info(f"Updated investigation {investigation_id}")
        return investigation

    async def add_history_entry(
        self,
        investigation_id: str,
        event_type: EventType,
        actor: ComponentName,
        summary: str,
        details: ConversationMessageMetadata,
    ) -> InvestigationModel:
        """Record an event in the investigation history trail."""
        return await self.investigation_data_service.add_history_entry(
            investigation_id=investigation_id,
            event_type=event_type,
            actor=actor,
            summary=summary,
            details=details,
        )

    async def add_command_execution_result(
        self,
        investigation_id: str,
        execution_id: str,
        command: str,
        result: CommandInternalResult,
        operator_id: str,
        operator_session_id: str,
        actor: ComponentName = ComponentName.G8EO,
    ) -> InvestigationModel:
        """Domain orchestration for recording a command execution result."""
        return await self.investigation_data_service.add_command_execution_result(
            investigation_id=investigation_id,
            execution_id=execution_id,
            command=command,
            result=result,
            operator_id=operator_id or "unknown",
            operator_session_id=operator_session_id,
        )

    async def add_file_operation_result(
        self,
        investigation_id: str,
        execution_id: str,
        operator_id: str,
        event_type: EventType,
        file_path: str,
        result: FileEditResult,
        operation: FileOperation,
        operator_session_id: str,
    ) -> InvestigationModel:
        """Domain orchestration for recording a file operation result."""
        return await self.investigation_data_service.add_file_operation_result(
            investigation_id=investigation_id,
            execution_id=execution_id,
            operator_id=operator_id,
            event_type=event_type,
            file_path=file_path,
            result=result,
            operation=operation,
        )

    async def add_chat_message(
        self,
        investigation_id: str | None,
        sender: str,
        content: str,
        metadata: ConversationMessageMetadata,
    ) -> bool:
        """Domain-layer wrapper for adding a chat message to history."""
        return await self.investigation_data_service.add_chat_message(
            investigation_id=investigation_id,
            sender=sender,
            content=content,
            metadata=metadata,
        )

    async def add_approval_record(
        self,
        investigation_id: str,
        event_type: EventType,
        metadata: ConversationMessageMetadata,
        actor: ComponentName = ComponentName.G8EE,
    ) -> InvestigationModel:
        """Domain orchestration for recording an approval lifecycle event."""
        return await self.investigation_data_service.add_approval_record(
            investigation_id=investigation_id,
            event_type=event_type,
            metadata=metadata,
            actor=actor,
        )

    async def get_command_execution_history(self, investigation_id: str) -> list[InvestigationHistoryEntry]:
        """Retrieve all command execution entries from investigation history."""
        return await self.investigation_data_service.get_command_execution_history(investigation_id)

    async def get_operator_actions_for_ai_context(self, investigation_id: str) -> str:
        """Domain logic for formatting operator action history for LLM consumption."""
        return await self.investigation_data_service.get_operator_actions_for_ai_context(investigation_id)



def _extract_single_operator_context(op: OperatorDocument) -> OperatorContext:
    """Extract typed system context from a single OperatorDocument."""
    sys_info = op.system_info
    hb = op.latest_heartbeat_snapshot

    # Use system_info for static details, heartbeat for dynamic metrics
    os_details = sys_info.os_details if sys_info else None
    user_details = sys_info.user_details if sys_info else None
    disk_details = sys_info.disk_details if sys_info else None
    memory_details = sys_info.memory_details if sys_info else None
    environment = sys_info.environment if sys_info else None

    # Override with latest heartbeat if available
    if hb:
        if hb.os_details: os_details = hb.os_details
        if hb.user_details: user_details = hb.user_details
        if hb.disk_details: disk_details = hb.disk_details
        if hb.memory_details: memory_details = hb.memory_details
        if hb.environment: environment = hb.environment

    return OperatorContext(
        operator_id=op.operator_id,
        operator_session_id=op.operator_session_id,
        os=sys_info.os if sys_info else None,
        hostname=sys_info.hostname if sys_info else None,
        architecture=sys_info.architecture if sys_info else None,
        cpu_count=sys_info.cpu_count if sys_info else None,
        memory_mb=sys_info.memory_mb if sys_info else None,
        public_ip=sys_info.public_ip if sys_info else None,
        operator_type=op.operator_type,
        cloud_subtype=op.cloud_subtype,
        is_cloud_operator=op.operator_type == OperatorType.CLOUD,
        granted_intents=op.granted_intents,
        distro=os_details.distro if os_details else None,
        kernel=os_details.kernel if os_details else None,
        os_version=os_details.version if os_details else None,
        username=user_details.username if user_details else None,
        home_directory=user_details.home if user_details else None,
        shell=user_details.shell if user_details else None,
        working_directory=environment.pwd if environment else None,
        timezone=environment.timezone if environment else None,
        is_container=bool(environment.is_container) if environment else False,
        container_runtime=environment.container_runtime if environment else None,
        init_system=environment.init_system if environment else None,
        disk_percent=disk_details.percent if disk_details else None,
        disk_total_gb=disk_details.total_gb if disk_details else None,
        disk_free_gb=disk_details.free_gb if disk_details else None,
        memory_percent=memory_details.percent if memory_details else None,
        memory_total_mb=memory_details.total_mb if memory_details else None,
        memory_available_mb=memory_details.available_mb if memory_details else None,
    )


def extract_system_context(
    investigation: EnrichedInvestigationContext | None,
) -> OperatorContext | None:
    """
    Extract system context for the PRIMARY (first) bound operator.

    For multi-operator support use extract_all_operators_context().
    """
    if not investigation or not investigation.operator_documents:
        return None

    operator_doc = investigation.operator_documents[0]
    context = _extract_single_operator_context(operator_doc)
    logger.info(
        "[CONTEXT] Primary operator context: operator_id=%s hostname=%s os=%s "
        "arch=%s memory_mb=%s cpu_count=%s public_ip=%s operator_type=%s "
        "is_cloud=%s granted_intents=%s",
        context.operator_id, context.hostname, context.os,
        context.architecture, context.memory_mb, context.cpu_count,
        context.public_ip, context.operator_type,
        context.is_cloud_operator, context.granted_intents or [],
    )
    return context


def extract_all_operators_context(
    investigation: EnrichedInvestigationContext | None,
) -> list[OperatorContext] | None:
    """
    Extract system context for ALL bound operators.

    Returns a list of OperatorContext models so the AI can understand every
    available system and choose appropriately.
    """
    if not investigation or not investigation.operator_documents:
        return None

    contexts: list[OperatorContext] = []
    for operator_doc in investigation.operator_documents:
        context = _extract_single_operator_context(operator_doc)
        logger.info(
            "[CONTEXT] Operator[%d] context: operator_id=%s hostname=%s os=%s "
            "arch=%s operator_type=%s is_cloud=%s granted_intents=%s",
            len(contexts), context.operator_id, context.hostname,
            context.os, context.architecture,
            context.operator_type, context.is_cloud_operator,
            context.granted_intents or [],
        )
        contexts.append(context)

    if contexts:
        logger.info(
            "[CONTEXT] All operators extracted: %d operator(s) ids=%s",
            len(contexts), [c.operator_id for c in contexts],
        )
    else:
        logger.info("[CONTEXT] extract_all_operators_context: no valid operator contexts found")

    return contexts if contexts else None


def extract_operator_context_by_target(
    investigation: EnrichedInvestigationContext | None,
    target_operator: str | None,
) -> OperatorContext | None:
    """
    Extract system context for a specific target operator.
    
    Args:
        investigation: Enriched investigation context
        target_operator: Target operator identifier (operator_id, hostname, or index)
        
    Returns:
        OperatorContext for the targeted operator, or None if not found
    """
    if not investigation or not investigation.operator_documents:
        return None
    
    # If no target specified, use first operator (backward compatibility)
    if not target_operator:
        return _extract_single_operator_context(investigation.operator_documents[0])
    
    # Try to find operator by operator_id
    for operator_doc in investigation.operator_documents:
        if operator_doc.operator_id == target_operator:
            return _extract_single_operator_context(operator_doc)
    
    # Try to find operator by hostname
    for operator_doc in investigation.operator_documents:
        if operator_doc.hostname == target_operator:
            return _extract_single_operator_context(operator_doc)
    
    # Try to parse as index (e.g., "0", "1", "2")
    try:
        index = int(target_operator)
        if 0 <= index < len(investigation.operator_documents):
            return _extract_single_operator_context(investigation.operator_documents[index])
    except ValueError:
        pass
    
    # If not found, fall back to first operator
    logger.warning(
        "[CONTEXT] Target operator '%s' not found, falling back to first operator",
        target_operator,
    )
    return _extract_single_operator_context(investigation.operator_documents[0])
