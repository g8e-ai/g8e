#!/usr/bin/env python3
# Copyright (c) 2026 Lateralus Labs, LLC.
# Use of this source code is governed by the Business Source License
# included in the LICENSE file.
#
# As of the Change Date listed in the LICENSE file, this software is
# released under the Apache License, 2.0.

"""Generate the root README.md from a narrative template and a public proof snapshot.

Usage:
    python3 scripts/generate_readme.py
    python3 scripts/generate_readme.py --check

The script is offline, deterministic, credential-free, and uses only the Python
standard library. It validates the selected public proof snapshot, computes bounded
projections, and renders the template. Run `make readme` to regenerate; use
`make readme-check` to detect drift.
"""

from __future__ import annotations

import argparse
import hashlib
import json
import math
import os
import re
import sys
from dataclasses import dataclass
from datetime import datetime
from pathlib import Path
from typing import Any


SUPPORTED_PUB_SCHEMAS = {"1.0.0"}
SUPPORTED_EVAL_SCHEMAS = {"1.33.0"}

MARKER_PATTERN = re.compile(r"\{\{([A-Z_][A-Z0-9_]*)\}\}")

MARKERS = {
    "GENERATED_HEADER",
    "CI_BADGES",
    "EVIDENCE_IDENTITY",
    "EVAL_METRICS",
    "RECEIPT_PROOF",
    "GOVERNANCE_PROOF",
    "DEMO_PROOF",
    "CI_REPRODUCIBILITY",
}

SAFE_LINK_LABELS = {
    "CI",
    "Latest Release",
}
SAFE_LINK_KINDS = {
    "workflow_status",
    "release_link",
}

CLAIM_LABELS = {
    "bounded_receipt_verification",
    "verified_metric_outcomes",
    "synthetic_demo_data",
    "curated_ifeval_subset",
    "deterministic_governance_chain",
    "live_ci_status",
    "reproducible_local_verification",
}


class ReadmeError(Exception):
    """Raised when the public snapshot or template is invalid."""


@dataclass(frozen=True)
class EvalRunRef:
    run_id: str
    manifest_path: str
    manifest_sha256: str
    attempts_path: str
    attempts_sha256: str
    metrics_path: str
    metrics_sha256: str
    receipts_path: str
    receipts_sha256: str
    stages_path: str
    stages_sha256: str


@dataclass(frozen=True)
class ReceiptVerificationRef:
    result_path: str
    result_sha256: str
    scope: str


@dataclass(frozen=True)
class DemoReportRef:
    run_id: str
    environment: str
    scenario_id: str
    report_path: str
    report_sha256: str


@dataclass(frozen=True)
class CILink:
    label: str
    url: str
    kind: str


@dataclass(frozen=True)
class PublicationManifest:
    publication_schema_version: str
    readme_evidence_version: str
    evidence_cutoff: str
    platform_version: str
    eval_runs: tuple[EvalRunRef, ...]
    receipt_verification: ReceiptVerificationRef
    demo_reports: tuple[DemoReportRef, ...]
    ci_links: tuple[CILink, ...]
    claim_labels: tuple[str, ...]
    caveats: tuple[str, ...]


@dataclass(frozen=True)
class MetricRow:
    schema_version: str
    run_id: str
    metric_id: str
    metric_version: str
    arm_id: str
    task_id: str
    eligible: bool
    denominator_contribution: int
    value: float
    unit: str
    verification_status: str
    grader_class: str
    evidence_ref: str


@dataclass(frozen=True)
class AttemptRow:
    schema_version: str
    run_id: str
    attempt_id: str
    task_id: str
    arm_id: str
    status: str
    eligible: bool
    receipt_id: str | None


@dataclass(frozen=True)
class ReceiptRow:
    schema_version: str
    run_id: str
    receipt_id: str
    attempt_id: str
    arm_id: str
    action_class: str
    signer_key_id: str
    signature_digest: str
    persistence_attestation_digest: str
    source_artifact_hash: str


@dataclass(frozen=True)
class StageRow:
    schema_version: str
    run_id: str
    stage_id: str
    attempt_id: str
    arm_id: str
    layer: str
    outcome: str
    detail: str


@dataclass(frozen=True)
class LoadedEvalRun:
    run_id: str
    manifest: dict[str, Any]
    attempts: tuple[AttemptRow, ...]
    metrics: tuple[MetricRow, ...]
    receipts: tuple[ReceiptRow, ...]
    stages: tuple[StageRow, ...]


@dataclass(frozen=True)
class ReceiptVerificationResult:
    schema_version: str
    run_id: str
    verified_at: str
    verifier_version: str
    scope: str
    total_receipts: int
    verified_signatures: int
    verified_persistence: int
    failed_signatures: int
    failed_persistence: int
    missing_keys: int
    distinct_signer_key_ids: tuple[str, ...]
    receipt_bound_eligible_attempts: int
    sample_receipt_fingerprints: tuple[dict[str, str], ...]


@dataclass(frozen=True)
class DemoReport:
    run_id: str
    verified_at: str
    verifier_id: str
    verifier_version: str
    valid: bool
    environment: str
    scenario_id: str
    failures: tuple[str, ...]


@dataclass(frozen=True)
class MetricProjection:
    metric_id: str
    metric_version: str
    unit: str
    arm_id: str
    numerator: float
    denominator: int
    rate: float
    verification_status: str
    task_count: int


@dataclass(frozen=True)
class ProofSnapshot:
    manifest: PublicationManifest
    eval_runs: dict[str, LoadedEvalRun]
    receipt_verification: ReceiptVerificationResult
    demo_reports: tuple[DemoReport, ...]
    artifact_paths: set[str]


def _repo_root() -> Path:
    script = Path(__file__).resolve()
    return script.parent.parent


def _sha256_file(path: Path) -> str:
    h = hashlib.sha256()
    with path.open("rb") as f:
        while True:
            chunk = f.read(8192)
            if not chunk:
                break
            h.update(chunk)
    return h.hexdigest()


def _load_json(path: Path) -> Any:
    with path.open("r", encoding="utf-8") as f:
        return json.load(f)


def _load_jsonl(path: Path) -> list[dict[str, Any]]:
    rows: list[dict[str, Any]] = []
    with path.open("r", encoding="utf-8") as f:
        for line in f:
            line = line.strip()
            if not line:
                continue
            rows.append(json.loads(line))
    return rows


def _safe_relative_path(snapshot_dir: Path, rel: str) -> Path:
    if rel.startswith("/") or rel.startswith("\\"):
        raise ReadmeError(f"absolute path not allowed: {rel}")
    if ".." in Path(rel).parts:
        raise ReadmeError(f"path traversal not allowed: {rel}")
    candidate = (snapshot_dir / rel).resolve()
    snapshot_resolved = snapshot_dir.resolve()
    try:
        candidate.relative_to(snapshot_resolved)
    except ValueError as exc:
        raise ReadmeError(f"path escapes snapshot directory: {rel}") from exc
    if candidate.is_symlink():
        raise ReadmeError(f"symlink not allowed: {rel}")
    return candidate


