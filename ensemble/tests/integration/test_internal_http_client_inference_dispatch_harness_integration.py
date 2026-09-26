# Copyright (c) 2026 Lateralus Labs, LLC.
# Use of this source code is governed by the Business Source License
# included in the LICENSE file.
#
# As of the Change Date listed in the LICENSE file, this software is
# released under the Apache License, Version 2.0.

"""Tier-2 Ensemble→Gateway inference dispatch harness integration."""

from __future__ import annotations

import json
import os
import shutil
import subprocess
import time
from pathlib import Path

import pytest

from app.models.internal_api import InferenceDispatchRequest
from app.models.settings import ComponentURLsSettings, G8eeAppSettings
from app.services.infra.internal_http_client import InternalHttpClient
from g8e.constants import PLATFORM
from g8e.operator.v1.operator_pb2 import (
    EXECUTION_STATUS_COMPLETED,
    INFERENCE_MESSAGE_ROLE_USER,
    MODEL_ROLE_ASSISTANT,
)

pytestmark = [pytest.mark.integration]

REPO_ROOT = Path(__file__).resolve().parents[3]
HARNESS_RUNNER = REPO_ROOT / "ensemble" / "tests" / "integration" / "inference_dispatch_harness_runner.py"


def _require_go() -> str:
    go_bin = shutil.which("go")
    if go_bin is None:
        pytest.skip("go toolchain is not available for inference dispatch harness")
    return go_bin


def _wait_for_fixture(path: Path, timeout_seconds: float = 120.0) -> dict[str, str]:
    deadline = time.monotonic() + timeout_seconds
    while time.monotonic() < deadline:
        if path.is_file():
            try:
                return json.loads(path.read_text(encoding="utf-8"))
            except json.JSONDecodeError:
                pass
        time.sleep(0.1)
    raise AssertionError(f"timed out waiting for harness fixture at {path}")


def _build_settings(fixture: dict[str, str]) -> G8eeAppSettings:
    settings = G8eeAppSettings(
        component_urls=ComponentURLsSettings(client_url=fixture["client_url"]),
    )
    settings._ca_cert_path = fixture["ca_cert_path"]
    settings._client_cert_path = fixture["client_cert_path"]
    settings._client_key_path = fixture["client_key_path"]
    return settings


@pytest.mark.asyncio
async def test_dispatch_inference_round_trips_through_go_harness_gateway(tmp_path: Path):
    """Python InternalHttpClient drives a real in-process Go gateway over mTLS."""
    go_bin = _require_go()
    if not HARNESS_RUNNER.is_file():
        pytest.skip("inference dispatch harness runner is unavailable")

    fixture_path = tmp_path / "harness.json"
    env = {
        **os.environ,
        "G8E_INFERENCE_DISPATCH_HARNESS": "serve",
        "G8E_INFERENCE_DISPATCH_HARNESS_FIXTURE": str(fixture_path),
    }
    proc = subprocess.Popen(
        [
            go_bin,
            "test",
            "-tags=integration",
            "-run",
            "^TestInferenceDispatchPythonHarnessServe$",
            "-count=1",
            "-timeout=0",
            "./internal/services/gateway/",
        ],
        cwd=REPO_ROOT,
        env=env,
        stdout=subprocess.DEVNULL,
        stderr=subprocess.DEVNULL,
    )
    harness_output = ""
    try:
        fixture = _wait_for_fixture(fixture_path)
        settings = _build_settings(fixture)
        client = InternalHttpClient(settings)

        request = InferenceDispatchRequest(
            role=MODEL_ROLE_ASSISTANT,
            request_schema_version=PLATFORM["platform"]["InferenceRequestSchemaVersion"]["value"],
            target_operator_session_id=fixture["target_operator_session_id"],
            provider_attempt_id="provider-attempt-pytest-harness",
        )
        message = request.messages.add(role=INFERENCE_MESSAGE_ROLE_USER)
        message.parts.add(text=fixture["prompt_text"])

        try:
            response = await client.dispatch_inference(request)
        finally:
            await client.close()

        assert response.HasField("result")
        assert response.result.parts[0].text == fixture["expected_response_text"]
        assert response.HasField("receipt")
        assert response.receipt.status == EXECUTION_STATUS_COMPLETED
        assert response.result.result_digest == response.receipt.result_summary
    finally:
        proc.terminate()
        try:
            harness_output, _ = proc.communicate(timeout=30)
        except subprocess.TimeoutExpired:
            proc.kill()
            harness_output, _ = proc.communicate(timeout=10)
        assert proc.returncode in {0, -15}, harness_output
