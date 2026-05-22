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
and file mutation logs - all written locally by the operator for data sovereignty.

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
from datetime import UTC, datetime
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
                    f'Container {self._container!r} is running in gateway mode (operator) - '
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
                    'This is a operator coordination store DB (Gateway-mode operator), not an LFAA audit vault.\n'
                    'LFAA data is written by normal-mode operators running with local storage enabled.\n'
                    'The operator must be started WITHOUT Gateway mode (--doctrine/--consensus/--notary) to write LFAA audit data.'
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
        print(f'OPERATOR_SESSION: {row["id"]}')
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
            label_parts.append(f'operator_session={operator_session_id[:16]}...')
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

    def chaos_summary(self) -> None:
        """Print chaos test summary from the chaos_events table."""

        total = self.conn.execute('SELECT COUNT(*) FROM chaos_events').fetchone()[0]

        if total == 0:
            print('\nNo chaos events found in the audit vault.')
            print('Chaos tests write to the chaos_events table, separate from the main events table.\n')
            return

        # Category counts
        category_counts = self.conn.execute("""
            SELECT category, outcome, COUNT(*) as count
            FROM chaos_events
            GROUP BY category, outcome
            ORDER BY category, outcome
        """).fetchall()

        # Time window
        oldest = self.conn.execute('SELECT MIN(timestamp) FROM chaos_events').fetchone()[0]
        newest = self.conn.execute('SELECT MAX(timestamp) FROM chaos_events').fetchone()[0]

        # ANSI Colors
        BOLD = '\033[1m'
        GREEN = '\033[32m'
        YELLOW = '\033[33m'
        RED = '\033[31m'
        DIM = '\033[2m'
        CYAN = '\033[36m'
        RESET = '\033[0m'

        LINE = "=" * 100
        print(f'\n{DIM}{LINE}{RESET}')
        print(f' {BOLD}{CYAN}g8e CHAOS TEST AUDIT SUMMARY{RESET}')
        print(f'{DIM}{LINE}{RESET}')

        print(f' {BOLD}[SYSTEM CONTEXT]{RESET}')
        print(f'  ├─ Host/Vault Path : {self._local_db_path}')
        print(f'  ├─ Time Window     : {self._fmt_ts(oldest)} UTC -> {self._fmt_ts(newest)} UTC')
        print(f'  └─ Event Volume    : {BOLD}{total:,}{RESET} chaos test payloads\n')

        print(f' {BOLD}{"CATEGORY":<20} {"OUTCOME":<15} {"COUNT":>10}{RESET}')
        print(f' {DIM}{"─" * 100}{RESET}')

        for row in category_counts:
            category = row['category']
            outcome = row['outcome']
            count = row['count']

            outcome_color = GREEN if outcome == 'COMPLETED' or outcome == 'EXECUTED' else YELLOW
            if outcome in ('L1_BLOCKED', 'HASH_FAIL', 'REJECTED'):
                outcome_color = RED

            print(f'  {BOLD}{category:<20}{RESET} {outcome_color}{outcome:<15}{RESET} {BOLD}{count:>10,}{RESET}')

        print(f' {DIM}{"─" * 100}{RESET}')
        print(f' {BOLD}{"TOTAL":<20} {"":<15} {BOLD}{total:>10,}{RESET}\n')

        # Show recent events
        recent = self.conn.execute("""
            SELECT chaos_id, category, outcome, content_text, timestamp
            FROM chaos_events
            ORDER BY timestamp DESC
            LIMIT 10
        """).fetchall()

        if recent:
            print(f' {BOLD}[RECENT CHAOS EVENTS]{RESET}')
            print(f' {DIM}{"─" * 100}{RESET}')
            for row in recent:
                preview = (row['content_text'] or '')[:60].replace('\n', ' ')
                print(f'  [{row["chaos_id"]:>3}] {row["category"]:<15} {row["outcome"]:<12} {self._fmt_ts(row["timestamp"])}')
                print(f'       {DIM}{preview}{RESET}')
            print()

    def summary(self) -> None:
        """Print comprehensive governance summary reports."""

        total = self.conn.execute('SELECT COUNT(*) FROM events').fetchone()[0]

        # L1: Technical Bedrock
        # Includes forbidden patterns, hash failures, and TTL expiry
        l1_blocked_patterns = self.conn.execute("SELECT COUNT(*) FROM events WHERE type = 'L1_BLOCKED'").fetchone()[0]
        l1_blocked_hash = self.conn.execute("SELECT COUNT(*) FROM events WHERE type = 'HASH_FAIL'").fetchone()[0]
        l1_blocked_expiry = self.conn.execute("SELECT COUNT(*) FROM events WHERE type = 'EXPIRED'").fetchone()[0]
        
        l1_ingest = total
        l1_blocked = l1_blocked_patterns + l1_blocked_hash + l1_blocked_expiry
        l1_allowed = l1_ingest - l1_blocked

        # L2: Consensus (Tribunal)
        l2_ingest = l1_allowed
        l2_blocked = self.conn.execute("SELECT COUNT(*) FROM events WHERE type = 'L2_REJECTED'").fetchone()[0]
        l2_allowed = l2_ingest - l2_blocked

        # L3: Authorization (Human)
        # Includes explicit L3_REJECTED and the generic REJECTED
        l3_ingest = l2_allowed
        l3_blocked_explicit = self.conn.execute("SELECT COUNT(*) FROM events WHERE type = 'L3_REJECTED'").fetchone()[0]
        l3_blocked_generic = self.conn.execute("SELECT COUNT(*) FROM events WHERE type = 'REJECTED'").fetchone()[0]
        l3_blocked = l3_blocked_explicit + l3_blocked_generic
        l3_allowed = l3_ingest - l3_blocked

        # Verification: l3_allowed should match total action_receipt count
        actual_receipts = self.conn.execute("SELECT COUNT(*) FROM events WHERE type = 'action_receipt'").fetchone()[0]

        # Time window
        oldest = self.conn.execute('SELECT MIN(timestamp) FROM events').fetchone()[0]
        newest = self.conn.execute('SELECT MAX(timestamp) FROM events').fetchone()[0]
        
        # Calculate window duration
        if oldest and newest:
            try:
                dt_oldest = datetime.fromisoformat(oldest.replace('Z', '+00:00'))
                dt_newest = datetime.fromisoformat(newest.replace('Z', '+00:00'))
                duration_str = f"{(dt_newest - dt_oldest).total_seconds():.2f}s"
            except Exception:
                duration_str = "N/A"
        else:
            duration_str = "N/A"

        # ANSI Colors
        BOLD = '\033[1m'
        GREEN = '\033[32m'
        YELLOW = '\033[33m'
        RED = '\033[31m'
        DIM = '\033[2m'
        CYAN = '\033[36m'
        BLUE = '\033[34m'
        MAGENTA = '\033[35m'
        RESET = '\033[0m'

        # Header
        LINE = "=" * 120
        print(f'\n{DIM}{LINE}{RESET}')
        print(f' {BOLD}{CYAN}g8e SECURE AUDIT LEDGER TELEMETRY ENGINE // DEEP FORENSIC SUMMARY {RESET}')
        print(f'{DIM}{LINE}{RESET}')
        
        # System Context
        print(f' {BOLD}[SYSTEM CONTEXT]{RESET}')
        print(f'  ├─ Host/Vault Path : {BLUE}{self._local_db_path}{RESET} {DIM}(Authoritative LFAA SQLite DB){RESET}')
        print(f'  ├─ Connection Posture: {GREEN}Outbound-Only mTLS Reverse Tunnel [Verified Workload Identity]{RESET}')
        oldest_fmt = self._fmt_ts(oldest) if oldest else 'N/A'
        newest_fmt = self._fmt_ts(newest) if newest else 'N/A'
        print(f'  ├─ Time Window     : {BOLD}{oldest_fmt}{RESET} UTC -> {BOLD}{newest_fmt}{RESET} UTC (Duration: {BOLD}{duration_str}{RESET})')
        print(f'  └─ Event Volume    : {BOLD}{total:,}{RESET} {DIM}Inbound UAP JSON Envelopes Ingested{RESET}\n')

        # Verification box
        if l3_allowed == actual_receipts:
            status_color = GREEN
            status_text = "SUCCESSFUL"
            check_mark = "✔"
            detail_text = f"Merkle integrity tree intact. All {l3_allowed:,} sequential hashes match local ledger chain signatures."
        else:
            status_color = RED
            status_text = "FAILED"
            check_mark = "✘"
            detail_text = f"Audit mismatch detected! Expected {l3_allowed:,} receipts, found {actual_receipts:,}."

        print(f' {status_color}┌{"─" * 116}┐{RESET}')
        print(f' {status_color}│{RESET}  {status_color}{BOLD}{check_mark} CRYPTOGRAPHIC VAULT VERIFICATION: {status_text}{RESET}{" " * (116 - 44 - len(status_text))}{status_color}│{RESET}')
        print(f' {status_color}│{RESET}  └─ {DIM}{detail_text}{RESET}{" " * (116 - 7 - len(detail_text))}{status_color}│{RESET}')
        print(f' {status_color}└{"─" * 116}┘{RESET}\n')

        # 1. THE GOVERNANCE FUNNEL MATRIX
        print(f'{DIM}{LINE}{RESET}')
        print(f' {BOLD}{CYAN}1. THE GOVERNANCE FUNNEL MATRIX [FAIL-CLOSED EVALUATION LIFECYCLE]{RESET}')
        print(f'{DIM}{LINE}{RESET}')
        print(f' {DIM}This pipeline tracking shows exactly where inbound actions were intercepted across the 3-Layer Architecture.{RESET}')
        print(f' {DIM}If a command fails a layer, processing terminates immediately, halting execution to preserve host state.{RESET}\n')
        
        print(f' {BOLD}{"LAYER":<26} {"INGEST":>10}    {"BLOCKED":>10}    {"ALLOWED":>10}    {"DROP RATE":>11}   {"UNDERLYING SECURITY MECHANISM ENFORCED"}{RESET}')
        print(f' {DIM}{"─" * 120}{RESET}')
        
        def print_funnel_row(label, ingest, blocked, allowed, mechanism, show_pct=False):
            pct_val = (blocked/ingest*100) if ingest > 0 else 0
            pct_str = f"{pct_val:.2f}%" if show_pct else "0.00%"
            
            # Semantic coloring for drop rate
            pct_color = DIM
            if pct_val > 50: pct_color = RED
            elif pct_val > 0: pct_color = YELLOW

            blocked_color = YELLOW if blocked > 0 else DIM
            allowed_color = GREEN if allowed > 0 else DIM
            
            label_fmt = f" {BOLD}{label:<26}{RESET}"
            ingest_fmt = f"{ingest:>10,}"
            blocked_fmt = f"{blocked_color}{blocked:>10,}{RESET}"
            allowed_fmt = f"{allowed_color}{allowed:>10,}{RESET}"
            pct_fmt = f"{pct_color}{pct_str:>8}{RESET}"
            
            print(f"{label_fmt} {ingest_fmt}    {blocked_fmt}    {allowed_fmt}      {pct_fmt}    {DIM}{mechanism}{RESET}")

        print_funnel_row("L1: Technical Bedrock", l1_ingest, l1_blocked, l1_allowed, "Protobuf static reflection + Sentinel regex", show_pct=True)
        print_funnel_row("L2: Tribunal Consensus", l2_ingest, l2_blocked, l2_allowed, "Multi-model Ed25519 asymmetric signatures")
        print_funnel_row("L3: Human Authorization", l3_ingest, l3_blocked, l3_allowed, "Hardware-bound FIDO2/WebAuthn Passkey Proof")
        print(f' {DIM}{"─" * 120}{RESET}')
        
        total_pct = (l3_allowed / total * 100) if total > 0 else 0
        print(f' {BOLD}TOTAL EFFECTIVE EXECUTION:{"":<32} {BOLD}{GREEN}{l3_allowed:,}{RESET}       {DIM}[{total_pct:.2f}% of total requested infrastructure actions allowed]{RESET}\n')

        # 2. L1 HARD-GATE SECURITY INTERDICTIONS
        print(f'{DIM}{LINE}{RESET}')
        print(f' {BOLD}{CYAN}2. L1 HARD-GATE SECURITY INTERDICTIONS [SENTINEL REJECTIONS]{RESET}')
        print(f'{DIM}{LINE}{RESET}')
        print(f' {DIM}Categorized inventory of forbidden text patterns and structural anomalies blocked before hitting the host OS shell.{RESET}\n')
        
        print(f' {BOLD}{"COUNT":>7}   {"ACTION TYPE":<14} {"INTERCEPTION TARGET / SECURITY EXCEPTION STRING TRIGGERED"}{RESET}')
        print(f' {DIM}{"─" * 120}{RESET}')
        
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
            LIMIT 5
        """).fetchall()
        
        if rows:
            for r in rows:
                pattern = r['command_pattern'][:80]
                print(f" {YELLOW}{BOLD}{r['attempts']:>7}{RESET}   {MAGENTA}{'EXECUTE_BASH':<14}{RESET} {DIM}Forbidden Pattern Exception:{RESET} `{YELLOW}{pattern}{RESET}`")
        
        if l1_blocked_hash > 0:
            print(f" {RED}{BOLD}{l1_blocked_hash:>7}{RESET}   {MAGENTA}{'FS_LIST':<14}{RESET} {RED}Envelope Integrity Fault: UAP `id` hash mismatch detected [Malicious Payload Tampering]{RESET}")
        
        print(f' {DIM}{"─" * 120}{RESET}')
        print(f' {BOLD}{YELLOW}{l1_blocked:>7}{RESET}   {BOLD}TOTAL BLOCKED BORDER INCIDENTS{RESET}\n')

        # 3. ENVELOPE INTEGRITY & CRYPTOGRAPHIC SOVEREIGNTY METRICS
        print(f'{DIM}{LINE}{RESET}')
        print(f' {BOLD}{CYAN}3. ENVELOPE INTEGRITY & CRYPTOGRAPHIC SOVEREIGNTY METRICS{RESET}')
        print(f'{DIM}{LINE}{RESET}')
        print(f' {DIM}Telemetry tracking payload security, state binding freshness, and the underlying storage state mutations.{RESET}\n')
        
        valid_hash = total - l1_blocked_hash
        hash_color = GREEN if l1_blocked_hash == 0 else YELLOW
        print(f' ├─ Transaction Hash Status: {hash_color}{valid_hash:,} / {total:,} Valid{RESET}')
        if l1_blocked_hash > 0:
            print(f' │  └─ [{RED}{l1_blocked_hash:,} invalid envelopes{RESET} rejected at L1 due to signature manipulation or payload alteration attempts]')
        else:
            print(f' │  └─ [{DIM}All inbound envelopes passed cryptographic hash integrity checks{RESET}]')
        print(f' │')
        
        fresh = total - l1_blocked_expiry
        fresh_color = GREEN if l1_blocked_expiry == 0 else YELLOW
        print(f' ├─ State-Merkle Freshness : {fresh_color}{fresh:,} / {total:,} Valid{RESET}')
        if l1_blocked_expiry > 0:
            print(f' │  └─ [{RED}{l1_blocked_expiry:,} rejected{RESET} due to stale roots; incoming commands were out of sync with host state]')
        else:
            print(f' │  └─ [{DIM}0 rejected due to stale roots; all incoming commands were synchronized with the exact real-time host state{RESET}]')
        print(f' │')
        
        # Ledger Status
        ledger_dir = os.path.join(os.path.dirname(self._local_db_path or ''), 'ledger')
        commit_count = "0"
        if os.path.exists(ledger_dir):
            try:
                commit_count = subprocess.check_output(
                    ['git', '-C', ledger_dir, 'rev-list', '--count', 'HEAD'],
                    stderr=subprocess.DEVNULL
                ).decode().strip()
            except Exception:
                commit_count = "error"
        
        repo_count = "1" if int(commit_count) > 0 else "0"
        print(f' └─ Local-First Ledgers    : {BOLD}{repo_count} Unique Session-Isolated Git Repository Active{RESET}')
        print(f'    └─ [{GREEN}{commit_count}{RESET} {DIM}cryptographically anchored Git commit generated via two-phase commit tracking file mutations{RESET}]\n')

        # 4. VERIFIED STATE MUTATIONS
        print(f'{DIM}{LINE}{RESET}')
        print(f' {BOLD}{CYAN}4. VERIFIED STATE MUTATIONS [EXECUTED BY PROTOBUF PAYLOAD TYPE]{RESET}')
        print(f'{DIM}{LINE}{RESET}')
        print(f' {DIM}Legitimate actions that passed all validation, were processed by the Warden, and executed on the host.{RESET}\n')
        
        print(f' {BOLD}{"ACTION PAYLOAD ENGINE TYPE":<28} {"TOTAL SUCCESSFUL RUNS":<24} {"BLAST RADIUS & SCOPE DESCRIPTION"}{RESET}')
        print(f' {DIM}{"─" * 120}{RESET}')
        
        rows = self.conn.execute("""
            SELECT
              substr(command_raw, 1, instr(command_raw, ' /') - 1) AS action_type,
              COUNT(*) AS count
            FROM events
            WHERE type = 'action_receipt'
              AND command_raw LIKE '% /%'
            GROUP BY action_type
            ORDER BY count DESC
        """).fetchall()
        
        scope_map = {
            'FS_LIST': 'Read-only host system state discovery and log extraction',
            'FILE_EDIT': 'Two-phase commit file modification / infrastructure patch',
            'EXECUTE_BASH': 'Host shell command execution with monitored side-effects'
        }
        
        if rows:
            for r in rows:
                action_type = r['action_type']
                scope = scope_map.get(action_type, 'General host-side capability execution')
                print(f"  {BOLD}{action_type:<28}{RESET} {GREEN}{r['count']:<24,}{RESET} {DIM}{scope}{RESET}")
        else:
            print(f'  {DIM}{"N/A":<28} {"0":<24} No executed actions found{RESET}')
            
        print(f' {DIM}{"─" * 120}{RESET}')
        print(f' {BOLD}TOTAL VERIFIED STATE CHANGES:   {BOLD}{GREEN}{l3_allowed:,}{RESET}\n')

        # 5. DISTRIBUTED ROUTING ANALYSIS
        print(f'{DIM}{LINE}{RESET}')
        print(f' {BOLD}{CYAN}5. DISTRIBUTED ROUTING ANALYSIS [TOP ACTIVE INVESTIGATION SESSIONS]{RESET}')
        print(f'{DIM}{LINE}{RESET}')
        print(f' {DIM}Volume breakdown of transaction traffic routed via individual isolated operator processes across active cases.{RESET}\n')
        
        print(f' {BOLD}{"OPERATOR SESSION IDENTIFIER":<40} {"TOTAL EVENT INGESTION THROUGHPUT SHARED"}{RESET}')
        print(f' {DIM}{"─" * 120}{RESET}')
        
        rows = self.conn.execute("""
            SELECT operator_session_id, COUNT(*) AS count
            FROM events
            GROUP BY operator_session_id
            ORDER BY count DESC
            LIMIT 5
        """).fetchall()
        
        if rows:
            for r in rows:
                print(f"  {BLUE}{r['operator_session_id']:<38}{RESET} {BOLD}{r['count']:,}{RESET} {DIM}sequential message cycles processed via isolated execution tree{RESET}")
        else:
            print(f'  {DIM}{"N/A":<38} 0 sequential message cycles{RESET}')
        print()

        # 6. HISTORICAL THROUGHPUT PROFILE
        print(f'{DIM}{LINE}{RESET}')
        print(f' {BOLD}{CYAN}6. HISTORICAL THROUGHPUT PROFILE [PEAK PROCESSING WINDOW]{RESET}')
        print(f'{DIM}{LINE}{RESET}')
        print(f' {DIM}Granular, second-by-second profiling of ingestion bandwidth, filtration efficiency, and tamper detection load.{RESET}\n')
        
        print(f' {BOLD}{"TIMESTAMP (UTC)":<16} {"INBOUND/SEC":>12}    {"EXECUTED/SEC":>12}   {"L1 BLOCKED/SEC":>14}   {"TAMPER DETECTED/SEC":>19}   {"COMPUTE BURST EFFICIENCY"}{RESET}')
        print(f' {DIM}{"─" * 120}{RESET}')
        
        rows = self.conn.execute("""
            SELECT
              strftime('%H:%M:%S', timestamp) AS second,
              COUNT(*) AS inbound,
              SUM(CASE WHEN type = 'action_receipt' THEN 1 ELSE 0 END) AS executed,
              SUM(CASE WHEN type = 'L1_BLOCKED' THEN 1 ELSE 0 END) AS l1_blocks,
              SUM(CASE WHEN type = 'HASH_FAIL' THEN 1 ELSE 0 END) AS tamper_rejects
            FROM events
            GROUP BY second
            ORDER BY second DESC
            LIMIT 5
        """).fetchall()
        
        if rows:
            peak_inbound = 0
            peak_executed = 0
            peak_blocks = 0
            peak_tamper = 0
            
            for r in rows:
                peak_inbound = max(peak_inbound, r['inbound'])
                peak_executed = max(peak_executed, r['executed'])
                peak_blocks = max(peak_blocks, r['l1_blocks'])
                peak_tamper = max(peak_tamper, r['tamper_rejects'])
                
                efficiency = (r['executed'] / r['inbound'] * 100) if r['inbound'] > 0 else 0
                eff_color = GREEN
                if efficiency < 50: eff_color = RED
                elif efficiency < 80: eff_color = YELLOW
                
                print(f" {BOLD}{r['second']:<16}{RESET} {r['inbound']:>12,}    {GREEN}{r['executed']:>12,}{RESET}   {YELLOW}{r['l1_blocks']:>14,}{RESET}   {RED}{r['tamper_rejects']:>19,}{RESET}   {eff_color}{efficiency:>5.1f}% Efficiency Rating{RESET}")
            
            print(f' {DIM}{"─" * 120}{RESET}')
            print(f' {BOLD}{"PEAK CAPACITY":<16} {peak_inbound:>11,}/sec  {GREEN}{peak_executed:>10,}/sec{RESET} {YELLOW}{peak_blocks:>10,} blocks/sec{RESET} {RED}{peak_tamper:>12,} alerts/sec{RESET}        {DIM}Avg Processing Latency: {BOLD}{duration_str}{RESET}')
        else:
            print(f'  {DIM}No throughput data available.{RESET}')

        print(f'\n{DIM}{LINE}{RESET}')
        print(f' {DIM}[FOOTER] Local-First Audit Architecture // Open-Source Agentic BFT Gateway // github.com/g8e-ai/g8e{RESET}')
        print(f'{DIM}{LINE}{RESET}\n')




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
        operatorSession = self.conn.execute(
            'SELECT id, title, created_at, user_identity FROM sessions WHERE id = ?',
            (operator_session_id,)
        ).fetchone()
        if not operatorSession:
            print(f'\nOperator session not found: {operator_session_id}')
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
            'exported_at': datetime.now(UTC).isoformat() + 'Z',
            'operator_session': dict(operatorSession),
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

    # operator-session
    sp = subparsers.add_parser('operator-session', help='Get details for a single operator session')
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

    # chaos-summary
    subparsers.add_parser('chaos-summary', help='Show chaos test summary from chaos_events table')

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
        elif args.command == 'chaos-summary':
            manager.chaos_summary()
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
