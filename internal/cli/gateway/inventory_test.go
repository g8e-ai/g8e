// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package gateway

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/g8e-ai/g8e/v2/internal/constants"
	"github.com/g8e-ai/g8e/v2/internal/models"
)

func TestFetchEnrolled(t *testing.T) {
	t.Run("filters completed enrollments and uses the canonical path", func(t *testing.T) {
		var gotPath string
		rows, ok := FetchEnrolled(func(path string) ([]byte, error) {
			gotPath = path
			return []byte(`{"enrollments":[{"request_id":"done","state":"completed"},{"request_id":"revoked","state":"revoked"}]}`), nil
		})
		require.True(t, ok)
		require.Len(t, rows, 1)
		assert.Equal(t, "done", rows[0].RequestID)
		assert.Equal(t, constants.APIPaths.AuthPlatformEnrollmentEnrolled, gotPath)
	})

	t.Run("reports request and decode failures", func(t *testing.T) {
		rows, ok := FetchEnrolled(func(string) ([]byte, error) { return nil, errors.New("offline") })
		assert.False(t, ok)
		assert.Nil(t, rows)

		rows, ok = FetchEnrolled(func(string) ([]byte, error) { return []byte("not json"), nil })
		assert.False(t, ok)
		assert.Nil(t, rows)
	})
}

func TestCompletedEnrollments(t *testing.T) {
	rows := CompletedEnrollments([]models.PlatformEnrollmentEnrolledRequest{
		{RequestID: "one", State: models.PlatformEnrollmentStateCompleted},
		{RequestID: "two", State: models.PlatformEnrollmentStateRevoked},
	})
	require.Len(t, rows, 1)
	assert.Equal(t, "one", rows[0].RequestID)
}
