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
Shared utilities for g8e data management scripts.

Provides authentication, HTTP clients for the Operator substrate,
and terminal display helpers used across all resource scripts.
"""

from __future__ import annotations

import json
import os
import shutil
import ssl
import urllib.error
import urllib.parse
import urllib.request
from pathlib import Path
from typing import Any, Dict, List

PROJECT_ROOT = Path(__file__).parent.parent.parent.absolute()
_SHARED_CONSTANTS = PROJECT_ROOT / 'shared' / 'constants'

with open(_SHARED_CONSTANTS / 'collections.json') as _f:
    _COLLECTIONS_DATA = json.load(_f)

_DEFAULT_PKI_DIR = str(PROJECT_ROOT / '.g8e' / 'pki')
_DEFAULT_SECRETS_DIR = str(PROJECT_ROOT / '.g8e' / 'secrets')
_DEFAULT_CREDENTIALS_DIR = str(Path.home() / '.g8e')

OPERATOR_BASE_URL = os.environ.get('G8E_INTERNAL_HTTP_URL', 'https://localhost:9000')
COLLECTIONS: List[str] = sorted(set(_COLLECTIONS_DATA['collections'].values()))
PRESERVE_COLLECTIONS = {'settings'}

PKI_DIR = Path(os.environ.get('G8E_PKI_DIR', _DEFAULT_PKI_DIR))
SECRETS_DIR = Path(os.environ.get('G8E_SECRETS_DIR', _DEFAULT_SECRETS_DIR))
CREDENTIALS_DIR = Path(os.environ.get('G8E_CREDENTIALS_DIR', _DEFAULT_CREDENTIALS_DIR))
TRUST_BUNDLE_PATH = PKI_DIR / 'trust' / 'hub-bundle.pem'
INTERNAL_AUTH_TOKEN_PATH = SECRETS_DIR / 'internal_auth_token'


def _get_cli_cert() -> tuple[str, str] | None:
    """Return (cert_path, key_path) for the CLI mTLS client certificate.

    Preference order:
    1. G8E_CLI_CERT / G8E_CLI_KEY env vars (set by `g8e login`)
    2. ~/.g8e/cli.crt + ~/.g8e/cli.key
    3. Platform app cert (.g8e/pki/issued/apps/g8ee.crt) as fallback for
       operator-local tooling that has not yet run `g8e login`.
    """
    cert = os.environ.get('G8E_CLI_CERT', str(CREDENTIALS_DIR / 'cli.crt'))
    key = os.environ.get('G8E_CLI_KEY', str(CREDENTIALS_DIR / 'cli.key'))
    if Path(cert).exists() and Path(key).exists():
        return cert, key
    app_cert = str(PKI_DIR / 'issued' / 'apps' / 'g8ee.crt')
    app_key = str(PKI_DIR / 'issued' / 'apps' / 'g8ee.key')
    if Path(app_cert).exists() and Path(app_key).exists():
        return app_cert, app_key
    return None


def _create_ssl_context() -> ssl.SSLContext | None:
    """Create SSL context that trusts the platform CA."""
    ctx = ssl.create_default_context()
    if TRUST_BUNDLE_PATH.exists():
        ctx.load_verify_locations(str(TRUST_BUNDLE_PATH))
    cli_cert = _get_cli_cert()
    if cli_cert:
        ctx.load_cert_chain(cli_cert[0], cli_cert[1])
    return ctx


# =============================================================================
# Authentication
# =============================================================================

def get_auth_token() -> str:
    """Return the operator session token from env (set by `g8e login`).

    The wrapper gates `g8e data` on a valid operator session before invoking
    this script, so OPERATOR_SESSION_ID is expected to be present.
    Operator calls use the internal auth token (see get_internal_auth_token).
    """
    return os.environ.get('OPERATOR_SESSION_ID', '')


def get_internal_auth_token() -> str:
    """Return the platform internal auth token.

    The Operator (listen mode) authenticates every internal request with
    `X-Internal-Auth`. The token is written by the Operator on first start to
    $G8E_SECRETS_DIR/internal_auth_token (default: $PROJECT_ROOT/.g8e/secrets/).
    Falls back to the G8E_INTERNAL_AUTH_TOKEN env var for test-runner contexts.
    """
    try:
        if INTERNAL_AUTH_TOKEN_PATH.exists():
            token = INTERNAL_AUTH_TOKEN_PATH.read_text().strip()
            if token:
                return token
    except OSError:
        pass
    return os.environ.get('G8E_INTERNAL_AUTH_TOKEN', '')


def get_auditor_hmac_key() -> str:
    """Return the Tribunal auditor HMAC-SHA256 signing key.

    The key is written by the Operator on first start to
    $G8E_SECRETS_DIR/auditor_hmac_key (default: $PROJECT_ROOT/.g8e/secrets/).
    """
    p = SECRETS_DIR / 'auditor_hmac_key'
    try:
        if p.exists():
            key = p.read_text().strip()
            if key:
                return key
    except OSError:
        pass
    return os.environ.get('AUDITOR_HMAC_KEY', '')


# =============================================================================
# Operator HTTP client — direct DB/KV access
# =============================================================================

def operator_request(method: str, path: str, body: Dict | None = None) -> Any:
    url = f'{OPERATOR_BASE_URL}{path}'
    data = json.dumps(body).encode() if body is not None else None
    headers = {'Content-Type': 'application/json'} if data is not None else {}

    session_token = get_auth_token()
    if session_token:
        headers['X-Operator-Session-Id'] = session_token

    req = urllib.request.Request(url, data=data, headers=headers, method=method)
    ctx = _create_ssl_context()
    try:
        with urllib.request.urlopen(req, context=ctx) as resp:
            text = resp.read().decode()
            return json.loads(text) if text else None
    except urllib.error.HTTPError as e:
        body_text = e.read().decode()
        if e.code == 404:
            return None
        try:
            err = json.loads(body_text).get('error', body_text)
        except Exception:
            err = body_text
        raise RuntimeError(f'HTTP {e.code} {method} {path}: {err}')
    except urllib.error.URLError as e:
        raise RuntimeError(
            f'Cannot reach the Operator listen-mode HTTP API at {OPERATOR_BASE_URL}. '
            f'Is the platform running? (./g8e platform start)\n  {e.reason}'
        )


def query_collection(collection: str, limit: int = 0) -> List[Dict]:
    body: Dict = {}
    if limit > 0:
        body['limit'] = limit
    result = operator_request('POST', f'/db/{urllib.parse.quote(collection, safe="")}/_query', body)
    return result if isinstance(result, list) else []


def get_document(collection: str, doc_id: str) -> Dict | None:
    return operator_request('GET', f'/db/{urllib.parse.quote(collection, safe="")}/{urllib.parse.quote(doc_id, safe="")}')


def delete_document(collection: str, doc_id: str) -> None:
    operator_request('DELETE', f'/db/{urllib.parse.quote(collection, safe="")}/{urllib.parse.quote(doc_id, safe="")}')


def kv_keys(pattern: str = '*') -> List[str]:
    result = operator_request('POST', '/kv/_keys', {'pattern': pattern})
    return result.get('keys', []) if isinstance(result, dict) else []


def kv_get(key: str) -> str | None:
    result = operator_request('GET', f'/kv/{urllib.parse.quote(key, safe="")}')
    return result.get('value') if isinstance(result, dict) else None


def kv_delete_pattern(pattern: str) -> int:
    result = operator_request('POST', '/kv/_delete_pattern', {'pattern': pattern})
    return result.get('deleted', 0) if isinstance(result, dict) else 0


# =============================================================================
# Operator API client — for resource management (users, operators, etc.)
# =============================================================================

def resolve_user_id(user_id: str | None, email: str | None) -> str | None:
    """Resolve user ID from email using the Operator's public API."""
    if user_id:
        return user_id
    if not email:
        return None
    # Query users collection by email using DocFilter format
    result = operator_request('POST', '/db/users/_query', {
        'filters': [{'field': 'email', 'op': '==', 'value': json.dumps(email)}]
    })
    if not isinstance(result, list) or len(result) == 0:
        print(f"\nUser not found with email: {email}")
        return None
    return result[0].get('id')


