"""Canonical auth + transport wiring for evals HTTP clients.

This is the *only* place the evals harness encodes how to talk to the
running g8e platform (g8ee Engine + Operator) over mTLS:

  - trust bundle resolution (via :mod:`g8e_evals.tls`)
  - mTLS client certificate / key (``G8E_CLI_CERT`` / ``G8E_CLI_KEY``)
  - ``g8e_session`` cookie + ``X-G8E-*`` context headers
  - URL resolution for the g8ee Engine and Operator listen mode

It exists to converge with the shell-side helpers in
``scripts/cmd/common.sh`` (``_build_protocol_curl_args``,
``_append_g8e_context_headers``, ``_g8ee_curl``). A new required header
on either side will trip the parity contract test in
``evals/tests/test_auth_wiring_parity.py`` so the bench and
``./g8e chat send`` cannot silently diverge.

Canonical header names are imported from
``app.constants.headers`` (the g8ee Engine's authoritative list) so the
SUT cannot drift from what the server actually validates.
"""

from __future__ import annotations

import os
from dataclasses import dataclass, field
from typing import Optional

import httpx

from app.constants.headers import (
    BOUND_OPERATORS_HEADER,
    CASE_ID_HEADER,
    CLI_SESSION_ID_HEADER,
    COMPONENT_NAME_HEADER,
    HTTP_CONTENT_TYPE_HEADER,
    INVESTIGATION_ID_HEADER,
    OPERATOR_SESSION_ID_HEADER,
    TASK_ID_HEADER,
    USER_ID_HEADER,
)

from g8e_evals.tls import resolve_trust_bundle


class AuthenticationError(Exception):
    """Raised when the canonical evals transport cannot resolve auth prerequisites."""


# The session cookie name g8eo's auth middleware accepts. Mirrors
# scripts/cmd/common.sh::_append_g8e_context_headers.
SESSION_COOKIE_NAME = "g8e_session"

# X-G8E-Source-Component value the shell helpers send.
SOURCE_COMPONENT_CLIENT = "client"


@dataclass
class AuthContext:
    """Resolved transport + auth context for talking to g8ee + Operator.

    Built once per bench run from the environment exported by
    ``scripts/cmd/evals.sh`` (which itself sources the canonical
    ``scripts/cmd/common.sh`` credential helpers).
    """

    g8ee_url: str
    operator_url: str
    trust_bundle: str
    client_cert: str
    client_key: str
    operator_session_id: str
    cli_session_id: str
    user_id: str
    # Optional request-scoped context. Set per-request, not at construction.
    case_id: str = ""
    investigation_id: str = ""
    bound_operators: str = ""
    task_id: str = ""
    # Filled in by from_env() so callers can introspect what was loaded.
    missing: tuple[str, ...] = field(default_factory=tuple)

    @classmethod
    def from_env(
        cls,
        *,
        operator_session_id: Optional[str] = None,
        operator_url: Optional[str] = None,
    ) -> "AuthContext":
        """Resolve the canonical auth context from the process environment.

        Raises :class:`RuntimeError` if a required value is missing or if
        the mTLS client certificate files do not exist on disk.
        """
        sid = (operator_session_id or os.environ.get("OPERATOR_SESSION_ID") or "").strip()
        cli_sid = (os.environ.get("CLI_SESSION_ID") or "").strip()
        uid = (os.environ.get("USER_ID") or "").strip()
        missing: list[str] = []
        if not sid:
            missing.append("OPERATOR_SESSION_ID")
        if not cli_sid:
            missing.append("CLI_SESSION_ID")
        if not uid:
            missing.append("USER_ID")
        if missing:
            raise AuthenticationError(
                "evals transport requires an authenticated session. "
                "Run `./g8e login` (or start the platform so superadmin is "
                "bootstrapped), then re-run. Missing: " + ", ".join(missing)
            )

        client_cert = os.environ.get("G8E_CLI_CERT") or ""
        client_key = os.environ.get("G8E_CLI_KEY") or ""
        if not (client_cert and client_key and os.path.isfile(client_cert) and os.path.isfile(client_key)):
            raise AuthenticationError(
                "evals transport requires an mTLS client certificate "
                "(G8E_CLI_CERT / G8E_CLI_KEY). Run `./g8e login` to mint one."
            )

        trust_bundle = resolve_trust_bundle()

        g8ee_url = (os.environ.get("G8EE_URL") or "https://localhost:8443").rstrip("/")
        op_url = (
            operator_url
            or os.environ.get("G8E_INTERNAL_HTTP_URL")
            or "https://localhost:9000"
        ).rstrip("/")

        return cls(
            g8ee_url=g8ee_url,
            operator_url=op_url,
            trust_bundle=trust_bundle,
            client_cert=client_cert,
            client_key=client_key,
            operator_session_id=sid,
            cli_session_id=cli_sid,
            user_id=uid,
        )

    # ---- Header / cookie construction ---------------------------------

    def context_headers(self) -> dict[str, str]:
        """Return the canonical ``X-G8E-*`` context header set.

        Mirrors ``scripts/cmd/common.sh::_append_g8e_context_headers``.
        Optional fields (case, investigation, bound operators, task) are
        only emitted when set on this context, exactly as the shell helper
        only appends them when the corresponding env var is non-empty.
        """
        headers: dict[str, str] = {
            HTTP_CONTENT_TYPE_HEADER: "application/json",
            COMPONENT_NAME_HEADER: SOURCE_COMPONENT_CLIENT,
        }
        if self.operator_session_id:
            headers[OPERATOR_SESSION_ID_HEADER] = self.operator_session_id
        if self.cli_session_id:
            headers[CLI_SESSION_ID_HEADER] = self.cli_session_id
        if self.user_id:
            headers[USER_ID_HEADER] = self.user_id
        if self.case_id:
            headers[CASE_ID_HEADER] = self.case_id
        if self.investigation_id:
            headers[INVESTIGATION_ID_HEADER] = self.investigation_id
        if self.bound_operators:
            headers[BOUND_OPERATORS_HEADER] = self.bound_operators
        if self.task_id:
            headers[TASK_ID_HEADER] = self.task_id
        return headers

    def cookies(self) -> dict[str, str]:
        if not self.operator_session_id:
            return {}
        return {SESSION_COOKIE_NAME: self.operator_session_id}

    # ---- httpx client factory -----------------------------------------

    def make_async_client(
        self,
        *,
        connect_timeout: float = 10.0,
        read_timeout: float = 60.0,
        write_timeout: float = 30.0,
        pool_timeout: float = 10.0,
    ) -> httpx.AsyncClient:
        """Construct an ``httpx.AsyncClient`` pre-wired with mTLS + cookie."""
        return httpx.AsyncClient(
            verify=self.trust_bundle,
            cert=(self.client_cert, self.client_key),
            timeout=httpx.Timeout(
                connect=connect_timeout,
                read=read_timeout,
                write=write_timeout,
                pool=pool_timeout,
            ),
            cookies=self.cookies(),
        )
