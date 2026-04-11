# g8e v4.3.1 — Passkey Auth & Operator Panel Fixes

v4.3.1 is a hotfix release resolving two critical bugs affecting user authentication and operator panel real-time updates.

## Major Changes

### Passkey Authentication Fix

**Root cause:** The `PasskeyVerifyResponse` model was missing the `success` field, causing frontend validation failures during the passkey authentication flow.

**Fix:**
- Added `success: true` to the `PasskeyVerifyResponse` model in `response_models.js`
- Updated passkey routes to properly return the complete response shape including the `success` field
- Updated integration test expectations to verify the `success` field is present

**Impact:** Users can now successfully complete passkey authentication during setup and login without validation errors.

### Operator Panel SSE Race Condition Fix

**Root cause:** SSE event pushes from g8ee to VSOD were missing the `user_id` field. The `SSEPushRequest` model did not require `user_id`, and g8ee services were not including it when publishing events. This caused operator panel updates to fail silently when the heartbeat service attempted to push events without the required user context.

**Fix:**
- Added `user_id` as a required field to `SSEPushRequest` in `request_models.js`
- Updated `HeartbeatService` to validate both `web_session_id` and `user_id` before pushing SSE events. Missing `user_id` now logs a warning and skips the push instead of failing
- Updated all g8ee services to include `user_id` when publishing events via `vsod_event_service`:
  - `AgentSSEService`
  - `AgentToolLoop`
  - `ChatPipeline`
  - `ChatTaskManager`
  - `CommandGenerator`
  - `ApprovalService`
  - `HeartbeatService`
- Fixed internal SSE route to properly construct `OperatorListUpdatedEvent` from the operator list payload instead of passing the raw result object

**Impact:** Operator panel now correctly receives real-time updates for operator status changes, heartbeat events, and list updates. The race condition where SSE events would fail due to missing user context is eliminated.

## Bug Fixes

- **Passkey auth** — `PasskeyVerifyResponse` now includes `success: true` field; frontend validation no longer fails during passkey authentication
- **SSE push** — `SSEPushRequest` now requires `user_id` as a mandatory field
- **Heartbeat service** — Validates both `web_session_id` and `user_id` before pushing SSE events; logs warning and skips push if missing
- **Operator panel list updated** — Internal SSE route now properly constructs `OperatorListUpdatedEvent` from operator list payload
- **g8ee event publishing** — All g8ee services now include `user_id` when publishing events via `vsod_event_service`
- **Terminal CSS** — Fixed max-width constraints to prevent overflow on wide screens

## Component Summary

| Component | Changes |
|-----------|---------|
| **g8ee** | Added `user_id` to all event publishing calls across 7 services |
| **VSOD** | Added `user_id` to `SSEPushRequest`, fixed passkey response model, fixed SSE route event construction |
| **Demo** | Automated SSH streaming configuration in `make up` |

## Quick Start

```bash
git clone https://github.com/g8e-ai/g8e-ai/g8e.git && cd g8e
git checkout v4.3.1
./g8e platform setup
```

## Security & Privacy

v4.3.1 fixes authentication and real-time update reliability issues:

- **Passkey authentication** — Response model now includes the required `success` field, ensuring users can complete FIDO2 passkey authentication during setup and login
- **SSE event integrity** — All SSE event pushes now include the required `user_id` field, ensuring operator panel updates are properly routed to the correct user session and not silently dropped

---

**g8e** — AI-powered, human-driven infrastructure operations. Fully self-hosted. Air-gap capable. Security and privacy by design.

[Website](https://lateraluslabs.com) | [Docs](../index.md) | [License](../../LICENSE)
