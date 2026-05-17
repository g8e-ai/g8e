import asyncio
import httpx
import time
from typing import Optional, Dict, Any

from g8e_evals.tls import resolve_trust_bundle


class ReceiptCollector:
    def __init__(self, operator_url: str, timeout_seconds: int = 30):
        self.operator_url = operator_url
        self.timeout_seconds = timeout_seconds

    async def collect_receipt(self, transaction_id: str) -> Optional[Dict[str, Any]]:
        """
        Poll the Operator for an ActionReceipt by transaction_id.
        """
        start_time = time.time()
        async with httpx.AsyncClient(verify=resolve_trust_bundle()) as client:
            while time.time() - start_time < self.timeout_seconds:
                try:
                    # Endpoint from listen_http.go: mux.HandleFunc("/api/audit/receipts", h.handleAuditReceipts)
                    resp = await client.get(
                        f"{self.operator_url}/api/audit/receipts",
                        params={"tx_id": transaction_id}
                    )
                    
                    if resp.status_code == 200:
                        data = resp.json()
                        # handleAuditReceipts likely returns a list or a single object
                        # Check if it's a list and get the first one if it exists
                        if isinstance(data, list) and len(data) > 0:
                            return data[0]
                        elif isinstance(data, dict) and data.get("transaction_id") == transaction_id:
                            return data
                            
                    elif resp.status_code == 404:
                        # Not found yet, keep polling
                        pass
                except Exception:
                    # Connection error or other issues, keep polling
                    pass
                
                await asyncio.sleep(0.5)
                
        return None
