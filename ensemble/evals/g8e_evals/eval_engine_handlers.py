# Copyright (c) 2026 Lateralus Labs, LLC.
# Use of this source code is governed by the Business Source License
# included in the LICENSE file.
#
# As of the Change Date listed in the LICENSE file, this software is
# released under the Apache License, Version 2.0.

from __future__ import annotations

import asyncio
import hashlib
import io
import json
import os
import subprocess
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
from g8e_evals.operation_config import CampaignConfig, DiagnosticConfig, OperationConfigBase, load_operation_config
from g8e_evals.operation_lifecycle import LaunchState, operation_status, resolve_report_root, stop_operation, verify_operation
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


def _build_launch_record(request: EvalEngineRequest, config: OperationConfigBase, report_root: Path, status: str, started_at: str | None = None) -> LaunchState:
    """Build a content-hashed, identity-bound launch record.

    Binds operation ID, revision, config content hash, lease ID, lease
    path, engine PID, candidate binary hash, report root, and start time.
    The content hash is computed over the canonical JSON excluding the
    content_hash field itself, matching the operation config convention.
    """
    lease_id = Path(request.lease_path).stem if request.lease_path else ""
    now = datetime.now(UTC).isoformat()
    effective_started_at = started_at or now
    record = LaunchState(
        operation_id=request.operation_id,
        revision=request.revision,
        config_content_hash=config.content_hash or "",
        lease_id=lease_id,
        lease_path=request.lease_path,
        pid=os.getpid(),
        candidate_binary_sha256=request.platform.g8e_binary_sha256,
        report_root=str(report_root),
        status=status,
        started_at=effective_started_at,
        updated_at=now,
    )
    hash_payload = record.model_dump(exclude={"content_hash"}, exclude_none=True)
    content_hash = hashlib.sha256(
        json.dumps(hash_payload, sort_keys=True, separators=(",", ":")).encode()
    ).hexdigest()
    return record.model_copy(update={"content_hash": content_hash})


def _write_launch_state(report_root: Path, request: EvalEngineRequest, config: OperationConfigBase, status: str, started_at: str | None = None) -> LaunchState:
    """Atomically write a content-hashed launch record.

    Writes to a temp file then renames so the record is never partially
    written. Returns the written record.
    """
    record = _build_launch_record(request, config, report_root, status, started_at)
    target = report_root / EVAL_LAUNCH_STATE_JSON
    tmp = target.with_suffix(".tmp")
    tmp.write_text(record.model_dump_json(indent=2) + "\n")
    tmp.replace(target)
    return record


def _transition_launch_state(report_root: Path, request: EvalEngineRequest, config: OperationConfigBase, status: str, started_at: str) -> None:
    """Atomically transition the launch record to a terminal state.

    Preserves the original started_at and verifies the operation identity
    before transitioning. Does not rely on PID existence alone.
    """
    target = report_root / EVAL_LAUNCH_STATE_JSON
    existing: LaunchState | None = None
    if target.is_file():
        try:
            existing = LaunchState.model_validate_json(target.read_text())
        except (ValueError, OSError):
            existing = None
    if existing is None or existing.operation_id != request.operation_id or existing.revision != request.revision:
        raise EngineError(
            EvalErrorCode.STATUS_RECONCILIATION_FAILED,
            "launch_state",
            "launch state identity does not match operation",
        )
    record = _build_launch_record(request, config, report_root, status, started_at)
    record = record.model_copy(update={"started_at": existing.started_at or started_at})
    tmp = target.with_suffix(".tmp")
    tmp.write_text(record.model_dump_json(indent=2) + "\n")
    tmp.replace(target)


