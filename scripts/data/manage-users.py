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
User Management Script for g8e Platform

Manage platform users via the Operator (g8eo) HTTP API.

Usage:
    python manage-operator.py users list
    python manage-operator.py users get --id USER_ID
    python manage-operator.py users get --email user@example.com
    python manage-operator.py users search "john"
    python manage-operator.py users create --email user@example.com --name "John Doe"
    python manage-operator.py users update-role --id USER_ID --role admin
    python manage-operator.py users delete --id USER_ID
    python manage-operator.py users stats
"""

from __future__ import annotations

import argparse
import json
import sys
import urllib.parse
from typing import List, Dict, Any

from _lib import (
    PROJECT_ROOT,
    OPERATOR_BASE_URL,
    print_banner,
    operator_request,
)

_SHARED_CONSTANTS = PROJECT_ROOT / 'shared' / 'constants'

with open(_SHARED_CONSTANTS / 'status.json') as _f:
    _STATUS = json.load(_f)
VALID_ROLES: List[str] = list(_STATUS['user.role'].values())

USERS_API = f'{OPERATOR_BASE_URL}/api/users'


class UserManager:
    """
    Manage platform users via the Operator (g8eo) HTTP API.
    """

    def _format_user_summary(self, user: Dict[str, Any]) -> str:
        roles = user.get('roles', [])
        role_str = ', '.join(roles) if roles else 'none'
        op_status = user.get('operator_status') or 'none'
        created = (user.get('created_at') or '')[:10]
        last_login = (user.get('last_login') or 'never')[:10]
        return (
            f"  {user['id']}  "
            f"{user.get('email', 'N/A'):<35} "
            f"{user.get('name', 'N/A'):<20} "
            f"roles=[{role_str}]  "
            f"op={op_status:<12} "
            f"created={created}  "
            f"login={last_login}"
        )

    def _format_user_detail(self, user: Dict[str, Any]) -> str:
        lines = [
            "",
            "=" * 70,
            f"USER: {user.get('name', 'N/A')} ({user.get('email', 'N/A')})",
            "=" * 70,
            f"  ID:              {user.get('id')}",
            f"  Email:           {user.get('email')}",
            f"  Name:            {user.get('name')}",
            f"  Provider:        {user.get('provider', 'N/A')}",
            f"  Roles:           {user.get('roles', [])}",
            f"  Organization ID: {user.get('organization_id', 'N/A')}",
            "",
            "  Operator:",
            f"    Operator ID:     {user.get('operator_id', 'none')}",
            f"    Operator Status: {user.get('operator_status', 'none')}",
            "",
            "  API Keys:",
            f"    Download Key:    {'set' if user.get('g8e_key') else 'not set'}",
            f"    Key Created:     {user.get('g8e_key_created_at', 'N/A')}",
            "",
            "  Timestamps:",
            f"    Created:     {user.get('created_at', 'N/A')}",
            f"    Updated:     {user.get('updated_at', 'N/A')}",
            f"    Last Login:  {user.get('last_login', 'N/A')}",
            "=" * 70,
            "",
        ]
        return '\n'.join(lines)

    # =========================================================================
    # Commands
    # =========================================================================

    def list_users(self, limit: int = 50) -> List[Dict[str, Any]]:
        # Query users collection directly from operator
        users = operator_request('POST', '/db/users/_query', {})
        if not isinstance(users, list):
            users = []
        total = len(users)
        users = users[:limit]

        print(f"\nUsers ({len(users)} of {total} total)")
        print("=" * 130)
        if not users:
            print("  No users found")
        else:
            for user in users:
                print(self._format_user_summary(user))
        if total > limit:
            print(f"\n  Showing {limit} of {total}. Use --limit to see more.")
        print()
        return users

    def get_user(self, user_id: str | None,
                 email: str | None) -> Dict[str, Any] | None:
        if not user_id and not email:
            print("Provide --id or --email")
            return None

        if user_id:
            user = operator_request('GET', f'/db/users/{user_id}')
        else:
            # Query by email
            users = operator_request('POST', '/db/users/_query', {'email': email})
            if not isinstance(users, list) or len(users) == 0:
                print(f"\nUser not found: {email}")
                return None
            user = users[0]

        if not user:
            identifier = user_id or email
            print(f"\nUser not found: {identifier}")
            return None

        print(self._format_user_detail(user))
        return user

    def search_users(self, query: str, limit: int = 50) -> List[Dict[str, Any]]:
        # Query all users and filter locally
        all_users = operator_request('POST', '/db/users/_query', {})
        if not isinstance(all_users, list):
            all_users = []

        q = query.lower()
        users = [
            u for u in all_users
            if q in (u.get('email') or '').lower() or q in (u.get('name') or '').lower()
        ][:limit]

        print(f"\nSearch results for '{query}' ({len(users)} found)")
        print("=" * 130)
        if not users:
            print("  No users found")
        else:
            for user in users:
                print(self._format_user_summary(user))
        print()
        return users

    def create_user(self, email: str, name: str,
                    roles: list[str] | None) -> Dict[str, Any] | None:
        if roles:
            for role in roles:
                if role not in VALID_ROLES:
                    print(f"Invalid role: {role}. Valid roles: {VALID_ROLES}")
                    return None

        import time
        user_id = f'user_{int(time.time() * 1000)}'
        body: Dict[str, Any] = {
            'id': user_id,
            'email': email,
            'name': name,
            'created_at': int(time.time() * 1000),
            'updated_at': int(time.time() * 1000),
        }
        if roles:
            body['roles'] = roles

        result = operator_request('PUT', f'/db/users/{user_id}', body)
        if not result:
            raise RuntimeError('Failed to create user')

        print("\nUser created successfully:")
        print(f"  ID:       {user_id}")
        print(f"  Email:    {email}")
        print(f"  Name:     {name}")
        print(f"  Roles:    {roles or ['user']}")
        print()
        return body

    def delete_user(self, user_id: str, force: bool = False) -> bool:
        user = operator_request('GET', f'/db/users/{user_id}')
        if not user:
            print(f"\nUser not found: {user_id}")
            return False

        print("\nAbout to delete user:")
        print(f"  ID:    {user.get('id')}")
        print(f"  Email: {user.get('email')}")
        print(f"  Name:  {user.get('name')}")

        if not force:
            response = input("\nType the user's email to confirm deletion: ")
            if response.strip().lower() != (user.get('email') or '').lower():
                print("Deletion cancelled.")
                return False

        operator_request('DELETE', f'/db/users/{user_id}')
        print(f"\nUser {user_id} deleted.")
        return True

    def update_role(self, user_id: str, role: str,
                    action: str = 'set') -> Dict[str, Any] | None:
        if role not in VALID_ROLES:
            print(f"Invalid role: {role}. Valid roles: {VALID_ROLES}")
            return None

        # Get current user
        user = operator_request('GET', f'/db/users/{user_id}')
        if not user:
            print(f"\nUser not found: {user_id}")
            return None

        # Update roles
        current_roles = user.get('roles', [])
        if action == 'set':
            new_roles = [role]
        elif action == 'add':
            new_roles = list(set(current_roles + [role]))
        elif action == 'remove':
            new_roles = [r for r in current_roles if r != role]
        else:
            new_roles = current_roles

        user['roles'] = new_roles
        user['updated_at'] = int(__import__('time').time() * 1000)
        
        operator_request('PUT', f'/db/users/{user_id}', user)
        
        print(f"\nRoles updated for {user.get('email')}:")
        print(f"  Roles: {new_roles}")
        print()
        return user

    def stats(self) -> Dict[str, Any]:
        # Query all users and count
        users = operator_request('POST', '/db/users/_query', {})
        if not isinstance(users, list):
            users = []
        total = len(users)
        
        print(f"\n{'=' * 60}")
        print("USER STATISTICS")
        print(f"{'=' * 60}")
        print(f"\n  Total users: {total}")
        print(f"\n{'=' * 60}\n")
        return {'total_users': total}


def build_parser() -> argparse.ArgumentParser:
    parser = argparse.ArgumentParser(
        description='User Management Script for g8e Platform',
        formatter_class=argparse.RawDescriptionHelpFormatter,
        epilog="""
