// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package gateway

import (
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/g8e-ai/g8e/v2/internal/constants"
	"github.com/g8e-ai/g8e/v2/internal/models"
)

// validAgentReq returns a fully valid agent producer request for mutation in
// table-driven tests.
func validAgentReq() models.ObserveProducerAgentStateRequest {
	return models.ObserveProducerAgentStateRequest{
		SchemaVersion: constants.ObserveEventPayloadSchemaVersion,
		AgentID:       "agent-1",
		DisplayName:   "Triage",
		Role:          "triage",
		Status:        models.AgentLifecycleStatusRunning,
		ObservedAt:    time.Date(2026, 9, 9, 12, 0, 0, 0, time.UTC),
		WebSessionID:  "web-1",
	}
}

// validRunReq returns a fully valid run producer request for mutation in
// table-driven tests.
func validRunReq() models.ObserveProducerRunStateRequest {
	return models.ObserveProducerRunStateRequest{
		SchemaVersion:  constants.ObserveEventPayloadSchemaVersion,
		RunID:          "run-1",
		RunKind:        models.RunKindInvestigation,
		DisplayName:    "Investigation",
		Status:         models.RunLifecycleStatusRunning,
		CompletedTasks: 1,
		TotalTasks:     5,
		ObservedAt:     time.Date(2026, 9, 9, 12, 0, 0, 0, time.UTC),
		WebSessionID:   "web-1",
	}
}

func TestValidateAgentProducerRequest_AcceptsEveryValidStatus(t *testing.T) {
	for _, status := range []models.AgentLifecycleStatus{
		models.AgentLifecycleStatusIdle, models.AgentLifecycleStatusQueued,
		models.AgentLifecycleStatusRunning, models.AgentLifecycleStatusWaiting,
		models.AgentLifecycleStatusCompleted, models.AgentLifecycleStatusFailed,
		models.AgentLifecycleStatusOffline,
	} {
		req := validAgentReq()
		req.Status = status
		assert.NoError(t, validateAgentProducerRequest(req),
			"valid status %s should be accepted", status)
	}
}

func TestValidateAgentProducerRequest_RejectsUnsupportedSchemaVersion(t *testing.T) {
	req := validAgentReq()
	req.SchemaVersion = "9.9.9"
	err := validateAgentProducerRequest(req)
	require.Error(t, err)
	assert.True(t, errors.Is(err, constants.ErrObserveUnsupportedSchemaVersion))
}

func TestValidateAgentProducerRequest_RejectsEmptyDisplayName(t *testing.T) {
	req := validAgentReq()
	req.DisplayName = ""
	err := validateAgentProducerRequest(req)
	require.Error(t, err)
	assert.True(t, errors.Is(err, constants.ErrObserveAgentDisplayNameRequired))
}

func TestValidateAgentProducerRequest_RejectsEmptyRole(t *testing.T) {
	req := validAgentReq()
	req.Role = ""
	err := validateAgentProducerRequest(req)
	require.Error(t, err)
	assert.True(t, errors.Is(err, constants.ErrObserveAgentRoleRequired))
}

func TestValidateAgentProducerRequest_RejectsUnknownStatusEvenOnFirstWrite(t *testing.T) {
	req := validAgentReq()
	req.Status = models.AgentLifecycleStatus("bogus")
	err := validateAgentProducerRequest(req)
	require.Error(t, err)
	assert.True(t, errors.Is(err, constants.ErrObserveInvalidTransition),
		"unknown first-write status must be rejected, not accepted as 'no existing projection'")
}

func TestValidateRunProducerRequest_AcceptsEveryValidRunKindAndStatus(t *testing.T) {
	for _, kind := range []models.RunKind{
		models.RunKindInvestigation, models.RunKindEval,
		models.RunKindDemo, models.RunKindWorkflow,
	} {
		for _, status := range []models.RunLifecycleStatus{
			models.RunLifecycleStatusQueued, models.RunLifecycleStatusRunning,
			models.RunLifecycleStatusWaiting, models.RunLifecycleStatusCompleted,
			models.RunLifecycleStatusFailed, models.RunLifecycleStatusCancelled,
		} {
			req := validRunReq()
			req.RunKind = kind
			req.Status = status
			assert.NoError(t, validateRunProducerRequest(req),
				"valid kind=%s status=%s should be accepted", kind, status)
		}
	}
}

func TestValidateRunProducerRequest_AcceptsValidZeroCounters(t *testing.T) {
	req := validRunReq()
	req.CompletedTasks = 0
	req.TotalTasks = 0
	assert.NoError(t, validateRunProducerRequest(req),
		"zero counters are valid")
}

