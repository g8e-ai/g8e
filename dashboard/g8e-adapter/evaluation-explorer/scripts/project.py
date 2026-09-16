#!/usr/bin/env python3
"""Project canonical Go-native evaluation reports to public-safe explorer records."""

from __future__ import annotations

import argparse
import hashlib
import json
import sys
from dataclasses import asdict, dataclass
from pathlib import Path

REPOSITORY_ROOT = Path(__file__).resolve().parents[3]
sys.path.insert(0, str(REPOSITORY_ROOT / "protocol" / "python"))

from google.protobuf import json_format  # noqa: E402
from g8e.compliance.v1 import compliance_pb2  # noqa: E402
from g8e.eval.v1 import eval_pb2  # noqa: E402

VIEW_SCHEMA_VERSION = "1.3.0"
DATASET_ID = "native-core-execution-boundary"
REPORT_FILENAME = "report.json"
VERIFICATION_FILENAME = "verification.json"
SUITE_ID = "core-execution-boundary"
SUITE_VERSION = "1.0.0"

ATTEMPT_STATUSES = {
    eval_pb2.EVALUATION_ATTEMPT_STATUS_COMPLETED: "completed",
    eval_pb2.EVALUATION_ATTEMPT_STATUS_REJECTED: "rejected",
    eval_pb2.EVALUATION_ATTEMPT_STATUS_FAILED: "failed",
    eval_pb2.EVALUATION_ATTEMPT_STATUS_UNAVAILABLE: "unavailable",
    eval_pb2.EVALUATION_ATTEMPT_STATUS_UNSUPPORTED: "unsupported",
    eval_pb2.EVALUATION_ATTEMPT_STATUS_INVALID_EVIDENCE: "invalid_evidence",
}
VERDICT_STATUSES = {
    eval_pb2.EVALUATION_VERDICT_STATUS_PASS: "pass",
    eval_pb2.EVALUATION_VERDICT_STATUS_FAIL: "fail",
    eval_pb2.EVALUATION_VERDICT_STATUS_UNAVAILABLE: "unavailable",
    eval_pb2.EVALUATION_VERDICT_STATUS_UNSUPPORTED: "unsupported",
    eval_pb2.EVALUATION_VERDICT_STATUS_INVALID_EVIDENCE: "invalid_evidence",
}
POSTURES = {
    eval_pb2.EVALUATION_GOVERNANCE_POSTURE_DOCTRINE: "doctrine",
    eval_pb2.EVALUATION_GOVERNANCE_POSTURE_CONSENSUS: "consensus",
    eval_pb2.EVALUATION_GOVERNANCE_POSTURE_RATIFY: "ratify",
    eval_pb2.EVALUATION_GOVERNANCE_POSTURE_NOTARY: "notary",
}
LANES = {eval_pb2.EVALUATION_LANE_PLATFORM: "platform"}
METRIC_UNITS = {
    eval_pb2.EVALUATION_METRIC_UNIT_COUNT: "count",
    eval_pb2.EVALUATION_METRIC_UNIT_RATIO: "ratio",
}


@dataclass(frozen=True)
class PublicVerdict:
    assertion_id: str
    assertion_version: str
    status: str


@dataclass(frozen=True)
class PublicScenario:
    scenario_id: str
    scenario_version: str
    status: str
    verdicts: list[PublicVerdict]


@dataclass(frozen=True)
class PublicMetric:
    metric_id: str
    metric_version: str
    numerator: int
    denominator: int
    value: float
    unit: str


@dataclass(frozen=True)
class NativeResult:
    active_posture: str
    lane: str
    summary_status: str
    summary: str
    required_verdict_count: int
    passed_verdict_count: int
    verification_valid: bool
    verification_failure_count: int
    scenarios: list[PublicScenario]
    metrics: list[PublicMetric]


@dataclass(frozen=True)
class MetricValue:
    value: float


@dataclass(frozen=True)
class HeadlineMetrics:
    pass_rate: MetricValue


@dataclass(frozen=True)
class TerminalOutcomes:
    completed: int
    model_failed: int
    grader_failed: int
    invalid_evidence: int
    stopped: int


@dataclass(frozen=True)
class EvaluationSummary:
    schema_version: str
    kind: str
    dataset_id: str
    quality_state: str
    observed_at: str
    run_id: str
    suite_id: str
    arm: str
    evaluation_unit: str
    lifecycle_state: str
    assignment_total: int
    assignment_completed: int
    assignment_failed: int
    terminal_outcomes: TerminalOutcomes
    started_at: str
    ended_at: str
    elapsed_seconds: float
    verifier_state: str
    headline_metrics: HeadlineMetrics
    native_result: NativeResult


