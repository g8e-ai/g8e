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

"""
Contract test: g8ee OperatorType and CloudSubtype enums must exactly match
the canonical values in shared/constants/status.json.

Prevents the class of bug where g8ee defines status constants independently
and a typo or omission silently breaks cross-component compatibility.
"""

import json

import pytest

from app.constants import (
    CloudSubtype,
    OperatorType,
    OperatorHistoryEventType,
    OperatorStatus,
)

pytestmark = pytest.mark.unit


def _load_status_json() -> dict:
    with open("/app/shared/constants/status.json") as f:
        return json.load(f)


class TestOperatorStatusMatchesSharedJSON:
    """OperatorStatus enum values must match shared/constants/status.json g8e.status."""

    def test_all_json_operator_statuses_have_enum_members(self):
        status = _load_status_json()["g8e.status"]
        for key, value in status.items():
            assert value in OperatorStatus._value2member_map_, (
                f"shared/constants/status.json g8e.status.{key}={value!r} "
                f"has no OperatorStatus member"
            )

    def test_no_extra_enum_members_beyond_json(self):
        status = _load_status_json()["g8e.status"]
        json_values = set(status.values())
        for member in OperatorStatus:
            assert member.value in json_values, (
                f"OperatorStatus.{member.name}={member.value!r} "
                f"not in shared/constants/status.json"
            )


class TestOperatorHistoryEventTypeMatchesSharedJSON:
    """OperatorHistoryEventType enum values must match shared/constants/status.json history.event.type."""

    def test_all_json_history_event_types_have_enum_members(self):
        status = _load_status_json()["history.event.type"]
        for key, value in status.items():
            assert value in OperatorHistoryEventType._value2member_map_, (
                f"shared/constants/status.json history.event.type.{key}={value!r} "
                f"has no OperatorHistoryEventType member"
            )

    def test_no_extra_enum_members_beyond_json(self):
        status = _load_status_json()["history.event.type"]
        json_values = set(status.values())
        for member in OperatorHistoryEventType:
            assert member.value in json_values, (
                f"OperatorHistoryEventType.{member.name}={member.value!r} "
                f"not in shared/constants/status.json"
            )


class TestOperatorTypeMatchesSharedJSON:
    """OperatorType enum values must match shared/constants/status.json g8e.type."""

    def test_system_matches(self):
        status = _load_status_json()["g8e.type"]
        assert OperatorType.SYSTEM.value == status["system"]

    def test_cloud_matches(self):
        status = _load_status_json()["g8e.type"]
        assert OperatorType.CLOUD.value == status["cloud"]

    def test_all_json_operator_types_have_enum_members(self):
        status = _load_status_json()["g8e.type"]
        for key, value in status.items():
            assert value in OperatorType._value2member_map_, (
                f"shared/constants/status.json g8e.type.{key}={value!r} "
                f"has no OperatorType member"
            )

    def test_no_extra_enum_members_beyond_json(self):
        status = _load_status_json()["g8e.type"]
        json_values = set(status.values())
        for member in OperatorType:
            assert member.value in json_values, (
                f"OperatorType.{member.name}={member.value!r} "
                f"not in shared/constants/status.json"
            )


class TestCloudSubtypeMatchesSharedJSON:
    """CloudSubtype enum values must match shared/constants/status.json cloud.subtype."""

    def test_aws_matches(self):
        status = _load_status_json()["cloud.subtype"]
        assert CloudSubtype.AWS.value == status["aws"]

    def test_azure_matches(self):
        status = _load_status_json()["cloud.subtype"]
        assert CloudSubtype.AZURE.value == status["azure"]

    def test_gcp_matches(self):
        status = _load_status_json()["cloud.subtype"]
        assert CloudSubtype.GCP.value == status["gcp"]

    def test_g8ep_matches(self):
        status = _load_status_json()["cloud.subtype"]
        assert CloudSubtype.G8E_POD.value == status["g8ep"]

    def test_all_json_cloud_subtypes_have_enum_members(self):
        status = _load_status_json()["cloud.subtype"]
        for key, value in status.items():
            assert value in CloudSubtype._value2member_map_, (
                f"shared/constants/status.json cloud.subtype.{key}={value!r} "
                f"has no CloudSubtype member"
            )

    def test_no_extra_enum_members_beyond_json(self):
        status = _load_status_json()["cloud.subtype"]
        json_values = set(status.values())
        for member in CloudSubtype:
            assert member.value in json_values, (
                f"CloudSubtype.{member.name}={member.value!r} "
                f"not in shared/constants/status.json"
            )
