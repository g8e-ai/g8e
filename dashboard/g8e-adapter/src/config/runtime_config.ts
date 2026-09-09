// FrontendRuntimeConfig is the audited runtime configuration a generated SPA
// consumes. It contains only the schema version, Gateway base URL, passkey RP
// identity, app name, optional docs URL, and feature flags. It rejects
// credentials, tokens, user IDs, session IDs, URL fragments, userinfo, insecure
// non-loopback HTTP, and unknown fields.

export const RUNTIME_CONFIG_SCHEMA_VERSION = '1.0.0';

export interface FrontendRuntimeConfigFeatures {
  readonly design_preview?: boolean;
  readonly eval_publication?: boolean;
  readonly downloads?: boolean;
}

export interface FrontendRuntimeConfig {
  readonly schema_version: string;
  readonly gateway_base_url: string;
  readonly passkey_rp_id: string;
  readonly passkey_rp_name: string;
  readonly app_name: string;
  readonly documentation_base_url?: string;
  readonly features?: FrontendRuntimeConfigFeatures;
}

export class RuntimeConfigError extends Error {
  constructor(public readonly fields: readonly string[]) {
    super(`invalid runtime config: ${fields.join(', ')}`);
    this.name = 'RuntimeConfigError';
  }
}

const FORBIDDEN_CONFIG_KEYS = [
  'token', 'tokens', 'secret', 'secrets', 'password', 'passwords',
  'credential', 'credentials', 'api_key', 'apikey', 'api_keys',
  'user_id', 'userid', 'user', 'session_id', 'sessionid', 'session',
  'cookie', 'cookies', 'authorization', 'auth_token', 'access_token',
  'refresh_token', 'bearer', 'private_key', 'privatekey',
] as const;

const ALLOWED_CONFIG_KEYS = new Set<string>([
  'schema_version',
  'gateway_base_url',
  'passkey_rp_id',
  'passkey_rp_name',
  'app_name',
  'documentation_base_url',
  'features',
]);

const ALLOWED_FEATURE_KEYS = new Set<string>([
  'design_preview',
  'eval_publication',
  'downloads',
]);

function isObject(value: unknown): value is Record<string, unknown> {
  return typeof value === 'object' && value !== null && !Array.isArray(value);
}

function findForbiddenKeys(obj: Record<string, unknown>, allowed: Set<string>): string[] {
  const forbidden: string[] = [];
  for (const key of Object.keys(obj)) {
    if (!allowed.has(key)) {
      forbidden.push(key);
    }
  }
  return forbidden;
}

function validateOrigin(url: string, errors: string[], field: string, allowLoopbackHttp: boolean): void {
  let parsed: URL;
  try {
    parsed = new URL(url);
  } catch {
    errors.push(`${field}: invalid URL`);
    return;
  }
  if (parsed.protocol !== 'https:' && !(allowLoopbackHttp && isLoopbackHttp(parsed))) {
    errors.push(`${field}: insecure origin (must be https or loopback http)`);
  }
  if (parsed.username || parsed.password) {
    errors.push(`${field}: userinfo forbidden`);
  }
  if (parsed.hash) {
    errors.push(`${field}: URL fragment forbidden`);
  }
  if (parsed.pathname !== '/' && parsed.pathname !== '') {
    errors.push(`${field}: path forbidden (origin only)`);
  }
  if (parsed.search) {
    errors.push(`${field}: query string forbidden`);
  }
}

function isLoopbackHttp(parsed: URL): boolean {
  if (parsed.protocol !== 'http:') return false;
  const host = parsed.hostname;
  return host === 'localhost' || host === '127.0.0.1' || host === '[::1]';
}

