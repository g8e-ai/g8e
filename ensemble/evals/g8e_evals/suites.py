# Copyright (c) 2026 Lateralus Labs, LLC.
# Use of this source code is governed by the Business Source License
# included in the LICENSE file.
#
# As of the Change Date listed in the LICENSE file, this software is
# released under the Apache License, Version 2.0.

"""Typed suite registry and execution-path classification.

Each suite declares its loader, provenance loader, deterministic grader,
compatible arms, required observers, whether candidate model inference
is part of the measured path, and default gold-set location. The
registry replaces inline if/elif suite branches in the CLI so adding a
new suite is a single registration rather than editing multiple command
functions.

Suite execution class determines model-comparison eligibility:

- ``REAL_MODEL``: The selected model receives the task and matching
  model-boundary telemetry is recorded. Eligible for model comparisons.
- ``REAL_SYSTEM``: The live g8ee/g8e path receives the task and the
  configured model-tier calls are observed. Eligible for model
  comparisons.
- ``DETERMINISTIC_SIMULATION``: A local simulator produces observations
  without calling the candidate model. Excluded from model rankings.

Only ``REAL_MODEL`` and ``REAL_SYSTEM`` suites contribute to model
comparisons. ``DETERMINISTIC_SIMULATION`` suites remain valuable for
protocol, grader, and evidence validation but are excluded from model
rankings. The ``run`` and ``campaign`` commands reject
``DETERMINISTIC_SIMULATION`` suites; the ``bench-synthetic`` command
rejects ``REAL_MODEL`` and ``REAL_SYSTEM`` suites.
"""

from __future__ import annotations

from collections.abc import Callable
from dataclasses import dataclass
from enum import StrEnum
from pathlib import Path
from typing import Any

from g8e_evals.arms import ALL_ARMS, Arm, GOVERNED_ARMS
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
from g8e_evals.benchmarks.ifeval.provenance import load_provenance as load_ifeval_provenance
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
from g8e_evals.benchmarks.privacy.provenance import load_provenance as load_synthetic_provenance
from g8e_evals.benchmarks.reliability.loader import ReliabilityLoader
from g8e_evals.benchmarks.reliability.observers import ReliabilityObserverImpl
from g8e_evals.benchmarks.utility.citation_backed_loader import CitationBackedLoader
from g8e_evals.benchmarks.utility.factual_qa_loader import FactualQALoader
from g8e_evals.benchmarks.utility.final_state_loader import FinalStateLoader
from g8e_evals.benchmarks.utility.ledger_consistency_loader import LedgerConsistencyLoader
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


class SuiteExecutionClass(StrEnum):
    """Classifies a suite by whether candidate model inference is part of the measured path.

    ``REAL_MODEL``: The selected model receives the task and matching
    model-boundary telemetry is recorded. This is the primary
    model-to-model quality comparison because it minimizes g8ee
    orchestration effects.

    ``REAL_SYSTEM``: The live g8ee/g8e path receives the task and the
    configured model-tier calls are observed. This measures practical
    suitability for a g8ee model tier.

    ``DETERMINISTIC_SIMULATION``: A local simulator produces observations
    without calling the candidate model. Excluded from model rankings
    but valuable for protocol, grader, and evidence validation.
    """

    REAL_MODEL = "real_model"
    REAL_SYSTEM = "real_system"
    DETERMINISTIC_SIMULATION = "deterministic_simulation"


_MODEL_COMPARISON_CLASSES: frozenset[SuiteExecutionClass] = frozenset({
    SuiteExecutionClass.REAL_MODEL,
    SuiteExecutionClass.REAL_SYSTEM,
})

_SYNTHETIC_ARMS: frozenset[Arm] = frozenset({Arm.DIRECT, *GOVERNED_ARMS})


