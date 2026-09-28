// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package report

import (
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"sort"
	"strings"
	"time"

	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/g8e-ai/g8e/v2/internal/constants"
	"github.com/g8e-ai/g8e/v2/internal/services/compliance/catalog"
	"github.com/g8e-ai/g8e/v2/internal/services/storage"
	compliancev1 "github.com/g8e-ai/g8e/v2/protocol/proto/g8e/compliance/v1"
	operatorv1 "github.com/g8e-ai/g8e/v2/protocol/proto/g8e/operator/v1"
)

// ReleaseScopeRequest carries the facts a Gateway operational-evidence release
// scope is derived from. Digests are lowercase SHA-256 hex.
type ReleaseScopeRequest struct {
	ScopeID            string
	OrganizationID     string
	DeploymentID       string
	ProductVersion     string
	BuildIdentity      string
	SourceRevision     string
	GatewayImageDigest string
	GatewayVersion     string
	WindowStart        time.Time
	WindowEnd          time.Time
}

// BuildReleaseAssessmentScope constructs and validates the protected
// assessment scope for a Gateway operational-evidence release. Context that is
// not captured as a content-addressed artifact is declared unavailable with a
// typed reason rather than omitted.
func BuildReleaseAssessmentScope(request ReleaseScopeRequest) (*compliancev1.AssessmentScope, error) {
	if request.ScopeID == "" || request.OrganizationID == "" || request.DeploymentID == "" || request.ProductVersion == "" || request.BuildIdentity == "" || request.SourceRevision == "" || request.GatewayImageDigest == "" || request.GatewayVersion == "" {
		return nil, fmt.Errorf("%w: release scope request is incomplete", constants.ErrValidationFailed)
	}
	if request.WindowStart.IsZero() || !request.WindowStart.Before(request.WindowEnd) {
		return nil, fmt.Errorf("%w: release assessment window is invalid", constants.ErrValidationFailed)
	}
	scope := &compliancev1.AssessmentScope{
		ScopeId:        request.ScopeID,
		OrganizationId: request.OrganizationID,
		DeploymentId:   request.DeploymentID,
		ProductVersion: strings.TrimPrefix(request.ProductVersion, "v"),
		BuildIdentity:  request.BuildIdentity,
		SourceRevision: request.SourceRevision,
		ComponentInventory: []*compliancev1.ComponentInventoryEntry{{
			ComponentId:   constants.ComplianceReleaseGatewayComponentID,
			ComponentType: constants.ComplianceReleaseGatewayComponentType,
			Version:       strings.TrimPrefix(request.GatewayVersion, "v"),
			Digest:        request.GatewayImageDigest,
		}},
		CryptographicMode:     constants.ComplianceReleaseCryptographicMode,
		AssessmentWindowStart: timestamppb.New(request.WindowStart.UTC()),
		AssessmentWindowEnd:   timestamppb.New(request.WindowEnd.UTC()),
		AssessmentAsOf:        timestamppb.New(request.WindowEnd.UTC()),
		ActivePosture:         constants.PostureDoctrine,
		SourceAdmissions: []*compliancev1.AssessmentSourceAdmission{{
			AdmissionId:              constants.ComplianceReleaseSourceAdmissionID,
			SourceKind:               constants.ComplianceReleaseSourceKind,
			SourceVersion:            constants.ComplianceReleaseSourceVersion,
			SourceScopeId:            request.ScopeID,
			OwnerRuntimeBoundary:     constants.ComplianceReleaseOwnerRuntimeBoundary,
			AcquisitionBoundary:      constants.ComplianceReleaseAcquisitionBoundary,
			RunId:                    request.ScopeID,
			VerifierRef:              &compliancev1.VersionedReference{Id: constants.ComplianceReleaseVerifierID, Version: constants.ComplianceReleaseVerifierVersion},
			DisclosureClassification: constants.ComplianceBundleProfilePublic,
		}},
		Applicability: &compliancev1.AssessmentApplicabilitySelection{
			Components:    []string{constants.ComplianceReleaseComponentGateway, constants.ComplianceReleaseComponentOperator},
			ActionClasses: []string{constants.ComplianceReleaseActionClassGovernedMut},
			Arms:          []string{constants.ComplianceReleaseArmGoverned},
		},
		SelectedPopulation: &compliancev1.AssessmentPopulationSelection{},
		UnavailableContext: []*compliancev1.UnavailableAssessmentContext{
			{Kind: compliancev1.AssessmentContextKind_ASSESSMENT_CONTEXT_KIND_NETWORK_TOPOLOGY, Reason: constants.ComplianceReleaseUnavailableNetworkTopology},
			{Kind: compliancev1.AssessmentContextKind_ASSESSMENT_CONTEXT_KIND_CONFIGURATION, Reason: constants.ComplianceReleaseUnavailableConfiguration},
			{Kind: compliancev1.AssessmentContextKind_ASSESSMENT_CONTEXT_KIND_DOCTRINE_BUNDLES, Reason: constants.ComplianceReleaseUnavailableDoctrineBundles},
			{Kind: compliancev1.AssessmentContextKind_ASSESSMENT_CONTEXT_KIND_TRUST_ANCHORS, Reason: constants.ComplianceReleaseUnavailableTrustAnchors},
		},
	}
	if err := catalog.ValidateAssessmentScope(scope); err != nil {
		return nil, fmt.Errorf("release scope: %w", err)
	}
	return scope, nil
}

