// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package sqliteutil

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"
)

// PruneFunc is the callback invoked on each prune tick.
// Implementations should perform their table-specific deletion logic.
type PruneFunc func(ctx context.Context, db *DB, logger *slog.Logger) error

// Pruner manages a background goroutine that periodically invokes a PruneFunc.
type Pruner struct {
	db       *DB
	logger   *slog.Logger
	interval time.Duration
	fn       PruneFunc
	ctx      context.Context
	cancel   context.CancelFunc
	mu       sync.Mutex
	started  bool
	wg       sync.WaitGroup
}

// NewPruner creates a new Pruner. Call Start() to begin the background loop.
func NewPruner(db *DB, logger *slog.Logger, interval time.Duration, fn PruneFunc) *Pruner {
	if interval <= 0 {
		interval = time.Hour
	}
	ctx, cancel := context.WithCancel(context.Background())
	return &Pruner{
		db:       db,
		logger:   logger,
		interval: interval,
		fn:       fn,
		ctx:      ctx,
		cancel:   cancel,
	}
}

// Start begins the background pruning goroutine.
// It is safe to call Start multiple times (subsequent calls are no-ops).
func (p *Pruner) Start() {
	p.mu.Lock()
	if p.started {
		p.mu.Unlock()
		return
	}
	p.started = true
	p.mu.Unlock()

	p.wg.Add(1)
	go func() {
		defer p.wg.Done()

		ticker := time.NewTicker(p.interval)
		defer ticker.Stop()

		for {
			select {
			case <-p.ctx.Done():
				return
			case <-ticker.C:
				if err := p.fn(p.ctx, p.db, p.logger); err != nil {
					wrappedErr := fmt.Errorf("pruner: prune function failed: %w", err)
					p.logger.Error("pruner: prune function failed", "error", wrappedErr)
				}
			}
		}
	}()
}

// Stop signals the pruning goroutine to exit and waits for it to finish.
// It is safe to call Stop multiple times (subsequent calls are no-ops).
func (p *Pruner) Stop() {
	p.mu.Lock()
	if !p.started {
		p.mu.Unlock()
		return
	}
	p.started = false
	p.mu.Unlock()

	p.cancel()
	p.wg.Wait()
}
