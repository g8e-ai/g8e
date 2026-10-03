// Copyright (c) 2026 Lateralus Labs, LLC.
// Licensed under the Business Source License 1.1 — see LICENSE for details.

import { useEffect, useMemo, useState } from 'react';
import { Empty } from '../../components/ui';
import { api } from '../../lib/api';
import {
  definitionEnum,
  definitionProperties,
  matchesQuery,
  parameterType,
  parseSpec,
  schemaRef,
  typeLabel,
  type ApiDoc,
  type ApiOperation,
  type SchemaNode,
} from '../../lib/openapi';
import { Paths } from '../../lib/paths';
import { errorText } from '../../state/toast';

const MAX_SCHEMA_DEPTH = 6;

/** A definition's properties, each expandable in place. Cycles stop at the repeated name. */
function SchemaTree({ doc, name, trail }: { doc: ApiDoc; name: string; trail: readonly string[] }) {
  const props = definitionProperties(doc.definitions, name);
  const values = definitionEnum(doc.definitions, name);
  if (values.length > 0) {
    return (
      <p className="api-enum">
        {values.map((v) => (
          <code key={v}>{v}</code>
        ))}
      </p>
    );
  }
  if (props.length === 0) return <p className="hint">No documented properties.</p>;
  return (
    <ul className="api-props">
      {props.map((p) => (
        <li key={p.name}>
          <div className="api-prop">
            <code className="api-prop-name">{p.name}</code>
            {p.required && <span className="api-req">required</span>}
            {p.ref ? <SchemaToggle doc={doc} label={p.type} refName={p.ref} trail={[...trail, name]} /> : <code className="api-type">{p.type}</code>}
            {p.description && <span className="api-prop-desc">{p.description}</span>}
          </div>
        </li>
      ))}
    </ul>
  );
}

function SchemaToggle({ doc, label, refName, trail }: { doc: ApiDoc; label: string; refName: string; trail: readonly string[] }) {
  const [open, setOpen] = useState(false);
  const cyclic = trail.includes(refName) || trail.length >= MAX_SCHEMA_DEPTH;
  return (
    <span className="api-toggle">
      <button type="button" className="api-type api-type-ref" aria-expanded={open} onClick={() => setOpen((o) => !o)} disabled={cyclic} title={cyclic ? 'Recursive type' : undefined}>
        {open ? '▾' : '▸'} {label}
      </button>
      {open && !cyclic && <SchemaTree doc={doc} name={refName} trail={trail} />}
    </span>
  );
}

/** Type label that expands into the definition it names, when there is one. */
function TypeCell({ doc, schema }: { doc: ApiDoc; schema: SchemaNode | undefined }) {
  const label = typeLabel(schema);
  if (!label) return null;
  const ref = schemaRef(schema) ?? (schema?.type === 'array' ? schemaRef(schema.items) : undefined);
  if (!ref) return <code className="api-type">{label}</code>;
  return <SchemaToggle doc={doc} label={label} refName={ref} trail={[]} />;
}

