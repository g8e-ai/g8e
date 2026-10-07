// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package tui

import "sync"

// sessionManager keeps the request session and SSE stream aligned when a
// Gateway action issues a replacement CLI session.
type sessionManager struct {
	mu         sync.RWMutex
	gw         *gateway
	generation uint64
	changed    chan struct{}
}

func newSessionManager(session Session, userID string) *sessionManager {
	manager := &sessionManager{changed: make(chan struct{})}
	if session != nil {
		manager.gw = &gateway{session: session, userID: userID}
	}
	return manager
}

func (s *sessionManager) snapshot() (*gateway, uint64, <-chan struct{}) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.gw, s.generation, s.changed
}

func (s *sessionManager) replace(session Session, userID string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.gw = nil
	if session != nil {
		s.gw = &gateway{session: session, userID: userID}
	}
	s.generation++
	close(s.changed)
	s.changed = make(chan struct{})
}
