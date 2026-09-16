// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package cmd

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/exec"
	"strings"

	"github.com/g8e-ai/g8e/v2/internal/constants"
	"github.com/g8e-ai/g8e/v2/internal/models"
	"github.com/g8e-ai/g8e/v2/internal/services/fs"
	"github.com/g8e-ai/g8e/v2/internal/services/gateway"
)

const (
	defaultPublicMirrorSourceID   = "opendevops-local"
	defaultPublicMirrorPrivateURL = "http://127.0.0.1:8081"
)

type publicMirrorRuntime struct {
	privateServer *http.Server
	publicServer  *http.Server
}

func ensureLocalPublicFeed(ctx context.Context, fileSvc fs.RuntimeFileService, sourceID, mirrorOrigin string) (models.PublicExportConfig, error) {
	exportConfig, err := readPublicExportConfig(ctx, fileSvc)
	if err == nil {
		return exportConfig, nil
	}
	if !errors.Is(err, constants.ErrPublicFeedConfigRequired) {
		return models.PublicExportConfig{}, err
	}
	if strings.TrimSpace(sourceID) == "" {
		sourceID = defaultPublicMirrorSourceID
	}
	if strings.TrimSpace(mirrorOrigin) == "" {
		mirrorOrigin = defaultPublicMirrorPrivateURL
	}
	if err := validatePublicMirrorOrigin(mirrorOrigin); err != nil {
		return models.PublicExportConfig{}, err
	}
	for _, relPath := range []string{constants.PublicFeedExportConfigPath, constants.PublicFeedSigningKeyPath, constants.PublicFeedIngestTokenPath} {
		exists, existsErr := fileSvc.FileExists(ctx, relPath)
		if existsErr != nil {
			return models.PublicExportConfig{}, fmt.Errorf("public-feed: inspect initialization path: %w", existsErr)
		}
		if exists {
			return models.PublicExportConfig{}, constants.ErrPublicFeedConfigExists
		}
	}
	if err := fileSvc.MkdirAll(ctx, constants.PublicFeedDirname, constants.PermDirPrivate); err != nil {
		return models.PublicExportConfig{}, fmt.Errorf("public-feed: create runtime directory: %w", err)
	}
	publicKey, privateKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return models.PublicExportConfig{}, fmt.Errorf("%w: %v", constants.ErrPublicFeedKeyGenFailed, err)
	}
	token := make([]byte, constants.PublicFeedIngestTokenBytes)
	if _, err := rand.Read(token); err != nil {
		return models.PublicExportConfig{}, fmt.Errorf("public-feed: generate ingest token: %w", err)
	}
	keyDigest := sha256.Sum256(publicKey)
	exportConfig = models.DefaultPublicExportConfig()
	exportConfig.Enabled = true
	exportConfig.SourceID = sourceID
	exportConfig.MirrorOrigin = mirrorOrigin
	exportConfig.SigningKeyID = hex.EncodeToString(keyDigest[:])
	if err := fileSvc.WriteFile(ctx, constants.PublicFeedSigningKeyPath, []byte(hex.EncodeToString(privateKey)), constants.PermFilePrivate); err != nil {
		return models.PublicExportConfig{}, fmt.Errorf("public-feed: write signing key: %w", err)
	}
	if err := fileSvc.WriteFile(ctx, constants.PublicFeedIngestTokenPath, []byte(hex.EncodeToString(token)), constants.PermFilePrivate); err != nil {
		return models.PublicExportConfig{}, fmt.Errorf("public-feed: write ingest token: %w", err)
	}
	if err := writePublicExportConfig(ctx, fileSvc, exportConfig); err != nil {
		return models.PublicExportConfig{}, err
	}
	return exportConfig, nil
}

func newPublicMirrorRuntime(ctx context.Context, fileSvc fs.RuntimeFileService, exportConfig models.PublicExportConfig, listenAddress, publicListenAddress string) (*publicMirrorRuntime, error) {
	if err := validatePublicMirrorListenAddresses(listenAddress, publicListenAddress); err != nil {
		return nil, err
	}
	key, err := readPublicSecret(ctx, fileSvc, constants.PublicFeedSigningKeyPath, ed25519.PrivateKeySize, constants.ErrPublicFeedSigningKeyRequired)
	if err != nil {
		return nil, err
	}
	token, err := readPublicSecret(ctx, fileSvc, constants.PublicFeedIngestTokenPath, constants.PublicFeedIngestTokenBytes, constants.ErrPublicFeedIngestTokenRequired)
	if err != nil {
		return nil, err
	}
	mirror, err := gateway.NewPublicMirrorServer(slog.Default(), gateway.NewRuntimePublicMirrorStore(fileSvc))
	if err != nil {
		return nil, err
	}
	mirror.SetIngestAuthToken(hex.EncodeToString(token))
	privateKey := ed25519.PrivateKey(key)
	if err := mirror.RegisterSourceKey(ctx, exportConfig.SourceID, exportConfig.SigningKeyID, privateKey.Public().(ed25519.PublicKey)); err != nil {
		return nil, err
	}
	return &publicMirrorRuntime{
		privateServer: newPublicMirrorHTTPServer(listenAddress, mirror.Handler()),
		publicServer:  newPublicMirrorHTTPServer(publicListenAddress, mirror.PublicHandler()),
	}, nil
}

func (runtime *publicMirrorRuntime) serve(ctx context.Context) error {
	if runtime == nil {
		return fmt.Errorf("public mirror: %w", constants.ErrMissingRequiredField)
	}
	return runPublicMirrorServers(ctx, runtime.privateServer, runtime.publicServer)
}

func startPublicMirrorDaemon(listenAddress, publicListenAddress, sourceID, mirrorOrigin string) (int, error) {
	executable, err := os.Executable()
	if err != nil {
		return 0, fmt.Errorf("public mirror: resolve executable: %w", err)
	}
	args := []string{
		"eval", "mirror", "run",
		"--listen", listenAddress,
		"--public-listen", publicListenAddress,
	}
	if strings.TrimSpace(sourceID) != "" {
		args = append(args, "--source-id", sourceID)
	}
	if strings.TrimSpace(mirrorOrigin) != "" {
		args = append(args, "--mirror-origin", mirrorOrigin)
	}
	command := exec.Command(executable, args...)
	command.Stdout = os.Stdout
	command.Stderr = os.Stderr
	if err := command.Start(); err != nil {
		return 0, fmt.Errorf("public mirror: start daemon: %w", err)
	}
	return command.Process.Pid, nil
}
