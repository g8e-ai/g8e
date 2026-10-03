// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package system

import (
	"net"
	"time"

	"github.com/g8e-ai/g8e/v2/internal/models"
)

// Clock is an injectable time source for deterministic testing.
type Clock interface {
	Now() time.Time
}

// RealClock uses actual wall time.
type RealClock struct{}

func (c *RealClock) Now() time.Time {
	return time.Now().UTC()
}

// GetConnectivityStatus returns network interfaces that are up and not loopback.
func GetConnectivityStatus() []models.HeartbeatNetworkInterface {
	interfaces, err := net.Interfaces()
	if err != nil {
		return []models.HeartbeatNetworkInterface{}
	}

	var activeInterfaces []models.HeartbeatNetworkInterface
	for _, iface := range interfaces {
		if iface.Flags&net.FlagUp != 0 && iface.Flags&net.FlagLoopback == 0 {
			addrs, err := iface.Addrs()
			if err != nil {
				continue
			}
			for _, addr := range addrs {
				if ipnet, ok := addr.(*net.IPNet); ok && !ipnet.IP.IsLoopback() {
					activeInterfaces = append(activeInterfaces, models.HeartbeatNetworkInterface{
						Name: iface.Name,
						IP:   ipnet.IP.String(),
						MTU:  iface.MTU,
					})
				}
			}
		}
	}
	return activeInterfaces
}
