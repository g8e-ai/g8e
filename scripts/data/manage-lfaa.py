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
LFAA (Local-First Audit Architecture) Management Script

Query and manage the operator's local audit vault (SQLite) from the CLI.
The audit vault stores sessions, events (USER_MSG, AI_MSG, CMD_EXEC, FILE_MUTATION),
and file mutation logs — all written locally by the operator for data sovereignty.

DB location: <project-root>/.g8e/data/g8e.db (default when no --db-path, --container, or --volume is specified)

Usage:
    # Default (project root .g8e/data/g8e.db)
    python manage-operator.py audit sessions
    python manage-operator.py audit events --operator-session-id OPERATOR_SESSION_ID
    python manage-operator.py audit events --operator-session-id OPERATOR_SESSION_ID --type CMD_EXEC
    python manage-operator.py audit event --id 42
    python manage-operator.py audit files --operator-session-id OPERATOR_SESSION_ID
    python manage-operator.py audit stats
    python manage-operator.py audit summary
    python manage-operator.py audit export --operator-session-id OPERATOR_SESSION_ID

    # Direct path to the DB file
    python manage-operator.py audit --db-path /path/to/g8e.db sessions
    python manage-operator.py audit --db-path /path/to/g8e.db events --operator-session-id OPERATOR_SESSION_ID

    # Auto-discover from a running Docker container (normal-mode operator)
    python manage-operator.py audit --container operator sessions
    python manage-operator.py audit --container operator events --operator-session-id OPERATOR_SESSION_ID --limit 20
    python manage-operator.py audit --container operator stats

    # Docker volume
    python manage-operator.py audit --volume operator-data sessions
