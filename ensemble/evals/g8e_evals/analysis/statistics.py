# Copyright (c) 2026 Lateralus Labs, LLC.
# Use of this source code is governed by the Business Source License
# included in the LICENSE file.
#
# As of the Change Date listed in the LICENSE file, this software is
# released under the Apache License, Version 2.0.

"""Paired statistical estimators for canonical eval analysis.

Implementations use numpy for vectorized computation and are deterministic:
where randomness is needed (bootstrap), a seeded ``numpy.random.Generator``
is used so identical inputs produce byte-identical outputs. All floating-
point results are rounded to a fixed precision for canonical serialization.

Estimators provided:

- **McNemar test** (exact binomial and chi-squared variants) for paired
  binary outcomes.
- **Paired t-test** for paired continuous outcomes.
- **Wilcoxon signed-rank test** for paired continuous outcomes.
- **Cohen's d** standardized effect size for paired samples.
- **Task-cluster bootstrap** confidence intervals with a fixed seed.
- **Holm-Bonferroni** step-down multiplicity correction.
- **Absolute and relative deltas**.
"""

from __future__ import annotations

import math
from collections.abc import Sequence

import numpy as np

_FLOAT_PRECISION = 10


def _round(value: float) -> float:
    return round(float(value), _FLOAT_PRECISION)


def _paired_sample_size(baseline_values: Sequence[object], comparison_values: Sequence[object]) -> int:
    if len(baseline_values) != len(comparison_values):
        raise ValueError("paired samples must have equal lengths")
    return len(baseline_values)


# ---------------------------------------------------------------------------
# Delta computations
# ---------------------------------------------------------------------------


def absolute_delta(baseline: float, comparison: float) -> float:
    """Return ``comparison - baseline`` rounded to fixed precision."""
    return _round(comparison - baseline)


def relative_delta(baseline: float, comparison: float) -> float | None:
    """Return ``(comparison - baseline) / |baseline|`` or ``None`` when baseline is zero."""
    if baseline == 0.0:
        return None
    return _round((comparison - baseline) / abs(baseline))


# ---------------------------------------------------------------------------
# Cohen's d (paired, standardized effect size)
# ---------------------------------------------------------------------------


def cohens_d_paired(baseline_values: Sequence[float], comparison_values: Sequence[float]) -> float | None:
    """Compute Cohen's d for paired samples.

    ``d = mean(diff) / stddev(diff)`` where ``diff = comparison - baseline``.
    Returns ``None`` when fewer than 2 paired values or zero variance.
    """
    n = _paired_sample_size(baseline_values, comparison_values)
    if n < 2:
        return None
    diffs = np.array(
        [comparison_values[i] - baseline_values[i] for i in range(n)],
        dtype=np.float64,
    )
    mean_diff = float(diffs.mean())
    stddev = float(diffs.std(ddof=1))
    if stddev == 0.0:
        return None
    return _round(mean_diff / stddev)


# ---------------------------------------------------------------------------
# McNemar test (paired binary)
# ---------------------------------------------------------------------------


def mcnemar_test(
    baseline_binary: Sequence[bool],
    comparison_binary: Sequence[bool],
) -> tuple[float | None, float | None]:
    """Compute the McNemar test statistic and two-sided p-value.

    Returns ``(statistic, p_value)``. Uses the exact binomial test when
    the discordant count is small (<= 25), and the chi-squared
    approximation with continuity correction otherwise.

    Returns ``(None, None)`` when there are no discordant pairs.
    """
    n = _paired_sample_size(baseline_binary, comparison_binary)
    if n == 0:
        return None, None

    b = sum(1 for i in range(n) if baseline_binary[i] and not comparison_binary[i])
    c = sum(1 for i in range(n) if not baseline_binary[i] and comparison_binary[i])
    discordant = b + c

    if discordant == 0:
        return None, None

    if discordant <= 25:
        p_value = _binomial_two_sided(b, discordant, 0.5)
        statistic = float(abs(b - c))
        return _round(statistic), _round(p_value)
    statistic = (abs(b - c) - 1) ** 2 / discordant
    p_value = _chi2_sf(statistic, df=1)
    return _round(statistic), _round(p_value)


