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
Platform Settings Script

Read and write effective platform settings via the Operator (g8eo) HTTP API.
Secret values (API keys, tokens) are never returned by the API.

Usage:
    python manage-operator.py settings show
    python manage-operator.py settings show --section llm
    python manage-operator.py settings show --section general
    python manage-operator.py settings get llm_provider
    python manage-operator.py settings get llm_model
    python manage-operator.py settings set llm_provider=openai llm_model=gemma3:4b
    python manage-operator.py settings set llm_endpoint=https://10.0.0.1:11434/v1
    python manage-operator.py settings rotate-token
"""

from __future__ import annotations

import argparse
import json
import secrets
import sys
from datetime import datetime, timezone
from pathlib import Path
from typing import Dict, Any, List

from _lib import (
    OPERATOR_BASE_URL,
    get_document,
    print_banner,
    operator_request,
)

SETTINGS_API = f'{OPERATOR_BASE_URL}/api/settings'
OPERATOR_SETTINGS_COLLECTION = 'settings'
PLATFORM_SETTINGS_ID = 'platform_settings'
USER_SETTINGS_ID_PREFIX = 'user_settings_'


def _api_get(user_id: str | None = None) -> Dict[str, Any]:
    # Query platform settings directly from operator
    doc = get_document('platform_settings', 'platform_settings')
    if not doc:
        return {'success': True, 'data': {'settings': {}}}
    
    # Transform flat settings into structured format
    settings = doc.get('settings', {})
    structured: Dict[str, Any] = {}
    for key, value in settings.items():
        structured[key] = {
            'value': value,
            'section': _infer_section(key),
            'label': key.replace('_', ' ').title()
        }
    return {'success': True, 'data': {'settings': structured}}


def _infer_section(key: str) -> str:
    """Infer settings section from key name."""
    if key.startswith('llm_') or key in ['temperature', 'max_tokens', 'provider', 'model', 'assistant_model']:
        return 'llm'
    if key.startswith('vertex_') or key.startswith('google_'):
        return 'search'
    if key in ['session_encryption_key', 'passkey_rp_id', 'passkey_origin']:
        return 'security'
    return 'general'


def _api_put(settings: Dict[str, str], user_id: str | None = None) -> Dict[str, Any]:
    """Write settings directly to operator platform_settings document."""
    if user_id:
        # User-specific settings not yet supported in g8eo
        raise RuntimeError('User-specific settings not yet supported')
    
    doc = get_document('platform_settings', 'platform_settings') or {}
    doc_settings = doc.get('settings', {})
    doc_settings.update(settings)
    doc['settings'] = doc_settings
    doc['id'] = 'platform_settings'
    
    operator_request('PUT', '/db/platform_settings/platform_settings', doc)
    return {'success': True, 'saved': list(settings.keys()), 'skipped': []}


_SECTION_ORDER = ['general', 'llm', 'search', 'security', 'validation']

_SECTION_LABELS = {
    'general':  'General',
    'llm':      'LLM Provider',
    'search':   'Web Search',
    'security': 'Security',
    'validation': 'Validation',
}

_KEY_ORDER = [
    'app_url',
    'passkey_rp_id',
    'passkey_origin',
    'session_encryption_key',
    'g8e_operator_endpoint',

    'g8e_operator_pubsub_url',
    'llm_provider',
    'llm_endpoint',
    'llm_model',
    'llm_assistant_model',
    'llm_max_tokens',
    'provider',
    'model',
    'assistant_model',
    'openai_endpoint',
    'openai_api_key',
    'ollama_endpoint',
    'ollama_api_key',
    'gemini_api_key',
    'anthropic_endpoint',
    'anthropic_api_key',
    'temperature',
    'max_tokens',
    'command_gen_enabled',
    'command_gen_verifier',
    'command_gen_passes',
    'command_gen_temp',
    'vertex_search_enabled',
    'vertex_search_project_id',
    'vertex_search_engine_id',
    'vertex_search_location',
    'vertex_search_api_key',
    'google_search_enabled',
    'google_search_api_key',
    'google_search_engine_id',
]


def _print_settings(settings: Dict[str, Any], section_filter: str | None) -> None:
    by_section: Dict[str, list] = {}
    for key, meta in settings.items():
        sec = meta.get('section', 'general')
        by_section.setdefault(sec, []).append((key, meta))

    sections = _SECTION_ORDER + [s for s in by_section if s not in _SECTION_ORDER]

    printed_any = False
    for sec in sections:
        if sec not in by_section:
            continue
        if section_filter and sec != section_filter:
            continue

        label = _SECTION_LABELS.get(sec, sec.title())
        print(f"\n{'━' * 52}")
        print(f"  {label}")
        print(f"{'━' * 52}")

        entries = by_section[sec]
        key_index = {k: i for i, k in enumerate(_KEY_ORDER)}
        entries.sort(key=lambda x: (key_index.get(x[0], 999), x[0]))

        for key, meta in entries:
            value = meta.get('value') or ''
            env_locked = meta.get('envLocked', False)
            label_str = meta.get('label', key)
            lock_marker = ' [env]' if env_locked else ''
            display = value if value else '(default)'
            print(f"  {label_str:<35} {display}{lock_marker}")

        printed_any = True

    if not printed_any and section_filter:
        print(f"\n  No settings found for section: {section_filter}")
        print(f"  Valid sections: {', '.join(_SECTION_ORDER)}")

    print()


def exec_show(args: argparse.Namespace) -> None:
    result = _api_get(user_id=args.user_id)
    if not result.get('success'):
        print(f"[settings] Error: {result.get('error', 'Unknown error')}", file=sys.stderr)
        sys.exit(1)

    data = result.get('data', {})
    settings = data.get('settings', {})
    if not settings:
        print("[settings] No settings returned from platform.")
        return

    section = getattr(args, 'section', None)
    _print_settings(settings, section)


def exec_get(args: argparse.Namespace) -> None:
    result = _api_get(user_id=args.user_id)
    if not result.get('success'):
        sys.exit(1)
    
    data = result.get('data', {})
    settings = data.get('settings', {})
    meta = settings.get(args.key)
    if meta is None:
        sys.exit(1)
    value = meta.get('value') or ''
    if value:
        print(value, end='')


def _operator_get_platform_settings() -> Dict[str, Any]:
    result = get_document(OPERATOR_SETTINGS_COLLECTION, PLATFORM_SETTINGS_ID)
    return result if result else {}


def _operator_put_platform_settings(doc: Dict[str, Any]) -> None:
    operator_request('PUT', f'/db/{OPERATOR_SETTINGS_COLLECTION}/{PLATFORM_SETTINGS_ID}', doc)


def exec_rotate_session_key(_args: argparse.Namespace) -> None:
    new_key = secrets.token_hex(32)
    now = datetime.now(timezone.utc).isoformat()

    # 1. Update operator document
    doc = _operator_get_platform_settings()

    if not doc:
        doc = {
            'settings': {},
            'created_at': now,
            'updated_at': now,
        }

    doc.setdefault('settings', {})
    doc['settings']['session_encryption_key'] = new_key
    doc['updated_at'] = now

    _operator_put_platform_settings(doc)

    print(new_key, end='')


def _parse_assignments(assignments: list) -> Dict[str, str]:
    settings: Dict[str, str] = {}
    for pair in assignments:
        if '=' not in pair:
            print(f"[settings] Invalid assignment '{pair}' — expected key=value", file=sys.stderr)
            sys.exit(1)
        key, _, value = pair.partition('=')
        settings[key.strip()] = value
    return settings


def _direct_set(settings: Dict[str, str]) -> None:
    """Write settings directly to the operator platform_settings document."""
    now = datetime.now(timezone.utc).isoformat()
    doc = _operator_get_platform_settings()

    if not doc:
        doc = {
            'settings': {},
            'created_at': now,
            'updated_at': now,
        }

    doc.setdefault('settings', {})
    for key, value in settings.items():
        doc['settings'][key] = value
    doc['updated_at'] = now

    _operator_put_platform_settings(doc)
    for key in settings:
        print(f"  [OK] {key}")


def exec_set(args: argparse.Namespace) -> None:
    settings = _parse_assignments(args.assignments)

    if not settings:
        print("[settings] No settings provided.", file=sys.stderr)
        sys.exit(1)

    result = _api_put(settings, user_id=args.user_id)
    if not result.get('success'):
        print(f"[settings] Error: {result.get('error', 'Unknown error')}", file=sys.stderr)
        sys.exit(1)

    for key in result.get('saved', []):
        print(f"  [OK] {key}")
    for key in result.get('skipped', []):
        print(f"  [SKIP] {key} (env-locked or unknown)")


def exec_export(args: argparse.Namespace) -> None:
    """Export current effective settings as a clean JSON blob."""
    result = _api_get(user_id=args.user_id)
    if not result.get('success'):
        print(f"[settings] Error: {result.get('error', 'Unknown error')}", file=sys.stderr)
        sys.exit(1)

    data = result.get('data', {})
    settings_meta = data.get('settings', {})
    
    # Flatten settings metadata to key:value map
    flat_settings = {k: v.get('value') for k, v in settings_meta.items()}
    
    if args.section:
        filtered = {}
        for k, v in settings_meta.items():
            if v.get('section') == args.section:
                filtered[k] = v.get('value')
        flat_settings = filtered

    print(json.dumps(flat_settings, indent=2))


def build_parser() -> argparse.ArgumentParser:
    parser = argparse.ArgumentParser(
        description='Read and write platform settings via the Operator API.',
        formatter_class=argparse.RawDescriptionHelpFormatter,
        epilog=__doc__,
    )
    subparsers = parser.add_subparsers(dest='command')

    sp = subparsers.add_parser('show', help='Show effective platform settings')
    sp.add_argument(
        '--section',
        metavar='SECTION',
        default=None,
        help='Filter by section: general, llm, search, security',
    )
    sp.add_argument('--user-id', help='User ID for user-specific settings')
    sp.set_defaults(func=exec_show)

    sp = subparsers.add_parser('get', help='Read a single setting value (plain text, for scripting)')
    sp.add_argument(
        'key',
        metavar='KEY',
        help='Setting key (e.g. llm_provider, llm_model)',
    )
    sp.add_argument('--user-id', help='User ID for user-specific settings')
    sp.set_defaults(func=exec_get)

    sp = subparsers.add_parser('set', help='Write settings to the DB (key=value ...)')
    sp.add_argument(
        'assignments',
        metavar='key=value',
        nargs='+',
        help='One or more key=value pairs to persist (e.g. llm_model=gemma3:4b)',
    )
    sp.add_argument('--user-id', help='User ID for user-specific settings')
    sp.set_defaults(func=exec_set)

    sp = subparsers.add_parser('export', help='Export settings as clean JSON')
    sp.add_argument(
        '--section',
        metavar='SECTION',
        default=None,
        help='Filter by section: general, llm, search, security',
    )
    sp.add_argument('--user-id', help='User ID for user-specific settings')
    sp.set_defaults(func=exec_export)

    subparsers.add_parser(
        'rotate-session-key',
        help='Generate a new session_encryption_key and write it directly to operator',
    ).set_defaults(func=exec_rotate_session_key)

    return parser


def run(argv: List[str]) -> int:
    parser = build_parser()
    args = parser.parse_args(argv)
    if not args.command:
        parser.print_help()
        return 1

    _machine_readable = args.command in ('get', 'rotate-session-key')
    if not _machine_readable:
        print_banner('manage-operator.py settings', ' '.join(argv))

    try:
        args.func(args)
    except RuntimeError as e:
        print(f'[manage-operator settings] {e}', file=sys.stderr)
        return 1
    return 0


def main() -> int:
    return run(sys.argv[1:])


if __name__ == '__main__':
    sys.exit(main())
