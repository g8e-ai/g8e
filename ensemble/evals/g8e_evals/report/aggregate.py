# Copyright (c) 2026 Lateralus Labs, LLC.
# Use of this source code is governed by the Business Source License
# included in the LICENSE file.
#
# As of the Change Date listed in the LICENSE file, this software is
# released under the Apache License, Version 2.0.

from g8e_evals.harness import Aggregate, BindingType, RowResult

# The current Go ConsensusService loops over enrolled signing members but
# every member calls the same deterministic L1Doctrine evaluateSafety()
# implementation. This is not heterogeneous small-model reasoning. Reports
# label the current L2 implementation ``deterministic_replicated_doctrine``
# until distinct reasoners exist and their independence is evidenced.
L2_IMPLEMENTATION_LABEL = "deterministic_replicated_doctrine"


def aggregate_results(suite: str, results: list[RowResult]) -> Aggregate:
    total = len(results)
    if total == 0:
        return Aggregate(suite, 0.0, 0, 0, 0.0, 0.0)

    passed = sum(1 for r in results if r.score.passed)
    pass_rate = (passed / total) * 100.0

    bound = sum(1 for r in results if r.response.binding == BindingType.RECEIPT_BOUND)
    coverage = (bound / total) * 100.0

    verified = sum(1 for r in results if r.response.receipts_verified)
    # Verification % is of the bound ones, or of total?
    # Plan §8 says "receipt verification %" - let's do of total for honesty.
    verification_pct = (verified / total) * 100.0

    arm_ids = sorted({r.arm.value for r in results if r.arm is not None})
    l2_label = L2_IMPLEMENTATION_LABEL if any(a in ("consensus", "notary") for a in arm_ids) else ""

    return Aggregate(
        suite=suite,
        pass_rate=pass_rate,
        total_tasks=total,
        passed_tasks=passed,
        receipt_coverage_pct=coverage,
        receipt_verification_pct=verification_pct,
        metadata={
            "arms": arm_ids,
            "l2_implementation_label": l2_label,
        },
    )
