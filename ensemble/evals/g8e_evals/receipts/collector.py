# Copyright (c) 2026 Lateralus Labs, LLC.
# Use of this source code is governed by the Business Source License
# included in the LICENSE file.
#
# As of the Change Date listed in the LICENSE file, this software is
# released under the Apache License, Version 2.0.

import asyncio
import time

from g8e.operator.v1.operator_pb2 import ActionReceipt
from g8e.receipts import parse_action_receipt
from app.constants.api_paths import GatewayAPIPaths
from g8e_evals.auth_bridge import CLIAuthContext
from g8e_evals.tls import RuntimeIdentity
from g8e_evals.transport import AuthContext


class ReceiptCollector:
    """Poll the Operator's audit receipts endpoint over the canonical mTLS
    transport used by the rest of the evals harness.

    The Operator audit routes require client-cert auth + the standard g8e
    context headers; using the shared :class:`AuthContext` keeps this in
    lockstep with the Go CLI so a new required header on either side trips
    the parity contract test rather than silently 401'ing the bench.
    """

    def __init__(
        self,
        operator_url: str,
        timeout_seconds: int = 30,
        auth: AuthContext | None = None,
        cli_context: CLIAuthContext | None = None,
    ):
        self.operator_url = operator_url.rstrip("/")
        self.timeout_seconds = timeout_seconds
        self.auth = auth or AuthContext.from_env(
            operator_url=operator_url,
            runtime_identity=RuntimeIdentity.GATEWAY,
            cli_context=cli_context,
        )

    async def collect_receipt(self, transaction_id: str) -> ActionReceipt | None:
        """Poll the Operator for an ActionReceipt by transaction_id."""
        return await self._collect_receipt(
            params={"tx_id": transaction_id},
            expected_transaction_id=transaction_id,
        )

    async def collect_receipt_for_investigation(
        self, investigation_id: str, action_type: str
    ) -> ActionReceipt | None:
        return await self._collect_receipt(
            params={"investigation_id": investigation_id, "action_type": action_type}
        )

    async def _collect_receipt(
        self,
        params: dict[str, str],
        expected_transaction_id: str | None = None,
    ) -> ActionReceipt | None:
        start_time = time.time()
        headers = self.auth.auth_headers()
        async with self.auth.make_async_client() as client:
            while time.time() - start_time < self.timeout_seconds:
                try:
                    resp = await client.get(
                        f"{self.operator_url}{GatewayAPIPaths.AUDIT_RECEIPTS}",
                        params=params,
                        headers=headers,
                    )
                    if resp.status_code == 200:
                        receipt = parse_action_receipt(resp.json())
                        if (
                            (
                                expected_transaction_id is None
                                or receipt.transaction_id == expected_transaction_id
                            )
                            and receipt.HasField("final_persistence_attestation")
                        ):
                            return receipt
                except Exception:
                    pass

                await asyncio.sleep(0.5)

        return None
