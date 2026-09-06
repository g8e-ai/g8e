// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package cmd

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log/slog"
	"path/filepath"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/g8e-ai/g8e/v2/internal/constants"
	"github.com/g8e-ai/g8e/v2/internal/pathutil"
	"github.com/g8e-ai/g8e/v2/internal/services/compliance"
	"github.com/g8e-ai/g8e/v2/internal/services/fs"
	"github.com/g8e-ai/g8e/v2/internal/services/sqliteutil"
	"github.com/g8e-ai/g8e/v2/internal/services/storage"
	"github.com/g8e-ai/g8e/v2/internal/services/vault"
)

func complianceCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "compliance",
		Short: "FedRAMP 20x KSI evaluation and history",
		Long: `FedRAMP 20x Key Security Indicator (KSI) evaluation, historical
snapshots, and COSAiS overlay inspection. Evaluates g8e's live state against
CR26 KSIs and persists KSI evaluation snapshots for historical metrics.`,
	}

	cmd.AddCommand(
		complianceKSICmdWithConfig(newFileSvc),
		complianceKSIHistoryCmdWithConfig(newFileSvc),
		complianceOverlayCmdWithConfig(newFileSvc),
		complianceDemoRunCmd(),
		complianceReleaseEvidenceCmdWithConfig(newFileSvc, defaultProvenanceSourceFactory),
		complianceEvidenceGraphCmd(),
	)

	return cmd
}

