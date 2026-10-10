// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package ssh

import (
	"context"
	"fmt"
	"sync"

	sshlib "golang.org/x/crypto/ssh"

	"github.com/g8e-ai/g8e/v2/internal/constants"
)

// ClientPool manages reusable SSH client connections per target host.
type ClientPool struct {
	mu      sync.Mutex
	clients map[string]*sshlib.Client
}

// NewClientPool creates a new empty SSH ClientPool.
func NewClientPool() *ClientPool {
	return &ClientPool{
		clients: make(map[string]*sshlib.Client),
	}
}

// Dial returns an existing healthy client for the target or dials a new one.
func (p *ClientPool) Dial(ctx context.Context, r HostConfig, clientConfig *sshlib.ClientConfig, addr string) (*sshlib.Client, error) {
	p.mu.Lock()
	defer p.mu.Unlock()

	key := addr
	if key == "" {
		port := r.Port
		if port == "" {
			port = "22"
		}
		key = r.Hostname + ":" + port
	}

	if client, ok := p.clients[key]; ok {
		// Probe client health with a keepalive request
		_, _, err := client.SendRequest(constants.SSHKeepaliveRequestType, true, nil)
		if err == nil {
			return client, nil
		}
		// Stale client; close and remove
		_ = client.Close()
		delete(p.clients, key)
	}

	client, err := DialSSH(ctx, r, clientConfig, addr)
	if err != nil {
		return nil, fmt.Errorf("ssh pool: dial: %w", err)
	}

	p.clients[key] = client
	return client, nil
}

// Close closes all active clients in the pool.
func (p *ClientPool) Close() error {
	p.mu.Lock()
	defer p.mu.Unlock()

	var firstErr error
	for key, client := range p.clients {
		if err := client.Close(); err != nil && firstErr == nil {
			firstErr = err
		}
		delete(p.clients, key)
	}
	if firstErr != nil {
		return fmt.Errorf("ssh pool: close: %w", firstErr)
	}
	return nil
}
