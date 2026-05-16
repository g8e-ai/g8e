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
	"bufio"
	"fmt"
	"io"
	"os"
	"strings"
	"syscall"
	"unsafe"
)

// promptForAPIKey prompts for an API key with obfuscated (starred) terminal input.
func promptForAPIKey() (string, error) {
	fmt.Print("Enter the unique API key for one of your g8e Operators: ")

	fd := int(os.Stdin.Fd())

	var oldState syscall.Termios
	if _, _, err := syscall.Syscall6(syscall.SYS_IOCTL, uintptr(fd), syscall.TCGETS, uintptr(unsafe.Pointer(&oldState)), 0, 0, 0); err != 0 {
		reader := bufio.NewReader(os.Stdin)
		apiKey, err := reader.ReadString('\n')
		if err != nil {
			return "", err
		}
		return strings.TrimSpace(apiKey), nil
	}

	newState := oldState
	newState.Lflag &^= syscall.ECHO | syscall.ICANON
	newState.Cc[syscall.VMIN] = 1
	newState.Cc[syscall.VTIME] = 0

	if _, _, err := syscall.Syscall6(syscall.SYS_IOCTL, uintptr(fd), syscall.TCSETS, uintptr(unsafe.Pointer(&newState)), 0, 0, 0); err != 0 {
		reader := bufio.NewReader(os.Stdin)
		apiKey, err := reader.ReadString('\n')
		if err != nil {
			return "", err
		}
		return strings.TrimSpace(apiKey), nil
	}

	defer syscall.Syscall6(syscall.SYS_IOCTL, uintptr(fd), syscall.TCSETS, uintptr(unsafe.Pointer(&oldState)), 0, 0, 0)

	return readObfuscatedInput(os.Stdin, os.Stdout)
}

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
			return "", fmt.Errorf("interrupted")
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
