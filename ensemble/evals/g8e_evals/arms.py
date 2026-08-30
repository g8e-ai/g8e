# Copyright (c) 2026 Lateralus Labs, LLC.
# Use of this source code is governed by the Business Source License
# included in the LICENSE file.
#
# As of the Change Date listed in the LICENSE file, this software is
# released under the Apache License, Version 2.0.

"""Typed experiment arm definitions.

An arm is a complete treatment specification: which SUT executes the task,
whether the g8ee ensemble and gateway/operator stack participate, what
governance posture is requested, and whether receipt binding is expected.

The five canonical arms from the position paper are:

  ``direct``
      A direct provider call without g8ee, the gateway, or the operator.
      Isolates raw model behaviour from all g8e orchestration and governance.

  ``ensemble_ungoverned``
      The real g8ee chat pipeline (Triage, Dash/Sage, Tribunal, Auditor,
      Warden events, g8ee prompts, context assembly) without gateway
      governance or receipt binding. Separates ensemble orchestration
      effects from governance effects. This is a diagnostic arm in a
      mutation-safe sandbox, not a production posture.

  ``doctrine``
      The real g8ee-to-gateway-to-operator path with L1 enforced and
      L2/L3 audited.

  ``consensus``
      The same real path with L1 and L2 enforced and L3 audited.

  ``notary``
      The same real path with L1, L2, and L3 enforced.

The posture-only causal estimates are ``consensus - doctrine`` and
``notary - consensus``. The ensemble orchestration estimate is
``ensemble_ungoverned - direct``. Never claim that posture alone caused a
``direct``-versus-governed difference.
"""

from __future__ import annotations

from dataclasses import dataclass
from enum import StrEnum


class Arm(StrEnum):
    DIRECT = "direct"
    ENSEMBLE_UNGOVERNED = "ensemble_ungoverned"
    DOCTRINE = "doctrine"
    CONSENSUS = "consensus"
    NOTARY = "notary"


class GovernancePosture(StrEnum):
    """Requested governance enforcement level for a governed arm.

    ``NONE`` applies to arms that do not route through the gateway. The
    gateway is the posture authority; the eval runner records both the
    requested posture and the independently observed effective posture and
    never infers posture from the CLI argument alone.
    """

    NONE = "none"
    L1_DOCTRINE = "l1_doctrine"
    L2_CONSENSUS = "l2_consensus"
    L3_NOTARY = "l3_notary"


@dataclass(frozen=True)
class ArmDefinition:
    """Static specification of one experiment arm.

    Attributes:
        arm_id: Canonical arm identifier.
        uses_g8ee: Whether the arm routes the task through the g8ee
            ensemble chat pipeline (Triage, Tribunal, Auditor, Warden).
        uses_gateway: Whether the arm routes through the gateway/operator
            governance stack.
        requested_posture: The governance posture to request for this arm.
        receipt_binding: Whether a Warden-signed ``ActionReceipt`` is
            expected and should be collected and verified.
        is_production_posture: Whether this arm represents a production
            security configuration. ``ensemble_ungoverned`` is a
            diagnostic arm and is not a production posture.
        description: Human-readable summary of what the arm measures.
    """

    arm_id: Arm
    uses_g8ee: bool
    uses_gateway: bool
    requested_posture: GovernancePosture
    receipt_binding: bool
    is_production_posture: bool
    description: str


ARM_DEFINITIONS: dict[Arm, ArmDefinition] = {
    Arm.DIRECT: ArmDefinition(
        arm_id=Arm.DIRECT,
        uses_g8ee=False,
        uses_gateway=False,
        requested_posture=GovernancePosture.NONE,
        receipt_binding=False,
        is_production_posture=False,
        description=(
            "Direct provider call without g8ee, the gateway, or the operator. "
            "Isolates raw model behaviour from all g8e orchestration and governance."
        ),
    ),
    Arm.ENSEMBLE_UNGOVERNED: ArmDefinition(
        arm_id=Arm.ENSEMBLE_UNGOVERNED,
        uses_g8ee=True,
        uses_gateway=False,
        requested_posture=GovernancePosture.NONE,
        receipt_binding=False,
        is_production_posture=False,
        description=(
            "Real g8ee chat pipeline without gateway governance or receipt binding. "
            "Diagnostic arm that separates ensemble orchestration effects from governance effects."
        ),
    ),
    Arm.DOCTRINE: ArmDefinition(
        arm_id=Arm.DOCTRINE,
        uses_g8ee=True,
        uses_gateway=True,
        requested_posture=GovernancePosture.L1_DOCTRINE,
        receipt_binding=True,
        is_production_posture=True,
        description=(
            "Real g8ee-to-gateway-to-operator path with L1 enforced and L2/L3 audited."
        ),
    ),
    Arm.CONSENSUS: ArmDefinition(
        arm_id=Arm.CONSENSUS,
        uses_g8ee=True,
        uses_gateway=True,
        requested_posture=GovernancePosture.L2_CONSENSUS,
        receipt_binding=True,
        is_production_posture=True,
        description=(
            "Real g8ee-to-gateway-to-operator path with L1 and L2 enforced and L3 audited."
        ),
    ),
    Arm.NOTARY: ArmDefinition(
        arm_id=Arm.NOTARY,
        uses_g8ee=True,
        uses_gateway=True,
        requested_posture=GovernancePosture.L3_NOTARY,
        receipt_binding=True,
        is_production_posture=True,
        description=(
            "Real g8ee-to-gateway-to-operator path with L1, L2, and L3 enforced."
        ),
    ),
}


def get_arm_definition(arm: Arm) -> ArmDefinition:
    """Return the static definition for ``arm``.

    Raises ``KeyError`` if the arm is not in :data:`ARM_DEFINITIONS`.
    """
    return ARM_DEFINITIONS[arm]


ALL_ARMS: tuple[Arm, ...] = tuple(ARM_DEFINITIONS.keys())

GOVERNED_ARMS: tuple[Arm, ...] = tuple(
    arm for arm, defn in ARM_DEFINITIONS.items() if defn.uses_gateway
)

UNGVERNED_ARMS: tuple[Arm, ...] = tuple(
    arm for arm, defn in ARM_DEFINITIONS.items() if not defn.uses_gateway
)
