// Canonical Primary / Assistant / Lite role names and scope copy.
// Wire values remain primary, assistant, lite per the public view contract.

import type { ModelRole } from '../contract/types';

export const MODEL_ROLES = [
  { name: 'Primary', wire: 'primary', scope: 'Task owner · plans · delegates · synthesizes' },
  { name: 'Assistant', wire: 'assistant', scope: 'Bounded technical work for Primary' },
  { name: 'Lite', wire: 'lite', scope: 'Constrained decisions or escalate' },
] as const satisfies ReadonlyArray<{ name: string; wire: ModelRole; scope: string }>;

export const MODEL_ROLE_WIRE_ORDER: ModelRole[] = MODEL_ROLES.map((role) => role.wire);

const scopeByWire = Object.fromEntries(MODEL_ROLES.map((role) => [role.wire, role.scope])) as Record<
  ModelRole,
  string
>;

export function roleScopeFor(role: ModelRole): string {
  return scopeByWire[role];
}
