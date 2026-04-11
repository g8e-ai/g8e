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
Shared VSO header fixtures for VSE unit tests.

TEST_VSO_HEADERS provides a complete, stable set of lowercase X-VSO-* headers
with predictable test values.  Use it wherever tests need to simulate an
inbound request that carries VSO context headers.
"""

from app.constants import INTERNAL_AUTH_HEADER, VSOHeaders

TEST_VSO_HEADERS: dict[str, str] = {
    VSOHeaders.WEB_SESSION_ID.lower():       "session-test-abc123",
    VSOHeaders.USER_ID.lower():          "user-test-001",
    VSOHeaders.ORGANIZATION_ID.lower():  "org-test-001",
    VSOHeaders.CASE_ID.lower():          "case-test-001",
    VSOHeaders.INVESTIGATION_ID.lower(): "inv-test-001",
    VSOHeaders.SOURCE_COMPONENT.lower(): "vsod",
    VSOHeaders.EXECUTION_ID.lower():     "exec-test-001",
    VSOHeaders.BOUND_OPERATORS.lower():  "[]",
    INTERNAL_AUTH_HEADER:                "test-internal-token",
}
