package cmd

import (
	"bufio"
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"time"

	"github.com/spf13/cobra"

	"github.com/g8e-ai/g8e/v2/internal/constants"
	"github.com/g8e-ai/g8e/v2/internal/models"
	"github.com/g8e-ai/g8e/v2/internal/services/fs"
	"github.com/g8e-ai/g8e/v2/internal/services/gateway"
)

type publicLoopRunner func(context.Context, string, string) error

type publicLoopCandidate struct {
	SourceTreeHash              string                     `json:"source_tree_hash"`
	ExecutionSourceManifestHash string                     `json:"execution_source_manifest_hash"`
	BinarySHA256                string                     `json:"binary_sha256"`
	Images                      []publicLoopCandidateImage `json:"images"`
	ContentHash                 string                     `json:"content_hash"`
}

type publicLoopCandidateImage struct {
	Components []string `json:"components"`
	ImageID    string   `json:"image_id"`
}

type publicLoopEvidence struct {
	SchemaVersion                      string `json:"schema_version"`
	CandidateContentHash               string `json:"candidate_content_hash"`
	SourceID                           string `json:"source_id"`
	FirstSequence                      int64  `json:"first_sequence"`
	HighWaterSequence                  int64  `json:"high_water_sequence"`
	BatchCount                         int    `json:"batch_count"`
	FeedChainHash                      string `json:"feed_chain_hash"`
	ProofArtifactSHA256                string `json:"proof_artifact_sha256"`
	ProofArtifactBytes                 int    `json:"proof_artifact_bytes"`
	RetryCount                         int    `json:"retry_count"`
	OldKeyID                           string `json:"old_key_id"`
	ReplacementKeyID                   string `json:"replacement_key_id"`
	OldKeyRevoked                      bool   `json:"old_key_revoked"`
	ReplacementKeyAcceptedAfterRestart bool   `json:"replacement_key_accepted_after_restart"`
	MirrorRestartRecovered             bool   `json:"mirror_restart_recovered"`
	InferenceInvocations               int    `json:"inference_invocations"`
	GeneratedAt                        string `json:"generated_at"`
	ContentHash                        string `json:"content_hash"`
}

func publicLoopCmd() *cobra.Command {
	return publicLoopCmdWithRunner(runPublicLoop)
}

