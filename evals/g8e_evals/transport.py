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

"""Canonical auth + transport wiring for evals HTTP clients.

This is the *only* place the evals harness encodes how to talk to the
running g8e platform (g8ee Ensemble + Operator) over mTLS:

  - trust bundle resolution (via :mod:`g8e_evals.tls`)
  - mTLS client certificate / key (``G8E_CLI_CERT`` / ``G8E_CLI_KEY``)
  - ``g8e_session`` cookie + ``X-G8E-*`` context headers
  - URL resolution for the g8ee Ensemble and Operator listen mode

It exists to converge with the Go CLI auth helpers. A new required header
on either side will trip the parity contract test in
``evals/tests/test_auth_wiring_parity.py`` so the bench and
``./g8e chat send`` cannot silently diverge.

Canonical header names are imported from
``app.constants.headers`` (the g8ee Ensemble's authoritative list) so the
SUT cannot drift from what the server actually validates.
"""

from __future__ import annotations

import os
import ssl
from dataclasses import dataclass, field
from typing import Optional

import httpx

from g8e_protocol.constants import (
    HTTP_AUTHORIZATION_HEADER,
    HTTP_BEARER_PREFIX,
    HTTP_CONTENT_TYPE_HEADER,
    HTTP_CLI_SESSION_ID_HEADER,
    ComponentName,
)
from g8e_protocol.generated_paths import PathConstants, PortConstants
from g8e_protocol.models import RequestContext, BoundOperator

from g8e_evals.tls import resolve_trust_bundle


class AuthenticationError(Exception):
    """Raised when the canonical evals transport cannot resolve auth prerequisites."""


# The session cookie name g8eo's auth middleware accepts.
SESSION_COOKIE_NAME = "g8e_session"

# X-G8E-Source-Component value the shell helpers send.
SOURCE_COMPONENT_CLIENT = "client"


@dataclass
class AuthContext:
    """Resolved transport + auth context for talking to g8ee + Operator.

    Built once per bench run from the environment exported by the Go CLI.
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
    operator_id: str = ""
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
        sid = (operator_session_id or os.environ.get("G8E_OPERATOR_SESSION_ID") or "").strip()
        cli_sid = (os.environ.get("G8E_CLI_SESSION_ID") or "").strip()
        web_sid = (os.environ.get("G8E_WEB_SESSION_ID") or "").strip()
        uid = (os.environ.get("G8E_USER_ID") or "").strip()
        oid = (os.environ.get("G8E_ORGANIZATION_ID") or "").strip()
        fingerprint = (os.environ.get("G8E_SYSTEM_FINGERPRINT") or "").strip()
        
        source = ComponentName.CLIENT
        raw_source = os.environ.get("G8E_SOURCE_COMPONENT")
        if raw_source:
            try:
                source = ComponentName(raw_source)
            except ValueError:
                raise ValueError(
                    f"Invalid G8E_SOURCE_COMPONENT='{raw_source}'. "
                    f"Must be one of: {[c.value for c in ComponentName]}"
                ) from None

        missing: list[str] = []
        if not sid:
            missing.append("G8E_OPERATOR_SESSION_ID")
        if not cli_sid:
            missing.append("G8E_CLI_SESSION_ID")
        if not uid:
            missing.append("G8E_USER_ID")
        if missing:
            raise AuthenticationError(
                "evals transport requires an authenticated session. "
                "Run `./g8e login` (or start the platform so the bootstrap "
                "superuser is auto-provisioned), then re-run. Missing: "
                + ", ".join(missing)
            )

        client_cert = os.environ.get("G8E_CLI_CERT") or ""
        client_key = os.environ.get("G8E_CLI_KEY") or ""
        if not (client_cert and client_key and os.path.isfile(client_cert) and os.path.isfile(client_key)):
            raise AuthenticationError(
                "evals transport requires an mTLS client certificate "
                "(G8E_CLI_CERT / G8E_CLI_KEY). Run `./g8e login` to mint one."
            )

        trust_bundle = resolve_trust_bundle()

        g8ee_url = (os.environ.get("G8E_G8EE_URL") or f"https://localhost:{PortConstants.G8E_PORT_G8EE_HTTPS}").rstrip("/")
        op_url = (
            operator_url
            or os.environ.get("G8E_OPERATOR_URL")
            or f"https://localhost:{PortConstants.PORT_OPERATOR_HTTPS}"
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
        """Return the minimal header set required for Gateway (g8eo) auth.

        Mirrors the Go CLI auth headers.
        """
        headers: dict[str, str] = {
            HTTP_CONTENT_TYPE_HEADER: "application/json",
        }
        if self.operator_session_id:
            # Gateway uses Authorization: Bearer <token>.
            headers[HTTP_AUTHORIZATION_HEADER] = f"{HTTP_BEARER_PREFIX} {self.operator_session_id}"
        
        if self.cli_session_id:
            headers[HTTP_CLI_SESSION_ID_HEADER] = self.cli_session_id
        
        return headers

    def to_request_context(
        self,
        *,
        case_id: Optional[str] = None,
        investigation_id: Optional[str] = None,
        task_id: Optional[str] = None,
        source_component: Optional[ComponentName] = None,
        web_session_id: Optional[str] = None,
        operator_id: Optional[str] = None,
        operator_session_id: Optional[str] = None,
    ) -> RequestContext:
        """Return a ``RequestContext`` model for request bodies.

        Matches ``app.models.http_context.RequestContext`` in g8ee.
        """
        # Extract operator_id/operator_session_id from bound_operators if not provided
        if not operator_id and self.bound_operators:
            operator_id = self.bound_operators[0].operator_id
        if not operator_session_id and self.bound_operators:
            operator_session_id = self.bound_operators[0].operator_session_id

        return RequestContext(
            web_session_id=web_session_id or self.web_session_id or None,
            cli_session_id=self.cli_session_id,
            user_id=self.user_id,
            organization_id=self.organization_id,
            case_id=case_id or self.case_id,
            investigation_id=investigation_id or self.investigation_id,
            task_id=task_id or self.task_id,
            bound_operators=self.bound_operators,
            source_component=source_component or self.source_component,
            system_fingerprint=self.system_fingerprint,
            operator_id=operator_id or self.operator_id or None,
            operator_session_id=operator_session_id or self.operator_session_id or None,
        )

    def cookies(self) -> dict[str, str]:
        if not self.operator_session_id:
            return {}
        return {SESSION_COOKIE_NAME: self.operator_session_id}

    # ---- httpx client factory -----------------------------------------

    def _build_ssl_context(self) -> ssl.SSLContext:
        """Build a single SSLContext with both server CA trust and the
        client cert chain loaded.

        httpx's split ``verify=`` / ``cert=`` parameters do not load the
        client chain into the resulting SSLContext in a way that survives
        the TLS 1.3 CertificateRequest from a server using
        ``RequireAndVerifyClientCert``. Constructing the context here
        keeps mTLS working end to end.
        """
        ctx = ssl.create_default_context(cafile=self.trust_bundle)
        ctx.load_cert_chain(certfile=self.client_cert, keyfile=self.client_key)
        return ctx

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
            verify=self._build_ssl_context(),
            timeout=httpx.Timeout(
                connect=connect_timeout,
                read=read_timeout,
                write=write_timeout,
                pool=pool_timeout,
            ),
            cookies=self.cookies(),
        )
