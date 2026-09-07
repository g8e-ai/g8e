package evidence

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/g8e-ai/g8e/v2/internal/constants"
	compliancev1 "github.com/g8e-ai/g8e/v2/protocol/proto/g8e/compliance/v1"
	operatorv1 "github.com/g8e-ai/g8e/v2/protocol/proto/g8e/operator/v1"
)

type evalRunManifest struct {
	SchemaVersion       string    `json:"schema_version"`
	RunID               string    `json:"run_id"`
	SuiteID             string    `json:"suite_id"`
	SuiteVersion        string    `json:"suite_version"`
	CreatedAt           time.Time `json:"created_at"`
	OrchestratorVersion string    `json:"orchestrator_version"`
}

type evalTaskDefinition struct {
	SchemaVersion string `json:"schema_version"`
	TaskID        string `json:"task_id"`
	SuiteID       string `json:"suite_id"`
	SuiteVersion  string `json:"suite_version"`
}

type evalAttemptRecord struct {
	SchemaVersion                          string     `json:"schema_version"`
	AttemptID                              string     `json:"attempt_id"`
	RunID                                  string     `json:"run_id"`
	TaskID                                 string     `json:"task_id"`
	StartedAt                              *time.Time `json:"started_at"`
	EndedAt                                *time.Time `json:"ended_at"`
	AnswerRef                              *string    `json:"answer_ref"`
	FinalStateObservationRefs              []string   `json:"final_state_observation_refs"`
	StateObservationRefs                   []string   `json:"state_observation_refs"`
	RehydrationObservationRefs             []string   `json:"rehydration_observation_refs"`
	SecretDetectionObservationRefs         []string   `json:"secret_detection_observation_refs"`
	UnauthorizedMutationObservationRefs    []string   `json:"unauthorized_mutation_observation_refs"`
	TokenStorePersistenceObservationRefs   []string   `json:"token_store_persistence_observation_refs"`
	TokenTTLExpiryObservationRefs          []string   `json:"token_ttl_expiry_observation_refs"`
	TokenPersistenceFailureObservationRefs []string   `json:"token_persistence_failure_observation_refs"`
	ExfiltrationAttemptObservationRefs     []string   `json:"exfiltration_attempt_observation_refs"`
	ArtifactLeakageObservationRefs         []string   `json:"artifact_leakage_observation_refs"`
	ReplayAttemptObservationRefs           []string   `json:"replay_attempt_observation_refs"`
	SignedFieldTamperingObservationRefs    []string   `json:"signed_field_tampering_observation_refs"`
	PayloadTamperingObservationRefs        []string   `json:"payload_tampering_observation_refs"`
	StaleStateRootObservationRefs          []string   `json:"stale_state_root_observation_refs"`
	IdentityMismatchObservationRefs        []string   `json:"identity_mismatch_observation_refs"`
	NonceExpirationObservationRefs         []string   `json:"nonce_expiration_observation_refs"`
	SignerDefectObservationRefs            []string   `json:"signer_defect_observation_refs"`
	L3ProofTransplantObservationRefs       []string   `json:"l3_proof_transplant_observation_refs"`
	RevokedCredentialObservationRefs       []string   `json:"revoked_credential_observation_refs"`
	EvidencePreservationObservationRefs    []string   `json:"evidence_preservation_observation_refs"`
	PolicyAttackObservationRefs            []string   `json:"policy_attack_observation_refs"`
	ToolSequenceObservationRefs            []string   `json:"tool_sequence_observation_refs"`
	FactualQAObservationRefs               []string   `json:"factual_qa_observation_refs"`
	CitationBackedObservationRefs          []string   `json:"citation_backed_observation_refs"`
	PartialMilestoneObservationRefs        []string   `json:"partial_milestone_observation_refs"`
	ReliabilityObservationRefs             []string   `json:"reliability_observation_refs"`
	EconomicsPerformanceObservationRefs    []string   `json:"economics_performance_observation_refs"`
	ReceiptRefs                            []string   `json:"receipt_refs"`
	GradeRefs                              []string   `json:"grade_refs"`
}

type evalReceiptObservation struct {
	SchemaVersion string          `json:"schema_version"`
	ReceiptID     string          `json:"receipt_id"`
	AttemptID     string          `json:"attempt_id"`
	RunID         string          `json:"run_id"`
	TransactionID string          `json:"transaction_id"`
	ActionType    string          `json:"action_type"`
	ActionReceipt json.RawMessage `json:"action_receipt"`
}

type evalStageObservation struct {
	SchemaVersion string `json:"schema_version"`
	StageID       string `json:"stage_id"`
	AttemptID     string `json:"attempt_id"`
	RunID         string `json:"run_id"`
	Provider      string `json:"provider"`
	Model         string `json:"model"`
	AgentRole     string `json:"agent_role"`
}

