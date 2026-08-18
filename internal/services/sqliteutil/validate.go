// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package sqliteutil

import (
	"fmt"
	"regexp"

	"github.com/g8e-ai/g8e/internal/constants"
)

// validIdentifierRe guards against SQL injection when field names must be
// interpolated into queries (e.g., json_extract paths, ORDER BY columns).
var validIdentifierRe = regexp.MustCompile(`^[a-zA-Z_][a-zA-Z0-9_]*$`)

func ValidateIdentifier(name string) error {
	if name == "" {
		return fmt.Errorf("sqliteutil: validate identifier: %w", constants.ErrSQLiteValidateEmptyIdentifier)
	}
	if !validIdentifierRe.MatchString(name) {
		return fmt.Errorf("sqliteutil: validate identifier %q: %w", name, constants.ErrSQLiteValidateInvalidPattern)
	}
	return nil
}
