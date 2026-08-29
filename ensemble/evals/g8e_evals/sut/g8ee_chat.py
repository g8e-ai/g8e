# Copyright (c) 2026 Lateralus Labs, LLC.
# Use of this source code is governed by the Business Source License
# included in the LICENSE file.
#
# As of the Change Date listed in the LICENSE file, this software is
# released under the Apache License, Version 2.0.

"""G8eeChatSUT - drives the real g8ee chat pipeline end-to-end.

This SUT submits every task as a real chat turn against the running g8ee
Engine via POST /api/internal/chat (mTLS + g8e_session cookie), then streams
events from the Operator's SSE endpoint to capture *every* agent stage the
pipeline executes (Triage, Dash/Sage, Tribunal, Auditor, Warden, tool calls,
reputation updates, thinking, response chunks, etc.).

The full agent trail is folded into the response receipt for offline replay
and per-event statistics, and a live callback streams each stage to the
CLI as it fires.

This is the gold-standard evaluation path: the model under test exercises
the same code paths a real user hits via `./g8e chat send` - no shortcuts.

Event Streaming
---------------
This SUT consumes agent events via a real Server-Sent Events (SSE) stream
from the gateway's `/api/v1/sse/stream` endpoint. The stream:

  - Replays historical events from the buffer (via `since_id` cursor).
  - Pushes new events in real-time via the Operator's PubSub broker.
  - Filters events by `cli_session_id` and optionally `investigation_id`.
  - Maintains a heartbeat keep-alive to detect idle timeouts.

This eliminates the wall-clock latency floor and excessive HTTP requests
that existed in the previous polling-based implementation.
"""

from __future__ import annotations

import asyncio
import json
import logging
import time
from typing import Any, Optional
from collections.abc import Awaitable, Callable

import httpx
from pydantic import BaseModel, Field
from httpx_sse import aconnect_sse

from app.models.internal_api import (
    ChatMessageRequest,
    ChatStartedResponse,
    ResourceCreationRequest,
    SettingsGetRequest,
)
from app.models.http_context import RequestContext
from app.models.settings import G8eeUserSettings
from app.constants.api_paths import API_PATHS, GatewayAPIPaths
from g8e_evals.harness import BindingType, Response, SUTConfig, Task
from g8e_evals.sut.wire import SSEWireEnvelope
from g8e_evals.transport import AuthContext, AuthenticationError


logger = logging.getLogger(__name__)

# Re-exported so `cli.py` (and any external caller) can keep catching
# AuthenticationError from this module's public surface.
__all__ = ["AgentTrailEvent", "AuthenticationError", "G8eeChatSUT", "_extract_Gateway_transaction_id"]


# Terminal SSE event types that conclude a single chat turn.
_TERMINAL_EVENTS = {
    "g8e.v1.ai.llm.chat.iteration.text.completed",
    "g8e.v1.ai.llm.chat.iteration.completed",
    "g8e.v1.ai.llm.chat.iteration.failed",
    "g8e.v1.ai.llm.chat.iteration.stopped",
    "g8e.v1.ai.llm.chat.message.processing.failed",
    "g8e.v1.ai.llm.chat.message.dead.lettered",
    "g8e.v1.ai.llm.chat.iteration.text.truncated",
}

# Failure terminal events that mean we did NOT get a valid answer.
_FAILURE_TERMINAL_EVENTS = {
    "g8e.v1.ai.llm.chat.iteration.failed",
    "g8e.v1.ai.llm.chat.iteration.stopped",
    "g8e.v1.ai.llm.chat.message.processing.failed",
    "g8e.v1.ai.llm.chat.message.dead.lettered",
}


class AgentTrailEvent(BaseModel):
    """A single SSE event observed during one chat turn."""
    id: int
    event_type: str
    payload: dict[str, Any]
    received_at: float = Field(default_factory=time.time)


class ChatEvaluationReceipt(BaseModel):
    """Bundle of agent trail events and metadata for a single chat turn."""
    binding: str = "g8ee_chat"
    case_id: str
    investigation_id: str
    terminal_event: str | None = None
    answer_chars: int
    event_count: int
    event_counts_by_type: dict[str, int]
    agent_trail: list[AgentTrailEvent]

# Callback invoked for every observed agent event so the CLI can render
# each stage live. Receives (event_type, payload_dict).
EventCallback = Callable[[str, dict[str, Any]], Awaitable[None] | None]


