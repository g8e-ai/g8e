// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package gateway

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strconv"
	"strings"

	"github.com/g8e-ai/g8e/internal/config"
	"github.com/g8e-ai/g8e/internal/constants"
	"github.com/g8e-ai/g8e/internal/marshaler"
	"github.com/g8e-ai/g8e/internal/models"
	"github.com/g8e-ai/g8e/internal/response"
)

// DataController handles document DB, KV store, blob storage, SSE events, and pub/sub publish endpoints.
type DataController struct {
	cfg       *config.Config
	logger    *slog.Logger
	docStore  *DocumentStoreService
	kvStore   *KVStoreService
	sseStore  *SSEEventService
	blobStore *BlobStoreService
	pubsub    *GatewayWebSocketHandler
	responder *response.Writer
}

// DataControllerDeps groups all dependencies for DataController.
type DataControllerDeps struct {
	Cfg       *config.Config
	Logger    *slog.Logger
	DocStore  *DocumentStoreService
	KVStore   *KVStoreService
	SSEStore  *SSEEventService
	BlobStore *BlobStoreService
	Pubsub    *GatewayWebSocketHandler
	Responder *response.Writer
}

func newDataController(d DataControllerDeps) *DataController {
	return &DataController{
		cfg:       d.Cfg,
		logger:    d.Logger,
		docStore:  d.DocStore,
		kvStore:   d.KVStore,
		sseStore:  d.SSEStore,
		blobStore: d.BlobStore,
		pubsub:    d.Pubsub,
		responder: d.Responder,
	}
}

func (c *DataController) readBody(r *http.Request) ([]byte, error) {
	return readRequestBody(r, c.cfg.Gateway.MaxPayloadBytes)
}

func (c *DataController) handleDataSettings(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		doc, err := c.docStore.DocGet(marshaler.CollectionName(constants.CollectionSettings), marshaler.DocumentID(constants.DocIDPlatformSettings))
		if err != nil {
			c.responder.Error(w, http.StatusInternalServerError, fmt.Errorf("data_controller: handleDataSettings: %w", err).Error())
			return
		}
		if doc == nil {
			c.responder.Error(w, http.StatusNotFound, constants.ErrNotFound.Error())
			return
		}
		c.responder.JSON(w, http.StatusOK, doc.ForWire())
	case http.MethodPut, http.MethodPatch:
		body, err := c.readBody(r)
		if err != nil {
			c.responder.Error(w, http.StatusBadRequest, constants.ErrInvalidJSONBody.Error())
			return
		}
		if !json.Valid(body) {
			c.responder.Error(w, http.StatusBadRequest, constants.ErrInvalidJSONBody.Error())
			return
		}
		var err2 error
		if r.Method == http.MethodPut {
			err2 = c.docStore.DocSet(marshaler.CollectionName(constants.CollectionSettings), marshaler.DocumentID(constants.DocIDPlatformSettings), json.RawMessage(body))
		} else {
			_, err2 = c.docStore.DocUpdate(marshaler.CollectionName(constants.CollectionSettings), marshaler.DocumentID(constants.DocIDPlatformSettings), json.RawMessage(body))
		}
		if err2 != nil {
			c.responder.Error(w, http.StatusInternalServerError, fmt.Errorf("data_controller: handleDataSettings: %w", err2).Error())
			return
		}
		c.responder.JSON(w, http.StatusOK, models.StatusResponse{Status: constants.GatewayModeStatusOK})
	default:
		c.responder.Error(w, http.StatusMethodNotAllowed, constants.ErrMethodNotAllowed.Error())
	}
}