function OperationBody({ doc, op }: { doc: ApiDoc; op: ApiOperation }) {
  return (
    <div className="api-op-body">
      {op.description && <p className="api-desc">{op.description}</p>}
      {(op.consumes.length > 0 || op.produces.length > 0) && (
        <dl className="kv api-media">
          {op.consumes.length > 0 && (
            <>
              <dt>Consumes</dt>
              <dd className="mono">{op.consumes.join(', ')}</dd>
            </>
          )}
          {op.produces.length > 0 && (
            <>
              <dt>Produces</dt>
              <dd className="mono">{op.produces.join(', ')}</dd>
            </>
          )}
        </dl>
      )}

      {op.parameters.length > 0 && (
        <>
          <h4 className="api-sub">Parameters</h4>
          <div className="table-wrap">
            <table className="table api-table">
              <thead>
                <tr>
                  <th>Name</th>
                  <th>In</th>
                  <th>Type</th>
                  <th>Description</th>
                </tr>
              </thead>
              <tbody>
                {op.parameters.map((p) => (
                  <tr key={`${p.in}:${p.name}`}>
                    <td>
                      <code>{p.name}</code>
                      {p.required && <span className="api-req">required</span>}
                    </td>
                    <td>{p.in}</td>
                    <td>{p.in === 'body' ? <TypeCell doc={doc} schema={p.schema} /> : <code className="api-type">{parameterType(p)}</code>}</td>
                    <td>{p.description}</td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        </>
      )}

      {op.responses.length > 0 && (
        <>
          <h4 className="api-sub">Responses</h4>
          <div className="table-wrap">
            <table className="table api-table">
              <thead>
                <tr>
                  <th>Status</th>
                  <th>Body</th>
                  <th>Description</th>
                </tr>
              </thead>
              <tbody>
                {op.responses.map((r) => (
                  <tr key={r.status}>
                    <td>
                      <span className={`pill ${r.status.startsWith('2') ? 'pill-ok' : r.status.startsWith('4') || r.status.startsWith('5') ? 'pill-bad' : ''}`}>{r.status}</span>
                    </td>
                    <td>
                      <TypeCell doc={doc} schema={r.schema} />
                    </td>
                    <td>{r.description}</td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        </>
      )}
    </div>
  );
}

function OperationRow({ doc, op }: { doc: ApiDoc; op: ApiOperation }) {
  const [expanded, setExpanded] = useState(false);
  return (
    <div className="api-op">
      <button type="button" className="api-op-head" aria-expanded={expanded} onClick={() => setExpanded((e) => !e)}>
        <span className={`api-method api-method-${op.method.toLowerCase()}`}>{op.method}</span>
        <code className="api-path">{op.path}</code>
        <span className="api-summary">{op.summary}</span>
      </button>
      {expanded && <OperationBody doc={doc} op={op} />}
    </div>
  );
}

export function ApiView() {
  const [doc, setDoc] = useState<ApiDoc | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [query, setQuery] = useState('');
  const [tag, setTag] = useState<string | null>(null);

  useEffect(() => {
    const ctrl = new AbortController();
    api
      .get<unknown>(Paths.apiSpec, { signal: ctrl.signal })
      .then((raw) => setDoc(parseSpec(raw)))
      .catch((err: unknown) => {
        if (!ctrl.signal.aborted) setError(errorText(err));
      });
    return () => ctrl.abort();
  }, []);

  const groups = useMemo(() => {
    if (!doc) return [];
    return doc.groups
      .filter((g) => tag === null || g.tag === tag)
      .map((g) => ({ tag: g.tag, operations: g.operations.filter((op) => matchesQuery(op, query)) }))
      .filter((g) => g.operations.length > 0);
  }, [doc, query, tag]);

  const shown = groups.reduce((n, g) => n + g.operations.length, 0);

  return (
    <div className="page">
      <div className="page-head">
        <div>
          <h1>API</h1>
          <p>
            Reference for the Gateway&apos;s HTTP API, generated from the code annotations
            {doc ? ` — ${doc.operations.length} operations` : ''}. Routes without annotations are not listed.
          </p>
        </div>
        <a className="btn btn-sm" href={Paths.apiSpec} target="_blank" rel="noreferrer">
          Raw spec
        </a>
      </div>

      {error && (
        <Empty title="Could not load the API spec">
          <span>{error}</span>
        </Empty>
      )}
      {!error && !doc && (
        <div className="auth">
          <div className="spinner" aria-label="Loading" />
        </div>
      )}

      {doc && (
        <>
          <div className="api-filters">
            <input className="input" type="search" placeholder="Filter by path, method, or summary" aria-label="Filter operations" value={query} onChange={(e) => setQuery(e.target.value)} />
            <div className="row api-tags" role="group" aria-label="Filter by tag">
              <button type="button" className="chip" aria-pressed={tag === null} onClick={() => setTag(null)}>
                all
              </button>
              {doc.groups.map((g) => (
                <button key={g.tag} type="button" className="chip" aria-pressed={tag === g.tag} onClick={() => setTag(tag === g.tag ? null : g.tag)}>
                  {g.tag} <span className="hint">{g.operations.length}</span>
                </button>
              ))}
            </div>
          </div>

          {shown === 0 && <Empty title="No operations match">Try a different filter.</Empty>}
          {groups.map((g) => (
            <div className="card api-group" key={g.tag}>
              <div className="card-head">
                <h2>{g.tag}</h2>
                <span className="hint">{g.operations.length}</span>
              </div>
              {g.operations.map((op) => (
                <OperationRow key={op.id} doc={doc} op={op} />
              ))}
            </div>
          ))}
        </>
      )}
    </div>
  );
}
