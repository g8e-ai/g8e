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
import ast
from pathlib import Path

g8e_root = Path(__file__).parent.parent.parent
agents_json_path = g8e_root / "shared" / "constants" / "agents.json"
personas_dir = g8e_root / "components" / "g8ee" / "app" / "models" / "personas"

def extract_persona_from_file(file_path: Path) -> dict:
    """Extract persona data from a Python persona file without importing."""
    content = file_path.read_text()
    
    # Find the class definition
    tree = ast.parse(content)
    
    for node in ast.walk(tree):
        if isinstance(node, ast.ClassDef):
            # Find __init__ method
            for item in node.body:
                if isinstance(item, ast.FunctionDef) and item.name == '__init__':
                    # Extract the super().__init__ call
                    for stmt in ast.walk(item):
                        if isinstance(stmt, ast.Call):
                            # Look for super().__init__(...) call
                            if isinstance(stmt.func, ast.Attribute) and stmt.func.attr == '__init__':
                                # Extract keyword arguments
                                persona_data = {}
                                for kw in stmt.keywords:
                                    if kw.arg in ['id', 'display_name', 'icon', 'description', 'role', 'model_tier', 'purpose', 'autonomy']:
                                        if isinstance(kw.value, ast.Constant):
                                            persona_data[kw.arg] = kw.value.value
                                    elif kw.arg == 'tools':
                                        if isinstance(kw.value, ast.List):
                                            tools = []
                                            for elt in kw.value.elts:
                                                if isinstance(elt, ast.Constant):
                                                    tools.append(elt.value)
                                            persona_data['tools'] = tools
                                    elif kw.arg == 'identity':
                                        # Identity is a method call - skip for now, will be reconstructed
                                        persona_data['identity'] = None
                                return persona_data
    return {}

def reconstruct_identity(persona_id: str, file_path: Path) -> str:
    """Reconstruct identity field from the persona file by finding the _get_identity method."""
    content = file_path.read_text()
    
    # Find _get_identity method and extract its return string
    tree = ast.parse(content)
    
    for node in ast.walk(tree):
        if isinstance(node, ast.FunctionDef) and node.name == '_get_identity':
            # Find the return statement
            for stmt in ast.walk(node):
                if isinstance(stmt, ast.Return) and stmt.value:
                    if isinstance(stmt.value, ast.JoinedStr):  # f-string
                        parts = []
                        for value in stmt.value.values:
                            if isinstance(value, ast.Constant):
                                parts.append(value.value)
                            elif isinstance(value, ast.FormattedValue):
                                # Handle method calls in f-strings - this is complex
                                # For now, return a placeholder
                                pass
                        return ''.join(parts)
                    elif isinstance(stmt.value, ast.Constant):
                        return stmt.value.value
    
    # If we can't parse it, read from the existing agents.json
    with open(agents_json_path) as f:
        data = json.load(f)
        return data.get("agent.metadata", {}).get(persona_id, {}).get("identity", "")

def sync_personas():
    with open(agents_json_path, "r") as f:
        data = json.load(f)
    
    # Get list of persona files
    persona_files = {
        'triage': personas_dir / 'triage.py',
        'sage': personas_dir / 'sage.py',
        'dash': personas_dir / 'dash.py',
        'auditor': personas_dir / 'auditor.py',
        'axiom': personas_dir / 'axiom.py',
        'concord': personas_dir / 'concord.py',
        'variance': personas_dir / 'variance.py',
        'pragma': personas_dir / 'pragma.py',
        'nemesis': personas_dir / 'nemesis.py',
        'scribe': personas_dir / 'scribe.py',
        'codex': personas_dir / 'codex.py',
        'judge': personas_dir / 'judge.py',
        'tribunal': personas_dir / 'tribunal.py',
        'warden': personas_dir / 'warden.py',
    }
    
    # Also add warden sub-agents
    persona_files['warden_command_risk'] = personas_dir / 'warden.py'
    persona_files['warden_error'] = personas_dir / 'warden.py'
    persona_files['warden_file_risk'] = personas_dir / 'warden.py'
    
    new_metadata = {}
    existing_metadata = data.get("agent.metadata", {})
    
    for agent_id, file_path in persona_files.items():
        if not file_path.exists():
            print(f"Warning: {file_path} does not exist, skipping {agent_id}")
            continue
        
        # Extract basic persona data
        persona_data = extract_persona_from_file(file_path)
        
        if not persona_data:
            print(f"Warning: Could not extract data from {file_path}, skipping {agent_id}")
            continue
        
        # For warden sub-agents, they're all in warden.py - handle specially
        if agent_id.startswith('warden_'):
            # Map to the correct class in warden.py
            class_map = {
                'warden_command_risk': 'WardenCommandRiskPersona',
                'warden_error': 'WardenErrorPersona',
                'warden_file_risk': 'WardenFileRiskPersona',
            }
            # Use existing metadata for these since they're complex
            if agent_id in existing_metadata:
                new_metadata[agent_id] = existing_metadata[agent_id]
            continue
        
        # Reconstruct identity from existing agents.json since parsing is complex
        if agent_id in existing_metadata:
            persona_data['identity'] = existing_metadata[agent_id].get('identity', '')
            persona_data['purpose'] = existing_metadata[agent_id].get('purpose', persona_data.get('purpose', ''))
            persona_data['autonomy'] = existing_metadata[agent_id].get('autonomy', persona_data.get('autonomy', ''))
        
        # Add output_contract if it exists in existing data
        if agent_id in existing_metadata and 'output_contract' in existing_metadata[agent_id]:
            persona_data['output_contract'] = existing_metadata[agent_id]['output_contract']
        
        new_metadata[agent_id] = persona_data
    
    # Preserve any metadata that wasn't in our persona_files dict
    for agent_id, metadata in existing_metadata.items():
        if agent_id not in new_metadata:
            new_metadata[agent_id] = metadata
    
    data["agent.metadata"] = new_metadata
    data["_generated_warning"] = "DO NOT EDIT THIS FILE MANUALLY. This file is generated by scripts/data/sync-personas.py from the Python persona models in components/g8ee/app/models/personas/. Edit the Python models instead, then run the sync script."
    
    with open(agents_json_path, "w") as f:
        json.dump(data, f, indent=2)
        f.write("\n")
    
    print(f"Successfully synced {len(new_metadata)} personas to {agents_json_path}")

if __name__ == "__main__":
    sync_personas()
