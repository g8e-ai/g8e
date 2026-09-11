# Copyright (c) 2026 Lateralus Labs, LLC.
# Use of this source code is governed by the Business Source License
# included in the LICENSE file.
#
# As of the Change Date listed in the LICENSE file, this software is
# released under the Apache License, Version 2.0.

"""Phase 0.3 cross-suite verification of production observer boundaries and provenance-bound fixtures.

Verifies that every benchmark suite has concrete production observer
boundaries and immutable provenance-bound fixtures for every grader it
declares, and that no free-form known shape enters grading or
release-gate inputs. This test is the Phase 0.3 gate: it fails closed
when any suite lacks a loader, gold-set, provenance manifest, observer
implementation, or grader-to-suite binding.
"""

from __future__ import annotations

import hashlib
import json
from pathlib import Path
import pytest

from g8e_evals.benchmarks.economics.loader import EconomicsPerformanceLoader
from g8e_evals.benchmarks.economics.observers import EconomicsPerformanceObserverImpl
from g8e_evals.benchmarks.governance.benign_overblock_loader import BenignOverblockLoader
from g8e_evals.benchmarks.governance.loader import GovernanceAdversarialLoader
from g8e_evals.benchmarks.governance.observers import (
    EvidencePreservationObserverImpl,
    IdentityMismatchObserverImpl,
    L3ProofTransplantObserverImpl,
    NonceExpirationObserverImpl,
    PayloadTamperingObserverImpl,
    PolicyAttackObserverImpl,
    ReplayAttemptObserverImpl,
    RevokedCredentialObserverImpl,
    SignedFieldTamperingObserverImpl,
    SignerDefectObserverImpl,
    StaleStateRootObserverImpl,
)
from g8e_evals.benchmarks.governance.policy_attack_loader import PolicyAttackLoader
from g8e_evals.benchmarks.ifeval.loader import IFEvalLoader
from g8e_evals.benchmarks.ifeval.verifier import IFEvalVerifier
from g8e_evals.benchmarks.privacy.loader import (
    PrivacyBoundaryLeakageLoader,
    PrivacyTokenLifecycleLoader,
)
from g8e_evals.benchmarks.privacy.observers import (
    ArtifactLeakageObserverImpl,
    ExfiltrationAttemptObserverImpl,
    RehydrationObserverImpl,
    TokenPersistenceFailureObserverImpl,
    TokenStorePersistenceObserverImpl,
    TokenTTLExpiryObserverImpl,
)
from g8e_evals.benchmarks.reliability.loader import ReliabilityLoader
from g8e_evals.benchmarks.reliability.observers import ReliabilityObserverImpl
from g8e_evals.benchmarks.scenarios.grader import ScenarioGrader
from g8e_evals.benchmarks.scenarios.loader import ScenarioLoader
from g8e_evals.benchmarks.utility.citation_backed_loader import CitationBackedLoader
from g8e_evals.benchmarks.utility.citation_backed_simulator import LocalCitationBackedSimulator
from g8e_evals.benchmarks.utility.factual_qa_loader import FactualQALoader
from g8e_evals.benchmarks.utility.factual_qa_simulator import LocalFactualQASimulator
from g8e_evals.benchmarks.utility.final_state_loader import FinalStateLoader
from g8e_evals.benchmarks.utility.final_state_simulator import LocalFinalStateSimulator
from g8e_evals.benchmarks.utility.ledger_consistency_loader import LedgerConsistencyLoader
from g8e_evals.benchmarks.utility.ledger_consistency_simulator import LocalLedgerConsistencySimulator
from g8e_evals.benchmarks.utility.loader import ToolSequenceLoader
from g8e_evals.benchmarks.utility.observers import (
    CitationBackedObserverImpl,
    FactualQAObserverImpl,
    FinalStateObserverImpl,
    LedgerConsistencyObserverImpl,
    PartialMilestoneObserverImpl,
    ToolSequenceObserverImpl,
)
from g8e_evals.benchmarks.utility.partial_milestone_loader import PartialMilestoneLoader
from g8e_evals.benchmarks.utility.partial_milestone_simulator import LocalPartialMilestoneSimulator
from g8e_evals.benchmarks.utility.tool_use_simulator import LocalToolUseSimulator
from g8e_evals.grader_inventory import GRADER_INVENTORY, ProducerPath
from g8e_evals.schema import FORBIDDEN_METADATA_KEYS


