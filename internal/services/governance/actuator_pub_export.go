package governance

import (
	"context"
	"crypto/ed25519"
	"crypto/x509"
	"encoding/hex"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"log/slog"
	"path/filepath"

	"github.com/g8e-ai/g8e/v2/internal/constants"
	"github.com/g8e-ai/g8e/v2/internal/models"
	"github.com/g8e-ai/g8e/v2/internal/services/fs"
)

// ExportActuatorPublicKey writes the Actuator's public key to both PEM and JSON formats
// in the PKI directory for receipt verification by the evals harness.
func ExportActuatorPublicKey(fileSvc fs.RuntimeFileService, pubKey ed25519.PublicKey, keyID string, logger *slog.Logger) error {
	if err := fileSvc.MkdirAll(context.Background(), constants.PkiDirname, constants.PermDirPrivate); err != nil {
		return fmt.Errorf("%w: %w", constants.ErrDirCreateFailed, err)
	}

	pemRelPath := filepath.Join(constants.PkiDirname, constants.ActuatorPubPEMFilename)
	publicKeyDER, err := x509.MarshalPKIXPublicKey(pubKey)
	if err != nil {
		return fmt.Errorf("%w: marshal actuator public key: %w", constants.ErrCertSaveFailed, err)
	}
	pemData := pem.EncodeToMemory(&pem.Block{
		Type:  "PUBLIC KEY",
		Bytes: publicKeyDER,
	})
	if err := fileSvc.WriteFile(context.Background(), pemRelPath, pemData, constants.PermFilePrivate); err != nil {
		return fmt.Errorf("%w: %w", constants.ErrCertSaveFailed, err)
	}
	if logger != nil {
		logger.Info("Actuator public key exported", "path", fileSvc.Resolve(pemRelPath), "format", "PEM")
	}

	jsonRelPath := filepath.Join(constants.PkiDirname, constants.ActuatorPubJSONFilename)
	jsonData := models.ActuatorPublicKeyExport{
		KeyID:     keyID,
		PublicKey: hex.EncodeToString(pubKey),
		Algorithm: "ed25519",
	}
	jsonBytes, err := json.MarshalIndent(jsonData, "", "  ")
	if err != nil {
		return fmt.Errorf("%w: %w", constants.ErrJSONMarshalFailed, err)
	}
	if err := fileSvc.WriteFile(context.Background(), jsonRelPath, jsonBytes, constants.PermFilePrivate); err != nil {
		return fmt.Errorf("%w: %w", constants.ErrCertSaveFailed, err)
	}
	if logger != nil {
		logger.Info("Actuator public key exported", "path", fileSvc.Resolve(jsonRelPath), "format", "JSON")
	}

	return nil
}
