# Copyright (c) 2026 Lateralus Labs, LLC.
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#     http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.

"""Contract tests for PubSub Service payload parsing."""

import pytest
from app.services.operator.pubsub_service import parse_inbound_g8eo_payload
from app.models.pubsub_messages import (
    G8eoResultPayload,
    ExecutionResultsPayload,
    PortCheckResultPayload,
    FsListResultPayload,
)
from typing import get_args

pytestmark = [pytest.mark.unit]

def test_discriminator_parsing_execution_result():
    """Verify that discriminator-based parsing works for execution results."""
    payload_raw = {
        "payload_type": "execution_result",
        "execution_id": "exec-123",
        "status": "completed",
        "duration_seconds": 1.5,
    }
    result = parse_inbound_g8eo_payload(payload_raw)
    assert isinstance(result, ExecutionResultsPayload)
    assert result.execution_id == "exec-123"
    assert result.payload_type == "execution_result"

def test_discriminator_parsing_port_check():
    """Verify that discriminator-based parsing works for port check results."""
    payload_raw = {
        "payload_type": "port_check_result",
        "execution_id": "exec-456",
        "host": "localhost",
        "port": 8080,
        "is_open": True,
    }
    result = parse_inbound_g8eo_payload(payload_raw)
    assert isinstance(result, PortCheckResultPayload)
    assert result.execution_id == "exec-456"
    assert result.payload_type == "port_check_result"

def test_discriminator_parsing_fs_list():
    """Verify that discriminator-based parsing works for filesystem list results."""
    payload_raw = {
        "payload_type": "fs_list_result",
        "execution_id": "exec-789",
        "path": "/tmp",
        "status": "completed",
        "entries": [],
    }
    result = parse_inbound_g8eo_payload(payload_raw)
    assert isinstance(result, FsListResultPayload)
    assert result.execution_id == "exec-789"
    assert result.payload_type == "fs_list_result"

def test_invalid_payload_type_raises_validation_error():
    """Verify that invalid payload_type raises ValidationError."""
    from app.errors import ValidationError

    payload_raw = {
        "payload_type": "invalid_type",
        "execution_id": "exec-123",
    }

    with pytest.raises(ValidationError):
        parse_inbound_g8eo_payload(payload_raw)

def test_all_payload_models_have_discriminator():
    """Verify that all models in G8eoResultPayload union have a payload_type field."""
    union_types = get_args(G8eoResultPayload)

    for model_class in union_types:
        # Check if the model has a payload_type field
        assert "payload_type" in model_class.model_fields, (
            f"Model {model_class.__name__} is missing payload_type discriminator field"
        )
