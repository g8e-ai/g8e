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

"""Pydantic models for the Tribunal reputation scoreboard (GDD §14.4).

`ReputationState` is the cross-chain per-agent EMA scalar (Artifact A) and is
the *only* cross-chain state the platform persists. `ReputationCommitment`
is the per-verdict Merkle commitment (Artifact B) the Auditor writes inside
its verdict step to bind a tribunal verdict to the scoreboard at verdict
time. `ReputationLeaf` is the in-memory leaf representation used to compute
the Merkle root deterministically from a `ReputationState` snapshot.

Authority: this file. The canonical doc-style JSON schemas live at
`shared/models/reputation_state.json` and `shared/models/reputation_commitment.json`
and are kept aligned via the contract test in
`tests/unit/models/test_reputation_models_alignment.py`.
"""

from __future__ import annotations

from enum import IntEnum

from pydantic import Field

from .base import G8eBaseModel, G8eIdentifiableModel, UTCDatetime

__all__ = [
    "ReputationState",
    "ReputationCommitment",
    "ReputationCommitmentCreatedPayload",
    "ReputationCommitmentFailedPayload",
    "ReputationLeaf",
    "SlashTier",
    "StakeResolution",
    "StakeResolutionPayload",
    "GENESIS_PREV_ROOT",
]


class SlashTier(IntEnum):
    """Slash tiers per GDD §6.

    TIER_1: correlated / catastrophic (50-100% stake; destructive outcomes)
    TIER_2: provable faults (5-20% stake; verifier/auditor objective failures)
    TIER_3: liveness (0.1-1% stake; missed passes, ignored questions)

    Int values are preserved in the `stake_resolution.slash_tier` column and
    mirror the literal values in the shared schema enum.
    """

    TIER_1 = 1
    TIER_2 = 2
    TIER_3 = 3


# Sentinel `prev_root` used by the genesis commitment in a deployment.
# Any 64-char hex value distinguishable from a valid root works; we use all
# zeros which matches the canonical "no parent" convention used in similar
# hash-chained ledgers.
GENESIS_PREV_ROOT = "0" * 64


class ReputationState(G8eBaseModel):
    """Per-agent reputation scalar maintained as an EMA across conversations.

    One row per persona id (axiom, concord, variance, pragma, nemesis, sage,
    triage, auditor). Scalar only — never history (GDD §5). The document id
    in the `reputation_state` collection is the `agent_id`.

    Read by: `auditor_service` (cross-chain memory, sole reader); the Phase 3
    `reputation_service` post-execution writer.
    Write by: `reputation_service` only (post-execution stake resolution).
    """

    agent_id: str = Field(
        ...,
        description="Stable persona identifier; serves as the document id in the reputation_state collection.",
    )
    scalar: float = Field(
        ...,
        ge=0.0,
        le=1.0,
        description="EMA scalar in [0.0, 1.0]. 0.0 = no influence (Sybil floor); 1.0 = max influence; bootstrap = 0.5.",
    )
    unbonding_until: UTCDatetime | None = Field(
        default=None,
        description="When the unbonding period ends after a Tier 1 slash. Null = normal-influence operation.",
    )
    last_slash_tier: int | None = Field(
        default=None,
        ge=1,
        le=3,
        description="Tier of the most recent slash event (per GDD §6). Null when no slash has occurred.",
    )
    updated_at: UTCDatetime = Field(
        ...,
        description="When the scalar was last updated (UTC, ISO 8601 with Z suffix).",
    )


class ReputationLeaf(G8eBaseModel):
    """Deterministic leaf for the per-verdict Merkle tree.

    A leaf encodes a single ``(agent_id, scalar)`` pair. Leaves are sorted
    by ``agent_id`` ASCII-ascending before hashing so two parties reading
    the same `reputation_state` snapshot always produce the same root.

    The wire form fed to the hasher is ``f"{agent_id}:{scalar_str}"`` where
    ``scalar_str`` is the canonical decimal representation of the scalar
    (see `app.utils.merkle.scalar_to_canonical_str`). The string form keeps
    the leaf format human-inspectable when grounding-citing a commitment.
    """

    agent_id: str = Field(..., description="Persona id this leaf represents.")
    scalar: float = Field(
        ...,
        ge=0.0,
        le=1.0,
        description="EMA scalar at commit time, identical to the matching ReputationState.scalar.",
    )


