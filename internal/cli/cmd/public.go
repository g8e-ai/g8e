package cmd

import (
	"bufio"
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"strings"

	"github.com/spf13/cobra"

	"github.com/g8e-ai/g8e/v2/internal/cli/config"
	"github.com/g8e-ai/g8e/v2/internal/constants"
	"github.com/g8e-ai/g8e/v2/internal/models"
	"github.com/g8e-ai/g8e/v2/internal/services/fs"
	"github.com/g8e-ai/g8e/v2/internal/services/gateway"
)

type publicConfigLoader func(string) (*config.Config, error)
type publicFileSvcFactory func(string, *slog.Logger) (fs.RuntimeFileService, error)

func publicInitCmdWithConfig(configLoader publicConfigLoader, fileSvcFactory publicFileSvcFactory) *cobra.Command {
	var sourceID string
	var mirrorOrigin string
	cmd := &cobra.Command{
		Use:   "init",
		Short: "Initialize the local public-feed publisher",
		RunE: func(cmd *cobra.Command, _ []string) error {
			if strings.TrimSpace(sourceID) == "" {
				return constants.ErrPublicFeedSourceIDRequired
			}
			if err := validatePublicMirrorOrigin(mirrorOrigin); err != nil {
				return err
			}
			cfg, err := configLoader("")
			if err != nil {
				return err
			}
			fileSvc, err := fileSvcFactory(cfg.ProjectRoot, slog.Default())
			if err != nil {
				return fmt.Errorf("%w: %w", constants.ErrFileServiceInit, err)
			}
			ctx := commandContext(cmd)
			for _, relPath := range []string{constants.PublicFeedExportConfigPath, constants.PublicFeedSigningKeyPath, constants.PublicFeedIngestTokenPath} {
				exists, existsErr := fileSvc.FileExists(ctx, relPath)
				if existsErr != nil {
					return fmt.Errorf("public-feed: inspect initialization path: %w", existsErr)
				}
				if exists {
					return constants.ErrPublicFeedConfigExists
				}
			}
			if err := fileSvc.MkdirAll(ctx, constants.PublicFeedDirname, constants.PermDirPrivate); err != nil {
				return fmt.Errorf("public-feed: create runtime directory: %w", err)
			}
			createdPaths := make([]string, 0, 3)
			rollback := func(cause error) error {
				result := cause
				for index := len(createdPaths) - 1; index >= 0; index-- {
					if removeErr := fileSvc.Remove(ctx, createdPaths[index]); removeErr != nil {
						result = errors.Join(result, fmt.Errorf("public-feed: roll back initialization path %s: %w", createdPaths[index], removeErr))
					}
				}
				return result
			}
			publicKey, privateKey, err := ed25519.GenerateKey(rand.Reader)
			if err != nil {
				return fmt.Errorf("%w: %v", constants.ErrPublicFeedKeyGenFailed, err)
			}
			token := make([]byte, constants.PublicFeedIngestTokenBytes)
			if _, err := rand.Read(token); err != nil {
				return fmt.Errorf("public-feed: generate ingest token: %w", err)
			}
			keyDigest := sha256.Sum256(publicKey)
			exportConfig := models.DefaultPublicExportConfig()
			exportConfig.Enabled = true
			exportConfig.SourceID = sourceID
			exportConfig.MirrorOrigin = mirrorOrigin
			exportConfig.SigningKeyID = hex.EncodeToString(keyDigest[:])
			if err := fileSvc.WriteFile(ctx, constants.PublicFeedSigningKeyPath, []byte(hex.EncodeToString(privateKey)), constants.PermFilePrivate); err != nil {
				return fmt.Errorf("public-feed: write signing key: %w", err)
			}
			createdPaths = append(createdPaths, constants.PublicFeedSigningKeyPath)
			if err := fileSvc.WriteFile(ctx, constants.PublicFeedIngestTokenPath, []byte(hex.EncodeToString(token)), constants.PermFilePrivate); err != nil {
				return rollback(fmt.Errorf("public-feed: write ingest token: %w", err))
			}
			createdPaths = append(createdPaths, constants.PublicFeedIngestTokenPath)
			if err := writePublicExportConfig(ctx, fileSvc, exportConfig); err != nil {
				return rollback(err)
			}
			createdPaths = append(createdPaths, constants.PublicFeedExportConfigPath)
			_, err = fmt.Fprintf(cmd.OutOrStdout(), "Public feed initialized for source %s\n", sourceID)
			return err
		},
	}
	cmd.Flags().StringVar(&sourceID, "source-id", "", "Public source deployment pseudonym")
	cmd.Flags().StringVar(&mirrorOrigin, "mirror-origin", "", "Hosted public mirror origin")
	return cmd
}

