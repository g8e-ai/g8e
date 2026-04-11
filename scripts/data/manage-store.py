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
VSODB Document Store & KV Management

Queries and manages the VSODB document store and KV store via the HTTP API.
Runs inside g8e-pod and communicates with vsodb directly.

Usage:
    python manage-vsodb.py store stats
    python manage-vsodb.py store operators
    python manage-vsodb.py store web_sessions
    python manage-vsodb.py store doc --collection operators --id <id>
    python manage-vsodb.py store kv
    python manage-vsodb.py store kv --pattern "g8e:session:*"
    python manage-vsodb.py store kv-get --key "g8e:session:web:session_123"
    python manage-vsodb.py store network
    python manage-vsodb.py store find --collection operators --field status --value active
    python manage-vsodb.py store wipe [--dry-run]
    python manage-vsodb.py store get-setting <key>
"""

import argparse
import json
import sys
from typing import Dict, List, Optional

from _lib import (
    COLLECTIONS,
    PRESERVE_COLLECTIONS,
    delete_document,
    get_document,
    kv_delete_pattern,
    kv_get,
    kv_keys,
    print_banner,
    print_table,
    query_collection,
    vsodb_request,
)


# =============================================================================
# Summary field extractors
# =============================================================================

def _summary_fields(collection: str, d: Dict) -> Dict:
    if collection == 'operators':
        si = d.get('system_info') or {}
        lhs = d.get('latest_heartbeat_snapshot') or {}
        sys_id = lhs.get('system_identity') or {}
        cs = (lhs.get('network') or {}).get('connectivity_status') or []
        skip = ('172.', '127.')
        private_ip = next(
            (i.get('ip') for i in cs if i.get('ip') and not any(i['ip'].startswith(p) for p in skip)),
            cs[0].get('ip') if cs else None,
        )
        return {
            'id': d.get('id'),
            'status': d.get('status'),
            'name': d.get('name'),
            'hostname': si.get('hostname') or sys_id.get('hostname'),
            'os': si.get('os') or sys_id.get('os'),
            'public_ip': si.get('public_ip'),
            'private_ip': private_ip,
            'updated_at': d.get('updated_at'),
        }
    if collection in ('web_sessions', 'operator_sessions'):
        return {
            'id': d.get('id'),
            'session_type': d.get('session_type'),
            'is_active': d.get('is_active'),
            'login_method': d.get('login_method'),
            'user_id': d.get('user_id'),
            'created_at': d.get('created_at'),
        }
    if collection == 'investigations':
        return {
            'id': d.get('id'),
            'status': d.get('status'),
            'case_title': d.get('case_title'),
            'case_id': d.get('case_id'),
            'message_count': len(d.get('conversation_history', [])),
            'updated_at': d.get('updated_at'),
        }
    if collection == 'cases':
        return {
            'id': d.get('id'),
            'status': d.get('status'),
            'title': d.get('title'),
            'priority': d.get('priority'),
            'user_id': d.get('user_id'),
            'created_at': d.get('created_at'),
        }
    if collection == 'users':
        return {
            'id': d.get('id'),
            'email': d.get('email'),
            'roles': d.get('roles'),
            'provider': d.get('provider'),
            'created_at': d.get('created_at'),
        }
    if collection == 'organizations':
        return {
            'id': d.get('id'),
            'name': d.get('name'),
            'owner_id': d.get('owner_id'),
            'created_at': d.get('created_at'),
        }
    if collection == 'api_keys':
        return {
            'id': d.get('id'),
            'status': d.get('status'),
            'client_name': d.get('client_name'),
            'operator_id': d.get('operator_id'),
            'permissions': d.get('permissions'),
            'created_at': d.get('created_at'),
        }
    if collection == 'login_audit':
        return {
            'id': d.get('id'),
            'event_type': d.get('event_type'),
            'result': d.get('result'),
            'auth_method': d.get('auth_method'),
            'user_id': d.get('user_id'),
            'timestamp': d.get('timestamp'),
        }
    return d


# =============================================================================
# Commands
# =============================================================================

def exec_stats() -> None:
    total_docs = 0
    collection_counts: List[Dict] = []
    for col in COLLECTIONS:
        docs = query_collection(col)
        count = len(docs)
        total_docs += count
        if count:
            collection_counts.append({'collection': col, 'count': count})

    keys = kv_keys('*')

    print(f'\n{"=" * 60}')
    print('VSODB Statistics')
    print(f'{"=" * 60}')
    print(f'\n  Records:')
    print(f'    Documents:  {total_docs}')
    print(f'    KV keys:    {len(keys)}')
    if collection_counts:
        print('\n  Collections:')
        for row in sorted(collection_counts, key=lambda r: -r['count']):
            print(f'    {row["collection"]:<25} {row["count"]}')
    print(f'\n{"=" * 60}\n')


def exec_list_collection(collection: str, limit: int = 50,
                        fields: Optional[List[str]] = None,
                        as_json: bool = False) -> None:
    docs = query_collection(collection, limit=limit)
    out = []
    for d in docs:
        if fields:
            out.append({f: d.get(f) for f in fields})
        else:
            out.append(_summary_fields(collection, d))

    print(f'\n{collection} ({len(docs)})')
    if not docs:
        print('  (empty)')
    elif as_json:
        print(json.dumps(out, indent=2, default=str))
    else:
        print_table(out)
    print()


def exec_doc(collection: str, doc_id: str) -> None:
    doc = get_document(collection, doc_id)
    if doc is None:
        print(f'\nDocument not found: {collection}/{doc_id}')
        return
    print(f'\n{"=" * 80}')
    print(f'DOCUMENT: {collection}/{doc_id}')
    print(f'{"=" * 80}')
    print(json.dumps(doc, indent=2, default=str))
    print(f'{"=" * 80}\n')


def exec_network(limit: int = 50) -> None:
    docs = query_collection('operators', limit=limit)
    out = []
    for d in docs:
        si = d.get('system_info') or {}
        lhs = d.get('latest_heartbeat_snapshot') or {}
        net = lhs.get('network') or {}
        cs = net.get('connectivity_status') or []
        sys_id = lhs.get('system_identity') or {}
        skip = ('172.', '127.')
        private_ip = next(
            (i.get('ip') for i in cs if i.get('ip') and not any(i['ip'].startswith(p) for p in skip)),
            cs[0].get('ip') if cs else None,
        )
        iface_summary = ', '.join(
            f"{i['name']}={i['ip']}" for i in cs if i.get('ip')
        ) or None
        out.append({
            'name': d.get('name'),
            'status': d.get('status'),
            'hostname': si.get('hostname') or sys_id.get('hostname'),
            'public_ip': si.get('public_ip') or net.get('public_ip'),
            'private_ip': private_ip,
            'interfaces': iface_summary,
            'os': si.get('os') or sys_id.get('os'),
            'arch': si.get('architecture') or sys_id.get('architecture'),
            'updated_at': d.get('updated_at'),
        })

    print(f'\noperators network ({len(out)})')
    if not out:
        print('  (empty)')
    else:
        print_table(out)
    print()


def exec_find(collection: str, field: str, value: str,
             limit: int = 50, as_json: bool = False) -> None:
    docs = query_collection(collection, limit=limit)
    matched = [d for d in docs if str(d.get(field, '')) == value]

    print(f'\nfind {collection} where {field}={value!r} ({len(matched)} results)')
    if not matched:
        print('  (no matches)')
    elif as_json:
        print(json.dumps(matched, indent=2, default=str))
    else:
        out = [_summary_fields(collection, d) for d in matched]
        print_table(out)
    print()


def exec_kv(pattern: Optional[str], limit: int = 50, as_json: bool = False) -> None:
    keys = kv_keys(pattern or '*')
    if limit:
        keys = keys[:limit]

    label = f' [pattern={pattern}]' if pattern else ''
    print(f'\nKV Store{label} ({len(keys)} keys)')
    if not keys:
        print('  (empty)')
    elif as_json:
        print(json.dumps(keys, indent=2))
    else:
        for k in keys:
            print(f'  {k}')
    print()


def exec_kv_get(key: str) -> None:
    value = kv_get(key)
    if value is None:
        print(f'\nKey not found: {key}')
        return
    print(f'\n{"=" * 60}')
    print(f'KEY: {key}')
    print(f'{"=" * 60}')
    try:
        print(json.dumps(json.loads(value), indent=2))
    except (json.JSONDecodeError, TypeError):
        print(value)
    print(f'{"=" * 60}\n')


def _sse_events_count() -> int:
    result = vsodb_request('GET', '/db/_sse_events/count')
    return int(result.get('count', 0)) if isinstance(result, dict) else 0


def _sse_events_wipe() -> int:
    result = vsodb_request('DELETE', '/db/_sse_events')
    return int(result.get('deleted', 0)) if isinstance(result, dict) else 0


def exec_wipe(dry_run: bool = False) -> None:
    app_collections = sorted(c for c in COLLECTIONS if c not in PRESERVE_COLLECTIONS)

    print('')
    if dry_run:
        print('  [dry-run] No changes will be made.')
    print(f'  Preserving: {sorted(PRESERVE_COLLECTIONS)}')
    print(f'  Collections to clear: {app_collections}')
    print('')

    total_docs = 0
    for collection in app_collections:
        docs = query_collection(collection)
        if not docs:
            print(f'  {collection}: (empty)')
            continue
        print(f'  {collection}: {len(docs)} document(s)')
        if not dry_run:
            for doc in docs:
                doc_id = doc.get('id')
                if doc_id:
                    delete_document(collection, doc_id)
        total_docs += len(docs)

    print('')
    kv_deleted = 0
    if not dry_run:
        kv_deleted = kv_delete_pattern('*')
    else:
        kv_deleted = len(kv_keys('*'))
    print(f'  KV store: {kv_deleted} key(s) {"would be " if dry_run else ""}deleted')

    sse_count = _sse_events_count()
    if not dry_run:
        sse_deleted = _sse_events_wipe()
        print(f'  SSE events: {sse_deleted} row(s) deleted')
    else:
        print(f'  SSE events: {sse_count} row(s) would be deleted')

    print('')
    if dry_run:
        print(f'  [dry-run] Would delete {total_docs} document(s) across {len(app_collections)} collection(s).')
    else:
        print(f'  Done. {total_docs} document(s) deleted across {len(app_collections)} collection(s).')
    print('')


def exec_get_setting(key: str) -> None:
    doc = get_document('settings', 'platform_settings')
    if doc is None:
        return
    value = (doc.get('settings') or {}).get(key)
    if value is not None:
        print(value, end='')


# =============================================================================
# CLI
# =============================================================================

def build_parser() -> argparse.ArgumentParser:
    parser = argparse.ArgumentParser(
        description='VSODB Document Store & KV Management',
        formatter_class=argparse.RawDescriptionHelpFormatter,
        epilog="""
