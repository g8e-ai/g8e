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
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/g8e-ai/g8e/internal/config"
	"github.com/g8e-ai/g8e/internal/constants"
	"github.com/g8e-ai/g8e/internal/marshaler"
	"github.com/g8e-ai/g8e/internal/models"
	"github.com/g8e-ai/g8e/internal/responder"
	"github.com/g8e-ai/g8e/internal/services/sqliteutil"
)

// DBController handles database, KV, blob, and audit endpoints.
type DBController struct {
	cfg       *config.Config
	logger    *slog.Logger
	db        *GatewayDBService
	auth      *AuthService
	pubsub    *PubSubBroker
	userSvc   *UserService
	responder *responder.Responder
}

func newDBController(cfg *config.Config, logger *slog.Logger, db *GatewayDBService, auth *AuthService, pubsub *PubSubBroker, userSvc *UserService, responder *responder.Responder) *DBController {
	return &DBController{
		cfg:       cfg,
		logger:    logger,
		db:        db,
		auth:      auth,
		pubsub:    pubsub,
		userSvc:   userSvc,
		responder: responder,
	}
}

func (c *DBController) readBody(r *http.Request) ([]byte, error) {
	r.Body = http.MaxBytesReader(nil, r.Body, c.cfg.Gateway.MaxPayloadBytes)
	return io.ReadAll(r.Body)
}

func (c *DBController) handleDataSettings(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		doc, err := c.db.DocGet(marshaler.CollectionName(constants.CollectionSettings), marshaler.DocumentID(constants.DocIDPlatformSettings))
		if err != nil {
			c.responder.Error(w, http.StatusInternalServerError, err.Error())
			return
		}
		if doc == nil {
			c.responder.Error(w, http.StatusNotFound, "settings not found")
			return
		}
		c.responder.JSON(w, http.StatusOK, doc.ForWire())
	case http.MethodPut, http.MethodPatch:
		body, err := c.readBody(r)
		if err != nil {
			c.responder.Error(w, http.StatusBadRequest, "invalid body")
			return
		}
		if !json.Valid(body) {
			c.responder.Error(w, http.StatusBadRequest, "invalid JSON")
			return
		}
		var err2 error
		if r.Method == http.MethodPut {
			err2 = c.db.DocSet(marshaler.CollectionName(constants.CollectionSettings), marshaler.DocumentID(constants.DocIDPlatformSettings), json.RawMessage(body))
		} else {
			_, err2 = c.db.DocUpdate(marshaler.CollectionName(constants.CollectionSettings), marshaler.DocumentID(constants.DocIDPlatformSettings), json.RawMessage(body))
		}
		if err2 != nil {
			c.responder.Error(w, http.StatusInternalServerError, err2.Error())
			return
		}
		c.responder.JSON(w, http.StatusOK, models.StatusResponse{Status: constants.GatewayModeStatusOK})
	default:
		c.responder.Error(w, http.StatusMethodNotAllowed, "method not allowed")
	}
}