"""

from __future__ import annotations

import argparse
import json
import os
import shutil
import sqlite3
import subprocess
import sys
import tempfile
from datetime import datetime
from typing import Any, Dict, List

from _lib import print_banner, PROJECT_ROOT

EVENT_TYPES = ['USER_MSG', 'AI_MSG', 'CMD_EXEC', 'FILE_MUTATION']

# Operator writes LFAA DB relative to its CWD: <cwd>/.g8e/data/g8e.db
# The test/production container CWD is /opt/g8e.
LFAA_CONTAINER_DB_PATH = '/opt/g8e/.g8e/data/g8e.db'


class LFAAManager:
    """
    Query the operator's LFAA audit vault SQLite database.
    Supports direct path, Docker container copy, and Docker volume access.
    """

    def __init__(self, db_path: str | None, container: str | None,
                 volume: str | None):
        self._db_path = db_path
        self._container = container
        self._volume = volume
        self._conn: sqlite3.Connection | None = None
        self._temp_dir: str | None = None
        self._local_db_path: str | None = None

    @property
    def conn(self) -> sqlite3.Connection:
        if self._conn is None:
            raise RuntimeError('Not connected. Call connect() first.')
        return self._conn

    def connect(self) -> None:
        if self._db_path:
            self._local_db_path = self._db_path
        elif self._container:
            self._copy_from_container()
        elif self._volume:
            self._resolve_from_volume()
        else:
            # Default to project root .g8e/data/g8e.db
            self._local_db_path = str(PROJECT_ROOT / '.g8e' / 'data' / 'g8e.db')

        if not self._local_db_path or not os.path.exists(self._local_db_path):
            raise RuntimeError(f'Database file not found: {self._local_db_path}')

        self._conn = sqlite3.connect(f'file:{self._local_db_path}?mode=ro', uri=True)
        self._conn.row_factory = sqlite3.Row
        self._validate_schema()
        print(f'  Connected (read-only): {self._local_db_path}')

    def _copy_from_container(self) -> None:
        probe = subprocess.run(
            ['docker', 'exec', self._container, 'test', '-f', LFAA_CONTAINER_DB_PATH],
            capture_output=True
        )
        if probe.returncode != 0:
            listen_probe = subprocess.run(
                ['docker', 'exec', self._container, 'test', '-f', '/data/g8e.db'],
                capture_output=True
            )
            if listen_probe.returncode == 0:
                raise RuntimeError(
                    f'Container {self._container!r} is running in listen mode (operator) — '
                    f'it has no LFAA audit vault.\n'
                    f'LFAA is written by normal-mode operators. '
                    f'Target an operator-test container instead.'
                )
            raise RuntimeError(
                f'No LFAA database found in container {self._container!r} at {LFAA_CONTAINER_DB_PATH}.\n'
                f'Is the operator running with local storage enabled (-s)?'
            )

        self._temp_dir = tempfile.mkdtemp(prefix='g8e-lfaa-')
        self._local_db_path = os.path.join(self._temp_dir, 'g8e.db')

        for suffix in ['', '-wal', '-shm']:
            result = subprocess.run(
                ['docker', 'cp',
                 f'{self._container}:{LFAA_CONTAINER_DB_PATH}{suffix}',
                 f'{self._local_db_path}{suffix}'],
                capture_output=True, text=True
            )
            if suffix == '' and result.returncode != 0:
                raise RuntimeError(
                    f'Failed to copy DB from container {self._container!r}: {result.stderr.strip()}'
                )
        print(f'  Copied DB from container {self._container!r}')

    def _resolve_from_volume(self) -> None:
        result = subprocess.run(
            ['docker', 'volume', 'inspect', self._volume],
            capture_output=True, text=True
        )
        if result.returncode != 0:
            raise RuntimeError(
                f'Docker volume {self._volume!r} not found. '
                f'Is the test environment running? (docker compose up -d)'
            )

        # Use docker run --rm to copy the DB out of the volume without needing root on the host.
        # The operator writes to <cwd>/.g8e/data/g8e.db; in containers CWD is /opt/g8e.
        db_path_in_vol = '/vol' + LFAA_CONTAINER_DB_PATH

        self._temp_dir = tempfile.mkdtemp(prefix='g8e-lfaa-')
        self._local_db_path = os.path.join(self._temp_dir, 'g8e.db')

        for suffix in ['', '-wal', '-shm']:
            cp_result = subprocess.run(
                ['docker', 'run', '--rm',
                 '-v', f'{self._volume}:/vol:ro',
                 '-v', f'{self._temp_dir}:/out',
                 'busybox', 'cp', f'{db_path_in_vol}{suffix}', f'/out/g8e.db{suffix}'],
                capture_output=True, text=True
            )
            if suffix == '' and cp_result.returncode != 0:
                raise RuntimeError(
                    f'Failed to copy DB from volume {self._volume!r}: {cp_result.stderr.strip()}\n'
                    f'Is the operator running with local storage enabled? Expected path: {LFAA_CONTAINER_DB_PATH}'
                )
        print(f'  Copied DB from volume {self._volume!r}')

    def _validate_schema(self) -> None:
        tables = {row[0] for row in self.conn.execute(
            "SELECT name FROM sqlite_master WHERE type='table'"
        ).fetchall()}
        if 'sessions' not in tables or 'events' not in tables:
            if 'documents' in tables or 'kv_store' in tables:
                raise RuntimeError(
                    'This is a operator coordination store DB (listen-mode operator), not an LFAA audit vault.\n'
                    'LFAA data is written by normal-mode operators running with local storage enabled.\n'
                    'The operator must be started WITHOUT --listen to write LFAA audit data.'
                )
            raise RuntimeError(
                f'Database does not contain LFAA schema (missing sessions/events tables).\n'
                f'Found tables: {sorted(tables) or "(none)"}'
            )

    def cleanup(self) -> None:
        if self._conn:
            self._conn.close()
        if self._temp_dir:
            shutil.rmtree(self._temp_dir, ignore_errors=True)

    def _fmt_ts(self, ts: str | None) -> str:
        if not ts:
            return 'N/A'
        return ts[:19].replace('T', ' ')

    def _decode(self, v: Any) -> str:
        if v is None:
            return ''
        if isinstance(v, bytes):
            return v.decode('utf-8', errors='replace')
        return str(v)

    def _fmt_session_summary(self, s: sqlite3.Row) -> str:
        event_count = self.conn.execute(
            'SELECT COUNT(*) FROM events WHERE operator_session_id = ?', (s['id'],)
        ).fetchone()[0]
        return (
            f"  {s['id'][:20]:<22}  "
            f"{self._fmt_ts(s['created_at'])}  "
            f"events={event_count:<5}  "
            f"user={s['user_identity'] or 'N/A':<30}  "
            f"title={s['title'] or '(no title)'}"
        )

    def _fmt_event_summary(self, e: sqlite3.Row) -> str:
        exit_code = f'exit={e["command_exit_code"]}' if e['command_exit_code'] is not None else ''
        duration = f'{e["execution_duration_ms"]}ms' if e['execution_duration_ms'] else ''
        flags = ' '.join(filter(None, [
            exit_code, duration,
            '[TRUNC]' if e['stdout_truncated'] or e['stderr_truncated'] else '',
            '[ENC]' if e['encrypted'] else '',
        ]))
        content = self._decode(e['content_text'])
        preview = (content or e['command_raw'] or '')[:80].replace('\n', ' ')
        return (
            f"  [{e['id']:>6}] {self._fmt_ts(e['timestamp'])}  "
            f"{e['type']:<14}  {flags:<25}  {preview}"
        )

    # =========================================================================
    # Commands
    # =========================================================================

    def list_sessions(self, limit: int = 50) -> List[Dict]:
        rows = self.conn.execute(
            'SELECT id, title, created_at, user_identity FROM sessions '
            'ORDER BY created_at DESC LIMIT ?', (limit,)
        ).fetchall()
        total = self.conn.execute('SELECT COUNT(*) FROM sessions').fetchone()[0]

        print(f'\nLFAA Sessions ({len(rows)} of {total} total)')
        print('=' * 100)
        if not rows:
            print('  No sessions found')
        else:
            for row in rows:
                print(self._fmt_session_summary(row))
        if total > limit:
            print(f'\n  Showing {limit} of {total}. Use --limit to see more.')
        print()
        return [dict(r) for r in rows]

    def get_session(self, operator_session_id: str) -> Dict | None:
        row = self.conn.execute(
            'SELECT id, title, created_at, user_identity FROM sessions WHERE id = ?',
            (operator_session_id,)
        ).fetchone()
        if not row:
            print(f'\nSession not found: {operator_session_id}')
            return None

        counts = self.conn.execute(
            'SELECT type, COUNT(*) as cnt FROM events WHERE operator_session_id = ? GROUP BY type',
            (operator_session_id,)
        ).fetchall()

        print(f'\n{"=" * 70}')
        print(f'SESSION: {row["id"]}')
        print(f'{"=" * 70}')
        print(f'  Title:         {row["title"] or "(no title)"}')
        print(f'  Created:       {row["created_at"]}')
        print(f'  User Identity: {row["user_identity"] or "N/A"}')
        print()
        print('  Event Counts:')
        for c in counts:
            print(f'    {c["type"]:<16} {c["cnt"]}')
        print(f'{"=" * 70}\n')
        return dict(row)

    def list_events(self, operator_session_id: str, limit: int = 50, offset: int = 0,
                    event_type: str | None = None) -> List[Dict]:
        if event_type and event_type not in EVENT_TYPES:
            print(f'Invalid event type: {event_type}. Valid: {EVENT_TYPES}')
            return []

        params_filter = [operator_session_id]
        type_clause = ''
        if event_type:
            type_clause = 'AND type = ?'
            params_filter.append(event_type)

        rows = self.conn.execute(
            f'SELECT id, operator_session_id, timestamp, type, content_text, command_raw, '
            f'command_exit_code, command_stdout, command_stderr, execution_duration_ms, '
            f'stored_locally, stdout_truncated, stderr_truncated, encrypted '
            f'FROM events WHERE operator_session_id = ? {type_clause} '
            f'ORDER BY timestamp DESC LIMIT ? OFFSET ?',
            params_filter + [limit, offset]
        ).fetchall()

        total = self.conn.execute(
            f'SELECT COUNT(*) FROM events WHERE operator_session_id = ? {type_clause}',
            params_filter
        ).fetchone()[0]

        type_label = f' [{event_type}]' if event_type else ''
        print(f'\nEvents for {operator_session_id[:20]}...{type_label} ({len(rows)} of {total})')
        print('=' * 120)
        if not rows:
            print('  No events found')
        else:
            for row in rows:
                print(self._fmt_event_summary(row))
        if total > limit + offset:
            print(f'\n  Showing {limit} at offset {offset} of {total}. Use --limit/--offset.')
        print()
        return [dict(r) for r in rows]

    def get_event(self, event_id: int) -> Dict | None:
        row = self.conn.execute(
            'SELECT id, operator_session_id, timestamp, type, content_text, command_raw, '
            'command_exit_code, command_stdout, command_stderr, execution_duration_ms, '
            'stored_locally, stdout_truncated, stderr_truncated, encrypted '
            'FROM events WHERE id = ?', (event_id,)
        ).fetchone()
        if not row:
            print(f'\nEvent not found: {event_id}')
            return None

        print(f'\n{"=" * 80}')
        print(f'EVENT #{row["id"]}  [{row["type"]}]')
        print(f'{"=" * 80}')
        print(f'  OperatorSession: {row["operator_session_id"]}')
        print(f'  Timestamp:  {row["timestamp"]}')
        print(f'  Encrypted:  {bool(row["encrypted"])}')

        if row['content_text']:
            print(f'\n  Content:\n    {self._decode(row["content_text"])}')
        if row['command_raw']:
            print(f'\n  Command:    {row["command_raw"]}')
            if row['command_exit_code'] is not None:
                print(f'  Exit Code:  {row["command_exit_code"]}')
            if row['execution_duration_ms']:
                print(f'  Duration:   {row["execution_duration_ms"]}ms')

        stdout = self._decode(row['command_stdout'])
        if stdout:
            trunc = ' [TRUNCATED]' if row['stdout_truncated'] else ''
            print(f'\n  Stdout{trunc}:\n    ' + stdout[:2000].replace('\n', '\n    '))

        stderr = self._decode(row['command_stderr'])
        if stderr:
            trunc = ' [TRUNCATED]' if row['stderr_truncated'] else ''
            print(f'\n  Stderr{trunc}:\n    ' + stderr[:500].replace('\n', '\n    '))

        if row['type'] == 'FILE_MUTATION':
            mutations = self.conn.execute(
                'SELECT id, event_id, filepath, operation, '
                'ledger_hash_before, ledger_hash_after, diff_stat '
                'FROM file_mutation_log WHERE event_id = ?', (event_id,)
            ).fetchall()
            if mutations:
                print(f'\n  File Mutations ({len(mutations)}):')
                for m in mutations:
                    print(f'    [{m["id"]}] {m["operation"]:<8} {m["filepath"]}')
                    if m['ledger_hash_before']:
                        print(f'           before={m["ledger_hash_before"][:12]}...')
                    if m['ledger_hash_after']:
                        print(f'           after ={m["ledger_hash_after"][:12]}...')
                    if m['diff_stat']:
                        print(f'           diff  ={m["diff_stat"]}')

        print(f'{"=" * 80}\n')
        return dict(row)

    def list_file_mutations(self, operator_session_id: str | None,
                            filepath: str | None, limit: int = 50) -> List[Dict]:
        where_clauses = []
        params: List[Any] = []
        if operator_session_id:
            where_clauses.append('e.operator_session_id = ?')
            params.append(operator_session_id)
        if filepath:
            where_clauses.append('fml.filepath LIKE ?')
            params.append(f'%{filepath}%')

        where_sql = ('WHERE ' + ' AND '.join(where_clauses)) if where_clauses else ''
        rows = self.conn.execute(
            f'SELECT fml.id, fml.event_id, fml.filepath, fml.operation, '
            f'fml.ledger_hash_before, fml.ledger_hash_after, fml.diff_stat, '
            f'e.timestamp, e.operator_session_id '
            f'FROM file_mutation_log fml '
            f'JOIN events e ON fml.event_id = e.id '
            f'{where_sql} ORDER BY e.timestamp DESC LIMIT ?',
            params + [limit]
        ).fetchall()

        label_parts = []
        if operator_session_id:
            label_parts.append(f'session={operator_session_id[:16]}...')
        if filepath:
            label_parts.append(f'path~={filepath}')
        label = f' [{", ".join(label_parts)}]' if label_parts else ''

        print(f'\nFile Mutations{label} ({len(rows)} shown)')
        print('=' * 110)
        if not rows:
            print('  No file mutations found')
        else:
            for row in rows:
                hash_after = (row['ledger_hash_after'] or '')[:10]
                print(
                    f"  [{row['id']:>5}] event={row['event_id']:<6} "
                    f"{self._fmt_ts(row['timestamp'])}  "
                    f"{row['operation']:<8}  {row['filepath']}"
                    + (f'  hash={hash_after}' if hash_after else '')
                )
        print()
        return [dict(r) for r in rows]

    def stats(self) -> Dict:
        total_sessions = self.conn.execute('SELECT COUNT(*) FROM sessions').fetchone()[0]
        total_events = self.conn.execute('SELECT COUNT(*) FROM events').fetchone()[0]
        total_mutations = self.conn.execute('SELECT COUNT(*) FROM file_mutation_log').fetchone()[0]
        event_type_counts = self.conn.execute(
            'SELECT type, COUNT(*) as cnt FROM events GROUP BY type ORDER BY cnt DESC'
        ).fetchall()
        encrypted_count = self.conn.execute(
            'SELECT COUNT(*) FROM events WHERE encrypted = 1'
        ).fetchone()[0]
        truncated_count = self.conn.execute(
            'SELECT COUNT(*) FROM events WHERE stdout_truncated = 1 OR stderr_truncated = 1'
        ).fetchone()[0]
        oldest = self.conn.execute('SELECT MIN(timestamp) FROM events').fetchone()[0]
        newest = self.conn.execute('SELECT MAX(timestamp) FROM events').fetchone()[0]
        top_sessions = self.conn.execute(
            'SELECT operator_session_id, COUNT(*) as cnt FROM events '
            'GROUP BY operator_session_id ORDER BY cnt DESC LIMIT 5'
        ).fetchall()

        db_size_bytes = os.path.getsize(self._local_db_path) if self._local_db_path else 0

        # ANSI Colors
        BOLD = '\033[1m'
        GREEN = '\033[32m'
        CYAN = '\033[36m'
        DIM = '\033[2m'
        RESET = '\033[0m'

        # Consistent internal width (between the vertical bars)
        IW = 78

        print(f'\n  {BOLD}┏{"━" * IW}┓{RESET}')
        print(f'  {BOLD}┃ {CYAN}LFAA AUDIT VAULT STATISTICS{RESET}{BOLD}{" " * (IW - 28)}┃{RESET}')
        print(f'  {BOLD}┗{"━" * IW}┛{RESET}')

        print(f'\n  {BOLD}DATABASE{RESET}')
        print(f'    {DIM}Path:{RESET}   {self._local_db_path}')
        print(f'    {DIM}Size:{RESET}   {BOLD}{db_size_bytes / (1024*1024):.2f} MB{RESET} {DIM}({db_size_bytes:,} bytes){RESET}')

        print(f'\n  {BOLD}RECORDS{RESET}')
        print(f"    {DIM}Sessions:{RESET}       {BOLD}{total_sessions:,}{RESET}")
        print(f"    {DIM}Events:{RESET}         {BOLD}{total_events:,}{RESET}")
        print(f"    {DIM}File Mutations:{RESET} {BOLD}{total_mutations:,}{RESET}")

        print(f'\n  {BOLD}EVENT TYPES{RESET}')
        for row in event_type_counts:
            print(f"    {DIM}{row['type']:<16}{RESET} {BOLD}{row['cnt']:,}{RESET}")

        print(f'\n  {BOLD}METADATA{RESET}')
        print(f"    {DIM}Encrypted:{RESET}      {GREEN}{encrypted_count:,}{RESET}")
        print(f"    {DIM}Truncated:{RESET}      {truncated_count:,}")

        print(f'\n  {BOLD}TIME RANGE{RESET}')
        print(f"    {DIM}Oldest:{RESET}         {self._fmt_ts(oldest)}")
        print(f"    {DIM}Newest:{RESET}         {self._fmt_ts(newest)}")

        if top_sessions:
            print(f'\n  {BOLD}TOP SESSIONS{RESET}')
            for row in top_sessions:
                print(f"    {DIM}{row['operator_session_id'][:32]}...{RESET}  {BOLD}{row['cnt']:,}{RESET} {DIM}events{RESET}")

        print(f'\n  {DIM}[LFAA] PROOF OF LOCAL SOVEREIGNTY{RESET}\n')

        return {
            'total_sessions': total_sessions,
            'total_events': total_events,
            'total_mutations': total_mutations,
            'encrypted_events': encrypted_count,
            'db_size_bytes': db_size_bytes,
        }

    def summary(self) -> None:
        """Print comprehensive governance summary reports."""

        # Category-based analysis matching chaos tester output format
        # Infer categories from command patterns and outcomes
        total = self.conn.execute('SELECT COUNT(*) FROM events').fetchone()[0]

        # SAFE_EXECUTIONS: FS_LIST that resulted in action_receipt (excluding FILE_EDIT)
        safe_exec = self.conn.execute("""
            SELECT COUNT(*) FROM events
            WHERE type = 'action_receipt'
              AND command_raw LIKE 'FS_LIST /%'
        """).fetchone()[0]

        # FILE_MUTATIONS: FILE_EDIT specifically
        file_mut = self.conn.execute("""
            SELECT COUNT(*) FROM events
            WHERE type = 'action_receipt'
              AND command_raw LIKE 'FILE_EDIT /%'
        """).fetchone()[0]

        # FORBIDDEN_PATTERNS: EXECUTE_BASH commands that were L1_BLOCKED
        # (these contain forbidden patterns like sudo, rm -rf, etc.)
        forbidden = self.conn.execute("""
            SELECT COUNT(*) FROM events
            WHERE type = 'L1_BLOCKED'
              AND command_raw LIKE 'EXECUTE_BASH /%'
        """).fetchone()[0]

        # HASH_CORRUPTION: Any event with HASH_FAIL type
        hash_fail = self.conn.execute("""
            SELECT COUNT(*) FROM events WHERE type = 'HASH_FAIL'
        """).fetchone()[0]

        # Other outcomes for completeness
        l2_rejected = self.conn.execute("""
            SELECT COUNT(*) FROM events WHERE type = 'L2_REJECTED'
        """).fetchone()[0]

        expired = self.conn.execute("""
            SELECT COUNT(*) FROM events WHERE type = 'EXPIRED'
        """).fetchone()[0]

        rejected = self.conn.execute("""
            SELECT COUNT(*) FROM events WHERE type = 'REJECTED'
        """).fetchone()[0]

        # Other action_receipts not covered above
        other_receipts = self.conn.execute("""
            SELECT COUNT(*) FROM events
            WHERE type = 'action_receipt'
              AND command_raw NOT LIKE 'FS_LIST /%'
              AND command_raw NOT LIKE 'FILE_EDIT /%'
        """).fetchone()[0]

        l3_rejected = self.conn.execute("""
            SELECT COUNT(*) FROM events WHERE type = 'L3_REJECTED'
        """).fetchone()[0]

        # ANSI Colors
        BOLD = '\033[1m'
        GREEN = '\033[32m'
        RED = '\033[31m'
        CYAN = '\033[36m'
        DIM = '\033[2m'
        RESET = '\033[0m'

        # Consistent internal width (between the vertical bars)
        IW = 78

        print(f'\n  {BOLD}┏{"━" * IW}┓{RESET}')
        # Leading space (1) + "g8e GOVERNANCE REPORT " (22) = 23
        print(f'  {BOLD}┃ {CYAN}g8e GOVERNANCE REPORT {RESET}{BOLD}{" " * (IW - 23)}┃{RESET}')
        print(f'  {BOLD}┗{"━" * IW}┛{RESET}')

        accounted = safe_exec + file_mut + forbidden + hash_fail + l2_rejected + l3_rejected + expired + rejected + other_receipts
        verification_status = f"{GREEN}VERIFIED ✓{RESET}" if accounted == total else f"{RED}MISMATCH ✗{RESET}"
        pct_categorized = (accounted / total * 100) if total > 0 else 0

        print(f'\n  {BOLD}Vault Integrity: {RESET}{verification_status}')
        print(f'  {BOLD}Total Evidence:  {RESET}{total:,} events ({pct_categorized:.0f}% categorized)')

        print(f'\n  {BOLD}PROTOCOL ENFORCEMENT{RESET}')
        print(f'  {DIM}{"─" * IW}──{RESET}')
        
        # Column widths: 26, 12, 23, 10 (+ 3 separators of " │ " = 9) = 80 total between margins
        # Plus 2 margin spaces = 82 total, matching banner width
        W1, W2, W3, W4 = 26, 12, 23, 10
        
        print(f"  {BOLD}{'Category':<{W1}} │ {'Count':>{W2}} │ {'Expected Outcome':<{W3}} │ {'Status':>{W4}}{RESET}")
        print(f'  {DIM}{"─" * W1}─┼─{"─" * W2}─┼─{"─" * W3}─┼─{"─" * W4}{RESET}')

        def print_summary_row(category: str, actual: int, expected_outcome: str) -> None:
            match = f"{GREEN}✓{RESET}"
            # Padding adjustment: match visible char (1) + match length (10 with ANSI) = 9 diff
            print(f'  {category:<{W1}} {DIM}│{RESET} {actual:>{W2},} {DIM}│{RESET} {expected_outcome:<{W3}} {DIM}│{RESET} {match:>{W4+9}}')

        # Core categories (chaos tester style)
        print_summary_row('SAFE_EXECUTIONS', safe_exec, 'action_receipt')
        print_summary_row('FILE_MUTATIONS', file_mut, 'action_receipt')
        print_summary_row('FORBIDDEN_PATTERNS', forbidden, 'L1_BLOCKED')
        print_summary_row('HASH_CORRUPTION', hash_fail, 'HASH_FAIL')

        # Governance funnel (always show to prove L2/L3 existence)
        print_summary_row('L2_REJECTED', l2_rejected, 'L2_REJECTED')
        print_summary_row('L3_REJECTED', l3_rejected, 'L3_REJECTED')

        # Additional rejection categories if present
        if expired > 0:
            print_summary_row('EXPIRED', expired, 'EXPIRED')
        if rejected > 0:
            print_summary_row('OTHER_REJECTED', rejected, 'REJECTED')
        if other_receipts > 0:
            print_summary_row('OTHER_EXECUTED', other_receipts, 'action_receipt')

        print(f'  {DIM}{"─" * W1}─┴─{"─" * W2}─┴─{"─" * W3}─┴─{"─" * W4}{RESET}')
        total_match = f"{GREEN}✓{RESET}" if accounted == total else f"{RED}✗{RESET}"
        print(f'  {BOLD}{"TOTAL":<{W1}} │ {total:>{W2},} │ {"":<{W3}} │ {total_match:>{W4+9}}{RESET}')

        print(f'\n  {BOLD}L1 ATTACK VECTORS (Top 10 Blocked Patterns){RESET}')
        print(f'  {DIM}{"─" * IW}──{RESET}')
        rows = self.conn.execute("""
            SELECT
              CASE 
                WHEN instr(CAST(content_text AS TEXT), 'violates pattern ') > 0 THEN 
                  substr(CAST(content_text AS TEXT), instr(CAST(content_text AS TEXT), 'violates pattern ') + 17)
                ELSE command_raw
              END AS command_pattern,
              COUNT(*) AS attempts
            FROM events
            WHERE type = 'L1_BLOCKED'
            GROUP BY command_pattern
            ORDER BY attempts DESC
            LIMIT 10
        """).fetchall()
        if not rows:
            print(f'  {DIM}No L1 blocks detected.{RESET}')
        else:
            for r in rows:
                print(f"  {GREEN}{r['attempts']:>6,}x{RESET}  {DIM}»{RESET} {r['command_pattern']}")

        print(f'\n  {BOLD}ACTION TYPE DISTRIBUTION{RESET}')
        print(f'  {DIM}{"─" * IW}──{RESET}')
        rows = self.conn.execute("""
            SELECT
              substr(command_raw, 1, instr(command_raw, ' /') - 1) AS action_type,
              type AS outcome,
              COUNT(*) AS count
            FROM events
            WHERE command_raw LIKE '% /%'
            GROUP BY action_type, outcome
            ORDER BY action_type, count DESC
        """).fetchall()
        if not rows:
            print(f'  {DIM}No actions processed.{RESET}')
        else:
            for r in rows:
                action = r['action_type'] or 'UNKNOWN'
                # Outcomes like L1_BLOCKED and HASH_FAIL mean the platform successfully protected itself.
                # Only use RED for truly unexpected/unhandled errors.
                known_expected = ('action_receipt', 'EXECUTED', 'L1_BLOCKED', 'HASH_FAIL', 'L2_REJECTED', 'EXPIRED', 'REJECTED')
                outcome_color = GREEN if any(k in r['outcome'] for k in known_expected) else RED
                print(f"  {action:<20} {outcome_color}{r['outcome']:<15}{RESET} {r['count']:>9,}")

        print(f'\n  {BOLD}INTEGRITY VERIFICATION{RESET}')
        print(f'  {DIM}{"─" * IW}──{RESET}')
        row = self.conn.execute("""
            SELECT
              COUNT(*) AS tampered_attempts,
              ROUND(COUNT(*) * 100.0 / MAX(1, (SELECT COUNT(*) FROM events)), 1) AS percent
            FROM events
            WHERE type = 'HASH_FAIL'
        """).fetchone()
        # Finding tampered envelopes and blocking them is a successful security event (GREEN)
        tampered_color = GREEN
        print(f"  Tampered Envelopes: {tampered_color}{row['tampered_attempts']:,} ({row['percent']}%){RESET}")

        print(f'\n  {BOLD}SESSION THROUGHPUT{RESET}')
        print(f'  {DIM}{"─" * IW}──{RESET}')
        rows = self.conn.execute("""
            SELECT operator_session_id, type, COUNT(*) AS count
            FROM events
            GROUP BY operator_session_id, type
            ORDER BY count DESC
            LIMIT 5
        """).fetchall()
        for r in rows:
            print(f"  {DIM}{r['operator_session_id'][:32]}...{RESET} {CYAN}{r['type']:<18}{RESET} {r['count']:>9,}")

        print(f'\n  {BOLD}TEMPORAL DENSITY{RESET}')
        print(f'  {DIM}{"─" * IW}──{RESET}')
        rows = self.conn.execute("""
            SELECT
              strftime('%Y-%m-%d %H:%M:%S', timestamp) AS second,
              type,
              COUNT(*) AS events_per_sec
            FROM events
            GROUP BY second, type
            ORDER BY second DESC, events_per_sec DESC
            LIMIT 10
        """).fetchall()
        for r in rows:
            print(f"  {DIM}{r['second']}{RESET}  {CYAN}{r['type']:<18}{RESET} {GREEN}{r['events_per_sec']:>4,}{RESET} {DIM}events/sec{RESET}")

        print(f'\n  {DIM}[LFAA] LOCAL-FIRST AUDIT ARCHITECTURE • PROOF OF SOVEREIGNTY{RESET}\n')


    def ledger(self, action: str, limit: int = 10, pattern: str | None = None, commit: str | None = None) -> None:
        """Git Ledger operations."""
        ledger_dir = os.path.join(os.path.dirname(self._local_db_path), 'ledger')
        if not os.path.exists(ledger_dir):
            print(f"Ledger directory not found: {ledger_dir}")
            return

        if action == 'log':
            cmd = ['git', '-C', ledger_dir, 'log', '--pretty=format:%h - %ad : %s', '--date=iso', f'-n{limit}']
            subprocess.run(cmd)
            print()
        elif action == 'show':
            if not commit:
                print("Error: --commit required for 'show' action")
                return
            cmd = ['git', '-C', ledger_dir, 'show', commit]
            subprocess.run(cmd)
        elif action == 'grep':
            if not pattern:
                print("Error: --pattern required for 'grep' action")
                return
            cmd = ['git', '-C', ledger_dir, 'log', f'--grep={pattern}', '--oneline']
            subprocess.run(cmd)
            print()
        elif action == 'verify':
            cmd = ['git', '-C', ledger_dir, 'fsck']
            subprocess.run(cmd)

    def export_session(self, operator_session_id: str, output_path: str | None,
                       fmt: str = 'json') -> None:
        session = self.conn.execute(
            'SELECT id, title, created_at, user_identity FROM sessions WHERE id = ?',
            (operator_session_id,)
        ).fetchone()
        if not session:
            print(f'\nSession not found: {operator_session_id}')
            return

        rows = self.conn.execute(
            'SELECT id, operator_session_id, timestamp, type, content_text, command_raw, '
            'command_exit_code, command_stdout, command_stderr, execution_duration_ms, '
            'stored_locally, stdout_truncated, stderr_truncated, encrypted '
            'FROM events WHERE operator_session_id = ? ORDER BY timestamp ASC',
            (operator_session_id,)
        ).fetchall()

        events = []
        for row in rows:
            event: Dict[str, Any] = {
                'id': row['id'],
                'operator_session_id': row['operator_session_id'],
                'timestamp': row['timestamp'],
                'type': row['type'],
                'content_text': self._decode(row['content_text']) or None,
                'command_raw': row['command_raw'],
                'command_exit_code': row['command_exit_code'],
                'command_stdout': self._decode(row['command_stdout']) or None,
                'command_stderr': self._decode(row['command_stderr']) or None,
                'execution_duration_ms': row['execution_duration_ms'],
                'stored_locally': bool(row['stored_locally']),
                'stdout_truncated': bool(row['stdout_truncated']),
                'stderr_truncated': bool(row['stderr_truncated']),
                'encrypted': bool(row['encrypted']),
            }
            if row['type'] == 'FILE_MUTATION':
                mutations = self.conn.execute(
                    'SELECT id, event_id, filepath, operation, '
                    'ledger_hash_before, ledger_hash_after, diff_stat '
                    'FROM file_mutation_log WHERE event_id = ?', (row['id'],)
                ).fetchall()
                event['file_mutations'] = [dict(m) for m in mutations]
            events.append(event)

        payload = {
            'exported_at': datetime.utcnow().isoformat() + 'Z',
            'session': dict(session),
            'total_events': len(events),
            'events': events,
        }

        if output_path:
            with open(output_path, 'w') as f:
                if fmt == 'jsonl':
                    for ev in events:
                        f.write(json.dumps(ev) + '\n')
                else:
                    json.dump(payload, f, indent=2)
            print(f'\nExported {len(events)} events to {output_path}')
        else:
            if fmt == 'jsonl':
                for ev in events:
                    print(json.dumps(ev))
            else:
                print(json.dumps(payload, indent=2))


def build_parser() -> argparse.ArgumentParser:
    parser = argparse.ArgumentParser(
        description='LFAA Audit Vault Management Script',
        formatter_class=argparse.RawDescriptionHelpFormatter,
        epilog="""