def _build_stop_consumer(report_root: Path, request: EvalEngineRequest, config: OperationConfigBase) -> StopRequestConsumer | None:
    """Construct a StopRequestConsumer bound to the launch record's
    content hash. Returns None when the launch record has no content hash
    (the consumer cannot bind without it)."""
    launch_path = report_root / EVAL_LAUNCH_STATE_JSON
    if not launch_path.is_file():
        return None
    try:
        launch = LaunchState.model_validate_json(launch_path.read_text())
    except (ValueError, OSError):
        return None
    launch_content_hash = launch.content_hash
    if not isinstance(launch_content_hash, str) or len(launch_content_hash) != 64:
        return None
    config_content_hash = config.content_hash or ""
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
    callback = run.callback
    if callback is None:
        raise EngineError(EvalErrorCode.CHILD_START_FAILED, "diagnostic_start", "diagnostic command is unavailable")
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
    callback = campaign_run.callback
    if callback is None:
        raise EngineError(EvalErrorCode.CHILD_START_FAILED, "campaign_start", "campaign command is unavailable")
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


def _required_parameter(request: EvalEngineRequest, name: str) -> str:
    value = request.parameters.get(name, "")
    if not value:
        raise EngineError(EvalErrorCode.CONFIG_INVALID, request.operation.value, f"missing required parameter: {name}")
    return value


def _optional_path(request: EvalEngineRequest, name: str) -> Path | None:
    value = request.parameters.get(name)
    return Path(value) if value else None


def _campaign_set_plan(request: EvalEngineRequest) -> EvalEngineResult:
    from g8e_evals.campaign_set import compute_dry_run_plan, load_campaign_set_plan

    try:
        plan = load_campaign_set_plan(Path(request.config_path))
        return succeeded_result(request, compute_dry_run_plan(plan))
    except (OSError, ValueError) as exc:
        raise EngineError(EvalErrorCode.CONFIG_INVALID, "campaign_set_plan", _safe_detail(exc)) from exc


def _campaign_set_validate(request: EvalEngineRequest) -> EvalEngineResult:
    from g8e_evals.campaign_set import (
        load_campaign_set_index,
        load_campaign_set_plan,
        validate_campaign_set_index,
        validate_campaign_set_plan,
    )

    try:
        plan = load_campaign_set_plan(Path(request.config_path))
        validate_campaign_set_plan(plan)
        payload = {
            "ok": True,
            "set_id": plan.set_id,
            "child_count": len(plan.child_plans),
            "total_tasks": len(plan.population_task_ids),
            "expected_total_assignment_count": plan.expected_total_assignment_count,
        }
        index_path = _optional_path(request, "index")
        if index_path is not None:
            index = load_campaign_set_index(index_path)
            validate_campaign_set_index(index, plan)
            payload["index"] = index.model_dump(mode="json")
        return succeeded_result(request, payload)
    except (OSError, ValueError) as exc:
        raise EngineError(EvalErrorCode.AUTHORITY_INVALID, "campaign_set_validate", _safe_detail(exc)) from exc


def _campaign_set_verify(request: EvalEngineRequest) -> EvalEngineResult:
    from pydantic import TypeAdapter, ValidationError

    from g8e_evals.campaign_set import (
        load_campaign_set_index,
        load_campaign_set_plan,
        verify_campaign_set_aggregate,
    )
    from g8e_evals.replacement_rule import ReplacementManifestRule

    try:
        plan = load_campaign_set_plan(Path(request.config_path))
        index = load_campaign_set_index(Path(_required_parameter(request, "index")))
        child_paths = TypeAdapter(dict[str, str]).validate_json(_required_parameter(request, "child_dirs"))
        replacement_path = _optional_path(request, "replacement_rule")
        replacement = (
            ReplacementManifestRule.model_validate_json(replacement_path.read_text())
            if replacement_path is not None
            else None
        )
        result = verify_campaign_set_aggregate(
            plan,
            index,
            {child_id: Path(path) for child_id, path in child_paths.items()},
            replacement_rule=replacement,
        )
    except EngineError:
        raise
    except (OSError, ValueError, ValidationError) as exc:
        raise EngineError(EvalErrorCode.CONFIG_INVALID, "campaign_set_verify", _safe_detail(exc)) from exc
    payload = result.model_dump(mode="json")
    if not result.ok:
        return failed_result(
            request,
            EvalErrorCode.AUTHORITY_INVALID,
            "campaign_set_verify",
            f"campaign-set verification failed with {len(result.failures)} failure(s)",
            payload,
        )
    return succeeded_result(request, payload)


