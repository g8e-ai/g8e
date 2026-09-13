from __future__ import annotations

from pathlib import Path

import pytest

from g8e_evals.constants import (
    EF7_TRANSPORT_DISPOSITION_JSON,
    GOVERNED_INFERENCE_SMOKE_AUTHORITY_JSON,
    LIVE_OPERATION_BUDGET_AUTHORITIES_JSON,
    LIVE_OPERATION_LEASE_TEMPLATES_JSON,
    LIVE_PACKET_RELATIVE_DIR,
)
from g8e_evals.phase0_authority_builder import (
    build_phase0_authority_packet,
    compare_rendered_authority,
)

pytestmark = pytest.mark.integration


def test_phase0_authority_artifacts_reproduce_byte_for_byte() -> None:
    repository_root = Path(__file__).resolve().parents[3]
    live_packet = repository_root / LIVE_PACKET_RELATIVE_DIR
    packet = build_phase0_authority_packet()

    assert compare_rendered_authority(
        packet.ef7_transport_disposition,
        live_packet / EF7_TRANSPORT_DISPOSITION_JSON,
    )
    assert compare_rendered_authority(
        packet.governed_inference_smoke,
        live_packet / GOVERNED_INFERENCE_SMOKE_AUTHORITY_JSON,
    )
    assert compare_rendered_authority(
        packet.budget_authorities,
        live_packet / LIVE_OPERATION_BUDGET_AUTHORITIES_JSON,
    )
    assert compare_rendered_authority(
        packet.lease_templates,
        live_packet / LIVE_OPERATION_LEASE_TEMPLATES_JSON,
    )
