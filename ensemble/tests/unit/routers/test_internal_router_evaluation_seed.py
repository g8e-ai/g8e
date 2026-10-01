# Copyright (c) 2026 Lateralus Labs, LLC.
# Use of this source code is governed by the Business Source License
# included in the LICENSE file.
#
# As of the Change Date listed in the LICENSE file, this software is
# released under the Apache License, Version 2.0.

"""internal_chat's handling of a scored request's investigation seed.

A seed is accepted only when the request creates the case, is written
synchronously right after inline case creation and before the turn runs, and
replaces prompt-derived title generation. A production request never touches
any of this.
"""

from unittest.mock import AsyncMock, MagicMock, patch

import pytest
from g8e.models.internal_api import (
    EvaluationInferenceContext,
    EvaluationInvestigationSeed,
    EvaluationSeedTurn,
    InferenceModelVariant,
)

from app.constants import ComponentName
from app.errors import ValidationError
from app.models.evaluation_trace import EvaluationSeedApplication, ToolGate
from app.models.http_context import G8eHttpContext, RequestContext
from app.models.internal_api import ChatMessageRequest, ResourceCreationRequest
from app.routers.internal_router import internal_chat
from tests.fakes.factories import build_case_model, create_investigation_data

pytestmark = pytest.mark.unit

SEED = EvaluationInvestigationSeed(
    case_title="Checkout payment timeouts",
    case_description="Card payments time out during deploys.",
    turns=[EvaluationSeedTurn(sender="user", content="Checkout is failing.")],
)


def _evaluation_context(seed: EvaluationInvestigationSeed | None) -> EvaluationInferenceContext:
    return EvaluationInferenceContext(
        campaign_id="campaign-1",
        run_id="run-1",
        assignment_id="assignment-1",
        evaluation_attempt_id="attempt-1",
        scenario_id="scenario-1",
        model_registry_digest="d" * 64,
        model_registry=[InferenceModelVariant(model="model-a", digest="a" * 64)],
        target_operator_session_id="session-1",
        seed=seed,
    )


def _request(
    *, evaluation_context: EvaluationInferenceContext | None, create_case: bool
) -> ChatMessageRequest:
    return ChatMessageRequest(
        context=RequestContext(
            user_id="user-123",
            web_session_id="session-123",
            organization_id="org-123",
            source_component=ComponentName.CLIENT,
        ),
        message="Search for AUTH_FAILURE.",
        sentinel_mode=True,
        resource_creation=ResourceCreationRequest(create_case=True) if create_case else None,
        evaluation_context=evaluation_context,
    )


class _Router:
    """The router's collaborators, recording the order they are called in."""

    def __init__(self, *, case_description: str = "derived from the prompt") -> None:
        self.calls: list[str] = []
        self.pipeline = MagicMock()
        self.pipeline.run_chat = MagicMock(side_effect=lambda **kw: self.calls.append("run_chat"))
        self.cases = MagicMock()
        self.cases.create_case = AsyncMock(
            side_effect=lambda *a, **kw: self._created(
                "create_case",
                build_case_model(
                    case_id="case-123", user_id="user-123", description=case_description
                ),
            )
        )
        self.cases.update_case = AsyncMock()
        self.cases.publish_case_update_sse = AsyncMock()
        self.investigations = MagicMock()
        self.investigations.create_investigation = AsyncMock(
            side_effect=lambda request: self._created(
                "create_investigation",
                create_investigation_data(investigation_id="inv-123", case_id="case-123"),
            )
        )
        self.seed_service = MagicMock()
        self.seed_service.apply = AsyncMock(
            side_effect=lambda seed, context: self._created(
                "seed_apply", EvaluationSeedApplication(turns=len(seed.turns))
            )
        )

    def _created(self, name: str, value):
        self.calls.append(name)
        return value

    async def chat(self, request: ChatMessageRequest, task_tracker):
        with task_tracker.patch_create_task("app.routers.internal_router"):
            return await internal_chat(
                request=request,
                app_settings=MagicMock(),
                user_settings=MagicMock(),
                chat_pipeline=self.pipeline,
                chat_task_manager=MagicMock(),
                case_service=self.cases,
                investigation_service=self.investigations,
                attachment_service=MagicMock(),
                event_service=MagicMock(),
                g8e_context=G8eHttpContext(
                    user_id="user-123",
                    web_session_id="session-123",
                    organization_id="org-123",
                    source_component=ComponentName.CLIENT,
                ),
                settings_service=AsyncMock(),
                seed_service=self.seed_service,
            )


