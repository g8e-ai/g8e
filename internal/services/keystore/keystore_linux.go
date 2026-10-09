// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package keystore

import (
	"github.com/g8e-ai/g8e/v2/internal/services/fs"
)

// platformKeyring returns the Secret Service keyring. Hosts without one
// (headless servers, containers) must provision --master-key-file instead.
func platformKeyring(fs.RuntimeFileService) (Keyring, error) {
	return newLibsecretKeyring(runSecretTool)
}
