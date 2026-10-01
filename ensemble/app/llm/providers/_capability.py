# Copyright (c) 2026 Lateralus Labs, LLC.
# Use of this source code is governed by the Business Source License
# included in the LICENSE file.
#
# As of the Change Date listed in the LICENSE file, this software is
# released under the Apache License, Version 2.0.

"""Shared translation from raw SDK errors to typed ModelCapabilityError.

Provider adapters call ``translate_capability_error`` at their catch sites
with context about what the request asked for (thinking / tools). If the
SDK error matches a known capability-rejection fingerprint, a typed
``ThinkingNotSupportedError`` or ``ToolsNotSupportedError`` is raised so
downstream code can branch via ``isinstance`` instead of string parsing.

All substring heuristics live exclusively in this module. Adding a new
pattern belongs here, never at a consumer.
"""

from __future__ import annotations


# Fingerprints (lowercased) that mean "model does not support thinking".
_THINKING_PATTERNS: tuple[str, ...] = (
    "thinking_config",
    "thinking is not supported",
    "invalid thinking_level",
    "reasoning is not supported",
    "does not support thinking",
    "does not support reasoning",
    "extended thinking",
)

# Fingerprints (lowercased) that mean "model does not support tools".
_TOOLS_PATTERNS: tuple[str, ...] = (
    "tools not supported",
    "tool use not supported",
    "function calling not supported",
    "does not support tools",
    "does not support function",
    "tool_use is not supported",
)


# Public-safe text of the Gateway's typed ``ErrInferenceCapabilityUnsupported``
# sentinel (internal/constants/errors.go). The Gateway never forwards provider
# error text, so this is the only fingerprint a governed-dispatch rejection
# carries: HTTP 422 body for a pre-stream rejection, or the failure-frame reason.
_GOVERNED_CAPABILITY_UNSUPPORTED = "requested capability unsupported"


def _governed_rejection_text(exc: BaseException) -> str:
    """The Gateway-reported rejection text carried by a governed-dispatch failure."""
    detail = getattr(exc, "error_detail", None)
    details = getattr(detail, "details", None)
    response = details.get("response") if isinstance(details, dict) else None
    if isinstance(response, str) and response:
        return response
    return str(exc)


def translate_governed_tool_rejection(
    exc: BaseException,
    *,
    service_name: str,
    model: str,
    tools_declared: bool,
) -> None:
    """Raise ``ToolsNotSupportedError`` when the Gateway reports that the
    provider rejected a request that declared tools.

    The scored agent call declares tools and requests no other optional
    capability (no response format, default tool choice), so a typed
    capability-unsupported rejection there is the tool declaration. Returns
    without raising for any other failure so the caller re-raises the
    original exception.
    """
    if not tools_declared:
        return
    rejection_text = _governed_rejection_text(exc)
    if _GOVERNED_CAPABILITY_UNSUPPORTED not in rejection_text.lower():
        return

    from app.errors import ToolsNotSupportedError

    raise ToolsNotSupportedError(
        f"Provider rejected the tool declaration: {rejection_text}",
        model=model,
        service_name=service_name,
        cause=exc if isinstance(exc, Exception) else None,
    ) from exc


def translate_capability_error(
    exc: BaseException,
    *,
    service_name: str,
    model: str,
    thinking_requested: bool,
    tools_requested: bool,
) -> None:
    """Inspect ``exc`` and raise a typed capability error if it matches.

    If the error does not look like a capability rejection, returns without
    raising - the caller is expected to re-raise the original exception.

    Heuristics only run for capabilities the caller actually requested; an
    error mentioning "tools" when tools were not requested is not a tool
    capability error.
    """
    # Lazy import to avoid circular dependency: app.errors -> app.models ->
    # app.llm -> providers -> this module.
    from app.errors import ThinkingNotSupportedError, ToolsNotSupportedError

    message = str(exc).lower()
    if not message:
        return

    if thinking_requested and any(p in message for p in _THINKING_PATTERNS):
        raise ThinkingNotSupportedError(
            str(exc),
            model=model,
            service_name=service_name,
            cause=exc if isinstance(exc, Exception) else None,
        ) from exc

    if tools_requested and any(p in message for p in _TOOLS_PATTERNS):
        raise ToolsNotSupportedError(
            str(exc),
            model=model,
            service_name=service_name,
            cause=exc if isinstance(exc, Exception) else None,
        ) from exc
