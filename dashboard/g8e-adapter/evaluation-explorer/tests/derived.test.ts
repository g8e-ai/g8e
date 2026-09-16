import { describe, expect, it } from 'vitest';
import { roleLabel } from '../src/views/derived';

describe('roleLabel', () => {
  it('maps wire roles to display labels', () => {
    expect(roleLabel('primary')).toBe('Primary');
    expect(roleLabel('assistant')).toBe('Assistant');
    expect(roleLabel('lite')).toBe('Light');
  });
});
