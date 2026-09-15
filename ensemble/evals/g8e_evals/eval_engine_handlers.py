# Copyright (c) 2026 Lateralus Labs, LLC.
# Use of this source code is governed by the Business Source License
# included in the LICENSE file.
#
# As of the Change Date listed in the LICENSE file, this software is
# released under the Apache License, Version 2.0.

from __future__ import annotations

import hashlib
import json
import os
import sys
from collections.abc import Callable
from contextlib import redirect_stdout
from datetime import UTC, datetime
from pathlib import Path
from typing import TYPE_CHECKING

from g8e_evals.engine_cli import EngineError, OperationHandler
from g8e_evals.engine_protocol import (
    EvalErrorCode,
    EvalEngineRequest,
    EvalEngineResult,
    EvalOperation,
    failed_result,
    stopped_result,
    succeeded_result,
)
from g8e_evals.constants import EVAL_LAUNCH_STATE_JSON
from g8e_evals.operation_config import CampaignConfig, DiagnosticConfig, load_operation_config
from g8e_evals.operation_lifecycle import operation_status, resolve_report_root, stop_operation, verify_operation
from g8e_evals.stop_request import (
    StopRequestConsumer,
    StopRequestError,
    StopRequestIdentity,
)

if TYPE_CHECKING:
    from g8e_evals.auth_bridge import CLIAuthContext
    from g8e_evals.schema import SourceBuildProvenance


def _provider_api_key(provider: str) -> str | None:
    """Return the provider credential from the process environment.

    API keys are secrets and never travel through the typed engine
    request; the process environment is the only permitted channel.
    Provider endpoints are never sourced here — the verified lease is
    the only authority for the approved endpoint URL.
    """
    names = {
        "openai": "OPENAI_API_KEY",
        "anthropic": "ANTHROPIC_API_KEY",
    }
    return os.environ.get(names.get(provider, "G8E_TEST_LLM_PRIMARY_API_KEY"))


def _request_provider_endpoint(request: EvalEngineRequest) -> str:
    """Load the approved provider endpoint from the verified lease.

    The endpoint is bound into the lease at issue time and re-verified
    by the shared preflight before engine launch, so the engine child
    never reads provider endpoint environment variables and never
    invents a default.
    """
    from g8e_evals.live_operations_authority import LiveOperationLease

    try:
        lease = LiveOperationLease.model_validate_json(Path(request.lease_path).read_text())
    except FileNotFoundError as exc:
        raise EngineError(
            EvalErrorCode.LEASE_MISSING,
            "lease",
            "verified lease file is absent",
        ) from exc
    except (OSError, ValueError) as exc:
        raise EngineError(
            EvalErrorCode.LEASE_MISMATCHED,
            "lease",
            f"verified lease failed to load: {_safe_detail(exc)}",
        ) from exc
    return lease.endpoint


# Patterns that must never appear in safe_detail output. The redaction is
# conservative: any exception string containing a known secret-bearing
# pattern is replaced with a generic safe message.
_SECRET_PATTERNS = (
    "api_key",
    "api-key",
    "apikey",
    "token",
    "secret",
    "password",
    "credential",
    "authorization",
    "bearer",
    "private_key",
    "key_b64",
)


def _safe_detail(exc: BaseException) -> str:
    """Return a redacted error detail that cannot expose credentials or
    secret-bearing provider errors. If the exception message contains any
    known secret-bearing pattern, a generic safe message is returned.
    """
    message = str(exc)
    lower = message.lower()
    for pattern in _SECRET_PATTERNS:
        if pattern in lower:
            return f"{type(exc).__name__}: redacted (secret-bearing error)"
    return f"{type(exc).__name__}: {message}"


