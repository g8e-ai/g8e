// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package gateway

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
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/g8e-ai/g8e/v2/internal/constants"
	"github.com/g8e-ai/g8e/v2/internal/models"
	"github.com/g8e-ai/g8e/v2/internal/services/fs"
)

const (
	defaultPublicSpectatorSourceID = "opendevops-local"
)

// ReadPublicExportConfig loads the durable public-feed export configuration.
func ReadPublicExportConfig(ctx context.Context, fileSvc fs.RuntimeFileService) (models.PublicExportConfig, error) {
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
	if err := ValidatePublicExportConfig(exportConfig); err != nil {
		return models.PublicExportConfig{}, err
	}
	return exportConfig, nil
}

// ValidatePublicExportConfig checks required public-feed publisher fields.
func ValidatePublicExportConfig(exportConfig models.PublicExportConfig) error {
	if strings.TrimSpace(exportConfig.SourceID) == "" {
		return constants.ErrPublicFeedSourceIDRequired
	}
	if strings.TrimSpace(exportConfig.SigningKeyID) == "" {
		return constants.ErrPublicFeedSigningKeyIDRequired
	}
	if exportConfig.BatchMaxRecords <= 0 || exportConfig.BatchMaxRecords > constants.PublicFeedBatchMaxRecords || exportConfig.BatchMaxBytes <= 0 || exportConfig.BatchMaxBytes > constants.PublicFeedBatchMaxBytes || exportConfig.RetryMaxAttempts <= 0 || exportConfig.RetryInitialBackoffSecs < 0 || exportConfig.RetryMaxBackoffSecs < exportConfig.RetryInitialBackoffSecs || exportConfig.AckWindowSecs <= 0 {
		return constants.ErrPublicFeedConfigRequired
	}
	return ValidatePublicMirrorOrigin(exportConfig.MirrorOrigin)
}

// ValidatePublicMirrorOrigin ensures the mirror origin is a bare loopback or
// hosted origin without path or credentials.
func ValidatePublicMirrorOrigin(origin string) error {
	parsed, err := url.Parse(origin)
	if err != nil || parsed.Host == "" || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" || parsed.Path != "" {
		return constants.ErrPublicFeedMirrorOriginRequired
	}
	return nil
}

// ValidatePublicMirrorListenAddresses enforces loopback-only private/public
// mirror listeners with distinct addresses.
func ValidatePublicMirrorListenAddresses(privateAddress, publicAddress string) error {
	if privateAddress == publicAddress {
		return fmt.Errorf("%w: duplicate address %q", constants.ErrPublicFeedListenAddress, privateAddress)
	}
	for _, address := range []string{privateAddress, publicAddress} {
		host, _, err := net.SplitHostPort(address)
		if err != nil {
			return fmt.Errorf("%w: %q: %v", constants.ErrPublicFeedListenAddress, address, err)
		}
		ip := net.ParseIP(host)
		if host != "localhost" && (ip == nil || !ip.IsLoopback()) {
			return fmt.Errorf("%w: %q", constants.ErrPublicFeedListenAddress, address)
		}
	}
	return nil
}

// EnsureLocalPublicFeed initializes export config and signing material when
// absent so the gateway-owned mirror can accept publisher ingest.
func EnsureLocalPublicFeed(ctx context.Context, fileSvc fs.RuntimeFileService, sourceID, mirrorOrigin string) (models.PublicExportConfig, error) {
	exportConfig, err := ReadPublicExportConfig(ctx, fileSvc)
	if err == nil {
		return exportConfig, nil
	}
	if !errors.Is(err, constants.ErrPublicFeedConfigRequired) {
		return models.PublicExportConfig{}, err
	}
	if strings.TrimSpace(sourceID) == "" {
		sourceID = defaultPublicSpectatorSourceID
	}
	if strings.TrimSpace(mirrorOrigin) == "" {
		mirrorOrigin = fmt.Sprintf("http://127.0.0.1:%d", constants.PublicSpectatorPrivatePort)
	}
	if err := ValidatePublicMirrorOrigin(mirrorOrigin); err != nil {
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

func writePublicExportConfig(ctx context.Context, fileSvc fs.RuntimeFileService, exportConfig models.PublicExportConfig) error {
	if err := ValidatePublicExportConfig(exportConfig); err != nil {
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

func rejectTrailingPublicJSON(decoder *json.Decoder) error {
	if decoder.More() {
		return fmt.Errorf("%w: trailing JSON", constants.ErrPublicFeedConfigRequired)
	}
	return nil
}

func newPublicMirrorHTTPServer(address string, handler http.Handler) *http.Server {
	return &http.Server{
		Addr:              address,
		Handler:           handler,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       30 * time.Second,
		IdleTimeout:       60 * time.Second,
	}
}
