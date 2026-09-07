# Copyright (c) 2026 Lateralus Labs, LLC.
# Use of this source code is governed by the Business Source License
# included in the LICENSE file.
#
# As of the Change Date listed in the LICENSE file, this software is
# released under the Apache License, Version 2.0.

import asyncio
import binascii
import hashlib
import json
import logging
import os
import platform
import sys
import uuid
from collections.abc import Sequence
from dataclasses import asdict, dataclass
from datetime import UTC, datetime
from pathlib import Path
from typing import Protocol

import click
from rich.console import Console

from app.constants import LLMProvider
from app.llm.factory import get_llm_provider
from app.models.settings import LLMSettings
from app.services.ai.eval_judge import EvalJudge, EvalJudgeError
from g8e.constants import PORTS
from g8e.operator.v1.operator_pb2 import (
    DETERMINISTIC_STAGE_KIND_L4_VERIFICATION,
    DETERMINISTIC_STAGE_OUTCOME_FAILED,
)
from g8e.receipts import (
    action_receipt_to_dict,
    decode_ed25519_public_key,
    verify_action_receipt_signature,
    verify_receipt_persistence_attestation,
)
from g8e_evals import __version__ as EVALS_VERSION
from g8e_evals.arms import ALL_ARMS, GOVERNED_ARMS, Arm, GovernancePosture
from g8e_evals.auth_bridge import AuthBridgeError, load_cli_auth_context
from g8e_evals.graders import (
    DeterministicGradingContext,
    grade_deterministically,
    observe_receipt_final_state,
)
from g8e_evals.evidence import EvidenceEncryptionKey, encrypt_evidence_artifact, load_evidence_encryption_key
from g8e_evals.harness import BindingType, LLMRoleConfig, ReceiptEvidence, RowResult, SUTConfig
from g8e_evals.stages import EvidenceArtifact, normalize_attempt_evidence
from g8e_evals.schema import (
    ArmManifestEntry,
    ArtifactLeakageObservation,
    AttemptRecord,
    ContentHash,
    EvidenceIndex,
    EvidenceMediaType,
    ExfiltrationAttemptObservation,
    FinalStateObservation,
    GraderClass,
    GraderReference,
    MetricObservation,
    ModelIdentity,
    PolicyOutcome,
    PostureObservation,
    PayloadTamperingObservation,
    PrivacyClassification,
    ReceiptObservation,
    RehydrationObservation,
    ReplayAttemptObservation,
    RoleToModelMapping,
    RunManifest,
    SecretDetectionObservation,
    SignedFieldTamperingObservation,
    StaleStateRootObservation,
    IdentityMismatchObservation,
    NonceExpirationObservation,
    SignerDefectObservation,
    L3ProofTransplantObservation,
    RevokedCredentialObservation,
    EvidencePreservationObservation,
    PolicyAttackObservation,
    StackEnvironment,
    ToolSequenceObservation,
    FactualQAObservation,
    CitationBackedObservation,
    PartialMilestoneObservation,
    ReliabilityObservation,
    EconomicsPerformanceObservation,
    StateObservation,
    StageObservation,
    TaskDefinition,
    TerminalStatus,
    UnauthorizedMutationObservation,
    TokenStorePersistenceObservation,
    TokenTTLExpiryObservation,
    TokenPersistenceFailureObservation,
    VerificationStatus,
)
from g8e_evals.metrics import DEFAULT_METRIC_REGISTRY
from g8e_evals.sut.direct_provider import DirectProviderSUT
from g8e_evals.sut.g8ee_chat import ChatEvaluationReceipt, G8eeChatSUT, AuthenticationError
from g8e_evals.posture import observe_gateway_posture
from g8e_evals.transport import AuthContext
from g8e_evals.agent_trail_renderer import TurnRenderer
from g8e_evals.benchmarks.ifeval.loader import IFEvalLoader
from g8e_evals.benchmarks.ifeval.provenance import load_provenance
from g8e_evals.benchmarks.ifeval.verifier import IFEvalVerifier
from g8e_evals.benchmarks.privacy.loader import PrivacyBoundaryLeakageLoader, PrivacyTokenLifecycleLoader
from g8e_evals.benchmarks.privacy.observers import (
    ArtifactLeakageObserverImpl,
    ExfiltrationAttemptObserverImpl,
    RehydrationObserverImpl,
    TokenPersistenceFailureObserverImpl,
    TokenStorePersistenceObserverImpl,
    TokenTTLExpiryObserverImpl,
)
from g8e_evals.benchmarks.privacy.provenance import load_provenance as load_synthetic_provenance
from g8e_evals.benchmarks.privacy.token_store import LocalEncryptedTokenStore, LocalRehydrationArtifact, TokenEntry
from g8e_evals.benchmarks.privacy.artifact_emitter import LocalArtifactEmitter
from g8e_evals.benchmarks.privacy.exfiltration import LocalExfiltrationSimulator
from g8e_evals.benchmarks.governance.benign_overblock_loader import BenignOverblockLoader
from g8e_evals.benchmarks.governance.loader import GovernanceAdversarialLoader
from g8e_evals.benchmarks.governance.policy_attack_loader import PolicyAttackLoader
from g8e_evals.benchmarks.governance.observers import (
    L3ProofTransplantObserverImpl,
    NonceExpirationObserverImpl,
    PolicyAttackObserverImpl,
    ReplayAttemptObserverImpl,
    RevokedCredentialObserverImpl,
    SignedFieldTamperingObserverImpl,
    SignerDefectObserverImpl,
    StaleStateRootObserverImpl,
    PayloadTamperingObserverImpl,
    IdentityMismatchObserverImpl,
    EvidencePreservationObserverImpl,
)
from g8e_evals.benchmarks.governance.simulator import LocalGovernanceSimulator
from g8e_evals.benchmarks.utility.citation_backed_loader import CitationBackedLoader
from g8e_evals.benchmarks.utility.citation_backed_simulator import LocalCitationBackedSimulator
from g8e_evals.benchmarks.utility.factual_qa_loader import FactualQALoader
from g8e_evals.benchmarks.utility.factual_qa_simulator import LocalFactualQASimulator
from g8e_evals.benchmarks.utility.final_state_loader import FinalStateLoader
from g8e_evals.benchmarks.utility.final_state_simulator import LocalFinalStateSimulator
from g8e_evals.benchmarks.utility.ledger_consistency_loader import LedgerConsistencyLoader
from g8e_evals.benchmarks.utility.ledger_consistency_simulator import LocalLedgerConsistencySimulator
from g8e_evals.benchmarks.utility.partial_milestone_loader import PartialMilestoneLoader
from g8e_evals.benchmarks.utility.partial_milestone_simulator import LocalPartialMilestoneSimulator
from g8e_evals.benchmarks.utility.loader import ToolSequenceLoader
from g8e_evals.benchmarks.utility.observers import (
    CitationBackedObserverImpl,
    FactualQAObserverImpl,
    FinalStateObserverImpl,
    LedgerConsistencyObserverImpl,
    PartialMilestoneObserverImpl,
    ToolSequenceObserverImpl,
)
from g8e_evals.benchmarks.utility.tool_use_simulator import LocalToolUseSimulator
from g8e_evals.benchmarks.reliability.loader import ReliabilityLoader
from g8e_evals.benchmarks.reliability.simulator import LocalReliabilitySimulator
from g8e_evals.benchmarks.reliability.observers import ReliabilityObserverImpl
from g8e_evals.benchmarks.economics.loader import EconomicsPerformanceLoader
from g8e_evals.benchmarks.economics.simulator import LocalEconomicsPerformanceSimulator
from g8e_evals.benchmarks.economics.observers import EconomicsPerformanceObserverImpl
from g8e_evals.receipts.collector import ReceiptCollector
from g8e_evals.receipts.verify import receipt_action_type
from g8e_evals.report.aggregate import aggregate_results
from g8e_evals.report.cli_renderer import render_summary
from g8e_evals.models import ScoreDetails, TaskMetadata

console = Console()
logger = logging.getLogger(__name__)

_PROVIDER_CHOICES = ["openai", "anthropic", "gemini", "ollama", "llamacpp", "fake"]
_KEYLESS_PROVIDERS = frozenset({"ollama", "llamacpp", "fake"})
_IFEVAL_GRADER_ID = "ifeval_subset_verifier"
_EVAL_JUDGE_GRADER_ID = "eval_judge"
_RECEIPT_INTEGRITY_GRADER_ID = "receipt_integrity"
_PROTOCOL_CHAIN_GRADER_ID = "protocol_chain"
_CANARY_SCRUBBING_GRADER_ID = "canary_scrubbing"
_MODEL_BOUNDARY_RAW_SECRET_GRADER_ID = "model_boundary_raw_secret_rate"
_EXACT_LOCAL_REHYDRATION_GRADER_ID = "exact_local_rehydration"
_SECRET_DETECTION_PRECISION_GRADER_ID = "secret_detection_precision"
_SECRET_DETECTION_RECALL_GRADER_ID = "secret_detection_recall"
_FINAL_STATE_GRADER_ID = "final_state_assertions"
_FINAL_STATE_METRIC_ID = "final_state_accuracy"
_INDEPENDENT_STATE_GRADER_ID = "independent_state"
_INDEPENDENT_STATE_METRIC_ID = "independent_state_accuracy"
_POLICY_OUTCOME_GRADER_ID = "policy_outcome"
_UNAUTHORIZED_MUTATION_GRADER_ID = "unauthorized_mutation"
_TOKEN_STORE_PERSISTENCE_GRADER_ID = "token_store_persistence"
_TOKEN_TTL_EXPIRY_GRADER_ID = "token_ttl_expiry"
_TOKEN_PERSISTENCE_FAILURE_GRADER_ID = "token_persistence_failure"
_EXFILTRATION_ATTEMPT_GRADER_ID = "exfiltration_attempt"
_ARTIFACT_LEAKAGE_GRADER_ID = "artifact_leakage"
_REPLAY_ATTEMPT_GRADER_ID = "replay_attempt"
_SIGNED_FIELD_TAMPERING_GRADER_ID = "signed_field_tampering"
_PAYLOAD_TAMPERING_GRADER_ID = "payload_tampering"
_STALE_STATE_ROOT_GRADER_ID = "stale_state_root"
_IDENTITY_MISMATCH_GRADER_ID = "identity_mismatch"
_NONCE_EXPIRATION_GRADER_ID = "nonce_expiration"
_SIGNER_DEFECT_GRADER_ID = "signer_defect"
_L3_PROOF_TRANSPLANT_GRADER_ID = "l3_proof_transplant"
_REVOKED_CREDENTIAL_GRADER_ID = "revoked_credential"
_EVIDENCE_PRESERVATION_GRADER_ID = "evidence_preservation"
_POLICY_ATTACK_GRADER_ID = "policy_attack"
_TOOL_SEQUENCE_GRADER_ID = "tool_sequence"
_FACTUAL_QA_GRADER_ID = "factual_qa"
_CITATION_BACKED_GRADER_ID = "citation_backed"
_PARTIAL_MILESTONE_GRADER_ID = "partial_milestone"
_RELIABILITY_GRADER_ID = "reliability"
_ECONOMICS_PERFORMANCE_GRADER_ID = "economics_performance"
_GRADER_VERSION = "1.0.0"
_RECEIPT_VERIFICATION_SCHEMA_VERSION = "1.0.0"
_RECEIPT_VERIFIER_VERSION = "g8e-evals-verify-receipts-1.0.0"
_RECEIPT_VERIFICATION_SCOPE = "canonical receipt signatures and final-persistence attestations"
_RECEIPT_FINGERPRINT_SAMPLE_LIMIT = 3

# Maps typed assertion-list field names on TaskDefinition/TaskMetadata to the
# grader ID that grades assertions of that type.  Used to derive grader
# references from typed assertions rather than free-form benchmark_specific
# metadata.
_ASSERTION_FIELD_TO_GRADER_ID: list[tuple[str, str]] = [
    ("token_store_persistence_assertions", _TOKEN_STORE_PERSISTENCE_GRADER_ID),
    ("token_ttl_expiry_assertions", _TOKEN_TTL_EXPIRY_GRADER_ID),
    ("token_persistence_failure_assertions", _TOKEN_PERSISTENCE_FAILURE_GRADER_ID),
    ("exfiltration_attempt_assertions", _EXFILTRATION_ATTEMPT_GRADER_ID),
    ("artifact_leakage_assertions", _ARTIFACT_LEAKAGE_GRADER_ID),
    ("rehydration_assertions", _EXACT_LOCAL_REHYDRATION_GRADER_ID),
    ("replay_attempt_assertions", _REPLAY_ATTEMPT_GRADER_ID),
    ("signed_field_tampering_assertions", _SIGNED_FIELD_TAMPERING_GRADER_ID),
    ("payload_tampering_assertions", _PAYLOAD_TAMPERING_GRADER_ID),
    ("nonce_expiration_assertions", _NONCE_EXPIRATION_GRADER_ID),
    ("stale_state_root_assertions", _STALE_STATE_ROOT_GRADER_ID),
    ("identity_mismatch_assertions", _IDENTITY_MISMATCH_GRADER_ID),
    ("signer_defect_assertions", _SIGNER_DEFECT_GRADER_ID),
    ("l3_proof_transplant_assertions", _L3_PROOF_TRANSPLANT_GRADER_ID),
    ("revoked_credential_assertions", _REVOKED_CREDENTIAL_GRADER_ID),
    ("evidence_preservation_assertions", _EVIDENCE_PRESERVATION_GRADER_ID),
    ("policy_attack_assertions", _POLICY_ATTACK_GRADER_ID),
    ("tool_sequence_assertions", _TOOL_SEQUENCE_GRADER_ID),
    ("factual_qa_assertions", _FACTUAL_QA_GRADER_ID),
    ("citation_backed_assertions", _CITATION_BACKED_GRADER_ID),
    ("partial_milestone_assertions", _PARTIAL_MILESTONE_GRADER_ID),
    ("reliability_assertions", _RELIABILITY_GRADER_ID),
    ("economics_performance_assertions", _ECONOMICS_PERFORMANCE_GRADER_ID),
    ("expected_final_state_assertions", _FINAL_STATE_GRADER_ID),
    ("state_fixture", _INDEPENDENT_STATE_GRADER_ID),
]


def _derive_grader_refs_from_assertions(task_metadata: TaskMetadata) -> list[GraderReference]:
    """Derive grader references from typed assertion lists on task metadata.

    Replaces the old approach of reading grader IDs from the untyped
    ``benchmark_specific`` dict.  Each non-empty typed assertion list maps
    to exactly one deterministic grader reference.
    """
    refs: list[GraderReference] = []
    for field_name, grader_id in _ASSERTION_FIELD_TO_GRADER_ID:
        assertions = getattr(task_metadata, field_name, None)
        if assertions:
            refs.append(GraderReference(
                grader_id=grader_id,
                grader_version=_GRADER_VERSION,
                grader_class=GraderClass.DETERMINISTIC,
            ))
    return refs


class _SyntheticObservation(Protocol):
    observation_id: str
    attempt_id: str

    def model_dump_json(self) -> str: ...


def _persist_evidence_artifact(
    content: str,
    *,
    run_id: str,
    attempt_id: str,
    artifact_id: str,
    schema_ref: str,
    report_dir: Path,
) -> EvidenceIndex:
    """Persist content as a content-addressed evidence artifact.

    Writes the content to ``evidence/<sha256>.json`` and returns the
    ``EvidenceIndex`` entry.  The SHA-256 and byte length in the index
    are computed from the actual persisted bytes, not from a descriptive
    string.
    """
    content_bytes = content.encode()
    digest = hashlib.sha256(content_bytes).hexdigest()
    storage_location = f"evidence/{digest}.json"
    index = EvidenceIndex(
        artifact_id=artifact_id,
        run_id=run_id,
        attempt_id=attempt_id,
        media_type=EvidenceMediaType.APPLICATION_JSON,
        schema_ref=schema_ref,
        byte_length=len(content_bytes),
        sha256=digest,
        producer_identity="g8e-evals-synthetic",
        privacy_classification=PrivacyClassification.INTERNAL,
        storage_location=storage_location,
    )
    artifact_path = report_dir / storage_location
    artifact_path.parent.mkdir(parents=True, exist_ok=True)
    artifact_path.write_text(content)
    return index


def _resolve_and_digest_check(
    evidence_index: dict[str, EvidenceIndex],
    refs: list[str],
    expected_sha256: str,
    report_dir: Path,
) -> bool:
    """Resolve evidence references and verify their digests.

    Returns ``True`` only if every reference resolves to an EvidenceIndex
    entry whose persisted content matches the declared SHA-256 and the
    expected SHA-256 on the observation.
    """
    if not refs:
        return False
    for ref in refs:
        index = evidence_index.get(ref)
        if index is None:
            return False
        if index.sha256 != expected_sha256:
            return False
        artifact_path = report_dir / index.storage_location
        if not artifact_path.is_file():
            return False
        content_bytes = artifact_path.read_bytes()
        if hashlib.sha256(content_bytes).hexdigest() != index.sha256:
            return False
        if len(content_bytes) != index.byte_length:
            return False
    return True


def _persist_receipt_evidence(
    receipt_obs: ReceiptObservation,
    *,
    run_id: str,
    attempt_id: str,
    artifact_id: str,
    report_dir: Path,
) -> tuple[EvidenceIndex, str]:
    """Persist a receipt as a content-addressed artifact and return its index and SHA-256."""
    receipt_content = json.dumps(
        action_receipt_to_dict(receipt_obs.action_receipt),
        sort_keys=True, separators=(",", ":"), ensure_ascii=False,
    )
    index = _persist_evidence_artifact(
        receipt_content,
        run_id=run_id,
        attempt_id=attempt_id,
        artifact_id=artifact_id,
        schema_ref="g8e.operator.v1.ActionReceipt",
        report_dir=report_dir,
    )
    return index, index.sha256


def _persist_synthetic_observation(
    observation: _SyntheticObservation,
    *,
    run_id: str,
    report_dir: Path,
) -> EvidenceIndex:
    return _persist_evidence_artifact(
        observation.model_dump_json(),
        run_id=run_id,
        attempt_id=observation.attempt_id,
        artifact_id=observation.observation_id,
        schema_ref=f"g8e_evals.schema.{type(observation).__name__}",
        report_dir=report_dir,
    )


@dataclass(frozen=True)
class ReceiptFingerprint:
    receipt_id: str
    signature_digest: str
    artifact_ref: str


@dataclass(frozen=True)
class ReceiptVerificationResult:
    schema_version: str
    run_id: str
    verified_at: str
    verifier_version: str
    scope: str
    total_receipts: int
    verified_signatures: int
    verified_persistence: int
    failed_signatures: int
    failed_persistence: int
    missing_keys: int
    distinct_signer_key_ids: tuple[str, ...]
    receipt_bound_eligible_attempts: int
    sample_receipt_fingerprints: tuple[ReceiptFingerprint, ...]


class EvaluationRunError(Exception):
    pass


def _create_eval_judge(config: LLMRoleConfig) -> EvalJudge:
    if not config.provider or not config.model:
        raise EvaluationRunError("eval judge requires both provider and model")
    settings = LLMSettings(
        llm_lite_provider=LLMProvider(config.provider),
        llm_lite_model=config.model,
        lite_api_key=config.api_key,
        lite_endpoint=config.endpoint,
    )
    return EvalJudge(
        provider=get_llm_provider(settings, is_lite=True),
        model=config.model,
    )


@click.group()
def main():
    """g8e High-Fidelity AI Evaluation Harness"""
    handler = logging.StreamHandler()
    handler.setFormatter(logging.Formatter("%(asctime)s - %(name)s - %(levelname)s - %(message)s"))
    root_logger = logging.getLogger()
    root_logger.setLevel(logging.INFO)
    root_logger.addHandler(handler)

    # Silence noisy loggers
    logging.getLogger("httpx").setLevel(logging.WARNING)
    logging.getLogger("httpcore").setLevel(logging.WARNING)


@main.command()
@click.option("--suite", type=click.Choice(["ifeval_subset"]), required=True)
@click.option("--provider", type=click.Choice(_PROVIDER_CHOICES), envvar="G8E_TEST_LLM_PRIMARY_PROVIDER", help="Primary LLM provider")
@click.option("--model", envvar="G8E_TEST_LLM_PRIMARY_MODEL", help="Primary model name (e.g., gpt-4o)")
@click.option("--assistant-provider", type=click.Choice(_PROVIDER_CHOICES), envvar="G8E_TEST_LLM_ASSISTANT_PROVIDER", help="Assistant LLM provider")
@click.option("--assistant-model", envvar="G8E_TEST_LLM_ASSISTANT_MODEL", help="Assistant model name")
@click.option("--lite-provider", type=click.Choice(_PROVIDER_CHOICES), envvar="G8E_TEST_LLM_LITE_PROVIDER", help="Lite LLM provider")
@click.option("--lite-model", envvar="G8E_TEST_LLM_LITE_MODEL", help="Lite model name")
@click.option("--judge-provider", type=click.Choice(_PROVIDER_CHOICES), envvar="G8E_TEST_LLM_JUDGE_PROVIDER", help="Secondary Eval Judge provider")
@click.option("--judge-model", envvar="G8E_TEST_LLM_JUDGE_MODEL", help="Secondary Eval Judge model")
@click.option("--headless/--no-headless", default=False,
              help="Approve correlated command requests from the authenticated SSE stream")
@click.option("--verbose-text/--no-verbose-text", default=False,
              help="Stream the agent's response text inline as chunks arrive")
@click.option("--idle-timeout", type=float, default=10.0,
              help="Seconds without an SSE event before declaring a task idle")
@click.option("--g8ee-url", envvar="G8E_G8EE_URL", required=True,
              help="URL of the g8ee application endpoint.")
@click.option("--operator-url", default=f"https://localhost:{PORTS['ports']['OperatorHttps']['value']}")
@click.option("--operator-session-id", envvar="G8E_OPERATOR_SESSION_ID",
              help="Operator session id override. The default comes from the canonical CLI identity.")
@click.option("--g8e-cli", default="./g8e", envvar="G8E_CLI_BIN", show_default=True,
              help="Path to the g8e CLI used to load the canonical authentication context.")
@click.option("--auth-project-root", type=click.Path(path_type=Path, file_okay=False),
              envvar="G8E_AUTH_PROJECT_ROOT", required=True,
              help="Project root containing the canonical CLI runtime identity.")
@click.option("--arm", type=click.Choice([a.value for a in ALL_ARMS]), default=Arm.ENSEMBLE_UNGOVERNED.value,
              help="Experiment arm: direct (raw provider call), ensemble_ungoverned (g8ee without governance), doctrine (L1), consensus (L1+L2), notary (L1+L2+L3)")
@click.option("--state-root", default="test-state-root-v1")
@click.option("--output-dir", type=click.Path(path_type=Path), default=Path("reports"))
@click.option("--evidence-key-file", type=click.Path(path_type=Path, dir_okay=False), envvar="G8E_EVIDENCE_KEY_FILE",
              help='Owner-only JSON key file: {"version":1,"key_id":"...","key_b64":"<32-byte-base64>"}.')
