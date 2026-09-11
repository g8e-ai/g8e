# Copyright (c) 2026 Lateralus Labs, LLC.
# Use of this source code is governed by the Business Source License
# included in the LICENSE file.
#
# As of the Change Date listed in the LICENSE file, this software is
# released under the Apache License, Version 2.0.

"""Deterministic renderers for publication schema v5 artifacts.

Renders the radar profile and score family summaries (tool scorecard,
escalation, security events, correlated errors, cold-start vs warm
inference tradeoff) as Markdown, HTML, and CLI views. The renderers
read only the typed fields of the v5 publication models. They perform
no aggregation, no re-computation, and no rounding beyond formatting
the already-rounded values stored on the models.

The v5 renderers are separate from the canonical analysis renderers in
``analysis/renderers.py`` because the v5 publication schema is a
different surface: it carries the radar profile and score family
summaries, not the canonical eval analysis. Both renderers are
deterministic: identical immutable inputs produce byte-identical
output.
"""

from __future__ import annotations

from g8e_evals.radar_profile import (
    ColdStartWarmInferenceTradeoffSummary,
    CorrelatedErrorSummary,
    EscalationSummary,
    RadarProfile,
    SecurityEventSummary,
    ToolScorecardSummary,
)

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


def render_radar_profile_markdown(profile: RadarProfile) -> str:
    """Render the radar profile as a deterministic Markdown section."""
    lines: list[str] = []
    lines.append("## Radar Profile")
    lines.append("")
    lines.append("| Dimension | Value | Source Metrics |")
    lines.append("| --- | ---: | --- |")
    for dim in profile.dimensions:
        lines.append(
            f"| {dim.name.value} | {_fmt_pct(dim.value)} | {', '.join(dim.source_metric_ids)} |"
        )
    lines.append("")
    return "\n".join(lines)


def render_radar_profile_html(profile: RadarProfile) -> str:
    """Render the radar profile as a deterministic HTML section."""
    lines: list[str] = []
    lines.append("<h2>Radar Profile</h2>")
    lines.append("<table>")
    lines.append("<thead><tr><th>Dimension</th><th>Value</th><th>Source Metrics</th></tr></thead>")
    lines.append("<tbody>")
    for dim in profile.dimensions:
        lines.append(
            f"<tr><td>{dim.name.value}</td><td>{_fmt_pct(dim.value)}</td>"
            f"<td>{', '.join(dim.source_metric_ids)}</td></tr>"
        )
    lines.append("</tbody></table>")
    return "\n".join(lines)


def render_radar_profile_cli(profile: RadarProfile) -> str:
    """Render the radar profile as a deterministic plain-text CLI section."""
    lines: list[str] = []
    lines.append("--- Radar Profile ---")
    for dim in profile.dimensions:
        lines.append(f"  {dim.name.value}: {_fmt_pct(dim.value)}")
    lines.append("")
    return "\n".join(lines)


def render_tool_scorecard_summary_markdown(summary: ToolScorecardSummary) -> str:
    """Render the tool scorecard summary as a deterministic Markdown section."""
    lines: list[str] = []
    lines.append("## Tool Calling Scorecard")
    lines.append("")
    lines.append(f"- Total tool calls: {summary.total_tool_calls}")
    lines.append("")
    lines.append("| Dimension | Pass Rate | Tool Call Count |")
    lines.append("| --- | ---: | ---: |")
    for dim in summary.dimensions:
        lines.append(
            f"| {dim.dimension} | {_fmt_pct(dim.pass_rate)} | {dim.tool_call_count} |"
        )
    lines.append("")
    return "\n".join(lines)


def render_tool_scorecard_summary_html(summary: ToolScorecardSummary) -> str:
    """Render the tool scorecard summary as a deterministic HTML section."""
    lines: list[str] = []
    lines.append("<h2>Tool Calling Scorecard</h2>")
    lines.append(f"<p>Total tool calls: {summary.total_tool_calls}</p>")
    lines.append("<table>")
    lines.append("<thead><tr><th>Dimension</th><th>Pass Rate</th><th>Tool Call Count</th></tr></thead>")
    lines.append("<tbody>")
    for dim in summary.dimensions:
        lines.append(
            f"<tr><td>{dim.dimension}</td><td>{_fmt_pct(dim.pass_rate)}</td>"
            f"<td>{dim.tool_call_count}</td></tr>"
        )
    lines.append("</tbody></table>")
    return "\n".join(lines)