def _controller_status(request: EvalEngineRequest) -> EvalEngineResult:
    from g8e_evals.constants import CONTROLLER_STATE_JSON
    from g8e_evals.controller import load_controller_state

    try:
        state = load_controller_state(Path(_required_parameter(request, "work_dir")) / CONTROLLER_STATE_JSON)
        return succeeded_result(request, state.model_dump(mode="json"))
    except EngineError:
        raise
    except (OSError, ValueError) as exc:
        raise EngineError(EvalErrorCode.STATUS_RECONCILIATION_FAILED, "controller_status", _safe_detail(exc)) from exc


def _controller_stop(request: EvalEngineRequest) -> EvalEngineResult:
    from g8e_evals.controller import request_controller_stop

    try:
        result = request_controller_stop(
            Path(_required_parameter(request, "work_dir")),
            immediate=request.flags.immediate_stop,
        )
        return succeeded_result(request, result.model_dump(mode="json"))
    except EngineError:
        raise
    except (OSError, ValueError) as exc:
        raise EngineError(EvalErrorCode.STATUS_RECONCILIATION_FAILED, "controller_stop", _safe_detail(exc)) from exc


def _controller_recover(request: EvalEngineRequest) -> EvalEngineResult:
    from g8e_evals.controller import recover_interrupted_controller, retry_controller_publication
    from g8e_evals.controller_cli import PublicOutboxPublisher

    try:
        work_dir = Path(_required_parameter(request, "work_dir"))
        if request.parameters.get("publication") == "true":
            state = retry_controller_publication(
                work_dir,
                PublicOutboxPublisher(Path(request.platform.g8e_binary_path), work_dir),
            )
        else:
            state = recover_interrupted_controller(work_dir)
        payload = state.model_dump(mode="json")
        if request.parameters.get("publication") == "true" and state.status.value != "completed":
            return failed_result(
                request,
                EvalErrorCode.STATUS_RECONCILIATION_FAILED,
                "controller_recover",
                f"controller publication remains stopped: {state.stop_reason}",
                payload,
            )
        return succeeded_result(request, payload)
    except EngineError:
        raise
    except (OSError, ValueError) as exc:
        raise EngineError(EvalErrorCode.STATUS_RECONCILIATION_FAILED, "controller_recover", _safe_detail(exc)) from exc


def _bundle(request: EvalEngineRequest) -> EvalEngineResult:
    from g8e_evals.bundle.produce import BundleProductionError, produce_bundle
    from g8e_evals.bundle.signing import EvalSigningKey

    try:
        signing_key = EvalSigningKey.from_seed(Path(_required_parameter(request, "signing_key_path")).read_bytes())
        manifest = produce_bundle(
            report_dir=Path(request.report_root),
            bundle_dir=Path(_required_parameter(request, "bundle_dir")),
            bundle_id=_required_parameter(request, "bundle_id"),
            signing_key=signing_key,
        )
        return succeeded_result(request, manifest.model_dump(mode="json"))
    except EngineError:
        raise
    except FileExistsError as exc:
        raise EngineError(EvalErrorCode.REPORT_ROOT_REUSED, "bundle", _safe_detail(exc)) from exc
    except (BundleProductionError, OSError, ValueError) as exc:
        raise EngineError(EvalErrorCode.AUTHORITY_INVALID, "bundle", _safe_detail(exc)) from exc


