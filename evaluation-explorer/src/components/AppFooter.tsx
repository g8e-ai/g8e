// Copyright (c) 2026 Lateralus Labs, LLC.
// Licensed under the Business Source License 1.1 — see LICENSE for details.

import { Link } from 'react-router-dom';
import { useStoreState } from '../state/store';
import { useMirrorOrigin } from '../state/mirror';
import { G8E_REPO_URL } from '../content/platform';
import descriptorUrl from '../contract/descriptor.json?url';

/** Single-row footer: mirror transport endpoints, schema, and site links. */
export function AppFooter() {
  const origin = useMirrorOrigin();
  const proofsPublished = useStoreState((state) => state.proofArtifactCount > 0);
  const external = { target: '_blank', rel: 'noopener noreferrer' } as const;

  return (
    <footer className="app-footer" aria-label="Public mirror endpoints and site links">
      <span className="app-footer-label" title="Anonymous, read-only. Mirror availability is not verification evidence.">
        Public mirror · read-only
      </span>
      <nav className="app-footer-links" aria-label="Mirror transport">
        {origin ? (
          <>
            <a href={`${origin}/history?cursor=0&limit=500`} {...external}>History <code>JSON</code></a>
            <a href={`${origin}/bootstrap`} {...external}>Bootstrap <code>JSON</code></a>
            {proofsPublished ? (
              <a href={`${origin}/proof-manifest`} {...external}>Proof manifest <code>JSON</code></a>
            ) : null}
            <a href={`${origin}/stream`} {...external}>Live updates <code>SSE</code></a>
          </>
        ) : (
          <span className="app-footer-unavailable">Mirror origin unavailable</span>
        )}
        <a href={descriptorUrl} {...external}>Schema <code>v1.5</code></a>
      </nav>
      <nav className="app-footer-links app-footer-site" aria-label="Site">
        <Link to="/methodology">Methodology</Link>
        <Link to="/tasks">Tasks</Link>
        <Link to="/about">About</Link>
        <a href={G8E_REPO_URL} {...external}>Source</a>
      </nav>
    </footer>
  );
}
