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
Passkey Management Script for g8e Platform

Manage FIDO2/WebAuthn passkey credentials via the g8ed internal HTTP API.
Runs inside g8ep and communicates with g8ed over the internal network.

Usage:
    ./g8e security passkeys list --id USER_ID
    ./g8e security passkeys list --email user@example.com
    ./g8e security passkeys revoke --id USER_ID --credential CRED_ID
    ./g8e security passkeys revoke-all --id USER_ID
    ./g8e security passkeys revoke-all --email user@example.com
    ./g8e security passkeys reset --email user@example.com
    ./g8e security passkeys reset --id USER_ID --force
"""

import argparse
import sys
import urllib.parse
from pathlib import Path
from typing import Optional, List, Dict, Any

PROJECT_ROOT = Path(__file__).parent.parent.parent.absolute()
sys.path.insert(0, str(PROJECT_ROOT / 'scripts' / 'data'))

from _lib import (
    G8ED_BASE_URL,
    resolve_user_id,
    g8ed_request,
)

INTERNAL_USER_BASE = f'{G8ED_BASE_URL}/api/internal/users'


def _api_request(method: str, path: str, body: Optional[Dict] = None) -> Dict:
    return g8ed_request(method, f'{INTERNAL_USER_BASE}{path}', body)


class PasskeyManager:

    def _format_credential(self, cred: Dict[str, Any], index: int) -> str:
        cred_id = cred.get('id', '')
        short_id = cred_id[:16] + '...' if len(cred_id) > 16 else cred_id
        transports = ', '.join(cred.get('transports') or []) or 'unknown'
        created = (cred.get('created_at') or 'unknown')[:19].replace('T', ' ')
        last_used = (cred.get('last_used_at') or 'never')
        if last_used != 'never':
            last_used = last_used[:19].replace('T', ' ')
        return (
            f"  [{index}] id={short_id:<20}  "
            f"transports=[{transports}]  "
            f"created={created}  "
            f"last_used={last_used}"
        )

    def list_credentials(self, user_id: Optional[str],
                         email: Optional[str]) -> Optional[List[Dict]]:
        uid = resolve_user_id(user_id, email)
        if not uid:
            return None

        result = _api_request('GET', f'/{uid}/passkeys')
        if not result.get('success'):
            raise RuntimeError(result.get('error', 'Failed to list passkeys'))

        credentials = result.get('credentials', [])
        identifier = email or uid

        print(f"\nPasskey credentials for {identifier} ({len(credentials)} registered)")
        print("=" * 90)
        if not credentials:
            print("  No passkeys registered.")
            print()
            print("  The user will need to visit the setup page to register a passkey.")
        else:
            for i, cred in enumerate(credentials):
                print(self._format_credential(cred, i + 1))
            print()
            print("  Full credential IDs:")
            for i, cred in enumerate(credentials):
                print(f"  [{i + 1}] {cred.get('id')}")
        print()
        return credentials

    def revoke_credential(self, credential_id: str, user_id: Optional[str],
                          email: Optional[str], force: bool = False) -> bool:
        uid = resolve_user_id(user_id, email)
        if not uid:
            return False

        identifier = email or uid
        print(f"\nAbout to revoke passkey credential:")
        print(f"  User:       {identifier}")
        print(f"  Credential: {credential_id}")

        if not force:
            response = input("\nType 'yes' to confirm: ")
            if response.strip().lower() != 'yes':
                print("Revocation cancelled.")
                return False

        del_result = _api_request('DELETE', f'/{uid}/passkeys/{urllib.parse.quote(credential_id, safe="")}')
        if not del_result.get('success'):
            if del_result.get('_status_code') == 404:
                error = del_result.get('error', '')
                if 'credential' in error.lower():
                    print(f"\nCredential not found: {credential_id}")
                    print("  Use 'list' to see registered credential IDs.")
                else:
                    print(f"\nUser not found: {uid}")
                return False
            raise RuntimeError(del_result.get('error', 'Failed to revoke credential'))

        remaining = del_result.get('remaining', 0)
        print(f"\nCredential revoked. User has {remaining} passkey(s) remaining.")
        if remaining == 0:
            print("  User must re-register a passkey at the setup page.")
        print()
        return True

    def reset(self, user_id: Optional[str],
              email: Optional[str], force: bool = False) -> bool:
        uid = resolve_user_id(user_id, email)
        if not uid:
            return False

        identifier = email or uid
        print(f"\nAbout to reset passkey credentials for {identifier}.")
        print("  Their existing sessions will expire naturally.")
        print("  On next login attempt, they will be prompted to register a new passkey.")

        if not force:
            response = input("\nType 'yes' to confirm: ")
            if response.strip().lower() != 'yes':
                print("Reset cancelled.")
                return False

        del_result = _api_request('DELETE', f'/{uid}/passkeys')
        if not del_result.get('success'):
            if del_result.get('_status_code') == 404:
                print(f"\nUser not found: {uid}")
                return False
            raise RuntimeError(del_result.get('error', 'Failed to reset passkeys'))

        revoked = del_result.get('revoked', 0)
        if revoked == 0:
            print(f"\nNo passkeys registered for {identifier}. Nothing to reset.")
        else:
            print(f"\nPasskey reset complete for {identifier} ({revoked} credential(s) removed).")
            print("  User will be prompted to register a new passkey on next login.")
        print()
        return True

    def revoke_all(self, user_id: Optional[str],
                   email: Optional[str], force: bool = False) -> bool:
        uid = resolve_user_id(user_id, email)
        if not uid:
            return False

        identifier = email or uid
        print(f"\nAbout to revoke ALL passkey credential(s) for {identifier}.")
        print("  The user will be locked out until a new passkey is registered.")

        if not force:
            response = input("\nType 'yes' to confirm: ")
            if response.strip().lower() != 'yes':
                print("Revocation cancelled.")
                return False

        del_result = _api_request('DELETE', f'/{uid}/passkeys')
        if not del_result.get('success'):
            if del_result.get('_status_code') == 404:
                print(f"\nUser not found: {uid}")
                return False
            raise RuntimeError(del_result.get('error', 'Failed to revoke passkeys'))

        revoked = del_result.get('revoked', 0)
        if revoked == 0:
            print(f"\nNo passkeys registered for {identifier}. Nothing to revoke.")
        else:
            print(f"\nAll {revoked} passkey credential(s) revoked for {identifier}.")
            print("  User must re-register a passkey at the setup page.")
        print()
        return True


def main():
    print("")
    print("━" * 52)
    print(f"  manage-passkeys.py {' '.join(sys.argv[1:])}")
    print("━" * 52)
    print("")

    parser = argparse.ArgumentParser(
        description='Passkey Management Script for g8e Platform',
        formatter_class=argparse.RawDescriptionHelpFormatter,
        epilog="""
