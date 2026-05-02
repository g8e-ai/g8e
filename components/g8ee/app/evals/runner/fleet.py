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
        self._env = {**subprocess.os.environ, "COMPOSE_PROJECT_NAME": "evals"}

    def up(self, nodes: int, device_token: str) -> None:
        """Bring up the eval fleet with N nodes.

        Args:
            nodes: Number of eval nodes to start
            device_token: Device link token for operator authentication
        """
        env = {**self._env, "DEVICE_TOKEN": device_token}
        cmd = [
            "docker", "compose", "-f", str(self.compose_file),
            "up", "-d",
            "--scale", f"eval-node={nodes}",
        ]
        subprocess.run(cmd, env=env, check=True)

    def down(self) -> None:
        """Tear down the eval fleet."""
        cmd = ["docker", "compose", "-f", str(self.compose_file), "down"]
        subprocess.run(cmd, env=self._env, check=True)

    def status(self) -> str:
        """Get status of eval nodes.

        Returns:
            Output from docker compose ps
        """
        cmd = ["docker", "compose", "-f", str(self.compose_file), "ps"]
        result = subprocess.run(cmd, capture_output=True, text=True, env=self._env, check=True)
        return result.stdout

    def restart(self, node_id: str) -> None:
        """Restart a specific eval node.

        Args:
            node_id: Container name (e.g., evals-eval-node-1)
        """
        cmd = ["docker", "restart", node_id]
        subprocess.run(cmd, env=self._env, check=True)

    def logs(self, node_id: str, tail: int = 200) -> str:
        """Get logs for a specific node.

        Args:
            node_id: Container name (e.g., evals-eval-node-1)
            tail: Number of lines from the end of logs

        Returns:
            Log output
        """
        cmd = ["docker", "logs", "--tail", str(tail), node_id]
        result = subprocess.run(cmd, capture_output=True, text=True, check=True)
        return result.stdout

    def is_running(self) -> bool:
        """Check if any eval nodes are running.

        Returns:
            True if at least one eval node is running
        """
        cmd = ["docker", "compose", "-f", str(self.compose_file), "ps", "-q"]
        result = subprocess.run(cmd, capture_output=True, text=True, env=self._env)
        container_ids = result.stdout.strip().split("\n")
        
        for container_id in container_ids:
            if not container_id:
                continue
            cmd = ["docker", "inspect", "-f", "{{.State.Running}}", container_id]
            result = subprocess.run(cmd, capture_output=True, text=True)
            if result.stdout.strip() == "true":
                return True
        return False

    def get_device_token(self) -> str | None:
        """Read DEVICE_TOKEN from a running eval node container.

        Returns:
            Device token if found, None otherwise
        """
        cmd = ["docker", "compose", "-f", str(self.compose_file), "ps", "-q"]
        result = subprocess.run(cmd, capture_output=True, text=True, env=self._env)
        container_ids = result.stdout.strip().split("\n")
        
        for container_id in container_ids:
            if not container_id:
                continue
            cmd = ["docker", "inspect", "-f", "{{.State.Running}}", container_id]
            result = subprocess.run(cmd, capture_output=True, text=True)
            if result.stdout.strip() != "true":
                continue
            
            cmd = ["docker", "exec", container_id, "printenv", "DEVICE_TOKEN"]
            result = subprocess.run(cmd, capture_output=True, text=True)
            token = result.stdout.strip()
            if token:
                return token
        return None

    async def wait_bound(self, timeout: int = 60) -> None:
        """Wait for all eval nodes to reach BOUND status.

        Args:
            timeout: Maximum seconds to wait
        """
        import time
        import aiohttp
        import ssl

        ssl_context = ssl.create_default_context()
        ssl_context.check_hostname = False
        ssl_context.verify_mode = ssl.CERT_NONE
        connector = aiohttp.TCPConnector(ssl=ssl_context)

        print("[fleet] Polling for fleet ready status...")
        health_url = "https://g8e.local/health"
        start_time = time.time()

        async with aiohttp.ClientSession(connector=connector) as session:
            while time.time() - start_time < timeout:
                try:
                    async with session.get(health_url) as resp:
                        if resp.status == 200:
                            data = await resp.json()
                            if data.get("status") == "ok":
                                print("[fleet] Fleet is ready (health check passed)")
                                return
                except Exception:
                    pass

                await asyncio.sleep(2)

        raise TimeoutError(f"Fleet failed to become ready within {timeout} seconds")
