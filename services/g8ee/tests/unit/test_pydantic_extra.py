import pytest
from pydantic import ValidationError

from app.constants import ComponentName
from app.models.http_context import RequestContext
from app.models.internal_api import InternalOperatorAuthCall


def _ctx() -> dict:
    return RequestContext(
        case_id="case-auth",
        investigation_id="inv-auth",
        source_component=ComponentName.CLIENT,
    ).model_dump()


def test_operator_authenticate_request_fails_with_extra():
    payload = {
        "context": _ctx(),
        "authorization": "Bearer some_key",
        "extra_field": "some_value",
    }
    # This should fail because G8eBaseModel (via ConfigDict) forbids extra fields
    with pytest.raises(ValidationError):
        InternalOperatorAuthCall(**payload)


def test_operator_authenticate_request_succeeds_with_auth():
    payload = {
        "context": _ctx(),
        "authorization": "Bearer some_key",
        "operator_session_id": "session-uuid-123",
    }
    model = InternalOperatorAuthCall(**payload)
    assert model.authorization == "Bearer some_key"
    assert model.operator_session_id == "session-uuid-123"
    assert model.context.source_component == ComponentName.CLIENT


def test_operator_authenticate_request_requires_context():
    # Without a context, the typed body-based identity contract is violated
    with pytest.raises(ValidationError):
        InternalOperatorAuthCall(authorization="Bearer some_key")
