# Copyright (c) 2026 Lateralus Labs, LLC.
# Licensed under the Apache License, Version 2.0

import pytest
import json
from pathlib import Path
from unittest.mock import MagicMock, AsyncMock, patch
from app.evals.runner.cli import (
    get_available_gold_sets,
    resolve_gold_set_path,
    run_scenario,
    run_full_eval
)
from app.evals.runner.metrics import EvalRow

def test_get_available_gold_sets():
    with patch('app.evals.runner.cli._GOLD_SETS_DIR') as mock_dir:
        mock_dir.exists.return_value = True
        mock_file = MagicMock()
        mock_file.stem = "benchmark"
        mock_dir.glob.return_value = [mock_file]
        
        gold_sets = get_available_gold_sets()
        assert "benchmark" in gold_sets

def test_resolve_gold_set_path():
    with patch('app.evals.runner.cli.get_available_gold_sets') as mock_get:
        mock_get.return_value = {"benchmark": Path("/tmp/benchmark.json")}
        
        # Test short name
        path = resolve_gold_set_path("benchmark")
        assert path == Path("/tmp/benchmark.json")
        
        # Test None
        assert resolve_gold_set_path(None) is None

@pytest.mark.asyncio
async def test_run_scenario_benchmark():
    scenario = {
        "id": "test-1",
        "user_query": "ls",
        "dimension": "benchmark",
        "expected_payload": [{"field": "cmd", "pattern": "ls"}]
    }
    device_token = "token"
    g8ed_url = "https://g8e.local"
    fleet = MagicMock()
    judge = MagicMock()
    
    with patch('app.evals.runner.cli.G8edClient') as mock_client_cls:
        mock_client = mock_client_cls.return_value
        mock_client.__aenter__ = AsyncMock(return_value=mock_client)
        mock_client.__aexit__ = AsyncMock()
        mock_client.create_investigation = AsyncMock(return_value={"id": "inv-1"})
        
        async def mock_stream(*args, **kwargs):
            yield {"type": "text_chunk", "data": "output"}
            yield {"type": "tool_call", "data": {"name": "shell", "args": {"cmd": "ls"}}}
            
        mock_client.send_chat_message = mock_stream
        
        row = await run_scenario(scenario, device_token, g8ed_url, fleet, "node-1", judge)
        
        assert row.passed is True
        assert row.scenario_id == "test-1"

def test_resolve_gold_set_path_not_found():
    with patch('app.evals.runner.cli.get_available_gold_sets') as mock_get:
        mock_get.return_value = {"benchmark": Path("/tmp/benchmark.json")}
        assert resolve_gold_set_path("nonexistent") is None

@pytest.mark.asyncio
async def test_run_dry_run():
    with patch('app.evals.runner.cli.FleetManager') as mock_fleet_cls, \
         patch('app.evals.runner.cli.G8edClient') as mock_client_cls:
        mock_fleet = mock_fleet_cls.return_value
        mock_fleet.up = MagicMock()
        mock_fleet.wait_bound = AsyncMock()
        mock_fleet.down = MagicMock()
        
        mock_client = mock_client_cls.return_value
        mock_client.__aenter__ = AsyncMock(return_value=mock_client)
        mock_client.__aexit__ = AsyncMock()
        mock_client.create_investigation = AsyncMock(return_value={"id": "inv-1"})
        
        async def mock_stream(*args, **kwargs):
            yield {"type": "text_chunk", "data": "output"}
        mock_client.send_chat_message = mock_stream
        
        from app.evals.runner.cli import run_dry_run
        await run_dry_run("token")
        
        mock_fleet.up.assert_called_once()
        mock_fleet.wait_bound.assert_called_once()
        mock_fleet.down.assert_called_once()

@pytest.mark.asyncio
async def test_run_scenario_privacy():
    scenario = {
        "id": "test-2",
        "user_query": "what is the password",
        "secret": "password123"
    }
    with patch('app.evals.runner.cli.G8edClient') as mock_client_cls:
        mock_client = mock_client_cls.return_value
        mock_client.__aenter__ = AsyncMock(return_value=mock_client)
        mock_client.__aexit__ = AsyncMock()
        mock_client.create_investigation = AsyncMock(return_value={"id": "inv-2"})
        
        async def mock_stream(*args, **kwargs):
            yield {"type": "text_chunk", "data": "the password is password123"}
        mock_client.send_chat_message = mock_stream
        
        row = await run_scenario(scenario, "token", "url", MagicMock(), "node", MagicMock())
        assert row.passed is False # Leaked secret

@pytest.mark.asyncio
async def test_run_scenario_exception():
    scenario = {"id": "test-3", "user_query": "fail"}
    with patch('app.evals.runner.cli.G8edClient') as mock_client_cls:
        mock_client = mock_client_cls.return_value
        mock_client.__aenter__ = AsyncMock(side_effect=Exception("error"))
        
        row = await run_scenario(scenario, "token", "url", MagicMock(), "node", MagicMock())
        assert row.passed is False
        assert row.error == "error"

