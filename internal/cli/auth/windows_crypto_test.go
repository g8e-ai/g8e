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

//go:build windows
// +build windows

package auth

import (
	"testing"
)

func TestGenerateWindowsCSR(t *testing.T) {
	csr, privKey, err := GenerateWindowsCSR("test-g8e-windows", false)
	if err != nil {
		t.Fatalf("GenerateWindowsCSR failed: %v", err)
	}
	if csr == "" {
		t.Fatal("CSR is empty")
	}
	if privKey == nil {
		t.Fatal("Private key is nil")
	}
}

func TestGenerateWindowsCSRWithTPM(t *testing.T) {
	csr, privKey, err := GenerateWindowsCSR("test-g8e-windows-tpm", true)
	if err != nil {
		t.Fatalf("GenerateWindowsCSR with TPM failed: %v", err)
	}
	if csr == "" {
		t.Fatal("CSR is empty")
	}
	if privKey == nil {
		t.Fatal("Private key is nil")
	}
}