func TestValidateRunProducerRequest_RejectsUnsupportedSchemaVersion(t *testing.T) {
	req := validRunReq()
	req.SchemaVersion = "0.0.1"
	err := validateRunProducerRequest(req)
	require.Error(t, err)
	assert.True(t, errors.Is(err, constants.ErrObserveUnsupportedSchemaVersion))
}

func TestValidateRunProducerRequest_RejectsEmptyDisplayName(t *testing.T) {
	req := validRunReq()
	req.DisplayName = ""
	err := validateRunProducerRequest(req)
	require.Error(t, err)
	assert.True(t, errors.Is(err, constants.ErrObserveRunDisplayNameRequired))
}

func TestValidateRunProducerRequest_RejectsUnknownRunKind(t *testing.T) {
	req := validRunReq()
	req.RunKind = models.RunKind("bogus")
	err := validateRunProducerRequest(req)
	require.Error(t, err)
	assert.True(t, errors.Is(err, constants.ErrObserveRunKindRequired))
}

func TestValidateRunProducerRequest_RejectsUnknownStatusEvenOnFirstWrite(t *testing.T) {
	req := validRunReq()
	req.Status = models.RunLifecycleStatus("bogus")
	err := validateRunProducerRequest(req)
	require.Error(t, err)
	assert.True(t, errors.Is(err, constants.ErrObserveInvalidTransition),
		"unknown first-write status must be rejected")
}

func TestValidateRunProducerRequest_RejectsNegativeCompletedTasks(t *testing.T) {
	req := validRunReq()
	req.CompletedTasks = -1
	err := validateRunProducerRequest(req)
	require.Error(t, err)
	assert.True(t, errors.Is(err, constants.ErrObserveNegativeTaskCount))
}

func TestValidateRunProducerRequest_RejectsNegativeTotalTasks(t *testing.T) {
	req := validRunReq()
	req.TotalTasks = -1
	err := validateRunProducerRequest(req)
	require.Error(t, err)
	assert.True(t, errors.Is(err, constants.ErrObserveNegativeTaskCount))
}

func TestValidateRunProducerRequest_RejectsCompletedExceedsTotal(t *testing.T) {
	req := validRunReq()
	req.CompletedTasks = 5
	req.TotalTasks = 3
	err := validateRunProducerRequest(req)
	require.Error(t, err)
	assert.True(t, errors.Is(err, constants.ErrObserveCompletedExceedsTotal))
}

func TestValidateRunProducerRequest_RejectsEndedBeforeStarted(t *testing.T) {
	started := time.Date(2026, 9, 9, 12, 0, 0, 0, time.UTC)
	ended := started.Add(-1 * time.Minute)
	req := validRunReq()
	req.StartedAt = &started
	req.EndedAt = &ended
	err := validateRunProducerRequest(req)
	require.Error(t, err)
	assert.True(t, errors.Is(err, constants.ErrObserveEndBeforeStart))
}

func TestValidateRunProducerRequest_AcceptsEndedEqualToStarted(t *testing.T) {
	ts := time.Date(2026, 9, 9, 12, 0, 0, 0, time.UTC)
	req := validRunReq()
	req.StartedAt = &ts
	req.EndedAt = &ts
	assert.NoError(t, validateRunProducerRequest(req),
		"ended_at equal to started_at is valid (not strictly before)")
}

func TestValidateRunProducerRequest_AcceptsStartedOnly(t *testing.T) {
	started := time.Date(2026, 9, 9, 12, 0, 0, 0, time.UTC)
	req := validRunReq()
	req.StartedAt = &started
	assert.NoError(t, validateRunProducerRequest(req),
		"started_at without ended_at is valid")
}

func TestValidateRunProducerRequest_AcceptsEndedOnly(t *testing.T) {
	ended := time.Date(2026, 9, 9, 12, 0, 0, 0, time.UTC)
	req := validRunReq()
	req.EndedAt = &ended
	assert.NoError(t, validateRunProducerRequest(req),
		"ended_at without started_at is valid")
}

// TestDecodeProducerRequest_RejectsUnknownFields asserts the strict decode
// helper rejects unknown fields in the request body.
func TestDecodeProducerRequest_RejectsUnknownFields(t *testing.T) {
	body := []byte(`{"schema_version":"1.0.0","agent_id":"a","display_name":"A","role":"r","status":"running","observed_at":"2026-09-09T12:00:00Z","web_session_id":"w","evil":"no"}`)
	var dst models.ObserveProducerAgentStateRequest
	err := decodeProducerRequest(body, &dst)
	require.Error(t, err)
	assert.True(t, errors.Is(err, constants.ErrInvalidJSONBody))
}

