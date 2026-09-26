# Copyright (c) 2026 Lateralus Labs, LLC.
# Use of this source code is governed by the Business Source License
# included in the LICENSE file.
#
# As of the Change Date listed in the LICENSE file, this software is
# released under the Apache License, Version 2.0.

import pytest

from g8e.enums import EventType
from g8e.registry import (
    EventNotGovernedError,
    UnknownEventError,
    action_for,
    meta,
    validate_result_envelope,
)


def test_action_for_governed_document_request():
    assert action_for(EventType.APP_CASE_CREATE_REQUESTED) == "DOCUMENT_UPDATE"
    assert action_for(EventType.APP_CASE_DELETE_REQUESTED) == "DOCUMENT_DELETE"
    assert action_for(EventType.OPERATOR_FILESYSTEM_READ_REQUESTED) == "FS_READ"


def test_action_for_rejects_unknown_event():
    with pytest.raises(UnknownEventError):
        action_for("g8e.v1.not.registered")


def test_action_for_rejects_outcome_event():
    with pytest.raises(EventNotGovernedError):
        action_for(EventType.APP_CASE_CREATED)


def test_meta_returns_registry_entry():
    entry = meta(EventType.APP_CASE_CREATE_REQUESTED)
    assert entry["kind"] == "request"
    assert entry["governance"]["action_type"] == "DOCUMENT_UPDATE"


def test_validate_result_envelope_accepts_declared_outcome():
    validate_result_envelope(
        EventType.OPERATOR_COMMAND_REQUESTED,
        EventType.OPERATOR_COMMAND_COMPLETED,
        "EXECUTE_BASH",
    )


def test_validate_result_envelope_rejects_wrong_outcome():
    with pytest.raises(EventNotGovernedError):
        validate_result_envelope(
            EventType.OPERATOR_COMMAND_REQUESTED,
            EventType.OPERATOR_FILESYSTEM_READ_COMPLETED,
            "EXECUTE_BASH",
        )
