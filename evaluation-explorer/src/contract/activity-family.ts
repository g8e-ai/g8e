// Copyright (c) 2026 Lateralus Labs, LLC.
// Licensed under the Business Source License 1.1 — see LICENSE for details.

// Typed activity-family wire parsing and view normalization. Campaign wire
// payloads use protobuf enum names; the explorer view contract uses short
// discriminated-union values.

import type { ActivityAvailability, PublicActivityFamily, PublicUnavailableReason } from './types';
import { ValidationError } from './validators';

export const WIRE_ACTIVITY_AVAILABILITIES = [
  'PUBLIC_ACTIVITY_AVAILABILITY_OBSERVED',
  'PUBLIC_ACTIVITY_AVAILABILITY_UNAVAILABLE',
  'PUBLIC_ACTIVITY_AVAILABILITY_NOT_APPLICABLE',
] as const;
export type WireActivityAvailability = (typeof WIRE_ACTIVITY_AVAILABILITIES)[number];

export const WIRE_UNAVAILABLE_REASONS = [
  'PUBLIC_UNAVAILABLE_REASON_HISTORICAL_NOT_CAPTURED',
  'PUBLIC_UNAVAILABLE_REASON_SOURCE_NOT_CAPTURED',
  'PUBLIC_UNAVAILABLE_REASON_SOURCE_UNAVAILABLE',
  'PUBLIC_UNAVAILABLE_REASON_SCENARIO_NOT_APPLICABLE',
  'PUBLIC_UNAVAILABLE_REASON_INCOMPLETE_CONTRIBUTOR_EVIDENCE',
  'PUBLIC_UNAVAILABLE_REASON_NO_SCORED_CALLS',
] as const;
export type WireUnavailableReason = (typeof WIRE_UNAVAILABLE_REASONS)[number];

const WIRE_AVAILABILITY_TO_VIEW: Record<WireActivityAvailability, ActivityAvailability> = {
  PUBLIC_ACTIVITY_AVAILABILITY_OBSERVED: 'observed',
  PUBLIC_ACTIVITY_AVAILABILITY_UNAVAILABLE: 'unavailable',
  PUBLIC_ACTIVITY_AVAILABILITY_NOT_APPLICABLE: 'not_applicable',
};

const WIRE_UNAVAILABLE_REASON_TO_VIEW: Record<WireUnavailableReason, PublicUnavailableReason> = {
  PUBLIC_UNAVAILABLE_REASON_HISTORICAL_NOT_CAPTURED: 'historical_not_captured',
  PUBLIC_UNAVAILABLE_REASON_SOURCE_NOT_CAPTURED: 'source_not_captured',
  PUBLIC_UNAVAILABLE_REASON_SOURCE_UNAVAILABLE: 'source_unavailable',
  PUBLIC_UNAVAILABLE_REASON_SCENARIO_NOT_APPLICABLE: 'scenario_not_applicable',
  PUBLIC_UNAVAILABLE_REASON_INCOMPLETE_CONTRIBUTOR_EVIDENCE: 'incomplete_contributor_evidence',
  PUBLIC_UNAVAILABLE_REASON_NO_SCORED_CALLS: 'no_scored_calls',
};

export interface WireActivityFamily<T> {
  availability: WireActivityAvailability;
  unavailable_reason?: WireUnavailableReason;
  records?: T[];
}

function isObject(value: unknown): value is Record<string, unknown> {
  return typeof value === 'object' && value !== null && !Array.isArray(value);
}

function assertObject(value: unknown, path: string): Record<string, unknown> {
  if (!isObject(value)) throw new ValidationError(path, 'expected object');
  return value;
}

function rejectUnknown(value: Record<string, unknown>, allowed: readonly string[], path: string): void {
  for (const key of Object.keys(value)) {
    if (!allowed.includes(key)) throw new ValidationError(`${path}.${key}`, 'unknown field');
  }
}

