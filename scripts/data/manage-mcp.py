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
MCP Integration Script

Generate configuration snippets for external MCP clients (Claude Code, Windsurf,
Cursor, etc.) and test connectivity to the g8e MCP endpoint.

Runs inside g8ep and communicates with g8ed over the internal network.

Usage:
    python manage-g8es.py mcp config --client claude-code --email user@example.com
    python manage-g8es.py mcp config --client windsurf --email user@example.com
    python manage-g8es.py mcp config --client cursor --email user@example.com
    python manage-g8es.py mcp config --client generic --email user@example.com
    python manage-g8es.py mcp test --email user@example.com
    python manage-g8es.py mcp status
"""

import argparse
import json
import ssl
import sys
import urllib.error
import urllib.parse
import urllib.request
from pathlib import Path
from typing import Any, Dict, Optional

from _lib import (
    G8ED_BASE_URL,
    g8ed_request,
    get_internal_auth_token,
    print_banner,
    resolve_user_id,
)

SUPPORTED_CLIENTS = ['claude-code', 'windsurf', 'cursor', 'generic']

CA_CERT_PATH = '/g8es/ssl/ca.crt'


def _get_platform_url() -> str:
    result = g8ed_request('GET', f'{G8ED_BASE_URL}/api/internal/settings')
    if not result.get('success'):
        return 'https://localhost'
    settings = result.get('data', {}).get('settings', {})
    app_url_meta = settings.get('app_url', {})
    url = app_url_meta.get('value', '') if isinstance(app_url_meta, dict) else str(app_url_meta)
    return url or 'https://localhost'


def _get_user_g8e_key(user_id: str) -> Optional[str]:
    result = g8ed_request('GET', f'{G8ED_BASE_URL}/api/internal/users/{user_id}')
    if not result.get('success'):
        return None
    user = result.get('data', {})
    return user.get('g8e_key')


def _generate_config(client: str, mcp_url: str, api_key: str) -> str:
    if client == 'claude-code':
        config = {
            "mcpServers": {
                "g8e": {
                    "type": "streamable-http",
                    "url": f"{mcp_url}/mcp",
                    "headers": {
                        "x-oauth-client-id": api_key
                    }
                }
            }
        }
        return json.dumps(config, indent=2)

    elif client == 'windsurf':
        config = {
            "mcpServers": {
                "g8e": {
                    "serverUrl": f"{mcp_url}/mcp",
                    "headers": {
                        "x-oauth-client-id": api_key
                    }
                }
            }
        }
        return json.dumps(config, indent=2)

    elif client == 'cursor':
        config = {
            "mcpServers": {
                "g8e": {
                    "transport": "streamable-http",
                    "url": f"{mcp_url}/mcp",
                    "headers": {
                        "x-oauth-client-id": api_key
                    }
                }
            }
        }
        return json.dumps(config, indent=2)

    else:
        config = {
            "mcpServers": {
                "g8e": {
                    "transport": {
                        "type": "streamable-http",
                        "url": f"{mcp_url}/mcp",
                        "headers": {
                            "x-oauth-client-id": api_key
                        }
                    }
                }
            }
        }
        return json.dumps(config, indent=2)


def _mcp_request(mcp_url: str, api_key: str, method: str, req_id: str = "1",
                 params: Optional[Dict] = None) -> Dict[str, Any]:
    payload = {
        "jsonrpc": "2.0",
        "id": req_id,
        "method": method,
    }
    if params:
        payload["params"] = params

    data = json.dumps(payload).encode()
    headers = {
        'Content-Type': 'application/json',
        'x-oauth-client-id': api_key,
    }

    url = f"{mcp_url}/mcp"
    req = urllib.request.Request(url, data=data, headers=headers, method='POST')

    ctx = ssl.create_default_context()
    if Path(CA_CERT_PATH).exists():
        ctx.load_verify_locations(CA_CERT_PATH)

    try:
        with urllib.request.urlopen(req, context=ctx) as resp:
            return json.loads(resp.read().decode())
    except urllib.error.HTTPError as e:
        body = e.read().decode()
        try:
            return json.loads(body)
        except Exception:
            return {"error": {"code": e.code, "message": body}}
    except urllib.error.URLError as e:
        return {"error": {"code": -1, "message": str(e.reason)}}


def exec_config(args: argparse.Namespace) -> int:
    print_banner('mcp', f'config --client {args.client}')

    user_id = resolve_user_id(args.user_id, args.email)
    if not user_id:
        print("[mcp] Error: Could not resolve user. Provide --email or --user-id.", file=sys.stderr)
        return 1

    mcp_url = _get_platform_url()
    api_key = _get_user_g8e_key(user_id)

    if not api_key:
        print("[mcp] Error: No G8eKey found for this user.", file=sys.stderr)
        print("  Generate one in the g8e dashboard or via:", file=sys.stderr)
        print("    curl -X POST https://<host>/api/user/me/refresh-g8e-key", file=sys.stderr)
        return 1

    config_json = _generate_config(args.client, mcp_url, api_key)

    print(f"  Platform URL:  {mcp_url}")
    print(f"  MCP Endpoint:  {mcp_url}/mcp")
    print(f"  Client:        {args.client}")
    print(f"  Auth:          G8eKey (x-oauth-client-id)")
    print()

    if args.client == 'claude-code':
        print("  Add to ~/.claude/settings.json (or run: claude mcp add-json 'g8e' '<config>'):")
    elif args.client == 'windsurf':
        print("  Add to your Windsurf MCP configuration:")
    elif args.client == 'cursor':
        print("  Add to your Cursor MCP configuration:")
    else:
        print("  Generic MCP server configuration (Streamable HTTP transport):")
    print()
    for line in config_json.split('\n'):
        print(f"  {line}")
    print()

    if mcp_url == 'https://localhost':
        print("  Note: app_url is 'https://localhost'. If connecting from another machine,")
        print("  update app_url via: ./g8e data settings set app_url=https://your-host")
        print()

    return 0


def exec_test(args: argparse.Namespace) -> int:
    print_banner('mcp', 'test')

    user_id = resolve_user_id(args.user_id, args.email)
    if not user_id:
        print("[mcp] Error: Could not resolve user. Provide --email or --user-id.", file=sys.stderr)
        return 1

    mcp_url = _get_platform_url()
    api_key = _get_user_g8e_key(user_id)

    if not api_key:
        print("[mcp] Error: No G8eKey found for this user.", file=sys.stderr)
        return 1

    print(f"  Endpoint:  {mcp_url}/mcp")
    print()

    # Test 1: initialize
    print("  [1/3] initialize ... ", end='', flush=True)
    resp = _mcp_request(mcp_url, api_key, "initialize", "test-init")
    if resp.get('error'):
        err = resp['error']
        print(f"FAIL ({err.get('message', err)})")
        return 1
    result = resp.get('result', {})
    server_name = result.get('serverInfo', {}).get('name', '?')
    server_version = result.get('serverInfo', {}).get('version', '?')
    protocol = result.get('protocolVersion', '?')
    print(f"OK ({server_name} v{server_version}, protocol {protocol})")

    # Test 2: tools/list
    print("  [2/3] tools/list ... ", end='', flush=True)
    resp = _mcp_request(mcp_url, api_key, "tools/list", "test-tools")
    if resp.get('error'):
        err = resp['error']
        print(f"FAIL ({err.get('message', err)})")
        return 1
    tools = resp.get('result', {}).get('tools', [])
    tool_names = [t.get('name', '?') for t in tools]
    print(f"OK ({len(tools)} tools)")
    for name in tool_names:
        print(f"    - {name}")

    # Test 3: ping
    print("  [3/3] ping ......... ", end='', flush=True)
    resp = _mcp_request(mcp_url, api_key, "ping", "test-ping")
    if resp.get('error'):
        err = resp['error']
        print(f"FAIL ({err.get('message', err)})")
        return 1
    print("OK")

    print()
    print("  MCP endpoint is healthy and responding.")
    if not tools:
        print("  Warning: No tools returned. Ensure at least one operator is bound to this user.")
    print()
    return 0


def exec_status(args: argparse.Namespace) -> int:
    print_banner('mcp', 'status')

    mcp_url = _get_platform_url()
    print(f"  Platform URL:   {mcp_url}")
    print(f"  MCP Endpoint:   {mcp_url}/mcp")
    print(f"  Transport:      Streamable HTTP (POST)")
    print(f"  Protocol:       MCP 2025-03-26 (JSON-RPC 2.0)")
    print(f"  Auth Methods:   G8eKey (x-oauth-client-id), Session Token (Bearer)")
    print()
    print("  Supported Methods:")
    print("    initialize, notifications/initialized, ping, tools/list, tools/call")
    print()
    print("  Supported Clients:")
    print("    Claude Code, Windsurf, Cursor, Cline, or any MCP Streamable HTTP client")
    print()
    return 0


def build_parser() -> argparse.ArgumentParser:
    parser = argparse.ArgumentParser(
        prog='manage-mcp',
        description='MCP client integration management',
        formatter_class=argparse.RawDescriptionHelpFormatter,
        epilog="""
