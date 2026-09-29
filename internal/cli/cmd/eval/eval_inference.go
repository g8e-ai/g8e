// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package eval

import (
	"context"
	"crypto/x509"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"google.golang.org/protobuf/encoding/protojson"

	"github.com/g8e-ai/g8e/v2/internal/cli/auth"
	"github.com/g8e-ai/g8e/v2/internal/cli/config"
	"github.com/g8e-ai/g8e/v2/internal/cli/output"
	"github.com/g8e-ai/g8e/v2/internal/constants"
	"github.com/g8e-ai/g8e/v2/internal/models"
	"github.com/g8e-ai/g8e/v2/internal/pkg/certutil"
	"github.com/g8e-ai/g8e/v2/internal/services/evaluation"
	"github.com/g8e-ai/g8e/v2/internal/services/fs"
	harnessclient "github.com/g8e-ai/g8e/v2/internal/tools/agent_harness/client"
	harnessconfig "github.com/g8e-ai/g8e/v2/internal/tools/agent_harness/config"
	operatorv1 "github.com/g8e-ai/g8e/v2/protocol/proto/g8e/operator/v1"
)

type inferenceAcceptanceResultJSON struct {
	Case            string `json:"case"`
	ProviderAttempt string `json:"provider_attempt"`
	Stream          bool   `json:"stream"`
	Status          string `json:"status"`
	Error           string `json:"error,omitempty"`
	ProgressEvents  int    `json:"progress_events,omitempty"`
	ResultDigest    string `json:"result_digest,omitempty"`
}

type inferenceAcceptanceOutputJSON struct {
	OperatorSessionID string                          `json:"operator_session_id"`
	Model             string                          `json:"model"`
	Passed            int                             `json:"passed"`
	Failed            int                             `json:"failed"`
	Cases             []inferenceAcceptanceResultJSON `json:"cases"`
}

type inferenceEvalDeps struct {
	configLoader     func(string) (*config.Config, error)
	fileSvcFactory   func(string, *slog.Logger) (fs.RuntimeFileService, error)
	authLoader       func(fs.RuntimeFileService, *config.Config) (*auth.ClientAuthContext, error)
	clientFactory    func(harnessconfig.Config) (*harnessclient.Client, error)
	appClientFactory func(harnessconfig.Config) (*harnessclient.Client, error)
	now              func() time.Time
	newID            func() string
}

