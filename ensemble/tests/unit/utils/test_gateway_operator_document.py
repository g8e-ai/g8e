# Copyright (c) 2026 Lateralus Labs, LLC.

import pytest

from app.constants import OperatorStatus
from app.models.operators import OperatorDocument
from app.utils.gateway_decoding.gateway_operator_document import operator_document_from_gateway

pytestmark = [pytest.mark.unit]


def test_operator_document_from_gateway_accepts_canonical_id():
    doc = operator_document_from_gateway(
        {
            "id": "op-1",
            "user_id": "user-1",
            "status": OperatorStatus.ACTIVE,
            "current_hostname": "worker-1",
        }
    )

    assert isinstance(doc, OperatorDocument)
    assert doc.id == "op-1"
    assert doc.current_hostname == "worker-1"


def test_operator_document_from_gateway_normalizes_operator_id_alias():
    doc = operator_document_from_gateway(
        {
            "operator_id": "op-2",
            "user_id": "user-1",
            "status": OperatorStatus.BOUND,
        }
    )

    assert doc.id == "op-2"


def test_operator_document_from_gateway_parses_canonical_heartbeat_snapshot():
    doc = operator_document_from_gateway(
        {
            "id": "op-3",
            "user_id": "user-1",
            "status": OperatorStatus.ACTIVE,
            "latest_heartbeat_snapshot": {
                "timestamp": "2026-09-18T12:00:00Z",
                "status": "automatic",
                "system_identity": {
                    "hostname": "worker-1",
                    "os": "linux",
                },
                "performance_metrics": {
                    "cpu_percent": 12.5,
                },
            },
        }
    )

    assert doc.current_hostname == "worker-1"
    assert doc.latest_heartbeat_snapshot is not None
    assert doc.latest_heartbeat_snapshot.system_identity.hostname == "worker-1"
    assert doc.latest_heartbeat_snapshot.status == "automatic"


def test_operator_document_validator_tolerates_magicmock_in_enriched_context():
    """Regression: after-validator must not break MagicMock(spec=OperatorDocument) test doubles."""
    from unittest.mock import MagicMock

    from app.models.investigations import EnrichedInvestigationContext

    mock_op = MagicMock(spec=OperatorDocument)
    mock_op.id = "op-mock"
    mock_op.operator_session_id = "sess-mock"

    investigation = EnrichedInvestigationContext(
        id="inv-1",
        case_id="case-1",
        user_id="user-1",
        sentinel_mode=False,
        operator_documents=[mock_op],
    )

    assert investigation.operator_documents == [mock_op]