func publicConfigSetCmdWithConfig(configLoader publicConfigLoader, fileSvcFactory publicFileSvcFactory) *cobra.Command {
	var sourceID string
	var mirrorOrigin string
	var enabled bool
	cmd := &cobra.Command{
		Use:   "set",
		Short: "Update public-feed publisher configuration",
		RunE: func(cmd *cobra.Command, _ []string) error {
			if !cmd.Flags().Changed("source-id") && !cmd.Flags().Changed("mirror-origin") && !cmd.Flags().Changed("enabled") {
				return fmt.Errorf("%w: no configuration field was specified", constants.ErrValidationFailed)
			}
			cfg, err := configLoader("")
			if err != nil {
				return err
			}
			fileSvc, err := fileSvcFactory(cfg.ProjectRoot, slog.Default())
			if err != nil {
				return fmt.Errorf("%w: %w", constants.ErrFileServiceInit, err)
			}
			ctx := commandContext(cmd)
			exportConfig, err := readPublicExportConfig(ctx, fileSvc)
			if err != nil {
				return err
			}
			if cmd.Flags().Changed("source-id") {
				exportConfig.SourceID = sourceID
			}
			if cmd.Flags().Changed("mirror-origin") {
				exportConfig.MirrorOrigin = mirrorOrigin
			}
			if cmd.Flags().Changed("enabled") {
				exportConfig.Enabled = enabled
			}
			if err := validatePublicExportConfig(exportConfig); err != nil {
				return err
			}
			if err := writePublicExportConfig(ctx, fileSvc, exportConfig); err != nil {
				return err
			}
			_, err = fmt.Fprintln(cmd.OutOrStdout(), "Public feed configuration updated")
			return err
		},
	}
	cmd.Flags().StringVar(&sourceID, "source-id", "", "Public source deployment pseudonym")
	cmd.Flags().StringVar(&mirrorOrigin, "mirror-origin", "", "Hosted public mirror origin")
	cmd.Flags().BoolVar(&enabled, "enabled", true, "Enable or disable public publication")
	return cmd
}

func commandContext(cmd *cobra.Command) context.Context {
	if cmd.Context() != nil {
		return cmd.Context()
	}
	return context.Background()
}

