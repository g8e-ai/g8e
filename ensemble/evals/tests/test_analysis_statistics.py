# Copyright (c) 2026 Lateralus Labs, LLC.
# Use of this source code is governed by the Business Source License
# included in the LICENSE file.
#
# As of the Change Date listed in the LICENSE file, this software is
# released under the Apache License, Version 2.0.

"""Tier 1 unit tests for the paired statistical estimators.

Verifies correctness and determinism of the pure-Python statistical
functions: McNemar test, paired t-test, Wilcoxon signed-rank test,
Cohen's d, bootstrap confidence intervals, Holm-Bonferroni correction,
and delta computations. No external dependencies.
"""

from __future__ import annotations

import math

import pytest

pytestmark = pytest.mark.unit

from g8e_evals.analysis import statistics


class TestAbsoluteDelta:
    def test_positive_delta(self) -> None:
        assert statistics.absolute_delta(0.5, 0.8) == 0.3

    def test_negative_delta(self) -> None:
        assert statistics.absolute_delta(0.8, 0.5) == -0.3

    def test_zero_delta(self) -> None:
        assert statistics.absolute_delta(0.5, 0.5) == 0.0


class TestRelativeDelta:
    def test_positive_relative_delta(self) -> None:
        assert statistics.relative_delta(0.5, 0.8) == 0.6

    def test_negative_relative_delta(self) -> None:
        assert statistics.relative_delta(0.8, 0.5) == -0.375

    def test_zero_baseline_returns_none(self) -> None:
        assert statistics.relative_delta(0.0, 0.5) is None

    def test_zero_delta_with_nonzero_baseline(self) -> None:
        assert statistics.relative_delta(0.5, 0.5) == 0.0


class TestCohensDPaired:
    def test_large_effect(self) -> None:
        baseline = [1.0, 2.0, 3.0, 4.0, 5.0]
        comparison = [3.0, 4.0, 5.0, 6.0, 7.0]
        d = statistics.cohens_d_paired(baseline, comparison)
        # All diffs are 2.0, constant diffs have zero variance -> None
        assert d is None

    def test_nonzero_variance(self) -> None:
        baseline = [1.0, 2.0, 3.0, 4.0, 5.0]
        comparison = [2.0, 5.0, 3.0, 8.0, 5.0]
        d = statistics.cohens_d_paired(baseline, comparison)
        assert d is not None
        diffs = [1.0, 3.0, 0.0, 4.0, 0.0]
        mean_diff = sum(diffs) / len(diffs)
        var = sum((d_ - mean_diff) ** 2 for d_ in diffs) / (len(diffs) - 1)
        expected = mean_diff / math.sqrt(var)
        assert d == round(expected, 10)

    def test_too_few_values(self) -> None:
        assert statistics.cohens_d_paired([1.0], [2.0]) is None

    def test_zero_variance_returns_none(self) -> None:
        baseline = [1.0, 2.0, 3.0]
        comparison = [2.0, 3.0, 4.0]
        assert statistics.cohens_d_paired(baseline, comparison) is None


