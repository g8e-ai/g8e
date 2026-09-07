package evidence

import (
	"bytes"
	"context"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/g8e-ai/g8e/v2/internal/constants"
	"github.com/g8e-ai/g8e/v2/internal/timesvc"
)

const gitObjectIDHexLength = 40

type LedgerImportBinding struct {
	CommitsReference string
	StateReference   string
	CommitsPath      string
	StatePath        string
	ScopeID          string
	RunID            string
	AttemptID        string
	ScenarioID       string
}

type LedgerImporter struct {
	reader  ArtifactReader
	binding LedgerImportBinding
}

type ledgerCommitRecord struct {
	SchemaVersion    string `json:"schema_version"`
	ProducerIdentity string `json:"producer_identity"`
	CommitHash       string `json:"commit_hash"`
	ParentHash       string `json:"parent_hash"`
	TimestampUTC     string `json:"timestamp_utc"`
	Message          string `json:"message"`
	FilesChanged     int    `json:"files_changed"`
	DiffStat         string `json:"diff_stat"`
}

type ledgerStateRecord struct {
	SchemaVersion    string `json:"schema_version"`
	ProducerIdentity string `json:"producer_identity"`
	MerkleRoot       string `json:"merkle_root"`
	CapturedAtUTC    string `json:"captured_at_utc"`
}

type importedLedgerCommit struct {
	record    ledgerCommitRecord
	body      []byte
	produced  time.Time
	artifact  string
	reference []string
}

func NewLedgerImporter(reader ArtifactReader, binding LedgerImportBinding) *LedgerImporter {
	return &LedgerImporter{reader: reader, binding: binding}
}

func (i *LedgerImporter) SourceID() string {
	return "ledger"
}

