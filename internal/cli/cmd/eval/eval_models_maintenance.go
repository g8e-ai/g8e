// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package eval

import (
	"github.com/spf13/cobra"

	"github.com/g8e-ai/g8e/v2/internal/services/evaluation"
)

type governedModelMaintenanceEnv struct {
	Maintenance     evaluation.OllamaModelMaintenanceContext
	ModelDispatcher evaluation.OllamaModelCommandDispatcher

	// ResolveProbeRunner resolves the delegated g8ee app credential and
	// builds a capability-probe runner on demand. Only capability probing
	// dispatches governed inference (a distinct, app-credentialed Gateway
	// surface per docs/guides/connect_apps_to_gateway.md); plain registry
	// maintenance (freeze/pull/diff) never touches it, so callers must not
	// force app-credential resolution unless they actually run probes.
	ResolveProbeRunner func() (evaluation.GovernedCapabilityProbeRunner, error)
}

// resolveGovernedModelMaintenance binds provider maintenance to the resolved
// inference and data operator sessions.
func resolveGovernedModelMaintenance(cmd *cobra.Command, deps nativeEvalDeps) (governedModelMaintenanceEnv, error) {
	inferencePin, dataPin, err := sessionPinsFromFlags(cmd)
	if err != nil {
		return governedModelMaintenanceEnv{}, err
	}
	chatDeps := deps.chatDeps()
	cfg, fileSvc, authContext, err := chatEvalEnvironment(cmd, chatDeps)
	if err != nil {
		return governedModelMaintenanceEnv{}, err
	}
	operators, err := chatEvalListOperators(cmd, chatDeps, cfg, authContext)
	if err != nil {
		return governedModelMaintenanceEnv{}, err
	}
	sessions, err := resolveOperatorSessionsFrom(operators, inferencePin, dataPin, operatorRoleInference, operatorRoleData)
	if err != nil {
		return governedModelMaintenanceEnv{}, err
	}
	endpoint, err := evaluation.GovernedInferenceOllamaEndpoint(operators, sessions.InferenceSessionID)
	if err != nil {
		return governedModelMaintenanceEnv{}, err
	}
	dataOperator, err := evaluation.SelectCampaignDataOperator(operators, sessions.DataSessionID)
	if err != nil {
		return governedModelMaintenanceEnv{}, err
	}
	modelDispatcher, err := newHarnessOllamaModelCommandDispatcher(cfg, authContext, dataOperator, chatDeps)
	if err != nil {
		return governedModelMaintenanceEnv{}, err
	}
	prefixedNewID := func(prefix string) string { return prefix + "-" + deps.newID() }
	inferenceSessionID := sessions.InferenceSessionID
	return governedModelMaintenanceEnv{
		Maintenance: evaluation.OllamaModelMaintenanceContext{
			TargetOperatorSessionID: inferenceSessionID,
			Environment:             modelCommandEnvironment(endpoint),
			CaseID:                  "eval-models-maintenance",
			NewID:                   prefixedNewID,
		},
		ModelDispatcher: modelDispatcher,
		ResolveProbeRunner: func() (evaluation.GovernedCapabilityProbeRunner, error) {
			appClient, err := inferenceEvalAppClient(cfg, fileSvc, authContext, inferenceEvalDeps{
				configLoader:     deps.configLoader,
				fileSvcFactory:   deps.fileSvcFactory,
				authLoader:       deps.authLoader,
				clientFactory:    deps.clientFactory,
				appClientFactory: deps.clientFactory,
				now:              deps.now,
				newID:            deps.newID,
			})
			if err != nil {
				return nil, err
			}
			inferenceDispatcher := &harnessFormationInferenceDispatcher{client: appClient}
			return evaluation.NewGovernedCapabilityProbeRunner(inferenceDispatcher, inferenceSessionID, prefixedNewID), nil
		},
	}, nil
}