def render_tool_scorecard_summary_cli(summary: ToolScorecardSummary) -> str:
    """Render the tool scorecard summary as a deterministic plain-text CLI section."""
    lines: list[str] = []
    lines.append("--- Tool Calling Scorecard ---")
    lines.append(f"  Total tool calls: {summary.total_tool_calls}")
    for dim in summary.dimensions:
        lines.append(f"  {dim.dimension}: {_fmt_pct(dim.pass_rate)} ({dim.tool_call_count} calls)")
    lines.append("")
    return "\n".join(lines)


def render_escalation_summary_markdown(summary: EscalationSummary) -> str:
    """Render the escalation summary as a deterministic Markdown section."""
    lines: list[str] = []
    lines.append("## Escalation Metrics")
    lines.append("")
    lines.append(f"- Total records: {summary.total_records}")
    lines.append(f"- Correct autonomous: {summary.correct_autonomous_count}")
    lines.append(f"- Correct escalation: {summary.correct_escalation_count}")
    lines.append(f"- False escalation: {summary.false_escalation_count}")
    lines.append(f"- Missed escalation: {summary.missed_escalation_count}")
    lines.append(f"- Escalation efficiency: {_fmt_pct(summary.escalation_efficiency)}")
    lines.append("")
    return "\n".join(lines)


def render_escalation_summary_html(summary: EscalationSummary) -> str:
    """Render the escalation summary as a deterministic HTML section."""
    lines: list[str] = []
    lines.append("<h2>Escalation Metrics</h2>")
    lines.append("<ul>")
    lines.append(f"<li>Total records: {summary.total_records}</li>")
    lines.append(f"<li>Correct autonomous: {summary.correct_autonomous_count}</li>")
    lines.append(f"<li>Correct escalation: {summary.correct_escalation_count}</li>")
    lines.append(f"<li>False escalation: {summary.false_escalation_count}</li>")
    lines.append(f"<li>Missed escalation: {summary.missed_escalation_count}</li>")
    lines.append(f"<li>Escalation efficiency: {_fmt_pct(summary.escalation_efficiency)}</li>")
    lines.append("</ul>")
    return "\n".join(lines)


def render_escalation_summary_cli(summary: EscalationSummary) -> str:
    """Render the escalation summary as a deterministic plain-text CLI section."""
    lines: list[str] = []
    lines.append("--- Escalation Metrics ---")
    lines.append(f"  Total records: {summary.total_records}")
    lines.append(f"  Correct autonomous: {summary.correct_autonomous_count}")
    lines.append(f"  Correct escalation: {summary.correct_escalation_count}")
    lines.append(f"  False escalation: {summary.false_escalation_count}")
    lines.append(f"  Missed escalation: {summary.missed_escalation_count}")
    lines.append(f"  Escalation efficiency: {_fmt_pct(summary.escalation_efficiency)}")
    lines.append("")
    return "\n".join(lines)


def render_security_event_summary_markdown(summary: SecurityEventSummary) -> str:
    """Render the security event summary as a deterministic Markdown section."""
    lines: list[str] = []
    lines.append("## Security and Privacy Events")
    lines.append("")
    lines.append(f"- Total records: {summary.total_records}")
    lines.append("")
    lines.append("| Event | Rate |")
    lines.append("| --- | ---: |")
    lines.append(f"| Sensitive data present | {_fmt_pct(summary.sensitive_data_present_rate)} |")
    lines.append(f"| Sensitive data required | {_fmt_pct(summary.sensitive_data_required_rate)} |")
    lines.append(f"| Sensitive data sent externally | {_fmt_pct(summary.sensitive_data_sent_externally_rate)} |")
    lines.append(f"| Unnecessary data sent externally | {_fmt_pct(summary.unnecessary_data_sent_externally_rate)} |")
    lines.append(f"| Policy prevented disclosure | {_fmt_pct(summary.policy_prevented_disclosure_rate)} |")
    lines.append(f"| Model attempted unauthorized access | {_fmt_pct(summary.model_attempted_unauthorized_access_rate)} |")
    lines.append(f"| Tool attempted unauthorized operation | {_fmt_pct(summary.tool_attempted_unauthorized_operation_rate)} |")
    lines.append(f"| Authorization correctly enforced | {_fmt_pct(summary.authorization_correctly_enforced_rate)} |")
    lines.append(f"| Audit record complete | {_fmt_pct(summary.audit_record_complete_rate)} |")
    lines.append(f"| Audit record tampered | {_fmt_pct(summary.audit_record_tampered_rate)} |")
    lines.append(f"| Secret redaction successful | {_fmt_pct(summary.secret_redaction_successful_rate)} |")
    lines.append("")
    return "\n".join(lines)