func readPublicExportConfig(ctx context.Context, fileSvc fs.RuntimeFileService) (models.PublicExportConfig, error) {
	data, err := fileSvc.ReadFile(ctx, constants.PublicFeedExportConfigPath)
	if err != nil {
		return models.PublicExportConfig{}, fmt.Errorf("%w: %v", constants.ErrPublicFeedConfigRequired, err)
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	var exportConfig models.PublicExportConfig
	if err := decoder.Decode(&exportConfig); err != nil {
		return models.PublicExportConfig{}, fmt.Errorf("%w: decode: %v", constants.ErrPublicFeedConfigRequired, err)
	}
	if err := rejectTrailingPublicJSON(decoder); err != nil {
		return models.PublicExportConfig{}, err
	}
	if err := validatePublicExportConfig(exportConfig); err != nil {
		return models.PublicExportConfig{}, err
	}
	return exportConfig, nil
}

func writePublicExportConfig(ctx context.Context, fileSvc fs.RuntimeFileService, exportConfig models.PublicExportConfig) error {
	if err := validatePublicExportConfig(exportConfig); err != nil {
		return err
	}
	data, err := json.MarshalIndent(exportConfig, "", "  ")
	if err != nil {
		return fmt.Errorf("public-feed: encode export config: %w", err)
	}
	data = append(data, '\n')
	if err := fileSvc.WriteFile(ctx, constants.PublicFeedExportConfigPath, data, constants.PermFilePrivate); err != nil {
		return fmt.Errorf("public-feed: write export config: %w", err)
	}
	return nil
}

func validatePublicExportConfig(exportConfig models.PublicExportConfig) error {
	if strings.TrimSpace(exportConfig.SourceID) == "" {
		return constants.ErrPublicFeedSourceIDRequired
	}
	if strings.TrimSpace(exportConfig.SigningKeyID) == "" {
		return constants.ErrPublicFeedSigningKeyIDRequired
	}
	if exportConfig.BatchMaxRecords <= 0 || exportConfig.BatchMaxRecords > constants.PublicFeedBatchMaxRecords || exportConfig.BatchMaxBytes <= 0 || exportConfig.BatchMaxBytes > constants.PublicFeedBatchMaxBytes || exportConfig.RetryMaxAttempts <= 0 || exportConfig.RetryInitialBackoffSecs < 0 || exportConfig.RetryMaxBackoffSecs < exportConfig.RetryInitialBackoffSecs || exportConfig.AckWindowSecs <= 0 {
		return constants.ErrPublicFeedConfigRequired
	}
	return validatePublicMirrorOrigin(exportConfig.MirrorOrigin)
}

func validatePublicMirrorOrigin(origin string) error {
	return gateway.ValidatePublicMirrorOrigin(origin)
}

func rejectTrailingPublicJSON(decoder *json.Decoder) error {
	var trailing json.RawMessage
	if err := decoder.Decode(&trailing); err != io.EOF {
		return fmt.Errorf("%w: trailing JSON", constants.ErrPublicFeedConfigRequired)
	}
	return nil
}

func publicCmd() *cobra.Command {
	cmd := &cobra.Command{Use: "public", Short: "Manage the public spectator feed"}
	configCmd := &cobra.Command{Use: "config", Short: "Manage public-feed configuration"}
	configCmd.AddCommand(publicConfigSetCmdWithConfig(loadConfig, newFileSvc))
	cmd.AddCommand(
		publicInitCmdWithConfig(loadConfig, newFileSvc),
		configCmd,
		publicPublishCmdWithConfig(loadConfig, newFileSvc),
		publicPushCmdWithConfig(loadConfig, newFileSvc),
		publicRepairOutboxCmdWithConfig(loadConfig, newFileSvc),
		publicStatusCmdWithConfig(loadConfig, newFileSvc),
		publicRotateKeyCmdWithConfig(loadConfig, newFileSvc),
	)
	return cmd
}

func loadPublicCommandRuntimeUnchecked(cmd *cobra.Command, configLoader publicConfigLoader, fileSvcFactory publicFileSvcFactory) (fs.RuntimeFileService, models.PublicExportConfig, error) {
	cfg, err := configLoader("")
	if err != nil {
		return nil, models.PublicExportConfig{}, err
	}
	fileSvc, err := fileSvcFactory(cfg.ProjectRoot, slog.Default())
	if err != nil {
		return nil, models.PublicExportConfig{}, fmt.Errorf("%w: %w", constants.ErrFileServiceInit, err)
	}
	exportConfig, err := readPublicExportConfig(commandContext(cmd), fileSvc)
	if err != nil {
		return nil, models.PublicExportConfig{}, err
	}
	return fileSvc, exportConfig, nil
}

func loadPublicCommandRuntime(cmd *cobra.Command, configLoader publicConfigLoader, fileSvcFactory publicFileSvcFactory) (fs.RuntimeFileService, models.PublicExportConfig, error) {
	fileSvc, exportConfig, err := loadPublicCommandRuntimeUnchecked(cmd, configLoader, fileSvcFactory)
	if err != nil {
		return nil, models.PublicExportConfig{}, err
	}
	exists, err := fileSvc.FileExists(commandContext(cmd), constants.PublicFeedKeyRotationPath)
	if err != nil {
		return nil, models.PublicExportConfig{}, fmt.Errorf("public-feed: inspect key rotation state: %w", err)
	}
	if exists {
		return nil, models.PublicExportConfig{}, constants.ErrPublicFeedKeyRotationPending
	}
	return fileSvc, exportConfig, nil
}

func readPublicSecret(ctx context.Context, fileSvc fs.RuntimeFileService, relPath string, expectedBytes int, missingErr error) ([]byte, error) {
	data, err := fileSvc.ReadFile(ctx, relPath)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", missingErr, err)
	}
	decoded, err := hex.DecodeString(strings.TrimSpace(string(data)))
	if err != nil || len(decoded) != expectedBytes {
		return nil, missingErr
	}
	return decoded, nil
}

