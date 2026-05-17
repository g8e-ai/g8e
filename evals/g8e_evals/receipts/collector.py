import asyncio
import time
from typing import Any, Dict, Optional

from g8e_evals.transport import AuthContext


class ReceiptCollector:
    """Poll the Operator's audit receipts endpoint over the canonical mTLS
    transport used by the rest of the evals harness.

    The Operator audit routes require client-cert auth + the standard g8e
    context headers; using the shared :class:`AuthContext` keeps this in
    lockstep with ``scripts/cmd/common.sh::_operator_curl`` so a new
    required header on either side trips the parity contract test rather
    than silently 401'ing the bench.
    """

    def __init__(
        self,
        operator_url: str,
        timeout_seconds: int = 30,
        auth: Optional[AuthContext] = None,
    ):
        self.operator_url = operator_url.rstrip("/")
        self.timeout_seconds = timeout_seconds
        self.auth = auth or AuthContext.from_env(operator_url=operator_url)

    async def collect_receipt(self, transaction_id: str) -> Optional[Dict[str, Any]]:
        """Poll the Operator for an ActionReceipt by transaction_id."""
        start_time = time.time()
        headers = self.auth.context_headers()
        async with self.auth.make_async_client() as client:
            while time.time() - start_time < self.timeout_seconds:
                try:
                    # Endpoint from listen_http.go:
                    #   mux.HandleFunc("/api/audit/receipts", h.handleAuditReceipts)
                    resp = await client.get(
                        f"{self.operator_url}/api/audit/receipts",
                        params={"tx_id": transaction_id},
                        headers=headers,
                    )
                    if resp.status_code == 200:
                        data = resp.json()
                        if isinstance(data, list) and len(data) > 0:
                            return data[0]
                        if isinstance(data, dict) and data.get("transaction_id") == transaction_id:
                            return data
                    # 404 == not yet committed; any other status falls through
                    # to the retry/backoff path.
                except Exception:
                    # Connection blip — keep polling until the deadline.
                    pass

                await asyncio.sleep(0.5)

        return None
