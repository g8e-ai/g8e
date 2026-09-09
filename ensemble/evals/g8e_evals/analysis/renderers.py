# Copyright (c) 2026 Lateralus Labs, LLC.
# Use of this source code is governed by the Business Source License
# included in the LICENSE file.
#
# As of the Change Date listed in the LICENSE file, this software is
# released under the Apache License, Version 2.0.

"""Deterministic renderers derived from the canonical eval analysis.

Markdown, HTML, and CLI views are derived from
``CanonicalEvalAnalysis``, never computed independently. Identical
immutable inputs and analysis version produce byte-identical analysis
and renderer output.

The renderers read only the typed fields of the canonical analysis.
They perform no aggregation, no re-computation, and no rounding beyond
formatting the already-rounded values stored on the analysis model.
"""

from __future__ import annotations

from g8e_evals.analysis.canonical import CanonicalEvalAnalysis

_FLOAT_FORMAT = "{:.10f}"


def _fmt_float(value: float | None) -> str:
    """Format a float for display, trimming trailing zeros.

    Returns ``"N/A"`` for ``None``. Non-None values are formatted to
    the canonical float precision with trailing zeros removed so that
    ``1.0`` renders as ``1`` and ``0.5`` renders as ``0.5``.
    """
    if value is None:
        return "N/A"
    formatted = _FLOAT_FORMAT.format(value)
    if "." in formatted:
        formatted = formatted.rstrip("0").rstrip(".")
    return formatted if formatted else "0"


def _fmt_pct(value: float | None) -> str:
    """Format a proportion (0.0-1.0) as a percentage string."""
    if value is None:
        return "N/A"
    return f"{_fmt_float(value * 100)}%"


