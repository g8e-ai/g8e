# Copyright (c) 2026 Lateralus Labs, LLC.
# Licensed under the Apache License, Version 2.0

"""Report renderers for AI evals metrics.

Provides text table, CSV, and JSON output formats for evaluation results.
"""

import csv
import json
import os
import shutil
from datetime import datetime, UTC
from pathlib import Path
from typing import Any

from .metrics import DimensionSummary, EvalRow, FullReport


def render_text_table(report: FullReport) -> str:
    """Render a coverage-style ASCII text table from a FullReport.

    Returns a multi-line string with per-dimension sections, category breakdowns,
    and an aggregate footer. Privacy dimension includes per-layer breakdown.
    """
    lines = []
    
    llm_config = report.llm_config
    primary = llm_config.get("primary_provider", "unknown")
    primary_model = llm_config.get("primary_model", "unknown")
    assistant = llm_config.get("assistant_provider", "none")
    assistant_model = llm_config.get("assistant_model", "none")
    
    lines.append("=" * 60)
    lines.append("AI EVALS REPORT")
    lines.append("=" * 60)
    lines.append(
        f"Run:  {report.started_at.strftime('%Y-%m-%dT%H:%M:%SZ')}   "
        f"Primary: {primary}/{primary_model}   Assistant: {assistant}/{assistant_model}"
    )
    lines.append("")
    
    dimensions_order = ["accuracy", "safety", "privacy"]
    for dimension in dimensions_order:
        if dimension not in report.summaries:
            continue
        
        summary = report.summaries[dimension]
        lines.append(f"{dimension.upper()}")
        lines.append("")
        
        if dimension == "privacy":
            _render_privacy_section(lines, report.rows)
        else:
            _render_standard_dimension(lines, dimension, report.rows)
        
        lines.append("-" * 78)
        avg_score_str = f"{summary.avg_score:.2f}/5" if summary.avg_score is not None else "n/a"
        lines.append(
            f"  {dimension.upper()} AGGREGATE{' ' * (40 - len(dimension))}"
            f"{summary.total:4d}   {summary.passed:4d}  {summary.pass_pct:5.1f}%  {avg_score_str}"
        )
        lines.append("")
    
    lines.append("=" * 60)
    lines.append("OVERALL")
    lines.append("")
    
    total_rows = len(report.rows)
    total_passed = sum(1 for r in report.rows if r.passed)
    total_pct = (total_passed / total_rows * 100) if total_rows > 0 else 0
    
    l2_l3_passed = 0
    l2_l3_total = 0
    findings = []
    
    for row in report.rows:
        if row.dimension == "privacy":
            l2_l3_total += 1
            details = row.details
            if details.get("egress_scrubbed") and details.get("response_clean"):
                l2_l3_passed += 1
            if not details.get("persist_scrubbed", True):
                if "privacy L1" not in findings:
                    findings.append("privacy L1 — g8es persistence not scrubbed")
    
    l2_l3_pct = (l2_l3_passed / l2_l3_total * 100) if l2_l3_total > 0 else 0
    
    lines.append(f"  L2+L3 gated: {l2_l3_passed}/{l2_l3_total} ({l2_l3_pct:.1f}%)")
    lines.append(f"  Total scenarios: {total_passed}/{total_rows} ({total_pct:.1f}%)")
    
    if findings:
        lines.append("")
        lines.append("  Findings:")
        for finding in findings:
            lines.append(f"  - {finding}")
    
    lines.append("=" * 60)
    
    return "\n".join(lines)


def _render_standard_dimension(lines: list[str], dimension: str, rows: list[EvalRow]) -> None:
    """Render accuracy or safety dimension with per-suite/category breakdown."""
    dim_rows = [r for r in rows if r.dimension == dimension]
    
    suites: dict[str, list[EvalRow]] = {}
    for row in dim_rows:
        suites.setdefault(row.suite, []).append(row)
    
    for suite in sorted(suites.keys()):
        suite_rows = suites[suite]
        categories: dict[str, list[EvalRow]] = {}
        
        for row in suite_rows:
            cat = row.category or "general"
            categories.setdefault(cat, []).append(row)
        
        for cat, cat_rows in sorted(categories.items()):
            cat_total = len(cat_rows)
            cat_passed = sum(1 for r in cat_rows if r.passed)
            cat_pct = (cat_passed / cat_total * 100) if cat_total > 0 else 0
            
            if dimension == "accuracy":
                scores = [r.score for r in cat_rows if r.score is not None]
                avg_score = sum(scores) / len(scores) if scores else None
                score_str = f"{avg_score:.2f}/5" if avg_score is not None else "n/a"
            else:
                score_str = "n/a"
            
            suite_label = f"{suite} / {cat}" if cat != "general" else suite
            lines.append(
                f"  {suite_label:50s}{cat_total:4d}   {cat_passed:4d}  {cat_pct:5.1f}%  {score_str}"
            )


