// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

// Package identityreset removes only installed workload enrollment identities.
package identityreset

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	runtimepaths "github.com/g8e-ai/g8e/v2/internal/constants"
)

func paths(component string) ([]string, error) {
	common := []string{"pki/trust/g8eg-ca-bundle.pem", "pki/trust/root.pem", "pki/trust/operator-bundle.pem", "pki/trust/trust-domain.json"}
	switch component {
	case "operator":
		return append(common, "credentials", "cli.crt", "cli.key", "pki/operator.crt", "pki/operator.key", "pki/cli.crt", "pki/cli.key", "pki/pending-enrollment/g8eo.json", "pki/trusted_signers", "pki/Actuator_pub.pem", "pki/Actuator_pub.json"), nil
	case "ensemble":
		return append(common, "pki/issued/apps/g8ee.crt", "pki/issued/apps/g8ee.key", "pki/pending-enrollment/g8ee.json"), nil
	default:
		return nil, fmt.Errorf("unknown workload component %q", component)
	}
}

// Reset uses a confined filesystem handle and refuses symlinked identity paths.
// Databases, vault keys, configuration, logs, and other applications are retained.
// Callers must stop the selected workload before invoking Reset.
func Reset(directory, component string) error {
	selected, err := paths(component)
	if err != nil {
		return err
	}
	runtime := filepath.Join(directory, runtimepaths.RuntimeDirname)
	info, err := os.Lstat(runtime)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("refusing non-directory or symlink runtime: %s", runtime)
	}
	root, err := os.OpenRoot(runtime)
	if err != nil {
		return err
	}
	defer root.Close()
	if _, err := root.Stat("pki/root/root_ca.crt"); err == nil {
		return fmt.Errorf("refusing to reset a gateway runtime: %s", runtime)
	} else if !os.IsNotExist(err) {
		return err
	}
	// Validate the entire selection before deleting anything.
	for _, name := range selected {
		parts := strings.Split(name, "/")
		for i := range parts {
			path := strings.Join(parts[:i+1], "/")
			info, err := root.Lstat(path)
			if os.IsNotExist(err) {
				break
			}
			if err != nil {
				return err
			}
			if info.Mode()&os.ModeSymlink != 0 {
				return fmt.Errorf("refusing symlink identity path: %s", path)
			}
		}
	}
	for _, name := range selected {
		if err := root.RemoveAll(name); err != nil {
			return fmt.Errorf("reset identity %s: %w", name, err)
		}
	}
	return nil
}
