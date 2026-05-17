"""Canonical auth + transport wiring for evals HTTP clients.

This is the *only* place the evals harness encodes how to talk to the
running g8e platform (g8ee Engine + Operator) over mTLS:

  - trust bundle resolution (via :mod:`g8e_evals.tls`)
  - mTLS client certificate / key (``G8E_CLI_CERT`` / ``G8E_CLI_KEY``)
  - ``g8e_session`` cookie + ``X-G8E-*`` context headers
  - URL resolution for the g8ee Engine and Operator listen mode

It exists to converge with the shell-side helpers in
``scripts/cmd/common.sh`` (``_build_protocol_curl_args``,
``_append_g8e_auth_headers``, ``_g8ee_curl``). A new required header
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

from g8e_protocol.constants import (
    BOUND_OPERATORS_HEADER,
    CASE_ID_HEADER,
    CLI_SESSION_ID_HEADER,
    COMPONENT_NAME_HEADER,
    HTTP_CONTENT_TYPE_HEADER,
    INVESTIGATION_ID_HEADER,
    OPERATOR_SESSION_ID_HEADER,
    ORGANIZATION_ID_HEADER,
    TASK_ID_HEADER,
    USER_ID_HEADER,
    ComponentName,
)
from g8e_protocol.models import RequestContext, BoundOperator

from g8e_evals.tls import resolve_trust_bundle


class AuthenticationError(Exception):
    """Raised when the canonical evals transport cannot resolve auth prerequisites."""


# The session cookie name g8eo's auth middleware accepts. Mirrors
# scripts/cmd/common.sh::_append_g8e_auth_headers.
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
    organization_id: str = ""
    # Optional request-scoped context. Set per-request, not at construction.
    case_id: str = ""
    investigation_id: str = ""
    bound_operators: list[BoundOperator] = field(default_factory=list)
    task_id: str = ""
    web_session_id: str = ""
    source_component: ComponentName = ComponentName.CLIENT
    system_fingerprint: str = ""
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
        web_sid = (os.environ.get("WEB_SESSION_ID") or "").strip()
        uid = (os.environ.get("USER_ID") or "").strip()
        oid = (os.environ.get("ORGANIZATION_ID") or "").strip()
        fingerprint = (os.environ.get("G8E_SYSTEM_FINGERPRINT") or "").strip()
        
        source = ComponentName.CLIENT
        raw_source = os.environ.get("G8E_SOURCE_COMPONENT")
        if raw_source:
            try:
                source = ComponentName(raw_source)
            except ValueError:
                # Fallback to CLIENT if invalid, or I could raise error.
                # Given 'rip and replace' and 'no tech debt', maybe raise error?
                # But evals might want to be robust.
                pass

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
            web_session_id=web_sid,
            user_id=uid,
            organization_id=oid,
            source_component=source,
            system_fingerprint=fingerprint,
        )

    # ---- Header / cookie construction ---------------------------------

    def auth_headers(self) -> dict[str, str]:
        """Return the minimal header set required for substrate (g8eo) auth.

        Mirrors ``scripts/cmd/common.sh::_operator_curl``.
        """
        headers: dict[str, str] = {
            HTTP_CONTENT_TYPE_HEADER: "application/json",
        }
        if self.operator_session_id:
            # Substrate uses Authorization: Bearer <token>.
            headers[HTTP_AUTHORIZATION_HEADER] = f"{HTTP_BEARER_PREFIX} {self.operator_session_id}"
        if self.cli_session_id:
            headers[CLI_SESSION_ID_HEADER] = self.cli_session_id
        return headers

    def legacy_substrate_headers(self) -> dict[str, str]:
        """Return the legacy ``X-G8E-*`` context header set.

        DEPRECATED: This remains strictly for backward compatibility with g8eo
        routes that haven't migrated to body-embedded context or URI-SAN identity.
        NEVER use this for new routes; use ``auth_headers()`` instead.
        """
        headers = self.auth_headers()
        headers[COMPONENT_NAME_HEADER] = self.source_component.value
        
        if self.user_id:
            headers[USER_ID_HEADER] = self.user_id
        if self.organization_id:
            headers[ORGANIZATION_ID_HEADER] = self.organization_id
        if self.case_id:
            headers[CASE_ID_HEADER] = self.case_id
        if self.investigation_id:
            headers[INVESTIGATION_ID_HEADER] = self.investigation_id
        if self.task_id:
            headers[TASK_ID_HEADER] = self.task_id
        if self.bound_operators:
            if isinstance(self.bound_operators, str):
                headers[BOUND_OPERATORS_HEADER] = self.bound_operators
            else:
                headers[BOUND_OPERATORS_HEADER] = ",".join(op.operator_id for op in self.bound_operators)
        return headers

    def to_request_context(self) -> RequestContext:
        """Return a ``RequestContext`` model for request bodies.

        Matches ``app.models.http_context.RequestContext`` in g8ee.
        """
        return RequestContext(
            web_session_id=self.web_session_id or None,
            cli_session_id=self.cli_session_id,
            user_id=self.user_id,
            organization_id=self.organization_id,
            case_id=self.case_id,
            investigation_id=self.investigation_id,
            task_id=self.task_id,
            bound_operators=self.bound_operators,
            source_component=self.source_component,
            system_fingerprint=self.system_fingerprint,
        )

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