def _validate_sha256(path: Path, expected: str, label: str) -> None:
    if not path.exists():
        raise ReadmeError(f"missing artifact: {label} ({path})")
    actual = _sha256_file(path)
    if actual != expected:
        raise ReadmeError(
            f"checksum mismatch for {label}: expected {expected}, got {actual}"
        )


def _require_field(d: dict[str, Any], key: str, label: str) -> Any:
    if key not in d:
        raise ReadmeError(f"missing field {key} in {label}")
    return d[key]


def _require_bool(d: dict[str, Any], key: str, label: str) -> bool:
    value = _require_field(d, key, label)
    if not isinstance(value, bool):
        raise ReadmeError(f"field {key} in {label} must be a boolean")
    return value


def _require_int(d: dict[str, Any], key: str, label: str, minimum: int = 0) -> int:
    value = _require_field(d, key, label)
    if not isinstance(value, int) or isinstance(value, bool):
        raise ReadmeError(f"field {key} in {label} must be an integer")
    if value < minimum:
        raise ReadmeError(f"field {key} in {label} must be >= {minimum}, got {value}")
    return value


def _require_str(d: dict[str, Any], key: str, label: str, allow_empty: bool = False) -> str:
    value = _require_field(d, key, label)
    if not isinstance(value, str):
        raise ReadmeError(f"field {key} in {label} must be a string")
    if not value and not allow_empty:
        raise ReadmeError(f"field {key} in {label} must not be empty")
    return value


def _require_list(d: dict[str, Any], key: str, label: str) -> list[Any]:
    value = _require_field(d, key, label)
    if not isinstance(value, list):
        raise ReadmeError(f"field {key} in {label} must be a list")
    return value


def _require_finite(value: float, label: str) -> None:
    if not isinstance(value, (int, float)) or isinstance(value, bool):
        raise ReadmeError(f"{label} must be a number")
    if math.isnan(value) or math.isinf(value):
        raise ReadmeError(f"{label} must be finite, got {value}")


def _parse_timestamp(value: str, label: str) -> None:
    if value.endswith("Z"):
        value = value[:-1] + "+00:00"
    try:
        datetime.fromisoformat(value)
    except ValueError as exc:
        raise ReadmeError(f"invalid timestamp {label}: {value}") from exc


def _manifest_from_dict(d: dict[str, Any], snapshot_dir: Path) -> PublicationManifest:
    pub_version = _require_str(d, "publication_schema_version", "index.json")
    if pub_version not in SUPPORTED_PUB_SCHEMAS:
        raise ReadmeError(f"unsupported publication_schema_version: {pub_version}")

    readme_version = _require_str(d, "readme_evidence_version", "index.json")
    cutoff = _require_str(d, "evidence_cutoff", "index.json")
    _parse_timestamp(cutoff, "evidence_cutoff")
    platform_version = _require_str(d, "platform_version", "index.json")

    raw_eval_runs = _require_list(d, "eval_runs", "index.json")
    if not raw_eval_runs:
        raise ReadmeError("index.json must contain at least one eval run")

    seen_run_ids: set[str] = set()
    eval_runs: list[EvalRunRef] = []
    for idx, raw in enumerate(raw_eval_runs):
        label = f"index.json eval_runs[{idx}]"
        run_id = _require_str(raw, "run_id", label)
        if run_id in seen_run_ids:
            raise ReadmeError(f"duplicate run_id in index.json: {run_id}")
        seen_run_ids.add(run_id)
        manifest_path = _require_str(raw, "manifest_path", label)
        manifest_sha256 = _require_str(raw, "manifest_sha256", label)
        attempts_path = _require_str(raw, "attempts_path", label)
        attempts_sha256 = _require_str(raw, "attempts_sha256", label)
        metrics_path = _require_str(raw, "metrics_path", label)
        metrics_sha256 = _require_str(raw, "metrics_sha256", label)
        receipts_path = _require_str(raw, "receipts_path", label)
        receipts_sha256 = _require_str(raw, "receipts_sha256", label)
        stages_path = _require_str(raw, "stages_path", label)
        stages_sha256 = _require_str(raw, "stages_sha256", label)
        eval_runs.append(
            EvalRunRef(
                run_id=run_id,
                manifest_path=manifest_path,
                manifest_sha256=manifest_sha256,
                attempts_path=attempts_path,
                attempts_sha256=attempts_sha256,
                metrics_path=metrics_path,
                metrics_sha256=metrics_sha256,
                receipts_path=receipts_path,
                receipts_sha256=receipts_sha256,
                stages_path=stages_path,
                stages_sha256=stages_sha256,
            )
        )

    raw_receipt = _require_field(d, "receipt_verification", "index.json")
    if not isinstance(raw_receipt, dict):
        raise ReadmeError("receipt_verification must be an object")
    receipt_verification = ReceiptVerificationRef(
        result_path=_require_str(raw_receipt, "result_path", "index.json receipt_verification"),
        result_sha256=_require_str(raw_receipt, "result_sha256", "index.json receipt_verification"),
        scope=_require_str(raw_receipt, "scope", "index.json receipt_verification"),
    )

    raw_demos = _require_list(d, "demo_reports", "index.json")
    seen_demo_ids: set[str] = set()
    demo_reports: list[DemoReportRef] = []
    for idx, raw in enumerate(raw_demos):
        label = f"index.json demo_reports[{idx}]"
        run_id = _require_str(raw, "run_id", label)
        if run_id in seen_demo_ids:
            raise ReadmeError(f"duplicate demo run_id in index.json: {run_id}")
        seen_demo_ids.add(run_id)
        demo_reports.append(
            DemoReportRef(
                run_id=run_id,
                environment=_require_str(raw, "environment", label),
                scenario_id=_require_str(raw, "scenario_id", label),
                report_path=_require_str(raw, "report_path", label),
                report_sha256=_require_str(raw, "report_sha256", label),
            )
        )

    raw_ci = _require_list(d, "ci_links", "index.json")
    ci_links: list[CILink] = []
    for idx, raw in enumerate(raw_ci):
        label = f"index.json ci_links[{idx}]"
        link = CILink(
            label=_require_str(raw, "label", label),
            url=_require_str(raw, "url", label),
            kind=_require_str(raw, "kind", label),
        )
        if link.kind not in SAFE_LINK_KINDS:
            raise ReadmeError(f"unsupported ci link kind: {link.kind}")
        if link.label not in SAFE_LINK_LABELS:
            raise ReadmeError(f"unsupported ci link label: {link.label}")
        ci_links.append(link)

    raw_claims = _require_list(d, "claim_labels", "index.json")
    for claim in raw_claims:
        if not isinstance(claim, str) or claim not in CLAIM_LABELS:
            raise ReadmeError(f"unsupported claim label: {claim}")

    raw_caveats = _require_list(d, "caveats", "index.json")
    for caveat in raw_caveats:
        if not isinstance(caveat, str):
            raise ReadmeError("caveats must be strings")

    return PublicationManifest(
        publication_schema_version=pub_version,
        readme_evidence_version=readme_version,
        evidence_cutoff=cutoff,
        platform_version=platform_version,
        eval_runs=tuple(eval_runs),
        receipt_verification=receipt_verification,
        demo_reports=tuple(demo_reports),
        ci_links=tuple(ci_links),
        claim_labels=tuple(raw_claims),
        caveats=tuple(raw_caveats),
    )


