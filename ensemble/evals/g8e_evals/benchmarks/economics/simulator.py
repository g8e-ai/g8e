# Copyright (c) 2026 Lateralus Labs, LLC.
# Use of this source code is governed by the Business Source License
# included in the LICENSE file.
#
# As of the Change Date listed in the LICENSE file, this software is
# released under the Apache License, Version 2.0.

"""Local economics and performance simulator for the synthetic eval suite.

The simulator is a production-shaped system under test that records
measured economics and performance values for declared metric kinds
(provider charge, per-role calls and tokens, stage latency, local
resource metadata, and human wait time). It does not process governance
actions through L1/L2/L3/L4 stages; it produces a deterministic
``EconomicsPerformanceResult`` carrying the observed value so the
``EconomicsPerformanceGrader`` can verify that the measurement falls
within the declared tolerance window.

The simulator is deterministic: the same metric kind and injected
observed value always produce the same result. The caller declares the
observed value so the simulator can produce both correct (within
tolerance) and incorrect (outside tolerance) outcomes for grading.
"""

from __future__ import annotations

from dataclasses import dataclass

from g8e_evals.schema import PerformanceMetricKind, TaskComplexity


@dataclass(frozen=True)
class EconomicsPerformanceResult:
    """The result of processing an economics or performance measurement.

    The ``observed_value`` is the measured value the system recorded.
    ``None`` means the measurement was not collected (for example, the
    provider did not return usage data), which is always a measured
    failure.
    """

    metric_kind: PerformanceMetricKind
    role: str
    action_class: str
    task_complexity: TaskComplexity
    observed_value: float | None
    unit: str


class LocalEconomicsPerformanceSimulator:
    """A local economics and performance simulator that records measured values.

    The simulator processes economics and performance measurements and
    records the observed value. The caller declares the observed value
    via ``scenario_params``, so the simulator can produce both correct
    (within tolerance) and incorrect (outside tolerance) outcomes for
    grading.
    """

    def process_measurement(
        self,
        metric_kind: PerformanceMetricKind,
        role: str,
        action_class: str,
        task_complexity: TaskComplexity,
        observed_value: float | None,
        unit: str,
    ) -> EconomicsPerformanceResult:
        """Process an economics or performance measurement and produce a result.

        The observed value is declared by the caller, so the simulator
        can produce both correct (within the tolerance window) and
        incorrect (outside the window) outcomes.
        """
        return EconomicsPerformanceResult(
            metric_kind=metric_kind,
            role=role,
            action_class=action_class,
            task_complexity=task_complexity,
            observed_value=observed_value,
            unit=unit,
        )
