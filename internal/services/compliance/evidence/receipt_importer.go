package evidence

import (
	"context"
	"crypto/ed25519"
	"errors"
	"fmt"
	"time"

	"google.golang.org/protobuf/proto"

	"github.com/g8e-ai/g8e/v2/internal/constants"
	compliancev1 "github.com/g8e-ai/g8e/v2/protocol/proto/g8e/compliance/v1"
	operatorv1 "github.com/g8e-ai/g8e/v2/protocol/proto/g8e/operator/v1"
)

type AssessedSignerSource interface {
	GetTrustedSignerPublicKey(context.Context, string) (ed25519.PublicKey, error)
}

type ReceiptImportBinding struct {
	Reference  string
	Path       string
	ScopeID    string
	RunID      string
	AttemptID  string
	ScenarioID string
}

type ReceiptImporter struct {
	reader  ArtifactReader
	trust   AssessedSignerSource
	binding ReceiptImportBinding
	nowFunc func() time.Time
}

func NewReceiptImporter(reader ArtifactReader, trust AssessedSignerSource, binding ReceiptImportBinding) *ReceiptImporter {
	return &ReceiptImporter{reader: reader, trust: trust, binding: binding, nowFunc: time.Now}
}

func (i *ReceiptImporter) SourceID() string {
	return "receipt"
}

