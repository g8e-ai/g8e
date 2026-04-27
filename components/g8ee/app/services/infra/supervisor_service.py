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

import base64
import logging
from typing import Any, List

import httpx

from app.services.infra.settings_service import SettingsService

logger = logging.getLogger(__name__)


class SupervisorService:
    """Service for communicating with Supervisord via XML-RPC.
    
    Used to manage processes in the g8ep container (e.g., the local operator).
    """

    def __init__(self, settings_service: SettingsService):
        self._settings_service = settings_service
        self._timeout = 10.0  # 10 seconds, matching G8E_GATEWAY_OPERATOR_LAUNCH_TIMEOUT_MS

    async def _resolve_settings(self):
        settings = await self._settings_service.get_platform_settings()
        port = settings.get("supervisor_port", "443")
        token = settings.get("internal_auth_token", "")
        
        auth_bytes = f"g8e-internal:{token}".encode("utf-8")
        auth_header = f"Basic {base64.b64encode(auth_bytes).decode('utf-8')}"
        
        return {
            "supervisor_url": f"http://g8ep:{port}/RPC2",
            "auth_header": auth_header,
        }

    async def xmlrpc_call(self, method: str, params: List[Any]) -> str:
        """Performs an XML-RPC call to Supervisor."""
        
        # Build XML-RPC body manually for simplicity and control
        params_xml = ""
        for p in params:
            if isinstance(p, bool):
                val = f"<boolean>{'1' if p else '0'}</boolean>"
            else:
                val = f"<string>{str(p)}</string>"
            params_xml += f"<param><value>{val}</value></param>"

        body = f"""<?xml version="1.0"?>
<methodCall>
  <methodName>{method}</methodName>
  <params>
    {params_xml}
  </params>
</methodCall>"""

        resolved = await self._resolve_settings()
        
        async with httpx.AsyncClient(timeout=self._timeout) as client:
            try:
                response = await client.post(
                    resolved["supervisor_url"],
                    content=body,
                    headers={
                        "Content-Type": "text/xml",
                        "Authorization": resolved["auth_header"],
                    }
                )
                
                if response.status_code != 200:
                    logger.error(
                        f"[SUPERVISOR] Connection failed: {response.status_code} {response.text}",
                        extra={"method": method}
                    )
                    raise Exception(f"Supervisor connection failed: {response.status_code}")

                xml = response.text
                if "<fault>" in xml:
                    # Very basic fault parsing
                    import re
                    code_match = re.search(r"<int>([^<]+)</int>", xml)
                    string_match = re.search(r"<string>([^<]+)</string>", xml)
                    
                    fault_code = int(code_match.group(1)) if code_match else None
                    fault_string = string_match.group(1) if string_match else "Unknown Supervisor fault"
                    
                    # Map common Supervisor fault codes
                    if fault_code == 10:  # BAD_NAME
                        raise Exception("Operator process not found in g8ep configuration.")
                    elif fault_code == 60:  # ALREADY_STARTED
                        raise Exception("ALREADY_STARTED")
                    elif fault_code == 70:  # NOT_RUNNING
                        return xml  # Ignore if trying to stop
                    elif fault_code == 90:  # SPAWN_ERROR
                        raise Exception("Failed to spawn operator process. Check g8ep logs.")
                    else:
                        raise Exception(f"Supervisor error ({fault_code}): {fault_string}")

                return xml

            except httpx.RequestError as exc:
                logger.error(f"[SUPERVISOR] Request error: {exc}", extra={"method": method})
                raise Exception(f"Failed to communicate with Supervisor: {str(exc)}")

    async def start_process(self, name: str, wait: bool = False) -> bool:
        """Starts a supervised process."""
        try:
            await self.xmlrpc_call("supervisor.startProcess", [name, wait])
            return True
        except Exception as e:
            if "ALREADY_STARTED" in str(e):
                logger.info(f"[SUPERVISOR] Process {name} already running, restarting")
                await self.stop_process(name, wait=True)
                await self.xmlrpc_call("supervisor.startProcess", [name, wait])
                return True
            if "NOT_RUNNING" in str(e):
                logger.info(f"[SUPERVISOR] Process {name} not running (may be FATAL), attempting start")
                await self.xmlrpc_call("supervisor.startProcess", [name, wait])
                return True
            logger.error(f"[SUPERVISOR] Failed to start process {name}: {str(e)}")
            raise

    async def stop_process(self, name: str, wait: bool = True) -> bool:
        """Stops a supervised process."""
        try:
            await self.xmlrpc_call("supervisor.stopProcess", [name, wait])
            return True
        except Exception as e:
            logger.warning(f"[SUPERVISOR] Failed to stop process {name} (non-fatal): {str(e)}")
            return False
