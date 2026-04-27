import pytest
from pydantic import ValidationError
from app.models.internal_api import InternalOperatorAuthCall

def test_operator_authenticate_request_fails_with_extra():
    payload = {
        "authorization": "Bearer some_key",
        "system_info": {"os": "linux"},
        "extra_field": "some_value"
    }
    # This should fail because G8eBaseModel (via ConfigDict) forbids extra fields
    with pytest.raises(ValidationError):
        InternalOperatorAuthCall(**payload)

def test_operator_authenticate_request_succeeds_with_auth():
    payload = {
        "authorization": "Bearer some_key",
        "system_info": {"os": "linux"}
    }
    model = InternalOperatorAuthCall(**payload)
    assert model.system_info == {"os": "linux"}
    assert model.authorization == "Bearer some_key"