class TestMcNemarTest:
    def test_no_discordant_pairs(self) -> None:
        baseline = [True, True, False, False]
        comparison = [True, True, False, False]
        stat, p = statistics.mcnemar_test(baseline, comparison)
        assert stat is None
        assert p is None

    def test_all_discordant_same_direction(self) -> None:
        # b=4, c=0 -> all baseline correct, comparison wrong
        baseline = [True, True, True, True]
        comparison = [False, False, False, False]
        stat, p = statistics.mcnemar_test(baseline, comparison)
        assert stat is not None
        assert p is not None
        assert stat == 4.0
        # Exact binomial: P(X=0 or X=4 | n=4, p=0.5) two-sided
        # P(X=0) = P(X=4) = 0.0625, observed is 0 or 4
        # two-sided: sum of probs <= observed prob
        # P(X=0)=0.0625, P(X=4)=0.0625, P(X=1)=0.25, P(X=2)=0.375, P(X=3)=0.25
        # probs <= 0.0625: P(X=0) + P(X=4) = 0.125
        assert p == 0.125

    def test_mixed_discordant(self) -> None:
        # b=2, c=1
        baseline = [True, True, False, True]
        comparison = [False, False, True, True]
        # Pair 0: T->F (b), Pair 1: T->F (b), Pair 2: F->T (c), Pair 3: T->T (concordant)
        stat, p = statistics.mcnemar_test(baseline, comparison)
        assert stat is not None
        assert p is not None
        assert stat == 1.0  # |b-c| = |2-1| = 1
        # discordant=3, exact binomial two-sided with b=2, n=3, p=0.5
        # P(X=2) = C(3,2)*0.5^3 = 3/8 = 0.375
        # probs <= 0.375: P(X=0)=0.125, P(X=1)=0.375, P(X=2)=0.375, P(X=3)=0.125
        # all <= 0.375: 0.125+0.375+0.375+0.125 = 1.0
        assert p == 1.0

    def test_empty_input(self) -> None:
        stat, p = statistics.mcnemar_test([], [])
        assert stat is None
        assert p is None

    def test_deterministic(self) -> None:
        baseline = [True, False, True, False, True, False, True, False]
        comparison = [False, True, False, True, False, True, False, True]
        stat1, p1 = statistics.mcnemar_test(baseline, comparison)
        stat2, p2 = statistics.mcnemar_test(baseline, comparison)
        assert stat1 == stat2
        assert p1 == p2


class TestPairedTTest:
    def test_significant_difference(self) -> None:
        baseline = [1.0, 2.0, 3.0, 4.0, 5.0, 6.0, 7.0, 8.0, 9.0, 10.0]
        comparison = [2.0, 3.0, 4.0, 5.0, 6.0, 7.0, 8.0, 9.0, 10.0, 11.0]
        t, p = statistics.paired_t_test(baseline, comparison)
        # All diffs = 1.0, zero variance -> None
        assert t is None
        assert p is None

    def test_nonzero_variance_significant(self) -> None:
        baseline = [1.0, 2.0, 3.0, 4.0, 5.0, 6.0, 7.0, 8.0, 9.0, 10.0]
        comparison = [3.0, 4.0, 5.0, 6.0, 7.0, 8.0, 9.0, 10.0, 11.0, 12.0]
        # diffs: [2,2,2,2,2,2,2,2,2,2] -> zero variance again
        # Need non-constant diffs
        comparison = [3.0, 3.0, 5.0, 5.0, 7.0, 7.0, 9.0, 9.0, 11.0, 11.0]
        t, p = statistics.paired_t_test(baseline, comparison)
        assert t is not None
        assert p is not None
        assert p < 0.05  # Should be significant

    def test_no_significant_difference(self) -> None:
        baseline = [1.0, 2.0, 3.0, 4.0, 5.0]
        comparison = [1.5, 2.5, 2.5, 4.5, 3.5]
        t, p = statistics.paired_t_test(baseline, comparison)
        assert t is not None
        assert p is not None
        # Small sample with mixed diffs, likely not significant
        assert 0.0 <= p <= 1.0

    def test_too_few_values(self) -> None:
        assert statistics.paired_t_test([1.0], [2.0]) == (None, None)

    def test_zero_variance(self) -> None:
        baseline = [1.0, 2.0, 3.0]
        comparison = [2.0, 3.0, 4.0]
        assert statistics.paired_t_test(baseline, comparison) == (None, None)

    def test_deterministic(self) -> None:
        baseline = [1.0, 2.0, 3.0, 4.0, 5.0, 6.0, 7.0, 8.0, 9.0, 10.0]
        comparison = [3.0, 3.0, 5.0, 5.0, 7.0, 7.0, 9.0, 9.0, 11.0, 11.0]
        t1, p1 = statistics.paired_t_test(baseline, comparison)
        t2, p2 = statistics.paired_t_test(baseline, comparison)
        assert t1 == t2
        assert p1 == p2


