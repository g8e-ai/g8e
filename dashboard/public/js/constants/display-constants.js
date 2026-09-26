// Copyright (c) 2026 Lateralus Labs, LLC.
// Licensed under the Business Source License 1.1 — see LICENSE for details.

/**
 * Dashboard UI display constants sourced from protocol/constants/status.json.
 */

export const CitationLayout = Object.freeze({
    FAVICON_SIZE_PX: 16,
    HOVER_CARD_WIDTH_PX: 360,
    HOVER_CARD_VIEWPORT_MARGIN_PX: 8,
    HEADING_SEGMENT_MAX_LENGTH: 60,
    SENTENCE_LOOKAHEAD_CHARS: 120,
    PARAGRAPH_LOOKAHEAD_CHARS: 200,
});

export const TribunalOutcome = Object.freeze({
    CONSENSUS: 'consensus',
    VERIFIED: 'verified',
    VERIFICATION_FAILED: 'verification_failed',
    CONSENSUS_FAILED: 'consensus_failed',
});

export const TribunalFallbackReason = Object.freeze({
    DISABLED: 'disabled',
    PROVIDER_UNAVAILABLE: 'provider_unavailable',
    ALL_PASSES_FAILED: 'all_passes_failed',
    NO_VOTE_WINNER: 'no_vote_winner',
});

export const ToolDisplayCategory = Object.freeze({
    EXECUTION: 'execution',
    FILE: 'file',
    GENERAL: 'general',
    NETWORK: 'network',
    SEARCH: 'search',
});