func gatesInferenceEvalCmd(deps nativeEvalDeps) *cobra.Command {
	var model string
	var role string
	var casesCSV string
	cmd := &cobra.Command{
		Use:   "inference",
		Short: "Inference-only vertical acceptance matrix",
		RunE: func(cmd *cobra.Command, args []string) error {
			if model == "" {
				return fmt.Errorf("evaluation: gates inference: --model is required")
			}
			caseIDs, err := parseInferenceAcceptanceCases(casesCSV)
			if err != nil {
				return err
			}
			cases, err := evaluation.SelectInferenceAcceptanceCases(caseIDs)
			if err != nil {
				return fmt.Errorf("evaluation: gates inference: %w", err)
			}
			cfg, fileSvc, err := nativeEvalEnvironment(cmd, deps)
			if err != nil {
				return err
			}
			authContext, err := deps.authLoader(fileSvc, cfg)
			if err != nil {
				return fmt.Errorf("evaluation: load CLI identity: %w", err)
			}
			gatewayClient, err := deps.clientFactory(nativeEvalClientConfig(cfg, authContext))
			if err != nil {
				return fmt.Errorf("evaluation: initialize gateway client: %w", err)
			}
			operators, _, err := gatewayClient.ListOperators(cmd.Context())
			if err != nil {
				return fmt.Errorf("evaluation: list operators: %w", err)
			}
			selected, err := evaluation.SelectInferenceOperator(operators, "")
			if err != nil {
				return fmt.Errorf("evaluation: gates inference: %w", err)
			}
			appClient, err := inferenceEvalAppClientFrom(cfg, authContext, deps.clientFactory)
			if err != nil {
				return fmt.Errorf("evaluation: gates inference: %w", err)
			}
			probeRole, err := parseInferenceProbeRole(role)
			if err != nil {
				return fmt.Errorf("evaluation: gates inference: %w", err)
			}
			baseReq := evaluation.InferenceProbeRequest{
				ProviderAttemptID:       deps.newID(),
				Role:                    probeRole,
				Model:                   model,
				TargetOperatorSessionID: selected.OperatorSessionID,
			}
			results := make([]inferenceAcceptanceResultJSON, 0, len(cases))
			failures := 0
			for _, acceptanceCase := range cases {
				probeReq := acceptanceCase.Apply(baseReq)
				probeReq.ProviderAttemptID = deps.newID()
				probeReq.Stream = acceptanceCase.Stream
				ctx, cancel := context.WithTimeout(cmd.Context(), 6*time.Minute)
				response, progress, runErr := inferenceEvalExecuteProbe(ctx, appClient, probeReq)
				cancel()
				entry := inferenceAcceptanceResultJSON{
					Case:            string(acceptanceCase.ID),
					ProviderAttempt: probeReq.ProviderAttemptID,
					Stream:          probeReq.Stream,
				}
				if runErr != nil {
					entry.Status = "failed"
					entry.Error = runErr.Error()
					failures++
				} else if err := evaluation.ValidateInferenceProbeStream(probeReq, progress, response); err != nil {
					entry.Status = "failed"
					entry.Error = err.Error()
					failures++
				} else if err := evaluation.ValidateInferenceAcceptanceCase(acceptanceCase.ID, response); err != nil {
					entry.Status = "failed"
					entry.Error = err.Error()
					failures++
				} else {
					entry.Status = "passed"
					entry.ProgressEvents = len(progress)
					entry.ResultDigest = response.GetResult().GetResultDigest()
				}
				results = append(results, entry)
				if output.JSONEnabled(cmd) {
					continue
				}
				if entry.Status == "passed" {
					_, _ = fmt.Fprintf(cmd.OutOrStdout(), "PASS %s\n", acceptanceCase.ID)
				} else {
					_, _ = fmt.Fprintf(cmd.OutOrStdout(), "FAIL %s: %s\n", acceptanceCase.ID, entry.Error)
				}
			}
			if output.JSONEnabled(cmd) {
				payload, err := json.MarshalIndent(inferenceAcceptanceOutputJSON{
					OperatorSessionID: selected.OperatorSessionID,
					Model:             model,
					Passed:            len(cases) - failures,
					Failed:            failures,
					Cases:             results,
				}, "", "  ")
				if err != nil {
					return err
				}
				_, err = fmt.Fprintln(cmd.OutOrStdout(), string(payload))
				if err != nil {
					return err
				}
			} else {
				_, _ = fmt.Fprintf(cmd.OutOrStdout(), "\nPhase 1A inference acceptance: %d passed, %d failed\n", len(cases)-failures, failures)
			}
			if failures > 0 {
				return fmt.Errorf("evaluation: gates inference: %d case(s) failed", failures)
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&model, "model", "", "Requested provider model tag")
	cmd.Flags().StringVar(&role, "role", "primary", "Default governed model role for cases that do not override it")
	cmd.Flags().StringVar(&casesCSV, "cases", "", "Comma-separated case IDs (default: full Phase 1A inference matrix)")
	return cmd
}

func gatesProbeEvalCmd(deps nativeEvalDeps) *cobra.Command {
	var model string
	var role string
	var prompt string
	var seed int32 = -1
	var stream bool
	cmd := &cobra.Command{
		Use:   "probe <model>",
		Short: "Single non-scored governed inference probe",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			model = args[0]
			cfg, fileSvc, err := nativeEvalEnvironment(cmd, deps)
			if err != nil {
				return err
			}
			authContext, err := deps.authLoader(fileSvc, cfg)
			if err != nil {
				return fmt.Errorf("evaluation: load CLI identity: %w", err)
			}
			gatewayClient, err := deps.clientFactory(nativeEvalClientConfig(cfg, authContext))
			if err != nil {
				return fmt.Errorf("evaluation: initialize gateway client: %w", err)
			}
			operators, _, err := gatewayClient.ListOperators(cmd.Context())
			if err != nil {
				return fmt.Errorf("evaluation: list operators: %w", err)
			}
			selected, err := evaluation.SelectInferenceOperator(operators, "")
			if err != nil {
				return fmt.Errorf("evaluation: gates probe: %w", err)
			}
			appClient, err := inferenceEvalAppClientFrom(cfg, authContext, deps.clientFactory)
			if err != nil {
				return fmt.Errorf("evaluation: gates probe: %w", err)
			}
			probeRole, err := parseInferenceProbeRole(role)
			if err != nil {
				return fmt.Errorf("evaluation: gates probe: %w", err)
			}
			probeReq := evaluation.InferenceProbeRequest{
				ProviderAttemptID:       deps.newID(),
				Role:                    probeRole,
				Model:                   model,
				TargetOperatorSessionID: selected.OperatorSessionID,
				Prompt:                  prompt,
				Stream:                  stream,
			}
			if seed >= 0 {
				probeReq.Seed = &seed
			}
			ctx, cancel := context.WithTimeout(cmd.Context(), 6*time.Minute)
			defer cancel()
			response, progress, err := inferenceEvalExecuteProbe(ctx, appClient, probeReq)
			if err != nil {
				return fmt.Errorf("evaluation: gates probe: %w", err)
			}
			if err := evaluation.ValidateInferenceProbeStream(probeReq, progress, response); err != nil {
				return fmt.Errorf("evaluation: gates probe: %w", err)
			}
			if output.JSONEnabled(cmd) {
				body, err := protojson.Marshal(response)
				if err != nil {
					return fmt.Errorf("evaluation: gates probe: marshal response: %w", err)
				}
				_, err = fmt.Fprintln(cmd.OutOrStdout(), string(body))
				return err
			}
			result := response.GetResult()
			_, err = fmt.Fprintf(cmd.OutOrStdout(), "Probe accepted\nSession: %s\nModel: %s\nAttempt: %s\nResult digest: %s\nOutput hash: %s\nParts: %d\nProgress events: %d\n", selected.OperatorSessionID, result.GetRequestedModel(), result.GetProviderAttemptId(), result.GetResultDigest(), result.GetOutputHash(), len(result.GetParts()), len(progress))
			return err
		},
	}
	cmd.Flags().StringVar(&model, "model", "", "[DEPRECATED] use positional argument instead")
	cmd.Flags().StringVar(&role, "role", "primary", "Governed model role: primary, assistant, or lite")
	cmd.Flags().StringVar(&prompt, "prompt", "", "Probe prompt (default: Reply with exactly: probe-ok)")
	cmd.Flags().Int32Var(&seed, "seed", -1, "Optional deterministic generation seed (omit for provider default)")
	cmd.Flags().BoolVar(&stream, "stream", false, "Request live NDJSON progress telemetry during generation")
	return cmd
}

func inferenceEvalAppClientFrom(cfg *config.Config, authContext *auth.ClientAuthContext, clientFactory func(harnessconfig.Config) (*harnessclient.Client, error)) (*harnessclient.Client, error) {
	certFile, keyFile, err := resolveInferenceProbeAppCredentials(nil, cfg)
	if err != nil {
		return nil, err
	}
	trustBundle := cfg.ResolvedTrustBundlePath()
	appConfig := harnessconfig.Config{
		MTLSBaseURL: cfg.OperatorHTTPURL(),
		Auth: harnessconfig.Auth{
			ClientCert: certFile,
			ClientKey:  keyFile,
			CABundle:   trustBundle,
		},
		UserID: authContext.UserID,
	}
	return clientFactory(appConfig)
}

func inferenceEvalExecuteProbe(
	ctx context.Context,
	appClient *harnessclient.Client,
	probeReq evaluation.InferenceProbeRequest,
) (*operatorv1.InferenceDispatchResponse, []*operatorv1.InferenceProgressEvent, error) {
	dispatchReq, err := evaluation.BuildInferenceProbeDispatchRequest(probeReq)
	if err != nil {
		return nil, nil, err
	}
	if probeReq.Stream {
		streamResult, err := appClient.DispatchInferenceStream(ctx, dispatchReq)
		if err != nil {
			return nil, nil, err
		}
		return streamResult.Completion, streamResult.Progress, nil
	}
	response, _, err := appClient.DispatchInference(ctx, dispatchReq)
	return response, nil, err
}

func parseInferenceAcceptanceCases(raw string) ([]evaluation.InferenceAcceptanceCaseID, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, nil
	}
	parts := strings.Split(raw, ",")
	caseIDs := make([]evaluation.InferenceAcceptanceCaseID, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		caseIDs = append(caseIDs, evaluation.InferenceAcceptanceCaseID(part))
	}
	if len(caseIDs) == 0 {
		return nil, fmt.Errorf("evaluation: inference accept: no cases selected")
	}
	return caseIDs, nil
}

