# Copyright (c) 2026 Lateralus Labs, LLC.
# Use of this source code is governed by the Business Source License
# included in the LICENSE file.
#
# As of the Change Date listed in the LICENSE file, this software is
# released under the Apache License, Version 2.0.

import pytest

from app.db import DBClient
from app.models.settings import TLSConfig


@pytest.mark.unit
@pytest.mark.asyncio
class TestDBClientBootstrapAuth:
    async def test_init_uses_operator_session_id(self):
        operator_session_id = "session-456"
        tls_config = TLSConfig(ca_cert_path="/mock/ca.crt")
        client = DBClient(tls_config=tls_config, operator_session_id=operator_session_id)

        assert client._operator_session_id == operator_session_id