def _verify_bundle(request: EvalEngineRequest) -> EvalEngineResult:
    from g8e_evals.bundle.signing import EvalTrustStore
    from g8e_evals.bundle.verify import verify_bundle

    try:
        trust_store_path = _optional_path(request, "trust_store")
        trust_store = (
            EvalTrustStore.model_validate_json(trust_store_path.read_text())
            if trust_store_path is not None
            else None
        )
        report = verify_bundle(Path(request.report_root), trust_store=trust_store)
    except (OSError, ValueError) as exc:
        raise EngineError(EvalErrorCode.CONFIG_INVALID, "verify", _safe_detail(exc)) from exc
    payload = report.model_dump(mode="json")
    if not report.ok:
        return failed_result(
            request,
            EvalErrorCode.AUTHORITY_INVALID,
            "verify",
            f"bundle verification failed with {len(report.failures)} failure(s)",
            payload,
        )
    return succeeded_result(request, payload)


def _verify_receipts(request: EvalEngineRequest) -> EvalEngineResult:
    import click

    from g8e_evals.cli import verify_receipts

    output = io.StringIO()
    callback = verify_receipts.callback
    if callback is None:
        raise EngineError(EvalErrorCode.CHILD_START_FAILED, "verify_receipts", "receipt verifier is unavailable")
    try:
        with redirect_stdout(output):
            callback(
                report_dir=Path(request.report_root),
                pki_dir=Path(_required_parameter(request, "pki_dir")),
                json_output=True,
            )
    except click.exceptions.Exit as exc:
        payload = json.loads(output.getvalue())
        return failed_result(
            request,
            EvalErrorCode.AUTHORITY_INVALID,
            "verify_receipts",
            f"receipt verification exited with status {exc.exit_code}",
            payload,
        )
    except EngineError:
        raise
    except (OSError, ValueError, click.ClickException) as exc:
        raise EngineError(EvalErrorCode.AUTHORITY_INVALID, "verify_receipts", _safe_detail(exc)) from exc
    return succeeded_result(request, json.loads(output.getvalue()))


def _write_new_json(path: Path, payload: bytes) -> None:
    with path.open("xb") as output:
        output.write(payload)


def _qualification_hash_source(request: EvalEngineRequest) -> EvalEngineResult:
    from g8e_evals.qualification import compute_source_manifest_result, render_qualification_json

    try:
        result = compute_source_manifest_result(
            Path(_required_parameter(request, "authority")),
            Path(_required_parameter(request, "source_root")),
            _required_parameter(request, "authority_record_path"),
        )
        output = Path(_required_parameter(request, "output"))
        _write_new_json(output, render_qualification_json(result))
        return succeeded_result(request, result.model_dump(mode="json"))
    except EngineError:
        raise
    except FileExistsError as exc:
        raise EngineError(EvalErrorCode.REPORT_ROOT_REUSED, "qualification_hash_source", _safe_detail(exc)) from exc
    except (OSError, ValueError) as exc:
        raise EngineError(EvalErrorCode.CONFIG_INVALID, "qualification_hash_source", _safe_detail(exc)) from exc


def _qualification_candidate(request: EvalEngineRequest) -> EvalEngineResult:
    from pydantic import TypeAdapter, ValidationError

    from g8e_evals.qualification import (
        CandidateIdentityEvidence,
        ComponentImageIdentity,
        SourceManifestResult,
        render_qualification_json,
    )

    try:
        full_source = SourceManifestResult.model_validate_json(
            Path(_required_parameter(request, "full_source")).read_bytes()
        )
        execution_source = SourceManifestResult.model_validate_json(
            Path(_required_parameter(request, "execution_source")).read_bytes()
        )
        if full_source.scope != "full_source" or execution_source.scope != "execution_source":
            raise ValueError("candidate source manifest scopes are invalid")
        binary = Path(_required_parameter(request, "binary"))
        if binary.is_symlink():
            raise ValueError("candidate binary must not be a symlink")
        images = TypeAdapter(list[ComponentImageIdentity]).validate_json(_required_parameter(request, "images"))
        result = CandidateIdentityEvidence.build(
            source_tree_hash=full_source.source_tree_hash,
            execution_source_manifest_hash=execution_source.source_tree_hash,
            binary_sha256=hashlib.sha256(binary.read_bytes()).hexdigest(),
            images=images,
        )
        _write_new_json(Path(_required_parameter(request, "output")), render_qualification_json(result))
        return succeeded_result(request, result.model_dump(mode="json"))
    except EngineError:
        raise
    except FileExistsError as exc:
        raise EngineError(EvalErrorCode.REPORT_ROOT_REUSED, "qualification_candidate", _safe_detail(exc)) from exc
    except (OSError, ValueError, ValidationError) as exc:
        raise EngineError(EvalErrorCode.CONFIG_INVALID, "qualification_candidate", _safe_detail(exc)) from exc


