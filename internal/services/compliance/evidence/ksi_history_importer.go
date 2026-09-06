package evidence

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/g8e-ai/g8e/v2/internal/constants"
	"github.com/g8e-ai/g8e/v2/internal/services/compliance"
)

type KSIHistoryImportBinding struct {
	Reference        string
	Path             string
	ScopeID          string
	RunID            string
	Class            compliance.CertificationClass
	ProducerIdentity string
}

type KSIHistoryImporter struct {
	reader  ArtifactReader
	binding KSIHistoryImportBinding
}

type importedKSIHistorySnapshot struct {
	resultSet compliance.KSIResultSet
	body      []byte
	produced  time.Time
	artifact  string
}

func NewKSIHistoryImporter(reader ArtifactReader, binding KSIHistoryImportBinding) *KSIHistoryImporter {
	return &KSIHistoryImporter{reader: reader, binding: binding}
}

func (i *KSIHistoryImporter) SourceID() string {
	return "ksi-history"
}

func (i *KSIHistoryImporter) Import(ctx context.Context) ([]EvidenceNode, error) {
	if i == nil || i.reader == nil || !validKSIHistoryBinding(i.binding) {
		return nil, fmt.Errorf("%w: reader, content reference, path, scope, run, class, and producer are required", constants.ErrInvalidEvidenceGraph)
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	_, digest, _ := ParseExpectedContentReference(i.binding.Reference, constants.KSIHistoryReferencePrefix)
	result, err := ReadAndDigest(i.reader, ctx, i.binding.Path, constants.DemoRunMaxArtifactBytes)
	if err != nil {
		return nil, fmt.Errorf("%w: %s: %w", constants.ErrEvidenceImporterFailed, i.binding.Path, err)
	}
	if !VerifyDigest(result.Bytes, digest) {
		return nil, fmt.Errorf("%w: %s", constants.ErrChecksumMismatch, i.binding.Reference)
	}
	snapshots, err := decodeKSIHistory(result.Bytes, i.binding.Class)
	if err != nil {
		return nil, err
	}
	return i.buildNodes(snapshots), nil
}

func (i *KSIHistoryImporter) buildNodes(snapshots []importedKSIHistorySnapshot) []EvidenceNode {
	nodes := make([]EvidenceNode, 0, len(snapshots))
	for _, snapshot := range snapshots {
		nodes = append(nodes, EvidenceNode{
			ArtifactID:         snapshot.artifact,
			ArtifactType:       ArtifactTypeKSIResult,
			SHA256:             digestHex(snapshot.body),
			MediaType:          constants.MediaTypeJSON,
			SchemaRef:          "g8e.compliance.KSIResultSet@" + constants.KSIHistorySchemaVersion,
			ProducerIdentity:   i.binding.ProducerIdentity,
			ProducedAt:         snapshot.produced,
			ScopeID:            i.binding.ScopeID,
			RunID:              i.binding.RunID,
			VerificationStatus: VerificationStatusUnverified,
			BundlePath:         i.binding.Path,
			CanonicalBytes:     snapshot.body,
			References:         []string{},
		})
	}
	return nodes
}

func validKSIHistoryBinding(binding KSIHistoryImportBinding) bool {
	_, _, referenceValid := ParseExpectedContentReference(binding.Reference, constants.KSIHistoryReferencePrefix)
	return referenceValid && ValidRelativePath(binding.Path) && binding.ScopeID != "" && binding.RunID != "" && validCertificationClass(binding.Class) && binding.ProducerIdentity != ""
}

func decodeKSIHistory(body []byte, class compliance.CertificationClass) ([]importedKSIHistorySnapshot, error) {
	lines := bytes.Split(body, []byte{'\n'})
	snapshots := make([]importedKSIHistorySnapshot, 0, len(lines))
	for _, line := range lines {
		if len(line) == 0 {
			continue
		}
		if len(snapshots) >= constants.KSIHistoryMaxSnapshots {
			return nil, fmt.Errorf("%w: KSI history snapshot count exceeds limit", constants.ErrEvidenceArtifactTooLarge)
		}
		if err := ValidateCanonicalJSON(line); err != nil {
			return nil, fmt.Errorf("%w: decode KSI history snapshot: %w", constants.ErrEvidenceArtifactMalformed, err)
		}
		decoder := json.NewDecoder(bytes.NewReader(line))
		decoder.DisallowUnknownFields()
		var resultSet compliance.KSIResultSet
		if err := decoder.Decode(&resultSet); err != nil {
			return nil, fmt.Errorf("%w: decode KSI history snapshot: %w", constants.ErrEvidenceArtifactMalformed, err)
		}
		if err := validateKSIHistorySnapshot(resultSet, class, snapshots); err != nil {
			return nil, err
		}
		snapshots = append(snapshots, importedKSIHistorySnapshot{
			resultSet: resultSet,
			body:      append([]byte(nil), line...),
			produced:  time.UnixMilli(resultSet.EvaluatedAtMs).UTC(),
			artifact:  ContentReferenceForBody(constants.KSIResultReferencePrefix, line),
		})
	}
	if len(snapshots) == 0 {
		return nil, fmt.Errorf("%w: KSI history is empty", constants.ErrEvidenceArtifactMalformed)
	}
	return snapshots, nil
}

func validateKSIHistorySnapshot(resultSet compliance.KSIResultSet, class compliance.CertificationClass, prior []importedKSIHistorySnapshot) error {
	if resultSet.Class != class || resultSet.EvaluatedAtMs <= 0 || len(resultSet.Results) == 0 {
		return fmt.Errorf("%w: KSI history snapshot binding is incomplete", constants.ErrEvidenceArtifactMalformed)
	}
	if err := resultSet.Binding.Validate(); err != nil {
		return fmt.Errorf("%w: KSI history snapshot result set binding: %w", constants.ErrEvidenceArtifactMalformed, err)
	}
	if len(prior) > 0 && resultSet.EvaluatedAtMs <= prior[len(prior)-1].resultSet.EvaluatedAtMs {
		return fmt.Errorf("%w: KSI history chronology is not strictly increasing", constants.ErrEvidenceArtifactMalformed)
	}
	seen := make(map[string]struct{}, len(resultSet.Results))
	for _, result := range resultSet.Results {
		if result.ID == "" || !validKSIStatus(result.Status) || !result.Outcome.Valid() || result.Outcome.Status() != result.Status || result.MethodCount < 0 || result.LastValidatedUnixMs <= 0 || result.LastValidatedUnixMs > resultSet.EvaluatedAtMs {
			return fmt.Errorf("%w: KSI result binding is incomplete", constants.ErrEvidenceArtifactMalformed)
		}
		if err := result.Binding.Validate(); err != nil {
			return fmt.Errorf("%w: KSI result %s binding: %w", constants.ErrEvidenceArtifactMalformed, result.ID, err)
		}
		if result.Binding.ScopeID != resultSet.Binding.ScopeID || result.Binding.RunID != resultSet.Binding.RunID || result.Binding.WindowStartUnixMs != resultSet.Binding.WindowStartUnixMs || result.Binding.WindowEndUnixMs != resultSet.Binding.WindowEndUnixMs || result.Binding.EvaluatorID != resultSet.Binding.EvaluatorID || result.Binding.EvaluatorVersion != resultSet.Binding.EvaluatorVersion || result.Binding.MethodDefinitionID != resultSet.Binding.MethodDefinitionID {
			return fmt.Errorf("%w: KSI result %s binding does not match snapshot binding", constants.ErrEvidenceArtifactMalformed, result.ID)
		}
		if _, exists := seen[result.ID]; exists {
			return fmt.Errorf("%w: duplicate KSI result ID", constants.ErrEvidenceArtifactMalformed)
		}
		seen[result.ID] = struct{}{}
		for _, evidence := range result.Evidence {
			if evidence == nil || !validKSIEvidenceArtifactType(evidence.GetArtifactType()) || evidence.GetArtifactId() == "" {
				return fmt.Errorf("%w: KSI result evidence binding is incomplete", constants.ErrEvidenceArtifactMalformed)
			}
			if evidence.GetScopeId() != "" && evidence.GetScopeId() != resultSet.Binding.ScopeID {
				return fmt.Errorf("%w: KSI result %s evidence scope mismatch", constants.ErrEvidenceArtifactMalformed, result.ID)
			}
			if evidence.GetRunId() != "" && evidence.GetRunId() != resultSet.Binding.RunID {
				return fmt.Errorf("%w: KSI result %s evidence run mismatch", constants.ErrEvidenceArtifactMalformed, result.ID)
			}
		}
	}
	return nil
}

func validCertificationClass(class compliance.CertificationClass) bool {
	switch class {
	case compliance.ClassA, compliance.ClassB, compliance.ClassC, compliance.ClassD:
		return true
	default:
		return false
	}
}

func validKSIStatus(status compliance.KSIStatus) bool {
	switch status {
	case compliance.KSIStatusSatisfied, compliance.KSIStatusNotSatisfied, compliance.KSIStatusNotApplicable:
		return true
	default:
		return false
	}
}

func validKSIEvidenceArtifactType(artifactType string) bool {
	switch compliance.EvidenceType(artifactType) {
	case compliance.EvidenceTypeReceiptID, compliance.EvidenceTypeLedgerCommit, compliance.EvidenceTypeExecutionID, compliance.EvidenceTypeMerkleRoot:
		return true
	default:
		return false
	}
}
