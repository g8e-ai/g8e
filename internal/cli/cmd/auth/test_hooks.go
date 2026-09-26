// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package authcmd

import (
	"context"

	"github.com/g8e-ai/g8e/v2/internal/cli/auth"
	"github.com/g8e-ai/g8e/v2/internal/cli/config"
	"github.com/g8e-ai/g8e/v2/internal/services/fs"
)

// MockClientFactory returns an API client factory that always yields client.
func MockClientFactory(client APIClient) APIClientFactory {
	return func(fs.RuntimeFileService, *config.Config) (APIClient, error) {
		return client, nil
	}
}

// FailingClientFactory returns an API client factory that always returns err.
func FailingClientFactory(err error) APIClientFactory {
	return func(fs.RuntimeFileService, *config.Config) (APIClient, error) {
		return nil, err
	}
}

// PanickingClientFactory returns an API client factory that panics if called.
func PanickingClientFactory() APIClientFactory {
	return func(fs.RuntimeFileService, *config.Config) (APIClient, error) {
		panic("clientFactory should not be called when fileSvcFactory fails")
	}
}

// PanickingEnrollerFactory returns an enroller factory whose enroller panics if called.
func PanickingEnrollerFactory() EnrollerFactory {
	return func(_ auth.OutputFunc, _ fs.RuntimeFileService, _ *config.Config) Enroller {
		return panickingEnroller{}
	}
}

type panickingEnroller struct{}

func (panickingEnroller) Enroll(context.Context, auth.EnrollmentOptions) (*auth.EnrollmentResult, error) {
	panic("enrollerFactory should not be called on this code path")
}
