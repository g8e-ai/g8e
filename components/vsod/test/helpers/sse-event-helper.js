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

/**
 * SSE Event Testing Helper
 * 
 * Provides utilities for testing SSE event flows in E2E tests
 * Helps simulate g8ee events and validate event reception
 */

import { EventType } from '../../constants/events.js';
import { now } from '@vsod/models/base.js';
import {
    ConnectionEstablishedEvent,
    KeepaliveEvent,
    HeartbeatSSEEvent,
    OperatorStatusUpdatedEvent,
    OperatorStatusUpdatedData,
    G8eePassthroughEvent,
} from '@vsod/models/sse_models.js';

/**
 * Create mock SSE events for testing
 */
export const mockSSEEvents = {
  /**
   * Create a connection established event
   */
  connectionEstablished(sessionId) {
    return new ConnectionEstablishedEvent({
      type: EventType.PLATFORM_SSE_CONNECTION_ESTABLISHED,
      connectionId: sessionId,
    });
  },

  keepalive() {
    return new KeepaliveEvent({
      type: EventType.PLATFORM_SSE_KEEPALIVE_SENT,
      serverTime: Date.now(),
    });
  },

  chatThinkingStart(investigationId, webSessionId, thinkingText = 'Analyzing...', metadata = {}) {
    return new G8eePassthroughEvent({
      _payload: {
        type: EventType.LLM_CHAT_ITERATION_THINKING_STARTED,
        data: {
          investigation_id: investigationId,
          web_session_id: webSessionId,
          thinking: thinkingText,
          action_type: 'start',
          timestamp: now(),
          ...metadata,
        },
      },
    });
  },

  chatThinkingUpdate(investigationId, webSessionId, thinkingText = 'Processing...', metadata = {}) {
    return new G8eePassthroughEvent({
      _payload: {
        type: EventType.LLM_CHAT_ITERATION_THINKING_STARTED,
        data: {
          investigation_id: investigationId,
          web_session_id: webSessionId,
          thinking: thinkingText,
          action_type: 'update',
          timestamp: now(),
          ...metadata,
        },
      },
    });
  },

  chatThinkingEnd(investigationId, webSessionId, metadata = {}) {
    return new G8eePassthroughEvent({
      _payload: {
        type: EventType.LLM_CHAT_ITERATION_THINKING_STARTED,
        data: {
          investigation_id: investigationId,
          web_session_id: webSessionId,
          thinking: '',
          action_type: 'end',
          timestamp: now(),
          ...metadata,
        },
      },
    });
  },

  chatResponseChunk(investigationId, content, metadata = {}) {
    return new G8eePassthroughEvent({
      _payload: {
        type: EventType.LLM_CHAT_ITERATION_TEXT_CHUNK_RECEIVED,
        data: {
          investigation_id: investigationId,
          web_session_id: metadata.web_session_id || null,
          content: content,
          chunk_type: 'chat_response',
          workflow_type: metadata.workflow_type || 'operator_bound',
          grounding_detected: metadata.grounding_detected,
          timestamp: now(),
          ...metadata,
        },
      },
    });
  },

  aiSearchWebRequested(investigationId, webSessionId, info = {}) {
    return new G8eePassthroughEvent({
      _payload: {
        type: EventType.LLM_TOOL_G8E_WEB_SEARCH_REQUESTED,
        data: {
          investigation_id: investigationId,
          web_session_id: webSessionId,
          query: info.query || 'check disk usage',
          execution_id: info.execution_id || `search_${Date.now()}`,
          status: 'started',
          timestamp: now(),
        },
      },
    });
  },

  operatorNetworkPortCheckRequested(investigationId, webSessionId, info = {}) {
    return new G8eePassthroughEvent({
      _payload: {
        type: EventType.OPERATOR_NETWORK_PORT_CHECK_REQUESTED,
        data: {
          investigation_id: investigationId,
          web_session_id: webSessionId,
          port: info.port || '443',
          execution_id: info.execution_id || `port_${Date.now()}`,
          status: 'started',
          timestamp: now(),
        },
      },
    });
  },

  chatResponseComplete(investigationId, messageId, metadata = {}) {
    return new G8eePassthroughEvent({
      _payload: {
        type: EventType.LLM_CHAT_ITERATION_TEXT_COMPLETED,
        data: {
          investigation_id: investigationId,
          web_session_id: metadata.web_session_id || null,
          message_id: messageId,
          finish_reason: metadata.finish_reason || 'STOP',
          has_citations: metadata.has_citations,
          timestamp: now(),
          ...metadata,
        },
      },
    });
  },

  chatCitationsReady(investigationId, webSessionId, groundingMetadata = {}) {
    return new G8eePassthroughEvent({
      _payload: {
        type: EventType.LLM_CHAT_ITERATION_CITATIONS_RECEIVED,
        data: {
          investigation_id: investigationId,
          web_session_id: webSessionId,
          grounding_metadata: {
            grounding_used: true,
            sources: [],
            ...groundingMetadata,
          },
          timestamp: now(),
        },
      },
    });
  },

  chatError(investigationId, error, metadata = {}) {
    return new G8eePassthroughEvent({
      _payload: {
        type: EventType.LLM_CHAT_ITERATION_FAILED,
        data: {
          investigation_id: investigationId,
          web_session_id: metadata.web_session_id || null,
          error: error,
          error_type: metadata.error_type || 'streaming_error',
          timestamp: now(),
          ...metadata,
        },
      },
    });
  },

  operatorHeartbeat(operatorId, status = 'Active') {
    return new HeartbeatSSEEvent({
      type: EventType.OPERATOR_HEARTBEAT_SENT,
      operator_id: operatorId,
      data: { operator_id: operatorId, status },
    });
  },

  operatorStatusUpdate(operatorId, status, previousStatus = null) {
    return new OperatorStatusUpdatedEvent({
      type: EventType.OPERATOR_STATUS_UPDATED_ACTIVE,
      data: new OperatorStatusUpdatedData({
        operator_id: operatorId,
        status,
      }),
    });
  },

  caseCreated(caseId, title, metadata = {}) {
    return new G8eePassthroughEvent({
      _payload: {
        type: EventType.CASE_CREATED,
        data: {
          case_id: caseId,
          title,
          timestamp: now(),
          ...metadata,
        },
      },
    });
  },

  investigationCreated(investigationId, caseId, metadata = {}) {
    return new G8eePassthroughEvent({
      _payload: {
        type: EventType.INVESTIGATION_CREATED,
        data: {
          investigation_id: investigationId,
          case_id: caseId,
          timestamp: now(),
          ...metadata,
        },
      },
    });
  },
};