function validateFeatures(value: unknown, errors: string[]): FrontendRuntimeConfigFeatures | undefined {
  if (value === undefined || value === null) return undefined;
  if (!isObject(value)) {
    errors.push('features: must be an object');
    return undefined;
  }
  const unknown = findForbiddenKeys(value, ALLOWED_FEATURE_KEYS);
  for (const k of unknown) {
    if (FORBIDDEN_CONFIG_KEYS.includes(k as never)) {
      errors.push(`features.${k}: secret/credential field forbidden`);
    } else {
      errors.push(`features.${k}: unknown field`);
    }
  }
  const features: { design_preview?: boolean; eval_publication?: boolean; downloads?: boolean } = {};  if (value.design_preview !== undefined) {
    if (typeof value.design_preview !== 'boolean') {
      errors.push('features.design_preview: must be boolean');
    } else {
      features.design_preview = value.design_preview;
    }
  }
  if (value.eval_publication !== undefined) {
    if (typeof value.eval_publication !== 'boolean') {
      errors.push('features.eval_publication: must be boolean');
    } else {
      features.eval_publication = value.eval_publication;
    }
  }
  if (value.downloads !== undefined) {
    if (typeof value.downloads !== 'boolean') {
      errors.push('features.downloads: must be boolean');
    } else {
      features.downloads = value.downloads;
    }
  }
  return features;
}

export function parseRuntimeConfig(input: unknown): FrontendRuntimeConfig {
  const errors: string[] = [];
  if (!isObject(input)) {
    throw new RuntimeConfigError(['root: must be an object']);
  }

  // Unknown and forbidden top-level keys.
  for (const key of Object.keys(input)) {
    if (!ALLOWED_CONFIG_KEYS.has(key)) {
      if (FORBIDDEN_CONFIG_KEYS.includes(key as never)) {
        errors.push(`${key}: secret/credential field forbidden`);
      } else {
        errors.push(`${key}: unknown field`);
      }
    }
  }

  // schema_version
  if (input.schema_version === undefined) {
    errors.push('schema_version: required');
  } else if (typeof input.schema_version !== 'string') {
    errors.push('schema_version: must be a string');
  } else if (input.schema_version !== RUNTIME_CONFIG_SCHEMA_VERSION) {
    errors.push(`schema_version: unsupported (expected ${RUNTIME_CONFIG_SCHEMA_VERSION})`);
  }

  // gateway_base_url
  if (input.gateway_base_url === undefined) {
    errors.push('gateway_base_url: required');
  } else if (typeof input.gateway_base_url !== 'string' || input.gateway_base_url === '') {
    errors.push('gateway_base_url: must be a non-empty string');
  } else {
    validateOrigin(input.gateway_base_url, errors, 'gateway_base_url', true);
  }

  // passkey_rp_id
  if (input.passkey_rp_id === undefined) {
    errors.push('passkey_rp_id: required');
  } else if (typeof input.passkey_rp_id !== 'string' || input.passkey_rp_id === '') {
    errors.push('passkey_rp_id: must be a non-empty string');
  } else {
    // RP ID is a domain (e.g. example.com or localhost). Reject userinfo,
    // protocol, port, path, fragment, query.
    const rpId = input.passkey_rp_id as string;
    if (/[/?#:@]/.test(rpId)) {
      errors.push('passkey_rp_id: must be a bare domain (no protocol, port, path, fragment, or userinfo)');
    }
  }

  // passkey_rp_name
  if (input.passkey_rp_name === undefined) {
    errors.push('passkey_rp_name: required');
  } else if (typeof input.passkey_rp_name !== 'string' || input.passkey_rp_name === '') {
    errors.push('passkey_rp_name: must be a non-empty string');
  }

  // app_name
  if (input.app_name === undefined) {
    errors.push('app_name: required');
  } else if (typeof input.app_name !== 'string' || input.app_name === '') {
    errors.push('app_name: must be a non-empty string');
  }

  // documentation_base_url (optional)
  let documentationBaseUrl: string | undefined;
  if (input.documentation_base_url !== undefined) {
    if (typeof input.documentation_base_url !== 'string' || input.documentation_base_url === '') {
      errors.push('documentation_base_url: must be a non-empty string');
    } else {
      validateOrigin(input.documentation_base_url as string, errors, 'documentation_base_url', true);
      documentationBaseUrl = input.documentation_base_url as string;
    }
  }

  // features (optional)
  const features = validateFeatures(input.features, errors);

  if (errors.length > 0) {
    throw new RuntimeConfigError(errors);
  }

  return {
    schema_version: input.schema_version as string,
    gateway_base_url: (input.gateway_base_url as string).replace(/\/$/, ''),
    passkey_rp_id: input.passkey_rp_id as string,
    passkey_rp_name: input.passkey_rp_name as string,
    app_name: input.app_name as string,
    documentation_base_url: documentationBaseUrl,
    features,
  };
}
