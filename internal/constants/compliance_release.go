// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package constants

import "time"

// Release evidence preparation defaults. `g8e compliance release-prepare`
// derives the protected assessment scope and the first-party trust policies
// for a Gateway operational-evidence release from these values.
const (
	ComplianceReleaseGitExecutable           = "git"
	ComplianceReleaseVersionFilename         = "VERSION"
	ComplianceReleaseWorkDirname             = "release-evidence"
	ComplianceReleaseScopeFilename           = "scope.json"
	ComplianceReleaseReportTrustFilename     = "report-trust-policy.json"
	ComplianceReleaseEvidenceTrustFilename   = "evidence-trust-policy.json"
	ComplianceReleaseExportDirname           = "operational-export"
	ComplianceReleaseSnapshotDirname         = "gateway-snapshot"
	ComplianceReleaseSigningMetadataFilename = "compliance-report-signing-metadata.json"
	ComplianceReleaseSigningPrivateFilename  = "compliance-report-signing-private-key.hex"
	ComplianceReleaseScopeIDPrefix           = "gateway-operational-"
	ComplianceReleaseScopeDateLayout         = "20060102"
	ComplianceReleaseDeploymentIDPrefix      = "g8e-unified-compose-"
	ComplianceReleaseReportIDSuffix          = "-gateway-operational-public"
	ComplianceReleaseDefaultOrganization     = "Lateralus Labs, LLC."
	ComplianceReleaseDefaultAssessor         = "local-engineering-acceptance-self-assessment"
	ComplianceReleaseDefaultGatewayContainer = "g8e-gateway"
	ComplianceReleaseGatewayComponentID      = "g8e-gateway"
	ComplianceReleaseGatewayComponentType    = "gateway"
	ComplianceReleaseReportPolicyIDSuffix    = "-engineering-report-trust-gateway"
	ComplianceReleaseEvidencePolicyIDSuffix  = "-gateway-evidence-trust"
	ComplianceReleaseAssessmentIDSuffix      = "-engineering-acceptance-gateway"
	ComplianceReleaseSigningKeyIDSuffix      = "-engineering-report-key"
	ComplianceReleaseTrustPolicyVersion      = "1.0.0"
	ComplianceReleaseSourceAdmissionID       = "gateway-operational-source-1"
	ComplianceReleaseSourceKind              = "operator-audit"
	ComplianceReleaseSourceVersion           = "1.0.0"
	ComplianceReleaseVerifierID              = "operational-export"
	ComplianceReleaseVerifierVersion         = "1.0.0"
	ComplianceReleaseOwnerRuntimeBoundary    = "g8e-gateway container /root/.g8e"
	ComplianceReleaseAcquisitionBoundary     = "gateway-local read-only SQLite export"
	ComplianceReleaseCryptographicMode       = "standard"
	ComplianceReleaseActionClassGovernedMut  = "governed_mutation"
	ComplianceReleaseArmGoverned             = "governed"
	ComplianceReleaseComponentGateway        = "gateway"
	ComplianceReleaseComponentOperator       = "operator"
	ComplianceReleaseDefaultWindowSpan       = 30 * time.Second
	ComplianceReleaseNewKeyLifetime          = 30 * 24 * time.Hour
	ComplianceReleaseEvidenceTrustValidity   = 30 * 24 * time.Hour
)

// Reasons recorded for assessment context that release preparation does not
// capture as a content-addressed artifact. Omission would be invalid, and an
// unavailable declaration limits, rather than inflates, the release claim.
const (
	ComplianceReleaseUnavailableNetworkTopology = "The running Compose network topology was not captured as a content-addressed assessment artifact."
	ComplianceReleaseUnavailableConfiguration   = "The running Compose configuration was not captured as a content-addressed assessment artifact."
	ComplianceReleaseUnavailableDoctrineBundles = "The active doctrine bundle was not captured as a content-addressed assessment artifact."
	ComplianceReleaseUnavailableTrustAnchors    = "The active trust-anchor inventory was not captured as a content-addressed assessment artifact."
)
