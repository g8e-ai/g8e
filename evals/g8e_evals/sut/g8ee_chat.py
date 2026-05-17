"""G8eeChatSUT — drives the real g8ee chat pipeline end-to-end.

This SUT submits every task as a real chat turn against the running g8ee
Engine via POST /api/internal/chat (mTLS + g8e_session cookie), then drains
the Operator's per-session SSE buffer to capture *every* agent stage the
pipeline executes (Triage, Dash/Sage, Tribunal, Auditor, Warden, tool calls,
reputation updates, thinking, response chunks, etc.).

The full agent trail is folded into the response receipt for offline replay
and per-event statistics, and a live callback streams each stage to the
CLI as it fires.

This is the gold-standard evaluation path: the model under test exercises
the same code paths a real user hits via `./g8e chat send` — no shortcuts.

KNOWN LIMITATION (v1 — busy-poll, not a real stream)
----------------------------------------------------
This SUT drains agent events by polling the Operator's per-session SSE
*replay buffer* (`GET /api/internal/sse/events?since_id=...`) every
``poll_interval_s`` seconds (default 0.25s). Consequences:

  - Wall-clock latency floor of one ``poll_interval_s`` per observed event.
  - For long Sage ReAct turns, the bench issues thousands of GETs against
    the Operator while it waits for the terminal event.

This is acceptable for v1 (the replay buffer is the only consumer-facing
surface today) but is *not* the long-term shape. The correct fix is to
subscribe to a real ``text/event-stream`` endpoint on the Operator
(server-pushed SSE keyed by ``cli_session_id`` + ``since_id``) so the bench
receives each event as it is produced, with no polling and no buffer
scan.

TODO(evals): replace ``_drain_events`` with an httpx streaming reader
against an Operator-native SSE endpoint once one is exposed; remove
``poll_interval_s`` from the public constructor at that point.
"""

from __future__ import annotations

import asyncio
import json
import logging
import time
from dataclasses import dataclass, field
from typing import Any, Awaitable, Callable, Optional

import httpx

from g8e_evals.harness import BindingType, Response, SUTConfig, Task
from g8e_evals.sut.wire import SSEWireEnvelope
from g8e_evals.transport import AuthContext, AuthenticationError


logger = logging.getLogger(__name__)

# Re-exported so `cli.py` (and any external caller) can keep catching
# AuthenticationError from this module's public surface.
__all__ = ["G8eeChatSUT", "AuthenticationError", "AgentTrailEvent", "_extract_substrate_transaction_id"]


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


@dataclass
class AgentTrailEvent:
    """A single SSE event observed during one chat turn."""
    id: int
    event_type: str
    payload: dict[str, Any]
    received_at: float = field(default_factory=time.time)


# Callback invoked for every observed agent event so the CLI can render
# each stage live. Receives (event_type, payload_dict).
EventCallback = Callable[[str, dict[str, Any]], Awaitable[None] | None]


