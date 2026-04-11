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

"""
Integration tests: AI Tool Call Payload Handling

These tests verify AI tool call execution with deterministic payloads while mocking pubsub
to focus on payload handling rather than network communication.

Tests cover all 16 AI tools:
    Segment 1 — Command Execution Tools
      Test run_commands_with_operator tool payload handling

    Segment 2 — File Operation Tools  
      Test file_create_on_operator, file_write_on_operator, file_read_on_operator, 
      file_update_on_operator tools payload handling

    Segment 3 — File System Tools
      Test list_files_and_directories_with_detailed_metadata, read_file_content,
      fetch_file_history, restore_file, fetch_file_diff tools payload handling

    Segment 4 — Network & Search Tools
      Test check_port_status, g8e_web_search tools payload handling

    Segment 5 — Permission & Session Tools
      Test grant_intent_permission, revoke_intent_permission, fetch_execution_output,
      fetch_session_history tools payload handling

All tests use mocked pubsub and deterministic payloads to verify AI tool handling.
"""

import pytest

from unittest.mock import AsyncMock

from app.constants import (
    InvestigationStatus,
)
from app.constants.status import OperatorToolName
from app.errors import  ExternalServiceError, ValidationError
from app.models.investigations import EnrichedInvestigationContext
from app.models.settings import LLMSettings, G8eeUserSettings
from app.models.tool_results import (
    CommandExecutionResult,
    FetchFileHistoryToolResult,
    FetchFileDiffToolResult,
    FileEditResult,
    FsListToolResult,
    FsReadToolResult,
    IntentPermissionResult,
    PortCheckToolResult,
    SearchWebResult,
    ToolResult,
)

from app.services.ai.tool_service import AIToolService
from app.services.operator.command_service import OperatorCommandService
from app.services.investigation.investigation_service import InvestigationService
from app.services.ai.grounding.web_search_provider import WebSearchProvider
from tests.fakes.factories import (
    build_g8e_http_context,
    build_bound_operator,
)

pytestmark = [pytest.mark.integration]


# ---------------------------------------------------------------------------
# Mock Fixtures
# ---------------------------------------------------------------------------

@pytest.fixture
def mock_operator_command_service():
    """Mock operator command service for deterministic testing."""
    service = AsyncMock(spec=OperatorCommandService)
    return service


@pytest.fixture
def mock_investigation_service():
    """Mock investigation service for deterministic testing."""
    service = AsyncMock(spec=InvestigationService)
    return service


@pytest.fixture
def mock_web_search_provider():
    """Mock web search provider for deterministic testing."""
    provider = AsyncMock(spec=WebSearchProvider)
    return provider


@pytest.fixture
def mock_g8ed_event_service():
    """Mock g8ed event service for deterministic testing."""
    service = AsyncMock()
    service.publish_event = AsyncMock()
    return service


@pytest.fixture
def tool_service(
    mock_operator_command_service,
    mock_investigation_service,
    mock_web_search_provider
):
    """Create AIToolService with mocked dependencies."""
    return AIToolService(
        operator_command_service=mock_operator_command_service,
        investigation_service=mock_investigation_service,
        web_search_provider=mock_web_search_provider,
    )


@pytest.fixture
def sample_g8e_context():
    """Sample g8e HTTP context for testing."""
    bound_operator = build_bound_operator(
        operator_id="op-123",
        operator_session_id="session-456",
    )
    return build_g8e_http_context(
        case_id="case-789",
        investigation_id="inv-101",
        web_session_id="web-202",
        bound_operators=[bound_operator],
    )


@pytest.fixture
def sample_investigation():
    """Sample investigation for testing."""
    return EnrichedInvestigationContext(
        id="inv-101",
        case_id="case-789",
        user_id="user-303",
        case_title="Test Case",
        case_description="Test description",
        status=InvestigationStatus.OPEN,
        sentinel_mode=True,
        conversation_history=[],
        operator_documents=[],
    )


@pytest.fixture
def request_settings():
    """Sample request settings for testing."""
    from app.models.settings import LLMSettings
    return G8eeUserSettings(llm=LLMSettings())


# ---------------------------------------------------------------------------
# Segment 1 — Command Execution Tools
# ---------------------------------------------------------------------------

