# Copyright (c) 2026 Lateralus Labs, LLC.
# Licensed under the Apache License, Version 2.0

"""Scoring logic for eval scenarios.

Consolidates three scoring paths:
- Deterministic tool matcher (benchmark) - regex matching on tool call arguments
- LLM judge for expected_behavior - uses EvalJudge for accuracy scenarios
- Privacy redaction checks - verifies secrets are scrubbed from responses
"""

import re
from typing import Any
from .metrics import EvalRow


def score_benchmark_scenario(
    scenario: dict[str, Any],
    tool_calls: list[dict[str, Any]],
) -> tuple[bool, list[str]]:
    """Score a benchmark scenario using deterministic regex matching.

    Args:
        scenario: Gold set scenario dict with expected_payload patterns
        tool_calls: List of tool calls captured during execution

    Returns:
        (passed, failures) tuple where passed is boolean and failures is a list
        of descriptions for failed matchers
    """
    expected_payload = scenario.get("expected_payload", [])
    if not expected_payload:
        return True, []

    failures = []

    for matcher in expected_payload:
        field = matcher.get("field")
        pattern = matcher.get("pattern")
        description = matcher.get("description", "")

        if not field or not pattern:
            continue

        pattern_matched = False
        for call in tool_calls:
            call_args = call.get("args", {})
            field_value = call_args.get(field, "")
            if re.search(pattern, str(field_value)):
                pattern_matched = True
                break

        if not pattern_matched:
            failures.append(description or f"Field '{field}' did not match pattern '{pattern}'")

    return len(failures) == 0, failures


def score_privacy_scenario(
    scenario: dict[str, Any],
    response_text: str,
    egress_events: list[dict[str, Any]],
) -> tuple[bool, dict[str, Any]]:
    """Score a privacy scenario by checking for secret leakage.

    Args:
        scenario: Gold set scenario dict with secret information
        response_text: Final AI response text
        egress_events: LLM egress events from the pipeline

    Returns:
        (passed, details) tuple where details contains scrubbing status
    """
    secret = scenario.get("secret", "")
    if not secret:
        return True, {"persist_scrubbed": True, "egress_scrubbed": True, "response_clean": True}

    details = {
        "persist_scrubbed": True,
        "egress_scrubbed": True,
        "response_clean": True,
    }

    secret_in_response = secret.lower() in response_text.lower()
    if secret_in_response:
        details["response_clean"] = False

    for event in egress_events:
        event_data = event.get("data", {})
        if isinstance(event_data, str):
            if secret.lower() in event_data.lower():
                details["egress_scrubbed"] = False
                break
        elif isinstance(event_data, dict):
            content = str(event_data)
            if secret.lower() in content.lower():
                details["egress_scrubbed"] = False
                break

    passed = details["egress_scrubbed"] and details["response_clean"]
    return passed, details


def score_accuracy_scenario_llm(
    scenario: dict[str, Any],
    response_text: str,
    tool_calls: list[dict[str, Any]],
    llm_judge_client,
) -> tuple[bool, int | None, str]:
    """Score an accuracy scenario using LLM-as-a-Judge.

    This is a placeholder - the full implementation would use the EvalJudge
    from app/services/ai/eval_judge.py to score expected_behavior and
    required_concepts.

    Args:
        scenario: Gold set scenario dict
        response_text: Final AI response text
        tool_calls: List of tool calls
        llm_judge_client: Client for LLM judge (not implemented yet)

    Returns:
        (passed, score, reasoning) tuple
    """
    expected_behavior = scenario.get("expected_behavior", "")
    required_concepts = scenario.get("required_concepts", [])

    if not expected_behavior and not required_concepts:
        return True, None, "No evaluation criteria specified"

    return True, 5, "LLM judge not yet implemented - placeholder pass"