@dataclass(frozen=True)
class SuiteSpec:
    """Typed specification for one benchmark suite.

    Binds the suite ID to its loader factory, provenance loader,
    deterministic grader factory, compatible arms, required observers,
    execution class, and default gold-set location. The registry
    replaces inline if/elif suite branches in the CLI.

    Attributes:
        suite_id: Unique suite identifier (matches the loader's ``SUITE_ID``).
        description: Human-readable summary of what the suite measures.
        execution_class: Whether candidate model inference is part of
            the measured path.
        compatible_arms: Arms that can run this suite.
        default_gold_set: Default gold-set input file path relative to
            the evals package root.
        loader_factory: Callable that takes a gold-set Path and returns
            a loader object with a ``load()`` method yielding ``Task``
            objects.
        provenance_loader: Callable that takes a provenance Path and
            returns a provenance object with ``benchmark``, ``output``,
            ``partition``, and ``domain_strata`` attributes.
        grader_factory: Callable that returns a deterministic grader
            instance, or ``None`` for synthetic suites whose grading is
            done through the observer/simulator pipeline.
        required_observers: Tuple of concrete observer implementation
            classes that produce typed observations for the suite's
            graders. Empty for real-model suites that use a single
            deterministic grader.
    """

    suite_id: str
    description: str
    execution_class: SuiteExecutionClass
    compatible_arms: frozenset[Arm]
    default_gold_set: Path
    loader_factory: Callable[[Path], Any]
    provenance_loader: Callable[[Path], Any]
    grader_factory: Callable[[], Any] | None
    required_observers: tuple[type, ...]

    @property
    def is_model_comparison(self) -> bool:
        """Whether this suite contributes to model comparisons.

        ``REAL_MODEL`` and ``REAL_SYSTEM`` suites are eligible;
        ``DETERMINISTIC_SIMULATION`` suites are excluded from model
        rankings.
        """
        return self.execution_class in _MODEL_COMPARISON_CLASSES


SUITE_REGISTRY: dict[str, SuiteSpec] = {}


def register_suite(spec: SuiteSpec) -> None:
    """Register a suite specification. Rejects duplicate suite IDs."""
    if spec.suite_id in SUITE_REGISTRY:
        raise ValueError(f"duplicate suite registration: {spec.suite_id}")
    SUITE_REGISTRY[spec.suite_id] = spec


def get_suite(suite_id: str) -> SuiteSpec:
    """Return the suite specification for ``suite_id``.

    Raises ``KeyError`` if the suite is not registered.
    """
    return SUITE_REGISTRY[suite_id]


def get_model_comparison_suites() -> list[SuiteSpec]:
    """Return all registered suites eligible for model comparisons."""
    return [s for s in SUITE_REGISTRY.values() if s.is_model_comparison]


def get_simulation_suites() -> list[SuiteSpec]:
    """Return all registered deterministic-simulation suites."""
    return [s for s in SUITE_REGISTRY.values() if s.execution_class is SuiteExecutionClass.DETERMINISTIC_SIMULATION]


def get_suite_ids_by_class(execution_class: SuiteExecutionClass) -> list[str]:
    """Return sorted suite IDs for the given execution class."""
    return sorted(s.suite_id for s in SUITE_REGISTRY.values() if s.execution_class is execution_class)


def assert_model_comparison_eligible(suite_id: str) -> SuiteSpec:
    """Return the suite spec if it is eligible for model comparisons.

    Raises ``ValueError`` if the suite is not registered or is a
    deterministic simulation excluded from model rankings. This is the
    contract gate: a suite from the model-comparison set must have a
    real model or real system execution path so a provider-boundary call
    can be bound to the selected candidate.
    """
    spec = SUITE_REGISTRY.get(suite_id)
    if spec is None:
        raise ValueError(f"unknown suite: {suite_id}")
    if not spec.is_model_comparison:
        raise ValueError(
            f"suite '{suite_id}' is a {spec.execution_class.value} suite and "
            f"cannot be used for model comparisons; only REAL_MODEL and "
            f"REAL_SYSTEM suites are eligible"
        )
    return spec


def assert_simulation_eligible(suite_id: str) -> SuiteSpec:
    """Return the suite spec if it is a deterministic simulation.

    Raises ``ValueError`` if the suite is not registered or is a
    real-model/real-system suite that requires model inference.
    """
    spec = SUITE_REGISTRY.get(suite_id)
    if spec is None:
        raise ValueError(f"unknown suite: {suite_id}")
    if spec.execution_class is not SuiteExecutionClass.DETERMINISTIC_SIMULATION:
        raise ValueError(
            f"suite '{suite_id}' is a {spec.execution_class.value} suite and "
            f"cannot be used with bench-synthetic; only DETERMINISTIC_SIMULATION "
            f"suites are eligible"
        )
    return spec


# --- Suite registrations ---

_GOLD_SETS = Path("gold_sets")

