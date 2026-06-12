//go:build linux
// +build linux

// Copyright (c) 2026 Lateralus Labs, LLC.
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package main

import (
	"errors"
	"fmt"
	"io"

	"github.com/g8e-ai/g8e/internal/constants"
)

// readObfuscatedInput reads a password-style input from r, writing masked feedback
// to w. Each printable character echoes as '*'. Backspace/Delete removes the last
// character. Enter (\r or \n) submits. Ctrl+C returns an error.
func readObfuscatedInput(r io.Reader, w io.Writer) (string, error) {
	var input []byte
	buf := make([]byte, 1)

	for {
		n, err := r.Read(buf)
		if err != nil {
			fmt.Fprintln(w)
			return "", err
		}
		if n == 0 {
			continue
		}

		char := buf[0]

		if char == '\r' || char == '\n' {
			fmt.Fprintln(w)
			break
		}

		if char == 3 {
			fmt.Fprintln(w)
			return "", errors.New(string(constants.CommandExitStatusInterrupted))
		}

		if char == 127 || char == 8 {
			if len(input) > 0 {
				input = input[:len(input)-1]
				fmt.Fprint(w, "\b \b")
			}
			continue
		}

		if char >= 32 && char <= 126 {
			input = append(input, char)
			fmt.Fprint(w, "*")
		}
	}

	return string(input), nil
}
