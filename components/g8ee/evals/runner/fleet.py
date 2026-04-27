# Copyright (c) 2026 Lateralus Labs, LLC.
# Licensed under the Apache License, Version 2.0

"""Fleet management for eval nodes via docker compose."""

from __future__ import annotations

import subprocess
import asyncio
from pathlib import Path


class FleetManager:
    """Manages the eval-node fleet using docker compose."""

    def __init__(self, compose_file: Path):
        self.compose_file = compose_file

    def up(self, nodes: int, device_token: str) -> None:
        """Bring up the eval fleet with N nodes.

        Args:
            nodes: Number of eval nodes to start
            device_token: Device link token for operator authentication
        """
        env = {"DEVICE_TOKEN": device_token}
        cmd = [
            "docker", "compose", "-f", str(self.compose_file),
            "up", "-d",
            "--scale", "eval-node-01=1",
            "--scale", "eval-node-02=1",
            "--scale", "eval-node-03=1",
        ]
        subprocess.run(cmd, env={**subprocess.os.environ, **env}, check=True)

    def down(self) -> None:
        """Tear down the eval fleet."""
        cmd = ["docker", "compose", "-f", str(self.compose_file), "down"]
        subprocess.run(cmd, check=True)

    def status(self) -> str:
        """Get status of eval nodes.

        Returns:
            Output from docker compose ps
        """
        cmd = ["docker", "compose", "-f", str(self.compose_file), "ps"]
        result = subprocess.run(cmd, capture_output=True, text=True, check=True)
        return result.stdout

    def restart(self, node_id: str) -> None:
        """Restart a specific eval node.

        Args:
            node_id: Container name (e.g., eval-node-01)
        """
        cmd = ["docker", "compose", "-f", str(self.compose_file), "restart", node_id]
        subprocess.run(cmd, check=True)

    def logs(self, node_id: str, tail: int = 200) -> str:
        """Get logs for a specific node.

        Args:
            node_id: Container name (e.g., eval-node-01)
            tail: Number of lines from the end of logs

        Returns:
            Log output
        """
        cmd = ["docker", "logs", "--tail", str(tail), node_id]
        result = subprocess.run(cmd, capture_output=True, text=True, check=True)
        return result.stdout

    async def wait_bound(self, timeout: int = 60) -> None:
        """Wait for all eval nodes to reach BOUND status.

        This is a placeholder - in the full implementation, this would poll
        the g8ed API for operator status.

        Args:
            timeout: Maximum seconds to wait
        """
        await asyncio.sleep(5)