// TestDecodeProducerRequest_RejectsTrailingJSON asserts the strict decode
// helper rejects trailing JSON after the top-level value.
func TestDecodeProducerRequest_RejectsTrailingJSON(t *testing.T) {
	body := []byte(`{"schema_version":"1.0.0","agent_id":"a","display_name":"A","role":"r","status":"running","observed_at":"2026-09-09T12:00:00Z","web_session_id":"w"}{"extra":1}`)
	var dst models.ObserveProducerAgentStateRequest
	err := decodeProducerRequest(body, &dst)
	require.Error(t, err)
	assert.True(t, errors.Is(err, constants.ErrInvalidJSONBody))
}

// TestDecodeProducerRequest_RejectsMalformedJSON asserts the strict decode
// helper rejects malformed JSON.
func TestDecodeProducerRequest_RejectsMalformedJSON(t *testing.T) {
	body := []byte(`{not json}`)
	var dst models.ObserveProducerAgentStateRequest
	err := decodeProducerRequest(body, &dst)
	require.Error(t, err)
	assert.True(t, errors.Is(err, constants.ErrInvalidJSONBody))
}

// TestDecodeProducerRequest_AcceptsValidJSON asserts the strict decode
// helper accepts valid JSON with no unknown fields or trailing content.
func TestDecodeProducerRequest_AcceptsValidJSON(t *testing.T) {
	body := []byte(`{"schema_version":"1.0.0","agent_id":"a","display_name":"A","role":"r","status":"running","observed_at":"2026-09-09T12:00:00Z","web_session_id":"w"}`)
	var dst models.ObserveProducerAgentStateRequest
	err := decodeProducerRequest(body, &dst)
	require.NoError(t, err)
	assert.Equal(t, "a", dst.AgentID)
	assert.Equal(t, "w", dst.WebSessionID)
}

// validEvalPublicationReq returns a fully valid eval publication request for
// mutation in table-driven tests.
func validEvalPublicationReq() models.ObserveProducerEvalPublicationRequest {
	return models.ObserveProducerEvalPublicationRequest{
		SchemaVersion:    constants.ObservePublicationSchemaVersion,
		BundleID:         "bundle-001",
		RunID:            "run-001",
		ReleaseVersion:   "2.1.8",
		SuiteID:          "suite-001",
		SuiteVersion:     "1.0.0",
		ArmID:            "arm-001",
		ReceiptCount:     10,
		AssignedTasks:    5,
		TerminalAttempts: 5,
		Metrics: []models.EvalMetricSummary{
			{
				SchemaVersion:      constants.ObserveEventPayloadSchemaVersion,
				MetricID:           "metric-001",
				MetricVersion:      "1.0.0",
				Unit:               "count",
				Eligible:           5,
				Denominator:        5,
				VerificationStatus: models.EvalVerificationVerified,
			},
		},
		VerificationReport: models.VerificationReportWire{
			SchemaVersion:  constants.VerificationReportSchemaVersion,
			BundleID:       "bundle-001",
			RunID:          "run-001",
			ReleaseVersion: "2.1.8",
			VerifiedAt:     time.Date(2026, 9, 9, 12, 0, 0, 0, time.UTC),
			OK:             true,
			Layers: []models.LayerResultWire{
				{Layer: 1, Passed: true, FailureCount: 0},
			},
		},
		BundleManifest: models.BundleManifestWire{
			SchemaVersion: constants.BundleManifestSchemaVersion,
			BundleID:      "bundle-001",
			RunID:         "run-001",
			Artifacts: []models.BundleArtifactEntryWire{
				{
					Path:         "analysis/analysis.json",
					MediaType:    "application/json",
					PrivacyClass: "public",
					SHA256:       "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855",
					ByteLength:   42,
					ArtifactType: "analysis",
				},
			},
		},
		Downloads: []models.ObserveProducerDownloadArtifactInput{
			{
				ArtifactID:            "analysis-json",
				Filename:              "analysis.json",
				MediaType:             "application/json",
				ByteSize:              42,
				SHA256:                "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855",
				PrivacyClassification: models.DownloadPrivacyPublicSafe,
				SourceRunID:           "run-001",
				Content:               "e30=",
			},
		},
		WebSessionID: "web-1",
	}
}

func TestValidateEvalPublicationRequest_AcceptsValidRequest(t *testing.T) {
	req := validEvalPublicationReq()
	assert.NoError(t, validateEvalPublicationRequest(req))
}

