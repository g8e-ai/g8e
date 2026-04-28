"""Regression tests for operator actions context formatting.

Covers the use_enum_values footgun: G8eBaseModel stores enum fields as
primitive strings, not enum instances. Code that calls .value on these
fields crashes with AttributeError. These tests ensure the formatting
methods handle the already-extracted string values correctly.
"""

import pytest
from datetime import datetime, timezone
from unittest.mock import AsyncMock, MagicMock

from app.constants import ComponentName, EventType, ExecutionStatus
from app.models.investigations import (
    ConversationMessageMetadata,
    InvestigationHistoryEntry,
    InvestigationModel,
)
from app.services.investigation.investigation_data_service import InvestigationDataService
from app.services.investigation.investigation_service import InvestigationService


def _make_history_entry(
    event_type: EventType,
    status: ExecutionStatus | None = None,
    approved: bool | None = None,
    summary: str = "test action",
    prev_hash: str = "0" * 64,
    entry_hash: str = "0" * 64,
) -> InvestigationHistoryEntry:
    metadata = ConversationMessageMetadata(status=status, approved=approved)
    return InvestigationHistoryEntry(
        attempt_number=1,
        timestamp=datetime(2026, 1, 1, tzinfo=timezone.utc),
        event_type=event_type,
        actor=ComponentName.G8EE,
        summary=summary,
        details=metadata,
        prev_hash=prev_hash,
        entry_hash=entry_hash,
    )


class TestInvestigationDataServiceOperatorActions:
    """Tests for InvestigationDataService.get_operator_actions_for_ai_context."""

    @pytest.mark.asyncio
    async def test_status_field_is_str_not_enum(self):
        """Regression: status is already a str due to use_enum_values=True."""
        entry = _make_history_entry(
            event_type=EventType.OPERATOR_COMMAND_EXECUTION,
            status=ExecutionStatus.COMPLETED,
        )
        assert isinstance(entry.details.status, str)
        assert entry.details.status == "completed"

    @pytest.mark.asyncio
    async def test_formats_command_execution_status(self):
        investigation = MagicMock(spec=InvestigationModel)
        investigation.history_trail = [
            _make_history_entry(
                event_type=EventType.OPERATOR_COMMAND_EXECUTION,
                status=ExecutionStatus.COMPLETED,
                summary="ran ls -la",
            ),
        ]

        mock_cache = AsyncMock()
        service = InvestigationDataService(cache=mock_cache)
        service.get_investigation = AsyncMock(return_value=investigation)

        result = await service.get_operator_actions_for_ai_context("inv-1")
        assert "completed" in result
        assert "ran ls -la" in result

    @pytest.mark.asyncio
    async def test_formats_file_edit_approved(self):
        investigation = MagicMock(spec=InvestigationModel)
        investigation.history_trail = [
            _make_history_entry(
                event_type=EventType.OPERATOR_FILE_EDIT_COMPLETED,
                approved=True,
                summary="edited /etc/hosts",
            ),
        ]

        mock_cache = AsyncMock()
        service = InvestigationDataService(cache=mock_cache)
        service.get_investigation = AsyncMock(return_value=investigation)

        result = await service.get_operator_actions_for_ai_context("inv-1")
        assert "success" in result

    @pytest.mark.asyncio
    async def test_status_none_shows_unknown(self):
        investigation = MagicMock(spec=InvestigationModel)
        investigation.history_trail = [
            _make_history_entry(
                event_type=EventType.OPERATOR_COMMAND_EXECUTION,
                status=None,
                summary="pending command",
            ),
        ]

        mock_cache = AsyncMock()
        service = InvestigationDataService(cache=mock_cache)
        service.get_investigation = AsyncMock(return_value=investigation)

        result = await service.get_operator_actions_for_ai_context("inv-1")
        assert "unknown" in result


class TestInvestigationServiceOperatorActions:
    """Tests for InvestigationService.get_operator_actions_for_ai_context."""

    @pytest.mark.asyncio
    async def test_formats_command_execution_status(self):
        investigation = MagicMock(spec=InvestigationModel)
        investigation.history_trail = [
            _make_history_entry(
                event_type=EventType.OPERATOR_COMMAND_EXECUTION,
                status=ExecutionStatus.FAILED,
                summary="ran bad-cmd",
            ),
        ]

        mock_data_service = AsyncMock()
        mock_data_service.get_operator_actions_for_ai_context.return_value = "failed ran bad-cmd"
        mock_operator_service = AsyncMock()
        mock_memory_service = AsyncMock()

        service = InvestigationService(
            investigation_data_service=mock_data_service,
            operator_data_service=mock_operator_service,
            memory_data_service=mock_memory_service,
        )

        result = await service.get_operator_actions_for_ai_context("inv-1")
        assert "failed" in result
        assert "ran bad-cmd" in result