// complianceKSICmdWithConfig creates the `compliance ksi` subcommand with
// injectable dependencies for testing.
func complianceKSICmdWithConfig(fileSvcFactory func(string, *slog.Logger) (fs.RuntimeFileService, error)) *cobra.Command {
	var class string
	var catalogPath string
	var scopeID string
	var runID string

	cmd := &cobra.Command{
		Use:   "ksi",
		Short: "Evaluate KSIs and print the result set as JSON",
		Long: `Evaluate g8e's live state against the FedRAMP 20x Key Security Indicators
for the specified certification class and print the KSIResultSet as JSON.
Every result is bound to the declared --scope-id and --run-id so consumers can
prove no result was produced from evidence outside the assessment context.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := cmd.Context()
			if ctx == nil {
				ctx = context.Background()
			}

			fileSvc, err := fileSvcFactory("", slog.Default())
			if err != nil {
				return fmt.Errorf("%w: %w", constants.ErrFileServiceInit, err)
			}

			cat, err := loadKSICatalog(catalogPath)
			if err != nil {
				return err
			}

			certClass, err := validateCertClass(class)
			if err != nil {
				return err
			}
			binding, err := buildEvaluationBinding(scopeID, runID)
			if err != nil {
				return err
			}
			resultSet := evaluateKSIs(ctx, fileSvc, cat, certClass, binding)
			if resultSet == nil {
				return fmt.Errorf("%w: cannot evaluate KSIs (audit store or ledger unavailable)", constants.ErrReportStoreUnavailable)
			}

			historyStore := newKSIHistoryStore(fileSvc)
			if err := saveKSIHistorySnapshot(ctx, historyStore, resultSet); err != nil {
				slog.Default().Warn("compliance: failed to save KSI history snapshot", "error", err)
			}
			if err := newKSIUnavailableIntervalStore(fileSvc).AppendFromResultSet(ctx, resultSet); err != nil {
				return fmt.Errorf("compliance: save KSI unavailable intervals: %w", err)
			}

			output, err := json.MarshalIndent(resultSet, "", "  ")
			if err != nil {
				return fmt.Errorf("compliance: marshal KSI result set: %w", err)
			}

			fmt.Fprintln(cmd.OutOrStdout(), string(output))
			return nil
		},
	}

	cmd.Flags().StringVar(&class, "class", "C", "FedRAMP 20x certification class (A, B, C, D)")
	cmd.Flags().StringVar(&catalogPath, "catalog", constants.DefaultKSICatalogPath, "Path to KSI catalog JSON file")
	cmd.Flags().StringVar(&scopeID, "scope-id", "", "Assessment scope ID binding every result to the declared context (required)")
	cmd.Flags().StringVar(&runID, "run-id", "", "Assessment run ID binding every result to the declared context (required)")
	_ = cmd.MarkFlagRequired("scope-id")
	_ = cmd.MarkFlagRequired("run-id")

	return cmd
}

// buildEvaluationBinding constructs an EvaluationBinding from CLI flags. The
// evidence window starts at the current time and ends one second later so
// every evaluation has a non-empty window. Callers that need a different
// window can construct the binding directly.
func buildEvaluationBinding(scopeID, runID string) (compliance.EvaluationBinding, error) {
	if scopeID == "" || runID == "" {
		return compliance.EvaluationBinding{}, fmt.Errorf("%w: --scope-id and --run-id are required", constants.ErrKSIBindingIncomplete)
	}
	now := time.Now()
	return compliance.EvaluationBinding{
		ScopeID:            scopeID,
		RunID:              runID,
		WindowStartUnixMs:  now.UnixMilli(),
		WindowEndUnixMs:    now.Add(time.Second).UnixMilli(),
		EvaluatorID:        constants.KSIEvaluatorID,
		EvaluatorVersion:   constants.KSIEvaluatorVersion,
		MethodDefinitionID: constants.KSIMethodDefinitionVersion,
	}, nil
}

// loadKSICatalog reads and validates the KSI catalog from the given path.
func loadKSICatalog(path string) (*compliance.KSICatalog, error) {
	cat, err := compliance.LoadKSICatalog(path)
	if err != nil {
		return nil, fmt.Errorf("compliance: load KSI catalog: %w", err)
	}
	return cat, nil
}

// evaluateKSIs opens the runtime stores, creates a KSI evaluator, registers
// default methods, and runs evaluation bound to the given binding. Returns nil
// if stores are unavailable or evaluation fails. Callers must treat nil as a
// fail-closed error.
func evaluateKSIs(ctx context.Context, fileSvc fs.RuntimeFileService, cat *compliance.KSICatalog, class compliance.CertificationClass, binding compliance.EvaluationBinding) *compliance.KSIResultSet {
	if ctx == nil {
		ctx = context.Background()
	}

	deps, cleanup, ok := openEvaluatorDeps(ctx, fileSvc)
	if !ok {
		slog.Default().Warn("compliance: evaluator deps unavailable")
		return nil
	}
	defer cleanup()

	evaluator := compliance.NewKSIEvaluator(cat)
	if err := evaluator.RegisterDefaultMethods(deps); err != nil {
		slog.Default().Warn("compliance: KSI method registration failed", "error", err)
		return nil
	}

	resultSet, err := evaluator.Evaluate(ctx, class, binding)
	if err != nil {
		slog.Default().Warn("compliance: KSI evaluation failed", "error", err)
		return nil
	}
	return resultSet
}

// openEvaluatorDeps opens the audit store, git ledger, and commitment ledger
// from the runtime file service. Returns ok=false if any store is unavailable.
// Each evidence source is opened by a dedicated opener (openVault,
// openAuditStore, openCommitments, openLedger); this function composes them and
// aggregates their cleanups so a failure in a later opener releases resources
// acquired by earlier ones.
func openEvaluatorDeps(ctx context.Context, fileSvc fs.RuntimeFileService) (compliance.EvaluatorDeps, func(), bool) {
	var cleanups []func()

	v, vaultCleanup := openVault(ctx, fileSvc)
	cleanups = append(cleanups, vaultCleanup)

	auditStore, auditCleanup, ok := openAuditStore(fileSvc, v)
	if !ok {
		runCleanups(cleanups)
		return compliance.EvaluatorDeps{}, nil, false
	}
	cleanups = append(cleanups, auditCleanup)

	cl, commitCleanup, ok := openCommitments(fileSvc)
	if !ok {
		runCleanups(cleanups)
		return compliance.EvaluatorDeps{}, nil, false
	}
	cleanups = append(cleanups, commitCleanup)

	ledger, ledgerCleanup, ok := openLedger(fileSvc, v)
	if !ok {
		runCleanups(cleanups)
		return compliance.EvaluatorDeps{}, nil, false
	}
	cleanups = append(cleanups, ledgerCleanup)

	return compliance.EvaluatorDeps{
		Audit:       auditStore,
		Ledger:      ledger,
		Commitments: cl,
		History:     newKSIHistoryStore(fileSvc),
	}, func() { runCleanups(cleanups) }, true
}

// runCleanups invokes each cleanup in reverse acquisition order (LIFO) so
// resources are released in the opposite order they were opened.
func runCleanups(cleanups []func()) {
	for i := len(cleanups) - 1; i >= 0; i-- {
		cleanups[i]()
	}
}

// openVault opens the encryption vault and attempts to unlock it with the
// runtime vault key. A nil vault (with a no-op cleanup) is returned if vault
// creation fails — callers proceed without encryption, which is fine for
// metadata-only evidence reads.
func openVault(ctx context.Context, fileSvc fs.RuntimeFileService) (*vault.Vault, func()) {
	logger := slog.Default()
	v, vaultErr := vault.NewVault(&vault.VaultConfig{
		DataDir: fileSvc.Resolve(constants.VaultDirname),
		Logger:  logger,
	})
	if vaultErr != nil {
		logger.Warn("compliance: vault creation failed, proceeding without encryption", "error", vaultErr)
		return nil, func() {}
	}
	vaultKeyRel := constants.SecretsDirname + "/" + constants.VaultKeyFilename
	keyData, keyErr := fileSvc.ReadFile(ctx, vaultKeyRel)
	if keyErr != nil {
		logger.Warn("compliance: vault key file not found, proceeding with locked vault", "path", vaultKeyRel, "error", keyErr)
		return v, func() {}
	}
	keyHex := strings.TrimSpace(string(keyData))
	keyBytes, decErr := hex.DecodeString(keyHex)
	if decErr != nil {
		logger.Warn("compliance: vault key hex decode failed, proceeding with locked vault", "error", decErr)
		return v, func() {}
	}
	if unlockErr := v.Unlock(keyBytes); unlockErr != nil {
		logger.Warn("compliance: vault unlock failed, proceeding with locked vault", "error", unlockErr)
	}
	return v, func() {}
}

// openAuditStore opens the SQL audit store. Returns ok=false on failure.
func openAuditStore(fileSvc fs.RuntimeFileService, v *vault.Vault) (*storage.SQLAuditStore, func(), bool) {
	auditCfg := storage.DefaultAuditStoreConfig()
	auditCfg.EncryptionVault = v
	auditStore, err := storage.NewSQLAuditStore(auditCfg, slog.Default(), fileSvc)
	if err != nil {
		return nil, nil, false
	}
	return auditStore, func() { auditStore.Close() }, true
}

// openCommitments opens the shared g8e.db connection and constructs a
// commitment ledger over it. Returns ok=false on failure.
func openCommitments(fileSvc fs.RuntimeFileService) (*storage.CommitmentLedger, func(), bool) {
	dbPath := pathutil.ResolveDBPath(fileSvc.Resolve(constants.DataDirname), constants.DbFilename)
	mainDB, err := sqliteutil.OpenDB(sqliteutil.DefaultDBConfig(dbPath), slog.Default())
	if err != nil {
		return nil, nil, false
	}
	cl := storage.NewCommitmentLedger(mainDB, slog.Default())
	return cl, func() { mainDB.Close() }, true
}

// openLedger opens the git-backed ledger service. Returns ok=false on failure.
func openLedger(fileSvc fs.RuntimeFileService, v *vault.Vault) (*storage.GitLedgerService, func(), bool) {
	ledgerCfg := &storage.LedgerConfig{
		GitPath:         "git",
		EncryptionVault: v,
	}
	ledger, err := storage.NewGitLedgerService(ledgerCfg, slog.Default(), fileSvc)
	if err != nil {
		return nil, nil, false
	}
	return ledger, func() {}, true
}

// complianceKSIHistoryCmdWithConfig creates the `compliance ksi-history` subcommand
// with injectable dependencies for testing.
func complianceKSIHistoryCmdWithConfig(fileSvcFactory func(string, *slog.Logger) (fs.RuntimeFileService, error)) *cobra.Command {
	var ksiID string

	cmd := &cobra.Command{
		Use:   "ksi-history",
		Short: "Read KSI evaluation history snapshots",
		Long: `Read persisted KSI evaluation snapshots from .g8e/data/compliance/ksi-history/.
When --ksi is specified, prints the chronological series for that KSI ID.
Without --ksi, prints all snapshots as a JSON array.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			fileSvc, err := fileSvcFactory("", slog.Default())
			if err != nil {
				return fmt.Errorf("%w: %w", constants.ErrFileServiceInit, err)
			}

			ctx := cmd.Context()
			if ctx == nil {
				ctx = context.Background()
			}

			store := newKSIHistoryStore(fileSvc)

			if ksiID != "" {
				results, err := store.GetHistoryForKSI(ctx, ksiID)
				if err != nil {
					return err
				}
				output, err := json.MarshalIndent(results, "", "  ")
				if err != nil {
					return fmt.Errorf("compliance: marshal KSI history: %w", err)
				}
				fmt.Fprintln(cmd.OutOrStdout(), string(output))
				return nil
			}

			snapshots, err := store.ListSnapshots(ctx)
			if err != nil {
				return err
			}
			if len(snapshots) == 0 {
				fmt.Fprintln(cmd.OutOrStdout(), "[]")
				return nil
			}
			output, err := json.MarshalIndent(snapshots, "", "  ")
			if err != nil {
				return fmt.Errorf("compliance: marshal KSI history: %w", err)
			}
			fmt.Fprintln(cmd.OutOrStdout(), string(output))
			return nil
		},
	}

	cmd.Flags().StringVar(&ksiID, "ksi", "", "KSI ID to filter history (e.g. KSI-CMT-01)")

	return cmd
}