def _binomial_two_sided(successes: int, trials: int, p: float) -> float:
    """Two-sided p-value for a binomial test."""
    if trials == 0:
        return 1.0
    observed_prob = _binom_pmf(successes, trials, p)
    p_value = 0.0
    for k in range(trials + 1):
        pk = _binom_pmf(k, trials, p)
        if pk <= observed_prob + 1e-15:
            p_value += pk
    return min(p_value, 1.0)


def _binom_pmf(k: int, n: int, p: float) -> float:
    """Binomial probability mass function via log-space computation."""
    if k < 0 or k > n:
        return 0.0
    log_coeff = _log_binom_coeff(n, k)
    log_p = log_coeff + k * math.log(p) + (n - k) * math.log(1.0 - p)
    return math.exp(log_p)


def _log_binom_coeff(n: int, k: int) -> float:
    """Log of binomial coefficient C(n, k)."""
    if k < 0 or k > n:
        return float("-inf")
    k = min(k, n - k)
    result = 0.0
    for i in range(k):
        result += math.log(n - i) - math.log(i + 1)
    return result


def _chi2_sf(x: float, df: int) -> float:
    """Survival function (1 - CDF) of the chi-squared distribution."""
    if x <= 0.0:
        return 1.0
    return _regularized_upper_gamma(df / 2.0, x / 2.0)


def _regularized_upper_gamma(a: float, x: float) -> float:
    """Regularized upper incomplete gamma function Q(a, x).

    Uses the series expansion for small x and the continued fraction
    (Lentz's method) for large x.
    """
    if x < 0.0 or a <= 0.0:
        return 1.0
    if x == 0.0:
        return 1.0

    gln = math.lgamma(a)

    if x < a + 1.0:
        ap = a
        summ = 1.0 / a
        delta = summ
        for _ in range(200):
            ap += 1.0
            delta *= x / ap
            summ += delta
            if abs(delta) < abs(summ) * 1e-15:
                break
        p = summ * math.exp(-x + a * math.log(x) - gln)
        return max(0.0, min(1.0, 1.0 - p))
    b = x + 1.0 - a
    c = 1e30
    d = 1.0 / b
    h = d
    for i in range(1, 200):
        an = -i * (i - a)
        b += 2.0
        d = an * d + b
        if abs(d) < 1e-30:
            d = 1e-30
        c = b + an / c
        if abs(c) < 1e-30:
            c = 1e-30
        d = 1.0 / d
        delta = d * c
        h *= delta
        if abs(delta - 1.0) < 1e-15:
            break
    q = h * math.exp(-x + a * math.log(x) - gln)
    return max(0.0, min(1.0, q))


# ---------------------------------------------------------------------------
# Paired t-test
# ---------------------------------------------------------------------------


def paired_t_test(
    baseline_values: Sequence[float],
    comparison_values: Sequence[float],
) -> tuple[float | None, float | None]:
    """Compute the paired t-test statistic and two-sided p-value.

    Returns ``(t_statistic, p_value)``. Returns ``(None, None)`` when
    fewer than 2 paired values or zero variance.
    """
    n = _paired_sample_size(baseline_values, comparison_values)
    if n < 2:
        return None, None

    diffs = np.array(
        [comparison_values[i] - baseline_values[i] for i in range(n)],
        dtype=np.float64,
    )
    mean_diff = float(diffs.mean())
    variance = float(diffs.var(ddof=1))
    stddev = math.sqrt(variance)

    if stddev == 0.0:
        return None, None

    t_stat = mean_diff / (stddev / math.sqrt(n))
    df = n - 1
    p_value = _t_distribution_two_sided(t_stat, df)

    return _round(t_stat), _round(p_value)


def _t_distribution_two_sided(t: float, df: int) -> float:
    """Two-sided p-value for the t-distribution via the incomplete beta function."""
    if df <= 0:
        return 1.0
    x = df / (df + t * t)
    p_two_sided = _regularized_incomplete_beta(x, df / 2.0, 0.5)
    return min(1.0, max(0.0, p_two_sided))


def _regularized_incomplete_beta(x: float, a: float, b: float) -> float:
    """Regularized incomplete beta function I_x(a, b) via continued fraction."""
    if x <= 0.0:
        return 0.0
    if x >= 1.0:
        return 1.0

    lbeta = math.lgamma(a) + math.lgamma(b) - math.lgamma(a + b)
    front = math.exp(a * math.log(x) + b * math.log(1.0 - x) - lbeta)

    if x < (a + 1.0) / (a + b + 2.0):
        cf = _beta_cf(x, a, b)
        return front * cf / a
    cf = _beta_cf(1.0 - x, b, a)
    return 1.0 - front * cf / b


