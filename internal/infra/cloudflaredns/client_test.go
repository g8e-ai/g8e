// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package cloudflaredns

import (
	"net/http"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestZoneNameForHostname(t *testing.T) {
	zone, err := zoneNameForHostname("opendevops.ai")
	require.NoError(t, err)
	assert.Equal(t, "opendevops.ai", zone)

	zone, err = zoneNameForHostname("www.opendevops.ai")
	require.NoError(t, err)
	assert.Equal(t, "opendevops.ai", zone)
}

func TestDNSRecordName(t *testing.T) {
	assert.Equal(t, "opendevops.ai", dnsRecordName("opendevops.ai"))
	assert.Equal(t, "www", dnsRecordName("www.opendevops.ai"))
}

type rewriteTransport struct {
	base   http.RoundTripper
	target string
}

func (t *rewriteTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	req.URL.Scheme = "http"
	req.URL.Host = strings.TrimPrefix(strings.TrimPrefix(t.target, "https://"), "http://")
	return t.base.RoundTrip(req)
}
