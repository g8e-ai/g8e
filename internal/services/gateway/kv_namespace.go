// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package gateway

import (
	"net/http"
	"strings"

	"github.com/g8e-ai/g8e/v2/internal/constants"
)

type kvNamespace string

const (
	kvDocumentCache kvNamespace = "g8e:cache:doc:"
	kvQueryCache    kvNamespace = "g8e:cache:query:"
)

// authorizeKVNamespace admits only authenticated cache access. Patterns must
// contain the entire literal namespace before any glob metacharacter.
// Gateway services use KVStoreService directly for their private state.
func authorizeKVNamespace(r *http.Request, keyOrPattern string) error {
	var authenticated bool
	for _, key := range []constants.ContextKey{constants.ContextKeyAppID, constants.ContextKeyUserID, constants.ContextKeyOperatorID, constants.ContextKeyCLISessionID} {
		identity, _ := r.Context().Value(key).(string)
		authenticated = authenticated || identity != ""
	}
	if !authenticated {
		return constants.ErrUnauthorizedNoIdentity
	}
	for _, namespace := range []kvNamespace{kvDocumentCache, kvQueryCache} {
		if strings.HasPrefix(keyOrPattern, string(namespace)) {
			return nil
		}
	}
	return constants.ErrKVNamespaceForbidden
}
