# Protocol Documentation
<a name="top"></a>

## Table of Contents

- [g8e/pubsub/v1/pubsub.proto](#g8e_pubsub_v1_pubsub-proto)
    - [PubSubEvent](#g8e-pubsub-v1-PubSubEvent)
    - [PubSubMessage](#g8e-pubsub-v1-PubSubMessage)
- [Channel Conventions](#channel-conventions)
- [Scalar Value Types](#scalar-value-types)



<a name="g8e_pubsub_v1_pubsub-proto"></a>
<p align="right"><a href="#top">Top</a></p>

## g8e/pubsub/v1/pubsub.proto



<a name="g8e-pubsub-v1-PubSubEvent"></a>

### PubSubEvent

PubSubEvent represents a message or acknowledgment sent from the gateway to a connected client.

| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| type | [string](#string) |  | The event type. Supported values include `message`, `pmessage`, and `subscribed`. |
| channel | [string](#string) |  | The specific channel name associated with this event. |
| pattern | [string](#string) |  | The subscription pattern that matched this event (used with `pmessage`). |
| data | [bytes](#bytes) |  | The raw payload data associated with the event. |

<a name="g8e-pubsub-v1-PubSubMessage"></a>

### PubSubMessage

PubSubMessage represents an action or publication request sent from a client to the gateway.

| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| action | [string](#string) |  | The requested action. Supported values include `subscribe`, `psubscribe`, `unsubscribe`, and `publish`. |
| channel | [string](#string) |  | The target channel name for the action. |
| data | [bytes](#bytes) |  | The raw payload data to be published (used with the `publish` action). |

## Channel Conventions

The platform employs a structured channel naming convention to ensure secure routing and isolation. Channels are typically formatted as `{prefix}:{operator_id}:{session_id}`.

### Standard Channels

- **cmd**: Transmits execution requests from the gateway to an operator.
- **results**: Transmits execution receipts and logs from an operator back to the gateway.
- **heartbeat**: Transmits periodic status updates from an operator to maintain connection state.
- **governance**: Transmits signed governance envelopes for L4 verification.
- **sse_event**: Pushes events to Server-Sent Events (SSE) subscribers.

### Storage Channels

- **storage_document**: Manages document-oriented storage operations.
- **storage_kv**: Manages key-value pair storage operations.
- **storage_blob**: Manages binary large object storage operations.





 

 

 

 



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

