#!/usr/bin/env python3
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

"""Example usage of g8e-protocol constants."""

from g8e.constants import (
    EVENTS,
    STATUS,
    COLLECTIONS,
    ComponentName,
    HTTP_AUTHORIZATION_HEADER,
    WEB_SESSION_ID_HEADER,
    CLI_SESSION_ID_HEADER,
    OPERATOR_ID_HEADER,
    USER_ID_HEADER,
    ORGANIZATION_ID_HEADER,
)


def main():
    print("=== g8e Protocol Constants Example ===\n")

    # Access event constants
    print("Event Constants:")
    print(f"  Command Requested: {EVENTS['events']['OperatorCommandRequested']['value']}")
    print(f"  Command Completed: {EVENTS['events']['OperatorCommandCompleted']['value']}")
    print(f"  Heartbeat: {EVENTS['events']['OperatorHeartbeatRequested']['value']}")
    print()

    # Access status constants
    print("Status Constants:")
    print(f"  Status Categories: {list(STATUS.get('status', {}).keys())[:5]}")
    print()

    # Access collection constants
    print("Collection Constants:")
    print(f"  Operators Collection: {COLLECTIONS['collections']['operators']['value']}")
    print(f"  Users Collection: {COLLECTIONS['collections']['users']['value']}")
    print(f"  Cases Collection: {COLLECTIONS['collections']['cases']['value']}")
    print()

    # Use component enum
    print("Component Names:")
    print(f"  CLIENT: {ComponentName.CLIENT}")
    print(f"  G8EE: {ComponentName.G8EE}")
    print(f"  G8EO: {ComponentName.G8EO}")
    print(f"  OPERATOR (alias): {ComponentName.OPERATOR}")
    print()

    # Build HTTP headers using protocol constants
    print("HTTP Headers Example:")
    headers = {
        HTTP_AUTHORIZATION_HEADER: "Bearer token-abc-123",
        WEB_SESSION_ID_HEADER: "web-session-xyz",
        CLI_SESSION_ID_HEADER: "cli-session-456",
        OPERATOR_ID_HEADER: "operator-789",
        USER_ID_HEADER: "user-123",
        ORGANIZATION_ID_HEADER: "org-abc",
    }
    for key, value in headers.items():
        print(f"  {key}: {value}")
    print()


if __name__ == "__main__":
    main()
