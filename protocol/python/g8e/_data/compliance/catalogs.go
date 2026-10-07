// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package complianceconstants

import (
	"bytes"
	_ "embed"
)

//go:embed assertion-catalog.json
var assertionCatalogJSON []byte

//go:embed framework-catalog.json
var frameworkCatalogJSON []byte

//go:embed fedramp-nist-crosswalk.json
var fedRAMPAndNISTCrosswalkJSON []byte

func AssertionCatalogJSON() []byte {
	return bytes.Clone(assertionCatalogJSON)
}

func FrameworkCatalogJSON() []byte {
	return bytes.Clone(frameworkCatalogJSON)
}

func FedRAMPAndNISTCrosswalkJSON() []byte {
	return bytes.Clone(fedRAMPAndNISTCrosswalkJSON)
}