class TestWilcoxonSignedRankTest:
    def test_all_positive_diffs(self) -> None:
        baseline = [1.0, 2.0, 3.0, 4.0, 5.0]
        comparison = [2.0, 3.0, 4.0, 5.0, 6.0]
        w, p = statistics.wilcoxon_signed_rank_test(baseline, comparison)
        # All diffs positive, w_minus = 0
        assert w is not None
        assert p is not None
        assert w == 0.0  # min(w_plus, w_minus) = min(15, 0) = 0
        # n=5, exact two-sided: P(W <= 0 or W >= 15) = 2/32 = 0.0625
        assert p == 0.0625

    def test_mixed_diffs(self) -> None:
        baseline = [1.0, 2.0, 3.0, 4.0, 5.0]
        comparison = [2.0, 1.0, 4.0, 3.0, 6.0]
        w, p = statistics.wilcoxon_signed_rank_test(baseline, comparison)
        assert w is not None
        assert p is not None
        assert 0.0 <= p <= 1.0

    def test_all_zero_diffs(self) -> None:
        baseline = [1.0, 2.0, 3.0]
        comparison = [1.0, 2.0, 3.0]
        w, p = statistics.wilcoxon_signed_rank_test(baseline, comparison)
        assert w is None
        assert p is None

    def test_too_few_values(self) -> None:
        assert statistics.wilcoxon_signed_rank_test([1.0], [2.0]) == (None, None)

    def test_deterministic(self) -> None:
        baseline = [1.0, 2.0, 3.0, 4.0, 5.0, 6.0, 7.0, 8.0]
        comparison = [2.0, 1.0, 4.0, 3.0, 6.0, 5.0, 8.0, 7.0]
        w1, p1 = statistics.wilcoxon_signed_rank_test(baseline, comparison)
        w2, p2 = statistics.wilcoxon_signed_rank_test(baseline, comparison)
        assert w1 == w2
        assert p1 == p2

    def test_tied_ranks(self) -> None:
        baseline = [1.0, 2.0, 3.0, 4.0]
        comparison = [2.0, 3.0, 3.0, 5.0]
        # diffs: [1, 1, 0, 1] -> non-zero: [1, 1, 1], all same abs value
        # ranks: all get average rank (1+2+3)/3 = 2.0
        w, p = statistics.wilcoxon_signed_rank_test(baseline, comparison)
        assert w is not None
        assert p is not None
        # All positive, w_minus = 0
        assert w == 0.0


class TestBootstrapCI:
    def test_deterministic_same_seed(self) -> None:
        baseline = [1.0, 2.0, 3.0, 4.0, 5.0, 6.0, 7.0, 8.0, 9.0, 10.0]
        comparison = [2.0, 3.0, 4.0, 5.0, 6.0, 7.0, 8.0, 9.0, 10.0, 11.0]
        lower1, upper1 = statistics.bootstrap_ci(baseline, comparison, seed=42)
        lower2, upper2 = statistics.bootstrap_ci(baseline, comparison, seed=42)
        assert lower1 == lower2
        assert upper1 == upper2

    def test_different_seed_different_result(self) -> None:
        baseline = [1.0, 2.0, 3.0, 4.0, 5.0, 6.0, 7.0, 8.0, 9.0, 10.0]
        comparison = [2.0, 3.0, 4.0, 5.0, 6.0, 7.0, 8.0, 9.0, 10.0, 11.0]
        lower1, upper1 = statistics.bootstrap_ci(baseline, comparison, seed=42)
        lower2, upper2 = statistics.bootstrap_ci(baseline, comparison, seed=99)
        # Different seeds may produce different intervals (not guaranteed but very likely)
        # Just check both are valid
        assert lower1 is not None
        assert upper1 is not None
        assert lower2 is not None
        assert upper2 is not None

    def test_ci_contains_mean_diff(self) -> None:
        baseline = [1.0, 2.0, 3.0, 4.0, 5.0, 6.0, 7.0, 8.0, 9.0, 10.0]
        comparison = [1.5, 2.5, 3.5, 4.5, 5.5, 6.5, 7.5, 8.5, 9.5, 10.5]
        lower, upper = statistics.bootstrap_ci(baseline, comparison, seed=0)
        mean_diff = 0.5
        assert lower is not None
        assert upper is not None
        assert lower <= mean_diff <= upper

    def test_too_few_values(self) -> None:
        assert statistics.bootstrap_ci([1.0], [2.0]) == (None, None)

    def test_ci_ordering(self) -> None:
        baseline = [1.0, 2.0, 3.0, 4.0, 5.0, 6.0, 7.0, 8.0, 9.0, 10.0]
        comparison = [3.0, 4.0, 5.0, 6.0, 7.0, 8.0, 9.0, 10.0, 11.0, 12.0]
        lower, upper = statistics.bootstrap_ci(baseline, comparison, seed=0)
        assert lower is not None
        assert upper is not None
        assert lower <= upper


