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

import json
import sys
import os
from pathlib import Path
from unittest.mock import MagicMock

# Add components/g8ee to sys.path to import the models
g8e_root = Path(__file__).parent.parent.parent
sys.path.append(str(g8e_root / "components" / "g8ee"))

# Load agents.json directly to provide values for enums
agents_json_path = g8e_root / "shared" / "constants" / "agents.json"
with open(agents_json_path, "r") as f:
    agents_data = json.load(f)

# Mock the shared constants directory for when running outside a container
mock_shared = MagicMock()
mock_shared._AGENTS = agents_data
sys.modules["app.constants.shared"] = mock_shared
sys.modules["app.constants.api_paths"] = MagicMock()
sys.modules["app.constants.paths"] = MagicMock()

from app.models.personas import PERSONA_REGISTRY

def sync_personas():
    with open(agents_json_path, "r") as f:
        data = json.load(f)
    
    # Update agent.metadata with values from the models
    new_metadata = {}
    for agent_id, persona in PERSONA_REGISTRY.items():
        persona_dict = persona.model_dump(by_alias=True)
        # Remove None values
        persona_dict = {k: v for k, v in persona_dict.items() if v is not None}
        new_metadata[agent_id] = persona_dict
    
    data["agent.metadata"] = new_metadata
    
    with open(agents_json_path, "w") as f:
        json.dump(data, f, indent=2)
        f.write("\n")
    
    print(f"Successfully synced {len(new_metadata)} personas from models to {agents_json_path}")

if __name__ == "__main__":
    sync_personas()