def _load_eval_run(ref: EvalRunRef, snapshot_dir: Path) -> LoadedEvalRun:
    manifest_path = _safe_relative_path(snapshot_dir, ref.manifest_path)
    attempts_path = _safe_relative_path(snapshot_dir, ref.attempts_path)
    metrics_path = _safe_relative_path(snapshot_dir, ref.metrics_path)
    receipts_path = _safe_relative_path(snapshot_dir, ref.receipts_path)
    stages_path = _safe_relative_path(snapshot_dir, ref.stages_path)

    _validate_sha256(manifest_path, ref.manifest_sha256, f"manifest.json for {ref.run_id}")
    _validate_sha256(attempts_path, ref.attempts_sha256, f"attempts.jsonl for {ref.run_id}")
    _validate_sha256(metrics_path, ref.metrics_sha256, f"metrics.jsonl for {ref.run_id}")
    _validate_sha256(receipts_path, ref.receipts_sha256, f"receipts.jsonl for {ref.run_id}")
    _validate_sha256(stages_path, ref.stages_sha256, f"stages.jsonl for {ref.run_id}")

    manifest = _load_json(manifest_path)
    if not isinstance(manifest, dict):
        raise ReadmeError(f"manifest for {ref.run_id} must be an object")
    manifest_run_id = _require_str(manifest, "run_id", f"manifest for {ref.run_id}")
    if manifest_run_id != ref.run_id:
        raise ReadmeError(
            f"manifest run_id mismatch: {manifest_run_id} != {ref.run_id}"
        )
    manifest_schema = _require_str(manifest, "schema_version", f"manifest for {ref.run_id}")
    if manifest_schema not in SUPPORTED_EVAL_SCHEMAS:
        raise ReadmeError(
            f"unsupported eval schema version {manifest_schema} in {ref.run_id}"
        )

    attempts = _parse_attempts(attempts_path, ref.run_id)
    metrics = _parse_metrics(metrics_path, ref.run_id)
    receipts = _parse_receipts(receipts_path, ref.run_id)
    stages = _parse_stages(stages_path, ref.run_id)

    seen_attempt_ids = {a.attempt_id for a in attempts}
    seen_receipt_ids = {r.receipt_id for r in receipts}
    seen_stage_ids = {s.stage_id for s in stages}

    for m in metrics:
        if m.run_id != ref.run_id:
            raise ReadmeError(f"metric row references foreign run_id {m.run_id}")
        if m.evidence_ref.startswith("attempts/") and m.evidence_ref.split("/")[-1] not in seen_attempt_ids:
            raise ReadmeError(f"metric references missing attempt {m.evidence_ref}")
        if m.evidence_ref.startswith("receipts/") and m.evidence_ref.split("/")[-1] not in seen_receipt_ids:
            raise ReadmeError(f"metric references missing receipt {m.evidence_ref}")
        if m.evidence_ref.startswith("stages/") and m.evidence_ref.split("/")[-1] not in seen_stage_ids:
            raise ReadmeError(f"metric references missing stage {m.evidence_ref}")

    for r in receipts:
        if r.run_id != ref.run_id:
            raise ReadmeError(f"receipt row references foreign run_id {r.run_id}")
        if r.attempt_id not in seen_attempt_ids:
            raise ReadmeError(f"receipt {r.receipt_id} references missing attempt {r.attempt_id}")

    for s in stages:
        if s.run_id != ref.run_id:
            raise ReadmeError(f"stage row references foreign run_id {s.run_id}")
        if s.attempt_id not in seen_attempt_ids:
            raise ReadmeError(f"stage {s.stage_id} references missing attempt {s.attempt_id}")

    return LoadedEvalRun(
        run_id=ref.run_id,
        manifest=manifest,
        attempts=attempts,
        metrics=metrics,
        receipts=receipts,
        stages=stages,
    )


def _parse_attempts(path: Path, run_id: str) -> tuple[AttemptRow, ...]:
    rows: list[AttemptRow] = []
    seen: set[str] = set()
    for idx, raw in enumerate(_load_jsonl(path)):
        label = f"attempts.jsonl[{idx}] for {run_id}"
        if not isinstance(raw, dict):
            raise ReadmeError(f"{label} is not an object")
        attempt_id = _require_str(raw, "attempt_id", label)
        if attempt_id in seen:
            raise ReadmeError(f"duplicate attempt_id {attempt_id} in {run_id}")
        seen.add(attempt_id)
        # Accept both fixture (status) and real (terminal_status) field names.
        status = raw.get("terminal_status") or _require_str(raw, "status", label)
        # Derive eligible from terminal_status if the field is absent.
        if "eligible" in raw:
            eligible = _require_bool(raw, "eligible", label)
        else:
            eligible = status == "completed"
        # Accept both fixture (receipt_id) and real (receipt_refs) field names.
        receipt_id = raw.get("receipt_id")
        if receipt_id is None:
            receipt_refs = raw.get("receipt_refs", [])
            if isinstance(receipt_refs, list) and receipt_refs:
                receipt_id = receipt_refs[0]
        row = AttemptRow(
            schema_version=_require_str(raw, "schema_version", label),
            run_id=_require_str(raw, "run_id", label),
            attempt_id=attempt_id,
            task_id=_require_str(raw, "task_id", label),
            arm_id=_require_str(raw, "arm_id", label),
            status=status,
            eligible=eligible,
            receipt_id=receipt_id,
        )
        if row.schema_version not in SUPPORTED_EVAL_SCHEMAS:
            raise ReadmeError(f"unsupported schema version in {label}")
        if row.run_id != run_id:
            raise ReadmeError(f"attempt row run_id mismatch in {label}")
        rows.append(row)
    return tuple(rows)


