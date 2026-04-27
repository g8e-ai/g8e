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

    async def wait_bound(self, timeout: int = 60, device_token: str | None = None) -> None:
        """Wait for all eval nodes to reach BOUND status.

        Args:
            timeout: Maximum seconds to wait
            device_token: Device link token for auth when polling the API
        """
        import time
        import aiohttp
        import ssl
        from datetime import datetime, UTC

        ssl_context = ssl.create_default_context()
        ssl_context.check_hostname = False
        ssl_context.verify_mode = ssl.CERT_NONE
        connector = aiohttp.TCPConnector(ssl=ssl_context)

        if not device_token:
            print("[fleet] Warning: wait_bound called without device_token, polling /health endpoint")
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
                    except Exception as e:
                        pass
                    
                    await asyncio.sleep(2)
            
            raise TimeoutError(f"Fleet failed to become ready within {timeout} seconds")

        url = "https://g8e.local/api/auth/operator/validate"
        headers = {"X-Request-Timestamp": datetime.now(UTC).strftime("%Y-%m-%dT%H:%M:%S.%f")[:-3] + "Z"}
        payload = {
            "auth_mode": "operator_session",
            "operator_session_id": device_token
        }

        print("[fleet] Polling for operator ready status...")
        start_time = time.time()
        
        async with aiohttp.ClientSession(connector=connector) as session:
            while time.time() - start_time < timeout:
                try:
                    async with session.post(url, json=payload, headers=headers) as resp:
                        if resp.status == 200:
                            print("[fleet] Operator validation successful, checking fleet health...")
                            health_url = "https://g8e.local/api/internal/health"
                            health_start_time = time.time()
                            health_timeout = 30
                            
                            while time.time() - health_start_time < health_timeout:
                                try:
                                    async with session.get(health_url) as health_resp:
                                        if health_resp.status == 200:
                                            health_data = await health_resp.json()
                                            if health_data.get("status") == "healthy":
                                                print("[fleet] Fleet is ready (internal health check passed)")
                                                return
                                except Exception as e:
                                    pass
                                
                                await asyncio.sleep(1)
                            
                            raise TimeoutError(f"Fleet health check failed within {health_timeout} seconds after operator validation")
                except Exception as e:
                    pass
                
                await asyncio.sleep(2)
                
        raise TimeoutError(f"Operators failed to become ready within {timeout} seconds")
