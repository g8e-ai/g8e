# Copyright (c) 2026 Lateralus Labs, LLC.
# Use of this source code is governed by the Business Source License
# included in the LICENSE file.
#
# As of the Change Date listed in the LICENSE file, this software is
# released under the Apache License, Version 2.0.

"""Wire contract for the platform enrollment completion transcript.

The Gateway verifies the client's proof-of-possession signature over the
canonical ``PlatformEnrollmentCompletionTranscript`` protobuf bytes. The Python
client hand-encodes those bytes, so any drift from the generated message
changes the signed bytes and enrollment fails only against a real Gateway.

``GOLDEN_TRANSCRIPT_HEX`` is the deterministic serialization of the vector
below. The Go test ``TestPlatformEnrollmentCompletionTranscriptGoldenVector`` in
``internal/services/gateway/platform_enrollment_validation_test.go`` asserts the
same bytes, so both languages are pinned to one contract.
"""

import pytest
from g8e.common.v1 import common_pb2

from app.services.infra import app_enrollment_service as svc_module
from app.services.infra.app_enrollment_service import AppEnrollmentService

REQUEST_ID = "req-0123456789abcdef"
TOKEN_HASH = "a" * 64
INSTANCE_ID = "ensemble-contract-host"
APP_FINGERPRINT = "b" * 64

GOLDEN_TRANSCRIPT_HEX = (
    "0a013112147265712d303132333435363738396162636465661a40"
    + "61" * 64
    + "20022a16656e73656d626c652d636f6e74726163742d686f737432420a40"
    + "62" * 64
)

pytestmark = pytest.mark.unit


def _client_transcript() -> bytes:
    service = AppEnrollmentService(instance_id=INSTANCE_ID)
    return service._build_completion_transcript(
        REQUEST_ID, TOKEN_HASH, INSTANCE_ID, APP_FINGERPRINT
    )


def _generated_transcript() -> bytes:
    message = common_pb2.PlatformEnrollmentCompletionTranscript(
        protocol_version=svc_module._PROTOCOL_VERSION,
        request_id=REQUEST_ID,
        token_hash=TOKEN_HASH,
        component_kind=common_pb2.PLATFORM_COMPONENT_KIND_ENSEMBLE,
        instance_id=INSTANCE_ID,
        fingerprints=common_pb2.PlatformEnrollmentFingerprints(app=APP_FINGERPRINT),
    )
    return message.SerializeToString(deterministic=True)


def test_client_transcript_matches_generated_protobuf_serialization():
    assert _client_transcript() == _generated_transcript()


def test_client_transcript_matches_golden_vector():
    assert _client_transcript().hex() == GOLDEN_TRANSCRIPT_HEX


def test_client_transcript_round_trips_through_generated_message():
    parsed = common_pb2.PlatformEnrollmentCompletionTranscript.FromString(_client_transcript())

    assert parsed.protocol_version == svc_module._PROTOCOL_VERSION
    assert parsed.request_id == REQUEST_ID
    assert parsed.token_hash == TOKEN_HASH
    assert parsed.component_kind == common_pb2.PLATFORM_COMPONENT_KIND_ENSEMBLE
    assert parsed.instance_id == INSTANCE_ID
    assert parsed.fingerprints.app == APP_FINGERPRINT
    assert parsed.fingerprints.operator == ""
    assert parsed.fingerprints.cli == ""


def test_component_kind_constants_match_generated_enum():
    assert svc_module._COMPONENT_KIND_ENUM_DASHBOARD == common_pb2.PLATFORM_COMPONENT_KIND_DASHBOARD
    assert svc_module._COMPONENT_KIND_ENUM_ENSEMBLE == common_pb2.PLATFORM_COMPONENT_KIND_ENSEMBLE
    assert svc_module._COMPONENT_KIND_ENUM_OPERATOR == common_pb2.PLATFORM_COMPONENT_KIND_OPERATOR