@click.option("--gold-set", type=click.Path(exists=True, path_type=Path))
@click.option("--limit", type=int, help="Limit number of tasks to run")
@click.option("--l2-key", help="L2 private key hex")
@click.option("--l2-key-id", help="L2 key ID")
@click.option("--primary-api-key", envvar="G8E_TEST_LLM_PRIMARY_API_KEY", help="API key for the primary provider")
@click.option("--primary-endpoint", envvar="G8E_TEST_LLM_PRIMARY_ENDPOINT", help="Endpoint URL for the primary provider")
@click.option("--assistant-api-key", envvar="G8E_TEST_LLM_ASSISTANT_API_KEY", help="API key for the assistant provider")
@click.option("--assistant-endpoint", envvar="G8E_TEST_LLM_ASSISTANT_ENDPOINT", help="Endpoint URL for the assistant provider")
@click.option("--lite-api-key", envvar="G8E_TEST_LLM_LITE_API_KEY", help="API key for the lite provider")
@click.option("--lite-endpoint", envvar="G8E_TEST_LLM_LITE_ENDPOINT", help="Endpoint URL for the lite provider")
@click.option("--judge-api-key", envvar="G8E_TEST_LLM_JUDGE_API_KEY", help="API key for the Eval Judge provider")
@click.option("--judge-endpoint", envvar="G8E_TEST_LLM_JUDGE_ENDPOINT", help="Endpoint URL for the Eval Judge provider")
@click.option("--web-search-project", envvar="G8E_WEB_SEARCH_PROJECT", help="Web search project ID")
@click.option("--web-search-app", envvar="G8E_WEB_SEARCH_APP", help="Web search app ID")
@click.option("--web-search-api-key", envvar="G8E_WEB_SEARCH_API_KEY", help="Web search API key")
def run(suite, model, provider, assistant_model, assistant_provider, lite_model, lite_provider, judge_model, judge_provider, headless, verbose_text, idle_timeout, g8ee_url, operator_url, operator_session_id, g8e_cli, auth_project_root, arm, state_root, output_dir, evidence_key_file, gold_set, limit, l2_key, l2_key_id, primary_api_key, primary_endpoint, assistant_api_key, assistant_endpoint, lite_api_key, lite_endpoint, judge_api_key, judge_endpoint, web_search_project, web_search_app, web_search_api_key):
    """Run a benchmark suite"""
    # Reject the well-known footgun: passing the operator_id UUID as
    # --operator-session-id silently 401s downstream because the Gateway
    # has no session matching that id. Fail fast with an actionable hint
    # rather than warning-and-continuing with an invalid token.
    operator_id_env = (os.environ.get("G8E_OPERATOR_ID") or "").strip()
    if operator_session_id and operator_id_env and operator_session_id == operator_id_env:
        raise click.UsageError(
            "--operator-session-id was given the operator_id UUID, not a session id. "
            "Drop the flag so `./g8e auth context` can load the canonical session."
        )

    try:
        auth_context = load_cli_auth_context(g8e_cli, str(auth_project_root.resolve()))
    except AuthBridgeError as error:
        raise click.UsageError(
            f"Could not load the canonical CLI identity: {error}. "
            "Run `./g8e auth enroll user` or `./g8e auth refresh`, then retry."
        ) from error

    if evidence_key_file is None:
        raise click.UsageError(
            "--evidence-key-file or G8E_EVIDENCE_KEY_FILE is required to encrypt restricted evidence."
        )
    try:
        evidence_key = load_evidence_encryption_key(evidence_key_file)
    except ValueError as error:
        raise click.UsageError(f"Could not load the evidence encryption key: {error}") from error

    selected_arm = Arm(arm)

    config = SUTConfig(
        g8ee_url=g8ee_url,
        primary=LLMRoleConfig(provider=provider, model=model, api_key=primary_api_key, endpoint=primary_endpoint),
        assistant=LLMRoleConfig(provider=assistant_provider, model=assistant_model, api_key=assistant_api_key, endpoint=assistant_endpoint),
        lite=LLMRoleConfig(provider=lite_provider, model=lite_model, api_key=lite_api_key, endpoint=lite_endpoint),
        judge=LLMRoleConfig(provider=judge_provider, model=judge_model, api_key=judge_api_key, endpoint=judge_endpoint),
        operator_url=operator_url,
        operator_session_id=operator_session_id or auth_context.operator_session_id,
        auth_context=auth_context,
        state_root=state_root,
        l2_private_key=l2_key,
        l2_key_id=l2_key_id,
        arm=selected_arm,
        headless=headless,
    )

    try:
        asyncio.run(_run_suite(suite, config, gold_set, output_dir, limit, verbose_text=verbose_text, idle_timeout=idle_timeout, evidence_key=evidence_key))
    except EvaluationRunError as error:
        raise click.ClickException(str(error)) from error