def _classify_callback_exception(exc: BaseException) -> tuple[EvalErrorCode, str]:
    """Map a callback exception to the exact stable EvalErrorCode and a
    redacted safe detail.

    Terminal model/task outcomes (a model failing a grade) are not
    launcher defects and must not produce a nonzero launcher exit; those
    are handled inside the callback and surface as a succeeded run. This
    classifier reserves CHILD_EXIT_NON_ZERO for genuine lifecycle defects
    that are not config, report-root, stop-request, or interruption
    failures.
    """
    import click

    if isinstance(exc, StopRequestError):
        return EvalErrorCode.STATUS_RECONCILIATION_FAILED, _safe_detail(exc)
    if isinstance(exc, FileExistsError):
        return EvalErrorCode.REPORT_ROOT_REUSED, _safe_detail(exc)
    if isinstance(exc, (click.UsageError,)):
        return EvalErrorCode.CONFIG_INVALID, _safe_detail(exc)
    if isinstance(exc, KeyboardInterrupt):
        return EvalErrorCode.CHILD_INTERRUPTED, "interrupted by operator"
    return EvalErrorCode.CHILD_EXIT_NON_ZERO, _safe_detail(exc)


def _build_launch_record(request: EvalEngineRequest, config: object, report_root: Path, status: str, started_at: str | None = None) -> dict:
    """Build a content-hashed, identity-bound launch record.

    Binds operation ID, revision, config content hash, lease ID, lease
    path, engine PID, candidate binary hash, report root, and start time.
    The content hash is computed over the canonical JSON excluding the
    content_hash field itself, matching the operation config convention.
    """
    lease_id = Path(request.lease_path).stem if request.lease_path else ""
    record = {
        "operation_id": request.operation_id,
        "revision": request.revision,
        "config_content_hash": getattr(config, "content_hash", ""),
        "lease_id": lease_id,
        "lease_path": request.lease_path,
        "pid": os.getpid(),
        "candidate_binary_sha256": request.platform.g8e_binary_sha256,
        "report_root": str(report_root),
        "status": status,
        "started_at": started_at or datetime.now(UTC).isoformat(),
        "updated_at": datetime.now(UTC).isoformat(),
    }
    hash_payload = {k: v for k, v in record.items() if k != "content_hash"}
    record["content_hash"] = hashlib.sha256(
        json.dumps(hash_payload, sort_keys=True, separators=(",", ":")).encode()
    ).hexdigest()
    return record


def _write_launch_state(report_root: Path, request: EvalEngineRequest, config: object, status: str, started_at: str | None = None) -> dict:
    """Atomically write a content-hashed launch record.

    Writes to a temp file then renames so the record is never partially
    written. Returns the written record.
    """
    record = _build_launch_record(request, config, report_root, status, started_at)
    target = report_root / EVAL_LAUNCH_STATE_JSON
    tmp = target.with_suffix(".tmp")
    tmp.write_text(json.dumps(record, sort_keys=True, indent=2) + "\n")
    tmp.replace(target)
    return record


def _transition_launch_state(report_root: Path, request: EvalEngineRequest, config: object, status: str, started_at: str) -> None:
    """Atomically transition the launch record to a terminal state.

    Preserves the original started_at and verifies the operation identity
    before transitioning. Does not rely on PID existence alone.
    """
    target = report_root / EVAL_LAUNCH_STATE_JSON
    existing = {}
    if target.is_file():
        existing = json.loads(target.read_text())
    if existing.get("operation_id") != request.operation_id or existing.get("revision") != request.revision:
        raise EngineError(
            EvalErrorCode.STATUS_RECONCILIATION_FAILED,
            "launch_state",
            "launch state identity does not match operation",
        )
    record = _build_launch_record(request, config, report_root, status, started_at)
    record["started_at"] = existing.get("started_at", started_at)
    hash_payload = {k: v for k, v in record.items() if k != "content_hash"}
    record["content_hash"] = hashlib.sha256(
        json.dumps(hash_payload, sort_keys=True, separators=(",", ":")).encode()
    ).hexdigest()
    tmp = target.with_suffix(".tmp")
    tmp.write_text(json.dumps(record, sort_keys=True, indent=2) + "\n")
    tmp.replace(target)


def _build_stop_consumer(report_root: Path, request: EvalEngineRequest, config: object) -> StopRequestConsumer | None:
    """Construct a StopRequestConsumer bound to the launch record's
    content hash. Returns None when the launch record has no content hash
    (the consumer cannot bind without it)."""
    launch_path = report_root / EVAL_LAUNCH_STATE_JSON
    if not launch_path.is_file():
        return None
    launch = json.loads(launch_path.read_text())
    launch_content_hash = launch.get("content_hash")
    if not isinstance(launch_content_hash, str) or len(launch_content_hash) != 64:
        return None
    config_content_hash = getattr(config, "content_hash", "") or ""
    identity = StopRequestIdentity(
        operation_id=request.operation_id,
        revision=request.revision,
        config_content_hash=config_content_hash,
        launch_content_hash=launch_content_hash,
    )
    return StopRequestConsumer(request_path=report_root / "eval-stop-request.json", identity=identity)


