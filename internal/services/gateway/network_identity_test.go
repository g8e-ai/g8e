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

//go:build integration

package gateway

import (
	"encoding/json"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/g8e-ai/g8e/internal/constants"
	"github.com/g8e-ai/g8e/internal/services/network"
	"github.com/g8e-ai/g8e/internal/testutil"
)

func TestResolveGatewayCertificateIdentity(t *testing.T) {
	tests := []struct {
		name         string
		certMode     string
		identityFile string
		setupFile    func(t *testing.T, dir string) string
		wantErr      bool
		errContains  string
		checkResult  func(t *testing.T, extraIPs []net.IP, extraDNSNames []string)
	}{
		{
			name:     "loads identity from file",
			certMode: "full",
			setupFile: func(t *testing.T, dir string) string {
				identityFile := filepath.Join(dir, constants.NetworkIdentityFilename)
				identity := network.NetworkIdentity{
					IPs:       []string{"192.0.2.10", "2001:db8::10"},
					Hostnames: []string{"gateway.test.internal"},
				}
				identityData, err := json.Marshal(identity)
				require.NoError(t, err)
				require.NoError(t, os.WriteFile(identityFile, identityData, 0600))
				return identityFile
			},
			wantErr: false,
			checkResult: func(t *testing.T, extraIPs []net.IP, extraDNSNames []string) {
				assert.Equal(t, []net.IP{net.ParseIP("192.0.2.10")}, extraIPs)
				assert.Contains(t, extraDNSNames, "gateway.test.internal")
				assert.Contains(t, extraDNSNames, "localhost")
			},
		},
		{
			name:         "falls back to detection when file missing",
			certMode:     "full",
			identityFile: "",
			wantErr:      false,
			checkResult: func(t *testing.T, extraIPs []net.IP, extraDNSNames []string) {
				assert.NotEmpty(t, extraIPs, "should detect at least one IP")
				assert.NotEmpty(t, extraDNSNames, "should have DNS names")
				assert.Contains(t, extraDNSNames, "localhost")
			},
		},
		{
			name:         "localhost filters loopback only",
			certMode:     "localhost",
			identityFile: "",
			wantErr:      false,
			checkResult: func(t *testing.T, extraIPs []net.IP, extraDNSNames []string) {
				for _, ip := range extraIPs {
					assert.True(t, ip.IsLoopback(), "all IPs should be loopback")
				}
				assert.Equal(t, []string{"localhost"}, extraDNSNames)
			},
		},
		{
			name:     "malformed identity file",
			certMode: "full",
			setupFile: func(t *testing.T, dir string) string {
				identityFile := filepath.Join(dir, constants.NetworkIdentityFilename)
				require.NoError(t, os.WriteFile(identityFile, []byte("not-json"), 0600))
				return identityFile
			},
			wantErr:     true,
			errContains: "unmarshal",
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			logger := testutil.NewTestLogger()
			dir := testutil.TempDir(t)
			identityFile := tt.identityFile

			if tt.setupFile != nil {
				identityFile = tt.setupFile(t, dir)
			}

			detector := network.NewDetector(logger)
			extraIPs, extraDNSNames, err := resolveGatewayCertificateIdentity(tt.certMode, identityFile, detector, logger)

			if tt.wantErr {
				require.Error(t, err)
				if tt.errContains != "" {
					assert.Contains(t, strings.ToLower(err.Error()), tt.errContains)
				}
			} else {
				require.NoError(t, err)
				if tt.checkResult != nil {
					tt.checkResult(t, extraIPs, extraDNSNames)
				}
			}
		})
	}
}
