// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/g8e-ai/g8e/v2/internal/constants"
	"github.com/g8e-ai/g8e/v2/internal/services/compliance/catalog"
	compliancev1 "github.com/g8e-ai/g8e/v2/protocol/proto/g8e/compliance/v1"
)

type doctrineCatalog struct {
	Doctrines []doctrine `json:"doctrines"`
}

type doctrine struct {
	ID            string                             `json:"id"`
	KSIIDs        []string                           `json:"ksi_ids"`
	ControlIDs    []string                           `json:"control_ids"`
	AssertionRefs []*compliancev1.VersionedReference `json:"assertion_refs"`
}

func main() {
	if err := validate(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func validate() error {
	assertions, frameworks, _, err := catalog.LoadCanonicalCatalogs()
	if err != nil {
		return fmt.Errorf("doctrine references: load compliance catalogs: %w", err)
	}
	doctrinePath := filepath.Join(constants.DemosDirname, constants.DemosOrgFedRAMP, constants.DemosDoctrineDir, constants.DemosFedRAMPDoctrineFile)
	encoded, err := os.ReadFile(doctrinePath)
	if err != nil {
		return fmt.Errorf("doctrine references: read %s: %w", doctrinePath, err)
	}
	var doctrines doctrineCatalog
	if err := json.Unmarshal(encoded, &doctrines); err != nil {
		return fmt.Errorf("doctrine references: parse %s: %w", doctrinePath, err)
	}
	for _, doctrine := range doctrines.Doctrines {
		if doctrine.ID == "" || len(doctrine.AssertionRefs) == 0 {
			return fmt.Errorf("doctrine references: doctrine %q has no assertion references", doctrine.ID)
		}
		for _, controlID := range append(append([]string{}, doctrine.KSIIDs...), doctrine.ControlIDs...) {
			if !frameworkControlExists(frameworks, controlID) {
				return fmt.Errorf("doctrine references: doctrine %s references unknown framework control %s", doctrine.ID, controlID)
			}
		}
		for _, reference := range doctrine.AssertionRefs {
			if reference == nil || catalog.FindAssertion(assertions, reference.Id, reference.Version) == nil {
				return fmt.Errorf("doctrine references: doctrine %s references unknown assertion", doctrine.ID)
			}
		}
	}
	return nil
}

func frameworkControlExists(frameworks *compliancev1.FrameworkCatalog, controlID string) bool {
	for _, framework := range frameworks.Frameworks {
		if catalog.FindFrameworkControl(framework, controlID) != nil {
			return true
		}
	}
	return false
}
