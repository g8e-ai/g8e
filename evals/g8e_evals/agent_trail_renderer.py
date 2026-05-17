"""Live CLI renderer for agent trail events.

Each SSE event produced by the g8ee chat pipeline is mapped to a short,
colored line so the operator can see every agent in the stack as it
fires (Triage → Dash/Sage → Tribunal members → Auditor → Warden → tools →
reputation), plus thinking/streaming markers.

This module is deliberately stateless across calls: state for the current
turn (e.g. whether we have already emitted the Tribunal banner) is held on
a ``TurnRenderer`` instance the CLI creates per task.
"""

from __future__ import annotations

from typing import Any, Optional

from rich.console import Console


# Map of full event_type -> (label, color, kind).
# kind="line" prints one line; kind="dot" prints a single dot (for noisy streams).
_EVENT_STYLE: dict[str, tuple[str, str, str]] = {
    # Triage
    "g8e.v1.ai.triage.clarification.questions": ("TRIAGE/clarify", "yellow", "line"),
    "g8e.v1.ai.triage.clarification.answered":  ("TRIAGE/answered", "yellow", "line"),
    "g8e.v1.ai.triage.clarification.skipped":   ("TRIAGE/skipped", "yellow", "line"),
    "g8e.v1.ai.triage.clarification.timeout":   ("TRIAGE/timeout", "yellow", "line"),

    # Iteration lifecycle
    "g8e.v1.ai.llm.chat.iteration.started":     ("ITERATION/start", "blue", "line"),
    "g8e.v1.ai.llm.chat.iteration.completed":   ("ITERATION/done",  "blue", "line"),
    "g8e.v1.ai.llm.chat.iteration.failed":      ("ITERATION/failed","red",  "line"),
    "g8e.v1.ai.llm.chat.iteration.stopped":     ("ITERATION/stopped","red", "line"),
    "g8e.v1.ai.llm.chat.iteration.retry":       ("ITERATION/retry", "yellow","line"),
    "g8e.v1.ai.llm.chat.iteration.text.completed": ("ITERATION/text.completed", "blue", "line"),
    "g8e.v1.ai.llm.chat.iteration.text.truncated": ("ITERATION/text.truncated", "yellow", "line"),
    "g8e.v1.ai.llm.chat.iteration.thinking.started": ("THINKING", "magenta", "thinking"),
    "g8e.v1.ai.llm.chat.iteration.citations.received": ("CITATIONS", "cyan", "line"),
    "g8e.v1.ai.llm.chat.iteration.text.chunk.received": ("TEXT", "green", "chunk"),

    # Agent
    "g8e.v1.ai.agent.continue.approval.requested": ("AGENT/continue?",  "yellow", "line"),
    "g8e.v1.ai.agent.continue.approval.granted":   ("AGENT/continue+",  "green",  "line"),
    "g8e.v1.ai.agent.continue.approval.rejected":  ("AGENT/continue-",  "red",    "line"),
    "g8e.v1.ai.agent.conflict.detected":           ("AGENT/conflict!",  "red",    "line"),
    "g8e.v1.ai.agent.conflict.resolved":           ("AGENT/conflict.ok","green",  "line"),

    # Tribunal
    "g8e.v1.ai.tribunal.session.started":   ("TRIBUNAL/session.start", "cyan", "line"),
    "g8e.v1.ai.tribunal.session.completed": ("TRIBUNAL/session.done",  "cyan", "line"),
    "g8e.v1.ai.tribunal.session.disabled":  ("TRIBUNAL/disabled",      "yellow", "line"),
    "g8e.v1.ai.tribunal.session.model.not_configured": ("TRIBUNAL/no-model", "red", "line"),
    "g8e.v1.ai.tribunal.session.provider.unavailable": ("TRIBUNAL/no-provider", "red", "line"),
    "g8e.v1.ai.tribunal.session.system.error":         ("TRIBUNAL/system.error", "red", "line"),
    "g8e.v1.ai.tribunal.session.generation.failed":    ("TRIBUNAL/gen.failed", "red", "line"),
    "g8e.v1.ai.tribunal.session.auditor.failed":       ("TRIBUNAL/auditor.failed", "red", "line"),
    "g8e.v1.ai.tribunal.session.warden_blocked":       ("WARDEN/blocked", "red", "line"),

    "g8e.v1.ai.tribunal.voting.started":             ("TRIBUNAL/voting.start", "cyan", "line"),
    "g8e.v1.ai.tribunal.voting.failed":              ("TRIBUNAL/voting.failed", "red", "line"),
    "g8e.v1.ai.tribunal.voting.pass.completed":      ("TRIBUNAL/pass.done", "cyan", "line"),
    "g8e.v1.ai.tribunal.voting.pass.failed":         ("TRIBUNAL/pass.failed", "red", "line"),
    "g8e.v1.ai.tribunal.voting.consensus.reached":   ("TRIBUNAL/consensus+", "green", "line"),
    "g8e.v1.ai.tribunal.voting.consensus.not_reached": ("TRIBUNAL/consensus-", "yellow", "line"),
    "g8e.v1.ai.tribunal.voting.consensus.failed":    ("TRIBUNAL/consensus.failed", "red", "line"),
    "g8e.v1.ai.tribunal.voting.dissent.recorded":    ("TRIBUNAL/dissent", "yellow", "line"),
    "g8e.v1.ai.tribunal.voting.audit.started":       ("AUDITOR/start", "cyan", "line"),
    "g8e.v1.ai.tribunal.voting.audit.completed":     ("AUDITOR/done",  "cyan", "line"),
    "g8e.v1.ai.tribunal.voting.round.started":       ("TRIBUNAL/round.start", "cyan", "line"),
    "g8e.v1.ai.tribunal.voting.round.completed":     ("TRIBUNAL/round.done",  "cyan", "line"),
    "g8e.v1.ai.tribunal.voting.round_2.started":     ("TRIBUNAL/round2.start","cyan", "line"),
    "g8e.v1.ai.tribunal.voting.round_2.pass.completed": ("TRIBUNAL/round2.pass", "cyan", "line"),
    "g8e.v1.ai.tribunal.voting.round_2.consensus.reached": ("TRIBUNAL/round2.consensus+", "green", "line"),
    "g8e.v1.ai.tribunal.voting.round_2.consensus.failed":  ("TRIBUNAL/round2.consensus-", "red", "line"),

    # Reputation
    "g8e.v1.ai.reputation.commitment.created":  ("REPUTATION/commit",   "magenta", "line"),
    "g8e.v1.ai.reputation.commitment.verified": ("REPUTATION/verified", "magenta", "line"),
    "g8e.v1.ai.reputation.commitment.failed":   ("REPUTATION/failed",   "red",     "line"),
    "g8e.v1.ai.reputation.state.updated":       ("REPUTATION/update",   "magenta", "line"),
    "g8e.v1.ai.reputation.slash.tier1":         ("REPUTATION/slash.t1", "red",     "line"),
    "g8e.v1.ai.reputation.slash.tier2":         ("REPUTATION/slash.t2", "red",     "line"),
    "g8e.v1.ai.reputation.slash.tier3":         ("REPUTATION/slash.t3", "red",     "line"),

    # LLM lifecycle / chat-level failures (credential errors, provider
    # outages, dead-lettered messages — surface these prominently so the
    # operator sees them in the live stream instead of having to grep the
    # receipt JSON.
    "g8e.v1.ai.llm.lifecycle.requested":      ("LLM/requested", "blue",   "line"),
    "g8e.v1.ai.llm.lifecycle.started":        ("LLM/started",   "blue",   "line"),
    "g8e.v1.ai.llm.lifecycle.completed":      ("LLM/completed", "blue",   "line"),
    "g8e.v1.ai.llm.lifecycle.failed":         ("LLM/failed",    "red",    "line"),
    "g8e.v1.ai.llm.lifecycle.stopped":        ("LLM/stopped",   "red",    "line"),
    "g8e.v1.ai.llm.lifecycle.error.occurred": ("LLM/error",     "red",    "line"),
    "g8e.v1.ai.llm.chat.message.processing.failed": ("CHAT/processing.failed", "red", "line"),
    "g8e.v1.ai.llm.chat.message.dead.lettered":     ("CHAT/dead-lettered",     "red", "line"),

    # Audit
    "g8e.v1.operator.audit.user.recorded":              ("AUDIT/user",   "blue", "line"),
    "g8e.v1.operator.audit.ai.recorded":                ("AUDIT/ai",     "blue", "line"),
    "g8e.v1.operator.audit.command.recorded":           ("AUDIT/cmd",    "blue", "line"),
    "g8e.v1.operator.audit.direct.command.recorded":    ("AUDIT/direct", "blue", "line"),
    "g8e.v1.operator.audit.direct.command.result.recorded": ("AUDIT/direct.result", "blue", "line"),
}