def _beta_cf(x: float, a: float, b: float) -> float:
    """Continued fraction for the incomplete beta function (Lentz's method)."""
    qab = a + b
    qap = a + 1.0
    qam = a - 1.0

    c = 1.0
    d = 1.0 - qab * x / qap
    if abs(d) < 1e-30:
        d = 1e-30
    d = 1.0 / d
    h = d

    for m in range(1, 200):
        m2 = 2 * m
        aa = m * (b - m) * x / ((qam + m2) * (a + m2))
        d = 1.0 + aa * d
        if abs(d) < 1e-30:
            d = 1e-30
        c = 1.0 + aa / c
        if abs(c) < 1e-30:
            c = 1e-30
        d = 1.0 / d
        h *= d * c
        aa = -(a + m) * (qab + m) * x / ((a + m2) * (qap + m2))
        d = 1.0 + aa * d
        if abs(d) < 1e-30:
            d = 1e-30
        c = 1.0 + aa / c
        if abs(c) < 1e-30:
            c = 1e-30
        d = 1.0 / d
        delta = d * c
        h *= delta
        if abs(delta - 1.0) < 1e-15:
            break

    return h


# ---------------------------------------------------------------------------
# Wilcoxon signed-rank test
# ---------------------------------------------------------------------------


def wilcoxon_signed_rank_test(
    baseline_values: Sequence[float],
    comparison_values: Sequence[float],
) -> tuple[float | None, float | None]:
    """Compute the Wilcoxon signed-rank test statistic and two-sided p-value.

    Returns ``(w_statistic, p_value)``. Uses the exact distribution for
    small samples (n <= 20) and the normal approximation with continuity
    correction for larger samples.

    Returns ``(None, None)`` when fewer than 2 paired values or all
    differences are zero.
    """
    n = _paired_sample_size(baseline_values, comparison_values)
    if n < 2:
        return None, None

    diffs = np.array(
        [comparison_values[i] - baseline_values[i] for i in range(n)],
        dtype=np.float64,
    )
    non_zero = diffs[diffs != 0.0]
    if len(non_zero) < 2:
        return None, None

    abs_diffs = np.abs(non_zero)
    n_nz = len(non_zero)

    # Assign ranks with average ranks for ties
    order = np.argsort(abs_diffs, kind="stable")
    ranks = np.empty(n_nz, dtype=np.float64)
    i = 0
    while i < n_nz:
        j = i
        while j < n_nz and abs_diffs[order[j]] == abs_diffs[order[i]]:
            j += 1
        avg_rank = (i + 1 + j) / 2.0
        for k in range(i, j):
            ranks[order[k]] = avg_rank
        i = j

    w_plus = float(np.sum(ranks[non_zero > 0]))
    w_minus = float(np.sum(ranks[non_zero < 0]))
    w_stat = min(w_plus, w_minus)

    if n_nz <= 20:
        p_value = _wilcoxon_exact_p_value(w_stat, ranks)
    else:
        mean_w = n_nz * (n_nz + 1) / 4.0
        tie_counts: dict[float, int] = {}
        for a in abs_diffs:
            a_f = float(a)
            tie_counts[a_f] = tie_counts.get(a_f, 0) + 1
        tie_sum = sum(t * (t * t - 1) for t in tie_counts.values()) / 48.0
        var_w = n_nz * (n_nz + 1) * (2 * n_nz + 1) / 24.0 - tie_sum
        if var_w <= 0:
            return None, None
        stddev = math.sqrt(var_w)
        z = (w_stat - mean_w) / stddev
        if z > 0:
            z -= 0.5 / stddev
        elif z < 0:
            z += 0.5 / stddev
        p_value = 2.0 * _normal_sf(abs(z))

    return _round(w_stat), _round(min(max(p_value, 0.0), 1.0))