Examples:
  python manage-g8es.py mcp config --client claude-code --email user@example.com
  python manage-g8es.py mcp config --client windsurf --email user@example.com
  python manage-g8es.py mcp test --email user@example.com
  python manage-g8es.py mcp status
""",
    )
    subparsers = parser.add_subparsers(dest='command')

    sp = subparsers.add_parser('config', help='Generate MCP client configuration')
    sp.add_argument('--client', required=True, choices=SUPPORTED_CLIENTS,
                    help='Target MCP client')
    sp.add_argument('--email', help='User email')
    sp.add_argument('--user-id', dest='user_id', help='User ID')

    sp = subparsers.add_parser('test', help='Test MCP endpoint connectivity')
    sp.add_argument('--email', help='User email')
    sp.add_argument('--user-id', dest='user_id', help='User ID')

    sp = subparsers.add_parser('status', help='Show MCP endpoint status')

    return parser


def run(argv: list[str]) -> int:
    parser = build_parser()
    args = parser.parse_args(argv)

    if not args.command:
        parser.print_help()
        return 1

    try:
        if args.command == 'config':
            return exec_config(args)
        elif args.command == 'test':
            return exec_test(args)
        elif args.command == 'status':
            return exec_status(args)
    except RuntimeError as e:
        print(f'[manage-g8es mcp] {e}', file=sys.stderr)
        return 1
    return 0


def main() -> int:
    return run(sys.argv[1:])


if __name__ == '__main__':
    sys.exit(main())
