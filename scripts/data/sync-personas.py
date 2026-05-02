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

def extract_persona_from_file(file_path: Path, class_name: str) -> dict:
    """Extract persona data from a specific class in a Python persona file without importing."""
    content = file_path.read_text()
    
    # Find the class definition
    tree = ast.parse(content)
    
    for node in ast.walk(tree):
        if isinstance(node, ast.ClassDef) and node.name == class_name:
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

def extract_identity_from_class(file_path: Path, class_name: str) -> str:
    """Extract identity field from a specific class in a persona file."""
    content = file_path.read_text()
    
    # Find the class and its _get_identity method
    tree = ast.parse(content)
    
    for node in ast.walk(tree):
        if isinstance(node, ast.ClassDef) and node.name == class_name:
            # Find _get_identity method
            for item in node.body:
                if isinstance(item, ast.FunctionDef) and item.name == '_get_identity':
                    # Find the return statement
                    for stmt in ast.walk(item):
                        if isinstance(stmt, ast.Return) and stmt.value:
                            if isinstance(stmt.value, ast.Constant):
                                return stmt.value.value
                            elif isinstance(stmt.value, ast.JoinedStr):
                                # Handle f-strings by just taking the constant parts for now
                                # This is a fallback if method calls are present
                                parts = []
                                for value in stmt.value.values:
                                    if isinstance(value, ast.Constant):
                                        parts.append(value.value)
                                    elif isinstance(value, ast.FormattedValue):
                                        # If it's a method call like {self.format_xml_tag(...)}
                                        # we can't easily resolve it here without execution.
                                        # For now, just placeholder or skip.
                                        parts.append(f"{{...}}")
                                return ''.join(parts)
    return ""

def sync_personas():
    with open(agents_json_path, "r") as f:
        data = json.load(f)
    
    # Get list of persona files
    persona_files = {
        'triage': (personas_dir / 'triage.py', 'TriagePersona'),
        'sage': (personas_dir / 'sage.py', 'SagePersona'),
        'dash': (personas_dir / 'dash.py', 'DashPersona'),
        'auditor': (personas_dir / 'auditor.py', 'AuditorPersona'),
        'axiom': (personas_dir / 'axiom.py', 'AxiomPersona'),
        'concord': (personas_dir / 'concord.py', 'ConcordPersona'),
        'variance': (personas_dir / 'variance.py', 'VariancePersona'),
        'pragma': (personas_dir / 'pragma.py', 'PragmaPersona'),
        'nemesis': (personas_dir / 'nemesis.py', 'NemesisPersona'),
        'scribe': (personas_dir / 'scribe.py', 'ScribePersona'),
        'codex': (personas_dir / 'codex.py', 'CodexPersona'),
        'judge': (personas_dir / 'judge.py', 'JudgePersona'),
        'tribunal': (personas_dir / 'tribunal.py', 'TribunalPersona'),
        'warden': (personas_dir / 'warden.py', 'WardenPersona'),
        'warden_command_risk': (personas_dir / 'warden.py', 'WardenCommandRiskPersona'),
        'warden_error': (personas_dir / 'warden.py', 'WardenErrorPersona'),
        'warden_file_risk': (personas_dir / 'warden.py', 'WardenFileRiskPersona'),
    }
    
    new_metadata = {}
    existing_metadata = data.get("agent.metadata", {})
    
    for agent_id, (file_path, class_name) in persona_files.items():
        if not file_path.exists():
            print(f"Warning: {file_path} does not exist, skipping {agent_id}")
            continue
        
        # Extract basic persona data
        persona_data = extract_persona_from_file(file_path, class_name)
        
        if not persona_data:
            # Fallback to existing if we can't extract (e.g. documentation-only)
            if agent_id in existing_metadata:
                persona_data = existing_metadata[agent_id].copy()
            else:
                print(f"Warning: Could not extract data from {file_path}, skipping {agent_id}")
                continue
        
        # Extract identity from the specific class
        identity = extract_identity_from_class(file_path, class_name)
        
        if identity:
            # If we found a string, use it. 
            # If it has placeholders {...}, it's an f-string we couldn't fully resolve.
            if "{...}" in identity and agent_id in existing_metadata:
                # Keep existing identity if f-string resolution failed
                # This preserves the manual updates I made to agents.json earlier
                persona_data['identity'] = existing_metadata[agent_id].get('identity', '')
            else:
                persona_data['identity'] = identity
        elif agent_id in existing_metadata:
            persona_data['identity'] = existing_metadata[agent_id].get('identity', '')

        # Preserve other fields from existing metadata if not in extracted data
        if agent_id in existing_metadata:
            for key, value in existing_metadata[agent_id].items():
                if key not in persona_data or persona_data[key] is None:
                    persona_data[key] = value
        
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
