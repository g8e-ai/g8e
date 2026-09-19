import pytest

from app.errors import NetworkError
from app.models.internal_api import InferenceDispatchRequest
from app.services.infra.internal_http_client import InternalHttpClient
from app.services.infra.settings_service import SettingsService
from g8e.operator.v1.operator_pb2 import MODEL_ROLE_ASSISTANT

pytestmark = [pytest.mark.integration, pytest.mark.requires_operator]


@pytest.mark.asyncio
async def test_dispatch_inference_reaches_gateway_validation_over_real_mtls():
    settings = SettingsService().get_local_settings()
    if not settings.component_urls.client_url:
        pytest.skip("Gateway URL is not configured for live mTLS integration")
    if not settings.ca_cert_path:
        pytest.skip("Gateway CA bundle is not available for live mTLS integration")
    if not settings.client_cert_path or not settings.client_key_path:
        pytest.skip("Ensemble app credentials are not available for live mTLS integration")

    client = InternalHttpClient(settings)
    request = InferenceDispatchRequest(
        role=MODEL_ROLE_ASSISTANT,
        prompt="",
        model="integration-no-provider-call",
        max_tokens=1,
    )

    try:
        with pytest.raises(NetworkError) as exc_info:
            await client.dispatch_inference(request)
    finally:
        await client.close()

    assert exc_info.value.error_detail.details["status_code"] == 400
