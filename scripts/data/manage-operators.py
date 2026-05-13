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

Manage operator documents via the Operator (g8eo) HTTP API.

Usage:
    python manage-operator.py operators list --user-id USER_ID
    python manage-operator.py operators list --email user@example.com
    python manage-operator.py operators get --id OPERATOR_ID
    python manage-operator.py operators init-slots --user-id USER_ID
    python manage-operator.py operators init-slots --email user@example.com
    python manage-operator.py operators refresh-key --id OPERATOR_ID
    python manage-operator.py operators get-key --id OPERATOR_ID
    python manage-operator.py operators terminate --id OPERATOR_ID
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

OPERATORS_API = f'{OPERATOR_BASE_URL}/api/operators'


def _obfuscate_sensitive(value: str | None) -> str:
    if not value or len(value) < 10:
        return '***'
    return f"{value[:5]}...{value[-5:]}"


class OperatorManager:
    """
    Manage operator documents via the Operator (g8eo) HTTP API.
    """

    def _format_operator_summary(self, op: Dict[str, Any]) -> str:
        op_id = op.get('operator_id', op.get('id', 'N/A'))
        name = (op.get('name') or 'N/A')[:20]
        status = (op.get('status') or 'N/A')[:12]
        slot = op.get('slot_number', 'N/A')
        op_type = (op.get('operator_type') or 'N/A')[:10]
        has_key = 'yes' if op.get('has_api_key') or op.get('operator_api_key') else 'no'
        claimed = 'yes' if op.get('claimed') else 'no'
        heartbeat = (op.get('latest_heartbeat_snapshot', {}).get('timestamp') or 'never')[:19]
        return (
            f"  {op_id}  "
            f"{name:<22} "
            f"slot={str(slot):<3} "
            f"status={status:<12} "
            f"type={op_type:<10} "
            f"key={has_key:<3} "
            f"claimed={claimed:<3} "
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
            f"    Last Heartbeat:  {op.get('latest_heartbeat_snapshot', {}).get('timestamp') or 'never'}",
            f"    Terminated:      {op.get('terminated_at') or 'N/A'}",
        ]

        heartbeat_snapshot = op.get('latest_heartbeat_snapshot')
        if heartbeat_snapshot:
            system_identity = heartbeat_snapshot.get('system_identity') or {}
            network = heartbeat_snapshot.get('network') or {}
            lines += [
                "",
                "  System Info (from latest heartbeat):",
                f"    Hostname:        {system_identity.get('hostname') or 'N/A'}",
                f"    OS:              {system_identity.get('os') or 'N/A'}",
                f"    Architecture:    {system_identity.get('architecture') or 'N/A'}",
                f"    Public IP:       {network.get('public_ip') or 'N/A'}",
            ]

        lines += ["=" * 80, ""]
        return '\n'.join(lines)

    # =========================================================================
    # Commands
    # =========================================================================

    def list_operators(self, user_id: str | None, email: str | None, all_statuses: bool = False) -> List[Dict]:
        uid = None
        if user_id or email:
            uid = resolve_user_id(user_id, email)
            if not uid:
                return []
            header_text = f"Operators for user {uid}"
        else:
            header_text = "All Operators"

        # Query operators collection directly
        query = {'user_id': uid} if uid else {}
        operators = operator_request('POST', '/db/operators/_query', query)
        if not isinstance(operators, list):
            operators = []

        # Filter by status if needed
        if not all_statuses:
            operators = [op for op in operators if op.get('status') != 'terminated']

        active_count = sum(1 for op in operators if op.get('status') == 'active')

        print(f"\n{header_text} ({len(operators)} shown, {active_count} active)")
        print("=" * 140)
        if not operators:
            print("  No operators found")
        else:
            for op in operators:
                print(self._format_operator_summary(op))
        print(f"\n  Total: {len(operators)}  Active: {active_count}")
        print()
        return operators

    def get_operator(self, operator_id: str) -> Dict | None:
        op = operator_request('GET', f'/db/operators/{operator_id}')
        if not op:
            print(f"\nOperator not found: {operator_id}")
            return None

        print(self._format_operator_detail(op))
        return op

    def init_slots(self, user_id: str | None, email: str | None) -> list[str] | None:
        uid = resolve_user_id(user_id, email)
        if not uid:
            return None

        # Check if user already has operator slots
        existing = operator_request('POST', '/db/operators/_query', {'user_id': uid})
        if isinstance(existing, list) and len(existing) > 0:
            print(f"\nUser {uid} already has {len(existing)} operator slot(s)")
            for op in existing:
                print(f"    {op.get('id')} - slot {op.get('slot_number', 'N/A')}")
            print()
            return [op.get('id') for op in existing]

        # Create initial operator slot
        import time
        op_doc = {
            'id': f'op_{uid}_slot_0',
            'user_id': uid,
            'organization_id': uid,
            'status': 'available',
            'slot_number': 0,
            'is_slot': True,
            'claimed': False,
            'operator_type': 'slot',
            'cloud_subtype': 'g8ep',
            'created_at': int(time.time() * 1000),
            'updated_at': int(time.time() * 1000),
        }

        result = operator_request('PUT', f'/db/operators/{op_doc["id"]}', op_doc)
        if not result:
            raise RuntimeError('Failed to create operator slot')

        print(f"\nOperator slot initialized for user {uid}")
        print(f"  Slot 0: {op_doc['id']}")
        print()
        return [op_doc['id']]

    def refresh_key(self, operator_id: str, force: bool = False) -> Dict | None:
        op = operator_request('GET', f'/db/operators/{operator_id}')
        if not op:
            print(f"\nOperator not found: {operator_id}")
            return None

        print(f"\nAbout to rotate API key for operator:")
        print(f"  ID:       {operator_id}")
        print(f"  Name:     {op.get('name', 'N/A')}")
        print(f"  Slot:     {op.get('slot_number', 'N/A')}")
        print(f"  Status:   {op.get('status', 'N/A')}")
        print(f"  User ID:  {op.get('user_id', 'N/A')}")
        print()
        print("  This will rotate the operator's API key.")

        if not force:
            response = input("\nType 'rotate' to confirm: ")
            if response.strip().lower() != 'rotate':
                print("Rotation cancelled.")
                return None

        user_id = op.get('user_id')
        if not user_id:
            raise RuntimeError("Operator has no user_id — cannot rotate key")

        result = operator_request(
            'POST',
            f'{OPERATORS_API}/rotate-api-key',
            {'operator_id': operator_id, 'user_id': user_id}
        )

        if not result or not result.get('success'):
            raise RuntimeError(result.get('error', 'Failed to rotate API key') if result else 'Failed to rotate API key')

        print(f"\nAPI key rotated successfully.")
        print(f"  Operator ID:      {operator_id}")
        print(f"  New API Key:      {_obfuscate_sensitive(result.get('api_key'))}")
        print()
        return result

    def get_key(self, operator_id: str) -> str | None:
        op = operator_request('GET', f'/db/operators/{operator_id}')
        if not op:
            print(f"\nOperator not found: {operator_id}")
            return None

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

    def terminate(self, operator_id: str, force: bool = False) -> Dict | None:
        op = operator_request('GET', f'/db/operators/{operator_id}')
        if not op:
            print(f"\nOperator not found: {operator_id}")
            return None

        user_id = op.get('user_id')
        print(f"\nAbout to terminate operator:")
        print(f"  ID:       {operator_id}")
        print(f"  Name:     {op.get('name', 'N/A')}")
        print(f"  Slot:     {op.get('slot_number', 'N/A')}")
        print(f"  Status:   {op.get('status', 'N/A')}")
        print(f"  User ID:  {user_id or 'N/A'}")
        print()
        print("  This will terminate the operator session and mark it as terminated.")

        if not force:
            response = input("\nType 'terminate' to confirm: ")
            if response.strip().lower() != 'terminate':
                print("Termination cancelled.")
                return None

        if not user_id:
            raise RuntimeError("Operator has no user_id — cannot terminate")

        result = operator_request(
            'POST',
            f'{OPERATORS_API}/terminate',
            {'operator_id': operator_id, 'user_id': user_id, 'reason': 'manual termination'}
        )

        if not result or not result.get('success'):
            raise RuntimeError(result.get('error', 'Failed to terminate operator') if result else 'Failed to terminate operator')

        print(f"\nOperator terminated successfully.")
        print(f"  Operator ID:  {operator_id}")
        print()
        return result