func publicLoopCmdWithRunner(runner publicLoopRunner) *cobra.Command {
	var candidatePath string
	var outputPath string
	cmd := &cobra.Command{
		Use:   "public-loop",
		Short: "Run the provider-free public feed qualification loop",
		RunE: func(cmd *cobra.Command, _ []string) error {
			if err := runner(cmd.Context(), candidatePath, outputPath); err != nil {
				return fmt.Errorf("public loop qualification: %w", err)
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&candidatePath, "candidate", "", "Path to the typed candidate identity JSON")
	cmd.Flags().StringVar(&outputPath, "output", "", "Path for the typed public-loop evidence JSON")
	_ = cmd.MarkFlagRequired("candidate")
	_ = cmd.MarkFlagRequired("output")
	return cmd
}

func runPublicLoop(ctx context.Context, candidatePath, outputPath string) error {
	candidate, err := readPublicLoopCandidate(candidatePath)
	if err != nil {
		return err
	}
	tempDir, err := os.MkdirTemp("", constants.PublicLoopTempPrefix)
	if err != nil {
		return fmt.Errorf("create isolated runtime: %w", err)
	}
	defer func() {
		if cleanupErr := os.RemoveAll(tempDir); cleanupErr != nil {
			slog.Error("public loop: remove isolated runtime", "error", cleanupErr)
		}
	}()
	fileSvc, err := fs.NewRuntimeFileService(tempDir, slog.Default())
	if err != nil {
		return fmt.Errorf("create runtime file service: %w", err)
	}
	if err := fileSvc.CreateRuntimeTree(ctx); err != nil {
		return fmt.Errorf("create runtime tree: %w", err)
	}
	oldPublic, oldPrivate, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return fmt.Errorf("generate source key: %w", err)
	}
	oldKeyID := hashBytes(oldPublic)
	tokenBytes := make([]byte, constants.PublicFeedIngestTokenBytes)
	if _, err := rand.Read(tokenBytes); err != nil {
		return fmt.Errorf("generate ingest token: %w", err)
	}
	ingestToken := hex.EncodeToString(tokenBytes)
	store := gateway.NewRuntimePublicMirrorStore(fileSvc)
	mirror, err := gateway.NewPublicMirrorServer(slog.Default(), store)
	if err != nil {
		return err
	}
	mirror.SetIngestAuthToken(ingestToken)
	const sourceID = "phase3-qualified-public-loop"
	if err := mirror.RegisterSourceKey(ctx, sourceID, oldKeyID, oldPublic); err != nil {
		return err
	}
	privateServer := httptest.NewServer(mirror.Handler())
	publicServer := httptest.NewServer(mirror.PublicHandler())
	defer privateServer.Close()
	defer publicServer.Close()
	cfg := models.DefaultPublicExportConfig()
	cfg.Enabled = true
	cfg.SourceID = sourceID
	cfg.SigningKeyID = oldKeyID
	cfg.MirrorOrigin = privateServer.URL
	cfg.RetryMaxAttempts = 1
	publisher := gateway.NewPublicPublisherService(nil, fileSvc, slog.Default(), cfg, oldPrivate, oldKeyID)
	publisher.SetIngestAuthToken(ingestToken)
	if err := publisher.ExportBatch(ctx, []models.PublicFeedRecord{publicLoopRecord(1, "campaign-a")}); err != nil {
		return err
	}
	proofContent := []byte(`{"campaign_id":"campaign-a","verification_status":"passed"}`)
	manifest, err := publisher.BuildProofPackage(ctx, "campaign-a", "revision-a", hashBytes([]byte("verified-index")), true, []gateway.ProofArtifactInput{{
		Filename:   "qualification-proof.json",
		MediaType:  "application/json",
		Content:    proofContent,
		CampaignID: "campaign-a",
	}})
	if err != nil {
		return err
	}
	if err := publisher.PushProofPackage(ctx); err != nil {
		return err
	}
	if len(manifest.Artifacts) != 1 {
		return fmt.Errorf("public proof manifest artifact count mismatch")
	}
	privateServer.Close()
	publicServer.Close()
	if err := publisher.ExportBatch(ctx, []models.PublicFeedRecord{publicLoopRecord(2, "campaign-b")}); err == nil {
		return fmt.Errorf("mirror outage did not retain a retryable batch")
	}
	restartedMirror, err := gateway.NewPublicMirrorServer(slog.Default(), store)
	if err != nil {
		return err
	}
	restartedMirror.SetIngestAuthToken(ingestToken)
	restartedPrivateServer := httptest.NewServer(restartedMirror.Handler())
	restartedPublicServer := httptest.NewServer(restartedMirror.PublicHandler())
	defer restartedPrivateServer.Close()
	defer restartedPublicServer.Close()
	publisher.SetMirrorOrigin(restartedPrivateServer.URL)
	if err := publisher.RetransmitOutbox(ctx); err != nil {
		return err
	}
	newPublic, newPrivate, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return err
	}
	newKeyID := hashBytes(newPublic)
	if err := publisher.RotateKeyTo(ctx, newPrivate, newKeyID); err != nil {
		return err
	}
	restartedPrivateServer.Close()
	restartedPublicServer.Close()
	finalMirror, err := gateway.NewPublicMirrorServer(slog.Default(), store)
	if err != nil {
		return err
	}
	finalMirror.SetIngestAuthToken(ingestToken)
	finalPrivateServer := httptest.NewServer(finalMirror.Handler())
	finalPublicServer := httptest.NewServer(finalMirror.PublicHandler())
	defer finalPrivateServer.Close()
	defer finalPublicServer.Close()
	publisher.SetMirrorOrigin(finalPrivateServer.URL)
	if err := publisher.ExportBatch(ctx, []models.PublicFeedRecord{publicLoopRecord(4, "campaign-c")}); err != nil {
		return err
	}
	if err := verifyPublicLoopReads(ctx, finalPublicServer, sourceID, manifest.Artifacts[0].ImmutableURL); err != nil {
		return err
	}
	if err := verifyPublicLoopMutationRoutesAbsent(ctx, finalPublicServer); err != nil {
		return err
	}
	state, err := store.Load(ctx)
	if err != nil {
		return err
	}
	source := state.Sources[sourceID]
	if source == nil {
		return fmt.Errorf("durable mirror source state missing")
	}
	_, oldRevoked := state.RevokedKeys[sourceID+":"+oldKeyID]
	_, newRegistered := state.KeyRegistry[sourceID+":"+newKeyID]
	evidence := publicLoopEvidence{
		SchemaVersion:                      "1.0.0",
		CandidateContentHash:               candidate.ContentHash,
		SourceID:                           sourceID,
		FirstSequence:                      1,
		HighWaterSequence:                  source.HighWaterSequence,
		BatchCount:                         source.BatchCount,
		FeedChainHash:                      source.FeedChainHash,
		ProofArtifactSHA256:                hashBytes(proofContent),
		ProofArtifactBytes:                 len(proofContent),
		RetryCount:                         1,
		OldKeyID:                           oldKeyID,
		ReplacementKeyID:                   newKeyID,
		OldKeyRevoked:                      oldRevoked,
		ReplacementKeyAcceptedAfterRestart: newRegistered,
		MirrorRestartRecovered:             source.HighWaterSequence == 4,
		InferenceInvocations:               0,
		GeneratedAt:                        time.Now().UTC().Format(time.RFC3339),
	}
	if !evidence.OldKeyRevoked || !evidence.ReplacementKeyAcceptedAfterRestart || !evidence.MirrorRestartRecovered {
		return fmt.Errorf("public loop recovery or key transition incomplete")
	}
	evidence.ContentHash, err = canonicalContentHash(evidence)
	if err != nil {
		return err
	}
	return writePublicLoopEvidence(outputPath, evidence)
}

