# Copyright (c) 2026 Lateralus Labs, LLC.
# Licensed under the Apache License, Version 2.0

import pytest
from unittest.mock import MagicMock, AsyncMock
from app.evals.runner.scorer import (
    score_benchmark_scenario,
    score_privacy_scenario,
    score_accuracy_scenario_llm,
)

def test_score_benchmark_scenario_pass():
    scenario = {
        "expected_payload": [
            {"field": "cmd", "pattern": "ls -la", "description": "List files"}
        ]
    }
    tool_calls = [
        {"args": {"cmd": "ls -la"}}
    ]
    passed, failures = score_benchmark_scenario(scenario, tool_calls)
    assert passed is True
    assert len(failures) == 0

def test_score_benchmark_scenario_fail():
    scenario = {
        "expected_payload": [
            {"field": "cmd", "pattern": "ls -la", "description": "List files"}
        ]
    }
    tool_calls = [
        {"args": {"cmd": "whoami"}}
    ]
    passed, failures = score_benchmark_scenario(scenario, tool_calls)
    assert passed is False
    assert "List files" in failures[0]

def test_score_benchmark_scenario_multiple_matchers():
    scenario = {
        "expected_payload": [
            {"field": "cmd", "pattern": "ls", "description": "ls command"},
            {"field": "target", "pattern": "/tmp", "description": "tmp directory"}
        ]
    }
    tool_calls = [
        {"args": {"cmd": "ls", "target": "/tmp"}}
    ]
    passed, failures = score_benchmark_scenario(scenario, tool_calls)
    assert passed is True

def test_score_benchmark_scenario_no_expected_payload():
    scenario = {}
    tool_calls = [{"args": {"cmd": "ls"}}]
    passed, failures = score_benchmark_scenario(scenario, tool_calls)
    assert passed is True
    assert len(failures) == 0

def test_score_privacy_scenario_clean():
    scenario = {"secret": "supersecretpassword"}
    response_text = "The task is done."
    egress_events = []
    passed, details = score_privacy_scenario(scenario, response_text, egress_events)
    assert passed is True
    assert details["response_clean"] is True
    assert details["egress_scrubbed"] is True

def test_score_privacy_scenario_leak_in_response():
    scenario = {"secret": "supersecretpassword"}
    response_text = "The password is supersecretpassword"
    egress_events = []
    passed, details = score_privacy_scenario(scenario, response_text, egress_events)
    assert passed is False
    assert details["response_clean"] is False

def test_score_privacy_scenario_leak_in_egress():
    scenario = {"secret": "supersecretpassword"}
    response_text = "The task is done."
    egress_events = [{"data": "Sending supersecretpassword to logs"}]
    passed, details = score_privacy_scenario(scenario, response_text, egress_events)
    assert passed is False
    assert details["egress_scrubbed"] is False

def test_score_privacy_scenario_leak_in_egress_dict():
    scenario = {"secret": "supersecretpassword"}
    response_text = "The task is done."
    egress_events = [{"data": {"message": "password is supersecretpassword"}}]
    passed, details = score_privacy_scenario(scenario, response_text, egress_events)
    assert passed is False
    assert details["egress_scrubbed"] is False

@pytest.mark.asyncio
async def test_score_accuracy_scenario_llm_pass():
    scenario = {
        "user_query": "hello",
        "expected_behavior": "Be helpful",
        "required_concepts": ["helpfulness"]
    }
    response_text = "I am helpful"
    tool_calls = []

    judge = MagicMock()
    judge.grade_turn = AsyncMock()
    grade = MagicMock()
    grade.passed = True
    grade.score = 5
    grade.reasoning = "Very helpful"
    judge.grade_turn.return_value = grade

    passed, score, reasoning = await score_accuracy_scenario_llm(
        scenario, response_text, tool_calls, judge
    )

    assert passed is True
    assert score == 5
    assert reasoning == "Very helpful"
    judge.grade_turn.assert_called_once()

def test_score_benchmark_scenario_missing_field():
    scenario = {
        "expected_payload": [
            {"field": "", "pattern": "ls", "description": "ls command"},
            {"field": "target", "pattern": "/tmp", "description": "tmp directory"}
        ]
    }
    tool_calls = [{"args": {"target": "/tmp"}}]
    passed, failures = score_benchmark_scenario(scenario, tool_calls)
    assert passed is True

def test_score_privacy_scenario_no_secret():
    scenario = {}
    response_text = "clean"
    egress_events = []
    passed, details = score_privacy_scenario(scenario, response_text, egress_events)
    assert passed is True
    assert details["response_clean"] is True

def test_score_privacy_scenario_leak_in_egress_non_dict():
    scenario = {"secret": "secret"}
    response_text = "clean"
    egress_events = [{"data": "leak secret here"}]
    passed, details = score_privacy_scenario(scenario, response_text, egress_events)
    assert passed is False
    assert details["egress_scrubbed"] is False

@pytest.mark.asyncio
async def test_score_accuracy_scenario_llm_no_criteria():
    scenario = {}
    response_text = "hello"
    tool_calls = []
    judge = MagicMock()

    passed, score, reasoning = await score_accuracy_scenario_llm(
        scenario, response_text, tool_calls, judge
    )

    assert passed is True
    assert score is None
    assert "No evaluation criteria specified" in reasoning

@pytest.mark.asyncio
async def test_score_accuracy_scenario_llm_with_tools():
    scenario = {
        "user_query": "hello",
        "expected_behavior": "Be helpful",
    }
    response_text = "I am helpful"
    tool_calls = [
        {"name": "test_tool", "args": {"a": 1}},
        {"tool_name": "other_tool"}
    ]

    judge = MagicMock()
    judge.grade_turn = AsyncMock()
    grade = MagicMock()
    grade.passed = True
    grade.score = 5
    grade.reasoning = "OK"
    judge.grade_turn.return_value = grade

    await score_accuracy_scenario_llm(scenario, response_text, tool_calls, judge)

    args, kwargs = judge.grade_turn.call_args
    assert "TOOL_CALL: test_tool args={'a': 1}" in kwargs["interaction_trace"]
    assert "TOOL_CALL: other_tool args={}" in kwargs["interaction_trace"]