def render_security_event_summary_html(summary: SecurityEventSummary) -> str:
    """Render the security event summary as a deterministic HTML section."""
    lines: list[str] = []
    lines.append("<h2>Security and Privacy Events</h2>")
    lines.append(f"<p>Total records: {summary.total_records}</p>")
    lines.append("<table>")
    lines.append("<thead><tr><th>Event</th><th>Rate</th></tr></thead>")
    lines.append("<tbody>")
    for event, rate in (
        ("Sensitive data present", summary.sensitive_data_present_rate),
        ("Sensitive data required", summary.sensitive_data_required_rate),
        ("Sensitive data sent externally", summary.sensitive_data_sent_externally_rate),
        ("Unnecessary data sent externally", summary.unnecessary_data_sent_externally_rate),
        ("Policy prevented disclosure", summary.policy_prevented_disclosure_rate),
        ("Model attempted unauthorized access", summary.model_attempted_unauthorized_access_rate),
        ("Tool attempted unauthorized operation", summary.tool_attempted_unauthorized_operation_rate),
        ("Authorization correctly enforced", summary.authorization_correctly_enforced_rate),
        ("Audit record complete", summary.audit_record_complete_rate),
        ("Audit record tampered", summary.audit_record_tampered_rate),
        ("Secret redaction successful", summary.secret_redaction_successful_rate),
    ):
        lines.append(f"<tr><td>{event}</td><td>{_fmt_pct(rate)}</td></tr>")
    lines.append("</tbody></table>")
    return "\n".join(lines)


def render_security_event_summary_cli(summary: SecurityEventSummary) -> str:
    """Render the security event summary as a deterministic plain-text CLI section."""
    lines: list[str] = []
    lines.append("--- Security and Privacy Events ---")
    lines.append(f"  Total records: {summary.total_records}")
    lines.append(f"  Sensitive data present: {_fmt_pct(summary.sensitive_data_present_rate)}")
    lines.append(f"  Sensitive data required: {_fmt_pct(summary.sensitive_data_required_rate)}")
    lines.append(f"  Sensitive data sent externally: {_fmt_pct(summary.sensitive_data_sent_externally_rate)}")
    lines.append(f"  Unnecessary data sent externally: {_fmt_pct(summary.unnecessary_data_sent_externally_rate)}")
    lines.append(f"  Policy prevented disclosure: {_fmt_pct(summary.policy_prevented_disclosure_rate)}")
    lines.append(f"  Model attempted unauthorized access: {_fmt_pct(summary.model_attempted_unauthorized_access_rate)}")
    lines.append(f"  Tool attempted unauthorized operation: {_fmt_pct(summary.tool_attempted_unauthorized_operation_rate)}")
    lines.append(f"  Authorization correctly enforced: {_fmt_pct(summary.authorization_correctly_enforced_rate)}")
    lines.append(f"  Audit record complete: {_fmt_pct(summary.audit_record_complete_rate)}")
    lines.append(f"  Audit record tampered: {_fmt_pct(summary.audit_record_tampered_rate)}")
    lines.append(f"  Secret redaction successful: {_fmt_pct(summary.secret_redaction_successful_rate)}")
    lines.append("")
    return "\n".join(lines)


def render_correlated_error_summary_markdown(summary: CorrelatedErrorSummary) -> str:
    """Render the correlated error summary as a deterministic Markdown section."""
    lines: list[str] = []
    lines.append("## Correlated Error Tracking")
    lines.append("")
    lines.append(f"- Total scenarios: {summary.total_scenarios}")
    lines.append(f"- Correlated failure rate: {_fmt_pct(summary.correlated_failure_rate)}")
    lines.append(f"- Failure independence: {_fmt_pct(summary.failure_independence)}")
    lines.append(f"- Same-family correlated rate: {_fmt_pct(summary.same_family_correlated_rate)}")
    lines.append(f"- Cross-family correlated rate: {_fmt_pct(summary.cross_family_correlated_rate)}")
    lines.append("")
    return "\n".join(lines)


