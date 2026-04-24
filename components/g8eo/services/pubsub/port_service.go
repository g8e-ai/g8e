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

package pubsub

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net"
	"time"

	"github.com/g8e-ai/g8e/components/g8eo/config"
	"github.com/g8e-ai/g8e/components/g8eo/constants"
	"github.com/g8e-ai/g8e/components/g8eo/models"
)

// PortService owns port connectivity check handling.
type PortService struct {
	config *config.Config
	logger *slog.Logger
	client PubSubClient
}

// NewPortService creates a new PortService.
func NewPortService(cfg *config.Config, logger *slog.Logger, client PubSubClient) *PortService {
	return &PortService{
		config: cfg,
		logger: logger,
		client: client,
	}
}

// HandlePortCheckRequest processes an inbound port check request.
func (ps *PortService) HandlePortCheckRequest(ctx context.Context, msg PubSubCommandMessage) {
	var p models.PortCheckRequestPayload
	if err := json.Unmarshal(msg.Payload, &p); err != nil {
		ps.logger.Error("Failed to decode port check payload", "error", err)
		publishLFAAErrorTo(ctx, ps.client, ps.config, ps.logger, msg, constants.Event.Operator.PortCheck.Failed, "invalid request payload")
		return
	}
	if p.Port <= 0 || p.Port > 65535 {
		ps.logger.Warn("Port check request with invalid port", "port", p.Port)
		publishLFAAErrorTo(ctx, ps.client, ps.config, ps.logger, msg, constants.Event.Operator.PortCheck.Failed, "port must be between 1 and 65535")
		return
	}

	host := p.Host
	if host == "" {
		host = "localhost"
	}
	protocol := p.Protocol
	if protocol == "" {
		protocol = "tcp"
	}

	executionID := executionIDFromMessage(msg)

	ps.logger.Info("Port check requested", "host", host, "port", p.Port, "protocol", protocol)

	start := time.Now()
	address := net.JoinHostPort(host, fmt.Sprintf("%d", p.Port))
	conn, dialErr := net.DialTimeout(protocol, address, 5*time.Second)
	latencyMs := time.Since(start).Seconds() * 1000

	entry := models.PortCheckEntry{
		Host: host,
		Port: p.Port,
		Open: dialErr == nil,
	}
	if dialErr == nil {
		conn.Close()
		ms := latencyMs
		entry.LatencyMs = &ms
	} else {
		errMsg := dialErr.Error()
		entry.Error = &errMsg
	}

	payload := models.PortCheckResultPayload{
		ExecutionID:       executionID,
		Status:            constants.ExecutionStatusCompleted,
		OperatorID:        ps.config.OperatorID,
		OperatorSessionID: ps.config.OperatorSessionId,
		Results:           []models.PortCheckEntry{entry},
	}
	publishLFAATypedResponseTo(ctx, ps.client, ps.config, ps.logger, msg, constants.Event.Operator.PortCheck.Completed, payload)
}