def render_markdown(analysis: CanonicalEvalAnalysis) -> str:
    """Render the canonical analysis as a deterministic Markdown document.

    The output is byte-identical for identical ``CanonicalEvalAnalysis``
    inputs. No aggregation or re-computation is performed; every value
    is read from the typed analysis model.
    """
    lines: list[str] = []
    a = analysis

    lines.append(f"# Eval Analysis — {a.release_version}")
    lines.append("")
    lines.append(f"- Run ID: `{a.run_id}`")
    lines.append(f"- Analysis schema version: `{a.analysis_schema_version}`")
    lines.append(f"- Analysis computation version: `{a.analysis_computation_version}`")
    lines.append("")

    # Input summary
    s = a.input_summary
    lines.append("## Input Summary")
    lines.append("")
    lines.append(f"- Tasks: {s.task_count}")
    lines.append(f"- Attempts: {s.attempt_count}")
    lines.append(f"- Observations: {s.observation_count}")
    lines.append(f"- Receipts: {s.receipt_count}")
    lines.append(f"- Stages: {s.stage_count}")
    lines.append(f"- Metric observations: {s.metric_observation_count}")
    lines.append(f"- Input content hash: `{s.input_content_hash}`")
    lines.append("")

    # Missingness
    m = a.missingness
    lines.append("## Missingness Breakdown")
    lines.append("")
    lines.append("| Status | Count |")
    lines.append("| --- | --- |")
    lines.append(f"| Completed | {m.completed} |")
    lines.append(f"| Model failed | {m.model_failed} |")
    lines.append(f"| Governance rejected | {m.governance_rejected} |")
    lines.append(f"| Human denied | {m.human_denied} |")
    lines.append(f"| Timed out | {m.timed_out} |")
    lines.append(f"| Infrastructure failed | {m.infrastructure_failed} |")
    lines.append(f"| Invalid evidence | {m.invalid_evidence} |")
    lines.append(f"| **Total** | **{m.total}** |")
    lines.append("")

    # Receipt coverage
    rc = a.receipt_coverage
    lines.append("## Receipt Coverage")
    lines.append("")
    lines.append(f"- Eligible attempts: {rc.eligible_attempt_count}")
    lines.append(f"- Receipt-bound attempts: {rc.receipt_bound_count}")
    lines.append(f"- Receipt-verified attempts: {rc.receipt_verified_count}")
    lines.append(f"- Coverage: {_fmt_pct(rc.coverage_pct / 100)}")
    lines.append(f"- Verification: {_fmt_pct(rc.verification_pct / 100)}")
    lines.append("")

    # Arms
    lines.append("## Arms")
    lines.append("")
    for arm_id in a.arm_ids:
        lines.append(f"- `{arm_id}`")
    lines.append("")

    # Metric results
    lines.append("## Metric Results")
    lines.append("")
    lines.append("| Metric | Version | Arm | Domain | Direction | Value | Numerator | Denominator | Eligible | Missing |")
    lines.append("| --- | --- | --- | --- | --- | --- | --- | --- | --- | --- |")
    for mr in a.metric_results:
        lines.append(
            f"| {mr.metric_id} | {mr.metric_version} | {mr.arm_id} | {mr.domain.value} "
            f"| {mr.direction.value} | {_fmt_float(mr.value)} | {_fmt_float(mr.numerator)} "
            f"| {mr.denominator} | {mr.eligible_count} | {mr.missing_count} |"
        )
    lines.append("")

    # Domain-stratified results
    lines.append("## Domain-Stratified Results")
    lines.append("")
    lines.append("| Arm | Domain | Metrics | Passing | Failing | Not Applicable |")
    lines.append("| --- | --- | --- | --- | --- | --- |")
    for ds in a.domain_stratified_results:
        lines.append(
            f"| {ds.arm_id} | {ds.domain.value} | {ds.metric_count} "
            f"| {ds.passing_metric_count} | {ds.failing_metric_count} | {ds.not_applicable_metric_count} |"
        )
    lines.append("")

    # Confusion matrices (arm-level)
    if a.confusion_matrices:
        lines.append("## Confusion Matrices (Arm-Level)")
        lines.append("")
        lines.append("| Metric | Version | Arm | TP | FP | TN | FN | Accuracy | Balanced Accuracy | MCC |")
        lines.append("| --- | --- | --- | --- | --- | --- | --- | --- | --- | --- |")
        for cm in a.confusion_matrices:
            lines.append(
                f"| {cm.metric_id} | {cm.metric_version} | {cm.arm_id} "
                f"| {cm.true_positive} | {cm.false_positive} | {cm.true_negative} | {cm.false_negative} "
                f"| {_fmt_float(cm.accuracy)} | {_fmt_float(cm.balanced_accuracy)} "
                f"| {_fmt_float(cm.matthews_correlation_coefficient)} |"
            )
        lines.append("")

    # Pooled confusion matrices
    if a.pooled_confusion_matrices:
        lines.append("## Pooled Confusion Matrices")
        lines.append("")
        lines.append("| Metric | Version | Arms | TP | FP | TN | FN | Accuracy | Balanced Accuracy | MCC | Pooling Method |")
        lines.append("| --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- |")
        for pm in a.pooled_confusion_matrices:
            lines.append(
                f"| {pm.metric_id} | {pm.metric_version} | {pm.arm_count} ({', '.join(pm.arm_ids)}) "
                f"| {pm.true_positive} | {pm.false_positive} | {pm.true_negative} | {pm.false_negative} "
                f"| {_fmt_float(pm.accuracy)} | {_fmt_float(pm.balanced_accuracy)} "
                f"| {_fmt_float(pm.matthews_correlation_coefficient)} | {pm.pooling_method} |"
            )
        lines.append("")

    # Paired comparisons
    if a.comparisons:
        lines.append("## Paired Comparisons")
        lines.append("")
        lines.append(
            "| Metric | Version | Baseline | Comparison | Paired | Baseline Val | Comparison Val "
            "| Abs Delta | Rel Delta | Effect Size | Direction | McNemar p | Paired t p | Wilcoxon p | Bootstrap CI | Holm p | NI Margin | Gate |"
        )
        lines.append("| --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- |")
        for pc in a.comparisons:
            ci = (
                f"[{_fmt_float(pc.bootstrap_ci_lower)}, {_fmt_float(pc.bootstrap_ci_upper)}]"
                if pc.bootstrap_ci_lower is not None or pc.bootstrap_ci_upper is not None
                else "N/A"
            )
            lines.append(
                f"| {pc.metric_id} | {pc.metric_version} | {pc.baseline_arm_id} | {pc.comparison_arm_id} "
                f"| {pc.paired_count} | {_fmt_float(pc.baseline_value)} | {_fmt_float(pc.comparison_value)} "
                f"| {_fmt_float(pc.absolute_delta)} | {_fmt_float(pc.relative_delta)} "
                f"| {_fmt_float(pc.standardized_effect_size)} | {pc.direction.value} "
                f"| {_fmt_float(pc.mcnemar_p_value)} | {_fmt_float(pc.paired_t_p_value)} "
                f"| {_fmt_float(pc.wilcoxon_p_value)} | {ci} | {_fmt_float(pc.holm_corrected_p_value)} "
                f"| {_fmt_float(pc.non_inferiority_margin)} | {pc.gate_decision.value} |"
            )
        lines.append("")

    # Gate decisions
    lines.append("## Gate Decisions")
    lines.append("")
    lines.append("| Metric | Version | Arm | Status | Measured | Threshold | NI Margin | Reason |")
    lines.append("| --- | --- | --- | --- | --- | --- | --- | --- |")
    for gd in a.gate_decisions:
        lines.append(
            f"| {gd.metric_id} | {gd.metric_version} | {gd.arm_id} | {gd.status.value} "
            f"| {_fmt_float(gd.measured_value)} | {_fmt_float(gd.threshold_value)} "
            f"| {_fmt_float(gd.non_inferiority_margin)} | {gd.reason} |"
        )
    lines.append("")

    # Bridge runs
    if a.bridge_runs:
        lines.append("## Bridge Runs")
        lines.append("")
        lines.append("| Bridge ID | Version | Old Label | New Label | Old Suite Hash | New Suite Hash | Model Cohort | Tasks |")
        lines.append("| --- | --- | --- | --- | --- | --- | --- | --- |")
        for br in a.bridge_runs:
            lines.append(
                f"| {br.bridge_id} | {br.bridge_version} | {br.old_version_label} | {br.new_version_label} "
                f"| `{br.old_suite_hash}` | `{br.new_suite_hash}` | {br.model_cohort_id} | {br.task_count} |"
            )
        lines.append("")

    # Bridge run comparisons
    if a.bridge_run_comparisons:
        lines.append("## Bridge Run Comparisons")
        lines.append("")
        lines.append("| Bridge ID | Metric | Version | Old Value | New Value | Abs Delta | Gate | Reason |")
        lines.append("| --- | --- | --- | --- | --- | --- | --- | --- |")
        for brc in a.bridge_run_comparisons:
            lines.append(
                f"| {brc.bridge_id} | {brc.metric_id} | {brc.metric_version} "
                f"| {_fmt_float(brc.old_value)} | {_fmt_float(brc.new_value)} "
                f"| {_fmt_float(brc.absolute_delta)} | {brc.gate_decision.value} | {brc.reason} |"
            )
        lines.append("")

    # Unsupported claims
    lines.append("## Unsupported Claims")
    lines.append("")
    for claim in a.unsupported_claim_names:
        lines.append(f"- {claim}")
    lines.append("")

    return "\n".join(lines)


