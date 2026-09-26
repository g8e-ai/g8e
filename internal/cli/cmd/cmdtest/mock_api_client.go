// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package cmdtest

import "sync"

// MockAPIClient is a scripted authcmd.APIClient for command tests.
type MockAPIClient struct {
	mu        sync.Mutex
	GetResp   []byte
	GetErr    error
	PostResp  []byte
	PostErr   error
	GetCalls  []string
	PostCalls []MockPostCall
}

// MockPostCall records one Post invocation.
type MockPostCall struct {
	Path string
	Body interface{}
}

func (m *MockAPIClient) Get(path string) ([]byte, error) {
	m.mu.Lock()
	m.GetCalls = append(m.GetCalls, path)
	resp, err := m.GetResp, m.GetErr
	m.mu.Unlock()
	return resp, err
}

func (m *MockAPIClient) Post(path string, body interface{}) ([]byte, error) {
	m.mu.Lock()
	m.PostCalls = append(m.PostCalls, MockPostCall{Path: path, Body: body})
	resp, err := m.PostResp, m.PostErr
	m.mu.Unlock()
	return resp, err
}

func (m *MockAPIClient) Put(path string, body interface{}) ([]byte, error) {
	return nil, nil
}

func (m *MockAPIClient) Delete(path string) ([]byte, error) {
	return nil, nil
}