def _summary_for(event_type: str, payload: dict[str, Any]) -> str:
    """Return a one-line summary derived from the payload, when useful."""
    event = payload.get("event") if isinstance(payload, dict) else None
    data: Optional[dict[str, Any]] = None
    if isinstance(event, dict):
        d = event.get("data")
        if isinstance(d, dict):
            data = d
    if data is None:
        return ""

    # Triage classification summary, if exposed.
    if event_type == "g8e.v1.ai.triage.clarification.questions":
        qs = data.get("questions") or []
        return f"questions={len(qs)}"

    # Tool lifecycle summaries.
    if "tool" in event_type:
        bits = []
        for key in ("tool_name", "status", "execution_id", "category"):
            v = data.get(key)
            if v:
                bits.append(f"{key}={v}")
        return " ".join(bits)

    # Tribunal/auditor/warden free-form payloads
    interesting_keys = (
        "agent_id", "member", "candidate", "command", "decision",
        "risk", "risk_level", "winner", "audited_command", "round",
        "intent", "intent_summary", "complexity", "posture",
        "vote_count", "consensus", "error", "error_message",
        "error_type", "reason", "provider", "model",
    )
    bits = []
    for key in interesting_keys:
        v = data.get(key)
        if v is not None and v != "":
            text = str(v)
            if len(text) > 60:
                text = text[:60] + "…"
            bits.append(f"{key}={text}")
    return " ".join(bits)


