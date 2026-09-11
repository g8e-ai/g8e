# Copyright (c) 2026 Lateralus Labs, LLC.
# Use of this source code is governed by the Business Source License
# included in the LICENSE file.
#
# As of the Change Date listed in the LICENSE file, this software is
# released under the Apache License, Version 2.0.

"""Centralized artifact filename constants for the evals package.

Every report artifact filename is defined here as a constant. No
production code or test should construct artifact filenames from inline
string literals. Filenames are bare strings (no directory component);
callers join them with the report directory path.
"""

# Run manifest and task definitions
MANIFEST_JSON = "manifest.json"
TASKS_JSONL = "tasks.jsonl"
ATTEMPTS_JSONL = "attempts.jsonl"

# Canonical analysis artifacts (the authoritative release-facing output)
ANALYSIS_INPUT_JSON = "analysis-input.json"
ANALYSIS_JSON = "analysis.json"
ANALYSIS_MD = "analysis.md"
ANALYSIS_HTML = "analysis.html"
ANALYSIS_TXT = "analysis.txt"

# Immutable source records consumed by canonical analysis
RECEIPTS_JSONL = "receipts.jsonl"
STAGES_JSONL = "stages.jsonl"
METRICS_JSONL = "metrics.jsonl"
EVIDENCE_INDEX_JSONL = "evidence-index.jsonl"

# Observation JSONL files (bound record sequences)
FINAL_STATE_OBSERVATIONS_JSONL = "final-state-observations.jsonl"
STATE_OBSERVATIONS_JSONL = "state-observations.jsonl"
REHYDRATION_OBSERVATIONS_JSONL = "rehydration-observations.jsonl"
SECRET_DETECTION_OBSERVATIONS_JSONL = "secret-detection-observations.jsonl"
UNAUTHORIZED_MUTATION_OBSERVATIONS_JSONL = "unauthorized-mutation-observations.jsonl"
TOKEN_STORE_PERSISTENCE_OBSERVATIONS_JSONL = "token-store-persistence-observations.jsonl"
TOKEN_TTL_EXPIRY_OBSERVATIONS_JSONL = "token-ttl-expiry-observations.jsonl"
TOKEN_PERSISTENCE_FAILURE_OBSERVATIONS_JSONL = "token-persistence-failure-observations.jsonl"
EXFILTRATION_ATTEMPT_OBSERVATIONS_JSONL = "exfiltration-attempt-observations.jsonl"
ARTIFACT_LEAKAGE_OBSERVATIONS_JSONL = "artifact-leakage-observations.jsonl"
REPLAY_ATTEMPT_OBSERVATIONS_JSONL = "replay-attempt-observations.jsonl"
SIGNED_FIELD_TAMPERING_OBSERVATIONS_JSONL = "signed-field-tampering-observations.jsonl"
PAYLOAD_TAMPERING_OBSERVATIONS_JSONL = "payload-tampering-observations.jsonl"
STALE_STATE_ROOT_OBSERVATIONS_JSONL = "stale-state-root-observations.jsonl"
IDENTITY_MISMATCH_OBSERVATIONS_JSONL = "identity-mismatch-observations.jsonl"
NONCE_EXPIRATION_OBSERVATIONS_JSONL = "nonce-expiration-observations.jsonl"
SIGNER_DEFECT_OBSERVATIONS_JSONL = "signer-defect-observations.jsonl"
L3_PROOF_TRANSPLANT_OBSERVATIONS_JSONL = "l3-proof-transplant-observations.jsonl"
REVOKED_CREDENTIAL_OBSERVATIONS_JSONL = "revoked-credential-observations.jsonl"
EVIDENCE_PRESERVATION_OBSERVATIONS_JSONL = "evidence-preservation-observations.jsonl"
POLICY_ATTACK_OBSERVATIONS_JSONL = "policy-attack-observations.jsonl"
TOOL_SEQUENCE_OBSERVATIONS_JSONL = "tool-sequence-observations.jsonl"
FACTUAL_QA_OBSERVATIONS_JSONL = "factual-qa-observations.jsonl"
CITATION_BACKED_OBSERVATIONS_JSONL = "citation-backed-observations.jsonl"
PARTIAL_MILESTONE_OBSERVATIONS_JSONL = "partial-milestone-observations.jsonl"
RELIABILITY_OBSERVATIONS_JSONL = "reliability-observations.jsonl"
ECONOMICS_PERFORMANCE_OBSERVATIONS_JSONL = "economics-performance-observations.jsonl"

# Diagnostic-only artifacts (not release evidence)
DIAGNOSTIC_RESULTS_JSONL = "diagnostic-results.jsonl"

# Bundle metadata files (not listed as data artifacts in the manifest;
# the verifier reads these as the verification contract, not as data)
BUNDLE_MANIFEST_JSON = "bundle-manifest.json"
CHECKSUM_ROOT_JSON = "checksum-root.json"
BUNDLE_SIGNATURE_JSON = "bundle-signature.json"

# Campaign contract artifacts (the authoritative campaign records)
CAMPAIGN_MANIFEST_JSON = "campaign-manifest.json"
CAMPAIGN_ASSIGNMENTS_JSONL = "campaign-assignments.jsonl"
CAMPAIGN_COHORTS_JSONL = "campaign-cohorts.jsonl"
CAMPAIGN_SCHEDULE_JSON = "campaign-schedule.json"
CAMPAIGN_RETRY_POLICY_JSON = "campaign-retry-policy.json"
CAMPAIGN_STATUS_JSON = "campaign-status.json"
CAMPAIGN_INDEX_JSONL = "campaign-index.jsonl"
CAMPAIGN_PROGRESS_JSON = "campaign-progress.json"

# Campaign verification report (the typed output of the offline campaign verifier)
CAMPAIGN_VERIFICATION_REPORT_JSON = "campaign-verification-report.json"

# Resource observations (typed external resource observer output)
RESOURCE_OBSERVATIONS_JSONL = "resource-observations.jsonl"

# Tool call scorecards (per-tool-call 10-dimension scorecard records)
TOOL_CALL_SCORECARDS_JSONL = "tool-call-scorecards.jsonl"

# Escalation records (per-scenario routing-decision classification records)
ESCALATION_RECORDS_JSONL = "escalation-records.jsonl"

# Security event records (per-scenario security/privacy event records)
SECURITY_EVENTS_JSONL = "security-events.jsonl"

# Report checksum (optional per-report checksum file for standalone validation)
REPORT_CHECKSUM_JSON = "report-checksum.json"

# Source inclusion manifest (explicit reviewed list of source files with checksums)
SOURCE_INCLUSION_MANIFEST_JSON = "source-inclusion-manifest.json"

# Publication schema v4 artifacts (campaign-aware README evidence)
PUBLICATION_SCHEMA_V4 = "4.0.0"
MODEL_CAMPAIGN_JSON = "model-campaign.json"
CAMPAIGN_PROJECTIONS_JSONL = "campaign-projections.jsonl"
CAMPAIGN_STATISTICAL_ANALYSIS_JSON = "campaign-statistical-analysis.json"
CAMPAIGN_PROVENANCE_JSON = "campaign-provenance.json"
CAMPAIGN_VERIFICATION_REF_JSON = "campaign-verification-ref.json"
