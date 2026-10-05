// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package scenarios

import (
	"testing"

	"github.com/stretchr/testify/assert"

	clientpkg "github.com/g8e-ai/g8e/v2/internal/tools/agent_harness/client"
)

func TestVerifyReceiptIdentity_RejectsEmptyOrMismatchedRequestorUserID(t *testing.T) {
	oldKit := kit
	defer func() { kit = oldKit }()

	kit = &GovKit{UserID: "expected-user-123"}
	res := &Result{}

	t.Run("empty requestor_user_id fails", func(t *testing.T) {
		rec := &clientpkg.Receipt{
			TransactionID:   "tx-1",
			RequestorUserID: "",
			ActingAppID:     "app-1",
		}
		err := verifyReceiptIdentity(res, rec)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "receipt requestor_user_id is empty")
	})

	t.Run("mismatched requestor_user_id fails", func(t *testing.T) {
		rec := &clientpkg.Receipt{
			TransactionID:   "tx-1",
			RequestorUserID: "wrong-user",
			ActingAppID:     "app-1",
		}
		err := verifyReceiptIdentity(res, rec)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "receipt requestor_user_id=\"wrong-user\" != expected=\"expected-user-123\"")
	})

	t.Run("empty acting_app_id fails", func(t *testing.T) {
		rec := &clientpkg.Receipt{
			TransactionID:   "tx-1",
			RequestorUserID: "expected-user-123",
			ActingAppID:     "",
		}
		err := verifyReceiptIdentity(res, rec)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "receipt acting_app_id is empty")
	})

	t.Run("valid matching identity passes", func(t *testing.T) {
		rec := &clientpkg.Receipt{
			TransactionID:   "tx-1",
			RequestorUserID: "expected-user-123",
			ActingAppID:     "app-1",
		}
		err := verifyReceiptIdentity(res, rec)
		assert.NoError(t, err)
	})
}