Examples:
  ./g8e security passkeys list --email user@example.com
  ./g8e security passkeys list --id USER_ID
  ./g8e security passkeys reset --email user@example.com
  ./g8e security passkeys reset --id USER_ID --force
  ./g8e security passkeys revoke --id USER_ID --credential CRED_ID
  ./g8e security passkeys revoke --email user@example.com --credential CRED_ID --force
  ./g8e security passkeys revoke-all --email user@example.com
  ./g8e security passkeys revoke-all --id USER_ID --force
        """
    )

    subparsers = parser.add_subparsers(dest='command', help='Command to execute')

    list_parser = subparsers.add_parser('list', help='List passkey credentials for a user')
    list_parser.add_argument('--id', dest='user_id', help='User ID')
    list_parser.add_argument('--email', help='User email')

    revoke_parser = subparsers.add_parser('revoke', help='Revoke a specific passkey credential')
    revoke_parser.add_argument('--id', dest='user_id', help='User ID')
    revoke_parser.add_argument('--email', help='User email')
    revoke_parser.add_argument('--credential', required=True, dest='credential_id',
                               help='Full credential ID to revoke (from list output)')
    revoke_parser.add_argument('--force', action='store_true', help='Skip confirmation')

    reset_parser = subparsers.add_parser('reset', help='Reset passkey credentials — user will be prompted to register a new passkey on next login')
    reset_parser.add_argument('--id', dest='user_id', help='User ID')
    reset_parser.add_argument('--email', help='User email')
    reset_parser.add_argument('--force', action='store_true', help='Skip confirmation')

    revoke_all_parser = subparsers.add_parser('revoke-all', help='Revoke all passkey credentials for a user')
    revoke_all_parser.add_argument('--id', dest='user_id', help='User ID')
    revoke_all_parser.add_argument('--email', help='User email')
    revoke_all_parser.add_argument('--force', action='store_true', help='Skip confirmation')

    args = parser.parse_args()

    if not args.command:
        parser.print_help()
        return 1

    if args.command in ('list', 'revoke', 'revoke-all', 'reset'):
        if not args.user_id and not args.email:
            parser.error('--id or --email is required')

    manager = PasskeyManager()

    try:
        if args.command == 'list':
            manager.list_credentials(user_id=args.user_id, email=args.email)
        elif args.command == 'reset':
            manager.reset(user_id=args.user_id, email=args.email, force=args.force)
        elif args.command == 'revoke':
            manager.revoke_credential(
                credential_id=args.credential_id,
                user_id=args.user_id,
                email=args.email,
                force=args.force,
            )
        elif args.command == 'revoke-all':
            manager.revoke_all(user_id=args.user_id, email=args.email, force=args.force)

    except Exception as e:
        print(f'[manage-passkeys] {e}', file=sys.stderr)
        return 1

    return 0


if __name__ == '__main__':
    sys.exit(main())
