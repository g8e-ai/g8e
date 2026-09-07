package evidence

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/g8e-ai/g8e/v2/internal/constants"
	"github.com/g8e-ai/g8e/v2/internal/timesvc"
)

const (
	attesterTypeCustomer = "customer"
	attesterTypeAssessor = "assessor"
)

type AttestationImportBinding struct {
	Reference string
	Path      string
	ScopeID   string
	RunID     string
}

type AttestationImporter struct {
	reader  ArtifactReader
	trust   AssessedSignerSource
	binding AttestationImportBinding
	nowFunc func() time.Time
}

type attestationRecord struct {
	SchemaVersion    string   `json:"schema_version"`
	AttestationID    string   `json:"attestation_id"`
	AttesterType     string   `json:"attester_type"`
	AttesterIdentity string   `json:"attester_identity"`
	SignerKeyID      string   `json:"signer_key_id"`
	IssuedAtUTC      string   `json:"issued_at_utc"`
	ValidFromUTC     string   `json:"valid_from_utc"`
	ValidUntilUTC    string   `json:"valid_until_utc"`
	ScopeID          string   `json:"scope_id"`
	RunID            string   `json:"run_id"`
	AssertionIDs     []string `json:"assertion_ids"`
	EvidenceRefs     []string `json:"evidence_refs,omitempty"`
	Statement        string   `json:"statement"`
	Revoked          bool     `json:"revoked"`
	RevokedAtUTC     string   `json:"revoked_at_utc,omitempty"`
	Signature        string   `json:"signature,omitempty"`
}

type importedAttestation struct {
	record       attestationRecord
	body         []byte
	issuedAt     time.Time
	validFrom    time.Time
	validUntil   time.Time
	artifactID   string
	artifactType ArtifactType
	schemaRef    string
}

func NewAttestationImporter(reader ArtifactReader, trust AssessedSignerSource, binding AttestationImportBinding) *AttestationImporter {
	return &AttestationImporter{reader: reader, trust: trust, binding: binding, nowFunc: time.Now}
}

func (i *AttestationImporter) SourceID() string {
	return "attestation"
}

