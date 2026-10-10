// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package public

import (
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

	"github.com/g8e-ai/g8e/v2/internal/cli/cmd/shared"

	"github.com/spf13/cobra"

	"github.com/g8e-ai/g8e/v2/internal/cli/cmd/eval"
	"github.com/g8e-ai/g8e/v2/internal/cli/cmd/gwremote"
	"github.com/g8e-ai/g8e/v2/internal/cli/config"
	"github.com/g8e-ai/g8e/v2/internal/constants"
	"github.com/g8e-ai/g8e/v2/internal/models"
	"github.com/g8e-ai/g8e/v2/internal/services/evaluation"
	"github.com/g8e-ai/g8e/v2/internal/services/fs"
	"github.com/g8e-ai/g8e/v2/internal/services/gateway"
	"github.com/g8e-ai/g8e/v2/internal/services/keystore"
)

type publicConfigLoader func(string) (*config.Config, error)
type publicFileSvcFactory func(string, *slog.Logger) (fs.RuntimeFileService, error)

// openPublicKeystore opens the keystore that seals the public-feed signing key
// and ingest token. Tests replace it with an in-memory keyring.
var openPublicKeystore = func(fileSvc fs.RuntimeFileService, opts keystore.Options) (*keystore.Keystore, error) {
	return keystore.Open(fileSvc, slog.Default(), opts)
}