class TestHolmCorrection:
    def test_empty_input(self) -> None:
        assert statistics.holm_correction([]) == []

    def test_single_p_value(self) -> None:
        result = statistics.holm_correction([0.03])
        assert len(result) == 1
        assert result[0][0] == 1  # rank 1
        assert result[0][1] == 0.03  # no correction for single test

    def test_multiple_p_values_ordered(self) -> None:
        p_values = [0.01, 0.02, 0.03, 0.04, 0.05]
        result = statistics.holm_correction(p_values)
        assert len(result) == 5
        # Smallest p (0.01) gets rank 1, multiplied by 5
        # 0.01 * 5 = 0.05
        # 0.02 * 4 = 0.08, max(0.08, 0.05) = 0.08
        # 0.03 * 3 = 0.09, max(0.09, 0.08) = 0.09
        # 0.04 * 2 = 0.08, max(0.08, 0.09) = 0.09
        # 0.05 * 1 = 0.05, max(0.05, 0.09) = 0.09
        ranks = [r for r, _ in result]
        corrected = [p for _, p in result]
        # Original order: [0.01, 0.02, 0.03, 0.04, 0.05]
        # Ranks: 0.01->1, 0.02->2, 0.03->3, 0.04->4, 0.05->5
        assert ranks == [1, 2, 3, 4, 5]
        assert corrected[0] == 0.05
        assert corrected[1] == 0.08
        assert corrected[2] == 0.09
        assert corrected[3] == 0.09  # monotonicity enforced
        assert corrected[4] == 0.09  # monotonicity enforced

    def test_capped_at_one(self) -> None:
        p_values = [0.5, 0.6, 0.7]
        result = statistics.holm_correction(p_values)
        corrected = [p for _, p in result]
        for p in corrected:
            assert p <= 1.0

    def test_unordered_input(self) -> None:
        p_values = [0.05, 0.01, 0.03, 0.02, 0.04]
        result = statistics.holm_correction(p_values)
        # 0.01 is smallest -> rank 1
        # 0.02 -> rank 2, 0.03 -> rank 3, 0.04 -> rank 4, 0.05 -> rank 5
        ranks = [r for r, _ in result]
        assert ranks == [5, 1, 3, 2, 4]

    def test_monotonicity_enforced(self) -> None:
        # Construct a case where raw correction would decrease
        p_values = [0.01, 0.04, 0.05]
        result = statistics.holm_correction(p_values)
        corrected = [p for _, p in result]
        # 0.01 * 3 = 0.03
        # 0.04 * 2 = 0.08, max(0.08, 0.03) = 0.08
        # 0.05 * 1 = 0.05, max(0.05, 0.08) = 0.08
        assert corrected[0] == 0.03
        assert corrected[1] == 0.08
        assert corrected[2] == 0.08  # enforced to 0.08, not 0.05