// newKSIHistoryStore constructs a KSIHistoryStore rooted at the canonical
// runtime KSI history directory. Centralized so callers do not each rebuild
// the directory path and store independently.
func newKSIHistoryStore(fileSvc fs.RuntimeFileService) *compliance.KSIHistoryStore {
	historyDir := filepath.Join(constants.DataDirname, constants.ComplianceDirname, constants.KSIHistoryDirname)
	return compliance.NewKSIHistoryStore(fileSvc, historyDir)
}

func newKSIUnavailableIntervalStore(fileSvc fs.RuntimeFileService) *compliance.UnavailableIntervalStore {
	return compliance.NewUnavailableIntervalStore(fileSvc, filepath.Join(constants.DataDirname, constants.ComplianceDirname))
}

// saveKSIHistorySnapshot persists a KSIResultSet snapshot via the given history
// store. Prunes snapshots older than the retention period after saving.
func saveKSIHistorySnapshot(ctx context.Context, store *compliance.KSIHistoryStore, rs *compliance.KSIResultSet) error {
	if err := store.SaveSnapshot(ctx, rs); err != nil {
		return err
	}

	retention := time.Duration(constants.KSIHistoryRetentionDays) * 24 * time.Hour
	if _, err := store.PruneOlderThan(ctx, retention); err != nil {
		slog.Default().Warn("compliance: failed to prune KSI history", "error", err)
	}

	return nil
}