# Real model benchmark: the selected model receives the task directly and
# matching model-boundary telemetry is recorded.
register_suite(SuiteSpec(
    suite_id="ifeval_subset",
    description=(
        "IFEval instruction-following subset (5 tasks, deterministic grader). "
        "The selected model receives the task directly; model-boundary "
        "telemetry is recorded. Suitable for smoke and pipeline-integrity "
        "diagnostics, not statistically persuasive leaderboards."
    ),
    execution_class=SuiteExecutionClass.REAL_MODEL,
    compatible_arms=frozenset(ALL_ARMS),
    default_gold_set=_GOLD_SETS / "ifeval_subset" / "input_data.jsonl",
    loader_factory=lambda p: IFEvalLoader(p),
    provenance_loader=load_ifeval_provenance,
    grader_factory=lambda: IFEvalVerifier(),
    required_observers=(),
))

# Deterministic simulation suites: a local simulator produces observations
# without calling the candidate model. Excluded from model rankings.

register_suite(SuiteSpec(
    suite_id="privacy_token_lifecycle",
    description="Synthetic privacy token lifecycle suite (encrypted token store, TTL, persistence).",
    execution_class=SuiteExecutionClass.DETERMINISTIC_SIMULATION,
    compatible_arms=_SYNTHETIC_ARMS,
    default_gold_set=_GOLD_SETS / "privacy_token_lifecycle" / "input_data.jsonl",
    loader_factory=lambda p: PrivacyTokenLifecycleLoader(p),
    provenance_loader=load_synthetic_provenance,
    grader_factory=None,
    required_observers=(
        TokenStorePersistenceObserverImpl,
        TokenTTLExpiryObserverImpl,
        TokenPersistenceFailureObserverImpl,
        ExfiltrationAttemptObserverImpl,
    ),
))

register_suite(SuiteSpec(
    suite_id="privacy_boundary_leakage",
    description="Synthetic privacy boundary leakage suite (artifact leakage, rehydration).",
    execution_class=SuiteExecutionClass.DETERMINISTIC_SIMULATION,
    compatible_arms=_SYNTHETIC_ARMS,
    default_gold_set=_GOLD_SETS / "privacy_boundary_leakage" / "input_data.jsonl",
    loader_factory=lambda p: PrivacyBoundaryLeakageLoader(p),
    provenance_loader=load_synthetic_provenance,
    grader_factory=None,
    required_observers=(
        ArtifactLeakageObserverImpl,
        RehydrationObserverImpl,
    ),
))

register_suite(SuiteSpec(
    suite_id="governance_adversarial",
    description="Synthetic governance adversarial suite (replay, tampering, nonce, stale state).",
    execution_class=SuiteExecutionClass.DETERMINISTIC_SIMULATION,
    compatible_arms=_SYNTHETIC_ARMS,
    default_gold_set=_GOLD_SETS / "governance_adversarial" / "input_data.jsonl",
    loader_factory=lambda p: GovernanceAdversarialLoader(p),
    provenance_loader=load_synthetic_provenance,
    grader_factory=None,
    required_observers=(
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
    ),
))

register_suite(SuiteSpec(
    suite_id="policy_attack",
    description="Synthetic policy attack suite (adversarial policy bypass).",
    execution_class=SuiteExecutionClass.DETERMINISTIC_SIMULATION,
    compatible_arms=_SYNTHETIC_ARMS,
    default_gold_set=_GOLD_SETS / "policy_attack" / "input_data.jsonl",
    loader_factory=lambda p: PolicyAttackLoader(p),
    provenance_loader=load_synthetic_provenance,
    grader_factory=None,
    required_observers=(PolicyAttackObserverImpl,),
))

register_suite(SuiteSpec(
    suite_id="benign_overblock",
    description="Synthetic benign overblock suite (false-positive governance blocking).",
    execution_class=SuiteExecutionClass.DETERMINISTIC_SIMULATION,
    compatible_arms=_SYNTHETIC_ARMS,
    default_gold_set=_GOLD_SETS / "benign_overblock" / "input_data.jsonl",
    loader_factory=lambda p: BenignOverblockLoader(p),
    provenance_loader=load_synthetic_provenance,
    grader_factory=None,
    required_observers=(),
))

