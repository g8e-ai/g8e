// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package eval

import (
	"fmt"

	"github.com/spf13/cobra"

	authcmd "github.com/g8e-ai/g8e/v2/internal/cli/cmd/auth"
	"github.com/g8e-ai/g8e/v2/internal/services/evaluation"
)

type governedModelMaintenanceEnv struct {
	Maintenance         evaluation.OllamaModelMaintenanceContext
	ModelDispatcher     evaluation.OllamaModelCommandDispatcher
	InferenceDispatcher evaluation.FormationInferenceDispatcher
	ProbeRunner         evaluation.GovernedCapabilityProbeRunner
}

func resolveGovernedModelMaintenance(
	cmd *cobra.Command,
	deps nativeEvalDeps,
	inferenceSessionID string,
	dataSessionID string,
) (governedModelMaintenanceEnv, error) {
	if inferenceSessionID == "" {
		return governedModelMaintenanceEnv{}, fmt.Errorf("evaluation: models maintenance: --inference-session is required")
	}
	if dataSessionID == "" {
		return governedModelMaintenanceEnv{}, fmt.Errorf("evaluation: models maintenance: --data-session is required")
	}
	cfg, fileSvc, authContext, err := chatEvalEnvironment(cmd, chatEvalDeps{
		configLoader:         deps.configLoader,
		fileSvcFactory:       deps.fileSvcFactory,
		authLoader:           deps.authLoader,
		clientFactory:        deps.clientFactory,
		refreshClientFactory: authcmd.DefaultRefreshClientFactory,
		now:                  deps.now,
		newID:                deps.newID,
	})
	if err != nil {
		return governedModelMaintenanceEnv{}, err
	}
	operators, err := chatEvalListOperators(cmd, chatEvalDeps{
		configLoader:   deps.configLoader,
		fileSvcFactory: deps.fileSvcFactory,
		authLoader:     deps.authLoader,
		clientFactory:  deps.clientFactory,
		now:            deps.now,
		newID:          deps.newID,
	}, cfg, authContext)
	if err != nil {
		return governedModelMaintenanceEnv{}, err
	}
	sessions, err := resolveCampaignOperatorSessions(cmd, deps, cfg, inferenceSessionID, dataSessionID)
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
	modelDispatcher, err := newHarnessOllamaModelCommandDispatcher(cfg, authContext, dataOperator, chatEvalDeps{
		configLoader:   deps.configLoader,
		fileSvcFactory: deps.fileSvcFactory,
		authLoader:     deps.authLoader,
		clientFactory:  deps.clientFactory,
		now:            deps.now,
		newID:          deps.newID,
	})
	if err != nil {
		return governedModelMaintenanceEnv{}, err
	}
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
		return governedModelMaintenanceEnv{}, err
	}
	inferenceDispatcher := &harnessFormationInferenceDispatcher{client: appClient}
	prefixedNewID := func(prefix string) string { return prefix + "-" + deps.newID() }
	return governedModelMaintenanceEnv{
		Maintenance: evaluation.OllamaModelMaintenanceContext{
			TargetOperatorSessionID: sessions.InferenceSessionID,
			Environment:             modelCommandEnvironment(endpoint),
			CaseID:                  "eval-models-maintenance",
			NewID:                   prefixedNewID,
		},
		ModelDispatcher:     modelDispatcher,
		InferenceDispatcher: inferenceDispatcher,
		ProbeRunner:         evaluation.NewGovernedCapabilityProbeRunner(inferenceDispatcher, sessions.InferenceSessionID, prefixedNewID),
	}, nil
}