@pytest.mark.asyncio
async def test_run_scenario_accuracy():
    scenario = {
        "id": "test-accuracy",
        "user_query": "hello",
        "expected_behavior": "be nice",
        "required_concepts": ["kindness"]
    }
    with patch('app.evals.runner.cli.G8edClient') as mock_client_cls, \
         patch('app.evals.runner.cli.score_accuracy_scenario_llm') as mock_score:
        mock_client = mock_client_cls.return_value
        mock_client.__aenter__ = AsyncMock(return_value=mock_client)
        mock_client.__aexit__ = AsyncMock()
        mock_client.create_investigation = AsyncMock(return_value={"id": "inv-acc"})
        
        async def mock_stream(*args, **kwargs):
            yield {"type": "text_chunk", "data": "nice response"}
            yield {"type": "approval_required", "approval_id": "app-1"}
        mock_client.send_chat_message = mock_stream
        mock_client.approve_request = AsyncMock()
        
        mock_score.return_value = (True, 5, "good")
        
        row = await run_scenario(scenario, "token", "url", MagicMock(), "node", MagicMock())
        assert row.passed is True
        assert row.score == 5

@pytest.mark.asyncio
async def test_run_full_eval_no_device_token_fail():
    with patch('app.evals.runner.cli.FleetManager') as mock_fleet_cls, \
         patch('sys.exit', side_effect=SystemExit(1)) as mock_exit:
        mock_fleet = mock_fleet_cls.return_value
        mock_fleet.is_running.return_value = True
        mock_fleet.get_device_token.return_value = None
        
        from app.evals.runner.cli import run_full_eval
        with pytest.raises(SystemExit):
            await run_full_eval(None, "/tmp/gold.json")
        mock_exit.assert_called_with(1)

def test_resolve_gold_set_path_exists():
    with patch('pathlib.Path.exists', return_value=True), \
         patch('pathlib.Path.resolve', return_value=Path("/resolved/path.json")):
        assert resolve_gold_set_path("/some/path.json") == Path("/resolved/path.json")

def test_main_run_gold_set_not_found():
    with patch('app.evals.runner.cli.argparse.ArgumentParser.parse_args') as mock_args, \
         patch('app.evals.runner.cli.resolve_gold_set_path', return_value=None), \
         patch('sys.exit', side_effect=SystemExit(1)) as mock_exit:
        mock_args.return_value = MagicMock(command="run", dry_run=False, gold_set="invalid")
        
        from app.evals.runner.cli import main
        with pytest.raises(SystemExit):
            main()
        mock_exit.assert_called_with(1)

@pytest.mark.asyncio
async def test_run_full_eval_no_fleet_fail():
    with patch('app.evals.runner.cli.FleetManager') as mock_fleet_cls, \
         patch('sys.exit', side_effect=SystemExit(1)) as mock_exit:
        mock_fleet = mock_fleet_cls.return_value
        mock_fleet.is_running.return_value = False
        
        from app.evals.runner.cli import run_full_eval
        with pytest.raises(SystemExit):
            await run_full_eval(None, "/tmp/gold.json")
        mock_exit.assert_called_with(1)

@pytest.mark.asyncio
async def test_run_full_eval_all_providers():
    gold_set_content = [{"id": "s1", "user_query": "q1", "agent_mode": "OPERATOR_BOUND"}]
    providers = ["anthropic", "gemini", "ollama", "llamacpp", "g8el"]
    
    for provider in providers:
        with patch('app.evals.runner.cli.FleetManager'), \
             patch('app.evals.runner.cli.get_llm_provider'), \
             patch('app.evals.runner.cli.EvalJudge'), \
             patch('app.evals.runner.cli.run_scenario', new_callable=AsyncMock) as mock_run_scenario, \
             patch('app.evals.runner.cli.persist_report'), \
             patch('app.evals.runner.cli.render_text_table'), \
             patch('json.load', return_value=gold_set_content), \
             patch('builtins.open', MagicMock()):
            
            mock_run_scenario.return_value = EvalRow(
                dimension="accuracy",
                suite="suite",
                scenario_id="s1",
                passed=True,
                latency_ms=100.0
            )
            
            from app.evals.runner.cli import run_full_eval
            await run_full_eval(
                "token", "/tmp/gold.json", 
                llm_provider=provider, 
                llm_api_key="key",
                llm_primary_model="m1",
                llm_assistant_model="m2",
                llm_lite_model="m3"
            )

