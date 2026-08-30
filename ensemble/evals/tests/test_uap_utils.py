import pytest

from g8e_evals.uap_utils import build_envelope

pytestmark = pytest.mark.unit


def test_build_envelope_records_l2_consensus_set_id():
    envelope = build_envelope(
        action_type="test.action",
        payload=b"payload",
        operator_id="operator-id",
        operator_session_id="session-id",
        state_root="state-root",
        nonce="nonce",
        l2_private_key="00" * 32,
        l2_key_id="consensus-set-id",
    )

    assert envelope.governance.l2.consensus_set_id == "consensus-set-id"
    assert envelope.governance.l2.votes[0].signer_key_id == "consensus-set-id"