// publicKeystore opens the keystore using the public command group's
// --master-key-file flag.
func publicKeystore(cmd *cobra.Command, fileSvc fs.RuntimeFileService) (*keystore.Keystore, error) {
	var opts keystore.Options
	if flag := cmd.Flag("master-key-file"); flag != nil {
		opts.MasterKeyFile = flag.Value.String()
	}
	ks, err := openPublicKeystore(fileSvc, opts)
	if err != nil {
		return nil, fmt.Errorf("public-feed: open keystore: %w", err)
	}
	return ks, nil
}

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
			ctx := shared.CommandContext(cmd)
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
			ks, err := publicKeystore(cmd, fileSvc)
			if err != nil {
				return err
			}
			if err := gateway.WritePublicSecret(ctx, fileSvc, ks, constants.PublicFeedSigningKeyPath, privateKey); err != nil {
				return fmt.Errorf("public-feed: write signing key: %w", err)
			}
			createdPaths = append(createdPaths, constants.PublicFeedSigningKeyPath)
			if err := gateway.WritePublicSecret(ctx, fileSvc, ks, constants.PublicFeedIngestTokenPath, token); err != nil {
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
	var mirrorOrigin string
	var enabled bool
	cmd := &cobra.Command{
		Use:   "set",
		Short: "Update public-feed publisher configuration",
		RunE: func(cmd *cobra.Command, _ []string) error {
			if !cmd.Flags().Changed("mirror-origin") && !cmd.Flags().Changed("enabled") {
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
			ctx := shared.CommandContext(cmd)
			exportConfig, err := readPublicExportConfig(ctx, fileSvc)
			if err != nil {
				return err
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
	cmd.Flags().StringVar(&mirrorOrigin, "mirror-origin", "", "Hosted public mirror origin")
	cmd.Flags().BoolVar(&enabled, "enabled", true, "Enable or disable public publication")
	return cmd
}

func publicSourceTransitionCmdWithConfig(configLoader publicConfigLoader, fileSvcFactory publicFileSvcFactory) *cobra.Command {
	var sourceID string
	var confirmed bool
	cmd := &cobra.Command{
		Use:   "transition",
		Short: "Archive the current publisher state and start a fresh public source",
		RunE: func(cmd *cobra.Command, _ []string) error {
			if !confirmed {
				return constants.ErrPublicFeedSourceTransitionConfirmation
			}
			if strings.TrimSpace(sourceID) == "" {
				return constants.ErrPublicFeedSourceIDRequired
			}
			cfg, err := configLoader("")
			if err != nil {
				return err
			}
			fileSvc, err := fileSvcFactory(cfg.ProjectRoot, slog.Default())
			if err != nil {
				return fmt.Errorf("%w: %w", constants.ErrFileServiceInit, err)
			}
			ks, err := publicKeystore(cmd, fileSvc)
			if err != nil {
				return err
			}
			newConfig, err := gateway.TransitionLocalPublicFeed(shared.CommandContext(cmd), fileSvc, ks, sourceID)
			if err != nil {
				return err
			}
			_, err = fmt.Fprintf(cmd.OutOrStdout(), "Public feed transitioned to source %s; prior publisher state archived under %s\n", newConfig.SourceID, constants.PublicFeedArchiveDirname)
			return err
		},
	}
	cmd.Flags().StringVar(&sourceID, "source-id", "", "New public source deployment pseudonym")
	cmd.Flags().BoolVar(&confirmed, "yes", false, "Confirm archival of the current publisher state and creation of a fresh source")
	return cmd
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

func Cmd() *cobra.Command {
	cmd := &cobra.Command{Use: "public", Short: "Manage the public spectator feed"}
	cmd.PersistentFlags().String("master-key-file", "", "Absolute path to a provisioned base64 32-byte master key outside the runtime root; Linux mode 0400 or 0600. Nonblank flag overrides G8E_MASTER_KEY_FILE; blank uses environment, then OS key store. No fallback on failure.")
	configCmd := &cobra.Command{Use: "config", Short: "Manage public-feed configuration"}
	configCmd.AddCommand(publicConfigSetCmdWithConfig(shared.LoadConfig, shared.NewFileSvc))
	sourceCmd := &cobra.Command{Use: "source", Short: "Manage the active public-feed source"}
	sourceCmd.AddCommand(publicSourceTransitionCmdWithConfig(shared.LoadConfig, shared.NewFileSvc))
	cmd.AddCommand(
		publicInitCmdWithConfig(shared.LoadConfig, shared.NewFileSvc),
		configCmd,
		sourceCmd,
		publicPublishCmdWithConfig(shared.LoadConfig, shared.NewFileSvc),
		publicPushCmdWithConfig(shared.LoadConfig, shared.NewFileSvc),
		publicRepairOutboxCmdWithConfig(shared.LoadConfig, shared.NewFileSvc),
		eval.PublicRestoreCmdWithConfig(shared.LoadConfig, shared.NewFileSvc),
		publicStatusCmdWithConfig(shared.LoadConfig, shared.NewFileSvc),
		publicRotateKeyCmdWithConfig(shared.LoadConfig, shared.NewFileSvc),
		publicVerifyAssignmentCmd(),
	)
	return cmd
}

func publicVerifyAssignmentCmd() *cobra.Command {
	var dbPath string
	var vaultKeyPath string
	cmd := &cobra.Command{
		Use:   "verify-assignment",
		Short: "Verify an assignment audit slice offline",
		RunE: func(cmd *cobra.Command, _ []string) error {
			if strings.TrimSpace(dbPath) == "" || strings.TrimSpace(vaultKeyPath) == "" {
				return constants.ErrMissingRequiredField
			}
			dbBytes, err := os.ReadFile(dbPath)
			if err != nil {
				return fmt.Errorf("public-feed: read assignment audit db: %w", err)
			}
			keyBytes, err := os.ReadFile(vaultKeyPath)
			if err != nil {
				return fmt.Errorf("public-feed: read assignment audit vault key: %w", err)
			}
			if err := evaluation.VerifyAssignmentAuditSlice(dbBytes, keyBytes); err != nil {
				return err
			}
			_, err = fmt.Fprintln(cmd.OutOrStdout(), "Assignment audit slice verified")
			return err
		},
	}
	cmd.Flags().StringVar(&dbPath, "db", "", "Path to the assignment audit SQLite file")
	cmd.Flags().StringVar(&vaultKeyPath, "vault-key", "", "Path to the assignment audit vault key")
	_ = cmd.MarkFlagRequired("db")
	_ = cmd.MarkFlagRequired("vault-key")
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
	exportConfig, err := readPublicExportConfig(shared.CommandContext(cmd), fileSvc)
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
	exists, err := fileSvc.FileExists(shared.CommandContext(cmd), constants.PublicFeedKeyRotationPath)
	if err != nil {
		return nil, models.PublicExportConfig{}, fmt.Errorf("public-feed: inspect key rotation state: %w", err)
	}
	if exists {
		return nil, models.PublicExportConfig{}, constants.ErrPublicFeedKeyRotationPending
	}
	return fileSvc, exportConfig, nil
}

func newPublicPublisherForCommand(cmd *cobra.Command, fileSvc fs.RuntimeFileService, exportConfig models.PublicExportConfig) (*gateway.PublicPublisherService, error) {
	ctx := shared.CommandContext(cmd)
	ks, err := publicKeystore(cmd, fileSvc)
	if err != nil {
		return nil, err
	}
	key, err := gateway.ReadPublicSecret(ctx, fileSvc, ks, constants.PublicFeedSigningKeyPath, ed25519.PrivateKeySize, constants.ErrPublicFeedSigningKeyRequired)
	if err != nil {
		return nil, err
	}
	token, err := gateway.ReadPublicSecret(ctx, fileSvc, ks, constants.PublicFeedIngestTokenPath, constants.PublicFeedIngestTokenBytes, constants.ErrPublicFeedIngestTokenRequired)
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
			cfg, err := configLoader("")
			if err != nil {
				return err
			}
			fileSvc, err := fileSvcFactory(cfg.ProjectRoot, slog.Default())
			if err != nil {
				return fmt.Errorf("%w: %w", constants.ErrFileServiceInit, err)
			}
			ctx := shared.CommandContext(cmd)
			if gwremote.ShouldPublishViaGateway(ctx, fileSvc) {
				count, err := gwremote.PublishJSONLViaGateway(ctx, fileSvc, cfg, args[0])
				if err != nil {
					return err
				}
				_, err = fmt.Fprintf(cmd.OutOrStdout(), "Published %d records through the gateway-owned public mirror\n", count)
				return err
			}
			fileSvc, exportConfig, err := loadPublicCommandRuntime(cmd, configLoader, fileSvcFactory)
			if err != nil {
				return err
			}
			publisher, err := newPublicPublisherForCommand(cmd, fileSvc, exportConfig)
			if err != nil {
				return err
			}
			inputs, err := gwremote.ReadPublicRecordInputs(args[0], exportConfig.BatchMaxRecords, exportConfig.BatchMaxBytes)
			if err != nil {
				return err
			}
			nextSequence := int64(1)
			snapshot, err := publisher.GetSnapshot(shared.CommandContext(cmd))
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
			if err := publisher.ExportBatch(shared.CommandContext(cmd), records); err != nil {
				return err
			}
			_, err = fmt.Fprintf(cmd.OutOrStdout(), "Published %d records through sequence %d\n", len(records), records[len(records)-1].Sequence)
			return err
		},
	}
	return cmd
}

func publicRepairOutboxCmdWithConfig(configLoader publicConfigLoader, fileSvcFactory publicFileSvcFactory) *cobra.Command {
	return &cobra.Command{
		Use:   "repair-outbox",
		Short: "Compact a prefix-pruned public-feed outbox back to the mirror snapshot tip",
		RunE: func(cmd *cobra.Command, _ []string) error {
			cfg, err := configLoader("")
			if err != nil {
				return err
			}
			fileSvc, err := fileSvcFactory(cfg.ProjectRoot, slog.Default())
			if err != nil {
				return fmt.Errorf("%w: %w", constants.ErrFileServiceInit, err)
			}
			ctx := shared.CommandContext(cmd)
			if gwremote.ShouldPublishViaGateway(ctx, fileSvc) {
				_, err = fmt.Fprintln(cmd.OutOrStdout(), "Gateway-owned public mirror outbox is managed by the Gateway")
				return err
			}
			fileSvc, exportConfig, err := loadPublicCommandRuntime(cmd, configLoader, fileSvcFactory)
			if err != nil {
				return err
			}
			publisher, err := newPublicPublisherForCommand(cmd, fileSvc, exportConfig)
			if err != nil {
				return err
			}
			if err := publisher.RepairOutboxFromSnapshot(ctx); err != nil {
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
			cfg, err := configLoader("")
			if err != nil {
				return err
			}
			fileSvc, err := fileSvcFactory(cfg.ProjectRoot, slog.Default())
			if err != nil {
				return fmt.Errorf("%w: %w", constants.ErrFileServiceInit, err)
			}
			ctx := shared.CommandContext(cmd)
			if gwremote.ShouldPublishViaGateway(ctx, fileSvc) {
				_, err = fmt.Fprintln(cmd.OutOrStdout(), "Gateway-owned public mirror is already synchronized")
				return err
			}
			fileSvc, exportConfig, err := loadPublicCommandRuntime(cmd, configLoader, fileSvcFactory)
			if err != nil {
				return err
			}
			publisher, err := newPublicPublisherForCommand(cmd, fileSvc, exportConfig)
			if err != nil {
				return err
			}
			if err := publisher.RetransmitOutbox(shared.CommandContext(cmd)); err != nil {
				return err
			}
			if err := publisher.PushProofPackage(shared.CommandContext(cmd)); err != nil {
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
			cfg, err := configLoader("")
			if err != nil {
				return err
			}
			fileSvc, err := fileSvcFactory(cfg.ProjectRoot, slog.Default())
			if err != nil {
				return fmt.Errorf("%w: %w", constants.ErrFileServiceInit, err)
			}
			ctx := shared.CommandContext(cmd)
			if gwremote.ShouldPublishViaGateway(ctx, fileSvc) {
				status, err := gwremote.GatewayPublisherStatus(ctx, fileSvc, cfg)
				if err != nil {
					return err
				}
				body, err := json.Marshal(status)
				if err != nil {
					return fmt.Errorf("public-feed: encode status: %w", err)
				}
				_, err = fmt.Fprintln(cmd.OutOrStdout(), string(body))
				return err
			}
			fileSvc, exportConfig, err := loadPublicCommandRuntime(cmd, configLoader, fileSvcFactory)
			if err != nil {
				return err
			}
			publisher, err := newPublicPublisherForCommand(cmd, fileSvc, exportConfig)
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
			snapshot, err := publisher.GetSnapshot(shared.CommandContext(cmd))
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

// readPublicKeyRotation loads pending rotation state and unseals its new
// signing key. It returns nil state when no rotation is pending.
func readPublicKeyRotation(ctx context.Context, fileSvc fs.RuntimeFileService, ks *keystore.Keystore) (*models.PublicKeyRotationState, ed25519.PrivateKey, error) {
	exists, err := fileSvc.FileExists(ctx, constants.PublicFeedKeyRotationPath)
	if err != nil {
		return nil, nil, fmt.Errorf("public-feed: inspect key rotation state: %w", err)
	}
	if !exists {
		return nil, nil, nil
	}
	data, err := fileSvc.ReadFile(ctx, constants.PublicFeedKeyRotationPath)
	if err != nil {
		return nil, nil, fmt.Errorf("%w: read state: %v", constants.ErrPublicFeedKeyRotationPending, err)
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	var state models.PublicKeyRotationState
	if err := decoder.Decode(&state); err != nil {
		return nil, nil, fmt.Errorf("%w: decode state: %v", constants.ErrPublicFeedKeyRotationPending, err)
	}
	var trailing json.RawMessage
	if err := decoder.Decode(&trailing); err != io.EOF {
		return nil, nil, fmt.Errorf("%w: trailing JSON", constants.ErrPublicFeedKeyRotationPending)
	}
	privateKeyHex, err := ks.Decrypt(state.SealedNewPrivateKey)
	if err != nil {
		return nil, nil, fmt.Errorf("%w: unseal new signing key: %v", constants.ErrPublicFeedKeyRotationPending, err)
	}
	privateKey, err := hex.DecodeString(privateKeyHex)
	if err != nil || len(privateKey) != ed25519.PrivateKeySize || state.SourceID == "" || state.OldKeyID == "" || state.NewKeyID == "" || state.OldKeyID == state.NewKeyID {
		return nil, nil, constants.ErrPublicFeedKeyRotationPending
	}
	digest := sha256.Sum256(ed25519.PrivateKey(privateKey).Public().(ed25519.PublicKey))
	if state.NewKeyID != hex.EncodeToString(digest[:]) {
		return nil, nil, constants.ErrPublicFeedKeyRotationPending
	}
	return &state, ed25519.PrivateKey(privateKey), nil
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

func finalizePublicKeyRotation(ctx context.Context, fileSvc fs.RuntimeFileService, ks *keystore.Keystore, exportConfig models.PublicExportConfig, rotation models.PublicKeyRotationState, newPrivateKey ed25519.PrivateKey) error {
	if err := gateway.WritePublicSecret(ctx, fileSvc, ks, constants.PublicFeedSigningKeyPath, newPrivateKey); err != nil {
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
			ctx := shared.CommandContext(cmd)
			ks, err := publicKeystore(cmd, fileSvc)
			if err != nil {
				return err
			}
			rotation, newPrivateKey, err := readPublicKeyRotation(ctx, fileSvc, ks)
			if err != nil {
				return err
			}
			if rotation == nil {
				publicKey, privateKey, err := ed25519.GenerateKey(rand.Reader)
				if err != nil {
					return fmt.Errorf("%w: %v", constants.ErrPublicFeedKeyGenFailed, err)
				}
				digest := sha256.Sum256(publicKey)
				sealed, err := ks.Encrypt(hex.EncodeToString(privateKey))
				if err != nil {
					return fmt.Errorf("public-feed: seal new signing key: %w", err)
				}
				rotation = &models.PublicKeyRotationState{
					SourceID:            exportConfig.SourceID,
					OldKeyID:            exportConfig.SigningKeyID,
					NewKeyID:            hex.EncodeToString(digest[:]),
					SealedNewPrivateKey: sealed,
				}
				newPrivateKey = privateKey
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
				publisher, err := newPublicPublisherForCommand(cmd, fileSvc, exportConfig)
				if err != nil {
					return err
				}
				if found {
					err = publisher.RetransmitOutbox(ctx)
				} else {
					err = publisher.RotateKeyTo(ctx, newPrivateKey, rotation.NewKeyID)
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
			if err := finalizePublicKeyRotation(ctx, fileSvc, ks, exportConfig, *rotation, newPrivateKey); err != nil {
				return err
			}
			_, err = fmt.Fprintf(cmd.OutOrStdout(), "Public signing key rotated to %s\n", rotation.NewKeyID)
			return err
		},
	}
}