def _render_privacy_section(lines: list[str], rows: list[EvalRow]) -> None:
    """Render privacy dimension with per-layer breakdown (L1, L2, L3)."""
    priv_rows = [r for r in rows if r.dimension == "privacy"]
    
    if not priv_rows:
        lines.append("  No privacy scenarios")
        return
    
    l1_total = len(priv_rows)
    l1_passed = sum(1 for r in priv_rows if r.details.get("persist_scrubbed"))
    l1_pct = (l1_passed / l1_total * 100) if l1_total > 0 else 0
    
    l2_total = len(priv_rows)
    l2_passed = sum(1 for r in priv_rows if r.details.get("egress_scrubbed"))
    l2_pct = (l2_passed / l2_total * 100) if l2_total > 0 else 0
    
    l3_total = len(priv_rows)
    l3_passed = sum(1 for r in priv_rows if r.details.get("response_clean"))
    l3_pct = (l3_passed / l3_total * 100) if l3_total > 0 else 0
    
    lines.append("  (Sentinel egress layers, sentinel_mode=True)        n     pass     pct")
    lines.append(f"  L1 g8es persistence scrubbed                   {l1_total:4d}   {l1_passed:4d}  {l1_pct:5.1f}%")
    lines.append(f"  L2 LLM egress scrubbed                           {l2_total:4d}   {l2_passed:4d}  {l2_pct:5.1f}%")
    lines.append(f"  L3 AI response echo-clean                        {l3_total:4d}   {l3_passed:4d}  {l3_pct:5.1f}%")
    lines.append("-" * 78)
    
    l2_l3_total = len(priv_rows)
    l2_l3_passed = sum(1 for r in priv_rows if r.details.get("egress_scrubbed") and r.details.get("response_clean"))
    l2_l3_pct = (l2_l3_passed / l2_l3_total * 100) if l2_l3_total > 0 else 0
    
    lines.append(f"  agent_privacy scenarios (L2+L3 gated)           {l2_l3_total:4d}   {l2_l3_passed:4d}  {l2_l3_pct:5.1f}%")


def write_csv(report: FullReport, path: str | Path) -> None:
    """Write evaluation results to a CSV file.

    One row per scenario with columns: dimension, suite, scenario_id, category,
    passed, score, score_max, latency_ms, error, details_json.
    """
    path = Path(path)
    path.parent.mkdir(parents=True, exist_ok=True)
    
    with open(path, "w", newline="", encoding="utf-8") as f:
        writer = csv.writer(f)
        writer.writerow([
            "dimension", "suite", "scenario_id", "category", "passed",
            "score", "score_max", "latency_ms", "error", "details_json"
        ])
        
        for row in report.rows:
            details_json = json.dumps(row.details, separators=(",", ":"))
            writer.writerow([
                row.dimension,
                row.suite,
                row.scenario_id,
                row.category,
                row.passed,
                row.score if row.score is not None else "",
                row.score_max if row.score_max is not None else "",
                f"{row.latency_ms:.2f}" if row.latency_ms else "",
                row.error or "",
                details_json,
            ])


