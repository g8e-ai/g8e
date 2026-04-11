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
Shared g8e header fixtures for g8ee unit tests.

TEST_G8E_HEADERS provides a complete, stable set of lowercase X-G8E-* headers
with predictable test values.  Use it wherever tests need to simulate an
inbound request that carries g8e context headers.
"""

from app.constants import INTERNAL_AUTH_HEADER, G8eHeaders

TEST_G8E_HEADERS: dict[str, str] = {
    G8eHeaders.WEB_SESSION_ID.lower():       "session-test-abc123",
    G8eHeaders.USER_ID.lower():          "user-test-001",
    G8eHeaders.ORGANIZATION_ID.lower():  "org-test-001",
    G8eHeaders.CASE_ID.lower():          "case-test-001",
    G8eHeaders.INVESTIGATION_ID.lower(): "inv-test-001",
    G8eHeaders.SOURCE_COMPONENT.lower(): "g8ed",
    G8eHeaders.EXECUTION_ID.lower():     "exec-test-001",
    G8eHeaders.BOUND_OPERATORS.lower():  "[]",
    INTERNAL_AUTH_HEADER:                "test-internal-token",
}