def _request_cli_auth_context(request: EvalEngineRequest) -> CLIAuthContext:
    """Build the canonical CLI identity from the typed platform context.

    The Go facade populates the platform auth fields from the on-disk
    credentials before engine launch, so the engine child never reads
    G8E_* auth variables from the process environment. A request that
    carries no usable identity fails closed here rather than surfacing
    as an engine-child auth failure.
    """
    from pydantic import ValidationError

    from g8e_evals.auth_bridge import CLIAuthContext

    platform = request.platform
    try:
        return CLIAuthContext(
            operator_session_id=platform.operator_session_id,
            cli_session_id=platform.cli_session_id,
            user_id=platform.user_id,
            operator_id=platform.operator_id,
            client_cert=platform.cli_cert_path,
            client_key=platform.cli_key_path,
        )
    except ValidationError as exc:
        raise EngineError(
            EvalErrorCode.PLATFORM_IDENTITY_UNAVAILABLE,
            "auth",
            "platform context carries no usable CLI auth identity",
        ) from exc


def _request_build_provenance(request: EvalEngineRequest) -> SourceBuildProvenance:
    """Build source/build provenance from the typed platform context.

    The facade stamps the running binary's source revision and the
    preflight-computed source tree state hash into the request, so the
    engine child never consults G8E_EVALS_SOURCE_* env vars or shells
    out to `g8e version` for provenance.
    """
    from pydantic import ValidationError

    from g8e_evals.schema import SourceBuildProvenance

    platform = request.platform
    try:
        return SourceBuildProvenance(
            source_revision=platform.source_revision,
            source_tree_state_hash=platform.source_tree_state_hash,
            build_system="g8e-facade",
            binary_sha256=platform.g8e_binary_sha256,
        )
    except ValidationError as exc:
        raise EngineError(
            EvalErrorCode.PLATFORM_IDENTITY_UNAVAILABLE,
            "provenance",
            "platform context carries no usable source/build provenance",
        ) from exc