func (c *DataController) handleDataDB(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, constants.APIPaths.DataPrefix)
	parts := strings.SplitN(path, "/", 2)
	if len(parts) == 0 || parts[0] == "" {
		c.responder.Error(w, http.StatusBadRequest, constants.ErrGatewayCollectionRequired.Error())
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
		c.responder.Error(w, http.StatusBadRequest, constants.ErrGatewayDocumentIDRequired.Error())
		return
	}

	switch r.Method {
	case http.MethodGet:
		doc, err := c.docStore.DocGet(collection, id)
		if err != nil {
			c.responder.Error(w, http.StatusInternalServerError, fmt.Errorf("data_controller: handleDataDB: %w", err).Error())
			return
		}
		if doc == nil {
			c.responder.Error(w, http.StatusNotFound, constants.ErrNotFound.Error())
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
			c.responder.Error(w, http.StatusBadRequest, constants.ErrInvalidJSONBody.Error())
			return
		}
		if !json.Valid(body) {
			c.responder.Error(w, http.StatusBadRequest, constants.ErrInvalidJSONBody.Error())
			return
		}
		if err := c.docStore.DocSet(collection, id, json.RawMessage(body)); err != nil {
			if errors.Is(err, constants.ErrDatabaseLocked) {
				c.responder.Error(w, http.StatusServiceUnavailable, constants.ErrDatabaseLocked.Error())
			} else {
				c.responder.Error(w, http.StatusInternalServerError, fmt.Errorf("data_controller: handleDataDB: %w", err).Error())
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
			c.responder.Error(w, http.StatusBadRequest, constants.ErrInvalidJSONBody.Error())
			return
		}
		if !json.Valid(body) {
			c.responder.Error(w, http.StatusBadRequest, constants.ErrInvalidJSONBody.Error())
			return
		}
		doc, err := c.docStore.DocUpdate(collection, id, json.RawMessage(body))
		if err != nil {
			if errors.Is(err, constants.ErrNotFound) {
				c.responder.Error(w, http.StatusNotFound, err.Error())
			} else if errors.Is(err, constants.ErrConstraintViolation) {
				c.responder.Error(w, http.StatusConflict, constants.ErrConstraintViolation.Error())
			} else if errors.Is(err, constants.ErrDatabaseLocked) {
				c.responder.Error(w, http.StatusServiceUnavailable, constants.ErrDatabaseLocked.Error())
			} else {
				c.responder.Error(w, http.StatusInternalServerError, fmt.Errorf("data_controller: handleDataDB: %w", err).Error())
			}
			return
		}
		c.responder.JSON(w, http.StatusOK, doc.ForWire())

	case http.MethodDelete:
		if !isDirectDBMutationAllowed(collection) {
			c.responder.Error(w, http.StatusConflict, governanceEnvelopeRedirectError)
			return
		}
		deleted, err := c.docStore.DocDeleteWithResult(collection, id)
		if err != nil {
			c.responder.Error(w, http.StatusInternalServerError, fmt.Errorf("data_controller: handleDataDB: %w", err).Error())
			return
		}
		if !deleted {
			c.responder.Error(w, http.StatusNotFound, constants.ErrNotFound.Error())
			return
		}
		c.responder.JSON(w, http.StatusOK, models.StatusResponse{Status: constants.GatewayModeStatusOK})

	default:
		c.responder.Error(w, http.StatusMethodNotAllowed, constants.ErrMethodNotAllowed.Error())
	}
}

func (c *DataController) handleDBQuery(w http.ResponseWriter, r *http.Request, collection string) {
	body, err := c.readBody(r)
	if err != nil {
		c.responder.Error(w, http.StatusBadRequest, constants.ErrInvalidJSONBody.Error())
		return
	}

	var req models.DocQueryRequest
	if len(body) > 0 {
		if err := json.Unmarshal(body, &req); err != nil {
			c.responder.Error(w, http.StatusBadRequest, constants.ErrInvalidJSONBody.Error())
			return
		}
	}

	docs, err := c.docStore.DocQuery(collection, req.Filters, req.OrderBy, req.Limit)
	if err != nil {
		c.responder.Error(w, http.StatusInternalServerError, fmt.Errorf("data_controller: handleDBQuery: %w", err).Error())
		return
	}
	wire := make([]map[string]json.RawMessage, 0, len(docs))
	for _, d := range docs {
		wire = append(wire, d.ForWire())
	}
	c.responder.JSON(w, http.StatusOK, wire)
}