func newPublicPublisherForCommand(ctx context.Context, fileSvc fs.RuntimeFileService, exportConfig models.PublicExportConfig) (*gateway.PublicPublisherService, error) {
	key, err := readPublicSecret(ctx, fileSvc, constants.PublicFeedSigningKeyPath, ed25519.PrivateKeySize, constants.ErrPublicFeedSigningKeyRequired)
	if err != nil {
		return nil, err
	}
	token, err := readPublicSecret(ctx, fileSvc, constants.PublicFeedIngestTokenPath, constants.PublicFeedIngestTokenBytes, constants.ErrPublicFeedIngestTokenRequired)
	if err != nil {
		return nil, err
	}
	publisher := gateway.NewPublicPublisherService(nil, fileSvc, slog.Default(), exportConfig, ed25519.PrivateKey(key), exportConfig.SigningKeyID)
	publisher.SetIngestAuthToken(hex.EncodeToString(token))
	return publisher, nil
}

func publicPublishCmdWithConfig(configLoader publicConfigLoader, fileSvcFactory publicFileSvcFactory) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "publish <records.jsonl>",
		Short: "Publish public-safe records to the configured mirror",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			fileSvc, exportConfig, err := loadPublicCommandRuntime(cmd, configLoader, fileSvcFactory)
			if err != nil {
				return err
			}
			publisher, err := newPublicPublisherForCommand(commandContext(cmd), fileSvc, exportConfig)
			if err != nil {
				return err
			}
			inputs, err := readPublicRecordInputs(args[0], exportConfig.BatchMaxRecords, exportConfig.BatchMaxBytes)
			if err != nil {
				return err
			}
			nextSequence := int64(1)
			snapshot, err := publisher.GetSnapshot(commandContext(cmd))
			if err == nil {
				nextSequence = snapshot.HighWaterSequence + 1
			} else if !errors.Is(err, constants.ErrPublicFeedSnapshotNotFound) {
				return err
			}
			records := make([]models.PublicFeedRecord, len(inputs))
			for index, input := range inputs {
				digest := sha256.Sum256([]byte(input.RecordBytes))
				records[index] = models.PublicFeedRecord{
					Sequence:    nextSequence + int64(index),
					RecordType:  input.RecordType,
					RecordHash:  hex.EncodeToString(digest[:]),
					RecordBytes: input.RecordBytes,
				}
			}
			if err := publisher.ExportBatch(commandContext(cmd), records); err != nil {
				return err
			}
			_, err = fmt.Fprintf(cmd.OutOrStdout(), "Published %d records through sequence %d\n", len(records), records[len(records)-1].Sequence)
			return err
		},
	}
	return cmd
}

func readPublicRecordInputs(filename string, maxRecords, maxBytes int) ([]models.PublicFeedRecordInput, error) {
	file, err := os.Open(filename)
	if err != nil {
		return nil, fmt.Errorf("public-feed: open record input: %w", err)
	}
	scanner := bufio.NewScanner(file)
	scanner.Buffer(make([]byte, 64*1024), maxBytes)
	inputs := make([]models.PublicFeedRecordInput, 0)
	for scanner.Scan() {
		if len(bytes.TrimSpace(scanner.Bytes())) == 0 {
			continue
		}
		var input models.PublicFeedRecordInput
		decoder := json.NewDecoder(bytes.NewReader(scanner.Bytes()))
		decoder.DisallowUnknownFields()
		if err := decoder.Decode(&input); err != nil {
			_ = file.Close()
			return nil, fmt.Errorf("public-feed: decode record input: %w", err)
		}
		inputs = append(inputs, input)
		if len(inputs) > maxRecords {
			_ = file.Close()
			return nil, constants.ErrPublicFeedBatchOversized
		}
	}
	if err := scanner.Err(); err != nil {
		_ = file.Close()
		return nil, fmt.Errorf("public-feed: scan record input: %w", err)
	}
	if err := file.Close(); err != nil {
		return nil, fmt.Errorf("public-feed: close record input: %w", err)
	}
	if len(inputs) == 0 {
		return nil, constants.ErrPublicFeedBatchEmpty
	}
	return inputs, nil
}

