# Protocol Documentation

## Table of Contents

- [g8e/common/v1/common.proto](#g8e_common_v1_common-proto)
    - [GovernanceEnvelope](#g8e-common-v1-GovernanceEnvelope)
    - [GovernanceMetadata](#g8e-common-v1-GovernanceMetadata)
    - [L1Metadata](#g8e-common-v1-L1Metadata)
    - [L2Metadata](#g8e-common-v1-L2Metadata)
    - [L3Metadata](#g8e-common-v1-L3Metadata)
    - [L3Proof](#g8e-common-v1-L3Proof)
    - [Component](#g8e-common-v1-Component)
    - [File-level Extensions](#g8e_common_v1_common-proto-extensions)
- [Scalar Value Types](#scalar-value-types)

<a name="g8e_common_v1_common-proto"></a>
<p align="right"><a href="#top">Top</a></p>

## g8e/common/v1/common.proto

<a name="g8e-common-v1-GovernanceEnvelope"></a>

### GovernanceEnvelope
`GovernanceEnvelope` is the canonical container for platform mutations. It binds identity, intent, state, and governance proofs into a single transaction.

| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| id | [string](#string) | | Unique transaction identifier. |
| timestamp | [google.protobuf.Timestamp](#google-protobuf-Timestamp) | | Generation timestamp. |
| expires_at | [google.protobuf.Timestamp](#google-protobuf-Timestamp) | | Expiration timestamp. |
| source_component | [Component](#g8e-common-v1-Component) | | Originating system component. |
| operator_id | [string](#string) | | Operator identifier. |
| operator_session_id | [string](#string) | | Operator session identifier. |
| web_session_id | [string](#string) | | Web session identifier. |
| cli_session_id | [string](#string) | | CLI session identifier. |
| requestor_user_id | [string](#string) | | The human user who authorized the action (delegator). |
| acting_app_id | [string](#string) | | The application or tool acting on behalf of the user (delegate). |
| event_type | [string](#string) | | Event classification string. |
| payload | [bytes](#bytes) | | Raw protobuf payload. |
| intent_data | [google.protobuf.Struct](#google-protobuf-Struct) | | Structured JSON-first view of the intent. |
| action_type | [string](#string) | | UAP-compatible action type (e.g., `EXECUTE_BASH`). |
| target_resource | [string](#string) | | UAP-compatible target resource identifier. |
| state_merkle_root | [string](#string) | | Merkle root for state verification. |
| nonce | [string](#string) | | Replay protection nonce. |
| transaction_hash | [string](#string) | | Cryptographic hash of the transaction. |
| protocol_version | [string](#string) | | UAP-compatible protocol version (e.g., "1.0"). |
| governance | [GovernanceMetadata](#g8e-common-v1-GovernanceMetadata) | | Governance proofs covering L1, L2, and L3. |
| case_id | [string](#string) | | Application context case identifier. |
| investigation_id | [string](#string) | | Investigation identifier. |
| task_id | [string](#string) | | Task identifier. |
| system_fingerprint | [string](#string) | | System fingerprint for environment tracking. |
| tenant_id | [string](#string) | | Tenant identifier. |
| binding_persona | [string](#string) | | Associated security persona. |

<a name="g8e-common-v1-GovernanceMetadata"></a>

### GovernanceMetadata
`GovernanceMetadata` provides a unified container for multi-layered governance proofs.

| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| l1 | [L1Metadata](#g8e-common-v1-L1Metadata) | | L1 Doctrine metadata. |
| l2 | [L2Metadata](#g8e-common-v1-L2Metadata) | | L2 Consensus metadata. |
| l3 | [L3Metadata](#g8e-common-v1-L3Metadata) | | L3 Notary metadata. |
| gateway_signed | [bool](#bool) | | Set to true if signed by the local gateway without full L2 consensus. Used for single-agent MCP clients. |

<a name="g8e-common-v1-L1Metadata"></a>

### L1Metadata
Doctrine (L1Doctrine) Governance: Technical Bedrock (Hard Gates).

| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| validated | [bool](#bool) | | Indicates if the transaction passed doctrine validation. |
| violations | [string](#string) | repeated | List of detected doctrine violations. |

<a name="g8e-common-v1-L2Metadata"></a>

### L2Metadata
Consensus (L2Consensus) Governance: Multi-agent consensus verification.

| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| consensus_signature | [string](#string) | | Ed25519 signature over `transaction_hash\|decision`. |
| agent_ids | [string](#string) | repeated | Identifiers of agents that participated in the consensus. |
| key_id | [string](#string) | | Identifier of the key used for the signature. |

<a name="g8e-common-v1-L3Metadata"></a>

### L3Metadata
Notary (L3Notary) Governance: Authorization metadata.

| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| proof | [L3Proof](#g8e-common-v1-L3Proof) | | Cryptographic proof of human-in-the-loop authorization. |
| auto_approved | [bool](#bool) | | Set to true if the transaction bypassed human review via policy. |

<a name="g8e-common-v1-L3Proof"></a>

### L3Proof
`L3Proof` contains either WebAuthn or CLI-based cryptographic proofs of authorization.

| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| client_data_json | [string](#string) | | WebAuthn `clientDataJSON`. |
| authenticator_data | [string](#string) | | WebAuthn `authenticatorData`. |
| signature | [string](#string) | | Cryptographic signature. |
| credential_id | [string](#string) | | WebAuthn credential identifier. |
| mtls_cert_fingerprint | [string](#string) | | CLI mTLS proof fingerprint. |
| cli_signature | [string](#string) | | CLI-generated signature over `transaction_hash`. |

<a name="g8e-common-v1-Component"></a>

### Component
Source component identifier.

| Name | Number | Description |
| ---- | ------ | ----------- |
| COMPONENT_UNSPECIFIED | 0 | Default value. |
| COMPONENT_AGENT | 1 | Originating from an agent. |
| COMPONENT_G8EO | 2 | Originating from the g8e operator. |
| COMPONENT_CLIENT | 3 | Originating from a client application. |

<a name="g8e_common_v1_common-proto-extensions"></a>

### File-level Extensions
| Extension | Type | Base | Number | Description |
| --------- | ---- | ---- | ------ | ----------- |
| forbidden_patterns | string | .google.protobuf.FieldOptions | 50001 | Comma-separated list of forbidden regex patterns. |

## Scalar Value Types

| .proto Type | Notes | C++ | Java | Python | Go | C# | PHP | Ruby |
| ----------- | ----- | --- | ---- | ------ | -- | -- | --- | ---- |
| <a name="double" /> double | | double | double | float | float64 | double | float | Float |
| <a name="float" /> float | | float | float | float | float32 | float | float | Float |
| <a name="int32" /> int32 | Uses variable-length encoding. Inefficient for negative numbers. | int32 | int | int | int32 | int | integer | Bignum or Fixnum |
| <a name="int64" /> int64 | Uses variable-length encoding. Inefficient for negative numbers. | int64 | long | int/long | int64 | long | integer/string | Bignum |
| <a name="uint32" /> uint32 | Uses variable-length encoding. | uint32 | int | int/long | uint32 | uint | integer | Bignum or Fixnum |
| <a name="uint64" /> uint64 | Uses variable-length encoding. | uint64 | long | int/long | uint64 | ulong | integer/string | Bignum or Fixnum |
| <a name="sint32" /> sint32 | Uses variable-length encoding. Signed int value. More efficient for negative numbers than int32. | int32 | int | int | int32 | int | integer | Bignum or Fixnum |
| <a name="sint64" /> sint64 | Uses variable-length encoding. Signed int value. More efficient for negative numbers than int64. | int64 | long | int/long | int64 | long | integer/string | Bignum |
| <a name="fixed32" /> fixed32 | Always four bytes. More efficient than uint32 if values are often greater than 2^28. | uint32 | int | int | uint32 | uint | integer | Bignum or Fixnum |
| <a name="fixed64" /> fixed64 | Always eight bytes. More efficient than uint64 if values are often greater than 2^56. | uint64 | long | int/long | uint64 | ulong | integer/string | Bignum |
| <a name="sfixed32" /> sfixed32 | Always four bytes. | int32 | int | int | int32 | int | integer | Bignum or Fixnum |
| <a name="sfixed64" /> sfixed64 | Always eight bytes. | int64 | long | int/long | int64 | long | integer/string | Bignum |
| <a name="bool" /> bool | | bool | boolean | boolean | bool | bool | boolean | TrueClass/FalseClass |
| <a name="string" /> string | A string must always contain UTF-8 encoded or 7-bit ASCII text. | string | String | str/unicode | string | string | string | String (UTF-8) |
| <a name="bytes" /> bytes | May contain any arbitrary sequence of bytes. | string | ByteString | str | []byte | ByteString | string | String (ASCII-8BIT) |

