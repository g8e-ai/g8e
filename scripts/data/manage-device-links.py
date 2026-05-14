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
Device Link Management Script for g8e Platform

Manage device link tokens via the Operator (g8eo) HTTP API.

Usage:
    python manage-operator.py device-links list --user-id USER_ID
    python manage-operator.py device-links list --email user@example.com
    python manage-operator.py device-links create --user-id USER_ID
    python manage-operator.py device-links create --user-id USER_ID --name "prod-fleet" --max-uses 50 --expires-in-hours 24
    python manage-operator.py device-links create --email user@example.com --name "staging"
    python manage-operator.py device-links revoke --token dlk_...
    python manage-operator.py device-links delete --token dlk_...
"""

from __future__ import annotations

import argparse
import sys
from typing import Dict, Any, List

from _lib import (
    OPERATOR_BASE_URL,
    print_banner,
    resolve_user_id,
    operator_request,
)

DEVICE_LINKS_API = f'{OPERATOR_BASE_URL}/api/device-links'

TOKEN_RE_PREFIX = 'dlk_'


def _format_link(link: Dict[str, Any], verbose: bool = False) -> str:
    token = link.get('token', 'N/A')
    name = link.get('name') or ''
    status = link.get('status', 'N/A')
    uses = link.get('uses', 0)
    max_uses = link.get('max_uses', 'N/A')
    expires = (link.get('expires_at') or '')[:19].replace('T', ' ')
    created = (link.get('created_at') or '')[:10]

    name_str = f"  [{name}]" if name else ''
    line = (
        f"  {token}{name_str}  "
        f"status={status:<10} "
        f"uses={uses}/{max_uses:<6} "
        f"expires={expires}  "
        f"created={created}"
    )
    return line


class DeviceLinkManager:

    def list_links(self, user_id: str | None, email: str | None) -> List[Dict]:
        uid = resolve_user_id(user_id, email)
        if not uid:
            raise RuntimeError('Provide --user-id or --email')
        # Query device_links collection directly
        result = operator_request('POST', '/db/device_links/_query', {'user_id': uid})
        if not isinstance(result, list):
            result = []
        print(f"\nDevice Links for user {uid} ({len(result)} total)")
        print("=" * 110)
        if not result:
            print("  No device links found")
        else:
            for link in result:
                print(_format_link(link))
        print()
        return result

    def create_link(
        self,
        user_id: str | None,
        email: str | None,
        name: str | None,
        max_uses: int,
        expires_in_hours: int | None,
    ) -> Dict | None:
        uid = resolve_user_id(user_id, email)
        if not uid:
            raise RuntimeError('Provide --user-id or --email')

        if max_uses is None or max_uses < 1 or max_uses > 100:
            raise RuntimeError('max_uses must be between 1 and 100')

        body: Dict[str, Any] = {'max_uses': max_uses}
        if name:
            body['name'] = name
        if expires_in_hours is not None:
            body['expires_in_hours'] = expires_in_hours

        result = operator_request('POST', DEVICE_LINKS_API, body)
        if not result or not result.get('success'):
            raise RuntimeError(result.get('error', 'Failed to create device link') if result else 'Failed to create device link')

        print("\nDevice link created:")
        print(f"  Token:           {result['token']}")
        if result.get('name'):
            print(f"  Name:            {result['name']}")
        print(f"  Max Uses:        {result['max_uses']}")
        print(f"  Expires:         {result['expires_at']}")
        print(f"  Operator Command:")
        print(f"    {result['operator_command']}")
        print()
        return result

    def revoke_link(self, token: str) -> bool:
        if not token.startswith(TOKEN_RE_PREFIX):
            print(f"Invalid token format: {token}")
            return False

        result = operator_request('DELETE', f'{DEVICE_LINKS_API}/{token}')
        if not result or not result.get('success'):
            if isinstance(result, dict) and result.get('_status_code') == 404:
                print(f"\nDevice link not found: {token}")
            else:
                raise RuntimeError(result.get('error', 'Failed to revoke device link') if result else 'Failed to revoke device link')
            return False

        print(f"\nDevice link revoked: {token}")
        print()
        return True

    def delete_link(self, token: str, force: bool = False) -> bool:
        if not token.startswith(TOKEN_RE_PREFIX):
            print(f"Invalid token format: {token}")
            return False

        if not force:
            response = input(f"Permanently delete {token}? [y/N]: ")
            if response.strip().lower() != 'y':
                print("Deletion cancelled.")
                return False

        # For permanent delete, we use a query parameter
        import urllib.parse
        result = operator_request('DELETE', f'{DEVICE_LINKS_API}/{urllib.parse.quote(token, safe="")}?action=delete')
        if not result or not result.get('success'):
            if isinstance(result, dict) and result.get('_status_code') == 404:
                print(f"\nDevice link not found: {token}")
            else:
                raise RuntimeError(result.get('error', 'Failed to delete device link') if result else 'Failed to delete device link')
            return False

        print(f"\nDevice link deleted: {token}")
        print()
        return True


def _add_user_args(p: argparse.ArgumentParser) -> None:
    group = p.add_mutually_exclusive_group(required=True)
    group.add_argument('--user-id', dest='user_id', help='User ID')
    group.add_argument('--email', help='User email')


def build_parser() -> argparse.ArgumentParser:
    parser = argparse.ArgumentParser(
        description='Device Link Management Script for g8e Platform',
        formatter_class=argparse.RawDescriptionHelpFormatter,
        epilog="""
