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

"""Operator Stream Executor

Orchestrates the Phase 4 'stream' operation:
1. Mints a dlk_ token via g8ed (internal API)
2. Requests human approval via OperatorApprovalService
3. Executes 'docker exec g8ep ... stream' command
"""

import logging
import subprocess
import asyncio
from typing import TYPE_CHECKING

from app.constants.status import CommandErrorType, ComponentName
from app.models.operators import StreamApprovalRequest, ApprovalResult
from app.models.tool_results import CommandExecutionResult
from app.models.http_context import G8eHttpContext
from app.models.tool_args import StreamOperatorArgs
from app.errors import ExternalServiceError, ValidationError

if TYPE_CHECKING:
    from app.services.operator.approval_service import OperatorApprovalService
    from app.services.infra.internal_http_client import InternalHttpClient
    from app.models.settings import G8eePlatformSettings

logger = logging.getLogger(__name__)

class OperatorStreamExecutor:
    """Handles multi-host operator streaming orchestration."""

    def __init__(
        self,
        approval_service: "OperatorApprovalService",
        internal_http_client: "InternalHttpClient",
        settings: "G8eePlatformSettings",
    ) -> None:
        self._approval_service = approval_service
        self._internal_http_client = internal_http_client
        self._settings = settings

    async def execute_stream(
        self,
        args: StreamOperatorArgs,
        g8e_context: G8eHttpContext,
        execution_id: str,
    ) -> CommandExecutionResult:
        """Execute the stream operation across multiple hosts."""
        
        # 1. Mint dlk_ token
        try:
            # Note: We need to verify the exact endpoint for minting dlk_ tokens in g8ed
            # For now, we'll assume a standard internal path
            token_response = await self._internal_http_client.post(
                "/api/internal/tokens/mint-device-link",
                json={"user_id": g8e_context.user_id}
            )
            device_token = token_response.get("token")
            if not device_token:
                raise ExternalServiceError(
                    "Failed to mint device link token: no token in response",
                    service_name="g8ed",
                    component=ComponentName.G8EE
                )
        except Exception as e:
            logger.error("[STREAM_EXECUTOR] Failed to mint dlk_ token: %s", e)
            return CommandExecutionResult(
                success=False,
                error=f"Failed to mint device link token: {e}",
                error_type=CommandErrorType.EXECUTION_ERROR,
                execution_id=execution_id
            )

        # 2. Request Approval
        approval_request = StreamApprovalRequest(
            g8e_context=g8e_context,
            timeout_seconds=300,  # Default timeout for streaming approval
            justification=args.justification,
            execution_id=execution_id,
            operator_session_id="",  # Not bound to a specific session
            operator_id="",         # Not bound to a specific operator
            hosts=args.hosts,
            arch=args.arch,
            endpoint=self._settings.component_urls.g8ed_url,
            device_token=device_token,
            concurrency=args.concurrency,
            timeout=args.timeout_seconds,
        )

        approval_result = await self._approval_service.request_stream_approval(approval_request)
        if not approval_result.approved:
            return CommandExecutionResult(
                success=False,
                error=approval_result.reason or "Stream operation denied by user",
                error_type=CommandErrorType.APPROVAL_DENIED,
                execution_id=execution_id
            )

        # 3. Execute 'docker exec g8ep ... stream'
        # Construct the command
        hosts_str = ",".join(args.hosts)
        cmd = [
            "docker", "exec", "g8ep",
            "/home/bob/g8e/g8e", "operator", "stream",
            "--hosts", hosts_str,
            "--arch", args.arch,
            "--token", device_token,
            "--concurrency", str(args.concurrency),
            "--timeout", str(args.timeout_seconds)
        ]

        logger.info("[STREAM_EXECUTOR] Executing: %s", " ".join(cmd))
        
        try:
            # We use asyncio.create_subprocess_exec for non-blocking execution
            process = await asyncio.create_subprocess_exec(
                *cmd,
                stdout=asyncio.subprocess.PIPE,
                stderr=asyncio.subprocess.PIPE
            )
            
            stdout, stderr = await process.communicate()
            
            success = process.returncode == 0
            output = stdout.decode().strip()
            error_output = stderr.decode().strip()
            
            if success:
                logger.info("[STREAM_EXECUTOR] Stream succeeded: %s", output)
                return CommandExecutionResult(
                    success=True,
                    output=output,
                    execution_id=execution_id,
                    command_executed=" ".join(cmd[:6]) + " ... [REDACTED TOKEN]"
                )
            else:
                logger.error("[STREAM_EXECUTOR] Stream failed (exit %d): %s", process.returncode, error_output)
                return CommandExecutionResult(
                    success=False,
                    error=f"Stream execution failed: {error_output}",
                    output=output,
                    error_type=CommandErrorType.EXECUTION_ERROR,
                    execution_id=execution_id,
                    exit_code=process.returncode
                )
                
        except Exception as e:
            logger.exception("[STREAM_EXECUTOR] Exception during docker exec: %s", e)
            return CommandExecutionResult(
                success=False,
                error=f"Unexpected error during stream execution: {e}",
                error_type=CommandErrorType.EXECUTION_ERROR,
                execution_id=execution_id
            )
