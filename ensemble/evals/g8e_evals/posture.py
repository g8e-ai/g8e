# Copyright (c) 2026 Lateralus Labs, LLC.
# Use of this source code is governed by the Business Source License
# included in the LICENSE file.
#
# As of the Change Date listed in the LICENSE file, this software is
# released under the Apache License, Version 2.0.

"""Gateway posture observation.

The gateway is the posture authority. The eval runner records both the
requested posture (from the arm definition) and the independently observed
effective posture. It never infers posture from the CLI argument alone.

For governed arms, the observed posture is queried from the gateway's
``/health`` endpoint, which exposes the gateway's configured posture. For
ungoverned arms (``direct`` and ``ensemble_ungoverned``), the observed
posture is ``NONE`` because the task does not route through the gateway.
"""

from __future__ import annotations

import logging

import httpx

from app.constants.api_paths import GatewayAPIPaths
from g8e_evals.arms import GovernancePosture
from g8e_evals.transport import AuthContext

logger = logging.getLogger(__name__)

# The gateway reports posture using the short names from
# ``config.GatewayPosture`` (doctrine, consensus, notary). The eval
# ``GovernancePosture`` enum uses the prefixed names (l1_doctrine,
# l2_consensus, l3_notary) to make the enforcement layer explicit in
# analytical records. This map translates the wire form to the
# analytical form.
_GATEWAY_POSTURE_MAP: dict[str, GovernancePosture] = {
    "doctrine": GovernancePosture.L1_DOCTRINE,
    "consensus": GovernancePosture.L2_CONSENSUS,
    "notary": GovernancePosture.L3_NOTARY,
    "none": GovernancePosture.NONE,
    "": GovernancePosture.NONE,
}


async def observe_gateway_posture(env: AuthContext) -> GovernancePosture | None:
    """Query the gateway's ``/health`` endpoint and return its configured posture.

    Returns the observed ``GovernancePosture`` or ``None`` if the gateway
    could not be reached or the posture field was absent. The caller is
    responsible for recording the result (or its absence) in the attempt
    record. A ``None`` return means the posture could not be independently
    observed; it must never be silently replaced with the requested posture.
    """
    path = GatewayAPIPaths.HEALTH
    try:
        async with env.make_async_client() as client:
            resp = await client.get(
                f"{env.operator_url}{path}",
                headers=env.auth_headers(),
            )
            if resp.status_code != 200:
                logger.warning(
                    "Gateway health endpoint returned HTTP %s; posture not observed",
                    resp.status_code,
                )
                return None
            data = resp.json()
            posture_str = data.get("posture", "")
            if not posture_str:
                logger.warning("Gateway health response missing posture field")
                return None
            mapped = _GATEWAY_POSTURE_MAP.get(posture_str)
            if mapped is None:
                logger.warning("Gateway reported unknown posture '%s'", posture_str)
                return None
            return mapped
    except (httpx.HTTPError, ValueError, OSError) as exc:
        logger.warning("Failed to observe gateway posture: %s", exc)
        return None