async def _run_suite(suite: str, config: SUTConfig, gold_set: Path | None, output_dir: Path, limit: int | None = None, verbose_text: bool = False, idle_timeout: float = 180.0, evidence_key: EvidenceEncryptionKey | None = None):
    # 1. Load benchmark
    if suite == "ifeval_subset":
        if not gold_set:
            gold_set = Path("gold_sets/ifeval_subset/input_data.jsonl")
        loader = IFEvalLoader(gold_set)
        tasks = list(loader.load())
        if limit:
            tasks = tasks[:limit]
        verifier = IFEvalVerifier()
        provenance = load_provenance(gold_set.with_name("provenance.json"))
        suite_id = provenance.benchmark
        suite_version = provenance.output.sha256[:12]
        dataset_hash = provenance.output.sha256
    else:
        raise EvaluationRunError(f"unknown suite: {suite}")

    # 2. Apply G8E_TEST_LLM_* env vars as fallbacks (uniform with integration tests)
    # Priority: CLI flags > G8E_TEST_LLM_* env vars > g8ee settings
    if not config.primary.provider:
        config.primary.provider = os.environ.get("G8E_TEST_LLM_PRIMARY_PROVIDER", "").strip() or None
    if not config.primary.model:
        config.primary.model = os.environ.get("G8E_TEST_LLM_PRIMARY_MODEL", "").strip() or None
    if not config.primary.api_key:
        config.primary.api_key = os.environ.get("G8E_TEST_LLM_PRIMARY_API_KEY", "").strip() or None
    if not config.primary.endpoint:
        config.primary.endpoint = os.environ.get("G8E_TEST_LLM_PRIMARY_ENDPOINT_URL", "").strip() or None

    if not config.assistant.provider:
        config.assistant.provider = os.environ.get("G8E_TEST_LLM_ASSISTANT_PROVIDER", "").strip() or None
    if not config.assistant.model:
        config.assistant.model = os.environ.get("G8E_TEST_LLM_ASSISTANT_MODEL", "").strip() or None
    if not config.assistant.api_key:
        config.assistant.api_key = os.environ.get("G8E_TEST_LLM_ASSISTANT_API_KEY", "").strip() or None
    if not config.assistant.endpoint:
        config.assistant.endpoint = os.environ.get("G8E_TEST_LLM_ASSISTANT_ENDPOINT_URL", "").strip() or None

    if not config.lite.provider:
        config.lite.provider = os.environ.get("G8E_TEST_LLM_LITE_PROVIDER", "").strip() or None
    if not config.lite.model:
        config.lite.model = os.environ.get("G8E_TEST_LLM_LITE_MODEL", "").strip() or None
    if not config.lite.api_key:
        config.lite.api_key = os.environ.get("G8E_TEST_LLM_LITE_API_KEY", "").strip() or None
    if not config.lite.endpoint:
        config.lite.endpoint = os.environ.get("G8E_TEST_LLM_LITE_ENDPOINT_URL", "").strip() or None

    arm_def = config.arm_definition

    # 3. Build model identities. The manifest refuses to start when a
    #    required model identity is unavailable. The direct arm requires
    #    only the primary role; g8ee arms require at least one model
    #    (primary, assistant, or lite) either from CLI flags or g8ee
    #    settings (resolved during preflight).
    def _model_identity(role: str) -> ModelIdentity | None:
        rc = getattr(config, role)
        if not rc.provider or not rc.model:
            return None
        return ModelIdentity(
            role=role,
            provider=rc.provider,
            model=rc.model,
            endpoint=rc.endpoint,
            endpoint_class="local" if rc.provider in _KEYLESS_PROVIDERS or (rc.endpoint or "").startswith(("http://localhost", "http://127.")) else "remote",
            api_key_present=bool(rc.api_key),
        )

    primary_identity = _model_identity("primary")
    assistant_identity = _model_identity("assistant")
    lite_identity = _model_identity("lite")
    judge_identity = _model_identity("judge")

    if bool(config.judge.provider) != bool(config.judge.model):
        raise EvaluationRunError("eval judge requires both provider and model")
    if (
        judge_identity is not None
        and config.judge.provider not in _KEYLESS_PROVIDERS
        and not config.judge.api_key
    ):
        raise EvaluationRunError(
            f"Missing API key for judge provider '{config.judge.provider}'"
        )

    if arm_def.arm_id == Arm.DIRECT:
        if primary_identity is None:
            raise EvaluationRunError(
                "direct arm requires a primary model identity (provider and model). "
                "Provide them via --provider/--model or G8E_TEST_LLM_PRIMARY_*."
            )

    # 4. Initialize SUT and run preflight BEFORE writing the manifest.
    #    Preflight failures (auth, provider config) must not create a
    #    report directory; the diagnostic surface is the command output.
    current_renderer: dict[str, TurnRenderer | None] = {"r": None}

    async def _on_event(event_type: str, payload: dict) -> None:
        r = current_renderer["r"]
        if r is not None:
            r.render(event_type, payload)

    llm_settings = None
    remote_settings = None

    if arm_def.arm_id == Arm.DIRECT:
        sut = DirectProviderSUT(config)
    else:
        try:
            sut = G8eeChatSUT(
                config,
                on_event=_on_event,
                idle_timeout_s=idle_timeout,
            )
            remote_settings = await sut.check_settings()
        except AuthenticationError as e:
            console.print("[bold red]Authentication Error:[/bold red]")
            console.print(f"  {e}")
            console.print("\n[yellow]Run ./g8e auth enroll user or ./g8e auth refresh.[/yellow]")
            raise EvaluationRunError(f"preflight authentication failed: {e}") from e

        llm_settings = remote_settings.llm if remote_settings else None

        errors = []
        for role_name in ["primary", "assistant", "lite", "judge"]:
            role_config = getattr(config, role_name)
            if not role_config or not role_config.provider:
                continue

            if role_config.provider in _KEYLESS_PROVIDERS:
                continue

            if role_config.api_key:
                continue

            if role_name == "judge":
                errors.append(
                    f"Missing API key for judge provider '{role_config.provider}'"
                )
                continue

            if not llm_settings:
                errors.append(f"Missing API key for {role_name} provider '{role_config.provider}' (could not fetch remote settings)")
                continue

            provider_key_map = {
                "openai": "openai_api_key",
                "anthropic": "anthropic_api_key",
                "gemini": "gemini_api_key",
            }
            remote_key_field = provider_key_map.get(role_config.provider)
            if remote_key_field and getattr(llm_settings, remote_key_field, None):
                continue

            if getattr(llm_settings, f"{role_name}_api_key", None):
                continue

            errors.append(f"Missing API key for {role_name} provider '{role_config.provider}'")

        if errors:
            console.print("[bold red]Pre-flight validation failed:[/bold red]")
            for err in errors:
                console.print(f"  - {err}")
            console.print("\n[yellow]Provide keys via --primary-api-key, etc. or configure them in g8ee settings.[/yellow]")
            raise EvaluationRunError("provider preflight failed: " + "; ".join(errors))

        has_cli_primary = bool(config.primary.provider and config.primary.model)
        has_cli_assistant = bool(config.assistant.provider and config.assistant.model)
        has_cli_lite = bool(config.lite.provider and config.lite.model)

        if llm_settings is None and not (has_cli_primary or has_cli_assistant or has_cli_lite):
            console.print("[bold red]Pre-flight validation failed:[/bold red]")
            console.print("  - Could not fetch LLM settings from g8ee and no CLI model provided")
            console.print("\n[yellow]To configure LLM models:[/yellow]")
            console.print("  1. Run: ./g8e platform settings")
            console.print("  2. Set primary_model and/or assistant_model in the LLM section")
            console.print("  3. Or set G8E_TEST_LLM_* environment variables (uniform with integration tests):")
            console.print("     - G8E_TEST_LLM_PRIMARY_PROVIDER (e.g., 'openai')")
            console.print("     - G8E_TEST_LLM_PRIMARY_MODEL (e.g., 'gpt-4o')")
            console.print("     - G8E_TEST_LLM_PRIMARY_API_KEY (your API key)")
            console.print("     - G8E_TEST_LLM_PRIMARY_ENDPOINT_URL (if using custom endpoint)")
            console.print("  4. Restart g8ee: ./g8e apps restart g8ee")
            console.print("\n[yellow]Alternatively, use CLI flags:[/yellow]")
            console.print("  ./g8e evals bench --suite ifeval_subset --provider openai --model gpt-4o")
            raise EvaluationRunError("provider preflight failed: no model configured")

        if llm_settings:
            has_settings_primary = bool(llm_settings.primary_model)
            has_settings_assistant = bool(llm_settings.assistant_model)
            has_settings_lite = bool(llm_settings.lite_model)

            if not (has_cli_primary or has_cli_assistant or has_cli_lite or
                    has_settings_primary or has_settings_assistant or has_settings_lite):
                console.print("[bold red]Pre-flight validation failed:[/bold red]")
                console.print("  - No LLM model configured in g8ee settings")
                console.print("\n[yellow]To configure LLM models:[/yellow]")
                console.print("  1. Run: ./g8e platform settings")
                console.print("  2. Set primary_model and/or assistant_model in the LLM section")
                console.print("  3. Or set G8E_TEST_LLM_* environment variables (uniform with integration tests):")
                console.print("     - G8E_TEST_LLM_PRIMARY_PROVIDER (e.g., 'openai')")
                console.print("     - G8E_TEST_LLM_PRIMARY_MODEL (e.g., 'gpt-4o')")
                console.print("     - G8E_TEST_LLM_PRIMARY_API_KEY (your API key)")
                console.print("     - G8E_TEST_LLM_PRIMARY_ENDPOINT_URL (if using custom endpoint)")
                console.print("  4. Restart g8ee: ./g8e apps restart g8ee")
                console.print("\n[yellow]Alternatively, use CLI flags:[/yellow]")
                console.print("  ./g8e evals bench --suite ifeval_subset --provider openai --model gpt-4o")
                raise EvaluationRunError("provider preflight failed: no model configured")

    eval_judge = _create_eval_judge(config.judge) if judge_identity is not None else None

    if evidence_key is None:
        raise EvaluationRunError("restricted evidence encryption key is required")

    # 5. Build the run manifest. Required hashes: dataset hash (always),
    #    grader bundle hash (computed from verifier source), prompt bundle
    #    hash (computed from task prompts). The manifest refuses to start
    #    when a required hash or model identity is unavailable.
    run_id = str(uuid.uuid4())

    prompt_bundle_content = "\n".join(t.prompt for t in tasks).encode()
    prompt_bundle_hash = hashlib.sha256(prompt_bundle_content).hexdigest()

    grader_bundle_content = (
        f"{suite}:{suite_version}:"
        f"{_IFEVAL_GRADER_ID}@{_GRADER_VERSION}:"
        f"{_RECEIPT_INTEGRITY_GRADER_ID}@{_GRADER_VERSION}:"
        f"{_PROTOCOL_CHAIN_GRADER_ID}@{_GRADER_VERSION}:"
        f"{_CANARY_SCRUBBING_GRADER_ID}@{_GRADER_VERSION}:"
        f"{_MODEL_BOUNDARY_RAW_SECRET_GRADER_ID}@{_GRADER_VERSION}:"
        f"{_EXACT_LOCAL_REHYDRATION_GRADER_ID}@{_GRADER_VERSION}:"
        f"{_SECRET_DETECTION_PRECISION_GRADER_ID}@{_GRADER_VERSION}:"
        f"{_SECRET_DETECTION_RECALL_GRADER_ID}@{_GRADER_VERSION}:"
        f"{_FINAL_STATE_GRADER_ID}@{_GRADER_VERSION}:"
        f"{_INDEPENDENT_STATE_GRADER_ID}@{_GRADER_VERSION}:"
        f"{_POLICY_OUTCOME_GRADER_ID}@{_GRADER_VERSION}:"
        f"{_EVAL_JUDGE_GRADER_ID}@{_GRADER_VERSION}:"
        f"{config.judge.provider or ''}:{config.judge.model or ''}"
    ).encode()
    grader_bundle_hash = hashlib.sha256(grader_bundle_content).hexdigest()

    content_hashes = [
        ContentHash(name="dataset", sha256=dataset_hash, byte_length=gold_set.stat().st_size if gold_set.exists() else 0),
        ContentHash(name="prompt_bundle", sha256=prompt_bundle_hash, byte_length=len(prompt_bundle_content)),
        ContentHash(name="grader_bundle", sha256=grader_bundle_hash, byte_length=len(grader_bundle_content)),
    ]

    arm_entry = ArmManifestEntry(
        arm_id=arm_def.arm_id,
        requested_posture=arm_def.requested_posture,
        uses_g8ee=arm_def.uses_g8ee,
        uses_gateway=arm_def.uses_gateway,
        receipt_binding=arm_def.receipt_binding,
        is_production_posture=arm_def.is_production_posture,
    )

    manifest = RunManifest(
        run_id=run_id,
        suite_id=suite_id,
        suite_version=suite_version,
        orchestrator_version=EVALS_VERSION,
        arms=[arm_entry],
        content_hashes=content_hashes,
        role_to_model=RoleToModelMapping(
            primary=primary_identity,
            assistant=assistant_identity,
            lite=lite_identity,
            judge=judge_identity,
        ),
        stack_environment=StackEnvironment(
            os=platform.platform(),
            arch=platform.machine(),
            runtime_version=platform.python_version(),
        ),
    )

    # 6. Create report directory and write manifest BEFORE execution.
    ts = datetime.now(UTC).strftime("%Y%m%d-%H%M%S")
    report_dir = output_dir / f"{suite}-{ts}"
    report_dir.mkdir(parents=True, exist_ok=True)

    manifest_path = report_dir / "manifest.json"
    manifest_path.write_text(manifest.model_dump_json(indent=2))

    collector = ReceiptCollector(config.operator_url, cli_context=config.auth_context)

    # Load warden pub key for verification
    warden_pub_path = Path(os.environ.get("G8E_GATEWAY_PKI_DIR", ".g8e/pki")) / "warden_pub.pem"
    warden_pub = ""
    if warden_pub_path.exists():
        warden_pub = warden_pub_path.read_text()

    # Observe the gateway's configured posture for governed arms. The
    # gateway is the posture authority; the runner never infers posture
    # from the CLI argument alone. For ungoverned arms, the observed
    # posture is NONE because the task does not route through the gateway.
    observed_posture: GovernancePosture | None = GovernancePosture.NONE
    posture_source = "arm_definition"
    if arm_def.uses_gateway:
        gw_env = AuthContext.from_env(
            operator_session_id=config.operator_session_id,
            g8ee_url=config.g8ee_url,
            operator_url=config.operator_url,
            cli_context=config.auth_context,
        )
        observed_posture = await observe_gateway_posture(gw_env)
        posture_source = "gateway_health_endpoint"
        if observed_posture is None:
            console.print("[bold yellow]Warning: could not observe gateway posture; recording as unobserved[/bold yellow]")
        else:
            console.print(f"  [dim]Observed gateway posture: {observed_posture.value}[/dim]")

    results = []

    display_model = f"{config.primary.provider}:{config.primary.model}" if config.primary.provider and config.primary.model else (config.primary.model or "openai:gpt-4")
    console.print(f"[bold blue]Running {suite} [{arm_def.arm_id.value}] with {display_model}...[/bold blue]")

    # 7. Write task definitions before execution.
    task_defs = [
        TaskDefinition(
            task_id=t.id,
            suite_id=suite_id,
            suite_version=suite_version,
            category=t.metadata.category or "instruction_following",
            expected_action_class=t.metadata.expected_action_class,
            compatible_arms=list(
                GOVERNED_ARMS
                if (
                    t.metadata.expected_action_class
                    or t.metadata.state_fixture
                    or t.metadata.expected_final_state_assertions
                    or t.metadata.expected_allow_block_outcome
                    or t.metadata.unauthorized_mutation_assertions
                    or t.metadata.token_store_persistence_assertions
                    or t.metadata.token_ttl_expiry_assertions
                    or t.metadata.token_persistence_failure_assertions
                    or t.metadata.exfiltration_attempt_assertions
                    or t.metadata.artifact_leakage_assertions
                    or t.metadata.replay_attempt_assertions
                    or t.metadata.signed_field_tampering_assertions
                    or t.metadata.payload_tampering_assertions
                    or t.metadata.stale_state_root_assertions
                    or t.metadata.identity_mismatch_assertions
                    or t.metadata.nonce_expiration_assertions
                    or t.metadata.signer_defect_assertions
                    or t.metadata.l3_proof_transplant_assertions
                    or t.metadata.revoked_credential_assertions
                    or t.metadata.evidence_preservation_assertions
                )
                else ALL_ARMS
            ),
            prompt_hash=hashlib.sha256(t.prompt.encode()).hexdigest(),
            prompt_length=len(t.prompt),
            initial_state_fixture_hash=(
                t.metadata.state_fixture.fixture_sha256 if t.metadata.state_fixture else None
            ),
            state_fixture=t.metadata.state_fixture,
            expected_final_state_assertions=t.metadata.expected_final_state_assertions,
            expected_allow_block_outcome=t.metadata.expected_allow_block_outcome,
            expected_rejection_layer=t.metadata.expected_rejection_layer,
            sensitive_canary_annotations=t.metadata.sensitive_canary_annotations,
            rehydration_assertions=t.metadata.rehydration_assertions,
            secret_detection_assertions=t.metadata.secret_detection_assertions,
            unauthorized_mutation_assertions=t.metadata.unauthorized_mutation_assertions,
            token_store_persistence_assertions=t.metadata.token_store_persistence_assertions,
            token_ttl_expiry_assertions=t.metadata.token_ttl_expiry_assertions,
            token_persistence_failure_assertions=t.metadata.token_persistence_failure_assertions,
            exfiltration_attempt_assertions=t.metadata.exfiltration_attempt_assertions,
            artifact_leakage_assertions=t.metadata.artifact_leakage_assertions,
            replay_attempt_assertions=t.metadata.replay_attempt_assertions,
            signed_field_tampering_assertions=t.metadata.signed_field_tampering_assertions,
            payload_tampering_assertions=t.metadata.payload_tampering_assertions,
            stale_state_root_assertions=t.metadata.stale_state_root_assertions,
            identity_mismatch_assertions=t.metadata.identity_mismatch_assertions,
            nonce_expiration_assertions=t.metadata.nonce_expiration_assertions,
            signer_defect_assertions=t.metadata.signer_defect_assertions,
            l3_proof_transplant_assertions=t.metadata.l3_proof_transplant_assertions,
            revoked_credential_assertions=t.metadata.revoked_credential_assertions,
            evidence_preservation_assertions=t.metadata.evidence_preservation_assertions,
            unsupported_exclusions=t.metadata.unsupported_exclusions,
            graders=[
                GraderReference(
                    grader_id=_IFEVAL_GRADER_ID,
                    grader_version=_GRADER_VERSION,
                    grader_class=GraderClass.DETERMINISTIC,
                ),
                *([
                    GraderReference(
                        grader_id=_EVAL_JUDGE_GRADER_ID,
                        grader_version=_GRADER_VERSION,
                        grader_class=GraderClass.LLM_JUDGE,
                    )
                ] if eval_judge else []),
                *([
                    GraderReference(
                        grader_id=_RECEIPT_INTEGRITY_GRADER_ID,
                        grader_version=_GRADER_VERSION,
                        grader_class=GraderClass.DETERMINISTIC,
                    )
                ] if t.metadata.expected_action_class else []),
                *([
                    GraderReference(
                        grader_id=_PROTOCOL_CHAIN_GRADER_ID,
                        grader_version=_GRADER_VERSION,
                        grader_class=GraderClass.DETERMINISTIC,
                    )
                ] if t.metadata.expected_action_class else []),
                *([
                    GraderReference(
                        grader_id=_CANARY_SCRUBBING_GRADER_ID,
                        grader_version=_GRADER_VERSION,
                        grader_class=GraderClass.DETERMINISTIC,
                    )
                ] if t.metadata.sensitive_canary_annotations else []),
                *([
                    GraderReference(
                        grader_id=_MODEL_BOUNDARY_RAW_SECRET_GRADER_ID,
                        grader_version=_GRADER_VERSION,
                        grader_class=GraderClass.DETERMINISTIC,
                    )
                ] if t.metadata.sensitive_canary_annotations else []),
                *([
                    GraderReference(
                        grader_id=_EXACT_LOCAL_REHYDRATION_GRADER_ID,
                        grader_version=_GRADER_VERSION,
                        grader_class=GraderClass.DETERMINISTIC,
                    )
                ] if t.metadata.rehydration_assertions else []),
                *([
                    GraderReference(
                        grader_id=_SECRET_DETECTION_PRECISION_GRADER_ID,
                        grader_version=_GRADER_VERSION,
                        grader_class=GraderClass.DETERMINISTIC,
                    )
                ] if t.metadata.secret_detection_assertions else []),
                *([
                    GraderReference(
                        grader_id=_SECRET_DETECTION_RECALL_GRADER_ID,
                        grader_version=_GRADER_VERSION,
                        grader_class=GraderClass.DETERMINISTIC,
                    )
                ] if t.metadata.secret_detection_assertions else []),
                *([
                    GraderReference(
                        grader_id=_FINAL_STATE_GRADER_ID,
                        grader_version=_GRADER_VERSION,
                        grader_class=GraderClass.DETERMINISTIC,
                    )
                ] if t.metadata.expected_final_state_assertions else []),
                *([
                    GraderReference(
                        grader_id=_INDEPENDENT_STATE_GRADER_ID,
                        grader_version=_GRADER_VERSION,
                        grader_class=GraderClass.DETERMINISTIC,
                    )
                ] if t.metadata.state_fixture else []),
                *([
                    GraderReference(
                        grader_id=_POLICY_OUTCOME_GRADER_ID,
                        grader_version=_GRADER_VERSION,
                        grader_class=GraderClass.DETERMINISTIC,
                    )
                ] if t.metadata.expected_allow_block_outcome else []),
                *([
                    GraderReference(
                        grader_id=_UNAUTHORIZED_MUTATION_GRADER_ID,
                        grader_version=_GRADER_VERSION,
                        grader_class=GraderClass.DETERMINISTIC,
                    )
                ] if t.metadata.unauthorized_mutation_assertions else []),
                *([
                    GraderReference(
                        grader_id=_TOKEN_STORE_PERSISTENCE_GRADER_ID,
                        grader_version=_GRADER_VERSION,
                        grader_class=GraderClass.DETERMINISTIC,
                    )
                ] if t.metadata.token_store_persistence_assertions else []),
                *([
                    GraderReference(
                        grader_id=_TOKEN_TTL_EXPIRY_GRADER_ID,
                        grader_version=_GRADER_VERSION,
                        grader_class=GraderClass.DETERMINISTIC,
                    )
                ] if t.metadata.token_ttl_expiry_assertions else []),
                *([
                    GraderReference(
                        grader_id=_TOKEN_PERSISTENCE_FAILURE_GRADER_ID,
                        grader_version=_GRADER_VERSION,
                        grader_class=GraderClass.DETERMINISTIC,
                    )
                ] if t.metadata.token_persistence_failure_assertions else []),
                *([
                    GraderReference(
                        grader_id=_EXFILTRATION_ATTEMPT_GRADER_ID,
                        grader_version=_GRADER_VERSION,
                        grader_class=GraderClass.DETERMINISTIC,
                    )
                ] if t.metadata.exfiltration_attempt_assertions else []),
                *([
                    GraderReference(
                        grader_id=_ARTIFACT_LEAKAGE_GRADER_ID,
                        grader_version=_GRADER_VERSION,
                        grader_class=GraderClass.DETERMINISTIC,
                    )
                ] if t.metadata.artifact_leakage_assertions else []),
                *([
                    GraderReference(
                        grader_id=_REPLAY_ATTEMPT_GRADER_ID,
                        grader_version=_GRADER_VERSION,
                        grader_class=GraderClass.DETERMINISTIC,
                    )
                ] if t.metadata.replay_attempt_assertions else []),
                *([
                    GraderReference(
                        grader_id=_SIGNED_FIELD_TAMPERING_GRADER_ID,
                        grader_version=_GRADER_VERSION,
                        grader_class=GraderClass.DETERMINISTIC,
                    )
                ] if t.metadata.signed_field_tampering_assertions else []),
                *([
                    GraderReference(
                        grader_id=_PAYLOAD_TAMPERING_GRADER_ID,
                        grader_version=_GRADER_VERSION,
                        grader_class=GraderClass.DETERMINISTIC,
                    )
                ] if t.metadata.payload_tampering_assertions else []),
                *([
                    GraderReference(
                        grader_id=_STALE_STATE_ROOT_GRADER_ID,
                        grader_version=_GRADER_VERSION,
                        grader_class=GraderClass.DETERMINISTIC,
                    )
                ] if t.metadata.stale_state_root_assertions else []),
                *([
                    GraderReference(
                        grader_id=_IDENTITY_MISMATCH_GRADER_ID,
                        grader_version=_GRADER_VERSION,
                        grader_class=GraderClass.DETERMINISTIC,
                    )
                ] if t.metadata.identity_mismatch_assertions else []),
                *([
                    GraderReference(
                        grader_id=_NONCE_EXPIRATION_GRADER_ID,
                        grader_version=_GRADER_VERSION,
                        grader_class=GraderClass.DETERMINISTIC,
                    )
                ] if t.metadata.nonce_expiration_assertions else []),
                *([
                    GraderReference(
                        grader_id=_SIGNER_DEFECT_GRADER_ID,
                        grader_version=_GRADER_VERSION,
                        grader_class=GraderClass.DETERMINISTIC,
                    )
                ] if t.metadata.signer_defect_assertions else []),
                *([
                    GraderReference(
                        grader_id=_L3_PROOF_TRANSPLANT_GRADER_ID,
                        grader_version=_GRADER_VERSION,
                        grader_class=GraderClass.DETERMINISTIC,
                    )
                ] if t.metadata.l3_proof_transplant_assertions else []),
                *([
                    GraderReference(
                        grader_id=_REVOKED_CREDENTIAL_GRADER_ID,
                        grader_version=_GRADER_VERSION,
                        grader_class=GraderClass.DETERMINISTIC,
                    )
                ] if t.metadata.revoked_credential_assertions else []),
                *([
                    GraderReference(
                        grader_id=_EVIDENCE_PRESERVATION_GRADER_ID,
                        grader_version=_GRADER_VERSION,
                        grader_class=GraderClass.DETERMINISTIC,
                    )
                ] if t.metadata.evidence_preservation_assertions else []),
            ],
            metadata={"instruction_id_list": t.metadata.instruction_id_list},
        )
        for t in tasks
    ]
    task_defs_by_id = {task.task_id: task for task in task_defs}
    with open(report_dir / "tasks.jsonl", "w") as f:
        for td in task_defs:
            f.write(td.model_dump_json() + "\n")

    # 8. Execution loop
    attempt_records: list[AttemptRecord] = []
    stage_records: list[StageObservation] = []
    metric_records: list[MetricObservation] = []
    receipt_records: list[ReceiptObservation] = []
    final_state_records: list[FinalStateObservation] = []
    state_records: list[StateObservation] = []
    rehydration_records: list[RehydrationObservation] = []
    secret_detection_records: list[SecretDetectionObservation] = []
    unauthorized_mutation_records: list[UnauthorizedMutationObservation] = []
    token_store_persistence_records: list[TokenStorePersistenceObservation] = []
    token_ttl_expiry_records: list[TokenTTLExpiryObservation] = []
    token_persistence_failure_records: list[TokenPersistenceFailureObservation] = []
    exfiltration_attempt_records: list[ExfiltrationAttemptObservation] = []
    artifact_leakage_records: list[ArtifactLeakageObservation] = []
    replay_attempt_records: list[ReplayAttemptObservation] = []
    signed_field_tampering_records: list[SignedFieldTamperingObservation] = []
    payload_tampering_records: list[PayloadTamperingObservation] = []
    stale_state_root_records: list[StaleStateRootObservation] = []
    identity_mismatch_records: list[IdentityMismatchObservation] = []
    nonce_expiration_records: list[NonceExpirationObservation] = []
    signer_defect_records: list[SignerDefectObservation] = []
    l3_proof_transplant_records: list[L3ProofTransplantObservation] = []
    revoked_credential_records: list[RevokedCredentialObservation] = []
    evidence_preservation_records: list[EvidencePreservationObservation] = []
    evidence_artifacts: list[EvidenceArtifact] = []
    for task in tasks:
        intent = ""
        if suite == "ifeval_subset" and task.metadata.instruction_id_list:
            constraints = [instruction_id.split(":")[-1] for instruction_id in task.metadata.instruction_id_list]
            intent = f" [dim][{', '.join(constraints)}][/dim]"

        prompt_preview = task.prompt.replace("\n", " ")[:50]
        if len(task.prompt) > 50:
            prompt_preview += "..."

        console.print(f"  [cyan]{task.id:>4}[/cyan]: {prompt_preview}{intent}")

        renderer = TurnRenderer(console, task_id=str(task.id), verbose_text=verbose_text)
        current_renderer["r"] = renderer

        started_at = datetime.now(UTC)
        response = await sut.get_answer(task)
        current_renderer["r"] = None
        ended_at = datetime.now(UTC)

        terminal_event = response.chat_evidence.terminal_event if response.chat_evidence else None

        renderer.finish(
            terminal_event=terminal_event,
            answer_chars=len(response.answer or ""),
        )

        if arm_def.receipt_binding:
            collected_receipts = []
            collected_transaction_ids: set[str] = set()
            for transaction_id in response.transaction_ids:
                receipt = await collector.collect_receipt(transaction_id)
                if receipt is not None and receipt.transaction_id not in collected_transaction_ids:
                    collected_receipts.append(receipt)
                    collected_transaction_ids.add(receipt.transaction_id)

            correlated_action_types = [
                action_type
                for action_type in [
                    task.metadata.expected_action_class,
                    *(
                        assertion.action_type
                        for assertion in task.metadata.expected_final_state_assertions
                    ),
                    *(
                        assertion.action_type
                        for assertion in (
                            task.metadata.state_fixture.assertions
                            if task.metadata.state_fixture
                            else []
                        )
                    ),
                    *response.governed_action_types,
                ]
                if action_type
            ]
            if isinstance(response.chat_evidence, ChatEvaluationReceipt):
                for action_type in dict.fromkeys(correlated_action_types):
                    if any(
                        receipt_action_type(receipt) == action_type
                        for receipt in collected_receipts
                    ):
                        continue
                    receipt = await collector.collect_receipt_for_investigation(
                        response.chat_evidence.investigation_id,
                        action_type,
                    )
                    if receipt is not None and receipt.transaction_id not in collected_transaction_ids:
                        collected_receipts.append(receipt)
                        collected_transaction_ids.add(receipt.transaction_id)

            response.receipts = [
                ReceiptEvidence(
                    action_receipt=receipt,
                    verified=bool(
                        warden_pub
                        and verify_action_receipt_signature(receipt, warden_pub)
                        and verify_receipt_persistence_attestation(receipt, warden_pub)
                    ),
                )
                for receipt in collected_receipts
            ]
            if task.metadata.expected_action_class:
                matching_receipts = [
                    receipt.action_receipt.transaction_id
                    for receipt in response.receipts
                    if receipt_action_type(receipt.action_receipt)
                    == task.metadata.expected_action_class
                ]
                if len(matching_receipts) == 1:
                    response.primary_transaction_id = matching_receipts[0]
            elif response.receipts:
                response.primary_transaction_id = response.receipts[0].action_receipt.transaction_id

            if response.primary_transaction_id:
                response.binding = BindingType.RECEIPT_BOUND
                response.unbound_reason = None
            elif response.receipts:
                response.binding = BindingType.UNBOUND
                response.unbound_reason = "declared-action receipt was not uniquely identified"

        # Score
        if suite == "ifeval_subset":
            score = verifier.verify(
                task.id,
                task.prompt,
                response.answer,
                task.metadata.instruction_id_list,
                task.metadata.kwargs
            )

        judge_metric_value: float | None = None
        judge_metric_status = VerificationStatus.NOT_APPLICABLE
        if eval_judge is not None:
            try:
                judge_grade = await eval_judge.grade_turn(
                    user_query=task.prompt,
                    interaction_trace=response.answer,
                    expected_behavior="Satisfy every declared benchmark instruction.",
                    required_concepts=task.metadata.instruction_id_list,
                )
                score.model_calls.extend(judge_grade.model_calls)
                judge_metric_value = float(judge_grade.score)
                judge_metric_status = VerificationStatus.VERIFIED
                score.details.benchmark_specific["eval_judge"] = {
                    "status": "completed",
                    "score": judge_grade.score,
                    "passed": judge_grade.passed,
                }
            except EvalJudgeError as error:
                score.model_calls.extend(error.model_calls)
                judge_metric_status = VerificationStatus.FAILED
                score.details.benchmark_specific["eval_judge"] = {
                    "status": "failed",
                    "error_type": type(error).__name__,
                }

        res = RowResult(task=task, response=response, score=score, arm=arm_def.arm_id)
        results.append(res)

        # Build the attempt record with requested and observed posture.
        # The observed posture comes from the gateway health endpoint for
        # governed arms, not from the CLI argument. posture_match is None
        # when the posture could not be independently observed.
        if observed_posture is not None and arm_def.uses_gateway:
            posture_match = observed_posture == arm_def.requested_posture
        elif not arm_def.uses_gateway:
            posture_match = True
        else:
            posture_match = None

        posture = PostureObservation(
            requested_posture=arm_def.requested_posture,
            observed_posture=observed_posture,
            observation_source=posture_source,
            observation_timestamp=ended_at,
            posture_match=posture_match,
        )

        if "HTTP 403" in (response.unbound_reason or "") and "Governance verification failed" in (response.unbound_reason or ""):
            terminal_status = TerminalStatus.GOVERNANCE_REJECTED
        elif "HTTP 401" in (response.unbound_reason or "") or "HTTP 403" in (response.unbound_reason or ""):
            terminal_status = TerminalStatus.INFRASTRUCTURE_FAILED
        elif not response.answer or not terminal_event or terminal_event.endswith(("failed", "stopped", "dead.lettered")):
            terminal_status = TerminalStatus.MODEL_FAILED
        else:
            terminal_status = TerminalStatus.COMPLETED

        attempt_id = f"{run_id}:{task.id}:{arm_def.arm_id.value}:1"
        attempt_receipts = [
            ReceiptObservation(
                receipt_id=f"{attempt_id}:receipt:{receipt.action_receipt.transaction_id}",
                attempt_id=attempt_id,
                run_id=run_id,
                transaction_id=receipt.action_receipt.transaction_id,
                action_type=receipt_action_type(receipt.action_receipt),
                primary=receipt.action_receipt.transaction_id == response.primary_transaction_id,
                verified=receipt.verified,
                action_receipt=receipt.action_receipt,
            )
            for receipt in response.receipts
        ]
        receipt_records.extend(attempt_receipts)
        if any(
            receipt.primary
            and receipt.verified
            and any(
                stage.kind == DETERMINISTIC_STAGE_KIND_L4_VERIFICATION
                and stage.outcome == DETERMINISTIC_STAGE_OUTCOME_FAILED
                for stage in receipt.action_receipt.deterministic_stage_evidence
            )
            for receipt in attempt_receipts
        ):
            terminal_status = TerminalStatus.GOVERNANCE_REJECTED
        normalized = (
            normalize_attempt_evidence(
                response.chat_evidence,
                run_id,
                attempt_id,
                receipts=response.receipts,
                grading_model_calls=score.model_calls,
            )
            if response.chat_evidence
            else None
        )
        evidence_refs: list[str] = []
        attempt_stages = normalized.stages if normalized else []
        if normalized:
            stage_records.extend(attempt_stages)
            if normalized.raw_evidence:
                evidence_artifacts.append(normalized.raw_evidence)
                evidence_refs.append(normalized.raw_evidence.index.artifact_id)
            usage_metric = MetricObservation(
                metric_id="stage_usage_reconciled",
                attempt_id=attempt_id,
                run_id=run_id,
                arm_id=arm_def.arm_id,
                task_id=task.id,
                value=float(normalized.usage.reconciled),
                unit="boolean",
                verification_status=VerificationStatus.VERIFIED,
                evidence_refs=evidence_refs,
            )
            DEFAULT_METRIC_REGISTRY.validate(usage_metric)
            metric_records.append(usage_metric)

        primary_receipt = next(
            (receipt for receipt in attempt_receipts if receipt.primary),
            None,
        )
        attempt = AttemptRecord(
            attempt_id=attempt_id,
            run_id=run_id,
            task_id=task.id,
            arm_id=arm_def.arm_id,
            state_snapshot_hash=(
                task.metadata.state_fixture.fixture_sha256
                if task.metadata.state_fixture
                else config.state_root
            ),
            replicate_id="1",
            assignment_order=len(attempt_records),
            started_at=started_at,
            ended_at=ended_at,
            terminal_status=terminal_status,
            posture=posture,
            state_root_before=(
                primary_receipt.action_receipt.state_root_before or None
                if primary_receipt
                else None
            ),
            state_root_after=(
                primary_receipt.action_receipt.state_root_after or None
                if primary_receipt
                else None
            ),
            correlation_ids={
                "transaction_id": response.primary_transaction_id or "",
            },
            receipt_refs=[receipt.receipt_id for receipt in attempt_receipts],
            missingness_or_failure=response.unbound_reason if terminal_status != TerminalStatus.COMPLETED else None,
            usage_reconciliation=normalized.usage if normalized else None,
        )
        final_state_observations = observe_receipt_final_state(
            task_defs_by_id[task.id],
            attempt,
            attempt_receipts,
        )
        final_state_records.extend(final_state_observations)
        attempt.final_state_observation_refs = [
            observation.observation_id for observation in final_state_observations
        ]
        state_observations = (
            await config.state_observer.observe(task_defs_by_id[task.id], attempt)
            if task.metadata.state_fixture and config.state_observer is not None
            else []
        )
        state_records.extend(state_observations)
        attempt.state_observation_refs = [
            observation.observation_id for observation in state_observations
        ]
        rehydration_observations = (
            await config.rehydration_observer.observe(task_defs_by_id[task.id], attempt)
            if task.metadata.rehydration_assertions and config.rehydration_observer is not None
            else []
        )
        rehydration_records.extend(rehydration_observations)
        attempt.rehydration_observation_refs = [
            observation.observation_id for observation in rehydration_observations
        ]
        secret_detection_observations = (
            await config.secret_detection_observer.observe(task_defs_by_id[task.id], attempt)
            if task.metadata.secret_detection_assertions and config.secret_detection_observer is not None
            else []
        )
        secret_detection_records.extend(secret_detection_observations)
        attempt.secret_detection_observation_refs = [
            observation.observation_id for observation in secret_detection_observations
        ]
        unauthorized_mutation_observations = (
            await config.unauthorized_mutation_observer.observe(task_defs_by_id[task.id], attempt)
            if task.metadata.unauthorized_mutation_assertions and config.unauthorized_mutation_observer is not None
            else []
        )
        unauthorized_mutation_records.extend(unauthorized_mutation_observations)
        attempt.unauthorized_mutation_observation_refs = [
            observation.observation_id for observation in unauthorized_mutation_observations
        ]
        token_store_persistence_observations = (
            await config.token_store_persistence_observer.observe(task_defs_by_id[task.id], attempt)
            if task.metadata.token_store_persistence_assertions and config.token_store_persistence_observer is not None
            else []
        )
        token_store_persistence_records.extend(token_store_persistence_observations)
        attempt.token_store_persistence_observation_refs = [
            observation.observation_id for observation in token_store_persistence_observations
        ]
        token_ttl_expiry_observations = (
            await config.token_ttl_expiry_observer.observe(task_defs_by_id[task.id], attempt)
            if task.metadata.token_ttl_expiry_assertions and config.token_ttl_expiry_observer is not None
            else []
        )
        token_ttl_expiry_records.extend(token_ttl_expiry_observations)
        attempt.token_ttl_expiry_observation_refs = [
            observation.observation_id for observation in token_ttl_expiry_observations
        ]
        token_persistence_failure_observations = (
            await config.token_persistence_failure_observer.observe(task_defs_by_id[task.id], attempt)
            if task.metadata.token_persistence_failure_assertions and config.token_persistence_failure_observer is not None
            else []
        )
        token_persistence_failure_records.extend(token_persistence_failure_observations)
        attempt.token_persistence_failure_observation_refs = [
            observation.observation_id for observation in token_persistence_failure_observations
        ]
        exfiltration_attempt_observations = (
            await config.exfiltration_attempt_observer.observe(task_defs_by_id[task.id], attempt)
            if task.metadata.exfiltration_attempt_assertions and config.exfiltration_attempt_observer is not None
            else []
        )
        exfiltration_attempt_records.extend(exfiltration_attempt_observations)
        attempt.exfiltration_attempt_observation_refs = [
            observation.observation_id for observation in exfiltration_attempt_observations
        ]
        artifact_leakage_observations = (
            await config.artifact_leakage_observer.observe(task_defs_by_id[task.id], attempt)
            if task.metadata.artifact_leakage_assertions and config.artifact_leakage_observer is not None
            else []
        )
        artifact_leakage_records.extend(artifact_leakage_observations)
        attempt.artifact_leakage_observation_refs = [
            observation.observation_id for observation in artifact_leakage_observations
        ]
        replay_attempt_observations = (
            await config.replay_attempt_observer.observe(task_defs_by_id[task.id], attempt)
            if task.metadata.replay_attempt_assertions and config.replay_attempt_observer is not None
            else []
        )
        replay_attempt_records.extend(replay_attempt_observations)
        attempt.replay_attempt_observation_refs = [
            observation.observation_id for observation in replay_attempt_observations
        ]
        signed_field_tampering_observations = (
            await config.signed_field_tampering_observer.observe(task_defs_by_id[task.id], attempt)
            if task.metadata.signed_field_tampering_assertions and config.signed_field_tampering_observer is not None
            else []
        )
        signed_field_tampering_records.extend(signed_field_tampering_observations)
        attempt.signed_field_tampering_observation_refs = [
            observation.observation_id for observation in signed_field_tampering_observations
        ]
        payload_tampering_observations = (
            await config.payload_tampering_observer.observe(task_defs_by_id[task.id], attempt)
            if task.metadata.payload_tampering_assertions and config.payload_tampering_observer is not None
            else []
        )
        payload_tampering_records.extend(payload_tampering_observations)
        attempt.payload_tampering_observation_refs = [
            observation.observation_id for observation in payload_tampering_observations
        ]
        stale_state_root_observations = (
            await config.stale_state_root_observer.observe(task_defs_by_id[task.id], attempt)
            if task.metadata.stale_state_root_assertions and config.stale_state_root_observer is not None
            else []
        )
        stale_state_root_records.extend(stale_state_root_observations)
        attempt.stale_state_root_observation_refs = [
            observation.observation_id for observation in stale_state_root_observations
        ]
        identity_mismatch_observations = (
            await config.identity_mismatch_observer.observe(task_defs_by_id[task.id], attempt)
            if task.metadata.identity_mismatch_assertions and config.identity_mismatch_observer is not None
            else []
        )
        identity_mismatch_records.extend(identity_mismatch_observations)
        attempt.identity_mismatch_observation_refs = [
            observation.observation_id for observation in identity_mismatch_observations
        ]
        nonce_expiration_observations = (
            await config.nonce_expiration_observer.observe(task_defs_by_id[task.id], attempt)
            if task.metadata.nonce_expiration_assertions and config.nonce_expiration_observer is not None
            else []
        )
        nonce_expiration_records.extend(nonce_expiration_observations)
        attempt.nonce_expiration_observation_refs = [
            observation.observation_id for observation in nonce_expiration_observations
        ]
        signer_defect_observations = (
            await config.signer_defect_observer.observe(task_defs_by_id[task.id], attempt)
            if task.metadata.signer_defect_assertions and config.signer_defect_observer is not None
            else []
        )
        signer_defect_records.extend(signer_defect_observations)
        attempt.signer_defect_observation_refs = [
            observation.observation_id for observation in signer_defect_observations
        ]
        l3_proof_transplant_observations = (
            await config.l3_proof_transplant_observer.observe(task_defs_by_id[task.id], attempt)
            if task.metadata.l3_proof_transplant_assertions and config.l3_proof_transplant_observer is not None
            else []
        )
        l3_proof_transplant_records.extend(l3_proof_transplant_observations)
        attempt.l3_proof_transplant_observation_refs = [
            observation.observation_id for observation in l3_proof_transplant_observations
        ]
        revoked_credential_observations = (
            await config.revoked_credential_observer.observe(task_defs_by_id[task.id], attempt)
            if task.metadata.revoked_credential_assertions and config.revoked_credential_observer is not None
            else []
        )
        revoked_credential_records.extend(revoked_credential_observations)
        attempt.revoked_credential_observation_refs = [
            observation.observation_id for observation in revoked_credential_observations
        ]
        evidence_preservation_observations = (
            await config.evidence_preservation_observer.observe(task_defs_by_id[task.id], attempt)
            if task.metadata.evidence_preservation_assertions and config.evidence_preservation_observer is not None
            else []
        )
        evidence_preservation_records.extend(evidence_preservation_observations)
        attempt.evidence_preservation_observation_refs = [
            observation.observation_id for observation in evidence_preservation_observations
        ]
        grade_metrics = [
            MetricObservation(
                metric_id=_IFEVAL_GRADER_ID,
                attempt_id=attempt_id,
                run_id=run_id,
                arm_id=arm_def.arm_id,
                task_id=task.id,
                value=float(score.passed),
                unit="boolean",
                verification_status=VerificationStatus.VERIFIED,
                grader_class=GraderClass.DETERMINISTIC,
                evidence_refs=evidence_refs,
            )
        ]
        if eval_judge is not None:
            grade_metrics.append(MetricObservation(
                metric_id=_EVAL_JUDGE_GRADER_ID,
                attempt_id=attempt_id,
                run_id=run_id,
                arm_id=arm_def.arm_id,
                task_id=task.id,
                value=judge_metric_value,
                unit="score_1_to_5",
                eligible=judge_metric_status == VerificationStatus.VERIFIED,
                denominator_contribution=int(judge_metric_status == VerificationStatus.VERIFIED),
                verification_status=judge_metric_status,
                grader_class=GraderClass.LLM_JUDGE,
                evidence_refs=evidence_refs,
            ))
        if task.metadata.expected_action_class:
            receipt_grade = grade_deterministically(
                _RECEIPT_INTEGRITY_GRADER_ID,
                _GRADER_VERSION,
                DeterministicGradingContext(
                    task=task_defs_by_id[task.id],
                    attempt=attempt,
                    receipts=attempt_receipts,
                    stages=attempt_stages,
                    final_state_observations=final_state_observations,
                ),
            )
            grade_metrics.append(MetricObservation(
                metric_id=_RECEIPT_INTEGRITY_GRADER_ID,
                attempt_id=attempt_id,
                run_id=run_id,
                arm_id=arm_def.arm_id,
                task_id=task.id,
                value=receipt_grade.value,
                unit="boolean",
                verification_status=receipt_grade.verification_status,
                grader_class=GraderClass.DETERMINISTIC,
                evidence_refs=receipt_grade.evidence_refs,
            ))
            protocol_grade = grade_deterministically(
                _PROTOCOL_CHAIN_GRADER_ID,
                _GRADER_VERSION,
                DeterministicGradingContext(
                    task=task_defs_by_id[task.id],
                    attempt=attempt,
                    receipts=attempt_receipts,
                    stages=attempt_stages,
                    final_state_observations=final_state_observations,
                ),
            )
            grade_metrics.append(MetricObservation(
                metric_id=_PROTOCOL_CHAIN_GRADER_ID,
                attempt_id=attempt_id,
                run_id=run_id,
                arm_id=arm_def.arm_id,
                task_id=task.id,
                value=protocol_grade.value,
                unit="boolean",
                verification_status=protocol_grade.verification_status,
                grader_class=GraderClass.DETERMINISTIC,
                evidence_refs=protocol_grade.evidence_refs,
            ))
        if task.metadata.sensitive_canary_annotations:
            canary_grade = grade_deterministically(
                _CANARY_SCRUBBING_GRADER_ID,
                _GRADER_VERSION,
                DeterministicGradingContext(
                    task=task_defs_by_id[task.id],
                    attempt=attempt,
                    receipts=attempt_receipts,
                    stages=attempt_stages,
                    final_state_observations=final_state_observations,
                ),
            )
            grade_metrics.append(MetricObservation(
                metric_id=_CANARY_SCRUBBING_GRADER_ID,
                attempt_id=attempt_id,
                run_id=run_id,
                arm_id=arm_def.arm_id,
                task_id=task.id,
                value=canary_grade.value,
                unit="proportion",
                denominator_contribution=len(task.metadata.sensitive_canary_annotations),
                verification_status=canary_grade.verification_status,
                grader_class=GraderClass.DETERMINISTIC,
                evidence_refs=canary_grade.evidence_refs,
            ))
            model_boundary_grade = grade_deterministically(
                _MODEL_BOUNDARY_RAW_SECRET_GRADER_ID,
                _GRADER_VERSION,
                DeterministicGradingContext(
                    task=task_defs_by_id[task.id],
                    attempt=attempt,
                    receipts=attempt_receipts,
                    stages=attempt_stages,
                    final_state_observations=final_state_observations,
                ),
            )
            grade_metrics.append(MetricObservation(
                metric_id=_MODEL_BOUNDARY_RAW_SECRET_GRADER_ID,
                attempt_id=attempt_id,
                run_id=run_id,
                arm_id=arm_def.arm_id,
                task_id=task.id,
                value=model_boundary_grade.value,
                unit="raw_occurrences_per_injected_canary",
                denominator_contribution=sum(
                    assertion.expected_occurrences
                    for assertion in task.metadata.sensitive_canary_annotations
                ),
                verification_status=model_boundary_grade.verification_status,
                grader_class=GraderClass.DETERMINISTIC,
                evidence_refs=model_boundary_grade.evidence_refs,
            ))
        if task.metadata.rehydration_assertions:
            rehydration_grade = grade_deterministically(
                _EXACT_LOCAL_REHYDRATION_GRADER_ID,
                _GRADER_VERSION,
                DeterministicGradingContext(
                    task=task_defs_by_id[task.id],
                    attempt=attempt,
                    receipts=attempt_receipts,
                    stages=attempt_stages,
                    rehydration_observations=rehydration_observations,
                ),
            )
            grade_metrics.append(MetricObservation(
                metric_id=_EXACT_LOCAL_REHYDRATION_GRADER_ID,
                attempt_id=attempt_id,
                run_id=run_id,
                arm_id=arm_def.arm_id,
                task_id=task.id,
                value=rehydration_grade.value,
                unit="proportion",
                eligible=rehydration_grade.verification_status == VerificationStatus.VERIFIED,
                denominator_contribution=rehydration_grade.denominator_contribution,
                verification_status=rehydration_grade.verification_status,
                grader_class=GraderClass.DETERMINISTIC,
                evidence_refs=rehydration_grade.evidence_refs,
            ))
        if task.metadata.secret_detection_assertions:
            for grader_id in (
                _SECRET_DETECTION_PRECISION_GRADER_ID,
                _SECRET_DETECTION_RECALL_GRADER_ID,
            ):
                secret_detection_grade = grade_deterministically(
                    grader_id,
                    _GRADER_VERSION,
                    DeterministicGradingContext(
                        task=task_defs_by_id[task.id],
                        attempt=attempt,
                        receipts=attempt_receipts,
                        stages=attempt_stages,
                        secret_detection_observations=secret_detection_observations,
                    ),
                )
                grade_metrics.append(MetricObservation(
                    metric_id=grader_id,
                    attempt_id=attempt_id,
                    run_id=run_id,
                    arm_id=arm_def.arm_id,
                    task_id=task.id,
                    value=secret_detection_grade.value,
                    unit="proportion",
                    eligible=(
                        secret_detection_grade.verification_status
                        == VerificationStatus.VERIFIED
                    ),
                    denominator_contribution=(
                        secret_detection_grade.denominator_contribution
                    ),
                    verification_status=secret_detection_grade.verification_status,
                    grader_class=GraderClass.DETERMINISTIC,
                    evidence_refs=secret_detection_grade.evidence_refs,
                ))
        if task.metadata.expected_final_state_assertions:
            final_state_grade = grade_deterministically(
                _FINAL_STATE_GRADER_ID,
                _GRADER_VERSION,
                DeterministicGradingContext(
                    task=task_defs_by_id[task.id],
                    attempt=attempt,
                    receipts=attempt_receipts,
                    stages=attempt_stages,
                    final_state_observations=final_state_observations,
                ),
            )
            grade_metrics.append(MetricObservation(
                metric_id=_FINAL_STATE_METRIC_ID,
                attempt_id=attempt_id,
                run_id=run_id,
                arm_id=arm_def.arm_id,
                task_id=task.id,
                value=final_state_grade.value,
                unit="proportion",
                denominator_contribution=len(task.metadata.expected_final_state_assertions),
                verification_status=final_state_grade.verification_status,
                grader_class=GraderClass.DETERMINISTIC,
                evidence_refs=final_state_grade.evidence_refs,
            ))
        if task.metadata.state_fixture:
            independent_state_grade = grade_deterministically(
                _INDEPENDENT_STATE_GRADER_ID,
                _GRADER_VERSION,
                DeterministicGradingContext(
                    task=task_defs_by_id[task.id],
                    attempt=attempt,
                    receipts=attempt_receipts,
                    stages=attempt_stages,
                    state_observations=state_observations,
                ),
            )
            grade_metrics.append(MetricObservation(
                metric_id=_INDEPENDENT_STATE_METRIC_ID,
                attempt_id=attempt_id,
                run_id=run_id,
                arm_id=arm_def.arm_id,
                task_id=task.id,
                value=independent_state_grade.value,
                unit="proportion",
                denominator_contribution=len(task.metadata.state_fixture.assertions),
                verification_status=independent_state_grade.verification_status,
                grader_class=GraderClass.DETERMINISTIC,
                evidence_refs=independent_state_grade.evidence_refs,
            ))
        if task.metadata.expected_allow_block_outcome:
            policy_grade = grade_deterministically(
                _POLICY_OUTCOME_GRADER_ID,
                _GRADER_VERSION,
                DeterministicGradingContext(
                    task=task_defs_by_id[task.id],
                    attempt=attempt,
                    receipts=attempt_receipts,
                    stages=attempt_stages,
                    final_state_observations=final_state_observations,
                ),
            )
            grade_metrics.append(MetricObservation(
                metric_id=_POLICY_OUTCOME_GRADER_ID,
                attempt_id=attempt_id,
                run_id=run_id,
                arm_id=arm_def.arm_id,
                task_id=task.id,
                value=policy_grade.value,
                unit="boolean",
                verification_status=policy_grade.verification_status,
                grader_class=GraderClass.DETERMINISTIC,
                evidence_refs=policy_grade.evidence_refs,
            ))
        if task.metadata.unauthorized_mutation_assertions:
            unauthorized_mutation_grade = grade_deterministically(
                _UNAUTHORIZED_MUTATION_GRADER_ID,
                _GRADER_VERSION,
                DeterministicGradingContext(
                    task=task_defs_by_id[task.id],
                    attempt=attempt,
                    receipts=attempt_receipts,
                    stages=attempt_stages,
                    unauthorized_mutation_observations=unauthorized_mutation_observations,
                ),
            )
            grade_metrics.append(MetricObservation(
                metric_id=_UNAUTHORIZED_MUTATION_GRADER_ID,
                attempt_id=attempt_id,
                run_id=run_id,
                arm_id=arm_def.arm_id,
                task_id=task.id,
                value=unauthorized_mutation_grade.value,
                unit="proportion",
                eligible=unauthorized_mutation_grade.verification_status == VerificationStatus.VERIFIED,
                denominator_contribution=unauthorized_mutation_grade.denominator_contribution,
                verification_status=unauthorized_mutation_grade.verification_status,
                grader_class=GraderClass.DETERMINISTIC,
                evidence_refs=unauthorized_mutation_grade.evidence_refs,
            ))
        if task.metadata.token_store_persistence_assertions:
            token_store_persistence_grade = grade_deterministically(
                _TOKEN_STORE_PERSISTENCE_GRADER_ID,
                _GRADER_VERSION,
                DeterministicGradingContext(
                    task=task_defs_by_id[task.id],
                    attempt=attempt,
                    receipts=attempt_receipts,
                    stages=attempt_stages,
                    token_store_persistence_observations=token_store_persistence_observations,
                ),
            )
            grade_metrics.append(MetricObservation(
                metric_id=_TOKEN_STORE_PERSISTENCE_GRADER_ID,
                attempt_id=attempt_id,
                run_id=run_id,
                arm_id=arm_def.arm_id,
                task_id=task.id,
                value=token_store_persistence_grade.value,
                unit="proportion",
                eligible=token_store_persistence_grade.verification_status == VerificationStatus.VERIFIED,
                denominator_contribution=token_store_persistence_grade.denominator_contribution,
                verification_status=token_store_persistence_grade.verification_status,
                grader_class=GraderClass.DETERMINISTIC,
                evidence_refs=token_store_persistence_grade.evidence_refs,
            ))
        if task.metadata.token_ttl_expiry_assertions:
            token_ttl_expiry_grade = grade_deterministically(
                _TOKEN_TTL_EXPIRY_GRADER_ID,
                _GRADER_VERSION,
                DeterministicGradingContext(
                    task=task_defs_by_id[task.id],
                    attempt=attempt,
                    receipts=attempt_receipts,
                    stages=attempt_stages,
                    token_ttl_expiry_observations=token_ttl_expiry_observations,
                ),
            )
            grade_metrics.append(MetricObservation(
                metric_id=_TOKEN_TTL_EXPIRY_GRADER_ID,
                attempt_id=attempt_id,
                run_id=run_id,
                arm_id=arm_def.arm_id,
                task_id=task.id,
                value=token_ttl_expiry_grade.value,
                unit="proportion",
                eligible=token_ttl_expiry_grade.verification_status == VerificationStatus.VERIFIED,
                denominator_contribution=token_ttl_expiry_grade.denominator_contribution,
                verification_status=token_ttl_expiry_grade.verification_status,
                grader_class=GraderClass.DETERMINISTIC,
                evidence_refs=token_ttl_expiry_grade.evidence_refs,
            ))
        if task.metadata.token_persistence_failure_assertions:
            token_persistence_failure_grade = grade_deterministically(
                _TOKEN_PERSISTENCE_FAILURE_GRADER_ID,
                _GRADER_VERSION,
                DeterministicGradingContext(
                    task=task_defs_by_id[task.id],
                    attempt=attempt,
                    receipts=attempt_receipts,
                    stages=attempt_stages,
                    token_persistence_failure_observations=token_persistence_failure_observations,
                ),
            )
            grade_metrics.append(MetricObservation(
                metric_id=_TOKEN_PERSISTENCE_FAILURE_GRADER_ID,
                attempt_id=attempt_id,
                run_id=run_id,
                arm_id=arm_def.arm_id,
                task_id=task.id,
                value=token_persistence_failure_grade.value,
                unit="proportion",
                eligible=token_persistence_failure_grade.verification_status == VerificationStatus.VERIFIED,
                denominator_contribution=token_persistence_failure_grade.denominator_contribution,
                verification_status=token_persistence_failure_grade.verification_status,
                grader_class=GraderClass.DETERMINISTIC,
                evidence_refs=token_persistence_failure_grade.evidence_refs,
            ))
        if task.metadata.exfiltration_attempt_assertions:
            exfiltration_attempt_grade = grade_deterministically(
                _EXFILTRATION_ATTEMPT_GRADER_ID,
                _GRADER_VERSION,
                DeterministicGradingContext(
                    task=task_defs_by_id[task.id],
                    attempt=attempt,
                    receipts=attempt_receipts,
                    stages=attempt_stages,
                    exfiltration_attempt_observations=exfiltration_attempt_observations,
                ),
            )
            grade_metrics.append(MetricObservation(
                metric_id=_EXFILTRATION_ATTEMPT_GRADER_ID,
                attempt_id=attempt_id,
                run_id=run_id,
                arm_id=arm_def.arm_id,
                task_id=task.id,
                value=exfiltration_attempt_grade.value,
                unit="proportion",
                eligible=exfiltration_attempt_grade.verification_status == VerificationStatus.VERIFIED,
                denominator_contribution=exfiltration_attempt_grade.denominator_contribution,
                verification_status=exfiltration_attempt_grade.verification_status,
                grader_class=GraderClass.DETERMINISTIC,
                evidence_refs=exfiltration_attempt_grade.evidence_refs,
            ))
        if task.metadata.artifact_leakage_assertions:
            artifact_leakage_grade = grade_deterministically(
                _ARTIFACT_LEAKAGE_GRADER_ID,
                _GRADER_VERSION,
                DeterministicGradingContext(
                    task=task_defs_by_id[task.id],
                    attempt=attempt,
                    receipts=attempt_receipts,
                    stages=attempt_stages,
                    artifact_leakage_observations=artifact_leakage_observations,
                ),
            )
            grade_metrics.append(MetricObservation(
                metric_id=_ARTIFACT_LEAKAGE_GRADER_ID,
                attempt_id=attempt_id,
                run_id=run_id,
                arm_id=arm_def.arm_id,
                task_id=task.id,
                value=artifact_leakage_grade.value,
                unit="proportion",
                eligible=artifact_leakage_grade.verification_status == VerificationStatus.VERIFIED,
                denominator_contribution=artifact_leakage_grade.denominator_contribution,
                verification_status=artifact_leakage_grade.verification_status,
                grader_class=GraderClass.DETERMINISTIC,
                evidence_refs=artifact_leakage_grade.evidence_refs,
            ))
        if task.metadata.replay_attempt_assertions:
            replay_attempt_grade = grade_deterministically(
                _REPLAY_ATTEMPT_GRADER_ID,
                _GRADER_VERSION,
                DeterministicGradingContext(
                    task=task_defs_by_id[task.id],
                    attempt=attempt,
                    receipts=attempt_receipts,
                    stages=attempt_stages,
                    replay_attempt_observations=replay_attempt_observations,
                ),
            )
            grade_metrics.append(MetricObservation(
                metric_id=_REPLAY_ATTEMPT_GRADER_ID,
                attempt_id=attempt_id,
                run_id=run_id,
                arm_id=arm_def.arm_id,
                task_id=task.id,
                value=replay_attempt_grade.value,
                unit="proportion",
                eligible=replay_attempt_grade.verification_status == VerificationStatus.VERIFIED,
                denominator_contribution=replay_attempt_grade.denominator_contribution,
                verification_status=replay_attempt_grade.verification_status,
                grader_class=GraderClass.DETERMINISTIC,
                evidence_refs=replay_attempt_grade.evidence_refs,
            ))
        if task.metadata.signed_field_tampering_assertions:
            signed_field_tampering_grade = grade_deterministically(
                _SIGNED_FIELD_TAMPERING_GRADER_ID,
                _GRADER_VERSION,
                DeterministicGradingContext(
                    task=task_defs_by_id[task.id],
                    attempt=attempt,
                    receipts=attempt_receipts,
                    stages=attempt_stages,
                    signed_field_tampering_observations=signed_field_tampering_observations,
                ),
            )
            grade_metrics.append(MetricObservation(
                metric_id=_SIGNED_FIELD_TAMPERING_GRADER_ID,
                attempt_id=attempt_id,
                run_id=run_id,
                arm_id=arm_def.arm_id,
                task_id=task.id,
                value=signed_field_tampering_grade.value,
                unit="proportion",
                eligible=signed_field_tampering_grade.verification_status == VerificationStatus.VERIFIED,
                denominator_contribution=signed_field_tampering_grade.denominator_contribution,
                verification_status=signed_field_tampering_grade.verification_status,
                grader_class=GraderClass.DETERMINISTIC,
                evidence_refs=signed_field_tampering_grade.evidence_refs,
            ))
        if task.metadata.payload_tampering_assertions:
            payload_tampering_grade = grade_deterministically(
                _PAYLOAD_TAMPERING_GRADER_ID,
                _GRADER_VERSION,
                DeterministicGradingContext(
                    task=task_defs_by_id[task.id],
                    attempt=attempt,
                    receipts=attempt_receipts,
                    stages=attempt_stages,
                    payload_tampering_observations=payload_tampering_observations,
                ),
            )
            grade_metrics.append(MetricObservation(
                metric_id=_PAYLOAD_TAMPERING_GRADER_ID,
                attempt_id=attempt_id,
                run_id=run_id,
                arm_id=arm_def.arm_id,
                task_id=task.id,
                value=payload_tampering_grade.value,
                unit="proportion",
                eligible=payload_tampering_grade.verification_status == VerificationStatus.VERIFIED,
                denominator_contribution=payload_tampering_grade.denominator_contribution,
                verification_status=payload_tampering_grade.verification_status,
                grader_class=GraderClass.DETERMINISTIC,
                evidence_refs=payload_tampering_grade.evidence_refs,
            ))
        if task.metadata.stale_state_root_assertions:
            stale_state_root_grade = grade_deterministically(
                _STALE_STATE_ROOT_GRADER_ID,
                _GRADER_VERSION,
                DeterministicGradingContext(
                    task=task_defs_by_id[task.id],
                    attempt=attempt,
                    receipts=attempt_receipts,
                    stages=attempt_stages,
                    stale_state_root_observations=stale_state_root_observations,
                ),
            )
            grade_metrics.append(MetricObservation(
                metric_id=_STALE_STATE_ROOT_GRADER_ID,
                attempt_id=attempt_id,
                run_id=run_id,
                arm_id=arm_def.arm_id,
                task_id=task.id,
                value=stale_state_root_grade.value,
                unit="proportion",
                eligible=stale_state_root_grade.verification_status == VerificationStatus.VERIFIED,
                denominator_contribution=stale_state_root_grade.denominator_contribution,
                verification_status=stale_state_root_grade.verification_status,
                grader_class=GraderClass.DETERMINISTIC,
                evidence_refs=stale_state_root_grade.evidence_refs,
            ))
        if task.metadata.identity_mismatch_assertions:
            identity_mismatch_grade = grade_deterministically(
                _IDENTITY_MISMATCH_GRADER_ID,
                _GRADER_VERSION,
                DeterministicGradingContext(
                    task=task_defs_by_id[task.id],
                    attempt=attempt,
                    receipts=attempt_receipts,
                    stages=attempt_stages,
                    identity_mismatch_observations=identity_mismatch_observations,
                ),
            )
            grade_metrics.append(MetricObservation(
                metric_id=_IDENTITY_MISMATCH_GRADER_ID,
                attempt_id=attempt_id,
                run_id=run_id,
                arm_id=arm_def.arm_id,
                task_id=task.id,
                value=identity_mismatch_grade.value,
                unit="proportion",
                eligible=identity_mismatch_grade.verification_status == VerificationStatus.VERIFIED,
                denominator_contribution=identity_mismatch_grade.denominator_contribution,
                verification_status=identity_mismatch_grade.verification_status,
                grader_class=GraderClass.DETERMINISTIC,
                evidence_refs=identity_mismatch_grade.evidence_refs,
            ))
        if task.metadata.nonce_expiration_assertions:
            nonce_expiration_grade = grade_deterministically(
                _NONCE_EXPIRATION_GRADER_ID,
                _GRADER_VERSION,
                DeterministicGradingContext(
                    task=task_defs_by_id[task.id],
                    attempt=attempt,
                    receipts=attempt_receipts,
                    stages=attempt_stages,
                    nonce_expiration_observations=nonce_expiration_observations,
                ),
            )
            grade_metrics.append(MetricObservation(
                metric_id=_NONCE_EXPIRATION_GRADER_ID,
                attempt_id=attempt_id,
                run_id=run_id,
                arm_id=arm_def.arm_id,
                task_id=task.id,
                value=nonce_expiration_grade.value,
                unit="proportion",
                eligible=nonce_expiration_grade.verification_status == VerificationStatus.VERIFIED,
                denominator_contribution=nonce_expiration_grade.denominator_contribution,
                verification_status=nonce_expiration_grade.verification_status,
                grader_class=GraderClass.DETERMINISTIC,
                evidence_refs=nonce_expiration_grade.evidence_refs,
            ))
        if task.metadata.signer_defect_assertions:
            signer_defect_grade = grade_deterministically(
                _SIGNER_DEFECT_GRADER_ID,
                _GRADER_VERSION,
                DeterministicGradingContext(
                    task=task_defs_by_id[task.id],
                    attempt=attempt,
                    receipts=attempt_receipts,
                    stages=attempt_stages,
                    signer_defect_observations=signer_defect_observations,
                ),
            )
            grade_metrics.append(MetricObservation(
                metric_id=_SIGNER_DEFECT_GRADER_ID,
                attempt_id=attempt_id,
                run_id=run_id,
                arm_id=arm_def.arm_id,
                task_id=task.id,
                value=signer_defect_grade.value,
                unit="proportion",
                eligible=signer_defect_grade.verification_status == VerificationStatus.VERIFIED,
                denominator_contribution=signer_defect_grade.denominator_contribution,
                verification_status=signer_defect_grade.verification_status,
                grader_class=GraderClass.DETERMINISTIC,
                evidence_refs=signer_defect_grade.evidence_refs,
            ))
        if task.metadata.l3_proof_transplant_assertions:
            l3_proof_transplant_grade = grade_deterministically(
                _L3_PROOF_TRANSPLANT_GRADER_ID,
                _GRADER_VERSION,
                DeterministicGradingContext(
                    task=task_defs_by_id[task.id],
                    attempt=attempt,
                    receipts=attempt_receipts,
                    stages=attempt_stages,
                    l3_proof_transplant_observations=l3_proof_transplant_observations,
                ),
            )
            grade_metrics.append(MetricObservation(
                metric_id=_L3_PROOF_TRANSPLANT_GRADER_ID,
                attempt_id=attempt_id,
                run_id=run_id,
                arm_id=arm_def.arm_id,
                task_id=task.id,
                value=l3_proof_transplant_grade.value,
                unit="proportion",
                eligible=l3_proof_transplant_grade.verification_status == VerificationStatus.VERIFIED,
                denominator_contribution=l3_proof_transplant_grade.denominator_contribution,
                verification_status=l3_proof_transplant_grade.verification_status,
                grader_class=GraderClass.DETERMINISTIC,
                evidence_refs=l3_proof_transplant_grade.evidence_refs,
            ))
        if task.metadata.revoked_credential_assertions:
            revoked_credential_grade = grade_deterministically(
                _REVOKED_CREDENTIAL_GRADER_ID,
                _GRADER_VERSION,
                DeterministicGradingContext(
                    task=task_defs_by_id[task.id],
                    attempt=attempt,
                    receipts=attempt_receipts,
                    stages=attempt_stages,
                    revoked_credential_observations=revoked_credential_observations,
                ),
            )
            grade_metrics.append(MetricObservation(
                metric_id=_REVOKED_CREDENTIAL_GRADER_ID,
                attempt_id=attempt_id,
                run_id=run_id,
                arm_id=arm_def.arm_id,
                task_id=task.id,
                value=revoked_credential_grade.value,
                unit="proportion",
                eligible=revoked_credential_grade.verification_status == VerificationStatus.VERIFIED,
                denominator_contribution=revoked_credential_grade.denominator_contribution,
                verification_status=revoked_credential_grade.verification_status,
                grader_class=GraderClass.DETERMINISTIC,
                evidence_refs=revoked_credential_grade.evidence_refs,
            ))
        if task.metadata.evidence_preservation_assertions:
            evidence_preservation_grade = grade_deterministically(
                _EVIDENCE_PRESERVATION_GRADER_ID,
                _GRADER_VERSION,
                DeterministicGradingContext(
                    task=task_defs_by_id[task.id],
                    attempt=attempt,
                    receipts=attempt_receipts,
                    stages=attempt_stages,
                    evidence_preservation_observations=evidence_preservation_observations,
                ),
            )
            grade_metrics.append(MetricObservation(
                metric_id=_EVIDENCE_PRESERVATION_GRADER_ID,
                attempt_id=attempt_id,
                run_id=run_id,
                arm_id=arm_def.arm_id,
                task_id=task.id,
                value=evidence_preservation_grade.value,
                unit="proportion",
                eligible=evidence_preservation_grade.verification_status == VerificationStatus.VERIFIED,
                denominator_contribution=evidence_preservation_grade.denominator_contribution,
                verification_status=evidence_preservation_grade.verification_status,
                grader_class=GraderClass.DETERMINISTIC,
                evidence_refs=evidence_preservation_grade.evidence_refs,
            ))
        for exclusion in task_defs_by_id[task.id].unsupported_exclusions:
            if DEFAULT_METRIC_REGISTRY.is_registered(
                exclusion.grader_id, exclusion.grader_version,
            ):
                grade_metrics.append(MetricObservation(
                    metric_id=exclusion.grader_id,
                    metric_version=exclusion.grader_version,
                    attempt_id=attempt_id,
                    run_id=run_id,
                    arm_id=arm_def.arm_id,
                    task_id=task.id,
                    value=None,
                    unit=DEFAULT_METRIC_REGISTRY.get(
                        exclusion.grader_id, exclusion.grader_version,
                    ).unit,
                    eligible=False,
                    denominator_contribution=0,
                    verification_status=VerificationStatus.NOT_APPLICABLE,
                    grader_class=exclusion.grader_class,
                    evidence_refs=[],
                ))
        for metric in grade_metrics:
            DEFAULT_METRIC_REGISTRY.validate(metric)
        metric_records.extend(grade_metrics)
        attempt.grade_refs = [metric.metric_id for metric in grade_metrics]
        attempt.unsupported_exclusion_refs = [
            exclusion.exclusion_id
            for exclusion in task_defs_by_id[task.id].unsupported_exclusions
        ]
        attempt_records.append(attempt)

        status_color = "green" if score.passed else "red"
        receipt_status = ""
        if response.binding == BindingType.RECEIPT_BOUND:
            if response.receipts_verified:
                receipt_status = f" [cyan]({len(response.receipts)} receipts verified)[/cyan]"
            else:
                receipt_status = f" [yellow]({len(response.receipts)} receipts not fully verified)[/yellow]"
        elif response.unbound_reason:
            receipt_status = f" [yellow]({response.unbound_reason})[/yellow]"

        event_count = response.chat_evidence.event_count if response.chat_evidence else 0

        console.print(
            f"  [dim]{task.id}[/dim] [{status_color}]{'PASS' if score.passed else 'FAIL'}[/{status_color}]"
            f" answer_chars={len(response.answer or '')}"
            f" agent_events={event_count}{receipt_status}"
        )

    # 9. Write attempts.jsonl
    with open(report_dir / "attempts.jsonl", "w") as f:
        for ar in attempt_records:
            f.write(ar.model_dump_json() + "\n")

    with open(report_dir / "receipts.jsonl", "w") as f:
        for receipt in receipt_records:
            f.write(receipt.model_dump_json() + "\n")

    with open(report_dir / "final-state-observations.jsonl", "w") as f:
        for observation in final_state_records:
            f.write(observation.model_dump_json() + "\n")

    with open(report_dir / "state-observations.jsonl", "w") as f:
        for observation in state_records:
            f.write(observation.model_dump_json() + "\n")

    with open(report_dir / "rehydration-observations.jsonl", "w") as f:
        for observation in rehydration_records:
            f.write(observation.model_dump_json() + "\n")

    with open(report_dir / "secret-detection-observations.jsonl", "w") as f:
        for observation in secret_detection_records:
            f.write(observation.model_dump_json() + "\n")

    with open(report_dir / "unauthorized-mutation-observations.jsonl", "w") as f:
        for observation in unauthorized_mutation_records:
            f.write(observation.model_dump_json() + "\n")

    with open(report_dir / "token-store-persistence-observations.jsonl", "w") as f:
        for observation in token_store_persistence_records:
            f.write(observation.model_dump_json() + "\n")

    with open(report_dir / "token-ttl-expiry-observations.jsonl", "w") as f:
        for observation in token_ttl_expiry_records:
            f.write(observation.model_dump_json() + "\n")

    with open(report_dir / "token-persistence-failure-observations.jsonl", "w") as f:
        for observation in token_persistence_failure_records:
            f.write(observation.model_dump_json() + "\n")

    with open(report_dir / "exfiltration-attempt-observations.jsonl", "w") as f:
        for observation in exfiltration_attempt_records:
            f.write(observation.model_dump_json() + "\n")

    with open(report_dir / "artifact-leakage-observations.jsonl", "w") as f:
        for observation in artifact_leakage_records:
            f.write(observation.model_dump_json() + "\n")

    with open(report_dir / "replay-attempt-observations.jsonl", "w") as f:
        for observation in replay_attempt_records:
            f.write(observation.model_dump_json() + "\n")

    with open(report_dir / "signed-field-tampering-observations.jsonl", "w") as f:
        for observation in signed_field_tampering_records:
            f.write(observation.model_dump_json() + "\n")

    with open(report_dir / "payload-tampering-observations.jsonl", "w") as f:
        for observation in payload_tampering_records:
            f.write(observation.model_dump_json() + "\n")

    with open(report_dir / "stale-state-root-observations.jsonl", "w") as f:
        for observation in stale_state_root_records:
            f.write(observation.model_dump_json() + "\n")

    with open(report_dir / "identity-mismatch-observations.jsonl", "w") as f:
        for observation in identity_mismatch_records:
            f.write(observation.model_dump_json() + "\n")

    with open(report_dir / "nonce-expiration-observations.jsonl", "w") as f:
        for observation in nonce_expiration_records:
            f.write(observation.model_dump_json() + "\n")

    with open(report_dir / "signer-defect-observations.jsonl", "w") as f:
        for observation in signer_defect_records:
            f.write(observation.model_dump_json() + "\n")

    with open(report_dir / "l3-proof-transplant-observations.jsonl", "w") as f:
        for observation in l3_proof_transplant_records:
            f.write(observation.model_dump_json() + "\n")

    with open(report_dir / "revoked-credential-observations.jsonl", "w") as f:
        for observation in revoked_credential_records:
            f.write(observation.model_dump_json() + "\n")

    with open(report_dir / "evidence-preservation-observations.jsonl", "w") as f:
        for observation in evidence_preservation_records:
            f.write(observation.model_dump_json() + "\n")

    with open(report_dir / "stages.jsonl", "w") as f:
        for stage in stage_records:
            f.write(stage.model_dump_json() + "\n")

    with open(report_dir / "metrics.jsonl", "w") as f:
        for metric in metric_records:
            f.write(metric.model_dump_json() + "\n")

    encrypted_evidence_artifacts = [
        encrypt_evidence_artifact(artifact, evidence_key)
        for artifact in evidence_artifacts
    ]
    with open(report_dir / "evidence-index.jsonl", "w") as f:
        for artifact in encrypted_evidence_artifacts:
            f.write(artifact.index.model_dump_json() + "\n")

    for artifact in encrypted_evidence_artifacts:
        artifact_path = report_dir / artifact.index.storage_location
        artifact_path.parent.mkdir(parents=True, exist_ok=True)
        artifact_path.write_text(artifact.envelope_json)

    # 10. Aggregate & Report
    agg = aggregate_results(suite, results)
    render_summary(agg, arm=config.arm)

    # 11. Save legacy artifacts (results.jsonl, summary.json)
    evidence_index_by_attempt = {
        artifact.index.attempt_id: artifact.index
        for artifact in encrypted_evidence_artifacts
        if artifact.index.attempt_id
    }

    receipt_records_by_attempt = {
        attempt.attempt_id: [
            receipt
            for receipt in receipt_records
            if receipt.attempt_id == attempt.attempt_id
        ]
        for attempt in attempt_records
    }

    def row_to_dict(r: RowResult, attempt: AttemptRecord):
        evidence_index = evidence_index_by_attempt.get(attempt.attempt_id)
        attempt_receipts = receipt_records_by_attempt[attempt.attempt_id]

        details_data = None
        if r.score.details:
            if isinstance(r.score.details, ScoreDetails):
                details_data = r.score.details.model_dump()
            else:
                details_data = r.score.details  # Already a dict

        prompt_bytes = r.task.prompt.encode()
        answer_bytes = (r.response.answer or "").encode()
        return {
            "task_id": r.task.id,
            "prompt_sha256": hashlib.sha256(prompt_bytes).hexdigest(),
            "prompt_length": len(prompt_bytes),
            "answer_sha256": hashlib.sha256(answer_bytes).hexdigest(),
            "answer_length": len(answer_bytes),
            "primary_transaction_id": r.response.primary_transaction_id,
            "chat_evidence_ref": evidence_index.artifact_id if evidence_index else None,
            "chat_evidence_sha256": evidence_index.sha256 if evidence_index else None,
            "receipts": [receipt.model_dump(mode="json") for receipt in attempt_receipts],
            "receipts_verified": r.response.receipts_verified,
            "passed": r.score.passed,
            "details": details_data,
            "timestamp": r.timestamp.isoformat()
        }

    with open(report_dir / "results.jsonl", "w") as f:
        for result, attempt in zip(results, attempt_records, strict=True):
            f.write(json.dumps(row_to_dict(result, attempt)) + "\n")

    with open(report_dir / "summary.json", "w") as f:
        f.write(json.dumps(agg.__dict__, indent=2))

    console.print(f"\n[bold green]Report saved to {report_dir}[/bold green]")

    policy_metrics_by_attempt = {
        metric.attempt_id: metric
        for metric in metric_records
        if metric.metric_id == _POLICY_OUTCOME_GRADER_ID
    }
    invalid_results = []
    for result, attempt in zip(results, attempt_records, strict=True):
        policy_metric = policy_metrics_by_attempt.get(attempt.attempt_id)
        verified_expected_block = (
            result.task.metadata.expected_allow_block_outcome == PolicyOutcome.BLOCK
            and attempt.terminal_status == TerminalStatus.GOVERNANCE_REJECTED
            and policy_metric is not None
            and policy_metric.value == 1.0
            and policy_metric.verification_status == VerificationStatus.VERIFIED
        )
        if (
            not result.response.chat_evidence
            or not result.response.chat_evidence.terminal_event
            or (
                not verified_expected_block
                and (
                    not result.response.answer
                    or result.response.chat_evidence.terminal_event.endswith(
                        ("failed", "stopped", "dead.lettered")
                    )
                    or "HTTP 401" in (result.response.unbound_reason or "")
                    or "HTTP 403" in (result.response.unbound_reason or "")
                )
            )
        ):
            invalid_results.append(result)
    if invalid_results:
        failed_ids = ", ".join(str(result.task.id) for result in invalid_results)
        raise EvaluationRunError(
            f"run produced invalid evidence for task(s) {failed_ids}; diagnostic report retained at {report_dir}"
        )