func readPublicLoopCandidate(path string) (publicLoopCandidate, error) {
	info, err := os.Lstat(path)
	if err != nil {
		return publicLoopCandidate{}, fmt.Errorf("stat candidate identity: %w", err)
	}
	if !info.Mode().IsRegular() {
		return publicLoopCandidate{}, fmt.Errorf("candidate identity is not a regular file")
	}
	content, err := os.ReadFile(path)
	if err != nil {
		return publicLoopCandidate{}, fmt.Errorf("read candidate identity: %w", err)
	}
	var candidate publicLoopCandidate
	decoder := json.NewDecoder(bytesReader(content))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&candidate); err != nil {
		return publicLoopCandidate{}, fmt.Errorf("decode candidate identity: %w", err)
	}
	if err := rejectTrailingJSON(decoder); err != nil {
		return publicLoopCandidate{}, err
	}
	computed, err := canonicalContentHash(candidate)
	if err != nil {
		return publicLoopCandidate{}, err
	}
	if candidate.ContentHash != computed || !isHex64(candidate.SourceTreeHash) || !isHex64(candidate.ExecutionSourceManifestHash) || !isHex64(candidate.BinarySHA256) {
		return publicLoopCandidate{}, fmt.Errorf("candidate identity hash validation failed")
	}
	if len(candidate.Images) != 3 || len(candidate.Images[0].Components) != 2 || candidate.Images[0].Components[0] != "gateway" || candidate.Images[0].Components[1] != "operator" || len(candidate.Images[1].Components) != 1 || candidate.Images[1].Components[0] != "ensemble" || len(candidate.Images[2].Components) != 1 || candidate.Images[2].Components[0] != "dashboard" {
		return publicLoopCandidate{}, fmt.Errorf("candidate image component mapping is invalid")
	}
	for _, image := range candidate.Images {
		if len(image.ImageID) != len("sha256:")+64 || image.ImageID[:len("sha256:")] != "sha256:" || !isHex64(image.ImageID[len("sha256:"):]) {
			return publicLoopCandidate{}, fmt.Errorf("candidate image identity is invalid")
		}
	}
	return candidate, nil
}

