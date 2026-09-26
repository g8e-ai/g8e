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

// StubRefreshClient is a test refreshClient that returns a canned result.
// It lives outside _test.go so other command packages can inject it.
type StubRefreshClient struct {
	Result auth.CLISessionRefresh
	Err    error
	called bool
}

func (s *StubRefreshClient) Refresh(ctx context.Context, fileSvc fs.RuntimeFileService) (auth.CLISessionRefresh, error) {
	s.called = true
	return s.Result, s.Err
}

// Called reports whether Refresh ran.
func (s *StubRefreshClient) Called() bool { return s.called }

// StubRefreshClientFactory returns a RefreshClientFactory that always yields stub.
func StubRefreshClientFactory(stub *StubRefreshClient) RefreshClientFactory {
	return func(cfg *config.Config) refreshClient { return stub }
}
