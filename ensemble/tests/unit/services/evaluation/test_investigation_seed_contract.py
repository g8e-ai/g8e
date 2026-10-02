# Copyright (c) 2026 Lateralus Labs, LLC.
# Use of this source code is governed by the Business Source License
# included in the LICENSE file.
#
# As of the Change Date listed in the LICENSE file, this software is
# released under the Apache License, Version 2.0.

"""Pins the seed contract the Go evaluation catalog is tested against.

internal/services/evaluation/scenario_seed_contract_test.go asserts that every
shipped scenario seed satisfies these literals. Pinning the same literals here
makes either side drifting fail a test on that side.
"""

from __future__ import annotations

import pytest
from g8e.models.internal_api import (
    EVALUATION_SEED_MAX_HISTORY_EVENTS,
    EVALUATION_SEED_MAX_TEXT,
    EVALUATION_SEED_MAX_TURNS,
    EvaluationInvestigationSeed,
    EvaluationSeedActor,
    EvaluationSeedSender,
)
from pydantic import ValidationError
from typing import get_args

from app.services.evaluation.investigation_seed import SEEDABLE_HISTORY_EVENTS

pytestmark = [pytest.mark.unit]


def test_seed_bounds_are_the_ones_the_go_catalog_is_checked_against():
    assert EVALUATION_SEED_MAX_TURNS == 16
    assert EVALUATION_SEED_MAX_HISTORY_EVENTS == 16
    assert EVALUATION_SEED_MAX_TEXT == 8000
    assert EvaluationInvestigationSeed.model_fields["case_title"].metadata[-1].max_length == 200


def test_seed_senders_and_actors_are_the_ones_the_go_catalog_is_checked_against():
    assert set(get_args(EvaluationSeedSender)) == {"user", "primary", "assistant"}
    assert set(get_args(EvaluationSeedActor)) == {"g8eo", "system", "user"}


def test_seedable_history_events_are_the_ones_the_go_catalog_is_checked_against():
    assert sorted(event.value for event in SEEDABLE_HISTORY_EVENTS) == [
        "g8e.v1.operator.command.approval.rejected",
        "g8e.v1.operator.command.execution.started",
        "g8e.v1.operator.command.failed",
        "g8e.v1.operator.file.edit.failed",
        "g8e.v1.operator.filesystem.grep.completed",
        "g8e.v1.operator.filesystem.grep.failed",
        "g8e.v1.operator.filesystem.read.completed",
        "g8e.v1.operator.filesystem.read.failed",
    ]


@pytest.mark.parametrize(
    "seed",
    [
        {"case_title": "Checkout payment timeouts", "turns": [{"sender": "user", "content": "x" * 8000}]},
        {
            "case_title": "t" * 200,
            "history_events": [
                {
                    "event_type": "g8e.v1.operator.filesystem.grep.failed",
                    "actor": "g8eo",
                    "summary": "s",
                }
            ],
        },
    ],
)
def test_seeds_at_the_pinned_bounds_validate(seed):
    EvaluationInvestigationSeed.model_validate(seed)


@pytest.mark.parametrize(
    "seed",
    [
        {"case_title": "t", "turns": [{"sender": "user", "content": "x" * 8001}]},
        {"case_title": "t" * 201},
        {"case_title": "t", "turns": [{"sender": "system", "content": "x"}]},
        {"case_title": "t", "turns": [{"sender": "user", "content": "x"}] * 17},
    ],
)
def test_seeds_past_the_pinned_bounds_are_rejected(seed):
    with pytest.raises(ValidationError):
        EvaluationInvestigationSeed.model_validate(seed)
