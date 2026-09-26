// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package gateway

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

	"github.com/g8e-ai/g8e/v2/internal/constants"
	"github.com/g8e-ai/g8e/v2/internal/models"
	"github.com/g8e-ai/g8e/v2/internal/services/pubsub"
)

const auditIngestTimeout = 10 * time.Second

// AuditIngestPublisher publishes audit records to operator channels and waits for acks.
type AuditIngestPublisher interface {
	Publish(channel string, data []byte) int
	RegisterHandler(channel string, handler func(string, []byte)) func()
}

// AuditIngestService forwards LFAA audit records to operators over audit: channels.
type AuditIngestService struct {
	auth   *AuthService
	pubsub AuditIngestPublisher
	logger *slog.Logger
}

// NewAuditIngestService constructs an AuditIngestService.
func NewAuditIngestService(auth *AuthService, pubsub AuditIngestPublisher, logger *slog.Logger) *AuditIngestService {
	return &AuditIngestService{
		auth:   auth,
		pubsub: pubsub,
		logger: logger,
	}
}

// Ingest validates the request, verifies operator session binding, publishes to audit:,
// and waits for the operator acknowledgement containing chain metadata.
func (s *AuditIngestService) Ingest(ctx context.Context, req models.AuditRecordIngestRequest, requestorUserID string) (*models.AuditRecordIngestResponse, error) {
	if err := validateAuditRecordIngestRequest(&req); err != nil {
		return nil, err
	}
	if _, err := constants.ValidateAuditRecordRequest(constants.EventType(req.EventType)); err != nil {
		return nil, err
	}

	op, err := s.auth.ValidateOperatorSession(req.OperatorSessionID)
	if err != nil {
		return nil, err
	}
	if op.ID != req.OperatorID {
		return nil, fmt.Errorf("%w: operator_id does not match session", constants.ErrAuditIngestInvalidRequest)
	}
	if requestorUserID != "" && op.UserID != requestorUserID {
		return nil, constants.ErrRegistrationOperatorNotBelongToUser
	}

	wire := models.AuditRecordPublish{
		EventType:         req.EventType,
		OperatorID:        req.OperatorID,
		OperatorSessionID: req.OperatorSessionID,
		IdempotencyKey:    req.IdempotencyKey,
		Payload:           req.Payload,
		CaseID:            req.CaseID,
		InvestigationID:   req.InvestigationID,
		TaskID:            req.TaskID,
		WebSessionID:      req.WebSessionID,
		CliSessionID:      req.CliSessionID,
		RequestorUserID:   requestorUserID,
	}
	wireBytes, err := json.Marshal(wire)
	if err != nil {
		return nil, fmt.Errorf("audit ingest: marshal publish payload: %w", err)
	}

	resultsChannel := pubsub.ResultsChannel(req.OperatorID, req.OperatorSessionID)
	ackCh := make(chan *models.AuditRecordAck, 1)
	unregister := s.pubsub.RegisterHandler(resultsChannel, func(_ string, data []byte) {
		var ack models.AuditRecordAck
		if err := json.Unmarshal(data, &ack); err != nil {
			s.logger.Warn("audit ingest: failed to decode ack", "error", err)
			return
		}
		if ack.IdempotencyKey != req.IdempotencyKey {
			return
		}
		select {
		case ackCh <- &ack:
		default:
		}
	})
	defer unregister()

	auditChannel := pubsub.AuditChannel(req.OperatorID, req.OperatorSessionID)
	delivered := s.pubsub.Publish(auditChannel, wireBytes)
	if delivered == 0 {
		return nil, fmt.Errorf("audit ingest: %w", constants.ErrAuditIngestNoDelivery)
	}

	timeoutCtx, cancel := context.WithTimeout(ctx, auditIngestTimeout)
	defer cancel()

	select {
	case <-timeoutCtx.Done():
		return nil, fmt.Errorf("audit ingest: %w", constants.ErrAuditIngestTimeout)
	case ack := <-ackCh:
		return &models.AuditRecordIngestResponse{
			Seq:  ack.Seq,
			Hash: ack.Hash,
		}, nil
	}
}

func validateAuditRecordIngestRequest(req *models.AuditRecordIngestRequest) error {
	if req == nil {
		return constants.ErrAuditIngestInvalidRequest
	}
	if req.EventType == "" {
		return constants.ErrTxUnknownEventType
	}
	if req.OperatorID == "" || req.OperatorSessionID == "" {
		return constants.ErrAuditSessionMissing
	}
	if req.IdempotencyKey == "" {
		return fmt.Errorf("%w: idempotency_key required", constants.ErrAuditIngestInvalidRequest)
	}
	if len(req.Payload) == 0 {
		return constants.ErrTxPayloadMissing
	}
	return nil
}