class G8eeChatSUT:
    """Real-pipeline SUT. One instance per bench run; one chat turn per task."""

    def __init__(
        self,
        config: SUTConfig,
        on_event: Optional[EventCallback] = None,
        poll_interval_s: float = 0.25,
        idle_timeout_s: float = 180.0,
    ):
        self.config = config
        self.on_event = on_event
        self.poll_interval_s = poll_interval_s
        self.idle_timeout_s = idle_timeout_s
        # Canonical transport/auth wiring — single source of truth shared
        # with the shell-side helpers in scripts/cmd/common.sh. See
        # evals/tests/test_auth_wiring_parity.py for the contract test.
        self.env = AuthContext.from_env(
            operator_session_id=config.operator_session_id,
            operator_url=config.operator_url,
        )

        # Used by the report header / CLI banner.
        if config.primary.provider and config.primary.model:
            self.model_provider = f"{config.primary.provider}:{config.primary.model}"
        else:
            self.model_provider = config.primary.model or "g8ee:server-default"

    async def check_settings(self) -> dict[str, Any]:
        """Fetch current user settings from g8ee for pre-flight validation."""
        async with self._client() as client:
            try:
                resp = await client.get(
                    f"{self.env.g8ee_url}/api/internal/settings/user",
                    params={"user_id": self.env.user_id},
                    headers=self._g8ee_headers(),
                )
                resp.raise_for_status()
                return resp.json()
            except Exception as e:
                logger.warning("Failed to fetch settings from g8ee: %s", e)
                return {}

    # ---- HTTP client construction --------------------------------------

    def _client(self) -> httpx.AsyncClient:
        return self.env.make_async_client()

    def _g8ee_headers(self) -> dict[str, str]:
        return self.env.context_headers()

    # ---- Main entry point ----------------------------------------------

    async def get_answer(self, task: Task) -> Response:
        async with self._client() as client:
            # 1. Snapshot the operator SSE cursor BEFORE we post, so we
            #    only consume events produced by this turn.
            since_id = await self._current_cursor(client)

            # 2. POST chat — creates a fresh case+investigation and fires
            #    run_chat as a g8ee background task.
            body = self._build_chat_body(task)
            try:
                resp = await client.post(
                    f"{self.env.g8ee_url}/api/internal/chat",
                    headers=self._g8ee_headers(),
                    json=body,
                )
            except httpx.HTTPError as e:
                return Response(
                    answer="",
                    model=self.model_provider,
                    binding=BindingType.UNBOUND,
                    unbound_reason=f"g8ee chat POST failed: {e}",
                )

            if resp.status_code != 200:
                return Response(
                    answer="",
                    model=self.model_provider,
                    binding=BindingType.UNBOUND,
                    unbound_reason=f"g8ee chat returned HTTP {resp.status_code}: {resp.text[:400]}",
                )

            try:
                started = resp.json()
            except json.JSONDecodeError:
                return Response(
                    answer="",
                    model=self.model_provider,
                    binding=BindingType.UNBOUND,
                    unbound_reason=f"g8ee chat returned non-JSON body: {resp.text[:400]}",
                )

            case_id = started.get("case_id") or ""
            investigation_id = started.get("investigation_id") or ""

            # 3. Drain the per-session SSE buffer until terminal or idle.
            answer_text, trail, terminal_event = await self._drain_events(
                client, since_id=since_id, investigation_id=investigation_id
            )

        receipt = self._build_receipt(
            case_id=case_id,
            investigation_id=investigation_id,
            trail=trail,
            answer_text=answer_text,
            terminal_event=terminal_event,
        )

        # A real substrate transaction_id only exists when a Tribunal->Warden
        # mutation produced a signed ActionReceipt. The Operator's audit vault
        # keys receipts by the UAP envelope id (transaction_hash), NOT by the
        # g8ee investigation_id. Scan the agent trail for a substrate tx_id
        # before claiming RECEIPT_BOUND.
        substrate_tx_id = _extract_substrate_transaction_id(trail)

        if terminal_event in _FAILURE_TERMINAL_EVENTS:
            return Response(
                answer=answer_text,
                model=self.model_provider,
                transaction_id=substrate_tx_id,
                receipt=receipt,
                binding=BindingType.UNBOUND,
                unbound_reason=f"chat terminated with {terminal_event}",
            )

        # No terminal event observed within idle window — still surface what we got.
        if terminal_event is None:
            return Response(
                answer=answer_text,
                model=self.model_provider,
                transaction_id=substrate_tx_id,
                receipt=receipt,
                binding=BindingType.UNBOUND,
                unbound_reason=f"idle timeout after {self.idle_timeout_s}s without terminal event",
            )

        if substrate_tx_id:
            return Response(
                answer=answer_text,
                model=self.model_provider,
                transaction_id=substrate_tx_id,
                receipt=receipt,
                binding=BindingType.RECEIPT_BOUND,
            )

        return Response(
            answer=answer_text,
            model=self.model_provider,
            transaction_id=None,
            receipt=receipt,
            binding=BindingType.UNBOUND,
            unbound_reason="answer-only turn (no Warden-signed ActionReceipt emitted)",
        )

    # ---- Helpers --------------------------------------------------------

    def _build_chat_body(self, task: Task) -> dict[str, Any]:
        body: dict[str, Any] = {
            "context": {
                "cli_session_id": self.env.cli_session_id,
                "user_id": self.env.user_id,
                "case_id": "",
                "investigation_id": "",
                "source_component": "client",
            },
            "message": task.prompt,
            "attachments": [],
            "sentinel_mode": False,
            "resource_creation": {"create_case": True},
        }

        # Per-role provider/model overrides — the *request* drives which
        # models the pipeline uses, so the bench actually exercises the
        # model under test through Triage / Dash / Sage / Tribunal.
        primary = self.config.primary
        assistant = self.config.assistant
        lite = self.config.lite

        if primary.provider:
            body["llm_primary_provider"] = primary.provider
        if primary.model:
            body["llm_primary_model"] = primary.model
        if assistant.provider:
            body["llm_assistant_provider"] = assistant.provider
        if assistant.model:
            body["llm_assistant_model"] = assistant.model
        if lite.provider:
            body["llm_lite_provider"] = lite.provider
        if lite.model:
            body["llm_lite_model"] = lite.model

        if primary.api_key:
            body["llm_primary_api_key"] = primary.api_key
        if primary.endpoint:
            body["llm_primary_endpoint"] = primary.endpoint
        if assistant.api_key:
            body["llm_assistant_api_key"] = assistant.api_key
        if assistant.endpoint:
            body["llm_assistant_endpoint"] = assistant.endpoint
        if lite.api_key:
            body["llm_lite_api_key"] = lite.api_key
        if lite.endpoint:
            body["llm_lite_endpoint"] = lite.endpoint
        return body

    async def _current_cursor(self, client: httpx.AsyncClient) -> int:
        """Return the highest SSE event id currently in the operator buffer for our session."""
        try:
            resp = await client.get(
                f"{self.env.operator_url}/api/internal/sse/events",
                params={
                    "cli_session_id": self.env.cli_session_id,
                    "since_id": 0,
                    "limit": 1000,
                },
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
    ) -> tuple[str, list[AgentTrailEvent], Optional[str]]:
        """Poll the SSE buffer until a terminal event or idle timeout.

        Returns (assembled_answer_text, trail, terminal_event_type_or_None).

        NOTE: This is a busy-poll against the Operator's replay buffer, not
        a real SSE subscription. See the module docstring's "KNOWN
        LIMITATION" section. The long-term fix is to consume an
        Operator-native ``text/event-stream`` endpoint instead.
        """
        cursor = since_id
        text_buf: list[str] = []
        trail: list[AgentTrailEvent] = []
        terminal: Optional[str] = None
        last_event_at = time.time()

        while True:
            try:
                resp = await client.get(
                    f"{self.env.operator_url}/api/internal/sse/events",
                    params={
                        "cli_session_id": self.env.cli_session_id,
                        "since_id": cursor,
                        "limit": 500,
                    },
                )
                resp.raise_for_status()
                data = resp.json()
            except (httpx.HTTPError, ValueError) as e:
                # transient — back off and retry until idle timeout.
                if time.time() - last_event_at > self.idle_timeout_s:
                    return ("".join(text_buf), trail, None)
                await asyncio.sleep(self.poll_interval_s)
                continue

            rows = data.get("events") or []
            if rows:
                last_event_at = time.time()
                for row in rows:
                    row_id = int(row.get("id", 0))
                    event_type = row.get("event_type") or ""
                    raw_payload = row.get("payload") or "{}"
                    try:
                        payload_obj = json.loads(raw_payload) if isinstance(raw_payload, str) else raw_payload
                    except json.JSONDecodeError:
                        payload_obj = {"_raw": raw_payload}

                    envelope = SSEWireEnvelope.parse(payload_obj)

                    # Filter on the current investigation when available —
                    # other parallel SSE traffic on this session id (e.g.
                    # operator heartbeats) shouldn't bleed into the trail.
                    if investigation_id and envelope is not None:
                        evt_inv = envelope.investigation_id()
                        if evt_inv and evt_inv != investigation_id:
                            cursor = max(cursor, row_id)
                            continue

                    trail.append(AgentTrailEvent(
                        id=row_id,
                        event_type=event_type,
                        payload=payload_obj,
                    ))
                    cursor = max(cursor, row_id)

                    # Accumulate response text.
                    if event_type == "g8e.v1.ai.llm.chat.iteration.text.chunk.received" and envelope is not None:
                        chunk = envelope.text_chunk()
                        if chunk:
                            text_buf.append(chunk)

                    if self.on_event is not None:
                        try:
                            r = self.on_event(event_type, payload_obj)
                            if asyncio.iscoroutine(r):
                                await r
                        except Exception as e:
                            logger.debug("Renderer callback exception: %s", e, exc_info=True)

                    if event_type in _TERMINAL_EVENTS:
                        terminal = event_type

                if terminal is not None:
                    return ("".join(text_buf), trail, terminal)

            if time.time() - last_event_at > self.idle_timeout_s:
                return ("".join(text_buf), trail, terminal)

            await asyncio.sleep(self.poll_interval_s)

    def _build_receipt(
        self,
        case_id: str,
        investigation_id: str,
        trail: list[AgentTrailEvent],
        answer_text: str,
        terminal_event: Optional[str],
    ) -> dict[str, Any]:
        event_counts: dict[str, int] = {}
        for evt in trail:
            event_counts[evt.event_type] = event_counts.get(evt.event_type, 0) + 1

        return {
            "binding": "g8ee_chat",
            "case_id": case_id,
            "investigation_id": investigation_id,
            "terminal_event": terminal_event,
            "answer_chars": len(answer_text),
            "event_count": len(trail),
            "event_counts_by_type": event_counts,
            "agent_trail": [
                {
                    "id": evt.id,
                    "event_type": evt.event_type,
                    "received_at": evt.received_at,
                    "payload": evt.payload,
                }
                for evt in trail
            ],
        }



# ----- Payload helpers ---------------------------------------------------
#
# All SSE payload introspection MUST go through SSEWireEnvelope so schema
# drift in `services/g8ee/app/models/events.py` fails the contract test in
# `evals/tests/test_sse_wire.py` instead of silently breaking the bench.


def _extract_text_chunk(payload_obj: dict[str, Any]) -> str:
    envelope = SSEWireEnvelope.parse(payload_obj)
    return envelope.text_chunk() if envelope is not None else ""


def _extract_investigation_id(payload_obj: dict[str, Any]) -> str:
    envelope = SSEWireEnvelope.parse(payload_obj)
    return envelope.investigation_id() if envelope is not None else ""


def _extract_substrate_transaction_id(trail: list[AgentTrailEvent]) -> str | None:
    """Scan the agent trail for a Warden-signed ActionReceipt and return its transaction_hash."""
    for evt in trail:
        if evt.event_type != "g8e.v1.ai.governance.warden.receipt.signed":
            continue
        envelope = SSEWireEnvelope.parse(evt.payload)
        if envelope is None:
            continue
        tx_id = envelope.field_in_data("transaction_hash")
        if isinstance(tx_id, str) and tx_id:
            return tx_id
    return None