func (i *ReceiptImporter) Import(ctx context.Context) ([]EvidenceNode, error) {
	if i == nil || i.reader == nil || i.trust == nil || !validReceiptBinding(i.binding, constants.ActionReceiptReferencePrefix) {
		return nil, fmt.Errorf("%w: reader, assessed trust, content reference, path, scope, and run are required", constants.ErrInvalidEvidenceGraph)
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	receipt, result, err := readCanonicalReceipt(ctx, i.reader, i.binding)
	if err != nil {
		return nil, err
	}
	status, verifierID, verifierVersion, verifiedAt, err := verifyAssessedReceiptSignature(ctx, i.trust, receipt, i.nowFunc)
	if err != nil {
		return nil, err
	}
	references := []string{}
	if receipt.GetFinalPersistenceAttestation() != nil {
		body, err := compliancev1.MarshalCanonical(receipt.GetFinalPersistenceAttestation())
		if err != nil {
			return nil, fmt.Errorf("%w: marshal persistence attestation: %w", constants.ErrEvidenceArtifactMalformed, err)
		}
		references = append(references, ContentReferenceForBody(constants.ReceiptPersistenceReferencePrefix, body))
	}
	_, digest, _ := ParseExpectedContentReference(i.binding.Reference, constants.ActionReceiptReferencePrefix)
	return []EvidenceNode{{ArtifactID: i.binding.Reference, ArtifactType: ArtifactTypeActionReceipt, SHA256: digest, MediaType: constants.MediaTypeJSON, SchemaRef: "g8e.operator.v1.ActionReceipt", ProducerIdentity: receipt.GetSignerKeyId(), ProducedAt: time.UnixMilli(receipt.GetExecutedAtUnixMs()), ScopeID: i.binding.ScopeID, RunID: i.binding.RunID, AttemptID: i.binding.AttemptID, ScenarioID: i.binding.ScenarioID, TransactionID: receipt.GetTransactionId(), VerificationStatus: status, VerifierID: verifierID, VerifierVersion: verifierVersion, VerifiedAt: verifiedAt, BundlePath: i.binding.Path, CanonicalBytes: result.Bytes, References: references}}, nil
}

type PersistenceImportBinding struct {
	Reference        string
	Path             string
	ReceiptReference string
	ReceiptPath      string
	ScopeID          string
	RunID            string
	AttemptID        string
	ScenarioID       string
}

type PersistenceImporter struct {
	reader  ArtifactReader
	trust   AssessedSignerSource
	binding PersistenceImportBinding
	nowFunc func() time.Time
}

func NewPersistenceImporter(reader ArtifactReader, trust AssessedSignerSource, binding PersistenceImportBinding) *PersistenceImporter {
	return &PersistenceImporter{reader: reader, trust: trust, binding: binding, nowFunc: time.Now}
}

func (i *PersistenceImporter) SourceID() string {
	return "receipt-persistence"
}

func (i *PersistenceImporter) Import(ctx context.Context) ([]EvidenceNode, error) {
	if i == nil || i.reader == nil || i.trust == nil || !validPersistenceBinding(i.binding) {
		return nil, fmt.Errorf("%w: reader, assessed trust, content references, paths, scope, and run are required", constants.ErrInvalidEvidenceGraph)
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	receiptBinding := ReceiptImportBinding{Reference: i.binding.ReceiptReference, Path: i.binding.ReceiptPath, ScopeID: i.binding.ScopeID, RunID: i.binding.RunID, AttemptID: i.binding.AttemptID, ScenarioID: i.binding.ScenarioID}
	receipt, _, err := readCanonicalReceipt(ctx, i.reader, receiptBinding)
	if err != nil {
		return nil, err
	}
	attestation, result, err := readCanonicalPersistence(ctx, i.reader, i.binding)
	if err != nil {
		return nil, err
	}
	if receipt.GetFinalPersistenceAttestation() == nil || !proto.Equal(receipt.GetFinalPersistenceAttestation(), attestation) || receipt.GetTransactionId() != attestation.GetTransactionId() || receipt.GetSignerKeyId() != attestation.GetSignerKeyId() {
		return nil, fmt.Errorf("%w: persistence attestation does not match receipt", constants.ErrEvidenceScopeMismatch)
	}
	status, verifierID, verifierVersion, verifiedAt, err := verifyAssessedPersistence(ctx, i.trust, receipt, i.nowFunc)
	if err != nil {
		return nil, err
	}
	_, digest, _ := ParseExpectedContentReference(i.binding.Reference, constants.ReceiptPersistenceReferencePrefix)
	return []EvidenceNode{{ArtifactID: i.binding.Reference, ArtifactType: ArtifactTypeReceiptPersistence, SHA256: digest, MediaType: constants.MediaTypeJSON, SchemaRef: "g8e.operator.v1.ReceiptPersistenceAttestation", ProducerIdentity: attestation.GetSignerKeyId(), ProducedAt: time.UnixMilli(attestation.GetPersistedAtUnixMs()), ScopeID: i.binding.ScopeID, RunID: i.binding.RunID, AttemptID: i.binding.AttemptID, ScenarioID: i.binding.ScenarioID, TransactionID: attestation.GetTransactionId(), VerificationStatus: status, VerifierID: verifierID, VerifierVersion: verifierVersion, VerifiedAt: verifiedAt, BundlePath: i.binding.Path, CanonicalBytes: result.Bytes, References: []string{}}}, nil
}

func readCanonicalReceipt(ctx context.Context, reader ArtifactReader, binding ReceiptImportBinding) (*operatorv1.ActionReceipt, ReadResult, error) {
	_, digest, _ := ParseExpectedContentReference(binding.Reference, constants.ActionReceiptReferencePrefix)
	result, err := ReadAndDigest(reader, ctx, binding.Path, constants.DemoRunMaxArtifactBytes)
	if err != nil {
		return nil, ReadResult{}, fmt.Errorf("%w: %s: %w", constants.ErrEvidenceImporterFailed, binding.Path, err)
	}
	if !VerifyDigest(result.Bytes, digest) {
		return nil, ReadResult{}, fmt.Errorf("%w: %s", constants.ErrChecksumMismatch, binding.Reference)
	}
	receipt := &operatorv1.ActionReceipt{}
	if err := compliancev1.UnmarshalCanonical(result.Bytes, receipt); err != nil {
		return nil, ReadResult{}, fmt.Errorf("%w: %s: %w", constants.ErrEvidenceArtifactMalformed, binding.Path, err)
	}
	if receipt.GetTransactionId() == "" || receipt.GetSignerKeyId() == "" || receipt.GetExecutedAtUnixMs() <= 0 {
		return nil, ReadResult{}, fmt.Errorf("%w: receipt binding is incomplete", constants.ErrEvidenceArtifactMalformed)
	}
	return receipt, result, nil
}

func readCanonicalPersistence(ctx context.Context, reader ArtifactReader, binding PersistenceImportBinding) (*operatorv1.ReceiptPersistenceAttestation, ReadResult, error) {
	_, digest, _ := ParseExpectedContentReference(binding.Reference, constants.ReceiptPersistenceReferencePrefix)
	result, err := ReadAndDigest(reader, ctx, binding.Path, constants.DemoRunMaxArtifactBytes)
	if err != nil {
		return nil, ReadResult{}, fmt.Errorf("%w: %s: %w", constants.ErrEvidenceImporterFailed, binding.Path, err)
	}
	if !VerifyDigest(result.Bytes, digest) {
		return nil, ReadResult{}, fmt.Errorf("%w: %s", constants.ErrChecksumMismatch, binding.Reference)
	}
	attestation := &operatorv1.ReceiptPersistenceAttestation{}
	if err := compliancev1.UnmarshalCanonical(result.Bytes, attestation); err != nil {
		return nil, ReadResult{}, fmt.Errorf("%w: %s: %w", constants.ErrEvidenceArtifactMalformed, binding.Path, err)
	}
	if attestation.GetTransactionId() == "" || attestation.GetSignerKeyId() == "" || attestation.GetPersistedAtUnixMs() <= 0 {
		return nil, ReadResult{}, fmt.Errorf("%w: persistence binding is incomplete", constants.ErrEvidenceArtifactMalformed)
	}
	return attestation, result, nil
}

func verifyAssessedReceiptSignature(ctx context.Context, trust AssessedSignerSource, receipt *operatorv1.ActionReceipt, nowFunc func() time.Time) (VerificationStatus, string, string, time.Time, error) {
	publicKey, err := trust.GetTrustedSignerPublicKey(ctx, receipt.GetSignerKeyId())
	if errors.Is(err, constants.ErrTrustedSignerKeyNotFound) {
		return VerificationStatusUnverified, "", "", time.Time{}, nil
	}
	if err != nil {
		return "", "", "", time.Time{}, fmt.Errorf("%w: resolve signer %s: %w", constants.ErrEvidenceTrustNotAssessed, receipt.GetSignerKeyId(), err)
	}
	status := VerificationStatusVerified
	if err := VerifyReceiptSignature(receipt, publicKey); err != nil {
		status = VerificationStatusFailed
	}
	return status, constants.ReceiptEvidenceVerifierID, constants.ReceiptEvidenceVerifierVersion, nowFunc(), nil
}

func verifyAssessedPersistence(ctx context.Context, trust AssessedSignerSource, receipt *operatorv1.ActionReceipt, nowFunc func() time.Time) (VerificationStatus, string, string, time.Time, error) {
	publicKey, err := trust.GetTrustedSignerPublicKey(ctx, receipt.GetSignerKeyId())
	if errors.Is(err, constants.ErrTrustedSignerKeyNotFound) {
		return VerificationStatusUnverified, "", "", time.Time{}, nil
	}
	if err != nil {
		return "", "", "", time.Time{}, fmt.Errorf("%w: resolve signer %s: %w", constants.ErrEvidenceTrustNotAssessed, receipt.GetSignerKeyId(), err)
	}
	status := VerificationStatusVerified
	if VerifyReceiptSignature(receipt, publicKey) != nil || VerifyReceiptPersistence(receipt, publicKey) != nil {
		status = VerificationStatusFailed
	}
	return status, constants.ReceiptEvidenceVerifierID, constants.ReceiptEvidenceVerifierVersion, nowFunc(), nil
}

func validReceiptBinding(binding ReceiptImportBinding, prefix string) bool {
	_, _, referenceValid := ParseExpectedContentReference(binding.Reference, prefix)
	return referenceValid && ValidRelativePath(binding.Path) && binding.ScopeID != "" && binding.RunID != ""
}

func validPersistenceBinding(binding PersistenceImportBinding) bool {
	return validReceiptBinding(ReceiptImportBinding{Reference: binding.Reference, Path: binding.Path, ScopeID: binding.ScopeID, RunID: binding.RunID}, constants.ReceiptPersistenceReferencePrefix) && validReceiptBinding(ReceiptImportBinding{Reference: binding.ReceiptReference, Path: binding.ReceiptPath, ScopeID: binding.ScopeID, RunID: binding.RunID}, constants.ActionReceiptReferencePrefix)
}
