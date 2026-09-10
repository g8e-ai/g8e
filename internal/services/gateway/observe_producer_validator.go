// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package gateway

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"path/filepath"
	"strings"
	"time"

	"github.com/g8e-ai/g8e/v2/internal/constants"
	"github.com/g8e-ai/g8e/v2/internal/models"
)

// validAgentLifecycleStatuses is the set of recognized agent lifecycle
// statuses. A first write accepts only a member of this set; "no existing
// projection" relaxes the source-state requirement, not target enum
// validation.
var validAgentLifecycleStatuses = map[models.AgentLifecycleStatus]struct{}{
	models.AgentLifecycleStatusIdle:      {},
	models.AgentLifecycleStatusQueued:    {},
	models.AgentLifecycleStatusRunning:   {},
	models.AgentLifecycleStatusWaiting:   {},
	models.AgentLifecycleStatusCompleted: {},
	models.AgentLifecycleStatusFailed:    {},
	models.AgentLifecycleStatusOffline:   {},
}

// validRunLifecycleStatuses is the set of recognized run lifecycle statuses.
var validRunLifecycleStatuses = map[models.RunLifecycleStatus]struct{}{
	models.RunLifecycleStatusQueued:    {},
	models.RunLifecycleStatusRunning:   {},
	models.RunLifecycleStatusWaiting:   {},
	models.RunLifecycleStatusCompleted: {},
	models.RunLifecycleStatusFailed:    {},
	models.RunLifecycleStatusCancelled: {},
}

// validRunKinds is the set of recognized run kinds.
var validRunKinds = map[models.RunKind]struct{}{
	models.RunKindInvestigation: {},
	models.RunKindEval:          {},
	models.RunKindDemo:          {},
	models.RunKindWorkflow:      {},
}

// supportedProducerSchemaVersion is the single schema version accepted at the
// producer boundary. Both agent and run producer requests carry the observe
// event payload schema version.
const supportedProducerSchemaVersion = constants.ObserveEventPayloadSchemaVersion

// validateAgentProducerRequest validates the agent producer payload fields at
// the Gateway boundary. It checks the supported schema version, non-empty
// display name and role, and a recognized agent status even on first write.
// Transition validation is separate and performed by the producer service
// against the persisted projection.
func validateAgentProducerRequest(req models.ObserveProducerAgentStateRequest) error {
	if req.SchemaVersion != supportedProducerSchemaVersion {
		return fmt.Errorf("observe producer: validate agent request: %w: got %q", constants.ErrObserveUnsupportedSchemaVersion, req.SchemaVersion)
	}
	if req.DisplayName == "" {
		return fmt.Errorf("observe producer: validate agent request: %w", constants.ErrObserveAgentDisplayNameRequired)
	}
	if req.Role == "" {
		return fmt.Errorf("observe producer: validate agent request: %w", constants.ErrObserveAgentRoleRequired)
	}
	if _, ok := validAgentLifecycleStatuses[req.Status]; !ok {
		return fmt.Errorf("observe producer: validate agent request: %w: %q", constants.ErrObserveInvalidTransition, req.Status)
	}
	return nil
}

