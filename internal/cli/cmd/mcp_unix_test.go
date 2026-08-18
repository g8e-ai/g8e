// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

//go:build !windows

package cmd

import (
	"os/exec"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestSetSysProcAttr(t *testing.T) {
	cmd := exec.Command("echo", "test")
	setSysProcAttr(cmd)

	assert.NotNil(t, cmd.SysProcAttr)
	assert.True(t, cmd.SysProcAttr.Setpgid)
}
