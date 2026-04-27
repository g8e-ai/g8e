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
Operator Management Script for g8e Platform

Manage operator documents via the g8ed internal HTTP API.
Runs inside g8ep and communicates with g8ed over the internal network.

Usage:
    python manage-g8es.py operators list --user-id USER_ID
    python manage-g8es.py operators list --email user@example.com
    python manage-g8es.py operators get --id OPERATOR_ID
    python manage-g8es.py operators init-slots --user-id USER_ID
    python manage-g8es.py operators init-slots --email user@example.com
    python manage-g8es.py operators refresh-key --id OPERATOR_ID
    python manage-g8es.py operators get-key --id OPERATOR_ID
    python manage-g8es.py operators reset --id OPERATOR_ID
"""

from __future__ import annotations

import argparse
import sys
from typing import Dict, Any, List

from _lib import (
    G8ED_BASE_URL,
    print_banner,
    resolve_user_id,
    g8ed_request,
)

INTERNAL_OPERATORS_BASE = f'{G8ED_BASE_URL}/api/internal/operators'
INTERNAL_USERS_BASE = f'{G8ED_BASE_URL}/api/internal/users'


def _obfuscate_sensitive(value: str | None) -> str:
    if not value or len(value) < 10:
        return '***'
    return f"{value[:5]}...{value[-5:]}"


class OperatorManager:
    """
    Manage operator documents via the g8ed internal HTTP API.
    """

    def _format_operator_summary(self, op: Dict[str, Any]) -> str:
        op_id = op.get('operator_id', op.get('id', 'N/A'))
        name = (op.get('name') or 'N/A')[:20]
        status = (op.get('status') or 'N/A')[:12]
        slot = op.get('slot_number', 'N/A')
        op_type = (op.get('operator_type') or 'N/A')[:10]
        has_key = 'yes' if op.get('has_api_key') or op.get('operator_api_key') else 'no'
        claimed = 'yes' if op.get('claimed') else 'no'
        is_g8ep = 'yes' if op.get('is_g8ep') else 'no'
        heartbeat = (op.get('last_heartbeat') or 'never')[:19]
        return (
            f"  {op_id}  "
            f"{name:<22} "
            f"slot={str(slot):<3} "
            f"status={status:<12} "
            f"type={op_type:<10} "
            f"key={has_key:<3} "
            f"claimed={claimed:<3} "
            f"g8ep={is_g8ep:<3} "
            f"heartbeat={heartbeat}"
        )

    def _format_operator_detail(self, op: Dict[str, Any]) -> str:
        op_id = op.get('operator_id', op.get('id', 'N/A'))
        lines = [
            "",
            "=" * 80,
            f"OPERATOR: {op.get('name', 'N/A')} ({op_id})",
            "=" * 80,
            f"  Operator ID:       {op_id}",
            f"  Name:              {op.get('name', 'N/A')}",
            f"  User ID:           {op.get('user_id', 'N/A')}",
            f"  Organization ID:   {op.get('organization_id', 'N/A')}",
            f"  Slot Number:       {op.get('slot_number', 'N/A')}",
            f"  Operator Type:     {op.get('operator_type', 'N/A')}",
            f"  Cloud Subtype:     {op.get('cloud_subtype', 'N/A')}",
            f"  Is g8e node:       {op.get('is_g8ep', False)}",
            f"  Slot Cost:         {op.get('slot_cost', 'N/A')}",
            "",
            "  Status:",
            f"    Status:          {op.get('status', 'N/A')}",
            f"    Claimed:         {op.get('claimed', False)}",
            f"    Has API Key:     {bool(op.get('has_api_key') or op.get('operator_api_key'))}",
            f"    Cert Serial:     {op.get('operator_cert_serial') or 'none'}",
            "",
            "  Session:",
            f"    Operator Session:  {op.get('operator_session_id') or 'none'}",
            f"    Web Session:       {op.get('web_session_id') or 'none'}",
            "",
            "  Timestamps:",
            f"    Created:         {op.get('created_at', 'N/A')}",
            f"    Updated:         {op.get('updated_at', 'N/A')}",
            f"    Started:         {op.get('started_at') or 'N/A'}",
            f"    First Deployed:  {op.get('first_deployed') or 'N/A'}",
            f"    Last Heartbeat:  {op.get('last_heartbeat') or 'never'}",
            f"    Terminated:      {op.get('terminated_at') or 'N/A'}",
        ]

        system_info = op.get('system_info') or {}
        if system_info:
            lines += [
                "",
                "  System Info:",
                f"    Hostname:        {system_info.get('hostname') or 'N/A'}",
                f"    OS:              {system_info.get('os') or 'N/A'}",
                f"    Architecture:    {system_info.get('arch') or 'N/A'}",
                f"    Public IP:       {system_info.get('public_ip') or 'N/A'}",
            ]

        lines += ["=" * 80, ""]
        return '\n'.join(lines)

    # =========================================================================
    # Commands
    # =========================================================================

    def list_operators(self, user_id: str | None, email: str | None, all_statuses: bool = False) -> List[Dict]:
        if user_id or email:
            uid = resolve_user_id(user_id, email)
            if not uid:
                return []
            endpoint = f'{INTERNAL_OPERATORS_BASE}/user/{uid}'
            header_text = f"Operators for user {uid}"
        else:
            endpoint = INTERNAL_OPERATORS_BASE
            header_text = "All Operators"

        params = {}
        if all_statuses:
            params['all'] = 'true'

        if params:
            import urllib.parse
            query = urllib.parse.urlencode(params)
            endpoint = f"{endpoint}?{query}"

        result = g8ed_request('GET', endpoint)
        if not result.get('success'):
            raise RuntimeError(result.get('error', 'Failed to list operators'))

        operators = result.get('data', [])
        total = result.get('total_count', len(operators))
        active = result.get('active_count', 0)

        # The internal user endpoint filters terminated by default unless all_statuses is true
        # but the new listAllOperators also does this, so we don't necessarily need to filter here
        # but for consistency with existing behavior of list_operators we will.
        if not all_statuses and (user_id or email):
            # This is already handled by g8ed for the user-specific endpoint
            pass

        print(f"\n{header_text} ({len(operators)} shown, {active} active)")
        print("=" * 140)
        if not operators:
            print("  No operators found")
        else:
            for op in operators:
                print(self._format_operator_summary(op))
        print(f"\n  Total (non-terminated): {total}  Active: {active}")
        print()
        return operators

    def get_operator(self, operator_id: str) -> Dict | None:
        result = g8ed_request('GET', f'{INTERNAL_OPERATORS_BASE}/{operator_id}')
        if not result.get('success'):
            if result.get('_status_code') == 404:
                print(f"\nOperator not found: {operator_id}")
            else:
                raise RuntimeError(result.get('error', 'Failed to get operator'))
            return None

        op = result['data']
        print(self._format_operator_detail(op))
        return op

    def init_slots(self, user_id: str | None, email: str | None) -> list[str] | None:
        uid = resolve_user_id(user_id, email)
        if not uid:
            return None

        user_result = g8ed_request('GET', f'{INTERNAL_USERS_BASE}/{uid}')
        org_id = uid
        if user_result.get('success'):
            org_id = user_result['data'].get('organization_id') or uid

        result = g8ed_request(
            'POST',
            f'{INTERNAL_OPERATORS_BASE}/user/{uid}/initialize-slots',
            {'organization_id': org_id}
        )
        if not result.get('success'):
            raise RuntimeError(result.get('error', 'Failed to initialize operator slots'))

        operator_ids = result.get('operator_ids', [])
        count = result.get('count', len(operator_ids))

        print(f"\nOperator slots initialized for user {uid}")
        print(f"  Total slots: {count}")
        for op_id in operator_ids:
            print(f"    {op_id}")
        print()
        return operator_ids

    def refresh_key(self, operator_id: str, force: bool = False) -> Dict | None:
        existing = g8ed_request('GET', f'{INTERNAL_OPERATORS_BASE}/{operator_id}')
        if not existing.get('success'):
            if existing.get('_status_code') == 404:
                print(f"\nOperator not found: {operator_id}")
            else:
                raise RuntimeError(existing.get('error', 'Failed to get operator'))
            return None

        op = existing['data']
        print(f"\nAbout to refresh API key for operator:")
        print(f"  ID:       {operator_id}")
        print(f"  Name:     {op.get('name', 'N/A')}")
        print(f"  Slot:     {op.get('slot_number', 'N/A')}")
        print(f"  Status:   {op.get('status', 'N/A')}")
        print(f"  User ID:  {op.get('user_id', 'N/A')}")
        print()
        print("  This terminates the old operator document and creates a new one.")
        print("  The old API key will be invalidated immediately.")

        if not force:
            response = input("\nType 'refresh' to confirm: ")
            if response.strip().lower() != 'refresh':
                print("Refresh cancelled.")
                return None

        user_id = op.get('user_id')
        if not user_id:
            raise RuntimeError("Operator has no user_id — cannot refresh")

        result = g8ed_request(
            'POST',
            f'{INTERNAL_OPERATORS_BASE}/{operator_id}/refresh-key',
            {'user_id': user_id}
        )

        if not result.get('success'):
            raise RuntimeError(result.get('error', result.get('message', 'Failed to refresh API key')))

        print(f"\nAPI key refreshed successfully.")
        print(f"  Old Operator ID:  {result.get('old_operator_id', operator_id)}")
        print(f"  New Operator ID:  {result.get('new_operator_id', 'N/A')}")
        print(f"  Slot Number:      {result.get('slot_number', 'N/A')}")
        print(f"  New API Key:      {_obfuscate_sensitive(result.get('new_api_key'))}")
        print()
        return result

    def get_key(self, operator_id: str) -> str | None:
        result = g8ed_request('GET', f'{INTERNAL_OPERATORS_BASE}/{operator_id}')
        if not result.get('success'):
            if result.get('_status_code') == 404:
                print(f"\nOperator not found: {operator_id}")
            else:
                raise RuntimeError(result.get('error', 'Failed to get operator'))
            return None

        op = result['data']
        api_key = op.get('operator_api_key') or op.get('api_key')

        if not api_key:
            print(f"\nOperator {operator_id} has no API key set.")
            return None

        print(f"\nOperator API Key")
        print(f"  Operator ID:  {operator_id}")
        print(f"  Name:         {op.get('name', 'N/A')}")
        print(f"  Slot:         {op.get('slot_number', 'N/A')}")
        print(f"  Status:       {op.get('status', 'N/A')}")
        print(f"  API Key:      {_obfuscate_sensitive(api_key)}")
        print()
        return api_key

    def reset(self, operator_id: str, force: bool = False) -> Dict | None:
        existing = g8ed_request('GET', f'{INTERNAL_OPERATORS_BASE}/{operator_id}')
        if not existing.get('success'):
            if existing.get('_status_code') == 404:
                print(f"\nOperator not found: {operator_id}")
            else:
                raise RuntimeError(existing.get('error', 'Failed to get operator'))
            return None

        op = existing['data']
        print(f"\nAbout to reset operator to fresh AVAILABLE state:")
        print(f"  ID:       {operator_id}")
        print(f"  Name:     {op.get('name', 'N/A')}")
        print(f"  Slot:     {op.get('slot_number', 'N/A')}")
        print(f"  Status:   {op.get('status', 'N/A')}")
        print(f"  User ID:  {op.get('user_id', 'N/A')}")
        print()
        print("  This deletes and recreates the operator document with default values.")
        print("  The existing API key is preserved. All session state is cleared.")

        if not force:
            response = input("\nType 'reset' to confirm: ")
            if response.strip().lower() != 'reset':
                print("Reset cancelled.")
                return None

        result = g8ed_request(
            'POST',
            f'{INTERNAL_OPERATORS_BASE}/{operator_id}/reset-cache'
        )
        if not result.get('success'):
            raise RuntimeError(result.get('error', 'Failed to reset operator'))

        print(f"\nOperator reset successfully.")
        print(f"  Operator ID:  {operator_id}")
        print(f"  New Status:   {result.get('status', 'N/A')}")
        print()
        return result


def build_parser() -> argparse.ArgumentParser:
    parser = argparse.ArgumentParser(
        description='Operator Management Script for g8e Platform',
        formatter_class=argparse.RawDescriptionHelpFormatter,
        epilog="""