func TestValidateEvalPublicationRequest_RejectsUnsupportedSchemaVersion(t *testing.T) {
	req := validEvalPublicationReq()
	req.SchemaVersion = "9.9.9"
	err := validateEvalPublicationRequest(req)
	require.Error(t, err)
	assert.True(t, errors.Is(err, constants.ErrObserveUnsupportedSchemaVersion))
}

func TestValidateEvalPublicationRequest_RejectsEmptyBundleID(t *testing.T) {
	req := validEvalPublicationReq()
	req.BundleID = ""
	err := validateEvalPublicationRequest(req)
	require.Error(t, err)
	assert.True(t, errors.Is(err, constants.ErrObservePublicationBundleIDRequired))
}

func TestValidateEvalPublicationRequest_RejectsEmptyRunID(t *testing.T) {
	req := validEvalPublicationReq()
	req.RunID = ""
	err := validateEvalPublicationRequest(req)
	require.Error(t, err)
	assert.True(t, errors.Is(err, constants.ErrObservePublicationRunIDRequired))
}

func TestValidateEvalPublicationRequest_RejectsNotVerifiedReport(t *testing.T) {
	req := validEvalPublicationReq()
	req.VerificationReport.OK = false
	err := validateEvalPublicationRequest(req)
	require.Error(t, err)
	assert.True(t, errors.Is(err, constants.ErrObservePublicationNotVerified))
}

func TestValidateEvalPublicationRequest_RejectsNoPublicArtifacts(t *testing.T) {
	req := validEvalPublicationReq()
	req.BundleManifest.Artifacts[0].PrivacyClass = "restricted"
	err := validateEvalPublicationRequest(req)
	require.Error(t, err)
	assert.True(t, errors.Is(err, constants.ErrObservePublicationNoPublicArtifacts))
}

func TestValidateEvalPublicationRequest_RejectsAbsoluteArtifactPath(t *testing.T) {
	req := validEvalPublicationReq()
	req.BundleManifest.Artifacts[0].Path = "/etc/passwd"
	err := validateEvalPublicationRequest(req)
	require.Error(t, err)
}

func TestValidateEvalPublicationRequest_RejectsTraversalArtifactPath(t *testing.T) {
	req := validEvalPublicationReq()
	req.BundleManifest.Artifacts[0].Path = "../escape.json"
	err := validateEvalPublicationRequest(req)
	require.Error(t, err)
}

func TestValidateEvalPublicationRequest_RejectsBackslashArtifactPath(t *testing.T) {
	req := validEvalPublicationReq()
	req.BundleManifest.Artifacts[0].Path = `analysis\analysis.json`
	err := validateEvalPublicationRequest(req)
	require.Error(t, err)
}

func TestValidateEvalPublicationRequest_AcceptsNestedRelativeArtifactPath(t *testing.T) {
	req := validEvalPublicationReq()
	req.BundleManifest.Artifacts[0].Path = "reports/sub/report.json"
	assert.NoError(t, validateEvalPublicationRequest(req))
}

func TestValidateEvalPublicationRequest_RejectsEmptyDownloadArtifactID(t *testing.T) {
	req := validEvalPublicationReq()
	req.Downloads[0].ArtifactID = ""
	err := validateEvalPublicationRequest(req)
	require.Error(t, err)
	assert.True(t, errors.Is(err, constants.ErrObservePublicationArtifactIDRequired))
}

func TestValidateEvalPublicationRequest_RejectsEmptyDownloadFilename(t *testing.T) {
	req := validEvalPublicationReq()
	req.Downloads[0].Filename = ""
	err := validateEvalPublicationRequest(req)
	require.Error(t, err)
	assert.True(t, errors.Is(err, constants.ErrObservePublicationFilenameRequired))
}

func TestValidateEvalPublicationRequest_RejectsEmptyDownloadMediaType(t *testing.T) {
	req := validEvalPublicationReq()
	req.Downloads[0].MediaType = ""
	err := validateEvalPublicationRequest(req)
	require.Error(t, err)
	assert.True(t, errors.Is(err, constants.ErrObservePublicationMediaTypeRequired))
}

func TestValidateEvalPublicationRequest_RejectsInvalidSHA256(t *testing.T) {
	req := validEvalPublicationReq()
	req.Downloads[0].SHA256 = "not-hex"
	err := validateEvalPublicationRequest(req)
	require.Error(t, err)
	assert.True(t, errors.Is(err, constants.ErrObservePublicationSHA256Invalid))
}

