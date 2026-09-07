package evidence

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/g8e-ai/g8e/v2/internal/constants"
	"github.com/g8e-ai/g8e/v2/internal/timesvc"
)

const (
	buildAttestationTypeBuild         = "build"
	buildAttestationTypeConfiguration = "configuration"
)

type BuildConfigImportBinding struct {
	Reference        string
	Path             string
	ScopeID          string
	RunID            string
	BuildIdentity    string
	SourceRevision   string
	ProducerIdentity string
}

type BuildConfigImporter struct {
	reader  ArtifactReader
	binding BuildConfigImportBinding
}

type buildConfigNamedDigest struct {
	Name   string `json:"name"`
	SHA256 string `json:"sha256"`
}

type buildConfigComponent struct {
	ComponentID   string `json:"component_id"`
	ComponentType string `json:"component_type"`
	Version       string `json:"version"`
	Digest        string `json:"digest"`
}

type buildConfigAttestationRecord struct {
	SchemaVersion       string                   `json:"schema_version"`
	AttestationType     string                   `json:"attestation_type"`
	ProducerIdentity    string                   `json:"producer_identity"`
	ProducedAtUTC       string                   `json:"produced_at_utc"`
	ScopeID             string                   `json:"scope_id"`
	RunID               string                   `json:"run_id"`
	BuildIdentity       string                   `json:"build_identity"`
	SourceRevision      string                   `json:"source_revision"`
	ImageDigests        []buildConfigNamedDigest `json:"image_digests,omitempty"`
	ComponentInventory  []buildConfigComponent   `json:"component_inventory,omitempty"`
	ConfigurationHashes []buildConfigNamedDigest `json:"configuration_hashes,omitempty"`
}

type importedBuildConfigAttestation struct {
	record       buildConfigAttestationRecord
	body         []byte
	producedAt   time.Time
	artifactID   string
	artifactType ArtifactType
	schemaRef    string
}

func NewBuildConfigImporter(reader ArtifactReader, binding BuildConfigImportBinding) *BuildConfigImporter {
	return &BuildConfigImporter{reader: reader, binding: binding}
}

func (i *BuildConfigImporter) SourceID() string {
	return "build-config"
}

