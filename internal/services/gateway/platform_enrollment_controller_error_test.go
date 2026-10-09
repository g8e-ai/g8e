// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package gateway

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/g8e-ai/g8e/v2/internal/constants"
	"github.com/g8e-ai/g8e/v2/internal/response"
	"github.com/g8e-ai/g8e/v2/internal/testutil"
	"github.com/stretchr/testify/require"
)

func TestPlatformEnrollmentController_ContentionAndCancellationErrors(t *testing.T) {
	for _, cause := range []error{constants.ErrSQLiteBusy, context.Canceled, context.DeadlineExceeded} {
		t.Run(cause.Error(), func(t *testing.T) {
			logger := testutil.NewTestLogger()
			controller := &PlatformEnrollmentController{logger: logger, responder: response.NewWriter(logger)}
			rr := httptest.NewRecorder()
			err := fmt.Errorf("submit enrollment: %w", errors.Join(constants.ErrPlatformEnrollmentGovernanceRejected, cause))
			controller.writeEnrollmentError(rr, err)
			require.Equal(t, http.StatusServiceUnavailable, rr.Code)
		})
	}
}

func TestPlatformEnrollmentController_TokenConflictUsesHTTPConflict(t *testing.T) {
	logger := testutil.NewTestLogger()
	controller := &PlatformEnrollmentController{logger: logger, responder: response.NewWriter(logger)}
	rr := httptest.NewRecorder()
	controller.writeEnrollmentError(rr, constants.ErrPlatformEnrollmentTokenConflict)
	require.Equal(t, http.StatusConflict, rr.Code)
}