@main.command()
@click.argument("report_dir", type=click.Path(exists=True, path_type=Path))
@click.option("--pki-dir", type=click.Path(exists=True, path_type=Path))
@click.option("--json", "json_output", is_flag=True, help="Emit a machine-readable verification result")
def verify_receipts(report_dir: Path, pki_dir: Path | None, json_output: bool):
    """Re-verify all receipts in a report directory offline.

    Loads every ``*Actuator_pub.pem`` file in the PKI directory, derives the
    key_id from each PEM, and matches each receipt to its ``signer_key_id``.
    Unified-stack runs produce receipts signed by two distinct actuators
    (gateway and operator), so the verifier needs both keys. A receipt whose
    ``signer_key_id`` has no matching key fails verification.
    """
    if not pki_dir:
        pki_dir = Path(os.environ.get("G8E_GATEWAY_PKI_DIR", ".g8e/pki"))

    keys: dict[str, str] = {}
    for pem_path in sorted(pki_dir.glob("*Actuator_pub.pem")):
        pem = pem_path.read_text()
        key_id = binascii.hexlify(decode_ed25519_public_key(pem)).decode()
        keys[key_id] = pem
        if not json_output:
            console.print(f"  loaded key {key_id[:16]}... from {pem_path.name}")

    if not keys and not json_output:
        console.print(
            f"[bold red]Error:[/bold red] No actuator public keys "
            f"(*Actuator_pub.pem) found in {pki_dir}"
        )
        sys.exit(1)

    receipts_path = report_dir / "receipts.jsonl"
    if not receipts_path.exists():
        raise click.ClickException(f"receipts.jsonl not found in {report_dir}")

    run_id = ""
    if json_output:
        manifest_path = report_dir / "manifest.json"
        if not manifest_path.exists():
            raise click.ClickException(f"manifest.json not found in {report_dir}")
        try:
            run_id = RunManifest.model_validate_json(manifest_path.read_text()).run_id
        except ValueError as exc:
            raise click.ClickException(f"invalid manifest.json in {report_dir}: {exc}") from exc
    else:
        console.print(f"[bold blue]Verifying receipts in {report_dir}...[/bold blue]")

    total = 0
    verified_signatures = 0
    verified_persistence = 0
    failed_signatures = 0
    failed_persistence = 0
    missing_keys = 0
    failed_receipts = 0
    signer_key_ids: set[str] = set()
    receipt_bound_attempt_ids: set[str] = set()
    sample_fingerprints: list[ReceiptFingerprint] = []

    with receipts_path.open() as receipts_file:
        for line in receipts_file:
            total += 1
            try:
                observation = ReceiptObservation.model_validate_json(line)
            except ValueError:
                failed_signatures += 1
                failed_persistence += 1
                failed_receipts += 1
                if not json_output:
                    console.print("  [red]FAILED:[/red] Could not parse typed receipt observation")
                continue

            receipt = observation.action_receipt
            signer_key_ids.add(receipt.signer_key_id)
            if observation.primary:
                receipt_bound_attempt_ids.add(observation.attempt_id)
            public_key = keys.get(receipt.signer_key_id)
            if public_key is None:
                missing_keys += 1
                failed_receipts += 1
                if not json_output:
                    console.print(
                        f"  [red]FAILED:[/red] No key for signer_key_id "
                        f"{receipt.signer_key_id[:16]}... "
                        f"(attempt {observation.attempt_id}, TX: {observation.transaction_id})"
                    )
                continue

            signature_valid = verify_action_receipt_signature(receipt, public_key)
            persistence_valid = verify_receipt_persistence_attestation(receipt, public_key)
            verified_signatures += int(signature_valid)
            verified_persistence += int(persistence_valid)
            failed_signatures += int(not signature_valid)
            failed_persistence += int(not persistence_valid)
            if not signature_valid or not persistence_valid:
                failed_receipts += 1
                if not json_output:
                    console.print(
                        f"  [red]FAILED:[/red] Receipt for attempt {observation.attempt_id} "
                        f"(TX: {observation.transaction_id})"
                    )
            elif len(sample_fingerprints) < _RECEIPT_FINGERPRINT_SAMPLE_LIMIT:
                sample_fingerprints.append(
                    ReceiptFingerprint(
                        receipt_id=observation.receipt_id,
                        signature_digest=hashlib.sha256(receipt.signature.encode()).hexdigest(),
                        artifact_ref="receipts.jsonl",
                    )
                )

    if json_output:
        result = ReceiptVerificationResult(
            schema_version=_RECEIPT_VERIFICATION_SCHEMA_VERSION,
            run_id=run_id,
            verified_at=datetime.now(UTC).isoformat(timespec="seconds").replace("+00:00", "Z"),
            verifier_version=_RECEIPT_VERIFIER_VERSION,
            scope=_RECEIPT_VERIFICATION_SCOPE,
            total_receipts=total,
            verified_signatures=verified_signatures,
            verified_persistence=verified_persistence,
            failed_signatures=failed_signatures,
            failed_persistence=failed_persistence,
            missing_keys=missing_keys,
            distinct_signer_key_ids=tuple(sorted(signer_key_ids)),
            receipt_bound_eligible_attempts=len(receipt_bound_attempt_ids),
            sample_receipt_fingerprints=tuple(sample_fingerprints),
        )
        click.echo(json.dumps(asdict(result), sort_keys=True))
        if failed_signatures or failed_persistence or missing_keys:
            raise click.exceptions.Exit(1)
        return

    if total == 0:
        console.print("[yellow]No bound receipts found in report.[/yellow]")
        return

    status = "green" if failed_receipts == 0 else "red"
    jointly_verified = total - failed_receipts
    console.print(f"\n[{status}]Re-verification complete:[/{status}]")
    console.print(f"  Total receipts: {total}")
    console.print(f"  Verified: {jointly_verified}")
    console.print(f"  Failed: {failed_receipts}")
    console.print(f"  Keys loaded: {len(keys)}")
    console.print(f"  No-key receipts: {missing_keys}")
    if failed_receipts > 0:
        sys.exit(1)