func inferenceEvalAppClient(cfg *config.Config, fileSvc fs.RuntimeFileService, authContext *auth.ClientAuthContext, deps inferenceEvalDeps) (*harnessclient.Client, error) {
	certFile, keyFile, err := resolveInferenceProbeAppCredentials(fileSvc, cfg)
	if err != nil {
		return nil, err
	}
	trustBundle := cfg.ResolvedTrustBundlePath()
	appConfig := harnessconfig.Config{
		MTLSBaseURL: cfg.OperatorHTTPURL(),
		Auth: harnessconfig.Auth{
			ClientCert: certFile,
			ClientKey:  keyFile,
			CABundle:   trustBundle,
		},
		UserID: authContext.UserID,
	}
	return deps.appClientFactory(appConfig)
}

// resolveInferenceProbeAppCredentials picks the delegated g8ee app credential
// the formation runner presents for mTLS inference dispatch. A candidate is
// only usable when its leaf certificate still chains to the gateway's
// current trust bundle: the gateway mints a brand-new PKI hierarchy on every
// fresh boot (see [PKI] Generating root CA in gateway startup logs), so a
// certificate copied out of a prior ensemble enrollment silently stops being
// trusted once the gateway restarts. Presenting it anyway does not fail
// clearly — the gateway's TLS layer rejects the handshake before the
// evaluation ever reaches the ensemble, and every assignment in the run
// comes back as an opaque PROVIDER_FAILED. Checking trust here turns that
// into one actionable error instead.
func resolveInferenceProbeAppCredentials(fileSvc fs.RuntimeFileService, cfg *config.Config) (string, string, error) {
	issuedAppCert := filepath.Join(cfg.Paths.Infra.AppCertDir, "g8ee"+constants.FileExtCert)
	issuedAppKey := filepath.Join(cfg.Paths.Infra.AppCertDir, "g8ee"+constants.FileExtKey)
	pairs := []struct{ cert, key string }{
		{os.Getenv(string(constants.EnvVar.AppCert)), os.Getenv(string(constants.EnvVar.AppKey))},
		{issuedAppCert, issuedAppKey},
		{cfg.AppCertFile("g8ee"), cfg.AppKeyFile("g8ee")},
	}
	trustBundlePath := cfg.ResolvedTrustBundlePath()
	trustBundle, err := os.ReadFile(trustBundlePath)
	if err != nil {
		return "", "", fmt.Errorf("evaluation: inference probe: read trust bundle %s: %w", trustBundlePath, err)
	}
	trustPool := x509.NewCertPool()
	if !trustPool.AppendCertsFromPEM(trustBundle) {
		return "", "", fmt.Errorf("evaluation: inference probe: trust bundle %s: %w", trustBundlePath, constants.ErrEmptyTrustBundle)
	}
	var staleCandidate string
	for _, pair := range pairs {
		if pair.cert == "" || pair.key == "" {
			continue
		}
		if _, err := os.Stat(pair.cert); err != nil {
			continue
		}
		if _, err := os.Stat(pair.key); err != nil {
			continue
		}
		certPEM, err := os.ReadFile(pair.cert)
		if err != nil {
			continue
		}
		leaf, err := certutil.ParseCertFromPEM(certPEM)
		if err != nil {
			continue
		}
		if _, err := leaf.Verify(x509.VerifyOptions{
			Roots:         trustPool,
			Intermediates: trustPool,
			KeyUsages:     []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth},
		}); err != nil {
			staleCandidate = pair.cert
			continue
		}
		return pair.cert, pair.key, nil
	}
	if staleCandidate != "" {
		return "", "", fmt.Errorf("%w: %s was not issued by the gateway's current trust bundle (%s); the gateway's PKI was regenerated since this app cert was copied out of the ensemble container. Re-run: docker cp g8e-ensemble:/root/.g8e/pki/issued/apps/g8ee.crt %s && docker cp g8e-ensemble:/root/.g8e/pki/issued/apps/g8ee.key %s", constants.ErrEvaluationAppCredentialStale, staleCandidate, trustBundlePath, issuedAppCert, issuedAppKey)
	}
	return "", "", fmt.Errorf("%w: set G8E_APP_CERT/G8E_APP_KEY or copy the enrolled apps/g8ee cert out of the ensemble container (see docs/guides/unified_stack.md)", constants.ErrEvaluationAppCredentialMissing)
}

func parseInferenceProbeRole(role string) (models.InferenceModelRole, error) {
	switch strings.ToLower(strings.TrimSpace(role)) {
	case "primary":
		return models.InferenceModelRolePrimary, nil
	case "assistant":
		return models.InferenceModelRoleAssistant, nil
	case "lite", "light":
		return models.InferenceModelRoleLite, nil
	default:
		return models.InferenceModelRoleUnspecified, constants.ErrInferenceRoleInvalid
	}
}
