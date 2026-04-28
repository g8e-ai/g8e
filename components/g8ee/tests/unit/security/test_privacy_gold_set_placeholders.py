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

"""Regression test: the privacy eval gold set must stay in lockstep with the
SentinelScrubber implementation.

The host-driven privacy evals in `components/g8ee/evals/gold_sets/privacy.json`
declare two contracts per scenario:

  - ``expected_scrub_types``: scrubber names (matching ``RegexScrubber.name``)
    that must fire when the scenario's ``user_query`` is processed.
  - ``expected_placeholders``: the literal replacement strings (e.g. ``[EMAIL]``)
    that must appear in the scrubbed output.

If anyone renames a scrubber, removes a replacement literal, or tweaks a regex
in ``app/security/sentinel_scrubber.py``, this test catches the drift before a
live eval run silently passes against an empty contract.
"""

from __future__ import annotations

import json
from pathlib import Path

import pytest

from app.security.sentinel_scrubber import (
    SentinelConfig,
    SentinelScrubber,
)

pytestmark = [pytest.mark.unit]

_GOLD_SET = (
    Path(__file__).resolve().parents[3]
    / "evals"
    / "gold_sets"
    / "privacy.json"
)


def _load_scenarios() -> list[dict]:
    with open(_GOLD_SET, encoding="utf-8") as f:
        scenarios = json.load(f)
    assert isinstance(scenarios, list) and scenarios, (
        f"Privacy gold set is missing or empty: {_GOLD_SET}"
    )
    return scenarios


@pytest.fixture(scope="module")
def scrubber() -> SentinelScrubber:
    return SentinelScrubber(SentinelConfig(log_scrubs=False))


@pytest.fixture(scope="module")
def scrubber_names() -> set[str]:
    return {s.name for s in SentinelScrubber._scrubbers}


@pytest.fixture(scope="module")
def replacement_literals() -> set[str]:
    return {s.replacement for s in SentinelScrubber._scrubbers}


@pytest.mark.parametrize("scenario", _load_scenarios(), ids=lambda s: s["id"])
class TestPrivacyGoldSet:
    def test_expected_placeholders_are_known_replacements(
        self, scenario: dict, replacement_literals: set[str]
    ) -> None:
        """Every declared placeholder must be a literal produced by some scrubber."""
        for placeholder in scenario.get("expected_placeholders", []):
            assert placeholder in replacement_literals, (
                f"Scenario {scenario['id']} declares placeholder {placeholder!r} "
                f"which is not a replacement literal in sentinel_scrubber.py. "
                f"Known literals: {sorted(replacement_literals)}"
            )

    def test_expected_scrub_types_are_known_scrubbers(
        self, scenario: dict, scrubber_names: set[str]
    ) -> None:
        """Every declared scrub type must correspond to a registered scrubber name."""
        for scrub_type in scenario.get("expected_scrub_types", []):
            assert scrub_type in scrubber_names, (
                f"Scenario {scenario['id']} declares scrub_type {scrub_type!r} "
                f"which is not a registered RegexScrubber name. "
                f"Known names: {sorted(scrubber_names)}"
            )

    def test_scrubbing_user_query_produces_expected_placeholders(
        self, scenario: dict, scrubber: SentinelScrubber
    ) -> None:
        """Running the scrubber on ``user_query`` must emit every expected placeholder."""
        result = scrubber.scrub(scenario["user_query"])
        for placeholder in scenario.get("expected_placeholders", []):
            assert placeholder in result.scrubbed_text, (
                f"Scenario {scenario['id']}: scrubber failed to emit "
                f"{placeholder!r}. Scrubbed text: {result.scrubbed_text!r}. "
                f"Scrub types fired: {result.scrub_types}"
            )

    def test_scrubbing_user_query_fires_expected_scrub_types(
        self, scenario: dict, scrubber: SentinelScrubber
    ) -> None:
        """Each expected scrub type must actually fire when scrubbing the query."""
        result = scrubber.scrub(scenario["user_query"])
        for scrub_type in scenario.get("expected_scrub_types", []):
            assert scrub_type in result.scrub_types, (
                f"Scenario {scenario['id']}: scrubber {scrub_type!r} did not "
                f"fire on query. Fired scrubbers: {result.scrub_types}. "
                f"Scrubbed text: {result.scrubbed_text!r}"
            )

    def test_forbidden_leak_tokens_not_in_scrubbed_output(
        self, scenario: dict, scrubber: SentinelScrubber
    ) -> None:
        """No raw secret/PII value may survive in the scrubbed output."""
        result = scrubber.scrub(scenario["user_query"])
        for forbidden in scenario.get("forbidden_leak_tokens", []):
            assert forbidden not in result.scrubbed_text, (
                f"Scenario {scenario['id']}: forbidden token {forbidden!r} "
                f"leaked through scrubber. Scrubbed text: "
                f"{result.scrubbed_text!r}"
            )