/**
 * Simulate a complete AI chat streaming flow with realistic event sequences.
 *
 * @param {Object} sseService - VSODB KV SSE service
 * @param {string} sessionId - SSE session ID (also used as web_session_id)
 * @param {string} investigationId - Investigation ID
 * @param {string} flowType - One of: thinking_then_text, thinking_then_tool_then_text
 * @param {Object} options - { chunks: string[] }
 */
export async function simulateAIChatFlow(sseService, sessionId, investigationId, flowType, options = {}) {
  const delay = (ms) => new Promise(resolve => setTimeout(resolve, ms));
  const events = [];

  async function publish(event) {
    await sseService.publishEvent(sessionId, event);
    events.push(event);
    await delay(10);
  }

  async function thinkingPhase(text = 'Analyzing...') {
    await publish(mockSSEEvents.chatThinkingStart(investigationId, sessionId, text));
    await publish(mockSSEEvents.chatThinkingUpdate(investigationId, sessionId, 'Processing...'));
    await publish(mockSSEEvents.chatThinkingEnd(investigationId, sessionId));
  }

  async function textChunks(chunks) {
    for (const chunk of chunks) {
      await publish(mockSSEEvents.chatResponseChunk(investigationId, chunk, { web_session_id: sessionId }));
    }
  }

  async function complete() {
    await publish(mockSSEEvents.chatResponseComplete(investigationId, `msg_${Date.now()}`, {
      web_session_id: sessionId, finish_reason: 'STOP'
    }));
  }

  switch (flowType) {
    case 'thinking_then_text': {
      await thinkingPhase();
      await textChunks(options.chunks || ['Hello.']);
      await complete();
      break;
    }
    case 'thinking_then_tool_then_text': {
      await thinkingPhase();
      await publish(mockSSEEvents.aiSearchWebRequested(investigationId, sessionId, { query: 'check disk usage' }));
      await thinkingPhase('Analyzing results...');
      await textChunks(options.chunks || ['Done.']);
      await complete();
      break;
    }
    case 'multi_turn_function': {
      await thinkingPhase('Checking disk space...');
      await publish(mockSSEEvents.aiSearchWebRequested(investigationId, sessionId, { query: 'disk usage' }));
      await thinkingPhase('Now checking memory...');
      await publish(mockSSEEvents.aiSearchWebRequested(investigationId, sessionId, { query: 'memory usage' }));
      await thinkingPhase('Compiling results...');
      await textChunks(options.chunks || ['Done.']);
      await complete();
      break;
    }
    case 'error_mid_stream': {
      await thinkingPhase();
      await publish(mockSSEEvents.chatResponseChunk(investigationId, 'Partial response...', { web_session_id: sessionId }));
      await publish(mockSSEEvents.chatError(investigationId, 'Model overloaded, please try again', {
        web_session_id: sessionId, error_type: 'streaming_error'
      }));
      break;
    }
    case 'thinking_only': {
      await thinkingPhase();
      await complete();
      break;
    }
    case 'simple_text': {
      await textChunks(options.chunks || ['Hello.']);
      await complete();
      break;
    }
    default:
      throw new Error(`Unknown flow type: ${flowType}`);
  }

  return events;
}

/**
 * Assert event sequence matches expected pattern
 */
export function assertEventSequence(actualEvents, expectedTypes) {
  expect(actualEvents.length).toBeGreaterThanOrEqual(expectedTypes.length);
  
  for (let i = 0; i < expectedTypes.length; i++) {
    expect(actualEvents[i].type).toBe(expectedTypes[i]);
  }
}