def _qualification_collect_runtime(request: EvalEngineRequest) -> EvalEngineResult:
    from pydantic import ValidationError

    from g8e_evals.qualification import RuntimeCollectionRequest, collect_runtime_identity, render_qualification_json

    try:
        request_path = Path(_required_parameter(request, "request"))
        collection = RuntimeCollectionRequest.model_validate_json(request_path.read_bytes())
        result = collect_runtime_identity(collection, request_path.parent)
        _write_new_json(Path(_required_parameter(request, "output")), render_qualification_json(result))
        return succeeded_result(request, result.model_dump(mode="json"))
    except EngineError:
        raise
    except FileExistsError as exc:
        raise EngineError(EvalErrorCode.REPORT_ROOT_REUSED, "qualification_collect_runtime", _safe_detail(exc)) from exc
    except (OSError, ValueError, ValidationError) as exc:
        raise EngineError(EvalErrorCode.CONFIG_INVALID, "qualification_collect_runtime", _safe_detail(exc)) from exc


def _qualification_run_gate(request: EvalEngineRequest) -> EvalEngineResult:
    from pydantic import TypeAdapter, ValidationError

    from g8e_evals.qualification import (
        CandidateIdentityEvidence,
        GateResultEvidence,
        render_qualification_json,
    )

    try:
        candidate = CandidateIdentityEvidence.model_validate_json(
            Path(_required_parameter(request, "candidate")).read_bytes()
        )
        command = TypeAdapter(list[str]).validate_json(_required_parameter(request, "command"))
        versions = TypeAdapter(dict[str, str]).validate_json(_required_parameter(request, "tool_versions"))
        if not command or any(not value for value in command):
            raise ValueError("gate command arguments must be non-empty")
        started_at = datetime.now(UTC)
        completed = subprocess.run(command, check=False, capture_output=True)
        result = GateResultEvidence.build(
            gate_id=_required_parameter(request, "gate_id"),
            candidate_content_hash=candidate.content_hash,
            command=command,
            started_at=started_at,
            completed_at=datetime.now(UTC),
            exit_code=completed.returncode,
            stdout_sha256=hashlib.sha256(completed.stdout).hexdigest(),
            stderr_sha256=hashlib.sha256(completed.stderr).hexdigest(),
            tool_versions=versions,
            skipped=[],
        )
        _write_new_json(Path(_required_parameter(request, "output")), render_qualification_json(result))
    except EngineError:
        raise
    except FileExistsError as exc:
        raise EngineError(EvalErrorCode.REPORT_ROOT_REUSED, "qualification_run_gate", _safe_detail(exc)) from exc
    except (OSError, ValueError, ValidationError) as exc:
        raise EngineError(EvalErrorCode.CONFIG_INVALID, "qualification_run_gate", _safe_detail(exc)) from exc
    payload = result.model_dump(mode="json")
    if result.exit_code != 0:
        return failed_result(
            request,
            EvalErrorCode.CHILD_EXIT_NON_ZERO,
            "qualification_run_gate",
            f"qualification gate exited with status {result.exit_code}",
            payload,
        )
    return succeeded_result(request, payload)