def _parse_metrics(path: Path, run_id: str) -> tuple[MetricRow, ...]:
    rows: list[MetricRow] = []
    seen: set[tuple[str, str, str, str]] = set()
    for idx, raw in enumerate(_load_jsonl(path)):
        label = f"metrics.jsonl[{idx}] for {run_id}"
        if not isinstance(raw, dict):
            raise ReadmeError(f"{label} is not an object")
        key = (
            _require_str(raw, "metric_id", label),
            _require_str(raw, "metric_version", label),
            _require_str(raw, "arm_id", label),
            _require_str(raw, "task_id", label),
        )
        if key in seen:
            raise ReadmeError(f"duplicate metric row {key} in {run_id}")
        seen.add(key)
        value = _require_field(raw, "value", label)
        _require_finite(value, f"{label} value")
        denom = _require_int(raw, "denominator_contribution", label, minimum=0)
        # Accept both fixture (evidence_ref string) and real (evidence_refs list).
        if "evidence_ref" in raw:
            evidence_ref = _require_str(raw, "evidence_ref", label)
        else:
            evidence_refs = raw.get("evidence_refs", [])
            evidence_ref = evidence_refs[0] if isinstance(evidence_refs, list) and evidence_refs else ""
        row = MetricRow(
            schema_version=_require_str(raw, "schema_version", label),
            run_id=_require_str(raw, "run_id", label),
            metric_id=key[0],
            metric_version=key[1],
            arm_id=key[2],
            task_id=key[3],
            eligible=_require_bool(raw, "eligible", label),
            denominator_contribution=denom,
            value=float(value),
            unit=_require_str(raw, "unit", label),
            verification_status=_require_str(raw, "verification_status", label),
            grader_class=_require_str(raw, "grader_class", label),
            evidence_ref=evidence_ref,
        )
        if row.schema_version not in SUPPORTED_EVAL_SCHEMAS:
            raise ReadmeError(f"unsupported schema version in {label}")
        if row.run_id != run_id:
            raise ReadmeError(f"metric row run_id mismatch in {label}")
        if row.eligible and row.denominator_contribution <= 0:
            raise ReadmeError(f"eligible metric must have positive denominator in {label}")
        rows.append(row)
    return tuple(rows)


def _parse_receipts(path: Path, run_id: str) -> tuple[ReceiptRow, ...]:
    rows: list[ReceiptRow] = []
    seen: set[str] = set()
    for idx, raw in enumerate(_load_jsonl(path)):
        label = f"receipts.jsonl[{idx}] for {run_id}"
        if not isinstance(raw, dict):
            raise ReadmeError(f"{label} is not an object")
        receipt_id = _require_str(raw, "receipt_id", label)
        if receipt_id in seen:
            raise ReadmeError(f"duplicate receipt_id {receipt_id} in {run_id}")
        seen.add(receipt_id)
        row = ReceiptRow(
            schema_version=_require_str(raw, "schema_version", label),
            run_id=_require_str(raw, "run_id", label),
            receipt_id=receipt_id,
            attempt_id=_require_str(raw, "attempt_id", label),
            arm_id=_require_str(raw, "arm_id", label),
            action_class=_require_str(raw, "action_class", label),
            signer_key_id=_require_str(raw, "signer_key_id", label),
            signature_digest=_require_str(raw, "signature_digest", label),
            persistence_attestation_digest=_require_str(raw, "persistence_attestation_digest", label),
            source_artifact_hash=_require_str(raw, "source_artifact_hash", label),
        )
        if row.schema_version not in SUPPORTED_EVAL_SCHEMAS:
            raise ReadmeError(f"unsupported schema version in {label}")
        if row.run_id != run_id:
            raise ReadmeError(f"receipt row run_id mismatch in {label}")
        rows.append(row)
    return tuple(rows)


def _parse_stages(path: Path, run_id: str) -> tuple[StageRow, ...]:
    rows: list[StageRow] = []
    seen: set[str] = set()
    for idx, raw in enumerate(_load_jsonl(path)):
        label = f"stages.jsonl[{idx}] for {run_id}"
        if not isinstance(raw, dict):
            raise ReadmeError(f"{label} is not an object")
        stage_id = _require_str(raw, "stage_id", label)
        if stage_id in seen:
            raise ReadmeError(f"duplicate stage_id {stage_id} in {run_id}")
        seen.add(stage_id)
        # Accept both fixture (layer/outcome/detail) and real (kind/decision) field names.
        layer = raw.get("layer") or _require_str(raw, "kind", label)
        # decision can be null in real stage records; fall back to kind as outcome.
        outcome = raw.get("outcome")
        if outcome is None:
            decision = raw.get("decision")
            outcome = decision if isinstance(decision, str) and decision else layer
        detail = raw.get("detail", "")
        if not detail:
            source = raw.get("source", "")
            detail = source if isinstance(source, str) and source else outcome
        # Accept fixture (arm_id) or derive from attempt_id (<run_id>:<task>:<arm>:<rep>).
        arm_id = raw.get("arm_id")
        if not arm_id:
            attempt_id_str = _require_str(raw, "attempt_id", label)
            parts = attempt_id_str.split(":")
            arm_id = parts[2] if len(parts) >= 4 else "unknown"
        row = StageRow(
            schema_version=_require_str(raw, "schema_version", label),
            run_id=_require_str(raw, "run_id", label),
            stage_id=stage_id,
            attempt_id=_require_str(raw, "attempt_id", label),
            arm_id=arm_id,
            layer=layer,
            outcome=outcome,
            detail=detail,
        )
        if row.schema_version not in SUPPORTED_EVAL_SCHEMAS:
            raise ReadmeError(f"unsupported schema version in {label}")
        if row.run_id != run_id:
            raise ReadmeError(f"stage row run_id mismatch in {label}")
        rows.append(row)
    return tuple(rows)


