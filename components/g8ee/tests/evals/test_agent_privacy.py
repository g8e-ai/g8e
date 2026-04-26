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
Privacy Evaluation Test Suite for Sentinel PII Redaction.

Tests the three-layer Sentinel egress verification:
- L1: g8es persistence scrubbed (expected FAIL today - upstream fix pending)
- L2: LLM egress scrubbed (expected PASS - strict assert)
- L3: AI response echo-clean (expected PASS - strict assert)

Per-scenario flow:
1. Create investigation with sentinel_mode=True
2. Spy request_builder.build_contents_from_history and llm_provider.generate_content_* calls
3. Attach caplog handler on app.security.sentinel_scrubber
4. Run chat_pipeline.run_chat with sentinel_mode=True
5. Verify three layers independently
6. Emit EvalRow with per-layer details

TODO: Follow-up task to fix L1 leak by adding Sentinel scrubbing to g8es-write path in
chat_pipeline._run_chat_impl. Once fixed, flip L1 to strict assert.
"""

import json
import logging
import pytest
from datetime import datetime, timezone
from typing import Any
from unittest.mock import patch

from app.constants import EventType
from app.constants.paths import PATHS
from app.services.ai.chat_task_manager import ChatTaskManager
from app.models.http_context import G8eHttpContext
from app.models.investigations import InvestigationCreateRequest
from app.models.settings import G8eeUserSettings, SearchSettings

from tests.evals.metrics import EvalRow

logger = logging.getLogger(__name__)


def load_privacy_gold_set() -> list[dict[str, Any]]:
    """Load privacy gold set for parameterization."""
    with open(PATHS["g8ee"]["evals"]["privacy_gold_set_path"], "r", encoding="utf-8") as f:
        return json.load(f)


pytestmark = [
    pytest.mark.integration,
    pytest.mark.ai_integration,
    pytest.mark.agent_privacy,
    pytest.mark.slow,
    pytest.mark.timeout(180),
]


@pytest.mark.asyncio(loop_scope="session")
@pytest.mark.parametrize("scenario", load_privacy_gold_set(), ids=lambda s: s["id"])
async def test_agent_privacy(
    scenario: dict[str, Any],
    all_services,
    cache_aside_service,
    test_settings,
    user_settings,
    cleanup,
    unique_investigation_id,
    unique_case_id,
    unique_web_session_id,
    unique_user_id,
    caplog,
    unified_metrics_collector,
):
    """
    Test Sentinel PII redaction across three egress layers.

    L1 (g8es persistence): Report-only, expected False today due to upstream bug
    L2 (LLM egress): Strict assert - this is the existing guarantee
    L3 (AI response): Strict assert - a real leak here is a safety/privacy regression
    """
    start_time = datetime.now(timezone.utc)

    investigation_service = all_services['investigation_service']
    investigation_data_service = all_services['investigation_data_service']
    chat_pipeline = all_services['chat_pipeline']
    request_builder = all_services['request_builder']
    llm_provider = all_services['llm_provider']

    captured_request_builder_contents: list[list[Any]] = []
    captured_provider_contents: list[list[Any]] = []

    def spy_request_builder(*args, **kwargs):
        result = original_build_contents_from_history(*args, **kwargs)
        captured_request_builder_contents.append(result)
        return result

    original_build_contents_from_history = request_builder.build_contents_from_history

    async def spy_generate_content(*args, **kwargs):
        if 'contents' in kwargs:
            captured_provider_contents.append(kwargs['contents'])
        return await original_generate_content(*args, **kwargs)

    original_generate_content = llm_provider.generate_content_primary

    with patch.object(request_builder, 'build_contents_from_history', side_effect=spy_request_builder):
        with patch.object(llm_provider, 'generate_content_primary', side_effect=spy_generate_content):
            llm_settings = user_settings.llm
            user_query = scenario["user_query"]
            forbidden_tokens = scenario["forbidden_leak_tokens"]
            expected_placeholders = scenario["expected_placeholders"]
            expected_scrub_types = scenario["expected_scrub_types"]

            investigation_request = InvestigationCreateRequest(
                case_id=unique_case_id,
                case_title=f"Privacy Test: {scenario['id']}",
                case_description=scenario.get("description", "Privacy evaluation test"),
                user_id=unique_user_id,
                web_session_id=unique_web_session_id,
            )
            created_investigation = await investigation_data_service.create_investigation(investigation_request)
            logger.info(f"[PRIVACY] Created investigation {created_investigation.id} for scenario {scenario['id']}")
            cleanup.track_investigation(created_investigation.id)

            g8e_context = G8eHttpContext(
                web_session_id=unique_web_session_id,
                user_id=unique_user_id,
                case_id=unique_case_id,
                investigation_id=created_investigation.id,
                organization_id="org-priv-001",
                source_component="g8ee",
                bound_operators=[],
            )

            search_settings = SearchSettings(enabled=False)
            user_settings = G8eeUserSettings(llm=user_settings.llm, search=search_settings)
            task_manager = all_services['chat_task_manager']

            logger.info(f"[PRIVACY] Running scenario {scenario['id']} with sentinel_mode=True")

            with caplog.at_level(logging.INFO, logger="app.security.sentinel_scrubber"):
                await chat_pipeline.run_chat(
                    message=user_query,
                    g8e_context=g8e_context,
                    attachments=[],
                    sentinel_mode=True,
                    llm_primary_provider=None,
                    llm_assistant_provider=None,
                    llm_lite_provider=None,
                    llm_primary_model=llm_settings.primary_model,
                    llm_assistant_model=llm_settings.assistant_model,
                    llm_lite_model=llm_settings.lite_model,
                    _task_manager=task_manager,
                    user_settings=user_settings,
                    _track_task=False,
                )

            scrub_types_logged = []
            for record in caplog.records:
                if "Sentinel scrubbed" in record.message:
                    scrub_types_logged.append(record.message)

            conversation_history = await investigation_service.investigation_data_service.get_chat_messages(
                investigation_id=created_investigation.id
            )

            user_chat_messages = [msg for msg in conversation_history if msg.sender == EventType.EVENT_SOURCE_USER_CHAT]
            user_message_content = user_chat_messages[0].content if user_chat_messages else ""

            persist_scrubbed = (
                all(token not in user_message_content for token in forbidden_tokens) and
                any(placeholder in user_message_content for placeholder in expected_placeholders)
            )

            egress_scrubbed = False
            leaked_tokens_in_egress = []
            placeholders_found_in_egress = []

            if captured_provider_contents and len(captured_provider_contents) > 0:
                contents = captured_provider_contents[-1]
                contents_str = str(contents)
                egress_scrubbed = (
                    all(token not in contents_str for token in forbidden_tokens) and
                    all(placeholder in contents_str for placeholder in expected_placeholders)
                )
                leaked_tokens_in_egress = [token for token in forbidden_tokens if token in contents_str]
                placeholders_found_in_egress = [ph for ph in expected_placeholders if ph in contents_str]

            ai_response_text = ""
            for msg in reversed(conversation_history):
                if msg.sender == EventType.EVENT_SOURCE_AI_PRIMARY:
                    ai_response_text = msg.content
                    break

            response_clean = all(token not in ai_response_text for token in forbidden_tokens)
            leaked_tokens_in_response = [token for token in forbidden_tokens if token in ai_response_text]

            passed = egress_scrubbed and response_clean
            execution_time_ms = (datetime.now(timezone.utc) - start_time).total_seconds() * 1000

            unified_metrics_collector.add_row(EvalRow(
                dimension="privacy",
                suite="agent_privacy",
                scenario_id=scenario["id"],
                category=",".join(expected_scrub_types),
                passed=passed,
                score=None,
                latency_ms=execution_time_ms,
                error=None if passed else "Privacy leak detected",
                details={
                    "persist_scrubbed": persist_scrubbed,
                    "egress_scrubbed": egress_scrubbed,
                    "response_clean": response_clean,
                    "scrub_types_logged": scrub_types_logged,
                    "placeholders_found_in_egress": placeholders_found_in_egress,
                    "leaked_tokens_in_egress": leaked_tokens_in_egress,
                    "leaked_tokens_in_response": leaked_tokens_in_response,
                    "leaked_tokens_in_g8es": [token for token in forbidden_tokens if token in user_message_content],
                },
            ))

            logger.info(f"[PRIVACY] L1 persist_scrubbed={persist_scrubbed}")
            logger.info(f"[PRIVACY] L2 egress_scrubbed={egress_scrubbed}")
            logger.info(f"[PRIVACY] L3 response_clean={response_clean}")

            assert egress_scrubbed, (
                f"LLM egress leaked PII for {scenario['id']}: "
                f"leaked_tokens={leaked_tokens_in_egress}, "
                f"expected_placeholders={expected_placeholders}"
            )

            assert response_clean, (
                f"AI response leaked PII for {scenario['id']}: "
                f"leaked_tokens={leaked_tokens_in_response}"
            )

            logger.info(f"[PRIVACY] Scenario {scenario['id']} passed (L1={persist_scrubbed}, L2={egress_scrubbed}, L3={response_clean})")