func publicRepairOutboxCmdWithConfig(configLoader publicConfigLoader, fileSvcFactory publicFileSvcFactory) *cobra.Command {
	return &cobra.Command{
		Use:   "repair-outbox",
		Short: "Compact a prefix-pruned public-feed outbox back to the mirror snapshot tip",
		RunE: func(cmd *cobra.Command, _ []string) error {
			fileSvc, exportConfig, err := loadPublicCommandRuntime(cmd, configLoader, fileSvcFactory)
			if err != nil {
				return err
			}
			publisher, err := newPublicPublisherForCommand(commandContext(cmd), fileSvc, exportConfig)
			if err != nil {
				return err
			}
			if err := publisher.RepairOutboxFromSnapshot(commandContext(cmd)); err != nil {
				return err
			}
			_, err = fmt.Fprintln(cmd.OutOrStdout(), "Public outbox repaired against snapshot")
			return err
		},
	}
}

func publicPushCmdWithConfig(configLoader publicConfigLoader, fileSvcFactory publicFileSvcFactory) *cobra.Command {
	return &cobra.Command{
		Use:   "push",
		Short: "Retry the durable public-feed outbox",
		RunE: func(cmd *cobra.Command, _ []string) error {
			fileSvc, exportConfig, err := loadPublicCommandRuntime(cmd, configLoader, fileSvcFactory)
			if err != nil {
				return err
			}
			publisher, err := newPublicPublisherForCommand(commandContext(cmd), fileSvc, exportConfig)
			if err != nil {
				return err
			}
			if err := publisher.RetransmitOutbox(commandContext(cmd)); err != nil {
				return err
			}
			if err := publisher.PushProofPackage(commandContext(cmd)); err != nil {
				return err
			}
			_, err = fmt.Fprintln(cmd.OutOrStdout(), "Public outbox and proof package delivered")
			return err
		},
	}
}

func publicStatusCmdWithConfig(configLoader publicConfigLoader, fileSvcFactory publicFileSvcFactory) *cobra.Command {
	return &cobra.Command{
		Use:   "status",
		Short: "Show public-feed publication state",
		RunE: func(cmd *cobra.Command, _ []string) error {
			fileSvc, exportConfig, err := loadPublicCommandRuntime(cmd, configLoader, fileSvcFactory)
			if err != nil {
				return err
			}
			publisher, err := newPublicPublisherForCommand(commandContext(cmd), fileSvc, exportConfig)
			if err != nil {
				return err
			}
			status := models.PublicPublisherStatus{
				Enabled:       exportConfig.Enabled,
				MirrorOrigin:  exportConfig.MirrorOrigin,
				SourceID:      exportConfig.SourceID,
				SigningKeyID:  exportConfig.SigningKeyID,
				FeedChainHash: constants.PublicFeedZeroHashHex,
			}
			snapshot, err := publisher.GetSnapshot(commandContext(cmd))
			if err == nil {
				status.HighWaterSequence = snapshot.HighWaterSequence
				status.FeedChainHash = snapshot.FeedChainHash
				status.BatchCount = snapshot.BatchCount
			} else if !errors.Is(err, constants.ErrPublicFeedSnapshotNotFound) {
				return err
			}
			body, err := json.Marshal(status)
			if err != nil {
				return fmt.Errorf("public-feed: encode status: %w", err)
			}
			_, err = fmt.Fprintln(cmd.OutOrStdout(), string(body))
			return err
		},
	}
}