_SYNTHETIC_SUITE_CHOICES = ["privacy_token_lifecycle", "governance_adversarial", "privacy_boundary_leakage", "policy_attack", "benign_overblock", "tool_sequence", "factual_qa", "citation_backed", "partial_milestone", "final_state", "ledger_consistency", "reliability", "economics_performance"]


def _generate_per_run_key() -> bytes:
    """Generate a random 32-byte AES-256 key for a synthetic run.

    The key is generated outside the public report directory and is never
    written to the report tree.  It encrypts restricted evidence (token
    stores and rehydration artifacts) so that ciphertext in the public
    report cannot be decrypted by anyone with access to the source code.
    """
    import secrets
    return secrets.token_bytes(32)


def _extract_canary_values(tasks: list) -> set[str]:
    """Extract all synthetic sensitive values from task scenario params.

    These values are classified as canaries: they must never appear in
    plaintext in any public report artifact.  The regression scan checks
    every file in the report tree against this set.
    """
    canaries: set[str] = set()
    for task in tasks:
        params = task.metadata.benchmark_specific
        for token_spec in params.get("tokens", []):
            if token_spec.get("value"):
                canaries.add(token_spec["value"])
        expired = params.get("expired_token")
        if expired and expired.get("value"):
            canaries.add(expired["value"])
        ttl_token = params.get("ttl_token")
        if ttl_token and ttl_token.get("value"):
            canaries.add(ttl_token["value"])
        for token_spec in params.get("rehydration_tokens", []):
            if token_spec.get("value"):
                canaries.add(token_spec["value"])
    return canaries


def _scan_report_for_canary_leaks(
    report_dir: Path,
    canary_values: set[str],
    per_run_key: bytes,
) -> None:
    """Scan every file in the report tree for raw canary values and the key.

    This is the fail-closed regression gate: no public report artifact may
    contain raw canaries (synthetic sensitive values) or the per-run
    decryption key.  The scan enumerates every file recursively, reads its
    bytes, and checks for any canary value or the key bytes.  If any leak
    is found, the function raises ``EvaluationRunError`` so the run fails
    closed rather than producing a report that leaks sensitive values.
    """
    if not canary_values:
        return
    key_bytes = per_run_key
    key_hex = per_run_key.hex()
    leaks: list[str] = []
    for file_path in sorted(report_dir.rglob("*")):
        if not file_path.is_file():
            continue
        try:
            content = file_path.read_bytes()
        except OSError:
            continue
        text = content.decode("utf-8", errors="replace")
        for canary in canary_values:
            if canary and canary in text:
                rel = file_path.relative_to(report_dir)
                leaks.append(f"raw canary '{canary}' found in {rel}")
        if key_bytes in content:
            rel = file_path.relative_to(report_dir)
            leaks.append(f"per-run key bytes found in {rel}")
        if key_hex in text:
            rel = file_path.relative_to(report_dir)
            leaks.append(f"per-run key hex found in {rel}")
    if leaks:
        raise EvaluationRunError(
            "canary leak scan failed: " + "; ".join(leaks)
        )


@main.command(name="bench-synthetic")
@click.option("--suite", type=click.Choice(_SYNTHETIC_SUITE_CHOICES), required=True)
@click.option("--gold-set", type=click.Path(exists=True, path_type=Path))
@click.option("--output-dir", type=click.Path(path_type=Path), default=Path("reports"))
@click.option("--limit", type=int, help="Limit number of tasks to run")
def bench_synthetic(suite: str, gold_set: Path | None, output_dir: Path, limit: int | None):
    """Run a synthetic deterministic suite without a real LLM provider.

    Synthetic suites exercise the observer and grader pipeline against
    local production-shaped systems under test (e.g. an encrypted token
    store on disk).  No LLM provider, g8ee, operator, or auth context is
    required.  The command wires boundary-specific observers into the
    grading pipeline, collects typed observations, runs deterministic
    graders, validates every metric against the registry, and writes
    per-attempt evidence to a report directory.
    """
    try:
        asyncio.run(_run_synthetic_suite(suite, gold_set, output_dir, limit))
    except EvaluationRunError as error:
        raise click.ClickException(str(error)) from error


