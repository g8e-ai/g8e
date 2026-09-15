# Copyright (c) 2026 Lateralus Labs, LLC.
# Use of this source code is governed by the Business Source License
# included in the LICENSE file.
#
# As of the Change Date listed in the LICENSE file, this software is
# released under the Apache License, Version 2.0.

from __future__ import annotations

import json
import os
import sys
from collections.abc import Callable
from contextlib import redirect_stdout
from datetime import UTC, datetime
from pathlib import Path

from g8e_evals.engine_cli import EngineError, OperationHandler
from g8e_evals.engine_protocol import (
    EvalErrorCode,
    EvalEngineRequest,
    EvalEngineResult,
    EvalOperation,
    succeeded_result,
)
from g8e_evals.constants import EVAL_LAUNCH_STATE_JSON
from g8e_evals.operation_config import CampaignConfig, DiagnosticConfig, load_operation_config
from g8e_evals.operation_lifecycle import operation_status, resolve_report_root, stop_operation, verify_operation


def _provider_api_key(provider: str) -> str | None:
    names = {
        "openai": "OPENAI_API_KEY",
        "anthropic": "ANTHROPIC_API_KEY",
    }
    return os.environ.get(names.get(provider, "G8E_TEST_LLM_PRIMARY_API_KEY"))


def _provider_endpoint(provider: str) -> str | None:
    names = {
        "openai": "OPENAI_BASE_URL",
        "ollama": "OLLAMA_HOST",
    }
    return os.environ.get(names.get(provider, "G8E_TEST_LLM_PRIMARY_ENDPOINT_URL"))


def _write_launch_state(report_root: Path, request: EvalEngineRequest, status: str) -> None:
    state = {
        "operation_id": request.operation_id,
        "revision": request.revision,
        "pid": os.getpid(),
        "status": status,
        "updated_at": datetime.now(UTC).isoformat(),
    }
    (report_root / EVAL_LAUNCH_STATE_JSON).write_text(json.dumps(state, sort_keys=True, indent=2) + "\n")


def _diagnostic_start(request: EvalEngineRequest) -> EvalEngineResult:
    from g8e_evals.cli import run

    config = load_operation_config(Path(request.config_path))
    if not isinstance(config, DiagnosticConfig):
        raise EngineError(EvalErrorCode.CONFIG_INVALID, "start", "diagnostic start requires a diagnostic config")
    repository_root = Path(request.platform.repository_root)
    report_root = resolve_report_root(config, repository_root)
    if report_root.exists():
        raise EngineError(EvalErrorCode.REPORT_ROOT_REUSED, "start", "report root already exists")
    report_root.mkdir(parents=True)
    _write_launch_state(report_root, request, "running")
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
                state_root="eval-operation-state",
                output_dir=report_root.parent,
                evidence_key_file=repository_root / config.evidence_key.path,
                gold_set=repository_root / config.gold_set.path,
                limit=config.task_limit,
                l2_key=None,
                l2_key_id=None,
                primary_api_key=_provider_api_key(config.provider_endpoint.provider),
                primary_endpoint=_provider_endpoint(config.provider_endpoint.provider),
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
            )
    except Exception as exc:
        _write_launch_state(report_root, request, "failed")
        raise EngineError(EvalErrorCode.CHILD_EXIT_NON_ZERO, "diagnostic_start", str(exc)) from exc
    _write_launch_state(report_root, request, "completed")
    return succeeded_result(request, {"report_root": str(report_root)})


def _campaign_start(request: EvalEngineRequest) -> EvalEngineResult:
    from g8e_evals.cli import campaign_run

    config = load_operation_config(Path(request.config_path))
    if not isinstance(config, CampaignConfig):
        raise EngineError(EvalErrorCode.CONFIG_INVALID, "start", "campaign start requires a campaign config")
    repository_root = Path(request.platform.repository_root)
    report_root = resolve_report_root(config, repository_root)
    if report_root.exists():
        raise EngineError(EvalErrorCode.REPORT_ROOT_REUSED, "start", "report root already exists")
    report_root.mkdir(parents=True)
    _write_launch_state(report_root, request, "running")
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
            )
    except Exception as exc:
        _write_launch_state(report_root, request, "failed")
        raise EngineError(EvalErrorCode.CHILD_EXIT_NON_ZERO, "campaign_start", str(exc)) from exc
    _write_launch_state(report_root, request, "completed")
    return succeeded_result(request, {"report_root": str(report_root)})


def _status(request: EvalEngineRequest) -> EvalEngineResult:
    try:
        result = operation_status(Path(request.config_path), Path(request.platform.repository_root))
    except (OSError, ValueError) as exc:
        raise EngineError(EvalErrorCode.STATUS_RECONCILIATION_FAILED, "status", str(exc)) from exc
    return succeeded_result(request, result.model_dump(exclude_none=True))


def _stop(request: EvalEngineRequest) -> EvalEngineResult:
    try:
        result = stop_operation(
            Path(request.config_path),
            Path(request.platform.repository_root),
            immediate=request.flags.immediate_stop,
        )
    except (OSError, ValueError) as exc:
        raise EngineError(EvalErrorCode.STATUS_RECONCILIATION_FAILED, "stop", str(exc)) from exc
    return succeeded_result(request, result.model_dump(exclude_none=True))


def _verify(request: EvalEngineRequest) -> EvalEngineResult:
    try:
        result = verify_operation(Path(request.config_path), Path(request.platform.repository_root))
    except (OSError, ValueError) as exc:
        raise EngineError(EvalErrorCode.AUTHORITY_INVALID, "verify", str(exc)) from exc
    if not result.ok:
        raise EngineError(
            EvalErrorCode.AUTHORITY_INVALID,
            "verify",
            f"offline verification failed with {len(result.failures)} failure(s)",
        )
    return succeeded_result(request, result.model_dump(exclude_none=True))


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