func readPublicKeyRotation(ctx context.Context, fileSvc fs.RuntimeFileService) (*models.PublicKeyRotationState, error) {
	exists, err := fileSvc.FileExists(ctx, constants.PublicFeedKeyRotationPath)
	if err != nil {
		return nil, fmt.Errorf("public-feed: inspect key rotation state: %w", err)
	}
	if !exists {
		return nil, nil
	}
	data, err := fileSvc.ReadFile(ctx, constants.PublicFeedKeyRotationPath)
	if err != nil {
		return nil, fmt.Errorf("%w: read state: %v", constants.ErrPublicFeedKeyRotationPending, err)
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	var state models.PublicKeyRotationState
	if err := decoder.Decode(&state); err != nil {
		return nil, fmt.Errorf("%w: decode state: %v", constants.ErrPublicFeedKeyRotationPending, err)
	}
	var trailing json.RawMessage
	if err := decoder.Decode(&trailing); err != io.EOF {
		return nil, fmt.Errorf("%w: trailing JSON", constants.ErrPublicFeedKeyRotationPending)
	}
	privateKey, err := hex.DecodeString(state.NewPrivateKey)
	if err != nil || len(privateKey) != ed25519.PrivateKeySize || state.SourceID == "" || state.OldKeyID == "" || state.NewKeyID == "" || state.OldKeyID == state.NewKeyID {
		return nil, constants.ErrPublicFeedKeyRotationPending
	}
	digest := sha256.Sum256(ed25519.PrivateKey(privateKey).Public().(ed25519.PublicKey))
	if state.NewKeyID != hex.EncodeToString(digest[:]) {
		return nil, constants.ErrPublicFeedKeyRotationPending
	}
	return &state, nil
}

func writePublicKeyRotation(ctx context.Context, fileSvc fs.RuntimeFileService, state models.PublicKeyRotationState) error {
	data, err := json.Marshal(state)
	if err != nil {
		return fmt.Errorf("public-feed: encode key rotation state: %w", err)
	}
	if err := fileSvc.WriteFile(ctx, constants.PublicFeedKeyRotationPath, data, constants.PermFilePrivate); err != nil {
		return fmt.Errorf("public-feed: write key rotation state: %w", err)
	}
	return nil
}

func publicKeyRotationOutboxStatus(ctx context.Context, fileSvc fs.RuntimeFileService, rotation models.PublicKeyRotationState) (bool, bool, error) {
	exists, err := fileSvc.FileExists(ctx, constants.PublicFeedOutboxPath)
	if err != nil {
		return false, false, fmt.Errorf("public-feed: inspect rotation outbox: %w", err)
	}
	if !exists {
		return false, false, nil
	}
	data, err := fileSvc.ReadFile(ctx, constants.PublicFeedOutboxPath)
	if err != nil {
		return false, false, fmt.Errorf("public-feed: read rotation outbox: %w", err)
	}
	for lineNumber, line := range bytes.Split(data, []byte{'\n'}) {
		if len(bytes.TrimSpace(line)) == 0 {
			continue
		}
		var entry models.PublicOutboxEntry
		if err := json.Unmarshal(line, &entry); err != nil {
			return false, false, fmt.Errorf("%w: rotation outbox line %d: %v", constants.ErrPublicFeedOutboxCorrupt, lineNumber+1, err)
		}
		var batch models.PublicFeedBatch
		if err := json.Unmarshal([]byte(entry.BatchBytes), &batch); err != nil {
			return false, false, fmt.Errorf("%w: rotation batch line %d: %v", constants.ErrPublicFeedOutboxCorrupt, lineNumber+1, err)
		}
		for _, record := range batch.Records {
			if record.RecordType != models.PublicFeedRecordTypeKeyRevocation {
				continue
			}
			var revocation models.PublicKeyRevocationRecord
			if err := json.Unmarshal([]byte(record.RecordBytes), &revocation); err != nil {
				return false, false, fmt.Errorf("%w: rotation record line %d: %v", constants.ErrPublicFeedOutboxCorrupt, lineNumber+1, err)
			}
			if revocation.SourceID == rotation.SourceID && revocation.RevokedKeyID == rotation.OldKeyID && revocation.NewKeyID == rotation.NewKeyID {
				return true, entry.Status == models.PublicFeedOutboxStatusAcknowledged, nil
			}
		}
	}
	return false, false, nil
}

func finalizePublicKeyRotation(ctx context.Context, fileSvc fs.RuntimeFileService, exportConfig models.PublicExportConfig, rotation models.PublicKeyRotationState) error {
	if err := fileSvc.WriteFile(ctx, constants.PublicFeedSigningKeyPath, []byte(rotation.NewPrivateKey), constants.PermFilePrivate); err != nil {
		return fmt.Errorf("public-feed: persist rotated signing key: %w", err)
	}
	exportConfig.SigningKeyID = rotation.NewKeyID
	if err := writePublicExportConfig(ctx, fileSvc, exportConfig); err != nil {
		return err
	}
	if err := fileSvc.Remove(ctx, constants.PublicFeedKeyRotationPath); err != nil {
		return fmt.Errorf("public-feed: remove completed key rotation state: %w", err)
	}
	return nil
}

func publicRotateKeyCmdWithConfig(configLoader publicConfigLoader, fileSvcFactory publicFileSvcFactory) *cobra.Command {
	return &cobra.Command{
		Use:   "rotate-key",
		Short: "Rotate the public-feed signing key",
		RunE: func(cmd *cobra.Command, _ []string) error {
			fileSvc, exportConfig, err := loadPublicCommandRuntimeUnchecked(cmd, configLoader, fileSvcFactory)
			if err != nil {
				return err
			}
			ctx := commandContext(cmd)
			rotation, err := readPublicKeyRotation(ctx, fileSvc)
			if err != nil {
				return err
			}
			if rotation == nil {
				publicKey, privateKey, err := ed25519.GenerateKey(rand.Reader)
				if err != nil {
					return fmt.Errorf("%w: %v", constants.ErrPublicFeedKeyGenFailed, err)
				}
				digest := sha256.Sum256(publicKey)
				rotation = &models.PublicKeyRotationState{
					SourceID:      exportConfig.SourceID,
					OldKeyID:      exportConfig.SigningKeyID,
					NewKeyID:      hex.EncodeToString(digest[:]),
					NewPrivateKey: hex.EncodeToString(privateKey),
				}
				if err := writePublicKeyRotation(ctx, fileSvc, *rotation); err != nil {
					return err
				}
			}
			if rotation.SourceID != exportConfig.SourceID || (exportConfig.SigningKeyID != rotation.OldKeyID && exportConfig.SigningKeyID != rotation.NewKeyID) {
				return constants.ErrPublicFeedKeyRotationPending
			}
			found, acknowledged, err := publicKeyRotationOutboxStatus(ctx, fileSvc, *rotation)
			if err != nil {
				return err
			}
			if !acknowledged {
				if exportConfig.SigningKeyID != rotation.OldKeyID {
					return constants.ErrPublicFeedKeyRotationPending
				}
				publisher, err := newPublicPublisherForCommand(ctx, fileSvc, exportConfig)
				if err != nil {
					return err
				}
				if found {
					err = publisher.RetransmitOutbox(ctx)
				} else {
					privateKey, _ := hex.DecodeString(rotation.NewPrivateKey)
					err = publisher.RotateKeyTo(ctx, ed25519.PrivateKey(privateKey), rotation.NewKeyID)
				}
				if err != nil {
					return err
				}
				_, acknowledged, err = publicKeyRotationOutboxStatus(ctx, fileSvc, *rotation)
				if err != nil {
					return err
				}
				if !acknowledged {
					return constants.ErrPublicFeedKeyRotationPending
				}
			}
			if err := finalizePublicKeyRotation(ctx, fileSvc, exportConfig, *rotation); err != nil {
				return err
			}
			_, err = fmt.Fprintf(cmd.OutOrStdout(), "Public signing key rotated to %s\n", rotation.NewKeyID)
			return err
		},
	}
}