class TurnRenderer:
    """Per-task renderer. Owns the live agent-stack output for one chat turn."""

    def __init__(self, console: Console, task_id: str, verbose_text: bool = False):
        self.console = console
        self.task_id = task_id
        self._thinking_open = False
        self._verbose_text = verbose_text
        self._text_chars = 0

    def render(self, event_type: str, payload: dict[str, Any]) -> None:
        style = _EVENT_STYLE.get(event_type)
        if style is None:
            # Unknown / less interesting event — log compactly so it still
            # shows up in the trail without dominating the terminal.
            return

        label, color, kind = style

        if kind == "thinking":
            if not self._thinking_open:
                self.console.print(
                    f"  [dim]{self.task_id}[/dim] [{color}]\\[{label}][/{color}] …",
                    highlight=False,
                )
                self._thinking_open = True
            return

        if kind == "chunk":
            self._text_chars += 1
            if self._verbose_text:
                # Print the chunk inline without prefix to preserve formatting.
                content = _safe_chunk(payload)
                if content:
                    self.console.print(content, end="", highlight=False, markup=False)
            return

        # Any non-thinking, non-chunk event closes a previously open thinking block.
        self._thinking_open = False

        summary = _summary_for(event_type, payload)
        suffix = f" [dim]{summary}[/dim]" if summary else ""
        self.console.print(
            f"  [dim]{self.task_id}[/dim] [{color}]\\[{label}][/{color}]{suffix}",
            highlight=False,
        )

    def finish(self, terminal_event: Optional[str], answer_chars: int) -> None:
        if terminal_event is None:
            self.console.print(
                f"  [dim]{self.task_id}[/dim] [red]\\[TIMEOUT][/red] no terminal event"
            )
        # Final summary line is rendered by the caller.


def _safe_chunk(payload: dict[str, Any]) -> str:
    event = payload.get("event") if isinstance(payload, dict) else None
    if isinstance(event, dict):
        data = event.get("data")
        if isinstance(data, dict):
            content = data.get("content")
            if isinstance(content, str):
                return content
    return ""