func (i *BuildConfigImporter) Import(ctx context.Context) ([]EvidenceNode, error) {
	if i == nil || i.reader == nil || !validBuildConfigBinding(i.binding) {
		return nil, fmt.Errorf("%w: reader, content reference, path, scope, run, build identity, source revision, and producer are required", constants.ErrInvalidEvidenceGraph)
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	_, digest, _ := ParseExpectedContentReference(i.binding.Reference, constants.BuildAttestationReferencePrefix)
	result, err := ReadAndDigest(i.reader, ctx, i.binding.Path, constants.DemoRunMaxArtifactBytes)
	if err != nil {
		return nil, fmt.Errorf("%w: %s: %w", constants.ErrEvidenceImporterFailed, i.binding.Path, err)
	}
	if !VerifyDigest(result.Bytes, digest) {
		return nil, fmt.Errorf("%w: %s", constants.ErrChecksumMismatch, i.binding.Reference)
	}
	records, err := decodeBuildConfigAttestations(result.Bytes, i.binding)
	if err != nil {
		return nil, err
	}
	return i.buildNodes(records), nil
}

func (i *BuildConfigImporter) buildNodes(records []importedBuildConfigAttestation) []EvidenceNode {
	nodes := make([]EvidenceNode, 0, len(records))
	for _, record := range records {
		nodes = append(nodes, EvidenceNode{
			ArtifactID:         record.artifactID,
			ArtifactType:       record.artifactType,
			SHA256:             digestHex(record.body),
			MediaType:          constants.MediaTypeJSON,
			SchemaRef:          record.schemaRef,
			ProducerIdentity:   record.record.ProducerIdentity,
			ProducedAt:         record.producedAt,
			ScopeID:            record.record.ScopeID,
			RunID:              record.record.RunID,
			VerificationStatus: VerificationStatusUnverified,
			BundlePath:         i.binding.Path,
			CanonicalBytes:     record.body,
			References:         []string{},
		})
	}
	return nodes
}

func validBuildConfigBinding(binding BuildConfigImportBinding) bool {
	_, _, referenceValid := ParseExpectedContentReference(binding.Reference, constants.BuildAttestationReferencePrefix)
	return referenceValid && ValidRelativePath(binding.Path) && binding.ScopeID != "" && binding.RunID != "" && binding.BuildIdentity != "" && binding.SourceRevision != "" && binding.ProducerIdentity != ""
}

func decodeBuildConfigAttestations(body []byte, binding BuildConfigImportBinding) ([]importedBuildConfigAttestation, error) {
	lines := bytes.Split(body, []byte{'\n'})
	records := make([]importedBuildConfigAttestation, 0, len(lines))
	seen := make(map[string]struct{}, len(lines))
	for _, line := range lines {
		if len(line) == 0 {
			continue
		}
		if len(records) >= constants.BuildAttestationMaxRecords {
			return nil, fmt.Errorf("%w: build and configuration attestation count exceeds limit", constants.ErrEvidenceArtifactTooLarge)
		}
		if err := ValidateCanonicalJSON(line); err != nil {
			return nil, fmt.Errorf("%w: decode build or configuration attestation: %w", constants.ErrEvidenceArtifactMalformed, err)
		}
		decoder := json.NewDecoder(bytes.NewReader(line))
		decoder.DisallowUnknownFields()
		var record buildConfigAttestationRecord
		if err := decoder.Decode(&record); err != nil {
			return nil, fmt.Errorf("%w: decode build or configuration attestation: %w", constants.ErrEvidenceArtifactMalformed, err)
		}
		imported, err := validateBuildConfigAttestation(record, line, binding)
		if err != nil {
			return nil, err
		}
		if _, exists := seen[imported.artifactID]; exists {
			return nil, fmt.Errorf("%w: duplicate build or configuration attestation", constants.ErrEvidenceArtifactMalformed)
		}
		seen[imported.artifactID] = struct{}{}
		records = append(records, imported)
	}
	if len(records) == 0 {
		return nil, fmt.Errorf("%w: build and configuration attestation collection is empty", constants.ErrEvidenceArtifactMalformed)
	}
	return records, nil
}

func validateBuildConfigAttestation(record buildConfigAttestationRecord, body []byte, binding BuildConfigImportBinding) (importedBuildConfigAttestation, error) {
	if record.SchemaVersion != constants.BuildAttestationSchemaVersion || record.ProducerIdentity != binding.ProducerIdentity || record.ScopeID != binding.ScopeID || record.RunID != binding.RunID || record.BuildIdentity != binding.BuildIdentity || record.SourceRevision != binding.SourceRevision {
		return importedBuildConfigAttestation{}, fmt.Errorf("%w: build or configuration attestation binding is incomplete or unsupported", constants.ErrEvidenceArtifactMalformed)
	}
	producedAt, err := timesvc.ParseTimestamp(record.ProducedAtUTC)
	if err != nil {
		return importedBuildConfigAttestation{}, fmt.Errorf("%w: parse build or configuration attestation timestamp: %w", constants.ErrEvidenceArtifactMalformed, err)
	}
	imported := importedBuildConfigAttestation{record: record, body: append([]byte(nil), body...), producedAt: producedAt}
	switch record.AttestationType {
	case buildAttestationTypeBuild:
		if len(record.ImageDigests) == 0 || len(record.ComponentInventory) == 0 || len(record.ConfigurationHashes) != 0 || !validBuildConfigDigests(record.ImageDigests) || !validBuildConfigComponents(record.ComponentInventory) {
			return importedBuildConfigAttestation{}, fmt.Errorf("%w: build attestation evidence is incomplete", constants.ErrEvidenceArtifactMalformed)
		}
		imported.artifactID = ContentReferenceForBody(constants.BuildAttestationReferencePrefix, body)
		imported.artifactType = ArtifactTypeBuildAttestation
		imported.schemaRef = "g8e.evidence.BuildAttestation@" + constants.BuildAttestationSchemaVersion
	case buildAttestationTypeConfiguration:
		if len(record.ConfigurationHashes) == 0 || len(record.ImageDigests) != 0 || len(record.ComponentInventory) != 0 || !validBuildConfigDigests(record.ConfigurationHashes) {
			return importedBuildConfigAttestation{}, fmt.Errorf("%w: configuration attestation evidence is incomplete", constants.ErrEvidenceArtifactMalformed)
		}
		imported.artifactID = ContentReferenceForBody(constants.ConfigAttestationReferencePrefix, body)
		imported.artifactType = ArtifactTypeConfigAttestation
		imported.schemaRef = "g8e.evidence.ConfigAttestation@" + constants.BuildAttestationSchemaVersion
	default:
		return importedBuildConfigAttestation{}, fmt.Errorf("%w: build or configuration attestation type is unsupported", constants.ErrEvidenceArtifactMalformed)
	}
	return imported, nil
}

func validBuildConfigDigests(digests []buildConfigNamedDigest) bool {
	seen := make(map[string]struct{}, len(digests))
	for _, digest := range digests {
		if digest.Name == "" || !validBuildConfigSHA256(digest.SHA256) {
			return false
		}
		if _, exists := seen[digest.Name]; exists {
			return false
		}
		seen[digest.Name] = struct{}{}
	}
	return true
}

func validBuildConfigComponents(components []buildConfigComponent) bool {
	seen := make(map[string]struct{}, len(components))
	for _, component := range components {
		if component.ComponentID == "" || component.ComponentType == "" || component.Version == "" || !validBuildConfigSHA256(component.Digest) {
			return false
		}
		if _, exists := seen[component.ComponentID]; exists {
			return false
		}
		seen[component.ComponentID] = struct{}{}
	}
	return true
}

func validBuildConfigSHA256(value string) bool {
	if len(value) != sha256.Size*2 || strings.ToLower(value) != value {
		return false
	}
	decoded, err := hex.DecodeString(value)
	return err == nil && len(decoded) == sha256.Size
}