type evalMetricObservation struct {
	SchemaVersion      string   `json:"schema_version"`
	MetricID           string   `json:"metric_id"`
	MetricVersion      string   `json:"metric_version"`
	AttemptID          string   `json:"attempt_id"`
	RunID              string   `json:"run_id"`
	TaskID             string   `json:"task_id"`
	VerificationStatus string   `json:"verification_status"`
	GraderClass        string   `json:"grader_class"`
	EvidenceRefs       []string `json:"evidence_refs"`
}

type evalEvidenceEncryption struct {
	Algorithm            string `json:"algorithm"`
	KeyID                string `json:"key_id"`
	AADSHA256            string `json:"aad_sha256"`
	CiphertextSHA256     string `json:"ciphertext_sha256"`
	CiphertextByteLength int    `json:"ciphertext_byte_length"`
}

type evalEvidenceAccessControl struct {
	Policy             string `json:"policy"`
	AuthorizationScope string `json:"authorization_scope"`
}

type evalEncryptedEvidenceEnvelope struct {
	Version          int    `json:"version"`
	Algorithm        string `json:"algorithm"`
	KeyID            string `json:"key_id"`
	NonceBase64      string `json:"nonce_b64"`
	CiphertextBase64 string `json:"ciphertext_b64"`
}

type evalEvidenceAAD struct {
	AccessControl         *evalEvidenceAccessControl `json:"access_control"`
	ArtifactID            string                     `json:"artifact_id"`
	AttemptID             *string                    `json:"attempt_id"`
	ByteLength            int                        `json:"byte_length"`
	MediaType             string                     `json:"media_type"`
	PrivacyClassification string                     `json:"privacy_classification"`
	RunID                 string                     `json:"run_id"`
	SchemaRef             string                     `json:"schema_ref"`
	SHA256                string                     `json:"sha256"`
	StorageLocation       string                     `json:"storage_location"`
}

type evalEvidenceIndex struct {
	SchemaVersion         string                     `json:"schema_version"`
	ArtifactID            string                     `json:"artifact_id"`
	RunID                 string                     `json:"run_id"`
	AttemptID             *string                    `json:"attempt_id"`
	MediaType             string                     `json:"media_type"`
	SchemaRef             string                     `json:"schema_ref"`
	ByteLength            int                        `json:"byte_length"`
	SHA256                string                     `json:"sha256"`
	ProducerIdentity      string                     `json:"producer_identity"`
	PrivacyClassification string                     `json:"privacy_classification"`
	StorageLocation       string                     `json:"storage_location"`
	Encryption            *evalEvidenceEncryption    `json:"encryption"`
	AccessControl         *evalEvidenceAccessControl `json:"access_control"`
	ParentEvidenceRefs    []string                   `json:"parent_evidence_refs"`
}

type evalJSONRecord interface {
	evalTaskDefinition | evalAttemptRecord | evalReceiptObservation | evalStageObservation | evalMetricObservation | evalEvidenceIndex
}

type evalJSONLRecord[T evalJSONRecord] struct {
	Value T
	Bytes []byte
	Line  int
}

type EvalBundleImporter struct {
	reader  ArtifactReader
	runID   string
	runDir  string
	nowFunc func() time.Time
}

func NewEvalBundleImporter(reader ArtifactReader, runID, runDir string) *EvalBundleImporter {
	return &EvalBundleImporter{reader: reader, runID: runID, runDir: runDir, nowFunc: time.Now}
}

func (i *EvalBundleImporter) SourceID() string {
	return "eval-bundle"
}

func (i *EvalBundleImporter) sourceRunID() string {
	if i == nil {
		return ""
	}
	return i.runID
}

func EvalScopeID(suiteID string) string {
	return constants.EvalScopePrefix + suiteID
}