func verifyPublicLoopReads(ctx context.Context, server *httptest.Server, sourceID, proofPath string) error {
	paths := []string{
		"/bootstrap?source=" + sourceID,
		"/history?source=" + sourceID + "&cursor=0&limit=100",
		"/snapshot?source=" + sourceID,
		proofPath,
	}
	for _, path := range paths {
		request, err := http.NewRequestWithContext(ctx, http.MethodGet, server.URL+path, nil)
		if err != nil {
			return err
		}
		response, err := server.Client().Do(request)
		if err != nil {
			return err
		}
		_, readErr := io.Copy(io.Discard, response.Body)
		closeErr := response.Body.Close()
		if readErr != nil || closeErr != nil || response.StatusCode != http.StatusOK {
			return fmt.Errorf("public read %s failed with status %d", path, response.StatusCode)
		}
	}
	streamCtx, cancel := context.WithTimeout(ctx, time.Second)
	defer cancel()
	request, err := http.NewRequestWithContext(streamCtx, http.MethodGet, server.URL+"/stream?source="+sourceID+"&since_id=0", nil)
	if err != nil {
		return err
	}
	response, err := server.Client().Do(request)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	line, err := bufio.NewReader(response.Body).ReadString('\n')
	if err != nil || line == "" || response.StatusCode != http.StatusOK {
		return fmt.Errorf("public SSE replay failed")
	}
	return nil
}

func verifyPublicLoopMutationRoutesAbsent(ctx context.Context, server *httptest.Server) error {
	for _, path := range []string{"/ingest", "/keys/register", "/proof-ingest"} {
		request, err := http.NewRequestWithContext(ctx, http.MethodPost, server.URL+path, nil)
		if err != nil {
			return err
		}
		request.Header.Set(constants.HeaderAuthorization, "Bearer qualification-token")
		request.Header.Set("Content-Type", "application/json")
		response, err := server.Client().Do(request)
		if err != nil {
			return err
		}
		_, readErr := io.Copy(io.Discard, response.Body)
		closeErr := response.Body.Close()
		if readErr != nil {
			return fmt.Errorf("read public mutation route %s response: %w", path, readErr)
		}
		if closeErr != nil {
			return fmt.Errorf("close public mutation route %s response: %w", path, closeErr)
		}
		if response.StatusCode != http.StatusNotFound {
			return fmt.Errorf("public mutation route %s returned status %d", path, response.StatusCode)
		}
	}
	return nil
}

func publicLoopRecord(sequence int64, campaignID string) models.PublicFeedRecord {
	content := fmt.Sprintf(`{"campaign_id":%q}`, campaignID)
	return models.PublicFeedRecord{
		Sequence:    sequence,
		RecordType:  models.PublicFeedRecordTypeProjection,
		RecordHash:  hashBytes([]byte(content)),
		RecordBytes: content,
	}
}

func writePublicLoopEvidence(path string, evidence publicLoopEvidence) error {
	content, err := json.MarshalIndent(evidence, "", "  ")
	if err != nil {
		return err
	}
	content = append(content, '\n')
	file, err := os.OpenFile(filepath.Clean(path), os.O_WRONLY|os.O_CREATE|os.O_EXCL, constants.PermFilePrivate)
	if err != nil {
		return fmt.Errorf("create public loop evidence: %w", err)
	}
	if _, err := file.Write(content); err != nil {
		_ = file.Close()
		return fmt.Errorf("write public loop evidence: %w", err)
	}
	if err := file.Close(); err != nil {
		return fmt.Errorf("close public loop evidence: %w", err)
	}
	return nil
}

func canonicalContentHash(value any) (string, error) {
	content, err := json.Marshal(value)
	if err != nil {
		return "", err
	}
	var object map[string]any
	if err := json.Unmarshal(content, &object); err != nil {
		return "", err
	}
	delete(object, "content_hash")
	canonical, err := json.Marshal(object)
	if err != nil {
		return "", err
	}
	return hashBytes(canonical), nil
}

func hashBytes(content []byte) string {
	sum := sha256.Sum256(content)
	return hex.EncodeToString(sum[:])
}

func bytesReader(content []byte) io.Reader {
	return &byteReader{content: content}
}

type byteReader struct {
	content []byte
}

func (r *byteReader) Read(target []byte) (int, error) {
	if len(r.content) == 0 {
		return 0, io.EOF
	}
	count := copy(target, r.content)
	r.content = r.content[count:]
	return count, nil
}

func rejectTrailingJSON(decoder *json.Decoder) error {
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		return fmt.Errorf("candidate identity contains trailing JSON")
	}
	return nil
}