def _load_receipt_verification(
    ref: ReceiptVerificationRef, snapshot_dir: Path
) -> ReceiptVerificationResult:
    path = _safe_relative_path(snapshot_dir, ref.result_path)
    _validate_sha256(path, ref.result_sha256, "receipt-verification.json")
    raw = _load_json(path)
    if not isinstance(raw, dict):
        raise ReadmeError("receipt-verification.json must be an object")

    total = _require_int(raw, "total_receipts", "receipt-verification.json", minimum=0)
    verified = _require_int(raw, "verified_signatures", "receipt-verification.json", minimum=0)
    persistence = _require_int(raw, "verified_persistence", "receipt-verification.json", minimum=0)
    failed_sig = _require_int(raw, "failed_signatures", "receipt-verification.json", minimum=0)
    failed_persist = _require_int(raw, "failed_persistence", "receipt-verification.json", minimum=0)
    missing = _require_int(raw, "missing_keys", "receipt-verification.json", minimum=0)
    bound = _require_int(raw, "receipt_bound_eligible_attempts", "receipt-verification.json", minimum=0)

    if verified > total:
        raise ReadmeError("verified_signatures exceeds total_receipts")
    if persistence > total:
        raise ReadmeError("verified_persistence exceeds total_receipts")
    if failed_sig > total:
        raise ReadmeError("failed_signatures exceeds total_receipts")
    if failed_persist > total:
        raise ReadmeError("failed_persistence exceeds total_receipts")
    if missing > total:
        raise ReadmeError("missing_keys exceeds total_receipts")

    raw_keys = _require_list(raw, "distinct_signer_key_ids", "receipt-verification.json")
    for key in raw_keys:
        if not isinstance(key, str):
            raise ReadmeError("distinct_signer_key_ids must be strings")

    raw_samples = _require_list(raw, "sample_receipt_fingerprints", "receipt-verification.json")
    samples: list[dict[str, str]] = []
    for idx, sample in enumerate(raw_samples):
        if not isinstance(sample, dict):
            raise ReadmeError(f"sample_receipt_fingerprints[{idx}] must be an object")
        samples.append({str(k): str(v) for k, v in sample.items()})

    return ReceiptVerificationResult(
        schema_version=_require_str(raw, "schema_version", "receipt-verification.json"),
        run_id=_require_str(raw, "run_id", "receipt-verification.json"),
        verified_at=_require_str(raw, "verified_at", "receipt-verification.json"),
        verifier_version=_require_str(raw, "verifier_version", "receipt-verification.json"),
        scope=_require_str(raw, "scope", "receipt-verification.json"),
        total_receipts=total,
        verified_signatures=verified,
        verified_persistence=persistence,
        failed_signatures=failed_sig,
        failed_persistence=failed_persist,
        missing_keys=missing,
        distinct_signer_key_ids=tuple(raw_keys),
        receipt_bound_eligible_attempts=bound,
        sample_receipt_fingerprints=tuple(samples),
    )


def _load_demo_report(ref: DemoReportRef, snapshot_dir: Path) -> DemoReport:
    path = _safe_relative_path(snapshot_dir, ref.report_path)
    _validate_sha256(path, ref.report_sha256, f"demo report {ref.run_id}")
    raw = _load_json(path)
    if not isinstance(raw, dict):
        raise ReadmeError(f"demo report {ref.run_id} must be an object")

    report_id = _require_str(raw, "report_id", f"demo report {ref.run_id}")
    if report_id != ref.run_id:
        raise ReadmeError(
            f"demo report report_id mismatch: {report_id} != {ref.run_id}"
        )

    raw_failures = raw.get("failures", [])
    if not isinstance(raw_failures, list):
        raise ReadmeError(f"failures in demo report {ref.run_id} must be a list")
    failures: list[str] = []
    for idx, failure in enumerate(raw_failures):
        if not isinstance(failure, dict):
            raise ReadmeError(f"failures[{idx}] in demo report {ref.run_id} must be an object")
        code = _require_str(failure, "code", f"failures[{idx}] in demo report {ref.run_id}")
        reason = _require_str(failure, "reason", f"failures[{idx}] in demo report {ref.run_id}")
        failures.append(f"{code}: {reason}")

    return DemoReport(
        run_id=report_id,
        verified_at=_require_str(raw, "verified_at", f"demo report {ref.run_id}"),
        verifier_id=_require_str(raw, "verifier_id", f"demo report {ref.run_id}"),
        verifier_version=_require_str(raw, "verifier_version", f"demo report {ref.run_id}"),
        valid=_require_bool(raw, "valid", f"demo report {ref.run_id}"),
        environment=ref.environment,
        scenario_id=ref.scenario_id,
        failures=tuple(failures),
    )


def load_snapshot(snapshot_dir: Path) -> ProofSnapshot:
    index_path = _safe_relative_path(snapshot_dir, "index.json")
    if not index_path.exists():
        raise ReadmeError(f"missing index.json in {snapshot_dir}")
    index_raw = _load_json(index_path)
    if not isinstance(index_raw, dict):
        raise ReadmeError("index.json must be an object")

    manifest = _manifest_from_dict(index_raw, snapshot_dir)

    eval_runs: dict[str, LoadedEvalRun] = {}
    artifact_paths: set[str] = {"index.json"}
    for ref in manifest.eval_runs:
        run = _load_eval_run(ref, snapshot_dir)
        eval_runs[ref.run_id] = run
        artifact_paths.update([
            ref.manifest_path,
            ref.attempts_path,
            ref.metrics_path,
            ref.receipts_path,
            ref.stages_path,
        ])

    receipt_verification = _load_receipt_verification(manifest.receipt_verification, snapshot_dir)
    artifact_paths.add(manifest.receipt_verification.result_path)

    demo_reports: list[DemoReport] = []
    for ref in manifest.demo_reports:
        report = _load_demo_report(ref, snapshot_dir)
        demo_reports.append(report)
        artifact_paths.add(ref.report_path)

    # Verify no extra files in snapshot directory that are not declared.
    _scan_for_undeclared_artifacts(snapshot_dir, artifact_paths)
    # Scan declared artifact content for forbidden private keys, credential fields, and raw canaries.
    _scan_artifact_content(snapshot_dir, artifact_paths)

    return ProofSnapshot(
        manifest=manifest,
        eval_runs=eval_runs,
        receipt_verification=receipt_verification,
        demo_reports=tuple(demo_reports),
        artifact_paths=artifact_paths,
    )


def _scan_for_undeclared_artifacts(snapshot_dir: Path, declared: set[str]) -> None:
    for path in snapshot_dir.rglob("*"):
        if path.is_dir():
            continue
        rel = path.relative_to(snapshot_dir).as_posix()
        if rel not in declared:
            raise ReadmeError(f"undeclared artifact in public snapshot: {rel}")


_PRIVATE_KEY_MARKERS = (
    b"-----BEGIN PRIVATE KEY-----",
    b"-----BEGIN RSA PRIVATE KEY-----",
    b"-----BEGIN EC PRIVATE KEY-----",
    b"-----BEGIN OPENSSH PRIVATE KEY-----",
    b"-----BEGIN PGP PRIVATE KEY BLOCK-----",
)

_CREDENTIAL_FIELD_NAMES = (
    "api_key",
    "apikey",
    "secret_key",
    "access_token",
    "refresh_token",
    "client_secret",
    "private_key",
    "password",
)

_CANARY_MARKERS = (
    "CANARY-",
    "RAW-CANARY-",
)


def _scan_artifact_content(snapshot_dir: Path, declared: set[str]) -> None:
    """Scan declared artifacts for forbidden private keys, credential fields, and raw canary values."""
    for rel in sorted(declared):
        if rel == "index.json":
            continue
        path = _safe_relative_path(snapshot_dir, rel)
        if not path.is_file():
            continue
        _scan_file_for_private_keys(path, rel)
        _scan_file_for_credential_fields(path, rel)
        _scan_file_for_canaries(path, rel)