func (c *DataController) handleSSEEvents(w http.ResponseWriter, r *http.Request, id string) {
	if id == "count" && r.Method == http.MethodGet {
		count, err := c.sseStore.SSEEventsCount()
		if err != nil {
			c.responder.Error(w, http.StatusInternalServerError, fmt.Errorf("data_controller: handleSSEEvents: %w", err).Error())
			return
		}
		c.responder.JSON(w, http.StatusOK, models.SSEEventsCountResponse{Count: count})
		return
	}

	if id == "" && r.Method == http.MethodDelete {
		deleted, err := c.sseStore.SSEEventsWipe()
		if err != nil {
			c.responder.Error(w, http.StatusInternalServerError, fmt.Errorf("data_controller: handleSSEEvents: %w", err).Error())
			return
		}
		c.responder.JSON(w, http.StatusOK, models.SSEEventsWipeResponse{Deleted: deleted})
		return
	}

	c.responder.Error(w, http.StatusMethodNotAllowed, constants.ErrMethodNotAllowed.Error())
}

func (c *DataController) handleKV(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, constants.APIPaths.KVPrefix)
	if path == "" {
		c.responder.Error(w, http.StatusBadRequest, constants.ErrDBControllerKeyRequired.Error())
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
		ttl := c.kvStore.KVTTL(key)
		c.responder.JSON(w, http.StatusOK, models.KVTTLResponse{TTL: ttl})
		return
	}
	if strings.HasSuffix(path, "/_expire") && r.Method == http.MethodPut {
		key := strings.TrimSuffix(path, "/_expire")
		body, err := c.readBody(r)
		if err != nil {
			c.responder.Error(w, http.StatusBadRequest, constants.ErrInvalidJSONBody.Error())
			return
		}
		var req models.KVExpireRequest
		if err := json.Unmarshal(body, &req); err != nil {
			c.responder.Error(w, http.StatusBadRequest, constants.ErrInvalidJSONBody.Error())
			return
		}
		if req.TTL <= 0 {
			c.responder.Error(w, http.StatusBadRequest, constants.ErrDBControllerTTLRequired.Error())
			return
		}
		ok := c.kvStore.KVExpire(key, req.TTL)
		if !ok {
			c.responder.Error(w, http.StatusNotFound, constants.ErrNotFound.Error())
			return
		}
		c.responder.JSON(w, http.StatusOK, models.StatusResponse{Status: constants.GatewayModeStatusOK})
		return
	}

	key := path

	switch r.Method {
	case http.MethodGet:
		value, ok := c.kvStore.KVGet(key)
		if !ok {
			c.responder.Error(w, http.StatusNotFound, constants.ErrNotFound.Error())
			return
		}
		c.responder.JSON(w, http.StatusOK, models.KVGetResponse{Value: value})

	case http.MethodPut:
		body, err := c.readBody(r)
		if err != nil {
			c.responder.Error(w, http.StatusBadRequest, constants.ErrInvalidJSONBody.Error())
			return
		}
		var req models.KVSetRequest
		if err := json.Unmarshal(body, &req); err != nil {
			c.responder.Error(w, http.StatusBadRequest, constants.ErrInvalidJSONBody.Error())
			return
		}
		if err := c.kvStore.KVSet(key, req.Value, req.TTL); err != nil {
			c.responder.Error(w, http.StatusInternalServerError, fmt.Errorf("data_controller: handleKV: %w", err).Error())
			return
		}
		c.responder.JSON(w, http.StatusOK, models.StatusResponse{Status: constants.GatewayModeStatusOK})

	case http.MethodDelete:
		if err := c.kvStore.KVDelete(key); err != nil {
			c.responder.Error(w, http.StatusInternalServerError, fmt.Errorf("data_controller: handleKV: %w", err).Error())
			return
		}
		c.responder.JSON(w, http.StatusOK, models.StatusResponse{Status: constants.GatewayModeStatusOK})

	default:
		c.responder.Error(w, http.StatusMethodNotAllowed, constants.ErrMethodNotAllowed.Error())
	}
}

func (c *DataController) handleKVKeys(w http.ResponseWriter, r *http.Request) {
	body, err := c.readBody(r)
	if err != nil {
		c.responder.Error(w, http.StatusBadRequest, constants.ErrInvalidJSONBody.Error())
		return
	}
	var req models.KVPatternRequest
	if len(body) > 0 {
		if err := json.Unmarshal(body, &req); err != nil {
			c.responder.Error(w, http.StatusBadRequest, constants.ErrInvalidJSONBody.Error())
			return
		}
	}
	if req.Pattern == "" {
		req.Pattern = "*"
	}
	keys, err := c.kvStore.KVKeys(req.Pattern)
	if err != nil {
		c.responder.Error(w, http.StatusInternalServerError, fmt.Errorf("data_controller: handleKVKeys: %w", err).Error())
		return
	}
	if keys == nil {
		keys = []string{}
	}
	c.responder.JSON(w, http.StatusOK, models.KVKeysResponse{Keys: keys})
}