class G8eeChatSUT:
    """Real-pipeline SUT. One instance per bench run; one chat turn per task."""

    def __init__(
        self,
        config: SUTConfig,
        on_event: EventCallback | None = None,
        idle_timeout_s: float = 180.0,
    ):
        self.config = config
        self.on_event = on_event
        self.idle_timeout_s = idle_timeout_s
        # Canonical transport/auth wiring - single source of truth shared
        # with the Go CLI. See evals/tests/test_auth_wiring_parity.py for the contract test.
        self.env = AuthContext.from_env(
            operator_session_id=config.operator_session_id,
            g8ee_url=config.g8ee_url,
            operator_url=config.operator_url,
            cli_context=config.auth_context,
        )

        # Used by the report header / CLI banner.
        if config.primary.provider and config.primary.model:
            self.model_provider = f"{config.primary.provider}:{config.primary.model}"
        else:
            self.model_provider = config.primary.model or "g8ee:server-default"

    async def check_settings(self) -> G8eeUserSettings | None:
        """Fetch current user settings from g8ee for pre-flight validation."""
        path = API_PATHS["g8ee_full"]["settings_user"]
        async with self._client() as client:
            request = SettingsGetRequest(context=self.env.to_request_context())
            try:
                resp = await client.post(
                    f"{self.env.g8ee_url}{path}/get",
                    headers=self._g8ee_headers(),
                    content=request.model_dump_json(),
                )
                if resp.status_code == 200:
                    return G8eeUserSettings.model_validate(resp.json())
                if resp.status_code in (401, 403):
                    raise AuthenticationError(
                        f"g8ee settings returned HTTP {resp.status_code}"
                    )
                logger.warning("Failed to fetch settings from g8ee: HTTP %s", resp.status_code)
                return None
            except AuthenticationError:
                raise
            except Exception as e:
                logger.warning("Error fetching settings from g8ee: %s", e)
                return None

    # ---- HTTP client construction --------------------------------------

    def _client(self) -> httpx.AsyncClient:
        return self.env.make_async_client()

    def _g8ee_headers(self) -> dict[str, str]:
        """Minimal headers for g8ee (now authenticated by Gateway/mTLS)."""
        return self.env.auth_headers()

    # ---- Main entry point ----------------------------------------------

    async def get_answer(self, task: Task) -> Response:
        async with self._client() as client:
            # 1. Snapshot the operator SSE cursor BEFORE we post, so we
            #    only consume events produced by this turn.
            since_id = await self._current_cursor(client)

            # 2. POST chat - creates a fresh case+investigation and fires
            #    run_chat as a g8ee background task.
            request = self._build_chat_request(task)
            path = API_PATHS["g8ee_full"]["chat"]
            try:
                resp = await client.post(
                    f"{self.env.g8ee_url}{path}",
                    headers=self._g8ee_headers(),
                    content=request.model_dump_json(),
                )
            except httpx.HTTPError as e:
                return Response(
                    answer="",
                    model=self.model_provider,
                    binding=BindingType.UNBOUND,
                    unbound_reason=f"g8ee chat POST failed: {e}",
                )

            if resp.status_code != 200:
                reason = f"g8ee chat returned HTTP {resp.status_code}: {resp.text[:400]}"
                if resp.status_code == 401:
                    reason = (
                        "g8ee chat returned HTTP 401 Unauthorized. Your session may have expired. "
                        "Run `./g8e auth refresh` to re-authenticate."
                    )
                return Response(
                    answer="",
                    model=self.model_provider,
                    binding=BindingType.UNBOUND,
                    unbound_reason=reason,
                )

            try:
                started = ChatStartedResponse.model_validate(resp.json())
            except (json.JSONDecodeError, ValueError) as e:
                return Response(
                    answer="",
                    model=self.model_provider,
                    binding=BindingType.UNBOUND,
                    unbound_reason=f"g8ee chat returned invalid body: {e}",
                )

            case_id = started.case_id
            investigation_id = started.investigation_id

            # 3. Drain the per-session SSE buffer until terminal or idle.
            answer_text, trail, terminal_event, sse_error = await self._drain_events(
                client, since_id=since_id, investigation_id=investigation_id
            )

        receipt = self._build_receipt(
            case_id=case_id,
            investigation_id=investigation_id,
            trail=trail,
            answer_text=answer_text,
            terminal_event=terminal_event,
        )

        # A real Gateway transaction_id only exists when a Tribunal->Warden
        # mutation produced a signed ActionReceipt. The Operator's audit vault
        # keys receipts by the UAP envelope id (transaction_hash), NOT by the
        # g8ee investigation_id. Scan the agent trail for a Gateway tx_id
        # before claiming RECEIPT_BOUND.
        Gateway_tx_id = _extract_Gateway_transaction_id(trail)

        if sse_error:
            return Response(
                answer=answer_text,
                model=self.model_provider,
                transaction_id=Gateway_tx_id,
                chat_evidence=receipt,
                binding=BindingType.UNBOUND,
                unbound_reason=sse_error,
            )

        if self.config.mode == "baseline":
            return Response(
                answer=answer_text,
                model=self.model_provider,
                transaction_id=None,
                chat_evidence=receipt,
                binding=BindingType.UNBOUND,
                unbound_reason="baseline mode (binding disabled)",
            )

        if terminal_event in _FAILURE_TERMINAL_EVENTS:
            return Response(
                answer=answer_text,
                model=self.model_provider,
                transaction_id=Gateway_tx_id,
                chat_evidence=receipt,
                binding=BindingType.UNBOUND,
                unbound_reason=f"chat terminated with {terminal_event}",
            )

        # No terminal event observed within idle window - still surface what we got.
        if terminal_event is None:
            return Response(
                answer=answer_text,
                model=self.model_provider,
                transaction_id=Gateway_tx_id,
                chat_evidence=receipt,
                binding=BindingType.UNBOUND,
                unbound_reason=f"idle timeout after {self.idle_timeout_s}s without terminal event",
            )

        if Gateway_tx_id:
            return Response(
                answer=answer_text,
                model=self.model_provider,
                transaction_id=Gateway_tx_id,
                chat_evidence=receipt,
                binding=BindingType.RECEIPT_BOUND,
            )

        return Response(
            answer=answer_text,
            model=self.model_provider,
            transaction_id=None,
            chat_evidence=receipt,
            binding=BindingType.UNBOUND,
            unbound_reason="answer-only turn (no Warden-signed ActionReceipt emitted)",
        )

    # ---- Helpers --------------------------------------------------------

    def _build_chat_request(self, task: Task) -> ChatMessageRequest:
        """Assemble a strictly typed ChatMessageRequest for the g8ee Ensemble."""
        primary = self.config.primary
        assistant = self.config.assistant
        lite = self.config.lite

        return ChatMessageRequest(
            context=self.env.to_request_context(),
            message=task.prompt,
            attachments=[],
            sentinel_mode=False,
            resource_creation=ResourceCreationRequest(create_case=True),
            llm_primary_provider=primary.provider,
            llm_primary_model=primary.model,
            llm_assistant_provider=assistant.provider,
            llm_assistant_model=assistant.model,
            llm_lite_provider=lite.provider,
            llm_lite_model=lite.model,
            llm_primary_api_key=primary.api_key,
            llm_primary_endpoint=primary.endpoint,
            llm_assistant_api_key=assistant.api_key,
            llm_assistant_endpoint=assistant.endpoint,
            llm_lite_api_key=lite.api_key,
            llm_lite_endpoint=lite.endpoint,
            web_search_project=self.config.web_search_project,
            web_search_app=self.config.web_search_app,
            web_search_api_key=self.config.web_search_api_key,
        )

    async def _current_cursor(self, client: httpx.AsyncClient) -> int:
        """Return the highest SSE event id currently in the operator buffer for our session."""
        path = GatewayAPIPaths.SSE_EVENTS
        try:
            resp = await client.get(
                f"{self.env.operator_url}{path}",
                params={
                    "cli_session_id": self.env.cli_session_id,
                    "since_id": 0,
                    "limit": 1000,
                },
                headers=self.env.auth_headers(),
            )
            resp.raise_for_status()
            payload = resp.json()
            events = payload.get("events") or []
            if not events:
                return 0
            return max(int(e.get("id", 0)) for e in events)
        except (httpx.HTTPError, ValueError):
            return 0

    async def _drain_events(
        self,
        client: httpx.AsyncClient,
        since_id: int,
        investigation_id: str,
    ) -> tuple[str, list[AgentTrailEvent], str | None, str | None]:
        """Stream events from the Operator's SSE endpoint until terminal or idle.

        Returns (assembled_answer_text, trail, terminal_event_type_or_None, error_reason_or_None).
        """
        text_buf: list[str] = []
        trail: list[AgentTrailEvent] = []
        terminal: str | None = None
        error_reason: str | None = None
        last_event_at = time.time()

        params = {
            "cli_session_id": self.env.cli_session_id,
            "since_id": since_id,
        }

        path = GatewayAPIPaths.SSE_STREAM

        logger.info("[SSE] Draining events for investigation_id=%s since_id=%d", investigation_id, since_id)

        try:
            # Hard timeout for the entire drain operation
            async with asyncio.timeout(self.idle_timeout_s + 10):
                async with aconnect_sse(
                    client,
                    "GET",
                    f"{self.env.operator_url}{path}",
                    params=params,
                    headers=self.env.auth_headers(),
                ) as event_source:
                    async for event in event_source.aiter_sse():
                        if event.event == "heartbeat":
                            if time.time() - last_event_at > self.idle_timeout_s:
                                logger.warning("[SSE] Idle timeout (no actual events) reached during heartbeat-only stream")
                                return ("".join(text_buf), trail, terminal, None)
                            continue

                        last_event_at = time.time()

                        try:
                            payload_obj = json.loads(event.data)
                        except json.JSONDecodeError:
                            payload_obj = {"_raw": event.data}

                        row_id = int(event.id) if event.id else 0
                        sse_event_name = event.event or "unknown"

                        envelope = SSEWireEnvelope.parse(payload_obj)

                        # The Gateway SSE stream wraps every g8e event in a
                        # generic SSE "message" frame. The canonical g8e
                        # event type lives inside the payload at
                        # envelope.event.type, not in the SSE event field.
                        if envelope is not None and envelope.event is not None:
                            event_type = envelope.event.type or sse_event_name
                        else:
                            event_type = sse_event_name

                        # Filter on the current investigation when available
                        if investigation_id and envelope is not None:
                            evt_inv = envelope.investigation_id()
                            if evt_inv and evt_inv != investigation_id:
                                continue

                        trail.append(AgentTrailEvent(
                            id=row_id,
                            event_type=event_type,
                            payload=payload_obj,
                        ))

                        # Accumulate response text from streaming chunks.
                        if event_type == "g8e.v1.ai.llm.chat.iteration.text.chunk.received" and envelope is not None:
                            chunk = envelope.text_chunk()
                            if chunk:
                                text_buf.append(chunk)

                        # The text.completed terminal event carries the full
                        # response content. If we missed earlier chunks (e.g.
                        # SSE subscription started mid-stream), use the complete
                        # payload as the authoritative answer text.
                        if event_type == "g8e.v1.ai.llm.chat.iteration.text.completed" and envelope is not None:
                            complete_text = envelope.text_chunk()
                            if complete_text:
                                text_buf = [complete_text]

                        if self.on_event is not None:
                            try:
                                r = self.on_event(event_type, payload_obj)
                                if asyncio.iscoroutine(r):
                                    await r
                            except Exception as e:
                                logger.debug("Renderer callback exception: %s", e, exc_info=True)

                        if event_type in _TERMINAL_EVENTS:
                            terminal = event_type
                            return ("".join(text_buf), trail, terminal, None)

        except TimeoutError:
            logger.warning("[SSE] Idle timeout or hard deadline reached for investigation %s", investigation_id)
            error_reason = f"idle_timeout_{self.idle_timeout_s}s"
        except httpx.HTTPStatusError as e:
            logger.warning("[SSE] Stream auth/transport failure: %s", e)
            error_reason = f"sse_auth_failed: {e.response.status_code}"
        except Exception as e:
            logger.warning("[SSE] Stream interrupted: %s", e)
            error_reason = f"sse_interrupted: {e}"

        return ("".join(text_buf), trail, terminal, error_reason)

    def _build_receipt(
        self,
        case_id: str,
        investigation_id: str,
        trail: list[AgentTrailEvent],
        answer_text: str,
        terminal_event: str | None,
    ) -> ChatEvaluationReceipt:
        event_counts: dict[str, int] = {}
        for evt in trail:
            event_counts[evt.event_type] = event_counts.get(evt.event_type, 0) + 1

        return ChatEvaluationReceipt(
            case_id=case_id,
            investigation_id=investigation_id,
            terminal_event=terminal_event,
            answer_chars=len(answer_text),
            event_count=len(trail),
            event_counts_by_type=event_counts,
            agent_trail=trail,
        )



# ----- Payload helpers ---------------------------------------------------
#
# All SSE payload introspection MUST go through SSEWireEnvelope so schema
# drift in `services/g8ee/app/models/events.py` fails the contract test in
# `evals/tests/test_sse_wire.py` instead of silently breaking the bench.


def _extract_text_chunk(payload_obj: dict[str, Any]) -> str:
    envelope = SSEWireEnvelope.parse(payload_obj)
    return envelope.text_chunk() if envelope is not None else ""


def _extract_investigation_id(payload_obj: object) -> str:
    envelope = SSEWireEnvelope.parse(payload_obj)
    return envelope.investigation_id() if envelope is not None else ""


def _extract_Gateway_transaction_id(trail: list[AgentTrailEvent]) -> str | None:
    """Scan the agent trail for a Warden-signed ActionReceipt and return its transaction_hash."""
    for evt in trail:
        if evt.event_type != "g8e.v1.ai.governance.warden.receipt.signed":
            continue
        envelope = SSEWireEnvelope.parse(evt.payload)
        if envelope is None:
            continue
        tx_id = envelope.transaction_hash()
        if tx_id:
            return tx_id
    return None