def _scan_file_for_private_keys(path: Path, rel: str) -> None:
    data = path.read_bytes()
    for marker in _PRIVATE_KEY_MARKERS:
        if marker in data:
            raise ReadmeError(
                f"forbidden private key material in public snapshot artifact {rel}: "
                f"found {marker.decode()}"
            )


def _scan_file_for_credential_fields(path: Path, rel: str) -> None:
    text = path.read_text(errors="replace")
    lowered = text.lower()
    for field in _CREDENTIAL_FIELD_NAMES:
        needle = f'"{field}"'
        if needle in lowered:
            raise ReadmeError(
                f"forbidden credential field in public snapshot artifact {rel}: "
                f"found {field}"
            )


def _scan_file_for_canaries(path: Path, rel: str) -> None:
    text = path.read_text(errors="replace")
    for marker in _CANARY_MARKERS:
        if marker in text:
            raise ReadmeError(
                f"forbidden raw canary value in public snapshot artifact {rel}: "
                f"found marker {marker!r}"
            )


def _project_metrics(runs: dict[str, LoadedEvalRun]) -> dict[str, MetricProjection]:
    groups: dict[tuple[str, str, str, str], list[MetricRow]] = {}
    for run in runs.values():
        for metric in run.metrics:
            if not metric.eligible:
                continue
            key = (metric.metric_id, metric.metric_version, metric.unit, metric.arm_id)
            groups.setdefault(key, []).append(metric)

    projections: dict[str, MetricProjection] = {}
    for (metric_id, version, unit, arm_id), rows in sorted(groups.items()):
        denom = sum(r.denominator_contribution for r in rows)
        if denom <= 0:
            raise ReadmeError(f"metric {metric_id} has non-positive denominator")
        values = [r.value for r in rows]
        if unit == "pass_fail":
            numerator = sum(1 for v in values if v == 1.0)
            rate = numerator / denom
        else:
            numerator = sum(values)
            rate = numerator / denom
        projections[f"{metric_id}__{arm_id}"] = MetricProjection(
            metric_id=metric_id,
            metric_version=version,
            unit=unit,
            arm_id=arm_id,
            numerator=numerator,
            denominator=denom,
            rate=rate,
            verification_status=rows[0].verification_status,
            task_count=len(rows),
        )
    return projections


def _attempt_outcomes(runs: dict[str, LoadedEvalRun]) -> dict[str, int]:
    counts: dict[str, int] = {}
    for run in runs.values():
        for attempt in run.attempts:
            counts[attempt.status] = counts.get(attempt.status, 0) + 1
    return counts


def _render_link(url: str, text: str) -> str:
    esc_url = url.replace("(", "%28").replace(")", "%29")
    return f"[{text}]({esc_url})"


def _escape_cell(value: str) -> str:
    escaped = (
        value.replace("\\", "\\\\")
        .replace("|", "\\|")
        .replace("\n", " ")
        .replace("<", "&lt;")
        .replace(">", "&gt;")
    )
    return escaped


def _format_rate(rate: float) -> str:
    if rate == 1.0:
        return "100.0%"
    if rate == 0.0:
        "0.0%"
    return f"{rate * 100:.1f}%"


def _render_generated_header() -> str:
    return (
        "<!-- Generated by scripts/generate_readme.py. "
        "Do not edit README.md directly. Source: docs/templates/README.md.tmpl "
        "and docs/evidence/readme/current/. -->"
    )


def _render_ci_badges(manifest: PublicationManifest) -> str:
    badges: list[str] = []
    for link in manifest.ci_links:
        if link.kind == "workflow_status":
            badges.append(
                f"[![{link.label}]({link.url}/badge.svg)]({link.url})"
            )
    return " ".join(badges)


def _render_evidence_identity(snapshot: ProofSnapshot) -> str:
    m = snapshot.manifest
    run_ids = ", ".join(sorted(r.run_id for r in m.eval_runs))
    demo_ids = ", ".join(sorted(d.run_id for d in m.demo_reports))

    schema_versions: set[str] = set()
    suite_versions: set[str] = set()
    source_revs: set[str] = set()
    tree_hashes: set[str] = set()
    dataset_hashes: set[str] = set()
    grader_hashes: set[str] = set()
    prompt_hashes: set[str] = set()
    model_cohorts: set[str] = set()
    for run in snapshot.eval_runs.values():
        manifest = run.manifest
        schema_versions.add(_require_str(manifest, "schema_version", f"manifest {run.run_id}"))
        suite_versions.add(_require_str(manifest, "suite_version", f"manifest {run.run_id}"))
        source_revs.add(_require_str(manifest, "source_revision", f"manifest {run.run_id}", allow_empty=True))
        # Accept both fixture (source_tree_hash) and real (source_tree_state_hash) field names.
        tree_hash = manifest.get("source_tree_hash")
        if tree_hash is None:
            tree_hash = manifest.get("source_tree_state_hash", "")
        if not isinstance(tree_hash, str):
            raise ReadmeError(f"source_tree_hash in manifest {run.run_id} must be a string")
        tree_hashes.add(tree_hash)
        # Accept both fixture (dataset_hash/grader_hash/prompt_hash) and real (content_hashes list).
        if "dataset_hash" in manifest:
            dataset_hashes.add(_require_str(manifest, "dataset_hash", f"manifest {run.run_id}"))
            grader_hashes.add(_require_str(manifest, "grader_hash", f"manifest {run.run_id}"))
            prompt_hashes.add(_require_str(manifest, "prompt_hash", f"manifest {run.run_id}"))
        else:
            content_hashes = manifest.get("content_hashes", [])
            if isinstance(content_hashes, list):
                for ch in content_hashes:
                    if not isinstance(ch, dict):
                        continue
                    name = ch.get("name", "")
                    sha = ch.get("sha256", "")
                    if not isinstance(sha, str):
                        continue
                    if name == "dataset":
                        dataset_hashes.add(sha)
                    elif name == "grader_bundle":
                        grader_hashes.add(sha)
                    elif name == "prompt_bundle":
                        prompt_hashes.add(sha)
        # Accept both fixture (model_mapping) and real (role_to_model) field names.
        mapping = manifest.get("model_mapping", {})
        if isinstance(mapping, dict) and mapping:
            for arm, model in mapping.items():
                model_cohorts.add(f"{arm}={model}")
        else:
            role_map = manifest.get("role_to_model", {})
            if isinstance(role_map, dict):
                for role, info in role_map.items():
                    if isinstance(info, dict):
                        model_cohorts.add(f"{role}={info.get('provider','?')}/{info.get('model','?')}")

    lines = [
        "### Evidence Identity",
        "",
        "| Property | Value |",
        "| --- | --- |",
        f"| Evidence cutoff | {_escape_cell(m.evidence_cutoff)} |",
        f"| Platform version | {_escape_cell(m.platform_version)} |",
        f"| Publication schema | {_escape_cell(m.publication_schema_version)} |",
        f"| README evidence version | {_escape_cell(m.readme_evidence_version)} |",
        f"| Eval schema version | {_escape_cell(', '.join(sorted(schema_versions)))} |",
        f"| Suite version | {_escape_cell(', '.join(sorted(suite_versions)))} |",
        f"| Selected eval runs | {_escape_cell(run_ids)} |",
        f"| Selected demo runs | {_escape_cell(demo_ids)} |",
        f"| Source revision | {_escape_cell(', '.join(sorted(source_revs)) or '(not populated)')} |",
        f"| Source tree hash | {_escape_cell(', '.join(sorted(tree_hashes)) or '(not populated)')} |",
        f"| Dataset hash | {_escape_cell(', '.join(sorted(dataset_hashes)))} |",
        f"| Grader hash | {_escape_cell(', '.join(sorted(grader_hashes)))} |",
        f"| Prompt hash | {_escape_cell(', '.join(sorted(prompt_hashes)))} |",
        f"| Model cohort | {_escape_cell('; '.join(sorted(model_cohorts)))} |",
        f"| Receipt verifier scope | {_escape_cell(m.receipt_verification.scope)} |",
    ]
    return "\n".join(lines)


