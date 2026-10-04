//go:build !linux

// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package operatorcmd

import "fmt"

func discoverLocalOperators() ([]localOperatorProcess, error) {
	return nil, fmt.Errorf("operator stop: local worker discovery is currently supported on Linux only")
}
