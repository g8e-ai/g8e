# Copyright (c) 2026 Lateralus Labs, LLC.
# Use of this source code is governed by the Business Source License
# included in the LICENSE file.
#
# As of the Change Date listed in the LICENSE file, this software is
# released under the Apache License, Version 2.0.

"""Immutable eval bundle package: versioned manifest, path validation, and deterministic canonicalization."""

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

__all__ = [
    "BUNDLE_MANIFEST_SCHEMA_VERSION",
    "EVAL_SIGNING_ALGORITHM",
    "EVAL_TRUST_SCOPE",
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
    "PrivacyClass",
    "TrustStatus",
    "canonical_checksum_root_bytes",
    "canonical_manifest_bytes",
    "compute_checksum_root_hash",
    "compute_manifest_hash",
    "sign_bundle",
    "validate_bundle_manifest",
    "validate_bundle_path",
    "validate_no_missing_files",
    "validate_no_orphans",
    "verify_bundle_signature",
]