def _render_eval_metrics(snapshot: ProofSnapshot) -> str:
    projections = _project_metrics(snapshot.eval_runs)
    if not projections:
        return "### Eval Metrics\n\nNo eligible metrics present in the selected snapshot.\n"

    lines = [
        "### Eval Metrics",
        "",
        "Metrics aggregate eligible, verified observations from the selected eval runs. "
        "Values link to the metrics artifact in `docs/evidence/readme/current/`.",
        "",
        "| Metric | Version | Arm | Unit | Value | Denominator | Rate | Status | Tasks |",
        "| --- | --- | --- | --- | --- | --- | --- | --- | --- |",
    ]

    metrics_path = snapshot.manifest.eval_runs[0].metrics_path
    for key in sorted(projections.keys()):
        p = projections[key]
        if p.unit in ("pass_fail", "boolean"):
            value = f"{int(p.numerator)}"
            rate = _format_rate(p.rate)
        else:
            value = f"{p.numerator:.3f}"
            rate = f"{p.rate:.3f}"
        link = _render_link(f"docs/evidence/readme/current/{metrics_path}", p.metric_id)
        lines.append(
            f"| {link} | {_escape_cell(p.metric_version)} | {_escape_cell(p.arm_id)} | "
            f"{_escape_cell(p.unit)} | {value} | {p.denominator} | {rate} | "
            f"{_escape_cell(p.verification_status)} | {p.task_count} |"
        )

    # Report missing metrics.
    headline_candidates = [
        "ifeval_subset_verifier",
        "receipt_integrity",
        "protocol_chain",
        "policy_outcome",
        "final_state_accuracy",
        "canary_scrubbing",
        "independent_state_accuracy",
        "model_boundary_raw_secret_rate",
        "exact_local_rehydration",
        "secret_detection_precision",
        "secret_detection_recall",
        "unauthorized_mutation",
    ]
    present_metric_ids = {p.metric_id for p in projections.values()}
    missing = [m for m in headline_candidates if m not in present_metric_ids]
    if missing:
        lines.append("")
        lines.append(
            "The following candidate metrics are absent from the selected evidence and are omitted: "
            + ", ".join(_escape_cell(m) for m in missing)
            + "."
        )

    # Attempt outcomes table.
    outcomes = _attempt_outcomes(snapshot.eval_runs)
    if outcomes:
        lines.append("")
        lines.append("#### Attempt Outcomes")
        lines.append("")
        lines.append("| Status | Count |")
        lines.append("| --- | --- |")
        for status, count in sorted(outcomes.items()):
            lines.append(f"| {_escape_cell(status)} | {count} |")

    return "\n".join(lines)


def _render_receipt_proof(snapshot: ProofSnapshot) -> str:
    r = snapshot.receipt_verification
    pass_label = "no"
    if (
        r.total_receipts > 0
        and r.failed_signatures == 0
        and r.failed_persistence == 0
        and r.missing_keys == 0
        and r.verified_signatures == r.total_receipts
        and r.verified_persistence == r.total_receipts
    ):
        pass_label = "yes"

    lines = [
        "### Receipt Verification",
        "",
        "Receipt verification is bounded to canonical receipt signatures and final-persistence attestations. "
        "It does not constitute a complete eval-bundle verification.",
        "",
        "| Property | Value |",
        "| --- | --- |",
        f"| Total receipts | {r.total_receipts} |",
        f"| Verified signatures | {r.verified_signatures} |",
        f"| Verified persistence | {r.verified_persistence} |",
        f"| Failed signatures | {r.failed_signatures} |",
        f"| Failed persistence | {r.failed_persistence} |",
        f"| Missing keys | {r.missing_keys} |",
        f"| Distinct signer key IDs | {_escape_cell(', '.join(r.distinct_signer_key_ids))} |",
        f"| Receipt-bound eligible attempts | {r.receipt_bound_eligible_attempts} |",
        f"| Pass | {pass_label} |",
    ]

    if r.sample_receipt_fingerprints:
        lines.append("")
        lines.append("#### Sample Receipt Fingerprints")
        lines.append("")
        lines.append("| Receipt ID | Signature Digest | Artifact |")
        lines.append("| --- | --- | --- |")
        for sample in r.sample_receipt_fingerprints:
            receipt_id = sample.get("receipt_id", "n/a")
            digest = sample.get("signature_digest", "n/a")[:16] + "..."
            artifact = sample.get("artifact_ref", "n/a")
            lines.append(
                f"| {_escape_cell(receipt_id)} | `{_escape_cell(digest)}` | {_escape_cell(artifact)} |"
            )

    return "\n".join(lines)


