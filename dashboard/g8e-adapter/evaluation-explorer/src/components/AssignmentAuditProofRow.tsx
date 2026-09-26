// Copyright (c) 2026 Lateralus Labs, LLC.
// Licensed under the Business Source License 1.1 — see LICENSE for details.

import { useEffect, useMemo, useState } from 'react';
import type { PublicEvidenceBinding } from '../contract/types';
import { loadRuntimeConfig } from '../state/feed';
import { useStoreState } from '../state/store';
import { formatNumber } from './shared';

function bindingForKind(bindings: PublicEvidenceBinding[] | undefined, kind: PublicEvidenceBinding['kind']) {
  return bindings?.find((binding) => binding.kind === kind);
}

function formatBytes(value: number): string {
  if (value >= 1024 ** 3) return `${formatNumber(value / 1024 ** 3, 1)} GiB`;
  if (value >= 1024 ** 2) return `${formatNumber(value / 1024 ** 2, 1)} MiB`;
  if (value >= 1024) return `${formatNumber(value / 1024, 1)} KiB`;
  return `${formatNumber(value)} B`;
}

export function AssignmentAuditProofRow({ bindings }: { bindings: PublicEvidenceBinding[] | undefined }) {
  const [origin, setOrigin] = useState<string | undefined>();
  const sliceBinding = bindingForKind(bindings, 'assignment_audit_slice');
  const keyBinding = bindingForKind(bindings, 'assignment_audit_vault_key');
  const sliceArtifact = useStoreState((state) =>
    sliceBinding ? state.proofArtifacts.get(sliceBinding.sha256) : undefined,
  );
  const keyArtifact = useStoreState((state) =>
    keyBinding ? state.proofArtifacts.get(keyBinding.sha256) : undefined,
  );

  useEffect(() => {
    let cancelled = false;
    loadRuntimeConfig()
      .then((config) => {
        if (!cancelled) setOrigin(config.mirror_origin);
      })
      .catch(() => {
        if (!cancelled) setOrigin(undefined);
      });
    return () => {
      cancelled = true;
    };
  }, []);

  const verifyCommand = useMemo(() => {
    if (!sliceBinding || !keyBinding) return undefined;
    return 'g8e public verify-assignment --db assignment.db --vault-key assignment.vault.key';
  }, [sliceBinding, keyBinding]);

  if (!sliceBinding || !keyBinding || !sliceArtifact || !keyArtifact) return null;

  const dbUrl = origin ? `${origin}${sliceArtifact.immutable_url}` : undefined;
  const keyUrl = origin ? `${origin}${keyArtifact.immutable_url}` : undefined;

  return (
    <section className="assignment-audit-proof" aria-label="Audit slice">
      <div className="assignment-audit-proof-row">
        <span className="assignment-audit-proof-label">Audit slice</span>
        <span className="assignment-audit-proof-meta">
          <code>{sliceBinding.sha256.slice(0, 12)}…</code>
          <span>{formatBytes(sliceArtifact.byte_size)}</span>
        </span>
        <span className="assignment-audit-proof-actions">
          {dbUrl ? (
            <a href={dbUrl} download={sliceArtifact.filename}>
              Download SQLite
            </a>
          ) : (
            <span>Download SQLite</span>
          )}
          {keyUrl ? (
            <a href={keyUrl} download={keyArtifact.filename}>
              Download vault key
            </a>
          ) : (
            <span>Download vault key</span>
          )}
          {verifyCommand ? (
            <button
              type="button"
              className="copy-verify-command"
              onClick={() => {
                if (verifyCommand && navigator.clipboard?.writeText) {
                  void navigator.clipboard.writeText(verifyCommand);
                }
              }}
            >
              Copy verify command
            </button>
          ) : null}
        </span>
      </div>
      <p className="assignment-audit-proof-note">
        Files verify offline with <code>{verifyCommand}</code>. Browser download checks content addressing only; commitment-chain verification requires the CLI.
      </p>
    </section>
  );
}

export function filterNonAuditEvidenceBindings(bindings: PublicEvidenceBinding[] | undefined): PublicEvidenceBinding[] | undefined {
  if (!bindings?.length) return bindings;
  const filtered = bindings.filter(
    (binding) => binding.kind !== 'assignment_audit_slice' && binding.kind !== 'assignment_audit_vault_key',
  );
  return filtered.length > 0 ? filtered : undefined;
}