async def _run_synthetic_suite(
    suite: str,
    gold_set: Path | None,
    output_dir: Path,
    limit: int | None,
) -> None:
    if suite == "privacy_token_lifecycle":
        if gold_set is None:
            gold_set = Path("gold_sets/privacy_token_lifecycle/input_data.jsonl")
        loader = PrivacyTokenLifecycleLoader(gold_set)
        tasks = list(loader.load())
        provenance = load_synthetic_provenance(gold_set.with_name("provenance.json"))
    elif suite == "governance_adversarial":
        if gold_set is None:
            gold_set = Path("gold_sets/governance_adversarial/input_data.jsonl")
        loader = GovernanceAdversarialLoader(gold_set)
        tasks = list(loader.load())
        provenance = load_synthetic_provenance(gold_set.with_name("provenance.json"))
    elif suite == "privacy_boundary_leakage":
        if gold_set is None:
            gold_set = Path("gold_sets/privacy_boundary_leakage/input_data.jsonl")
        loader = PrivacyBoundaryLeakageLoader(gold_set)
        tasks = list(loader.load())
        provenance = load_synthetic_provenance(gold_set.with_name("provenance.json"))
    elif suite == "policy_attack":
        if gold_set is None:
            gold_set = Path("gold_sets/policy_attack/input_data.jsonl")
        loader = PolicyAttackLoader(gold_set)
        tasks = list(loader.load())
        provenance = load_synthetic_provenance(gold_set.with_name("provenance.json"))
    elif suite == "benign_overblock":
        if gold_set is None:
            gold_set = Path("gold_sets/benign_overblock/input_data.jsonl")
        loader = BenignOverblockLoader(gold_set)
        tasks = list(loader.load())
        provenance = load_synthetic_provenance(gold_set.with_name("provenance.json"))
    elif suite == "tool_sequence":
        if gold_set is None:
            gold_set = Path("gold_sets/utility/input_data.jsonl")
        loader = ToolSequenceLoader(gold_set)
        tasks = list(loader.load())
        provenance = load_synthetic_provenance(gold_set.with_name("provenance.json"))
    elif suite == "factual_qa":
        if gold_set is None:
            gold_set = Path("gold_sets/factual_qa/input_data.jsonl")
        loader = FactualQALoader(gold_set)
        tasks = list(loader.load())
        provenance = load_synthetic_provenance(gold_set.with_name("provenance.json"))
    elif suite == "citation_backed":
        if gold_set is None:
            gold_set = Path("gold_sets/citation_backed/input_data.jsonl")
        loader = CitationBackedLoader(gold_set)
        tasks = list(loader.load())
        provenance = load_synthetic_provenance(gold_set.with_name("provenance.json"))
    elif suite == "partial_milestone":
        if gold_set is None:
            gold_set = Path("gold_sets/partial_milestone/input_data.jsonl")
        loader = PartialMilestoneLoader(gold_set)
        tasks = list(loader.load())
        provenance = load_synthetic_provenance(gold_set.with_name("provenance.json"))
    elif suite == "final_state":
        if gold_set is None:
            gold_set = Path("gold_sets/final_state/input_data.jsonl")
        loader = FinalStateLoader(gold_set)
        tasks = list(loader.load())
        provenance = load_synthetic_provenance(gold_set.with_name("provenance.json"))
    elif suite == "ledger_consistency":
        if gold_set is None:
            gold_set = Path("gold_sets/ledger_consistency/input_data.jsonl")
        loader = LedgerConsistencyLoader(gold_set)
        tasks = list(loader.load())
        provenance = load_synthetic_provenance(gold_set.with_name("provenance.json"))
    elif suite == "reliability":
        if gold_set is None:
            gold_set = Path("gold_sets/reliability/input_data.jsonl")
        loader = ReliabilityLoader(gold_set)
        tasks = list(loader.load())
        provenance = load_synthetic_provenance(gold_set.with_name("provenance.json"))
    elif suite == "economics_performance":
        if gold_set is None:
            gold_set = Path("gold_sets/economics_performance/input_data.jsonl")
        loader = EconomicsPerformanceLoader(gold_set)
        tasks = list(loader.load())
        provenance = load_synthetic_provenance(gold_set.with_name("provenance.json"))
    else:
        raise EvaluationRunError(f"unknown synthetic suite: {suite}")

    if limit:
        tasks = tasks[:limit]

    suite_id = provenance.benchmark
    suite_version = provenance.output.sha256[:12]
    dataset_hash = provenance.output.sha256

    run_id = str(uuid.uuid4())
    prompt_bundle_content = "\n".join(t.prompt for t in tasks).encode()
    prompt_bundle_hash = hashlib.sha256(prompt_bundle_content).hexdigest()

    grader_bundle_parts: list[str] = [f"{suite}:{suite_version}"]
    for task in tasks:
        for ref in _derive_grader_refs_from_assertions(task.metadata):
            grader_bundle_parts.append(f"{ref.grader_id}@{ref.grader_version}")
    grader_bundle_content = ":".join(grader_bundle_parts).encode()
    grader_bundle_hash = hashlib.sha256(grader_bundle_content).hexdigest()

    content_hashes = [
        ContentHash(name="dataset", sha256=dataset_hash, byte_length=gold_set.stat().st_size),
        ContentHash(name="prompt_bundle", sha256=prompt_bundle_hash, byte_length=len(prompt_bundle_content)),
        ContentHash(name="grader_bundle", sha256=grader_bundle_hash, byte_length=len(grader_bundle_content)),
    ]

    arm_entry = ArmManifestEntry(
        arm_id=Arm.DIRECT,
        requested_posture=GovernancePosture.NONE,
        uses_g8ee=False,
        uses_gateway=False,
        receipt_binding=False,
        is_production_posture=False,
    )

    manifest = RunManifest(
        run_id=run_id,
        suite_id=suite_id,
        suite_version=suite_version,
        orchestrator_version=EVALS_VERSION,
        arms=[arm_entry],
        content_hashes=content_hashes,
        role_to_model=RoleToModelMapping(),
        stack_environment=StackEnvironment(
            os=platform.platform(),
            arch=platform.machine(),
            runtime_version=platform.python_version(),
        ),
    )

    ts = datetime.now(UTC).strftime("%Y%m%d-%H%M%S")
    report_dir = output_dir / f"{suite}-synthetic-{ts}"
    report_dir.mkdir(parents=True, exist_ok=True)
    (report_dir / "manifest.json").write_text(manifest.model_dump_json(indent=2))

    per_run_key = _generate_per_run_key()
    canary_values = _extract_canary_values(tasks)

    task_defs: list[TaskDefinition] = []
    for task in tasks:
        grader_refs = _derive_grader_refs_from_assertions(task.metadata)
        if not grader_refs:
            raise EvaluationRunError(
                f"task {task.id} has no applicable grader: no typed assertions declared"
            )
        task_defs.append(TaskDefinition(
            task_id=task.id,
            suite_id=suite_id,
            suite_version=suite_version,
            category=task.metadata.category or "privacy",
            expected_action_class=task.metadata.expected_action_class,
            compatible_arms=[Arm.DIRECT, *GOVERNED_ARMS],
            prompt_hash=hashlib.sha256(task.prompt.encode()).hexdigest(),
            prompt_length=len(task.prompt),
            token_store_persistence_assertions=task.metadata.token_store_persistence_assertions,
            token_ttl_expiry_assertions=task.metadata.token_ttl_expiry_assertions,
            token_persistence_failure_assertions=task.metadata.token_persistence_failure_assertions,
            exfiltration_attempt_assertions=task.metadata.exfiltration_attempt_assertions,
            artifact_leakage_assertions=task.metadata.artifact_leakage_assertions,
            rehydration_assertions=task.metadata.rehydration_assertions,
            replay_attempt_assertions=task.metadata.replay_attempt_assertions,
            signed_field_tampering_assertions=task.metadata.signed_field_tampering_assertions,
            payload_tampering_assertions=task.metadata.payload_tampering_assertions,
            nonce_expiration_assertions=task.metadata.nonce_expiration_assertions,
            stale_state_root_assertions=task.metadata.stale_state_root_assertions,
            identity_mismatch_assertions=task.metadata.identity_mismatch_assertions,
            signer_defect_assertions=task.metadata.signer_defect_assertions,
            l3_proof_transplant_assertions=task.metadata.l3_proof_transplant_assertions,
            revoked_credential_assertions=task.metadata.revoked_credential_assertions,
            evidence_preservation_assertions=task.metadata.evidence_preservation_assertions,
            policy_attack_assertions=task.metadata.policy_attack_assertions,
            tool_sequence_assertions=task.metadata.tool_sequence_assertions,
            factual_qa_assertions=task.metadata.factual_qa_assertions,
            citation_backed_assertions=task.metadata.citation_backed_assertions,
            partial_milestone_assertions=task.metadata.partial_milestone_assertions,
            reliability_assertions=task.metadata.reliability_assertions,
            economics_performance_assertions=task.metadata.economics_performance_assertions,
            expected_final_state_assertions=task.metadata.expected_final_state_assertions,
            state_fixture=task.metadata.state_fixture,
            initial_state_fixture_hash=(
                task.metadata.state_fixture.fixture_sha256
                if task.metadata.state_fixture is not None
                else None
            ),
            graders=grader_refs,
        ))
    task_defs_by_id = {td.task_id: td for td in task_defs}
    with open(report_dir / "tasks.jsonl", "w") as f:
        for td in task_defs:
            f.write(td.model_dump_json() + "\n")

    attempt_records: list[AttemptRecord] = []
    metric_records: list[MetricObservation] = []
    token_store_persistence_records: list[TokenStorePersistenceObservation] = []
    token_ttl_expiry_records: list[TokenTTLExpiryObservation] = []
    token_persistence_failure_records: list[TokenPersistenceFailureObservation] = []
    exfiltration_attempt_records: list[ExfiltrationAttemptObservation] = []
    artifact_leakage_records: list[ArtifactLeakageObservation] = []
    rehydration_records: list[RehydrationObservation] = []
    privacy_receipt_records: list[ReceiptObservation] = []
    replay_attempt_records: list[ReplayAttemptObservation] = []
    signed_field_tampering_records: list[SignedFieldTamperingObservation] = []
    payload_tampering_records: list[PayloadTamperingObservation] = []
    nonce_expiration_records: list[NonceExpirationObservation] = []
    stale_state_root_records: list[StaleStateRootObservation] = []
    identity_mismatch_records: list[IdentityMismatchObservation] = []
    signer_defect_records: list[SignerDefectObservation] = []
    l3_proof_transplant_records: list[L3ProofTransplantObservation] = []
    revoked_credential_records: list[RevokedCredentialObservation] = []
    evidence_preservation_records: list[EvidencePreservationObservation] = []
    policy_attack_records: list[PolicyAttackObservation] = []
    tool_sequence_records: list[ToolSequenceObservation] = []
    factual_qa_records: list[FactualQAObservation] = []
    citation_backed_records: list[CitationBackedObservation] = []
    partial_milestone_records: list[PartialMilestoneObservation] = []
    reliability_records: list[ReliabilityObservation] = []
    economics_performance_records: list[EconomicsPerformanceObservation] = []
    final_state_records: list[FinalStateObservation] = []
    state_records: list[StateObservation] = []
    governance_receipt_records: list[ReceiptObservation] = []
    evidence_index_records: list[EvidenceIndex] = []
    synthetic_observation_groups: tuple[Sequence[_SyntheticObservation], ...] = (
        token_store_persistence_records,
        token_ttl_expiry_records,
        token_persistence_failure_records,
        exfiltration_attempt_records,
        artifact_leakage_records,
        rehydration_records,
        replay_attempt_records,
        signed_field_tampering_records,
        payload_tampering_records,
        nonce_expiration_records,
        stale_state_root_records,
        identity_mismatch_records,
        signer_defect_records,
        l3_proof_transplant_records,
        revoked_credential_records,
        evidence_preservation_records,
        policy_attack_records,
        tool_sequence_records,
        factual_qa_records,
        citation_backed_records,
        partial_milestone_records,
        reliability_records,
        economics_performance_records,
        final_state_records,
        state_records,
    )

    console.print(f"[bold blue]Running synthetic {suite} ({len(tasks)} tasks)...[/bold blue]")

    for task in tasks:
        task_def = task_defs_by_id[task.id]
        params = task.metadata.benchmark_specific
        attempt_id = f"{run_id}:{task.id}:synthetic:1"
        started_at = datetime.now(UTC)

        attempt = AttemptRecord(
            attempt_id=attempt_id,
            run_id=run_id,
            task_id=task.id,
            arm_id=Arm.DIRECT,
            state_snapshot_hash=dataset_hash,
            replicate_id="1",
            assignment_order=len(attempt_records),
            started_at=started_at,
            terminal_status=TerminalStatus.COMPLETED,
        )

        store_dir = report_dir / "stores" / task.id
        store_dir.mkdir(parents=True, exist_ok=True)
        store_path = store_dir / "store.json"
        store = LocalEncryptedTokenStore(store_path, per_run_key)

        task_evidence_indices: list[EvidenceIndex] = []
        task_receipt_ids: list[str] = []

        token_store_observations: list[TokenStorePersistenceObservation] = []
        ttl_observations: list[TokenTTLExpiryObservation] = []
        failure_observations: list[TokenPersistenceFailureObservation] = []

        if task.metadata.token_store_persistence_assertions:
            for token_spec in params.get("tokens", []):
                store.store(
                    token_spec["token_id"],
                    token_spec["value"],
                    token_spec["sensitive_type"],
                    token_spec["ttl_seconds"],
                )
            expired = params.get("expired_token")
            if expired:
                store.store(
                    expired["token_id"],
                    expired["value"],
                    expired["sensitive_type"],
                    expired["ttl_seconds"],
                )
            store.persist()
            store_ciphertext = store_path.read_bytes()
            store_sha = hashlib.sha256(store_ciphertext).hexdigest()
            store_artifact_id = f"{attempt_id}:token-store"
            store_index = _persist_evidence_artifact(
                store_ciphertext.decode(errors="replace"),
                run_id=run_id,
                attempt_id=attempt_id,
                artifact_id=store_artifact_id,
                schema_ref="g8e_evals.benchmarks.privacy.LocalEncryptedTokenStore",
                report_dir=report_dir,
            )
            task_evidence_indices.append(store_index)
            observer = TokenStorePersistenceObserverImpl(store, store_sha, store_artifact_id)
            token_store_observations = await observer.observe(task_def, attempt)
            token_store_persistence_records.extend(token_store_observations)
            attempt.token_store_persistence_observation_refs = [
                obs.observation_id for obs in token_store_observations
            ]

        if task.metadata.token_ttl_expiry_assertions:
            ttl_token = params.get("ttl_token")
            if ttl_token:
                store.store(
                    ttl_token["token_id"],
                    ttl_token["value"],
                    ttl_token["sensitive_type"],
                    ttl_token["ttl_seconds"],
                )
            store.persist()
            store_ciphertext = store_path.read_bytes()
            store_sha = hashlib.sha256(store_ciphertext).hexdigest()
            store_artifact_id = f"{attempt_id}:token-ttl-store"
            store_index = _persist_evidence_artifact(
                store_ciphertext.decode(errors="replace"),
                run_id=run_id,
                attempt_id=attempt_id,
                artifact_id=store_artifact_id,
                schema_ref="g8e_evals.benchmarks.privacy.LocalEncryptedTokenStore",
                report_dir=report_dir,
            )
            task_evidence_indices.append(store_index)
            observer = TokenTTLExpiryObserverImpl(
                store,
                store_sha,
                store_artifact_id,
                visible_before_expiry=params.get("visible_before_expiry", True),
                invisible_after_expiry=params.get("invisible_after_expiry", True),
            )
            ttl_observations = await observer.observe(task_def, attempt)
            token_ttl_expiry_records.extend(ttl_observations)
            attempt.token_ttl_expiry_observation_refs = [
                obs.observation_id for obs in ttl_observations
            ]

        if task.metadata.token_persistence_failure_assertions:
            token_specs = params.get("tokens", [])
            pre_existing_token_ids = [ts["token_id"] for ts in token_specs]
            uncommitted_token_id = "uncommitted-rollback-token"
            store = LocalEncryptedTokenStore(store_path, per_run_key)
            for token_spec in token_specs:
                store.store(
                    token_spec["token_id"],
                    token_spec["value"],
                    token_spec["sensitive_type"],
                    token_spec["ttl_seconds"],
                )
            store.persist()
            store.store(
                uncommitted_token_id,
                "uncommitted-rollback-value",
                "email",
                3600,
            )
            store.set_fail_persist(True)
            store_ciphertext = store_path.read_bytes() if store_path.exists() else b""
            store_sha = hashlib.sha256(store_ciphertext).hexdigest()
            store_artifact_id = f"{attempt_id}:token-persist-fail-store"
            store_index = _persist_evidence_artifact(
                store_ciphertext.decode(errors="replace"),
                run_id=run_id,
                attempt_id=attempt_id,
                artifact_id=store_artifact_id,
                schema_ref="g8e_evals.benchmarks.privacy.LocalEncryptedTokenStore",
                report_dir=report_dir,
            )
            task_evidence_indices.append(store_index)
            observer = TokenPersistenceFailureObserverImpl(
                store, store_sha, store_artifact_id,
                pre_existing_token_ids=pre_existing_token_ids,
                uncommitted_token_id=uncommitted_token_id,
            )
            failure_observations = await observer.observe(task_def, attempt)
            token_persistence_failure_records.extend(failure_observations)
            attempt.token_persistence_failure_observation_refs = [
                obs.observation_id for obs in failure_observations
            ]

        exfiltration_observations: list[ExfiltrationAttemptObservation] = []
        artifact_leakage_observations: list[ArtifactLeakageObservation] = []
        rehydration_observations: list[RehydrationObservation] = []
        privacy_receipt: ReceiptObservation | None = None

        if task.metadata.exfiltration_attempt_assertions:
            simulator = LocalExfiltrationSimulator()
            receipt_artifact_id = f"{attempt_id}:exfiltration-receipt"
            observer = ExfiltrationAttemptObserverImpl(
                simulator, "", receipt_artifact_id,
            )
            privacy_receipt, exfiltration_observations = await observer.observe(task_def, attempt)
            receipt_content = json.dumps(
                action_receipt_to_dict(privacy_receipt.action_receipt),
                sort_keys=True, separators=(",", ":"), ensure_ascii=False,
            )
            receipt_index = _persist_evidence_artifact(
                receipt_content,
                run_id=run_id,
                attempt_id=attempt_id,
                artifact_id=receipt_artifact_id,
                schema_ref="g8e.operator.v1.ActionReceipt",
                report_dir=report_dir,
            )
            task_evidence_indices.append(receipt_index)
            receipt_sha = receipt_index.sha256
            privacy_receipt = privacy_receipt.model_copy(update={"verified": privacy_receipt.verified})
            exfiltration_observations = [
                obs.model_copy(update={
                    "source_evidence_refs": [receipt_artifact_id],
                    "source_evidence_sha256": receipt_sha,
                    "verification_status": VerificationStatus.VERIFIED,
                })
                for obs in exfiltration_observations
            ]
            exfiltration_attempt_records.extend(exfiltration_observations)
            privacy_receipt_records.append(privacy_receipt)
            task_receipt_ids.append(privacy_receipt.receipt_id)
            attempt.exfiltration_attempt_observation_refs = [
                obs.observation_id for obs in exfiltration_observations
            ]

        if task.metadata.artifact_leakage_assertions:
            artifact_dir = report_dir / "artifacts" / task.id
            emitter = LocalArtifactEmitter(artifact_dir)
            leak_types = list(params.get("leak_types", []))
            observer = ArtifactLeakageObserverImpl(
                emitter, "", "", leak_types=leak_types,
            )
            artifact_leakage_observations = await observer.observe(task_def, attempt)
            updated_artifact_obs: list[ArtifactLeakageObservation] = []
            for obs in artifact_leakage_observations:
                obs_content = obs.model_dump_json(indent=2)
                obs_artifact_id = f"{obs.observation_id}:source"
                obs_index = _persist_evidence_artifact(
                    obs_content,
                    run_id=run_id,
                    attempt_id=attempt_id,
                    artifact_id=obs_artifact_id,
                    schema_ref="g8e_evals.ArtifactLeakageObservation",
                    report_dir=report_dir,
                )
                task_evidence_indices.append(obs_index)
                updated_artifact_obs.append(obs.model_copy(update={
                    "source_evidence_refs": [obs_artifact_id],
                    "source_evidence_sha256": obs_index.sha256,
                    "verification_status": VerificationStatus.VERIFIED,
                }))
            artifact_leakage_records.extend(updated_artifact_obs)
            artifact_leakage_observations = updated_artifact_obs
            attempt.artifact_leakage_observation_refs = [
                obs.observation_id for obs in updated_artifact_obs
            ]

        if task.metadata.rehydration_assertions:
            rehydration_artifact_path = store_dir / "rehydration.json"
            rehydration_public_path = store_dir / "rehydration-public.json"
            artifact = LocalRehydrationArtifact(
                rehydration_artifact_path,
                key=per_run_key,
                public_path=rehydration_public_path,
            )
            rehydration_token_specs = params.get("rehydration_tokens", [])
            rehydration_tokens: list[TokenEntry] = []
            for token_spec in rehydration_token_specs:
                rehydration_tokens.append(TokenEntry(
                    token_id=token_spec["token_id"],
                    value=token_spec["value"],
                    sensitive_type=token_spec["sensitive_type"],
                    created_at=datetime.fromisoformat(token_spec["created_at"]),
                    expires_at=datetime.fromisoformat(token_spec["expires_at"]),
                ))
            rehydration_artifact_id = f"{attempt_id}:rehydration-artifact"
            observer = RehydrationObserverImpl(
                artifact, rehydration_tokens, "", rehydration_artifact_id,
            )
            rehydration_observations = await observer.observe(task_def, attempt)
            rehydration_content = rehydration_artifact_path.read_text() if rehydration_artifact_path.exists() else ""
            rehydration_index = _persist_evidence_artifact(
                rehydration_content,
                run_id=run_id,
                attempt_id=attempt_id,
                artifact_id=rehydration_artifact_id,
                schema_ref="g8e_evals.benchmarks.privacy.LocalRehydrationArtifact",
                report_dir=report_dir,
            )
            task_evidence_indices.append(rehydration_index)
            rehydration_observations = [
                obs.model_copy(update={
                    "source_evidence_refs": [rehydration_artifact_id],
                    "source_evidence_sha256": rehydration_index.sha256,
                    "verification_status": VerificationStatus.VERIFIED,
                })
                for obs in rehydration_observations
            ]
            rehydration_records.extend(rehydration_observations)
            attempt.rehydration_observation_refs = [
                obs.observation_id for obs in rehydration_observations
            ]

        replay_observations: list[ReplayAttemptObservation] = []
        signed_field_observations: list[SignedFieldTamperingObservation] = []
        nonce_observations: list[NonceExpirationObservation] = []
        stale_state_root_observations: list[StaleStateRootObservation] = []
        signer_defect_observations: list[SignerDefectObservation] = []
        l3_proof_transplant_observations: list[L3ProofTransplantObservation] = []
        revoked_credential_observations: list[RevokedCredentialObservation] = []
        policy_attack_observations: list[PolicyAttackObservation] = []
        governance_receipt: ReceiptObservation | None = None

        if task.metadata.replay_attempt_assertions:
            simulator = LocalGovernanceSimulator()
            receipt_artifact_id = f"{attempt_id}:replay-receipt"
            observer = ReplayAttemptObserverImpl(simulator, "", receipt_artifact_id)
            governance_receipt, replay_observations = await observer.observe(task_def, attempt)
            receipt_index, receipt_sha = _persist_receipt_evidence(
                governance_receipt,
                run_id=run_id, attempt_id=attempt_id,
                artifact_id=receipt_artifact_id, report_dir=report_dir,
            )
            task_evidence_indices.append(receipt_index)
            replay_observations = [
                obs.model_copy(update={
                    "source_evidence_refs": [receipt_artifact_id],
                    "source_evidence_sha256": receipt_sha,
                    "verification_status": VerificationStatus.VERIFIED,
                })
                for obs in replay_observations
            ]
            replay_attempt_records.extend(replay_observations)
            governance_receipt_records.append(governance_receipt)
            task_receipt_ids.append(governance_receipt.receipt_id)
            attempt.replay_attempt_observation_refs = [
                obs.observation_id for obs in replay_observations
            ]

        if task.metadata.signed_field_tampering_assertions:
            simulator = LocalGovernanceSimulator()
            receipt_artifact_id = f"{attempt_id}:signed-field-receipt"
            observer = SignedFieldTamperingObserverImpl(simulator, "", receipt_artifact_id)
            governance_receipt, signed_field_observations = await observer.observe(task_def, attempt)
            receipt_index, receipt_sha = _persist_receipt_evidence(
                governance_receipt,
                run_id=run_id, attempt_id=attempt_id,
                artifact_id=receipt_artifact_id, report_dir=report_dir,
            )
            task_evidence_indices.append(receipt_index)
            signed_field_observations = [
                obs.model_copy(update={
                    "source_evidence_refs": [receipt_artifact_id],
                    "source_evidence_sha256": receipt_sha,
                    "verification_status": VerificationStatus.VERIFIED,
                })
                for obs in signed_field_observations
            ]
            signed_field_tampering_records.extend(signed_field_observations)
            governance_receipt_records.append(governance_receipt)
            task_receipt_ids.append(governance_receipt.receipt_id)
            attempt.signed_field_tampering_observation_refs = [
                obs.observation_id for obs in signed_field_observations
            ]

        if task.metadata.nonce_expiration_assertions:
            simulator = LocalGovernanceSimulator()
            receipt_artifact_id = f"{attempt_id}:nonce-receipt"
            observer = NonceExpirationObserverImpl(simulator, "", receipt_artifact_id)
            governance_receipt, nonce_observations = await observer.observe(task_def, attempt)
            receipt_index, receipt_sha = _persist_receipt_evidence(
                governance_receipt,
                run_id=run_id, attempt_id=attempt_id,
                artifact_id=receipt_artifact_id, report_dir=report_dir,
            )
            task_evidence_indices.append(receipt_index)
            nonce_observations = [
                obs.model_copy(update={
                    "source_evidence_refs": [receipt_artifact_id],
                    "source_evidence_sha256": receipt_sha,
                    "verification_status": VerificationStatus.VERIFIED,
                })
                for obs in nonce_observations
            ]
            nonce_expiration_records.extend(nonce_observations)
            governance_receipt_records.append(governance_receipt)
            task_receipt_ids.append(governance_receipt.receipt_id)
            attempt.nonce_expiration_observation_refs = [
                obs.observation_id for obs in nonce_observations
            ]

        if task.metadata.stale_state_root_assertions:
            simulator = LocalGovernanceSimulator()
            receipt_artifact_id = f"{attempt_id}:stale-root-receipt"
            observer = StaleStateRootObserverImpl(simulator, "", receipt_artifact_id)
            governance_receipt, stale_state_root_observations = await observer.observe(task_def, attempt)
            receipt_index, receipt_sha = _persist_receipt_evidence(
                governance_receipt,
                run_id=run_id, attempt_id=attempt_id,
                artifact_id=receipt_artifact_id, report_dir=report_dir,
            )
            task_evidence_indices.append(receipt_index)
            stale_state_root_observations = [
                obs.model_copy(update={
                    "source_evidence_refs": [receipt_artifact_id],
                    "source_evidence_sha256": receipt_sha,
                    "verification_status": VerificationStatus.VERIFIED,
                })
                for obs in stale_state_root_observations
            ]
            stale_state_root_records.extend(stale_state_root_observations)
            governance_receipt_records.append(governance_receipt)
            task_receipt_ids.append(governance_receipt.receipt_id)
            attempt.stale_state_root_observation_refs = [
                obs.observation_id for obs in stale_state_root_observations
            ]

        if task.metadata.signer_defect_assertions:
            simulator = LocalGovernanceSimulator()
            receipt_artifact_id = f"{attempt_id}:signer-defect-receipt"
            observer = SignerDefectObserverImpl(simulator, "", receipt_artifact_id)
            governance_receipt, signer_defect_observations = await observer.observe(task_def, attempt)
            receipt_index, receipt_sha = _persist_receipt_evidence(
                governance_receipt,
                run_id=run_id, attempt_id=attempt_id,
                artifact_id=receipt_artifact_id, report_dir=report_dir,
            )
            task_evidence_indices.append(receipt_index)
            signer_defect_observations = [
                obs.model_copy(update={
                    "source_evidence_refs": [receipt_artifact_id],
                    "source_evidence_sha256": receipt_sha,
                    "verification_status": VerificationStatus.VERIFIED,
                })
                for obs in signer_defect_observations
            ]
            signer_defect_records.extend(signer_defect_observations)
            governance_receipt_records.append(governance_receipt)
            task_receipt_ids.append(governance_receipt.receipt_id)
            attempt.signer_defect_observation_refs = [
                obs.observation_id for obs in signer_defect_observations
            ]

        if task.metadata.l3_proof_transplant_assertions:
            simulator = LocalGovernanceSimulator()
            receipt_artifact_id = f"{attempt_id}:l3-proof-receipt"
            observer = L3ProofTransplantObserverImpl(simulator, "", receipt_artifact_id)
            governance_receipt, l3_proof_transplant_observations = await observer.observe(task_def, attempt)
            receipt_index, receipt_sha = _persist_receipt_evidence(
                governance_receipt,
                run_id=run_id, attempt_id=attempt_id,
                artifact_id=receipt_artifact_id, report_dir=report_dir,
            )
            task_evidence_indices.append(receipt_index)
            l3_proof_transplant_observations = [
                obs.model_copy(update={
                    "source_evidence_refs": [receipt_artifact_id],
                    "source_evidence_sha256": receipt_sha,
                    "verification_status": VerificationStatus.VERIFIED,
                })
                for obs in l3_proof_transplant_observations
            ]
            l3_proof_transplant_records.extend(l3_proof_transplant_observations)
            governance_receipt_records.append(governance_receipt)
            task_receipt_ids.append(governance_receipt.receipt_id)
            attempt.l3_proof_transplant_observation_refs = [
                obs.observation_id for obs in l3_proof_transplant_observations
            ]

        if task.metadata.revoked_credential_assertions:
            simulator = LocalGovernanceSimulator()
            receipt_artifact_id = f"{attempt_id}:revoked-cred-receipt"
            observer = RevokedCredentialObserverImpl(simulator, "", receipt_artifact_id)
            governance_receipt, revoked_credential_observations = await observer.observe(task_def, attempt)
            receipt_index, receipt_sha = _persist_receipt_evidence(
                governance_receipt,
                run_id=run_id, attempt_id=attempt_id,
                artifact_id=receipt_artifact_id, report_dir=report_dir,
            )
            task_evidence_indices.append(receipt_index)
            revoked_credential_observations = [
                obs.model_copy(update={
                    "source_evidence_refs": [receipt_artifact_id],
                    "source_evidence_sha256": receipt_sha,
                    "verification_status": VerificationStatus.VERIFIED,
                })
                for obs in revoked_credential_observations
            ]
            revoked_credential_records.extend(revoked_credential_observations)
            governance_receipt_records.append(governance_receipt)
            task_receipt_ids.append(governance_receipt.receipt_id)
            attempt.revoked_credential_observation_refs = [
                obs.observation_id for obs in revoked_credential_observations
            ]

        if task.metadata.payload_tampering_assertions:
            simulator = LocalGovernanceSimulator()
            receipt_artifact_id = f"{attempt_id}:payload-tamper-receipt"
            observer = PayloadTamperingObserverImpl(simulator, "", receipt_artifact_id)
            governance_receipt, payload_tampering_observations = await observer.observe(task_def, attempt)
            receipt_index, receipt_sha = _persist_receipt_evidence(
                governance_receipt,
                run_id=run_id, attempt_id=attempt_id,
                artifact_id=receipt_artifact_id, report_dir=report_dir,
            )
            task_evidence_indices.append(receipt_index)
            payload_tampering_observations = [
                obs.model_copy(update={
                    "source_evidence_refs": [receipt_artifact_id],
                    "source_evidence_sha256": receipt_sha,
                    "verification_status": VerificationStatus.VERIFIED,
                })
                for obs in payload_tampering_observations
            ]
            payload_tampering_records.extend(payload_tampering_observations)
            governance_receipt_records.append(governance_receipt)
            task_receipt_ids.append(governance_receipt.receipt_id)
            attempt.payload_tampering_observation_refs = [
                obs.observation_id for obs in payload_tampering_observations
            ]

        if task.metadata.identity_mismatch_assertions:
            simulator = LocalGovernanceSimulator()
            receipt_artifact_id = f"{attempt_id}:identity-mismatch-receipt"
            observer = IdentityMismatchObserverImpl(simulator, "", receipt_artifact_id)
            governance_receipt, identity_mismatch_observations = await observer.observe(task_def, attempt)
            receipt_index, receipt_sha = _persist_receipt_evidence(
                governance_receipt,
                run_id=run_id, attempt_id=attempt_id,
                artifact_id=receipt_artifact_id, report_dir=report_dir,
            )
            task_evidence_indices.append(receipt_index)
            identity_mismatch_observations = [
                obs.model_copy(update={
                    "source_evidence_refs": [receipt_artifact_id],
                    "source_evidence_sha256": receipt_sha,
                    "verification_status": VerificationStatus.VERIFIED,
                })
                for obs in identity_mismatch_observations
            ]
            identity_mismatch_records.extend(identity_mismatch_observations)
            governance_receipt_records.append(governance_receipt)
            task_receipt_ids.append(governance_receipt.receipt_id)
            attempt.identity_mismatch_observation_refs = [
                obs.observation_id for obs in identity_mismatch_observations
            ]

        if task.metadata.evidence_preservation_assertions:
            simulator = LocalGovernanceSimulator()
            receipt_artifact_id = f"{attempt_id}:evidence-preservation-receipt"
            observer = EvidencePreservationObserverImpl(simulator, "", receipt_artifact_id)
            governance_receipt, evidence_preservation_observations = await observer.observe(task_def, attempt)
            receipt_index, receipt_sha = _persist_receipt_evidence(
                governance_receipt,
                run_id=run_id, attempt_id=attempt_id,
                artifact_id=receipt_artifact_id, report_dir=report_dir,
            )
            task_evidence_indices.append(receipt_index)
            evidence_preservation_observations = [
                obs.model_copy(update={
                    "source_evidence_refs": [receipt_artifact_id],
                    "source_evidence_sha256": receipt_sha,
                    "verification_status": VerificationStatus.VERIFIED,
                })
                for obs in evidence_preservation_observations
            ]
            evidence_preservation_records.extend(evidence_preservation_observations)
            governance_receipt_records.append(governance_receipt)
            task_receipt_ids.append(governance_receipt.receipt_id)
            attempt.evidence_preservation_observation_refs = [
                obs.observation_id for obs in evidence_preservation_observations
            ]

        if task.metadata.policy_attack_assertions:
            simulator = LocalGovernanceSimulator()
            receipt_artifact_id = f"{attempt_id}:policy-attack-receipt"
            observer = PolicyAttackObserverImpl(simulator, "", receipt_artifact_id)
            governance_receipt, policy_attack_observations = await observer.observe(task_def, attempt)
            receipt_index, receipt_sha = _persist_receipt_evidence(
                governance_receipt,
                run_id=run_id, attempt_id=attempt_id,
                artifact_id=receipt_artifact_id, report_dir=report_dir,
            )
            task_evidence_indices.append(receipt_index)
            policy_attack_observations = [
                obs.model_copy(update={
                    "source_evidence_refs": [receipt_artifact_id],
                    "source_evidence_sha256": receipt_sha,
                    "verification_status": VerificationStatus.VERIFIED,
                })
                for obs in policy_attack_observations
            ]
            policy_attack_records.extend(policy_attack_observations)
            governance_receipt_records.append(governance_receipt)
            task_receipt_ids.append(governance_receipt.receipt_id)
            attempt.policy_attack_observation_refs = [
                obs.observation_id for obs in policy_attack_observations
            ]

        tool_sequence_observations: list[ToolSequenceObservation] = []
        if task.metadata.tool_sequence_assertions:
            tool_sim = LocalToolUseSimulator()
            observed_seq = params.get("observed_sequence", [])
            tool_sim.invoke_sequence(observed_seq)
            tool_artifact_id = f"{attempt_id}:tool-sequence-source"
            tool_content = json.dumps(
                {"sequence": observed_seq, "hash": tool_sim.finish().sequence_hash},
                sort_keys=True, separators=(",", ":"), ensure_ascii=False,
            )
            tool_index = _persist_evidence_artifact(
                tool_content,
                run_id=run_id, attempt_id=attempt_id,
                artifact_id=tool_artifact_id,
                schema_ref="g8e_evals.benchmarks.utility.LocalToolUseSimulator",
                report_dir=report_dir,
            )
            task_evidence_indices.append(tool_index)
            observer = ToolSequenceObserverImpl(tool_sim, tool_index.sha256, tool_artifact_id)
            tool_sequence_observations = await observer.observe(task_def, attempt)
            tool_sequence_records.extend(tool_sequence_observations)
            attempt.tool_sequence_observation_refs = [
                obs.observation_id for obs in tool_sequence_observations
            ]

        factual_qa_observations: list[FactualQAObservation] = []
        if task.metadata.factual_qa_assertions:
            qa_sim = LocalFactualQASimulator()
            observed_answer = params.get("observed_answer", "")
            qa_sim.set_answer(observed_answer)
            qa_artifact_id = f"{attempt_id}:factual-qa-source"
            qa_content = json.dumps(
                {"answer": observed_answer, "hash": qa_sim.finish().answer_hash},
                sort_keys=True, separators=(",", ":"), ensure_ascii=False,
            )
            qa_index = _persist_evidence_artifact(
                qa_content,
                run_id=run_id, attempt_id=attempt_id,
                artifact_id=qa_artifact_id,
                schema_ref="g8e_evals.benchmarks.utility.LocalFactualQASimulator",
                report_dir=report_dir,
            )
            task_evidence_indices.append(qa_index)
            observer = FactualQAObserverImpl(qa_sim, qa_index.sha256, qa_artifact_id)
            factual_qa_observations = await observer.observe(task_def, attempt)
            factual_qa_records.extend(factual_qa_observations)
            attempt.factual_qa_observation_refs = [
                obs.observation_id for obs in factual_qa_observations
            ]

        citation_backed_observations: list[CitationBackedObservation] = []
        if task.metadata.citation_backed_assertions:
            citation_sim = LocalCitationBackedSimulator()
            observed_citation = params.get("observed_citation", "")
            citation_sim.set_citation(observed_citation)
            citation_artifact_id = f"{attempt_id}:citation-backed-source"
            citation_content = json.dumps(
                {"citation": observed_citation, "hash": citation_sim.finish().citation_hash},
                sort_keys=True, separators=(",", ":"), ensure_ascii=False,
            )
            citation_index = _persist_evidence_artifact(
                citation_content,
                run_id=run_id, attempt_id=attempt_id,
                artifact_id=citation_artifact_id,
                schema_ref="g8e_evals.benchmarks.utility.LocalCitationBackedSimulator",
                report_dir=report_dir,
            )
            task_evidence_indices.append(citation_index)
            observer = CitationBackedObserverImpl(citation_sim, citation_index.sha256, citation_artifact_id)
            citation_backed_observations = await observer.observe(task_def, attempt)
            citation_backed_records.extend(citation_backed_observations)
            attempt.citation_backed_observation_refs = [
                obs.observation_id for obs in citation_backed_observations
            ]

        partial_milestone_observations: list[PartialMilestoneObservation] = []
        if task.metadata.partial_milestone_assertions:
            milestone_sim = LocalPartialMilestoneSimulator()
            observed_milestones = params.get("observed_milestones", [])
            from g8e_evals.benchmarks.utility.partial_milestone_simulator import MilestoneRecord
            milestone_sim.set_milestones([
                MilestoneRecord(label=m["label"], order=m["order"])
                for m in observed_milestones
            ])
            milestone_artifact_id = f"{attempt_id}:partial-milestone-source"
            milestone_content = json.dumps(
                {"milestones": observed_milestones, "hash": milestone_sim.finish().milestone_hash},
                sort_keys=True, separators=(",", ":"), ensure_ascii=False,
            )
            milestone_index = _persist_evidence_artifact(
                milestone_content,
                run_id=run_id, attempt_id=attempt_id,
                artifact_id=milestone_artifact_id,
                schema_ref="g8e_evals.benchmarks.utility.LocalPartialMilestoneSimulator",
                report_dir=report_dir,
            )
            task_evidence_indices.append(milestone_index)
            observer = PartialMilestoneObserverImpl(milestone_sim, milestone_index.sha256, milestone_artifact_id)
            partial_milestone_observations = await observer.observe(task_def, attempt)
            partial_milestone_records.extend(partial_milestone_observations)
            attempt.partial_milestone_observation_refs = [
                obs.observation_id for obs in partial_milestone_observations
            ]

        reliability_observations: list[ReliabilityObservation] = []
        if task.metadata.reliability_assertions:
            reliability_sim = LocalReliabilitySimulator()
            reliability_params = params.get("reliability_params", {})
            reliability_artifact_id = f"{attempt_id}:reliability-source"
            reliability_content = json.dumps(
                {"reliability_params": reliability_params},
                sort_keys=True, separators=(",", ":"), ensure_ascii=False,
            )
            reliability_index = _persist_evidence_artifact(
                reliability_content,
                run_id=run_id, attempt_id=attempt_id,
                artifact_id=reliability_artifact_id,
                schema_ref="g8e_evals.benchmarks.reliability.LocalReliabilitySimulator",
                report_dir=report_dir,
            )
            task_evidence_indices.append(reliability_index)
            observer = ReliabilityObserverImpl(
                reliability_sim, reliability_params,
                reliability_index.sha256, reliability_artifact_id,
            )
            reliability_observations = await observer.observe(task_def, attempt)
            updated_reliability_obs: list[ReliabilityObservation] = []
            for rel_obs in reliability_observations:
                updated_reliability_obs.append(rel_obs.model_copy(update={
                    "source_evidence_refs": [reliability_artifact_id],
                    "source_evidence_sha256": reliability_index.sha256,
                    "verification_status": VerificationStatus.VERIFIED,
                }))
            reliability_records.extend(updated_reliability_obs)
            reliability_observations = updated_reliability_obs
            attempt.reliability_observation_refs = [
                obs.observation_id for obs in updated_reliability_obs
            ]

        economics_performance_observations: list[EconomicsPerformanceObservation] = []
        if task.metadata.economics_performance_assertions:
            econ_sim = LocalEconomicsPerformanceSimulator()
            econ_params = params.get("economics_params", {})
            econ_artifact_id = f"{attempt_id}:economics-performance-source"
            econ_content = json.dumps(
                {"economics_params": econ_params},
                sort_keys=True, separators=(",", ":"), ensure_ascii=False,
            )
            econ_index = _persist_evidence_artifact(
                econ_content,
                run_id=run_id, attempt_id=attempt_id,
                artifact_id=econ_artifact_id,
                schema_ref="g8e_evals.benchmarks.economics.LocalEconomicsPerformanceSimulator",
                report_dir=report_dir,
            )
            task_evidence_indices.append(econ_index)
            econ_observer = EconomicsPerformanceObserverImpl(
                econ_sim, econ_params,
                econ_index.sha256, econ_artifact_id,
            )
            economics_performance_observations = await econ_observer.observe(task_def, attempt)
            updated_econ_obs: list[EconomicsPerformanceObservation] = []
            for ep_obs in economics_performance_observations:
                updated_econ_obs.append(ep_obs.model_copy(update={
                    "source_evidence_refs": [econ_artifact_id],
                    "source_evidence_sha256": econ_index.sha256,
                    "verification_status": VerificationStatus.VERIFIED,
                }))
            economics_performance_records.extend(updated_econ_obs)
            economics_performance_observations = updated_econ_obs
            attempt.economics_performance_observation_refs = [
                obs.observation_id for obs in updated_econ_obs
            ]

        final_state_observations: list[FinalStateObservation] = []
        final_state_receipts: list[ReceiptObservation] = []
        if task.metadata.expected_final_state_assertions:
            final_state_sim = LocalFinalStateSimulator()
            final_state_params = params.get("final_state_params", {})
            final_state_receipt_artifact_id = f"{attempt_id}:final-state-receipt"
            observer = FinalStateObserverImpl(
                final_state_sim, final_state_params, "", final_state_receipt_artifact_id,
            )
            fs_receipt_obs, final_state_observations = await observer.observe(task_def, attempt)
            updated_final_state_obs: list[FinalStateObservation] = []
            for fs_receipt, fs_obs in zip(fs_receipt_obs, final_state_observations, strict=True):
                fs_receipt_artifact_id = f"{fs_receipt.receipt_id}:receipt"
                receipt_index, receipt_sha = _persist_receipt_evidence(
                    fs_receipt,
                    run_id=run_id, attempt_id=attempt_id,
                    artifact_id=fs_receipt_artifact_id, report_dir=report_dir,
                )
                task_evidence_indices.append(receipt_index)
                governance_receipt_records.append(fs_receipt)
                task_receipt_ids.append(fs_receipt.receipt_id)
                final_state_receipts.append(fs_receipt)
                updated_final_state_obs.append(fs_obs.model_copy(update={
                    "source_evidence_refs": [fs_receipt_artifact_id],
                    "source_evidence_sha256": receipt_sha,
                    "verification_status": VerificationStatus.VERIFIED,
                }))
            final_state_records.extend(updated_final_state_obs)
            final_state_observations = updated_final_state_obs
            attempt.final_state_observation_refs = [
                obs.observation_id for obs in updated_final_state_obs
            ]

        state_observations: list[StateObservation] = []
        if task.metadata.state_fixture:
            ledger_sim = LocalLedgerConsistencySimulator()
            ledger_payloads = params.get("ledger_payloads", [])
            ledger_sim.append_entries(ledger_payloads)
            if params.get("inject_inconsistency", False):
                ledger_sim.inject_inconsistency()
            if params.get("inject_sequence_gap", False):
                ledger_sim.inject_sequence_gap()
            ledger_artifact_id = f"{attempt_id}:ledger-consistency-source"
            ledger_result = ledger_sim.finish()
            ledger_content = json.dumps(
                {
                    "consistent": ledger_result.consistent,
                    "entry_count": ledger_result.entry_count,
                    "head_sha256": ledger_result.head_sha256,
                },
                sort_keys=True, separators=(",", ":"), ensure_ascii=False,
            )
            ledger_index = _persist_evidence_artifact(
                ledger_content,
                run_id=run_id, attempt_id=attempt_id,
                artifact_id=ledger_artifact_id,
                schema_ref="g8e_evals.benchmarks.utility.LocalLedgerConsistencySimulator",
                report_dir=report_dir,
            )
            task_evidence_indices.append(ledger_index)
            fixture_sha256 = task.metadata.state_fixture.fixture_sha256
            observer = LedgerConsistencyObserverImpl(
                ledger_sim, fixture_sha256, ledger_index.sha256, ledger_artifact_id,
            )
            state_observations = await observer.observe(task_def, attempt)
            state_records.extend(state_observations)
            attempt.state_observation_refs = [
                obs.observation_id for obs in state_observations
            ]

        attempt.ended_at = datetime.now(UTC)

        grade_metrics: list[MetricObservation] = []
        if task.metadata.token_store_persistence_assertions:
            grade = grade_deterministically(
                _TOKEN_STORE_PERSISTENCE_GRADER_ID,
                _GRADER_VERSION,
                DeterministicGradingContext(
                    task=task_def,
                    attempt=attempt,
                    receipts=[],
                    stages=[],
                    token_store_persistence_observations=token_store_observations,
                ),
            )
            grade_metrics.append(MetricObservation(
                metric_id=_TOKEN_STORE_PERSISTENCE_GRADER_ID,
                attempt_id=attempt_id,
                run_id=run_id,
                arm_id=Arm.DIRECT,
                task_id=task.id,
                value=grade.value,
                unit="proportion",
                eligible=grade.verification_status == VerificationStatus.VERIFIED,
                denominator_contribution=grade.denominator_contribution,
                verification_status=grade.verification_status,
                grader_class=GraderClass.DETERMINISTIC,
                evidence_refs=grade.evidence_refs,
            ))
        if task.metadata.token_ttl_expiry_assertions:
            grade = grade_deterministically(
                _TOKEN_TTL_EXPIRY_GRADER_ID,
                _GRADER_VERSION,
                DeterministicGradingContext(
                    task=task_def,
                    attempt=attempt,
                    receipts=[],
                    stages=[],
                    token_ttl_expiry_observations=ttl_observations,
                ),
            )
            grade_metrics.append(MetricObservation(
                metric_id=_TOKEN_TTL_EXPIRY_GRADER_ID,
                attempt_id=attempt_id,
                run_id=run_id,
                arm_id=Arm.DIRECT,
                task_id=task.id,
                value=grade.value,
                unit="proportion",
                eligible=grade.verification_status == VerificationStatus.VERIFIED,
                denominator_contribution=grade.denominator_contribution,
                verification_status=grade.verification_status,
                grader_class=GraderClass.DETERMINISTIC,
                evidence_refs=grade.evidence_refs,
            ))
        if task.metadata.token_persistence_failure_assertions:
            grade = grade_deterministically(
                _TOKEN_PERSISTENCE_FAILURE_GRADER_ID,
                _GRADER_VERSION,
                DeterministicGradingContext(
                    task=task_def,
                    attempt=attempt,
                    receipts=[],
                    stages=[],
                    token_persistence_failure_observations=failure_observations,
                ),
            )
            grade_metrics.append(MetricObservation(
                metric_id=_TOKEN_PERSISTENCE_FAILURE_GRADER_ID,
                attempt_id=attempt_id,
                run_id=run_id,
                arm_id=Arm.DIRECT,
                task_id=task.id,
                value=grade.value,
                unit="proportion",
                eligible=grade.verification_status == VerificationStatus.VERIFIED,
                denominator_contribution=grade.denominator_contribution,
                verification_status=grade.verification_status,
                grader_class=GraderClass.DETERMINISTIC,
                evidence_refs=grade.evidence_refs,
            ))

        if task.metadata.exfiltration_attempt_assertions and privacy_receipt is not None:
            grade = grade_deterministically(
                _EXFILTRATION_ATTEMPT_GRADER_ID,
                _GRADER_VERSION,
                DeterministicGradingContext(
                    task=task_def,
                    attempt=attempt,
                    receipts=[privacy_receipt],
                    stages=[],
                    exfiltration_attempt_observations=exfiltration_observations,
                ),
            )
            grade_metrics.append(MetricObservation(
                metric_id=_EXFILTRATION_ATTEMPT_GRADER_ID,
                attempt_id=attempt_id,
                run_id=run_id,
                arm_id=Arm.DIRECT,
                task_id=task.id,
                value=grade.value,
                unit="proportion",
                eligible=grade.verification_status == VerificationStatus.VERIFIED,
                denominator_contribution=grade.denominator_contribution,
                verification_status=grade.verification_status,
                grader_class=GraderClass.DETERMINISTIC,
                evidence_refs=grade.evidence_refs,
            ))
        if task.metadata.artifact_leakage_assertions:
            grade = grade_deterministically(
                _ARTIFACT_LEAKAGE_GRADER_ID,
                _GRADER_VERSION,
                DeterministicGradingContext(
                    task=task_def,
                    attempt=attempt,
                    receipts=[],
                    stages=[],
                    artifact_leakage_observations=artifact_leakage_observations,
                ),
            )
            grade_metrics.append(MetricObservation(
                metric_id=_ARTIFACT_LEAKAGE_GRADER_ID,
                attempt_id=attempt_id,
                run_id=run_id,
                arm_id=Arm.DIRECT,
                task_id=task.id,
                value=grade.value,
                unit="proportion",
                eligible=grade.verification_status == VerificationStatus.VERIFIED,
                denominator_contribution=grade.denominator_contribution,
                verification_status=grade.verification_status,
                grader_class=GraderClass.DETERMINISTIC,
                evidence_refs=grade.evidence_refs,
            ))
        if task.metadata.rehydration_assertions:
            grade = grade_deterministically(
                _EXACT_LOCAL_REHYDRATION_GRADER_ID,
                _GRADER_VERSION,
                DeterministicGradingContext(
                    task=task_def,
                    attempt=attempt,
                    receipts=[],
                    stages=[],
                    rehydration_observations=rehydration_observations,
                ),
            )
            grade_metrics.append(MetricObservation(
                metric_id=_EXACT_LOCAL_REHYDRATION_GRADER_ID,
                attempt_id=attempt_id,
                run_id=run_id,
                arm_id=Arm.DIRECT,
                task_id=task.id,
                value=grade.value,
                unit="proportion",
                eligible=grade.verification_status == VerificationStatus.VERIFIED,
                denominator_contribution=grade.denominator_contribution,
                verification_status=grade.verification_status,
                grader_class=GraderClass.DETERMINISTIC,
                evidence_refs=grade.evidence_refs,
            ))

        if task.metadata.replay_attempt_assertions and governance_receipt is not None:
            grade = grade_deterministically(
                _REPLAY_ATTEMPT_GRADER_ID,
                _GRADER_VERSION,
                DeterministicGradingContext(
                    task=task_def,
                    attempt=attempt,
                    receipts=[governance_receipt],
                    stages=[],
                    replay_attempt_observations=replay_observations,
                ),
            )
            grade_metrics.append(MetricObservation(
                metric_id=_REPLAY_ATTEMPT_GRADER_ID,
                attempt_id=attempt_id,
                run_id=run_id,
                arm_id=Arm.DIRECT,
                task_id=task.id,
                value=grade.value,
                unit="proportion",
                eligible=grade.verification_status == VerificationStatus.VERIFIED,
                denominator_contribution=grade.denominator_contribution,
                verification_status=grade.verification_status,
                grader_class=GraderClass.DETERMINISTIC,
                evidence_refs=grade.evidence_refs,
            ))
        if task.metadata.signed_field_tampering_assertions and governance_receipt is not None:
            grade = grade_deterministically(
                _SIGNED_FIELD_TAMPERING_GRADER_ID,
                _GRADER_VERSION,
                DeterministicGradingContext(
                    task=task_def,
                    attempt=attempt,
                    receipts=[governance_receipt],
                    stages=[],
                    signed_field_tampering_observations=signed_field_observations,
                ),
            )
            grade_metrics.append(MetricObservation(
                metric_id=_SIGNED_FIELD_TAMPERING_GRADER_ID,
                attempt_id=attempt_id,
                run_id=run_id,
                arm_id=Arm.DIRECT,
                task_id=task.id,
                value=grade.value,
                unit="proportion",
                eligible=grade.verification_status == VerificationStatus.VERIFIED,
                denominator_contribution=grade.denominator_contribution,
                verification_status=grade.verification_status,
                grader_class=GraderClass.DETERMINISTIC,
                evidence_refs=grade.evidence_refs,
            ))
        if task.metadata.payload_tampering_assertions and governance_receipt is not None:
            grade = grade_deterministically(
                _PAYLOAD_TAMPERING_GRADER_ID,
                _GRADER_VERSION,
                DeterministicGradingContext(
                    task=task_def,
                    attempt=attempt,
                    receipts=[governance_receipt],
                    stages=[],
                    payload_tampering_observations=payload_tampering_observations,
                ),
            )
            grade_metrics.append(MetricObservation(
                metric_id=_PAYLOAD_TAMPERING_GRADER_ID,
                attempt_id=attempt_id,
                run_id=run_id,
                arm_id=Arm.DIRECT,
                task_id=task.id,
                value=grade.value,
                unit="proportion",
                eligible=grade.verification_status == VerificationStatus.VERIFIED,
                denominator_contribution=grade.denominator_contribution,
                verification_status=grade.verification_status,
                grader_class=GraderClass.DETERMINISTIC,
                evidence_refs=grade.evidence_refs,
            ))
        if task.metadata.nonce_expiration_assertions and governance_receipt is not None:
            grade = grade_deterministically(
                _NONCE_EXPIRATION_GRADER_ID,
                _GRADER_VERSION,
                DeterministicGradingContext(
                    task=task_def,
                    attempt=attempt,
                    receipts=[governance_receipt],
                    stages=[],
                    nonce_expiration_observations=nonce_observations,
                ),
            )
            grade_metrics.append(MetricObservation(
                metric_id=_NONCE_EXPIRATION_GRADER_ID,
                attempt_id=attempt_id,
                run_id=run_id,
                arm_id=Arm.DIRECT,
                task_id=task.id,
                value=grade.value,
                unit="proportion",
                eligible=grade.verification_status == VerificationStatus.VERIFIED,
                denominator_contribution=grade.denominator_contribution,
                verification_status=grade.verification_status,
                grader_class=GraderClass.DETERMINISTIC,
                evidence_refs=grade.evidence_refs,
            ))
        if task.metadata.stale_state_root_assertions and governance_receipt is not None:
            grade = grade_deterministically(
                _STALE_STATE_ROOT_GRADER_ID,
                _GRADER_VERSION,
                DeterministicGradingContext(
                    task=task_def,
                    attempt=attempt,
                    receipts=[governance_receipt],
                    stages=[],
                    stale_state_root_observations=stale_state_root_observations,
                ),
            )
            grade_metrics.append(MetricObservation(
                metric_id=_STALE_STATE_ROOT_GRADER_ID,
                attempt_id=attempt_id,
                run_id=run_id,
                arm_id=Arm.DIRECT,
                task_id=task.id,
                value=grade.value,
                unit="proportion",
                eligible=grade.verification_status == VerificationStatus.VERIFIED,
                denominator_contribution=grade.denominator_contribution,
                verification_status=grade.verification_status,
                grader_class=GraderClass.DETERMINISTIC,
                evidence_refs=grade.evidence_refs,
            ))
        if task.metadata.identity_mismatch_assertions and governance_receipt is not None:
            grade = grade_deterministically(
                _IDENTITY_MISMATCH_GRADER_ID,
                _GRADER_VERSION,
                DeterministicGradingContext(
                    task=task_def,
                    attempt=attempt,
                    receipts=[governance_receipt],
                    stages=[],
                    identity_mismatch_observations=identity_mismatch_observations,
                ),
            )
            grade_metrics.append(MetricObservation(
                metric_id=_IDENTITY_MISMATCH_GRADER_ID,
                attempt_id=attempt_id,
                run_id=run_id,
                arm_id=Arm.DIRECT,
                task_id=task.id,
                value=grade.value,
                unit="proportion",
                eligible=grade.verification_status == VerificationStatus.VERIFIED,
                denominator_contribution=grade.denominator_contribution,
                verification_status=grade.verification_status,
                grader_class=GraderClass.DETERMINISTIC,
                evidence_refs=grade.evidence_refs,
            ))
        if task.metadata.signer_defect_assertions and governance_receipt is not None:
            grade = grade_deterministically(
                _SIGNER_DEFECT_GRADER_ID,
                _GRADER_VERSION,
                DeterministicGradingContext(
                    task=task_def,
                    attempt=attempt,
                    receipts=[governance_receipt],
                    stages=[],
                    signer_defect_observations=signer_defect_observations,
                ),
            )
            grade_metrics.append(MetricObservation(
                metric_id=_SIGNER_DEFECT_GRADER_ID,
                attempt_id=attempt_id,
                run_id=run_id,
                arm_id=Arm.DIRECT,
                task_id=task.id,
                value=grade.value,
                unit="proportion",
                eligible=grade.verification_status == VerificationStatus.VERIFIED,
                denominator_contribution=grade.denominator_contribution,
                verification_status=grade.verification_status,
                grader_class=GraderClass.DETERMINISTIC,
                evidence_refs=grade.evidence_refs,
            ))
        if task.metadata.l3_proof_transplant_assertions and governance_receipt is not None:
            grade = grade_deterministically(
                _L3_PROOF_TRANSPLANT_GRADER_ID,
                _GRADER_VERSION,
                DeterministicGradingContext(
                    task=task_def,
                    attempt=attempt,
                    receipts=[governance_receipt],
                    stages=[],
                    l3_proof_transplant_observations=l3_proof_transplant_observations,
                ),
            )
            grade_metrics.append(MetricObservation(
                metric_id=_L3_PROOF_TRANSPLANT_GRADER_ID,
                attempt_id=attempt_id,
                run_id=run_id,
                arm_id=Arm.DIRECT,
                task_id=task.id,
                value=grade.value,
                unit="proportion",
                eligible=grade.verification_status == VerificationStatus.VERIFIED,
                denominator_contribution=grade.denominator_contribution,
                verification_status=grade.verification_status,
                grader_class=GraderClass.DETERMINISTIC,
                evidence_refs=grade.evidence_refs,
            ))
        if task.metadata.revoked_credential_assertions and governance_receipt is not None:
            grade = grade_deterministically(
                _REVOKED_CREDENTIAL_GRADER_ID,
                _GRADER_VERSION,
                DeterministicGradingContext(
                    task=task_def,
                    attempt=attempt,
                    receipts=[governance_receipt],
                    stages=[],
                    revoked_credential_observations=revoked_credential_observations,
                ),
            )
            grade_metrics.append(MetricObservation(
                metric_id=_REVOKED_CREDENTIAL_GRADER_ID,
                attempt_id=attempt_id,
                run_id=run_id,
                arm_id=Arm.DIRECT,
                task_id=task.id,
                value=grade.value,
                unit="proportion",
                eligible=grade.verification_status == VerificationStatus.VERIFIED,
                denominator_contribution=grade.denominator_contribution,
                verification_status=grade.verification_status,
                grader_class=GraderClass.DETERMINISTIC,
                evidence_refs=grade.evidence_refs,
            ))
        if task.metadata.evidence_preservation_assertions and governance_receipt is not None:
            grade = grade_deterministically(
                _EVIDENCE_PRESERVATION_GRADER_ID,
                _GRADER_VERSION,
                DeterministicGradingContext(
                    task=task_def,
                    attempt=attempt,
                    receipts=[governance_receipt],
                    stages=[],
                    evidence_preservation_observations=evidence_preservation_observations,
                ),
            )
            grade_metrics.append(MetricObservation(
                metric_id=_EVIDENCE_PRESERVATION_GRADER_ID,
                attempt_id=attempt_id,
                run_id=run_id,
                arm_id=Arm.DIRECT,
                task_id=task.id,
                value=grade.value,
                unit="proportion",
                eligible=grade.verification_status == VerificationStatus.VERIFIED,
                denominator_contribution=grade.denominator_contribution,
                verification_status=grade.verification_status,
                grader_class=GraderClass.DETERMINISTIC,
                evidence_refs=grade.evidence_refs,
            ))
        if task.metadata.policy_attack_assertions and governance_receipt is not None:
            grade = grade_deterministically(
                _POLICY_ATTACK_GRADER_ID,
                _GRADER_VERSION,
                DeterministicGradingContext(
                    task=task_def,
                    attempt=attempt,
                    receipts=[governance_receipt],
                    stages=[],
                    policy_attack_observations=policy_attack_observations,
                ),
            )
            grade_metrics.append(MetricObservation(
                metric_id=_POLICY_ATTACK_GRADER_ID,
                attempt_id=attempt_id,
                run_id=run_id,
                arm_id=Arm.DIRECT,
                task_id=task.id,
                value=grade.value,
                unit="proportion",
                eligible=grade.verification_status == VerificationStatus.VERIFIED,
                denominator_contribution=grade.denominator_contribution,
                verification_status=grade.verification_status,
                grader_class=GraderClass.DETERMINISTIC,
                evidence_refs=grade.evidence_refs,
            ))
        if task.metadata.tool_sequence_assertions:
            grade = grade_deterministically(
                _TOOL_SEQUENCE_GRADER_ID,
                _GRADER_VERSION,
                DeterministicGradingContext(
                    task=task_def,
                    attempt=attempt,
                    receipts=[],
                    stages=[],
                    tool_sequence_observations=tool_sequence_observations,
                ),
            )
            grade_metrics.append(MetricObservation(
                metric_id=_TOOL_SEQUENCE_GRADER_ID,
                attempt_id=attempt_id,
                run_id=run_id,
                arm_id=Arm.DIRECT,
                task_id=task.id,
                value=grade.value,
                unit="proportion",
                eligible=grade.verification_status == VerificationStatus.VERIFIED,
                denominator_contribution=grade.denominator_contribution,
                verification_status=grade.verification_status,
                grader_class=GraderClass.DETERMINISTIC,
                evidence_refs=grade.evidence_refs,
            ))
        if task.metadata.factual_qa_assertions:
            grade = grade_deterministically(
                _FACTUAL_QA_GRADER_ID,
                _GRADER_VERSION,
                DeterministicGradingContext(
                    task=task_def,
                    attempt=attempt,
                    receipts=[],
                    stages=[],
                    factual_qa_observations=factual_qa_observations,
                ),
            )
            grade_metrics.append(MetricObservation(
                metric_id=_FACTUAL_QA_GRADER_ID,
                attempt_id=attempt_id,
                run_id=run_id,
                arm_id=Arm.DIRECT,
                task_id=task.id,
                value=grade.value,
                unit="proportion",
                eligible=grade.verification_status == VerificationStatus.VERIFIED,
                denominator_contribution=grade.denominator_contribution,
                verification_status=grade.verification_status,
                grader_class=GraderClass.DETERMINISTIC,
                evidence_refs=grade.evidence_refs,
            ))
        if task.metadata.citation_backed_assertions:
            grade = grade_deterministically(
                _CITATION_BACKED_GRADER_ID,
                _GRADER_VERSION,
                DeterministicGradingContext(
                    task=task_def,
                    attempt=attempt,
                    receipts=[],
                    stages=[],
                    citation_backed_observations=citation_backed_observations,
                ),
            )
            grade_metrics.append(MetricObservation(
                metric_id=_CITATION_BACKED_GRADER_ID,
                attempt_id=attempt_id,
                run_id=run_id,
                arm_id=Arm.DIRECT,
                task_id=task.id,
                value=grade.value,
                unit="proportion",
                eligible=grade.verification_status == VerificationStatus.VERIFIED,
                denominator_contribution=grade.denominator_contribution,
                verification_status=grade.verification_status,
                grader_class=GraderClass.DETERMINISTIC,
                evidence_refs=grade.evidence_refs,
            ))
        if task.metadata.partial_milestone_assertions:
            grade = grade_deterministically(
                _PARTIAL_MILESTONE_GRADER_ID,
                _GRADER_VERSION,
                DeterministicGradingContext(
                    task=task_def,
                    attempt=attempt,
                    receipts=[],
                    stages=[],
                    partial_milestone_observations=partial_milestone_observations,
                ),
            )
            grade_metrics.append(MetricObservation(
                metric_id=_PARTIAL_MILESTONE_GRADER_ID,
                attempt_id=attempt_id,
                run_id=run_id,
                arm_id=Arm.DIRECT,
                task_id=task.id,
                value=grade.value,
                unit="proportion",
                eligible=grade.verification_status == VerificationStatus.VERIFIED,
                denominator_contribution=grade.denominator_contribution,
                verification_status=grade.verification_status,
                grader_class=GraderClass.DETERMINISTIC,
                evidence_refs=grade.evidence_refs,
            ))
        if task.metadata.reliability_assertions:
            grade = grade_deterministically(
                _RELIABILITY_GRADER_ID,
                _GRADER_VERSION,
                DeterministicGradingContext(
                    task=task_def,
                    attempt=attempt,
                    receipts=[],
                    stages=[],
                    reliability_observations=reliability_observations,
                ),
            )
            grade_metrics.append(MetricObservation(
                metric_id=_RELIABILITY_GRADER_ID,
                attempt_id=attempt_id,
                run_id=run_id,
                arm_id=Arm.DIRECT,
                task_id=task.id,
                value=grade.value,
                unit="proportion",
                eligible=grade.verification_status == VerificationStatus.VERIFIED,
                denominator_contribution=grade.denominator_contribution,
                verification_status=grade.verification_status,
                grader_class=GraderClass.DETERMINISTIC,
                evidence_refs=grade.evidence_refs,
            ))
        if task.metadata.economics_performance_assertions:
            grade = grade_deterministically(
                _ECONOMICS_PERFORMANCE_GRADER_ID,
                _GRADER_VERSION,
                DeterministicGradingContext(
                    task=task_def,
                    attempt=attempt,
                    receipts=[],
                    stages=[],
                    economics_performance_observations=economics_performance_observations,
                ),
            )
            grade_metrics.append(MetricObservation(
                metric_id=_ECONOMICS_PERFORMANCE_GRADER_ID,
                attempt_id=attempt_id,
                run_id=run_id,
                arm_id=Arm.DIRECT,
                task_id=task.id,
                value=grade.value,
                unit="proportion",
                eligible=grade.verification_status == VerificationStatus.VERIFIED,
                denominator_contribution=grade.denominator_contribution,
                verification_status=grade.verification_status,
                grader_class=GraderClass.DETERMINISTIC,
                evidence_refs=grade.evidence_refs,
            ))
        if task.metadata.expected_final_state_assertions:
            grade = grade_deterministically(
                _FINAL_STATE_GRADER_ID,
                _GRADER_VERSION,
                DeterministicGradingContext(
                    task=task_def,
                    attempt=attempt,
                    receipts=final_state_receipts,
                    stages=[],
                    final_state_observations=final_state_observations,
                ),
            )
            grade_metrics.append(MetricObservation(
                metric_id=_FINAL_STATE_METRIC_ID,
                attempt_id=attempt_id,
                run_id=run_id,
                arm_id=Arm.DIRECT,
                task_id=task.id,
                value=grade.value,
                unit="proportion",
                eligible=grade.verification_status == VerificationStatus.VERIFIED,
                denominator_contribution=grade.denominator_contribution,
                verification_status=grade.verification_status,
                grader_class=GraderClass.DETERMINISTIC,
                evidence_refs=grade.evidence_refs,
            ))
        if task.metadata.state_fixture:
            grade = grade_deterministically(
                _INDEPENDENT_STATE_GRADER_ID,
                _GRADER_VERSION,
                DeterministicGradingContext(
                    task=task_def,
                    attempt=attempt,
                    receipts=[],
                    stages=[],
                    state_observations=state_observations,
                ),
            )
            grade_metrics.append(MetricObservation(
                metric_id=_INDEPENDENT_STATE_METRIC_ID,
                attempt_id=attempt_id,
                run_id=run_id,
                arm_id=Arm.DIRECT,
                task_id=task.id,
                value=grade.value,
                unit="proportion",
                eligible=grade.verification_status == VerificationStatus.VERIFIED,
                denominator_contribution=grade.denominator_contribution,
                verification_status=grade.verification_status,
                grader_class=GraderClass.DETERMINISTIC,
                evidence_refs=grade.evidence_refs,
            ))

        for metric in grade_metrics:
            DEFAULT_METRIC_REGISTRY.validate(metric)

        if not grade_metrics:
            raise EvaluationRunError(
                f"task {task.id} produced no graded metrics: empty metric set is invalid evidence"
            )

        attempt.receipt_refs = task_receipt_ids
        metric_records.extend(grade_metrics)
        attempt.grade_refs = [metric.metric_id for metric in grade_metrics]
        for observations in synthetic_observation_groups:
            for observation in observations:
                if observation.attempt_id == attempt_id:
                    task_evidence_indices.append(_persist_synthetic_observation(
                        observation,
                        run_id=run_id,
                        report_dir=report_dir,
                    ))
        evidence_index_records.extend(task_evidence_indices)
        attempt_records.append(attempt)

        status = "PASS" if all(m.value == 1.0 for m in grade_metrics) else "FAIL"
        color = "green" if status == "PASS" else "red"
        console.print(f"  [cyan]{task.id}[/cyan] [{color}]{status}[/{color}]")

    with open(report_dir / "attempts.jsonl", "w") as f:
        for ar in attempt_records:
            f.write(ar.model_dump_json() + "\n")
    with open(report_dir / "token-store-persistence-observations.jsonl", "w") as f:
        for obs in token_store_persistence_records:
            f.write(obs.model_dump_json() + "\n")
    with open(report_dir / "token-ttl-expiry-observations.jsonl", "w") as f:
        for obs in token_ttl_expiry_records:
            f.write(obs.model_dump_json() + "\n")
    with open(report_dir / "token-persistence-failure-observations.jsonl", "w") as f:
        for obs in token_persistence_failure_records:
            f.write(obs.model_dump_json() + "\n")
    with open(report_dir / "exfiltration-attempt-observations.jsonl", "w") as f:
        for obs in exfiltration_attempt_records:
            f.write(obs.model_dump_json() + "\n")
    with open(report_dir / "artifact-leakage-observations.jsonl", "w") as f:
        for obs in artifact_leakage_records:
            f.write(obs.model_dump_json() + "\n")
    with open(report_dir / "rehydration-observations.jsonl", "w") as f:
        for obs in rehydration_records:
            f.write(obs.model_dump_json() + "\n")
    with open(report_dir / "replay-attempt-observations.jsonl", "w") as f:
        for obs in replay_attempt_records:
            f.write(obs.model_dump_json() + "\n")
    with open(report_dir / "signed-field-tampering-observations.jsonl", "w") as f:
        for obs in signed_field_tampering_records:
            f.write(obs.model_dump_json() + "\n")
    with open(report_dir / "payload-tampering-observations.jsonl", "w") as f:
        for obs in payload_tampering_records:
            f.write(obs.model_dump_json() + "\n")
    with open(report_dir / "nonce-expiration-observations.jsonl", "w") as f:
        for obs in nonce_expiration_records:
            f.write(obs.model_dump_json() + "\n")
    with open(report_dir / "stale-state-root-observations.jsonl", "w") as f:
        for obs in stale_state_root_records:
            f.write(obs.model_dump_json() + "\n")
    with open(report_dir / "identity-mismatch-observations.jsonl", "w") as f:
        for obs in identity_mismatch_records:
            f.write(obs.model_dump_json() + "\n")
    with open(report_dir / "signer-defect-observations.jsonl", "w") as f:
        for obs in signer_defect_records:
            f.write(obs.model_dump_json() + "\n")
    with open(report_dir / "l3-proof-transplant-observations.jsonl", "w") as f:
        for obs in l3_proof_transplant_records:
            f.write(obs.model_dump_json() + "\n")
    with open(report_dir / "revoked-credential-observations.jsonl", "w") as f:
        for obs in revoked_credential_records:
            f.write(obs.model_dump_json() + "\n")
    with open(report_dir / "evidence-preservation-observations.jsonl", "w") as f:
        for obs in evidence_preservation_records:
            f.write(obs.model_dump_json() + "\n")
    with open(report_dir / "policy-attack-observations.jsonl", "w") as f:
        for obs in policy_attack_records:
            f.write(obs.model_dump_json() + "\n")
    with open(report_dir / "tool-sequence-observations.jsonl", "w") as f:
        for obs in tool_sequence_records:
            f.write(obs.model_dump_json() + "\n")
    with open(report_dir / "factual-qa-observations.jsonl", "w") as f:
        for obs in factual_qa_records:
            f.write(obs.model_dump_json() + "\n")
    with open(report_dir / "citation-backed-observations.jsonl", "w") as f:
        for obs in citation_backed_records:
            f.write(obs.model_dump_json() + "\n")
    with open(report_dir / "partial-milestone-observations.jsonl", "w") as f:
        for obs in partial_milestone_records:
            f.write(obs.model_dump_json() + "\n")
    with open(report_dir / "reliability-observations.jsonl", "w") as f:
        for obs in reliability_records:
            f.write(obs.model_dump_json() + "\n")
    with open(report_dir / "economics-performance-observations.jsonl", "w") as f:
        for obs in economics_performance_records:
            f.write(obs.model_dump_json() + "\n")
    with open(report_dir / "final-state-observations.jsonl", "w") as f:
        for obs in final_state_records:
            f.write(obs.model_dump_json() + "\n")
    with open(report_dir / "state-observations.jsonl", "w") as f:
        for obs in state_records:
            f.write(obs.model_dump_json() + "\n")
    with open(report_dir / "receipts.jsonl", "w") as f:
        for obs in [*privacy_receipt_records, *governance_receipt_records]:
            f.write(obs.model_dump_json() + "\n")
    (report_dir / "stages.jsonl").write_text("")
    with open(report_dir / "metrics.jsonl", "w") as f:
        for metric in metric_records:
            f.write(metric.model_dump_json() + "\n")
    with open(report_dir / "evidence-index.jsonl", "w") as f:
        for index in evidence_index_records:
            f.write(index.model_dump_json() + "\n")

    _scan_report_for_canary_leaks(report_dir, canary_values, per_run_key)

    console.print(f"\n[bold green]Synthetic report saved to {report_dir}[/bold green]")


if __name__ == "__main__":
    main()
