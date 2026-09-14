from __future__ import annotations

from pathlib import Path

import pytest

from g8e_evals.constants import HISTORICAL_QUALIFICATION_FILENAMES, LIVE_PACKET_RELATIVE_DIR
from g8e_evals.qualification import verify_historical_qualification

pytestmark = pytest.mark.integration


def test_historical_qualification_records_reproduce_content_and_file_hashes() -> None:
    repository_root = Path(__file__).resolve().parents[3]
    live_packet = repository_root / LIVE_PACKET_RELATIVE_DIR

    digests = [
        verify_historical_qualification(live_packet / filename)
        for filename in HISTORICAL_QUALIFICATION_FILENAMES
    ]

    assert [digest.name for digest in digests] == list(HISTORICAL_QUALIFICATION_FILENAMES)
    assert all(digest.content_sha256 != digest.file_sha256 for digest in digests)