async def test_a_seed_without_create_case_is_rejected_with_400_before_anything_is_written(
    task_tracker,
):
    router = _Router()
    request = _request(evaluation_context=_evaluation_context(SEED), create_case=False)

    with pytest.raises(ValidationError) as rejected:
        await router.chat(request, task_tracker)

    assert rejected.value.get_http_status() == 400
    assert rejected.value.error_detail.details["field"] == "evaluation_context.seed"
    assert router.calls == []
    router.pipeline.evaluation_trace_service.begin.assert_not_called()
    router.seed_service.apply.assert_not_awaited()


async def test_a_seeded_request_applies_the_seed_after_case_creation_and_before_the_turn_runs(
    task_tracker,
):
    router = _Router()
    request = _request(evaluation_context=_evaluation_context(SEED), create_case=True)

    response = await router.chat(request, task_tracker)

    assert response.success is True
    assert response.case_id == "case-123"
    assert response.investigation_id == "inv-123"
    assert router.calls == ["create_case", "create_investigation", "seed_apply", "run_chat"]
    applied_seed, applied_context = router.seed_service.apply.await_args.args
    assert applied_seed == SEED
    assert applied_context.case_id == "case-123"
    assert applied_context.investigation_id == "inv-123"


async def test_the_seed_application_reaches_the_turn_so_the_trace_can_record_it(task_tracker):
    router = _Router()
    request = _request(evaluation_context=_evaluation_context(SEED), create_case=True)

    await router.chat(request, task_tracker)

    run_kwargs = router.pipeline.run_chat.call_args.kwargs
    assert run_kwargs["seed_application"] == EvaluationSeedApplication(turns=1)


async def test_a_seeded_request_keeps_its_scenario_title_by_skipping_title_generation(
    task_tracker,
):
    router = _Router()
    request = _request(evaluation_context=_evaluation_context(SEED), create_case=True)

    with patch(
        "app.routers.internal_router._generate_and_update_title", new_callable=AsyncMock
    ) as generate_title:
        await router.chat(request, task_tracker)

    generate_title.assert_not_called()


@pytest.mark.parametrize(
    ("seed", "expected_description"),
    [
        (SEED, "Card payments time out during deploys."),
        (EvaluationInvestigationSeed(case_title="Retry tuning"), "derived from the prompt"),
    ],
)
async def test_the_investigation_inherits_the_seed_description_when_the_seed_has_one(
    task_tracker, seed, expected_description
):
    router = _Router(case_description="derived from the prompt")
    request = _request(evaluation_context=_evaluation_context(seed), create_case=True)

    await router.chat(request, task_tracker)

    created = router.investigations.create_investigation.await_args.args[0]
    assert created.case_description == expected_description


async def test_a_scored_request_without_a_seed_still_gets_a_generated_title(task_tracker):
    router = _Router()
    request = _request(evaluation_context=_evaluation_context(None), create_case=True)

    with patch(
        "app.routers.internal_router._generate_and_update_title", new_callable=AsyncMock
    ) as generate_title:
        await router.chat(request, task_tracker)

    generate_title.assert_called_once()
    router.seed_service.apply.assert_not_awaited()
    assert router.pipeline.run_chat.call_args.kwargs["seed_application"] is None


async def test_a_production_request_never_touches_the_seed_service(task_tracker):
    router = _Router()
    request = _request(evaluation_context=None, create_case=True)

    with patch(
        "app.routers.internal_router._generate_and_update_title", new_callable=AsyncMock
    ) as generate_title:
        response = await router.chat(request, task_tracker)

    assert response.success is True
    router.seed_service.apply.assert_not_awaited()
    router.pipeline.evaluation_trace_service.begin.assert_not_called()
    generate_title.assert_called_once()
    assert router.pipeline.run_chat.call_args.kwargs["seed_application"] is None


async def test_a_failed_seed_closes_the_trace_and_surfaces_the_error_without_running_the_turn(
    task_tracker,
):
    router = _Router()
    router.seed_service.apply = AsyncMock(side_effect=RuntimeError("boom"))
    request = _request(evaluation_context=_evaluation_context(SEED), create_case=True)

    with pytest.raises(RuntimeError, match="boom"):
        await router.chat(request, task_tracker)

    trace_service = router.pipeline.evaluation_trace_service
    trace_service.begin.assert_called_once()
    trace_service.finalize_crashed.assert_called_once()
    _, kwargs = trace_service.finalize_crashed.call_args
    assert kwargs["error"] == "investigation seed failed: boom"
    assert kwargs["tool_gate"] is ToolGate.BYPASSED_FOR_EVAL
    assert "run_chat" not in router.calls
