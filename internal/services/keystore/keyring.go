// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package keystore

// Keyring abstracts the protected store that holds the master encryption key.
// Implementations wrap the macOS Keychain (darwin), the Secret Service via
// libsecret (linux), DPAPI (windows), or an operator-provisioned key file kept
// outside the runtime directory, plus an in-memory implementation for tests.
// There is deliberately no implementation that writes the master key in
// plaintext next to the secrets it protects.
type Keyring interface {
	// RetrieveMasterKey retrieves the master encryption key from the keyring.
	RetrieveMasterKey() ([]byte, error)
	// StoreMasterKey stores the master encryption key in the keyring.
	StoreMasterKey(key []byte) error
	// DeleteMasterKey removes the master encryption key from the keyring.
	DeleteMasterKey() error
	// Name returns the human-readable name of the keyring implementation.
	Name() string
}
