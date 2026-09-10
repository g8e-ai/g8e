# Copyright (c) 2026 Lateralus Labs, LLC.
# Use of this source code is governed by the Business Source License
# included in the LICENSE file.
#
# As of the Change Date listed in the LICENSE file, this software is
# released under the Apache License, Version 2.0.

"""Tier 1 unit tests for cohort-to-model-tag derivation in the campaign CLI.

Regression tests for the model-tag derivation bug where
``cohort_id.replace("cohort-", "").replace("-", ":")`` replaced every
hyphen, converting ``cohort-granite-3.3-8b`` to ``granite:3.3:8b``
instead of the correct Ollama tag ``granite3.3:8b``. The fix uses
``rpartition("-")`` so only the last hyphen becomes the colon
separator, and adds an explicit ``--model-tags`` JSON file override.
"""

from __future__ import annotations

import pytest

from g8e_evals.cli import _derive_model_tag

pytestmark = [pytest.mark.unit]


class TestDeriveModelTagExplicitMapping:
    def test_explicit_mapping_takes_precedence(self):
        """The --model-tags file overrides the heuristic for every cohort."""
        tag_map = {"cohort-granite-3.3-8b": "granite3.3:8b"}
        assert _derive_model_tag("cohort-granite-3.3-8b", tag_map) == "granite3.3:8b"

    def test_explicit_mapping_for_simple_cohort(self):
        tag_map = {"cohort-qwen3-8b": "qwen3:8b"}
        assert _derive_model_tag("cohort-qwen3-8b", tag_map) == "qwen3:8b"

    def test_explicit_mapping_empty_map_falls_back_to_heuristic(self):
        assert _derive_model_tag("cohort-qwen3-8b", {}) == "qwen3:8b"


class TestDeriveModelTagHeuristic:
    def test_single_hyphen_cohort_id(self):
        """cohort-qwen3-8b → qwen3:8b (last hyphen becomes colon)."""
        assert _derive_model_tag("cohort-qwen3-8b", {}) == "qwen3:8b"

    def test_multi_hyphen_cohort_id_only_last_hyphen_replaced(self):
        """cohort-granite-3.3-8b → granite-3.3:8b (not granite:3.3:8b).

        The old code replaced every hyphen, producing the invalid tag
        ``granite:3.3:8b``. The fix uses ``rpartition("-")`` so only the
        last hyphen becomes the colon. When the derived tag does not
        match the backend's served tag, the operator must supply an
        explicit ``--model-tags`` mapping.
        """
        result = _derive_model_tag("cohort-granite-3.3-8b", {})
        assert result == "granite-3.3:8b"
        assert ":" not in result.split(":")[0], "only one colon separator expected"

    def test_no_hyphen_after_prefix(self):
        """cohort-model → model (no colon when no hyphen remains)."""
        assert _derive_model_tag("cohort-model", {}) == "model"

    def test_prefix_without_cohort_dash(self):
        """A cohort ID without the 'cohort-' prefix is handled gracefully."""
        assert _derive_model_tag("qwen3-8b", {}) == "qwen3:8b"

    def test_cohort_id_with_no_hyphen_at_all(self):
        """A bare cohort ID with no hyphen returns itself."""
        assert _derive_model_tag("cohortmodel", {}) == "cohortmodel"


class TestDeriveModelTagRegression:
    def test_granite_tag_not_over_colonized(self):
        """Regression: the old replace('-', ':') produced granite:3.3:8b.

        The new rpartition('-') heuristic must never produce a tag with
        more than one colon from the fallback path.
        """
        result = _derive_model_tag("cohort-granite-3.3-8b", {})
        assert result.count(":") <= 1

    def test_explicit_mapping_corrects_granite_tag(self):
        """The --model-tags file supplies the exact correct Ollama tag
        for multi-hyphen cohort IDs where the heuristic is insufficient."""
        tag_map = {"cohort-granite-3.3-8b": "granite3.3:8b"}
        result = _derive_model_tag("cohort-granite-3.3-8b", tag_map)
        assert result == "granite3.3:8b"
        assert ":" not in result.replace("granite3.3:8b", "", 1)
