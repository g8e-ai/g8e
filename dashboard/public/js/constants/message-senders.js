// Copyright (c) 2026 Lateralus Labs, LLC.
// Licensed under the Business Source License 1.1 — see LICENSE for details.

/**
 * Message sender identifiers for conversation history persistence.
 * Canonical values: protocol/constants/senders.json
 */
export const MessageSender = Object.freeze({
    USER_CHAT: 'g8e.v1.source.user.chat',
    USER_TERMINAL: 'g8e.v1.source.user.terminal',
    AI_PRIMARY: 'g8e.v1.source.ai.primary',
    AI_ASSISTANT: 'g8e.v1.source.ai.assistant',
    AI_TRIAGE: 'g8e.v1.source.ai.triage',
    SYSTEM: 'g8e.v1.source.system',
});