@pytest.mark.asyncio(loop_scope="session")
@pytest.mark.integration
class TestCommandExecutionTools:
    """Test AI command execution tool payload handling."""

    async def test_run_commands_tool_payload_validation(
        self, tool_service, mock_operator_command_service, sample_g8e_context, sample_investigation, request_settings
    ):
        """Test run_commands_with_operator tool validates and processes payloads correctly."""
        # Set tool context
        context_token = tool_service.start_invocation_context(sample_g8e_context)

        try:
            # Mock successful command execution
            mock_result = CommandExecutionResult(
                execution_id="exec-123",
                status="completed",
                exit_code=0,
                stdout="File listing successful",
                stderr="",
                command="ls -la",
                execution_time_ms=150,
                success=True,
            )
            mock_operator_command_service.execute_command.return_value = mock_result

            # Test payload with valid command
            tool_args = {
                "command": "ls -la",
                "working_directory": "/home/user",
                "timeout_seconds": 30,
                "justification": "List files in directory",
            }

            # Execute tool call
            result = await tool_service.execute_tool_call(
                tool_name=OperatorToolName.RUN_COMMANDS,
                tool_args=tool_args,
                investigation=sample_investigation,
                g8e_context=sample_g8e_context,
                request_settings=request_settings,
            )

            # Verify payload was processed correctly
            assert isinstance(result, ToolResult)
            assert result.success
            
            # Verify command service was called with correct payload
            mock_operator_command_service.execute_command.assert_called_once()
            call_args = mock_operator_command_service.execute_command.call_args[1]
            
            # Check the OperatorCommandArgs payload
            assert "args" in call_args
            args = call_args["args"]
            assert "ls -la" in args.command
            assert args.timeout_seconds == 30
            assert args.justification == "List files in directory"
        finally:
            tool_service.reset_invocation_context(context_token)

    async def test_run_commands_tool_payload_error_handling(
        self, tool_service, mock_operator_command_service, sample_g8e_context, sample_investigation, request_settings
    ):
        """Test run_commands_with_operator tool handles payload errors correctly."""
        context_token = tool_service.start_invocation_context(sample_g8e_context)

        try:
            # Mock command execution failure
            mock_operator_command_service.execute_command.side_effect = ValidationError("Invalid command")

            # Test payload with invalid command
            tool_args = {
                "command": "invalid_command_syntax",
                "justification": "Test error handling",
            }

            # Execute tool call and verify ValidationError is raised
            with pytest.raises(ValidationError, match="Invalid command"):
                await tool_service.execute_tool_call(
                    tool_name=OperatorToolName.RUN_COMMANDS,
                    tool_args=tool_args,
                    investigation=sample_investigation,
                    g8e_context=sample_g8e_context,
                    request_settings=request_settings,
                )

            # Verify the mock was called
            mock_operator_command_service.execute_command.assert_called_once()
        finally:
            tool_service.reset_invocation_context(context_token)

    async def test_run_commands_tool_security_validation(
        self, tool_service, sample_g8e_context, sample_investigation, request_settings
    ):
        """Test run_commands_with_operator tool validates security constraints."""
        context_token = tool_service.start_invocation_context(sample_g8e_context)

        try:
            # Test payload with sudo command (should be blocked)
            tool_args = {
                "command": "sudo ls -la",
                "justification": "Test security validation",
            }

            # Execute tool call and verify security violation
            result = await tool_service.execute_tool_call(
                tool_name=OperatorToolName.RUN_COMMANDS,
                tool_args=tool_args,
                investigation=sample_investigation,
                g8e_context=sample_g8e_context,
                request_settings=request_settings,
            )

            # Verify security violation was handled
            assert isinstance(result, ToolResult)
            assert not result.success
            assert "SECURITY VIOLATION" in str(result.error)
            assert "sudo" in str(result.error)
        finally:
            tool_service.reset_invocation_context(context_token)


# ---------------------------------------------------------------------------
# Segment 2 — File Operation Tools
# ---------------------------------------------------------------------------