// CollectOperationalSignerKeys returns the sorted distinct signer key IDs that
// authenticate the receipts, deterministic-stage evidence, persistence
// attestations, and commitments in a snapshot. A retained row without a
// canonical body contributes no key; the exporter records that gap itself.
func CollectOperationalSignerKeys(snapshot *storage.OperationalEvidenceSnapshot) ([]string, error) {
	if snapshot == nil {
		return nil, fmt.Errorf("%w: operational snapshot is required", constants.ErrValidationFailed)
	}
	keys := make(map[string]struct{})
	add := func(keyID string) {
		if keyID != "" {
			keys[keyID] = struct{}{}
		}
	}
	for _, receipt := range snapshot.Receipts {
		if len(receipt.Body) == 0 {
			continue
		}
		message := &operatorv1.ActionReceipt{}
		if err := compliancev1.UnmarshalCanonical(receipt.Body, message); err != nil {
			return nil, fmt.Errorf("release trust: decode receipt %s: %w", receipt.TransactionID, err)
		}
		add(message.GetSignerKeyId())
		add(message.GetFinalPersistenceAttestation().GetSignerKeyId())
		for _, stage := range message.GetDeterministicStageEvidence() {
			add(stage.GetSignerKeyId())
		}
	}
	for _, commitment := range snapshot.Commitments {
		if len(commitment.Body) == 0 {
			continue
		}
		message := &operatorv1.CommitmentAttestation{}
		if err := compliancev1.UnmarshalCanonical(commitment.Body, message); err != nil {
			return nil, fmt.Errorf("release trust: decode commitment %s: %w", commitment.TransactionID, err)
		}
		add(message.GetAuditorKeyId())
	}
	sorted := make([]string, 0, len(keys))
	for keyID := range keys {
		sorted = append(sorted, keyID)
	}
	sort.Strings(sorted)
	return sorted, nil
}

// ReleaseTrustRequest identifies one first-party signer assessment.
type ReleaseTrustRequest struct {
	PolicyID         string
	AssessmentID     string
	AssessorIdentity string
	ScopeID          string
	AssessedAt       time.Time
}

func (r ReleaseTrustRequest) validate() error {
	if r.PolicyID == "" || r.AssessmentID == "" || r.AssessorIdentity == "" || r.ScopeID == "" || r.AssessedAt.IsZero() {
		return fmt.Errorf("%w: release trust request is incomplete", constants.ErrValidationFailed)
	}
	return nil
}

// BuildReleaseReportTrustPolicy binds the report-signing identity to the
// release scope. The policy is an assessment record and must be stored outside
// the report bundle.
func BuildReleaseReportTrustPolicy(request ReleaseTrustRequest, identity *ComplianceReportSigningIdentity) (*compliancev1.ComplianceReportTrustPolicy, error) {
	if err := request.validate(); err != nil {
		return nil, err
	}
	publicKey, err := identity.PublicKeyHex()
	if err != nil {
		return nil, err
	}
	policy := &compliancev1.ComplianceReportTrustPolicy{
		PolicyId:      request.PolicyID,
		PolicyVersion: constants.ComplianceReleaseTrustPolicyVersion,
		TrustedKeys: []*compliancev1.ComplianceReportTrustedKey{{
			Metadata:         identity.Metadata(),
			PublicKey:        publicKey,
			AssessmentId:     request.AssessmentID,
			AssessorIdentity: request.AssessorIdentity,
			AssessedAt:       timestamppb.New(request.AssessedAt.UTC()),
			AllowedScopeRefs: []string{request.ScopeID},
		}},
	}
	if err := validateComplianceReportTrustPolicy(policy); err != nil {
		return nil, err
	}
	return policy, nil
}