def _render_governance_proof(snapshot: ProofSnapshot) -> str:
    """Project governance and state metrics from eligible metric rows and stage records."""
    projections = _project_metrics(snapshot.eval_runs)

    governance_metrics = {
        "protocol_chain",
        "receipt_integrity",
        "policy_outcome",
        "final_state_accuracy",
        "independent_state_accuracy",
        "canary_scrubbing",
    }

    lines = [
        "### Governance and State Proof",
        "",
        "| Metric | Version | Arm | Denominator | Rate | Status |",
        "| --- | --- | --- | --- | --- | --- |",
    ]

    for key in sorted(projections.keys()):
        p = projections[key]
        if p.metric_id not in governance_metrics:
            continue
        if p.unit in ("pass_fail", "boolean"):
            rate = _format_rate(p.rate)
        else:
            rate = f"{p.rate:.3f}"
        lines.append(
            f"| {_escape_cell(p.metric_id)} | {_escape_cell(p.metric_version)} | "
            f"{_escape_cell(p.arm_id)} | {p.denominator} | {rate} | "
            f"{_escape_cell(p.verification_status)} |"
        )

    # Stage posture summary for governed arm.
    posture_rows: list[str] = []
    for run in snapshot.eval_runs.values():
        for stage in run.stages:
            if stage.arm_id == "governed" and stage.layer == "L2":
                posture_rows.append(stage.detail)

    if posture_rows:
        distinct = sorted(set(posture_rows))
        lines.append("")
        lines.append(f"Observed L2 posture: {_escape_cell(', '.join(distinct))}.")

    return "\n".join(lines)


def _render_demo_proof(snapshot: ProofSnapshot) -> str:
    if not snapshot.demo_reports:
        return "### Independently Verified Demonstrations\n\nNo demo verification reports in the selected snapshot.\n"

    lines = [
        "### Independently Verified Demonstrations",
        "",
        "The following scenarios are backed by canonical `ComplianceVerificationReport` records. "
        "All resources and data are synthetic or simulated.",
        "",
        "| Run ID | Environment | Scenario | Verifier | Version | Valid | Failures |",
        "| --- | --- | --- | --- | --- | --- | --- |",
    ]

    for report in snapshot.demo_reports:
        if not report.valid or report.failures:
            raise ReadmeError(
                f"demo report {report.run_id} is invalid or has failures and cannot contribute to proof"
            )
        lines.append(
            f"| {_escape_cell(report.run_id)} | {_escape_cell(report.environment)} | "
            f"{_escape_cell(report.scenario_id)} | {_escape_cell(report.verifier_id)} | "
            f"{_escape_cell(report.verifier_version)} | yes | 0 |"
        )

    return "\n".join(lines)


def _render_ci_reproducibility(snapshot: ProofSnapshot) -> str:
    links: list[str] = []
    for link in snapshot.manifest.ci_links:
        links.append(_render_link(link.url, link.label))

    lines = [
        "### CI and Reproducibility",
        "",
        "CI status is a live external signal, not a frozen pass claim.",
        "",
        "| Link | Kind |",
        "| --- | --- |",
    ]
    for link in snapshot.manifest.ci_links:
        lines.append(f"| {_render_link(link.url, link.label)} | {_escape_cell(link.kind)} |")

    lines.append("")
    lines.append("Local verification commands:")
    lines.append("")
    lines.append("```bash")
    lines.append("# Regenerate the README from the selected proof snapshot")
    lines.append("make readme")
    lines.append("")
    lines.append("# Check that README.md is up to date without modifying files")
    lines.append("make readme-check")
    lines.append("")
    lines.append("# Run platform unit and integration tests")
    lines.append("./g8e test unit")
    lines.append("./g8e test integration")
    lines.append("")
    lines.append("# Run eval tests")
    lines.append("make evals-test-unit")
    lines.append("make evals-test-integration")
    lines.append("")
    lines.append("# Run lint")
    lines.append("make lint")
    lines.append("```")

    if snapshot.manifest.caveats:
        lines.append("")
        lines.append("**Evidence caveats**")
        for caveat in snapshot.manifest.caveats:
            lines.append(f"- {_escape_cell(caveat)}")

    return "\n".join(lines)


def _replace_markers(template: str, renderers: dict[str, str]) -> str:
    found: set[str] = set()

    def repl(match: re.Match[str]) -> str:
        name = match.group(1)
        if name not in MARKERS:
            raise ReadmeError(f"unknown template marker: {name}")
        found.add(name)
        return renderers[name]

    rendered = MARKER_PATTERN.sub(repl, template)
    missing = MARKERS - found
    if missing:
        raise ReadmeError(f"missing template markers: {', '.join(sorted(missing))}")
    return rendered


def render_readme(snapshot: ProofSnapshot, template: str) -> str:
    renderers = {
        "GENERATED_HEADER": _render_generated_header(),
        "CI_BADGES": _render_ci_badges(snapshot.manifest),
        "EVIDENCE_IDENTITY": _render_evidence_identity(snapshot),
        "EVAL_METRICS": _render_eval_metrics(snapshot),
        "RECEIPT_PROOF": _render_receipt_proof(snapshot),
        "GOVERNANCE_PROOF": _render_governance_proof(snapshot),
        "DEMO_PROOF": _render_demo_proof(snapshot),
        "CI_REPRODUCIBILITY": _render_ci_reproducibility(snapshot),
    }
    return _replace_markers(template, renderers)


def generate(
    template_path: Path,
    snapshot_dir: Path,
    output_path: Path,
    check: bool = False,
) -> None:
    snapshot = load_snapshot(snapshot_dir)

    with template_path.open("r", encoding="utf-8") as f:
        template = f.read()

    rendered = render_readme(snapshot, template)

    if check:
        if not output_path.exists():
            raise ReadmeError(
                f"README drift check failed: {output_path} does not exist"
            )
        with output_path.open("r", encoding="utf-8") as f:
            existing = f.read()
        if existing != rendered:
            raise ReadmeError("README drift check failed: rendered output differs")
        return

    tmp_path = output_path.with_suffix(output_path.suffix + ".tmp")
    with tmp_path.open("w", encoding="utf-8") as f:
        f.write(rendered)
    os.replace(tmp_path, output_path)


def main(argv: list[str] | None = None) -> int:
    parser = argparse.ArgumentParser(description="Generate README.md from template and proof snapshot.")
    parser.add_argument(
        "--check",
        action="store_true",
        help="Render in memory and exit nonzero if README.md differs.",
    )
    parser.add_argument(
        "--snapshot-dir",
        type=Path,
        default=None,
        help="Path to the public proof snapshot directory (default: docs/evidence/readme/current).",
    )
    parser.add_argument(
        "--template",
        type=Path,
        default=None,
        help="Path to the README template (default: docs/templates/README.md.tmpl).",
    )
    parser.add_argument(
        "--output",
        type=Path,
        default=None,
        help="Path to write README.md (default: README.md).",
    )
    args = parser.parse_args(argv)

    repo_root = _repo_root()
    template_path = args.template or repo_root / "docs" / "templates" / "README.md.tmpl"
    snapshot_dir = args.snapshot_dir or repo_root / "docs" / "evidence" / "readme" / "current"
    output_path = args.output or repo_root / "README.md"

    try:
        generate(template_path, snapshot_dir, output_path, check=args.check)
    except ReadmeError as exc:
        print(f"error: {exc}", file=sys.stderr)
        return 1
    return 0


if __name__ == "__main__":
    sys.exit(main())