# =============================================================================
# Display helpers
# =============================================================================

def print_table(rows: List[Dict]) -> None:
    if not rows:
        return
    keys = list(rows[0].keys())
    term_width = shutil.get_terminal_size((200, 40)).columns

    widths: Dict[str, int] = {k: len(k) for k in keys}
    for row in rows:
        for k in keys:
            val = row.get(k)
            widths[k] = max(widths[k], len(str(val) if val is not None else ''))

    total = sum(widths.values()) + 3 * (len(keys) - 1) + 2
    if total > term_width:
        overflow = total - term_width
        shrinkable = sorted(keys, key=lambda k: -widths[k])
        for k in shrinkable:
            trim = min(overflow, widths[k] - max(len(k), 8))
            if trim > 0:
                widths[k] -= trim
                overflow -= trim
            if overflow <= 0:
                break

    sep = '  '
    header = sep.join(f'{k.upper()[:widths[k]]:<{widths[k]}}' for k in keys)
    divider = sep.join('-' * widths[k] for k in keys)
    print(f'  {header}')
    print(f'  {divider}')
    for row in rows:
        parts = []
        for k in keys:
            val = row.get(k)
            s = str(val) if val is not None else ''
            if len(s) > widths[k]:
                s = s[:widths[k] - 1] + '+'
            parts.append(f'{s:<{widths[k]}}')
        print(f'  {sep.join(parts)}')


def print_banner(script_name: str, args_str: str) -> None:
    print('')
    print('\u2501' * 52)
    print(f'  {script_name} {args_str}')
    print('\u2501' * 52)
    print('')