@dataclass(frozen=True)
class CatalogSnapshot:
    schema_version: str
    kind: str
    dataset_id: str
    quality_state: str
    observed_at: str
    dataset_kind: str
    title: str
    description: str
    limitations: list[str]
    model_count: int
    evaluated_count: int
    suite_count: int
    run_count: int
    assignment_count: int
    provider_request_count: int
    provider_token_count: int
    retry_count: int
    verifier_passed_count: int
    verifier_failed_count: int
    generated_at: str


def read_canonical_message(path: Path, message):
    body = path.read_bytes()
    try:
        decoded = json.loads(body)
    except (UnicodeDecodeError, json.JSONDecodeError) as error:
        raise ValueError(f"{path.name} is not valid JSON: {error}") from error
    compact = json.dumps(decoded, separators=(",", ":"), ensure_ascii=False).encode()
    if compact != body:
        raise ValueError(f"{path.name} is not canonical compact JSON")
    try:
        json_format.Parse(body.decode(), message, ignore_unknown_fields=False)
    except json_format.ParseError as error:
        raise ValueError(f"{path.name} is not canonical protocol JSON: {error}") from error
    return body, message


def validate_and_project(run_dir: Path) -> EvaluationSummary:
    verification_body, verification = read_canonical_message(run_dir / VERIFICATION_FILENAME, compliance_pb2.ComplianceVerificationReport())
    _, report = read_canonical_message(run_dir / REPORT_FILENAME, eval_pb2.EvaluationReport())
    run = report.run
    if not run.run_id or run.run_id != run_dir.name or verification.report_id != run.run_id:
        raise ValueError("run directory, report, and verification report IDs must match")
    if report.schema_version != "1.0.0" or run.schema_version != report.schema_version:
        raise ValueError("native evaluation schema version is unsupported")
    if run.suite_ref.id != SUITE_ID or run.suite_ref.version != SUITE_VERSION:
        raise ValueError("native evaluation suite is unsupported")
    if not run.HasField("final_verification_report_ref"):
        raise ValueError("final verification reference is required")
    verification_ref = run.final_verification_report_ref
    digest = hashlib.sha256(verification_body).hexdigest()
    if verification_ref.run_id != run.run_id or verification_ref.artifact_type != "evaluation-verification" or verification_ref.sha256 != digest or verification_ref.artifact_id != f"evaluation-verification:sha256:{digest}":
        raise ValueError("final verification reference does not bind verification.json")
    if verification.valid != (len(verification.failures) == 0 and all(check.status == compliance_pb2.VERIFICATION_CHECK_STATUS_PASSED and not check.failures for check in verification.checks)):
        raise ValueError("verification validity and failures disagree")
    if run.active_posture not in POSTURES or run.lane not in LANES:
        raise ValueError("native posture or lane is unsupported")
    if report.summary_status not in VERDICT_STATUSES:
        raise ValueError("native summary status is unsupported")

    assertions = {assertion.assertion_id: assertion for assertion in report.assertions}
    verdicts = {verdict.verdict_id: verdict for verdict in report.verdicts}
    if len(assertions) != len(report.assertions) or len(verdicts) != len(report.verdicts):
        raise ValueError("assertion and verdict IDs must be unique")
    passed = sum(verdict.status == eval_pb2.EVALUATION_VERDICT_STATUS_PASS for verdict in report.verdicts)
    if report.required_verdict_count != len(report.verdicts) or report.passed_verdict_count != passed:
        raise ValueError("native verdict counts are inconsistent")
    if report.summary_status == eval_pb2.EVALUATION_VERDICT_STATUS_PASS and passed != len(report.verdicts):
        raise ValueError("passing summary requires every verdict to pass")

    projected_scenarios: list[PublicScenario] = []
    assigned_verdicts: set[str] = set()
    for attempt in report.attempts:
        if attempt.run_id != run.run_id or attempt.status not in ATTEMPT_STATUSES or not attempt.scenario_ref.id or not attempt.scenario_ref.version:
            raise ValueError("native attempt binding or status is invalid")
        scenario_verdicts: list[PublicVerdict] = []
        for verdict_id in attempt.verdict_refs:
            if verdict_id in assigned_verdicts or verdict_id not in verdicts:
                raise ValueError("attempt verdict references must be unique and complete")
            verdict = verdicts[verdict_id]
            assertion = assertions.get(verdict.assertion_ref.id)
            if assertion is None or assertion.assertion_version != verdict.assertion_ref.version or verdict.status not in VERDICT_STATUSES:
                raise ValueError("verdict assertion binding or status is invalid")
            assigned_verdicts.add(verdict_id)
            scenario_verdicts.append(PublicVerdict(assertion.assertion_id, assertion.assertion_version, VERDICT_STATUSES[verdict.status]))
        projected_scenarios.append(PublicScenario(attempt.scenario_ref.id, attempt.scenario_ref.version, ATTEMPT_STATUSES[attempt.status], scenario_verdicts))
    if assigned_verdicts != set(verdicts):
        raise ValueError("every verdict must belong to exactly one scenario")
    if set(run.attempt_refs) != {attempt.attempt_id for attempt in report.attempts}:
        raise ValueError("run attempt references are inconsistent")

    projected_metrics: list[PublicMetric] = []
    for metric in report.metrics:
        if metric.unit not in METRIC_UNITS or metric.denominator < 0 or metric.numerator < 0 or metric.numerator > metric.denominator or any(reference not in verdicts for reference in metric.source_verdict_refs):
            raise ValueError("native metric is inconsistent")
        projected_metrics.append(PublicMetric(metric.metric_id, metric.metric_version, metric.numerator, metric.denominator, metric.value, METRIC_UNITS[metric.unit]))

    started_at = run.started_at.ToJsonString()
    ended_at = run.completed_at.ToJsonString()
    elapsed = max(0.0, (run.completed_at.ToDatetime() - run.started_at.ToDatetime()).total_seconds())
    scenario_failures = sum(attempt.status not in (eval_pb2.EVALUATION_ATTEMPT_STATUS_COMPLETED, eval_pb2.EVALUATION_ATTEMPT_STATUS_REJECTED) for attempt in report.attempts)
    summary = f"{passed}/{report.required_verdict_count} required invariants passed"
    native = NativeResult(
        POSTURES[run.active_posture], LANES[run.lane], VERDICT_STATUSES[report.summary_status], summary,
        report.required_verdict_count, report.passed_verdict_count, verification.valid, len(verification.failures),
        projected_scenarios, projected_metrics,
    )
    terminal_status = "completed" if report.summary_status == eval_pb2.EVALUATION_VERDICT_STATUS_PASS and verification.valid else "failed"
    return EvaluationSummary(
        VIEW_SCHEMA_VERSION, "evaluation_summary", DATASET_ID, "verified_public" if verification.valid else "terminal_failed",
        ended_at, run.run_id, f"{SUITE_ID}@{SUITE_VERSION}", "platform", "system", terminal_status,
        len(report.attempts), len(report.attempts) - scenario_failures, scenario_failures,
        TerminalOutcomes(len(report.attempts) - scenario_failures, scenario_failures, 0, 0, 0),
        started_at, ended_at, elapsed, "passed" if verification.valid else "failed",
        HeadlineMetrics(MetricValue(passed / report.required_verdict_count if report.required_verdict_count else 0.0)), native,
    )