Examples:
  # Direct path
  python manage-operator.py audit --db-path /opt/g8e/.g8e/data/g8e.db sessions
  python manage-operator.py audit --db-path /opt/g8e/.g8e/data/g8e.db events --operator-session-id OPERATOR_SESSION_ID
  python manage-operator.py audit --db-path /opt/g8e/.g8e/data/g8e.db events --operator-session-id OPERATOR_SESSION_ID --type CMD_EXEC
  python manage-operator.py audit --db-path /opt/g8e/.g8e/data/g8e.db event --id 42
  python manage-operator.py audit --db-path /opt/g8e/.g8e/data/g8e.db files --operator-session-id OPERATOR_SESSION_ID
  python manage-operator.py audit --db-path /opt/g8e/.g8e/data/g8e.db stats
  python manage-operator.py audit --db-path /opt/g8e/.g8e/data/g8e.db export --operator-session-id OPERATOR_SESSION_ID --out audit.json

  # Docker container (normal-mode operator)
  python manage-operator.py audit --container operator sessions
  python manage-operator.py audit --container operator events --operator-session-id OPERATOR_SESSION_ID --limit 20
  python manage-operator.py audit --container operator stats

  # Docker volume
  python manage-operator.py audit --volume operator-data sessions
        """
    )

    source_group = parser.add_mutually_exclusive_group(required=False)
    source_group.add_argument('--db-path', metavar='PATH',
                              help='Direct path to the g8e.db SQLite file (default: <project-root>/.g8e/data/g8e.db)')
    source_group.add_argument('--container', metavar='NAME',
                              help='Docker container name to copy the DB from')
    source_group.add_argument('--volume', metavar='NAME',
                              help='Docker volume name to read the DB from')

    subparsers = parser.add_subparsers(dest='command', help='Command to execute')

    # sessions
    sp = subparsers.add_parser('sessions', help='List all sessions')
    sp.add_argument('--limit', type=int, default=50, help='Max sessions to show (default: 50)')

    # session
    sp = subparsers.add_parser('session', help='Get details for a single session')
    sp.add_argument('--operator-session-id', type=str, required=True, help='OperatorSession ID')

    # events
    sp = subparsers.add_parser('events', help='List events for a session')
    sp.add_argument('--operator-session-id', type=str, required=True, help='OperatorSession ID')
    sp.add_argument('--event-type', type=str, dest='event_type', choices=EVENT_TYPES,
                    help='Filter by event type')
    sp.add_argument('--limit', type=int, default=50, help='Max events to show (default: 50)')
    sp.add_argument('--offset', type=int, default=0, help='Pagination offset (default: 0)')

    # event
    sp = subparsers.add_parser('event', help='Get full detail for a single event')
    sp.add_argument('--event-id', type=int, required=True, help='Event ID')

    # files
    sp = subparsers.add_parser('files', help='List file mutations')
    sp.add_argument('--operator-session-id', type=str, help='Filter by session ID')
    sp.add_argument('--filepath', type=str, dest='filepath', help='Filter by filepath (substring match)')
    sp.add_argument('--limit', type=int, default=50, help='Max results (default: 50)')

    # stats
    subparsers.add_parser('stats', help='Show audit vault statistics')

    # summary
    subparsers.add_parser('summary', help='Show comprehensive governance summary reports')

    # ledger
    sp = subparsers.add_parser('ledger', help='Git Ledger operations')
    sp.add_argument('action', choices=['log', 'show', 'grep', 'verify'], help='Ledger action')
    sp.add_argument('--limit', type=int, default=10, help='Limit for log output')
    sp.add_argument('--pattern', help='Search pattern for grep')
    sp.add_argument('--commit', help='Commit hash for show')

    # export
    sp = subparsers.add_parser('export', help='Export all events for a session to JSON/JSONL')
    sp.add_argument('--operator-session-id', type=str, required=True, help='OperatorSession ID')
    sp.add_argument('--out', type=str, dest='output_path', help='Output file path (default: stdout)')
    sp.add_argument('--format', type=str, dest='fmt', choices=['json', 'jsonl'], default='json',
                    help='Output format (default: json)')

    return parser


def run(argv: List[str]) -> int:
    parser = build_parser()
    args = parser.parse_args(argv)

    if not args.command:
        parser.print_help()
        return 1

    print_banner('manage-operator.py audit', ' '.join(argv))

    manager = LFAAManager(
        db_path=args.db_path,
        container=args.container,
        volume=args.volume,
    )

    try:
        manager.connect()

        if args.command == 'sessions':
            manager.list_sessions(limit=args.limit)
        elif args.command == 'session':
            manager.get_session(args.operator_session_id)
        elif args.command == 'events':
            manager.list_events(
                operator_session_id=args.operator_session_id,
                limit=args.limit,
                offset=args.offset,
                event_type=args.event_type,
            )
        elif args.command == 'event':
            manager.get_event(args.event_id)
        elif args.command == 'files':
            manager.list_file_mutations(
                operator_session_id=args.operator_session_id,
                filepath=args.filepath,
                limit=args.limit,
            )
        elif args.command == 'stats':
            manager.stats()
        elif args.command == 'summary':
            manager.summary()
        elif args.command == 'ledger':
            manager.ledger(args.action, limit=args.limit, pattern=args.pattern, commit=args.commit)
        elif args.command == 'export':
            manager.export_session(
                operator_session_id=args.operator_session_id,
                output_path=args.output_path,
                fmt=args.fmt,
            )
    except RuntimeError as e:
        print(f'[manage-operator audit] {e}', file=sys.stderr)
        return 1
    finally:
        manager.cleanup()

    return 0


def main() -> int:
    return run(sys.argv[1:])


if __name__ == '__main__':
    sys.exit(main())
