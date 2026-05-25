# Protocol Documentation
<a name="top"></a>

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
GovernanceEnvelope is the single canonical container for all g8e mutations.
It binds identity, intent, state, and governance proofs into one transaction.


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| id | [string](#string) |  | Identity &amp; Metadata |
| timestamp | [google.protobuf.Timestamp](#google-protobuf-Timestamp) |  |  |
| expires_at | [google.protobuf.Timestamp](#google-protobuf-Timestamp) |  |  |
| source_component | [Component](#g8e-common-v1-Component) |  |  |
| operator_id | [string](#string) |  |  |
| operator_session_id | [string](#string) |  |  |
| web_session_id | [string](#string) |  |  |
| cli_session_id | [string](#string) |  |  |
| event_type | [string](#string) |  | Intent &amp; Payload |
| payload | [bytes](#bytes) |  | Raw protobuf payload |
| intent_data | [google.protobuf.Struct](#google-protobuf-Struct) |  | Structured JSON-first view |
| action_type | [string](#string) |  | UAP-compatible action type (e.g., EXECUTE_BASH) |
| target_resource | [string](#string) |  | UAP-compatible target resource |
| state_merkle_root | [string](#string) |  | State &amp; Replay Protection |
| nonce | [string](#string) |  |  |
| transaction_hash | [string](#string) |  |  |
| protocol_version | [string](#string) |  | UAP-compatible protocol version (e.g., &#34;1.0&#34;) |
| governance | [GovernanceMetadata](#g8e-common-v1-GovernanceMetadata) |  | Governance Proofs |
| case_id | [string](#string) |  | Application Context |
| investigation_id | [string](#string) |  |  |
| task_id | [string](#string) |  |  |
| system_fingerprint | [string](#string) |  |  |






<a name="g8e-common-v1-GovernanceMetadata"></a>

### GovernanceMetadata
Unified Governance Metadata


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| l1 | [L1Metadata](#g8e-common-v1-L1Metadata) |  |  |
| l2 | [L2Metadata](#g8e-common-v1-L2Metadata) |  |  |
| l3 | [L3Metadata](#g8e-common-v1-L3Metadata) |  |  |
| gateway_signed | [bool](#bool) |  | Set to true if signed by the local gateway without full Tribunal consensus. This is used for single-agent MCP clients that bypass the L2 consensus layer. |






<a name="g8e-common-v1-L1Metadata"></a>

### L1Metadata
Doctrine (L1Doctrine) Governance: Technical Bedrock (Hard Gates)


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| validated | [bool](#bool) |  |  |
| violations | [string](#string) | repeated |  |






<a name="g8e-common-v1-L2Metadata"></a>

### L2Metadata
Consensus (L2Consensus) Governance: Consensus (Tribunal)


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| consensus_signature | [string](#string) |  | ED25519 signature over transaction_hash|decision |
| agent_ids | [string](#string) | repeated | IDs of agents that voted |
| key_id | [string](#string) |  | ID of the key used for signature |






<a name="g8e-common-v1-L3Metadata"></a>

### L3Metadata



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| proof | [L3Proof](#g8e-common-v1-L3Proof) |  | Real WebAuthn proof |
| auto_approved | [bool](#bool) |  | True if skipped via auto-approval policy |






<a name="g8e-common-v1-L3Proof"></a>

### L3Proof
Notary (L3Notary) Governance: Authorization (Human-in-the-loop)


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| client_data_json | [string](#string) |  |  |
| authenticator_data | [string](#string) |  |  |
| signature | [string](#string) |  |  |
| credential_id | [string](#string) |  |  |
| mtls_cert_fingerprint | [string](#string) |  | CLI mTLS proof: fingerprint of the CLI certificate used for authentication Used when the L3Notary proof is based on mTLS certificate validation rather than WebAuthn |





 


<a name="g8e-common-v1-Component"></a>

### Component
Source component identifier

| Name | Number | Description |
| ---- | ------ | ----------- |
| COMPONENT_UNSPECIFIED | 0 |  |
| COMPONENT_G8EE | 1 |  |
| COMPONENT_G8EO | 2 |  |
| COMPONENT_CLIENT | 3 |  |


 


<a name="g8e_common_v1_common-proto-extensions"></a>

### File-level Extensions
| Extension | Type | Base | Number | Description |
| --------- | ---- | ---- | ------ | ----------- |
| forbidden_patterns | string | .google.protobuf.FieldOptions | 50001 | Comma-separated list of forbidden regex patterns |

 

 



## Scalar Value Types

| .proto Type | Notes | C++ | Java | Python | Go | C# | PHP | Ruby |
| ----------- | ----- | --- | ---- | ------ | -- | -- | --- | ---- |
| <a name="double" /> double |  | double | double | float | float64 | double | float | Float |
| <a name="float" /> float |  | float | float | float | float32 | float | float | Float |
| <a name="int32" /> int32 | Uses variable-length encoding. Inefficient for encoding negative numbers – if your field is likely to have negative values, use sint32 instead. | int32 | int | int | int32 | int | integer | Bignum or Fixnum (as required) |
| <a name="int64" /> int64 | Uses variable-length encoding. Inefficient for encoding negative numbers – if your field is likely to have negative values, use sint64 instead. | int64 | long | int/long | int64 | long | integer/string | Bignum |
| <a name="uint32" /> uint32 | Uses variable-length encoding. | uint32 | int | int/long | uint32 | uint | integer | Bignum or Fixnum (as required) |
| <a name="uint64" /> uint64 | Uses variable-length encoding. | uint64 | long | int/long | uint64 | ulong | integer/string | Bignum or Fixnum (as required) |
| <a name="sint32" /> sint32 | Uses variable-length encoding. Signed int value. These more efficiently encode negative numbers than regular int32s. | int32 | int | int | int32 | int | integer | Bignum or Fixnum (as required) |
| <a name="sint64" /> sint64 | Uses variable-length encoding. Signed int value. These more efficiently encode negative numbers than regular int64s. | int64 | long | int/long | int64 | long | integer/string | Bignum |
| <a name="fixed32" /> fixed32 | Always four bytes. More efficient than uint32 if values are often greater than 2^28. | uint32 | int | int | uint32 | uint | integer | Bignum or Fixnum (as required) |
| <a name="fixed64" /> fixed64 | Always eight bytes. More efficient than uint64 if values are often greater than 2^56. | uint64 | long | int/long | uint64 | ulong | integer/string | Bignum |
| <a name="sfixed32" /> sfixed32 | Always four bytes. | int32 | int | int | int32 | int | integer | Bignum or Fixnum (as required) |
| <a name="sfixed64" /> sfixed64 | Always eight bytes. | int64 | long | int/long | int64 | long | integer/string | Bignum |
| <a name="bool" /> bool |  | bool | boolean | boolean | bool | bool | boolean | TrueClass/FalseClass |
| <a name="string" /> string | A string must always contain UTF-8 encoded or 7-bit ASCII text. | string | String | str/unicode | string | string | string | String (UTF-8) |
| <a name="bytes" /> bytes | May contain any arbitrary sequence of bytes. | string | ByteString | str | []byte | ByteString | string | String (ASCII-8BIT) |

