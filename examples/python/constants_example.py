#!/usr/bin/env python3
# Copyright (c) 2026 Lateralus Labs, LLC.
# Use of this source code is governed by the Business Source License
# included in the LICENSE file.
#
# As of the Change Date listed in the LICENSE file, this software is
# released under the Apache License, Version 2.0.

"""Example usage of g8e constants."""

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
    print(f"  G8EO: {ComponentName.G8EO}")
    print(f"  G8EO_GATEWAY: {ComponentName.G8EO_GATEWAY}")
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
