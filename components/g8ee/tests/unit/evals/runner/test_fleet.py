# Copyright (c) 2026 Lateralus Labs, LLC.
# Licensed under the Apache License, Version 2.0

import pytest
from pathlib import Path
from unittest.mock import MagicMock, AsyncMock, patch
from app.evals.runner.fleet import FleetManager

@pytest.fixture
def fleet_manager():
    return FleetManager(Path("/tmp/docker-compose.yml"))

def test_fleet_up(fleet_manager):
    with patch('subprocess.run') as mock_run:
        fleet_manager.up(nodes=2, device_token="token-123")
        mock_run.assert_called_once()
        args, kwargs = mock_run.call_args
        assert "up" in args[0]
        assert "--scale" in args[0]
        assert "eval-node=2" in args[0]
        assert kwargs["env"]["DEVICE_TOKEN"] == "token-123"

def test_fleet_down(fleet_manager):
    with patch('subprocess.run') as mock_run:
        fleet_manager.down()
        mock_run.assert_called_once()
        assert "down" in mock_run.call_args[0][0]

def test_fleet_status(fleet_manager):
    with patch('subprocess.run') as mock_run:
        mock_run.return_value.stdout = "running"
        status = fleet_manager.status()
        assert status == "running"
        assert "ps" in mock_run.call_args[0][0]

def test_fleet_restart(fleet_manager):
    with patch('subprocess.run') as mock_run:
        fleet_manager.restart("node-1")
        mock_run.assert_called_once()
        assert "restart" in mock_run.call_args[0][0]
        assert "node-1" in mock_run.call_args[0][0]

def test_fleet_is_running(fleet_manager):
    with patch('subprocess.run') as mock_run:
        # Mock ps -q returning container IDs
        mock_run.side_effect = [
            MagicMock(stdout="id1\nid2\n"),
            MagicMock(stdout="true\n"), # inspect id1
        ]
        assert fleet_manager.is_running() is True

def test_fleet_get_device_token(fleet_manager):
    with patch('subprocess.run') as mock_run:
        # Mock ps -q
        # Mock inspect -> true
        # Mock exec printenv DEVICE_TOKEN -> token
        mock_run.side_effect = [
            MagicMock(stdout="id1\n"),
            MagicMock(stdout="true\n"),
            MagicMock(stdout="token-xyz\n")
        ]
        token = fleet_manager.get_device_token()
        assert token == "token-xyz"

def test_fleet_is_running_none(fleet_manager):
    with patch('subprocess.run') as mock_run:
        # Mock ps -q returning empty
        mock_run.return_value = MagicMock(stdout="")
        assert fleet_manager.is_running() is False

def test_fleet_get_device_token_not_found(fleet_manager):
    with patch('subprocess.run') as mock_run:
        mock_run.side_effect = [
            MagicMock(stdout="id1\n"),
            MagicMock(stdout="false\n") # not running
        ]
        assert fleet_manager.get_device_token() is None

@pytest.mark.asyncio
async def test_fleet_wait_bound_timeout(fleet_manager):
    with patch('aiohttp.ClientSession') as mock_session_cls, \
         patch('asyncio.sleep', AsyncMock()) as mock_sleep:
        mock_session = MagicMock()
        mock_session.__aenter__ = AsyncMock(return_value=mock_session)
        mock_session.__aexit__ = AsyncMock()
        mock_session_cls.return_value = mock_session
        
        mock_resp = MagicMock()
        mock_resp.status = 500 # simulate failure
        mock_resp.__aenter__ = AsyncMock(return_value=mock_resp)
        mock_resp.__aexit__ = AsyncMock()
        mock_session.get.return_value = mock_resp
        
        with pytest.raises(TimeoutError):
            await fleet_manager.wait_bound(timeout=0.1)

@pytest.mark.asyncio
async def test_fleet_wait_bound(fleet_manager):
    with patch('aiohttp.ClientSession') as mock_session_cls, \
         patch('asyncio.sleep', AsyncMock()) as mock_sleep:
        mock_session = MagicMock()
        mock_session.__aenter__ = AsyncMock(return_value=mock_session)
        mock_session.__aexit__ = AsyncMock()
        mock_session_cls.return_value = mock_session
        
        mock_resp = MagicMock()
        mock_resp.status = 200
        mock_resp.json = AsyncMock(return_value={"status": "ok"})
        mock_resp.__aenter__ = AsyncMock(return_value=mock_resp)
        mock_resp.__aexit__ = AsyncMock()
        mock_session.get.return_value = mock_resp
        
        await fleet_manager.wait_bound(timeout=5)
        mock_session.get.assert_called_once_with("https://g8e.local/health")