class ReputationCommitment(G8eIdentifiableModel):
    """Auditor-signed Merkle commitment binding a tribunal verdict to the scoreboard.

    Written inside the auditor's verdict step (one commitment per passing
    verdict). Forms an append-only chain: each commitment's ``prev_root``
    equals the previous commitment's ``merkle_root`` (deployment-scoped).
    The genesis commitment uses ``prev_root = GENESIS_PREV_ROOT``.

    Citation falsifiability (GDD §7): any party can recompute the Merkle
    root from a stored `reputation_state` snapshot and verify the HMAC
    signature without the Auditor's cooperation, given the HMAC key.

    Inherits ``id``, ``created_at``, ``updated_at`` from
    ``G8eIdentifiableModel`` (note: ``updated_at`` is unused — commitments
    are never revised).
    """

    investigation_id: str = Field(
        ...,
        description="Investigation that produced the verdict triggering this commitment.",
    )
    tribunal_command_id: str = Field(
        ...,
        description="TribunalCommand whose auditor verdict triggered this commitment.",
    )
    merkle_root: str = Field(
        ...,
        min_length=64,
        max_length=64,
        description="SHA-256 hex Merkle root over sorted (agent_id, scalar) leaves of reputation_state at verdict time.",
    )
    prev_root: str = Field(
        ...,
        min_length=64,
        max_length=64,
        description="merkle_root of the previous commitment, or GENESIS_PREV_ROOT for the genesis commitment.",
    )
    leaves_count: int = Field(
        ...,
        ge=0,
        description="Number of (agent_id, scalar) leaves in the Merkle tree.",
    )
    signed_by: str = Field(
        default="auditor",
        description="Signer identity. Day-1: literal 'auditor'.",
    )
    signature: str = Field(
        ...,
        min_length=64,
        max_length=64,
        description="HMAC-SHA256 hex over `merkle_root || prev_root || tribunal_command_id`.",
    )


class ReputationCommitmentCreatedPayload(G8eBaseModel):
    """SSE payload for `REPUTATION_COMMITMENT_CREATED` events."""

    commitment_id: str = Field(..., description="Id of the reputation_commitment row just written.")
    tribunal_command_id: str = Field(..., description="Tribunal session id that triggered the commitment.")
    investigation_id: str = Field(..., description="Investigation id this commitment is bound to.")
    merkle_root: str = Field(..., min_length=64, max_length=64)
    prev_root: str = Field(..., min_length=64, max_length=64)
    leaves_count: int = Field(..., ge=0)
    correlation_id: str | None = Field(default=None, description="Tribunal session correlation id, if any.")


class ReputationCommitmentFailedPayload(G8eBaseModel):
    """SSE payload for `REPUTATION_COMMITMENT_FAILED` events.

    Commitment failures are non-fatal in Phase 2 — the verdict still stands.
    This payload surfaces the failure so ops can observe commitment-chain
    gaps without needing to scrape logs.
    """

    tribunal_command_id: str = Field(..., description="Tribunal session id that would have triggered the commitment.")
    investigation_id: str = Field(..., description="Investigation id the verdict was produced for.")
    error: str = Field(..., description="Human-readable failure reason.")
    correlation_id: str | None = Field(default=None, description="Tribunal session correlation id, if any.")


class StakeResolution(G8eBaseModel):
    """Per-agent stake resolution record tied to a specific tribunal verdict.

    Written by the Phase 3 `reputation_service` after each verdict. Document
    id is the composite ``{tribunal_command_id}:{agent_id}`` which gives
    write-once idempotency: replaying the same verdict cannot double-resolve.

    ``scalar_before`` and ``scalar_after`` are captured so peer auditors can
    replay the EMA trajectory without reconstructing the scoreboard
    snapshot at the time of the resolution.
    """

    id: str = Field(
        ...,
        description="Composite document id `{tribunal_command_id}:{agent_id}`. Write-once idempotency guard.",
    )
    investigation_id: str = Field(
        ...,
        description="Investigation that produced the verdict this resolution is attributed to.",
    )
    tribunal_command_id: str = Field(
        ...,
        description="TribunalCommand whose outcome drove this stake resolution.",
    )
    agent_id: str = Field(
        ...,
        description="Stable persona id whose scalar was updated.",
    )
    outcome_score: float = Field(
        ...,
        ge=0.0,
        le=1.0,
        description="Scalar in [0.0, 1.0] fed into the agent's EMA update.",
    )
    rationale: str = Field(
        ...,
        description="Short machine-readable reason code grounding the scalar.",
    )
    slash_tier: SlashTier | None = Field(
        default=None,
        description="Slash tier applied. Null when no slash was applied.",
    )
    scalar_before: float = Field(
        ...,
        ge=0.0,
        le=1.0,
        description="Agent's EMA scalar prior to this resolution.",
    )
    scalar_after: float = Field(
        ...,
        ge=0.0,
        le=1.0,
        description="Agent's EMA scalar after applying ema_update and any slash-tier adjustment.",
    )
    half_life: int = Field(
        ...,
        ge=1,
        description="EMA half-life used for this resolution (captured for replayability).",
    )
    created_at: UTCDatetime = Field(
        ...,
        description="When the resolution was written (UTC, ISO 8601 with Z suffix).",
    )


class StakeResolutionPayload(G8eBaseModel):
    """SSE payload for REPUTATION_STATE_UPDATED and REPUTATION_SLASH_* events."""

    agent_id: str = Field(..., description="Persona id whose scalar changed.")
    investigation_id: str = Field(..., description="Investigation the resolution is attributed to.")
    tribunal_command_id: str = Field(..., description="Tribunal command id the resolution resolves.")
    scalar_before: float = Field(..., ge=0.0, le=1.0)
    scalar_after: float = Field(..., ge=0.0, le=1.0)
    outcome_score: float = Field(..., ge=0.0, le=1.0)
    rationale: str = Field(...)
    slash_tier: SlashTier | None = Field(default=None)
