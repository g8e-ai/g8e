// Copyright (c) 2026 Lateralus Labs, LLC.
// Licensed under the Business Source License 1.1 — see LICENSE for details.

// URL-fragment intents. The Gateway and CLI deep-link into the console with
// fragments so tokens never reach server access logs:
//   #enroll=1&token=<t>          CLI passkey enrollment (secret, one-time)
//   #recovery=<t>                CLI recovery approval (secret)
//   #approve=<tx_hash>           L3 approval (Gateway /api/v1/approve redirect)
//   #platform-enrollment=<id>    workload enrollment review (non-secret)
// Secret-bearing fragments are cleared from the address bar immediately.

export interface FragmentIntent {
  enrollmentToken?: string;
  recoveryToken?: string;
  approveTxHash?: string;
  platformEnrollmentId?: string;
}

export function parseFragment(hash: string): FragmentIntent {
  const params = new URLSearchParams(hash.replace(/^#/, ''));
  const intent: FragmentIntent = {};
  const token = params.get('token');
  if (params.get('enroll') === '1' && token) intent.enrollmentToken = token;
  const recovery = params.get('recovery');
  if (recovery) intent.recoveryToken = recovery;
  const approve = params.get('approve');
  if (approve) intent.approveTxHash = approve;
  const platform = params.get('platform-enrollment');
  if (platform) intent.platformEnrollmentId = platform;
  return intent;
}

/** Reads and clears the fragment so tokens do not linger in history. */
export function consumeFragment(): FragmentIntent {
  const intent = parseFragment(window.location.hash);
  if (window.location.hash) {
    history.replaceState(null, '', window.location.pathname + window.location.search);
  }
  return intent;
}
