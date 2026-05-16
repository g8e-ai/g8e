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
	"fmt"
	"log/slog"
	"net"
	"time"

	"github.com/g8e-ai/g8e/components/g8eo/internal/config"
	"github.com/g8e-ai/g8e/components/g8eo/internal/constants"
	"github.com/g8e-ai/g8e/components/g8eo/internal/protocol/proto/operatorv1"
	"google.golang.org/protobuf/proto"
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
	var protoPort operatorv1.CheckPortRequested
	if err := proto.Unmarshal(msg.Payload, &protoPort); err != nil {
		ps.logger.Error("Failed to decode port check payload as protobuf CheckPortRequested", "error", err)
		publishLFAAErrorTo(ctx, ps.client, ps.config, ps.logger, msg, constants.Event.Operator.PortCheck.Failed, "invalid request payload")
		return
	}

	if protoPort.Port <= 0 || protoPort.Port > 65535 {
		ps.logger.Warn("Port check request with invalid port", "port", protoPort.Port)
		publishLFAAErrorTo(ctx, ps.client, ps.config, ps.logger, msg, constants.Event.Operator.PortCheck.Failed, "port must be between 1 and 65535")
		return
	}

	host := protoPort.Host
	if host == "" {
		host = "localhost"
	}
	protocol := protoPort.Protocol
	if protocol == "" {
		protocol = "tcp"
	}

	executionID := executionIDFromMessage(msg)

	ps.logger.Info("Port check requested (via Protobuf)", "host", host, "port", protoPort.Port, "protocol", protocol)

	start := time.Now()
	address := net.JoinHostPort(host, fmt.Sprintf("%d", protoPort.Port))
	conn, dialErr := net.DialTimeout(protocol, address, 5*time.Second)
	latencyMs := time.Since(start).Seconds() * 1000

	entry := &operatorv1.PortCheckEntry{
		Host: host,
		Port: protoPort.Port,
		Open: dialErr == nil,
	}
	if dialErr == nil {
		conn.Close()
		entry.LatencyMs = float32(latencyMs)
	} else {
		entry.Error = dialErr.Error()
	}

	payload := &operatorv1.PortCheckResult{
		ExecutionId: executionID,
		Status:      protoExecutionStatus(constants.ExecutionStatusCompleted),
		Results:     []*operatorv1.PortCheckEntry{entry},
	}
	publishLFAATypedResponseTo(ctx, ps.client, ps.config, ps.logger, msg, constants.Event.Operator.PortCheck.Completed, payload)
}
