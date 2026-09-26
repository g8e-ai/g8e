# Copyright (c) 2026 Lateralus Labs, LLC.
# Use of this source code is governed by the Business Source License
# included in the LICENSE file.
#
# As of the Change Date listed in the LICENSE file, this software is
# released under the Apache License, Version 2.0.

"""Run InternalHttpClient.dispatch_inference against a harness fixture.

The Go integration harness writes a JSON fixture with TLS material and the
listener URL, then invokes this script as a subprocess. The script uses the
real Python HTTP client and protobuf transport without mocking gateway internals.
"""

from __future__ import annotations

import asyncio
import json
import os
import sys
from pathlib import Path

_ENSEMBLE_ROOT = Path(__file__).resolve().parents[2]
_REPO_ROOT = _ENSEMBLE_ROOT.parent
_PROTOCOL_PYTHON_ROOT = _REPO_ROOT / "protocol" / "python"

for _path in (_ENSEMBLE_ROOT, _PROTOCOL_PYTHON_ROOT, _REPO_ROOT):
    _path_str = str(_path)
    if _path_str not in sys.path:
        sys.path.insert(0, _path_str)

from app.models.internal_api import InferenceDispatchRequest
from app.models.settings import ComponentURLsSettings, G8eeAppSettings
from app.services.infra.internal_http_client import InternalHttpClient
from g8e.constants import PLATFORM
from g8e.operator.v1.operator_pb2 import (
    EXECUTION_STATUS_COMPLETED,
    INFERENCE_MESSAGE_ROLE_USER,
    MODEL_ROLE_ASSISTANT,
)


def _load_fixture() -> dict[str, str]:
    fixture_path = os.environ.get("G8E_INFERENCE_DISPATCH_HARNESS_FIXTURE", "").strip()
    if not fixture_path:
        raise SystemExit("G8E_INFERENCE_DISPATCH_HARNESS_FIXTURE is required")
    return json.loads(Path(fixture_path).read_text(encoding="utf-8"))


def _build_settings(fixture: dict[str, str]) -> G8eeAppSettings:
    settings = G8eeAppSettings(
        component_urls=ComponentURLsSettings(client_url=fixture["client_url"]),
    )
    settings._ca_cert_path = fixture["ca_cert_path"]
    settings._client_cert_path = fixture["client_cert_path"]
    settings._client_key_path = fixture["client_key_path"]
    return settings


async def _run() -> None:
    fixture = _load_fixture()
    settings = _build_settings(fixture)
    client = InternalHttpClient(settings)

    request = InferenceDispatchRequest(
        role=MODEL_ROLE_ASSISTANT,
        request_schema_version=PLATFORM["platform"]["InferenceRequestSchemaVersion"]["value"],
        target_operator_session_id=fixture["target_operator_session_id"],
        provider_attempt_id="provider-attempt-python-harness",
    )
    message = request.messages.add(role=INFERENCE_MESSAGE_ROLE_USER)
    message.parts.add(text=fixture["prompt_text"])

    try:
        response = await client.dispatch_inference(request)
    except Exception as exc:  # pragma: no cover - surfaced to Go harness test output
        details = getattr(exc, "error_detail", None)
        if details is not None and details.details:
            raise SystemExit(f"dispatch_inference failed: {exc}; details={details.details}") from exc
        raise
    finally:
        await client.close()

    expected_text = fixture["expected_response_text"]
    if not response.HasField("result"):
        raise SystemExit("dispatch_inference response missing result")
    if response.result.parts[0].text != expected_text:
        raise SystemExit(
            f"unexpected result text: {response.result.parts[0].text!r} != {expected_text!r}"
        )
    if not response.HasField("receipt"):
        raise SystemExit("dispatch_inference response missing receipt")
    if response.receipt.status != EXECUTION_STATUS_COMPLETED:
        raise SystemExit(f"unexpected receipt status: {response.receipt.status}")
    if response.result.result_digest != response.receipt.result_summary:
        raise SystemExit("result_digest does not match receipt.result_summary")


def main() -> None:
    asyncio.run(_run())


if __name__ == "__main__":
    main()
