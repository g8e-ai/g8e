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
Contract test enforcing that all command request payload types are present
in the G8eOutboundPayload discriminated union.

This prevents regressions like the one in April 2026 where CheckPortRequestPayload
was missing from the outbound union, forcing port-check to go through MCP wrapping
instead of using native event types.
"""

from typing import get_args

import pytest

from app.models.command_request_payloads import (
    CommandRequestPayload,
    CommandCancelRequestPayload,
    FileEditRequestPayload,
    FsListRequestPayload,
    FsReadRequestPayload,
    FetchLogsRequestPayload,
    FetchHistoryRequestPayload,
    FetchFileHistoryRequestPayload,
    FetchFileDiffRequestPayload,
    CheckPortRequestPayload,
    RestoreFileRequestPayload,
    DirectCommandAuditRequestPayload,
)
from app.models.pubsub_messages import G8eOutboundPayload


class TestOutboundPayloadUnionContract:
    """Verify all command request payloads are present in G8eOutboundPayload union."""

    def test_all_command_request_payloads_in_outbound_union(self):
        """Every command request payload type must be present in G8eOutboundPayload.

        This contract test prevents regressions where a new payload type is added
        to command_request_payloads.py but forgotten in the G8eOutboundPayload union,
        which would force that operation to go through MCP wrapping instead of using
        native event types.

        Regression history: April 2026 - CheckPortRequestPayload was missing from
        G8eOutboundPayload, causing port-check to incorrectly route through MCP.
        """
        # All command request payload types that should be in the outbound union
        command_payloads = {
            CommandRequestPayload,
            CommandCancelRequestPayload,
            FileEditRequestPayload,
            FsListRequestPayload,
            FsReadRequestPayload,
            FetchLogsRequestPayload,
            FetchHistoryRequestPayload,
            FetchFileHistoryRequestPayload,
            FetchFileDiffRequestPayload,
            CheckPortRequestPayload,
            RestoreFileRequestPayload,
            DirectCommandAuditRequestPayload,
        }

        # Get all types in the G8eOutboundPayload Union
        outbound_union_types = set(get_args(G8eOutboundPayload))

        # Check that every command payload is present
        missing_payloads = []
        for payload_type in command_payloads:
            if payload_type not in outbound_union_types:
                missing_payloads.append(payload_type.__name__)

        assert not missing_payloads, (
            f"The following command request payload types are missing from G8eOutboundPayload: {missing_payloads}. "
            "Add them to the Union in app/models/pubsub_messages.py to ensure they can be dispatched "
            "using native g8e event types instead of being forced through MCP wrapping."
        )

        # Also verify there are no extra types in the union that shouldn't be there
        extra_types = outbound_union_types - command_payloads
        assert not extra_types, (
            f"G8eOutboundPayload contains unexpected types: {[t.__name__ for t in extra_types]}. "
            "If these are legitimate command payloads, add them to the command_payloads set above. "
            "If they are legacy MCP types (e.g., JSONRPCRequest), remove them from the union in pubsub_messages.py "
            "since MCP is now a gateway-only concern and should not be in the operator command channel union."
        )
