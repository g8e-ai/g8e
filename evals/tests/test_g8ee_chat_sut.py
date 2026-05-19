"""Regression tests for G8eeChatSUT receipt-binding semantics.

The Operator's audit vault keys ActionReceipts by the UAP envelope
transaction_id (i.e. transaction_hash of a Warden-signed envelope), NOT by
the g8ee-issued investigation_id. A plain answer-only chat turn does not
trigger a Tribunal->Warden mutation and therefore produces no on-substrate
ActionReceipt. The SUT must not lie about that:

  - It must not pass investigation_id off as transaction_id.
  - It must not claim BindingType.RECEIPT_BOUND when no Warden-signed
    receipt was observed in the agent trail.

This guards against a latent semantic bug where ReceiptCollector polled the
Operator with an investigation_id and silently timed out for every task.
"""

from g8e_evals.harness import BindingType
from g8e_evals.sut.g8ee_chat import (
    AgentTrailEvent,
    _extract_substrate_transaction_id,
)


def test_extract_substrate_transaction_id_ignores_investigation_only_trail():
    trail = [
        AgentTrailEvent(
            id=1,
            event_type="g8e.v1.ai.llm.chat.iteration.text.chunk.received",
            payload={"investigation_id": "inv-abc", "data": {"content": "hi"}},
        ),
        AgentTrailEvent(
            id=2,
            event_type="g8e.v1.ai.llm.chat.iteration.text.completed",
            payload={"investigation_id": "inv-abc"},
        ),
    ]
    assert _extract_substrate_transaction_id(trail) is None


def test_extract_substrate_transaction_id_picks_warden_signed_receipt():
    trail = [
        AgentTrailEvent(
            id=1,
            event_type="g8e.v1.ai.llm.chat.iteration.text.chunk.received",
            payload={"investigation_id": "inv-abc"},
        ),
        AgentTrailEvent(
            id=2,
            event_type="g8e.v1.ai.governance.warden.receipt.signed",
            payload={
                "event": {
                    "type": "g8e.v1.ai.governance.warden.receipt.signed",
                    "data": {"transaction_hash": "tx-real-substrate-id"},
                },
                "investigation_id": "inv-abc",
            },
        ),
    ]
    assert _extract_substrate_transaction_id(trail) == "tx-real-substrate-id"


def test_extract_substrate_transaction_id_ignores_investigation_id_lookalikes():
    # An investigation_id used as a transaction_id on a non-substrate event
    # must NOT be promoted to a substrate transaction id.
    trail = [
        AgentTrailEvent(
            id=1,
            event_type="g8e.v1.app.case.investigation.created",
            payload={"transaction_id": "inv-abc"},
        ),
    ]
    assert _extract_substrate_transaction_id(trail) is None


def test_binding_unbound_when_no_substrate_receipt(monkeypatch):
    # Smoke-import to ensure the SUT's UNBOUND-no-receipt branch references
    # are valid module-level symbols; full end-to-end binding is exercised by
    # the live ifeval bench.
    from g8e_evals.sut import g8ee_chat

    assert hasattr(g8ee_chat, "_extract_substrate_transaction_id")
    assert BindingType.UNBOUND.value == "UNBOUND"
    assert BindingType.RECEIPT_BOUND.value == "RECEIPT_BOUND"