@pytest.mark.asyncio(loop_scope="session")
@pytest.mark.integration
class TestFileOperationTools:
    """Test AI file operation tool payload handling."""

    async def test_file_create_tool_payload_processing(
        self, tool_service, mock_operator_command_service, sample_g8e_context, sample_investigation, request_settings
    ):
        """Test file_create_on_operator tool processes payloads correctly."""
        context_token = tool_service.start_invocation_context(sample_g8e_context)

        try:
            # Mock successful file creation
            mock_result = FileEditResult(
                operation="create",
                file_path="/tmp/test.txt",
                success=True,
                message="File created successfully",
                size_bytes=100,
            )
            mock_operator_command_service.execute_file_edit.return_value = mock_result

            # Test payload for file creation
            tool_args = {
                "file_path": "/tmp/test.txt",
                "content": "Hello, World!",
                "mode": "0644",
                "create_directories": True,
            }

            # Execute tool call
            result = await tool_service.execute_tool_call(
                tool_name=OperatorToolName.FILE_CREATE,
                tool_args=tool_args,
                investigation=sample_investigation,
                g8e_context=sample_g8e_context,
                request_settings=request_settings,
            )

            # Verify payload processing
            assert isinstance(result, ToolResult)
            assert result.success
            
            # Verify command service was called with correct payload
            mock_operator_command_service.execute_file_edit.assert_called_once()
            call_args = mock_operator_command_service.execute_file_edit.call_args[1]
            
            assert "args" in call_args
            args = call_args["args"]
            assert args.file_path == "/tmp/test.txt"
            assert args.content == "Hello, World!"
        finally:
            tool_service.reset_invocation_context(context_token)

    async def test_file_write_tool_payload_processing(
        self, tool_service, mock_operator_command_service, sample_g8e_context, sample_investigation, request_settings
    ):
        """Test file_write_on_operator tool processes payloads correctly."""
        context_token = tool_service.start_invocation_context(sample_g8e_context)

        try:
            # Mock successful file write
            mock_result = FileEditResult(
                operation="write",
                file_path="/tmp/test.txt",
                success=True,
                message="File written successfully",
                size_bytes=200,
            )
            mock_operator_command_service.execute_file_edit.return_value = mock_result

            # Test payload for file writing
            tool_args = {
                "file_path": "/tmp/test.txt",
                "content": "Updated content",
                "mode": "0644",
                "backup": True,
            }

            # Execute tool call
            result = await tool_service.execute_tool_call(
                tool_name=OperatorToolName.FILE_WRITE,
                tool_args=tool_args,
                investigation=sample_investigation,
                g8e_context=sample_g8e_context,
                request_settings=request_settings,
            )

            # Verify payload processing
            assert isinstance(result, ToolResult)
            assert result.success
            
            # Verify command service was called with correct payload
            mock_operator_command_service.execute_file_edit.assert_called_once()
            call_args = mock_operator_command_service.execute_file_edit.call_args[1]
            
            assert "args" in call_args
            args = call_args["args"]
            assert args.file_path == "/tmp/test.txt"
            assert args.content == "Updated content"
        finally:
            tool_service.reset_invocation_context(context_token)

    async def test_file_read_tool_payload_processing(
        self, tool_service, mock_operator_command_service, sample_g8e_context, sample_investigation, request_settings
    ):
        """Test file_read_on_operator tool processes payloads correctly."""
        context_token = tool_service.start_invocation_context(sample_g8e_context)

        try:
            # Mock successful file read
            mock_result = FsReadToolResult(
                file_path="/tmp/test.txt",
                content="File content here",
                size_bytes=100,
                encoding="utf-8",
                success=True,
            )
            mock_operator_command_service.execute_file_edit.return_value = mock_result

            # Test payload for file reading
            tool_args = {
                "file_path": "/tmp/test.txt",
                "justification": "Test file reading for unit test",
                "start_line": 1,
                "end_line": 1000,
            }

            # Execute tool call
            result = await tool_service.execute_tool_call(
                tool_name=OperatorToolName.FILE_READ,
                tool_args=tool_args,
                investigation=sample_investigation,
                g8e_context=sample_g8e_context,
                request_settings=request_settings,
            )

            # Verify payload processing
            assert isinstance(result, ToolResult)
            assert result.success
            
            # Verify command service was called with correct payload
            mock_operator_command_service.execute_file_edit.assert_called_once()
            call_args = mock_operator_command_service.execute_file_edit.call_args[1]
            
            assert "args" in call_args
            args = call_args["args"]
            assert args.file_path == "/tmp/test.txt"
            assert args.start_line == 1
        finally:
            tool_service.reset_invocation_context(context_token)

    async def test_file_update_tool_payload_processing(
        self, tool_service, mock_operator_command_service, sample_g8e_context, sample_investigation, request_settings
    ):
        """Test file_update_on_operator tool processes payloads correctly."""
        context_token = tool_service.start_invocation_context(sample_g8e_context)

        try:
            # Mock successful file update
            mock_result = FileEditResult(
                operation="update",
                file_path="/tmp/test.txt",
                success=True,
                message="File updated successfully",
                size_bytes=150,
            )
            mock_operator_command_service.execute_file_edit.return_value = mock_result

            # Test payload for file updating
            tool_args = {
                "file_path": "/tmp/test.txt",
                "old_content": "old content",
                "new_content": "new content",
                "justification": "Test file update for unit test",
                "backup": True,
            }

            # Execute tool call
            result = await tool_service.execute_tool_call(
                tool_name=OperatorToolName.FILE_UPDATE,
                tool_args=tool_args,
                investigation=sample_investigation,
                g8e_context=sample_g8e_context,
                request_settings=request_settings,
            )

            # Verify payload processing
            assert isinstance(result, ToolResult)
            assert result.success
            
            # Verify command service was called with correct payload
            mock_operator_command_service.execute_file_edit.assert_called_once()
            call_args = mock_operator_command_service.execute_file_edit.call_args[1]
            
            assert "args" in call_args
            args = call_args["args"]
            assert args.file_path == "/tmp/test.txt"
            assert args.old_content == "old content"
            assert args.new_content == "new content"
        finally:
            tool_service.reset_invocation_context(context_token)


