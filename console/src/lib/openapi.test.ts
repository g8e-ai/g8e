// Copyright (c) 2026 Lateralus Labs, LLC.
// Licensed under the Business Source License 1.1 — see LICENSE for details.

import { describe, expect, it } from 'vitest';
import { definitionEnum, definitionProperties, matchesQuery, parameterType, parseSpec, refName, schemaRef, typeLabel } from './openapi';

const SPEC = {
  swagger: '2.0',
  info: { title: 'g8e Gateway' },
  paths: {
    '/api/v1/approvals/{txHash}/verify': {
      post: {
        summary: 'Verify approval',
        description: 'Verifies a WebAuthn assertion.',
        tags: ['approvals'],
        consumes: ['application/json'],
        parameters: [
          { name: 'txHash', in: 'path', required: true, type: 'string', description: 'Transaction hash' },
          { name: 'body', in: 'body', required: true, schema: { $ref: '#/definitions/models.Assertion' } },
        ],
        responses: {
          '404': { description: 'Not found', schema: { type: 'string' } },
          '200': { description: 'OK', schema: { $ref: '#/definitions/models.Receipt' } },
        },
      },
    },
    '/api/v1/approvals': {
      get: { summary: 'List approvals', tags: ['approvals'], responses: { '200': { description: 'OK' } } },
      post: { summary: 'Create', tags: ['approvals'], responses: {} },
    },
    '/health': { get: { summary: 'Health', responses: { '200': { description: 'OK' } } } },
    '/skipped': { parameters: [], get: 'not an operation' },
  },
  definitions: {
    'models.Assertion': {
      type: 'object',
      required: ['id'],
      properties: {
        id: { type: 'string', description: 'Credential ID' },
        kind: { allOf: [{ $ref: '#/definitions/constants.Kind' }], description: 'Kind' },
        tags: { type: 'array', items: { type: 'string' } },
        children: { type: 'array', items: { $ref: '#/definitions/models.Receipt' } },
      },
    },
    'models.Receipt': { type: 'object', properties: { at: { type: 'integer', format: 'int64' } } },
    'constants.Kind': { type: 'string', enum: ['A', 'B'] },
  },
};

describe('parseSpec', () => {
  const doc = parseSpec(SPEC);

  it('flattens operations and drops non-operations', () => {
    expect(doc.title).toBe('g8e Gateway');
    expect(doc.operations.map((o) => o.id).sort()).toEqual(['GET /api/v1/approvals', 'GET /health', 'POST /api/v1/approvals', 'POST /api/v1/approvals/{txHash}/verify']);
  });

  it('groups by first tag, sorted, with untagged operations under "other"', () => {
    expect(doc.groups.map((g) => g.tag)).toEqual(['approvals', 'other']);
    expect(doc.groups[0]?.operations.map((o) => o.id)).toEqual(['GET /api/v1/approvals', 'POST /api/v1/approvals', 'POST /api/v1/approvals/{txHash}/verify']);
  });

  it('orders responses by numeric status and keeps parameters typed', () => {
    const op = doc.operations.find((o) => o.path.endsWith('/verify'));
    expect(op?.responses.map((r) => r.status)).toEqual(['200', '404']);
    expect(op?.parameters.map((p) => [p.name, p.in, p.required])).toEqual([
      ['txHash', 'path', true],
      ['body', 'body', true],
    ]);
  });

  it('rejects payloads that are not an OpenAPI document', () => {
    expect(() => parseSpec(null)).toThrow(/OpenAPI/);
    expect(() => parseSpec({ error: 'unauthorized' })).toThrow(/OpenAPI/);
    expect(() => parseSpec('<html>')).toThrow(/OpenAPI/);
  });
});

describe('schema helpers', () => {
  it('resolves definition refs, including single-element allOf wrappers', () => {
    expect(refName('#/definitions/models.Foo')).toBe('models.Foo');
    expect(refName('#/components/schemas/Foo')).toBeUndefined();
    expect(schemaRef({ allOf: [{ $ref: '#/definitions/models.Foo' }] })).toBe('models.Foo');
    expect(schemaRef({ type: 'string' })).toBeUndefined();
  });

  it('labels types', () => {
    expect(typeLabel({ type: 'string' })).toBe('string');
    expect(typeLabel({ type: 'integer', format: 'int64' })).toBe('integer (int64)');
    expect(typeLabel({ $ref: '#/definitions/models.Foo' })).toBe('models.Foo');
    expect(typeLabel({ type: 'array', items: { $ref: '#/definitions/models.Foo' } })).toBe('models.Foo[]');
    expect(typeLabel({ type: 'object', additionalProperties: { type: 'integer' } })).toBe('map<string, integer>');
    expect(typeLabel({ type: 'object' })).toBe('object');
    expect(typeLabel(undefined)).toBe('');
  });

  it('labels inline parameter types', () => {
    expect(parameterType({ name: 'limit', in: 'query', required: false, type: 'integer', format: 'int32' })).toBe('integer (int32)');
    expect(parameterType({ name: 'ids', in: 'query', required: false, type: 'array', items: { type: 'string' } })).toBe('string[]');
  });

  it('lists definition properties, marking required and expandable ones', () => {
    const { definitions } = parseSpec(SPEC);
    const props = definitionProperties(definitions, 'models.Assertion');
    expect(props.map((p) => p.name)).toEqual(['children', 'id', 'kind', 'tags']);
    expect(props.find((p) => p.name === 'id')).toMatchObject({ required: true, type: 'string', description: 'Credential ID' });
    expect(props.find((p) => p.name === 'kind')).toMatchObject({ ref: 'constants.Kind', required: false });
    expect(props.find((p) => p.name === 'children')).toMatchObject({ ref: 'models.Receipt', type: 'models.Receipt[]' });
    expect(props.find((p) => p.name === 'tags')?.ref).toBeUndefined();
    expect(definitionProperties(definitions, 'missing')).toEqual([]);
    expect(definitionEnum(definitions, 'constants.Kind')).toEqual(['A', 'B']);
  });
});

describe('matchesQuery', () => {
  const op = parseSpec(SPEC).operations.find((o) => o.path.endsWith('/verify'))!;
  it('matches method, path, tag, summary, and description case-insensitively', () => {
    for (const q of ['post', 'VERIFY', 'approvals', 'verify approval', 'webauthn', '  ']) expect(matchesQuery(op, q)).toBe(true);
    expect(matchesQuery(op, 'passkey')).toBe(false);
  });
});