func (c *DBController) handleDataDB(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/api/v1/data/")
	parts := strings.SplitN(path, "/", 2)
	if len(parts) == 0 || parts[0] == "" {
		c.responder.Error(w, http.StatusBadRequest, "collection required")
		return
	}

	collection := parts[0]
	id := ""
	if len(parts) > 1 {
		id = parts[1]
	}

	if id == "_query" && r.Method == http.MethodPost {
		c.handleDBQuery(w, r, collection)
		return
	}

	if collection == "_sse_events" {
		c.handleSSEEvents(w, r, id)
		return
	}

	if id == "" {
		c.responder.Error(w, http.StatusBadRequest, "document id required")
		return
	}

	switch r.Method {
	case http.MethodGet:
		doc, err := c.db.DocGet(collection, id)
		if err != nil {
			c.responder.Error(w, http.StatusInternalServerError, err.Error())
			return
		}
		if doc == nil {
			c.responder.Error(w, http.StatusNotFound, fmt.Sprintf("document %s/%s not found", collection, id))
			return
		}
		c.responder.JSON(w, http.StatusOK, doc.ForWire())

	case http.MethodPut:
		if !isDirectDBMutationAllowed(collection) {
			c.responder.Error(w, http.StatusConflict, governanceEnvelopeRedirectError)
			return
		}
		body, err := c.readBody(r)
		if err != nil {
			c.responder.Error(w, http.StatusBadRequest, "invalid JSON body")
			return
		}
		if !json.Valid(body) {
			c.responder.Error(w, http.StatusBadRequest, "invalid JSON body")
			return
		}
		if err := c.db.DocSet(collection, id, json.RawMessage(body)); err != nil {
			if strings.Contains(err.Error(), "locked") {
				c.responder.Error(w, http.StatusServiceUnavailable, "database is locked")
			} else {
				c.responder.Error(w, http.StatusInternalServerError, err.Error())
			}
			return
		}
		c.responder.JSON(w, http.StatusOK, models.StatusResponse{Status: constants.GatewayModeStatusOK})

	case http.MethodPatch:
		if !isDirectDBMutationAllowed(collection) {
			c.responder.Error(w, http.StatusConflict, governanceEnvelopeRedirectError)
			return
		}
		body, err := c.readBody(r)
		if err != nil {
			c.responder.Error(w, http.StatusBadRequest, "invalid JSON body")
			return
		}
		if !json.Valid(body) {
			c.responder.Error(w, http.StatusBadRequest, "invalid JSON body")
			return
		}
		doc, err := c.db.DocUpdate(collection, id, json.RawMessage(body))
		if err != nil {
			if strings.Contains(err.Error(), "not found") {
				c.responder.Error(w, http.StatusNotFound, err.Error())
			} else if strings.Contains(err.Error(), "constraint") {
				c.responder.Error(w, http.StatusConflict, "database constraint violation")
			} else if strings.Contains(err.Error(), "locked") {
				c.responder.Error(w, http.StatusServiceUnavailable, "database is locked")
			} else {
				c.responder.Error(w, http.StatusInternalServerError, err.Error())
			}
			return
		}
		c.responder.JSON(w, http.StatusOK, doc.ForWire())

	case http.MethodDelete:
		if !isDirectDBMutationAllowed(collection) {
			c.responder.Error(w, http.StatusConflict, governanceEnvelopeRedirectError)
			return
		}
		deleted, err := c.db.DocDelete(collection, id)
		if err != nil {
			c.responder.Error(w, http.StatusInternalServerError, err.Error())
			return
		}
		if !deleted {
			c.responder.Error(w, http.StatusNotFound, "document not found")
			return
		}
		c.responder.JSON(w, http.StatusOK, models.StatusResponse{Status: constants.GatewayModeStatusOK})

	default:
		c.responder.Error(w, http.StatusMethodNotAllowed, "method not allowed")
	}
}

func (c *DBController) handleDBQuery(w http.ResponseWriter, r *http.Request, collection string) {
	body, err := c.readBody(r)
	if err != nil {
		c.responder.Error(w, http.StatusBadRequest, "invalid JSON body")
		return
	}

	var req models.DocQueryRequest
	if len(body) > 0 {
		if err := json.Unmarshal(body, &req); err != nil {
			c.responder.Error(w, http.StatusBadRequest, "invalid JSON body")
			return
		}
	}

	docs, err := c.db.DocQuery(collection, req.Filters, req.OrderBy, req.Limit)
	if err != nil {
		c.responder.Error(w, http.StatusInternalServerError, err.Error())
		return
	}
	wire := make([]map[string]json.RawMessage, 0, len(docs))
	for _, d := range docs {
		wire = append(wire, d.ForWire())
	}
	c.responder.JSON(w, http.StatusOK, wire)
}

func (c *DBController) handleSSEEvents(w http.ResponseWriter, r *http.Request, id string) {
	if id == "count" && r.Method == http.MethodGet {
		count, err := c.db.SSEEventsCount()
		if err != nil {
			c.responder.Error(w, http.StatusInternalServerError, err.Error())
			return
		}
		c.responder.JSON(w, http.StatusOK, models.SSEEventsCountResponse{Count: count})
		return
	}

	if id == "" && r.Method == http.MethodDelete {
		deleted, err := c.db.SSEEventsWipe()
		if err != nil {
			c.responder.Error(w, http.StatusInternalServerError, err.Error())
			return
		}
		c.responder.JSON(w, http.StatusOK, models.SSEEventsWipeResponse{Deleted: deleted})
		return
	}

	c.responder.Error(w, http.StatusMethodNotAllowed, "method not allowed")
}