// validateCertClass validates that the class flag is one of A, B, C, D.
func validateCertClass(class string) (compliance.CertificationClass, error) {
	if len(class) != 1 || !strings.ContainsRune("ABCD", rune(class[0])) {
		return "", fmt.Errorf("%w: invalid certification class %q (must be A, B, C, or D)", constants.ErrValidationFailed, class)
	}
	return compliance.CertificationClass(class), nil
}

// complianceOverlayCmdWithConfig creates the `compliance overlay` subcommand
// with injectable dependencies for testing. It loads COSAiS overlay catalogs
// from a directory and validates KSI overlay references.
func complianceOverlayCmdWithConfig(fileSvcFactory func(string, *slog.Logger) (fs.RuntimeFileService, error)) *cobra.Command {
	var overlayDir string
	var catalogPath string

	cmd := &cobra.Command{
		Use:   "overlay",
		Short: "Load and validate COSAiS overlay catalogs",
		Long: `Load COSAiS (Control Overlays for Securing AI Systems) overlay JSON files
from the specified directory and validate that KSI overlay references resolve.
Prints the merged overlay catalog as JSON. Use --overlay-dir to specify a
custom overlay directory (default: docs/reference/).`,
		RunE: func(cmd *cobra.Command, args []string) error {
			// fileSvc is initialized for error-injection test parity with the
			// other compliance subcommands. The overlay command reads from
			// docs/reference/ (not .g8e/), so fileSvc itself is unused here.
			if _, err := fileSvcFactory("", slog.Default()); err != nil {
				return fmt.Errorf("%w: %w", constants.ErrFileServiceInit, err)
			}

			overlayCat, err := compliance.LoadOverlaysFromDir(overlayDir)
			if err != nil {
				return fmt.Errorf("compliance: load overlays: %w", err)
			}

			if len(overlayCat.Overlays) == 0 {
				fmt.Fprintln(cmd.OutOrStdout(), "[]")
				return nil
			}

			ksiCat, err := loadKSICatalog(catalogPath)
			if err != nil {
				return err
			}

			dangling := compliance.ValidateOverlayRefs(ksiCat, overlayCat)
			if len(dangling) > 0 {
				cmd.Printf("WARNING: %d dangling overlay reference(s):\n", len(dangling))
				for _, ref := range dangling {
					cmd.Printf("  %s\n", ref)
				}
			}

			output, err := json.MarshalIndent(overlayCat, "", "  ")
			if err != nil {
				return fmt.Errorf("compliance: marshal overlay catalog: %w", err)
			}
			fmt.Fprintln(cmd.OutOrStdout(), string(output))
			return nil
		},
	}

	cmd.Flags().StringVar(&overlayDir, "overlay-dir", constants.DefaultOverlayDirPath, "Directory containing COSAiS overlay JSON files")
	cmd.Flags().StringVar(&catalogPath, "catalog", constants.DefaultKSICatalogPath, "Path to KSI catalog JSON file")

	return cmd
}