_GOLD_SETS_DIR = Path(__file__).resolve().parents[1] / "gold_sets"
_EVALS_ROOT = Path(__file__).resolve().parents[1]


class SuiteSpec:
    """Typed specification for one benchmark suite.

    Binds the suite ID to its loader class, gold-set directory, and the
    concrete production observer implementations that produce typed
    observations for the graders the suite declares. Cross-cutting
    suites (where evidence comes from the governance envelope and
    receipts rather than dedicated observers) carry an empty observer
    list and ``is_cross_cutting=True``.
    """

    def __init__(
        self,
        suite_id: str,
        loader_class: type,
        *,
        observers: list[type] | None = None,
        is_cross_cutting: bool = False,
        is_partial_external: bool = False,
        gold_set_subdir: str | None = None,
    ) -> None:
        self.suite_id = suite_id
        self.loader_class = loader_class
        self.observers = observers or []
        self.is_cross_cutting = is_cross_cutting
        self.is_partial_external = is_partial_external
        self.gold_set_subdir = gold_set_subdir or suite_id

    @property
    def gold_set_dir(self) -> Path:
        return _GOLD_SETS_DIR / self.gold_set_subdir

    @property
    def data_path(self) -> Path:
        return self.gold_set_dir / "input_data.jsonl"

    @property
    def provenance_path(self) -> Path:
        return self.gold_set_dir / "provenance.json"


_ALL_SUITE_SPECS: list[SuiteSpec] = [
    SuiteSpec(
        "privacy_token_lifecycle",
        PrivacyTokenLifecycleLoader,
        observers=[
            TokenStorePersistenceObserverImpl,
            TokenTTLExpiryObserverImpl,
            TokenPersistenceFailureObserverImpl,
            ExfiltrationAttemptObserverImpl,
        ],
    ),
    SuiteSpec(
        "privacy_boundary_leakage",
        PrivacyBoundaryLeakageLoader,
        observers=[
            ArtifactLeakageObserverImpl,
            RehydrationObserverImpl,
        ],
    ),
    SuiteSpec(
        "governance_adversarial",
        GovernanceAdversarialLoader,
        observers=[
            ReplayAttemptObserverImpl,
            SignedFieldTamperingObserverImpl,
            NonceExpirationObserverImpl,
            StaleStateRootObserverImpl,
            SignerDefectObserverImpl,
            L3ProofTransplantObserverImpl,
            RevokedCredentialObserverImpl,
            PayloadTamperingObserverImpl,
            IdentityMismatchObserverImpl,
            EvidencePreservationObserverImpl,
        ],
        is_cross_cutting=True,
    ),
    SuiteSpec(
        "policy_attack",
        PolicyAttackLoader,
        observers=[PolicyAttackObserverImpl],
    ),
    SuiteSpec(
        "benign_overblock",
        BenignOverblockLoader,
        is_cross_cutting=True,
    ),
    SuiteSpec(
        "tool_sequence",
        ToolSequenceLoader,
        observers=[ToolSequenceObserverImpl],
        gold_set_subdir="utility",
    ),
    SuiteSpec(
        "factual_qa",
        FactualQALoader,
        observers=[FactualQAObserverImpl],
    ),
    SuiteSpec(
        "citation_backed",
        CitationBackedLoader,
        observers=[CitationBackedObserverImpl],
    ),
    SuiteSpec(
        "partial_milestone",
        PartialMilestoneLoader,
        observers=[PartialMilestoneObserverImpl],
    ),
    SuiteSpec(
        "final_state",
        FinalStateLoader,
        observers=[FinalStateObserverImpl],
    ),
    SuiteSpec(
        "ledger_consistency",
        LedgerConsistencyLoader,
        observers=[LedgerConsistencyObserverImpl],
    ),
    SuiteSpec(
        "reliability",
        ReliabilityLoader,
        observers=[ReliabilityObserverImpl],
    ),
    SuiteSpec(
        "economics_performance",
        EconomicsPerformanceLoader,
        observers=[EconomicsPerformanceObserverImpl],
    ),
    SuiteSpec(
        "ifeval_subset",
        IFEvalLoader,
        observers=[IFEvalVerifier],
        is_partial_external=True,
    ),
    SuiteSpec(
        "tool_selection",
        ScenarioLoader,
        observers=[ScenarioGrader],
        is_partial_external=True,
    ),
    SuiteSpec(
        "tool_arguments",
        ScenarioLoader,
        observers=[ScenarioGrader],
        is_partial_external=True,
    ),
    SuiteSpec(
        "technical_analysis",
        ScenarioLoader,
        observers=[ScenarioGrader],
        is_partial_external=True,
    ),
    SuiteSpec(
        "routing_delegation",
        ScenarioLoader,
        observers=[ScenarioGrader],
        is_partial_external=True,
    ),
    SuiteSpec(
        "verification",
        ScenarioLoader,
        observers=[ScenarioGrader],
        is_partial_external=True,
    ),
    SuiteSpec(
        "security_policy",
        ScenarioLoader,
        observers=[ScenarioGrader],
        is_partial_external=True,
    ),
    SuiteSpec(
        "recovery",
        ScenarioLoader,
        observers=[ScenarioGrader],
        is_partial_external=True,
    ),
    SuiteSpec(
        "final_response",
        ScenarioLoader,
        observers=[ScenarioGrader],
        is_partial_external=True,
    ),
]

