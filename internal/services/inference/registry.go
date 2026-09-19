// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package inference

import (
	"fmt"
	"sync"

	"github.com/g8e-ai/g8e/v2/internal/constants"
)

// BackendRegistry owns explicit backend registration. It mirrors
// mcp.RegisterNativeTools: explicit, returns errors, fails closed on nil.
// No init()-based auto-registration.
type BackendRegistry struct {
	mu       sync.RWMutex
	backends map[string]Backend
}

// NewBackendRegistry constructs an empty BackendRegistry.
func NewBackendRegistry() *BackendRegistry {
	return &BackendRegistry{
		backends: make(map[string]Backend),
	}
}

// Register registers a backend under the given name. Fails closed on nil
// backend or duplicate registration.
func (r *BackendRegistry) Register(name string, backend Backend) error {
	if r == nil {
		return fmt.Errorf("inference: register: %w", constants.ErrInternal)
	}
	if backend == nil {
		return fmt.Errorf("inference: register: %w: backend is nil", constants.ErrInternal)
	}
	if name == "" {
		return fmt.Errorf("inference: register: %w: name is empty", constants.ErrInternal)
	}

	r.mu.Lock()
	defer r.mu.Unlock()
	if _, exists := r.backends[name]; exists {
		return fmt.Errorf("inference: register: backend %q already registered", name)
	}
	r.backends[name] = backend
	return nil
}

// Get returns the registered backend for the given name, or
// ErrInferenceBackendNotRegistered if not found.
func (r *BackendRegistry) Get(name string) (Backend, error) {
	if r == nil {
		return nil, fmt.Errorf("inference: get: %w", constants.ErrInternal)
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	backend, ok := r.backends[name]
	if !ok {
		return nil, fmt.Errorf("inference: get: %w: %q", constants.ErrInferenceBackendNotRegistered, name)
	}
	return backend, nil
}

// RegisterBackends registers all built-in backends into the provided
// registry. Explicit, returns errors, fails closed on nil. Mirrors
// mcp.RegisterNativeTools.
func RegisterBackends(registry *BackendRegistry, backends map[string]Backend) error {
	if registry == nil {
		return fmt.Errorf("inference: register backends: %w", constants.ErrInternal)
	}
	for name, backend := range backends {
		if err := registry.Register(name, backend); err != nil {
			return err
		}
	}
	return nil
}
