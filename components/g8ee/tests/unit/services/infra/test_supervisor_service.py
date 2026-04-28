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

import pytest
from unittest.mock import AsyncMock, MagicMock, patch
import httpx

from app.services.infra.supervisor_service import SupervisorService

pytestmark = [pytest.mark.unit, pytest.mark.asyncio(loop_scope="session")]

class TestSupervisorService:
    @pytest.fixture
    def mock_settings_service(self):
        settings_service = MagicMock()
        settings_service.get_platform_settings = AsyncMock(return_value={
            "supervisor_port": "443",
            "internal_auth_token": "test-token"
        })
        return settings_service

    @pytest.fixture
    def supervisor_service(self, mock_settings_service):
        return SupervisorService(mock_settings_service)

    async def test_start_process_success(self, supervisor_service):
        with patch("httpx.AsyncClient.post") as mock_post:
            mock_post.return_value = MagicMock(status_code=200, text="<methodResponse><params><param><value><boolean>1</boolean></value></param></params></methodResponse>")
            
            result = await supervisor_service.start_process("operator")
            
            assert result is True
            assert mock_post.call_count == 1
            args, kwargs = mock_post.call_args
            assert "supervisor.startProcess" in kwargs["content"]
            assert "operator" in kwargs["content"]

    async def test_start_process_already_started(self, supervisor_service):
        fault_xml = """<?xml version="1.0"?>
<methodResponse>
  <fault>
    <value>
      <struct>
        <member>
          <name>faultCode</name>
          <value><int>60</int></value>
        </member>
        <member>
          <name>faultString</name>
          <value><string>ALREADY_STARTED</string></value>
        </member>
      </struct>
    </value>
  </fault>
</methodResponse>"""
        
        with patch("httpx.AsyncClient.post") as mock_post:
            mock_post.side_effect = [
                MagicMock(status_code=200, text=fault_xml), # First start fails
                MagicMock(status_code=200, text="OK"),       # Stop succeeds
                MagicMock(status_code=200, text="OK"),       # Second start succeeds
            ]
            
            result = await supervisor_service.start_process("operator")
            
            assert result is True
            assert mock_post.call_count == 3

    async def test_stop_process_success(self, supervisor_service):
        with patch("httpx.AsyncClient.post") as mock_post:
            mock_post.return_value = MagicMock(status_code=200, text="OK")
            
            result = await supervisor_service.stop_process("operator")
            
            assert result is True
            assert mock_post.call_count == 1
            args, kwargs = mock_post.call_args
            assert "supervisor.stopProcess" in kwargs["content"]

    async def test_xmlrpc_call_fault(self, supervisor_service):
        fault_xml = """<?xml version="1.0"?>
<methodResponse>
  <fault>
    <value>
      <struct>
        <member>
          <name>faultCode</name>
          <value><int>10</int></value>
        </member>
        <member>
          <name>faultString</name>
          <value><string>BAD_NAME</string></value>
        </member>
      </struct>
    </value>
  </fault>
</methodResponse>"""
        
        with patch("httpx.AsyncClient.post") as mock_post:
            mock_post.return_value = MagicMock(status_code=200, text=fault_xml)
            
            with pytest.raises(Exception, match="Operator process not found"):
                await supervisor_service.xmlrpc_call("supervisor.startProcess", ["wrong-name"])