func (c *DBController) handleAuditReceipts(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		c.responder.Error(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	txID := r.URL.Query().Get("tx_id")
	if txID != "" {
		receipt, err := c.db.AuditVault.GetActionReceipt(txID)
		if err != nil {
			c.responder.Error(w, http.StatusInternalServerError, err.Error())
			return
		}
		if receipt == nil {
			c.responder.Error(w, http.StatusNotFound, "receipt not found")
			return
		}
		c.responder.JSON(w, http.StatusOK, receipt)
		return
	}

	operatorSessionID := r.URL.Query().Get("operator_session_id")
	limitStr := r.URL.Query().Get("limit")
	offsetStr := r.URL.Query().Get("offset")

	limit := 50
	if limitStr != "" {
		if l, err := strconv.Atoi(limitStr); err == nil {
			limit = l
		}
	}
	offset := 0
	if offsetStr != "" {
		if o, err := strconv.Atoi(offsetStr); err == nil {
			offset = o
		}
	}

	receipts, err := c.db.AuditVault.ListActionReceipts(operatorSessionID, limit, offset)
	if err != nil {
		c.responder.Error(w, http.StatusInternalServerError, err.Error())
		return
	}

	c.responder.JSON(w, http.StatusOK, models.AuditReceiptsResponse{
		Success:  true,
		Receipts: receipts,
	})
}

func (c *DBController) handleAuditReceiptsExport(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		c.responder.Error(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	sinceStr := r.URL.Query().Get("since")
	limitStr := r.URL.Query().Get("limit")

	since := time.Time{}
	if sinceStr != "" {
		if t, err := time.Parse(time.RFC3339, sinceStr); err == nil {
			since = t
		} else if t, err := sqliteutil.ParseTimestamp(sinceStr); err == nil {
			since = t
		}
	}

	limit := 100
	if limitStr != "" {
		if l, err := strconv.Atoi(limitStr); err == nil {
			limit = l
		}
	}

	receipts, err := c.db.AuditVault.ListActionReceiptsSince(since, limit)
	if err != nil {
		c.responder.Error(w, http.StatusInternalServerError, err.Error())
		return
	}

	w.Header().Set("Content-Type", "application/x-ndjson")
	w.WriteHeader(http.StatusOK)
	encoder := json.NewEncoder(w)
	for _, r := range receipts {
		if err := encoder.Encode(r); err != nil {
			c.logger.Error("Failed to encode audit receipt for export", "transaction_id", r.TransactionID, string(constants.ConnectionStateError), err)
			break
		}
	}
}

func (c *DBController) handleGovernanceSigners(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		signers, err := c.db.ListTrustedSigners()
		if err != nil {
			c.responder.Error(w, http.StatusInternalServerError, err.Error())
			return
		}
		c.responder.JSON(w, http.StatusOK, models.TrustedSignersResponse{
			Success: true,
			Signers: signers,
		})

	case http.MethodPost:
		body, err := c.readBody(r)
		if err != nil {
			c.responder.Error(w, http.StatusBadRequest, "failed to read body")
			return
		}
		var signer models.TrustedSigner
		if err := json.Unmarshal(body, &signer); err != nil {
			c.responder.Error(w, http.StatusBadRequest, "invalid JSON")
			return
		}
		if signer.ID == "" || signer.PublicKey == "" {
			c.responder.Error(w, http.StatusBadRequest, "id and public_key_hex required")
			return
		}
		if err := c.db.AddTrustedSigner(signer); err != nil {
			c.responder.Error(w, http.StatusInternalServerError, err.Error())
			return
		}
		c.responder.JSON(w, http.StatusCreated, models.StatusResponse{Status: constants.GatewayModeStatusOK})

	default:
		c.responder.Error(w, http.StatusMethodNotAllowed, "method not allowed")
	}
}

func (c *DBController) handleGovernanceSignerByID(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimPrefix(r.URL.Path, "/api/v1/governance/signers/")
	if id == "" || strings.Contains(id, "/") {
		c.responder.Error(w, http.StatusBadRequest, "invalid signer id")
		return
	}

	switch r.Method {
	case http.MethodGet:
		pubKey, err := c.db.GetTrustedSigner(id)
		if err != nil {
			c.responder.Error(w, http.StatusInternalServerError, err.Error())
			return
		}
		if pubKey == nil {
			c.responder.Error(w, http.StatusNotFound, "signer not found")
			return
		}
		doc, _ := c.db.DocGet(marshaler.CollectionName(constants.CollectionTrustedSigners), id)
		c.responder.JSON(w, http.StatusOK, doc.ForWire())

	case http.MethodDelete:
		deleted, err := c.db.DeleteTrustedSigner(id)
		if err != nil {
			c.responder.Error(w, http.StatusInternalServerError, err.Error())
			return
		}
		if !deleted {
			c.responder.Error(w, http.StatusNotFound, "signer not found")
			return
		}
		c.responder.JSON(w, http.StatusOK, models.StatusResponse{Status: constants.GatewayModeStatusOK})

	default:
		c.responder.Error(w, http.StatusMethodNotAllowed, "method not allowed")
	}
}

func (c *DBController) handleKV(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/api/v1/kv/")
	if path == "" {
		c.responder.Error(w, http.StatusBadRequest, "key required")
		return
	}

	if path == "_keys" && r.Method == http.MethodPost {
		c.handleKVKeys(w, r)
		return
	}
	if path == "_scan" && r.Method == http.MethodPost {
		c.handleKVScan(w, r)
		return
	}
	if path == "_delete_pattern" && r.Method == http.MethodPost {
		c.handleKVDeletePattern(w, r)
		return
	}

	if strings.HasSuffix(path, "/_ttl") {
		key := strings.TrimSuffix(path, "/_ttl")
		ttl := c.db.KVTTL(key)
		c.responder.JSON(w, http.StatusOK, models.KVTTLResponse{TTL: ttl})
		return
	}
	if strings.HasSuffix(path, "/_expire") && r.Method == http.MethodPut {
		key := strings.TrimSuffix(path, "/_expire")
		body, err := c.readBody(r)
		if err != nil {
			c.responder.Error(w, http.StatusBadRequest, "invalid JSON body")
			return
		}
		var req models.KVExpireRequest
		if err := json.Unmarshal(body, &req); err != nil {
			c.responder.Error(w, http.StatusBadRequest, "invalid JSON body")
			return
		}
		if req.TTL <= 0 {
			c.responder.Error(w, http.StatusBadRequest, "ttl required and must be > 0")
			return
		}
		ok := c.db.KVExpire(key, req.TTL)
		if !ok {
			c.responder.Error(w, http.StatusNotFound, "key not found")
			return
		}
		c.responder.JSON(w, http.StatusOK, models.StatusResponse{Status: constants.GatewayModeStatusOK})
		return
	}

	key := path

	switch r.Method {
	case http.MethodGet:
		value, found := c.db.KVGet(key)
		if !found {
			c.responder.Error(w, http.StatusNotFound, "key not found")
			return
		}
		c.responder.JSON(w, http.StatusOK, models.KVGetResponse{Value: value})

	case http.MethodPut:
		body, err := c.readBody(r)
		if err != nil {
			c.responder.Error(w, http.StatusBadRequest, "invalid JSON body")
			return
		}
		var req models.KVSetRequest
		if err := json.Unmarshal(body, &req); err != nil {
			c.responder.Error(w, http.StatusBadRequest, "invalid JSON body")
			return
		}
		if err := c.db.KVSet(key, req.Value, req.TTL); err != nil {
			c.responder.Error(w, http.StatusInternalServerError, err.Error())
			return
		}
		c.responder.JSON(w, http.StatusOK, models.StatusResponse{Status: constants.GatewayModeStatusOK})

	case http.MethodDelete:
		if err := c.db.KVDelete(key); err != nil {
			c.responder.Error(w, http.StatusInternalServerError, err.Error())
			return
		}
		c.responder.JSON(w, http.StatusOK, models.StatusResponse{Status: constants.GatewayModeStatusOK})

	default:
		c.responder.Error(w, http.StatusMethodNotAllowed, "method not allowed")
	}
}

func (c *DBController) handleKVKeys(w http.ResponseWriter, r *http.Request) {
	body, err := c.readBody(r)
	if err != nil {
		c.responder.Error(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	var req models.KVPatternRequest
	if len(body) > 0 {
		if err := json.Unmarshal(body, &req); err != nil {
			c.responder.Error(w, http.StatusBadRequest, "invalid JSON body")
			return
		}
	}
	if req.Pattern == "" {
		req.Pattern = "*"
	}
	keys, err := c.db.KVKeys(req.Pattern)
	if err != nil {
		c.responder.Error(w, http.StatusInternalServerError, err.Error())
		return
	}
	if keys == nil {
		keys = []string{}
	}
	c.responder.JSON(w, http.StatusOK, models.KVKeysResponse{Keys: keys})
}

func (c *DBController) handleKVScan(w http.ResponseWriter, r *http.Request) {
	body, err := c.readBody(r)
	if err != nil {
		c.responder.Error(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	var req models.KVPatternRequest
	if len(body) > 0 {
		if err := json.Unmarshal(body, &req); err != nil {
			c.responder.Error(w, http.StatusBadRequest, "invalid JSON body")
			return
		}
	}
	if req.Pattern == "" {
		req.Pattern = "*"
	}
	if req.Count <= 0 {
		req.Count = 100
	}
	nextCursor, keys, err := c.db.KVScan(req.Pattern, req.Cursor, req.Count)
	if err != nil {
		c.responder.Error(w, http.StatusInternalServerError, err.Error())
		return
	}
	if keys == nil {
		keys = []string{}
	}
	c.responder.JSON(w, http.StatusOK, models.KVScanResponse{Cursor: nextCursor, Keys: keys})
}

func (c *DBController) handleKVDeletePattern(w http.ResponseWriter, r *http.Request) {
	body, err := c.readBody(r)
	if err != nil {
		c.responder.Error(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	var req models.KVPatternRequest
	if err := json.Unmarshal(body, &req); err != nil {
		c.responder.Error(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	if req.Pattern == "" {
		c.responder.Error(w, http.StatusBadRequest, "pattern required")
		return
	}
	count, err := c.db.KVDeletePattern(req.Pattern)
	if err != nil {
		c.responder.Error(w, http.StatusInternalServerError, err.Error())
		return
	}
	c.responder.JSON(w, http.StatusOK, models.KVDeletePatternResponse{Deleted: count})
}

func blobSegmentValid(s string) bool {
	if s == "" || s == ".." {
		return false
	}
	for _, c := range s {
		if c == '/' || c == '\\' || c == 0 {
			return false
		}
	}
	return true
}

const maxBlobBodySize = 50 * 1024 * 1024

// blobNamespaceAllowed checks if a namespace is in the allowlist for direct mutations.
// This is a governance boundary: only allowlisted namespaces can be mutated directly.
// All other namespaces must go through the governance envelope path.
func blobNamespaceAllowed(namespace string) bool {
	// Allowlist of namespaces that can be mutated directly.
	// These are ephemeral caches or client-uploaded artifacts that do not
	// require full governance envelope processing.
	allowedNamespaces := map[string]bool{
		"temp":    true, // Temporary cache
		"uploads": true, // Client-uploaded files
		"cache":   true, // Ephemeral cache
		"scratch": true, // Scratch space
	}
	return allowedNamespaces[namespace]
}

// extractCallerIdentity extracts the caller's identity from the request context.
// Returns user_id, app_id, operator_session_id, cli_session_id.
func (c *DBController) extractCallerIdentity(r *http.Request) (string, string, string, string) {
	userID, _ := r.Context().Value(userIDKey).(string)
	appID, _ := r.Context().Value(appIDKey).(string)
	operatorSessionID := c.auth.ExtractOperatorSessionID(r)
	cliSessionID := r.Header.Get(constants.HeaderCLISessionID)
	return userID, appID, operatorSessionID, cliSessionID
}

// verifyBlobOwnership checks if the caller is authorized to mutate the given namespace.
// This enforces per-namespace ownership to prevent cross-tenant blob access.
// Allowlisted namespaces (temp, cache, uploads, scratch) are accessible by any authenticated user.
func (c *DBController) verifyBlobOwnership(r *http.Request, namespace string) error {
	userID, appID, operatorSessionID, cliSessionID := c.extractCallerIdentity(r)

	// If no identity is present, reject
	if userID == "" && appID == "" && operatorSessionID == "" && cliSessionID == "" {
		return fmt.Errorf("unauthorized: no identity present")
	}

	// Allowlisted namespaces are accessible by any authenticated identity
	if blobNamespaceAllowed(namespace) {
		return nil
	}

	// For app identities, check if the app is authorized for this namespace
	if appID != "" {
		// Apps can only write to their own namespace (app/<app_id>)
		expectedNamespace := "app/" + appID
		if namespace != expectedNamespace {
			return fmt.Errorf("unauthorized: app can only write to its own namespace (expected %s, got %s)", expectedNamespace, namespace)
		}
		return nil
	}

	// For operator/CLI identities, check if the namespace is user-scoped
	if operatorSessionID != "" || cliSessionID != "" {
		if userID == "" {
			return fmt.Errorf("unauthorized: operator/CLI identity without user_id")
		}
		// Operators/CLI can only write to user-scoped namespaces
		expectedNamespace := "user/" + userID
		if namespace != expectedNamespace {
			return fmt.Errorf("unauthorized: user can only write to their own namespace (expected %s, got %s)", expectedNamespace, namespace)
		}
		return nil
	}

	// For user identities (web session), check user-scoped namespace
	if userID != "" {
		expectedNamespace := "user/" + userID
		if namespace != expectedNamespace {
			return fmt.Errorf("unauthorized: user can only write to their own namespace (expected %s, got %s)", expectedNamespace, namespace)
		}
		return nil
	}

	return fmt.Errorf("unauthorized: unknown identity type")
}

func (c *DBController) handleBlob(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/api/v1/blobs/")
	if path == "" {
		c.responder.Error(w, http.StatusBadRequest, "namespace required")
		return
	}

	parts := strings.SplitN(path, "/", 3)
	namespace := parts[0]
	if !blobSegmentValid(namespace) {
		c.responder.Error(w, http.StatusBadRequest, "invalid namespace")
		return
	}

	if len(parts) == 1 {
		if r.Method != http.MethodDelete {
			c.responder.Error(w, http.StatusMethodNotAllowed, "method not allowed")
			return
		}
		// Check if namespace is allowlisted for direct mutations
		if !blobNamespaceAllowed(namespace) {
			c.responder.Error(w, http.StatusConflict, governanceEnvelopeRedirectError)
			return
		}
		// Enforce ownership for namespace deletion
		if err := c.verifyBlobOwnership(r, namespace); err != nil {
			c.logger.Warn("Blob namespace delete: ownership check failed", "namespace", namespace, "error", err)
			c.responder.Error(w, http.StatusForbidden, err.Error())
			return
		}
		count, err := c.db.BlobDeleteNamespace(namespace)
		if err != nil {
			c.responder.Error(w, http.StatusInternalServerError, err.Error())
			return
		}
		c.logger.Info("Blob namespace deleted", "namespace", namespace, "count", count)
		c.responder.JSON(w, http.StatusOK, models.BlobDeleteResponse{Deleted: count})
		return
	}

	blobID := parts[1]
	if !blobSegmentValid(blobID) {
		c.responder.Error(w, http.StatusBadRequest, "invalid blob id")
		return
	}

	if len(parts) == 3 {
		if parts[2] != "meta" {
			c.responder.Error(w, http.StatusBadRequest, "invalid path")
			return
		}
		if r.Method != http.MethodGet {
			c.responder.Error(w, http.StatusMethodNotAllowed, "method not allowed")
			return
		}
		rec, ok := c.db.BlobMeta(namespace, blobID)
		if !ok {
			c.responder.Error(w, http.StatusNotFound, "blob not found")
			return
		}
		c.responder.JSON(w, http.StatusOK, models.BlobMetaResponse{
			ID:          rec.ID,
			Namespace:   rec.Namespace,
			Size:        rec.Size,
			ContentType: rec.ContentType,
			CreatedAt:   rec.CreatedAt.UTC(),
		})
		return
	}

	switch r.Method {
	case http.MethodPut:
		// Check if namespace is allowlisted for direct mutations
		if !blobNamespaceAllowed(namespace) {
			c.responder.Error(w, http.StatusConflict, governanceEnvelopeRedirectError)
			return
		}

		// Enforce ownership for blob writes
		if err := c.verifyBlobOwnership(r, namespace); err != nil {
			c.logger.Warn("Blob put: ownership check failed", "namespace", namespace, "blob_id", blobID, "error", err)
			c.responder.Error(w, http.StatusForbidden, err.Error())
			return
		}

		contentType := r.Header.Get("Content-Type")
		if contentType == "" {
			c.responder.Error(w, http.StatusBadRequest, "Content-Type header required")
			return
		}

		ttl := 0
		if v := r.Header.Get("X-Blob-TTL"); v != "" {
			n, err := strconv.Atoi(v)
			if err != nil || n < 0 {
				c.responder.Error(w, http.StatusBadRequest, "X-Blob-TTL must be a non-negative integer")
				return
			}
			ttl = n
		}

		body, err := io.ReadAll(io.LimitReader(r.Body, maxBlobBodySize+1))
		if err != nil {
			c.responder.Error(w, http.StatusBadRequest, "failed to read body")
			return
		}
		if int64(len(body)) > maxBlobBodySize {
			c.responder.Error(w, http.StatusRequestEntityTooLarge, "blob exceeds maximum size")
			return
		}
		if len(body) == 0 {
			c.responder.Error(w, http.StatusBadRequest, "body must not be empty")
			return
		}

		if err := c.db.BlobPut(namespace, blobID, body, contentType, ttl); err != nil {
			c.responder.Error(w, http.StatusInternalServerError, err.Error())
			return
		}
		c.logger.Info("Blob stored", "namespace", namespace, "blob_id", blobID, "size", len(body), "content_type", contentType)
		c.responder.JSON(w, http.StatusOK, models.StatusResponse{Status: constants.GatewayModeStatusOK})

	case http.MethodGet:
		data, contentType, ok := c.db.BlobGet(namespace, blobID)
		if !ok {
			c.responder.Error(w, http.StatusNotFound, "blob not found")
			return
		}

		safeContentType := "application/octet-stream"
		allowedTypes := map[string]bool{
			"application/json":       true,
			"application/pdf":        true,
			"image/png":              true,
			"image/jpeg":             true,
			"image/gif":              true,
			"text/plain":             true,
			"application/x-ndjson":   true,
			"application/x-pem-file": true,
		}
		if allowedTypes[contentType] {
			safeContentType = contentType
		}

		w.Header().Set("Content-Type", safeContentType)
		w.Header().Set("Content-Length", strconv.Itoa(len(data)))
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("X-Frame-Options", "DENY")
		w.Header().Set("Content-Security-Policy", "default-src 'none'; frame-ancestors 'none'")
		w.WriteHeader(http.StatusOK)
		if _, err := w.Write(data); err != nil {
			c.logger.Error("failed to write blob response", "error", err)
		}

	case http.MethodDelete:
		// Check if namespace is allowlisted for direct mutations
		if !blobNamespaceAllowed(namespace) {
			c.responder.Error(w, http.StatusConflict, governanceEnvelopeRedirectError)
			return
		}

		// Enforce ownership for blob deletion
		if err := c.verifyBlobOwnership(r, namespace); err != nil {
			c.logger.Warn("Blob delete: ownership check failed", "namespace", namespace, "blob_id", blobID, "error", err)
			c.responder.Error(w, http.StatusForbidden, err.Error())
			return
		}

		deleted, err := c.db.BlobDelete(namespace, blobID)
		if err != nil {
			c.responder.Error(w, http.StatusInternalServerError, err.Error())
			return
		}
		if !deleted {
			c.responder.Error(w, http.StatusNotFound, "blob not found")
			return
		}
		c.logger.Info("Blob deleted", "namespace", namespace, "blob_id", blobID)
		c.responder.JSON(w, http.StatusOK, models.StatusResponse{Status: constants.GatewayModeStatusOK})

	default:
		c.responder.Error(w, http.StatusMethodNotAllowed, "method not allowed")
	}
}

func (c *DBController) handlePubSubPublish(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		c.responder.Error(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	body, err := c.readBody(r)
	if err != nil {
		c.responder.Error(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	var req models.PubSubPublishRequest
	if err := json.Unmarshal(body, &req); err != nil {
		c.responder.Error(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	if req.Channel == "" {
		c.responder.Error(w, http.StatusBadRequest, "channel required")
		return
	}
	if !isMutationPubSubChannelAllowed(req.Channel) {
		c.responder.Error(w, http.StatusConflict, governanceEnvelopeRedirectError)
		return
	}

	var binData []byte
	var receivers int
	if err := json.Unmarshal(req.Data, &binData); err == nil {
		receivers = c.pubsub.Publish(req.Channel, binData)
	} else {
		receivers = c.pubsub.Publish(req.Channel, req.Data)
	}
	c.responder.JSON(w, http.StatusOK, models.PubSubPublishResponse{Receivers: receivers})
}