def build_parser() -> argparse.ArgumentParser:
    parser = argparse.ArgumentParser(
        description='Operator Management Script for g8e Platform',
        formatter_class=argparse.RawDescriptionHelpFormatter,
        epilog="""
Examples:
  python manage-operator.py operators list --user-id USER_ID
  python manage-operator.py operators list --email user@example.com
  python manage-operator.py operators list --email user@example.com --all
  python manage-operator.py operators get --id OPERATOR_ID
  python manage-operator.py operators init-slots --user-id USER_ID
  python manage-operator.py operators init-slots --email user@example.com
  python manage-operator.py operators refresh-key --id OPERATOR_ID
  python manage-operator.py operators refresh-key --id OPERATOR_ID --force
  python manage-operator.py operators get-key --id OPERATOR_ID
  python manage-operator.py operators terminate --id OPERATOR_ID
  python manage-operator.py operators terminate --id OPERATOR_ID --force
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
        'terminate',
        help='Terminate an operator session'
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

    print_banner('manage-operator.py operators', ' '.join(argv))
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
        elif args.command == 'terminate':
            manager.terminate(args.operator_id, force=args.force)
    except RuntimeError as e:
        print(f'[manage-operator operators] {e}', file=sys.stderr)
        return 1

    return 0


def main() -> int:
    return run(sys.argv[1:])


if __name__ == '__main__':
    sys.exit(main())
