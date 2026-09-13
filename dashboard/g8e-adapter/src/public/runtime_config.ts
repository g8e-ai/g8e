export const PUBLIC_RUNTIME_CONFIG_SCHEMA_VERSION = '1.0.0';

export interface PublicRuntimeConfig {
  readonly schema_version: string;
  readonly mirror_origin: string;
}

export class PublicRuntimeConfigError extends Error {
  constructor(public readonly fields: readonly string[]) {
    super(`invalid public runtime config: ${fields.join(', ')}`);
    this.name = 'PublicRuntimeConfigError';
  }
}

function isObject(value: unknown): value is Record<string, unknown> {
  return typeof value === 'object' && value !== null && !Array.isArray(value);
}

function isLoopback(parsed: URL): boolean {
  return parsed.hostname === 'localhost' || parsed.hostname === '127.0.0.1' || parsed.hostname === '[::1]';
}

export function parsePublicRuntimeConfig(input: unknown): PublicRuntimeConfig {
  if (!isObject(input)) throw new PublicRuntimeConfigError(['root: must be an object']);
  const errors: string[] = [];
  for (const key of Object.keys(input)) {
    if (key !== 'schema_version' && key !== 'mirror_origin') errors.push(`${key}: forbidden field`);
  }
  if (input.schema_version !== PUBLIC_RUNTIME_CONFIG_SCHEMA_VERSION) {
    errors.push(`schema_version: expected ${PUBLIC_RUNTIME_CONFIG_SCHEMA_VERSION}`);
  }
  let origin = '';
  if (typeof input.mirror_origin !== 'string' || input.mirror_origin === '') {
    errors.push('mirror_origin: required');
  } else {
    try {
      const parsed = new URL(input.mirror_origin);
      if (parsed.protocol !== 'https:' && !(parsed.protocol === 'http:' && isLoopback(parsed))) {
        errors.push('mirror_origin: HTTPS or loopback HTTP required');
      }
      if (parsed.username || parsed.password) errors.push('mirror_origin: userinfo forbidden');
      if (parsed.pathname !== '/' && parsed.pathname !== '') errors.push('mirror_origin: path forbidden');
      if (parsed.search) errors.push('mirror_origin: query forbidden');
      if (parsed.hash) errors.push('mirror_origin: fragment forbidden');
      origin = parsed.origin;
    } catch {
      errors.push('mirror_origin: invalid URL');
    }
  }
  if (errors.length > 0) throw new PublicRuntimeConfigError(errors);
  return { schema_version: PUBLIC_RUNTIME_CONFIG_SCHEMA_VERSION, mirror_origin: origin };
}
