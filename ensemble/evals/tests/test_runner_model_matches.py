# Copyright (c) 2026 Lateralus Labs, LLC.
# Use of this source code is governed by the Business Source License
# included in the LICENSE file.
#
# As of the Change Date listed in the LICENSE file, this software is
# released under the Apache License, Version 2.0.

"""Tier 1 unit tests for the ``_model_matches`` cohort-drift helper.

Regression tests for the false positive where ``DirectProviderSUT``
reports ``model`` as ``"ollama:qwen3:8b"`` (the ``provider:model_tag``
format) while the cohort declares only the model tag
(e.g. ``"qwen3:8b"``). The old exact-string comparison always failed,
marking every completed attempt as ``model_failed`` with cohort drift.
The fix accepts either an exact match or a ``provider:model`` prefix
match.
"""

from __future__ import annotations

import pytest

from g8e_evals.runner import _model_matches

pytestmark = [pytest.mark.unit]


class TestModelMatchesExact:
    def test_exact_match_returns_true(self):
        assert _model_matches("qwen3:8b", "qwen3:8b") is True

    def test_exact_match_different_model_returns_false(self):
        assert _model_matches("granite3.3:8b", "qwen3:8b") is False

    def test_exact_match_no_colon_returns_true(self):
        assert _model_matches("model", "model") is True


class TestModelMatchesProviderPrefix:
    def test_provider_prefix_match_returns_true(self):
        """The DirectProviderSUT format 'ollama:qwen3:8b' matches 'qwen3:8b'."""
        assert _model_matches("ollama:qwen3:8b", "qwen3:8b") is True

    def test_provider_prefix_different_model_returns_false(self):
        assert _model_matches("ollama:granite3.3:8b", "qwen3:8b") is False

    def test_provider_prefix_with_different_provider_returns_true(self):
        """Any provider prefix is accepted as long as the tail matches."""
        assert _model_matches("vllm:qwen3:8b", "qwen3:8b") is True

    def test_provider_prefix_empty_tail_returns_false(self):
        assert _model_matches("ollama:", "qwen3:8b") is False


class TestModelMatchesNoMatch:
    def test_completely_different_returns_false(self):
        assert _model_matches("wrong-model:7b", "qwen3:8b") is False

    def test_provider_prefix_wrong_tail_returns_false(self):
        """Regression: 'ollama:wrong-model:7b' must not match 'qwen3:8b'."""
        assert _model_matches("ollama:wrong-model:7b", "qwen3:8b") is False

    def test_empty_observed_returns_false(self):
        assert _model_matches("", "qwen3:8b") is False

    def test_empty_expected_returns_false(self):
        assert _model_matches("ollama:qwen3:8b", "") is False


class TestModelMatchesRegression:
    def test_granite_provider_prefix_matches_tag(self):
        """Regression: 'ollama:granite3.3:8b' must match 'granite3.3:8b'.

        The old exact-string comparison failed because the observed
        string has the 'ollama:' prefix. This caused every Granite
        cohort attempt to be marked as model_failed with cohort drift.
        """
        assert _model_matches("ollama:granite3.3:8b", "granite3.3:8b") is True

    def test_qwen_provider_prefix_matches_tag(self):
        """Regression: 'ollama:qwen3:8b' must match 'qwen3:8b'."""
        assert _model_matches("ollama:qwen3:8b", "qwen3:8b") is True
