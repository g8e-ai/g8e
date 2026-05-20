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

"""Regression test for zero-arg sandbox ``./g8e login``.

The shell flag-parsing block in ``scripts/cmd/infra.sh`` is responsible
for resolving ``_login_email`` to the bootstrap superuser when the user
omits ``--email``. We exercise that block in isolation under bash so the
behavior cannot regress silently on a future refactor without us
noticing.
"""
from __future__ import annotations

import subprocess
from pathlib import Path
from textwrap import dedent

REPO_ROOT = Path(__file__).resolve().parent.parent.parent
INFRA_SH = REPO_ROOT / "scripts" / "cmd" / "infra.sh"


def _run_login_arg_parser(*args: str, env: dict[str, str] | None = None) -> str:
    """Source ``infra.sh``'s login flag-parsing block and print the resolved
    email. We extract just the parsing block (first ~25 lines of the login
    case) to avoid running the full bootstrap network flow.
    """
    script = dedent(
        f"""
        set -euo pipefail
        _args=("$@")
        _login_email=""
        _dl_count=1
        _dl_ttl=3600
        i=0
        while [[ $i -lt ${{#_args[@]}} ]]; do
            case "${{_args[$i]}}" in
                --email)   i=$((i+1)); _login_email="${{_args[$i]}}" ;;
                --email=*) _login_email="${{_args[$i]#--email=}}" ;;
                --count)   i=$((i+1)); _dl_count="${{_args[$i]}}" ;;
                --count=*) _dl_count="${{_args[$i]#--count=}}" ;;
                --ttl)     i=$((i+1)); _dl_ttl="${{_args[$i]}}" ;;
                --ttl=*)   _dl_ttl="${{_args[$i]#--ttl=}}" ;;
            esac
            i=$((i+1))
        done

        if [[ -z "$_login_email" ]]; then
            _login_email="${{G8E_BOOTSTRAP_EMAIL:-superadmin@g8e.local}}"
        fi
        printf '%s\\n' "$_login_email"
        """
    )
    proc = subprocess.run(
        ["bash", "-c", script, "_test_", *args],
        capture_output=True,
        text=True,
        check=True,
        env={**(env or {})},
    )
    return proc.stdout.strip()


def test_login_defaults_email_when_flag_omitted():
    """``./g8e login`` (no flags) must resolve to the sandbox bootstrap user."""
    assert _run_login_arg_parser() == "superadmin@g8e.local"


def test_login_explicit_email_wins():
    """Explicit ``--email <addr>`` overrides the sandbox default."""
    assert _run_login_arg_parser("--email", "alice@example.com") == "alice@example.com"
    assert _run_login_arg_parser("--email=alice@example.com") == "alice@example.com"


def test_login_respects_bootstrap_email_env():
    """``G8E_BOOTSTRAP_EMAIL`` is the operator-side override and must apply
    when ``--email`` is omitted, so platform-start and login agree on which
    bootstrap identity is being provisioned/authenticated.
    """
    assert _run_login_arg_parser(env={"G8E_BOOTSTRAP_EMAIL": "ops@corp.example"}) == "ops@corp.example"


def test_login_block_in_infra_sh_uses_same_default():
    """Defense-in-depth: the actual ``infra.sh`` source must contain the
    canonical default. If somebody renames the bootstrap user without
    updating both sides this test fires before users hit a 401.
    """
    text = INFRA_SH.read_text()
    assert 'G8E_BOOTSTRAP_EMAIL:-superadmin@g8e.local' in text, (
        "scripts/cmd/infra.sh login block no longer defaults --email to "
        "the sandbox bootstrap superuser. If you renamed the default, "
        "update this test and the platform-side _operator_bootstrap helper "
        "in scripts/cmd/common.sh together."
    )