func TestValidateEvalPublicationRequest_RejectsUppercaseSHA256(t *testing.T) {
	req := validEvalPublicationReq()
	req.Downloads[0].SHA256 = "E3B0C44298FC1C149AFBF4C8996FB92427AE41E4649B934CA495991B7852B855"
	err := validateEvalPublicationRequest(req)
	require.Error(t, err)
	assert.True(t, errors.Is(err, constants.ErrObservePublicationSHA256Invalid))
}

func TestValidateEvalPublicationRequest_RejectsShortSHA256(t *testing.T) {
	req := validEvalPublicationReq()
	req.Downloads[0].SHA256 = "abc123"
	err := validateEvalPublicationRequest(req)
	require.Error(t, err)
	assert.True(t, errors.Is(err, constants.ErrObservePublicationSHA256Invalid))
}

func TestValidateEvalPublicationRequest_RejectsNegativeByteSize(t *testing.T) {
	req := validEvalPublicationReq()
	req.Downloads[0].ByteSize = -1
	err := validateEvalPublicationRequest(req)
	require.Error(t, err)
	assert.True(t, errors.Is(err, constants.ErrObservePublicationByteSizeNegative))
}

func TestValidateEvalPublicationRequest_AcceptsZeroByteSize(t *testing.T) {
	req := validEvalPublicationReq()
	req.Downloads[0].ByteSize = 0
	assert.NoError(t, validateEvalPublicationRequest(req))
}

func TestValidateEvalPublicationRequest_RejectsRestrictedDownload(t *testing.T) {
	req := validEvalPublicationReq()
	req.Downloads[0].PrivacyClassification = models.DownloadPrivacyRestricted
	err := validateEvalPublicationRequest(req)
	require.Error(t, err)
	assert.True(t, errors.Is(err, constants.ErrObservePublicationRestrictedArtifact))
}

func TestValidateEvalPublicationRequest_RejectsDuplicateArtifactID(t *testing.T) {
	req := validEvalPublicationReq()
	req.Downloads = append(req.Downloads, req.Downloads[0])
	err := validateEvalPublicationRequest(req)
	require.Error(t, err)
	assert.True(t, errors.Is(err, constants.ErrObservePublicationDuplicateArtifactID))
}

func TestValidateEvalPublicationRequest_RejectsTraversalArtifactID(t *testing.T) {
	req := validEvalPublicationReq()
	req.Downloads[0].ArtifactID = "../escape"
	err := validateEvalPublicationRequest(req)
	require.Error(t, err)
}

func TestValidateEvalPublicationRequest_AcceptsMultiplePublicArtifacts(t *testing.T) {
	req := validEvalPublicationReq()
	req.BundleManifest.Artifacts = append(req.BundleManifest.Artifacts, models.BundleArtifactEntryWire{
		Path:         "reports/report.md",
		MediaType:    "text/markdown",
		PrivacyClass: "public",
		SHA256:       "abcdef0123456789abcdef0123456789abcdef0123456789abcdef0123456789",
		ByteLength:   100,
		ArtifactType: "report",
	})
	req.Downloads = append(req.Downloads, models.ObserveProducerDownloadArtifactInput{
		ArtifactID:            "report-md",
		Filename:              "report.md",
		MediaType:             "text/markdown",
		ByteSize:              100,
		SHA256:                "abcdef0123456789abcdef0123456789abcdef0123456789abcdef0123456789",
		PrivacyClassification: models.DownloadPrivacyPublicSafe,
		SourceRunID:           "run-001",
		Content:               "e30=",
	})
	assert.NoError(t, validateEvalPublicationRequest(req))
}

func TestValidateEvalPublicationRequest_AcceptsEmptyDownloadCatalog(t *testing.T) {
	req := validEvalPublicationReq()
	req.Downloads = nil
	assert.NoError(t, validateEvalPublicationRequest(req),
		"an empty download catalog is valid; the manifest still has public artifacts")
}

func TestValidateEvalPublicationRequest_AcceptsInternalArtifactInManifest(t *testing.T) {
	req := validEvalPublicationReq()
	req.BundleManifest.Artifacts = append(req.BundleManifest.Artifacts, models.BundleArtifactEntryWire{
		Path:         "internal/log.txt",
		MediaType:    "text/plain",
		PrivacyClass: "internal",
		SHA256:       "abcdef0123456789abcdef0123456789abcdef0123456789abcdef0123456789",
		ByteLength:   10,
		ArtifactType: "log",
	})
	assert.NoError(t, validateEvalPublicationRequest(req),
		"internal artifacts in the manifest are valid; only the download catalog rejects restricted")
}