Examples:
  python manage-operator.py device-links list --user-id USER_ID
  python manage-operator.py device-links list --email user@example.com
  python manage-operator.py device-links create --user-id USER_ID
  python manage-operator.py device-links create --user-id USER_ID --name "prod-fleet" --max-uses 50
  python manage-operator.py device-links create --email user@example.com --expires-in-hours 48
  python manage-operator.py device-links revoke --token dlk_...
  python manage-operator.py device-links delete --token dlk_...
        """
    )

    subparsers = parser.add_subparsers(dest='command', help='Command to execute')

    sp = subparsers.add_parser('list', help='List device links for a user')
    _add_user_args(sp)

    sp = subparsers.add_parser('create', help='Create a new device link')
    _add_user_args(sp)
    sp.add_argument('--name', help='Human-readable label for this link')
    sp.add_argument('--max-uses', type=int, required=True,
                    help='Max number of devices that can claim this link (required, min: 1, max: 100)')
    sp.add_argument('--expires-in-hours', type=int, default=None,
                    help='Token lifetime in hours (default: 1 hour)')

    sp = subparsers.add_parser('revoke', help='Revoke a device link (marks as revoked, cannot be claimed)')
    sp.add_argument('--token', required=True, help='Device link token (dlk_...)')

    sp = subparsers.add_parser('delete', help='Permanently delete a device link (must be revoked/expired first)')
    sp.add_argument('--token', required=True, help='Device link token (dlk_...)')
    sp.add_argument('--force', action='store_true', help='Skip confirmation')

    return parser


def run(argv: List[str]) -> int:
    parser = build_parser()
    args = parser.parse_args(argv)

    if not args.command:
        parser.print_help()
        return 1

    print_banner('manage-operator.py device-links', ' '.join(argv))
    manager = DeviceLinkManager()

    try:
        if args.command == 'list':
            manager.list_links(user_id=args.user_id, email=args.email)
        elif args.command == 'create':
            manager.create_link(
                user_id=args.user_id,
                email=args.email,
                name=args.name,
                max_uses=args.max_uses,
                expires_in_hours=args.expires_in_hours,
            )
        elif args.command == 'revoke':
            manager.revoke_link(args.token)
        elif args.command == 'delete':
            manager.delete_link(args.token, force=args.force)
    except RuntimeError as e:
        print(f'[manage-operator device-links] {e}', file=sys.stderr)
        return 1

    return 0


def main() -> int:
    return run(sys.argv[1:])


if __name__ == '__main__':
    sys.exit(main())