func (c *DataController) handleKVScan(w http.ResponseWriter, r *http.Request) {
	body, err := c.readBody(r)
	if err != nil {
		c.responder.Error(w, http.StatusBadRequest, constants.ErrInvalidJSONBody.Error())
		return
	}
	var req models.KVPatternRequest
	if len(body) > 0 {
		if err := json.Unmarshal(body, &req); err != nil {
			c.responder.Error(w, http.StatusBadRequest, constants.ErrInvalidJSONBody.Error())
			return
		}
	}
	if req.Pattern == "" {
		req.Pattern = "*"
	}
	if req.Count <= 0 {
		req.Count = 100
	}
	nextCursor, keys, err := c.kvStore.KVScan(req.Pattern, req.Cursor, req.Count)
	if err != nil {
		c.responder.Error(w, http.StatusInternalServerError, fmt.Errorf("data_controller: handleKVScan: %w", err).Error())
		return
	}
	if keys == nil {
		keys = []string{}
	}
	c.responder.JSON(w, http.StatusOK, models.KVScanResponse{Cursor: nextCursor, Keys: keys})
}

func (c *DataController) handleKVDeletePattern(w http.ResponseWriter, r *http.Request) {
	body, err := c.readBody(r)
	if err != nil {
		c.responder.Error(w, http.StatusBadRequest, constants.ErrInvalidJSONBody.Error())
		return
	}
	var req models.KVPatternRequest
	if err := json.Unmarshal(body, &req); err != nil {
		c.responder.Error(w, http.StatusBadRequest, constants.ErrInvalidJSONBody.Error())
		return
	}
	if req.Pattern == "" {
		c.responder.Error(w, http.StatusBadRequest, constants.ErrDBControllerPatternRequired.Error())
		return
	}
	count, err := c.kvStore.KVDeletePattern(req.Pattern)
	if err != nil {
		c.responder.Error(w, http.StatusInternalServerError, fmt.Errorf("data_controller: handleKVDeletePattern: %w", err).Error())
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
	allowedNamespaces := map[string]bool{
		"temp":    true,
		"uploads": true,
		"cache":   true,
		"scratch": true,
	}
	return allowedNamespaces[namespace]
}

// extractCallerIdentity extracts the caller's identity from the request context.
// Returns user_id, app_id, operator_session_id, cli_session_id.
func (c *DataController) extractCallerIdentity(r *http.Request) (string, string, string, string) {
	userID, ok := r.Context().Value(constants.ContextKeyUserID).(string)
	if !ok {
		userID = ""
	}
	appID, ok := r.Context().Value(constants.ContextKeyAppID).(string)
	if !ok {
		appID = ""
	}
	operatorSessionID, _ := r.Context().Value(constants.ContextKeyOperatorSessionID).(string)
	cliSessionID := r.Header.Get(constants.HeaderCLISessionID)
	return userID, appID, operatorSessionID, cliSessionID
}

// verifyBlobOwnership checks if the caller is authorized to mutate the given namespace.
// This enforces per-namespace ownership to prevent cross-tenant blob access.
// Allowlisted namespaces (temp, cache, uploads, scratch) are accessible by any authenticated user.
func (c *DataController) verifyBlobOwnership(r *http.Request, namespace string) error {
	userID, appID, operatorSessionID, cliSessionID := c.extractCallerIdentity(r)

	if userID == "" && appID == "" && operatorSessionID == "" && cliSessionID == "" {
		return constants.ErrUnauthorizedNoIdentity
	}

	if blobNamespaceAllowed(namespace) {
		return nil
	}

	if appID != "" {
		expectedNamespace := "app/" + appID
		if namespace != expectedNamespace {
			return constants.ErrUnauthorizedAppNamespace
		}
		return nil
	}

	if operatorSessionID != "" || cliSessionID != "" {
		if userID == "" {
			return constants.ErrUnauthorizedOperatorNoUserID
		}
		expectedNamespace := "user/" + userID
		if namespace != expectedNamespace {
			return constants.ErrUnauthorizedUserNamespace
		}
		return nil
	}

	if userID != "" {
		expectedNamespace := "user/" + userID
		if namespace != expectedNamespace {
			return constants.ErrUnauthorizedUserNamespace
		}
		return nil
	}

	return constants.ErrUnauthorizedUnknownIdentity
}

// @Summary		Get blob
// @Description	Retrieves a blob from the data store
// @Tags			data
// @Produce		application/octet-stream
// @Success		200	{file}	file
// @Router			/api/v1/data/blobs/{namespace}/{id} [get]
func (c *DataController) handleBlob(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, constants.APIPaths.DataBlobsPrefix)
	if path == "" {
		c.responder.Error(w, http.StatusBadRequest, constants.ErrDBControllerNamespaceRequired.Error())
		return
	}

	parts := strings.SplitN(path, "/", 3)
	namespace := parts[0]
	if !blobSegmentValid(namespace) {
		c.responder.Error(w, http.StatusBadRequest, constants.ErrDBControllerInvalidNamespace.Error())
		return
	}

	if len(parts) == 1 {
		if r.Method != http.MethodDelete {
			c.responder.Error(w, http.StatusMethodNotAllowed, constants.ErrMethodNotAllowed.Error())
			return
		}
		if !blobNamespaceAllowed(namespace) {
			c.responder.Error(w, http.StatusConflict, governanceEnvelopeRedirectError)
			return
		}
		if err := c.verifyBlobOwnership(r, namespace); err != nil {
			c.logger.Warn("Blob namespace delete: ownership check failed", "namespace", namespace, "error", err)
			c.responder.Error(w, http.StatusForbidden, err.Error())
			return
		}
		count, err := c.blobStore.BlobDeleteNamespace(namespace)
		if err != nil {
			c.responder.Error(w, http.StatusInternalServerError, fmt.Errorf("data_controller: handleBlob: %w", err).Error())
			return
		}
		c.logger.Info("Blob namespace deleted", "namespace", namespace, "count", count)
		c.responder.JSON(w, http.StatusOK, models.BlobDeleteResponse{Deleted: count})
		return
	}

	blobID := parts[1]
	if !blobSegmentValid(blobID) {
		c.responder.Error(w, http.StatusBadRequest, constants.ErrDBControllerInvalidBlobID.Error())
		return
	}

	if len(parts) == 3 {
		if parts[2] != "meta" {
			c.responder.Error(w, http.StatusBadRequest, constants.ErrDBControllerInvalidPath.Error())
			return
		}
		if r.Method != http.MethodGet {
			c.responder.Error(w, http.StatusMethodNotAllowed, constants.ErrMethodNotAllowed.Error())
			return
		}
		rec, ok := c.blobStore.BlobMeta(namespace, blobID)
		if !ok {
			c.responder.Error(w, http.StatusNotFound, constants.ErrNotFound.Error())
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
		if !blobNamespaceAllowed(namespace) {
			c.responder.Error(w, http.StatusConflict, governanceEnvelopeRedirectError)
			return
		}
		if err := c.verifyBlobOwnership(r, namespace); err != nil {
			c.logger.Warn("Blob put: ownership check failed", "namespace", namespace, "blob_id", blobID, "error", err)
			c.responder.Error(w, http.StatusForbidden, err.Error())
			return
		}
		contentType := r.Header.Get("Content-Type")
		if contentType == "" {
			c.responder.Error(w, http.StatusBadRequest, constants.ErrDBControllerContentTypeRequired.Error())
			return
		}
		ttl := 0
		if v := r.Header.Get("X-Blob-TTL"); v != "" {
			n, err := strconv.Atoi(v)
			if err != nil || n < 0 {
				c.responder.Error(w, http.StatusBadRequest, constants.ErrDBControllerInvalidTTL.Error())
				return
			}
			ttl = n
		}
		body, err := io.ReadAll(io.LimitReader(r.Body, maxBlobBodySize+1))
		if err != nil {
			c.responder.Error(w, http.StatusBadRequest, constants.ErrDBControllerBodyReadFailed.Error())
			return
		}
		if int64(len(body)) > maxBlobBodySize {
			c.responder.Error(w, http.StatusRequestEntityTooLarge, constants.ErrDBControllerBlobTooLarge.Error())
			return
		}
		if len(body) == 0 {
			c.responder.Error(w, http.StatusBadRequest, constants.ErrDBControllerBodyEmpty.Error())
			return
		}
		if err := c.blobStore.BlobPut(namespace, blobID, body, contentType, ttl); err != nil {
			c.responder.Error(w, http.StatusInternalServerError, fmt.Errorf("data_controller: handleBlob: %w", err).Error())
			return
		}
		c.logger.Info("Blob stored", "namespace", namespace, "blob_id", blobID, "size", len(body), "content_type", contentType)
		c.responder.JSON(w, http.StatusOK, models.StatusResponse{Status: constants.GatewayModeStatusOK})

	case http.MethodGet:
		data, contentType, ok := c.blobStore.BlobGet(namespace, blobID)
		if !ok {
			c.responder.Error(w, http.StatusNotFound, constants.ErrNotFound.Error())
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
		if !blobNamespaceAllowed(namespace) {
			c.responder.Error(w, http.StatusConflict, governanceEnvelopeRedirectError)
			return
		}
		if err := c.verifyBlobOwnership(r, namespace); err != nil {
			c.logger.Warn("Blob delete: ownership check failed", "namespace", namespace, "blob_id", blobID, "error", err)
			c.responder.Error(w, http.StatusForbidden, err.Error())
			return
		}
		deleted, err := c.blobStore.BlobDelete(namespace, blobID)
		if err != nil {
			c.responder.Error(w, http.StatusInternalServerError, fmt.Errorf("data_controller: handleBlob: %w", err).Error())
			return
		}
		if !deleted {
			c.responder.Error(w, http.StatusNotFound, constants.ErrNotFound.Error())
			return
		}
		c.logger.Info("Blob deleted", "namespace", namespace, "blob_id", blobID)
		c.responder.JSON(w, http.StatusOK, models.StatusResponse{Status: constants.GatewayModeStatusOK})

	default:
		c.responder.Error(w, http.StatusMethodNotAllowed, constants.ErrMethodNotAllowed.Error())
	}
}

func (c *DataController) handlePubSubPublish(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		c.responder.Error(w, http.StatusMethodNotAllowed, constants.ErrMethodNotAllowed.Error())
		return
	}

	body, err := c.readBody(r)
	if err != nil {
		c.responder.Error(w, http.StatusBadRequest, constants.ErrInvalidJSONBody.Error())
		return
	}
	var req models.PubSubPublishRequest
	if err := json.Unmarshal(body, &req); err != nil {
		c.responder.Error(w, http.StatusBadRequest, constants.ErrInvalidJSONBody.Error())
		return
	}
	if req.Channel == "" {
		c.responder.Error(w, http.StatusBadRequest, constants.ErrDBControllerChannelRequired.Error())
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

var governanceEnvelopeRedirectError = fmt.Sprintf("submit via POST %s", constants.APIPaths.GovernanceEnvelopes)

func isDirectDBMutationAllowed(collection string) bool {
	switch constants.CollectionName(collection) {
	case constants.CollectionSettings,
		constants.CollectionUsers,
		constants.CollectionOperators,
		constants.CollectionOperatorSessions,
		constants.CollectionBoundSessions,
		constants.CollectionPasskeyChallenges,
		constants.CollectionRevokedCertificates,
		constants.CollectionTrustedSigners,
		constants.CollectionConsoleAudit:
		return true
	case constants.CollectionCases,
		constants.CollectionInvestigations,
		constants.CollectionTasks,
		constants.CollectionMemories,
		constants.CollectionReputationState,
		constants.CollectionReputationCommitments,
		constants.CollectionAgentActivityMetadata,
		constants.CollectionStakeResolutions:
		return false
	default:
		return false
	}
}

func isMutationPubSubChannelAllowed(channel string) bool {
	for _, prefix := range []string{"heartbeat:", "results:", "sse:", "ws_session:", "internal:"} {
		if strings.HasPrefix(channel, prefix) {
			return true
		}
	}
	return false
}