// validateRunProducerRequest validates the run producer payload fields at the
// Gateway boundary. It checks the supported schema version, non-empty display
// name, recognized run kind, recognized run status even on first write,
// non-negative task counters, completed tasks not exceeding total tasks, and
// end time not preceding start time. Transition validation is separate and
// performed by the producer service against the persisted projection.
func validateRunProducerRequest(req models.ObserveProducerRunStateRequest) error {
	if req.SchemaVersion != supportedProducerSchemaVersion {
		return fmt.Errorf("observe producer: validate run request: %w: got %q", constants.ErrObserveUnsupportedSchemaVersion, req.SchemaVersion)
	}
	if req.DisplayName == "" {
		return fmt.Errorf("observe producer: validate run request: %w", constants.ErrObserveRunDisplayNameRequired)
	}
	if _, ok := validRunKinds[req.RunKind]; !ok {
		return fmt.Errorf("observe producer: validate run request: %w: %q", constants.ErrObserveRunKindRequired, req.RunKind)
	}
	if _, ok := validRunLifecycleStatuses[req.Status]; !ok {
		return fmt.Errorf("observe producer: validate run request: %w: %q", constants.ErrObserveInvalidTransition, req.Status)
	}
	if req.CompletedTasks < 0 || req.TotalTasks < 0 {
		return fmt.Errorf("observe producer: validate run request: %w: completed=%d total=%d", constants.ErrObserveNegativeTaskCount, req.CompletedTasks, req.TotalTasks)
	}
	if req.CompletedTasks > req.TotalTasks {
		return fmt.Errorf("observe producer: validate run request: %w: completed=%d total=%d", constants.ErrObserveCompletedExceedsTotal, req.CompletedTasks, req.TotalTasks)
	}
	if req.StartedAt != nil && req.EndedAt != nil && req.EndedAt.Before(*req.StartedAt) {
		return fmt.Errorf("observe producer: validate run request: %w: started=%s ended=%s", constants.ErrObserveEndBeforeStart, req.StartedAt.Format(time.RFC3339Nano), req.EndedAt.Format(time.RFC3339Nano))
	}
	return nil
}

// supportedPublicationSchemaVersion is the single schema version accepted at
// the eval publication boundary.
const supportedPublicationSchemaVersion = constants.ObservePublicationSchemaVersion

// bundlePrivacyClassPublic is the privacy class value in the bundle manifest
// that marks an artifact as publicly disclosable. It mirrors the Python
// PrivacyClass.PUBLIC = "public" value from
// ensemble/evals/g8e_evals/bundle/manifest.py.
const bundlePrivacyClassPublic = "public"

// validateEvalPublicationRequest validates the eval publication payload
// fields at the Gateway boundary. It checks the supported schema version,
// non-empty bundle_id and run_id, a verified verification report (ok=true),
// at least one public artifact in the bundle manifest, rooted relative
// artifact paths, non-empty media types, 64-char hex SHA-256 hashes,
// non-negative byte sizes, no duplicate download artifact IDs, and no
// restricted artifacts in the download catalog. Content hash and size
// verification against the base64-encoded bytes is performed by the producer
// service when persisting the artifact.
func validateEvalPublicationRequest(req models.ObserveProducerEvalPublicationRequest) error {
	if req.SchemaVersion != supportedPublicationSchemaVersion {
		return fmt.Errorf("observe producer: validate eval publication: %w: got %q", constants.ErrObserveUnsupportedSchemaVersion, req.SchemaVersion)
	}
	if req.BundleID == "" {
		return fmt.Errorf("observe producer: validate eval publication: %w", constants.ErrObservePublicationBundleIDRequired)
	}
	if req.RunID == "" {
		return fmt.Errorf("observe producer: validate eval publication: %w", constants.ErrObservePublicationRunIDRequired)
	}
	if !req.VerificationReport.OK {
		return fmt.Errorf("observe producer: validate eval publication: %w", constants.ErrObservePublicationNotVerified)
	}
	if err := validateBundleManifestWire(req.BundleManifest); err != nil {
		return err
	}
	if err := validateDownloadCatalog(req.Downloads); err != nil {
		return err
	}
	return nil
}

// validateBundleManifestWire validates the bundle manifest fields: at least
// one public artifact exists and all artifact paths are rooted relative (no
// absolute paths, no ".." segments, no backslashes).
func validateBundleManifestWire(manifest models.BundleManifestWire) error {
	hasPublic := false
	for _, art := range manifest.Artifacts {
		if art.PrivacyClass == bundlePrivacyClassPublic {
			hasPublic = true
		}
		if err := validateRootedRelativePath(art.Path); err != nil {
			return fmt.Errorf("observe producer: validate eval publication: artifact path %q: %w", art.Path, err)
		}
	}
	if !hasPublic {
		return fmt.Errorf("observe producer: validate eval publication: %w", constants.ErrObservePublicationNoPublicArtifacts)
	}
	return nil
}