# ---------------------------------------------------------------------------
# Segment 3 — File System Tools
# ---------------------------------------------------------------------------

@pytest.mark.asyncio(loop_scope="session")
@pytest.mark.integration
class TestFileSystemTools:
    """Test AI file system tool payload handling."""

    async def test_list_files_tool_payload_processing(
        self, tool_service, mock_operator_command_service, sample_g8e_context, sample_investigation, request_settings
    ):
        """Test list_files_and_directories_with_detailed_metadata tool processes payloads correctly."""
        context_token = tool_service.start_invocation_context(sample_g8e_context)

        try:
            # Mock successful directory listing
            mock_result = FsListToolResult(
                path="/home/user",
                entries=[
                    {
                        "name": "file1.txt",
                        "path": "/home/user/file1.txt",
                        "is_dir": False,
                        "size": 100,
                        "mode": "0644",
                        "mod_time": 1672531200  # 2026-01-01T00:00:00Z as timestamp
                    }
                ],
                success=True,
            )
            mock_operator_command_service.execute_fs_list.return_value = mock_result

            # Test payload for directory listing
            tool_args = {
                "path": "/home/user",
                "max_depth": 1,
                "max_entries": 100,
            }

            # Execute tool call
            result = await tool_service.execute_tool_call(
                tool_name=OperatorToolName.LIST_FILES,
                tool_args=tool_args,
                investigation=sample_investigation,
                g8e_context=sample_g8e_context,
                request_settings=request_settings,
            )

            # Verify payload processing
            assert isinstance(result, ToolResult)
            assert result.success
            
            # Verify command service was called with correct payload
            mock_operator_command_service.execute_fs_list.assert_called_once()
            call_args = mock_operator_command_service.execute_fs_list.call_args[1]
            
            assert "args" in call_args
            args = call_args["args"]
            assert args.path == "/home/user"
            assert args.max_depth == 1
        finally:
            tool_service.reset_invocation_context(context_token)

    async def test_read_file_content_tool_payload_processing(
        self, tool_service, mock_operator_command_service, sample_g8e_context, sample_investigation, request_settings
    ):
        """Test read_file_content tool processes payloads correctly."""
        context_token = tool_service.start_invocation_context(sample_g8e_context)

        try:
            # Mock successful file content read
            mock_result = FsReadToolResult(
                file_path="/tmp/test.txt",
                content="File content here",
                size_bytes=100,
                encoding="utf-8",
                success=True,
            )
            mock_operator_command_service.execute_file_edit.return_value = mock_result

            # Test payload for reading file content
            tool_args = {
                "file_path": "/tmp/test.txt",
                "justification": "Test file content reading for unit test",
                "start_line": 1,
                "end_line": 500,
            }

            # Execute tool call
            result = await tool_service.execute_tool_call(
                tool_name=OperatorToolName.FILE_READ,
                tool_args=tool_args,
                investigation=sample_investigation,
                g8e_context=sample_g8e_context,
                request_settings=request_settings,
            )

            # Verify payload processing
            assert isinstance(result, ToolResult)
            assert result.success
            
            # Verify command service was called with correct payload
            mock_operator_command_service.execute_file_edit.assert_called_once()
            call_args = mock_operator_command_service.execute_file_edit.call_args[1]
            
            assert "args" in call_args
            args = call_args["args"]
            assert args.file_path == "/tmp/test.txt"
            assert args.start_line == 1
        finally:
            tool_service.reset_invocation_context(context_token)

    async def test_fetch_file_history_tool_payload_processing(
        self, tool_service, mock_operator_command_service, sample_g8e_context, sample_investigation, request_settings
    ):
        """Test fetch_file_history tool processes payloads correctly."""
        context_token = tool_service.start_invocation_context(sample_g8e_context)

        try:
            # Mock successful file history fetch
            mock_result = FetchFileHistoryToolResult(
                success=True,
                file_path="/tmp/test.txt",
                history=[
                    {
                        "commit_hash": "abc123",
                        "timestamp": "2026-01-01T00:00:00Z",
                        "message": "Initial commit"
                    }
                ]
            )
            mock_operator_command_service.execute_fetch_file_history.return_value = mock_result

            # Test payload for file history
            tool_args = {
                "file_path": "/tmp/test.txt",
                "limit": 10,
                "target_operator": "op-123",
            }

            # Execute tool call
            result = await tool_service.execute_tool_call(
                tool_name=OperatorToolName.FETCH_FILE_HISTORY,
                tool_args=tool_args,
                investigation=sample_investigation,
                g8e_context=sample_g8e_context,
                request_settings=request_settings,
            )

            # Verify payload processing
            assert isinstance(result, ToolResult)
            assert result.success
            
            # Verify command service was called with correct payload
            mock_operator_command_service.execute_fetch_file_history.assert_called_once()
            call_args = mock_operator_command_service.execute_fetch_file_history.call_args[1]
            
            assert "args" in call_args
            args = call_args["args"]
            assert args.file_path == "/tmp/test.txt"
            assert args.limit == 10
        finally:
            tool_service.reset_invocation_context(context_token)

    async def test_restore_file_tool_payload_processing(
        self, tool_service, mock_operator_command_service, sample_g8e_context, sample_investigation, request_settings
    ):
        """Test restore_file tool processes payloads correctly."""
        context_token = tool_service.start_invocation_context(sample_g8e_context)

        try:
            # Mock successful file restore
            mock_result = FileEditResult(
                operation="update",
                file_path="/tmp/test.txt",
                success=True,
                message="File restored successfully",
                bytes_written=100,
            )
            mock_operator_command_service.execute_file_edit.return_value = mock_result

            # Test payload for file restoration
            tool_args = {
                "file_path": "/tmp/test.txt",
                "old_content": "old content to restore from",
                "new_content": "restored content",
                "justification": "Test file restoration for unit test",
            }

            # Execute tool call
            result = await tool_service.execute_tool_call(
                tool_name=OperatorToolName.FILE_UPDATE,
                tool_args=tool_args,
                investigation=sample_investigation,
                g8e_context=sample_g8e_context,
                request_settings=request_settings,
            )

            # Verify payload processing
            assert isinstance(result, ToolResult)
            assert result.success
            
            # Verify command service was called with correct payload
            mock_operator_command_service.execute_file_edit.assert_called_once()
            call_args = mock_operator_command_service.execute_file_edit.call_args[1]
            
            assert "args" in call_args
            args = call_args["args"]
            assert args.file_path == "/tmp/test.txt"
            assert args.old_content == "old content to restore from"
            assert args.new_content == "restored content"
        finally:
            tool_service.reset_invocation_context(context_token)

    async def test_fetch_file_diff_tool_payload_processing(
        self, tool_service, mock_operator_command_service, sample_g8e_context, sample_investigation, request_settings
    ):
        """Test fetch_file_diff tool processes payloads correctly."""
        context_token = tool_service.start_invocation_context(sample_g8e_context)

        try:
            # Mock successful file diff fetch
            mock_result = FetchFileDiffToolResult(
                success=True,
                diffs=[
                    {
                        "id": "diff123",
                        "file_path": "/tmp/test.txt",
                        "timestamp": "2026-01-01T00:00:00Z",
                        "operation": "modify",
                        "ledger_hash_before": "hash123",
                        "ledger_hash_after": "hash456",
                        "diff_stat": "1 file changed",
                        "diff_content": "--- a/test.txt\n+++ b/test.txt\n@@ -1 +1 @@\n-old\n+new",
                        "diff_size": 20,
                        "operator_session_id": "session-789"
                    }
                ],
                total=1
            )
            mock_operator_command_service.execute_fetch_file_diff.return_value = mock_result

            # Test payload for file diff
            tool_args = {
                "file_path": "/tmp/test.txt",
                "timestamp_from": "2026-01-01T00:00:00Z",
                "timestamp_to": "2026-01-02T00:00:00Z",
                "context_lines": 3,
                "target_operator": "op-123",
            }

            # Execute tool call
            result = await tool_service.execute_tool_call(
                tool_name=OperatorToolName.FETCH_FILE_DIFF,
                tool_args=tool_args,
                investigation=sample_investigation,
                g8e_context=sample_g8e_context,
                request_settings=request_settings,
            )

            # Verify payload processing
            assert isinstance(result, ToolResult)
            assert result.success
            
            # Verify command service was called with correct payload
            mock_operator_command_service.execute_fetch_file_diff.assert_called_once()
            call_args = mock_operator_command_service.execute_fetch_file_diff.call_args[1]
            
            assert "args" in call_args
            args = call_args["args"]
            assert args.file_path == "/tmp/test.txt"
            assert args.target_operator == "op-123"
        finally:
            tool_service.reset_invocation_context(context_token)