func (i *EvalBundleImporter) Import(ctx context.Context) ([]EvidenceNode, error) {
	if i == nil || i.reader == nil || !ValidPathElement(i.runID) || !ValidRelativePath(i.runDir) {
		return nil, fmt.Errorf("%w: reader, canonical run ID, and matching relative run directory are required", constants.ErrInvalidEvidenceGraph)
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	manifest, manifestBody, err := i.loadManifest(ctx)
	if err != nil {
		return nil, err
	}
	tasks, err := readEvalJSONL[evalTaskDefinition](ctx, i.reader, i.path(constants.EvalRunTasksFilename), false)
	if err != nil {
		return nil, err
	}
	attempts, err := readEvalJSONL[evalAttemptRecord](ctx, i.reader, i.path(constants.EvalRunAttemptsFilename), false)
	if err != nil {
		return nil, err
	}
	receipts, err := readEvalJSONL[evalReceiptObservation](ctx, i.reader, i.path(constants.EvalRunReceiptsFilename), true)
	if err != nil {
		return nil, err
	}
	stages, err := readEvalJSONL[evalStageObservation](ctx, i.reader, i.path(constants.EvalRunStagesFilename), true)
	if err != nil {
		return nil, err
	}
	metrics, err := readEvalJSONL[evalMetricObservation](ctx, i.reader, i.path(constants.EvalRunMetricsFilename), false)
	if err != nil {
		return nil, err
	}
	indices, err := readEvalJSONL[evalEvidenceIndex](ctx, i.reader, i.path(constants.EvalRunEvidenceIndexFilename), true)
	if err != nil {
		return nil, err
	}
	return i.buildNodes(ctx, manifest, manifestBody, tasks, attempts, receipts, stages, metrics, indices)
}

func (i *EvalBundleImporter) loadManifest(ctx context.Context) (evalRunManifest, []byte, error) {
	path := i.path(constants.EvalRunManifestFilename)
	result, err := ReadAndDigest(i.reader, ctx, path, constants.DemoRunMaxArtifactBytes)
	if err != nil {
		return evalRunManifest{}, nil, fmt.Errorf("%w: %s: %w", constants.ErrEvidenceImporterFailed, path, err)
	}
	var manifest evalRunManifest
	if err := json.Unmarshal(result.Bytes, &manifest); err != nil {
		return evalRunManifest{}, nil, fmt.Errorf("%w: %s: %w", constants.ErrEvidenceArtifactMalformed, path, err)
	}
	if manifest.RunID != i.runID {
		return evalRunManifest{}, nil, fmt.Errorf("%w: manifest run ID %s does not match importer run ID %s", constants.ErrEvidenceScopeMismatch, manifest.RunID, i.runID)
	}
	if manifest.SchemaVersion == "" || manifest.SuiteID == "" || manifest.SuiteVersion == "" || manifest.CreatedAt.IsZero() {
		return evalRunManifest{}, nil, fmt.Errorf("%w: %s: required manifest binding is empty", constants.ErrEvidenceArtifactMalformed, path)
	}
	return manifest, result.Bytes, nil
}

func readEvalJSONL[T evalJSONRecord](ctx context.Context, reader ArtifactReader, path string, allowEmpty bool) ([]evalJSONLRecord[T], error) {
	result, err := ReadAndDigest(reader, ctx, path, constants.DemoRunMaxArtifactBytes)
	if err != nil {
		return nil, fmt.Errorf("%w: %s: %w", constants.ErrEvidenceImporterFailed, path, err)
	}
	lines := splitJSONL(result.Bytes)
	if (!allowEmpty && len(lines) == 0) || len(lines) > constants.EvalRunMaxRecords {
		return nil, fmt.Errorf("%w: %s: record count is empty or exceeds the limit", constants.ErrEvidenceArtifactMalformed, path)
	}
	records := make([]evalJSONLRecord[T], 0, len(lines))
	for index, line := range lines {
		if err := ValidateCanonicalJSON(line); err != nil {
			return nil, fmt.Errorf("%w: %s#%d: %w", constants.ErrEvidenceArtifactMalformed, path, index+1, err)
		}
		var value T
		if err := json.Unmarshal(line, &value); err != nil {
			return nil, fmt.Errorf("%w: %s#%d: %w", constants.ErrEvidenceArtifactMalformed, path, index+1, err)
		}
		records = append(records, evalJSONLRecord[T]{Value: value, Bytes: line, Line: index + 1})
	}
	return records, nil
}

func (i *EvalBundleImporter) buildNodes(ctx context.Context, manifest evalRunManifest, manifestBody []byte, tasks []evalJSONLRecord[evalTaskDefinition], attempts []evalJSONLRecord[evalAttemptRecord], receipts []evalJSONLRecord[evalReceiptObservation], stages []evalJSONLRecord[evalStageObservation], metrics []evalJSONLRecord[evalMetricObservation], indices []evalJSONLRecord[evalEvidenceIndex]) ([]EvidenceNode, error) {
	scopeID := EvalScopeID(manifest.SuiteID)
	manifestID := ContentAddress(ArtifactTypeEvalManifest, manifestBody)
	nodes := []EvidenceNode{{ArtifactID: manifestID, ArtifactType: ArtifactTypeEvalManifest, SHA256: digestHex(manifestBody), MediaType: constants.MediaTypeJSON, SchemaRef: "g8e_evals.schema.RunManifest", ProducerIdentity: manifest.SuiteID, ProducedAt: manifest.CreatedAt, ScopeID: scopeID, RunID: manifest.RunID, VerificationStatus: VerificationStatusVerified, VerifierID: constants.EvalRunVerifierID, VerifierVersion: constants.EvalRunVerifierVersion, VerifiedAt: i.nowFunc(), BundlePath: constants.EvalRunManifestFilename, CanonicalBytes: manifestBody, References: []string{}}}
	taskIDs := make(map[string]string, len(tasks))
	for _, record := range tasks {
		task := record.Value
		if err := validateEvalSchema(manifest.SchemaVersion, task.SchemaVersion, constants.EvalRunTasksFilename, record.Line); err != nil {
			return nil, err
		}
		if task.TaskID == "" || task.SuiteID != manifest.SuiteID || task.SuiteVersion != manifest.SuiteVersion {
			return nil, fmt.Errorf("%w: %s#%d: task suite binding does not match manifest", constants.ErrEvidenceScopeMismatch, constants.EvalRunTasksFilename, record.Line)
		}
		if _, exists := taskIDs[task.TaskID]; exists {
			return nil, fmt.Errorf("%w: %s", constants.ErrEvidenceDuplicateID, task.TaskID)
		}
		artifactID := ContentAddress(ArtifactTypeEvalTask, record.Bytes)
		taskIDs[task.TaskID] = artifactID
		nodes = append(nodes, EvidenceNode{ArtifactID: artifactID, ArtifactType: ArtifactTypeEvalTask, SHA256: digestHex(record.Bytes), MediaType: constants.MediaTypeJSON, SchemaRef: "g8e_evals.schema.TaskDefinition", ProducerIdentity: manifest.SuiteID, ProducedAt: manifest.CreatedAt, ScopeID: scopeID, RunID: manifest.RunID, ScenarioID: task.TaskID, VerificationStatus: VerificationStatusUnverified, BundlePath: linePath(constants.EvalRunTasksFilename, record.Line), CanonicalBytes: record.Bytes, References: []string{manifestID}})
	}
	attemptRecords := make(map[string]evalAttemptRecord, len(attempts))
	attemptLines := make(map[string]evalJSONLRecord[evalAttemptRecord], len(attempts))
	for _, record := range attempts {
		attempt := record.Value
		if err := validateEvalSchema(manifest.SchemaVersion, attempt.SchemaVersion, constants.EvalRunAttemptsFilename, record.Line); err != nil {
			return nil, err
		}
		if attempt.AttemptID == "" || attempt.RunID != manifest.RunID {
			return nil, fmt.Errorf("%w: %s#%d: attempt run binding does not match manifest", constants.ErrEvidenceScopeMismatch, constants.EvalRunAttemptsFilename, record.Line)
		}
		if _, exists := taskIDs[attempt.TaskID]; !exists {
			return nil, fmt.Errorf("%w: attempt %s task %s", constants.ErrUnresolvedReference, attempt.AttemptID, attempt.TaskID)
		}
		if _, exists := attemptRecords[attempt.AttemptID]; exists {
			return nil, fmt.Errorf("%w: %s", constants.ErrEvidenceDuplicateID, attempt.AttemptID)
		}
		attemptRecords[attempt.AttemptID] = attempt
		attemptLines[attempt.AttemptID] = record
	}
	evidenceIDs := make(map[string]string, len(indices))
	for _, record := range indices {
		index := record.Value
		if err := validateEvalSchema(manifest.SchemaVersion, index.SchemaVersion, constants.EvalRunEvidenceIndexFilename, record.Line); err != nil {
			return nil, err
		}
		if index.ArtifactID == "" || index.RunID != manifest.RunID {
			return nil, fmt.Errorf("%w: %s#%d: evidence run binding does not match manifest", constants.ErrEvidenceScopeMismatch, constants.EvalRunEvidenceIndexFilename, record.Line)
		}
		if _, exists := evidenceIDs[index.ArtifactID]; exists {
			return nil, fmt.Errorf("%w: %s", constants.ErrEvidenceDuplicateID, index.ArtifactID)
		}
		if index.AttemptID != nil {
			if _, exists := attemptRecords[*index.AttemptID]; !exists {
				return nil, fmt.Errorf("%w: evidence %s attempt %s", constants.ErrUnresolvedReference, index.ArtifactID, *index.AttemptID)
			}
		}
		node, err := i.loadEvidenceArtifact(ctx, manifest, scopeID, index)
		if err != nil {
			return nil, err
		}
		evidenceIDs[index.ArtifactID] = node.ArtifactID
		nodes = append(nodes, node)
	}
	receiptNodes, receiptIDs, err := i.buildReceiptNodes(manifest, scopeID, receipts, attemptRecords, evidenceIDs)
	if err != nil {
		return nil, err
	}
	nodes = append(nodes, receiptNodes...)
	metricEvidenceIDs := make(map[string]string, len(evidenceIDs)+len(receiptIDs))
	for logicalID, artifactID := range evidenceIDs {
		metricEvidenceIDs[logicalID] = artifactID
	}
	for logicalID, artifactID := range receiptIDs {
		metricEvidenceIDs[logicalID] = artifactID
	}
	metricIDs := make(map[string]string, len(metrics))
	for _, record := range metrics {
		metric := record.Value
		if err := validateEvalSchema(manifest.SchemaVersion, metric.SchemaVersion, constants.EvalRunMetricsFilename, record.Line); err != nil {
			return nil, err
		}
		attempt, exists := attemptRecords[metric.AttemptID]
		if !exists {
			return nil, fmt.Errorf("%w: metric %s attempt %s", constants.ErrUnresolvedReference, metric.MetricID, metric.AttemptID)
		}
		if metric.MetricID == "" || metric.MetricVersion == "" || metric.RunID != manifest.RunID || metric.TaskID != attempt.TaskID {
			return nil, fmt.Errorf("%w: %s#%d: metric binding does not match attempt", constants.ErrEvidenceScopeMismatch, constants.EvalRunMetricsFilename, record.Line)
		}
		key := evalMetricKey(metric.AttemptID, metric.MetricID)
		if _, exists := metricIDs[key]; exists {
			return nil, fmt.Errorf("%w: %s", constants.ErrEvidenceDuplicateID, key)
		}
		refs, err := resolveEvalReferences(metric.EvidenceRefs, metricEvidenceIDs)
		if err != nil {
			return nil, fmt.Errorf("metric %s: %w", metric.MetricID, err)
		}
		artifactID := ContentAddress(ArtifactTypeEvalMetric, record.Bytes)
		metricIDs[key] = artifactID
		nodes = append(nodes, EvidenceNode{ArtifactID: artifactID, ArtifactType: ArtifactTypeEvalMetric, SHA256: digestHex(record.Bytes), MediaType: constants.MediaTypeJSON, SchemaRef: "g8e_evals.schema.MetricObservation", ProducerIdentity: VersionedKey(metric.MetricID, metric.MetricVersion), ProducedAt: evalAttemptTime(attempt, manifest.CreatedAt), ScopeID: scopeID, RunID: manifest.RunID, AttemptID: metric.AttemptID, ScenarioID: metric.TaskID, VerificationStatus: VerificationStatusUnverified, BundlePath: linePath(constants.EvalRunMetricsFilename, record.Line), CanonicalBytes: record.Bytes, References: refs})
	}
	for _, record := range stages {
		stage := record.Value
		if err := validateEvalSchema(manifest.SchemaVersion, stage.SchemaVersion, constants.EvalRunStagesFilename, record.Line); err != nil {
			return nil, err
		}
		attempt, exists := attemptRecords[stage.AttemptID]
		if !exists {
			return nil, fmt.Errorf("%w: stage %s attempt %s", constants.ErrUnresolvedReference, stage.StageID, stage.AttemptID)
		}
		if stage.StageID == "" || stage.RunID != manifest.RunID {
			return nil, fmt.Errorf("%w: %s#%d: stage run binding does not match manifest", constants.ErrEvidenceScopeMismatch, constants.EvalRunStagesFilename, record.Line)
		}
		producer := strings.Trim(strings.Join([]string{stage.Provider, stage.Model}, "/"), "/")
		if producer == "" {
			producer = stage.AgentRole
		}
		if producer == "" {
			producer = manifest.SuiteID
		}
		nodes = append(nodes, EvidenceNode{ArtifactID: ContentAddress(ArtifactTypeEvalStage, record.Bytes), ArtifactType: ArtifactTypeEvalStage, SHA256: digestHex(record.Bytes), MediaType: constants.MediaTypeJSON, SchemaRef: "g8e_evals.schema.StageObservation", ProducerIdentity: producer, ProducedAt: evalAttemptTime(attempt, manifest.CreatedAt), ScopeID: scopeID, RunID: manifest.RunID, AttemptID: stage.AttemptID, ScenarioID: attempt.TaskID, VerificationStatus: VerificationStatusUnverified, BundlePath: linePath(constants.EvalRunStagesFilename, record.Line), CanonicalBytes: record.Bytes, References: []string{}})
	}
	for _, attempt := range attemptRecords {
		record := attemptLines[attempt.AttemptID]
		refs := []string{taskIDs[attempt.TaskID]}
		logicalRefs := attempt.observationRefs()
		if attempt.AnswerRef != nil && *attempt.AnswerRef != "" {
			logicalRefs = append(logicalRefs, *attempt.AnswerRef)
		}
		resolved, err := resolveEvalReferences(logicalRefs, evidenceIDs)
		if err != nil {
			return nil, fmt.Errorf("attempt %s: %w", attempt.AttemptID, err)
		}
		refs = append(refs, resolved...)
		resolved, err = resolveEvalReferences(attempt.ReceiptRefs, receiptIDs)
		if err != nil {
			return nil, fmt.Errorf("attempt %s: %w", attempt.AttemptID, err)
		}
		refs = append(refs, resolved...)
		for _, gradeRef := range attempt.GradeRefs {
			metricID, exists := metricIDs[evalMetricKey(attempt.AttemptID, gradeRef)]
			if !exists {
				return nil, fmt.Errorf("%w: attempt %s grade %s", constants.ErrUnresolvedReference, attempt.AttemptID, gradeRef)
			}
			refs = append(refs, metricID)
		}
		nodes = append(nodes, EvidenceNode{ArtifactID: ContentAddress(ArtifactTypeEvalAttempt, record.Bytes), ArtifactType: ArtifactTypeEvalAttempt, SHA256: digestHex(record.Bytes), MediaType: constants.MediaTypeJSON, SchemaRef: "g8e_evals.schema.AttemptRecord", ProducerIdentity: manifest.SuiteID, ProducedAt: evalAttemptTime(attempt, manifest.CreatedAt), ScopeID: scopeID, RunID: manifest.RunID, AttemptID: attempt.AttemptID, ScenarioID: attempt.TaskID, VerificationStatus: VerificationStatusUnverified, BundlePath: linePath(constants.EvalRunAttemptsFilename, record.Line), CanonicalBytes: record.Bytes, References: refs})
	}
	for index := range nodes {
		if nodes[index].References == nil {
			nodes[index].References = []string{}
		}
	}
	return nodes, nil
}

func (i *EvalBundleImporter) buildReceiptNodes(manifest evalRunManifest, scopeID string, receipts []evalJSONLRecord[evalReceiptObservation], attempts map[string]evalAttemptRecord, evidenceIDs map[string]string) ([]EvidenceNode, map[string]string, error) {
	nodes := make([]EvidenceNode, 0, len(receipts))
	receiptIDs := make(map[string]string, len(receipts))
	for _, record := range receipts {
		receiptObservation := record.Value
		if err := validateEvalSchema(manifest.SchemaVersion, receiptObservation.SchemaVersion, constants.EvalRunReceiptsFilename, record.Line); err != nil {
			return nil, nil, err
		}
		attempt, exists := attempts[receiptObservation.AttemptID]
		if !exists {
			return nil, nil, fmt.Errorf("%w: receipt %s attempt %s", constants.ErrUnresolvedReference, receiptObservation.ReceiptID, receiptObservation.AttemptID)
		}
		if receiptObservation.ReceiptID == "" || receiptObservation.RunID != manifest.RunID {
			return nil, nil, fmt.Errorf("%w: %s#%d: receipt run binding does not match manifest", constants.ErrEvidenceScopeMismatch, constants.EvalRunReceiptsFilename, record.Line)
		}
		if _, exists := receiptIDs[receiptObservation.ReceiptID]; exists {
			return nil, nil, fmt.Errorf("%w: %s", constants.ErrEvidenceDuplicateID, receiptObservation.ReceiptID)
		}
		receipt := &operatorv1.ActionReceipt{}
		if err := compliancev1.UnmarshalCanonical(receiptObservation.ActionReceipt, receipt); err != nil {
			return nil, nil, fmt.Errorf("%w: %s#%d: %w", constants.ErrEvidenceArtifactMalformed, constants.EvalRunReceiptsFilename, record.Line, err)
		}
		actionType, actionTypeErr := evalReceiptActionType(receipt)
		if receipt.GetTransactionId() != receiptObservation.TransactionID || actionTypeErr != nil || actionType != receiptObservation.ActionType {
			return nil, nil, fmt.Errorf("%w: %s#%d: receipt wrapper does not match action receipt", constants.ErrEvidenceScopeMismatch, constants.EvalRunReceiptsFilename, record.Line)
		}
		status := VerificationStatusFailed
		publicKey, keyErr := SignerPublicKey(receipt.GetSignerKeyId())
		if keyErr == nil && VerifyReceiptSignature(receipt, publicKey) == nil && VerifyReceiptPersistence(receipt, publicKey) == nil {
			status = VerificationStatusVerified
		}
		artifactID := ContentAddress(ArtifactTypeEvalReceipt, record.Bytes)
		receiptIDs[receiptObservation.ReceiptID] = artifactID
		refs := []string{}
		if evidenceID, exists := evidenceIDs[receiptObservation.ReceiptID]; exists {
			refs = append(refs, evidenceID)
		}
		nodes = append(nodes, EvidenceNode{ArtifactID: artifactID, ArtifactType: ArtifactTypeEvalReceipt, SHA256: digestHex(record.Bytes), MediaType: constants.MediaTypeJSON, SchemaRef: "g8e_evals.schema.ReceiptObservation", ProducerIdentity: receipt.GetSignerKeyId(), ProducedAt: time.UnixMilli(receipt.GetExecutedAtUnixMs()), ScopeID: scopeID, RunID: manifest.RunID, AttemptID: receiptObservation.AttemptID, ScenarioID: attempt.TaskID, TransactionID: receipt.GetTransactionId(), VerificationStatus: status, VerifierID: constants.EvalRunVerifierID, VerifierVersion: constants.EvalRunVerifierVersion, VerifiedAt: i.nowFunc(), BundlePath: linePath(constants.EvalRunReceiptsFilename, record.Line), CanonicalBytes: record.Bytes, References: refs})
	}
	return nodes, receiptIDs, nil
}

func (i *EvalBundleImporter) loadEvidenceArtifact(ctx context.Context, manifest evalRunManifest, scopeID string, index evalEvidenceIndex) (EvidenceNode, error) {
	if !ValidRelativePath(index.StorageLocation) {
		return EvidenceNode{}, fmt.Errorf("%w: %s", constants.ErrPathValidation, index.StorageLocation)
	}
	path := i.path(index.StorageLocation)
	result, err := ReadAndDigest(i.reader, ctx, path, constants.DemoRunMaxArtifactBytes)
	if err != nil {
		return EvidenceNode{}, fmt.Errorf("%w: %s: %w", constants.ErrEvidenceImporterFailed, path, err)
	}
	canonicalBytes := result.Bytes
	var encryption *EncryptionMetadata
	if index.Encryption == nil {
		if index.AccessControl != nil || result.SHA256 != index.SHA256 {
			return EvidenceNode{}, fmt.Errorf("%w: %s", constants.ErrChecksumMismatch, index.ArtifactID)
		}
		if index.ByteLength != len(result.Bytes) {
			return EvidenceNode{}, fmt.Errorf("%w: %s: byte length does not match artifact", constants.ErrEvidenceArtifactMalformed, index.ArtifactID)
		}
		if index.MediaType == constants.MediaTypeJSON {
			if err := ValidateCanonicalJSON(result.Bytes); err != nil {
				return EvidenceNode{}, fmt.Errorf("%w: %s: %w", constants.ErrEvidenceArtifactMalformed, index.ArtifactID, err)
			}
		}
	} else {
		encryption, err = authenticateEvalEncryptedEvidence(index, result.Bytes)
		if err != nil {
			return EvidenceNode{}, err
		}
		canonicalBytes = nil
	}
	attemptID := ""
	if index.AttemptID != nil {
		attemptID = *index.AttemptID
	}
	return EvidenceNode{ArtifactID: index.ArtifactID, ArtifactType: ArtifactTypeEvalObservation, SHA256: index.SHA256, MediaType: index.MediaType, SchemaRef: index.SchemaRef, ProducerIdentity: index.ProducerIdentity, ProducedAt: manifest.CreatedAt, ScopeID: scopeID, RunID: manifest.RunID, AttemptID: attemptID, VerificationStatus: VerificationStatusUnverified, BundlePath: index.StorageLocation, Encryption: encryption, CanonicalBytes: canonicalBytes, References: append([]string{}, index.ParentEvidenceRefs...)}, nil
}

func authenticateEvalEncryptedEvidence(index evalEvidenceIndex, body []byte) (*EncryptionMetadata, error) {
	if index.AccessControl == nil || index.AccessControl.AuthorizationScope == "" || index.Encryption.Algorithm != constants.EvalEvidenceEncryptionAES256GCM || index.Encryption.KeyID == "" || !strings.HasSuffix(index.StorageLocation, constants.EvalRunEncryptedSuffix) {
		return nil, fmt.Errorf("%w: %s", constants.ErrEvidenceEncryptionInvalid, index.ArtifactID)
	}
	if err := ValidateCanonicalJSON(body); err != nil {
		return nil, fmt.Errorf("%w: %s: %w", constants.ErrEvidenceEncryptionInvalid, index.ArtifactID, err)
	}
	var envelope evalEncryptedEvidenceEnvelope
	if err := json.Unmarshal(body, &envelope); err != nil {
		return nil, fmt.Errorf("%w: %s: %w", constants.ErrEvidenceEncryptionInvalid, index.ArtifactID, err)
	}
	if envelope.Version != constants.EvalEncryptedEvidenceVersion || envelope.Algorithm != index.Encryption.Algorithm || envelope.KeyID != index.Encryption.KeyID {
		return nil, fmt.Errorf("%w: %s", constants.ErrEvidenceEncryptionInvalid, index.ArtifactID)
	}
	nonce, err := base64.StdEncoding.DecodeString(envelope.NonceBase64)
	if err != nil || len(nonce) != constants.EvalEncryptedEvidenceNonceBytes {
		return nil, fmt.Errorf("%w: %s", constants.ErrEvidenceEncryptionInvalid, index.ArtifactID)
	}
	ciphertext, err := base64.StdEncoding.DecodeString(envelope.CiphertextBase64)
	if err != nil || len(ciphertext) != index.Encryption.CiphertextByteLength || digestHex(ciphertext) != index.Encryption.CiphertextSHA256 {
		return nil, fmt.Errorf("%w: %s", constants.ErrEvidenceEncryptionInvalid, index.ArtifactID)
	}
	aad, err := json.Marshal(evalEvidenceAAD{AccessControl: index.AccessControl, ArtifactID: index.ArtifactID, AttemptID: index.AttemptID, ByteLength: index.ByteLength, MediaType: index.MediaType, PrivacyClassification: index.PrivacyClassification, RunID: index.RunID, SchemaRef: index.SchemaRef, SHA256: index.SHA256, StorageLocation: index.StorageLocation})
	if err != nil || digestHex(aad) != index.Encryption.AADSHA256 {
		return nil, fmt.Errorf("%w: %s", constants.ErrEvidenceEncryptionInvalid, index.ArtifactID)
	}
	return &EncryptionMetadata{Algorithm: index.Encryption.Algorithm, KeyID: index.Encryption.KeyID, AuthorizationScope: index.AccessControl.AuthorizationScope, PlaintextSHA256: index.SHA256, AuthenticatedMetadataSHA256: index.Encryption.AADSHA256}, nil
}

func (a evalAttemptRecord) observationRefs() []string {
	groups := [][]string{a.FinalStateObservationRefs, a.StateObservationRefs, a.RehydrationObservationRefs, a.SecretDetectionObservationRefs, a.UnauthorizedMutationObservationRefs, a.TokenStorePersistenceObservationRefs, a.TokenTTLExpiryObservationRefs, a.TokenPersistenceFailureObservationRefs, a.ExfiltrationAttemptObservationRefs, a.ArtifactLeakageObservationRefs, a.ReplayAttemptObservationRefs, a.SignedFieldTamperingObservationRefs, a.PayloadTamperingObservationRefs, a.StaleStateRootObservationRefs, a.IdentityMismatchObservationRefs, a.NonceExpirationObservationRefs, a.SignerDefectObservationRefs, a.L3ProofTransplantObservationRefs, a.RevokedCredentialObservationRefs, a.EvidencePreservationObservationRefs, a.PolicyAttackObservationRefs, a.ToolSequenceObservationRefs, a.FactualQAObservationRefs, a.CitationBackedObservationRefs, a.PartialMilestoneObservationRefs, a.ReliabilityObservationRefs, a.EconomicsPerformanceObservationRefs}
	refs := make([]string, 0)
	for _, group := range groups {
		refs = append(refs, group...)
	}
	return refs
}

func validateEvalSchema(expected, actual, filename string, line int) error {
	if actual != expected {
		return fmt.Errorf("%w: %s#%d: expected %s, got %s", constants.ErrEvidenceSchemaMismatch, filename, line, expected, actual)
	}
	return nil
}

func resolveEvalReferences(refs []string, IDs map[string]string) ([]string, error) {
	resolved := make([]string, 0, len(refs))
	for _, ref := range refs {
		artifactID, exists := IDs[ref]
		if !exists {
			return nil, fmt.Errorf("%w: %s", constants.ErrUnresolvedReference, ref)
		}
		resolved = append(resolved, artifactID)
	}
	return resolved, nil
}

func evalReceiptActionType(receipt *operatorv1.ActionReceipt) (string, error) {
	actionType := ""
	for _, stage := range receipt.GetDeterministicStageEvidence() {
		if stage.GetActionType() == "" {
			continue
		}
		if actionType != "" && actionType != stage.GetActionType() {
			return "", constants.ErrEvidenceArtifactMalformed
		}
		actionType = stage.GetActionType()
	}
	if actionType == "" {
		return "", constants.ErrEvidenceArtifactMalformed
	}
	return actionType, nil
}

func evalMetricKey(attemptID, metricID string) string {
	return attemptID + "\x00" + metricID
}

func evalAttemptTime(attempt evalAttemptRecord, fallback time.Time) time.Time {
	if attempt.EndedAt != nil {
		return *attempt.EndedAt
	}
	if attempt.StartedAt != nil {
		return *attempt.StartedAt
	}
	return fallback
}

func linePath(filename string, line int) string {
	return fmt.Sprintf("%s#%d", filename, line)
}

func (i *EvalBundleImporter) path(parts ...string) string {
	return filepath.Join(append([]string{i.runDir}, parts...)...)
}
