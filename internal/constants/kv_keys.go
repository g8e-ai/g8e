// Copyright (c) 2026 Lateralus Labs, LLC.
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package constants

// KV key and session type constants.
// Canonical values defined in protocol/constants/kv_keys.json (the source of truth).

const (
	KVCachePrefix = "g8e"
)

// SentinelKeyPrefix namespaces sentinel UEI tokens in kv_store to avoid collisions
// with cache/doc invalidation entries written by the document store triggers.
const SentinelKeyPrefix = "g8e:sentinel:"

const (
	KVKeyCacheDoc                     = "g8e:cache:doc:{collection}:{id}"
	KVKeyCacheQuery                   = "g8e:cache:query:{collection}:{hash}"
	KVKeySessionWeb                   = "g8e:sessions:{session.type}:{session.id}"
	KVKeySessionOperator              = "g8e:sessions:operator:{operator.session.id}"
	KVKeySessionOperatorBind          = "g8e:sessions:operator:{operator.session.id}:bind"
	KVKeySessionWebBind               = "g8e:sessions:web:{web.session.id}:bind"
	KVKeyOperatorFirstDeployed        = "g8e:operator:{operator.id}:first.deployed"
	KVKeyOperatorTrackedStatus        = "g8e:operator:{operator.id}:tracked.status"
	KVKeyUserOperators                = "g8e:user:{user.id}:operators"
	KVKeyUserWebSessions              = "g8e:user:{user.id}:web_sessions"
	KVKeyUserMemories                 = "g8e:user:{user.id}:memories"
	KVKeyInvestigationAttachment      = "g8e:investigation:{investigation.id}:attachment:{attachment.id}"
	KVKeyInvestigationAttachmentIndex = "g8e:investigation:{investigation.id}:attachment.index"
	KVKeyAuthNonce                    = "g8e:auth:nonce:{nonce}"
	KVKeyAuthTokenDownload            = "g8e:auth:token:download:{token}"            // #nosec G101 - constant string, not credential
	KVKeyAuthTokenDevice              = "g8e:auth:token:device:{token}"              // #nosec G101 - constant string, not credential
	KVKeyAuthTokenDeviceUses          = "g8e:auth:token:device:{token}:uses"         // #nosec G101 - constant string, not credential
	KVKeyAuthTokenDeviceFingerprints  = "g8e:auth:token:device:{token}:fingerprints" // #nosec G101 - constant string, not credential
	KVKeyAuthTokenDeviceRegLock       = "g8e:auth:token:device:{token}:reg.lock"     // #nosec G101 - constant string, not credential
	KVKeyAuthDeviceList               = "g8e:auth:device.list:{user.id}"
	KVKeyAuthLoginFailed              = "g8e:auth:login:{identifier}:failed"
	KVKeyAuthLoginLock                = "g8e:auth:login:{identifier}:lock"
	KVKeyAuthLoginIPAccounts          = "g8e:auth:login:ip:{ip}:accounts"
	KVKeyExecutionPendingCmd          = "g8e:execution:{execution.id}:pending.cmd"
)

const (
	KVSessionTypeWeb      = "web"
	KVSessionTypeOperator = "operator"
)