def render_correlated_error_summary_html(summary: CorrelatedErrorSummary) -> str:
    """Render the correlated error summary as a deterministic HTML section."""
    lines: list[str] = []
    lines.append("<h2>Correlated Error Tracking</h2>")
    lines.append("<ul>")
    lines.append(f"<li>Total scenarios: {summary.total_scenarios}</li>")
    lines.append(f"<li>Correlated failure rate: {_fmt_pct(summary.correlated_failure_rate)}</li>")
    lines.append(f"<li>Failure independence: {_fmt_pct(summary.failure_independence)}</li>")
    lines.append(f"<li>Same-family correlated rate: {_fmt_pct(summary.same_family_correlated_rate)}</li>")
    lines.append(f"<li>Cross-family correlated rate: {_fmt_pct(summary.cross_family_correlated_rate)}</li>")
    lines.append("</ul>")
    return "\n".join(lines)


def render_correlated_error_summary_cli(summary: CorrelatedErrorSummary) -> str:
    """Render the correlated error summary as a deterministic plain-text CLI section."""
    lines: list[str] = []
    lines.append("--- Correlated Error Tracking ---")
    lines.append(f"  Total scenarios: {summary.total_scenarios}")
    lines.append(f"  Correlated failure rate: {_fmt_pct(summary.correlated_failure_rate)}")
    lines.append(f"  Failure independence: {_fmt_pct(summary.failure_independence)}")
    lines.append(f"  Same-family correlated rate: {_fmt_pct(summary.same_family_correlated_rate)}")
    lines.append(f"  Cross-family correlated rate: {_fmt_pct(summary.cross_family_correlated_rate)}")
    lines.append("")
    return "\n".join(lines)


def render_cold_start_tradeoff_summary_markdown(summary: ColdStartWarmInferenceTradeoffSummary) -> str:
    """Render the cold-start vs warm inference tradeoff as a deterministic Markdown section."""
    lines: list[str] = []
    lines.append("## Cold-Start vs Warm Inference Tradeoff")
    lines.append("")
    lines.append("| Variant | Load Time (s) | TTFT (s) | Generation (s) | Whole Task (s) |")
    lines.append("| --- | ---: | ---: | ---: | ---: |")
    for t in summary.tradeoffs:
        lines.append(
            f"| {t.variant_id} | {_fmt_float(t.model_load_time_seconds)} "
            f"| {_fmt_float(t.time_to_first_token_seconds)} "
            f"| {_fmt_float(t.generation_duration_seconds)} "
            f"| {_fmt_float(t.whole_task_duration_seconds)} |"
        )
    lines.append("")
    return "\n".join(lines)


def render_cold_start_tradeoff_summary_html(summary: ColdStartWarmInferenceTradeoffSummary) -> str:
    """Render the cold-start vs warm inference tradeoff as a deterministic HTML section."""
    lines: list[str] = []
    lines.append("<h2>Cold-Start vs Warm Inference Tradeoff</h2>")
    lines.append("<table>")
    lines.append("<thead><tr><th>Variant</th><th>Load Time (s)</th><th>TTFT (s)</th><th>Generation (s)</th><th>Whole Task (s)</th></tr></thead>")
    lines.append("<tbody>")
    for t in summary.tradeoffs:
        lines.append(
            f"<tr><td>{t.variant_id}</td>"
            f"<td>{_fmt_float(t.model_load_time_seconds)}</td>"
            f"<td>{_fmt_float(t.time_to_first_token_seconds)}</td>"
            f"<td>{_fmt_float(t.generation_duration_seconds)}</td>"
            f"<td>{_fmt_float(t.whole_task_duration_seconds)}</td></tr>"
        )
    lines.append("</tbody></table>")
    return "\n".join(lines)


def render_cold_start_tradeoff_summary_cli(summary: ColdStartWarmInferenceTradeoffSummary) -> str:
    """Render the cold-start vs warm inference tradeoff as a deterministic plain-text CLI section."""
    lines: list[str] = []
    lines.append("--- Cold-Start vs Warm Inference Tradeoff ---")
    for t in summary.tradeoffs:
        lines.append(
            f"  {t.variant_id}: load={_fmt_float(t.model_load_time_seconds)}s "
            f"ttft={_fmt_float(t.time_to_first_token_seconds)}s "
            f"gen={_fmt_float(t.generation_duration_seconds)}s "
            f"whole={_fmt_float(t.whole_task_duration_seconds)}s"
        )
    lines.append("")
    return "\n".join(lines)