def emit(records: list[CatalogSnapshot | EvaluationSummary], output: Path) -> None:
    output.parent.mkdir(parents=True, exist_ok=True)
    with output.open("w", encoding="utf-8") as stream:
        for record in records:
            record_bytes = json.dumps(asdict(record), sort_keys=True, separators=(",", ":"), ensure_ascii=False)
            stream.write(json.dumps({"record_type": "projection", "record_bytes": record_bytes}, sort_keys=True, separators=(",", ":")) + "\n")


def main() -> int:
    parser = argparse.ArgumentParser(description="Project canonical Go-native evaluation reports")
    parser.add_argument("--run-dir", action="append", required=True, help="Native run directory containing report.json and verification.json")
    parser.add_argument("--output", required=True, help="Mirror-ready public record JSONL")
    args = parser.parse_args()
    try:
        summaries = sorted((validate_and_project(Path(path)) for path in args.run_dir), key=lambda item: item.run_id)
        latest = max(summary.observed_at for summary in summaries)
        catalog = CatalogSnapshot(
            VIEW_SCHEMA_VERSION, "catalog_snapshot", DATASET_ID,
            "verified_public" if all(summary.native_result.verification_valid for summary in summaries) else "terminal_failed",
            latest, "live_run", "Go-native execution-boundary evaluations",
            "Public-safe projections of canonical Go-native core execution-boundary reports.", [], 0, 0, 1,
            len(summaries), sum(summary.assignment_total for summary in summaries), 0, 0, 0,
            sum(summary.native_result.verification_valid for summary in summaries),
            sum(not summary.native_result.verification_valid for summary in summaries), latest,
        )
        emit([catalog, *summaries], Path(args.output))
    except (OSError, ValueError) as error:
        print(f"native evaluation projection failed: {error}", file=sys.stderr)
        return 1
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