def render_html(analysis: CanonicalEvalAnalysis) -> str:
    """Render the canonical analysis as a deterministic HTML document.

    The output is byte-identical for identical ``CanonicalEvalAnalysis``
    inputs. No aggregation or re-computation is performed; every value
    is read from the typed analysis model.
    """
    lines: list[str] = []
    a = analysis

    def _esc(text: str) -> str:
        return text.replace("&", "&").replace("<", "<").replace(">", ">")

    lines.append("<!DOCTYPE html>")
    lines.append('<html lang="en">')
    lines.append("<head>")
    lines.append('<meta charset="utf-8">')
    lines.append(f"<title>Eval Analysis — {_esc(a.release_version)}</title>")
    lines.append("</head>")
    lines.append("<body>")

    lines.append(f"<h1>Eval Analysis — {_esc(a.release_version)}</h1>")
    lines.append("<ul>")
    lines.append(f"<li>Run ID: <code>{_esc(a.run_id)}</code></li>")
    lines.append(f"<li>Analysis schema version: <code>{_esc(a.analysis_schema_version)}</code></li>")
    lines.append(f"<li>Analysis computation version: <code>{_esc(a.analysis_computation_version)}</code></li>")
    lines.append("</ul>")

    # Input summary
    s = a.input_summary
    lines.append("<h2>Input Summary</h2>")
    lines.append("<ul>")
    lines.append(f"<li>Tasks: {s.task_count}</li>")
    lines.append(f"<li>Attempts: {s.attempt_count}</li>")
    lines.append(f"<li>Observations: {s.observation_count}</li>")
    lines.append(f"<li>Receipts: {s.receipt_count}</li>")
    lines.append(f"<li>Stages: {s.stage_count}</li>")
    lines.append(f"<li>Metric observations: {s.metric_observation_count}</li>")
    lines.append(f"<li>Input content hash: <code>{_esc(s.input_content_hash)}</code></li>")
    lines.append("</ul>")

    # Missingness
    m = a.missingness
    lines.append("<h2>Missingness Breakdown</h2>")
    lines.append("<table>")
    lines.append("<thead><tr><th>Status</th><th>Count</th></tr></thead>")
    lines.append("<tbody>")
    lines.append(f"<tr><td>Completed</td><td>{m.completed}</td></tr>")
    lines.append(f"<tr><td>Model failed</td><td>{m.model_failed}</td></tr>")
    lines.append(f"<tr><td>Governance rejected</td><td>{m.governance_rejected}</td></tr>")
    lines.append(f"<tr><td>Human denied</td><td>{m.human_denied}</td></tr>")
    lines.append(f"<tr><td>Timed out</td><td>{m.timed_out}</td></tr>")
    lines.append(f"<tr><td>Infrastructure failed</td><td>{m.infrastructure_failed}</td></tr>")
    lines.append(f"<tr><td>Invalid evidence</td><td>{m.invalid_evidence}</td></tr>")
    lines.append(f"<tr><td><strong>Total</strong></td><td><strong>{m.total}</strong></td></tr>")
    lines.append("</tbody></table>")

    # Receipt coverage
    rc = a.receipt_coverage
    lines.append("<h2>Receipt Coverage</h2>")
    lines.append("<ul>")
    lines.append(f"<li>Eligible attempts: {rc.eligible_attempt_count}</li>")
    lines.append(f"<li>Receipt-bound attempts: {rc.receipt_bound_count}</li>")
    lines.append(f"<li>Receipt-verified attempts: {rc.receipt_verified_count}</li>")
    lines.append(f"<li>Coverage: {_fmt_pct(rc.coverage_pct / 100)}</li>")
    lines.append(f"<li>Verification: {_fmt_pct(rc.verification_pct / 100)}</li>")
    lines.append("</ul>")

    # Arms
    lines.append("<h2>Arms</h2>")
    lines.append("<ul>")
    for arm_id in a.arm_ids:
        lines.append(f"<li><code>{_esc(arm_id)}</code></li>")
    lines.append("</ul>")

    # Metric results
    lines.append("<h2>Metric Results</h2>")
    lines.append("<table>")
    lines.append("<thead><tr><th>Metric</th><th>Version</th><th>Arm</th><th>Domain</th><th>Direction</th><th>Value</th><th>Numerator</th><th>Denominator</th><th>Eligible</th><th>Missing</th></tr></thead>")
    lines.append("<tbody>")
    for mr in a.metric_results:
        lines.append(
            f"<tr><td>{_esc(mr.metric_id)}</td><td>{_esc(mr.metric_version)}</td><td>{_esc(mr.arm_id)}</td>"
            f"<td>{_esc(mr.domain.value)}</td><td>{_esc(mr.direction.value)}</td>"
            f"<td>{_fmt_float(mr.value)}</td><td>{_fmt_float(mr.numerator)}</td>"
            f"<td>{mr.denominator}</td><td>{mr.eligible_count}</td><td>{mr.missing_count}</td></tr>"
        )
    lines.append("</tbody></table>")

    # Domain-stratified results
    lines.append("<h2>Domain-Stratified Results</h2>")
    lines.append("<table>")
    lines.append("<thead><tr><th>Arm</th><th>Domain</th><th>Metrics</th><th>Passing</th><th>Failing</th><th>Not Applicable</th></tr></thead>")
    lines.append("<tbody>")
    for ds in a.domain_stratified_results:
        lines.append(
            f"<tr><td>{_esc(ds.arm_id)}</td><td>{_esc(ds.domain.value)}</td><td>{ds.metric_count}</td>"
            f"<td>{ds.passing_metric_count}</td><td>{ds.failing_metric_count}</td><td>{ds.not_applicable_metric_count}</td></tr>"
        )
    lines.append("</tbody></table>")

    # Confusion matrices (arm-level)
    if a.confusion_matrices:
        lines.append("<h2>Confusion Matrices (Arm-Level)</h2>")
        lines.append("<table>")
        lines.append("<thead><tr><th>Metric</th><th>Version</th><th>Arm</th><th>TP</th><th>FP</th><th>TN</th><th>FN</th><th>Accuracy</th><th>Balanced Accuracy</th><th>MCC</th></tr></thead>")
        lines.append("<tbody>")
        for cm in a.confusion_matrices:
            lines.append(
                f"<tr><td>{_esc(cm.metric_id)}</td><td>{_esc(cm.metric_version)}</td><td>{_esc(cm.arm_id)}</td>"
                f"<td>{cm.true_positive}</td><td>{cm.false_positive}</td><td>{cm.true_negative}</td><td>{cm.false_negative}</td>"
                f"<td>{_fmt_float(cm.accuracy)}</td><td>{_fmt_float(cm.balanced_accuracy)}</td>"
                f"<td>{_fmt_float(cm.matthews_correlation_coefficient)}</td></tr>"
            )
        lines.append("</tbody></table>")

    # Pooled confusion matrices
    if a.pooled_confusion_matrices:
        lines.append("<h2>Pooled Confusion Matrices</h2>")
        lines.append("<table>")
        lines.append("<thead><tr><th>Metric</th><th>Version</th><th>Arms</th><th>TP</th><th>FP</th><th>TN</th><th>FN</th><th>Accuracy</th><th>Balanced Accuracy</th><th>MCC</th><th>Pooling Method</th></tr></thead>")
        lines.append("<tbody>")
        for pm in a.pooled_confusion_matrices:
            lines.append(
                f"<tr><td>{_esc(pm.metric_id)}</td><td>{_esc(pm.metric_version)}</td>"
                f"<td>{pm.arm_count} ({', '.join(_esc(aid) for aid in pm.arm_ids)})</td>"
                f"<td>{pm.true_positive}</td><td>{pm.false_positive}</td><td>{pm.true_negative}</td><td>{pm.false_negative}</td>"
                f"<td>{_fmt_float(pm.accuracy)}</td><td>{_fmt_float(pm.balanced_accuracy)}</td>"
                f"<td>{_fmt_float(pm.matthews_correlation_coefficient)}</td><td>{_esc(pm.pooling_method)}</td></tr>"
            )
        lines.append("</tbody></table>")

    # Paired comparisons
    if a.comparisons:
        lines.append("<h2>Paired Comparisons</h2>")
        lines.append("<table>")
        lines.append("<thead><tr><th>Metric</th><th>Version</th><th>Baseline</th><th>Comparison</th><th>Paired</th><th>Baseline Val</th><th>Comparison Val</th><th>Abs Delta</th><th>Rel Delta</th><th>Effect Size</th><th>Direction</th><th>McNemar p</th><th>Paired t p</th><th>Wilcoxon p</th><th>Bootstrap CI</th><th>Holm p</th><th>NI Margin</th><th>Gate</th></tr></thead>")
        lines.append("<tbody>")
        for pc in a.comparisons:
            ci = (
                f"[{_fmt_float(pc.bootstrap_ci_lower)}, {_fmt_float(pc.bootstrap_ci_upper)}]"
                if pc.bootstrap_ci_lower is not None or pc.bootstrap_ci_upper is not None
                else "N/A"
            )
            lines.append(
                f"<tr><td>{_esc(pc.metric_id)}</td><td>{_esc(pc.metric_version)}</td>"
                f"<td>{_esc(pc.baseline_arm_id)}</td><td>{_esc(pc.comparison_arm_id)}</td><td>{pc.paired_count}</td>"
                f"<td>{_fmt_float(pc.baseline_value)}</td><td>{_fmt_float(pc.comparison_value)}</td>"
                f"<td>{_fmt_float(pc.absolute_delta)}</td><td>{_fmt_float(pc.relative_delta)}</td>"
                f"<td>{_fmt_float(pc.standardized_effect_size)}</td><td>{_esc(pc.direction.value)}</td>"
                f"<td>{_fmt_float(pc.mcnemar_p_value)}</td><td>{_fmt_float(pc.paired_t_p_value)}</td>"
                f"<td>{_fmt_float(pc.wilcoxon_p_value)}</td><td>{ci}</td>"
                f"<td>{_fmt_float(pc.holm_corrected_p_value)}</td><td>{_fmt_float(pc.non_inferiority_margin)}</td>"
                f"<td>{_esc(pc.gate_decision.value)}</td></tr>"
            )
        lines.append("</tbody></table>")

    # Gate decisions
    lines.append("<h2>Gate Decisions</h2>")
    lines.append("<table>")
    lines.append("<thead><tr><th>Metric</th><th>Version</th><th>Arm</th><th>Status</th><th>Measured</th><th>Threshold</th><th>NI Margin</th><th>Reason</th></tr></thead>")
    lines.append("<tbody>")
    for gd in a.gate_decisions:
        lines.append(
            f"<tr><td>{_esc(gd.metric_id)}</td><td>{_esc(gd.metric_version)}</td><td>{_esc(gd.arm_id)}</td>"
            f"<td>{_esc(gd.status.value)}</td><td>{_fmt_float(gd.measured_value)}</td>"
            f"<td>{_fmt_float(gd.threshold_value)}</td><td>{_fmt_float(gd.non_inferiority_margin)}</td>"
            f"<td>{_esc(gd.reason)}</td></tr>"
        )
    lines.append("</tbody></table>")

    # Bridge runs
    if a.bridge_runs:
        lines.append("<h2>Bridge Runs</h2>")
        lines.append("<table>")
        lines.append("<thead><tr><th>Bridge ID</th><th>Version</th><th>Old Label</th><th>New Label</th><th>Old Suite Hash</th><th>New Suite Hash</th><th>Model Cohort</th><th>Tasks</th></tr></thead>")
        lines.append("<tbody>")
        for br in a.bridge_runs:
            lines.append(
                f"<tr><td>{_esc(br.bridge_id)}</td><td>{_esc(br.bridge_version)}</td>"
                f"<td>{_esc(br.old_version_label)}</td><td>{_esc(br.new_version_label)}</td>"
                f"<td><code>{_esc(br.old_suite_hash)}</code></td><td><code>{_esc(br.new_suite_hash)}</code></td>"
                f"<td>{_esc(br.model_cohort_id)}</td><td>{br.task_count}</td></tr>"
            )
        lines.append("</tbody></table>")

    # Bridge run comparisons
    if a.bridge_run_comparisons:
        lines.append("<h2>Bridge Run Comparisons</h2>")
        lines.append("<table>")
        lines.append("<thead><tr><th>Bridge ID</th><th>Metric</th><th>Version</th><th>Old Value</th><th>New Value</th><th>Abs Delta</th><th>Gate</th><th>Reason</th></tr></thead>")
        lines.append("<tbody>")
        for brc in a.bridge_run_comparisons:
            lines.append(
                f"<tr><td>{_esc(brc.bridge_id)}</td><td>{_esc(brc.metric_id)}</td><td>{_esc(brc.metric_version)}</td>"
                f"<td>{_fmt_float(brc.old_value)}</td><td>{_fmt_float(brc.new_value)}</td>"
                f"<td>{_fmt_float(brc.absolute_delta)}</td><td>{_esc(brc.gate_decision.value)}</td>"
                f"<td>{_esc(brc.reason)}</td></tr>"
            )
        lines.append("</tbody></table>")

    # Unsupported claims
    lines.append("<h2>Unsupported Claims</h2>")
    lines.append("<ul>")
    for claim in a.unsupported_claim_names:
        lines.append(f"<li>{_esc(claim)}</li>")
    lines.append("</ul>")

    lines.append("</body>")
    lines.append("</html>")

    return "\n".join(lines)


