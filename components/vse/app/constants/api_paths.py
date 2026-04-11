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

import json
from app.constants.paths import PATHS

_SHARED_DIR = PATHS["infra"]["shared_constants_dir"]

def _load(filename: str) -> dict:
    path = _SHARED_DIR + "/" + filename
    try:
        with open(path) as f:
            return json.load(f)
    except FileNotFoundError as e:
        raise RuntimeError(f"Shared constants file not found: {path}") from e
    except json.JSONDecodeError as e:
        raise RuntimeError(f"Invalid JSON in shared constants file {path}: {e}") from e

API_PATHS = _load("api_paths.json")

class InternalApiPaths:
    """Internal API paths shared across VSE and VSOD."""
    PREFIX: str = API_PATHS["internal_prefix"]
    
    # VSE Endpoints (relative to PREFIX)
    VSE_CHAT: str = PREFIX + API_PATHS["vse"]["chat"]
    VSE_CHAT_STOP: str = PREFIX + API_PATHS["vse"]["chat_stop"]
    VSE_INVESTIGATIONS: str = PREFIX + API_PATHS["vse"]["investigations"]
    VSE_INVESTIGATION: str = PREFIX + API_PATHS["vse"]["investigation"]
    VSE_CASES: str = PREFIX + API_PATHS["vse"]["cases"]
    VSE_CASE: str = PREFIX + API_PATHS["vse"]["case"]
    VSE_OPERATORS_STOP: str = PREFIX + API_PATHS["vse"]["operators_stop"]
    VSE_OPERATORS_REGISTER_SESSION: str = PREFIX + API_PATHS["vse"]["operators_register_session"]
    VSE_OPERATORS_DEREGISTER_SESSION: str = PREFIX + API_PATHS["vse"]["operators_deregister_session"]
    VSE_OPERATOR_DIRECT_COMMAND: str = PREFIX + API_PATHS["vse"]["operator_direct_command"]
    VSE_OPERATOR_APPROVAL_RESPOND: str = PREFIX + API_PATHS["vse"]["operator_approval_respond"]
    VSE_OPERATOR_APPROVAL_PENDING: str = PREFIX + API_PATHS["vse"]["operator_approval_pending"]
    VSE_HEALTH: str = PREFIX + API_PATHS["vse"]["health"]
    VSE_SETTINGS_USER: str = PREFIX + API_PATHS["vse"]["settings_user"]
    VSE_MCP_TOOLS_LIST: str = PREFIX + API_PATHS["vse"]["mcp_tools_list"]
    VSE_MCP_TOOLS_CALL: str = PREFIX + API_PATHS["vse"]["mcp_tools_call"]
    
    # VSOD Endpoints (relative to PREFIX)
    VSOD_SSE_PUSH: str = "/sse/push"
    VSOD_GRANT_INTENT: str = "/operators/{operator_id}/grant-intent"
    VSOD_REVOKE_INTENT: str = "/operators/{operator_id}/revoke-intent"
    VSOD_HEALTH: str = "/health"