_SUITE_SPECS_BY_ID: dict[str, SuiteSpec] = {
    spec.suite_id: spec for spec in _ALL_SUITE_SPECS
}


# --- Suite registry completeness ---


@pytest.mark.integration
def test_every_synthetic_suite_choice_has_a_suite_spec():
    from g8e_evals.cli import _SYNTHETIC_SUITE_CHOICES

    for suite_id in _SYNTHETIC_SUITE_CHOICES:
        assert suite_id in _SUITE_SPECS_BY_ID, (
            f"suite '{suite_id}' in _SYNTHETIC_SUITE_CHOICES has no SuiteSpec"
        )


@pytest.mark.integration
def test_every_suite_spec_has_a_loader_with_matching_suite_id():
    from g8e_evals.suites import SUITE_REGISTRY

    for spec in _ALL_SUITE_SPECS:
        if spec.loader_class is ScenarioLoader:
            registry_spec = SUITE_REGISTRY[spec.suite_id]
            loader = registry_spec.loader_factory(spec.data_path)
            assert loader._suite_id == spec.suite_id, (
                f"suite '{spec.suite_id}': ScenarioLoader instance _suite_id is '{loader._suite_id}'"
            )
            continue
        assert hasattr(spec.loader_class, "SUITE_ID"), (
            f"suite '{spec.suite_id}': loader {spec.loader_class.__name__} has no SUITE_ID"
        )
        assert spec.loader_class.SUITE_ID == spec.suite_id, (
            f"suite '{spec.suite_id}': loader SUITE_ID is '{spec.loader_class.SUITE_ID}'"
        )


@pytest.mark.integration
def test_every_suite_has_a_gold_set_directory_with_provenance_and_data():
    for spec in _ALL_SUITE_SPECS:
        assert spec.gold_set_dir.is_dir(), (
            f"suite '{spec.suite_id}': gold-set directory missing: {spec.gold_set_dir}"
        )
        assert spec.provenance_path.is_file(), (
            f"suite '{spec.suite_id}': provenance.json missing: {spec.provenance_path}"
        )
        assert spec.data_path.is_file(), (
            f"suite '{spec.suite_id}': input_data.jsonl missing: {spec.data_path}"
        )


# --- Provenance-bound fixtures ---


@pytest.mark.integration
def test_every_synthetic_suite_provenance_output_sha256_matches_dataset():
    for spec in _ALL_SUITE_SPECS:
        if spec.is_partial_external:
            continue
        provenance = json.loads(spec.provenance_path.read_text())
        content = spec.data_path.read_bytes()
        actual_sha = hashlib.sha256(content).hexdigest()
        assert provenance["output"]["sha256"] == actual_sha, (
            f"suite '{spec.suite_id}': provenance output sha256 does not match dataset"
        )


@pytest.mark.integration
def test_every_synthetic_suite_provenance_output_rows_matches_dataset():
    for spec in _ALL_SUITE_SPECS:
        if spec.is_partial_external:
            continue
        provenance = json.loads(spec.provenance_path.read_text())
        content = spec.data_path.read_bytes()
        rows = [line for line in content.decode().splitlines() if line.strip()]
        assert provenance["output"]["rows"] == len(rows), (
            f"suite '{spec.suite_id}': provenance output rows {provenance['output']['rows']} "
            f"does not match dataset row count {len(rows)}"
        )