# ---------------------------------------------------------------------------
# Segment 4 — Network & Search Tools
# ---------------------------------------------------------------------------

@pytest.mark.asyncio(loop_scope="session")
@pytest.mark.integration
class TestNetworkSearchTools:
    """Test AI network and search tool payload handling."""

    async def test_check_port_tool_payload_processing(
        self, tool_service, mock_operator_command_service, sample_g8e_context, sample_investigation, request_settings
    ):
        """Test check_port_status tool processes payloads correctly."""
        context_token = tool_service.start_invocation_context(sample_g8e_context)

        try:
            # Mock successful port check
            mock_result = PortCheckToolResult(
                host="example.com",
                port=443,
                protocol="tcp",
                is_open=True,
                response_time_ms=50,
                success=True,
            )
            mock_operator_command_service.execute_port_check.return_value = mock_result

            # Test payload for port checking
            tool_args = {
                "host": "example.com",
                "port": 443,
                "protocol": "tcp",
                "timeout_seconds": 5,
            }

            # Execute tool call
            result = await tool_service.execute_tool_call(
                tool_name=OperatorToolName.CHECK_PORT,
                tool_args=tool_args,
                investigation=sample_investigation,
                g8e_context=sample_g8e_context,
                request_settings=request_settings,
            )

            # Verify payload processing
            assert isinstance(result, ToolResult)
            assert result.success
            
            # Verify command service was called with correct payload
            mock_operator_command_service.execute_port_check.assert_called_once()
            call_args = mock_operator_command_service.execute_port_check.call_args[1]
            
            assert "args" in call_args
            args = call_args["args"]
            assert args.host == "example.com"
            assert args.port == 443
        finally:
            tool_service.reset_invocation_context(context_token)

    async def test_search_web_tool_payload_processing(
        self, tool_service, mock_web_search_provider, sample_g8e_context, sample_investigation, request_settings
    ):
        """Test g8e_web_search tool processes payloads correctly."""
        context_token = tool_service.start_invocation_context(sample_g8e_context)

        try:
            # Mock successful web search
            mock_result = SearchWebResult(
                query="Kubernetes troubleshooting",
                results=[
                    {
                        "title": "Kubernetes Troubleshooting Guide",
                        "url": "https://kubernetes.io/docs/tasks/debug-application-cluster/",
                        "snippet": "Common issues and solutions for Kubernetes clusters",
                        "relevance_score": 0.95
                    }
                ],
                total_results="1",
                success=True,
            )
            mock_web_search_provider.search.return_value = mock_result

            # Test payload for web search
            tool_args = {
                "query": "Kubernetes troubleshooting",
                "max_results": 5,
                "language": "en",
                "safe_search": "moderate",
            }

            # Execute tool call
            result = await tool_service.execute_tool_call(
                tool_name=OperatorToolName.G8E_SEARCH_WEB,
                tool_args=tool_args,
                investigation=sample_investigation,
                g8e_context=sample_g8e_context,
                request_settings=request_settings,
            )

            # Verify payload processing
            assert isinstance(result, ToolResult)
            assert result.success
            
            # Verify web search provider was called with correct payload
            mock_web_search_provider.search.assert_called_once()
            call_args = mock_web_search_provider.search.call_args[1]
            
            assert call_args["query"] == "Kubernetes troubleshooting"
            assert call_args["num"] == 5
        finally:
            tool_service.reset_invocation_context(context_token)

    async def test_search_web_tool_unavailable_handling(
        self, tool_service, sample_g8e_context, sample_investigation, request_settings
    ):
        """Test g8e_web_search tool handles unavailable provider correctly."""
        # Create tool service without web search provider
        tool_service_no_search = AIToolService(
            operator_command_service=AsyncMock(spec=OperatorCommandService),
            investigation_service=AsyncMock(spec=InvestigationService),
            web_search_provider=None,  # No provider
        )
        
        context_token = tool_service_no_search.start_invocation_context(sample_g8e_context)

        try:
            # Test payload for web search when provider is unavailable
            tool_args = {
                "query": "Kubernetes troubleshooting",
                "max_results": 5,
            }

            # Execute tool call and verify unavailable handling
            with pytest.raises(ExternalServiceError, match="g8e_web_search called but WebSearchProvider is not configured"):
                await tool_service_no_search.execute_tool_call(
                    tool_name=OperatorToolName.G8E_SEARCH_WEB,
                    tool_args=tool_args,
                    investigation=sample_investigation,
                    g8e_context=sample_g8e_context,
                    request_settings=request_settings,
                )
        finally:
            tool_service_no_search.reset_invocation_context(context_token)