def render_cli(analysis: CanonicalEvalAnalysis) -> str:
    """Render the canonical analysis as a deterministic plain-text CLI view.

    The output is byte-identical for identical ``CanonicalEvalAnalysis``
    inputs. This renderer produces plain text (not rich-formatted) so
    that output is deterministic and diffable. No aggregation or
    re-computation is performed; every value is read from the typed
    analysis model.
    """
    lines: list[str] = []
    a = analysis

    lines.append(f"=== Eval Analysis — {a.release_version} ===")
    lines.append(f"Run ID: {a.run_id}")
    lines.append(f"Analysis schema: {a.analysis_schema_version}")
    lines.append(f"Analysis computation: {a.analysis_computation_version}")
    lines.append("")

    # Input summary
    s = a.input_summary
    lines.append("--- Input Summary ---")
    lines.append(f"  Tasks: {s.task_count}")
    lines.append(f"  Attempts: {s.attempt_count}")
    lines.append(f"  Observations: {s.observation_count}")
    lines.append(f"  Receipts: {s.receipt_count}")
    lines.append(f"  Stages: {s.stage_count}")
    lines.append(f"  Metric observations: {s.metric_observation_count}")
    lines.append(f"  Input content hash: {s.input_content_hash}")
    lines.append("")

    # Missingness
    m = a.missingness
    lines.append("--- Missingness Breakdown ---")
    lines.append(f"  Completed: {m.completed}")
    lines.append(f"  Model failed: {m.model_failed}")
    lines.append(f"  Governance rejected: {m.governance_rejected}")
    lines.append(f"  Human denied: {m.human_denied}")
    lines.append(f"  Timed out: {m.timed_out}")
    lines.append(f"  Infrastructure failed: {m.infrastructure_failed}")
    lines.append(f"  Invalid evidence: {m.invalid_evidence}")
    lines.append(f"  Total: {m.total}")
    lines.append("")

    # Receipt coverage
    rc = a.receipt_coverage
    lines.append("--- Receipt Coverage ---")
    lines.append(f"  Eligible attempts: {rc.eligible_attempt_count}")
    lines.append(f"  Receipt-bound: {rc.receipt_bound_count}")
    lines.append(f"  Receipt-verified: {rc.receipt_verified_count}")
    lines.append(f"  Coverage: {_fmt_pct(rc.coverage_pct / 100)}")
    lines.append(f"  Verification: {_fmt_pct(rc.verification_pct / 100)}")
    lines.append("")

    # Arms
    lines.append("--- Arms ---")
    for arm_id in a.arm_ids:
        lines.append(f"  {arm_id}")
    lines.append("")

    # Metric results
    lines.append("--- Metric Results ---")
    for mr in a.metric_results:
        lines.append(
            f"  {mr.metric_id}@{mr.metric_version} [{mr.arm_id}] "
            f"domain={mr.domain.value} dir={mr.direction.value} "
            f"value={_fmt_float(mr.value)} num={_fmt_float(mr.numerator)} "
            f"den={mr.denominator} eligible={mr.eligible_count} missing={mr.missing_count}"
        )
    lines.append("")

    # Domain-stratified results
    lines.append("--- Domain-Stratified Results ---")
    for ds in a.domain_stratified_results:
        lines.append(
            f"  {ds.arm_id} / {ds.domain.value}: "
            f"metrics={ds.metric_count} pass={ds.passing_metric_count} "
            f"fail={ds.failing_metric_count} na={ds.not_applicable_metric_count}"
        )
    lines.append("")

    # Confusion matrices (arm-level)
    if a.confusion_matrices:
        lines.append("--- Confusion Matrices (Arm-Level) ---")
        for cm in a.confusion_matrices:
            lines.append(
                f"  {cm.metric_id}@{cm.metric_version} [{cm.arm_id}] "
                f"TP={cm.true_positive} FP={cm.false_positive} "
                f"TN={cm.true_negative} FN={cm.false_negative} "
                f"acc={_fmt_float(cm.accuracy)} bacc={_fmt_float(cm.balanced_accuracy)} "
                f"mcc={_fmt_float(cm.matthews_correlation_coefficient)}"
            )
        lines.append("")

    # Pooled confusion matrices
    if a.pooled_confusion_matrices:
        lines.append("--- Pooled Confusion Matrices ---")
        for pm in a.pooled_confusion_matrices:
            lines.append(
                f"  {pm.metric_id}@{pm.metric_version} arms={pm.arm_count} ({', '.join(pm.arm_ids)}) "
                f"TP={pm.true_positive} FP={pm.false_positive} "
                f"TN={pm.true_negative} FN={pm.false_negative} "
                f"acc={_fmt_float(pm.accuracy)} bacc={_fmt_float(pm.balanced_accuracy)} "
                f"mcc={_fmt_float(pm.matthews_correlation_coefficient)} "
                f"pool={pm.pooling_method}"
            )
        lines.append("")

    # Paired comparisons
    if a.comparisons:
        lines.append("--- Paired Comparisons ---")
        for pc in a.comparisons:
            ci = (
                f"[{_fmt_float(pc.bootstrap_ci_lower)}, {_fmt_float(pc.bootstrap_ci_upper)}]"
                if pc.bootstrap_ci_lower is not None or pc.bootstrap_ci_upper is not None
                else "N/A"
            )
            lines.append(
                f"  {pc.metric_id}@{pc.metric_version} "
                f"{pc.baseline_arm_id}->{pc.comparison_arm_id} "
                f"paired={pc.paired_count} "
                f"base={_fmt_float(pc.baseline_value)} comp={_fmt_float(pc.comparison_value)} "
                f"delta={_fmt_float(pc.absolute_delta)} reldelta={_fmt_float(pc.relative_delta)} "
                f"es={_fmt_float(pc.standardized_effect_size)} dir={pc.direction.value} "
                f"mcnemar_p={_fmt_float(pc.mcnemar_p_value)} "
                f"paired_t_p={_fmt_float(pc.paired_t_p_value)} "
                f"wilcoxon_p={_fmt_float(pc.wilcoxon_p_value)} ci={ci} "
                f"holm_p={_fmt_float(pc.holm_corrected_p_value)} "
                f"ni_margin={_fmt_float(pc.non_inferiority_margin)} "
                f"gate={pc.gate_decision.value}"
            )
        lines.append("")

    # Gate decisions
    lines.append("--- Gate Decisions ---")
    for gd in a.gate_decisions:
        lines.append(
            f"  {gd.metric_id}@{gd.metric_version} [{gd.arm_id}] "
            f"status={gd.status.value} measured={_fmt_float(gd.measured_value)} "
            f"threshold={_fmt_float(gd.threshold_value)} "
            f"ni_margin={_fmt_float(gd.non_inferiority_margin)} "
            f"reason={gd.reason}"
        )
    lines.append("")

    # Bridge runs
    if a.bridge_runs:
        lines.append("--- Bridge Runs ---")
        for br in a.bridge_runs:
            lines.append(
                f"  {br.bridge_id} v{br.bridge_version} "
                f"old={br.old_version_label} new={br.new_version_label} "
                f"old_hash={br.old_suite_hash} new_hash={br.new_suite_hash} "
                f"cohort={br.model_cohort_id} tasks={br.task_count}"
            )
        lines.append("")

    # Bridge run comparisons
    if a.bridge_run_comparisons:
        lines.append("--- Bridge Run Comparisons ---")
        for brc in a.bridge_run_comparisons:
            lines.append(
                f"  {brc.bridge_id} {brc.metric_id}@{brc.metric_version} "
                f"old={_fmt_float(brc.old_value)} new={_fmt_float(brc.new_value)} "
                f"delta={_fmt_float(brc.absolute_delta)} "
                f"gate={brc.gate_decision.value} reason={brc.reason}"
            )
        lines.append("")

    # Unsupported claims
    lines.append("--- Unsupported Claims ---")
    for claim in a.unsupported_claim_names:
        lines.append(f"  {claim}")
    lines.append("")

    return "\n".join(lines)


__all__ = [
    "render_cli",
    "render_html",
    "render_markdown",
]