Examples:
  python manage-g8es.py operators list --user-id USER_ID
  python manage-g8es.py operators list --email user@example.com
  python manage-g8es.py operators list --email user@example.com --all
  python manage-g8es.py operators get --id OPERATOR_ID
  python manage-g8es.py operators init-slots --user-id USER_ID
  python manage-g8es.py operators init-slots --email user@example.com
  python manage-g8es.py operators refresh-key --id OPERATOR_ID
  python manage-g8es.py operators refresh-key --id OPERATOR_ID --force
  python manage-g8es.py operators get-key --id OPERATOR_ID
  python manage-g8es.py operators reset --id OPERATOR_ID
  python manage-g8es.py operators reset --id OPERATOR_ID --force
        """
    )

    subparsers = parser.add_subparsers(dest='command', help='Command to execute')

    sp = subparsers.add_parser('list', help='List operators for a user')
    sp.add_argument('--user-id', dest='user_id', help='User ID')
    sp.add_argument('--email', help='User email (resolved to user ID)')
    sp.add_argument('--all', dest='all_statuses', action='store_true',
                    help='Include all statuses (default: excludes terminated)')

    sp = subparsers.add_parser('get', help='Get full operator details')
    sp.add_argument('--id', dest='operator_id', required=True, help='Operator ID')

    sp = subparsers.add_parser('init-slots', help='Initialize operator slots for a user')
    sp.add_argument('--user-id', dest='user_id', help='User ID')
    sp.add_argument('--email', help='User email (resolved to user ID)')

    sp = subparsers.add_parser(
        'refresh-key',
        help='Refresh operator API key (terminates old operator, creates new one)'
    )
    sp.add_argument('--id', dest='operator_id', required=True, help='Operator ID')
    sp.add_argument('--force', action='store_true', help='Skip confirmation prompt')

    sp = subparsers.add_parser('get-key', help='Fetch current API key for an operator')
    sp.add_argument('--id', dest='operator_id', required=True, help='Operator ID')

    sp = subparsers.add_parser(
        'reset',
        help='Reset operator to fresh AVAILABLE state (preserves API key, clears session state)'
    )
    sp.add_argument('--id', dest='operator_id', required=True, help='Operator ID')
    sp.add_argument('--force', action='store_true', help='Skip confirmation prompt')

    return parser


def run(argv: List[str]) -> int:
    parser = build_parser()
    args = parser.parse_args(argv)

    if not args.command:
        parser.print_help()
        return 1

    print_banner('manage-g8es.py operators', ' '.join(argv))
    manager = OperatorManager()

    try:
        if args.command == 'list':
            manager.list_operators(
                user_id=args.user_id,
                email=args.email,
                all_statuses=args.all_statuses
            )
        elif args.command == 'get':
            manager.get_operator(args.operator_id)
        elif args.command == 'init-slots':
            if not args.user_id and not args.email:
                print("Error: provide --user-id or --email")
                return 1
            manager.init_slots(user_id=args.user_id, email=args.email)
        elif args.command == 'refresh-key':
            manager.refresh_key(args.operator_id, force=args.force)
        elif args.command == 'get-key':
            manager.get_key(args.operator_id)
        elif args.command == 'reset':
            manager.reset(args.operator_id, force=args.force)
    except RuntimeError as e:
        print(f'[manage-g8es operators] {e}', file=sys.stderr)
        return 1

    return 0


def main() -> int:
    return run(sys.argv[1:])


if __name__ == '__main__':
    sys.exit(main())