// validateDownloadCatalog validates the download artifact entries: each has a
// non-empty artifact_id and filename, non-empty media_type, 64-char hex
// sha256, non-negative byte_size, no restricted artifacts, and no duplicate
// artifact IDs.
func validateDownloadCatalog(downloads []models.ObserveProducerDownloadArtifactInput) error {
	seen := make(map[string]struct{}, len(downloads))
	for _, dl := range downloads {
		if dl.ArtifactID == "" {
			return fmt.Errorf("observe producer: validate eval publication: %w", constants.ErrObservePublicationArtifactIDRequired)
		}
		if dl.Filename == "" {
			return fmt.Errorf("observe producer: validate eval publication: %w", constants.ErrObservePublicationFilenameRequired)
		}
		if dl.MediaType == "" {
			return fmt.Errorf("observe producer: validate eval publication: %w", constants.ErrObservePublicationMediaTypeRequired)
		}
		if !isValidSHA256Hex(dl.SHA256) {
			return fmt.Errorf("observe producer: validate eval publication: %w: %q", constants.ErrObservePublicationSHA256Invalid, dl.SHA256)
		}
		if dl.ByteSize < 0 {
			return fmt.Errorf("observe producer: validate eval publication: %w: %d", constants.ErrObservePublicationByteSizeNegative, dl.ByteSize)
		}
		if dl.PrivacyClassification != models.DownloadPrivacyPublicSafe {
			return fmt.Errorf("observe producer: validate eval publication: %w: %q", constants.ErrObservePublicationRestrictedArtifact, dl.PrivacyClassification)
		}
		if err := validateRootedRelativePath(dl.ArtifactID); err != nil {
			return fmt.Errorf("observe producer: validate eval publication: artifact_id %q: %w", dl.ArtifactID, err)
		}
		if _, dup := seen[dl.ArtifactID]; dup {
			return fmt.Errorf("observe producer: validate eval publication: %w: %q", constants.ErrObservePublicationDuplicateArtifactID, dl.ArtifactID)
		}
		seen[dl.ArtifactID] = struct{}{}
	}
	return nil
}

// validateRootedRelativePath returns an error if the path is empty, absolute,
// contains a ".." segment, or contains a backslash. A valid rooted relative
// path stays within the bundle root.
func validateRootedRelativePath(path string) error {
	if path == "" {
		return fmt.Errorf("empty path: %w", constants.ErrInvalidJSONBody)
	}
	if strings.Contains(path, "\\") {
		return fmt.Errorf("backslash in path: %w", constants.ErrInvalidJSONBody)
	}
	if filepath.IsAbs(path) {
		return fmt.Errorf("absolute path: %w", constants.ErrInvalidJSONBody)
	}
	for _, seg := range strings.Split(path, "/") {
		if seg == ".." {
			return fmt.Errorf("traversal segment: %w", constants.ErrInvalidJSONBody)
		}
	}
	return nil
}

// isValidSHA256Hex returns true if s is a 64-character lowercase hexadecimal
// string.
func isValidSHA256Hex(s string) bool {
	if len(s) != 64 {
		return false
	}
	for _, c := range s {
		if !((c >= '0' && c <= '9') || (c >= 'a' && c <= 'f')) {
			return false
		}
	}
	return true
}

// decodeProducerRequest decodes a producer request body with strict JSON
// semantics: unknown fields and trailing JSON after the top-level value are
// rejected. It returns a wrapped constants.ErrInvalidJSONBody so the
// controller maps the failure to a single 400 response without hand-rolled
// error text.
func decodeProducerRequest(body []byte, dst any) error {
	dec := json.NewDecoder(bytes.NewReader(body))
	dec.DisallowUnknownFields()
	if err := dec.Decode(dst); err != nil {
		return fmt.Errorf("observe producer: decode request: %w", constants.ErrInvalidJSONBody)
	}
	// Reject trailing JSON after the top-level value.
	if err := dec.Decode(&struct{}{}); err != io.EOF {
		return fmt.Errorf("observe producer: decode request: %w: trailing content", constants.ErrInvalidJSONBody)
	}
	return nil
}
