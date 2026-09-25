# Copyright (c) 2026 Lateralus Labs, LLC.

import pytest

from app.constants import OperatorStatus
from app.models.operators import OperatorDocument
from app.utils.gateway_operator_document import operator_document_from_gateway

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
