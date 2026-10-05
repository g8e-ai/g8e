// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

//go:build integration

package audit

import (
	"bytes"
	"context"
	"crypto/ecdsa"
	"crypto/ed25519"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/structpb"
	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/g8e-ai/g8e/v2/internal/cli/api"
	"github.com/g8e-ai/g8e/v2/internal/cli/auth"
	authcmd "github.com/g8e-ai/g8e/v2/internal/cli/cmd/auth"
	"github.com/g8e-ai/g8e/v2/internal/cli/cmd/cmdtest"
	clicfg "github.com/g8e-ai/g8e/v2/internal/cli/config"
	"github.com/g8e-ai/g8e/v2/internal/config"
	"github.com/g8e-ai/g8e/v2/internal/constants"
	govtypes "github.com/g8e-ai/g8e/v2/internal/governance"
	"github.com/g8e-ai/g8e/v2/internal/marshaler"
	"github.com/g8e-ai/g8e/v2/internal/models"
	"github.com/g8e-ai/g8e/v2/internal/services/fs"
	"github.com/g8e-ai/g8e/v2/internal/services/gateway"
	"github.com/g8e-ai/g8e/v2/internal/services/governance"
	"github.com/g8e-ai/g8e/v2/internal/testutil"
	clientpkg "github.com/g8e-ai/g8e/v2/internal/tools/agent_harness/client"
	commonv1 "github.com/g8e-ai/g8e/v2/protocol/proto/g8e/common/v1"
	operatorv1 "github.com/g8e-ai/g8e/v2/protocol/proto/g8e/operator/v1"
)

