// Copyright (c) 2026 Lateralus Labs, LLC.
// Licensed under the Business Source License 1.1 — see LICENSE for details.

import { describe, expect, it } from 'vitest';

import {
  AGENT_LIFECYCLE_STATUSES,
  DOWNLOAD_PRIVACY_CLASSIFICATIONS,
  EVAL_VERIFICATION_STATUSES,
  MEASUREMENT_STATUSES,
  PUBLIC_FEED_INGEST_REJECTION_REASONS,
  PUBLIC_FEED_OUTBOX_STATUSES,
  PUBLIC_FEED_PROOF_CLASSIFICATIONS,
  PUBLIC_FEED_RECORD_TYPES,
  RUN_KINDS,
  RUN_LIFECYCLE_STATUSES,
  SNAPSHOT_FRESHNESS_VALUES,
  isAgentLifecycleStatus,
  isDownloadPrivacyClassification,
  isEvalVerificationStatus,
  isMeasurementStatus,
  isPublicFeedIngestRejectionReason,
  isPublicFeedOutboxStatus,
  isPublicFeedProofClassification,
  isPublicFeedRecordType,
  isRunKind,
  isRunLifecycleStatus,
  isSnapshotFreshness,
} from '../src/types/enums';

describe('enums', () => {
  describe('AgentLifecycleStatus', () => {
    it('contains the seven Go enum values in order', () => {
      expect([...AGENT_LIFECYCLE_STATUSES]).toEqual([
        'idle', 'queued', 'running', 'waiting', 'completed', 'failed', 'offline',
      ]);
    });

    it('recognizes every member and rejects unknown strings', () => {
      for (const s of AGENT_LIFECYCLE_STATUSES) {
        expect(isAgentLifecycleStatus(s)).toBe(true);
      }
      expect(isAgentLifecycleStatus('pending')).toBe(false);
      expect(isAgentLifecycleStatus('')).toBe(false);
      expect(isAgentLifecycleStatus(42)).toBe(false);
      expect(isAgentLifecycleStatus(null)).toBe(false);
    });
  });

  describe('RunLifecycleStatus', () => {
    it('contains the six Go enum values in order', () => {
      expect([...RUN_LIFECYCLE_STATUSES]).toEqual([
        'queued', 'running', 'waiting', 'completed', 'failed', 'cancelled',
      ]);
    });

    it('recognizes every member and rejects unknown strings', () => {
      for (const s of RUN_LIFECYCLE_STATUSES) {
        expect(isRunLifecycleStatus(s)).toBe(true);
      }
      expect(isRunLifecycleStatus('idle')).toBe(false);
      expect(isRunLifecycleStatus('done')).toBe(false);
    });
  });

  describe('RunKind', () => {
    it('contains the four Go enum values in order', () => {
      expect([...RUN_KINDS]).toEqual(['investigation', 'eval', 'demo', 'workflow']);
    });

    it('recognizes every member and rejects unknown strings', () => {
      for (const k of RUN_KINDS) {
        expect(isRunKind(k)).toBe(true);
      }
      expect(isRunKind('chat')).toBe(false);
      expect(isRunKind('EVAL')).toBe(false);
    });
  });

  describe('EvalVerificationStatus', () => {
    it('contains the three Go enum values in order', () => {
      expect([...EVAL_VERIFICATION_STATUSES]).toEqual([
        'projection_validated', 'receipt_verification_not_applicable', 'verified',
      ]);
    });

    it('recognizes every member and rejects unknown strings', () => {
      for (const s of EVAL_VERIFICATION_STATUSES) {
        expect(isEvalVerificationStatus(s)).toBe(true);
      }
      expect(isEvalVerificationStatus('unverified')).toBe(false);
      expect(isEvalVerificationStatus('partial')).toBe(false);
    });
  });

  describe('MeasurementStatus', () => {
    it('contains the four Go enum values in order', () => {
      expect([...MEASUREMENT_STATUSES]).toEqual([
        'observed', 'stale', 'unavailable', 'unsupported',
      ]);
    });

    it('recognizes every member and rejects unknown strings', () => {
      for (const s of MEASUREMENT_STATUSES) {
        expect(isMeasurementStatus(s)).toBe(true);
      }
      expect(isMeasurementStatus('fresh')).toBe(false);
    });
  });

  describe('SnapshotFreshness', () => {
    it('contains the three Go enum values in order', () => {
      expect([...SNAPSHOT_FRESHNESS_VALUES]).toEqual([
        'observed', 'stale', 'unavailable',
      ]);
    });

    it('recognizes every member and rejects unknown strings', () => {
      for (const s of SNAPSHOT_FRESHNESS_VALUES) {
        expect(isSnapshotFreshness(s)).toBe(true);
      }
      expect(isSnapshotFreshness('unsupported')).toBe(false);
    });
  });

  describe('DownloadPrivacyClassification', () => {
    it('contains the two Go enum values in order', () => {
      expect([...DOWNLOAD_PRIVACY_CLASSIFICATIONS]).toEqual([
        'public_safe', 'restricted',
      ]);
    });

    it('recognizes every member and rejects unknown strings', () => {
      for (const c of DOWNLOAD_PRIVACY_CLASSIFICATIONS) {
        expect(isDownloadPrivacyClassification(c)).toBe(true);
      }
      expect(isDownloadPrivacyClassification('public')).toBe(false);
      expect(isDownloadPrivacyClassification('private')).toBe(false);
    });
  });

  describe('PublicFeedRecordType', () => {
    it('contains the four Go enum values in order', () => {
      expect([...PUBLIC_FEED_RECORD_TYPES]).toEqual([
        'projection', 'event', 'proof_manifest', 'key_revocation',
      ]);
    });

    it('recognizes every member and rejects unknown strings', () => {
      for (const t of PUBLIC_FEED_RECORD_TYPES) {
        expect(isPublicFeedRecordType(t)).toBe(true);
      }
      expect(isPublicFeedRecordType('heartbeat')).toBe(false);
      expect(isPublicFeedRecordType('')).toBe(false);
    });
  });

  describe('PublicFeedOutboxStatus', () => {
    it('contains the four Go enum values in order', () => {
      expect([...PUBLIC_FEED_OUTBOX_STATUSES]).toEqual([
        'pending', 'sent', 'acknowledged', 'failed',
      ]);
    });

    it('recognizes every member and rejects unknown strings', () => {
      for (const s of PUBLIC_FEED_OUTBOX_STATUSES) {
        expect(isPublicFeedOutboxStatus(s)).toBe(true);
      }
      expect(isPublicFeedOutboxStatus('delivered')).toBe(false);
    });
  });

  describe('PublicFeedIngestRejectionReason', () => {
    it('contains the seven Go enum values in order', () => {
      expect([...PUBLIC_FEED_INGEST_REJECTION_REASONS]).toEqual([
        'signature_invalid', 'sequence_out_of_order', 'hash_chain_mismatch',
        'duplicate_sequence', 'oversized_batch', 'revoked_key', 'unknown_key',
      ]);
    });

    it('recognizes every member and rejects unknown strings', () => {
      for (const r of PUBLIC_FEED_INGEST_REJECTION_REASONS) {
        expect(isPublicFeedIngestRejectionReason(r)).toBe(true);
      }
      expect(isPublicFeedIngestRejectionReason('forbidden')).toBe(false);
    });
  });

  describe('PublicFeedProofClassification', () => {
    it('contains the single Go enum value', () => {
      expect([...PUBLIC_FEED_PROOF_CLASSIFICATIONS]).toEqual(['public_safe']);
    });

    it('recognizes the member and rejects unknown strings', () => {
      for (const c of PUBLIC_FEED_PROOF_CLASSIFICATIONS) {
        expect(isPublicFeedProofClassification(c)).toBe(true);
      }
      expect(isPublicFeedProofClassification('restricted')).toBe(false);
      expect(isPublicFeedProofClassification('public')).toBe(false);
    });
  });
});