register_suite(SuiteSpec(
    suite_id="tool_sequence",
    description="Synthetic tool sequence suite (tool-call ordering and typed output).",
    execution_class=SuiteExecutionClass.DETERMINISTIC_SIMULATION,
    compatible_arms=_SYNTHETIC_ARMS,
    default_gold_set=_GOLD_SETS / "utility" / "input_data.jsonl",
    loader_factory=lambda p: ToolSequenceLoader(p),
    provenance_loader=load_synthetic_provenance,
    grader_factory=None,
    required_observers=(ToolSequenceObserverImpl,),
))

register_suite(SuiteSpec(
    suite_id="factual_qa",
    description="Synthetic factual QA suite (citation-backed factual accuracy).",
    execution_class=SuiteExecutionClass.DETERMINISTIC_SIMULATION,
    compatible_arms=_SYNTHETIC_ARMS,
    default_gold_set=_GOLD_SETS / "factual_qa" / "input_data.jsonl",
    loader_factory=lambda p: FactualQALoader(p),
    provenance_loader=load_synthetic_provenance,
    grader_factory=None,
    required_observers=(FactualQAObserverImpl,),
))

register_suite(SuiteSpec(
    suite_id="citation_backed",
    description="Synthetic citation-backed suite (source citation verification).",
    execution_class=SuiteExecutionClass.DETERMINISTIC_SIMULATION,
    compatible_arms=_SYNTHETIC_ARMS,
    default_gold_set=_GOLD_SETS / "citation_backed" / "input_data.jsonl",
    loader_factory=lambda p: CitationBackedLoader(p),
    provenance_loader=load_synthetic_provenance,
    grader_factory=None,
    required_observers=(CitationBackedObserverImpl,),
))

register_suite(SuiteSpec(
    suite_id="partial_milestone",
    description="Synthetic partial milestone suite (intermediate progress verification).",
    execution_class=SuiteExecutionClass.DETERMINISTIC_SIMULATION,
    compatible_arms=_SYNTHETIC_ARMS,
    default_gold_set=_GOLD_SETS / "partial_milestone" / "input_data.jsonl",
    loader_factory=lambda p: PartialMilestoneLoader(p),
    provenance_loader=load_synthetic_provenance,
    grader_factory=None,
    required_observers=(PartialMilestoneObserverImpl,),
))

register_suite(SuiteSpec(
    suite_id="final_state",
    description="Synthetic final state suite (terminal state verification).",
    execution_class=SuiteExecutionClass.DETERMINISTIC_SIMULATION,
    compatible_arms=_SYNTHETIC_ARMS,
    default_gold_set=_GOLD_SETS / "final_state" / "input_data.jsonl",
    loader_factory=lambda p: FinalStateLoader(p),
    provenance_loader=load_synthetic_provenance,
    grader_factory=None,
    required_observers=(FinalStateObserverImpl,),
))

register_suite(SuiteSpec(
    suite_id="ledger_consistency",
    description="Synthetic ledger consistency suite (audit trail integrity).",
    execution_class=SuiteExecutionClass.DETERMINISTIC_SIMULATION,
    compatible_arms=_SYNTHETIC_ARMS,
    default_gold_set=_GOLD_SETS / "ledger_consistency" / "input_data.jsonl",
    loader_factory=lambda p: LedgerConsistencyLoader(p),
    provenance_loader=load_synthetic_provenance,
    grader_factory=None,
    required_observers=(LedgerConsistencyObserverImpl,),
))

register_suite(SuiteSpec(
    suite_id="reliability",
    description="Synthetic reliability suite (fault tolerance and recovery).",
    execution_class=SuiteExecutionClass.DETERMINISTIC_SIMULATION,
    compatible_arms=_SYNTHETIC_ARMS,
    default_gold_set=_GOLD_SETS / "reliability" / "input_data.jsonl",
    loader_factory=lambda p: ReliabilityLoader(p),
    provenance_loader=load_synthetic_provenance,
    grader_factory=None,
    required_observers=(ReliabilityObserverImpl,),
))

register_suite(SuiteSpec(
    suite_id="economics_performance",
    description="Synthetic economics performance suite (cost and throughput).",
    execution_class=SuiteExecutionClass.DETERMINISTIC_SIMULATION,
    compatible_arms=_SYNTHETIC_ARMS,
    default_gold_set=_GOLD_SETS / "economics_performance" / "input_data.jsonl",
    loader_factory=lambda p: EconomicsPerformanceLoader(p),
    provenance_loader=load_synthetic_provenance,
    grader_factory=None,
    required_observers=(EconomicsPerformanceObserverImpl,),
))
