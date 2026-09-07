package evidence

import (
	"context"
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"time"

	"github.com/g8e-ai/g8e/v2/internal/constants"
	"github.com/g8e-ai/g8e/v2/internal/services/governance"
	compliancev1 "github.com/g8e-ai/g8e/v2/protocol/proto/g8e/compliance/v1"
	operatorv1 "github.com/g8e-ai/g8e/v2/protocol/proto/g8e/operator/v1"
)

type CommitmentImportBinding struct {
	Reference     string
	Path          string
	ScopeID       string
	RunID         string
	AttemptID     string
	ScenarioID    string
	TransactionID string
}

type CommitmentImporter struct {
	reader  ArtifactReader
	trust   AssessedSignerSource
	binding CommitmentImportBinding
	nowFunc func() time.Time
}

func NewCommitmentImporter(reader ArtifactReader, trust AssessedSignerSource, binding CommitmentImportBinding) *CommitmentImporter {
	return &CommitmentImporter{reader: reader, trust: trust, binding: binding, nowFunc: time.Now}
}

func (i *CommitmentImporter) SourceID() string {
	return "commitment"
}

func (i *CommitmentImporter) Import(ctx context.Context) ([]EvidenceNode, error) {
	if i == nil || i.reader == nil || i.trust == nil || !validCommitmentBinding(i.binding) {
		return nil, fmt.Errorf("%w: reader, assessed trust, content reference, path, scope, run, and transaction are required", constants.ErrInvalidEvidenceGraph)
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	_, digest, _ := ParseExpectedContentReference(i.binding.Reference, constants.CommitmentReferencePrefix)
	result, err := ReadAndDigest(i.reader, ctx, i.binding.Path, constants.DemoRunMaxArtifactBytes)
	if err != nil {
		return nil, fmt.Errorf("%w: %s: %w", constants.ErrEvidenceImporterFailed, i.binding.Path, err)
	}
	if !VerifyDigest(result.Bytes, digest) {
		return nil, fmt.Errorf("%w: %s", constants.ErrChecksumMismatch, i.binding.Reference)
	}
	attestation := &operatorv1.CommitmentAttestation{}
	if err := compliancev1.UnmarshalCanonical(result.Bytes, attestation); err != nil {
		return nil, fmt.Errorf("%w: %s: %w", constants.ErrEvidenceArtifactMalformed, i.binding.Path, err)
	}
	if !validCommitmentAttestation(attestation) {
		return nil, fmt.Errorf("%w: commitment attestation binding is incomplete", constants.ErrEvidenceArtifactMalformed)
	}
	if attestation.GetTransactionId() != i.binding.TransactionID {
		return nil, fmt.Errorf("%w: commitment transaction does not match import binding", constants.ErrEvidenceScopeMismatch)
	}
	status, verifierID, verifierVersion, verifiedAt, err := verifyAssessedCommitment(ctx, i.trust, attestation, i.nowFunc)
	if err != nil {
		return nil, err
	}
	return []EvidenceNode{{ArtifactID: i.binding.Reference, ArtifactType: ArtifactTypeCommitment, SHA256: digest, MediaType: constants.MediaTypeJSON, SchemaRef: "g8e.operator.v1.CommitmentAttestation", ProducerIdentity: attestation.GetAuditorKeyId(), ProducedAt: time.UnixMilli(attestation.GetCommittedAtUnixMs()), ScopeID: i.binding.ScopeID, RunID: i.binding.RunID, AttemptID: i.binding.AttemptID, ScenarioID: i.binding.ScenarioID, TransactionID: attestation.GetTransactionId(), VerificationStatus: status, VerifierID: verifierID, VerifierVersion: verifierVersion, VerifiedAt: verifiedAt, BundlePath: i.binding.Path, CanonicalBytes: result.Bytes, References: []string{}}}, nil
}

func validCommitmentBinding(binding CommitmentImportBinding) bool {
	_, _, referenceValid := ParseExpectedContentReference(binding.Reference, constants.CommitmentReferencePrefix)
	return referenceValid && ValidRelativePath(binding.Path) && binding.ScopeID != "" && binding.RunID != "" && binding.TransactionID != ""
}

func validCommitmentAttestation(attestation *operatorv1.CommitmentAttestation) bool {
	return attestation.GetTransactionId() != "" && attestation.GetTransactionHash() != "" && attestation.GetStateRootAtCommit() != "" && attestation.GetWardenIntentSignatureDigest() != "" && attestation.GetActionType() != "" && attestation.GetTargetResource() != "" && attestation.GetCommittedAtUnixMs() > 0 && attestation.GetAuditorKeyId() != "" && attestation.GetSignature() != "" && attestation.GetHash() != ""
}

func verifyAssessedCommitment(ctx context.Context, trust AssessedSignerSource, attestation *operatorv1.CommitmentAttestation, nowFunc func() time.Time) (VerificationStatus, string, string, time.Time, error) {
	payload, err := governance.CanonicalizeCommitmentAttestation(attestation)
	if err != nil {
		return "", "", "", time.Time{}, fmt.Errorf("%w: canonicalize commitment: %w", constants.ErrEvidenceArtifactMalformed, err)
	}
	payloadDigest := sha256.Sum256(payload)
	cryptographicallyValid := hex.EncodeToString(payloadDigest[:]) == attestation.GetHash()
	publicKey, err := trust.GetTrustedSignerPublicKey(ctx, attestation.GetAuditorKeyId())
	if errors.Is(err, constants.ErrTrustedSignerKeyNotFound) {
		if cryptographicallyValid {
			return VerificationStatusUnverified, "", "", time.Time{}, nil
		}
		return VerificationStatusFailed, constants.CommitmentEvidenceVerifierID, constants.CommitmentEvidenceVerifierVersion, nowFunc(), nil
	}
	if err != nil {
		return "", "", "", time.Time{}, fmt.Errorf("%w: resolve auditor %s: %w", constants.ErrEvidenceTrustNotAssessed, attestation.GetAuditorKeyId(), err)
	}
	signature, err := hex.DecodeString(attestation.GetSignature())
	if err != nil || len(signature) != ed25519.SignatureSize || !ed25519.Verify(publicKey, payload, signature) {
		cryptographicallyValid = false
	}
	status := VerificationStatusVerified
	if !cryptographicallyValid {
		status = VerificationStatusFailed
	}
	return status, constants.CommitmentEvidenceVerifierID, constants.CommitmentEvidenceVerifierVersion, nowFunc(), nil
}