function assertEnum<T extends string>(value: unknown, allowed: readonly T[], path: string): T {
  if (typeof value !== 'string' || !allowed.includes(value as T)) {
    throw new ValidationError(path, `expected one of ${allowed.join(', ')}`);
  }
  return value as T;
}

export function parseWireActivityAvailability(value: unknown, path: string): ActivityAvailability {
  const wire = assertEnum(value, WIRE_ACTIVITY_AVAILABILITIES, path);
  return WIRE_AVAILABILITY_TO_VIEW[wire];
}

export function parseWireUnavailableReason(value: unknown, path: string): PublicUnavailableReason {
  const wire = assertEnum(value, WIRE_UNAVAILABLE_REASONS, path);
  return WIRE_UNAVAILABLE_REASON_TO_VIEW[wire];
}

export function parseWireActivityFamily<T>(
  value: unknown,
  path: string,
  parseRecord: (value: unknown, recordPath: string) => T,
): WireActivityFamily<T> {
  const family = assertObject(value, path);
  rejectUnknown(family, ['availability', 'unavailable_reason', 'records'], path);
  const availability = assertEnum(family.availability, WIRE_ACTIVITY_AVAILABILITIES, `${path}.availability`);
  if (availability === 'PUBLIC_ACTIVITY_AVAILABILITY_OBSERVED') {
    if (family.unavailable_reason !== undefined) {
      throw new ValidationError(`${path}.unavailable_reason`, 'observed activity cannot have unavailable_reason');
    }
  } else {
    assertEnum(family.unavailable_reason, WIRE_UNAVAILABLE_REASONS, `${path}.unavailable_reason`);
    if (availability === 'PUBLIC_ACTIVITY_AVAILABILITY_NOT_APPLICABLE' && family.unavailable_reason !== 'PUBLIC_UNAVAILABLE_REASON_SCENARIO_NOT_APPLICABLE') {
      throw new ValidationError(`${path}.unavailable_reason`, 'not_applicable activity requires scenario_not_applicable');
    }
  }
  const rawRecords = family.records === undefined ? [] : family.records;
  if (!Array.isArray(rawRecords)) throw new ValidationError(`${path}.records`, 'expected array');
  if (rawRecords.length > 128) throw new ValidationError(`${path}.records`, 'expected at most 128 entries');
  const records = rawRecords.map((record, index) => parseRecord(record, `${path}.records[${index}]`));
  if (availability !== 'PUBLIC_ACTIVITY_AVAILABILITY_OBSERVED' && records.length > 0) {
    throw new ValidationError(`${path}.records`, 'unavailable activity cannot contain records');
  }
  return { availability, unavailable_reason: family.unavailable_reason as WireUnavailableReason | undefined, records: records };
}

export function normalizeActivityFamily<T>(family: WireActivityFamily<T>): PublicActivityFamily<T> {
  const availability = WIRE_AVAILABILITY_TO_VIEW[family.availability];
  if (availability === 'observed') {
    return { availability: 'observed', records: family.records ?? [] };
  }
  if (availability === 'not_applicable') {
    return { availability: 'not_applicable' };
  }
  if (family.unavailable_reason === undefined) {
    throw new ValidationError('unavailable activity requires unavailable_reason', 'activity_family.unavailable_reason');
  }
  return {
    availability: 'unavailable',
    unavailable_reason: WIRE_UNAVAILABLE_REASON_TO_VIEW[family.unavailable_reason],
  };
}

export function activityFamilyHasObservedRecords<T>(family: PublicActivityFamily<T> | undefined): boolean {
  return family?.availability === 'observed' && family.records.length > 0;
}

export function activityFamilySummaryText<T>(family: PublicActivityFamily<T>): string {
  if (family.availability === 'observed') return `${family.records.length} observed`;
  if (family.availability === 'not_applicable') return 'Not applicable to this scenario';
  return `Unavailable: ${family.unavailable_reason.replace(/_/g, ' ')}`;
}