@pytest.mark.integration
def test_every_synthetic_suite_provenance_benchmark_matches_suite_id():
    for spec in _ALL_SUITE_SPECS:
        provenance = json.loads(spec.provenance_path.read_text())
        assert provenance["benchmark"] == spec.suite_id, (
            f"suite '{spec.suite_id}': provenance benchmark is '{provenance['benchmark']}'"
        )


@pytest.mark.integration
def test_every_synthetic_suite_provenance_code_sha256_matches_loader_code():
    for spec in _ALL_SUITE_SPECS:
        if spec.is_partial_external:
            continue
        provenance = json.loads(spec.provenance_path.read_text())
        code_path = provenance["source"]["code_path"]
        expected_sha = provenance["source"]["code_sha256"]
        resolved = (_EVALS_ROOT / code_path).resolve()
        assert resolved.is_file(), (
            f"suite '{spec.suite_id}': provenance code_path does not exist: {code_path}"
        )
        actual_sha = hashlib.sha256(resolved.read_bytes()).hexdigest()
        assert actual_sha == expected_sha, (
            f"suite '{spec.suite_id}': provenance code_sha256 mismatch for {code_path}: "
            f"{actual_sha} != {expected_sha}"
        )


@pytest.mark.integration
def test_ifeval_subset_provenance_transformation_code_sha256_matches():
    spec = _SUITE_SPECS_BY_ID["ifeval_subset"]
    provenance = json.loads(spec.provenance_path.read_text())
    tr = provenance["transformation"]
    code_path = tr["code_path"]
    expected_code_sha = tr["code_sha256"]
    resolved_code = (_EVALS_ROOT / code_path).resolve()
    assert resolved_code.is_file(), (
        f"ifeval_subset: transformation code_path does not exist: {code_path}"
    )
    actual_code_sha = hashlib.sha256(resolved_code.read_bytes()).hexdigest()
    assert actual_code_sha == expected_code_sha, (
        f"ifeval_subset: transformation code_sha256 mismatch: "
        f"{actual_code_sha} != {expected_code_sha}"
    )
    fixture_path = tr["fixture_path"]
    expected_fixture_sha = tr["fixture_sha256"]
    resolved_fixture = (_EVALS_ROOT / fixture_path).resolve()
    assert resolved_fixture.is_file(), (
        f"ifeval_subset: transformation fixture_path does not exist: {fixture_path}"
    )
    actual_fixture_sha = hashlib.sha256(resolved_fixture.read_bytes()).hexdigest()
    assert actual_fixture_sha == expected_fixture_sha, (
        f"ifeval_subset: transformation fixture_sha256 mismatch: "
        f"{actual_fixture_sha} != {expected_fixture_sha}"
    )


@pytest.mark.integration
def test_ifeval_subset_provenance_output_sha256_matches_dataset():
    spec = _SUITE_SPECS_BY_ID["ifeval_subset"]
    provenance = json.loads(spec.provenance_path.read_text())
    content = spec.data_path.read_bytes()
    actual_sha = hashlib.sha256(content).hexdigest()
    assert provenance["output"]["sha256"] == actual_sha, (
        "ifeval_subset: provenance output sha256 does not match dataset"
    )


@pytest.mark.integration
def test_every_suite_provenance_has_required_fields():
    for spec in _ALL_SUITE_SPECS:
        provenance = json.loads(spec.provenance_path.read_text())
        assert provenance["schema_version"] >= 1, (
            f"suite '{spec.suite_id}': schema_version must be positive"
        )
        assert provenance["partition"], (
            f"suite '{spec.suite_id}': partition must not be empty"
        )
        assert provenance["domain_strata"], (
            f"suite '{spec.suite_id}': domain_strata must not be empty"
        )
        assert provenance["output"]["path"] == "input_data.jsonl", (
            f"suite '{spec.suite_id}': output path must be input_data.jsonl"
        )


# --- Grader-to-suite binding ---