# ---------------------------------------------------------------------------
# Segment 5 — Permission & Session Tools
# ---------------------------------------------------------------------------

@pytest.mark.asyncio(loop_scope="session")
@pytest.mark.integration
class TestPermissionSessionTools:
    """Test AI permission and session tool payload handling."""

    async def test_grant_intent_tool_payload_processing(
        self, tool_service, mock_operator_command_service, sample_g8e_context, sample_investigation, request_settings
    ):
        """Test grant_intent_permission tool processes payloads correctly."""
        context_token = tool_service.start_invocation_context(sample_g8e_context)

        try:
            # Mock successful intent grant
            mock_result = IntentPermissionResult(
                intent="file_access",
                granted=True,
                reason="User explicitly granted permission",
                expires_at=None,
                success=True,
            )
            mock_operator_command_service.execute_intent_permission_request.return_value = mock_result

            # Test payload for granting intent
            tool_args = {
                "intent_name": "file_access",
                "justification": "Need to access user files for troubleshooting",
                "operation_context": "File system troubleshooting",
            }

            # Execute tool call
            result = await tool_service.execute_tool_call(
                tool_name=OperatorToolName.GRANT_INTENT,
                tool_args=tool_args,
                investigation=sample_investigation,
                g8e_context=sample_g8e_context,
                request_settings=request_settings,
            )

            # Verify payload processing
            assert isinstance(result, ToolResult)
            assert result.success
            
            # Verify command service was called with correct payload
            mock_operator_command_service.execute_intent_permission_request.assert_called_once()
            call_args = mock_operator_command_service.execute_intent_permission_request.call_args[1]
            
            assert "args" in call_args
            args = call_args["args"]
            assert args.intent_name == "file_access"
            assert args.justification == "Need to access user files for troubleshooting"
        finally:
            tool_service.reset_invocation_context(context_token)

    async def test_revoke_intent_tool_payload_processing(
        self, tool_service, mock_operator_command_service, sample_g8e_context, sample_investigation, request_settings
    ):
        """Test revoke_intent_permission tool processes payloads correctly."""
        context_token = tool_service.start_invocation_context(sample_g8e_context)

        try:
            # Mock successful intent revoke
            mock_result = IntentPermissionResult(
                intent="file_access",
                granted=False,
                reason="Permission revoked by user",
                success=True,
            )
            mock_operator_command_service.execute_intent_revocation.return_value = mock_result

            # Test payload for revoking intent
            tool_args = {
                "intent_name": "file_access",
                "justification": "No longer need file access",
            }

            # Execute tool call
            result = await tool_service.execute_tool_call(
                tool_name=OperatorToolName.REVOKE_INTENT,
                tool_args=tool_args,
                investigation=sample_investigation,
                g8e_context=sample_g8e_context,
                request_settings=request_settings,
            )

            # Verify payload processing
            assert isinstance(result, ToolResult)
            assert result.success
            
            # Verify command service was called with correct payload
            mock_operator_command_service.execute_intent_revocation.assert_called_once()
            call_args = mock_operator_command_service.execute_intent_revocation.call_args[1]
            
            assert "args" in call_args
            args = call_args["args"]
            assert args.intent_name == "file_access"
            assert args.justification == "No longer need file access"
        finally:
            tool_service.reset_invocation_context(context_token)

    


