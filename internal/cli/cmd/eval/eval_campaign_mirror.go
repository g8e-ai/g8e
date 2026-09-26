// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package eval

import (
	"context"
	"fmt"
	"time"

	"github.com/g8e-ai/g8e/v2/internal/cli/cmd/gwremote"
	"github.com/g8e-ai/g8e/v2/internal/cli/config"
	"github.com/g8e-ai/g8e/v2/internal/services/evaluation"
	"github.com/g8e-ai/g8e/v2/internal/services/fs"
)

func ReconcileVerifiedCampaignMirrorQueue(ctx context.Context, projectRoot string, reconciler *evaluation.CampaignMirrorReconciler, runTimeout time.Duration, force bool, progress evaluation.CampaignMirrorReconcileProgressFunc) (*evaluation.CampaignMirrorReconcileResult, error) {
	fileSvc, err := fs.NewRuntimeFileService(projectRoot, nil)
	if err != nil {
		return nil, fmt.Errorf("evaluation: campaign mirror queue file service: %w", err)
	}
	queue, err := evaluation.LoadInitCampaignQueueFromRuntime(ctx, fileSvc, evaluation.DefaultInitCampaignQueueRelPath)
	if err != nil {
		return nil, err
	}
	return reconciler.ReconcileVerifiedQueue(ctx, queue, runTimeout, true, force, progress)
}

const DockerInitCampaignMirrorRestoreTimeout = 10 * time.Second

func runDockerInitCampaignMirrorRestore(ctx context.Context, timeout time.Duration, restore func(context.Context) (*evaluation.CampaignMirrorReconcileResult, error)) (*evaluation.CampaignMirrorReconcileResult, error) {
	restoreCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	result, err := restore(restoreCtx)
	if restoreCtx.Err() != nil {
		return nil, fmt.Errorf("docker init: verified campaign mirror restore timed out after %s: %w", timeout, restoreCtx.Err())
	}
	return result, err
}

func ReconcileVerifiedCampaignMirrorFromDockerInit(ctx context.Context, fileSvc fs.RuntimeFileService, cfg *config.Config) (*evaluation.CampaignMirrorReconcileResult, error) {
	return runDockerInitCampaignMirrorRestore(ctx, DockerInitCampaignMirrorRestoreTimeout, func(restoreCtx context.Context) (*evaluation.CampaignMirrorReconcileResult, error) {
		if !gwremote.IsGatewayHealthy() {
			return nil, fmt.Errorf("gateway is not healthy")
		}
		publication, err := newCampaignPublicationCoordinatorFromConfig(restoreCtx, fileSvc, cfg)
		if err != nil {
			return nil, err
		}
		mirrorProbe := gwremote.NewHTTPCampaignMirrorProbe(restoreCtx)
		publication.WithMirrorProbe(mirrorProbe)
		reconciler := evaluation.NewCampaignMirrorReconciler(
			publication,
			evaluation.NewStore(fileSvc),
			mirrorProbe,
		)
		queue, err := evaluation.LoadInitCampaignQueueFromRuntime(restoreCtx, fileSvc, evaluation.DefaultInitCampaignQueueRelPath)
		if err != nil {
			return nil, err
		}
		return reconciler.ReconcileVerifiedQueue(restoreCtx, queue, DockerInitCampaignMirrorRestoreTimeout, false, false, nil)
	})
}

func newCampaignPublicationCoordinatorFromConfig(ctx context.Context, fileSvc fs.RuntimeFileService, cfg *config.Config) (*evaluation.CampaignPublicationCoordinator, error) {
	exporter, err := gwremote.NewCampaignFeedExporter(ctx, fileSvc, cfg)
	if err != nil {
		return nil, fmt.Errorf("campaign publication: %w", err)
	}
	proofPublisher, err := gwremote.NewCampaignProofPublisher(ctx, fileSvc, cfg)
	if err != nil {
		return nil, fmt.Errorf("campaign publication: %w", err)
	}
	remote, err := gwremote.NewProviderObservationRemote(fileSvc, cfg)
	if err != nil {
		return nil, fmt.Errorf("campaign publication: provider observation remote: %w", err)
	}
	publicationState, err := newGatewayCampaignPublicationStateStoreFromConfig(fileSvc, cfg)
	if err != nil {
		return nil, err
	}
	return evaluation.NewCampaignPublicationCoordinator(
		evaluation.NewStore(fileSvc),
		fileSvc,
		publicationState,
		exporter,
		remote,
	).WithProofPublisher(proofPublisher).WithMirrorProbe(gwremote.NewHTTPCampaignMirrorProbe(ctx)), nil
}
