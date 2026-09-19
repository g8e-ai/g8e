// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package output

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"

	"github.com/spf13/cobra"
)

const jsonIndent = "  "

// JSONEnabled reports whether machine-readable JSON output was requested via
// the global --json flag (g8e --json … or … --json on any subcommand).
func JSONEnabled(cmd *cobra.Command) bool {
	if cmd == nil {
		return false
	}
	root := cmd.Root()
	if root == nil {
		return false
	}
	value, err := root.PersistentFlags().GetBool("json")
	return err == nil && value
}

// WriteJSON writes v as pretty-printed JSON followed by a newline.
func WriteJSON(w io.Writer, v any) error {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	enc.SetIndent("", jsonIndent)
	if err := enc.Encode(v); err != nil {
		return fmt.Errorf("marshal json: %w", err)
	}
	if _, err := io.Copy(w, &buf); err != nil {
		return fmt.Errorf("write json: %w", err)
	}
	return nil
}

// WriteRawJSON pretty-prints raw JSON bytes. Invalid JSON is written as-is.
func WriteRawJSON(w io.Writer, data []byte) error {
	trimmed := bytes.TrimSpace(data)
	if len(trimmed) == 0 {
		_, err := fmt.Fprintln(w)
		return err
	}

	var decoded any
	if err := json.Unmarshal(trimmed, &decoded); err != nil {
		_, err := fmt.Fprintln(w, string(trimmed))
		return err
	}
	return WriteJSON(w, decoded)
}
