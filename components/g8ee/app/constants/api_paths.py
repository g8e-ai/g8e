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
    """Internal API paths shared across g8ee and g8ed."""
    PREFIX: str = API_PATHS["internal_prefix"]
    
    # g8ee Endpoints (relative to PREFIX)
    G8EE_CHAT: str = PREFIX + API_PATHS["g8ee"]["chat"]
    G8EE_CHAT_STOP: str = PREFIX + API_PATHS["g8ee"]["chat_stop"]
    G8EE_INVESTIGATIONS: str = PREFIX + API_PATHS["g8ee"]["investigations"]
    G8EE_INVESTIGATION: str = PREFIX + API_PATHS["g8ee"]["investigation"]
    G8EE_CASES: str = PREFIX + API_PATHS["g8ee"]["cases"]
    G8EE_CASE: str = PREFIX + API_PATHS["g8ee"]["case"]
    G8EE_OPERATORS_STOP: str = PREFIX + API_PATHS["g8ee"]["operators_stop"]
    G8EE_OPERATORS_REGISTER_SESSION: str = PREFIX + API_PATHS["g8ee"]["operators_register_session"]
    G8EE_OPERATORS_DEREGISTER_SESSION: str = PREFIX + API_PATHS["g8ee"]["operators_deregister_session"]
    G8EE_OPERATOR_DIRECT_COMMAND: str = PREFIX + API_PATHS["g8ee"]["operator_direct_command"]
    G8EE_OPERATOR_APPROVAL_RESPOND: str = PREFIX + API_PATHS["g8ee"]["operator_approval_respond"]
    G8EE_OPERATOR_APPROVAL_PENDING: str = PREFIX + API_PATHS["g8ee"]["operator_approval_pending"]
    G8EE_SETTINGS_USER: str = PREFIX + API_PATHS["g8ee"]["settings_user"]
    
    # g8ed Endpoints (relative to PREFIX)
    G8ED_SSE_PUSH: str = "/sse/push"
    G8ED_GRANT_INTENT: str = "/operators/{operator_id}/grant-intent"
    G8ED_REVOKE_INTENT: str = "/operators/{operator_id}/revoke-intent"
    G8ED_HEALTH: str = "/health"