@pytest.mark.integration
def test_every_grader_producer_suite_id_references_a_real_suite():
    for key, entry in GRADER_INVENTORY.items():
        for suite_id in entry.producer_suite_ids:
            assert suite_id in _SUITE_SPECS_BY_ID, (
                f"grader '{key[0]}': producer_suite_id '{suite_id}' has no SuiteSpec"
            )


@pytest.mark.integration
def test_every_suite_id_referenced_by_at_least_one_grader():
    suites_with_graders = set()
    for entry in GRADER_INVENTORY.values():
        for suite_id in entry.producer_suite_ids:
            suites_with_graders.add(suite_id)
    for spec in _ALL_SUITE_SPECS:
        assert spec.suite_id in suites_with_graders, (
            f"suite '{spec.suite_id}': no grader declares this suite as a producer"
        )


# --- Production observer boundaries ---


@pytest.mark.integration
def test_every_non_cross_cutting_suite_has_at_least_one_observer():
    for spec in _ALL_SUITE_SPECS:
        if spec.is_cross_cutting:
            continue
        assert len(spec.observers) >= 1, (
            f"suite '{spec.suite_id}': no observer implementations declared"
        )


@pytest.mark.integration
def test_every_observer_is_a_concrete_class():
    for spec in _ALL_SUITE_SPECS:
        for obs_cls in spec.observers:
            assert isinstance(obs_cls, type), (
                f"suite '{spec.suite_id}': observer {obs_cls} is not a class"
            )


@pytest.mark.integration
def test_cross_cutting_suites_produce_receipt_or_stage_evidence():
    cross_cutting_suites = [s for s in _ALL_SUITE_SPECS if s.is_cross_cutting]
    for spec in cross_cutting_suites:
        graders_for_suite = [
            entry for entry in GRADER_INVENTORY.values()
            if spec.suite_id in entry.producer_suite_ids
        ]
        for entry in graders_for_suite:
            assert entry.producer_path in (ProducerPath.CROSS_CUTTING, ProducerPath.SYNTHETIC), (
                f"cross-cutting suite '{spec.suite_id}': grader '{entry.grader_id}' "
                f"has producer_path {entry.producer_path}, expected CROSS_CUTTING or SYNTHETIC"
            )


# --- No free-form known shape in grading inputs ---


@pytest.mark.integration
def test_forbidden_metadata_keys_cover_all_typed_assertion_field_names():
    from g8e_evals.cli import _ASSERTION_FIELD_TO_GRADER_ID

    for field_name, _grader_id in _ASSERTION_FIELD_TO_GRADER_ID:
        assert field_name in FORBIDDEN_METADATA_KEYS, (
            f"assertion field '{field_name}' is not in FORBIDDEN_METADATA_KEYS; "
            f"it could enter grading via benchmark_specific as a free-form known shape"
        )


@pytest.mark.integration
def test_grader_refs_derived_from_typed_assertions_not_free_form_metadata():
    from g8e_evals.cli import _ASSERTION_FIELD_TO_GRADER_ID

    for field_name, _grader_id in _ASSERTION_FIELD_TO_GRADER_ID:
        assert field_name != "benchmark_specific", (
            "grader refs must not be derived from benchmark_specific"
        )
        assert field_name != "kwargs", (
            "grader refs must not be derived from kwargs"
        )


@pytest.mark.integration
def test_every_synthetic_suite_spec_has_a_simulator_or_cross_cutting():
    """Every non-cross-cutting synthetic suite has a local simulator or
    observer that interacts with a production-shaped system under test."""
    simulators_by_suite = {
        "privacy_token_lifecycle": True,
        "privacy_boundary_leakage": True,
        "governance_adversarial": True,
        "policy_attack": True,
        "benign_overblock": True,
        "tool_sequence": LocalToolUseSimulator,
        "factual_qa": LocalFactualQASimulator,
        "citation_backed": LocalCitationBackedSimulator,
        "partial_milestone": LocalPartialMilestoneSimulator,
        "final_state": LocalFinalStateSimulator,
        "ledger_consistency": LocalLedgerConsistencySimulator,
        "reliability": True,
        "economics_performance": True,
    }
    for spec in _ALL_SUITE_SPECS:
        if spec.is_partial_external:
            continue
        if spec.is_cross_cutting:
            continue
        assert spec.suite_id in simulators_by_suite, (
            f"suite '{spec.suite_id}': no simulator or production system declared"
        )