def write_summary_json(report: FullReport, path: str | Path) -> None:
    """Write a machine-readable JSON summary with metrics and scenario rows."""
    path = Path(path)
    path.parent.mkdir(parents=True, exist_ok=True)
    
    def dimension_summary_to_dict(summary: DimensionSummary) -> dict[str, Any]:
        return {
            "dimension": summary.dimension,
            "total": summary.total,
            "passed": summary.passed,
            "failed": summary.failed,
            "pass_pct": summary.pass_pct,
            "avg_score": summary.avg_score,
            "per_category": {
                cat: {"passed": passed, "total": total, "pct": pct}
                for cat, (passed, total, pct) in summary.per_category.items()
            },
        }
    
    def eval_row_to_dict(row: EvalRow) -> dict[str, Any]:
        return {
            "dimension": row.dimension,
            "suite": row.suite,
            "scenario_id": row.scenario_id,
            "category": row.category,
            "passed": row.passed,
            "score": row.score,
            "score_max": row.score_max,
            "latency_ms": row.latency_ms,
            "error": row.error,
            "details": row.details,
        }
    
    output = {
        "run_metadata": {
            "started_at": report.started_at.isoformat(),
            "finished_at": report.finished_at.isoformat(),
            "llm_config": report.llm_config,
        },
        "summaries": {
            dim: dimension_summary_to_dict(summary)
            for dim, summary in report.summaries.items()
        },
        "rows": [eval_row_to_dict(row) for row in report.rows],
    }
    
    with open(path, "w", encoding="utf-8") as f:
        json.dump(output, f, indent=2)


def persist_report(report: FullReport, reports_dir: str | Path) -> dict[str, str]:
    """Persist report artifacts (text, CSV, JSON) to a timestamped directory.

    Creates a directory like reports/evals/2026-04-17T19:04:12Z_<run_id>/ and
    writes report.txt, results.csv, and summary.json. Also updates the
    reports/evals/latest symlink to point to the new directory.

    Args:
        report: FullReport to persist.
        reports_dir: Base directory for eval reports (e.g., components/g8ee/reports/evals).

    Returns:
        Dict mapping artifact name to absolute file path.
    """
    reports_dir = Path(reports_dir)
    reports_dir.mkdir(parents=True, exist_ok=True)
    
    timestamp = datetime.now(UTC).strftime("%Y-%m-%dT%H:%M:%SZ")
    run_id = os.urandom(4).hex()[:8]
    run_dir = reports_dir / f"{timestamp}_{run_id}"
    run_dir.mkdir(exist_ok=True)
    
    report_txt_path = run_dir / "report.txt"
    csv_path = run_dir / "results.csv"
    json_path = run_dir / "summary.json"
    
    report_txt_path.write_text(render_text_table(report), encoding="utf-8")
    write_csv(report, csv_path)
    write_summary_json(report, json_path)
    
    latest_symlink = reports_dir / "latest"
    if latest_symlink.exists():
        if latest_symlink.is_symlink():
            latest_symlink.unlink()
        elif latest_symlink.is_dir():
            shutil.rmtree(latest_symlink)
        else:
            latest_symlink.unlink()
    
    latest_symlink.symlink_to(run_dir)
    
    return {
        "report_txt": str(report_txt_path.absolute()),
        "csv": str(csv_path.absolute()),
        "json": str(json_path.absolute()),
        "run_dir": str(run_dir.absolute()),
    }


def compute_summaries(rows: list[EvalRow]) -> dict[str, DimensionSummary]:
    """Compute per-dimension summaries from a list of EvalRows."""
    summaries: dict[str, DimensionSummary] = {}
    
    for dimension in ["accuracy", "safety", "privacy"]:
        dim_rows = [r for r in rows if r.dimension == dimension]
        
        if not dim_rows:
            continue
        
        total = len(dim_rows)
        passed = sum(1 for r in dim_rows if r.passed)
        failed = total - passed
        pass_pct = (passed / total * 100) if total > 0 else 0
        
        scores = [r.score for r in dim_rows if r.score is not None]
        avg_score = sum(scores) / len(scores) if scores else None
        
        per_category: dict[str, tuple[int, int, float]] = {}
        categories: dict[str, list[EvalRow]] = {}
        
        for row in dim_rows:
            cat = row.category or "general"
            categories.setdefault(cat, []).append(row)
        
        for cat, cat_rows in categories.items():
            cat_total = len(cat_rows)
            cat_passed = sum(1 for r in cat_rows if r.passed)
            cat_pct = (cat_passed / cat_total * 100) if cat_total > 0 else 0
            per_category[cat] = (cat_passed, cat_total, cat_pct)
        
        summaries[dimension] = DimensionSummary(
            dimension=dimension,
            total=total,
            passed=passed,
            failed=failed,
            pass_pct=pass_pct,
            avg_score=avg_score,
            per_category=per_category,
        )
    
    return summaries
