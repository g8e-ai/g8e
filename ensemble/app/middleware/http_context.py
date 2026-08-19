import json
import logging
from collections.abc import Callable, Awaitable
from fastapi import Request, Response
from starlette.middleware.base import BaseHTTPMiddleware
from app.models.http_context import RequestContext, G8eHttpContext

logger = logging.getLogger(__name__)


class G8eHttpContextMiddleware(BaseHTTPMiddleware):
    async def dispatch(
        self, request: Request, call_next: Callable[[Request], Awaitable[Response]]
    ) -> Response:
        try:
            # We must be careful reading body
            body = b""
            if request.method in ("POST", "PUT", "PATCH"):
                body = await request.body()

            context_data = None
            if body:
                try:
                    payload = json.loads(body)
                    if isinstance(payload, dict) and "context" in payload:
                        context_data = payload["context"]
                except json.JSONDecodeError:
                    pass

            if not context_data:
                # We no longer fall back to headers. RequestContext must be in the body.
                pass

            if context_data:
                try:
                    rc = RequestContext(**context_data)
                    g8e_context = G8eHttpContext.from_request_context(rc)
                    request.state.g8e_context = g8e_context
                    request.state.request_context = rc
                except Exception as e:
                    logger.warning(f"Failed to parse context: {e}")

        except Exception as e:
            logger.warning(f"Error in context middleware: {e}")

        return await call_next(request)
