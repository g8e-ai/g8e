// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package governance

//go:generate mockery --name StateRootProvider --output ./mocks --dir .

// StateRootProvider defines the interface for obtaining the current state root.
type StateRootProvider interface {
	GetCurrentStateRoot() (string, error)
}
