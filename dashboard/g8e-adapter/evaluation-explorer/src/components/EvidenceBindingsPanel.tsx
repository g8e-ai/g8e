// Copyright (c) 2026 Lateralus Labs, LLC.
// Licensed under the Business Source License 1.1 — see LICENSE for details.

import { useState } from 'react';
import type { PublicEvidenceBinding, PublicVerificationMetadata } from '../contract/types';
import { DetailRow, UnavailableValue } from './shared';

function label(value: string): string {
  return value.replace(/_/g, ' ').replace(/^./, (letter) => letter.toUpperCase());
}

function CopyButton({ text }: { text: string }) {
  const [copied, setCopied] = useState(false);

  const handleCopy = async () => {
    try {
      await navigator.clipboard.writeText(text);
      setCopied(true);
      setTimeout(() => setCopied(false), 2000);
    } catch (err) {
      console.error('Failed to copy:', err);
    }
  };

  return (
    <button
      onClick={handleCopy}
      className="copy-button copy-hash"
      title="Copy hash"
      aria-label="Copy hash"
    >
      {copied ? '✓' : '📋'}
    </button>
  );
}

export function EvidenceBindingsPanel({ bindings, verification }: { bindings: PublicEvidenceBinding[] | undefined; verification: PublicVerificationMetadata | undefined }) {
  return (
    <section className="assignment-evidence">
      <h2>Evidence and methodology</h2>
      <dl>
        <DetailRow label="Verification scope">{verification ? `${label(verification.provenance)} · ${label(verification.verifier_state)}` : <UnavailableValue reason="verification scope not published" />}</DetailRow>
        {verification?.verifier_release_version ? <DetailRow label="Verifier release">{verification.verifier_release_version}</DetailRow> : null}
        {verification?.verifier_contract_version ? <DetailRow label="Verifier contract">{verification.verifier_contract_version}</DetailRow> : null}
        {verification?.report_digest ? <DetailRow label="Report digest"><code>{verification.report_digest}</code></DetailRow> : null}
        {verification?.population_digest ? <DetailRow label="Population digest"><code>{verification.population_digest}</code></DetailRow> : null}
      </dl>
      <h3>Public content bindings</h3>
      {!bindings || bindings.length === 0 ? <p className="panel-note">No approved public proof bindings were published for this assignment.</p> : <ul className="evidence-binding-list">
        {bindings.map((binding) => <li key={`${binding.kind}:${binding.sha256}`}><span>{label(binding.kind)}</span><code>{binding.sha256}</code><CopyButton text={binding.sha256} /><small>{binding.schema_ref}</small></li>)}
      </ul>}
      <p className="panel-note">Bindings identify approved public evidence content. A hash does not make private artifacts accessible or independently verify application telemetry.</p>
    </section>
  );
}