def _diagnostic_start(request: EvalEngineRequest) -> EvalEngineResult:
    from g8e_evals.cli import run

    config = load_operation_config(Path(request.config_path))
    if not isinstance(config, DiagnosticConfig):
        raise EngineError(EvalErrorCode.CONFIG_INVALID, "start", "diagnostic start requires a diagnostic config")
    auth_context = _request_cli_auth_context(request)
    build_provenance = _request_build_provenance(request)
    provider_endpoint = _request_provider_endpoint(request)
    repository_root = Path(request.platform.repository_root)
    report_root = resolve_report_root(config, repository_root)
    if report_root.exists():
        raise EngineError(EvalErrorCode.REPORT_ROOT_REUSED, "start", "report root already exists")
    report_root.mkdir(parents=True)
    started_at = datetime.now(UTC).isoformat()
    _write_launch_state(report_root, request, config, "running", started_at)
    stop_consumer = _build_stop_consumer(report_root, request, config)
    state_root = hashlib.sha256(
        f"{config.content_hash}:{request.operation_id}:{request.revision}".encode()
    ).hexdigest()
    callback = getattr(run, "callback", run)
    try:
        with redirect_stdout(sys.stderr):
            callback(
                suite=config.suite,
                model=config.model_variant_id,
                provider=config.provider_endpoint.provider,
                assistant_model=None,
                assistant_provider=None,
                lite_model=None,
                lite_provider=None,
                judge_model=None,
                judge_provider=None,
                headless=True,
                verbose_text=request.flags.verbose,
                idle_timeout=config.stop_conditions.idle_timeout_s,
                g8ee_url=request.platform.ensemble_url,
                operator_url=request.platform.gateway_https_url,
                operator_session_id=None,
                g8e_cli=request.platform.g8e_binary_path,
                auth_project_root=Path(request.platform.auth_project_root),
                arm=config.arm,
                state_root=state_root,
                output_dir=report_root.parent,
                evidence_key_file=repository_root / config.evidence_key.path,
                gold_set=repository_root / config.gold_set.path,
                limit=config.task_limit,
                l2_key=None,
                l2_key_id=None,
                primary_api_key=_provider_api_key(config.provider_endpoint.provider),
                primary_endpoint=provider_endpoint,
                assistant_api_key=None,
                assistant_endpoint=None,
                lite_api_key=None,
                lite_endpoint=None,
                judge_api_key=None,
                judge_endpoint=None,
                web_search_project=None,
                web_search_app=None,
                web_search_api_key=None,
                temperature=config.sampling.temperature,
                top_p=config.sampling.top_p,
                max_tokens=config.sampling.max_output_tokens,
                seed=config.seed,
                seed_support="unknown",
                preregistration=repository_root / config.preregistration.path if config.preregistration else None,
                exact_report_dir=report_root,
                exact_task_offset=config.task_offset,
                stop_consumer=stop_consumer,
                auth_context=auth_context,
                build_provenance=build_provenance,
                trust_bundle_path=request.platform.trust_bundle_path,
            )
    except KeyboardInterrupt:
        _transition_launch_state(report_root, request, config, "interrupted", started_at)
        raise EngineError(EvalErrorCode.CHILD_INTERRUPTED, "diagnostic_start", "interrupted by operator")
    except Exception as exc:
        code, detail = _classify_callback_exception(exc)
        if code == EvalErrorCode.STATUS_RECONCILIATION_FAILED:
            _transition_launch_state(report_root, request, config, "stopped", started_at)
        else:
            _transition_launch_state(report_root, request, config, "failed", started_at)
        raise EngineError(code, "diagnostic_start", detail) from exc
    if stop_consumer is not None and stop_consumer.consumed is not None:
        _transition_launch_state(report_root, request, config, "stopped", started_at)
        return stopped_result(request, {"report_root": str(report_root), "stop_reason": "operator_requested"})
    _transition_launch_state(report_root, request, config, "completed", started_at)
    return succeeded_result(request, {"report_root": str(report_root)})


def _campaign_start(request: EvalEngineRequest) -> EvalEngineResult:
    from g8e_evals.cli import campaign_run

    config = load_operation_config(Path(request.config_path))
    if not isinstance(config, CampaignConfig):
        raise EngineError(EvalErrorCode.CONFIG_INVALID, "start", "campaign start requires a campaign config")
    auth_context = _request_cli_auth_context(request)
    build_provenance = _request_build_provenance(request)
    repository_root = Path(request.platform.repository_root)
    report_root = resolve_report_root(config, repository_root)
    if report_root.exists():
        raise EngineError(EvalErrorCode.REPORT_ROOT_REUSED, "start", "report root already exists")
    report_root.mkdir(parents=True)
    started_at = datetime.now(UTC).isoformat()
    _write_launch_state(report_root, request, config, "running", started_at)
    stop_consumer = _build_stop_consumer(report_root, request, config)
    callback = getattr(campaign_run, "callback", campaign_run)
    try:
        with redirect_stdout(sys.stderr):
            callback(
                suite=config.suite,
                preregistration=repository_root / config.preregistration.path,
                campaign_id=config.campaign_id,
                release_version=config.release_version,
                seed=config.seed,
                output_dir=report_root.parent,
                gold_set=repository_root / config.gold_set.path,
                max_retries=config.budget.max_retries,
                max_requests=config.budget.max_requests,
                max_usd=config.budget.max_usd,
                max_tokens=config.budget.max_tokens,
                model_tags=repository_root / config.model_tags.path if config.model_tags else None,
                profile=repository_root / config.profile.path,
                models=repository_root / config.model_registry.path,
                g8ee_url=request.platform.ensemble_url,
                operator_url=request.platform.gateway_https_url,
                operator_session_id=None,
                g8e_cli=request.platform.g8e_binary_path,
                auth_project_root=Path(request.platform.auth_project_root),
                task_offset=config.task_offset,
                task_limit=config.task_limit,
                campaign_set_plan=repository_root / config.campaign_set_plan.path if config.campaign_set_plan else None,
                replacement_rule=repository_root / config.replacement_rule.path if config.replacement_rule else None,
                exact_report_dir=report_root,
                stop_consumer=stop_consumer,
                auth_context=auth_context,
                build_provenance=build_provenance,
                trust_bundle_path=request.platform.trust_bundle_path,
            )
    except KeyboardInterrupt:
        _transition_launch_state(report_root, request, config, "interrupted", started_at)
        raise EngineError(EvalErrorCode.CHILD_INTERRUPTED, "campaign_start", "interrupted by operator")
    except Exception as exc:
        code, detail = _classify_callback_exception(exc)
        if code == EvalErrorCode.STATUS_RECONCILIATION_FAILED:
            _transition_launch_state(report_root, request, config, "stopped", started_at)
        else:
            _transition_launch_state(report_root, request, config, "failed", started_at)
        raise EngineError(code, "campaign_start", detail) from exc
    if stop_consumer is not None and stop_consumer.consumed is not None:
        _transition_launch_state(report_root, request, config, "stopped", started_at)
        return stopped_result(request, {"report_root": str(report_root), "stop_reason": "operator_requested"})
    _transition_launch_state(report_root, request, config, "completed", started_at)
    return succeeded_result(request, {"report_root": str(report_root)})


