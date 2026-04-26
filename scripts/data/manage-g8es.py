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
    python manage-g8es.py store stats
    python manage-g8es.py store operators
    python manage-g8es.py store doc --collection operators --id <id>
    python manage-g8es.py store kv --pattern "g8e:session:*"
    python manage-g8es.py store wipe --dry-run

    python manage-g8es.py users list
    python manage-g8es.py users create --email user@example.com --name "John Doe"

    python manage-g8es.py operators list --email user@example.com
    python manage-g8es.py operators get --id OPERATOR_ID

    python manage-g8es.py settings show
    python manage-g8es.py settings set llm_model=gemma3:4b

    python manage-g8es.py device-links list --email user@example.com

    python manage-g8es.py audit --db-path /path/to/g8e.db sessions
    python manage-g8es.py audit --container operator-test-1 stats
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
    'mcp':          'manage-mcp',
    'reputation':   'manage-reputation',
}

HELP_TEXT = """
g8e Data Management CLI

Usage: manage-g8es.py <resource> <command> [options]

Resources:
  store          g8es document store & KV queries
  users          Platform user management
  operators      Operator document management
  settings       Platform settings (read/write)
  device-links   Device link token management
  audit          LFAA audit vault queries (SQLite)
  mcp            MCP client integration (config, test, status)
  reputation     Reputation state & commitment management

Examples:
  manage-g8es.py store stats
  manage-g8es.py store operators
  manage-g8es.py store wipe --dry-run
  manage-g8es.py users list
  manage-g8es.py users create --email user@example.com --name "John Doe"
  manage-g8es.py operators list --email user@example.com
  manage-g8es.py operators get --id OPERATOR_ID
  manage-g8es.py settings show --section llm
  manage-g8es.py settings set llm_model=gemma3:4b
  manage-g8es.py device-links list --email user@example.com
  manage-g8es.py audit --db-path /path/to/g8e.db sessions
  manage-g8es.py mcp config --client claude-code --email user@example.com
  manage-g8es.py mcp test --email user@example.com

Run 'manage-g8es.py <resource> --help' for resource-specific help.
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
        print(f'[manage-g8es] Unknown resource: {resource!r}', file=sys.stderr)
        print(f'  Valid: {", ".join(RESOURCES)}', file=sys.stderr)
        return 1

    module_name = RESOURCES[resource]
    try:
        mod = _import_resource(module_name)
    except ImportError as e:
        print(f'[manage-g8es] Failed to load {module_name}: {e}', file=sys.stderr)
        return 1

    return mod.run(sys.argv[2:])


if __name__ == '__main__':
    sys.exit(main())