func (i *AttestationImporter) Import(ctx context.Context) ([]EvidenceNode, error) {
	if i == nil || i.reader == nil || i.trust == nil || !validAttestationBinding(i.binding) {
		return nil, fmt.Errorf("%w: reader, assessed trust, content reference, path, scope, and run are required", constants.ErrInvalidEvidenceGraph)
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	_, digest, _ := ParseExpectedContentReference(i.binding.Reference, constants.AttestationCollectionReferencePrefix)
	result, err := ReadAndDigest(i.reader, ctx, i.binding.Path, constants.DemoRunMaxArtifactBytes)
	if err != nil {
		return nil, fmt.Errorf("%w: %s: %w", constants.ErrEvidenceImporterFailed, i.binding.Path, err)
	}
	if !VerifyDigest(result.Bytes, digest) {
		return nil, fmt.Errorf("%w: %s", constants.ErrChecksumMismatch, i.binding.Reference)
	}
	records, err := decodeAttestations(result.Bytes, i.binding)
	if err != nil {
		return nil, err
	}
	return i.buildNodes(ctx, records)
}

func (i *AttestationImporter) buildNodes(ctx context.Context, records []importedAttestation) ([]EvidenceNode, error) {
	nodes := make([]EvidenceNode, 0, len(records))
	for _, record := range records {
		status, verifierID, verifierVersion, verifiedAt, err := i.verify(ctx, record)
		if err != nil {
			return nil, err
		}
		nodes = append(nodes, EvidenceNode{
			ArtifactID:         record.artifactID,
			ArtifactType:       record.artifactType,
			SHA256:             digestHex(record.body),
			MediaType:          constants.MediaTypeJSON,
			SchemaRef:          record.schemaRef,
			ProducerIdentity:   record.record.AttesterIdentity,
			ProducedAt:         record.issuedAt,
			ScopeID:            record.record.ScopeID,
			RunID:              record.record.RunID,
			VerificationStatus: status,
			VerifierID:         verifierID,
			VerifierVersion:    verifierVersion,
			VerifiedAt:         verifiedAt,
			BundlePath:         i.binding.Path,
			CanonicalBytes:     record.body,
			References:         append([]string(nil), record.record.EvidenceRefs...),
		})
	}
	return nodes, nil
}

func (i *AttestationImporter) verify(ctx context.Context, record importedAttestation) (VerificationStatus, string, string, time.Time, error) {
	publicKey, err := i.trust.GetTrustedSignerPublicKey(ctx, record.record.SignerKeyID)
	if errors.Is(err, constants.ErrTrustedSignerKeyNotFound) {
		return VerificationStatusUnverified, "", "", time.Time{}, nil
	}
	if err != nil {
		return "", "", "", time.Time{}, fmt.Errorf("%w: resolve attestation signer %s: %w", constants.ErrEvidenceTrustNotAssessed, record.record.SignerKeyID, err)
	}
	payload, err := canonicalAttestationPayload(record.record)
	if err != nil {
		return "", "", "", time.Time{}, fmt.Errorf("%w: canonicalize attestation: %w", constants.ErrEvidenceArtifactMalformed, err)
	}
	signature, decodeErr := hex.DecodeString(record.record.Signature)
	now := i.nowFunc()
	valid := decodeErr == nil && len(signature) == ed25519.SignatureSize && ed25519.Verify(publicKey, payload, signature)
	valid = valid && !record.record.Revoked && !now.Before(record.validFrom) && !now.After(record.validUntil)
	status := VerificationStatusVerified
	if !valid {
		status = VerificationStatusFailed
	}
	return status, constants.AttestationEvidenceVerifierID, constants.AttestationEvidenceVerifierVersion, now, nil
}

func validAttestationBinding(binding AttestationImportBinding) bool {
	_, _, referenceValid := ParseExpectedContentReference(binding.Reference, constants.AttestationCollectionReferencePrefix)
	return referenceValid && ValidRelativePath(binding.Path) && binding.ScopeID != "" && binding.RunID != ""
}

func decodeAttestations(body []byte, binding AttestationImportBinding) ([]importedAttestation, error) {
	lines := bytes.Split(body, []byte{'\n'})
	records := make([]importedAttestation, 0, len(lines))
	seen := make(map[string]struct{}, len(lines))
	for _, line := range lines {
		if len(line) == 0 {
			continue
		}
		if len(records) >= constants.AttestationMaxRecords {
			return nil, fmt.Errorf("%w: attestation count exceeds limit", constants.ErrEvidenceArtifactTooLarge)
		}
		if err := ValidateCanonicalJSON(line); err != nil {
			return nil, fmt.Errorf("%w: decode attestation: %w", constants.ErrEvidenceArtifactMalformed, err)
		}
		decoder := json.NewDecoder(bytes.NewReader(line))
		decoder.DisallowUnknownFields()
		var record attestationRecord
		if err := decoder.Decode(&record); err != nil {
			return nil, fmt.Errorf("%w: decode attestation: %w", constants.ErrEvidenceArtifactMalformed, err)
		}
		imported, err := validateAttestation(record, line, binding)
		if err != nil {
			return nil, err
		}
		if _, exists := seen[record.AttestationID]; exists {
			return nil, fmt.Errorf("%w: duplicate attestation identity", constants.ErrEvidenceArtifactMalformed)
		}
		seen[record.AttestationID] = struct{}{}
		records = append(records, imported)
	}
	if len(records) == 0 {
		return nil, fmt.Errorf("%w: attestation collection is empty", constants.ErrEvidenceArtifactMalformed)
	}
	return records, nil
}

func validateAttestation(record attestationRecord, body []byte, binding AttestationImportBinding) (importedAttestation, error) {
	if record.SchemaVersion != constants.AttestationSchemaVersion || record.AttestationID == "" || record.AttesterIdentity == "" || record.SignerKeyID == "" || record.ScopeID != binding.ScopeID || record.RunID != binding.RunID || strings.TrimSpace(record.Statement) == "" || len(record.AssertionIDs) == 0 || !validUniqueStrings(record.AssertionIDs) || !validUniqueStrings(record.EvidenceRefs) || record.Signature == "" {
		return importedAttestation{}, fmt.Errorf("%w: attestation binding is incomplete or unsupported", constants.ErrEvidenceArtifactMalformed)
	}
	issuedAt, err := timesvc.ParseTimestamp(record.IssuedAtUTC)
	if err != nil {
		return importedAttestation{}, fmt.Errorf("%w: parse attestation issuance timestamp: %w", constants.ErrEvidenceArtifactMalformed, err)
	}
	validFrom, err := timesvc.ParseTimestamp(record.ValidFromUTC)
	if err != nil {
		return importedAttestation{}, fmt.Errorf("%w: parse attestation validity start: %w", constants.ErrEvidenceArtifactMalformed, err)
	}
	validUntil, err := timesvc.ParseTimestamp(record.ValidUntilUTC)
	if err != nil {
		return importedAttestation{}, fmt.Errorf("%w: parse attestation validity end: %w", constants.ErrEvidenceArtifactMalformed, err)
	}
	if issuedAt.After(validFrom) || !validFrom.Before(validUntil) {
		return importedAttestation{}, fmt.Errorf("%w: attestation time bounds are invalid", constants.ErrEvidenceArtifactMalformed)
	}
	if err := validateRevocation(record, issuedAt, validUntil); err != nil {
		return importedAttestation{}, err
	}
	imported := importedAttestation{record: record, body: append([]byte(nil), body...), issuedAt: issuedAt, validFrom: validFrom, validUntil: validUntil}
	switch record.AttesterType {
	case attesterTypeCustomer:
		imported.artifactID = ContentReferenceForBody(constants.CustomerAttestationReferencePrefix, body)
		imported.artifactType = ArtifactTypeCustomerAttestation
		imported.schemaRef = "g8e.evidence.CustomerAttestation@" + constants.AttestationSchemaVersion
	case attesterTypeAssessor:
		imported.artifactID = ContentReferenceForBody(constants.AssessorAttestationReferencePrefix, body)
		imported.artifactType = ArtifactTypeAssessorAttestation
		imported.schemaRef = "g8e.evidence.AssessorAttestation@" + constants.AttestationSchemaVersion
	default:
		return importedAttestation{}, fmt.Errorf("%w: attester type is unsupported", constants.ErrEvidenceArtifactMalformed)
	}
	return imported, nil
}

func validateRevocation(record attestationRecord, issuedAt, validUntil time.Time) error {
	if record.Revoked != (record.RevokedAtUTC != "") {
		return fmt.Errorf("%w: attestation revocation status is inconsistent", constants.ErrEvidenceArtifactMalformed)
	}
	if !record.Revoked {
		return nil
	}
	revokedAt, err := timesvc.ParseTimestamp(record.RevokedAtUTC)
	if err != nil {
		return fmt.Errorf("%w: parse attestation revocation timestamp: %w", constants.ErrEvidenceArtifactMalformed, err)
	}
	if revokedAt.Before(issuedAt) || revokedAt.After(validUntil) {
		return fmt.Errorf("%w: attestation revocation timestamp is outside its bounds", constants.ErrEvidenceArtifactMalformed)
	}
	return nil
}

func canonicalAttestationPayload(record attestationRecord) ([]byte, error) {
	record.Signature = ""
	return json.Marshal(record)
}

func validUniqueStrings(values []string) bool {
	seen := make(map[string]struct{}, len(values))
	for _, value := range values {
		if value == "" {
			return false
		}
		if _, exists := seen[value]; exists {
			return false
		}
		seen[value] = struct{}{}
	}
	return true
}
