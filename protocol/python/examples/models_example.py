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

"""Example usage of g8e models."""

from g8e.constants import ComponentName
from g8e.models import RequestContext, BoundOperator, PlatformSettings


def main():
    print("=== g8e Protocol Models Example ===\n")

    # Create a request context
    print("Request Context:")
    context = RequestContext(
        web_session_id="web-session-abc-123",
        user_id="user-xyz-456",
        source_component=ComponentName.CLIENT,
        case_id="case-789",
        investigation_id="investigation-123",
        task_id="task-456",
        organization_id="org-abc",
        bound_operators=[
            BoundOperator(
                operator_id="operator-1",
                operator_session_id="session-1",
                status="active",
            ),
            BoundOperator(
                operator_id="operator-2",
                operator_session_id="session-2",
                status="active",
            ),
        ],
    )
    print(f"  Web Session ID: {context.web_session_id}")
    print(f"  User ID: {context.user_id}")
    print(f"  Source Component: {context.source_component}")
    print(f"  Case ID: {context.case_id}")
    print(f"  Bound Operators: {len(context.bound_operators)}")
    for op in context.bound_operators:
        print(f"    - {op.operator_id} (status: {op.status})")
    print()

    # Serialize to dict
    print("Serialized Context:")
    context_dict = context.model_dump(mode="json")
    print(f"  Keys: {list(context_dict.keys())}")
    print()

    # Create platform settings
    print("Platform Settings:")
    settings = PlatformSettings(
        governance_enabled=True,
        l1_doctrine_enabled=True,
        l2_consensus_enabled=True,
        l3_notary_enabled=True,
        audit_enabled=True,
        sentinel_enabled=True,
    )
    print(f"  Governance Enabled: {settings.governance_enabled}")
    print(f"  L1 Doctrine Enabled: {settings.l1_doctrine_enabled}")
    print(f"  L2 Consensus Enabled: {settings.l2_consensus_enabled}")
    print(f"  L3 Notary Enabled: {settings.l3_notary_enabled}")
    print(f"  Audit Enabled: {settings.audit_enabled}")
    print(f"  Sentinel Enabled: {settings.sentinel_enabled}")
    print()

    # Validate model
    # RequestContext enforces session identity for CLIENT source: it must have
    # either web_session_id or cli_session_id (not both) and a user_id. An empty
    # string is treated as missing, so this raises "Context must have either
    # web_session_id or cli_session_id for CLIENT source".
    print("Model Validation:")
    try:
        invalid_context = RequestContext(
            web_session_id="",  # Empty string is falsy, treated as missing
            user_id="user-123",
            source_component=ComponentName.CLIENT,
        )
        print("  ERROR: Should have failed validation with missing session id")
    except ValueError as e:
        print(f"  Validation error caught: {e}")
    print()


if __name__ == "__main__":
    main()