func (i *LedgerImporter) Import(ctx context.Context) ([]EvidenceNode, error) {
	if i == nil || i.reader == nil || !validLedgerBinding(i.binding) {
		return nil, fmt.Errorf("%w: reader, content references, paths, scope, and run are required", constants.ErrInvalidEvidenceGraph)
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	commitsBody, err := i.readSource(ctx, i.binding.CommitsPath, i.binding.CommitsReference, constants.LedgerCommitCollectionReferencePrefix)
	if err != nil {
		return nil, err
	}
	stateBody, err := i.readSource(ctx, i.binding.StatePath, i.binding.StateReference, constants.LedgerStateReferencePrefix)
	if err != nil {
		return nil, err
	}
	commits, err := decodeLedgerCommits(commitsBody)
	if err != nil {
		return nil, err
	}
	state, capturedAt, err := decodeLedgerState(stateBody)
	if err != nil {
		return nil, err
	}
	if err := validateLedgerSnapshot(commits, state, capturedAt); err != nil {
		return nil, err
	}
	return i.buildNodes(commits, stateBody, state, capturedAt), nil
}

func (i *LedgerImporter) readSource(ctx context.Context, path, reference, prefix string) ([]byte, error) {
	_, digest, _ := ParseExpectedContentReference(reference, prefix)
	result, err := ReadAndDigest(i.reader, ctx, path, constants.DemoRunMaxArtifactBytes)
	if err != nil {
		return nil, fmt.Errorf("%w: %s: %w", constants.ErrEvidenceImporterFailed, path, err)
	}
	if !VerifyDigest(result.Bytes, digest) {
		return nil, fmt.Errorf("%w: %s", constants.ErrChecksumMismatch, reference)
	}
	return result.Bytes, nil
}

func (i *LedgerImporter) buildNodes(commits []importedLedgerCommit, stateBody []byte, state ledgerStateRecord, capturedAt time.Time) []EvidenceNode {
	nodes := make([]EvidenceNode, 0, len(commits)+1)
	for _, commit := range commits {
		nodes = append(nodes, EvidenceNode{
			ArtifactID:         commit.artifact,
			ArtifactType:       ArtifactTypeLedgerCommit,
			SHA256:             digestHex(commit.body),
			MediaType:          constants.MediaTypeJSON,
			SchemaRef:          "g8e.evidence.LedgerCommit@" + constants.LedgerEvidenceSchemaVersion,
			ProducerIdentity:   commit.record.ProducerIdentity,
			ProducedAt:         commit.produced,
			ScopeID:            i.binding.ScopeID,
			RunID:              i.binding.RunID,
			AttemptID:          i.binding.AttemptID,
			ScenarioID:         i.binding.ScenarioID,
			VerificationStatus: VerificationStatusUnverified,
			BundlePath:         i.binding.CommitsPath,
			CanonicalBytes:     commit.body,
			References:         commit.reference,
		})
	}
	nodes = append(nodes, EvidenceNode{
		ArtifactID:         i.binding.StateReference,
		ArtifactType:       ArtifactTypeLedgerState,
		SHA256:             digestHex(stateBody),
		MediaType:          constants.MediaTypeJSON,
		SchemaRef:          "g8e.evidence.LedgerState@" + constants.LedgerEvidenceSchemaVersion,
		ProducerIdentity:   state.ProducerIdentity,
		ProducedAt:         capturedAt,
		ScopeID:            i.binding.ScopeID,
		RunID:              i.binding.RunID,
		AttemptID:          i.binding.AttemptID,
		ScenarioID:         i.binding.ScenarioID,
		VerificationStatus: VerificationStatusUnverified,
		BundlePath:         i.binding.StatePath,
		CanonicalBytes:     stateBody,
		References:         []string{commits[len(commits)-1].artifact},
	})
	return nodes
}

func validLedgerBinding(binding LedgerImportBinding) bool {
	_, _, commitsValid := ParseExpectedContentReference(binding.CommitsReference, constants.LedgerCommitCollectionReferencePrefix)
	_, _, stateValid := ParseExpectedContentReference(binding.StateReference, constants.LedgerStateReferencePrefix)
	return commitsValid && stateValid && ValidRelativePath(binding.CommitsPath) && ValidRelativePath(binding.StatePath) && binding.ScopeID != "" && binding.RunID != ""
}

func decodeLedgerCommits(body []byte) ([]importedLedgerCommit, error) {
	lines := bytes.Split(body, []byte{'\n'})
	commits := make([]importedLedgerCommit, 0, len(lines))
	for _, line := range lines {
		if len(line) == 0 {
			continue
		}
		if len(commits) >= constants.LedgerEvidenceMaxCommits {
			return nil, fmt.Errorf("%w: ledger commit count exceeds limit", constants.ErrEvidenceArtifactTooLarge)
		}
		decoder, err := newCanonicalLedgerDecoder(line)
		if err != nil {
			return nil, fmt.Errorf("%w: decode ledger commit: %w", constants.ErrEvidenceArtifactMalformed, err)
		}
		var record ledgerCommitRecord
		if err := decoder.Decode(&record); err != nil {
			return nil, fmt.Errorf("%w: decode ledger commit: %w", constants.ErrEvidenceArtifactMalformed, err)
		}
		producedAt, err := timesvc.ParseTimestamp(record.TimestampUTC)
		if err != nil {
			return nil, fmt.Errorf("%w: parse ledger commit timestamp: %w", constants.ErrEvidenceArtifactMalformed, err)
		}
		commits = append(commits, importedLedgerCommit{record: record, body: append([]byte(nil), line...), produced: producedAt, artifact: ContentReferenceForBody(constants.LedgerCommitReferencePrefix, line), reference: []string{}})
	}
	if len(commits) == 0 {
		return nil, fmt.Errorf("%w: ledger commit collection is empty", constants.ErrEvidenceArtifactMalformed)
	}
	return commits, nil
}

func decodeLedgerState(body []byte) (ledgerStateRecord, time.Time, error) {
	decoder, err := newCanonicalLedgerDecoder(body)
	if err != nil {
		return ledgerStateRecord{}, time.Time{}, fmt.Errorf("%w: decode ledger state: %w", constants.ErrEvidenceArtifactMalformed, err)
	}
	var state ledgerStateRecord
	if err := decoder.Decode(&state); err != nil {
		return ledgerStateRecord{}, time.Time{}, fmt.Errorf("%w: decode ledger state: %w", constants.ErrEvidenceArtifactMalformed, err)
	}
	capturedAt, err := timesvc.ParseTimestamp(state.CapturedAtUTC)
	if err != nil {
		return ledgerStateRecord{}, time.Time{}, fmt.Errorf("%w: parse ledger state timestamp: %w", constants.ErrEvidenceArtifactMalformed, err)
	}
	return state, capturedAt, nil
}

func newCanonicalLedgerDecoder(body []byte) (*json.Decoder, error) {
	if err := ValidateCanonicalJSON(body); err != nil {
		return nil, err
	}
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.DisallowUnknownFields()
	return decoder, nil
}

func validateLedgerSnapshot(commits []importedLedgerCommit, state ledgerStateRecord, capturedAt time.Time) error {
	if state.SchemaVersion != constants.LedgerEvidenceSchemaVersion || state.ProducerIdentity == "" || !validGitObjectID(state.MerkleRoot) {
		return fmt.Errorf("%w: ledger state binding is incomplete or unsupported", constants.ErrEvidenceArtifactMalformed)
	}
	seen := make(map[string]struct{}, len(commits))
	for index := range commits {
		commit := &commits[index]
		if commit.record.SchemaVersion != constants.LedgerEvidenceSchemaVersion || commit.record.ProducerIdentity == "" || commit.record.Message == "" || commit.record.FilesChanged < 0 || !validGitObjectID(commit.record.CommitHash) {
			return fmt.Errorf("%w: ledger commit binding is incomplete or unsupported", constants.ErrEvidenceArtifactMalformed)
		}
		if commit.record.ProducerIdentity != state.ProducerIdentity {
			return fmt.Errorf("%w: ledger producer identity changes within snapshot", constants.ErrEvidenceArtifactMalformed)
		}
		if _, exists := seen[commit.record.CommitHash]; exists {
			return fmt.Errorf("%w: duplicate ledger commit hash", constants.ErrEvidenceArtifactMalformed)
		}
		seen[commit.record.CommitHash] = struct{}{}
		if index == 0 {
			if commit.record.ParentHash != "" {
				return fmt.Errorf("%w: first ledger commit has a parent", constants.ErrEvidenceArtifactMalformed)
			}
			continue
		}
		prior := &commits[index-1]
		if commit.record.ParentHash != prior.record.CommitHash || commit.produced.Before(prior.produced) {
			return fmt.Errorf("%w: ledger commit chain or timestamp order is invalid", constants.ErrEvidenceArtifactMalformed)
		}
		commit.reference = []string{prior.artifact}
	}
	head := commits[len(commits)-1]
	if state.MerkleRoot != head.record.CommitHash || capturedAt.Before(head.produced) {
		return fmt.Errorf("%w: ledger state does not bind the captured head", constants.ErrEvidenceArtifactMalformed)
	}
	return nil
}

func validGitObjectID(value string) bool {
	if len(value) != gitObjectIDHexLength || strings.ToLower(value) != value {
		return false
	}
	decoded, err := hex.DecodeString(value)
	return err == nil && len(decoded)*2 == gitObjectIDHexLength
}
