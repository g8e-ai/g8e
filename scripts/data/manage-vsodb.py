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
"""
g8e Data Management CLI

Single entry point for all platform data operations. Dispatches to
individual resource scripts in scripts/data/.

Usage:
    python manage-vsodb.py store stats
    python manage-vsodb.py store operators
    python manage-vsodb.py store doc --collection operators --id <id>
    python manage-vsodb.py store kv --pattern "g8e:session:*"
    python manage-vsodb.py store wipe --dry-run

    python manage-vsodb.py users list
    python manage-vsodb.py users create --email user@example.com --name "John Doe"

    python manage-vsodb.py operators list --email user@example.com
    python manage-vsodb.py operators get --id OPERATOR_ID

    python manage-vsodb.py settings show
    python manage-vsodb.py settings set llm_model=gemma3:4b

    python manage-vsodb.py device-links list --email user@example.com

    python manage-vsodb.py audit --db-path /path/to/g8e.db sessions
    python manage-vsodb.py audit --container operator-test-1 stats
"""

import os
import sys
from pathlib import Path
from typing import List

SCRIPT_DIR = Path(__file__).parent.absolute()
sys.path.insert(0, str(SCRIPT_DIR))

RESOURCES = {
    'store':        'manage-store',
    'users':        'manage-users',
    'operators':    'manage-operators',
    'settings':     'manage-settings',
    'device-links': 'manage-device-links',
    'audit':        'manage-lfaa',
}

HELP_TEXT = """
g8e Data Management CLI

Usage: manage-vsodb.py <resource> <command> [options]

Resources:
  store          VSODB document store & KV queries
  users          Platform user management
  operators      Operator document management
  settings       Platform settings (read/write)
  device-links   Device link token management
  audit          LFAA audit vault queries (SQLite)

Examples:
  manage-vsodb.py store stats
  manage-vsodb.py store operators
  manage-vsodb.py store wipe --dry-run
  manage-vsodb.py users list
  manage-vsodb.py users create --email user@example.com --name "John Doe"
  manage-vsodb.py operators list --email user@example.com
  manage-vsodb.py operators get --id OPERATOR_ID
  manage-vsodb.py settings show --section llm
  manage-vsodb.py settings set llm_model=gemma3:4b
  manage-vsodb.py device-links list --email user@example.com
  manage-vsodb.py audit --db-path /path/to/g8e.db sessions

Run 'manage-vsodb.py <resource> --help' for resource-specific help.
""".strip()


def _import_resource(name: str):
    """Import a resource module by its script name (without .py)."""
    import importlib.util
    spec = importlib.util.spec_from_file_location(name, SCRIPT_DIR / f'{name}.py')
    mod = importlib.util.module_from_spec(spec)
    spec.loader.exec_module(mod)
    return mod


def main() -> int:
    if len(sys.argv) < 2 or sys.argv[1] in ('-h', '--help', ''):
        print(HELP_TEXT)
        return 0 if sys.argv[1:] in (['-h'], ['--help']) else 1

    resource = sys.argv[1]

    if resource not in RESOURCES:
        print(f'[manage-vsodb] Unknown resource: {resource!r}', file=sys.stderr)
        print(f'  Valid: {", ".join(RESOURCES)}', file=sys.stderr)
        return 1

    module_name = RESOURCES[resource]
    try:
        mod = _import_resource(module_name)
    except ImportError as e:
        print(f'[manage-vsodb] Failed to load {module_name}: {e}', file=sys.stderr)
        return 1

    return mod.run(sys.argv[2:])


if __name__ == '__main__':
    sys.exit(main())