def render_v5_markdown(
    *,
    radar_profile: RadarProfile | None = None,
    tool_scorecard: ToolScorecardSummary | None = None,
    escalation: EscalationSummary | None = None,
    security_events: SecurityEventSummary | None = None,
    correlated_errors: CorrelatedErrorSummary | None = None,
    cold_start_tradeoff: ColdStartWarmInferenceTradeoffSummary | None = None,
) -> str:
    """Render all v5 artifacts as a deterministic Markdown document.

    Each section is included only when the corresponding artifact is not
    None. The output is byte-identical for identical inputs.
    """
    sections: list[str] = []
    if radar_profile is not None:
        sections.append(render_radar_profile_markdown(radar_profile))
    if tool_scorecard is not None:
        sections.append(render_tool_scorecard_summary_markdown(tool_scorecard))
    if escalation is not None:
        sections.append(render_escalation_summary_markdown(escalation))
    if security_events is not None:
        sections.append(render_security_event_summary_markdown(security_events))
    if correlated_errors is not None:
        sections.append(render_correlated_error_summary_markdown(correlated_errors))
    if cold_start_tradeoff is not None:
        sections.append(render_cold_start_tradeoff_summary_markdown(cold_start_tradeoff))
    return "\n".join(sections)


def render_v5_html(
    *,
    radar_profile: RadarProfile | None = None,
    tool_scorecard: ToolScorecardSummary | None = None,
    escalation: EscalationSummary | None = None,
    security_events: SecurityEventSummary | None = None,
    correlated_errors: CorrelatedErrorSummary | None = None,
    cold_start_tradeoff: ColdStartWarmInferenceTradeoffSummary | None = None,
) -> str:
    """Render all v5 artifacts as a deterministic HTML document.

    Each section is included only when the corresponding artifact is not
    None. The output is byte-identical for identical inputs.
    """
    sections: list[str] = []
    if radar_profile is not None:
        sections.append(render_radar_profile_html(radar_profile))
    if tool_scorecard is not None:
        sections.append(render_tool_scorecard_summary_html(tool_scorecard))
    if escalation is not None:
        sections.append(render_escalation_summary_html(escalation))
    if security_events is not None:
        sections.append(render_security_event_summary_html(security_events))
    if correlated_errors is not None:
        sections.append(render_correlated_error_summary_html(correlated_errors))
    if cold_start_tradeoff is not None:
        sections.append(render_cold_start_tradeoff_summary_html(cold_start_tradeoff))
    return "\n".join(sections)


def render_v5_cli(
    *,
    radar_profile: RadarProfile | None = None,
    tool_scorecard: ToolScorecardSummary | None = None,
    escalation: EscalationSummary | None = None,
    security_events: SecurityEventSummary | None = None,
    correlated_errors: CorrelatedErrorSummary | None = None,
    cold_start_tradeoff: ColdStartWarmInferenceTradeoffSummary | None = None,
) -> str:
    """Render all v5 artifacts as a deterministic plain-text CLI view.

    Each section is included only when the corresponding artifact is not
    None. The output is byte-identical for identical inputs.
    """
    sections: list[str] = []
    if radar_profile is not None:
        sections.append(render_radar_profile_cli(radar_profile))
    if tool_scorecard is not None:
        sections.append(render_tool_scorecard_summary_cli(tool_scorecard))
    if escalation is not None:
        sections.append(render_escalation_summary_cli(escalation))
    if security_events is not None:
        sections.append(render_security_event_summary_cli(security_events))
    if correlated_errors is not None:
        sections.append(render_correlated_error_summary_cli(correlated_errors))
    if cold_start_tradeoff is not None:
        sections.append(render_cold_start_tradeoff_summary_cli(cold_start_tradeoff))
    return "\n".join(sections)


__all__ = [
    "render_cold_start_tradeoff_summary_cli",
    "render_cold_start_tradeoff_summary_html",
    "render_cold_start_tradeoff_summary_markdown",
    "render_correlated_error_summary_cli",
    "render_correlated_error_summary_html",
    "render_correlated_error_summary_markdown",
    "render_escalation_summary_cli",
    "render_escalation_summary_html",
    "render_escalation_summary_markdown",
    "render_radar_profile_cli",
    "render_radar_profile_html",
    "render_radar_profile_markdown",
    "render_security_event_summary_cli",
    "render_security_event_summary_html",
    "render_security_event_summary_markdown",
    "render_tool_scorecard_summary_cli",
    "render_tool_scorecard_summary_html",
    "render_tool_scorecard_summary_markdown",
    "render_v5_cli",
    "render_v5_html",
    "render_v5_markdown",
]
