# Copyright (c) 2026 Lateralus Labs, LLC.
# Use of this source code is governed by the Business Source License
# included in the LICENSE file.
#
# As of the Change Date listed in the LICENSE file, this software is
# released under the Apache License, Version 2.0.

"""
Shared g8e header fixtures for g8ee unit tests.

TEST_G8E_HEADERS provides a complete, stable set of lowercase X-G8E-* headers
with predictable test values.  Use it wherever tests need to simulate an
inbound request that carries g8e context headers.
"""

from app.constants import EXECUTION_ID

TEST_G8E_HEADERS: dict[str, str] = {
    EXECUTION_ID.lower(): "exec-test-001",
}
