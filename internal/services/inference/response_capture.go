// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package inference

import (
	"bytes"
	"errors"
	"io"
	"sync"
	"time"
)

// providerResponseCapture is an append-only, bounded provider artifact with an
// independent reader cursor. Ingestion never waits for the decoder or reporter.
// result joins ingestion even when the decoder fails, preserving the raw tail.
type providerResponseCapture struct {
	mu        sync.Mutex
	changed   *sync.Cond
	finished  chan struct{}
	raw       bytes.Buffer
	reads     []responseRead
	cursor    int
	done      bool
	err       error
	limit     int64
	startedAt time.Time
}

func newProviderResponseCapture(limit int64, startedAt time.Time) *providerResponseCapture {
	capture := &providerResponseCapture{limit: limit, startedAt: startedAt, finished: make(chan struct{})}
	capture.changed = sync.NewCond(&capture.mu)
	return capture
}

func (c *providerResponseCapture) receive(body io.Reader) {
	limited := io.LimitReader(body, c.limit+1)
	buffer := make([]byte, 32<<10)
	for {
		n, err := limited.Read(buffer)
		elapsed := time.Since(c.startedAt).Nanoseconds()
		c.mu.Lock()
		if n > 0 {
			c.raw.Write(buffer[:n])
			c.reads = append(c.reads, responseRead{endOffset: int64(c.raw.Len()), elapsedNS: elapsed})
		}
		if err != nil {
			c.done = true
			if !errors.Is(err, io.EOF) {
				c.err = err
			}
		}
		c.changed.Broadcast()
		c.mu.Unlock()
		if err != nil {
			close(c.finished)
			return
		}
	}
}

func (c *providerResponseCapture) Read(p []byte) (int, error) {
	if len(p) == 0 {
		return 0, nil
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	for c.cursor == c.raw.Len() && !c.done {
		c.changed.Wait()
	}
	if c.cursor < c.raw.Len() {
		n := copy(p, c.raw.Bytes()[c.cursor:])
		c.cursor += n
		return n, nil
	}
	if c.err != nil {
		return 0, c.err
	}
	return 0, io.EOF
}

func (c *providerResponseCapture) arrivalTime(offset int64) *int64 {
	c.mu.Lock()
	defer c.mu.Unlock()
	for _, read := range c.reads {
		if read.endOffset >= offset {
			elapsed := read.elapsedNS
			return &elapsed
		}
	}
	return nil
}

func (c *providerResponseCapture) result() ([]byte, error) {
	<-c.finished
	return c.raw.Bytes(), c.err
}