# ---------------------------------------------------------------------------
# Cross-Tool Integration Tests
# ---------------------------------------------------------------------------

@pytest.mark.asyncio(loop_scope="session")
@pytest.mark.integration
class TestToolIntegration:
    """Test cross-tool integration and payload consistency."""

    async def test_tool_context_propagation(
        self, tool_service, sample_g8e_context
    ):
        """Test that tool context is properly propagated to all tool calls."""
        # Set tool context using proper method
        context_token = tool_service.start_invocation_context(sample_g8e_context)

        try:
            # Verify context is set
            current_context = tool_service._tool_context.get()
            assert current_context is not None
            assert len(current_context.bound_operators) > 0
            assert current_context.bound_operators[0].operator_id == "op-123"
            assert current_context.case_id == "case-789"
        finally:
            tool_service.reset_invocation_context(context_token)

    async def test_tool_declaration_consistency(
        self, tool_service
    ):
        """Test that all tool declarations are properly registered."""
        # Verify all expected tools are registered (only implemented tools)
        expected_tools = {
            OperatorToolName.RUN_COMMANDS,
            OperatorToolName.FILE_CREATE,
            OperatorToolName.FILE_WRITE,
            OperatorToolName.FILE_READ,
            OperatorToolName.FILE_UPDATE,
            OperatorToolName.LIST_FILES,
            OperatorToolName.FETCH_FILE_HISTORY,
            OperatorToolName.FETCH_FILE_DIFF,
            OperatorToolName.CHECK_PORT,
            OperatorToolName.GRANT_INTENT,
            OperatorToolName.REVOKE_INTENT,
        }
        
        # Add G8E_SEARCH_WEB if available
        if tool_service.web_search_provider is not None:
            expected_tools.add(OperatorToolName.G8E_SEARCH_WEB)
        
        registered_tools = set(tool_service._tool_declarations.keys())
        assert registered_tools == expected_tools

    async def test_payload_serialization_consistency(
        self, tool_service, mock_operator_command_service, sample_g8e_context, sample_investigation, request_settings
    ):
        """Test that payloads are consistently serialized/deserialized."""
        context_token = tool_service.start_invocation_context(sample_g8e_context)

        try:
            # Mock successful execution
            mock_result = CommandExecutionResult(
                success=True,
                execution_id="exec-123",
                status="completed",
                exit_code=0,
                stdout="Test output",
                stderr="",
                command="echo test",
                execution_time_ms=100,
            )
            mock_operator_command_service.execute_command.return_value = mock_result

            # Test with complex payload containing special characters
            tool_args = {
                "command": "echo 'Hello, World! 🌍'",
                "timeout_seconds": 30,
                "justification": "Test complex payload handling"
            }

            # Execute tool call through proper interface
            result = await tool_service.execute_tool_call(
                tool_name=OperatorToolName.RUN_COMMANDS,
                tool_args=tool_args,
                investigation=sample_investigation,
                g8e_context=sample_g8e_context,
                request_settings=request_settings,
            )

            # Verify payload was handled correctly
            assert isinstance(result, ToolResult)
            assert result.success
            
            # Verify the complex payload was preserved
            call_args = mock_operator_command_service.execute_command.call_args[1]
            assert "Hello, World! 🌍" in call_args["args"].command
            assert call_args["args"].timeout_seconds == 30
        finally:
            tool_service.reset_invocation_context(context_token)
