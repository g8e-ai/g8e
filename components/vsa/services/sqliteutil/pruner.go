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

package sqliteutil

import (
	"log/slog"
	"sync"
	"time"
)

// PruneFunc is the callback invoked on each prune tick.
// Implementations should perform their table-specific deletion logic.
type PruneFunc func(db *DB, logger *slog.Logger)

// Pruner manages a background goroutine that periodically invokes a PruneFunc.
type Pruner struct {
	db       *DB
	logger   *slog.Logger
	interval time.Duration
	fn       PruneFunc
	stop     chan struct{}
	wg       sync.WaitGroup
}

// NewPruner creates a new Pruner. Call Start() to begin the background loop.
func NewPruner(db *DB, logger *slog.Logger, interval time.Duration, fn PruneFunc) *Pruner {
	if interval <= 0 {
		interval = time.Hour
	}
	return &Pruner{
		db:       db,
		logger:   logger,
		interval: interval,
		fn:       fn,
		stop:     make(chan struct{}),
	}
}

// Start begins the background pruning goroutine.
func (p *Pruner) Start() {
	p.wg.Add(1)
	go func() {
		defer p.wg.Done()

		ticker := time.NewTicker(p.interval)
		defer ticker.Stop()

		for {
			select {
			case <-p.stop:
				return
			case <-ticker.C:
				p.fn(p.db, p.logger)
			}
		}
	}()
}

// Stop signals the pruning goroutine to exit and waits for it to finish.
// It is safe to call Stop multiple times (subsequent calls are no-ops).
func (p *Pruner) Stop() {
	select {
	case <-p.stop:
	default:
		close(p.stop)
	}
	p.wg.Wait()
}