// BuildReleaseEvidenceTrustPolicy binds the source signer keys observed in an
// operational snapshot to the release scope. Key IDs are the lowercase hex
// Ed25519 public keys the Gateway signs with.
func BuildReleaseEvidenceTrustPolicy(request ReleaseTrustRequest, keyIDs []string) (*compliancev1.ComplianceEvidenceTrustPolicy, error) {
	if err := request.validate(); err != nil {
		return nil, err
	}
	if len(keyIDs) == 0 {
		return nil, fmt.Errorf("%w: no source signer keys were observed in the assessment window", constants.ErrEvidenceTrustNotAssessed)
	}
	validFrom := timestamppb.New(request.AssessedAt.UTC().Add(-constants.ComplianceReleaseEvidenceTrustValidity))
	validUntil := timestamppb.New(request.AssessedAt.UTC().Add(constants.ComplianceReleaseEvidenceTrustValidity))
	policy := &compliancev1.ComplianceEvidenceTrustPolicy{
		PolicyId:      request.PolicyID,
		PolicyVersion: constants.ComplianceReleaseTrustPolicyVersion,
		TrustedKeys:   make([]*compliancev1.ComplianceEvidenceTrustedKey, 0, len(keyIDs)),
	}
	for _, keyID := range keyIDs {
		publicKey, err := hex.DecodeString(keyID)
		if err != nil || len(publicKey) != ed25519.PublicKeySize || keyID != strings.ToLower(keyID) {
			return nil, fmt.Errorf("%w: source signer key %q is not a lowercase hex Ed25519 public key", constants.ErrEvidenceTrustNotAssessed, keyID)
		}
		digest := sha256.Sum256(publicKey)
		policy.TrustedKeys = append(policy.TrustedKeys, &compliancev1.ComplianceEvidenceTrustedKey{
			KeyId:            keyID,
			PublicKey:        keyID,
			PublicKeySha256:  hex.EncodeToString(digest[:]),
			AssessmentId:     request.AssessmentID,
			AssessorIdentity: request.AssessorIdentity,
			AssessedAt:       timestamppb.New(request.AssessedAt.UTC()),
			ValidFrom:        validFrom,
			ValidUntil:       validUntil,
			AllowedScopeRefs: []string{request.ScopeID},
		})
	}
	return policy, nil
}

// GenerateReportSigningKey creates a dedicated Ed25519 report-signing key and
// its canonical metadata. Key generation is not a trust assessment; the caller
// records that separately in a trust policy.
func GenerateReportSigningKey(random io.Reader, keyID string, createdAt time.Time, lifetime time.Duration) (*compliancev1.ComplianceReportSigningKeyMetadata, string, error) {
	if keyID == "" || createdAt.IsZero() || lifetime <= 0 {
		return nil, "", fmt.Errorf("%w: signing key request is incomplete", constants.ErrValidationFailed)
	}
	publicKey, privateKey, err := ed25519.GenerateKey(random)
	if err != nil {
		return nil, "", fmt.Errorf("%w: generate Ed25519 signing key: %w", constants.ErrReportSignatureFailed, err)
	}
	digest := sha256.Sum256(publicKey)
	metadata := &compliancev1.ComplianceReportSigningKeyMetadata{
		KeyId:           keyID,
		Algorithm:       constants.ComplianceReportSignatureAlgorithm,
		Purpose:         constants.ComplianceReportSigningPurpose,
		PublicKeySha256: hex.EncodeToString(digest[:]),
		CreatedAt:       timestamppb.New(createdAt.UTC()),
		ExpiresAt:       timestamppb.New(createdAt.UTC().Add(lifetime)),
	}
	return metadata, hex.EncodeToString(privateKey), nil
}
