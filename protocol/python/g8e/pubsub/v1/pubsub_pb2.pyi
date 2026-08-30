from google.protobuf import descriptor as _descriptor
from google.protobuf import message as _message
from typing import ClassVar as _ClassVar, Optional as _Optional

DESCRIPTOR: _descriptor.FileDescriptor

class PubSubMessage(_message.Message):
    __slots__ = ("action", "channel", "data")
    ACTION_FIELD_NUMBER: _ClassVar[int]
    CHANNEL_FIELD_NUMBER: _ClassVar[int]
    DATA_FIELD_NUMBER: _ClassVar[int]
    action: str
    channel: str
    data: bytes
    def __init__(self, action: _Optional[str] = ..., channel: _Optional[str] = ..., data: _Optional[bytes] = ...) -> None: ...

class PubSubEvent(_message.Message):
    __slots__ = ("type", "channel", "pattern", "data")
    TYPE_FIELD_NUMBER: _ClassVar[int]
    CHANNEL_FIELD_NUMBER: _ClassVar[int]
    PATTERN_FIELD_NUMBER: _ClassVar[int]
    DATA_FIELD_NUMBER: _ClassVar[int]
    type: str
    channel: str
    pattern: str
    data: bytes
    def __init__(self, type: _Optional[str] = ..., channel: _Optional[str] = ..., pattern: _Optional[str] = ..., data: _Optional[bytes] = ...) -> None: ...