Examples:
  python manage-vsodb.py store stats
  python manage-vsodb.py store operators
  python manage-vsodb.py store doc --collection operators --id <id>
  python manage-vsodb.py store kv --pattern "g8e:session:*"
  python manage-vsodb.py store kv-get --key "g8e:session:web:session_123"
  python manage-vsodb.py store network
  python manage-vsodb.py store find --collection operators --field status --value active
  python manage-vsodb.py store wipe --dry-run
        """
    )

    parser.add_argument('--json', dest='as_json', action='store_true',
                        help='Output as JSON instead of table')

    subparsers = parser.add_subparsers(dest='command', help='Command to execute')

    subparsers.add_parser('stats', help='Show database statistics')

    sp = subparsers.add_parser('network', help='Show operator network details (IPs, interfaces)')
    sp.add_argument('--limit', type=int, default=50)

    sp = subparsers.add_parser('find', help='Find documents where field=value')
    sp.add_argument('--collection', required=True, help='Collection to search')
    sp.add_argument('--field', required=True, help='Field name to match')
    sp.add_argument('--value', required=True, help='Exact value to match')
    sp.add_argument('--limit', type=int, default=50)

    for col in COLLECTIONS:
        sp = subparsers.add_parser(col, help=f'List {col}')
        sp.add_argument('--limit', type=int, default=50)
        sp.add_argument('--fields', nargs='+', metavar='FIELD',
                        help='Fields to display (default: smart summary)')

    sp = subparsers.add_parser('doc', help='Get a single document by collection + id')
    sp.add_argument('--collection', required=True, help='Collection name')
    sp.add_argument('--id', dest='doc_id', required=True, help='Document ID')

    sp = subparsers.add_parser('kv', help='List KV store keys')
    sp.add_argument('--pattern', help='Key pattern (supports * and ? wildcards)')
    sp.add_argument('--limit', type=int, default=50)

    sp = subparsers.add_parser('kv-get', help='Get a single KV value')
    sp.add_argument('--key', required=True, help='Key to retrieve')

    sp = subparsers.add_parser('wipe', help='Clear all app data (preserves platform settings)')
    sp.add_argument('--dry-run', action='store_true', help='Show what would be deleted without deleting')

    sp = subparsers.add_parser('get-setting', help='Read a single platform setting value')
    sp.add_argument('key', help='Setting key (e.g. llm_model)')

    return parser


def run(argv: List[str]) -> int:
    parser = build_parser()
    args = parser.parse_args(argv)

    if not args.command:
        parser.print_help()
        return 1

    _machine_readable = args.command == 'get-setting'
    if not _machine_readable:
        print_banner('manage-vsodb.py store', ' '.join(argv))

    try:
        if args.command == 'stats':
            exec_stats()
        elif args.command == 'network':
            exec_network(limit=args.limit)
        elif args.command == 'find':
            exec_find(args.collection, args.field, args.value,
                     limit=args.limit, as_json=args.as_json)
        elif args.command in COLLECTIONS:
            exec_list_collection(args.command,
                                limit=args.limit,
                                fields=getattr(args, 'fields', None),
                                as_json=args.as_json)
        elif args.command == 'doc':
            exec_doc(args.collection, args.doc_id)
        elif args.command == 'kv':
            exec_kv(pattern=args.pattern, limit=args.limit, as_json=args.as_json)
        elif args.command == 'kv-get':
            exec_kv_get(args.key)
        elif args.command == 'wipe':
            exec_wipe(dry_run=args.dry_run)
        elif args.command == 'get-setting':
            exec_get_setting(args.key)
    except RuntimeError as e:
        print(f'[manage-vsodb store] {e}', file=sys.stderr)
        return 1

    return 0


def main() -> int:
    return run(sys.argv[1:])


if __name__ == '__main__':
    sys.exit(main())
