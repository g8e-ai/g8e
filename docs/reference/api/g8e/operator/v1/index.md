# Protocol Documentation
<a name="top"></a>

## Table of Contents

- [g8e/operator/v1/operator.proto](#g8e_operator_v1_operator-proto)
    - [A2aCallRequested](#g8e-operator-v1-A2aCallRequested)
    - [ActionReceipt](#g8e-operator-v1-ActionReceipt)
    - [AssertionResponse](#g8e-operator-v1-AssertionResponse)
    - [AttestationResponse](#g8e-operator-v1-AttestationResponse)
    - [AuditEvent](#g8e-operator-v1-AuditEvent)
    - [AuditFileMutation](#g8e-operator-v1-AuditFileMutation)
    - [AuditMsgRequested](#g8e-operator-v1-AuditMsgRequested)
    - [AuditWebSession](#g8e-operator-v1-AuditWebSession)
    - [BindOperatorsRequested](#g8e-operator-v1-BindOperatorsRequested)
    - [BindOperatorsResult](#g8e-operator-v1-BindOperatorsResult)
    - [CapabilityFlags](#g8e-operator-v1-CapabilityFlags)
    - [CheckPortRequested](#g8e-operator-v1-CheckPortRequested)
    - [CommandCancelRequested](#g8e-operator-v1-CommandCancelRequested)
    - [CommandRequested](#g8e-operator-v1-CommandRequested)
    - [CommandRequested.EnvironmentEntry](#g8e-operator-v1-CommandRequested-EnvironmentEntry)
    - [CommandResult](#g8e-operator-v1-CommandResult)
    - [CommitmentAttestation](#g8e-operator-v1-CommitmentAttestation)
    - [CreateDeviceLinkRequested](#g8e-operator-v1-CreateDeviceLinkRequested)
    - [DeleteDeviceLinkRequested](#g8e-operator-v1-DeleteDeviceLinkRequested)
    - [DeviceLink](#g8e-operator-v1-DeviceLink)
    - [DeviceLinkResult](#g8e-operator-v1-DeviceLinkResult)
    - [DirectCommandAuditRequested](#g8e-operator-v1-DirectCommandAuditRequested)
    - [DirectCommandResultAuditRequested](#g8e-operator-v1-DirectCommandResultAuditRequested)
    - [DiskDetails](#g8e-operator-v1-DiskDetails)
    - [EnvironmentDetails](#g8e-operator-v1-EnvironmentDetails)
    - [EvalAnswerRequested](#g8e-operator-v1-EvalAnswerRequested)
    - [ExecutionStatusUpdate](#g8e-operator-v1-ExecutionStatusUpdate)
    - [FetchFileDiffRequested](#g8e-operator-v1-FetchFileDiffRequested)
    - [FetchFileDiffResult](#g8e-operator-v1-FetchFileDiffResult)
    - [FetchFileHistoryRequested](#g8e-operator-v1-FetchFileHistoryRequested)
    - [FetchFileHistoryResult](#g8e-operator-v1-FetchFileHistoryResult)
    - [FetchHistoryRequested](#g8e-operator-v1-FetchHistoryRequested)
    - [FetchHistoryResult](#g8e-operator-v1-FetchHistoryResult)
    - [FetchLogsRequested](#g8e-operator-v1-FetchLogsRequested)
    - [FetchLogsResult](#g8e-operator-v1-FetchLogsResult)
    - [FileDiffEntry](#g8e-operator-v1-FileDiffEntry)
    - [FileEditRequested](#g8e-operator-v1-FileEditRequested)
    - [FileEditResult](#g8e-operator-v1-FileEditResult)
    - [FileHistoryEntry](#g8e-operator-v1-FileHistoryEntry)
    - [FingerprintDetails](#g8e-operator-v1-FingerprintDetails)
    - [FsEntry](#g8e-operator-v1-FsEntry)
    - [FsGrepMatch](#g8e-operator-v1-FsGrepMatch)
    - [FsGrepRequested](#g8e-operator-v1-FsGrepRequested)
    - [FsGrepResult](#g8e-operator-v1-FsGrepResult)
    - [FsListRequested](#g8e-operator-v1-FsListRequested)
    - [FsListResult](#g8e-operator-v1-FsListResult)
    - [FsReadRequested](#g8e-operator-v1-FsReadRequested)
    - [FsReadResult](#g8e-operator-v1-FsReadResult)
    - [GetRevocationBundleRequested](#g8e-operator-v1-GetRevocationBundleRequested)
    - [GetRevocationBundleResult](#g8e-operator-v1-GetRevocationBundleResult)
    - [HeartbeatRequested](#g8e-operator-v1-HeartbeatRequested)
    - [HeartbeatResult](#g8e-operator-v1-HeartbeatResult)
    - [ListDeviceLinksRequested](#g8e-operator-v1-ListDeviceLinksRequested)
    - [ListDeviceLinksResult](#g8e-operator-v1-ListDeviceLinksResult)
    - [ListOperatorSlotsRequested](#g8e-operator-v1-ListOperatorSlotsRequested)
    - [ListOperatorSlotsResult](#g8e-operator-v1-ListOperatorSlotsResult)
    - [ListPasskeyCredentialsRequested](#g8e-operator-v1-ListPasskeyCredentialsRequested)
    - [ListPasskeyCredentialsResult](#g8e-operator-v1-ListPasskeyCredentialsResult)
    - [McpCallRequested](#g8e-operator-v1-McpCallRequested)
    - [McpPromptGetRequested](#g8e-operator-v1-McpPromptGetRequested)
    - [McpPromptListRequested](#g8e-operator-v1-McpPromptListRequested)
    - [McpResourceListRequested](#g8e-operator-v1-McpResourceListRequested)
    - [McpResourceReadRequested](#g8e-operator-v1-McpResourceReadRequested)
    - [MemoryDetails](#g8e-operator-v1-MemoryDetails)
    - [NetworkInfo](#g8e-operator-v1-NetworkInfo)
    - [NetworkInterface](#g8e-operator-v1-NetworkInterface)
    - [OSDetails](#g8e-operator-v1-OSDetails)
    - [OperatorDocument](#g8e-operator-v1-OperatorDocument)
    - [PasskeyAuthChallengeRequested](#g8e-operator-v1-PasskeyAuthChallengeRequested)
    - [PasskeyAuthChallengeResult](#g8e-operator-v1-PasskeyAuthChallengeResult)
    - [PasskeyAuthVerifyRequested](#g8e-operator-v1-PasskeyAuthVerifyRequested)
    - [PasskeyAuthVerifyResult](#g8e-operator-v1-PasskeyAuthVerifyResult)
    - [PasskeyCredential](#g8e-operator-v1-PasskeyCredential)
    - [PasskeyRegisterChallengeRequested](#g8e-operator-v1-PasskeyRegisterChallengeRequested)
    - [PasskeyRegisterChallengeResult](#g8e-operator-v1-PasskeyRegisterChallengeResult)
    - [PasskeyRegisterChallengeResult.AuthenticatorSelection](#g8e-operator-v1-PasskeyRegisterChallengeResult-AuthenticatorSelection)
    - [PasskeyRegisterChallengeResult.PublicKeyCredentialParameters](#g8e-operator-v1-PasskeyRegisterChallengeResult-PublicKeyCredentialParameters)
    - [PasskeyRegisterChallengeResult.RelyingParty](#g8e-operator-v1-PasskeyRegisterChallengeResult-RelyingParty)
    - [PasskeyRegisterChallengeResult.UserInfo](#g8e-operator-v1-PasskeyRegisterChallengeResult-UserInfo)
    - [PasskeyRegisterVerifyRequested](#g8e-operator-v1-PasskeyRegisterVerifyRequested)
    - [PasskeyRegisterVerifyResult](#g8e-operator-v1-PasskeyRegisterVerifyResult)
    - [PerformanceMetrics](#g8e-operator-v1-PerformanceMetrics)
    - [PortCheckEntry](#g8e-operator-v1-PortCheckEntry)
    - [PortCheckResult](#g8e-operator-v1-PortCheckResult)
    - [RestoreFileRequested](#g8e-operator-v1-RestoreFileRequested)
    - [RestoreFileResult](#g8e-operator-v1-RestoreFileResult)
    - [RevokeCertificateRequested](#g8e-operator-v1-RevokeCertificateRequested)
    - [RevokeCertificateResult](#g8e-operator-v1-RevokeCertificateResult)
    - [RevokePasskeyCredentialRequested](#g8e-operator-v1-RevokePasskeyCredentialRequested)
    - [RevokePasskeyCredentialResult](#g8e-operator-v1-RevokePasskeyCredentialResult)
    - [SetTargetContextRequested](#g8e-operator-v1-SetTargetContextRequested)
    - [SetTargetContextResult](#g8e-operator-v1-SetTargetContextResult)
    - [ShutdownRequested](#g8e-operator-v1-ShutdownRequested)
    - [SignCertificateRequested](#g8e-operator-v1-SignCertificateRequested)
    - [SignCertificateResult](#g8e-operator-v1-SignCertificateResult)
    - [SystemIdentity](#g8e-operator-v1-SystemIdentity)
    - [TerminateOperatorRequested](#g8e-operator-v1-TerminateOperatorRequested)
    - [TerminateOperatorResult](#g8e-operator-v1-TerminateOperatorResult)
    - [UnbindOperatorsRequested](#g8e-operator-v1-UnbindOperatorsRequested)
    - [UnbindOperatorsResult](#g8e-operator-v1-UnbindOperatorsResult)
    - [UptimeInfo](#g8e-operator-v1-UptimeInfo)
    - [UserDetails](#g8e-operator-v1-UserDetails)
    - [VersionInfo](#g8e-operator-v1-VersionInfo)
  
    - [ExecutionStatus](#g8e-operator-v1-ExecutionStatus)
    - [HeartbeatType](#g8e-operator-v1-HeartbeatType)
    - [L2Status](#g8e-operator-v1-L2Status)
    - [L3Status](#g8e-operator-v1-L3Status)
  
    - [OperatorService](#g8e-operator-v1-OperatorService)
  
- [Scalar Value Types](#scalar-value-types)



<a name="g8e_operator_v1_operator-proto"></a>
<p align="right"><a href="#top">Top</a></p>

## g8e/operator/v1/operator.proto



<a name="g8e-operator-v1-A2aCallRequested"></a>

### A2aCallRequested
Payload for g8e.v1.operator.a2a.call.requested
Mirrors McpCallRequested for the Agent-to-Agent JSON protocol.


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| skill_name | [string](#string) |  | Target agent skill / capability name. |
| payload_json | [string](#string) |  | JSON-encoded A2A task payload. |
| execution_id | [string](#string) |  |  |






<a name="g8e-operator-v1-ActionReceipt"></a>

### ActionReceipt
ActionReceipt is the signed proof of a completed or failed mutation.
It is emitted by the Warden after execution.


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| transaction_id | [string](#string) |  | Original GovernanceEnvelope ID |
| transaction_hash | [string](#string) |  | Original GovernanceEnvelope hash |
| status | [ExecutionStatus](#g8e-operator-v1-ExecutionStatus) |  | Final execution status |
| result_summary | [string](#string) |  | Concise summary of the result (e.g. &#34;exit code 0&#34;, &#34;file written&#34;) |
| state_root_before | [string](#string) |  | State root before execution |
| state_root_after | [string](#string) |  | State root after execution |
| executed_at_unix_ms | [int64](#int64) |  | Timestamp when execution finished |
| signer_key_id | [string](#string) |  | ID of the Warden&#39;s signing key |
| signature | [string](#string) |  | ED25519 signature over canonical serialization of fields 1-8 |
| l2_status | [L2Status](#g8e-operator-v1-L2Status) |  | Status of L2 (Consensus) signature verification. Distinguishes between &#34;not required&#34; vs &#34;required but failed&#34; for compliance. |
| l3_status | [L3Status](#g8e-operator-v1-L3Status) |  | Status of L3 (Notary/Human) proof verification. Distinguishes between &#34;not required&#34; vs &#34;required but failed&#34; for compliance. |






<a name="g8e-operator-v1-AssertionResponse"></a>

### AssertionResponse



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| id | [string](#string) |  |  |
| raw_id | [string](#string) |  |  |
| client_data_json | [string](#string) |  |  |
| authenticator_data | [string](#string) |  |  |
| signature | [string](#string) |  |  |
| user_handle | [string](#string) |  |  |






<a name="g8e-operator-v1-AttestationResponse"></a>

### AttestationResponse



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| id | [string](#string) |  |  |
| raw_id | [string](#string) |  |  |
| client_data_json | [string](#string) |  |  |
| attestation_object | [string](#string) |  |  |
| transports | [string](#string) | repeated |  |






<a name="g8e-operator-v1-AuditEvent"></a>

### AuditEvent



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| id | [int64](#int64) |  |  |
| operator_session_id | [string](#string) |  |  |
| timestamp | [string](#string) |  |  |
| type | [string](#string) |  |  |
| content_text | [string](#string) |  |  |
| command_raw | [string](#string) |  |  |
| command_exit_code | [int32](#int32) |  |  |
| command_stdout | [string](#string) |  |  |
| command_stderr | [string](#string) |  |  |
| execution_duration_ms | [int64](#int64) |  |  |
| stored_locally | [bool](#bool) |  |  |
| stdout_truncated | [bool](#bool) |  |  |
| stderr_truncated | [bool](#bool) |  |  |
| file_mutations | [AuditFileMutation](#g8e-operator-v1-AuditFileMutation) | repeated |  |






<a name="g8e-operator-v1-AuditFileMutation"></a>

### AuditFileMutation



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| id | [int64](#int64) |  |  |
| filepath | [string](#string) |  |  |
| operation | [string](#string) |  |  |
| ledger_hash_before | [string](#string) |  |  |
| ledger_hash_after | [string](#string) |  |  |
| diff_stat | [string](#string) |  |  |






<a name="g8e-operator-v1-AuditMsgRequested"></a>

### AuditMsgRequested



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| content | [string](#string) |  |  |






<a name="g8e-operator-v1-AuditWebSession"></a>

### AuditWebSession



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| id | [string](#string) |  |  |
| title | [string](#string) |  |  |
| created_at | [string](#string) |  |  |
| user_identity | [string](#string) |  |  |






<a name="g8e-operator-v1-BindOperatorsRequested"></a>

### BindOperatorsRequested



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| operator_ids | [string](#string) | repeated |  |
| user_id | [string](#string) |  |  |
| web_session_id | [string](#string) |  | The client/web session ID |






<a name="g8e-operator-v1-BindOperatorsResult"></a>

### BindOperatorsResult



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| success | [bool](#bool) |  |  |
| bound_count | [int32](#int32) |  |  |
| failed_count | [int32](#int32) |  |  |
| bound_operator_ids | [string](#string) | repeated |  |
| failed_operator_ids | [string](#string) | repeated |  |
| error | [string](#string) |  |  |






<a name="g8e-operator-v1-CapabilityFlags"></a>

### CapabilityFlags



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| local_storage_enabled | [bool](#bool) |  |  |
| git_available | [bool](#bool) |  |  |
| ledger_mirror_enabled | [bool](#bool) |  |  |






<a name="g8e-operator-v1-CheckPortRequested"></a>

### CheckPortRequested



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| execution_id | [string](#string) |  |  |
| port | [int32](#int32) |  |  |
| host | [string](#string) |  |  |
| protocol | [string](#string) |  |  |






<a name="g8e-operator-v1-CommandCancelRequested"></a>

### CommandCancelRequested



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| execution_id | [string](#string) |  |  |






<a name="g8e-operator-v1-CommandRequested"></a>

### CommandRequested
Payload for g8e.v1.operator.command.requested


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| command | [string](#string) |  | Shell command string to execute |
| execution_id | [string](#string) |  | Unique execution identifier (correlation key) |
| justification | [string](#string) |  | Justification for running this command (High-level Sage request) |
| vault_mode | [string](#string) |  | Vault scrubbing mode for output storage |
| timeout_seconds | [int32](#int32) |  | Execution timeout override in seconds |
| intent | [string](#string) |  | Intent: The high-level goal (The &#34;Why&#34;) |
| environment | [CommandRequested.EnvironmentEntry](#g8e-operator-v1-CommandRequested-EnvironmentEntry) | repeated | Environment variables |
| working_directory | [string](#string) |  | Working directory override |






<a name="g8e-operator-v1-CommandRequested-EnvironmentEntry"></a>

### CommandRequested.EnvironmentEntry



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| key | [string](#string) |  |  |
| value | [string](#string) |  |  |






<a name="g8e-operator-v1-CommandResult"></a>

### CommandResult
Result payload for g8e.v1.operator.command.completed or failed


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| execution_id | [string](#string) |  |  |
| status | [ExecutionStatus](#g8e-operator-v1-ExecutionStatus) |  |  |
| stdout | [string](#string) |  |  |
| error | [string](#string) |  |  |
| stderr | [string](#string) |  |  |
| return_code | [int32](#int32) |  |  |
| execution_time_seconds | [float](#float) |  |  |
| start_time_unix_ms | [int64](#int64) |  | Timestamp when execution started |
| end_time_unix_ms | [int64](#int64) |  | Timestamp when execution ended |






<a name="g8e-operator-v1-CommitmentAttestation"></a>

### CommitmentAttestation
CommitmentAttestation is the Auditor&#39;s signed record that a verified
transaction is consistent with the prior ledger of commitments and is
authorized to be executed.


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| transaction_id | [string](#string) |  | TransactionID and TransactionHash identify the envelope being committed (these mirror UAPEnvelope.Id / TransactionHash). |
| transaction_hash | [string](#string) |  |  |
| prior_commitment_hash | [string](#string) |  | PriorCommitmentHash is the Hash of the immediately-preceding CommitmentAttestation in the ledger, or GenesisPriorHash for the first commitment. This is what makes the ledger a chain. |
| state_root_at_commit | [string](#string) |  | StateRootAtCommit is the Gateway&#39;s state root at the moment the Auditor signed this attestation. It is checked against the envelope.StateMerkleRoot the L2 consensus signed over. |
| l2_signature_digest | [string](#string) |  | L2SignatureDigest, WardenIntentSignatureDigest and HumanSignatureDigest are SHA-256 of the corresponding signatures, captured so the attestation cryptographically binds all three authorities into a single commitment without duplicating their raw signatures here. |
| warden_intent_signature_digest | [string](#string) |  |  |
| human_signature_digest | [string](#string) |  |  |
| action_type | [string](#string) |  | ActionType and TargetResource are carried for ledger readability without re-fetching the original envelope. |
| target_resource | [string](#string) |  |  |
| committed_at_unix_ms | [int64](#int64) |  | CommittedAtUnixMs is the Auditor&#39;s local timestamp at signing. |
| auditor_key_id | [string](#string) |  | AuditorKeyID identifies which Auditor key signed this attestation. |
| signature | [string](#string) |  | ED25519 signature over canonical serialization of fields 1-11 |
| hash | [string](#string) |  | SHA-256 of fields 1-11. It is what the next attestation chains to. |






<a name="g8e-operator-v1-CreateDeviceLinkRequested"></a>

### CreateDeviceLinkRequested



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| user_id | [string](#string) |  |  |
| organization_id | [string](#string) |  |  |
| operator_id | [string](#string) |  |  |
| web_session_id | [string](#string) |  |  |
| name | [string](#string) |  |  |
| max_uses | [int32](#int32) |  |  |
| ttl_seconds | [int32](#int32) |  |  |






<a name="g8e-operator-v1-DeleteDeviceLinkRequested"></a>

### DeleteDeviceLinkRequested



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| token | [string](#string) |  |  |
| user_id | [string](#string) |  |  |






<a name="g8e-operator-v1-DeviceLink"></a>

### DeviceLink



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| token | [string](#string) |  |  |
| user_id | [string](#string) |  |  |
| organization_id | [string](#string) |  |  |
| operator_id | [string](#string) |  |  |
| web_session_id | [string](#string) |  |  |
| name | [string](#string) |  |  |
| max_uses | [int32](#int32) |  |  |
| uses | [int32](#int32) |  |  |
| status | [string](#string) |  |  |
| created_at_unix_ms | [int64](#int64) |  |  |
| expires_at_unix_ms | [int64](#int64) |  |  |






<a name="g8e-operator-v1-DeviceLinkResult"></a>

### DeviceLinkResult



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| success | [bool](#bool) |  |  |
| link | [DeviceLink](#g8e-operator-v1-DeviceLink) |  |  |
| operator_command | [string](#string) |  |  |
| error | [string](#string) |  |  |






<a name="g8e-operator-v1-DirectCommandAuditRequested"></a>

### DirectCommandAuditRequested



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| command | [string](#string) |  |  |
| execution_id | [string](#string) |  |  |
| operator_session_id | [string](#string) |  |  |
| type | [string](#string) |  |  |






<a name="g8e-operator-v1-DirectCommandResultAuditRequested"></a>

### DirectCommandResultAuditRequested



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| command | [string](#string) |  |  |
| execution_id | [string](#string) |  |  |
| output | [string](#string) |  |  |
| stderr | [string](#string) |  |  |
| exit_code | [int32](#int32) |  |  |
| execution_time_seconds | [float](#float) |  |  |






<a name="g8e-operator-v1-DiskDetails"></a>

### DiskDetails



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| total_gb | [double](#double) |  |  |
| used_gb | [double](#double) |  |  |
| free_gb | [double](#double) |  |  |
| percent | [double](#double) |  |  |






<a name="g8e-operator-v1-EnvironmentDetails"></a>

### EnvironmentDetails



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| pwd | [string](#string) |  |  |
| lang | [string](#string) |  |  |
| timezone | [string](#string) |  |  |
| term | [string](#string) |  |  |
| is_container | [bool](#bool) |  |  |
| container_runtime | [string](#string) |  |  |
| container_signals | [string](#string) | repeated |  |
| init_system | [string](#string) |  |  |






<a name="g8e-operator-v1-EvalAnswerRequested"></a>

### EvalAnswerRequested



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| prompt_id | [string](#string) |  | Unique identifier for this prompt/task in the benchmark |
| benchmark | [string](#string) |  | Benchmark name (e.g., &#34;ifeval&#34;, &#34;simpleqa&#34;, &#34;gpqa&#34;) |
| answer | [string](#string) |  | The model&#39;s answer text (opaque to the Gateway) |
| model | [string](#string) |  | Model identifier (provider:model format, e.g., &#34;openai:gpt-4&#34;) |






<a name="g8e-operator-v1-ExecutionStatusUpdate"></a>

### ExecutionStatusUpdate



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| execution_id | [string](#string) |  |  |
| status | [ExecutionStatus](#g8e-operator-v1-ExecutionStatus) |  |  |
| command | [string](#string) |  |  |
| process_alive | [bool](#bool) |  |  |
| elapsed_seconds | [float](#float) |  |  |
| new_output | [string](#string) |  |  |
| new_stderr | [string](#string) |  |  |
| message | [string](#string) |  |  |






<a name="g8e-operator-v1-FetchFileDiffRequested"></a>

### FetchFileDiffRequested



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| execution_id | [string](#string) |  |  |
| diff_id | [string](#string) |  |  |
| operator_session_id | [string](#string) |  |  |
| file_path | [string](#string) |  |  |
| limit | [int32](#int32) |  |  |






<a name="g8e-operator-v1-FetchFileDiffResult"></a>

### FetchFileDiffResult



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| success | [bool](#bool) |  |  |
| execution_id | [string](#string) |  |  |
| diffs | [FileDiffEntry](#g8e-operator-v1-FileDiffEntry) | repeated |  |
| diff | [FileDiffEntry](#g8e-operator-v1-FileDiffEntry) |  |  |
| total | [int32](#int32) |  |  |
| operator_session_id | [string](#string) |  |  |
| error | [string](#string) |  |  |






<a name="g8e-operator-v1-FetchFileHistoryRequested"></a>

### FetchFileHistoryRequested



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| execution_id | [string](#string) |  |  |
| file_path | [string](#string) |  |  |
| limit | [int32](#int32) |  |  |
| operator_session_id | [string](#string) |  |  |






<a name="g8e-operator-v1-FetchFileHistoryResult"></a>

### FetchFileHistoryResult



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| success | [bool](#bool) |  |  |
| execution_id | [string](#string) |  |  |
| file_path | [string](#string) |  |  |
| history | [FileHistoryEntry](#g8e-operator-v1-FileHistoryEntry) | repeated |  |
| error | [string](#string) |  |  |






<a name="g8e-operator-v1-FetchHistoryRequested"></a>

### FetchHistoryRequested



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| execution_id | [string](#string) |  |  |
| operator_session_id | [string](#string) |  |  |
| limit | [int32](#int32) |  |  |
| offset | [int32](#int32) |  |  |
| include_commands | [bool](#bool) |  |  |
| include_file_mutations | [bool](#bool) |  |  |






<a name="g8e-operator-v1-FetchHistoryResult"></a>

### FetchHistoryResult



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| success | [bool](#bool) |  |  |
| execution_id | [string](#string) |  |  |
| operator_session_id | [string](#string) |  |  |
| web_session | [AuditWebSession](#g8e-operator-v1-AuditWebSession) |  |  |
| events | [AuditEvent](#g8e-operator-v1-AuditEvent) | repeated |  |
| total | [int32](#int32) |  |  |
| limit | [int32](#int32) |  |  |
| offset | [int32](#int32) |  |  |
| error | [string](#string) |  |  |






<a name="g8e-operator-v1-FetchLogsRequested"></a>

### FetchLogsRequested



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| execution_id | [string](#string) |  |  |
| vault_mode | [string](#string) |  |  |






<a name="g8e-operator-v1-FetchLogsResult"></a>

### FetchLogsResult



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| execution_id | [string](#string) |  |  |
| command | [string](#string) |  |  |
| return_code | [int32](#int32) |  |  |
| duration_ms | [int64](#int64) |  |  |
| stdout | [string](#string) |  |  |
| stderr | [string](#string) |  |  |
| stdout_size | [int32](#int32) |  |  |
| stderr_size | [int32](#int32) |  |  |
| timestamp | [string](#string) |  |  |
| vault_mode | [string](#string) |  |  |
| error | [string](#string) |  |  |






<a name="g8e-operator-v1-FileDiffEntry"></a>

### FileDiffEntry



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| id | [string](#string) |  |  |
| timestamp | [string](#string) |  |  |
| file_path | [string](#string) |  |  |
| operation | [string](#string) |  |  |
| ledger_hash_before | [string](#string) |  |  |
| ledger_hash_after | [string](#string) |  |  |
| diff_stat | [string](#string) |  |  |
| diff_content | [string](#string) |  |  |
| diff_size | [int32](#int32) |  |  |
| operator_session_id | [string](#string) |  |  |






<a name="g8e-operator-v1-FileEditRequested"></a>

### FileEditRequested



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| file_path | [string](#string) |  |  |
| operation | [string](#string) |  |  |
| execution_id | [string](#string) |  |  |
| justification | [string](#string) |  |  |
| content | [string](#string) |  |  |
| old_content | [string](#string) |  |  |
| new_content | [string](#string) |  |  |
| insert_content | [string](#string) |  |  |
| insert_position | [int32](#int32) |  |  |
| start_line | [int32](#int32) |  |  |
| end_line | [int32](#int32) |  |  |
| patch_content | [string](#string) |  |  |
| create_backup | [bool](#bool) |  |  |
| create_if_missing | [bool](#bool) |  |  |
| vault_mode | [string](#string) |  |  |






<a name="g8e-operator-v1-FileEditResult"></a>

### FileEditResult



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| execution_id | [string](#string) |  |  |
| status | [ExecutionStatus](#g8e-operator-v1-ExecutionStatus) |  |  |
| file_path | [string](#string) |  |  |
| operation | [string](#string) |  |  |
| duration_seconds | [float](#float) |  |  |
| bytes_written | [int64](#int64) |  |  |
| lines_changed | [int32](#int32) |  |  |
| backup_path | [string](#string) |  |  |
| error_message | [string](#string) |  |  |
| error_type | [string](#string) |  |  |
| content | [string](#string) |  |  |
| stdout_size | [int32](#int32) |  |  |
| stderr_size | [int32](#int32) |  |  |






<a name="g8e-operator-v1-FileHistoryEntry"></a>

### FileHistoryEntry



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| commit_hash | [string](#string) |  |  |
| timestamp | [string](#string) |  |  |
| message | [string](#string) |  |  |






<a name="g8e-operator-v1-FingerprintDetails"></a>

### FingerprintDetails



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| os | [string](#string) |  |  |
| architecture | [string](#string) |  |  |
| cpu_count | [int32](#int32) |  |  |
| machine_id | [string](#string) |  |  |






<a name="g8e-operator-v1-FsEntry"></a>

### FsEntry



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| name | [string](#string) |  |  |
| is_dir | [bool](#bool) |  |  |
| size | [int64](#int64) |  |  |
| mode | [int32](#int32) |  |  |
| mod_time | [int64](#int64) |  |  |






<a name="g8e-operator-v1-FsGrepMatch"></a>

### FsGrepMatch



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| path | [string](#string) |  |  |
| line_number | [int32](#int32) |  |  |
| content | [string](#string) |  |  |
| before | [string](#string) | repeated |  |
| after | [string](#string) | repeated |  |






<a name="g8e-operator-v1-FsGrepRequested"></a>

### FsGrepRequested



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| path | [string](#string) |  |  |
| execution_id | [string](#string) |  |  |
| pattern | [string](#string) |  |  |
| includes | [string](#string) | repeated |  |
| max_matches | [int32](#int32) |  |  |
| vault_mode | [string](#string) |  |  |






<a name="g8e-operator-v1-FsGrepResult"></a>

### FsGrepResult



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| execution_id | [string](#string) |  |  |
| status | [ExecutionStatus](#g8e-operator-v1-ExecutionStatus) |  |  |
| path | [string](#string) |  |  |
| matches | [FsGrepMatch](#g8e-operator-v1-FsGrepMatch) | repeated |  |
| total_matches | [int32](#int32) |  |  |
| truncated | [bool](#bool) |  |  |
| duration_seconds | [float](#float) |  |  |
| error_message | [string](#string) |  |  |
| error_type | [string](#string) |  |  |






<a name="g8e-operator-v1-FsListRequested"></a>

### FsListRequested



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| path | [string](#string) |  |  |
| execution_id | [string](#string) |  |  |
| max_depth | [int32](#int32) |  |  |
| max_entries | [int32](#int32) |  |  |
| vault_mode | [string](#string) |  |  |






<a name="g8e-operator-v1-FsListResult"></a>

### FsListResult



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| execution_id | [string](#string) |  |  |
| status | [ExecutionStatus](#g8e-operator-v1-ExecutionStatus) |  |  |
| path | [string](#string) |  |  |
| entries | [FsEntry](#g8e-operator-v1-FsEntry) | repeated |  |
| truncated | [bool](#bool) |  |  |
| total_count | [int32](#int32) |  |  |
| duration_seconds | [float](#float) |  |  |
| error_message | [string](#string) |  |  |
| error_type | [string](#string) |  |  |






<a name="g8e-operator-v1-FsReadRequested"></a>

### FsReadRequested



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| path | [string](#string) |  |  |
| execution_id | [string](#string) |  |  |
| max_size | [int32](#int32) |  |  |
| vault_mode | [string](#string) |  |  |






<a name="g8e-operator-v1-FsReadResult"></a>

### FsReadResult



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| execution_id | [string](#string) |  |  |
| status | [ExecutionStatus](#g8e-operator-v1-ExecutionStatus) |  |  |
| path | [string](#string) |  |  |
| content | [string](#string) |  |  |
| size_bytes | [int64](#int64) |  |  |
| truncated | [bool](#bool) |  |  |
| duration_seconds | [float](#float) |  |  |
| error_message | [string](#string) |  |  |
| error_type | [string](#string) |  |  |






<a name="g8e-operator-v1-GetRevocationBundleRequested"></a>

### GetRevocationBundleRequested







<a name="g8e-operator-v1-GetRevocationBundleResult"></a>

### GetRevocationBundleResult



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| success | [bool](#bool) |  |  |
| bundle_json | [string](#string) |  |  |
| signature | [string](#string) |  |  |
| error | [string](#string) |  |  |






<a name="g8e-operator-v1-HeartbeatRequested"></a>

### HeartbeatRequested
Empty message - just the event type matters






<a name="g8e-operator-v1-HeartbeatResult"></a>

### HeartbeatResult



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| operator_id | [string](#string) |  |  |
| operator_session_id | [string](#string) |  |  |
| timestamp | [string](#string) |  |  |
| status | [string](#string) |  |  |
| event_type | [string](#string) |  |  |
| source_component | [string](#string) |  |  |
| case_id | [string](#string) |  |  |
| investigation_id | [string](#string) |  |  |
| system_identity | [SystemIdentity](#g8e-operator-v1-SystemIdentity) |  |  |
| network_info | [NetworkInfo](#g8e-operator-v1-NetworkInfo) |  |  |
| version_info | [VersionInfo](#g8e-operator-v1-VersionInfo) |  |  |
| uptime_info | [UptimeInfo](#g8e-operator-v1-UptimeInfo) |  |  |
| performance_metrics | [PerformanceMetrics](#g8e-operator-v1-PerformanceMetrics) |  |  |
| os_details | [OSDetails](#g8e-operator-v1-OSDetails) |  |  |
| user_details | [UserDetails](#g8e-operator-v1-UserDetails) |  |  |
| disk_details | [DiskDetails](#g8e-operator-v1-DiskDetails) |  |  |
| memory_details | [MemoryDetails](#g8e-operator-v1-MemoryDetails) |  |  |
| environment | [EnvironmentDetails](#g8e-operator-v1-EnvironmentDetails) |  |  |
| capability_flags | [CapabilityFlags](#g8e-operator-v1-CapabilityFlags) |  |  |
| fingerprint_details | [FingerprintDetails](#g8e-operator-v1-FingerprintDetails) |  |  |
| system_fingerprint | [string](#string) |  |  |
| api_key | [string](#string) |  |  |






<a name="g8e-operator-v1-ListDeviceLinksRequested"></a>

### ListDeviceLinksRequested



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| user_id | [string](#string) |  |  |






<a name="g8e-operator-v1-ListDeviceLinksResult"></a>

### ListDeviceLinksResult



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| success | [bool](#bool) |  |  |
| links | [DeviceLink](#g8e-operator-v1-DeviceLink) | repeated |  |
| error | [string](#string) |  |  |






<a name="g8e-operator-v1-ListOperatorSlotsRequested"></a>

### ListOperatorSlotsRequested



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| user_id | [string](#string) |  |  |






<a name="g8e-operator-v1-ListOperatorSlotsResult"></a>

### ListOperatorSlotsResult



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| success | [bool](#bool) |  |  |
| operators | [OperatorDocument](#g8e-operator-v1-OperatorDocument) | repeated |  |
| error | [string](#string) |  |  |






<a name="g8e-operator-v1-ListPasskeyCredentialsRequested"></a>

### ListPasskeyCredentialsRequested



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| user_id | [string](#string) |  |  |






<a name="g8e-operator-v1-ListPasskeyCredentialsResult"></a>

### ListPasskeyCredentialsResult



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| success | [bool](#bool) |  |  |
| error | [string](#string) |  |  |
| credentials | [PasskeyCredential](#g8e-operator-v1-PasskeyCredential) | repeated |  |






<a name="g8e-operator-v1-McpCallRequested"></a>

### McpCallRequested
Payload for g8e.v1.operator.mcp.call.requested
Carries the downstream MCP tool name and its JSON-RPC arguments verbatim so
the Gateway can verify L1 forbidden patterns against the tool name before
the Warden dispatches the verified call to the configured downstream MCP
server.


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| tool_name | [string](#string) |  | The MCP tool name (e.g., &#34;fs_read&#34;, &#34;drop_table&#34;). Forbidden patterns here block the interlock sequence at L1 before any signature or state check. |
| arguments_json | [string](#string) |  | JSON-encoded arguments object exactly as the MCP client supplied it. Stored as a string (not Struct) so the canonical hash is computed over the bytes the client signed, with no normalization ambiguity. |
| execution_id | [string](#string) |  | Optional client-supplied invocation id surfaced back in the result. |






<a name="g8e-operator-v1-McpPromptGetRequested"></a>

### McpPromptGetRequested
Payload for g8e.v1.operator.mcp.prompts.get.requested
Gets a specific prompt template from the downstream MCP server.


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| name | [string](#string) |  | The prompt name/template identifier. |
| execution_id | [string](#string) |  | Optional client-supplied invocation id surfaced back in the result. |






<a name="g8e-operator-v1-McpPromptListRequested"></a>

### McpPromptListRequested
Payload for g8e.v1.operator.mcp.prompts.list.requested
Lists available prompts from the downstream MCP server.


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| execution_id | [string](#string) |  | Optional client-supplied invocation id surfaced back in the result. |






<a name="g8e-operator-v1-McpResourceListRequested"></a>

### McpResourceListRequested
Payload for g8e.v1.operator.mcp.resources.list.requested
Lists available resources from the downstream MCP server.


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| execution_id | [string](#string) |  | Optional client-supplied invocation id surfaced back in the result. |






<a name="g8e-operator-v1-McpResourceReadRequested"></a>

### McpResourceReadRequested
Payload for g8e.v1.operator.mcp.resources.read.requested
Reads a specific resource from the downstream MCP server.


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| uri | [string](#string) |  | The resource URI to read (e.g., &#34;file:///path/to/file&#34;, &#34;memory://var&#34;). |
| execution_id | [string](#string) |  | Optional client-supplied invocation id surfaced back in the result. |






<a name="g8e-operator-v1-MemoryDetails"></a>

### MemoryDetails



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| total_mb | [int64](#int64) |  |  |
| available_mb | [int64](#int64) |  |  |
| used_mb | [int64](#int64) |  |  |
| percent | [double](#double) |  |  |






<a name="g8e-operator-v1-NetworkInfo"></a>

### NetworkInfo



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| public_ip | [string](#string) |  |  |
| internal_ip | [string](#string) |  |  |
| interfaces | [string](#string) | repeated |  |
| connectivity_status | [NetworkInterface](#g8e-operator-v1-NetworkInterface) | repeated |  |






<a name="g8e-operator-v1-NetworkInterface"></a>

### NetworkInterface



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| name | [string](#string) |  |  |
| ip | [string](#string) |  |  |
| mtu | [int32](#int32) |  |  |






<a name="g8e-operator-v1-OSDetails"></a>

### OSDetails



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| kernel | [string](#string) |  |  |
| distro | [string](#string) |  |  |
| version | [string](#string) |  |  |






<a name="g8e-operator-v1-OperatorDocument"></a>

### OperatorDocument



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| id | [string](#string) |  |  |
| user_id | [string](#string) |  |  |
| organization_id | [string](#string) |  |  |
| component | [string](#string) |  |  |
| name | [string](#string) |  |  |
| status | [string](#string) |  |  |
| operator_session_id | [string](#string) |  |  |
| bound_web_session_id | [string](#string) |  |  |
| operator_cert | [string](#string) |  |  |
| operator_cert_serial | [string](#string) |  |  |
| slot_number | [int32](#int32) |  |  |
| is_slot | [bool](#bool) |  |  |
| claimed | [bool](#bool) |  |  |
| operator_type | [string](#string) |  |  |
| cloud_subtype | [string](#string) |  |  |
| system_fingerprint | [string](#string) |  |  |
| created_at_unix_ms | [int64](#int64) |  |  |
| updated_at_unix_ms | [int64](#int64) |  |  |






<a name="g8e-operator-v1-PasskeyAuthChallengeRequested"></a>

### PasskeyAuthChallengeRequested



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| user_id | [string](#string) |  |  |






<a name="g8e-operator-v1-PasskeyAuthChallengeResult"></a>

### PasskeyAuthChallengeResult



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| success | [bool](#bool) |  |  |
| error | [string](#string) |  |  |
| needs_setup | [bool](#bool) |  |  |
| challenge | [string](#string) |  | WebAuthn PublicKeyCredentialRequestOptions fields |
| timeout | [int64](#int64) |  |  |
| rp_id | [string](#string) |  |  |
| allow_credentials | [string](#string) | repeated |  |
| user_verification | [string](#string) |  |  |






<a name="g8e-operator-v1-PasskeyAuthVerifyRequested"></a>

### PasskeyAuthVerifyRequested



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| user_id | [string](#string) |  |  |
| assertion_response | [AssertionResponse](#g8e-operator-v1-AssertionResponse) |  |  |






<a name="g8e-operator-v1-PasskeyAuthVerifyResult"></a>

### PasskeyAuthVerifyResult



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| success | [bool](#bool) |  |  |
| error | [string](#string) |  |  |
| user_id | [string](#string) |  |  |
| web_session_id | [string](#string) |  |  |
| session_expires_at_unix_ms | [int64](#int64) |  |  |






<a name="g8e-operator-v1-PasskeyCredential"></a>

### PasskeyCredential
A stored passkey credential for a user


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| id | [string](#string) |  |  |
| public_key | [string](#string) |  | base64url-encoded public key |
| counter | [int64](#int64) |  |  |
| transports | [string](#string) | repeated |  |
| created_at_unix_ms | [int64](#int64) |  |  |
| last_used_at_unix_ms | [int64](#int64) |  |  |






<a name="g8e-operator-v1-PasskeyRegisterChallengeRequested"></a>

### PasskeyRegisterChallengeRequested



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| user_id | [string](#string) |  |  |
| user_name | [string](#string) |  |  |






<a name="g8e-operator-v1-PasskeyRegisterChallengeResult"></a>

### PasskeyRegisterChallengeResult



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| success | [bool](#bool) |  |  |
| error | [string](#string) |  |  |
| challenge | [string](#string) |  | WebAuthn PublicKeyCredentialCreationOptions fields |
| rp | [PasskeyRegisterChallengeResult.RelyingParty](#g8e-operator-v1-PasskeyRegisterChallengeResult-RelyingParty) |  |  |
| user | [PasskeyRegisterChallengeResult.UserInfo](#g8e-operator-v1-PasskeyRegisterChallengeResult-UserInfo) |  |  |
| pub_key_cred_params | [PasskeyRegisterChallengeResult.PublicKeyCredentialParameters](#g8e-operator-v1-PasskeyRegisterChallengeResult-PublicKeyCredentialParameters) | repeated |  |
| timeout | [int64](#int64) |  |  |
| attestation | [string](#string) |  |  |
| authenticator_selection | [PasskeyRegisterChallengeResult.AuthenticatorSelection](#g8e-operator-v1-PasskeyRegisterChallengeResult-AuthenticatorSelection) |  |  |
| exclude_credentials | [string](#string) | repeated |  |






<a name="g8e-operator-v1-PasskeyRegisterChallengeResult-AuthenticatorSelection"></a>

### PasskeyRegisterChallengeResult.AuthenticatorSelection



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| resident_key | [string](#string) |  |  |
| user_verification | [string](#string) |  |  |






<a name="g8e-operator-v1-PasskeyRegisterChallengeResult-PublicKeyCredentialParameters"></a>

### PasskeyRegisterChallengeResult.PublicKeyCredentialParameters



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| type | [string](#string) |  |  |
| alg | [int32](#int32) |  |  |






<a name="g8e-operator-v1-PasskeyRegisterChallengeResult-RelyingParty"></a>

### PasskeyRegisterChallengeResult.RelyingParty



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| name | [string](#string) |  |  |
| id | [string](#string) |  |  |






<a name="g8e-operator-v1-PasskeyRegisterChallengeResult-UserInfo"></a>

### PasskeyRegisterChallengeResult.UserInfo



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| id | [string](#string) |  |  |
| name | [string](#string) |  |  |
| display_name | [string](#string) |  |  |






<a name="g8e-operator-v1-PasskeyRegisterVerifyRequested"></a>

### PasskeyRegisterVerifyRequested



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| user_id | [string](#string) |  |  |
| attestation_response | [AttestationResponse](#g8e-operator-v1-AttestationResponse) |  |  |






<a name="g8e-operator-v1-PasskeyRegisterVerifyResult"></a>

### PasskeyRegisterVerifyResult



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| success | [bool](#bool) |  |  |
| error | [string](#string) |  |  |
| credential | [PasskeyCredential](#g8e-operator-v1-PasskeyCredential) |  |  |






<a name="g8e-operator-v1-PerformanceMetrics"></a>

### PerformanceMetrics



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| cpu_percent | [double](#double) |  |  |
| memory_percent | [double](#double) |  |  |
| disk_percent | [double](#double) |  |  |
| network_latency | [double](#double) |  |  |
| memory_used_mb | [int32](#int32) |  |  |
| memory_total_mb | [int32](#int32) |  |  |
| disk_used_gb | [double](#double) |  |  |
| disk_total_gb | [double](#double) |  |  |






<a name="g8e-operator-v1-PortCheckEntry"></a>

### PortCheckEntry



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| host | [string](#string) |  |  |
| port | [int32](#int32) |  |  |
| open | [bool](#bool) |  |  |
| latency_ms | [float](#float) |  |  |
| error | [string](#string) |  |  |






<a name="g8e-operator-v1-PortCheckResult"></a>

### PortCheckResult



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| execution_id | [string](#string) |  |  |
| status | [ExecutionStatus](#g8e-operator-v1-ExecutionStatus) |  |  |
| results | [PortCheckEntry](#g8e-operator-v1-PortCheckEntry) | repeated |  |
| error_message | [string](#string) |  |  |
| error_type | [string](#string) |  |  |






<a name="g8e-operator-v1-RestoreFileRequested"></a>

### RestoreFileRequested



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| execution_id | [string](#string) |  |  |
| file_path | [string](#string) |  |  |
| commit_hash | [string](#string) |  |  |
| operator_session_id | [string](#string) |  |  |






<a name="g8e-operator-v1-RestoreFileResult"></a>

### RestoreFileResult



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| success | [bool](#bool) |  |  |
| execution_id | [string](#string) |  |  |
| file_path | [string](#string) |  |  |
| commit_hash | [string](#string) |  |  |
| error | [string](#string) |  |  |






<a name="g8e-operator-v1-RevokeCertificateRequested"></a>

### RevokeCertificateRequested



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| serial | [string](#string) |  |  |
| reason | [string](#string) |  |  |






<a name="g8e-operator-v1-RevokeCertificateResult"></a>

### RevokeCertificateResult



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| success | [bool](#bool) |  |  |
| error | [string](#string) |  |  |






<a name="g8e-operator-v1-RevokePasskeyCredentialRequested"></a>

### RevokePasskeyCredentialRequested



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| user_id | [string](#string) |  |  |
| credential_id | [string](#string) |  |  |






<a name="g8e-operator-v1-RevokePasskeyCredentialResult"></a>

### RevokePasskeyCredentialResult



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| success | [bool](#bool) |  |  |
| error | [string](#string) |  |  |
| found | [bool](#bool) |  |  |
| remaining | [int32](#int32) |  |  |






<a name="g8e-operator-v1-SetTargetContextRequested"></a>

### SetTargetContextRequested



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| operator_id | [string](#string) |  |  |
| user_id | [string](#string) |  |  |
| web_session_id | [string](#string) |  |  |






<a name="g8e-operator-v1-SetTargetContextResult"></a>

### SetTargetContextResult



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| success | [bool](#bool) |  |  |
| operator_id | [string](#string) |  |  |
| error | [string](#string) |  |  |






<a name="g8e-operator-v1-ShutdownRequested"></a>

### ShutdownRequested



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| reason | [string](#string) |  |  |






<a name="g8e-operator-v1-SignCertificateRequested"></a>

### SignCertificateRequested



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| public_key_pem | [string](#string) |  |  |
| common_name | [string](#string) |  |  |
| organizational_unit | [string](#string) |  |  |
| validity_days | [int32](#int32) |  |  |






<a name="g8e-operator-v1-SignCertificateResult"></a>

### SignCertificateResult



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| success | [bool](#bool) |  |  |
| certificate_pem | [string](#string) |  |  |
| serial | [string](#string) |  |  |
| error | [string](#string) |  |  |






<a name="g8e-operator-v1-SystemIdentity"></a>

### SystemIdentity



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| hostname | [string](#string) |  |  |
| os | [string](#string) |  |  |
| architecture | [string](#string) |  |  |
| pwd | [string](#string) |  |  |
| current_user | [string](#string) |  |  |
| cpu_count | [int32](#int32) |  |  |
| memory_mb | [int32](#int32) |  |  |






<a name="g8e-operator-v1-TerminateOperatorRequested"></a>

### TerminateOperatorRequested



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| operator_id | [string](#string) |  |  |
| user_id | [string](#string) |  |  |
| reason | [string](#string) |  |  |






<a name="g8e-operator-v1-TerminateOperatorResult"></a>

### TerminateOperatorResult



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| success | [bool](#bool) |  |  |
| message | [string](#string) |  |  |
| error | [string](#string) |  |  |






<a name="g8e-operator-v1-UnbindOperatorsRequested"></a>

### UnbindOperatorsRequested



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| operator_ids | [string](#string) | repeated |  |
| user_id | [string](#string) |  |  |
| web_session_id | [string](#string) |  |  |






<a name="g8e-operator-v1-UnbindOperatorsResult"></a>

### UnbindOperatorsResult



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| success | [bool](#bool) |  |  |
| unbound_count | [int32](#int32) |  |  |
| failed_count | [int32](#int32) |  |  |
| unbound_operator_ids | [string](#string) | repeated |  |
| failed_operator_ids | [string](#string) | repeated |  |
| error | [string](#string) |  |  |






<a name="g8e-operator-v1-UptimeInfo"></a>

### UptimeInfo



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| uptime | [string](#string) |  |  |
| uptime_seconds | [int64](#int64) |  |  |






<a name="g8e-operator-v1-UserDetails"></a>

### UserDetails



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| username | [string](#string) |  |  |
| uid | [int32](#int32) |  |  |
| gid | [int32](#int32) |  |  |
| home | [string](#string) |  |  |
| name | [string](#string) |  |  |
| shell | [string](#string) |  |  |






<a name="g8e-operator-v1-VersionInfo"></a>

### VersionInfo



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| operator_version | [string](#string) |  |  |
| status | [string](#string) |  |  |





 


<a name="g8e-operator-v1-ExecutionStatus"></a>

### ExecutionStatus
Execution status enum for command and operation results

| Name | Number | Description |
| ---- | ------ | ----------- |
| EXECUTION_STATUS_UNSPECIFIED | 0 |  |
| EXECUTION_STATUS_EXECUTING | 1 |  |
| EXECUTION_STATUS_COMPLETED | 2 |  |
| EXECUTION_STATUS_FAILED | 3 |  |
| EXECUTION_STATUS_CANCELLED | 4 |  |
| EXECUTION_STATUS_TIMEOUT | 5 |  |



<a name="g8e-operator-v1-HeartbeatType"></a>

### HeartbeatType
Heartbeat type enum

| Name | Number | Description |
| ---- | ------ | ----------- |
| HEARTBEAT_TYPE_UNSPECIFIED | 0 |  |
| HEARTBEAT_TYPE_AUTOMATIC | 1 |  |
| HEARTBEAT_TYPE_MANUAL | 2 |  |



<a name="g8e-operator-v1-L2Status"></a>

### L2Status
L2 validation status enum for Consensus (L2Consensus) signature verification
Distinguishes between &#34;not required&#34; vs &#34;required but failed&#34; for compliance

| Name | Number | Description |
| ---- | ------ | ----------- |
| L2_STATUS_UNSPECIFIED | 0 |  |
| L2_STATUS_NOT_REQUIRED | 1 | L2 signature not required by posture (doctrine) |
| L2_STATUS_REQUIRED_VALID | 2 | L2 signature required and valid |
| L2_STATUS_REQUIRED_FAILED | 3 | L2 signature required but missing or invalid |



<a name="g8e-operator-v1-L3Status"></a>

### L3Status
L3 validation status enum for Notary (L3Notary) proof verification
Distinguishes between &#34;not required&#34; vs &#34;required but failed&#34; for compliance

| Name | Number | Description |
| ---- | ------ | ----------- |
| L3_STATUS_UNSPECIFIED | 0 |  |
| L3_STATUS_NOT_REQUIRED | 1 | L3 proof not required by posture (doctrine/consensus) |
| L3_STATUS_REQUIRED_VALID | 2 | L3 proof required and valid |
| L3_STATUS_REQUIRED_FAILED | 3 | L3 proof required but missing or invalid |


 

 


<a name="g8e-operator-v1-OperatorService"></a>

### OperatorService


| Method Name | Request Type | Response Type | Description |
| ----------- | ------------ | ------------- | ------------|
| ExecuteCommand | [CommandRequested](#g8e-operator-v1-CommandRequested) | [CommandResult](#g8e-operator-v1-CommandResult) | Execute a shell command |
| CancelCommand | [CommandCancelRequested](#g8e-operator-v1-CommandCancelRequested) | [CommandResult](#g8e-operator-v1-CommandResult) | Cancel a running command |
| EditFile | [FileEditRequested](#g8e-operator-v1-FileEditRequested) | [CommandResult](#g8e-operator-v1-CommandResult) | Edit a file |
| ListFileSystem | [FsListRequested](#g8e-operator-v1-FsListRequested) | [CommandResult](#g8e-operator-v1-CommandResult) | List directory contents |
| ReadFileSystem | [FsReadRequested](#g8e-operator-v1-FsReadRequested) | [CommandResult](#g8e-operator-v1-CommandResult) | Read file contents |

 



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