def _wilcoxon_exact_p_value(w_stat: float, ranks: np.ndarray) -> float:
    """Exact two-sided p-value for the Wilcoxon signed-rank test via DP."""
    scaled_ranks = [round(float(rank) * 2) for rank in ranks]
    total = sum(scaled_ranks)
    counts = np.zeros(total + 1, dtype=np.int64)
    counts[0] = 1
    reachable = 0
    for rank in scaled_ranks:
        for score in range(reachable, -1, -1):
            counts[score + rank] += counts[score]
        reachable += rank

    observed = round(w_stat * 2)
    extreme_count = sum(
        int(count)
        for score, count in enumerate(counts)
        if score <= observed or score >= total - observed
    )
    return min(extreme_count / (2 ** len(scaled_ranks)), 1.0)


def _normal_sf(z: float) -> float:
    """Survival function (1 - CDF) of the standard normal distribution."""
    return 0.5 * math.erfc(z / math.sqrt(2.0))


# ---------------------------------------------------------------------------
# Task-cluster bootstrap confidence interval
# ---------------------------------------------------------------------------


def bootstrap_ci(
    baseline_values: Sequence[float],
    comparison_values: Sequence[float],
    n_bootstrap: int = 10000,
    confidence: float = 0.95,
    seed: int = 0,
) -> tuple[float | None, float | None]:
    """Compute a task-cluster bootstrap confidence interval for the mean difference.

    Resamples paired differences with replacement using a seeded numpy
    ``Generator``. Returns ``(ci_lower, ci_upper)``. Returns ``(None, None)``
    when fewer than 2 paired values.

    The bootstrap is deterministic: identical inputs and seed produce
    identical intervals.
    """
    n = _paired_sample_size(baseline_values, comparison_values)
    if n_bootstrap <= 0:
        raise ValueError("bootstrap sample count must be positive")
    if not 0.0 < confidence < 1.0:
        raise ValueError("bootstrap confidence must be between zero and one")
    if n < 2:
        return None, None

    diffs = np.array(
        [comparison_values[i] - baseline_values[i] for i in range(n)],
        dtype=np.float64,
    )
    rng = np.random.default_rng(seed)
    indices = rng.integers(0, n, size=(n_bootstrap, n))
    boot_means = diffs[indices].mean(axis=1)
    boot_means.sort()

    alpha = 1.0 - confidence
    lower_idx = math.floor(alpha / 2.0 * n_bootstrap)
    upper_idx = math.ceil((1.0 - alpha / 2.0) * n_bootstrap) - 1
    upper_idx = min(upper_idx, n_bootstrap - 1)

    return _round(float(boot_means[lower_idx])), _round(float(boot_means[upper_idx]))


# ---------------------------------------------------------------------------
# Holm-Bonferroni correction
# ---------------------------------------------------------------------------


def holm_correction(p_values: Sequence[float]) -> list[tuple[int, float]]:
    """Apply the Holm-Bonferroni step-down correction.

    Returns a list of ``(rank, corrected_p_value)`` tuples in the original
    order. Rank is the 1-based position of the original p-value in the
    sorted order (1 = smallest). Corrected p-values are monotonically
    enforced and capped at 1.0.
    """
    n = len(p_values)
    if n == 0:
        return []
    if any(not math.isfinite(p_value) or not 0.0 <= p_value <= 1.0 for p_value in p_values):
        raise ValueError("Holm correction requires finite probabilities between zero and one")

    p_arr = np.array(p_values, dtype=np.float64)
    sorted_indices = np.argsort(p_arr, kind="stable")
    sorted_p = p_arr[sorted_indices]

    multipliers = np.arange(n, 0, -1, dtype=np.float64)
    raw_corrected = sorted_p * multipliers
    raw_corrected = np.minimum(raw_corrected, 1.0)
    # Enforce monotonicity: cumulative max
    corrected_sorted = np.maximum.accumulate(raw_corrected)

    corrected = np.empty(n, dtype=np.float64)
    corrected[sorted_indices] = corrected_sorted

    rank_by_orig: dict[int, int] = {}
    for rank, idx in enumerate(sorted_indices, start=1):
        rank_by_orig[int(idx)] = rank

    return [(rank_by_orig[i], _round(float(corrected[i]))) for i in range(n)]


__all__ = [
    "absolute_delta",
    "bootstrap_ci",
    "cohens_d_paired",
    "holm_correction",
    "mcnemar_test",
    "paired_t_test",
    "relative_delta",
    "wilcoxon_signed_rank_test",
]
