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

        logger.info("[REAL-OP-FIXTURE] Downloading operator binary from %s", self.g8ed_url)

        if os.path.exists("/g8es/ca.crt"):
            verify = ssl.create_default_context(cafile="/g8es/ca.crt")
        else:
            verify = False
        async with httpx.AsyncClient(
            verify=verify,
            timeout=30.0,
        ) as client:
            # Download binary
            response = await client.get(
                f"{self.g8ed_url}/operator/download/linux/amd64",
                headers={"Authorization": f"Bearer {self.device_token}"},
            )
            response.raise_for_status()

            with open(operator_path, "wb") as f:
                f.write(response.content)

            # Download and verify checksum
            response = await client.get(
                f"{self.g8ed_url}/operator/download/linux/amd64/sha256",
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
            "-e", "g8ed",
            "--http-port", "443",
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
            cwd="/tmp"
        )

        # Wait for operator to start and parse operator_id from stdout
        operator_info = await self._wait_for_operator_output(timeout=30)

        self.running_operator = RunningOperator(
            operator_id=operator_info["operator_id"],
            operator_session_id=operator_info["operator_session_id"],
            device_token=self.device_token,
        )

        logger.info(
            "[REAL-OP-FIXTURE] Operator started: %s (session: %s)",
            self.running_operator.operator_id,
            self.running_operator.operator_session_id,
        )

        return self.running_operator

    async def _wait_for_operator_output(
        self, timeout: int = 30
    ) -> dict:
        """Read operator stdout to extract operator_id and operator_session_id."""
        start_time = asyncio.get_event_loop().time()
        
        while asyncio.get_event_loop().time() - start_time < timeout:
            if self.operator_process.poll() is not None:
                # Process has exited
                stdout, stderr = self.operator_process.communicate()
                logger.error(
                    "[REAL-OP-FIXTURE] Operator process exited early - stdout: %s, stderr: %s",
                    stdout,
                    stderr,
                )
                raise RuntimeError(
                    f"Operator process exited with code {self.operator_process.returncode}"
                )
            
            # Try to read from stdout without blocking
            try:
                # Use select to check if there's data available
                import select
                if select.select([self.operator_process.stdout], [], [], 0.1)[0]:
                    line = self.operator_process.stdout.readline()
                    if line:
                        logger.info("[REAL-OP-FIXTURE] Operator output: %s", line.strip())
                        
                        # Parse operator_id and operator_session_id from output
                        # Expected format: "operator_id=op_xxx operator_session_id=sess_xxx"
                        import re
                        operator_id_match = re.search(r'operator_id=([a-zA-Z0-9_-]+)', line)
                        session_id_match = re.search(r'operator_session_id=([a-zA-Z0-9_-]+)', line)
                        
                        if operator_id_match and session_id_match:
                            return {
                                "operator_id": operator_id_match.group(1),
                                "operator_session_id": session_id_match.group(1),
                            }
            except Exception as e:
                logger.debug("[REAL-OP-FIXTURE] Read error (will retry): %s", e)
            
            await asyncio.sleep(0.5)
        
        # Timeout - try to get what we can from current output
        try:
            import select
            while select.select([self.operator_process.stdout], [], [], 0)[0]:
                line = self.operator_process.stdout.readline()
                if line:
                    logger.info("[REAL-OP-FIXTURE] Operator output (timeout): %s", line.strip())
        except:
            pass
        
        raise TimeoutError(
            f"Operator did not output operator_id within {timeout} seconds"
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

        self.running_operator = None

        # Clean up downloaded binary
        operator_path = os.path.join(tempfile.gettempdir(), "g8e.operator")
        operator_sha_path = f"{operator_path}.sha256"
        for path in [operator_path, operator_sha_path]:
            if os.path.exists(path):
                os.remove(path)
                logger.info("[REAL-OP-FIXTURE] Removed %s", path)

    async def __aenter__(self):
        return await self.start_operator()

    async def __aexit__(self, exc_type, exc_val, exc_tb):
        await self.stop_operator()
