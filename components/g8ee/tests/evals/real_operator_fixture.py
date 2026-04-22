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
Real operator fixture for eval tests.

This fixture starts a real g8e operator process for eval tests, following
the pattern used in the demo (device link token authentication).

The operator is started in the g8ep container via docker exec, since g8ep
already has the operator binary at /home/g8e/g8e.operator and is on the
g8e-network where it can reach g8e.local.
"""

import asyncio
import logging
import os
import ssl
import subprocess
import tempfile
from dataclasses import dataclass
from typing import Optional

import httpx

logger = logging.getLogger(__name__)


@dataclass
class RunningOperator:
    """Holds information about a running real operator."""
    operator_id: str
    operator_session_id: str
    device_token: str
    container_name: str = "g8ep"


class RealOperatorFixture:
    """Manages a real g8e operator for eval tests."""

    def __init__(
        self,
        device_token: Optional[str] = None,
        g8ed_url: str = "https://g8ed",
        internal_auth_token: Optional[str] = None,
    ):
        self.device_token = device_token or os.environ.get("TEST_DEVICE_TOKEN")
        if not self.device_token:
            raise ValueError(
                "TEST_DEVICE_TOKEN environment variable must be set, "
                "or device_token must be provided. "
                "Generate a device link token from the dashboard."
            )

        self.g8ed_url = g8ed_url
        self.internal_auth_token = internal_auth_token or os.environ.get(
            "G8E_INTERNAL_AUTH_TOKEN"
        )
        self.running_operator: Optional[RunningOperator] = None
        self.operator_process: Optional[subprocess.Popen] = None

    async def start_operator(self) -> RunningOperator:
        """Start a real operator in the test runner container.

        Follows the demo pattern:
        1. Download operator binary from g8e.local
        2. Run with device token
        3. Wait for operator to become ACTIVE
        4. Return operator details
        """
        logger.info("[REAL-OP-FIXTURE] Starting operator with device token")

        # Download operator binary from platform
        operator_path = os.path.join(tempfile.gettempdir(), "g8e.operator")
        operator_sha_path = f"{operator_path}.sha256"

        logger.info("[REAL-OP-FIXTURE] Downloading operator binary from g8e.local")

        verify_context = ssl.create_default_context(cafile="/g8es/ca.crt") if os.path.exists("/g8es/ca.crt") else False
        async with httpx.AsyncClient(
            verify=verify_context,
            timeout=30.0,
        ) as client:
            # Download binary
            response = await client.get(
                "https://g8e.local/operator/download/linux/amd64",
                headers={"Authorization": f"Bearer {self.device_token}"},
            )
            response.raise_for_status()

            with open(operator_path, "wb") as f:
                f.write(response.content)

            # Download and verify checksum
            response = await client.get(
                "https://g8e.local/operator/download/linux/amd64/sha256",
                headers={"Authorization": f"Bearer {self.device_token}"},
            )
            response.raise_for_status()

            expected_sha = response.text.strip()
            with open(operator_sha_path, "w") as f:
                f.write(expected_sha)

            # Verify checksum
            import hashlib
            with open(operator_path, "rb") as f:
                actual_sha = hashlib.sha256(f.read()).hexdigest()

            if actual_sha != expected_sha.split()[0]:
                os.remove(operator_path)
                raise ValueError(
                    f"Operator binary checksum mismatch: {actual_sha} != {expected_sha}"
                )

        # Make executable
        os.chmod(operator_path, 0o755)
        logger.info("[REAL-OP-FIXTURE] Operator binary downloaded and verified")

        # Start operator process
        cmd = [
            operator_path,
            "-D", self.device_token,
            "-e", "g8e.local",
            "--http-port", "9000",
            "--wss-port", "9001",
            "--no-git",
            "--log", "info",
        ]

        logger.info("[REAL-OP-FIXTURE] Executing: %s", " ".join(cmd))

        self.operator_process = subprocess.Popen(
            cmd,
            stdout=subprocess.PIPE,
            stderr=subprocess.PIPE,
            text=True,
        )

        # Wait for operator to register and become ACTIVE
        operator_info = await self._wait_for_operator_active(timeout=30)

        self.running_operator = RunningOperator(
            operator_id=operator_info["id"],
            operator_session_id=operator_info["operator_session_id"],
            device_token=self.device_token,
        )

        logger.info(
            "[REAL-OP-FIXTURE] Operator started: %s (session: %s)",
            self.running_operator.operator_id,
            self.running_operator.operator_session_id,
        )

        return self.running_operator

    async def _wait_for_operator_active(
        self, timeout: int = 30
    ) -> dict:
        """Poll g8ed API until operator becomes ACTIVE."""
        headers = {}
        if self.internal_auth_token:
            headers["X-G8E-Internal-Auth-Token"] = self.internal_auth_token

        verify_context = ssl.create_default_context(cafile="/g8es/ca.crt") if os.path.exists("/g8es/ca.crt") else False
        async with httpx.AsyncClient(
            verify=verify_context,
            timeout=10.0,
        ) as client:
            start_time = asyncio.get_event_loop().time()
            while asyncio.get_event_loop().time() - start_time < timeout:
                try:
                    response = await client.get(
                        f"{self.g8ed_url}/api/operators",
                        headers=headers,
                    )
                    response.raise_for_status()
                    operators = response.json()

                    # Find the operator that just started (most recent)
                    # Filter for ACTIVE status
                    active_operators = [
                        op for op in operators
                        if op.get("status") == "ACTIVE"
                    ]

                    if active_operators:
                        # Return the most recent ACTIVE operator
                        latest = max(
                            active_operators,
                            key=lambda op: op.get("created_at", ""),
                        )
                        logger.info(
                            "[REAL-OP-FIXTURE] Operator ACTIVE: %s",
                            latest["id"],
                        )
                        return latest

                    await asyncio.sleep(1)

                except Exception as e:
                    logger.debug(
                        "[REAL-OP-FIXTURE] Poll error (will retry): %s", e
                    )
                    await asyncio.sleep(1)

        raise TimeoutError(
            f"Operator did not become ACTIVE within {timeout} seconds"
        )

    async def stop_operator(self):
        """Stop the running operator."""
        if self.operator_process:
            logger.info("[REAL-OP-FIXTURE] Stopping operator process")
            self.operator_process.terminate()
            try:
                self.operator_process.wait(timeout=5)
            except subprocess.TimeoutExpired:
                self.operator_process.kill()
                self.operator_process.wait()
            self.operator_process = None

        if self.running_operator:
            logger.info(
                "[REAL-OP-FIXTURE] Stopping operator via API: %s",
                self.running_operator.operator_id,
            )
            await self._stop_operator_via_api(self.running_operator.operator_id)
            self.running_operator = None

        # Clean up downloaded binary
        operator_path = os.path.join(tempfile.gettempdir(), "g8e.operator")
        operator_sha_path = f"{operator_path}.sha256"
        for path in [operator_path, operator_sha_path]:
            if os.path.exists(path):
                os.remove(path)
                logger.info("[REAL-OP-FIXTURE] Removed %s", path)

    async def _stop_operator_via_api(self, operator_id: str):
        """Stop operator via g8ed API."""
        headers = {}
        if self.internal_auth_token:
            headers["X-G8E-Internal-Auth-Token"] = self.internal_auth_token

        verify_context = ssl.create_default_context(cafile="/g8es/ca.crt") if os.path.exists("/g8es/ca.crt") else False
        async with httpx.AsyncClient(
            verify=verify_context,
            timeout=10.0,
        ) as client:
            try:
                response = await client.post(
                    f"{self.g8ed_url}/api/operators/{operator_id}/stop",
                    headers=headers,
                )
                response.raise_for_status()
                logger.info("[REAL-OP-FIXTURE] Operator stopped via API")
            except Exception as e:
                logger.warning(
                    "[REAL-OP-FIXTURE] Failed to stop operator via API: %s", e
                )

    async def bind_to_session(
        self, web_session_id: str, user_id: str
    ) -> bool:
        """Bind the operator to a web session."""
        if not self.running_operator:
            raise RuntimeError("No operator running")

        logger.info(
            "[REAL-OP-FIXTURE] Binding operator %s to session %s",
            self.running_operator.operator_id,
            web_session_id,
        )

        headers = {}
        if self.internal_auth_token:
            headers["X-G8E-Internal-Auth-Token"] = self.internal_auth_token

        verify_context = ssl.create_default_context(cafile="/g8es/ca.crt") if os.path.exists("/g8es/ca.crt") else False
        async with httpx.AsyncClient(
            verify=verify_context,
            timeout=10.0,
        ) as client:
            try:
                response = await client.post(
                    f"{self.g8ed_url}/api/operators/bind",
                    headers=headers,
                    json={
                        "operator_id": self.running_operator.operator_id,
                        "web_session_id": web_session_id,
                        "user_id": user_id,
                    },
                )
                response.raise_for_status()
                logger.info("[REAL-OP-FIXTURE] Operator bound successfully")
                return True
            except Exception as e:
                logger.error(
                    "[REAL-OP-FIXTURE] Failed to bind operator: %s", e
                )
                return False

    async def __aenter__(self):
        return await self.start_operator()

    async def __aexit__(self, exc_type, exc_val, exc_tb):
        await self.stop_operator()
