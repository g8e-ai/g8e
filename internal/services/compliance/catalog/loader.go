// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package catalog

import (
	"fmt"

	"google.golang.org/protobuf/proto"

	"github.com/g8e-ai/g8e/v2/internal/constants"
	complianceconstants "github.com/g8e-ai/g8e/v2/protocol/constants/compliance"
	compliancev1 "github.com/g8e-ai/g8e/v2/protocol/proto/g8e/compliance/v1"
)

func LoadCanonicalCatalogs() (*compliancev1.ControlAssertionCatalog, *compliancev1.FrameworkCatalog, *compliancev1.ControlCrosswalkCatalog, error) {
	assertions := &compliancev1.ControlAssertionCatalog{}
	if err := compliancev1.UnmarshalCanonical(complianceconstants.AssertionCatalogJSON(), assertions); err != nil {
		return nil, nil, nil, fmt.Errorf("%w: parse assertion catalog: %v", constants.ErrInvalidEvidenceGraph, err)
	}
	frameworks := &compliancev1.FrameworkCatalog{}
	if err := compliancev1.UnmarshalCanonical(complianceconstants.FrameworkCatalogJSON(), frameworks); err != nil {
		return nil, nil, nil, fmt.Errorf("%w: parse framework catalog: %v", constants.ErrInvalidEvidenceGraph, err)
	}
	crosswalks := &compliancev1.ControlCrosswalkCatalog{}
	if err := compliancev1.UnmarshalCanonical(complianceconstants.FedRAMPAndNISTCrosswalkJSON(), crosswalks); err != nil {
		return nil, nil, nil, fmt.Errorf("%w: parse crosswalk catalog: %v", constants.ErrInvalidEvidenceGraph, err)
	}
	if err := ValidateCatalogSet(assertions, frameworks, crosswalks); err != nil {
		return nil, nil, nil, err
	}
	if err := verifyCatalogDigest(assertions.Sha256, assertions); err != nil {
		return nil, nil, nil, err
	}
	for _, framework := range frameworks.Frameworks {
		if err := verifyCatalogDigest(framework.CatalogSha256, framework); err != nil {
			return nil, nil, nil, err
		}
	}
	if err := verifyCatalogDigest(frameworks.Sha256, frameworks); err != nil {
		return nil, nil, nil, err
	}
	if err := verifyCatalogDigest(crosswalks.Sha256, crosswalks); err != nil {
		return nil, nil, nil, err
	}
	return assertions, frameworks, crosswalks, nil
}

func LoadDemoScenarioCatalog(assertions *compliancev1.ControlAssertionCatalog, frameworks *compliancev1.FrameworkCatalog) (*compliancev1.DemoScenarioCatalog, error) {
	catalog := &compliancev1.DemoScenarioCatalog{}
	if err := compliancev1.UnmarshalCanonical(complianceconstants.DemoScenarioCatalogJSON(), catalog); err != nil {
		return nil, fmt.Errorf("%w: parse demo scenario catalog: %v", constants.ErrInvalidEvidenceGraph, err)
	}
	if err := ValidateDemoScenarioCatalog(catalog, assertions, frameworks); err != nil {
		return nil, err
	}
	if err := verifyCatalogDigest(catalog.Sha256, catalog); err != nil {
		return nil, err
	}
	return catalog, nil
}

func verifyCatalogDigest(expected string, message proto.Message) error {
	actual, err := CatalogDigest(message)
	if err != nil {
		return err
	}
	if actual != expected {
		return fmt.Errorf("%w: catalog digest: expected %s, got %s", constants.ErrChecksumMismatch, expected, actual)
	}
	return nil
}