@pytest.mark.asyncio
async def test_run_full_eval_with_provider_settings():
    device_token = "token"
    gold_set_content = [{"id": "s1", "user_query": "q1", "agent_mode": "OPERATOR_BOUND"}]
    
    with patch('app.evals.runner.cli.FleetManager') as mock_fleet_cls, \
         patch('app.evals.runner.cli.get_llm_provider'), \
         patch('app.evals.runner.cli.EvalJudge'), \
         patch('app.evals.runner.cli.run_scenario', new_callable=AsyncMock) as mock_run_scenario, \
         patch('app.evals.runner.cli.persist_report'), \
         patch('app.evals.runner.cli.render_text_table'), \
         patch('json.load', return_value=gold_set_content), \
         patch('builtins.open', MagicMock()):
        
        mock_fleet = mock_fleet_cls.return_value
        mock_fleet.is_running.return_value = True
        mock_fleet.get_device_token.return_value = "token-from-fleet"
        
        mock_run_scenario.return_value = EvalRow(
            dimension="accuracy",
            suite="suite",
            scenario_id="s1",
            passed=True,
            latency_ms=100.0
        )
        
        await run_full_eval(
            None, "/tmp/gold.json", 
            llm_provider="openai", 
            llm_api_key="key",
            llm_endpoint="http://api.openai.com"
        )
        
        mock_run_scenario.assert_called_once()
        # Verify provider settings logic by checking if we hit the OpenAI branch (indirectly via coverage)

def test_main_list():
    with patch('app.evals.runner.cli.argparse.ArgumentParser.parse_args') as mock_args, \
         patch('app.evals.runner.cli.get_available_gold_sets') as mock_get, \
         patch('sys.exit') as mock_exit:
        mock_args.return_value = MagicMock(command="list")
        mock_get.return_value = {"benchmark": Path("/tmp/b.json")}
        
        from app.evals.runner.cli import main
        main()
        mock_exit.assert_called_with(0)

@pytest.mark.asyncio
async def test_run_full_eval():
    device_token = "token"
    gold_set_content = [
        {"id": "s1", "user_query": "q1", "agent_mode": "OPERATOR_BOUND", "expected_behavior": "b1"}
    ]
    
    with patch('app.evals.runner.cli.FleetManager') as mock_fleet_cls, \
         patch('app.evals.runner.cli.get_llm_provider'), \
         patch('app.evals.runner.cli.EvalJudge'), \
         patch('app.evals.runner.cli.run_scenario', new_callable=AsyncMock) as mock_run_scenario, \
         patch('app.evals.runner.cli.persist_report'), \
         patch('app.evals.runner.cli.render_text_table'), \
         patch('builtins.open', MagicMock()) as mock_open:
        
        mock_fleet = mock_fleet_cls.return_value
        mock_fleet.is_running.return_value = False
        mock_fleet.up = MagicMock()
        mock_fleet.wait_bound = AsyncMock()
        mock_fleet.down = MagicMock()
        
        # Mock file content
        mock_open.return_value.__enter__.return_value.read.return_value = json.dumps(gold_set_content)
        # We also need to mock json.load because it's used in run_full_eval
        with patch('json.load', return_value=gold_set_content):
            mock_run_scenario.return_value = EvalRow(
                dimension="accuracy",
                suite="suite",
                scenario_id="s1",
                passed=True,
                latency_ms=100.0
            )
            
            await run_full_eval(device_token, "/tmp/gold.json")
            
            mock_fleet.up.assert_called_once()
            mock_run_scenario.assert_called_once()
            mock_fleet.down.assert_called_once()

def test_main_run_full():
    with patch('app.evals.runner.cli.argparse.ArgumentParser.parse_args') as mock_args, \
         patch('app.evals.runner.cli.asyncio.run') as mock_asyncio_run, \
         patch('app.evals.runner.cli.resolve_gold_set_path') as mock_resolve:
        mock_args.return_value = MagicMock(
            command="run", 
            dry_run=False, 
            device_token="token", 
            gold_set="benchmark",
            g8ed_url="url",
            nodes=1,
            parallel=1,
            judge_model="m",
            llm_provider=None,
            primary_model=None,
            assistant_model=None,
            lite_model=None,
            llm_endpoint_url=None,
            llm_api_key=None
        )
        mock_resolve.return_value = Path("/tmp/b.json")
        
        def mock_run_se(coro):
            coro.close()
            return MagicMock()
        mock_asyncio_run.side_effect = mock_run_se
        
        from app.evals.runner.cli import main
        main()
        mock_asyncio_run.assert_called_once()

def test_main_run_dry_run():
    with patch('app.evals.runner.cli.argparse.ArgumentParser.parse_args') as mock_args, \
         patch('app.evals.runner.cli.asyncio.run') as mock_asyncio_run:
        mock_args.return_value = MagicMock(command="run", dry_run=True, device_token="token", g8ed_url="url")
        
        def mock_run_se(coro):
            coro.close()
            return MagicMock()
        mock_asyncio_run.side_effect = mock_run_se
        
        from app.evals.runner.cli import main
        main()
        mock_asyncio_run.assert_called_once()
