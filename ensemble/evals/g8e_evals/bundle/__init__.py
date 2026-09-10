# Copyright (c) 2026 Lateralus Labs, LLC.
# Use of this source code is governed by the Business Source License
# included in the LICENSE file.
#
# As of the Change Date listed in the LICENSE file, this software is
# released under the Apache License, Version 2.0.

"""Immutable eval bundle package: manifest, signing, production, and verification."""

from __future__ import annotations

from g8e_evals.bundle.canonical import (
    canonical_checksum_root_bytes,
    canonical_manifest_bytes,
    compute_checksum_root_hash,
    compute_manifest_hash,
)
from g8e_evals.bundle.manifest import (
    BUNDLE_MANIFEST_SCHEMA_VERSION,
    ArtifactType,
    BundleArtifactEntry,
    BundleManifest,
    ChecksumEntry,
    ChecksumRoot,
    ExternalReference,
    PrivacyClass,
)
from g8e_evals.bundle.produce import produce_bundle
from g8e_evals.bundle.publish import PublicationError, build_publication_request, publish_bundle
from g8e_evals.bundle.signing import (
    EVAL_SIGNING_ALGORITHM,
    EVAL_TRUST_SCOPE,
    BundleSignature,
    BundleSignatureVerificationResult,
    EvalSignature,
    EvalSigningKey,
    EvalTrustStore,
    EvalTrustedKey,
    TrustStatus,
    sign_bundle,
    verify_bundle_signature,
)
from g8e_evals.bundle.validation import (
    BundleManifestValidationError,
    BundlePathError,
    validate_bundle_manifest,
    validate_bundle_path,
    validate_no_missing_files,
    validate_no_orphans,
)
from g8e_evals.bundle.verify import (
    VERIFICATION_REPORT_SCHEMA_VERSION,
    LayerResult,
    VerificationCancelled,
    VerificationFailure,
    VerificationFailureCode,
    VerificationLayer,
    VerificationReport,
    verify_bundle,
)

__all__ = [
    "BUNDLE_MANIFEST_SCHEMA_VERSION",
    "EVAL_SIGNING_ALGORITHM",
    "EVAL_TRUST_SCOPE",
    "VERIFICATION_REPORT_SCHEMA_VERSION",
    "ArtifactType",
    "BundleArtifactEntry",
    "BundleManifest",
    "BundleManifestValidationError",
    "BundlePathError",
    "BundleSignature",
    "BundleSignatureVerificationResult",
    "ChecksumEntry",
    "ChecksumRoot",
    "EvalSignature",
    "EvalSigningKey",
    "EvalTrustStore",
    "EvalTrustedKey",
    "ExternalReference",
    "LayerResult",
    "PrivacyClass",
    "PublicationError",
    "TrustStatus",
    "VerificationCancelled",
    "VerificationFailure",
    "VerificationFailureCode",
    "VerificationLayer",
    "VerificationReport",
    "build_publication_request",
    "canonical_checksum_root_bytes",
    "canonical_manifest_bytes",
    "compute_checksum_root_hash",
    "compute_manifest_hash",
    "produce_bundle",
    "publish_bundle",
    "sign_bundle",
    "validate_bundle_manifest",
    "validate_bundle_path",
    "validate_no_missing_files",
    "validate_no_orphans",
    "verify_bundle",
    "verify_bundle_signature",
]
