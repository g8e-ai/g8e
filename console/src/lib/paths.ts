// Copyright (c) 2026 Lateralus Labs, LLC.
// Licensed under the Business Source License 1.1 — see LICENSE for details.

// Browser-reachable Gateway routes. Values mirror internal/constants/api_paths.go;
// every route here is classified RouteAuthNone, RouteAuthWebSession, or
// RouteAuthDual in internal/services/gateway/gateway_auth.go.

const enc = encodeURIComponent;

export const Paths = {
  health: '/api/v1/health',
  bootstrapStatus: '/api/v1/auth/bootstrap/status',
  logout: '/api/v1/auth/logout',
  usersMe: '/api/v1/users/me',
  sessionsMe: '/api/v1/auth/sessions/me',

  passkeys: '/api/v1/auth/passkeys',
  passkey: (credentialId: string) => `/api/v1/auth/passkeys/${enc(credentialId)}`,
  consoleRegisterChallenge: '/api/v1/auth/passkeys/console/register/challenge',
  consoleRegisterVerify: '/api/v1/auth/passkeys/console/register/verify',
  consoleAuthenticateChallenge: '/api/v1/auth/passkeys/console/authenticate/challenge',
  consoleAuthenticateVerify: '/api/v1/auth/passkeys/console/authenticate/verify',
  enrollmentRegisterChallenge: '/api/v1/auth/passkeys/enrollment/register/challenge',
  enrollmentRegisterVerify: '/api/v1/auth/passkeys/enrollment/register/verify',

  approvals: '/api/v1/approvals',
  approvalChallenge: (txHash: string) => `/api/v1/approvals/${enc(txHash)}/challenge`,
  approvalVerify: (txHash: string) => `/api/v1/approvals/${enc(txHash)}/verify`,

  cliRecoveryStatus: (token: string) => `/api/v1/auth/cli/recovery/status?token=${enc(token)}`,
  cliRecoveryApprove: '/api/v1/auth/cli/recovery/approve',
  platformEnrollmentsPending: '/api/v1/auth/platform-enrollments/pending',
  platformEnrollmentDecision: '/api/v1/auth/platform-enrollments/decision',

  operators: '/api/v1/operators',
  operator: (operatorId: string) => `/api/v1/operators/${enc(operatorId)}`,
  operatorStop: (operatorId: string) => `/api/v1/operators/${enc(operatorId)}/stop`,
  operatorsBind: '/api/v1/operators/bind',
  operatorsUnbind: '/api/v1/operators/unbind',

  // Ensemble browser proxy: the Gateway stamps user, web session, and the
  // session's bound Operators onto these requests before forwarding to g8ee.
  chat: '/api/v1/chat',
  chatStop: '/api/v1/chat/stop',
  investigations: '/api/v1/investigations',
  operatorApprovalRespond: '/api/v1/operator/approval/respond',
  llmSettingsGet: '/api/v1/settings/llm/get',
  llmSettings: '/api/v1/settings/llm',
  llmModels: '/api/v1/settings/llm/models',

  sseStream: '/api/v1/sse/stream',

  // OpenAPI 2.0 spec compiled into the Gateway (internal/services/gateway/docs);
  // classified RouteAuthDual, so the session cookie is accepted.
  apiSpec: '/swagger/doc.json',

  // Served on the Gateway's plain-HTTP bootstrap port, not the HTTPS console port.
  operatorBinary: (filename: string) => `/.well-known/g8e/bin/${filename}`,
  deployScriptLinux: '/g8e-deploy.sh',
  deployScriptWindows: '/g8e-deploy.ps1',
} as const;