def _qualification_build(request: EvalEngineRequest) -> EvalEngineResult:
    from pydantic import ValidationError

    from g8e_evals.qualification import (
        QualificationBuildRequest,
        build_collection_candidate_qualification,
        render_qualification_json,
        resolve_qualification_input,
    )

    try:
        input_path = Path(_required_parameter(request, "input"))
        build_request = QualificationBuildRequest.model_validate_json(input_path.read_bytes())
        result = build_collection_candidate_qualification(resolve_qualification_input(build_request, input_path.parent))
        rendered = render_qualification_json(result)
        check_path = _optional_path(request, "check")
        output_path = _optional_path(request, "output")
        if (check_path is None) == (output_path is None):
            raise ValueError("exactly one of output or check is required")
        if check_path is not None:
            if check_path.read_bytes() != rendered:
                return failed_result(
                    request,
                    EvalErrorCode.AUTHORITY_INVALID,
                    "qualification_build",
                    "qualification draft does not reproduce",
                    result.model_dump(mode="json"),
                )
        elif output_path is not None:
            _write_new_json(output_path, rendered)
        return succeeded_result(request, result.model_dump(mode="json"))
    except EngineError:
        raise
    except FileExistsError as exc:
        raise EngineError(EvalErrorCode.REPORT_ROOT_REUSED, "qualification_build", _safe_detail(exc)) from exc
    except (OSError, ValueError, ValidationError) as exc:
        raise EngineError(EvalErrorCode.CONFIG_INVALID, "qualification_build", _safe_detail(exc)) from exc


def _bench_synthetic(request: EvalEngineRequest) -> EvalEngineResult:
    from g8e_evals.cli import _run_synthetic_suite, load_preregistration
    from g8e_evals.suites import assert_simulation_eligible

    try:
        gold_set = _optional_path(request, "gold_set")
        if gold_set is None:
            gold_set = Path(request.platform.eval_project) / assert_simulation_eligible(
                _required_parameter(request, "suite")
            ).default_gold_set
        preregistration_path = _optional_path(request, "preregistration")
        preregistration = load_preregistration(preregistration_path) if preregistration_path is not None else None
        limit_value = request.parameters.get("limit")
        limit = int(limit_value) if limit_value else None
        with redirect_stdout(sys.stderr):
            report_dir = asyncio.run(
                _run_synthetic_suite(
                    _required_parameter(request, "suite"),
                    gold_set,
                    Path(request.report_root),
                    limit,
                    preregistration=preregistration,
                )
            )
        return succeeded_result(request, {"report_root": str(report_dir)})
    except EngineError:
        raise
    except (OSError, ValueError) as exc:
        raise EngineError(EvalErrorCode.CHILD_EXIT_NON_ZERO, "bench_synthetic", _safe_detail(exc)) from exc


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
        EvalOperation.CAMPAIGN_SET_PLAN: _campaign_set_plan,
        EvalOperation.CAMPAIGN_SET_VALIDATE: _campaign_set_validate,
        EvalOperation.CAMPAIGN_SET_VERIFY: _campaign_set_verify,
        EvalOperation.CONTROLLER_STATUS: _controller_status,
        EvalOperation.CONTROLLER_STOP: _controller_stop,
        EvalOperation.CONTROLLER_RECOVER: _controller_recover,
        EvalOperation.BUNDLE: _bundle,
        EvalOperation.VERIFY: _verify_bundle,
        EvalOperation.VERIFY_RECEIPTS: _verify_receipts,
        EvalOperation.QUALIFICATION_HASH_SOURCE: _qualification_hash_source,
        EvalOperation.QUALIFICATION_CANDIDATE: _qualification_candidate,
        EvalOperation.QUALIFICATION_COLLECT_RUNTIME: _qualification_collect_runtime,
        EvalOperation.QUALIFICATION_RUN_GATE: _qualification_run_gate,
        EvalOperation.QUALIFICATION_BUILD: _qualification_build,
        EvalOperation.BENCH_SYNTHETIC: _bench_synthetic,
    }
    return handlers