def _classify_lifecycle_exception(exc: Exception) -> EvalErrorCode:
    """Split config-load failures from reconciliation failures for the
    status/stop/verify lifecycle handlers."""
    from pydantic import ValidationError

    if isinstance(exc, (ValidationError, FileNotFoundError, json.JSONDecodeError)):
        return EvalErrorCode.CONFIG_INVALID
    if isinstance(exc, FileExistsError):
        return EvalErrorCode.REPORT_ROOT_REUSED
    return EvalErrorCode.STATUS_RECONCILIATION_FAILED


def _status(request: EvalEngineRequest) -> EvalEngineResult:
    try:
        result = operation_status(Path(request.config_path), Path(request.platform.repository_root))
    except (OSError, ValueError) as exc:
        raise EngineError(_classify_lifecycle_exception(exc), "status", _safe_detail(exc)) from exc
    return succeeded_result(request, result.model_dump(exclude_none=True))


def _stop(request: EvalEngineRequest) -> EvalEngineResult:
    try:
        result = stop_operation(
            Path(request.config_path),
            Path(request.platform.repository_root),
            immediate=request.flags.immediate_stop,
        )
    except (OSError, ValueError) as exc:
        raise EngineError(_classify_lifecycle_exception(exc), "stop", _safe_detail(exc)) from exc
    return succeeded_result(request, result.model_dump(exclude_none=True))


def _verify(request: EvalEngineRequest) -> EvalEngineResult:
    try:
        result = verify_operation(Path(request.config_path), Path(request.platform.repository_root))
    except (OSError, ValueError) as exc:
        raise EngineError(_classify_lifecycle_exception(exc), "verify", _safe_detail(exc)) from exc
    payload = result.model_dump(exclude_none=True)
    if not result.ok:
        # Emit the typed failure payload (checked_layers and failures)
        # even on a nonzero verify so the Go facade can render --json
        # without losing the checked layers or the specific failures.
        return failed_result(
            request,
            error_code=EvalErrorCode.AUTHORITY_INVALID,
            error_stage="verify",
            safe_detail=f"offline verification failed with {len(result.failures)} failure(s)",
            payload=payload,
        )
    return succeeded_result(request, payload)


def builtin_operation_handlers() -> dict[EvalOperation, OperationHandler]:
    handlers: dict[EvalOperation, Callable[[EvalEngineRequest], EvalEngineResult]] = {
        EvalOperation.DIAGNOSTIC_START: _diagnostic_start,
        EvalOperation.CAMPAIGN_START: _campaign_start,
        EvalOperation.DIAGNOSTIC_STATUS: _status,
        EvalOperation.DIAGNOSTIC_STOP: _stop,
        EvalOperation.DIAGNOSTIC_VERIFY: _verify,
        EvalOperation.CAMPAIGN_STATUS: _status,
        EvalOperation.CAMPAIGN_STOP: _stop,
        EvalOperation.CAMPAIGN_VERIFY: _verify,
    }
    return handlers
