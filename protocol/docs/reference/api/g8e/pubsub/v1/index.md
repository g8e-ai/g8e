# Protocol Documentation
<a name="top"></a>

## Table of Contents

- [g8e/pubsub/v1/pubsub.proto](#g8e_pubsub_v1_pubsub-proto)
    - [PubSubEvent](#g8e-pubsub-v1-PubSubEvent)
    - [PubSubMessage](#g8e-pubsub-v1-PubSubMessage)
  
- [Scalar Value Types](#scalar-value-types)



<a name="g8e_pubsub_v1_pubsub-proto"></a>
<p align="right"><a href="#top">Top</a></p>

## g8e/pubsub/v1/pubsub.proto



<a name="g8e-pubsub-v1-PubSubEvent"></a>

### PubSubEvent
PubSubEvent is the server-to-client WebSocket frame delivered to subscribers.
The broker emits events for message delivery, subscription acknowledgments, and errors.


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| type | [string](#string) |  | Event type: &#34;message&#34;, &#34;pmessage&#34;, &#34;subscribed&#34;, &#34;unsubscribed&#34;, or &#34;error&#34;. |
| channel | [string](#string) |  | Channel the event pertains to. |
| pattern | [string](#string) |  | Pattern matched for pmessage events. Empty for direct channel deliveries. |
| data | [bytes](#bytes) |  | Message payload for message/pmessage events. Empty for acknowledgment events. |






<a name="g8e-pubsub-v1-PubSubMessage"></a>

### PubSubMessage
PubSubMessage is the client-to-server WebSocket frame for pub/sub operations.
Clients send this message to subscribe, unsubscribe, or publish on a channel.


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| action | [string](#string) |  | Operation to perform: &#34;subscribe&#34;, &#34;psubscribe&#34;, &#34;unsubscribe&#34;, or &#34;publish&#34;. |
| channel | [string](#string) |  | Target channel name. Follows the convention &#34;{prefix}:{operator_id}:{session_id}&#34;. |
| data | [bytes](#bytes) |  | Payload bytes for publish actions. Empty for subscribe/unsubscribe. |





 

 

 

 



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

