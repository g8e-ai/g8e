package evidence

import (
	"context"
	"fmt"

	"github.com/g8e-ai/g8e/v2/internal/constants"
	"github.com/g8e-ai/g8e/v2/internal/timesvc"
	compliancev1 "github.com/g8e-ai/g8e/v2/protocol/proto/g8e/compliance/v1"
	operatorv1 "github.com/g8e-ai/g8e/v2/protocol/proto/g8e/operator/v1"
)

type AuditRecordImportBinding struct {
	Reference         string
	Path              string
	ScopeID           string
	RunID             string
	AttemptID         string
	ScenarioID        string
	OperatorSessionID string
}

type AuditRecordImporter struct {
	reader  ArtifactReader
	binding AuditRecordImportBinding
}

func NewAuditRecordImporter(reader ArtifactReader, binding AuditRecordImportBinding) *AuditRecordImporter {
	return &AuditRecordImporter{reader: reader, binding: binding}
}

func (i *AuditRecordImporter) SourceID() string {
	return "audit-record"
}

func (i *AuditRecordImporter) Import(ctx context.Context) ([]EvidenceNode, error) {
	if i == nil || i.reader == nil || !validAuditRecordBinding(i.binding) {
		return nil, fmt.Errorf("%w: reader, content reference, path, scope, run, and operator session are required", constants.ErrInvalidEvidenceGraph)
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	_, digest, _ := ParseExpectedContentReference(i.binding.Reference, constants.AuditRecordReferencePrefix)
	result, err := ReadAndDigest(i.reader, ctx, i.binding.Path, constants.DemoRunMaxArtifactBytes)
	if err != nil {
		return nil, fmt.Errorf("%w: %s: %w", constants.ErrEvidenceImporterFailed, i.binding.Path, err)
	}
	if !VerifyDigest(result.Bytes, digest) {
		return nil, fmt.Errorf("%w: %s", constants.ErrChecksumMismatch, i.binding.Reference)
	}
	event := &operatorv1.AuditEvent{}
	if err := compliancev1.UnmarshalCanonical(result.Bytes, event); err != nil {
		return nil, fmt.Errorf("%w: %s: %w", constants.ErrEvidenceArtifactMalformed, i.binding.Path, err)
	}
	if event.GetId() <= 0 || event.GetOperatorSessionId() == "" || event.GetTimestamp() == "" || event.GetType() == "" {
		return nil, fmt.Errorf("%w: audit event binding is incomplete", constants.ErrEvidenceArtifactMalformed)
	}
	if event.GetOperatorSessionId() != i.binding.OperatorSessionID {
		return nil, fmt.Errorf("%w: audit event operator session does not match import binding", constants.ErrEvidenceScopeMismatch)
	}
	producedAt, err := timesvc.ParseTimestamp(event.GetTimestamp())
	if err != nil {
		return nil, fmt.Errorf("%w: parse audit event timestamp: %w", constants.ErrEvidenceArtifactMalformed, err)
	}
	return []EvidenceNode{{ArtifactID: i.binding.Reference, ArtifactType: ArtifactTypeAuditRecord, SHA256: digest, MediaType: constants.MediaTypeJSON, SchemaRef: "g8e.operator.v1.AuditEvent", ProducerIdentity: event.GetOperatorSessionId(), ProducedAt: producedAt, ScopeID: i.binding.ScopeID, RunID: i.binding.RunID, AttemptID: i.binding.AttemptID, ScenarioID: i.binding.ScenarioID, VerificationStatus: VerificationStatusUnverified, BundlePath: i.binding.Path, CanonicalBytes: result.Bytes, References: []string{}}}, nil
}

func validAuditRecordBinding(binding AuditRecordImportBinding) bool {
	_, _, referenceValid := ParseExpectedContentReference(binding.Reference, constants.AuditRecordReferencePrefix)
	return referenceValid && ValidRelativePath(binding.Path) && binding.ScopeID != "" && binding.RunID != "" && binding.OperatorSessionID != ""
}
