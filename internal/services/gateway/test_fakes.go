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

package gateway

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"sync"
	"time"

	"github.com/g8e-ai/g8e/internal/models"
)

// fakeGatewayDBService provides a lightweight in-memory implementation of GatewayDBService
// for testing HTTP handlers without requiring a full SQLite database setup.
type fakeGatewayDBService struct {
	mu           sync.RWMutex
	docs         map[string]map[string]*models.Document  // collection -> id -> document
	suspendedTxs map[string]*models.SuspendedTransaction // transaction_hash -> suspended transaction
}

func newFakeGatewayDBService() *fakeGatewayDBService {
	return &fakeGatewayDBService{
		docs:         make(map[string]map[string]*models.Document),
		suspendedTxs: make(map[string]*models.SuspendedTransaction),
	}
}

func (f *fakeGatewayDBService) DocGet(collection, id string) (*models.Document, error) {
	f.mu.RLock()
	defer f.mu.RUnlock()

	if colDocs, ok := f.docs[collection]; ok {
		if doc, ok := colDocs[id]; ok {
			return doc, nil
		}
	}
	return nil, nil
}

func (f *fakeGatewayDBService) DocSet(collection, id string, data json.RawMessage) error {
	return f.DocSetWithTimestamps(collection, id, data, time.Time{}, time.Time{})
}

func (f *fakeGatewayDBService) DocSetWithTimestamps(collection, id string, data json.RawMessage, createdAt, updatedAt time.Time) error {
	f.mu.Lock()
	defer f.mu.Unlock()

	if f.docs[collection] == nil {
		f.docs[collection] = make(map[string]*models.Document)
	}

	now := time.Now().UTC()
	if createdAt.IsZero() {
		createdAt = now
	}
	if updatedAt.IsZero() {
		updatedAt = now
	}

	var dataMap map[string]json.RawMessage
	if err := json.Unmarshal(data, &dataMap); err != nil {
		return err
	}

	doc := &models.Document{
		ID:         id,
		Collection: collection,
		Data:       dataMap,
		CreatedAt:  createdAt,
		UpdatedAt:  updatedAt,
	}

	f.docs[collection][id] = doc
	return nil
}

func (f *fakeGatewayDBService) DocDelete(collection, id string) (bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	if colDocs, ok := f.docs[collection]; ok {
		if _, ok := colDocs[id]; ok {
			delete(colDocs, id)
			return true, nil
		}
	}
	return false, nil
}

func (f *fakeGatewayDBService) DocDeleteNamespace(collection string) (int64, error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	if colDocs, ok := f.docs[collection]; ok {
		count := int64(len(colDocs))
		delete(f.docs, collection)
		return count, nil
	}
	return 0, nil
}

func (f *fakeGatewayDBService) DeleteSuspendedTransaction(txHash string) error {
	f.mu.Lock()
	defer f.mu.Unlock()

	delete(f.suspendedTxs, txHash)
	return nil
}

func (f *fakeGatewayDBService) StoreSuspendedTransaction(tx *models.SuspendedTransaction) error {
	f.mu.Lock()
	defer f.mu.Unlock()

	f.suspendedTxs[tx.TransactionHash] = tx
	return nil
}

func (f *fakeGatewayDBService) GetSuspendedTransaction(txHash string) (*models.SuspendedTransaction, bool) {
	f.mu.RLock()
	defer f.mu.RUnlock()

	tx, ok := f.suspendedTxs[txHash]
	return tx, ok
}

func (f *fakeGatewayDBService) Close() error {
	return nil
}

// fakePKIAuthority provides a minimal PKI implementation for tests that don't need real PKI.
type fakePKIAuthority struct {
	logger *slog.Logger
}

func newFakePKIAuthority(logger *slog.Logger) *fakePKIAuthority {
	return &fakePKIAuthority{logger: logger}
}

func (f *fakePKIAuthority) EnsurePKI(initialCA []byte) error {
	return nil
}

func (f *fakePKIAuthority) GetCACertPEM() ([]byte, error) {
	return []byte("fake-ca-cert"), nil
}

func (f *fakePKIAuthority) GetCAFingerprint() (string, error) {
	return "fake-fingerprint", nil
}

func (f *fakePKIAuthority) IssueOperatorCert(operatorID string) ([]byte, []byte, error) {
	return []byte("fake-cert"), []byte("fake-key"), nil
}

func (f *fakePKIAuthority) RevokeCertificate(serial string, reason string) error {
	return nil
}

func (f *fakePKIAuthority) IsRevoked(serial string) (bool, error) {
	return false, nil
}

// fakeAuthService provides a minimal auth service for testing middleware without full auth logic.
type fakeAuthService struct {
	db     *fakeGatewayDBService
	logger *slog.Logger
	pki    *fakePKIAuthority
}

func newFakeAuthService(db *fakeGatewayDBService, logger *slog.Logger, pki *fakePKIAuthority) *fakeAuthService {
	return &fakeAuthService{
		db:     db,
		logger: logger,
		pki:    pki,
	}
}

func (f *fakeAuthService) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// For testing, allow all requests through unless specifically configured otherwise
		next.ServeHTTP(w, r)
	})
}

// fakePasskeyService provides a minimal passkey service for tests.
type fakePasskeyService struct {
	db     *fakeGatewayDBService
	logger *slog.Logger
}

func newFakePasskeyService(db *fakeGatewayDBService, logger *slog.Logger) *fakePasskeyService {
	return &fakePasskeyService{
		db:     db,
		logger: logger,
	}
}

// fakeSessionService provides a minimal session service for tests.
type fakeSessionService struct {
	db     *fakeGatewayDBService
	logger *slog.Logger
}

func newFakeSessionService(db *fakeGatewayDBService, logger *slog.Logger) *fakeSessionService {
	return &fakeSessionService{
		db:     db,
		logger: logger,
	}
}

// fakeRegistrationService provides a minimal registration service for tests.
type fakeRegistrationService struct {
	db     *fakeGatewayDBService
	logger *slog.Logger
}

func newFakeRegistrationService(db *fakeGatewayDBService, logger *slog.Logger) *fakeRegistrationService {
	return &fakeRegistrationService{
		db:     db,
		logger: logger,
	}
}