Examples:
  python manage-operator.py users list
  python manage-operator.py users get --email user@example.com
  python manage-operator.py users search "john"
  python manage-operator.py users create --email new@example.com --name "New User"
  python manage-operator.py users update-role --id USER_ID --role admin
  python manage-operator.py users update-role --id USER_ID --role admin --action add
  python manage-operator.py users delete --id USER_ID
  python manage-operator.py users stats
        """
    )

    subparsers = parser.add_subparsers(dest='command', help='Command to execute')

    sp = subparsers.add_parser('list', help='List all users')
    sp.add_argument('--limit', type=int, default=50, help='Max users to show (default: 50)')

    sp = subparsers.add_parser('get', help='Get user details')
    sp.add_argument('--id', dest='user_id', help='User ID')
    sp.add_argument('--email', help='User email')

    sp = subparsers.add_parser('search', help='Search users by name or email')
    sp.add_argument('query', help='Search query (substring match)')
    sp.add_argument('--limit', type=int, default=50, help='Max results (default: 50)')

    sp = subparsers.add_parser('create', help='Create a new user')
    sp.add_argument('--email', required=True, help='User email')
    sp.add_argument('--name', required=True, help='Display name')
    sp.add_argument('--roles', nargs='+', default=None,
                    help=f'Roles to assign (default: user). Valid: {VALID_ROLES}')

    sp = subparsers.add_parser('delete', help='Delete a user')
    sp.add_argument('--id', dest='user_id', required=True, help='User ID')
    sp.add_argument('--force', action='store_true', help='Skip confirmation')

    sp = subparsers.add_parser('update-role', help='Update user roles')
    sp.add_argument('--id', dest='user_id', required=True, help='User ID')
    sp.add_argument('--role', required=True, help=f'Role to set. Valid: {VALID_ROLES}')
    sp.add_argument('--action', choices=['set', 'add', 'remove'], default='set',
                    help="'set' replaces all roles, 'add' appends, 'remove' removes (default: set)")

    subparsers.add_parser('stats', help='Show user statistics')

    return parser


def run(argv: List[str]) -> int:
    parser = build_parser()
    args = parser.parse_args(argv)

    if not args.command:
        parser.print_help()
        return 1

    print_banner('manage-operator.py users', ' '.join(argv))
    manager = UserManager()

    try:
        if args.command == 'list':
            manager.list_users(limit=args.limit)
        elif args.command == 'get':
            manager.get_user(user_id=args.user_id, email=args.email)
        elif args.command == 'search':
            manager.search_users(args.query, limit=args.limit)
        elif args.command == 'create':
            manager.create_user(
                email=args.email, name=args.name,
                roles=args.roles
            )
        elif args.command == 'delete':
            manager.delete_user(args.user_id, force=args.force)
        elif args.command == 'update-role':
            manager.update_role(args.user_id, args.role, action=args.action)
        elif args.command == 'stats':
            manager.stats()
    except RuntimeError as e:
        print(f'[manage-operator users] {e}', file=sys.stderr)
        return 1

    return 0


def main() -> int:
    return run(sys.argv[1:])


if __name__ == '__main__':
    sys.exit(main())