// This test uses production Gateway construction, PKI, mTLS authentication,
// governance processing, SQLite/vault persistence, and the CLI API client.
func TestAuditAppQueries_DocumentCreateMergeThroughRealGateway(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	logger := testutil.NewTestLogger()
	gatewayFiles, err := fs.NewRuntimeFileService(testutil.TempDir(t), logger)
	require.NoError(t, err)
	require.NoError(t, gatewayFiles.CreateRuntimeTree(ctx))
	cfg := testutil.NewTestConfig(t)
	cfg.Gateway.Posture = config.PostureDoctrine
	cfg.Gateway.SecretsDir = gatewayFiles.Resolve(constants.SecretsDirname)
	cfg.Gateway.VaultDir = gatewayFiles.Resolve(constants.VaultDirname)
	svc, err := gateway.NewGatewayModeService(cfg, gatewayFiles, logger)
	require.NoError(t, err)
	t.Cleanup(func() { svc.Stop(ctx) })
	server := httptest.NewUnstartedServer(svc.GetHTTPHandler())
	server.TLS = svc.GetPKI().TLSConfig()
	serverCert, err := server.TLS.GetCertificate(&tls.ClientHelloInfo{})
	require.NoError(t, err)
	server.TLS.Certificates = []tls.Certificate{*serverCert}
	server.StartTLS()
	t.Cleanup(server.Close)

	// Bootstrap a real owner and CLI identity through the production HTTP route.
	bootstrapServer := httptest.NewServer(svc.GetHTTPHandler())
	t.Cleanup(bootstrapServer.Close)
	cliKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)
	cliCSR, err := x509.CreateCertificateRequest(rand.Reader, &x509.CertificateRequest{Subject: pkix.Name{CommonName: "audit-cli"}}, cliKey)
	require.NoError(t, err)
	bootstrapBody, err := json.Marshal(models.BootstrapRequest{CLICSR: string(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE REQUEST", Bytes: cliCSR})), SystemFingerprint: "audit-test-host"})
	require.NoError(t, err)
	bootstrapRequest, err := http.NewRequestWithContext(ctx, http.MethodPost, bootstrapServer.URL+constants.APIPaths.AuthBootstrap, bytes.NewReader(bootstrapBody))
	require.NoError(t, err)
	bootstrapResponse, err := bootstrapServer.Client().Do(bootstrapRequest)
	require.NoError(t, err)
	bootstrapWire, err := io.ReadAll(bootstrapResponse.Body)
	require.NoError(t, bootstrapResponse.Body.Close())
	require.NoError(t, err)
	require.Equal(t, http.StatusCreated, bootstrapResponse.StatusCode)
	var bootstrap models.BootstrapResponse
	require.NoError(t, json.Unmarshal(bootstrapWire, &bootstrap))
	require.NotEmpty(t, bootstrap.CLICert)
	require.NotEmpty(t, bootstrap.OperatorSessionID)
	cliKeyDER, err := x509.MarshalECPrivateKey(cliKey)
	require.NoError(t, err)
	cliKeyPEM := pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: cliKeyDER})
	files, cliConfig := cmdtest.NewCmdTestEnv(t)
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)
	csr, err := x509.CreateCertificateRequest(rand.Reader, &x509.CertificateRequest{Subject: pkix.Name{CommonName: "g8ee"}}, key)
	require.NoError(t, err)
	certPEM, _, err := svc.GetPKI().SignCSR(string(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE REQUEST", Bytes: csr})), constants.LeafTypeApp, "", "g8ee", "", "", "")
	require.NoError(t, err)
	keyDER, err := x509.MarshalECPrivateKey(key)
	require.NoError(t, err)
	keyPEM := pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: keyDER})
	for path, data := range map[string][]byte{cliConfig.CLICertFile(): []byte(bootstrap.CLICert), cliConfig.CLIKeyFile(): cliKeyPEM} {
		rel, err := files.Rel(path)
		require.NoError(t, err)
		require.NoError(t, files.WriteFile(ctx, rel, data, constants.PermFilePrivate))
	}
	trustRel, err := gatewayFiles.Rel(svc.GetPKI().TrustBundlePath())
	require.NoError(t, err)
	trust, err := gatewayFiles.ReadFile(ctx, trustRel)
	require.NoError(t, err)
	require.NoError(t, auth.WriteTrustBundleFS(files, cliConfig, trust, constants.PermFilePublic))
	// The saved binding must never become an implicit audit filter.
	require.NoError(t, auth.SaveCredentials(files, cliConfig, &auth.Credentials{OperatorSessionID: bootstrap.OperatorSessionID, OperatorID: bootstrap.OperatorID, CLISessionID: bootstrap.CLISessionID, UserID: bootstrap.UserID}))
	policy, err := json.Marshal(&models.AppPolicy{AppID: "spiffe://g8e.local/app/g8ee", CreatedAt: time.Now(), UpdatedAt: time.Now()})
	require.NoError(t, err)
	require.NoError(t, svc.GetDocStore().DocSet(marshaler.CollectionName(constants.CollectionAppPolicies), "spiffe://g8e.local/app/g8ee", policy))
	cert, err := tls.X509KeyPair([]byte(certPEM), keyPEM)
	require.NoError(t, err)
	roots := x509.NewCertPool()
	require.True(t, roots.AppendCertsFromPEM(trust))
	transport := &http.Transport{TLSClientConfig: &tls.Config{RootCAs: roots, Certificates: []tls.Certificate{cert}, MinVersion: tls.VersionTLS13}}
	t.Cleanup(transport.CloseIdleConnections)
	httpClient := &http.Client{Transport: transport, Timeout: 10 * time.Second}

	var transactionIDs []string
	for index, merge := range []bool{false, true} {
		fields := &structpb.Struct{Fields: map[string]*structpb.Value{"case_title": structpb.NewStringValue("created")}}
		if merge {
			fields.Fields["case_title"] = structpb.NewStringValue("merged")
		} else {
			fields.Fields["status"] = structpb.NewStringValue("open")
		}
		payload, err := proto.Marshal(&operatorv1.DocumentUpdateRequested{Collection: string(constants.CollectionInvestigations), DocumentId: "audit-document", Updates: fields, Merge: merge})
		require.NoError(t, err)
		root, err := svc.GetStateRootSvc().GetCurrentStateRoot()
		require.NoError(t, err)
		env := &commonv1.GovernanceEnvelope{ProtocolVersion: govtypes.GovernanceProtocolVersionV2, Timestamp: timestamppb.Now(), ExpiresAt: timestamppb.New(time.Now().Add(time.Minute)), SourceComponent: commonv1.Component_COMPONENT_AGENT, ActingAppId: "g8ee", RequestorUserId: bootstrap.UserID, EventType: string(constants.EventAppDocumentUpdateRequested), ActionType: string(constants.ActionTypeDocumentUpdate), TargetResource: string(constants.CollectionInvestigations) + "/audit-document", Payload: payload, StateMerkleRoot: root, Nonce: []string{"create-nonce", "merge-nonce"}[index]}
		env.Id, err = govtypes.GenerateMessageID(env)
		require.NoError(t, err)
		env.TransactionHash = env.Id
		wire, err := protojson.Marshal(env)
		require.NoError(t, err)
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, server.URL+constants.APIPaths.GovernanceEnvelopes, bytes.NewReader(wire))
		require.NoError(t, err)
		resp, err := httpClient.Do(req)
		require.NoError(t, err)
		body, err := io.ReadAll(resp.Body)
		require.NoError(t, resp.Body.Close())
		require.NoError(t, err)
		require.Equal(t, http.StatusOK, resp.StatusCode, string(body))
		receipt := &operatorv1.ActionReceipt{}
		require.NoError(t, protojson.Unmarshal(body, receipt))
		require.Equal(t, operatorv1.ExecutionStatus_EXECUTION_STATUS_COMPLETED, receipt.Status)
		secretManager, err := svc.GetSecretManager()
		require.NoError(t, err)
		signingKey, _, err := secretManager.GetActuatorKey()
		require.NoError(t, err)
		require.NoError(t, governance.VerifyActionReceiptSignature(receipt, signingKey.Public().(ed25519.PublicKey)))
		require.NoError(t, governance.VerifyReceiptPersistenceAttestation(receipt, signingKey.Public().(ed25519.PublicKey)))
		persisted, err := svc.GetAuditStore().GetActionReceipt(env.Id)
		require.NoError(t, err)
		require.NotNil(t, persisted)
		require.Empty(t, persisted.OperatorSessionID)
		require.Equal(t, "g8ee", persisted.ActingAppID)
		require.True(t, proto.Equal(receipt, persisted.ActionReceipt))
		transactionIDs = append(transactionIDs, env.Id)
	}
	readClient, err := api.NewClientWithURL(files, cliConfig, server.URL)
	require.NoError(t, err)
	docBytes, err := readClient.Get(constants.APIPaths.DataPrefix + string(constants.CollectionInvestigations) + "/audit-document")
	require.NoError(t, err)
	var doc clientpkg.DocumentResponse
	require.NoError(t, json.Unmarshal(docBytes, &doc))
	require.Equal(t, "merged", doc.GetString("case_title"))
	require.Equal(t, "open", doc.GetString("status"))

	loader := func(string) (*clicfg.Config, error) { return cliConfig, nil }
	factory := func(fs.RuntimeFileService, *clicfg.Config) (authcmd.APIClient, error) {
		return api.NewClientWithURL(files, cliConfig, server.URL)
	}
	fileFactory := func(string, *slog.Logger) (fs.RuntimeFileService, error) { return files, nil }
	for _, scope := range []struct {
		name, flag, value string
		receipts, events  int
	}{
		{"all", "", "", 2, 6}, {"app", "app", "g8ee", 2, 6}, {"different app", "app", "other-app", 0, 0}, {"bound Operator session", "session", bootstrap.OperatorSessionID, 0, 0},
	} {
		t.Run(scope.name, func(t *testing.T) {
			for _, name := range []string{"receipts", "events", "export", "summary", "report"} {
				t.Run(name, func(t *testing.T) {
					var cmd *cobra.Command
					switch name {
					case "receipts":
						cmd = auditReceiptsCmdWithConfig(loader, factory, fileFactory)
					case "events":
						cmd = auditEventsCmdWithConfig(loader, factory, fileFactory)
					case "export":
						cmd = auditExportCmdWithConfig(loader, factory, fileFactory)
					case "summary":
						cmd = auditSummaryCmdWithConfig(loader, factory, fileFactory)
					case "report":
						cmd = auditReportCmdWithConfig(loader, factory, fileFactory)
					}
					if scope.flag != "" {
						require.NoError(t, cmd.Flags().Set(scope.flag, scope.value))
					}
					out := &bytes.Buffer{}
					cmd.SetOut(out)
					if name == "receipts" || name == "events" {
						cmdtest.EnableGlobalJSON(t, cmd)
					}
					outputDir := testutil.TempDir(t)
					outputFile := outputDir + "/export.json"
					if name == "export" {
						require.NoError(t, cmd.Flags().Set("out", outputFile))
					}
					if name == "report" {
						require.NoError(t, cmd.Flags().Set("out", outputDir))
					}
					require.NoError(t, cmd.RunE(cmd, nil))
					switch name {
					case "receipts", "export":
						data := out.Bytes()
						if name == "export" {
							data, err = os.ReadFile(outputFile)
							require.NoError(t, err)
						}
						var result models.AuditReceiptsResponse
						require.NoError(t, json.Unmarshal(data, &result))
						require.Len(t, result.Receipts, scope.receipts)
						for _, r := range result.Receipts {
							require.Contains(t, transactionIDs, r.TransactionID)
							require.Equal(t, "g8ee", r.ActingAppID)
							require.Empty(t, r.OperatorSessionID)
						}
					case "events":
						var result models.AuditEventsResponse
						require.NoError(t, json.Unmarshal(out.Bytes(), &result))
						require.Len(t, result.Events, scope.events)
						for _, e := range result.Events {
							require.Contains(t, transactionIDs, e.TransactionID)
							require.Empty(t, e.OperatorSessionID)
						}
					case "summary":
						require.Contains(t, out.String(), map[bool]string{true: "No audit records found", false: "Total records: 8"}[scope.events == 0])
					case "report":
						require.Contains(t, out.String(), fmt.Sprintf("Events:   %d", scope.events))
						require.Contains(t, out.String(), fmt.Sprintf("Receipts: %d", scope.receipts))
						reportBytes, err := os.ReadFile(filepath.Join(outputDir, constants.ComplianceReportFilename))
						require.NoError(t, err)
						var result models.AuditReportResponse
						require.NoError(t, json.Unmarshal(reportBytes, &result))
						require.Len(t, result.Report.Events, scope.events)
						require.Len(t, result.Report.Receipts, scope.receipts)
						if scope.flag == "app" {
							require.Equal(t, scope.value, result.Report.ActingAppID)
						}
					}
				})
			}
		})
	}
	t.Run("transaction lookup displays canonical receipt", func(t *testing.T) {
		cmd := auditReceiptsCmdWithConfig(loader, factory, fileFactory)
		require.NoError(t, cmd.Flags().Set("tx-id", transactionIDs[0]))
		out := &bytes.Buffer{}
		cmd.SetOut(out)
		require.NoError(t, cmd.RunE(cmd, nil))
		require.Contains(t, out.String(), transactionIDs[0])
	})
	t.Run("event pagination and local report include sessionless records", func(t *testing.T) {
		events, err := svc.GetAuditStore().ListEvents("", 100, 0)
		require.NoError(t, err)
		require.Len(t, events, 6)
		first, err := svc.GetAuditStore().GetEvents(models.AuditScope{ActingAppID: "g8ee"}, 1, 0)
		require.NoError(t, err)
		second, err := svc.GetAuditStore().GetEvents(models.AuditScope{ActingAppID: "g8ee"}, 1, 1)
		require.NoError(t, err)
		require.Len(t, first, 1)
		require.Len(t, second, 1)
		require.NotEqual(t, first[0].ID, second[0].ID)
		require.Contains(t, transactionIDs, first[0].TransactionID)
		require.Contains(t, first[0].ContentText, first[0].TransactionID)
		require.Empty(t, first[0].OperatorSessionID)
	})
	require.NoError(t, svc.GetAuditStore().VerifyChain(ctx, 0))
}
