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

"""Contract tests for PubSub Service payload mapping."""

import pytest
from app.constants.events import EventType
from app.services.operator.pubsub_service import _PAYLOAD_MODELS
from app.models.pubsub_messages import G8eoResultPayload

pytestmark = [pytest.mark.unit]

def test_payload_models_exhaustiveness():
    """Verify that all relevant operator event types are mapped in _PAYLOAD_MODELS.
    
    This is a contract test to ensure that when we add new operator events,
    we also update the pubsub_service mapping to avoid falling back to
    ExecutionResultsPayload incorrectly.
    """
    # These are handled with explicit if/else in _parse_g8eo_payload
    EXPLICITLY_HANDLED = {
        EventType.OPERATOR_COMMAND_STATUS_UPDATED_RUNNING,
        EventType.OPERATOR_COMMAND_CANCELLED,
    }

    # These are inbound events that we expect to have a payload model mapping
    # We filter EventType to only include OPERATOR_* events that are results
    EXPECTED_OPERATOR_RESULTS = {
        EventType.OPERATOR_COMMAND_COMPLETED,
        EventType.OPERATOR_COMMAND_FAILED,
        EventType.OPERATOR_FILE_EDIT_COMPLETED,
        EventType.OPERATOR_FILE_EDIT_FAILED,
        EventType.OPERATOR_NETWORK_PORT_CHECK_COMPLETED,
        EventType.OPERATOR_NETWORK_PORT_CHECK_FAILED,
        EventType.OPERATOR_FILESYSTEM_LIST_COMPLETED,
        EventType.OPERATOR_FILESYSTEM_LIST_FAILED,
        EventType.OPERATOR_FILESYSTEM_READ_COMPLETED,
        EventType.OPERATOR_FILESYSTEM_READ_FAILED,
        EventType.OPERATOR_FILE_HISTORY_FETCH_COMPLETED,
        EventType.OPERATOR_FILE_HISTORY_FETCH_FAILED,
        EventType.OPERATOR_FILE_RESTORE_COMPLETED,
        EventType.OPERATOR_FILE_RESTORE_FAILED,
        EventType.OPERATOR_FILE_DIFF_FETCH_COMPLETED,
        EventType.OPERATOR_FILE_DIFF_FETCH_FAILED,
        EventType.OPERATOR_LOGS_FETCH_COMPLETED,
        EventType.OPERATOR_LOGS_FETCH_FAILED,
        EventType.OPERATOR_HISTORY_FETCH_COMPLETED,
        EventType.OPERATOR_HISTORY_FETCH_FAILED,
    }

    for event_type in EXPECTED_OPERATOR_RESULTS:
        if event_type in EXPLICITLY_HANDLED:
            continue
        assert event_type in _PAYLOAD_MODELS, f"EventType.{event_type.name} missing from _PAYLOAD_MODELS in pubsub_service.py"

def test_payload_models_registry_alignment():
    """Verify that all models in _PAYLOAD_MODELS are part of the G8eoResultPayload union."""
    from typing import get_args
    
    # Get all concrete types in the Union
    union_types = get_args(G8eoResultPayload)
    
    for event_type, model_class in _PAYLOAD_MODELS.items():
        assert model_class in union_types, (
            f"Model {model_class.__name__} for {event_type} is not in G8eoResultPayload union "
            "in app/models/pubsub_messages.py"
        )
