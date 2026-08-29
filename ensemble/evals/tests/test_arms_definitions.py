# Copyright (c) 2026 Lateralus Labs, LLC.
# Use of this source code is governed by the Business Source License
# included in the LICENSE file.
#
# As of the Change Date listed in the LICENSE file, this software is
# released under the Apache License, Version 2.0.

"""Tier 1 tests for the typed experiment arm definitions.

Verifies arm definition correctness, registry completeness, posture
mapping, and the helper tuples that classify arms into governed and
ungoverned groups.
"""

from __future__ import annotations

import pytest

from g8e_evals.arms import (
    ALL_ARMS,
    ARM_DEFINITIONS,
    GOVERNED_ARMS,
    UNGVERNED_ARMS,
    Arm,
    ArmDefinition,
    GovernancePosture,
    get_arm_definition,
)

pytestmark = pytest.mark.unit


def test_arm_enum_has_five_canonical_values():
    assert set(Arm) == {
        Arm.DIRECT,
        Arm.ENSEMBLE_UNGOVERNED,
        Arm.DOCTRINE,
        Arm.CONSENSUS,
        Arm.NOTARY,
    }


def test_arm_definitions_registry_covers_all_arms():
    assert set(ARM_DEFINITIONS.keys()) == set(Arm)
    for arm in Arm:
        defn = get_arm_definition(arm)
        assert defn.arm_id == arm


def test_direct_arm_bypasses_g8ee_and_gateway():
    defn = get_arm_definition(Arm.DIRECT)
    assert defn.uses_g8ee is False
    assert defn.uses_gateway is False
    assert defn.requested_posture == GovernancePosture.NONE
    assert defn.receipt_binding is False
    assert defn.is_production_posture is False


def test_ensemble_ungoverned_uses_g8ee_but_not_gateway():
    defn = get_arm_definition(Arm.ENSEMBLE_UNGOVERNED)
    assert defn.uses_g8ee is True
    assert defn.uses_gateway is False
    assert defn.requested_posture == GovernancePosture.NONE
    assert defn.receipt_binding is False
    assert defn.is_production_posture is False


def test_doctrine_arm_enforces_l1_only():
    defn = get_arm_definition(Arm.DOCTRINE)
    assert defn.uses_g8ee is True
    assert defn.uses_gateway is True
    assert defn.requested_posture == GovernancePosture.L1_DOCTRINE
    assert defn.receipt_binding is True
    assert defn.is_production_posture is True


def test_consensus_arm_enforces_l1_and_l2():
    defn = get_arm_definition(Arm.CONSENSUS)
    assert defn.uses_g8ee is True
    assert defn.uses_gateway is True
    assert defn.requested_posture == GovernancePosture.L2_CONSENSUS
    assert defn.receipt_binding is True
    assert defn.is_production_posture is True


def test_notary_arm_enforces_l1_l2_and_l3():
    defn = get_arm_definition(Arm.NOTARY)
    assert defn.uses_g8ee is True
    assert defn.uses_gateway is True
    assert defn.requested_posture == GovernancePosture.L3_NOTARY
    assert defn.receipt_binding is True
    assert defn.is_production_posture is True


def test_governed_arms_are_exactly_doctrine_consensus_notary():
    assert set(GOVERNED_ARMS) == {Arm.DOCTRINE, Arm.CONSENSUS, Arm.NOTARY}


def test_ungoverned_arms_are_exactly_direct_and_ensemble_ungoverned():
    assert set(UNGVERNED_ARMS) == {Arm.DIRECT, Arm.ENSEMBLE_UNGOVERNED}


def test_all_arms_union_equals_governed_plus_ungoverned():
    assert set(ALL_ARMS) == set(GOVERNED_ARMS) | set(UNGVERNED_ARMS)


def test_arm_definition_is_frozen():
    defn = get_arm_definition(Arm.DIRECT)
    with pytest.raises(AttributeError, match="cannot assign"):
        defn.uses_g8ee = True  # type: ignore[misc]


def test_get_arm_definition_raises_keyerror_for_non_arm():
    with pytest.raises(KeyError):
        get_arm_definition("nonexistent")  # type: ignore[arg-type]


def test_governance_posture_enum_has_four_values():
    assert set(GovernancePosture) == {
        GovernancePosture.NONE,
        GovernancePosture.L1_DOCTRINE,
        GovernancePosture.L2_CONSENSUS,
        GovernancePosture.L3_NOTARY,
    }


def test_production_posture_arms_are_receipt_binding():
    for arm in Arm:
        defn = get_arm_definition(arm)
        if defn.is_production_posture:
            assert defn.receipt_binding is True
            assert defn.uses_gateway is True


def test_non_gateway_arms_do_not_bind_receipts():
    for arm in UNGVERNED_ARMS:
        defn = get_arm_definition(arm)
        assert defn.receipt_binding is False
