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

Provides authentication, HTTP clients (g8es direct + g8ed internal API),
and terminal display helpers used across all resource scripts.
"""

import json
import os
import shutil
import ssl
import urllib.error
import urllib.parse
import urllib.request
from pathlib import Path
from typing import Any, Dict, List, Optional

PROJECT_ROOT = Path(__file__).parent.parent.parent.absolute()
_SHARED_CONSTANTS = PROJECT_ROOT / 'shared' / 'constants'

with open(_SHARED_CONSTANTS / 'collections.json') as _f:
    _COLLECTIONS_DATA = json.load(_f)

G8ES_BASE_URL = 'https://g8es:9000'
G8ED_BASE_URL = 'https://g8ed'
COLLECTIONS: List[str] = sorted(set(_COLLECTIONS_DATA['collections'].values()))
PRESERVE_COLLECTIONS = {'settings'}


CA_CERT_PATH = Path('/g8es/ca.crt')
INTERNAL_AUTH_TOKEN_PATHS = (
    Path('/g8es/internal_auth_token'),
    Path('/g8es/ssl/internal_auth_token'),
)


def _create_ssl_context() -> Optional[ssl.SSLContext]:
    """Create SSL context that trusts the platform CA."""
    ctx = ssl.create_default_context()
    if CA_CERT_PATH.exists():
        ctx.load_verify_locations(str(CA_CERT_PATH))
    else:
        # Fallback to internal volume path
        alt_path = Path('/g8es/ssl/ca.crt')
        if alt_path.exists():
            ctx.load_verify_locations(str(alt_path))
    return ctx


# =============================================================================
# Authentication
# =============================================================================

def get_auth_token() -> str:
    """Return the operator session token from env (set by `g8e login`).

    The wrapper gates `g8e data` on a valid operator session before invoking
    this script, so OPERATOR_SESSION_ID is expected to be present. It is
    forwarded to g8ed for operator-scoped API calls; g8es calls use the
    internal auth token instead (see get_internal_auth_token).
    """
    return os.environ.get('OPERATOR_SESSION_ID', '')


def get_internal_auth_token() -> str:
    """Return the platform internal auth token.

    g8es authenticates every request with `X-Internal-Auth`. Inside g8ep the
    token is mounted at /g8es/internal_auth_token (ro). Falls back to the
    G8E_INTERNAL_AUTH_TOKEN env var for test-runner contexts.
    """
    for p in INTERNAL_AUTH_TOKEN_PATHS:
        try:
            if p.exists():
                token = p.read_text().strip()
                if token:
                    return token
        except OSError:
            continue
    return os.environ.get('G8E_INTERNAL_AUTH_TOKEN', '')


# =============================================================================
# g8ed HTTP client — direct DB/KV access (same as g8ee's DBClient)
# =============================================================================

def g8es_request(method: str, path: str, body: Optional[Dict] = None) -> Any:
    url = f'{G8ES_BASE_URL}{path}'
    data = json.dumps(body).encode() if body is not None else None
    headers = {'Content-Type': 'application/json'} if data is not None else {}

    # g8es requires X-Internal-Auth on all non-health endpoints. The operator
    # session from `g8e login` gates the wrapper; inside the trusted g8ep
    # container we use the mounted internal auth token to reach g8es, the
    # same way g8ed and g8ee do.
    internal_token = get_internal_auth_token()
    if internal_token:
        headers['X-Internal-Auth'] = internal_token

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
            f'Cannot reach g8es at {G8ES_BASE_URL}. Is the platform running?\n  {e.reason}'
        )


def query_collection(collection: str, limit: int = 0) -> List[Dict]:
    body: Dict = {}
    if limit > 0:
        body['limit'] = limit
    result = g8es_request('POST', f'/db/{urllib.parse.quote(collection, safe="")}/_query', body)
    return result if isinstance(result, list) else []


def get_document(collection: str, doc_id: str) -> Optional[Dict]:
    return g8es_request('GET', f'/db/{urllib.parse.quote(collection, safe="")}/{urllib.parse.quote(doc_id, safe="")}')


def delete_document(collection: str, doc_id: str) -> None:
    g8es_request('DELETE', f'/db/{urllib.parse.quote(collection, safe="")}/{urllib.parse.quote(doc_id, safe="")}')


def kv_keys(pattern: str = '*') -> List[str]:
    result = g8es_request('POST', '/kv/_keys', {'pattern': pattern})
    return result.get('keys', []) if isinstance(result, dict) else []


def kv_get(key: str) -> Optional[str]:
    result = g8es_request('GET', f'/kv/{urllib.parse.quote(key, safe="")}')
    return result.get('value') if isinstance(result, dict) else None


def kv_delete_pattern(pattern: str) -> int:
    result = g8es_request('POST', '/kv/_delete_pattern', {'pattern': pattern})
    return result.get('deleted', 0) if isinstance(result, dict) else 0


# =============================================================================
# g8ed internal API client — for resource management (users, operators, etc.)
# =============================================================================

def g8ed_request(method: str, url: str, body: Optional[Dict] = None) -> Dict:
    data = json.dumps(body).encode() if body is not None else None
    headers = {
        'X-Operator-Session-Id': get_auth_token() or '',
        'Content-Type': 'application/json',
        'Accept': 'application/json',
    }
    # g8ed internal endpoints accept either an operator session OR the
    # platform internal auth token. Running inside g8ep (trusted container)
    # we forward the mounted internal token for bootstrap flows before
    # any user is authenticated.
    internal_token = get_internal_auth_token()
    if internal_token:
        headers['X-Internal-Auth'] = internal_token
    req = urllib.request.Request(url, data=data, headers=headers, method=method)
    ctx = _create_ssl_context()
    try:
        with urllib.request.urlopen(req, context=ctx) as resp:
            return json.loads(resp.read().decode())
    except urllib.error.HTTPError as e:
        body_bytes = e.read()
        try:
            err = json.loads(body_bytes.decode())
        except Exception:
            err = {'error': body_bytes.decode()}
        err['_status_code'] = e.code
        return err
    except urllib.error.URLError as e:
        raise RuntimeError(
            f"Could not reach g8ed at {G8ED_BASE_URL}. Is the platform running? "
            f"(./g8e platform start)\n  {e.reason}"
        )


def resolve_user_id(user_id: Optional[str], email: Optional[str]) -> Optional[str]:
    if user_id:
        return user_id
    if not email:
        return None
    result = g8ed_request('GET', f'{G8ED_BASE_URL}/api/internal/users/email/{urllib.parse.quote(email, safe="")}')
    # g8ed returns {user: {...}} on success and {error/...} on failure.
    user = result.get('user') if isinstance(result, dict) else None
    if not user:
        if result.get('_status_code') == 404:
            print(f"\nUser not found with email: {email}")
        else:
            raise RuntimeError(result.get('error', 'Failed to resolve user by email'))
        return None
    return user['id']


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
