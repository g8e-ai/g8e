# Copyright (c) 2026 Lateralus Labs, LLC.
# Use of this source code is governed by the Business Source License
# included in the LICENSE file.
#
# As of the Change Date listed in the LICENSE file, this software is
# released under the Apache License, Version 2.0.

import pytest

from app.errors import NetworkError
from app.models.internal_api import InferenceDispatchRequest
from app.services.infra.internal_http_client import InternalHttpClient
from app.services.infra.settings_service import SettingsService
from g8e.operator.v1.operator_pb2 import (
    INFERENCE_MESSAGE_ROLE_USER,
    MODEL_ROLE_ASSISTANT,
)

pytestmark = [pytest.mark.integration, pytest.mark.requires_operator]


@pytest.mark.asyncio
async def test_dispatch_inference_reaches_gateway_validation_over_real_mtls():
    settings = SettingsService().get_local_settings()
    if not settings.ca_cert_path:
        pytest.skip("Gateway CA bundle is not available for live mTLS integration")
    if not settings.client_cert_path or not settings.client_key_path:
        pytest.skip("Ensemble app credentials are not available for live mTLS integration")

    client = InternalHttpClient(settings)
    request = InferenceDispatchRequest(
        role=MODEL_ROLE_ASSISTANT,
        model="integration-no-provider-call",
        max_tokens=1,
    )
    message = request.messages.add(role=INFERENCE_MESSAGE_ROLE_USER)
    message.parts.add(text="")

    try:
        with pytest.raises(NetworkError) as exc_info:
            await client.dispatch_inference(request)
    finally:
        await client.close()

    details = exc_info.value.error_detail.details
    if "status_code" not in details:
        pytest.skip("Gateway not reachable for live mTLS integration")
    assert details["status_code"] == 400
