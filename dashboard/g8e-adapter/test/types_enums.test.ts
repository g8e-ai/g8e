import { describe, expect, it } from 'vitest';

import {
  AGENT_LIFECYCLE_STATUSES,
  DOWNLOAD_PRIVACY_CLASSIFICATIONS,
  EVAL_VERIFICATION_STATUSES,
  MEASUREMENT_STATUSES,
  RUN_KINDS,
  RUN_LIFECYCLE_STATUSES,
  SNAPSHOT_FRESHNESS_VALUES,
  isAgentLifecycleStatus,
  isDownloadPrivacyClassification,
  isEvalVerificationStatus,
  isMeasurementStatus,
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
});
