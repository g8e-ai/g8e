// source: operator.proto
/**
 * @fileoverview
 * @enhanceable
 * @suppress {missingRequire} reports error on implicit type usages.
 * @suppress {messageConventions} JS Compiler reports an error if a variable or
 *     field starts with 'MSG_' and isn't a translatable message.
 * @public
 */
// GENERATED CODE -- DO NOT EDIT!
/* eslint-disable */
// @ts-nocheck

var jspb = require('google-protobuf');
var goog = jspb;
var global =
    (typeof globalThis !== 'undefined' && globalThis) ||
    (typeof window !== 'undefined' && window) ||
    (typeof global !== 'undefined' && global) ||
    (typeof self !== 'undefined' && self) ||
    (function () { return this; }).call(null) ||
    Function('return this')();

var common_pb = require('./common_pb.js');
goog.object.extend(proto, common_pb);
goog.exportSymbol('proto.g8e.operator.v1.AssertionResponse', null, global);
goog.exportSymbol('proto.g8e.operator.v1.AttestationResponse', null, global);
goog.exportSymbol('proto.g8e.operator.v1.AuditEvent', null, global);
goog.exportSymbol('proto.g8e.operator.v1.AuditFileMutation', null, global);
goog.exportSymbol('proto.g8e.operator.v1.AuditMsgRequested', null, global);
goog.exportSymbol('proto.g8e.operator.v1.AuditWebSession', null, global);
goog.exportSymbol('proto.g8e.operator.v1.BindOperatorsRequested', null, global);
goog.exportSymbol('proto.g8e.operator.v1.BindOperatorsResult', null, global);
goog.exportSymbol('proto.g8e.operator.v1.CapabilityFlags', null, global);
goog.exportSymbol('proto.g8e.operator.v1.CheckPortRequested', null, global);
goog.exportSymbol('proto.g8e.operator.v1.CommandCancelRequested', null, global);
goog.exportSymbol('proto.g8e.operator.v1.CommandRequested', null, global);
goog.exportSymbol('proto.g8e.operator.v1.CommandResult', null, global);
goog.exportSymbol('proto.g8e.operator.v1.CreateDeviceLinkRequested', null, global);
goog.exportSymbol('proto.g8e.operator.v1.DeleteDeviceLinkRequested', null, global);
goog.exportSymbol('proto.g8e.operator.v1.DeviceLink', null, global);
goog.exportSymbol('proto.g8e.operator.v1.DeviceLinkResult', null, global);
goog.exportSymbol('proto.g8e.operator.v1.DirectCommandAuditRequested', null, global);
goog.exportSymbol('proto.g8e.operator.v1.DirectCommandResultAuditRequested', null, global);
goog.exportSymbol('proto.g8e.operator.v1.DiskDetails', null, global);
goog.exportSymbol('proto.g8e.operator.v1.EnvironmentDetails', null, global);
goog.exportSymbol('proto.g8e.operator.v1.ExecutionStatus', null, global);
goog.exportSymbol('proto.g8e.operator.v1.ExecutionStatusUpdate', null, global);
goog.exportSymbol('proto.g8e.operator.v1.FetchFileDiffRequested', null, global);
goog.exportSymbol('proto.g8e.operator.v1.FetchFileDiffResult', null, global);
goog.exportSymbol('proto.g8e.operator.v1.FetchFileHistoryRequested', null, global);
goog.exportSymbol('proto.g8e.operator.v1.FetchFileHistoryResult', null, global);
goog.exportSymbol('proto.g8e.operator.v1.FetchHistoryRequested', null, global);
goog.exportSymbol('proto.g8e.operator.v1.FetchHistoryResult', null, global);
goog.exportSymbol('proto.g8e.operator.v1.FetchLogsRequested', null, global);
goog.exportSymbol('proto.g8e.operator.v1.FetchLogsResult', null, global);
goog.exportSymbol('proto.g8e.operator.v1.FileDiffEntry', null, global);
goog.exportSymbol('proto.g8e.operator.v1.FileEditRequested', null, global);
goog.exportSymbol('proto.g8e.operator.v1.FileEditResult', null, global);
goog.exportSymbol('proto.g8e.operator.v1.FileHistoryEntry', null, global);
goog.exportSymbol('proto.g8e.operator.v1.FingerprintDetails', null, global);
goog.exportSymbol('proto.g8e.operator.v1.FsEntry', null, global);
goog.exportSymbol('proto.g8e.operator.v1.FsGrepMatch', null, global);
goog.exportSymbol('proto.g8e.operator.v1.FsGrepRequested', null, global);
goog.exportSymbol('proto.g8e.operator.v1.FsGrepResult', null, global);
goog.exportSymbol('proto.g8e.operator.v1.FsListRequested', null, global);
goog.exportSymbol('proto.g8e.operator.v1.FsListResult', null, global);
goog.exportSymbol('proto.g8e.operator.v1.FsReadRequested', null, global);
goog.exportSymbol('proto.g8e.operator.v1.FsReadResult', null, global);
goog.exportSymbol('proto.g8e.operator.v1.HeartbeatRequested', null, global);
goog.exportSymbol('proto.g8e.operator.v1.HeartbeatResult', null, global);
goog.exportSymbol('proto.g8e.operator.v1.HeartbeatType', null, global);
goog.exportSymbol('proto.g8e.operator.v1.ListDeviceLinksRequested', null, global);
goog.exportSymbol('proto.g8e.operator.v1.ListDeviceLinksResult', null, global);
goog.exportSymbol('proto.g8e.operator.v1.ListOperatorSlotsRequested', null, global);
goog.exportSymbol('proto.g8e.operator.v1.ListOperatorSlotsResult', null, global);
goog.exportSymbol('proto.g8e.operator.v1.ListPasskeyCredentialsRequested', null, global);
goog.exportSymbol('proto.g8e.operator.v1.ListPasskeyCredentialsResult', null, global);
goog.exportSymbol('proto.g8e.operator.v1.MemoryDetails', null, global);
goog.exportSymbol('proto.g8e.operator.v1.NetworkInfo', null, global);
goog.exportSymbol('proto.g8e.operator.v1.NetworkInterface', null, global);
goog.exportSymbol('proto.g8e.operator.v1.OSDetails', null, global);
goog.exportSymbol('proto.g8e.operator.v1.OperatorDocument', null, global);
goog.exportSymbol('proto.g8e.operator.v1.PasskeyAuthChallengeRequested', null, global);
goog.exportSymbol('proto.g8e.operator.v1.PasskeyAuthChallengeResult', null, global);
goog.exportSymbol('proto.g8e.operator.v1.PasskeyAuthVerifyRequested', null, global);
goog.exportSymbol('proto.g8e.operator.v1.PasskeyAuthVerifyResult', null, global);
goog.exportSymbol('proto.g8e.operator.v1.PasskeyCredential', null, global);
goog.exportSymbol('proto.g8e.operator.v1.PasskeyRegisterChallengeRequested', null, global);
goog.exportSymbol('proto.g8e.operator.v1.PasskeyRegisterChallengeResult', null, global);
goog.exportSymbol('proto.g8e.operator.v1.PasskeyRegisterChallengeResult.AuthenticatorSelection', null, global);
goog.exportSymbol('proto.g8e.operator.v1.PasskeyRegisterChallengeResult.PublicKeyCredentialParameters', null, global);
goog.exportSymbol('proto.g8e.operator.v1.PasskeyRegisterChallengeResult.RelyingParty', null, global);
goog.exportSymbol('proto.g8e.operator.v1.PasskeyRegisterChallengeResult.UserInfo', null, global);
goog.exportSymbol('proto.g8e.operator.v1.PasskeyRegisterVerifyRequested', null, global);
goog.exportSymbol('proto.g8e.operator.v1.PasskeyRegisterVerifyResult', null, global);
goog.exportSymbol('proto.g8e.operator.v1.PerformanceMetrics', null, global);
goog.exportSymbol('proto.g8e.operator.v1.PortCheckEntry', null, global);
goog.exportSymbol('proto.g8e.operator.v1.PortCheckResult', null, global);
goog.exportSymbol('proto.g8e.operator.v1.RestoreFileRequested', null, global);
goog.exportSymbol('proto.g8e.operator.v1.RestoreFileResult', null, global);
goog.exportSymbol('proto.g8e.operator.v1.RevokePasskeyCredentialRequested', null, global);
goog.exportSymbol('proto.g8e.operator.v1.RevokePasskeyCredentialResult', null, global);
goog.exportSymbol('proto.g8e.operator.v1.RotateAPIKeyRequested', null, global);
goog.exportSymbol('proto.g8e.operator.v1.RotateAPIKeyResult', null, global);
goog.exportSymbol('proto.g8e.operator.v1.SetTargetContextRequested', null, global);
goog.exportSymbol('proto.g8e.operator.v1.SetTargetContextResult', null, global);
goog.exportSymbol('proto.g8e.operator.v1.ShutdownRequested', null, global);
goog.exportSymbol('proto.g8e.operator.v1.SignCertificateRequested', null, global);
goog.exportSymbol('proto.g8e.operator.v1.SignCertificateResult', null, global);
goog.exportSymbol('proto.g8e.operator.v1.SystemIdentity', null, global);
goog.exportSymbol('proto.g8e.operator.v1.TerminateOperatorRequested', null, global);
goog.exportSymbol('proto.g8e.operator.v1.TerminateOperatorResult', null, global);
goog.exportSymbol('proto.g8e.operator.v1.UnbindOperatorsRequested', null, global);
goog.exportSymbol('proto.g8e.operator.v1.UnbindOperatorsResult', null, global);
goog.exportSymbol('proto.g8e.operator.v1.UptimeInfo', null, global);
goog.exportSymbol('proto.g8e.operator.v1.UserDetails', null, global);
goog.exportSymbol('proto.g8e.operator.v1.VersionInfo', null, global);
/**
 * Generated by JsPbCodeGenerator.
 * @param {Array=} opt_data Optional initial data array, typically from a
 * server response, or constructed directly in Javascript. The array is used
 * in place and becomes part of the constructed object. It is not cloned.
 * If no data is provided, the constructed object will be empty, but still
 * valid.
 * @extends {jspb.Message}
 * @constructor
 */
proto.g8e.operator.v1.CommandRequested = function(opt_data) {
  jspb.Message.initialize(this, opt_data, 0, -1, null, null);
};
goog.inherits(proto.g8e.operator.v1.CommandRequested, jspb.Message);
if (goog.DEBUG && !COMPILED) {
  /**
   * @public
   * @override
   */
  proto.g8e.operator.v1.CommandRequested.displayName = 'proto.g8e.operator.v1.CommandRequested';
}
/**
 * Generated by JsPbCodeGenerator.
 * @param {Array=} opt_data Optional initial data array, typically from a
 * server response, or constructed directly in Javascript. The array is used
 * in place and becomes part of the constructed object. It is not cloned.
 * If no data is provided, the constructed object will be empty, but still
 * valid.
 * @extends {jspb.Message}
 * @constructor
 */
proto.g8e.operator.v1.CommandCancelRequested = function(opt_data) {
  jspb.Message.initialize(this, opt_data, 0, -1, null, null);
};
goog.inherits(proto.g8e.operator.v1.CommandCancelRequested, jspb.Message);
if (goog.DEBUG && !COMPILED) {
  /**
   * @public
   * @override
   */
  proto.g8e.operator.v1.CommandCancelRequested.displayName = 'proto.g8e.operator.v1.CommandCancelRequested';
}
/**
 * Generated by JsPbCodeGenerator.
 * @param {Array=} opt_data Optional initial data array, typically from a
 * server response, or constructed directly in Javascript. The array is used
 * in place and becomes part of the constructed object. It is not cloned.
 * If no data is provided, the constructed object will be empty, but still
 * valid.
 * @extends {jspb.Message}
 * @constructor
 */
proto.g8e.operator.v1.FileEditRequested = function(opt_data) {
  jspb.Message.initialize(this, opt_data, 0, -1, null, null);
};
goog.inherits(proto.g8e.operator.v1.FileEditRequested, jspb.Message);
if (goog.DEBUG && !COMPILED) {
  /**
   * @public
   * @override
   */
  proto.g8e.operator.v1.FileEditRequested.displayName = 'proto.g8e.operator.v1.FileEditRequested';
}
/**
 * Generated by JsPbCodeGenerator.
 * @param {Array=} opt_data Optional initial data array, typically from a
 * server response, or constructed directly in Javascript. The array is used
 * in place and becomes part of the constructed object. It is not cloned.
 * If no data is provided, the constructed object will be empty, but still
 * valid.
 * @extends {jspb.Message}
 * @constructor
 */
proto.g8e.operator.v1.FsListRequested = function(opt_data) {
  jspb.Message.initialize(this, opt_data, 0, -1, null, null);
};
goog.inherits(proto.g8e.operator.v1.FsListRequested, jspb.Message);
if (goog.DEBUG && !COMPILED) {
  /**
   * @public
   * @override
   */
  proto.g8e.operator.v1.FsListRequested.displayName = 'proto.g8e.operator.v1.FsListRequested';
}
/**
 * Generated by JsPbCodeGenerator.
 * @param {Array=} opt_data Optional initial data array, typically from a
 * server response, or constructed directly in Javascript. The array is used
 * in place and becomes part of the constructed object. It is not cloned.
 * If no data is provided, the constructed object will be empty, but still
 * valid.
 * @extends {jspb.Message}
 * @constructor
 */
proto.g8e.operator.v1.FsReadRequested = function(opt_data) {
  jspb.Message.initialize(this, opt_data, 0, -1, null, null);
};
goog.inherits(proto.g8e.operator.v1.FsReadRequested, jspb.Message);
if (goog.DEBUG && !COMPILED) {
  /**
   * @public
   * @override
   */
  proto.g8e.operator.v1.FsReadRequested.displayName = 'proto.g8e.operator.v1.FsReadRequested';
}
/**
 * Generated by JsPbCodeGenerator.
 * @param {Array=} opt_data Optional initial data array, typically from a
 * server response, or constructed directly in Javascript. The array is used
 * in place and becomes part of the constructed object. It is not cloned.
 * If no data is provided, the constructed object will be empty, but still
 * valid.
 * @extends {jspb.Message}
 * @constructor
 */
proto.g8e.operator.v1.HeartbeatRequested = function(opt_data) {
  jspb.Message.initialize(this, opt_data, 0, -1, null, null);
};
goog.inherits(proto.g8e.operator.v1.HeartbeatRequested, jspb.Message);
if (goog.DEBUG && !COMPILED) {
  /**
   * @public
   * @override
   */
  proto.g8e.operator.v1.HeartbeatRequested.displayName = 'proto.g8e.operator.v1.HeartbeatRequested';
}
/**
 * Generated by JsPbCodeGenerator.
 * @param {Array=} opt_data Optional initial data array, typically from a
 * server response, or constructed directly in Javascript. The array is used
 * in place and becomes part of the constructed object. It is not cloned.
 * If no data is provided, the constructed object will be empty, but still
 * valid.
 * @extends {jspb.Message}
 * @constructor
 */
proto.g8e.operator.v1.FsGrepRequested = function(opt_data) {
  jspb.Message.initialize(this, opt_data, 0, -1, proto.g8e.operator.v1.FsGrepRequested.repeatedFields_, null);
};
goog.inherits(proto.g8e.operator.v1.FsGrepRequested, jspb.Message);
if (goog.DEBUG && !COMPILED) {
  /**
   * @public
   * @override
   */
  proto.g8e.operator.v1.FsGrepRequested.displayName = 'proto.g8e.operator.v1.FsGrepRequested';
}
/**
 * Generated by JsPbCodeGenerator.
 * @param {Array=} opt_data Optional initial data array, typically from a
 * server response, or constructed directly in Javascript. The array is used
 * in place and becomes part of the constructed object. It is not cloned.
 * If no data is provided, the constructed object will be empty, but still
 * valid.
 * @extends {jspb.Message}
 * @constructor
 */
proto.g8e.operator.v1.CheckPortRequested = function(opt_data) {
  jspb.Message.initialize(this, opt_data, 0, -1, null, null);
};
goog.inherits(proto.g8e.operator.v1.CheckPortRequested, jspb.Message);
if (goog.DEBUG && !COMPILED) {
  /**
   * @public
   * @override
   */
  proto.g8e.operator.v1.CheckPortRequested.displayName = 'proto.g8e.operator.v1.CheckPortRequested';
}
/**
 * Generated by JsPbCodeGenerator.
 * @param {Array=} opt_data Optional initial data array, typically from a
 * server response, or constructed directly in Javascript. The array is used
 * in place and becomes part of the constructed object. It is not cloned.
 * If no data is provided, the constructed object will be empty, but still
 * valid.
 * @extends {jspb.Message}
 * @constructor
 */
proto.g8e.operator.v1.FetchLogsRequested = function(opt_data) {
  jspb.Message.initialize(this, opt_data, 0, -1, null, null);
};
goog.inherits(proto.g8e.operator.v1.FetchLogsRequested, jspb.Message);
if (goog.DEBUG && !COMPILED) {
  /**
   * @public
   * @override
   */
  proto.g8e.operator.v1.FetchLogsRequested.displayName = 'proto.g8e.operator.v1.FetchLogsRequested';
}
/**
 * Generated by JsPbCodeGenerator.
 * @param {Array=} opt_data Optional initial data array, typically from a
 * server response, or constructed directly in Javascript. The array is used
 * in place and becomes part of the constructed object. It is not cloned.
 * If no data is provided, the constructed object will be empty, but still
 * valid.
 * @extends {jspb.Message}
 * @constructor
 */
proto.g8e.operator.v1.FetchHistoryRequested = function(opt_data) {
  jspb.Message.initialize(this, opt_data, 0, -1, null, null);
};
goog.inherits(proto.g8e.operator.v1.FetchHistoryRequested, jspb.Message);
if (goog.DEBUG && !COMPILED) {
  /**
   * @public
   * @override
   */
  proto.g8e.operator.v1.FetchHistoryRequested.displayName = 'proto.g8e.operator.v1.FetchHistoryRequested';
}
/**
 * Generated by JsPbCodeGenerator.
 * @param {Array=} opt_data Optional initial data array, typically from a
 * server response, or constructed directly in Javascript. The array is used
 * in place and becomes part of the constructed object. It is not cloned.
 * If no data is provided, the constructed object will be empty, but still
 * valid.
 * @extends {jspb.Message}
 * @constructor
 */
proto.g8e.operator.v1.FetchFileHistoryRequested = function(opt_data) {
  jspb.Message.initialize(this, opt_data, 0, -1, null, null);
};
goog.inherits(proto.g8e.operator.v1.FetchFileHistoryRequested, jspb.Message);
if (goog.DEBUG && !COMPILED) {
  /**
   * @public
   * @override
   */
  proto.g8e.operator.v1.FetchFileHistoryRequested.displayName = 'proto.g8e.operator.v1.FetchFileHistoryRequested';
}
/**
 * Generated by JsPbCodeGenerator.
 * @param {Array=} opt_data Optional initial data array, typically from a
 * server response, or constructed directly in Javascript. The array is used
 * in place and becomes part of the constructed object. It is not cloned.
 * If no data is provided, the constructed object will be empty, but still
 * valid.
 * @extends {jspb.Message}
 * @constructor
 */
proto.g8e.operator.v1.FetchFileDiffRequested = function(opt_data) {
  jspb.Message.initialize(this, opt_data, 0, -1, null, null);
};
goog.inherits(proto.g8e.operator.v1.FetchFileDiffRequested, jspb.Message);
if (goog.DEBUG && !COMPILED) {
  /**
   * @public
   * @override
   */
  proto.g8e.operator.v1.FetchFileDiffRequested.displayName = 'proto.g8e.operator.v1.FetchFileDiffRequested';
}
/**
 * Generated by JsPbCodeGenerator.
 * @param {Array=} opt_data Optional initial data array, typically from a
 * server response, or constructed directly in Javascript. The array is used
 * in place and becomes part of the constructed object. It is not cloned.
 * If no data is provided, the constructed object will be empty, but still
 * valid.
 * @extends {jspb.Message}
 * @constructor
 */
proto.g8e.operator.v1.RestoreFileRequested = function(opt_data) {
  jspb.Message.initialize(this, opt_data, 0, -1, null, null);
};
goog.inherits(proto.g8e.operator.v1.RestoreFileRequested, jspb.Message);
if (goog.DEBUG && !COMPILED) {
  /**
   * @public
   * @override
   */
  proto.g8e.operator.v1.RestoreFileRequested.displayName = 'proto.g8e.operator.v1.RestoreFileRequested';
}
/**
 * Generated by JsPbCodeGenerator.
 * @param {Array=} opt_data Optional initial data array, typically from a
 * server response, or constructed directly in Javascript. The array is used
 * in place and becomes part of the constructed object. It is not cloned.
 * If no data is provided, the constructed object will be empty, but still
 * valid.
 * @extends {jspb.Message}
 * @constructor
 */
proto.g8e.operator.v1.DirectCommandAuditRequested = function(opt_data) {
  jspb.Message.initialize(this, opt_data, 0, -1, null, null);
};
goog.inherits(proto.g8e.operator.v1.DirectCommandAuditRequested, jspb.Message);
if (goog.DEBUG && !COMPILED) {
  /**
   * @public
   * @override
   */
  proto.g8e.operator.v1.DirectCommandAuditRequested.displayName = 'proto.g8e.operator.v1.DirectCommandAuditRequested';
}
/**
 * Generated by JsPbCodeGenerator.
 * @param {Array=} opt_data Optional initial data array, typically from a
 * server response, or constructed directly in Javascript. The array is used
 * in place and becomes part of the constructed object. It is not cloned.
 * If no data is provided, the constructed object will be empty, but still
 * valid.
 * @extends {jspb.Message}
 * @constructor
 */
proto.g8e.operator.v1.DirectCommandResultAuditRequested = function(opt_data) {
  jspb.Message.initialize(this, opt_data, 0, -1, null, null);
};
goog.inherits(proto.g8e.operator.v1.DirectCommandResultAuditRequested, jspb.Message);
if (goog.DEBUG && !COMPILED) {
  /**
   * @public
   * @override
   */
  proto.g8e.operator.v1.DirectCommandResultAuditRequested.displayName = 'proto.g8e.operator.v1.DirectCommandResultAuditRequested';
}
/**
 * Generated by JsPbCodeGenerator.
 * @param {Array=} opt_data Optional initial data array, typically from a
 * server response, or constructed directly in Javascript. The array is used
 * in place and becomes part of the constructed object. It is not cloned.
 * If no data is provided, the constructed object will be empty, but still
 * valid.
 * @extends {jspb.Message}
 * @constructor
 */
proto.g8e.operator.v1.AuditMsgRequested = function(opt_data) {
  jspb.Message.initialize(this, opt_data, 0, -1, null, null);
};
goog.inherits(proto.g8e.operator.v1.AuditMsgRequested, jspb.Message);
if (goog.DEBUG && !COMPILED) {
  /**
   * @public
   * @override
   */
  proto.g8e.operator.v1.AuditMsgRequested.displayName = 'proto.g8e.operator.v1.AuditMsgRequested';
}
/**
 * Generated by JsPbCodeGenerator.
 * @param {Array=} opt_data Optional initial data array, typically from a
 * server response, or constructed directly in Javascript. The array is used
 * in place and becomes part of the constructed object. It is not cloned.
 * If no data is provided, the constructed object will be empty, but still
 * valid.
 * @extends {jspb.Message}
 * @constructor
 */
proto.g8e.operator.v1.SignCertificateRequested = function(opt_data) {
  jspb.Message.initialize(this, opt_data, 0, -1, null, null);
};
goog.inherits(proto.g8e.operator.v1.SignCertificateRequested, jspb.Message);
if (goog.DEBUG && !COMPILED) {
  /**
   * @public
   * @override
   */
  proto.g8e.operator.v1.SignCertificateRequested.displayName = 'proto.g8e.operator.v1.SignCertificateRequested';
}
/**
 * Generated by JsPbCodeGenerator.
 * @param {Array=} opt_data Optional initial data array, typically from a
 * server response, or constructed directly in Javascript. The array is used
 * in place and becomes part of the constructed object. It is not cloned.
 * If no data is provided, the constructed object will be empty, but still
 * valid.
 * @extends {jspb.Message}
 * @constructor
 */
proto.g8e.operator.v1.SignCertificateResult = function(opt_data) {
  jspb.Message.initialize(this, opt_data, 0, -1, null, null);
};
goog.inherits(proto.g8e.operator.v1.SignCertificateResult, jspb.Message);
if (goog.DEBUG && !COMPILED) {
  /**
   * @public
   * @override
   */
  proto.g8e.operator.v1.SignCertificateResult.displayName = 'proto.g8e.operator.v1.SignCertificateResult';
}
/**
 * Generated by JsPbCodeGenerator.
 * @param {Array=} opt_data Optional initial data array, typically from a
 * server response, or constructed directly in Javascript. The array is used
 * in place and becomes part of the constructed object. It is not cloned.
 * If no data is provided, the constructed object will be empty, but still
 * valid.
 * @extends {jspb.Message}
 * @constructor
 */
proto.g8e.operator.v1.CreateDeviceLinkRequested = function(opt_data) {
  jspb.Message.initialize(this, opt_data, 0, -1, null, null);
};
goog.inherits(proto.g8e.operator.v1.CreateDeviceLinkRequested, jspb.Message);
if (goog.DEBUG && !COMPILED) {
  /**
   * @public
   * @override
   */
  proto.g8e.operator.v1.CreateDeviceLinkRequested.displayName = 'proto.g8e.operator.v1.CreateDeviceLinkRequested';
}
/**
 * Generated by JsPbCodeGenerator.
 * @param {Array=} opt_data Optional initial data array, typically from a
 * server response, or constructed directly in Javascript. The array is used
 * in place and becomes part of the constructed object. It is not cloned.
 * If no data is provided, the constructed object will be empty, but still
 * valid.
 * @extends {jspb.Message}
 * @constructor
 */
proto.g8e.operator.v1.DeviceLink = function(opt_data) {
  jspb.Message.initialize(this, opt_data, 0, -1, null, null);
};
goog.inherits(proto.g8e.operator.v1.DeviceLink, jspb.Message);
if (goog.DEBUG && !COMPILED) {
  /**
   * @public
   * @override
   */
  proto.g8e.operator.v1.DeviceLink.displayName = 'proto.g8e.operator.v1.DeviceLink';
}
/**
 * Generated by JsPbCodeGenerator.
 * @param {Array=} opt_data Optional initial data array, typically from a
 * server response, or constructed directly in Javascript. The array is used
 * in place and becomes part of the constructed object. It is not cloned.
 * If no data is provided, the constructed object will be empty, but still
 * valid.
 * @extends {jspb.Message}
 * @constructor
 */
proto.g8e.operator.v1.DeviceLinkResult = function(opt_data) {
  jspb.Message.initialize(this, opt_data, 0, -1, null, null);
};
goog.inherits(proto.g8e.operator.v1.DeviceLinkResult, jspb.Message);
if (goog.DEBUG && !COMPILED) {
  /**
   * @public
   * @override
   */
  proto.g8e.operator.v1.DeviceLinkResult.displayName = 'proto.g8e.operator.v1.DeviceLinkResult';
}
/**
 * Generated by JsPbCodeGenerator.
 * @param {Array=} opt_data Optional initial data array, typically from a
 * server response, or constructed directly in Javascript. The array is used
 * in place and becomes part of the constructed object. It is not cloned.
 * If no data is provided, the constructed object will be empty, but still
 * valid.
 * @extends {jspb.Message}
 * @constructor
 */
proto.g8e.operator.v1.ListDeviceLinksRequested = function(opt_data) {
  jspb.Message.initialize(this, opt_data, 0, -1, null, null);
};
goog.inherits(proto.g8e.operator.v1.ListDeviceLinksRequested, jspb.Message);
if (goog.DEBUG && !COMPILED) {
  /**
   * @public
   * @override
   */
  proto.g8e.operator.v1.ListDeviceLinksRequested.displayName = 'proto.g8e.operator.v1.ListDeviceLinksRequested';
}
/**
 * Generated by JsPbCodeGenerator.
 * @param {Array=} opt_data Optional initial data array, typically from a
 * server response, or constructed directly in Javascript. The array is used
 * in place and becomes part of the constructed object. It is not cloned.
 * If no data is provided, the constructed object will be empty, but still
 * valid.
 * @extends {jspb.Message}
 * @constructor
 */
proto.g8e.operator.v1.ListDeviceLinksResult = function(opt_data) {
  jspb.Message.initialize(this, opt_data, 0, -1, proto.g8e.operator.v1.ListDeviceLinksResult.repeatedFields_, null);
};
goog.inherits(proto.g8e.operator.v1.ListDeviceLinksResult, jspb.Message);
if (goog.DEBUG && !COMPILED) {
  /**
   * @public
   * @override
   */
  proto.g8e.operator.v1.ListDeviceLinksResult.displayName = 'proto.g8e.operator.v1.ListDeviceLinksResult';
}
/**
 * Generated by JsPbCodeGenerator.
 * @param {Array=} opt_data Optional initial data array, typically from a
 * server response, or constructed directly in Javascript. The array is used
 * in place and becomes part of the constructed object. It is not cloned.
 * If no data is provided, the constructed object will be empty, but still
 * valid.
 * @extends {jspb.Message}
 * @constructor
 */
proto.g8e.operator.v1.DeleteDeviceLinkRequested = function(opt_data) {
  jspb.Message.initialize(this, opt_data, 0, -1, null, null);
};
goog.inherits(proto.g8e.operator.v1.DeleteDeviceLinkRequested, jspb.Message);
if (goog.DEBUG && !COMPILED) {
  /**
   * @public
   * @override
   */
  proto.g8e.operator.v1.DeleteDeviceLinkRequested.displayName = 'proto.g8e.operator.v1.DeleteDeviceLinkRequested';
}
/**
 * Generated by JsPbCodeGenerator.
 * @param {Array=} opt_data Optional initial data array, typically from a
 * server response, or constructed directly in Javascript. The array is used
 * in place and becomes part of the constructed object. It is not cloned.
 * If no data is provided, the constructed object will be empty, but still
 * valid.
 * @extends {jspb.Message}
 * @constructor
 */
proto.g8e.operator.v1.TerminateOperatorRequested = function(opt_data) {
  jspb.Message.initialize(this, opt_data, 0, -1, null, null);
};
goog.inherits(proto.g8e.operator.v1.TerminateOperatorRequested, jspb.Message);
if (goog.DEBUG && !COMPILED) {
  /**
   * @public
   * @override
   */
  proto.g8e.operator.v1.TerminateOperatorRequested.displayName = 'proto.g8e.operator.v1.TerminateOperatorRequested';
}
/**
 * Generated by JsPbCodeGenerator.
 * @param {Array=} opt_data Optional initial data array, typically from a
 * server response, or constructed directly in Javascript. The array is used
 * in place and becomes part of the constructed object. It is not cloned.
 * If no data is provided, the constructed object will be empty, but still
 * valid.
 * @extends {jspb.Message}
 * @constructor
 */
proto.g8e.operator.v1.TerminateOperatorResult = function(opt_data) {
  jspb.Message.initialize(this, opt_data, 0, -1, null, null);
};
goog.inherits(proto.g8e.operator.v1.TerminateOperatorResult, jspb.Message);
if (goog.DEBUG && !COMPILED) {
  /**
   * @public
   * @override
   */
  proto.g8e.operator.v1.TerminateOperatorResult.displayName = 'proto.g8e.operator.v1.TerminateOperatorResult';
}
/**
 * Generated by JsPbCodeGenerator.
 * @param {Array=} opt_data Optional initial data array, typically from a
 * server response, or constructed directly in Javascript. The array is used
 * in place and becomes part of the constructed object. It is not cloned.
 * If no data is provided, the constructed object will be empty, but still
 * valid.
 * @extends {jspb.Message}
 * @constructor
 */
proto.g8e.operator.v1.RotateAPIKeyRequested = function(opt_data) {
  jspb.Message.initialize(this, opt_data, 0, -1, null, null);
};
goog.inherits(proto.g8e.operator.v1.RotateAPIKeyRequested, jspb.Message);
if (goog.DEBUG && !COMPILED) {
  /**
   * @public
   * @override
   */
  proto.g8e.operator.v1.RotateAPIKeyRequested.displayName = 'proto.g8e.operator.v1.RotateAPIKeyRequested';
}
/**
 * Generated by JsPbCodeGenerator.
 * @param {Array=} opt_data Optional initial data array, typically from a
 * server response, or constructed directly in Javascript. The array is used
 * in place and becomes part of the constructed object. It is not cloned.
 * If no data is provided, the constructed object will be empty, but still
 * valid.
 * @extends {jspb.Message}
 * @constructor
 */
proto.g8e.operator.v1.RotateAPIKeyResult = function(opt_data) {
  jspb.Message.initialize(this, opt_data, 0, -1, null, null);
};
goog.inherits(proto.g8e.operator.v1.RotateAPIKeyResult, jspb.Message);
if (goog.DEBUG && !COMPILED) {
  /**
   * @public
   * @override
   */
  proto.g8e.operator.v1.RotateAPIKeyResult.displayName = 'proto.g8e.operator.v1.RotateAPIKeyResult';
}
/**
 * Generated by JsPbCodeGenerator.
 * @param {Array=} opt_data Optional initial data array, typically from a
 * server response, or constructed directly in Javascript. The array is used
 * in place and becomes part of the constructed object. It is not cloned.
 * If no data is provided, the constructed object will be empty, but still
 * valid.
 * @extends {jspb.Message}
 * @constructor
 */
proto.g8e.operator.v1.ListOperatorSlotsRequested = function(opt_data) {
  jspb.Message.initialize(this, opt_data, 0, -1, null, null);
};
goog.inherits(proto.g8e.operator.v1.ListOperatorSlotsRequested, jspb.Message);
if (goog.DEBUG && !COMPILED) {
  /**
   * @public
   * @override
   */
  proto.g8e.operator.v1.ListOperatorSlotsRequested.displayName = 'proto.g8e.operator.v1.ListOperatorSlotsRequested';
}
/**
 * Generated by JsPbCodeGenerator.
 * @param {Array=} opt_data Optional initial data array, typically from a
 * server response, or constructed directly in Javascript. The array is used
 * in place and becomes part of the constructed object. It is not cloned.
 * If no data is provided, the constructed object will be empty, but still
 * valid.
 * @extends {jspb.Message}
 * @constructor
 */
proto.g8e.operator.v1.ListOperatorSlotsResult = function(opt_data) {
  jspb.Message.initialize(this, opt_data, 0, -1, proto.g8e.operator.v1.ListOperatorSlotsResult.repeatedFields_, null);
};
goog.inherits(proto.g8e.operator.v1.ListOperatorSlotsResult, jspb.Message);
if (goog.DEBUG && !COMPILED) {
  /**
   * @public
   * @override
   */
  proto.g8e.operator.v1.ListOperatorSlotsResult.displayName = 'proto.g8e.operator.v1.ListOperatorSlotsResult';
}
/**
 * Generated by JsPbCodeGenerator.
 * @param {Array=} opt_data Optional initial data array, typically from a
 * server response, or constructed directly in Javascript. The array is used
 * in place and becomes part of the constructed object. It is not cloned.
 * If no data is provided, the constructed object will be empty, but still
 * valid.
 * @extends {jspb.Message}
 * @constructor
 */
proto.g8e.operator.v1.BindOperatorsRequested = function(opt_data) {
  jspb.Message.initialize(this, opt_data, 0, -1, proto.g8e.operator.v1.BindOperatorsRequested.repeatedFields_, null);
};
goog.inherits(proto.g8e.operator.v1.BindOperatorsRequested, jspb.Message);
if (goog.DEBUG && !COMPILED) {
  /**
   * @public
   * @override
   */
  proto.g8e.operator.v1.BindOperatorsRequested.displayName = 'proto.g8e.operator.v1.BindOperatorsRequested';
}
/**
 * Generated by JsPbCodeGenerator.
 * @param {Array=} opt_data Optional initial data array, typically from a
 * server response, or constructed directly in Javascript. The array is used
 * in place and becomes part of the constructed object. It is not cloned.
 * If no data is provided, the constructed object will be empty, but still
 * valid.
 * @extends {jspb.Message}
 * @constructor
 */
proto.g8e.operator.v1.BindOperatorsResult = function(opt_data) {
  jspb.Message.initialize(this, opt_data, 0, -1, proto.g8e.operator.v1.BindOperatorsResult.repeatedFields_, null);
};
goog.inherits(proto.g8e.operator.v1.BindOperatorsResult, jspb.Message);
if (goog.DEBUG && !COMPILED) {
  /**
   * @public
   * @override
   */
  proto.g8e.operator.v1.BindOperatorsResult.displayName = 'proto.g8e.operator.v1.BindOperatorsResult';
}
/**
 * Generated by JsPbCodeGenerator.
 * @param {Array=} opt_data Optional initial data array, typically from a
 * server response, or constructed directly in Javascript. The array is used
 * in place and becomes part of the constructed object. It is not cloned.
 * If no data is provided, the constructed object will be empty, but still
 * valid.
 * @extends {jspb.Message}
 * @constructor
 */
proto.g8e.operator.v1.UnbindOperatorsRequested = function(opt_data) {
  jspb.Message.initialize(this, opt_data, 0, -1, proto.g8e.operator.v1.UnbindOperatorsRequested.repeatedFields_, null);
};
goog.inherits(proto.g8e.operator.v1.UnbindOperatorsRequested, jspb.Message);
if (goog.DEBUG && !COMPILED) {
  /**
   * @public
   * @override
   */
  proto.g8e.operator.v1.UnbindOperatorsRequested.displayName = 'proto.g8e.operator.v1.UnbindOperatorsRequested';
}
/**
 * Generated by JsPbCodeGenerator.
 * @param {Array=} opt_data Optional initial data array, typically from a
 * server response, or constructed directly in Javascript. The array is used
 * in place and becomes part of the constructed object. It is not cloned.
 * If no data is provided, the constructed object will be empty, but still
 * valid.
 * @extends {jspb.Message}
 * @constructor
 */
proto.g8e.operator.v1.UnbindOperatorsResult = function(opt_data) {
  jspb.Message.initialize(this, opt_data, 0, -1, proto.g8e.operator.v1.UnbindOperatorsResult.repeatedFields_, null);
};
goog.inherits(proto.g8e.operator.v1.UnbindOperatorsResult, jspb.Message);
if (goog.DEBUG && !COMPILED) {
  /**
   * @public
   * @override
   */
  proto.g8e.operator.v1.UnbindOperatorsResult.displayName = 'proto.g8e.operator.v1.UnbindOperatorsResult';
}
/**
 * Generated by JsPbCodeGenerator.
 * @param {Array=} opt_data Optional initial data array, typically from a
 * server response, or constructed directly in Javascript. The array is used
 * in place and becomes part of the constructed object. It is not cloned.
 * If no data is provided, the constructed object will be empty, but still
 * valid.
 * @extends {jspb.Message}
 * @constructor
 */
proto.g8e.operator.v1.SetTargetContextRequested = function(opt_data) {
  jspb.Message.initialize(this, opt_data, 0, -1, null, null);
};
goog.inherits(proto.g8e.operator.v1.SetTargetContextRequested, jspb.Message);
if (goog.DEBUG && !COMPILED) {
  /**
   * @public
   * @override
   */
  proto.g8e.operator.v1.SetTargetContextRequested.displayName = 'proto.g8e.operator.v1.SetTargetContextRequested';
}
/**
 * Generated by JsPbCodeGenerator.
 * @param {Array=} opt_data Optional initial data array, typically from a
 * server response, or constructed directly in Javascript. The array is used
 * in place and becomes part of the constructed object. It is not cloned.
 * If no data is provided, the constructed object will be empty, but still
 * valid.
 * @extends {jspb.Message}
 * @constructor
 */
proto.g8e.operator.v1.SetTargetContextResult = function(opt_data) {
  jspb.Message.initialize(this, opt_data, 0, -1, null, null);
};
goog.inherits(proto.g8e.operator.v1.SetTargetContextResult, jspb.Message);
if (goog.DEBUG && !COMPILED) {
  /**
   * @public
   * @override
   */
  proto.g8e.operator.v1.SetTargetContextResult.displayName = 'proto.g8e.operator.v1.SetTargetContextResult';
}
/**
 * Generated by JsPbCodeGenerator.
 * @param {Array=} opt_data Optional initial data array, typically from a
 * server response, or constructed directly in Javascript. The array is used
 * in place and becomes part of the constructed object. It is not cloned.
 * If no data is provided, the constructed object will be empty, but still
 * valid.
 * @extends {jspb.Message}
 * @constructor
 */
proto.g8e.operator.v1.OperatorDocument = function(opt_data) {
  jspb.Message.initialize(this, opt_data, 0, -1, null, null);
};
goog.inherits(proto.g8e.operator.v1.OperatorDocument, jspb.Message);
if (goog.DEBUG && !COMPILED) {
  /**
   * @public
   * @override
   */
  proto.g8e.operator.v1.OperatorDocument.displayName = 'proto.g8e.operator.v1.OperatorDocument';
}
/**
 * Generated by JsPbCodeGenerator.
 * @param {Array=} opt_data Optional initial data array, typically from a
 * server response, or constructed directly in Javascript. The array is used
 * in place and becomes part of the constructed object. It is not cloned.
 * If no data is provided, the constructed object will be empty, but still
 * valid.
 * @extends {jspb.Message}
 * @constructor
 */
proto.g8e.operator.v1.ShutdownRequested = function(opt_data) {
  jspb.Message.initialize(this, opt_data, 0, -1, null, null);
};
goog.inherits(proto.g8e.operator.v1.ShutdownRequested, jspb.Message);
if (goog.DEBUG && !COMPILED) {
  /**
   * @public
   * @override
   */
  proto.g8e.operator.v1.ShutdownRequested.displayName = 'proto.g8e.operator.v1.ShutdownRequested';
}
/**
 * Generated by JsPbCodeGenerator.
 * @param {Array=} opt_data Optional initial data array, typically from a
 * server response, or constructed directly in Javascript. The array is used
 * in place and becomes part of the constructed object. It is not cloned.
 * If no data is provided, the constructed object will be empty, but still
 * valid.
 * @extends {jspb.Message}
 * @constructor
 */
proto.g8e.operator.v1.CommandResult = function(opt_data) {
  jspb.Message.initialize(this, opt_data, 0, -1, null, null);
};
goog.inherits(proto.g8e.operator.v1.CommandResult, jspb.Message);
if (goog.DEBUG && !COMPILED) {
  /**
   * @public
   * @override
   */
  proto.g8e.operator.v1.CommandResult.displayName = 'proto.g8e.operator.v1.CommandResult';
}
/**
 * Generated by JsPbCodeGenerator.
 * @param {Array=} opt_data Optional initial data array, typically from a
 * server response, or constructed directly in Javascript. The array is used
 * in place and becomes part of the constructed object. It is not cloned.
 * If no data is provided, the constructed object will be empty, but still
 * valid.
 * @extends {jspb.Message}
 * @constructor
 */
proto.g8e.operator.v1.FsEntry = function(opt_data) {
  jspb.Message.initialize(this, opt_data, 0, -1, null, null);
};
goog.inherits(proto.g8e.operator.v1.FsEntry, jspb.Message);
if (goog.DEBUG && !COMPILED) {
  /**
   * @public
   * @override
   */
  proto.g8e.operator.v1.FsEntry.displayName = 'proto.g8e.operator.v1.FsEntry';
}
/**
 * Generated by JsPbCodeGenerator.
 * @param {Array=} opt_data Optional initial data array, typically from a
 * server response, or constructed directly in Javascript. The array is used
 * in place and becomes part of the constructed object. It is not cloned.
 * If no data is provided, the constructed object will be empty, but still
 * valid.
 * @extends {jspb.Message}
 * @constructor
 */
proto.g8e.operator.v1.FsListResult = function(opt_data) {
  jspb.Message.initialize(this, opt_data, 0, -1, proto.g8e.operator.v1.FsListResult.repeatedFields_, null);
};
goog.inherits(proto.g8e.operator.v1.FsListResult, jspb.Message);
if (goog.DEBUG && !COMPILED) {
  /**
   * @public
   * @override
   */
  proto.g8e.operator.v1.FsListResult.displayName = 'proto.g8e.operator.v1.FsListResult';
}
/**
 * Generated by JsPbCodeGenerator.
 * @param {Array=} opt_data Optional initial data array, typically from a
 * server response, or constructed directly in Javascript. The array is used
 * in place and becomes part of the constructed object. It is not cloned.
 * If no data is provided, the constructed object will be empty, but still
 * valid.
 * @extends {jspb.Message}
 * @constructor
 */
proto.g8e.operator.v1.FsReadResult = function(opt_data) {
  jspb.Message.initialize(this, opt_data, 0, -1, null, null);
};
goog.inherits(proto.g8e.operator.v1.FsReadResult, jspb.Message);
if (goog.DEBUG && !COMPILED) {
  /**
   * @public
   * @override
   */
  proto.g8e.operator.v1.FsReadResult.displayName = 'proto.g8e.operator.v1.FsReadResult';
}
/**
 * Generated by JsPbCodeGenerator.
 * @param {Array=} opt_data Optional initial data array, typically from a
 * server response, or constructed directly in Javascript. The array is used
 * in place and becomes part of the constructed object. It is not cloned.
 * If no data is provided, the constructed object will be empty, but still
 * valid.
 * @extends {jspb.Message}
 * @constructor
 */
proto.g8e.operator.v1.FsGrepMatch = function(opt_data) {
  jspb.Message.initialize(this, opt_data, 0, -1, proto.g8e.operator.v1.FsGrepMatch.repeatedFields_, null);
};
goog.inherits(proto.g8e.operator.v1.FsGrepMatch, jspb.Message);
if (goog.DEBUG && !COMPILED) {
  /**
   * @public
   * @override
   */
  proto.g8e.operator.v1.FsGrepMatch.displayName = 'proto.g8e.operator.v1.FsGrepMatch';
}
/**
 * Generated by JsPbCodeGenerator.
 * @param {Array=} opt_data Optional initial data array, typically from a
 * server response, or constructed directly in Javascript. The array is used
 * in place and becomes part of the constructed object. It is not cloned.
 * If no data is provided, the constructed object will be empty, but still
 * valid.
 * @extends {jspb.Message}
 * @constructor
 */
proto.g8e.operator.v1.FsGrepResult = function(opt_data) {
  jspb.Message.initialize(this, opt_data, 0, -1, proto.g8e.operator.v1.FsGrepResult.repeatedFields_, null);
};
goog.inherits(proto.g8e.operator.v1.FsGrepResult, jspb.Message);
if (goog.DEBUG && !COMPILED) {
  /**
   * @public
   * @override
   */
  proto.g8e.operator.v1.FsGrepResult.displayName = 'proto.g8e.operator.v1.FsGrepResult';
}
/**
 * Generated by JsPbCodeGenerator.
 * @param {Array=} opt_data Optional initial data array, typically from a
 * server response, or constructed directly in Javascript. The array is used
 * in place and becomes part of the constructed object. It is not cloned.
 * If no data is provided, the constructed object will be empty, but still
 * valid.
 * @extends {jspb.Message}
 * @constructor
 */
proto.g8e.operator.v1.FileEditResult = function(opt_data) {
  jspb.Message.initialize(this, opt_data, 0, -1, null, null);
};
goog.inherits(proto.g8e.operator.v1.FileEditResult, jspb.Message);
if (goog.DEBUG && !COMPILED) {
  /**
   * @public
   * @override
   */
  proto.g8e.operator.v1.FileEditResult.displayName = 'proto.g8e.operator.v1.FileEditResult';
}
/**
 * Generated by JsPbCodeGenerator.
 * @param {Array=} opt_data Optional initial data array, typically from a
 * server response, or constructed directly in Javascript. The array is used
 * in place and becomes part of the constructed object. It is not cloned.
 * If no data is provided, the constructed object will be empty, but still
 * valid.
 * @extends {jspb.Message}
 * @constructor
 */
proto.g8e.operator.v1.ExecutionStatusUpdate = function(opt_data) {
  jspb.Message.initialize(this, opt_data, 0, -1, null, null);
};
goog.inherits(proto.g8e.operator.v1.ExecutionStatusUpdate, jspb.Message);
if (goog.DEBUG && !COMPILED) {
  /**
   * @public
   * @override
   */
  proto.g8e.operator.v1.ExecutionStatusUpdate.displayName = 'proto.g8e.operator.v1.ExecutionStatusUpdate';
}
/**
 * Generated by JsPbCodeGenerator.
 * @param {Array=} opt_data Optional initial data array, typically from a
 * server response, or constructed directly in Javascript. The array is used
 * in place and becomes part of the constructed object. It is not cloned.
 * If no data is provided, the constructed object will be empty, but still
 * valid.
 * @extends {jspb.Message}
 * @constructor
 */
proto.g8e.operator.v1.PortCheckEntry = function(opt_data) {
  jspb.Message.initialize(this, opt_data, 0, -1, null, null);
};
goog.inherits(proto.g8e.operator.v1.PortCheckEntry, jspb.Message);
if (goog.DEBUG && !COMPILED) {
  /**
   * @public
   * @override
   */
  proto.g8e.operator.v1.PortCheckEntry.displayName = 'proto.g8e.operator.v1.PortCheckEntry';
}
/**
 * Generated by JsPbCodeGenerator.
 * @param {Array=} opt_data Optional initial data array, typically from a
 * server response, or constructed directly in Javascript. The array is used
 * in place and becomes part of the constructed object. It is not cloned.
 * If no data is provided, the constructed object will be empty, but still
 * valid.
 * @extends {jspb.Message}
 * @constructor
 */
proto.g8e.operator.v1.PortCheckResult = function(opt_data) {
  jspb.Message.initialize(this, opt_data, 0, -1, proto.g8e.operator.v1.PortCheckResult.repeatedFields_, null);
};
goog.inherits(proto.g8e.operator.v1.PortCheckResult, jspb.Message);
if (goog.DEBUG && !COMPILED) {
  /**
   * @public
   * @override
   */
  proto.g8e.operator.v1.PortCheckResult.displayName = 'proto.g8e.operator.v1.PortCheckResult';
}
/**
 * Generated by JsPbCodeGenerator.
 * @param {Array=} opt_data Optional initial data array, typically from a
 * server response, or constructed directly in Javascript. The array is used
 * in place and becomes part of the constructed object. It is not cloned.
 * If no data is provided, the constructed object will be empty, but still
 * valid.
 * @extends {jspb.Message}
 * @constructor
 */
proto.g8e.operator.v1.FetchLogsResult = function(opt_data) {
  jspb.Message.initialize(this, opt_data, 0, -1, null, null);
};
goog.inherits(proto.g8e.operator.v1.FetchLogsResult, jspb.Message);
if (goog.DEBUG && !COMPILED) {
  /**
   * @public
   * @override
   */
  proto.g8e.operator.v1.FetchLogsResult.displayName = 'proto.g8e.operator.v1.FetchLogsResult';
}
/**
 * Generated by JsPbCodeGenerator.
 * @param {Array=} opt_data Optional initial data array, typically from a
 * server response, or constructed directly in Javascript. The array is used
 * in place and becomes part of the constructed object. It is not cloned.
 * If no data is provided, the constructed object will be empty, but still
 * valid.
 * @extends {jspb.Message}
 * @constructor
 */
proto.g8e.operator.v1.AuditWebSession = function(opt_data) {
  jspb.Message.initialize(this, opt_data, 0, -1, null, null);
};
goog.inherits(proto.g8e.operator.v1.AuditWebSession, jspb.Message);
if (goog.DEBUG && !COMPILED) {
  /**
   * @public
   * @override
   */
  proto.g8e.operator.v1.AuditWebSession.displayName = 'proto.g8e.operator.v1.AuditWebSession';
}
/**
 * Generated by JsPbCodeGenerator.
 * @param {Array=} opt_data Optional initial data array, typically from a
 * server response, or constructed directly in Javascript. The array is used
 * in place and becomes part of the constructed object. It is not cloned.
 * If no data is provided, the constructed object will be empty, but still
 * valid.
 * @extends {jspb.Message}
 * @constructor
 */
proto.g8e.operator.v1.AuditFileMutation = function(opt_data) {
  jspb.Message.initialize(this, opt_data, 0, -1, null, null);
};
goog.inherits(proto.g8e.operator.v1.AuditFileMutation, jspb.Message);
if (goog.DEBUG && !COMPILED) {
  /**
   * @public
   * @override
   */
  proto.g8e.operator.v1.AuditFileMutation.displayName = 'proto.g8e.operator.v1.AuditFileMutation';
}
/**
 * Generated by JsPbCodeGenerator.
 * @param {Array=} opt_data Optional initial data array, typically from a
 * server response, or constructed directly in Javascript. The array is used
 * in place and becomes part of the constructed object. It is not cloned.
 * If no data is provided, the constructed object will be empty, but still
 * valid.
 * @extends {jspb.Message}
 * @constructor
 */
proto.g8e.operator.v1.AuditEvent = function(opt_data) {
  jspb.Message.initialize(this, opt_data, 0, -1, proto.g8e.operator.v1.AuditEvent.repeatedFields_, null);
};
goog.inherits(proto.g8e.operator.v1.AuditEvent, jspb.Message);
if (goog.DEBUG && !COMPILED) {
  /**
   * @public
   * @override
   */
  proto.g8e.operator.v1.AuditEvent.displayName = 'proto.g8e.operator.v1.AuditEvent';
}
/**
 * Generated by JsPbCodeGenerator.
 * @param {Array=} opt_data Optional initial data array, typically from a
 * server response, or constructed directly in Javascript. The array is used
 * in place and becomes part of the constructed object. It is not cloned.
 * If no data is provided, the constructed object will be empty, but still
 * valid.
 * @extends {jspb.Message}
 * @constructor
 */
proto.g8e.operator.v1.FetchHistoryResult = function(opt_data) {
  jspb.Message.initialize(this, opt_data, 0, -1, proto.g8e.operator.v1.FetchHistoryResult.repeatedFields_, null);
};
goog.inherits(proto.g8e.operator.v1.FetchHistoryResult, jspb.Message);
if (goog.DEBUG && !COMPILED) {
  /**
   * @public
   * @override
   */
  proto.g8e.operator.v1.FetchHistoryResult.displayName = 'proto.g8e.operator.v1.FetchHistoryResult';
}
/**
 * Generated by JsPbCodeGenerator.
 * @param {Array=} opt_data Optional initial data array, typically from a
 * server response, or constructed directly in Javascript. The array is used
 * in place and becomes part of the constructed object. It is not cloned.
 * If no data is provided, the constructed object will be empty, but still
 * valid.
 * @extends {jspb.Message}
 * @constructor
 */
proto.g8e.operator.v1.FileHistoryEntry = function(opt_data) {
  jspb.Message.initialize(this, opt_data, 0, -1, null, null);
};
goog.inherits(proto.g8e.operator.v1.FileHistoryEntry, jspb.Message);
if (goog.DEBUG && !COMPILED) {
  /**
   * @public
   * @override
   */
  proto.g8e.operator.v1.FileHistoryEntry.displayName = 'proto.g8e.operator.v1.FileHistoryEntry';
}
/**
 * Generated by JsPbCodeGenerator.
 * @param {Array=} opt_data Optional initial data array, typically from a
 * server response, or constructed directly in Javascript. The array is used
 * in place and becomes part of the constructed object. It is not cloned.
 * If no data is provided, the constructed object will be empty, but still
 * valid.
 * @extends {jspb.Message}
 * @constructor
 */
proto.g8e.operator.v1.FetchFileHistoryResult = function(opt_data) {
  jspb.Message.initialize(this, opt_data, 0, -1, proto.g8e.operator.v1.FetchFileHistoryResult.repeatedFields_, null);
};
goog.inherits(proto.g8e.operator.v1.FetchFileHistoryResult, jspb.Message);
if (goog.DEBUG && !COMPILED) {
  /**
   * @public
   * @override
   */
  proto.g8e.operator.v1.FetchFileHistoryResult.displayName = 'proto.g8e.operator.v1.FetchFileHistoryResult';
}
/**
 * Generated by JsPbCodeGenerator.
 * @param {Array=} opt_data Optional initial data array, typically from a
 * server response, or constructed directly in Javascript. The array is used
 * in place and becomes part of the constructed object. It is not cloned.
 * If no data is provided, the constructed object will be empty, but still
 * valid.
 * @extends {jspb.Message}
 * @constructor
 */
proto.g8e.operator.v1.RestoreFileResult = function(opt_data) {
  jspb.Message.initialize(this, opt_data, 0, -1, null, null);
};
goog.inherits(proto.g8e.operator.v1.RestoreFileResult, jspb.Message);
if (goog.DEBUG && !COMPILED) {
  /**
   * @public
   * @override
   */
  proto.g8e.operator.v1.RestoreFileResult.displayName = 'proto.g8e.operator.v1.RestoreFileResult';
}
/**
 * Generated by JsPbCodeGenerator.
 * @param {Array=} opt_data Optional initial data array, typically from a
 * server response, or constructed directly in Javascript. The array is used
 * in place and becomes part of the constructed object. It is not cloned.
 * If no data is provided, the constructed object will be empty, but still
 * valid.
 * @extends {jspb.Message}
 * @constructor
 */
proto.g8e.operator.v1.FileDiffEntry = function(opt_data) {
  jspb.Message.initialize(this, opt_data, 0, -1, null, null);
};
goog.inherits(proto.g8e.operator.v1.FileDiffEntry, jspb.Message);
if (goog.DEBUG && !COMPILED) {
  /**
   * @public
   * @override
   */
  proto.g8e.operator.v1.FileDiffEntry.displayName = 'proto.g8e.operator.v1.FileDiffEntry';
}
/**
 * Generated by JsPbCodeGenerator.
 * @param {Array=} opt_data Optional initial data array, typically from a
 * server response, or constructed directly in Javascript. The array is used
 * in place and becomes part of the constructed object. It is not cloned.
 * If no data is provided, the constructed object will be empty, but still
 * valid.
 * @extends {jspb.Message}
 * @constructor
 */
proto.g8e.operator.v1.FetchFileDiffResult = function(opt_data) {
  jspb.Message.initialize(this, opt_data, 0, -1, proto.g8e.operator.v1.FetchFileDiffResult.repeatedFields_, null);
};
goog.inherits(proto.g8e.operator.v1.FetchFileDiffResult, jspb.Message);
if (goog.DEBUG && !COMPILED) {
  /**
   * @public
   * @override
   */
  proto.g8e.operator.v1.FetchFileDiffResult.displayName = 'proto.g8e.operator.v1.FetchFileDiffResult';
}
/**
 * Generated by JsPbCodeGenerator.
 * @param {Array=} opt_data Optional initial data array, typically from a
 * server response, or constructed directly in Javascript. The array is used
 * in place and becomes part of the constructed object. It is not cloned.
 * If no data is provided, the constructed object will be empty, but still
 * valid.
 * @extends {jspb.Message}
 * @constructor
 */
proto.g8e.operator.v1.HeartbeatResult = function(opt_data) {
  jspb.Message.initialize(this, opt_data, 0, -1, null, null);
};
goog.inherits(proto.g8e.operator.v1.HeartbeatResult, jspb.Message);
if (goog.DEBUG && !COMPILED) {
  /**
   * @public
   * @override
   */
  proto.g8e.operator.v1.HeartbeatResult.displayName = 'proto.g8e.operator.v1.HeartbeatResult';
}
/**
 * Generated by JsPbCodeGenerator.
 * @param {Array=} opt_data Optional initial data array, typically from a
 * server response, or constructed directly in Javascript. The array is used
 * in place and becomes part of the constructed object. It is not cloned.
 * If no data is provided, the constructed object will be empty, but still
 * valid.
 * @extends {jspb.Message}
 * @constructor
 */
proto.g8e.operator.v1.SystemIdentity = function(opt_data) {
  jspb.Message.initialize(this, opt_data, 0, -1, null, null);
};
goog.inherits(proto.g8e.operator.v1.SystemIdentity, jspb.Message);
if (goog.DEBUG && !COMPILED) {
  /**
   * @public
   * @override
   */
  proto.g8e.operator.v1.SystemIdentity.displayName = 'proto.g8e.operator.v1.SystemIdentity';
}
/**
 * Generated by JsPbCodeGenerator.
 * @param {Array=} opt_data Optional initial data array, typically from a
 * server response, or constructed directly in Javascript. The array is used
 * in place and becomes part of the constructed object. It is not cloned.
 * If no data is provided, the constructed object will be empty, but still
 * valid.
 * @extends {jspb.Message}
 * @constructor
 */
proto.g8e.operator.v1.NetworkInterface = function(opt_data) {
  jspb.Message.initialize(this, opt_data, 0, -1, null, null);
};
goog.inherits(proto.g8e.operator.v1.NetworkInterface, jspb.Message);
if (goog.DEBUG && !COMPILED) {
  /**
   * @public
   * @override
   */
  proto.g8e.operator.v1.NetworkInterface.displayName = 'proto.g8e.operator.v1.NetworkInterface';
}
/**
 * Generated by JsPbCodeGenerator.
 * @param {Array=} opt_data Optional initial data array, typically from a
 * server response, or constructed directly in Javascript. The array is used
 * in place and becomes part of the constructed object. It is not cloned.
 * If no data is provided, the constructed object will be empty, but still
 * valid.
 * @extends {jspb.Message}
 * @constructor
 */
proto.g8e.operator.v1.NetworkInfo = function(opt_data) {
  jspb.Message.initialize(this, opt_data, 0, -1, proto.g8e.operator.v1.NetworkInfo.repeatedFields_, null);
};
goog.inherits(proto.g8e.operator.v1.NetworkInfo, jspb.Message);
if (goog.DEBUG && !COMPILED) {
  /**
   * @public
   * @override
   */
  proto.g8e.operator.v1.NetworkInfo.displayName = 'proto.g8e.operator.v1.NetworkInfo';
}
/**
 * Generated by JsPbCodeGenerator.
 * @param {Array=} opt_data Optional initial data array, typically from a
 * server response, or constructed directly in Javascript. The array is used
 * in place and becomes part of the constructed object. It is not cloned.
 * If no data is provided, the constructed object will be empty, but still
 * valid.
 * @extends {jspb.Message}
 * @constructor
 */
proto.g8e.operator.v1.CapabilityFlags = function(opt_data) {
  jspb.Message.initialize(this, opt_data, 0, -1, null, null);
};
goog.inherits(proto.g8e.operator.v1.CapabilityFlags, jspb.Message);
if (goog.DEBUG && !COMPILED) {
  /**
   * @public
   * @override
   */
  proto.g8e.operator.v1.CapabilityFlags.displayName = 'proto.g8e.operator.v1.CapabilityFlags';
}
/**
 * Generated by JsPbCodeGenerator.
 * @param {Array=} opt_data Optional initial data array, typically from a
 * server response, or constructed directly in Javascript. The array is used
 * in place and becomes part of the constructed object. It is not cloned.
 * If no data is provided, the constructed object will be empty, but still
 * valid.
 * @extends {jspb.Message}
 * @constructor
 */
proto.g8e.operator.v1.VersionInfo = function(opt_data) {
  jspb.Message.initialize(this, opt_data, 0, -1, null, null);
};
goog.inherits(proto.g8e.operator.v1.VersionInfo, jspb.Message);
if (goog.DEBUG && !COMPILED) {
  /**
   * @public
   * @override
   */
  proto.g8e.operator.v1.VersionInfo.displayName = 'proto.g8e.operator.v1.VersionInfo';
}
/**
 * Generated by JsPbCodeGenerator.
 * @param {Array=} opt_data Optional initial data array, typically from a
 * server response, or constructed directly in Javascript. The array is used
 * in place and becomes part of the constructed object. It is not cloned.
 * If no data is provided, the constructed object will be empty, but still
 * valid.
 * @extends {jspb.Message}
 * @constructor
 */
proto.g8e.operator.v1.UptimeInfo = function(opt_data) {
  jspb.Message.initialize(this, opt_data, 0, -1, null, null);
};
goog.inherits(proto.g8e.operator.v1.UptimeInfo, jspb.Message);
if (goog.DEBUG && !COMPILED) {
  /**
   * @public
   * @override
   */
  proto.g8e.operator.v1.UptimeInfo.displayName = 'proto.g8e.operator.v1.UptimeInfo';
}
/**
 * Generated by JsPbCodeGenerator.
 * @param {Array=} opt_data Optional initial data array, typically from a
 * server response, or constructed directly in Javascript. The array is used
 * in place and becomes part of the constructed object. It is not cloned.
 * If no data is provided, the constructed object will be empty, but still
 * valid.
 * @extends {jspb.Message}
 * @constructor
 */
proto.g8e.operator.v1.PerformanceMetrics = function(opt_data) {
  jspb.Message.initialize(this, opt_data, 0, -1, null, null);
};
goog.inherits(proto.g8e.operator.v1.PerformanceMetrics, jspb.Message);
if (goog.DEBUG && !COMPILED) {
  /**
   * @public
   * @override
   */
  proto.g8e.operator.v1.PerformanceMetrics.displayName = 'proto.g8e.operator.v1.PerformanceMetrics';
}
/**
 * Generated by JsPbCodeGenerator.
 * @param {Array=} opt_data Optional initial data array, typically from a
 * server response, or constructed directly in Javascript. The array is used
 * in place and becomes part of the constructed object. It is not cloned.
 * If no data is provided, the constructed object will be empty, but still
 * valid.
 * @extends {jspb.Message}
 * @constructor
 */
proto.g8e.operator.v1.OSDetails = function(opt_data) {
  jspb.Message.initialize(this, opt_data, 0, -1, null, null);
};
goog.inherits(proto.g8e.operator.v1.OSDetails, jspb.Message);
if (goog.DEBUG && !COMPILED) {
  /**
   * @public
   * @override
   */
  proto.g8e.operator.v1.OSDetails.displayName = 'proto.g8e.operator.v1.OSDetails';
}
/**
 * Generated by JsPbCodeGenerator.
 * @param {Array=} opt_data Optional initial data array, typically from a
 * server response, or constructed directly in Javascript. The array is used
 * in place and becomes part of the constructed object. It is not cloned.
 * If no data is provided, the constructed object will be empty, but still
 * valid.
 * @extends {jspb.Message}
 * @constructor
 */
proto.g8e.operator.v1.UserDetails = function(opt_data) {
  jspb.Message.initialize(this, opt_data, 0, -1, null, null);
};
goog.inherits(proto.g8e.operator.v1.UserDetails, jspb.Message);
if (goog.DEBUG && !COMPILED) {
  /**
   * @public
   * @override
   */
  proto.g8e.operator.v1.UserDetails.displayName = 'proto.g8e.operator.v1.UserDetails';
}
/**
 * Generated by JsPbCodeGenerator.
 * @param {Array=} opt_data Optional initial data array, typically from a
 * server response, or constructed directly in Javascript. The array is used
 * in place and becomes part of the constructed object. It is not cloned.
 * If no data is provided, the constructed object will be empty, but still
 * valid.
 * @extends {jspb.Message}
 * @constructor
 */
proto.g8e.operator.v1.DiskDetails = function(opt_data) {
  jspb.Message.initialize(this, opt_data, 0, -1, null, null);
};
goog.inherits(proto.g8e.operator.v1.DiskDetails, jspb.Message);
if (goog.DEBUG && !COMPILED) {
  /**
   * @public
   * @override
   */
  proto.g8e.operator.v1.DiskDetails.displayName = 'proto.g8e.operator.v1.DiskDetails';
}
/**
 * Generated by JsPbCodeGenerator.
 * @param {Array=} opt_data Optional initial data array, typically from a
 * server response, or constructed directly in Javascript. The array is used
 * in place and becomes part of the constructed object. It is not cloned.
 * If no data is provided, the constructed object will be empty, but still
 * valid.
 * @extends {jspb.Message}
 * @constructor
 */
proto.g8e.operator.v1.MemoryDetails = function(opt_data) {
  jspb.Message.initialize(this, opt_data, 0, -1, null, null);
};
goog.inherits(proto.g8e.operator.v1.MemoryDetails, jspb.Message);
if (goog.DEBUG && !COMPILED) {
  /**
   * @public
   * @override
   */
  proto.g8e.operator.v1.MemoryDetails.displayName = 'proto.g8e.operator.v1.MemoryDetails';
}
/**
 * Generated by JsPbCodeGenerator.
 * @param {Array=} opt_data Optional initial data array, typically from a
 * server response, or constructed directly in Javascript. The array is used
 * in place and becomes part of the constructed object. It is not cloned.
 * If no data is provided, the constructed object will be empty, but still
 * valid.
 * @extends {jspb.Message}
 * @constructor
 */
proto.g8e.operator.v1.EnvironmentDetails = function(opt_data) {
  jspb.Message.initialize(this, opt_data, 0, -1, proto.g8e.operator.v1.EnvironmentDetails.repeatedFields_, null);
};
goog.inherits(proto.g8e.operator.v1.EnvironmentDetails, jspb.Message);
if (goog.DEBUG && !COMPILED) {
  /**
   * @public
   * @override
   */
  proto.g8e.operator.v1.EnvironmentDetails.displayName = 'proto.g8e.operator.v1.EnvironmentDetails';
}
/**
 * Generated by JsPbCodeGenerator.
 * @param {Array=} opt_data Optional initial data array, typically from a
 * server response, or constructed directly in Javascript. The array is used
 * in place and becomes part of the constructed object. It is not cloned.
 * If no data is provided, the constructed object will be empty, but still
 * valid.
 * @extends {jspb.Message}
 * @constructor
 */
proto.g8e.operator.v1.FingerprintDetails = function(opt_data) {
  jspb.Message.initialize(this, opt_data, 0, -1, null, null);
};
goog.inherits(proto.g8e.operator.v1.FingerprintDetails, jspb.Message);
if (goog.DEBUG && !COMPILED) {
  /**
   * @public
   * @override
   */
  proto.g8e.operator.v1.FingerprintDetails.displayName = 'proto.g8e.operator.v1.FingerprintDetails';
}
/**
 * Generated by JsPbCodeGenerator.
 * @param {Array=} opt_data Optional initial data array, typically from a
 * server response, or constructed directly in Javascript. The array is used
 * in place and becomes part of the constructed object. It is not cloned.
 * If no data is provided, the constructed object will be empty, but still
 * valid.
 * @extends {jspb.Message}
 * @constructor
 */
proto.g8e.operator.v1.PasskeyCredential = function(opt_data) {
  jspb.Message.initialize(this, opt_data, 0, -1, proto.g8e.operator.v1.PasskeyCredential.repeatedFields_, null);
};
goog.inherits(proto.g8e.operator.v1.PasskeyCredential, jspb.Message);
if (goog.DEBUG && !COMPILED) {
  /**
   * @public
   * @override
   */
  proto.g8e.operator.v1.PasskeyCredential.displayName = 'proto.g8e.operator.v1.PasskeyCredential';
}
/**
 * Generated by JsPbCodeGenerator.
 * @param {Array=} opt_data Optional initial data array, typically from a
 * server response, or constructed directly in Javascript. The array is used
 * in place and becomes part of the constructed object. It is not cloned.
 * If no data is provided, the constructed object will be empty, but still
 * valid.
 * @extends {jspb.Message}
 * @constructor
 */
proto.g8e.operator.v1.PasskeyRegisterChallengeRequested = function(opt_data) {
  jspb.Message.initialize(this, opt_data, 0, -1, null, null);
};
goog.inherits(proto.g8e.operator.v1.PasskeyRegisterChallengeRequested, jspb.Message);
if (goog.DEBUG && !COMPILED) {
  /**
   * @public
   * @override
   */
  proto.g8e.operator.v1.PasskeyRegisterChallengeRequested.displayName = 'proto.g8e.operator.v1.PasskeyRegisterChallengeRequested';
}
/**
 * Generated by JsPbCodeGenerator.
 * @param {Array=} opt_data Optional initial data array, typically from a
 * server response, or constructed directly in Javascript. The array is used
 * in place and becomes part of the constructed object. It is not cloned.
 * If no data is provided, the constructed object will be empty, but still
 * valid.
 * @extends {jspb.Message}
 * @constructor
 */
proto.g8e.operator.v1.PasskeyRegisterChallengeResult = function(opt_data) {
  jspb.Message.initialize(this, opt_data, 0, -1, proto.g8e.operator.v1.PasskeyRegisterChallengeResult.repeatedFields_, null);
};
goog.inherits(proto.g8e.operator.v1.PasskeyRegisterChallengeResult, jspb.Message);
if (goog.DEBUG && !COMPILED) {
  /**
   * @public
   * @override
   */
  proto.g8e.operator.v1.PasskeyRegisterChallengeResult.displayName = 'proto.g8e.operator.v1.PasskeyRegisterChallengeResult';
}
/**
 * Generated by JsPbCodeGenerator.
 * @param {Array=} opt_data Optional initial data array, typically from a
 * server response, or constructed directly in Javascript. The array is used
 * in place and becomes part of the constructed object. It is not cloned.
 * If no data is provided, the constructed object will be empty, but still
 * valid.
 * @extends {jspb.Message}
 * @constructor
 */
proto.g8e.operator.v1.PasskeyRegisterChallengeResult.RelyingParty = function(opt_data) {
  jspb.Message.initialize(this, opt_data, 0, -1, null, null);
};
goog.inherits(proto.g8e.operator.v1.PasskeyRegisterChallengeResult.RelyingParty, jspb.Message);
if (goog.DEBUG && !COMPILED) {
  /**
   * @public
   * @override
   */
  proto.g8e.operator.v1.PasskeyRegisterChallengeResult.RelyingParty.displayName = 'proto.g8e.operator.v1.PasskeyRegisterChallengeResult.RelyingParty';
}
/**
 * Generated by JsPbCodeGenerator.
 * @param {Array=} opt_data Optional initial data array, typically from a
 * server response, or constructed directly in Javascript. The array is used
 * in place and becomes part of the constructed object. It is not cloned.
 * If no data is provided, the constructed object will be empty, but still
 * valid.
 * @extends {jspb.Message}
 * @constructor
 */
proto.g8e.operator.v1.PasskeyRegisterChallengeResult.UserInfo = function(opt_data) {
  jspb.Message.initialize(this, opt_data, 0, -1, null, null);
};
goog.inherits(proto.g8e.operator.v1.PasskeyRegisterChallengeResult.UserInfo, jspb.Message);
if (goog.DEBUG && !COMPILED) {
  /**
   * @public
   * @override
   */
  proto.g8e.operator.v1.PasskeyRegisterChallengeResult.UserInfo.displayName = 'proto.g8e.operator.v1.PasskeyRegisterChallengeResult.UserInfo';
}
/**
 * Generated by JsPbCodeGenerator.
 * @param {Array=} opt_data Optional initial data array, typically from a
 * server response, or constructed directly in Javascript. The array is used
 * in place and becomes part of the constructed object. It is not cloned.
 * If no data is provided, the constructed object will be empty, but still
 * valid.
 * @extends {jspb.Message}
 * @constructor
 */
proto.g8e.operator.v1.PasskeyRegisterChallengeResult.PublicKeyCredentialParameters = function(opt_data) {
  jspb.Message.initialize(this, opt_data, 0, -1, null, null);
};
goog.inherits(proto.g8e.operator.v1.PasskeyRegisterChallengeResult.PublicKeyCredentialParameters, jspb.Message);
if (goog.DEBUG && !COMPILED) {
  /**
   * @public
   * @override
   */
  proto.g8e.operator.v1.PasskeyRegisterChallengeResult.PublicKeyCredentialParameters.displayName = 'proto.g8e.operator.v1.PasskeyRegisterChallengeResult.PublicKeyCredentialParameters';
}
/**
 * Generated by JsPbCodeGenerator.
 * @param {Array=} opt_data Optional initial data array, typically from a
 * server response, or constructed directly in Javascript. The array is used
 * in place and becomes part of the constructed object. It is not cloned.
 * If no data is provided, the constructed object will be empty, but still
 * valid.
 * @extends {jspb.Message}
 * @constructor
 */
proto.g8e.operator.v1.PasskeyRegisterChallengeResult.AuthenticatorSelection = function(opt_data) {
  jspb.Message.initialize(this, opt_data, 0, -1, null, null);
};
goog.inherits(proto.g8e.operator.v1.PasskeyRegisterChallengeResult.AuthenticatorSelection, jspb.Message);
if (goog.DEBUG && !COMPILED) {
  /**
   * @public
   * @override
   */
  proto.g8e.operator.v1.PasskeyRegisterChallengeResult.AuthenticatorSelection.displayName = 'proto.g8e.operator.v1.PasskeyRegisterChallengeResult.AuthenticatorSelection';
}
/**
 * Generated by JsPbCodeGenerator.
 * @param {Array=} opt_data Optional initial data array, typically from a
 * server response, or constructed directly in Javascript. The array is used
 * in place and becomes part of the constructed object. It is not cloned.
 * If no data is provided, the constructed object will be empty, but still
 * valid.
 * @extends {jspb.Message}
 * @constructor
 */
proto.g8e.operator.v1.AttestationResponse = function(opt_data) {
  jspb.Message.initialize(this, opt_data, 0, -1, proto.g8e.operator.v1.AttestationResponse.repeatedFields_, null);
};
goog.inherits(proto.g8e.operator.v1.AttestationResponse, jspb.Message);
if (goog.DEBUG && !COMPILED) {
  /**
   * @public
   * @override
   */
  proto.g8e.operator.v1.AttestationResponse.displayName = 'proto.g8e.operator.v1.AttestationResponse';
}
/**
 * Generated by JsPbCodeGenerator.
 * @param {Array=} opt_data Optional initial data array, typically from a
 * server response, or constructed directly in Javascript. The array is used
 * in place and becomes part of the constructed object. It is not cloned.
 * If no data is provided, the constructed object will be empty, but still
 * valid.
 * @extends {jspb.Message}
 * @constructor
 */
proto.g8e.operator.v1.PasskeyRegisterVerifyRequested = function(opt_data) {
  jspb.Message.initialize(this, opt_data, 0, -1, null, null);
};
goog.inherits(proto.g8e.operator.v1.PasskeyRegisterVerifyRequested, jspb.Message);
if (goog.DEBUG && !COMPILED) {
  /**
   * @public
   * @override
   */
  proto.g8e.operator.v1.PasskeyRegisterVerifyRequested.displayName = 'proto.g8e.operator.v1.PasskeyRegisterVerifyRequested';
}
/**
 * Generated by JsPbCodeGenerator.
 * @param {Array=} opt_data Optional initial data array, typically from a
 * server response, or constructed directly in Javascript. The array is used
 * in place and becomes part of the constructed object. It is not cloned.
 * If no data is provided, the constructed object will be empty, but still
 * valid.
 * @extends {jspb.Message}
 * @constructor
 */
proto.g8e.operator.v1.PasskeyRegisterVerifyResult = function(opt_data) {
  jspb.Message.initialize(this, opt_data, 0, -1, null, null);
};
goog.inherits(proto.g8e.operator.v1.PasskeyRegisterVerifyResult, jspb.Message);
if (goog.DEBUG && !COMPILED) {
  /**
   * @public
   * @override
   */
  proto.g8e.operator.v1.PasskeyRegisterVerifyResult.displayName = 'proto.g8e.operator.v1.PasskeyRegisterVerifyResult';
}
/**
 * Generated by JsPbCodeGenerator.
 * @param {Array=} opt_data Optional initial data array, typically from a
 * server response, or constructed directly in Javascript. The array is used
 * in place and becomes part of the constructed object. It is not cloned.
 * If no data is provided, the constructed object will be empty, but still
 * valid.
 * @extends {jspb.Message}
 * @constructor
 */
proto.g8e.operator.v1.PasskeyAuthChallengeRequested = function(opt_data) {
  jspb.Message.initialize(this, opt_data, 0, -1, null, null);
};
goog.inherits(proto.g8e.operator.v1.PasskeyAuthChallengeRequested, jspb.Message);
if (goog.DEBUG && !COMPILED) {
  /**
   * @public
   * @override
   */
  proto.g8e.operator.v1.PasskeyAuthChallengeRequested.displayName = 'proto.g8e.operator.v1.PasskeyAuthChallengeRequested';
}
/**
 * Generated by JsPbCodeGenerator.
 * @param {Array=} opt_data Optional initial data array, typically from a
 * server response, or constructed directly in Javascript. The array is used
 * in place and becomes part of the constructed object. It is not cloned.
 * If no data is provided, the constructed object will be empty, but still
 * valid.
 * @extends {jspb.Message}
 * @constructor
 */
proto.g8e.operator.v1.PasskeyAuthChallengeResult = function(opt_data) {
  jspb.Message.initialize(this, opt_data, 0, -1, proto.g8e.operator.v1.PasskeyAuthChallengeResult.repeatedFields_, null);
};
goog.inherits(proto.g8e.operator.v1.PasskeyAuthChallengeResult, jspb.Message);
if (goog.DEBUG && !COMPILED) {
  /**
   * @public
   * @override
   */
  proto.g8e.operator.v1.PasskeyAuthChallengeResult.displayName = 'proto.g8e.operator.v1.PasskeyAuthChallengeResult';
}
/**
 * Generated by JsPbCodeGenerator.
 * @param {Array=} opt_data Optional initial data array, typically from a
 * server response, or constructed directly in Javascript. The array is used
 * in place and becomes part of the constructed object. It is not cloned.
 * If no data is provided, the constructed object will be empty, but still
 * valid.
 * @extends {jspb.Message}
 * @constructor
 */
proto.g8e.operator.v1.AssertionResponse = function(opt_data) {
  jspb.Message.initialize(this, opt_data, 0, -1, null, null);
};
goog.inherits(proto.g8e.operator.v1.AssertionResponse, jspb.Message);
if (goog.DEBUG && !COMPILED) {
  /**
   * @public
   * @override
   */
  proto.g8e.operator.v1.AssertionResponse.displayName = 'proto.g8e.operator.v1.AssertionResponse';
}
/**
 * Generated by JsPbCodeGenerator.
 * @param {Array=} opt_data Optional initial data array, typically from a
 * server response, or constructed directly in Javascript. The array is used
 * in place and becomes part of the constructed object. It is not cloned.
 * If no data is provided, the constructed object will be empty, but still
 * valid.
 * @extends {jspb.Message}
 * @constructor
 */
proto.g8e.operator.v1.PasskeyAuthVerifyRequested = function(opt_data) {
  jspb.Message.initialize(this, opt_data, 0, -1, null, null);
};
goog.inherits(proto.g8e.operator.v1.PasskeyAuthVerifyRequested, jspb.Message);
if (goog.DEBUG && !COMPILED) {
  /**
   * @public
   * @override
   */
  proto.g8e.operator.v1.PasskeyAuthVerifyRequested.displayName = 'proto.g8e.operator.v1.PasskeyAuthVerifyRequested';
}
/**
 * Generated by JsPbCodeGenerator.
 * @param {Array=} opt_data Optional initial data array, typically from a
 * server response, or constructed directly in Javascript. The array is used
 * in place and becomes part of the constructed object. It is not cloned.
 * If no data is provided, the constructed object will be empty, but still
 * valid.
 * @extends {jspb.Message}
 * @constructor
 */
proto.g8e.operator.v1.PasskeyAuthVerifyResult = function(opt_data) {
  jspb.Message.initialize(this, opt_data, 0, -1, null, null);
};
goog.inherits(proto.g8e.operator.v1.PasskeyAuthVerifyResult, jspb.Message);
if (goog.DEBUG && !COMPILED) {
  /**
   * @public
   * @override
   */
  proto.g8e.operator.v1.PasskeyAuthVerifyResult.displayName = 'proto.g8e.operator.v1.PasskeyAuthVerifyResult';
}
/**
 * Generated by JsPbCodeGenerator.
 * @param {Array=} opt_data Optional initial data array, typically from a
 * server response, or constructed directly in Javascript. The array is used
 * in place and becomes part of the constructed object. It is not cloned.
 * If no data is provided, the constructed object will be empty, but still
 * valid.
 * @extends {jspb.Message}
 * @constructor
 */
proto.g8e.operator.v1.ListPasskeyCredentialsRequested = function(opt_data) {
  jspb.Message.initialize(this, opt_data, 0, -1, null, null);
};
goog.inherits(proto.g8e.operator.v1.ListPasskeyCredentialsRequested, jspb.Message);
if (goog.DEBUG && !COMPILED) {
  /**
   * @public
   * @override
   */
  proto.g8e.operator.v1.ListPasskeyCredentialsRequested.displayName = 'proto.g8e.operator.v1.ListPasskeyCredentialsRequested';
}
/**
 * Generated by JsPbCodeGenerator.
 * @param {Array=} opt_data Optional initial data array, typically from a
 * server response, or constructed directly in Javascript. The array is used
 * in place and becomes part of the constructed object. It is not cloned.
 * If no data is provided, the constructed object will be empty, but still
 * valid.
 * @extends {jspb.Message}
 * @constructor
 */
proto.g8e.operator.v1.ListPasskeyCredentialsResult = function(opt_data) {
  jspb.Message.initialize(this, opt_data, 0, -1, proto.g8e.operator.v1.ListPasskeyCredentialsResult.repeatedFields_, null);
};
goog.inherits(proto.g8e.operator.v1.ListPasskeyCredentialsResult, jspb.Message);
if (goog.DEBUG && !COMPILED) {
  /**
   * @public
   * @override
   */
  proto.g8e.operator.v1.ListPasskeyCredentialsResult.displayName = 'proto.g8e.operator.v1.ListPasskeyCredentialsResult';
}
/**
 * Generated by JsPbCodeGenerator.
 * @param {Array=} opt_data Optional initial data array, typically from a
 * server response, or constructed directly in Javascript. The array is used
 * in place and becomes part of the constructed object. It is not cloned.
 * If no data is provided, the constructed object will be empty, but still
 * valid.
 * @extends {jspb.Message}
 * @constructor
 */
proto.g8e.operator.v1.RevokePasskeyCredentialRequested = function(opt_data) {
  jspb.Message.initialize(this, opt_data, 0, -1, null, null);
};
goog.inherits(proto.g8e.operator.v1.RevokePasskeyCredentialRequested, jspb.Message);
if (goog.DEBUG && !COMPILED) {
  /**
   * @public
   * @override
   */
  proto.g8e.operator.v1.RevokePasskeyCredentialRequested.displayName = 'proto.g8e.operator.v1.RevokePasskeyCredentialRequested';
}
/**
 * Generated by JsPbCodeGenerator.
 * @param {Array=} opt_data Optional initial data array, typically from a
 * server response, or constructed directly in Javascript. The array is used
 * in place and becomes part of the constructed object. It is not cloned.
 * If no data is provided, the constructed object will be empty, but still
 * valid.
 * @extends {jspb.Message}
 * @constructor
 */
proto.g8e.operator.v1.RevokePasskeyCredentialResult = function(opt_data) {
  jspb.Message.initialize(this, opt_data, 0, -1, null, null);
};
goog.inherits(proto.g8e.operator.v1.RevokePasskeyCredentialResult, jspb.Message);
if (goog.DEBUG && !COMPILED) {
  /**
   * @public
   * @override
   */
  proto.g8e.operator.v1.RevokePasskeyCredentialResult.displayName = 'proto.g8e.operator.v1.RevokePasskeyCredentialResult';
}



if (jspb.Message.GENERATE_TO_OBJECT) {
/**
 * Creates an object representation of this proto.
 * Field names that are reserved in JavaScript and will be renamed to pb_name.
 * Optional fields that are not set will be set to undefined.
 * To access a reserved field use, foo.pb_<name>, eg, foo.pb_default.
 * For the list of reserved names please see:
 *     net/proto2/compiler/js/internal/generator.cc#kKeyword.
 * @param {boolean=} opt_includeInstance Deprecated. whether to include the
 *     JSPB instance for transitional soy proto support:
 *     http://goto/soy-param-migration
 * @return {!Object}
 */
proto.g8e.operator.v1.CommandRequested.prototype.toObject = function(opt_includeInstance) {
  return proto.g8e.operator.v1.CommandRequested.toObject(opt_includeInstance, this);
};


/**
 * Static version of the {@see toObject} method.
 * @param {boolean|undefined} includeInstance Deprecated. Whether to include
 *     the JSPB instance for transitional soy proto support:
 *     http://goto/soy-param-migration
 * @param {!proto.g8e.operator.v1.CommandRequested} msg The msg instance to transform.
 * @return {!Object}
 * @suppress {unusedLocalVariables} f is only used for nested messages
 */
proto.g8e.operator.v1.CommandRequested.toObject = function(includeInstance, msg) {
  var f, obj = {
    command: jspb.Message.getFieldWithDefault(msg, 1, ""),
    executionId: jspb.Message.getFieldWithDefault(msg, 2, ""),
    justification: jspb.Message.getFieldWithDefault(msg, 3, ""),
    sentinelMode: jspb.Message.getFieldWithDefault(msg, 4, ""),
    timeoutSeconds: jspb.Message.getFieldWithDefault(msg, 5, 0),
    intent: jspb.Message.getFieldWithDefault(msg, 6, ""),
    environmentMap: (f = msg.getEnvironmentMap()) ? f.toObject(includeInstance, undefined) : [],
    workingDirectory: jspb.Message.getFieldWithDefault(msg, 8, "")
  };

  if (includeInstance) {
    obj.$jspbMessageInstance = msg;
  }
  return obj;
};
}


/**
 * Deserializes binary data (in protobuf wire format).
 * @param {jspb.ByteSource} bytes The bytes to deserialize.
 * @return {!proto.g8e.operator.v1.CommandRequested}
 */
proto.g8e.operator.v1.CommandRequested.deserializeBinary = function(bytes) {
  var reader = new jspb.BinaryReader(bytes);
  var msg = new proto.g8e.operator.v1.CommandRequested;
  return proto.g8e.operator.v1.CommandRequested.deserializeBinaryFromReader(msg, reader);
};


/**
 * Deserializes binary data (in protobuf wire format) from the
 * given reader into the given message object.
 * @param {!proto.g8e.operator.v1.CommandRequested} msg The message object to deserialize into.
 * @param {!jspb.BinaryReader} reader The BinaryReader to use.
 * @return {!proto.g8e.operator.v1.CommandRequested}
 */
proto.g8e.operator.v1.CommandRequested.deserializeBinaryFromReader = function(msg, reader) {
  while (reader.nextField()) {
    if (reader.isEndGroup()) {
      break;
    }
    var field = reader.getFieldNumber();
    switch (field) {
    case 1:
      var value = /** @type {string} */ (reader.readString());
      msg.setCommand(value);
      break;
    case 2:
      var value = /** @type {string} */ (reader.readString());
      msg.setExecutionId(value);
      break;
    case 3:
      var value = /** @type {string} */ (reader.readString());
      msg.setJustification(value);
      break;
    case 4:
      var value = /** @type {string} */ (reader.readString());
      msg.setSentinelMode(value);
      break;
    case 5:
      var value = /** @type {number} */ (reader.readInt32());
      msg.setTimeoutSeconds(value);
      break;
    case 6:
      var value = /** @type {string} */ (reader.readString());
      msg.setIntent(value);
      break;
    case 7:
      var value = msg.getEnvironmentMap();
      reader.readMessage(value, function(message, reader) {
        jspb.Map.deserializeBinary(message, reader, jspb.BinaryReader.prototype.readString, jspb.BinaryReader.prototype.readString, null, "", "");
         });
      break;
    case 8:
      var value = /** @type {string} */ (reader.readString());
      msg.setWorkingDirectory(value);
      break;
    default:
      reader.skipField();
      break;
    }
  }
  return msg;
};


/**
 * Serializes the message to binary data (in protobuf wire format).
 * @return {!Uint8Array}
 */
proto.g8e.operator.v1.CommandRequested.prototype.serializeBinary = function() {
  var writer = new jspb.BinaryWriter();
  proto.g8e.operator.v1.CommandRequested.serializeBinaryToWriter(this, writer);
  return writer.getResultBuffer();
};


/**
 * Serializes the given message to binary data (in protobuf wire
 * format), writing to the given BinaryWriter.
 * @param {!proto.g8e.operator.v1.CommandRequested} message
 * @param {!jspb.BinaryWriter} writer
 * @suppress {unusedLocalVariables} f is only used for nested messages
 */
proto.g8e.operator.v1.CommandRequested.serializeBinaryToWriter = function(message, writer) {
  var f = undefined;
  f = message.getCommand();
  if (f.length > 0) {
    writer.writeString(
      1,
      f
    );
  }
  f = message.getExecutionId();
  if (f.length > 0) {
    writer.writeString(
      2,
      f
    );
  }
  f = message.getJustification();
  if (f.length > 0) {
    writer.writeString(
      3,
      f
    );
  }
  f = message.getSentinelMode();
  if (f.length > 0) {
    writer.writeString(
      4,
      f
    );
  }
  f = message.getTimeoutSeconds();
  if (f !== 0) {
    writer.writeInt32(
      5,
      f
    );
  }
  f = message.getIntent();
  if (f.length > 0) {
    writer.writeString(
      6,
      f
    );
  }
  f = message.getEnvironmentMap(true);
  if (f && f.getLength() > 0) {
    f.serializeBinary(7, writer, jspb.BinaryWriter.prototype.writeString, jspb.BinaryWriter.prototype.writeString);
  }
  f = message.getWorkingDirectory();
  if (f.length > 0) {
    writer.writeString(
      8,
      f
    );
  }
};


/**
 * optional string command = 1;
 * @return {string}
 */
proto.g8e.operator.v1.CommandRequested.prototype.getCommand = function() {
  return /** @type {string} */ (jspb.Message.getFieldWithDefault(this, 1, ""));
};


/**
 * @param {string} value
 * @return {!proto.g8e.operator.v1.CommandRequested} returns this
 */
proto.g8e.operator.v1.CommandRequested.prototype.setCommand = function(value) {
  return jspb.Message.setProto3StringField(this, 1, value);
};


/**
 * optional string execution_id = 2;
 * @return {string}
 */
proto.g8e.operator.v1.CommandRequested.prototype.getExecutionId = function() {
  return /** @type {string} */ (jspb.Message.getFieldWithDefault(this, 2, ""));
};


/**
 * @param {string} value
 * @return {!proto.g8e.operator.v1.CommandRequested} returns this
 */
proto.g8e.operator.v1.CommandRequested.prototype.setExecutionId = function(value) {
  return jspb.Message.setProto3StringField(this, 2, value);
};


/**
 * optional string justification = 3;
 * @return {string}
 */
proto.g8e.operator.v1.CommandRequested.prototype.getJustification = function() {
  return /** @type {string} */ (jspb.Message.getFieldWithDefault(this, 3, ""));
};


/**
 * @param {string} value
 * @return {!proto.g8e.operator.v1.CommandRequested} returns this
 */
proto.g8e.operator.v1.CommandRequested.prototype.setJustification = function(value) {
  return jspb.Message.setProto3StringField(this, 3, value);
};


/**
 * optional string sentinel_mode = 4;
 * @return {string}
 */
proto.g8e.operator.v1.CommandRequested.prototype.getSentinelMode = function() {
  return /** @type {string} */ (jspb.Message.getFieldWithDefault(this, 4, ""));
};


/**
 * @param {string} value
 * @return {!proto.g8e.operator.v1.CommandRequested} returns this
 */
proto.g8e.operator.v1.CommandRequested.prototype.setSentinelMode = function(value) {
  return jspb.Message.setProto3StringField(this, 4, value);
};


/**
 * optional int32 timeout_seconds = 5;
 * @return {number}
 */
proto.g8e.operator.v1.CommandRequested.prototype.getTimeoutSeconds = function() {
  return /** @type {number} */ (jspb.Message.getFieldWithDefault(this, 5, 0));
};


/**
 * @param {number} value
 * @return {!proto.g8e.operator.v1.CommandRequested} returns this
 */
proto.g8e.operator.v1.CommandRequested.prototype.setTimeoutSeconds = function(value) {
  return jspb.Message.setProto3IntField(this, 5, value);
};


/**
 * optional string intent = 6;
 * @return {string}
 */
proto.g8e.operator.v1.CommandRequested.prototype.getIntent = function() {
  return /** @type {string} */ (jspb.Message.getFieldWithDefault(this, 6, ""));
};


/**
 * @param {string} value
 * @return {!proto.g8e.operator.v1.CommandRequested} returns this
 */
proto.g8e.operator.v1.CommandRequested.prototype.setIntent = function(value) {
  return jspb.Message.setProto3StringField(this, 6, value);
};


/**
 * map<string, string> environment = 7;
 * @param {boolean=} opt_noLazyCreate Do not create the map if
 * empty, instead returning `undefined`
 * @return {!jspb.Map<string,string>}
 */
proto.g8e.operator.v1.CommandRequested.prototype.getEnvironmentMap = function(opt_noLazyCreate) {
  return /** @type {!jspb.Map<string,string>} */ (
      jspb.Message.getMapField(this, 7, opt_noLazyCreate,
      null));
};


/**
 * Clears values from the map. The map will be non-null.
 * @return {!proto.g8e.operator.v1.CommandRequested} returns this
 */
proto.g8e.operator.v1.CommandRequested.prototype.clearEnvironmentMap = function() {
  this.getEnvironmentMap().clear();
  return this;
};


/**
 * optional string working_directory = 8;
 * @return {string}
 */
proto.g8e.operator.v1.CommandRequested.prototype.getWorkingDirectory = function() {
  return /** @type {string} */ (jspb.Message.getFieldWithDefault(this, 8, ""));
};


/**
 * @param {string} value
 * @return {!proto.g8e.operator.v1.CommandRequested} returns this
 */
proto.g8e.operator.v1.CommandRequested.prototype.setWorkingDirectory = function(value) {
  return jspb.Message.setProto3StringField(this, 8, value);
};





if (jspb.Message.GENERATE_TO_OBJECT) {
/**
 * Creates an object representation of this proto.
 * Field names that are reserved in JavaScript and will be renamed to pb_name.
 * Optional fields that are not set will be set to undefined.
 * To access a reserved field use, foo.pb_<name>, eg, foo.pb_default.
 * For the list of reserved names please see:
 *     net/proto2/compiler/js/internal/generator.cc#kKeyword.
 * @param {boolean=} opt_includeInstance Deprecated. whether to include the
 *     JSPB instance for transitional soy proto support:
 *     http://goto/soy-param-migration
 * @return {!Object}
 */
proto.g8e.operator.v1.CommandCancelRequested.prototype.toObject = function(opt_includeInstance) {
  return proto.g8e.operator.v1.CommandCancelRequested.toObject(opt_includeInstance, this);
};


/**
 * Static version of the {@see toObject} method.
 * @param {boolean|undefined} includeInstance Deprecated. Whether to include
 *     the JSPB instance for transitional soy proto support:
 *     http://goto/soy-param-migration
 * @param {!proto.g8e.operator.v1.CommandCancelRequested} msg The msg instance to transform.
 * @return {!Object}
 * @suppress {unusedLocalVariables} f is only used for nested messages
 */
proto.g8e.operator.v1.CommandCancelRequested.toObject = function(includeInstance, msg) {
  var f, obj = {
    executionId: jspb.Message.getFieldWithDefault(msg, 1, "")
  };

  if (includeInstance) {
    obj.$jspbMessageInstance = msg;
  }
  return obj;
};
}


/**
 * Deserializes binary data (in protobuf wire format).
 * @param {jspb.ByteSource} bytes The bytes to deserialize.
 * @return {!proto.g8e.operator.v1.CommandCancelRequested}
 */
proto.g8e.operator.v1.CommandCancelRequested.deserializeBinary = function(bytes) {
  var reader = new jspb.BinaryReader(bytes);
  var msg = new proto.g8e.operator.v1.CommandCancelRequested;
  return proto.g8e.operator.v1.CommandCancelRequested.deserializeBinaryFromReader(msg, reader);
};


/**
 * Deserializes binary data (in protobuf wire format) from the
 * given reader into the given message object.
 * @param {!proto.g8e.operator.v1.CommandCancelRequested} msg The message object to deserialize into.
 * @param {!jspb.BinaryReader} reader The BinaryReader to use.
 * @return {!proto.g8e.operator.v1.CommandCancelRequested}
 */
proto.g8e.operator.v1.CommandCancelRequested.deserializeBinaryFromReader = function(msg, reader) {
  while (reader.nextField()) {
    if (reader.isEndGroup()) {
      break;
    }
    var field = reader.getFieldNumber();
    switch (field) {
    case 1:
      var value = /** @type {string} */ (reader.readString());
      msg.setExecutionId(value);
      break;
    default:
      reader.skipField();
      break;
    }
  }
  return msg;
};


/**
 * Serializes the message to binary data (in protobuf wire format).
 * @return {!Uint8Array}
 */
proto.g8e.operator.v1.CommandCancelRequested.prototype.serializeBinary = function() {
  var writer = new jspb.BinaryWriter();
  proto.g8e.operator.v1.CommandCancelRequested.serializeBinaryToWriter(this, writer);
  return writer.getResultBuffer();
};


/**
 * Serializes the given message to binary data (in protobuf wire
 * format), writing to the given BinaryWriter.
 * @param {!proto.g8e.operator.v1.CommandCancelRequested} message
 * @param {!jspb.BinaryWriter} writer
 * @suppress {unusedLocalVariables} f is only used for nested messages
 */
proto.g8e.operator.v1.CommandCancelRequested.serializeBinaryToWriter = function(message, writer) {
  var f = undefined;
  f = message.getExecutionId();
  if (f.length > 0) {
    writer.writeString(
      1,
      f
    );
  }
};


/**
 * optional string execution_id = 1;
 * @return {string}
 */
proto.g8e.operator.v1.CommandCancelRequested.prototype.getExecutionId = function() {
  return /** @type {string} */ (jspb.Message.getFieldWithDefault(this, 1, ""));
};


/**
 * @param {string} value
 * @return {!proto.g8e.operator.v1.CommandCancelRequested} returns this
 */
proto.g8e.operator.v1.CommandCancelRequested.prototype.setExecutionId = function(value) {
  return jspb.Message.setProto3StringField(this, 1, value);
};





if (jspb.Message.GENERATE_TO_OBJECT) {
/**
 * Creates an object representation of this proto.
 * Field names that are reserved in JavaScript and will be renamed to pb_name.
 * Optional fields that are not set will be set to undefined.
 * To access a reserved field use, foo.pb_<name>, eg, foo.pb_default.
 * For the list of reserved names please see:
 *     net/proto2/compiler/js/internal/generator.cc#kKeyword.
 * @param {boolean=} opt_includeInstance Deprecated. whether to include the
 *     JSPB instance for transitional soy proto support:
 *     http://goto/soy-param-migration
 * @return {!Object}
 */
proto.g8e.operator.v1.FileEditRequested.prototype.toObject = function(opt_includeInstance) {
  return proto.g8e.operator.v1.FileEditRequested.toObject(opt_includeInstance, this);
};


/**
 * Static version of the {@see toObject} method.
 * @param {boolean|undefined} includeInstance Deprecated. Whether to include
 *     the JSPB instance for transitional soy proto support:
 *     http://goto/soy-param-migration
 * @param {!proto.g8e.operator.v1.FileEditRequested} msg The msg instance to transform.
 * @return {!Object}
 * @suppress {unusedLocalVariables} f is only used for nested messages
 */
proto.g8e.operator.v1.FileEditRequested.toObject = function(includeInstance, msg) {
  var f, obj = {
    filePath: jspb.Message.getFieldWithDefault(msg, 1, ""),
    operation: jspb.Message.getFieldWithDefault(msg, 2, ""),
    executionId: jspb.Message.getFieldWithDefault(msg, 3, ""),
    justification: jspb.Message.getFieldWithDefault(msg, 4, ""),
    content: jspb.Message.getFieldWithDefault(msg, 5, ""),
    oldContent: jspb.Message.getFieldWithDefault(msg, 6, ""),
    newContent: jspb.Message.getFieldWithDefault(msg, 7, ""),
    insertContent: jspb.Message.getFieldWithDefault(msg, 8, ""),
    insertPosition: jspb.Message.getFieldWithDefault(msg, 9, 0),
    startLine: jspb.Message.getFieldWithDefault(msg, 10, 0),
    endLine: jspb.Message.getFieldWithDefault(msg, 11, 0),
    patchContent: jspb.Message.getFieldWithDefault(msg, 12, ""),
    createBackup: jspb.Message.getBooleanFieldWithDefault(msg, 13, false),
    createIfMissing: jspb.Message.getBooleanFieldWithDefault(msg, 14, false),
    sentinelMode: jspb.Message.getFieldWithDefault(msg, 15, "")
  };

  if (includeInstance) {
    obj.$jspbMessageInstance = msg;
  }
  return obj;
};
}


/**
 * Deserializes binary data (in protobuf wire format).
 * @param {jspb.ByteSource} bytes The bytes to deserialize.
 * @return {!proto.g8e.operator.v1.FileEditRequested}
 */
proto.g8e.operator.v1.FileEditRequested.deserializeBinary = function(bytes) {
  var reader = new jspb.BinaryReader(bytes);
  var msg = new proto.g8e.operator.v1.FileEditRequested;
  return proto.g8e.operator.v1.FileEditRequested.deserializeBinaryFromReader(msg, reader);
};


/**
 * Deserializes binary data (in protobuf wire format) from the
 * given reader into the given message object.
 * @param {!proto.g8e.operator.v1.FileEditRequested} msg The message object to deserialize into.
 * @param {!jspb.BinaryReader} reader The BinaryReader to use.
 * @return {!proto.g8e.operator.v1.FileEditRequested}
 */
proto.g8e.operator.v1.FileEditRequested.deserializeBinaryFromReader = function(msg, reader) {
  while (reader.nextField()) {
    if (reader.isEndGroup()) {
      break;
    }
    var field = reader.getFieldNumber();
    switch (field) {
    case 1:
      var value = /** @type {string} */ (reader.readString());
      msg.setFilePath(value);
      break;
    case 2:
      var value = /** @type {string} */ (reader.readString());
      msg.setOperation(value);
      break;
    case 3:
      var value = /** @type {string} */ (reader.readString());
      msg.setExecutionId(value);
      break;
    case 4:
      var value = /** @type {string} */ (reader.readString());
      msg.setJustification(value);
      break;
    case 5:
      var value = /** @type {string} */ (reader.readString());
      msg.setContent(value);
      break;
    case 6:
      var value = /** @type {string} */ (reader.readString());
      msg.setOldContent(value);
      break;
    case 7:
      var value = /** @type {string} */ (reader.readString());
      msg.setNewContent(value);
      break;
    case 8:
      var value = /** @type {string} */ (reader.readString());
      msg.setInsertContent(value);
      break;
    case 9:
      var value = /** @type {number} */ (reader.readInt32());
      msg.setInsertPosition(value);
      break;
    case 10:
      var value = /** @type {number} */ (reader.readInt32());
      msg.setStartLine(value);
      break;
    case 11:
      var value = /** @type {number} */ (reader.readInt32());
      msg.setEndLine(value);
      break;
    case 12:
      var value = /** @type {string} */ (reader.readString());
      msg.setPatchContent(value);
      break;
    case 13:
      var value = /** @type {boolean} */ (reader.readBool());
      msg.setCreateBackup(value);
      break;
    case 14:
      var value = /** @type {boolean} */ (reader.readBool());
      msg.setCreateIfMissing(value);
      break;
    case 15:
      var value = /** @type {string} */ (reader.readString());
      msg.setSentinelMode(value);
      break;
    default:
      reader.skipField();
      break;
    }
  }
  return msg;
};


/**
 * Serializes the message to binary data (in protobuf wire format).
 * @return {!Uint8Array}
 */
proto.g8e.operator.v1.FileEditRequested.prototype.serializeBinary = function() {
  var writer = new jspb.BinaryWriter();
  proto.g8e.operator.v1.FileEditRequested.serializeBinaryToWriter(this, writer);
  return writer.getResultBuffer();
};


/**
 * Serializes the given message to binary data (in protobuf wire
 * format), writing to the given BinaryWriter.
 * @param {!proto.g8e.operator.v1.FileEditRequested} message
 * @param {!jspb.BinaryWriter} writer
 * @suppress {unusedLocalVariables} f is only used for nested messages
 */
proto.g8e.operator.v1.FileEditRequested.serializeBinaryToWriter = function(message, writer) {
  var f = undefined;
  f = message.getFilePath();
  if (f.length > 0) {
    writer.writeString(
      1,
      f
    );
  }
  f = message.getOperation();
  if (f.length > 0) {
    writer.writeString(
      2,
      f
    );
  }
  f = message.getExecutionId();
  if (f.length > 0) {
    writer.writeString(
      3,
      f
    );
  }
  f = message.getJustification();
  if (f.length > 0) {
    writer.writeString(
      4,
      f
    );
  }
  f = message.getContent();
  if (f.length > 0) {
    writer.writeString(
      5,
      f
    );
  }
  f = message.getOldContent();
  if (f.length > 0) {
    writer.writeString(
      6,
      f
    );
  }
  f = message.getNewContent();
  if (f.length > 0) {
    writer.writeString(
      7,
      f
    );
  }
  f = message.getInsertContent();
  if (f.length > 0) {
    writer.writeString(
      8,
      f
    );
  }
  f = message.getInsertPosition();
  if (f !== 0) {
    writer.writeInt32(
      9,
      f
    );
  }
  f = message.getStartLine();
  if (f !== 0) {
    writer.writeInt32(
      10,
      f
    );
  }
  f = message.getEndLine();
  if (f !== 0) {
    writer.writeInt32(
      11,
      f
    );
  }
  f = message.getPatchContent();
  if (f.length > 0) {
    writer.writeString(
      12,
      f
    );
  }
  f = message.getCreateBackup();
  if (f) {
    writer.writeBool(
      13,
      f
    );
  }
  f = message.getCreateIfMissing();
  if (f) {
    writer.writeBool(
      14,
      f
    );
  }
  f = message.getSentinelMode();
  if (f.length > 0) {
    writer.writeString(
      15,
      f
    );
  }
};


/**
 * optional string file_path = 1;
 * @return {string}
 */
proto.g8e.operator.v1.FileEditRequested.prototype.getFilePath = function() {
  return /** @type {string} */ (jspb.Message.getFieldWithDefault(this, 1, ""));
};


/**
 * @param {string} value
 * @return {!proto.g8e.operator.v1.FileEditRequested} returns this
 */
proto.g8e.operator.v1.FileEditRequested.prototype.setFilePath = function(value) {
  return jspb.Message.setProto3StringField(this, 1, value);
};


/**
 * optional string operation = 2;
 * @return {string}
 */
proto.g8e.operator.v1.FileEditRequested.prototype.getOperation = function() {
  return /** @type {string} */ (jspb.Message.getFieldWithDefault(this, 2, ""));
};


/**
 * @param {string} value
 * @return {!proto.g8e.operator.v1.FileEditRequested} returns this
 */
proto.g8e.operator.v1.FileEditRequested.prototype.setOperation = function(value) {
  return jspb.Message.setProto3StringField(this, 2, value);
};


/**
 * optional string execution_id = 3;
 * @return {string}
 */
proto.g8e.operator.v1.FileEditRequested.prototype.getExecutionId = function() {
  return /** @type {string} */ (jspb.Message.getFieldWithDefault(this, 3, ""));
};


/**
 * @param {string} value
 * @return {!proto.g8e.operator.v1.FileEditRequested} returns this
 */
proto.g8e.operator.v1.FileEditRequested.prototype.setExecutionId = function(value) {
  return jspb.Message.setProto3StringField(this, 3, value);
};


/**
 * optional string justification = 4;
 * @return {string}
 */
proto.g8e.operator.v1.FileEditRequested.prototype.getJustification = function() {
  return /** @type {string} */ (jspb.Message.getFieldWithDefault(this, 4, ""));
};


/**
 * @param {string} value
 * @return {!proto.g8e.operator.v1.FileEditRequested} returns this
 */
proto.g8e.operator.v1.FileEditRequested.prototype.setJustification = function(value) {
  return jspb.Message.setProto3StringField(this, 4, value);
};


/**
 * optional string content = 5;
 * @return {string}
 */
proto.g8e.operator.v1.FileEditRequested.prototype.getContent = function() {
  return /** @type {string} */ (jspb.Message.getFieldWithDefault(this, 5, ""));
};


/**
 * @param {string} value
 * @return {!proto.g8e.operator.v1.FileEditRequested} returns this
 */
proto.g8e.operator.v1.FileEditRequested.prototype.setContent = function(value) {
  return jspb.Message.setProto3StringField(this, 5, value);
};


/**
 * optional string old_content = 6;
 * @return {string}
 */
proto.g8e.operator.v1.FileEditRequested.prototype.getOldContent = function() {
  return /** @type {string} */ (jspb.Message.getFieldWithDefault(this, 6, ""));
};


/**
 * @param {string} value
 * @return {!proto.g8e.operator.v1.FileEditRequested} returns this
 */
proto.g8e.operator.v1.FileEditRequested.prototype.setOldContent = function(value) {
  return jspb.Message.setProto3StringField(this, 6, value);
};


/**
 * optional string new_content = 7;
 * @return {string}
 */
proto.g8e.operator.v1.FileEditRequested.prototype.getNewContent = function() {
  return /** @type {string} */ (jspb.Message.getFieldWithDefault(this, 7, ""));
};


/**
 * @param {string} value
 * @return {!proto.g8e.operator.v1.FileEditRequested} returns this
 */
proto.g8e.operator.v1.FileEditRequested.prototype.setNewContent = function(value) {
  return jspb.Message.setProto3StringField(this, 7, value);
};


/**
 * optional string insert_content = 8;
 * @return {string}
 */
proto.g8e.operator.v1.FileEditRequested.prototype.getInsertContent = function() {
  return /** @type {string} */ (jspb.Message.getFieldWithDefault(this, 8, ""));
};


/**
 * @param {string} value
 * @return {!proto.g8e.operator.v1.FileEditRequested} returns this
 */
proto.g8e.operator.v1.FileEditRequested.prototype.setInsertContent = function(value) {
  return jspb.Message.setProto3StringField(this, 8, value);
};


/**
 * optional int32 insert_position = 9;
 * @return {number}
 */
proto.g8e.operator.v1.FileEditRequested.prototype.getInsertPosition = function() {
  return /** @type {number} */ (jspb.Message.getFieldWithDefault(this, 9, 0));
};


/**
 * @param {number} value
 * @return {!proto.g8e.operator.v1.FileEditRequested} returns this
 */
proto.g8e.operator.v1.FileEditRequested.prototype.setInsertPosition = function(value) {
  return jspb.Message.setProto3IntField(this, 9, value);
};


/**
 * optional int32 start_line = 10;
 * @return {number}
 */
proto.g8e.operator.v1.FileEditRequested.prototype.getStartLine = function() {
  return /** @type {number} */ (jspb.Message.getFieldWithDefault(this, 10, 0));
};


/**
 * @param {number} value
 * @return {!proto.g8e.operator.v1.FileEditRequested} returns this
 */
proto.g8e.operator.v1.FileEditRequested.prototype.setStartLine = function(value) {
  return jspb.Message.setProto3IntField(this, 10, value);
};


/**
 * optional int32 end_line = 11;
 * @return {number}
 */
proto.g8e.operator.v1.FileEditRequested.prototype.getEndLine = function() {
  return /** @type {number} */ (jspb.Message.getFieldWithDefault(this, 11, 0));
};


/**
 * @param {number} value
 * @return {!proto.g8e.operator.v1.FileEditRequested} returns this
 */
proto.g8e.operator.v1.FileEditRequested.prototype.setEndLine = function(value) {
  return jspb.Message.setProto3IntField(this, 11, value);
};


/**
 * optional string patch_content = 12;
 * @return {string}
 */
proto.g8e.operator.v1.FileEditRequested.prototype.getPatchContent = function() {
  return /** @type {string} */ (jspb.Message.getFieldWithDefault(this, 12, ""));
};


/**
 * @param {string} value
 * @return {!proto.g8e.operator.v1.FileEditRequested} returns this
 */
proto.g8e.operator.v1.FileEditRequested.prototype.setPatchContent = function(value) {
  return jspb.Message.setProto3StringField(this, 12, value);
};


/**
 * optional bool create_backup = 13;
 * @return {boolean}
 */
proto.g8e.operator.v1.FileEditRequested.prototype.getCreateBackup = function() {
  return /** @type {boolean} */ (jspb.Message.getBooleanFieldWithDefault(this, 13, false));
};


/**
 * @param {boolean} value
 * @return {!proto.g8e.operator.v1.FileEditRequested} returns this
 */
proto.g8e.operator.v1.FileEditRequested.prototype.setCreateBackup = function(value) {
  return jspb.Message.setProto3BooleanField(this, 13, value);
};


/**
 * optional bool create_if_missing = 14;
 * @return {boolean}
 */
proto.g8e.operator.v1.FileEditRequested.prototype.getCreateIfMissing = function() {
  return /** @type {boolean} */ (jspb.Message.getBooleanFieldWithDefault(this, 14, false));
};


/**
 * @param {boolean} value
 * @return {!proto.g8e.operator.v1.FileEditRequested} returns this
 */
proto.g8e.operator.v1.FileEditRequested.prototype.setCreateIfMissing = function(value) {
  return jspb.Message.setProto3BooleanField(this, 14, value);
};


/**
 * optional string sentinel_mode = 15;
 * @return {string}
 */
proto.g8e.operator.v1.FileEditRequested.prototype.getSentinelMode = function() {
  return /** @type {string} */ (jspb.Message.getFieldWithDefault(this, 15, ""));
};


/**
 * @param {string} value
 * @return {!proto.g8e.operator.v1.FileEditRequested} returns this
 */
proto.g8e.operator.v1.FileEditRequested.prototype.setSentinelMode = function(value) {
  return jspb.Message.setProto3StringField(this, 15, value);
};





if (jspb.Message.GENERATE_TO_OBJECT) {
/**
 * Creates an object representation of this proto.
 * Field names that are reserved in JavaScript and will be renamed to pb_name.
 * Optional fields that are not set will be set to undefined.
 * To access a reserved field use, foo.pb_<name>, eg, foo.pb_default.
 * For the list of reserved names please see:
 *     net/proto2/compiler/js/internal/generator.cc#kKeyword.
 * @param {boolean=} opt_includeInstance Deprecated. whether to include the
 *     JSPB instance for transitional soy proto support:
 *     http://goto/soy-param-migration
 * @return {!Object}
 */
proto.g8e.operator.v1.FsListRequested.prototype.toObject = function(opt_includeInstance) {
  return proto.g8e.operator.v1.FsListRequested.toObject(opt_includeInstance, this);
};


/**
 * Static version of the {@see toObject} method.
 * @param {boolean|undefined} includeInstance Deprecated. Whether to include
 *     the JSPB instance for transitional soy proto support:
 *     http://goto/soy-param-migration
 * @param {!proto.g8e.operator.v1.FsListRequested} msg The msg instance to transform.
 * @return {!Object}
 * @suppress {unusedLocalVariables} f is only used for nested messages
 */
proto.g8e.operator.v1.FsListRequested.toObject = function(includeInstance, msg) {
  var f, obj = {
    path: jspb.Message.getFieldWithDefault(msg, 1, ""),
    executionId: jspb.Message.getFieldWithDefault(msg, 2, ""),
    maxDepth: jspb.Message.getFieldWithDefault(msg, 3, 0),
    maxEntries: jspb.Message.getFieldWithDefault(msg, 4, 0),
    sentinelMode: jspb.Message.getFieldWithDefault(msg, 5, "")
  };

  if (includeInstance) {
    obj.$jspbMessageInstance = msg;
  }
  return obj;
};
}


/**
 * Deserializes binary data (in protobuf wire format).
 * @param {jspb.ByteSource} bytes The bytes to deserialize.
 * @return {!proto.g8e.operator.v1.FsListRequested}
 */
proto.g8e.operator.v1.FsListRequested.deserializeBinary = function(bytes) {
  var reader = new jspb.BinaryReader(bytes);
  var msg = new proto.g8e.operator.v1.FsListRequested;
  return proto.g8e.operator.v1.FsListRequested.deserializeBinaryFromReader(msg, reader);
};


/**
 * Deserializes binary data (in protobuf wire format) from the
 * given reader into the given message object.
 * @param {!proto.g8e.operator.v1.FsListRequested} msg The message object to deserialize into.
 * @param {!jspb.BinaryReader} reader The BinaryReader to use.
 * @return {!proto.g8e.operator.v1.FsListRequested}
 */
proto.g8e.operator.v1.FsListRequested.deserializeBinaryFromReader = function(msg, reader) {
  while (reader.nextField()) {
    if (reader.isEndGroup()) {
      break;
    }
    var field = reader.getFieldNumber();
    switch (field) {
    case 1:
      var value = /** @type {string} */ (reader.readString());
      msg.setPath(value);
      break;
    case 2:
      var value = /** @type {string} */ (reader.readString());
      msg.setExecutionId(value);
      break;
    case 3:
      var value = /** @type {number} */ (reader.readInt32());
      msg.setMaxDepth(value);
      break;
    case 4:
      var value = /** @type {number} */ (reader.readInt32());
      msg.setMaxEntries(value);
      break;
    case 5:
      var value = /** @type {string} */ (reader.readString());
      msg.setSentinelMode(value);
      break;
    default:
      reader.skipField();
      break;
    }
  }
  return msg;
};


/**
 * Serializes the message to binary data (in protobuf wire format).
 * @return {!Uint8Array}
 */
proto.g8e.operator.v1.FsListRequested.prototype.serializeBinary = function() {
  var writer = new jspb.BinaryWriter();
  proto.g8e.operator.v1.FsListRequested.serializeBinaryToWriter(this, writer);
  return writer.getResultBuffer();
};


/**
 * Serializes the given message to binary data (in protobuf wire
 * format), writing to the given BinaryWriter.
 * @param {!proto.g8e.operator.v1.FsListRequested} message
 * @param {!jspb.BinaryWriter} writer
 * @suppress {unusedLocalVariables} f is only used for nested messages
 */
proto.g8e.operator.v1.FsListRequested.serializeBinaryToWriter = function(message, writer) {
  var f = undefined;
  f = message.getPath();
  if (f.length > 0) {
    writer.writeString(
      1,
      f
    );
  }
  f = message.getExecutionId();
  if (f.length > 0) {
    writer.writeString(
      2,
      f
    );
  }
  f = message.getMaxDepth();
  if (f !== 0) {
    writer.writeInt32(
      3,
      f
    );
  }
  f = message.getMaxEntries();
  if (f !== 0) {
    writer.writeInt32(
      4,
      f
    );
  }
  f = message.getSentinelMode();
  if (f.length > 0) {
    writer.writeString(
      5,
      f
    );
  }
};


/**
 * optional string path = 1;
 * @return {string}
 */
proto.g8e.operator.v1.FsListRequested.prototype.getPath = function() {
  return /** @type {string} */ (jspb.Message.getFieldWithDefault(this, 1, ""));
};


/**
 * @param {string} value
 * @return {!proto.g8e.operator.v1.FsListRequested} returns this
 */
proto.g8e.operator.v1.FsListRequested.prototype.setPath = function(value) {
  return jspb.Message.setProto3StringField(this, 1, value);
};


/**
 * optional string execution_id = 2;
 * @return {string}
 */
proto.g8e.operator.v1.FsListRequested.prototype.getExecutionId = function() {
  return /** @type {string} */ (jspb.Message.getFieldWithDefault(this, 2, ""));
};


/**
 * @param {string} value
 * @return {!proto.g8e.operator.v1.FsListRequested} returns this
 */
proto.g8e.operator.v1.FsListRequested.prototype.setExecutionId = function(value) {
  return jspb.Message.setProto3StringField(this, 2, value);
};


/**
 * optional int32 max_depth = 3;
 * @return {number}
 */
proto.g8e.operator.v1.FsListRequested.prototype.getMaxDepth = function() {
  return /** @type {number} */ (jspb.Message.getFieldWithDefault(this, 3, 0));
};


/**
 * @param {number} value
 * @return {!proto.g8e.operator.v1.FsListRequested} returns this
 */
proto.g8e.operator.v1.FsListRequested.prototype.setMaxDepth = function(value) {
  return jspb.Message.setProto3IntField(this, 3, value);
};


/**
 * optional int32 max_entries = 4;
 * @return {number}
 */
proto.g8e.operator.v1.FsListRequested.prototype.getMaxEntries = function() {
  return /** @type {number} */ (jspb.Message.getFieldWithDefault(this, 4, 0));
};


/**
 * @param {number} value
 * @return {!proto.g8e.operator.v1.FsListRequested} returns this
 */
proto.g8e.operator.v1.FsListRequested.prototype.setMaxEntries = function(value) {
  return jspb.Message.setProto3IntField(this, 4, value);
};


/**
 * optional string sentinel_mode = 5;
 * @return {string}
 */
proto.g8e.operator.v1.FsListRequested.prototype.getSentinelMode = function() {
  return /** @type {string} */ (jspb.Message.getFieldWithDefault(this, 5, ""));
};


/**
 * @param {string} value
 * @return {!proto.g8e.operator.v1.FsListRequested} returns this
 */
proto.g8e.operator.v1.FsListRequested.prototype.setSentinelMode = function(value) {
  return jspb.Message.setProto3StringField(this, 5, value);
};





if (jspb.Message.GENERATE_TO_OBJECT) {
/**
 * Creates an object representation of this proto.
 * Field names that are reserved in JavaScript and will be renamed to pb_name.
 * Optional fields that are not set will be set to undefined.
 * To access a reserved field use, foo.pb_<name>, eg, foo.pb_default.
 * For the list of reserved names please see:
 *     net/proto2/compiler/js/internal/generator.cc#kKeyword.
 * @param {boolean=} opt_includeInstance Deprecated. whether to include the
 *     JSPB instance for transitional soy proto support:
 *     http://goto/soy-param-migration
 * @return {!Object}
 */
proto.g8e.operator.v1.FsReadRequested.prototype.toObject = function(opt_includeInstance) {
  return proto.g8e.operator.v1.FsReadRequested.toObject(opt_includeInstance, this);
};


/**
 * Static version of the {@see toObject} method.
 * @param {boolean|undefined} includeInstance Deprecated. Whether to include
 *     the JSPB instance for transitional soy proto support:
 *     http://goto/soy-param-migration
 * @param {!proto.g8e.operator.v1.FsReadRequested} msg The msg instance to transform.
 * @return {!Object}
 * @suppress {unusedLocalVariables} f is only used for nested messages
 */
proto.g8e.operator.v1.FsReadRequested.toObject = function(includeInstance, msg) {
  var f, obj = {
    path: jspb.Message.getFieldWithDefault(msg, 1, ""),
    executionId: jspb.Message.getFieldWithDefault(msg, 2, ""),
    maxSize: jspb.Message.getFieldWithDefault(msg, 3, 0),
    sentinelMode: jspb.Message.getFieldWithDefault(msg, 4, "")
  };

  if (includeInstance) {
    obj.$jspbMessageInstance = msg;
  }
  return obj;
};
}


/**
 * Deserializes binary data (in protobuf wire format).
 * @param {jspb.ByteSource} bytes The bytes to deserialize.
 * @return {!proto.g8e.operator.v1.FsReadRequested}
 */
proto.g8e.operator.v1.FsReadRequested.deserializeBinary = function(bytes) {
  var reader = new jspb.BinaryReader(bytes);
  var msg = new proto.g8e.operator.v1.FsReadRequested;
  return proto.g8e.operator.v1.FsReadRequested.deserializeBinaryFromReader(msg, reader);
};


/**
 * Deserializes binary data (in protobuf wire format) from the
 * given reader into the given message object.
 * @param {!proto.g8e.operator.v1.FsReadRequested} msg The message object to deserialize into.
 * @param {!jspb.BinaryReader} reader The BinaryReader to use.
 * @return {!proto.g8e.operator.v1.FsReadRequested}
 */
proto.g8e.operator.v1.FsReadRequested.deserializeBinaryFromReader = function(msg, reader) {
  while (reader.nextField()) {
    if (reader.isEndGroup()) {
      break;
    }
    var field = reader.getFieldNumber();
    switch (field) {
    case 1:
      var value = /** @type {string} */ (reader.readString());
      msg.setPath(value);
      break;
    case 2:
      var value = /** @type {string} */ (reader.readString());
      msg.setExecutionId(value);
      break;
    case 3:
      var value = /** @type {number} */ (reader.readInt32());
      msg.setMaxSize(value);
      break;
    case 4:
      var value = /** @type {string} */ (reader.readString());
      msg.setSentinelMode(value);
      break;
    default:
      reader.skipField();
      break;
    }
  }
  return msg;
};


/**
 * Serializes the message to binary data (in protobuf wire format).
 * @return {!Uint8Array}
 */
proto.g8e.operator.v1.FsReadRequested.prototype.serializeBinary = function() {
  var writer = new jspb.BinaryWriter();
  proto.g8e.operator.v1.FsReadRequested.serializeBinaryToWriter(this, writer);
  return writer.getResultBuffer();
};


/**
 * Serializes the given message to binary data (in protobuf wire
 * format), writing to the given BinaryWriter.
 * @param {!proto.g8e.operator.v1.FsReadRequested} message
 * @param {!jspb.BinaryWriter} writer
 * @suppress {unusedLocalVariables} f is only used for nested messages
 */
proto.g8e.operator.v1.FsReadRequested.serializeBinaryToWriter = function(message, writer) {
  var f = undefined;
  f = message.getPath();
  if (f.length > 0) {
    writer.writeString(
      1,
      f
    );
  }
  f = message.getExecutionId();
  if (f.length > 0) {
    writer.writeString(
      2,
      f
    );
  }
  f = message.getMaxSize();
  if (f !== 0) {
    writer.writeInt32(
      3,
      f
    );
  }
  f = message.getSentinelMode();
  if (f.length > 0) {
    writer.writeString(
      4,
      f
    );
  }
};


/**
 * optional string path = 1;
 * @return {string}
 */
proto.g8e.operator.v1.FsReadRequested.prototype.getPath = function() {
  return /** @type {string} */ (jspb.Message.getFieldWithDefault(this, 1, ""));
};


/**
 * @param {string} value
 * @return {!proto.g8e.operator.v1.FsReadRequested} returns this
 */
proto.g8e.operator.v1.FsReadRequested.prototype.setPath = function(value) {
  return jspb.Message.setProto3StringField(this, 1, value);
};


/**
 * optional string execution_id = 2;
 * @return {string}
 */
proto.g8e.operator.v1.FsReadRequested.prototype.getExecutionId = function() {
  return /** @type {string} */ (jspb.Message.getFieldWithDefault(this, 2, ""));
};


/**
 * @param {string} value
 * @return {!proto.g8e.operator.v1.FsReadRequested} returns this
 */
proto.g8e.operator.v1.FsReadRequested.prototype.setExecutionId = function(value) {
  return jspb.Message.setProto3StringField(this, 2, value);
};


/**
 * optional int32 max_size = 3;
 * @return {number}
 */
proto.g8e.operator.v1.FsReadRequested.prototype.getMaxSize = function() {
  return /** @type {number} */ (jspb.Message.getFieldWithDefault(this, 3, 0));
};


/**
 * @param {number} value
 * @return {!proto.g8e.operator.v1.FsReadRequested} returns this
 */
proto.g8e.operator.v1.FsReadRequested.prototype.setMaxSize = function(value) {
  return jspb.Message.setProto3IntField(this, 3, value);
};


/**
 * optional string sentinel_mode = 4;
 * @return {string}
 */
proto.g8e.operator.v1.FsReadRequested.prototype.getSentinelMode = function() {
  return /** @type {string} */ (jspb.Message.getFieldWithDefault(this, 4, ""));
};


/**
 * @param {string} value
 * @return {!proto.g8e.operator.v1.FsReadRequested} returns this
 */
proto.g8e.operator.v1.FsReadRequested.prototype.setSentinelMode = function(value) {
  return jspb.Message.setProto3StringField(this, 4, value);
};





if (jspb.Message.GENERATE_TO_OBJECT) {
/**
 * Creates an object representation of this proto.
 * Field names that are reserved in JavaScript and will be renamed to pb_name.
 * Optional fields that are not set will be set to undefined.
 * To access a reserved field use, foo.pb_<name>, eg, foo.pb_default.
 * For the list of reserved names please see:
 *     net/proto2/compiler/js/internal/generator.cc#kKeyword.
 * @param {boolean=} opt_includeInstance Deprecated. whether to include the
 *     JSPB instance for transitional soy proto support:
 *     http://goto/soy-param-migration
 * @return {!Object}
 */
proto.g8e.operator.v1.HeartbeatRequested.prototype.toObject = function(opt_includeInstance) {
  return proto.g8e.operator.v1.HeartbeatRequested.toObject(opt_includeInstance, this);
};


/**
 * Static version of the {@see toObject} method.
 * @param {boolean|undefined} includeInstance Deprecated. Whether to include
 *     the JSPB instance for transitional soy proto support:
 *     http://goto/soy-param-migration
 * @param {!proto.g8e.operator.v1.HeartbeatRequested} msg The msg instance to transform.
 * @return {!Object}
 * @suppress {unusedLocalVariables} f is only used for nested messages
 */
proto.g8e.operator.v1.HeartbeatRequested.toObject = function(includeInstance, msg) {
  var f, obj = {

  };

  if (includeInstance) {
    obj.$jspbMessageInstance = msg;
  }
  return obj;
};
}


/**
 * Deserializes binary data (in protobuf wire format).
 * @param {jspb.ByteSource} bytes The bytes to deserialize.
 * @return {!proto.g8e.operator.v1.HeartbeatRequested}
 */
proto.g8e.operator.v1.HeartbeatRequested.deserializeBinary = function(bytes) {
  var reader = new jspb.BinaryReader(bytes);
  var msg = new proto.g8e.operator.v1.HeartbeatRequested;
  return proto.g8e.operator.v1.HeartbeatRequested.deserializeBinaryFromReader(msg, reader);
};


/**
 * Deserializes binary data (in protobuf wire format) from the
 * given reader into the given message object.
 * @param {!proto.g8e.operator.v1.HeartbeatRequested} msg The message object to deserialize into.
 * @param {!jspb.BinaryReader} reader The BinaryReader to use.
 * @return {!proto.g8e.operator.v1.HeartbeatRequested}
 */
proto.g8e.operator.v1.HeartbeatRequested.deserializeBinaryFromReader = function(msg, reader) {
  while (reader.nextField()) {
    if (reader.isEndGroup()) {
      break;
    }
    var field = reader.getFieldNumber();
    switch (field) {
    default:
      reader.skipField();
      break;
    }
  }
  return msg;
};


/**
 * Serializes the message to binary data (in protobuf wire format).
 * @return {!Uint8Array}
 */
proto.g8e.operator.v1.HeartbeatRequested.prototype.serializeBinary = function() {
  var writer = new jspb.BinaryWriter();
  proto.g8e.operator.v1.HeartbeatRequested.serializeBinaryToWriter(this, writer);
  return writer.getResultBuffer();
};


/**
 * Serializes the given message to binary data (in protobuf wire
 * format), writing to the given BinaryWriter.
 * @param {!proto.g8e.operator.v1.HeartbeatRequested} message
 * @param {!jspb.BinaryWriter} writer
 * @suppress {unusedLocalVariables} f is only used for nested messages
 */
proto.g8e.operator.v1.HeartbeatRequested.serializeBinaryToWriter = function(message, writer) {
  var f = undefined;
};



/**
 * List of repeated fields within this message type.
 * @private {!Array<number>}
 * @const
 */
proto.g8e.operator.v1.FsGrepRequested.repeatedFields_ = [4];



if (jspb.Message.GENERATE_TO_OBJECT) {
/**
 * Creates an object representation of this proto.
 * Field names that are reserved in JavaScript and will be renamed to pb_name.
 * Optional fields that are not set will be set to undefined.
 * To access a reserved field use, foo.pb_<name>, eg, foo.pb_default.
 * For the list of reserved names please see:
 *     net/proto2/compiler/js/internal/generator.cc#kKeyword.
 * @param {boolean=} opt_includeInstance Deprecated. whether to include the
 *     JSPB instance for transitional soy proto support:
 *     http://goto/soy-param-migration
 * @return {!Object}
 */
proto.g8e.operator.v1.FsGrepRequested.prototype.toObject = function(opt_includeInstance) {
  return proto.g8e.operator.v1.FsGrepRequested.toObject(opt_includeInstance, this);
};


/**
 * Static version of the {@see toObject} method.
 * @param {boolean|undefined} includeInstance Deprecated. Whether to include
 *     the JSPB instance for transitional soy proto support:
 *     http://goto/soy-param-migration
 * @param {!proto.g8e.operator.v1.FsGrepRequested} msg The msg instance to transform.
 * @return {!Object}
 * @suppress {unusedLocalVariables} f is only used for nested messages
 */
proto.g8e.operator.v1.FsGrepRequested.toObject = function(includeInstance, msg) {
  var f, obj = {
    path: jspb.Message.getFieldWithDefault(msg, 1, ""),
    executionId: jspb.Message.getFieldWithDefault(msg, 2, ""),
    pattern: jspb.Message.getFieldWithDefault(msg, 3, ""),
    includesList: (f = jspb.Message.getRepeatedField(msg, 4)) == null ? undefined : f,
    maxMatches: jspb.Message.getFieldWithDefault(msg, 5, 0),
    sentinelMode: jspb.Message.getFieldWithDefault(msg, 6, "")
  };

  if (includeInstance) {
    obj.$jspbMessageInstance = msg;
  }
  return obj;
};
}


/**
 * Deserializes binary data (in protobuf wire format).
 * @param {jspb.ByteSource} bytes The bytes to deserialize.
 * @return {!proto.g8e.operator.v1.FsGrepRequested}
 */
proto.g8e.operator.v1.FsGrepRequested.deserializeBinary = function(bytes) {
  var reader = new jspb.BinaryReader(bytes);
  var msg = new proto.g8e.operator.v1.FsGrepRequested;
  return proto.g8e.operator.v1.FsGrepRequested.deserializeBinaryFromReader(msg, reader);
};


/**
 * Deserializes binary data (in protobuf wire format) from the
 * given reader into the given message object.
 * @param {!proto.g8e.operator.v1.FsGrepRequested} msg The message object to deserialize into.
 * @param {!jspb.BinaryReader} reader The BinaryReader to use.
 * @return {!proto.g8e.operator.v1.FsGrepRequested}
 */
proto.g8e.operator.v1.FsGrepRequested.deserializeBinaryFromReader = function(msg, reader) {
  while (reader.nextField()) {
    if (reader.isEndGroup()) {
      break;
    }
    var field = reader.getFieldNumber();
    switch (field) {
    case 1:
      var value = /** @type {string} */ (reader.readString());
      msg.setPath(value);
      break;
    case 2:
      var value = /** @type {string} */ (reader.readString());
      msg.setExecutionId(value);
      break;
    case 3:
      var value = /** @type {string} */ (reader.readString());
      msg.setPattern(value);
      break;
    case 4:
      var value = /** @type {string} */ (reader.readString());
      msg.addIncludes(value);
      break;
    case 5:
      var value = /** @type {number} */ (reader.readInt32());
      msg.setMaxMatches(value);
      break;
    case 6:
      var value = /** @type {string} */ (reader.readString());
      msg.setSentinelMode(value);
      break;
    default:
      reader.skipField();
      break;
    }
  }
  return msg;
};


/**
 * Serializes the message to binary data (in protobuf wire format).
 * @return {!Uint8Array}
 */
proto.g8e.operator.v1.FsGrepRequested.prototype.serializeBinary = function() {
  var writer = new jspb.BinaryWriter();
  proto.g8e.operator.v1.FsGrepRequested.serializeBinaryToWriter(this, writer);
  return writer.getResultBuffer();
};


/**
 * Serializes the given message to binary data (in protobuf wire
 * format), writing to the given BinaryWriter.
 * @param {!proto.g8e.operator.v1.FsGrepRequested} message
 * @param {!jspb.BinaryWriter} writer
 * @suppress {unusedLocalVariables} f is only used for nested messages
 */
proto.g8e.operator.v1.FsGrepRequested.serializeBinaryToWriter = function(message, writer) {
  var f = undefined;
  f = message.getPath();
  if (f.length > 0) {
    writer.writeString(
      1,
      f
    );
  }
  f = message.getExecutionId();
  if (f.length > 0) {
    writer.writeString(
      2,
      f
    );
  }
  f = message.getPattern();
  if (f.length > 0) {
    writer.writeString(
      3,
      f
    );
  }
  f = message.getIncludesList();
  if (f.length > 0) {
    writer.writeRepeatedString(
      4,
      f
    );
  }
  f = message.getMaxMatches();
  if (f !== 0) {
    writer.writeInt32(
      5,
      f
    );
  }
  f = message.getSentinelMode();
  if (f.length > 0) {
    writer.writeString(
      6,
      f
    );
  }
};


/**
 * optional string path = 1;
 * @return {string}
 */
proto.g8e.operator.v1.FsGrepRequested.prototype.getPath = function() {
  return /** @type {string} */ (jspb.Message.getFieldWithDefault(this, 1, ""));
};


/**
 * @param {string} value
 * @return {!proto.g8e.operator.v1.FsGrepRequested} returns this
 */
proto.g8e.operator.v1.FsGrepRequested.prototype.setPath = function(value) {
  return jspb.Message.setProto3StringField(this, 1, value);
};


/**
 * optional string execution_id = 2;
 * @return {string}
 */
proto.g8e.operator.v1.FsGrepRequested.prototype.getExecutionId = function() {
  return /** @type {string} */ (jspb.Message.getFieldWithDefault(this, 2, ""));
};


/**
 * @param {string} value
 * @return {!proto.g8e.operator.v1.FsGrepRequested} returns this
 */
proto.g8e.operator.v1.FsGrepRequested.prototype.setExecutionId = function(value) {
  return jspb.Message.setProto3StringField(this, 2, value);
};


/**
 * optional string pattern = 3;
 * @return {string}
 */
proto.g8e.operator.v1.FsGrepRequested.prototype.getPattern = function() {
  return /** @type {string} */ (jspb.Message.getFieldWithDefault(this, 3, ""));
};


/**
 * @param {string} value
 * @return {!proto.g8e.operator.v1.FsGrepRequested} returns this
 */
proto.g8e.operator.v1.FsGrepRequested.prototype.setPattern = function(value) {
  return jspb.Message.setProto3StringField(this, 3, value);
};


/**
 * repeated string includes = 4;
 * @return {!Array<string>}
 */
proto.g8e.operator.v1.FsGrepRequested.prototype.getIncludesList = function() {
  return /** @type {!Array<string>} */ (jspb.Message.getRepeatedField(this, 4));
};


/**
 * @param {!Array<string>} value
 * @return {!proto.g8e.operator.v1.FsGrepRequested} returns this
 */
proto.g8e.operator.v1.FsGrepRequested.prototype.setIncludesList = function(value) {
  return jspb.Message.setField(this, 4, value || []);
};


/**
 * @param {string} value
 * @param {number=} opt_index
 * @return {!proto.g8e.operator.v1.FsGrepRequested} returns this
 */
proto.g8e.operator.v1.FsGrepRequested.prototype.addIncludes = function(value, opt_index) {
  return jspb.Message.addToRepeatedField(this, 4, value, opt_index);
};


/**
 * Clears the list making it empty but non-null.
 * @return {!proto.g8e.operator.v1.FsGrepRequested} returns this
 */
proto.g8e.operator.v1.FsGrepRequested.prototype.clearIncludesList = function() {
  return this.setIncludesList([]);
};


/**
 * optional int32 max_matches = 5;
 * @return {number}
 */
proto.g8e.operator.v1.FsGrepRequested.prototype.getMaxMatches = function() {
  return /** @type {number} */ (jspb.Message.getFieldWithDefault(this, 5, 0));
};


/**
 * @param {number} value
 * @return {!proto.g8e.operator.v1.FsGrepRequested} returns this
 */
proto.g8e.operator.v1.FsGrepRequested.prototype.setMaxMatches = function(value) {
  return jspb.Message.setProto3IntField(this, 5, value);
};


/**
 * optional string sentinel_mode = 6;
 * @return {string}
 */
proto.g8e.operator.v1.FsGrepRequested.prototype.getSentinelMode = function() {
  return /** @type {string} */ (jspb.Message.getFieldWithDefault(this, 6, ""));
};


/**
 * @param {string} value
 * @return {!proto.g8e.operator.v1.FsGrepRequested} returns this
 */
proto.g8e.operator.v1.FsGrepRequested.prototype.setSentinelMode = function(value) {
  return jspb.Message.setProto3StringField(this, 6, value);
};





if (jspb.Message.GENERATE_TO_OBJECT) {
/**
 * Creates an object representation of this proto.
 * Field names that are reserved in JavaScript and will be renamed to pb_name.
 * Optional fields that are not set will be set to undefined.
 * To access a reserved field use, foo.pb_<name>, eg, foo.pb_default.
 * For the list of reserved names please see:
 *     net/proto2/compiler/js/internal/generator.cc#kKeyword.
 * @param {boolean=} opt_includeInstance Deprecated. whether to include the
 *     JSPB instance for transitional soy proto support:
 *     http://goto/soy-param-migration
 * @return {!Object}
 */
proto.g8e.operator.v1.CheckPortRequested.prototype.toObject = function(opt_includeInstance) {
  return proto.g8e.operator.v1.CheckPortRequested.toObject(opt_includeInstance, this);
};


/**
 * Static version of the {@see toObject} method.
 * @param {boolean|undefined} includeInstance Deprecated. Whether to include
 *     the JSPB instance for transitional soy proto support:
 *     http://goto/soy-param-migration
 * @param {!proto.g8e.operator.v1.CheckPortRequested} msg The msg instance to transform.
 * @return {!Object}
 * @suppress {unusedLocalVariables} f is only used for nested messages
 */
proto.g8e.operator.v1.CheckPortRequested.toObject = function(includeInstance, msg) {
  var f, obj = {
    executionId: jspb.Message.getFieldWithDefault(msg, 1, ""),
    port: jspb.Message.getFieldWithDefault(msg, 2, 0),
    host: jspb.Message.getFieldWithDefault(msg, 3, ""),
    protocol: jspb.Message.getFieldWithDefault(msg, 4, "")
  };

  if (includeInstance) {
    obj.$jspbMessageInstance = msg;
  }
  return obj;
};
}


/**
 * Deserializes binary data (in protobuf wire format).
 * @param {jspb.ByteSource} bytes The bytes to deserialize.
 * @return {!proto.g8e.operator.v1.CheckPortRequested}
 */
proto.g8e.operator.v1.CheckPortRequested.deserializeBinary = function(bytes) {
  var reader = new jspb.BinaryReader(bytes);
  var msg = new proto.g8e.operator.v1.CheckPortRequested;
  return proto.g8e.operator.v1.CheckPortRequested.deserializeBinaryFromReader(msg, reader);
};


/**
 * Deserializes binary data (in protobuf wire format) from the
 * given reader into the given message object.
 * @param {!proto.g8e.operator.v1.CheckPortRequested} msg The message object to deserialize into.
 * @param {!jspb.BinaryReader} reader The BinaryReader to use.
 * @return {!proto.g8e.operator.v1.CheckPortRequested}
 */
proto.g8e.operator.v1.CheckPortRequested.deserializeBinaryFromReader = function(msg, reader) {
  while (reader.nextField()) {
    if (reader.isEndGroup()) {
      break;
    }
    var field = reader.getFieldNumber();
    switch (field) {
    case 1:
      var value = /** @type {string} */ (reader.readString());
      msg.setExecutionId(value);
      break;
    case 2:
      var value = /** @type {number} */ (reader.readInt32());
      msg.setPort(value);
      break;
    case 3:
      var value = /** @type {string} */ (reader.readString());
      msg.setHost(value);
      break;
    case 4:
      var value = /** @type {string} */ (reader.readString());
      msg.setProtocol(value);
      break;
    default:
      reader.skipField();
      break;
    }
  }
  return msg;
};


/**
 * Serializes the message to binary data (in protobuf wire format).
 * @return {!Uint8Array}
 */
proto.g8e.operator.v1.CheckPortRequested.prototype.serializeBinary = function() {
  var writer = new jspb.BinaryWriter();
  proto.g8e.operator.v1.CheckPortRequested.serializeBinaryToWriter(this, writer);
  return writer.getResultBuffer();
};


/**
 * Serializes the given message to binary data (in protobuf wire
 * format), writing to the given BinaryWriter.
 * @param {!proto.g8e.operator.v1.CheckPortRequested} message
 * @param {!jspb.BinaryWriter} writer
 * @suppress {unusedLocalVariables} f is only used for nested messages
 */
proto.g8e.operator.v1.CheckPortRequested.serializeBinaryToWriter = function(message, writer) {
  var f = undefined;
  f = message.getExecutionId();
  if (f.length > 0) {
    writer.writeString(
      1,
      f
    );
  }
  f = message.getPort();
  if (f !== 0) {
    writer.writeInt32(
      2,
      f
    );
  }
  f = message.getHost();
  if (f.length > 0) {
    writer.writeString(
      3,
      f
    );
  }
  f = message.getProtocol();
  if (f.length > 0) {
    writer.writeString(
      4,
      f
    );
  }
};


/**
 * optional string execution_id = 1;
 * @return {string}
 */
proto.g8e.operator.v1.CheckPortRequested.prototype.getExecutionId = function() {
  return /** @type {string} */ (jspb.Message.getFieldWithDefault(this, 1, ""));
};


/**
 * @param {string} value
 * @return {!proto.g8e.operator.v1.CheckPortRequested} returns this
 */
proto.g8e.operator.v1.CheckPortRequested.prototype.setExecutionId = function(value) {
  return jspb.Message.setProto3StringField(this, 1, value);
};


/**
 * optional int32 port = 2;
 * @return {number}
 */
proto.g8e.operator.v1.CheckPortRequested.prototype.getPort = function() {
  return /** @type {number} */ (jspb.Message.getFieldWithDefault(this, 2, 0));
};


/**
 * @param {number} value
 * @return {!proto.g8e.operator.v1.CheckPortRequested} returns this
 */
proto.g8e.operator.v1.CheckPortRequested.prototype.setPort = function(value) {
  return jspb.Message.setProto3IntField(this, 2, value);
};


/**
 * optional string host = 3;
 * @return {string}
 */
proto.g8e.operator.v1.CheckPortRequested.prototype.getHost = function() {
  return /** @type {string} */ (jspb.Message.getFieldWithDefault(this, 3, ""));
};


/**
 * @param {string} value
 * @return {!proto.g8e.operator.v1.CheckPortRequested} returns this
 */
proto.g8e.operator.v1.CheckPortRequested.prototype.setHost = function(value) {
  return jspb.Message.setProto3StringField(this, 3, value);
};


/**
 * optional string protocol = 4;
 * @return {string}
 */
proto.g8e.operator.v1.CheckPortRequested.prototype.getProtocol = function() {
  return /** @type {string} */ (jspb.Message.getFieldWithDefault(this, 4, ""));
};


/**
 * @param {string} value
 * @return {!proto.g8e.operator.v1.CheckPortRequested} returns this
 */
proto.g8e.operator.v1.CheckPortRequested.prototype.setProtocol = function(value) {
  return jspb.Message.setProto3StringField(this, 4, value);
};





if (jspb.Message.GENERATE_TO_OBJECT) {
/**
 * Creates an object representation of this proto.
 * Field names that are reserved in JavaScript and will be renamed to pb_name.
 * Optional fields that are not set will be set to undefined.
 * To access a reserved field use, foo.pb_<name>, eg, foo.pb_default.
 * For the list of reserved names please see:
 *     net/proto2/compiler/js/internal/generator.cc#kKeyword.
 * @param {boolean=} opt_includeInstance Deprecated. whether to include the
 *     JSPB instance for transitional soy proto support:
 *     http://goto/soy-param-migration
 * @return {!Object}
 */
proto.g8e.operator.v1.FetchLogsRequested.prototype.toObject = function(opt_includeInstance) {
  return proto.g8e.operator.v1.FetchLogsRequested.toObject(opt_includeInstance, this);
};


/**
 * Static version of the {@see toObject} method.
 * @param {boolean|undefined} includeInstance Deprecated. Whether to include
 *     the JSPB instance for transitional soy proto support:
 *     http://goto/soy-param-migration
 * @param {!proto.g8e.operator.v1.FetchLogsRequested} msg The msg instance to transform.
 * @return {!Object}
 * @suppress {unusedLocalVariables} f is only used for nested messages
 */
proto.g8e.operator.v1.FetchLogsRequested.toObject = function(includeInstance, msg) {
  var f, obj = {
    executionId: jspb.Message.getFieldWithDefault(msg, 1, ""),
    sentinelMode: jspb.Message.getFieldWithDefault(msg, 2, "")
  };

  if (includeInstance) {
    obj.$jspbMessageInstance = msg;
  }
  return obj;
};
}


/**
 * Deserializes binary data (in protobuf wire format).
 * @param {jspb.ByteSource} bytes The bytes to deserialize.
 * @return {!proto.g8e.operator.v1.FetchLogsRequested}
 */
proto.g8e.operator.v1.FetchLogsRequested.deserializeBinary = function(bytes) {
  var reader = new jspb.BinaryReader(bytes);
  var msg = new proto.g8e.operator.v1.FetchLogsRequested;
  return proto.g8e.operator.v1.FetchLogsRequested.deserializeBinaryFromReader(msg, reader);
};


/**
 * Deserializes binary data (in protobuf wire format) from the
 * given reader into the given message object.
 * @param {!proto.g8e.operator.v1.FetchLogsRequested} msg The message object to deserialize into.
 * @param {!jspb.BinaryReader} reader The BinaryReader to use.
 * @return {!proto.g8e.operator.v1.FetchLogsRequested}
 */
proto.g8e.operator.v1.FetchLogsRequested.deserializeBinaryFromReader = function(msg, reader) {
  while (reader.nextField()) {
    if (reader.isEndGroup()) {
      break;
    }
    var field = reader.getFieldNumber();
    switch (field) {
    case 1:
      var value = /** @type {string} */ (reader.readString());
      msg.setExecutionId(value);
      break;
    case 2:
      var value = /** @type {string} */ (reader.readString());
      msg.setSentinelMode(value);
      break;
    default:
      reader.skipField();
      break;
    }
  }
  return msg;
};


/**
 * Serializes the message to binary data (in protobuf wire format).
 * @return {!Uint8Array}
 */
proto.g8e.operator.v1.FetchLogsRequested.prototype.serializeBinary = function() {
  var writer = new jspb.BinaryWriter();
  proto.g8e.operator.v1.FetchLogsRequested.serializeBinaryToWriter(this, writer);
  return writer.getResultBuffer();
};


/**
 * Serializes the given message to binary data (in protobuf wire
 * format), writing to the given BinaryWriter.
 * @param {!proto.g8e.operator.v1.FetchLogsRequested} message
 * @param {!jspb.BinaryWriter} writer
 * @suppress {unusedLocalVariables} f is only used for nested messages
 */
proto.g8e.operator.v1.FetchLogsRequested.serializeBinaryToWriter = function(message, writer) {
  var f = undefined;
  f = message.getExecutionId();
  if (f.length > 0) {
    writer.writeString(
      1,
      f
    );
  }
  f = message.getSentinelMode();
  if (f.length > 0) {
    writer.writeString(
      2,
      f
    );
  }
};


/**
 * optional string execution_id = 1;
 * @return {string}
 */
proto.g8e.operator.v1.FetchLogsRequested.prototype.getExecutionId = function() {
  return /** @type {string} */ (jspb.Message.getFieldWithDefault(this, 1, ""));
};


/**
 * @param {string} value
 * @return {!proto.g8e.operator.v1.FetchLogsRequested} returns this
 */
proto.g8e.operator.v1.FetchLogsRequested.prototype.setExecutionId = function(value) {
  return jspb.Message.setProto3StringField(this, 1, value);
};


/**
 * optional string sentinel_mode = 2;
 * @return {string}
 */
proto.g8e.operator.v1.FetchLogsRequested.prototype.getSentinelMode = function() {
  return /** @type {string} */ (jspb.Message.getFieldWithDefault(this, 2, ""));
};


/**
 * @param {string} value
 * @return {!proto.g8e.operator.v1.FetchLogsRequested} returns this
 */
proto.g8e.operator.v1.FetchLogsRequested.prototype.setSentinelMode = function(value) {
  return jspb.Message.setProto3StringField(this, 2, value);
};





if (jspb.Message.GENERATE_TO_OBJECT) {
/**
 * Creates an object representation of this proto.
 * Field names that are reserved in JavaScript and will be renamed to pb_name.
 * Optional fields that are not set will be set to undefined.
 * To access a reserved field use, foo.pb_<name>, eg, foo.pb_default.
 * For the list of reserved names please see:
 *     net/proto2/compiler/js/internal/generator.cc#kKeyword.
 * @param {boolean=} opt_includeInstance Deprecated. whether to include the
 *     JSPB instance for transitional soy proto support:
 *     http://goto/soy-param-migration
 * @return {!Object}
 */
proto.g8e.operator.v1.FetchHistoryRequested.prototype.toObject = function(opt_includeInstance) {
  return proto.g8e.operator.v1.FetchHistoryRequested.toObject(opt_includeInstance, this);
};


/**
 * Static version of the {@see toObject} method.
 * @param {boolean|undefined} includeInstance Deprecated. Whether to include
 *     the JSPB instance for transitional soy proto support:
 *     http://goto/soy-param-migration
 * @param {!proto.g8e.operator.v1.FetchHistoryRequested} msg The msg instance to transform.
 * @return {!Object}
 * @suppress {unusedLocalVariables} f is only used for nested messages
 */
proto.g8e.operator.v1.FetchHistoryRequested.toObject = function(includeInstance, msg) {
  var f, obj = {
    executionId: jspb.Message.getFieldWithDefault(msg, 1, ""),
    operatorSessionId: jspb.Message.getFieldWithDefault(msg, 2, ""),
    limit: jspb.Message.getFieldWithDefault(msg, 3, 0),
    offset: jspb.Message.getFieldWithDefault(msg, 4, 0),
    includeCommands: jspb.Message.getBooleanFieldWithDefault(msg, 5, false),
    includeFileMutations: jspb.Message.getBooleanFieldWithDefault(msg, 6, false)
  };

  if (includeInstance) {
    obj.$jspbMessageInstance = msg;
  }
  return obj;
};
}


/**
 * Deserializes binary data (in protobuf wire format).
 * @param {jspb.ByteSource} bytes The bytes to deserialize.
 * @return {!proto.g8e.operator.v1.FetchHistoryRequested}
 */
proto.g8e.operator.v1.FetchHistoryRequested.deserializeBinary = function(bytes) {
  var reader = new jspb.BinaryReader(bytes);
  var msg = new proto.g8e.operator.v1.FetchHistoryRequested;
  return proto.g8e.operator.v1.FetchHistoryRequested.deserializeBinaryFromReader(msg, reader);
};


/**
 * Deserializes binary data (in protobuf wire format) from the
 * given reader into the given message object.
 * @param {!proto.g8e.operator.v1.FetchHistoryRequested} msg The message object to deserialize into.
 * @param {!jspb.BinaryReader} reader The BinaryReader to use.
 * @return {!proto.g8e.operator.v1.FetchHistoryRequested}
 */
proto.g8e.operator.v1.FetchHistoryRequested.deserializeBinaryFromReader = function(msg, reader) {
  while (reader.nextField()) {
    if (reader.isEndGroup()) {
      break;
    }
    var field = reader.getFieldNumber();
    switch (field) {
    case 1:
      var value = /** @type {string} */ (reader.readString());
      msg.setExecutionId(value);
      break;
    case 2:
      var value = /** @type {string} */ (reader.readString());
      msg.setOperatorSessionId(value);
      break;
    case 3:
      var value = /** @type {number} */ (reader.readInt32());
      msg.setLimit(value);
      break;
    case 4:
      var value = /** @type {number} */ (reader.readInt32());
      msg.setOffset(value);
      break;
    case 5:
      var value = /** @type {boolean} */ (reader.readBool());
      msg.setIncludeCommands(value);
      break;
    case 6:
      var value = /** @type {boolean} */ (reader.readBool());
      msg.setIncludeFileMutations(value);
      break;
    default:
      reader.skipField();
      break;
    }
  }
  return msg;
};


/**
 * Serializes the message to binary data (in protobuf wire format).
 * @return {!Uint8Array}
 */
proto.g8e.operator.v1.FetchHistoryRequested.prototype.serializeBinary = function() {
  var writer = new jspb.BinaryWriter();
  proto.g8e.operator.v1.FetchHistoryRequested.serializeBinaryToWriter(this, writer);
  return writer.getResultBuffer();
};


/**
 * Serializes the given message to binary data (in protobuf wire
 * format), writing to the given BinaryWriter.
 * @param {!proto.g8e.operator.v1.FetchHistoryRequested} message
 * @param {!jspb.BinaryWriter} writer
 * @suppress {unusedLocalVariables} f is only used for nested messages
 */
proto.g8e.operator.v1.FetchHistoryRequested.serializeBinaryToWriter = function(message, writer) {
  var f = undefined;
  f = message.getExecutionId();
  if (f.length > 0) {
    writer.writeString(
      1,
      f
    );
  }
  f = message.getOperatorSessionId();
  if (f.length > 0) {
    writer.writeString(
      2,
      f
    );
  }
  f = message.getLimit();
  if (f !== 0) {
    writer.writeInt32(
      3,
      f
    );
  }
  f = message.getOffset();
  if (f !== 0) {
    writer.writeInt32(
      4,
      f
    );
  }
  f = message.getIncludeCommands();
  if (f) {
    writer.writeBool(
      5,
      f
    );
  }
  f = message.getIncludeFileMutations();
  if (f) {
    writer.writeBool(
      6,
      f
    );
  }
};


/**
 * optional string execution_id = 1;
 * @return {string}
 */
proto.g8e.operator.v1.FetchHistoryRequested.prototype.getExecutionId = function() {
  return /** @type {string} */ (jspb.Message.getFieldWithDefault(this, 1, ""));
};


/**
 * @param {string} value
 * @return {!proto.g8e.operator.v1.FetchHistoryRequested} returns this
 */
proto.g8e.operator.v1.FetchHistoryRequested.prototype.setExecutionId = function(value) {
  return jspb.Message.setProto3StringField(this, 1, value);
};


/**
 * optional string operator_session_id = 2;
 * @return {string}
 */
proto.g8e.operator.v1.FetchHistoryRequested.prototype.getOperatorSessionId = function() {
  return /** @type {string} */ (jspb.Message.getFieldWithDefault(this, 2, ""));
};


/**
 * @param {string} value
 * @return {!proto.g8e.operator.v1.FetchHistoryRequested} returns this
 */
proto.g8e.operator.v1.FetchHistoryRequested.prototype.setOperatorSessionId = function(value) {
  return jspb.Message.setProto3StringField(this, 2, value);
};


/**
 * optional int32 limit = 3;
 * @return {number}
 */
proto.g8e.operator.v1.FetchHistoryRequested.prototype.getLimit = function() {
  return /** @type {number} */ (jspb.Message.getFieldWithDefault(this, 3, 0));
};


/**
 * @param {number} value
 * @return {!proto.g8e.operator.v1.FetchHistoryRequested} returns this
 */
proto.g8e.operator.v1.FetchHistoryRequested.prototype.setLimit = function(value) {
  return jspb.Message.setProto3IntField(this, 3, value);
};


/**
 * optional int32 offset = 4;
 * @return {number}
 */
proto.g8e.operator.v1.FetchHistoryRequested.prototype.getOffset = function() {
  return /** @type {number} */ (jspb.Message.getFieldWithDefault(this, 4, 0));
};


/**
 * @param {number} value
 * @return {!proto.g8e.operator.v1.FetchHistoryRequested} returns this
 */
proto.g8e.operator.v1.FetchHistoryRequested.prototype.setOffset = function(value) {
  return jspb.Message.setProto3IntField(this, 4, value);
};


/**
 * optional bool include_commands = 5;
 * @return {boolean}
 */
proto.g8e.operator.v1.FetchHistoryRequested.prototype.getIncludeCommands = function() {
  return /** @type {boolean} */ (jspb.Message.getBooleanFieldWithDefault(this, 5, false));
};


/**
 * @param {boolean} value
 * @return {!proto.g8e.operator.v1.FetchHistoryRequested} returns this
 */
proto.g8e.operator.v1.FetchHistoryRequested.prototype.setIncludeCommands = function(value) {
  return jspb.Message.setProto3BooleanField(this, 5, value);
};


/**
 * optional bool include_file_mutations = 6;
 * @return {boolean}
 */
proto.g8e.operator.v1.FetchHistoryRequested.prototype.getIncludeFileMutations = function() {
  return /** @type {boolean} */ (jspb.Message.getBooleanFieldWithDefault(this, 6, false));
};


/**
 * @param {boolean} value
 * @return {!proto.g8e.operator.v1.FetchHistoryRequested} returns this
 */
proto.g8e.operator.v1.FetchHistoryRequested.prototype.setIncludeFileMutations = function(value) {
  return jspb.Message.setProto3BooleanField(this, 6, value);
};





if (jspb.Message.GENERATE_TO_OBJECT) {
/**
 * Creates an object representation of this proto.
 * Field names that are reserved in JavaScript and will be renamed to pb_name.
 * Optional fields that are not set will be set to undefined.
 * To access a reserved field use, foo.pb_<name>, eg, foo.pb_default.
 * For the list of reserved names please see:
 *     net/proto2/compiler/js/internal/generator.cc#kKeyword.
 * @param {boolean=} opt_includeInstance Deprecated. whether to include the
 *     JSPB instance for transitional soy proto support:
 *     http://goto/soy-param-migration
 * @return {!Object}
 */
proto.g8e.operator.v1.FetchFileHistoryRequested.prototype.toObject = function(opt_includeInstance) {
  return proto.g8e.operator.v1.FetchFileHistoryRequested.toObject(opt_includeInstance, this);
};


/**
 * Static version of the {@see toObject} method.
 * @param {boolean|undefined} includeInstance Deprecated. Whether to include
 *     the JSPB instance for transitional soy proto support:
 *     http://goto/soy-param-migration
 * @param {!proto.g8e.operator.v1.FetchFileHistoryRequested} msg The msg instance to transform.
 * @return {!Object}
 * @suppress {unusedLocalVariables} f is only used for nested messages
 */
proto.g8e.operator.v1.FetchFileHistoryRequested.toObject = function(includeInstance, msg) {
  var f, obj = {
    executionId: jspb.Message.getFieldWithDefault(msg, 1, ""),
    filePath: jspb.Message.getFieldWithDefault(msg, 2, ""),
    limit: jspb.Message.getFieldWithDefault(msg, 3, 0)
  };

  if (includeInstance) {
    obj.$jspbMessageInstance = msg;
  }
  return obj;
};
}


/**
 * Deserializes binary data (in protobuf wire format).
 * @param {jspb.ByteSource} bytes The bytes to deserialize.
 * @return {!proto.g8e.operator.v1.FetchFileHistoryRequested}
 */
proto.g8e.operator.v1.FetchFileHistoryRequested.deserializeBinary = function(bytes) {
  var reader = new jspb.BinaryReader(bytes);
  var msg = new proto.g8e.operator.v1.FetchFileHistoryRequested;
  return proto.g8e.operator.v1.FetchFileHistoryRequested.deserializeBinaryFromReader(msg, reader);
};


/**
 * Deserializes binary data (in protobuf wire format) from the
 * given reader into the given message object.
 * @param {!proto.g8e.operator.v1.FetchFileHistoryRequested} msg The message object to deserialize into.
 * @param {!jspb.BinaryReader} reader The BinaryReader to use.
 * @return {!proto.g8e.operator.v1.FetchFileHistoryRequested}
 */
proto.g8e.operator.v1.FetchFileHistoryRequested.deserializeBinaryFromReader = function(msg, reader) {
  while (reader.nextField()) {
    if (reader.isEndGroup()) {
      break;
    }
    var field = reader.getFieldNumber();
    switch (field) {
    case 1:
      var value = /** @type {string} */ (reader.readString());
      msg.setExecutionId(value);
      break;
    case 2:
      var value = /** @type {string} */ (reader.readString());
      msg.setFilePath(value);
      break;
    case 3:
      var value = /** @type {number} */ (reader.readInt32());
      msg.setLimit(value);
      break;
    default:
      reader.skipField();
      break;
    }
  }
  return msg;
};


/**
 * Serializes the message to binary data (in protobuf wire format).
 * @return {!Uint8Array}
 */
proto.g8e.operator.v1.FetchFileHistoryRequested.prototype.serializeBinary = function() {
  var writer = new jspb.BinaryWriter();
  proto.g8e.operator.v1.FetchFileHistoryRequested.serializeBinaryToWriter(this, writer);
  return writer.getResultBuffer();
};


/**
 * Serializes the given message to binary data (in protobuf wire
 * format), writing to the given BinaryWriter.
 * @param {!proto.g8e.operator.v1.FetchFileHistoryRequested} message
 * @param {!jspb.BinaryWriter} writer
 * @suppress {unusedLocalVariables} f is only used for nested messages
 */
proto.g8e.operator.v1.FetchFileHistoryRequested.serializeBinaryToWriter = function(message, writer) {
  var f = undefined;
  f = message.getExecutionId();
  if (f.length > 0) {
    writer.writeString(
      1,
      f
    );
  }
  f = message.getFilePath();
  if (f.length > 0) {
    writer.writeString(
      2,
      f
    );
  }
  f = message.getLimit();
  if (f !== 0) {
    writer.writeInt32(
      3,
      f
    );
  }
};


/**
 * optional string execution_id = 1;
 * @return {string}
 */
proto.g8e.operator.v1.FetchFileHistoryRequested.prototype.getExecutionId = function() {
  return /** @type {string} */ (jspb.Message.getFieldWithDefault(this, 1, ""));
};


/**
 * @param {string} value
 * @return {!proto.g8e.operator.v1.FetchFileHistoryRequested} returns this
 */
proto.g8e.operator.v1.FetchFileHistoryRequested.prototype.setExecutionId = function(value) {
  return jspb.Message.setProto3StringField(this, 1, value);
};


/**
 * optional string file_path = 2;
 * @return {string}
 */
proto.g8e.operator.v1.FetchFileHistoryRequested.prototype.getFilePath = function() {
  return /** @type {string} */ (jspb.Message.getFieldWithDefault(this, 2, ""));
};


/**
 * @param {string} value
 * @return {!proto.g8e.operator.v1.FetchFileHistoryRequested} returns this
 */
proto.g8e.operator.v1.FetchFileHistoryRequested.prototype.setFilePath = function(value) {
  return jspb.Message.setProto3StringField(this, 2, value);
};


/**
 * optional int32 limit = 3;
 * @return {number}
 */
proto.g8e.operator.v1.FetchFileHistoryRequested.prototype.getLimit = function() {
  return /** @type {number} */ (jspb.Message.getFieldWithDefault(this, 3, 0));
};


/**
 * @param {number} value
 * @return {!proto.g8e.operator.v1.FetchFileHistoryRequested} returns this
 */
proto.g8e.operator.v1.FetchFileHistoryRequested.prototype.setLimit = function(value) {
  return jspb.Message.setProto3IntField(this, 3, value);
};





if (jspb.Message.GENERATE_TO_OBJECT) {
/**
 * Creates an object representation of this proto.
 * Field names that are reserved in JavaScript and will be renamed to pb_name.
 * Optional fields that are not set will be set to undefined.
 * To access a reserved field use, foo.pb_<name>, eg, foo.pb_default.
 * For the list of reserved names please see:
 *     net/proto2/compiler/js/internal/generator.cc#kKeyword.
 * @param {boolean=} opt_includeInstance Deprecated. whether to include the
 *     JSPB instance for transitional soy proto support:
 *     http://goto/soy-param-migration
 * @return {!Object}
 */
proto.g8e.operator.v1.FetchFileDiffRequested.prototype.toObject = function(opt_includeInstance) {
  return proto.g8e.operator.v1.FetchFileDiffRequested.toObject(opt_includeInstance, this);
};


/**
 * Static version of the {@see toObject} method.
 * @param {boolean|undefined} includeInstance Deprecated. Whether to include
 *     the JSPB instance for transitional soy proto support:
 *     http://goto/soy-param-migration
 * @param {!proto.g8e.operator.v1.FetchFileDiffRequested} msg The msg instance to transform.
 * @return {!Object}
 * @suppress {unusedLocalVariables} f is only used for nested messages
 */
proto.g8e.operator.v1.FetchFileDiffRequested.toObject = function(includeInstance, msg) {
  var f, obj = {
    executionId: jspb.Message.getFieldWithDefault(msg, 1, ""),
    diffId: jspb.Message.getFieldWithDefault(msg, 2, ""),
    operatorSessionId: jspb.Message.getFieldWithDefault(msg, 3, ""),
    filePath: jspb.Message.getFieldWithDefault(msg, 4, ""),
    limit: jspb.Message.getFieldWithDefault(msg, 5, 0)
  };

  if (includeInstance) {
    obj.$jspbMessageInstance = msg;
  }
  return obj;
};
}


/**
 * Deserializes binary data (in protobuf wire format).
 * @param {jspb.ByteSource} bytes The bytes to deserialize.
 * @return {!proto.g8e.operator.v1.FetchFileDiffRequested}
 */
proto.g8e.operator.v1.FetchFileDiffRequested.deserializeBinary = function(bytes) {
  var reader = new jspb.BinaryReader(bytes);
  var msg = new proto.g8e.operator.v1.FetchFileDiffRequested;
  return proto.g8e.operator.v1.FetchFileDiffRequested.deserializeBinaryFromReader(msg, reader);
};


/**
 * Deserializes binary data (in protobuf wire format) from the
 * given reader into the given message object.
 * @param {!proto.g8e.operator.v1.FetchFileDiffRequested} msg The message object to deserialize into.
 * @param {!jspb.BinaryReader} reader The BinaryReader to use.
 * @return {!proto.g8e.operator.v1.FetchFileDiffRequested}
 */
proto.g8e.operator.v1.FetchFileDiffRequested.deserializeBinaryFromReader = function(msg, reader) {
  while (reader.nextField()) {
    if (reader.isEndGroup()) {
      break;
    }
    var field = reader.getFieldNumber();
    switch (field) {
    case 1:
      var value = /** @type {string} */ (reader.readString());
      msg.setExecutionId(value);
      break;
    case 2:
      var value = /** @type {string} */ (reader.readString());
      msg.setDiffId(value);
      break;
    case 3:
      var value = /** @type {string} */ (reader.readString());
      msg.setOperatorSessionId(value);
      break;
    case 4:
      var value = /** @type {string} */ (reader.readString());
      msg.setFilePath(value);
      break;
    case 5:
      var value = /** @type {number} */ (reader.readInt32());
      msg.setLimit(value);
      break;
    default:
      reader.skipField();
      break;
    }
  }
  return msg;
};


/**
 * Serializes the message to binary data (in protobuf wire format).
 * @return {!Uint8Array}
 */
proto.g8e.operator.v1.FetchFileDiffRequested.prototype.serializeBinary = function() {
  var writer = new jspb.BinaryWriter();
  proto.g8e.operator.v1.FetchFileDiffRequested.serializeBinaryToWriter(this, writer);
  return writer.getResultBuffer();
};


/**
 * Serializes the given message to binary data (in protobuf wire
 * format), writing to the given BinaryWriter.
 * @param {!proto.g8e.operator.v1.FetchFileDiffRequested} message
 * @param {!jspb.BinaryWriter} writer
 * @suppress {unusedLocalVariables} f is only used for nested messages
 */
proto.g8e.operator.v1.FetchFileDiffRequested.serializeBinaryToWriter = function(message, writer) {
  var f = undefined;
  f = message.getExecutionId();
  if (f.length > 0) {
    writer.writeString(
      1,
      f
    );
  }
  f = message.getDiffId();
  if (f.length > 0) {
    writer.writeString(
      2,
      f
    );
  }
  f = message.getOperatorSessionId();
  if (f.length > 0) {
    writer.writeString(
      3,
      f
    );
  }
  f = message.getFilePath();
  if (f.length > 0) {
    writer.writeString(
      4,
      f
    );
  }
  f = message.getLimit();
  if (f !== 0) {
    writer.writeInt32(
      5,
      f
    );
  }
};


/**
 * optional string execution_id = 1;
 * @return {string}
 */
proto.g8e.operator.v1.FetchFileDiffRequested.prototype.getExecutionId = function() {
  return /** @type {string} */ (jspb.Message.getFieldWithDefault(this, 1, ""));
};


/**
 * @param {string} value
 * @return {!proto.g8e.operator.v1.FetchFileDiffRequested} returns this
 */
proto.g8e.operator.v1.FetchFileDiffRequested.prototype.setExecutionId = function(value) {
  return jspb.Message.setProto3StringField(this, 1, value);
};


/**
 * optional string diff_id = 2;
 * @return {string}
 */
proto.g8e.operator.v1.FetchFileDiffRequested.prototype.getDiffId = function() {
  return /** @type {string} */ (jspb.Message.getFieldWithDefault(this, 2, ""));
};


/**
 * @param {string} value
 * @return {!proto.g8e.operator.v1.FetchFileDiffRequested} returns this
 */
proto.g8e.operator.v1.FetchFileDiffRequested.prototype.setDiffId = function(value) {
  return jspb.Message.setProto3StringField(this, 2, value);
};


/**
 * optional string operator_session_id = 3;
 * @return {string}
 */
proto.g8e.operator.v1.FetchFileDiffRequested.prototype.getOperatorSessionId = function() {
  return /** @type {string} */ (jspb.Message.getFieldWithDefault(this, 3, ""));
};


/**
 * @param {string} value
 * @return {!proto.g8e.operator.v1.FetchFileDiffRequested} returns this
 */
proto.g8e.operator.v1.FetchFileDiffRequested.prototype.setOperatorSessionId = function(value) {
  return jspb.Message.setProto3StringField(this, 3, value);
};


/**
 * optional string file_path = 4;
 * @return {string}
 */
proto.g8e.operator.v1.FetchFileDiffRequested.prototype.getFilePath = function() {
  return /** @type {string} */ (jspb.Message.getFieldWithDefault(this, 4, ""));
};


/**
 * @param {string} value
 * @return {!proto.g8e.operator.v1.FetchFileDiffRequested} returns this
 */
proto.g8e.operator.v1.FetchFileDiffRequested.prototype.setFilePath = function(value) {
  return jspb.Message.setProto3StringField(this, 4, value);
};


/**
 * optional int32 limit = 5;
 * @return {number}
 */
proto.g8e.operator.v1.FetchFileDiffRequested.prototype.getLimit = function() {
  return /** @type {number} */ (jspb.Message.getFieldWithDefault(this, 5, 0));
};


/**
 * @param {number} value
 * @return {!proto.g8e.operator.v1.FetchFileDiffRequested} returns this
 */
proto.g8e.operator.v1.FetchFileDiffRequested.prototype.setLimit = function(value) {
  return jspb.Message.setProto3IntField(this, 5, value);
};





if (jspb.Message.GENERATE_TO_OBJECT) {
/**
 * Creates an object representation of this proto.
 * Field names that are reserved in JavaScript and will be renamed to pb_name.
 * Optional fields that are not set will be set to undefined.
 * To access a reserved field use, foo.pb_<name>, eg, foo.pb_default.
 * For the list of reserved names please see:
 *     net/proto2/compiler/js/internal/generator.cc#kKeyword.
 * @param {boolean=} opt_includeInstance Deprecated. whether to include the
 *     JSPB instance for transitional soy proto support:
 *     http://goto/soy-param-migration
 * @return {!Object}
 */
proto.g8e.operator.v1.RestoreFileRequested.prototype.toObject = function(opt_includeInstance) {
  return proto.g8e.operator.v1.RestoreFileRequested.toObject(opt_includeInstance, this);
};


/**
 * Static version of the {@see toObject} method.
 * @param {boolean|undefined} includeInstance Deprecated. Whether to include
 *     the JSPB instance for transitional soy proto support:
 *     http://goto/soy-param-migration
 * @param {!proto.g8e.operator.v1.RestoreFileRequested} msg The msg instance to transform.
 * @return {!Object}
 * @suppress {unusedLocalVariables} f is only used for nested messages
 */
proto.g8e.operator.v1.RestoreFileRequested.toObject = function(includeInstance, msg) {
  var f, obj = {
    executionId: jspb.Message.getFieldWithDefault(msg, 1, ""),
    filePath: jspb.Message.getFieldWithDefault(msg, 2, ""),
    commitHash: jspb.Message.getFieldWithDefault(msg, 3, ""),
    operatorSessionId: jspb.Message.getFieldWithDefault(msg, 4, "")
  };

  if (includeInstance) {
    obj.$jspbMessageInstance = msg;
  }
  return obj;
};
}


/**
 * Deserializes binary data (in protobuf wire format).
 * @param {jspb.ByteSource} bytes The bytes to deserialize.
 * @return {!proto.g8e.operator.v1.RestoreFileRequested}
 */
proto.g8e.operator.v1.RestoreFileRequested.deserializeBinary = function(bytes) {
  var reader = new jspb.BinaryReader(bytes);
  var msg = new proto.g8e.operator.v1.RestoreFileRequested;
  return proto.g8e.operator.v1.RestoreFileRequested.deserializeBinaryFromReader(msg, reader);
};


/**
 * Deserializes binary data (in protobuf wire format) from the
 * given reader into the given message object.
 * @param {!proto.g8e.operator.v1.RestoreFileRequested} msg The message object to deserialize into.
 * @param {!jspb.BinaryReader} reader The BinaryReader to use.
 * @return {!proto.g8e.operator.v1.RestoreFileRequested}
 */
proto.g8e.operator.v1.RestoreFileRequested.deserializeBinaryFromReader = function(msg, reader) {
  while (reader.nextField()) {
    if (reader.isEndGroup()) {
      break;
    }
    var field = reader.getFieldNumber();
    switch (field) {
    case 1:
      var value = /** @type {string} */ (reader.readString());
      msg.setExecutionId(value);
      break;
    case 2:
      var value = /** @type {string} */ (reader.readString());
      msg.setFilePath(value);
      break;
    case 3:
      var value = /** @type {string} */ (reader.readString());
      msg.setCommitHash(value);
      break;
    case 4:
      var value = /** @type {string} */ (reader.readString());
      msg.setOperatorSessionId(value);
      break;
    default:
      reader.skipField();
      break;
    }
  }
  return msg;
};


/**
 * Serializes the message to binary data (in protobuf wire format).
 * @return {!Uint8Array}
 */
proto.g8e.operator.v1.RestoreFileRequested.prototype.serializeBinary = function() {
  var writer = new jspb.BinaryWriter();
  proto.g8e.operator.v1.RestoreFileRequested.serializeBinaryToWriter(this, writer);
  return writer.getResultBuffer();
};


/**
 * Serializes the given message to binary data (in protobuf wire
 * format), writing to the given BinaryWriter.
 * @param {!proto.g8e.operator.v1.RestoreFileRequested} message
 * @param {!jspb.BinaryWriter} writer
 * @suppress {unusedLocalVariables} f is only used for nested messages
 */
proto.g8e.operator.v1.RestoreFileRequested.serializeBinaryToWriter = function(message, writer) {
  var f = undefined;
  f = message.getExecutionId();
  if (f.length > 0) {
    writer.writeString(
      1,
      f
    );
  }
  f = message.getFilePath();
  if (f.length > 0) {
    writer.writeString(
      2,
      f
    );
  }
  f = message.getCommitHash();
  if (f.length > 0) {
    writer.writeString(
      3,
      f
    );
  }
  f = message.getOperatorSessionId();
  if (f.length > 0) {
    writer.writeString(
      4,
      f
    );
  }
};


/**
 * optional string execution_id = 1;
 * @return {string}
 */
proto.g8e.operator.v1.RestoreFileRequested.prototype.getExecutionId = function() {
  return /** @type {string} */ (jspb.Message.getFieldWithDefault(this, 1, ""));
};


/**
 * @param {string} value
 * @return {!proto.g8e.operator.v1.RestoreFileRequested} returns this
 */
proto.g8e.operator.v1.RestoreFileRequested.prototype.setExecutionId = function(value) {
  return jspb.Message.setProto3StringField(this, 1, value);
};


/**
 * optional string file_path = 2;
 * @return {string}
 */
proto.g8e.operator.v1.RestoreFileRequested.prototype.getFilePath = function() {
  return /** @type {string} */ (jspb.Message.getFieldWithDefault(this, 2, ""));
};


/**
 * @param {string} value
 * @return {!proto.g8e.operator.v1.RestoreFileRequested} returns this
 */
proto.g8e.operator.v1.RestoreFileRequested.prototype.setFilePath = function(value) {
  return jspb.Message.setProto3StringField(this, 2, value);
};


/**
 * optional string commit_hash = 3;
 * @return {string}
 */
proto.g8e.operator.v1.RestoreFileRequested.prototype.getCommitHash = function() {
  return /** @type {string} */ (jspb.Message.getFieldWithDefault(this, 3, ""));
};


/**
 * @param {string} value
 * @return {!proto.g8e.operator.v1.RestoreFileRequested} returns this
 */
proto.g8e.operator.v1.RestoreFileRequested.prototype.setCommitHash = function(value) {
  return jspb.Message.setProto3StringField(this, 3, value);
};


/**
 * optional string operator_session_id = 4;
 * @return {string}
 */
proto.g8e.operator.v1.RestoreFileRequested.prototype.getOperatorSessionId = function() {
  return /** @type {string} */ (jspb.Message.getFieldWithDefault(this, 4, ""));
};


/**
 * @param {string} value
 * @return {!proto.g8e.operator.v1.RestoreFileRequested} returns this
 */
proto.g8e.operator.v1.RestoreFileRequested.prototype.setOperatorSessionId = function(value) {
  return jspb.Message.setProto3StringField(this, 4, value);
};





if (jspb.Message.GENERATE_TO_OBJECT) {
/**
 * Creates an object representation of this proto.
 * Field names that are reserved in JavaScript and will be renamed to pb_name.
 * Optional fields that are not set will be set to undefined.
 * To access a reserved field use, foo.pb_<name>, eg, foo.pb_default.
 * For the list of reserved names please see:
 *     net/proto2/compiler/js/internal/generator.cc#kKeyword.
 * @param {boolean=} opt_includeInstance Deprecated. whether to include the
 *     JSPB instance for transitional soy proto support:
 *     http://goto/soy-param-migration
 * @return {!Object}
 */
proto.g8e.operator.v1.DirectCommandAuditRequested.prototype.toObject = function(opt_includeInstance) {
  return proto.g8e.operator.v1.DirectCommandAuditRequested.toObject(opt_includeInstance, this);
};


/**
 * Static version of the {@see toObject} method.
 * @param {boolean|undefined} includeInstance Deprecated. Whether to include
 *     the JSPB instance for transitional soy proto support:
 *     http://goto/soy-param-migration
 * @param {!proto.g8e.operator.v1.DirectCommandAuditRequested} msg The msg instance to transform.
 * @return {!Object}
 * @suppress {unusedLocalVariables} f is only used for nested messages
 */
proto.g8e.operator.v1.DirectCommandAuditRequested.toObject = function(includeInstance, msg) {
  var f, obj = {
    command: jspb.Message.getFieldWithDefault(msg, 1, ""),
    executionId: jspb.Message.getFieldWithDefault(msg, 2, ""),
    operatorSessionId: jspb.Message.getFieldWithDefault(msg, 3, ""),
    type: jspb.Message.getFieldWithDefault(msg, 4, "")
  };

  if (includeInstance) {
    obj.$jspbMessageInstance = msg;
  }
  return obj;
};
}


/**
 * Deserializes binary data (in protobuf wire format).
 * @param {jspb.ByteSource} bytes The bytes to deserialize.
 * @return {!proto.g8e.operator.v1.DirectCommandAuditRequested}
 */
proto.g8e.operator.v1.DirectCommandAuditRequested.deserializeBinary = function(bytes) {
  var reader = new jspb.BinaryReader(bytes);
  var msg = new proto.g8e.operator.v1.DirectCommandAuditRequested;
  return proto.g8e.operator.v1.DirectCommandAuditRequested.deserializeBinaryFromReader(msg, reader);
};


/**
 * Deserializes binary data (in protobuf wire format) from the
 * given reader into the given message object.
 * @param {!proto.g8e.operator.v1.DirectCommandAuditRequested} msg The message object to deserialize into.
 * @param {!jspb.BinaryReader} reader The BinaryReader to use.
 * @return {!proto.g8e.operator.v1.DirectCommandAuditRequested}
 */
proto.g8e.operator.v1.DirectCommandAuditRequested.deserializeBinaryFromReader = function(msg, reader) {
  while (reader.nextField()) {
    if (reader.isEndGroup()) {
      break;
    }
    var field = reader.getFieldNumber();
    switch (field) {
    case 1:
      var value = /** @type {string} */ (reader.readString());
      msg.setCommand(value);
      break;
    case 2:
      var value = /** @type {string} */ (reader.readString());
      msg.setExecutionId(value);
      break;
    case 3:
      var value = /** @type {string} */ (reader.readString());
      msg.setOperatorSessionId(value);
      break;
    case 4:
      var value = /** @type {string} */ (reader.readString());
      msg.setType(value);
      break;
    default:
      reader.skipField();
      break;
    }
  }
  return msg;
};


/**
 * Serializes the message to binary data (in protobuf wire format).
 * @return {!Uint8Array}
 */
proto.g8e.operator.v1.DirectCommandAuditRequested.prototype.serializeBinary = function() {
  var writer = new jspb.BinaryWriter();
  proto.g8e.operator.v1.DirectCommandAuditRequested.serializeBinaryToWriter(this, writer);
  return writer.getResultBuffer();
};


/**
 * Serializes the given message to binary data (in protobuf wire
 * format), writing to the given BinaryWriter.
 * @param {!proto.g8e.operator.v1.DirectCommandAuditRequested} message
 * @param {!jspb.BinaryWriter} writer
 * @suppress {unusedLocalVariables} f is only used for nested messages
 */
proto.g8e.operator.v1.DirectCommandAuditRequested.serializeBinaryToWriter = function(message, writer) {
  var f = undefined;
  f = message.getCommand();
  if (f.length > 0) {
    writer.writeString(
      1,
      f
    );
  }
  f = message.getExecutionId();
  if (f.length > 0) {
    writer.writeString(
      2,
      f
    );
  }
  f = message.getOperatorSessionId();
  if (f.length > 0) {
    writer.writeString(
      3,
      f
    );
  }
  f = message.getType();
  if (f.length > 0) {
    writer.writeString(
      4,
      f
    );
  }
};


/**
 * optional string command = 1;
 * @return {string}
 */
proto.g8e.operator.v1.DirectCommandAuditRequested.prototype.getCommand = function() {
  return /** @type {string} */ (jspb.Message.getFieldWithDefault(this, 1, ""));
};


/**
 * @param {string} value
 * @return {!proto.g8e.operator.v1.DirectCommandAuditRequested} returns this
 */
proto.g8e.operator.v1.DirectCommandAuditRequested.prototype.setCommand = function(value) {
  return jspb.Message.setProto3StringField(this, 1, value);
};


/**
 * optional string execution_id = 2;
 * @return {string}
 */
proto.g8e.operator.v1.DirectCommandAuditRequested.prototype.getExecutionId = function() {
  return /** @type {string} */ (jspb.Message.getFieldWithDefault(this, 2, ""));
};


/**
 * @param {string} value
 * @return {!proto.g8e.operator.v1.DirectCommandAuditRequested} returns this
 */
proto.g8e.operator.v1.DirectCommandAuditRequested.prototype.setExecutionId = function(value) {
  return jspb.Message.setProto3StringField(this, 2, value);
};


/**
 * optional string operator_session_id = 3;
 * @return {string}
 */
proto.g8e.operator.v1.DirectCommandAuditRequested.prototype.getOperatorSessionId = function() {
  return /** @type {string} */ (jspb.Message.getFieldWithDefault(this, 3, ""));
};


/**
 * @param {string} value
 * @return {!proto.g8e.operator.v1.DirectCommandAuditRequested} returns this
 */
proto.g8e.operator.v1.DirectCommandAuditRequested.prototype.setOperatorSessionId = function(value) {
  return jspb.Message.setProto3StringField(this, 3, value);
};


/**
 * optional string type = 4;
 * @return {string}
 */
proto.g8e.operator.v1.DirectCommandAuditRequested.prototype.getType = function() {
  return /** @type {string} */ (jspb.Message.getFieldWithDefault(this, 4, ""));
};


/**
 * @param {string} value
 * @return {!proto.g8e.operator.v1.DirectCommandAuditRequested} returns this
 */
proto.g8e.operator.v1.DirectCommandAuditRequested.prototype.setType = function(value) {
  return jspb.Message.setProto3StringField(this, 4, value);
};





if (jspb.Message.GENERATE_TO_OBJECT) {
/**
 * Creates an object representation of this proto.
 * Field names that are reserved in JavaScript and will be renamed to pb_name.
 * Optional fields that are not set will be set to undefined.
 * To access a reserved field use, foo.pb_<name>, eg, foo.pb_default.
 * For the list of reserved names please see:
 *     net/proto2/compiler/js/internal/generator.cc#kKeyword.
 * @param {boolean=} opt_includeInstance Deprecated. whether to include the
 *     JSPB instance for transitional soy proto support:
 *     http://goto/soy-param-migration
 * @return {!Object}
 */
proto.g8e.operator.v1.DirectCommandResultAuditRequested.prototype.toObject = function(opt_includeInstance) {
  return proto.g8e.operator.v1.DirectCommandResultAuditRequested.toObject(opt_includeInstance, this);
};


/**
 * Static version of the {@see toObject} method.
 * @param {boolean|undefined} includeInstance Deprecated. Whether to include
 *     the JSPB instance for transitional soy proto support:
 *     http://goto/soy-param-migration
 * @param {!proto.g8e.operator.v1.DirectCommandResultAuditRequested} msg The msg instance to transform.
 * @return {!Object}
 * @suppress {unusedLocalVariables} f is only used for nested messages
 */
proto.g8e.operator.v1.DirectCommandResultAuditRequested.toObject = function(includeInstance, msg) {
  var f, obj = {
    command: jspb.Message.getFieldWithDefault(msg, 1, ""),
    executionId: jspb.Message.getFieldWithDefault(msg, 2, ""),
    output: jspb.Message.getFieldWithDefault(msg, 3, ""),
    stderr: jspb.Message.getFieldWithDefault(msg, 4, ""),
    exitCode: jspb.Message.getFieldWithDefault(msg, 5, 0),
    executionTimeSeconds: jspb.Message.getFloatingPointFieldWithDefault(msg, 6, 0.0)
  };

  if (includeInstance) {
    obj.$jspbMessageInstance = msg;
  }
  return obj;
};
}


/**
 * Deserializes binary data (in protobuf wire format).
 * @param {jspb.ByteSource} bytes The bytes to deserialize.
 * @return {!proto.g8e.operator.v1.DirectCommandResultAuditRequested}
 */
proto.g8e.operator.v1.DirectCommandResultAuditRequested.deserializeBinary = function(bytes) {
  var reader = new jspb.BinaryReader(bytes);
  var msg = new proto.g8e.operator.v1.DirectCommandResultAuditRequested;
  return proto.g8e.operator.v1.DirectCommandResultAuditRequested.deserializeBinaryFromReader(msg, reader);
};


/**
 * Deserializes binary data (in protobuf wire format) from the
 * given reader into the given message object.
 * @param {!proto.g8e.operator.v1.DirectCommandResultAuditRequested} msg The message object to deserialize into.
 * @param {!jspb.BinaryReader} reader The BinaryReader to use.
 * @return {!proto.g8e.operator.v1.DirectCommandResultAuditRequested}
 */
proto.g8e.operator.v1.DirectCommandResultAuditRequested.deserializeBinaryFromReader = function(msg, reader) {
  while (reader.nextField()) {
    if (reader.isEndGroup()) {
      break;
    }
    var field = reader.getFieldNumber();
    switch (field) {
    case 1:
      var value = /** @type {string} */ (reader.readString());
      msg.setCommand(value);
      break;
    case 2:
      var value = /** @type {string} */ (reader.readString());
      msg.setExecutionId(value);
      break;
    case 3:
      var value = /** @type {string} */ (reader.readString());
      msg.setOutput(value);
      break;
    case 4:
      var value = /** @type {string} */ (reader.readString());
      msg.setStderr(value);
      break;
    case 5:
      var value = /** @type {number} */ (reader.readInt32());
      msg.setExitCode(value);
      break;
    case 6:
      var value = /** @type {number} */ (reader.readFloat());
      msg.setExecutionTimeSeconds(value);
      break;
    default:
      reader.skipField();
      break;
    }
  }
  return msg;
};


/**
 * Serializes the message to binary data (in protobuf wire format).
 * @return {!Uint8Array}
 */
proto.g8e.operator.v1.DirectCommandResultAuditRequested.prototype.serializeBinary = function() {
  var writer = new jspb.BinaryWriter();
  proto.g8e.operator.v1.DirectCommandResultAuditRequested.serializeBinaryToWriter(this, writer);
  return writer.getResultBuffer();
};


/**
 * Serializes the given message to binary data (in protobuf wire
 * format), writing to the given BinaryWriter.
 * @param {!proto.g8e.operator.v1.DirectCommandResultAuditRequested} message
 * @param {!jspb.BinaryWriter} writer
 * @suppress {unusedLocalVariables} f is only used for nested messages
 */
proto.g8e.operator.v1.DirectCommandResultAuditRequested.serializeBinaryToWriter = function(message, writer) {
  var f = undefined;
  f = message.getCommand();
  if (f.length > 0) {
    writer.writeString(
      1,
      f
    );
  }
  f = message.getExecutionId();
  if (f.length > 0) {
    writer.writeString(
      2,
      f
    );
  }
  f = message.getOutput();
  if (f.length > 0) {
    writer.writeString(
      3,
      f
    );
  }
  f = message.getStderr();
  if (f.length > 0) {
    writer.writeString(
      4,
      f
    );
  }
  f = message.getExitCode();
  if (f !== 0) {
    writer.writeInt32(
      5,
      f
    );
  }
  f = message.getExecutionTimeSeconds();
  if (f !== 0.0) {
    writer.writeFloat(
      6,
      f
    );
  }
};


/**
 * optional string command = 1;
 * @return {string}
 */
proto.g8e.operator.v1.DirectCommandResultAuditRequested.prototype.getCommand = function() {
  return /** @type {string} */ (jspb.Message.getFieldWithDefault(this, 1, ""));
};


/**
 * @param {string} value
 * @return {!proto.g8e.operator.v1.DirectCommandResultAuditRequested} returns this
 */
proto.g8e.operator.v1.DirectCommandResultAuditRequested.prototype.setCommand = function(value) {
  return jspb.Message.setProto3StringField(this, 1, value);
};


/**
 * optional string execution_id = 2;
 * @return {string}
 */
proto.g8e.operator.v1.DirectCommandResultAuditRequested.prototype.getExecutionId = function() {
  return /** @type {string} */ (jspb.Message.getFieldWithDefault(this, 2, ""));
};


/**
 * @param {string} value
 * @return {!proto.g8e.operator.v1.DirectCommandResultAuditRequested} returns this
 */
proto.g8e.operator.v1.DirectCommandResultAuditRequested.prototype.setExecutionId = function(value) {
  return jspb.Message.setProto3StringField(this, 2, value);
};


/**
 * optional string output = 3;
 * @return {string}
 */
proto.g8e.operator.v1.DirectCommandResultAuditRequested.prototype.getOutput = function() {
  return /** @type {string} */ (jspb.Message.getFieldWithDefault(this, 3, ""));
};


/**
 * @param {string} value
 * @return {!proto.g8e.operator.v1.DirectCommandResultAuditRequested} returns this
 */
proto.g8e.operator.v1.DirectCommandResultAuditRequested.prototype.setOutput = function(value) {
  return jspb.Message.setProto3StringField(this, 3, value);
};


/**
 * optional string stderr = 4;
 * @return {string}
 */
proto.g8e.operator.v1.DirectCommandResultAuditRequested.prototype.getStderr = function() {
  return /** @type {string} */ (jspb.Message.getFieldWithDefault(this, 4, ""));
};


/**
 * @param {string} value
 * @return {!proto.g8e.operator.v1.DirectCommandResultAuditRequested} returns this
 */
proto.g8e.operator.v1.DirectCommandResultAuditRequested.prototype.setStderr = function(value) {
  return jspb.Message.setProto3StringField(this, 4, value);
};


/**
 * optional int32 exit_code = 5;
 * @return {number}
 */
proto.g8e.operator.v1.DirectCommandResultAuditRequested.prototype.getExitCode = function() {
  return /** @type {number} */ (jspb.Message.getFieldWithDefault(this, 5, 0));
};


/**
 * @param {number} value
 * @return {!proto.g8e.operator.v1.DirectCommandResultAuditRequested} returns this
 */
proto.g8e.operator.v1.DirectCommandResultAuditRequested.prototype.setExitCode = function(value) {
  return jspb.Message.setProto3IntField(this, 5, value);
};


/**
 * optional float execution_time_seconds = 6;
 * @return {number}
 */
proto.g8e.operator.v1.DirectCommandResultAuditRequested.prototype.getExecutionTimeSeconds = function() {
  return /** @type {number} */ (jspb.Message.getFloatingPointFieldWithDefault(this, 6, 0.0));
};


/**
 * @param {number} value
 * @return {!proto.g8e.operator.v1.DirectCommandResultAuditRequested} returns this
 */
proto.g8e.operator.v1.DirectCommandResultAuditRequested.prototype.setExecutionTimeSeconds = function(value) {
  return jspb.Message.setProto3FloatField(this, 6, value);
};





if (jspb.Message.GENERATE_TO_OBJECT) {
/**
 * Creates an object representation of this proto.
 * Field names that are reserved in JavaScript and will be renamed to pb_name.
 * Optional fields that are not set will be set to undefined.
 * To access a reserved field use, foo.pb_<name>, eg, foo.pb_default.
 * For the list of reserved names please see:
 *     net/proto2/compiler/js/internal/generator.cc#kKeyword.
 * @param {boolean=} opt_includeInstance Deprecated. whether to include the
 *     JSPB instance for transitional soy proto support:
 *     http://goto/soy-param-migration
 * @return {!Object}
 */
proto.g8e.operator.v1.AuditMsgRequested.prototype.toObject = function(opt_includeInstance) {
  return proto.g8e.operator.v1.AuditMsgRequested.toObject(opt_includeInstance, this);
};


/**
 * Static version of the {@see toObject} method.
 * @param {boolean|undefined} includeInstance Deprecated. Whether to include
 *     the JSPB instance for transitional soy proto support:
 *     http://goto/soy-param-migration
 * @param {!proto.g8e.operator.v1.AuditMsgRequested} msg The msg instance to transform.
 * @return {!Object}
 * @suppress {unusedLocalVariables} f is only used for nested messages
 */
proto.g8e.operator.v1.AuditMsgRequested.toObject = function(includeInstance, msg) {
  var f, obj = {
    content: jspb.Message.getFieldWithDefault(msg, 1, "")
  };

  if (includeInstance) {
    obj.$jspbMessageInstance = msg;
  }
  return obj;
};
}


/**
 * Deserializes binary data (in protobuf wire format).
 * @param {jspb.ByteSource} bytes The bytes to deserialize.
 * @return {!proto.g8e.operator.v1.AuditMsgRequested}
 */
proto.g8e.operator.v1.AuditMsgRequested.deserializeBinary = function(bytes) {
  var reader = new jspb.BinaryReader(bytes);
  var msg = new proto.g8e.operator.v1.AuditMsgRequested;
  return proto.g8e.operator.v1.AuditMsgRequested.deserializeBinaryFromReader(msg, reader);
};


/**
 * Deserializes binary data (in protobuf wire format) from the
 * given reader into the given message object.
 * @param {!proto.g8e.operator.v1.AuditMsgRequested} msg The message object to deserialize into.
 * @param {!jspb.BinaryReader} reader The BinaryReader to use.
 * @return {!proto.g8e.operator.v1.AuditMsgRequested}
 */
proto.g8e.operator.v1.AuditMsgRequested.deserializeBinaryFromReader = function(msg, reader) {
  while (reader.nextField()) {
    if (reader.isEndGroup()) {
      break;
    }
    var field = reader.getFieldNumber();
    switch (field) {
    case 1:
      var value = /** @type {string} */ (reader.readString());
      msg.setContent(value);
      break;
    default:
      reader.skipField();
      break;
    }
  }
  return msg;
};


/**
 * Serializes the message to binary data (in protobuf wire format).
 * @return {!Uint8Array}
 */
proto.g8e.operator.v1.AuditMsgRequested.prototype.serializeBinary = function() {
  var writer = new jspb.BinaryWriter();
  proto.g8e.operator.v1.AuditMsgRequested.serializeBinaryToWriter(this, writer);
  return writer.getResultBuffer();
};


/**
 * Serializes the given message to binary data (in protobuf wire
 * format), writing to the given BinaryWriter.
 * @param {!proto.g8e.operator.v1.AuditMsgRequested} message
 * @param {!jspb.BinaryWriter} writer
 * @suppress {unusedLocalVariables} f is only used for nested messages
 */
proto.g8e.operator.v1.AuditMsgRequested.serializeBinaryToWriter = function(message, writer) {
  var f = undefined;
  f = message.getContent();
  if (f.length > 0) {
    writer.writeString(
      1,
      f
    );
  }
};


/**
 * optional string content = 1;
 * @return {string}
 */
proto.g8e.operator.v1.AuditMsgRequested.prototype.getContent = function() {
  return /** @type {string} */ (jspb.Message.getFieldWithDefault(this, 1, ""));
};


/**
 * @param {string} value
 * @return {!proto.g8e.operator.v1.AuditMsgRequested} returns this
 */
proto.g8e.operator.v1.AuditMsgRequested.prototype.setContent = function(value) {
  return jspb.Message.setProto3StringField(this, 1, value);
};





if (jspb.Message.GENERATE_TO_OBJECT) {
/**
 * Creates an object representation of this proto.
 * Field names that are reserved in JavaScript and will be renamed to pb_name.
 * Optional fields that are not set will be set to undefined.
 * To access a reserved field use, foo.pb_<name>, eg, foo.pb_default.
 * For the list of reserved names please see:
 *     net/proto2/compiler/js/internal/generator.cc#kKeyword.
 * @param {boolean=} opt_includeInstance Deprecated. whether to include the
 *     JSPB instance for transitional soy proto support:
 *     http://goto/soy-param-migration
 * @return {!Object}
 */
proto.g8e.operator.v1.SignCertificateRequested.prototype.toObject = function(opt_includeInstance) {
  return proto.g8e.operator.v1.SignCertificateRequested.toObject(opt_includeInstance, this);
};


/**
 * Static version of the {@see toObject} method.
 * @param {boolean|undefined} includeInstance Deprecated. Whether to include
 *     the JSPB instance for transitional soy proto support:
 *     http://goto/soy-param-migration
 * @param {!proto.g8e.operator.v1.SignCertificateRequested} msg The msg instance to transform.
 * @return {!Object}
 * @suppress {unusedLocalVariables} f is only used for nested messages
 */
proto.g8e.operator.v1.SignCertificateRequested.toObject = function(includeInstance, msg) {
  var f, obj = {
    publicKeyPem: jspb.Message.getFieldWithDefault(msg, 1, ""),
    commonName: jspb.Message.getFieldWithDefault(msg, 2, ""),
    organizationalUnit: jspb.Message.getFieldWithDefault(msg, 3, ""),
    validityDays: jspb.Message.getFieldWithDefault(msg, 4, 0)
  };

  if (includeInstance) {
    obj.$jspbMessageInstance = msg;
  }
  return obj;
};
}


/**
 * Deserializes binary data (in protobuf wire format).
 * @param {jspb.ByteSource} bytes The bytes to deserialize.
 * @return {!proto.g8e.operator.v1.SignCertificateRequested}
 */
proto.g8e.operator.v1.SignCertificateRequested.deserializeBinary = function(bytes) {
  var reader = new jspb.BinaryReader(bytes);
  var msg = new proto.g8e.operator.v1.SignCertificateRequested;
  return proto.g8e.operator.v1.SignCertificateRequested.deserializeBinaryFromReader(msg, reader);
};


/**
 * Deserializes binary data (in protobuf wire format) from the
 * given reader into the given message object.
 * @param {!proto.g8e.operator.v1.SignCertificateRequested} msg The message object to deserialize into.
 * @param {!jspb.BinaryReader} reader The BinaryReader to use.
 * @return {!proto.g8e.operator.v1.SignCertificateRequested}
 */
proto.g8e.operator.v1.SignCertificateRequested.deserializeBinaryFromReader = function(msg, reader) {
  while (reader.nextField()) {
    if (reader.isEndGroup()) {
      break;
    }
    var field = reader.getFieldNumber();
    switch (field) {
    case 1:
      var value = /** @type {string} */ (reader.readString());
      msg.setPublicKeyPem(value);
      break;
    case 2:
      var value = /** @type {string} */ (reader.readString());
      msg.setCommonName(value);
      break;
    case 3:
      var value = /** @type {string} */ (reader.readString());
      msg.setOrganizationalUnit(value);
      break;
    case 4:
      var value = /** @type {number} */ (reader.readInt32());
      msg.setValidityDays(value);
      break;
    default:
      reader.skipField();
      break;
    }
  }
  return msg;
};


/**
 * Serializes the message to binary data (in protobuf wire format).
 * @return {!Uint8Array}
 */
proto.g8e.operator.v1.SignCertificateRequested.prototype.serializeBinary = function() {
  var writer = new jspb.BinaryWriter();
  proto.g8e.operator.v1.SignCertificateRequested.serializeBinaryToWriter(this, writer);
  return writer.getResultBuffer();
};


/**
 * Serializes the given message to binary data (in protobuf wire
 * format), writing to the given BinaryWriter.
 * @param {!proto.g8e.operator.v1.SignCertificateRequested} message
 * @param {!jspb.BinaryWriter} writer
 * @suppress {unusedLocalVariables} f is only used for nested messages
 */
proto.g8e.operator.v1.SignCertificateRequested.serializeBinaryToWriter = function(message, writer) {
  var f = undefined;
  f = message.getPublicKeyPem();
  if (f.length > 0) {
    writer.writeString(
      1,
      f
    );
  }
  f = message.getCommonName();
  if (f.length > 0) {
    writer.writeString(
      2,
      f
    );
  }
  f = message.getOrganizationalUnit();
  if (f.length > 0) {
    writer.writeString(
      3,
      f
    );
  }
  f = message.getValidityDays();
  if (f !== 0) {
    writer.writeInt32(
      4,
      f
    );
  }
};


/**
 * optional string public_key_pem = 1;
 * @return {string}
 */
proto.g8e.operator.v1.SignCertificateRequested.prototype.getPublicKeyPem = function() {
  return /** @type {string} */ (jspb.Message.getFieldWithDefault(this, 1, ""));
};


/**
 * @param {string} value
 * @return {!proto.g8e.operator.v1.SignCertificateRequested} returns this
 */
proto.g8e.operator.v1.SignCertificateRequested.prototype.setPublicKeyPem = function(value) {
  return jspb.Message.setProto3StringField(this, 1, value);
};


/**
 * optional string common_name = 2;
 * @return {string}
 */
proto.g8e.operator.v1.SignCertificateRequested.prototype.getCommonName = function() {
  return /** @type {string} */ (jspb.Message.getFieldWithDefault(this, 2, ""));
};


/**
 * @param {string} value
 * @return {!proto.g8e.operator.v1.SignCertificateRequested} returns this
 */
proto.g8e.operator.v1.SignCertificateRequested.prototype.setCommonName = function(value) {
  return jspb.Message.setProto3StringField(this, 2, value);
};


/**
 * optional string organizational_unit = 3;
 * @return {string}
 */
proto.g8e.operator.v1.SignCertificateRequested.prototype.getOrganizationalUnit = function() {
  return /** @type {string} */ (jspb.Message.getFieldWithDefault(this, 3, ""));
};


/**
 * @param {string} value
 * @return {!proto.g8e.operator.v1.SignCertificateRequested} returns this
 */
proto.g8e.operator.v1.SignCertificateRequested.prototype.setOrganizationalUnit = function(value) {
  return jspb.Message.setProto3StringField(this, 3, value);
};


/**
 * optional int32 validity_days = 4;
 * @return {number}
 */
proto.g8e.operator.v1.SignCertificateRequested.prototype.getValidityDays = function() {
  return /** @type {number} */ (jspb.Message.getFieldWithDefault(this, 4, 0));
};


/**
 * @param {number} value
 * @return {!proto.g8e.operator.v1.SignCertificateRequested} returns this
 */
proto.g8e.operator.v1.SignCertificateRequested.prototype.setValidityDays = function(value) {
  return jspb.Message.setProto3IntField(this, 4, value);
};





if (jspb.Message.GENERATE_TO_OBJECT) {
/**
 * Creates an object representation of this proto.
 * Field names that are reserved in JavaScript and will be renamed to pb_name.
 * Optional fields that are not set will be set to undefined.
 * To access a reserved field use, foo.pb_<name>, eg, foo.pb_default.
 * For the list of reserved names please see:
 *     net/proto2/compiler/js/internal/generator.cc#kKeyword.
 * @param {boolean=} opt_includeInstance Deprecated. whether to include the
 *     JSPB instance for transitional soy proto support:
 *     http://goto/soy-param-migration
 * @return {!Object}
 */
proto.g8e.operator.v1.SignCertificateResult.prototype.toObject = function(opt_includeInstance) {
  return proto.g8e.operator.v1.SignCertificateResult.toObject(opt_includeInstance, this);
};


/**
 * Static version of the {@see toObject} method.
 * @param {boolean|undefined} includeInstance Deprecated. Whether to include
 *     the JSPB instance for transitional soy proto support:
 *     http://goto/soy-param-migration
 * @param {!proto.g8e.operator.v1.SignCertificateResult} msg The msg instance to transform.
 * @return {!Object}
 * @suppress {unusedLocalVariables} f is only used for nested messages
 */
proto.g8e.operator.v1.SignCertificateResult.toObject = function(includeInstance, msg) {
  var f, obj = {
    success: jspb.Message.getBooleanFieldWithDefault(msg, 1, false),
    certificatePem: jspb.Message.getFieldWithDefault(msg, 2, ""),
    serial: jspb.Message.getFieldWithDefault(msg, 3, ""),
    error: jspb.Message.getFieldWithDefault(msg, 4, "")
  };

  if (includeInstance) {
    obj.$jspbMessageInstance = msg;
  }
  return obj;
};
}


/**
 * Deserializes binary data (in protobuf wire format).
 * @param {jspb.ByteSource} bytes The bytes to deserialize.
 * @return {!proto.g8e.operator.v1.SignCertificateResult}
 */
proto.g8e.operator.v1.SignCertificateResult.deserializeBinary = function(bytes) {
  var reader = new jspb.BinaryReader(bytes);
  var msg = new proto.g8e.operator.v1.SignCertificateResult;
  return proto.g8e.operator.v1.SignCertificateResult.deserializeBinaryFromReader(msg, reader);
};


/**
 * Deserializes binary data (in protobuf wire format) from the
 * given reader into the given message object.
 * @param {!proto.g8e.operator.v1.SignCertificateResult} msg The message object to deserialize into.
 * @param {!jspb.BinaryReader} reader The BinaryReader to use.
 * @return {!proto.g8e.operator.v1.SignCertificateResult}
 */
proto.g8e.operator.v1.SignCertificateResult.deserializeBinaryFromReader = function(msg, reader) {
  while (reader.nextField()) {
    if (reader.isEndGroup()) {
      break;
    }
    var field = reader.getFieldNumber();
    switch (field) {
    case 1:
      var value = /** @type {boolean} */ (reader.readBool());
      msg.setSuccess(value);
      break;
    case 2:
      var value = /** @type {string} */ (reader.readString());
      msg.setCertificatePem(value);
      break;
    case 3:
      var value = /** @type {string} */ (reader.readString());
      msg.setSerial(value);
      break;
    case 4:
      var value = /** @type {string} */ (reader.readString());
      msg.setError(value);
      break;
    default:
      reader.skipField();
      break;
    }
  }
  return msg;
};


/**
 * Serializes the message to binary data (in protobuf wire format).
 * @return {!Uint8Array}
 */
proto.g8e.operator.v1.SignCertificateResult.prototype.serializeBinary = function() {
  var writer = new jspb.BinaryWriter();
  proto.g8e.operator.v1.SignCertificateResult.serializeBinaryToWriter(this, writer);
  return writer.getResultBuffer();
};


/**
 * Serializes the given message to binary data (in protobuf wire
 * format), writing to the given BinaryWriter.
 * @param {!proto.g8e.operator.v1.SignCertificateResult} message
 * @param {!jspb.BinaryWriter} writer
 * @suppress {unusedLocalVariables} f is only used for nested messages
 */
proto.g8e.operator.v1.SignCertificateResult.serializeBinaryToWriter = function(message, writer) {
  var f = undefined;
  f = message.getSuccess();
  if (f) {
    writer.writeBool(
      1,
      f
    );
  }
  f = message.getCertificatePem();
  if (f.length > 0) {
    writer.writeString(
      2,
      f
    );
  }
  f = message.getSerial();
  if (f.length > 0) {
    writer.writeString(
      3,
      f
    );
  }
  f = message.getError();
  if (f.length > 0) {
    writer.writeString(
      4,
      f
    );
  }
};


/**
 * optional bool success = 1;
 * @return {boolean}
 */
proto.g8e.operator.v1.SignCertificateResult.prototype.getSuccess = function() {
  return /** @type {boolean} */ (jspb.Message.getBooleanFieldWithDefault(this, 1, false));
};


/**
 * @param {boolean} value
 * @return {!proto.g8e.operator.v1.SignCertificateResult} returns this
 */
proto.g8e.operator.v1.SignCertificateResult.prototype.setSuccess = function(value) {
  return jspb.Message.setProto3BooleanField(this, 1, value);
};


/**
 * optional string certificate_pem = 2;
 * @return {string}
 */
proto.g8e.operator.v1.SignCertificateResult.prototype.getCertificatePem = function() {
  return /** @type {string} */ (jspb.Message.getFieldWithDefault(this, 2, ""));
};


/**
 * @param {string} value
 * @return {!proto.g8e.operator.v1.SignCertificateResult} returns this
 */
proto.g8e.operator.v1.SignCertificateResult.prototype.setCertificatePem = function(value) {
  return jspb.Message.setProto3StringField(this, 2, value);
};


/**
 * optional string serial = 3;
 * @return {string}
 */
proto.g8e.operator.v1.SignCertificateResult.prototype.getSerial = function() {
  return /** @type {string} */ (jspb.Message.getFieldWithDefault(this, 3, ""));
};


/**
 * @param {string} value
 * @return {!proto.g8e.operator.v1.SignCertificateResult} returns this
 */
proto.g8e.operator.v1.SignCertificateResult.prototype.setSerial = function(value) {
  return jspb.Message.setProto3StringField(this, 3, value);
};


/**
 * optional string error = 4;
 * @return {string}
 */
proto.g8e.operator.v1.SignCertificateResult.prototype.getError = function() {
  return /** @type {string} */ (jspb.Message.getFieldWithDefault(this, 4, ""));
};


/**
 * @param {string} value
 * @return {!proto.g8e.operator.v1.SignCertificateResult} returns this
 */
proto.g8e.operator.v1.SignCertificateResult.prototype.setError = function(value) {
  return jspb.Message.setProto3StringField(this, 4, value);
};





if (jspb.Message.GENERATE_TO_OBJECT) {
/**
 * Creates an object representation of this proto.
 * Field names that are reserved in JavaScript and will be renamed to pb_name.
 * Optional fields that are not set will be set to undefined.
 * To access a reserved field use, foo.pb_<name>, eg, foo.pb_default.
 * For the list of reserved names please see:
 *     net/proto2/compiler/js/internal/generator.cc#kKeyword.
 * @param {boolean=} opt_includeInstance Deprecated. whether to include the
 *     JSPB instance for transitional soy proto support:
 *     http://goto/soy-param-migration
 * @return {!Object}
 */
proto.g8e.operator.v1.CreateDeviceLinkRequested.prototype.toObject = function(opt_includeInstance) {
  return proto.g8e.operator.v1.CreateDeviceLinkRequested.toObject(opt_includeInstance, this);
};


/**
 * Static version of the {@see toObject} method.
 * @param {boolean|undefined} includeInstance Deprecated. Whether to include
 *     the JSPB instance for transitional soy proto support:
 *     http://goto/soy-param-migration
 * @param {!proto.g8e.operator.v1.CreateDeviceLinkRequested} msg The msg instance to transform.
 * @return {!Object}
 * @suppress {unusedLocalVariables} f is only used for nested messages
 */
proto.g8e.operator.v1.CreateDeviceLinkRequested.toObject = function(includeInstance, msg) {
  var f, obj = {
    userId: jspb.Message.getFieldWithDefault(msg, 1, ""),
    organizationId: jspb.Message.getFieldWithDefault(msg, 2, ""),
    operatorId: jspb.Message.getFieldWithDefault(msg, 3, ""),
    webSessionId: jspb.Message.getFieldWithDefault(msg, 4, ""),
    name: jspb.Message.getFieldWithDefault(msg, 5, ""),
    maxUses: jspb.Message.getFieldWithDefault(msg, 6, 0),
    ttlSeconds: jspb.Message.getFieldWithDefault(msg, 7, 0)
  };

  if (includeInstance) {
    obj.$jspbMessageInstance = msg;
  }
  return obj;
};
}


/**
 * Deserializes binary data (in protobuf wire format).
 * @param {jspb.ByteSource} bytes The bytes to deserialize.
 * @return {!proto.g8e.operator.v1.CreateDeviceLinkRequested}
 */
proto.g8e.operator.v1.CreateDeviceLinkRequested.deserializeBinary = function(bytes) {
  var reader = new jspb.BinaryReader(bytes);
  var msg = new proto.g8e.operator.v1.CreateDeviceLinkRequested;
  return proto.g8e.operator.v1.CreateDeviceLinkRequested.deserializeBinaryFromReader(msg, reader);
};


/**
 * Deserializes binary data (in protobuf wire format) from the
 * given reader into the given message object.
 * @param {!proto.g8e.operator.v1.CreateDeviceLinkRequested} msg The message object to deserialize into.
 * @param {!jspb.BinaryReader} reader The BinaryReader to use.
 * @return {!proto.g8e.operator.v1.CreateDeviceLinkRequested}
 */
proto.g8e.operator.v1.CreateDeviceLinkRequested.deserializeBinaryFromReader = function(msg, reader) {
  while (reader.nextField()) {
    if (reader.isEndGroup()) {
      break;
    }
    var field = reader.getFieldNumber();
    switch (field) {
    case 1:
      var value = /** @type {string} */ (reader.readString());
      msg.setUserId(value);
      break;
    case 2:
      var value = /** @type {string} */ (reader.readString());
      msg.setOrganizationId(value);
      break;
    case 3:
      var value = /** @type {string} */ (reader.readString());
      msg.setOperatorId(value);
      break;
    case 4:
      var value = /** @type {string} */ (reader.readString());
      msg.setWebSessionId(value);
      break;
    case 5:
      var value = /** @type {string} */ (reader.readString());
      msg.setName(value);
      break;
    case 6:
      var value = /** @type {number} */ (reader.readInt32());
      msg.setMaxUses(value);
      break;
    case 7:
      var value = /** @type {number} */ (reader.readInt32());
      msg.setTtlSeconds(value);
      break;
    default:
      reader.skipField();
      break;
    }
  }
  return msg;
};


/**
 * Serializes the message to binary data (in protobuf wire format).
 * @return {!Uint8Array}
 */
proto.g8e.operator.v1.CreateDeviceLinkRequested.prototype.serializeBinary = function() {
  var writer = new jspb.BinaryWriter();
  proto.g8e.operator.v1.CreateDeviceLinkRequested.serializeBinaryToWriter(this, writer);
  return writer.getResultBuffer();
};


/**
 * Serializes the given message to binary data (in protobuf wire
 * format), writing to the given BinaryWriter.
 * @param {!proto.g8e.operator.v1.CreateDeviceLinkRequested} message
 * @param {!jspb.BinaryWriter} writer
 * @suppress {unusedLocalVariables} f is only used for nested messages
 */
proto.g8e.operator.v1.CreateDeviceLinkRequested.serializeBinaryToWriter = function(message, writer) {
  var f = undefined;
  f = message.getUserId();
  if (f.length > 0) {
    writer.writeString(
      1,
      f
    );
  }
  f = message.getOrganizationId();
  if (f.length > 0) {
    writer.writeString(
      2,
      f
    );
  }
  f = message.getOperatorId();
  if (f.length > 0) {
    writer.writeString(
      3,
      f
    );
  }
  f = message.getWebSessionId();
  if (f.length > 0) {
    writer.writeString(
      4,
      f
    );
  }
  f = message.getName();
  if (f.length > 0) {
    writer.writeString(
      5,
      f
    );
  }
  f = message.getMaxUses();
  if (f !== 0) {
    writer.writeInt32(
      6,
      f
    );
  }
  f = message.getTtlSeconds();
  if (f !== 0) {
    writer.writeInt32(
      7,
      f
    );
  }
};


/**
 * optional string user_id = 1;
 * @return {string}
 */
proto.g8e.operator.v1.CreateDeviceLinkRequested.prototype.getUserId = function() {
  return /** @type {string} */ (jspb.Message.getFieldWithDefault(this, 1, ""));
};


/**
 * @param {string} value
 * @return {!proto.g8e.operator.v1.CreateDeviceLinkRequested} returns this
 */
proto.g8e.operator.v1.CreateDeviceLinkRequested.prototype.setUserId = function(value) {
  return jspb.Message.setProto3StringField(this, 1, value);
};


/**
 * optional string organization_id = 2;
 * @return {string}
 */
proto.g8e.operator.v1.CreateDeviceLinkRequested.prototype.getOrganizationId = function() {
  return /** @type {string} */ (jspb.Message.getFieldWithDefault(this, 2, ""));
};


/**
 * @param {string} value
 * @return {!proto.g8e.operator.v1.CreateDeviceLinkRequested} returns this
 */
proto.g8e.operator.v1.CreateDeviceLinkRequested.prototype.setOrganizationId = function(value) {
  return jspb.Message.setProto3StringField(this, 2, value);
};


/**
 * optional string operator_id = 3;
 * @return {string}
 */
proto.g8e.operator.v1.CreateDeviceLinkRequested.prototype.getOperatorId = function() {
  return /** @type {string} */ (jspb.Message.getFieldWithDefault(this, 3, ""));
};


/**
 * @param {string} value
 * @return {!proto.g8e.operator.v1.CreateDeviceLinkRequested} returns this
 */
proto.g8e.operator.v1.CreateDeviceLinkRequested.prototype.setOperatorId = function(value) {
  return jspb.Message.setProto3StringField(this, 3, value);
};


/**
 * optional string web_session_id = 4;
 * @return {string}
 */
proto.g8e.operator.v1.CreateDeviceLinkRequested.prototype.getWebSessionId = function() {
  return /** @type {string} */ (jspb.Message.getFieldWithDefault(this, 4, ""));
};


/**
 * @param {string} value
 * @return {!proto.g8e.operator.v1.CreateDeviceLinkRequested} returns this
 */
proto.g8e.operator.v1.CreateDeviceLinkRequested.prototype.setWebSessionId = function(value) {
  return jspb.Message.setProto3StringField(this, 4, value);
};


/**
 * optional string name = 5;
 * @return {string}
 */
proto.g8e.operator.v1.CreateDeviceLinkRequested.prototype.getName = function() {
  return /** @type {string} */ (jspb.Message.getFieldWithDefault(this, 5, ""));
};


/**
 * @param {string} value
 * @return {!proto.g8e.operator.v1.CreateDeviceLinkRequested} returns this
 */
proto.g8e.operator.v1.CreateDeviceLinkRequested.prototype.setName = function(value) {
  return jspb.Message.setProto3StringField(this, 5, value);
};


/**
 * optional int32 max_uses = 6;
 * @return {number}
 */
proto.g8e.operator.v1.CreateDeviceLinkRequested.prototype.getMaxUses = function() {
  return /** @type {number} */ (jspb.Message.getFieldWithDefault(this, 6, 0));
};


/**
 * @param {number} value
 * @return {!proto.g8e.operator.v1.CreateDeviceLinkRequested} returns this
 */
proto.g8e.operator.v1.CreateDeviceLinkRequested.prototype.setMaxUses = function(value) {
  return jspb.Message.setProto3IntField(this, 6, value);
};


/**
 * optional int32 ttl_seconds = 7;
 * @return {number}
 */
proto.g8e.operator.v1.CreateDeviceLinkRequested.prototype.getTtlSeconds = function() {
  return /** @type {number} */ (jspb.Message.getFieldWithDefault(this, 7, 0));
};


/**
 * @param {number} value
 * @return {!proto.g8e.operator.v1.CreateDeviceLinkRequested} returns this
 */
proto.g8e.operator.v1.CreateDeviceLinkRequested.prototype.setTtlSeconds = function(value) {
  return jspb.Message.setProto3IntField(this, 7, value);
};





if (jspb.Message.GENERATE_TO_OBJECT) {
/**
 * Creates an object representation of this proto.
 * Field names that are reserved in JavaScript and will be renamed to pb_name.
 * Optional fields that are not set will be set to undefined.
 * To access a reserved field use, foo.pb_<name>, eg, foo.pb_default.
 * For the list of reserved names please see:
 *     net/proto2/compiler/js/internal/generator.cc#kKeyword.
 * @param {boolean=} opt_includeInstance Deprecated. whether to include the
 *     JSPB instance for transitional soy proto support:
 *     http://goto/soy-param-migration
 * @return {!Object}
 */
proto.g8e.operator.v1.DeviceLink.prototype.toObject = function(opt_includeInstance) {
  return proto.g8e.operator.v1.DeviceLink.toObject(opt_includeInstance, this);
};


/**
 * Static version of the {@see toObject} method.
 * @param {boolean|undefined} includeInstance Deprecated. Whether to include
 *     the JSPB instance for transitional soy proto support:
 *     http://goto/soy-param-migration
 * @param {!proto.g8e.operator.v1.DeviceLink} msg The msg instance to transform.
 * @return {!Object}
 * @suppress {unusedLocalVariables} f is only used for nested messages
 */
proto.g8e.operator.v1.DeviceLink.toObject = function(includeInstance, msg) {
  var f, obj = {
    token: jspb.Message.getFieldWithDefault(msg, 1, ""),
    userId: jspb.Message.getFieldWithDefault(msg, 2, ""),
    organizationId: jspb.Message.getFieldWithDefault(msg, 3, ""),
    operatorId: jspb.Message.getFieldWithDefault(msg, 4, ""),
    webSessionId: jspb.Message.getFieldWithDefault(msg, 5, ""),
    name: jspb.Message.getFieldWithDefault(msg, 6, ""),
    maxUses: jspb.Message.getFieldWithDefault(msg, 7, 0),
    uses: jspb.Message.getFieldWithDefault(msg, 8, 0),
    status: jspb.Message.getFieldWithDefault(msg, 9, ""),
    createdAtUnixMs: jspb.Message.getFieldWithDefault(msg, 10, 0),
    expiresAtUnixMs: jspb.Message.getFieldWithDefault(msg, 11, 0)
  };

  if (includeInstance) {
    obj.$jspbMessageInstance = msg;
  }
  return obj;
};
}


/**
 * Deserializes binary data (in protobuf wire format).
 * @param {jspb.ByteSource} bytes The bytes to deserialize.
 * @return {!proto.g8e.operator.v1.DeviceLink}
 */
proto.g8e.operator.v1.DeviceLink.deserializeBinary = function(bytes) {
  var reader = new jspb.BinaryReader(bytes);
  var msg = new proto.g8e.operator.v1.DeviceLink;
  return proto.g8e.operator.v1.DeviceLink.deserializeBinaryFromReader(msg, reader);
};


/**
 * Deserializes binary data (in protobuf wire format) from the
 * given reader into the given message object.
 * @param {!proto.g8e.operator.v1.DeviceLink} msg The message object to deserialize into.
 * @param {!jspb.BinaryReader} reader The BinaryReader to use.
 * @return {!proto.g8e.operator.v1.DeviceLink}
 */
proto.g8e.operator.v1.DeviceLink.deserializeBinaryFromReader = function(msg, reader) {
  while (reader.nextField()) {
    if (reader.isEndGroup()) {
      break;
    }
    var field = reader.getFieldNumber();
    switch (field) {
    case 1:
      var value = /** @type {string} */ (reader.readString());
      msg.setToken(value);
      break;
    case 2:
      var value = /** @type {string} */ (reader.readString());
      msg.setUserId(value);
      break;
    case 3:
      var value = /** @type {string} */ (reader.readString());
      msg.setOrganizationId(value);
      break;
    case 4:
      var value = /** @type {string} */ (reader.readString());
      msg.setOperatorId(value);
      break;
    case 5:
      var value = /** @type {string} */ (reader.readString());
      msg.setWebSessionId(value);
      break;
    case 6:
      var value = /** @type {string} */ (reader.readString());
      msg.setName(value);
      break;
    case 7:
      var value = /** @type {number} */ (reader.readInt32());
      msg.setMaxUses(value);
      break;
    case 8:
      var value = /** @type {number} */ (reader.readInt32());
      msg.setUses(value);
      break;
    case 9:
      var value = /** @type {string} */ (reader.readString());
      msg.setStatus(value);
      break;
    case 10:
      var value = /** @type {number} */ (reader.readInt64());
      msg.setCreatedAtUnixMs(value);
      break;
    case 11:
      var value = /** @type {number} */ (reader.readInt64());
      msg.setExpiresAtUnixMs(value);
      break;
    default:
      reader.skipField();
      break;
    }
  }
  return msg;
};


/**
 * Serializes the message to binary data (in protobuf wire format).
 * @return {!Uint8Array}
 */
proto.g8e.operator.v1.DeviceLink.prototype.serializeBinary = function() {
  var writer = new jspb.BinaryWriter();
  proto.g8e.operator.v1.DeviceLink.serializeBinaryToWriter(this, writer);
  return writer.getResultBuffer();
};


/**
 * Serializes the given message to binary data (in protobuf wire
 * format), writing to the given BinaryWriter.
 * @param {!proto.g8e.operator.v1.DeviceLink} message
 * @param {!jspb.BinaryWriter} writer
 * @suppress {unusedLocalVariables} f is only used for nested messages
 */
proto.g8e.operator.v1.DeviceLink.serializeBinaryToWriter = function(message, writer) {
  var f = undefined;
  f = message.getToken();
  if (f.length > 0) {
    writer.writeString(
      1,
      f
    );
  }
  f = message.getUserId();
  if (f.length > 0) {
    writer.writeString(
      2,
      f
    );
  }
  f = message.getOrganizationId();
  if (f.length > 0) {
    writer.writeString(
      3,
      f
    );
  }
  f = message.getOperatorId();
  if (f.length > 0) {
    writer.writeString(
      4,
      f
    );
  }
  f = message.getWebSessionId();
  if (f.length > 0) {
    writer.writeString(
      5,
      f
    );
  }
  f = message.getName();
  if (f.length > 0) {
    writer.writeString(
      6,
      f
    );
  }
  f = message.getMaxUses();
  if (f !== 0) {
    writer.writeInt32(
      7,
      f
    );
  }
  f = message.getUses();
  if (f !== 0) {
    writer.writeInt32(
      8,
      f
    );
  }
  f = message.getStatus();
  if (f.length > 0) {
    writer.writeString(
      9,
      f
    );
  }
  f = message.getCreatedAtUnixMs();
  if (f !== 0) {
    writer.writeInt64(
      10,
      f
    );
  }
  f = message.getExpiresAtUnixMs();
  if (f !== 0) {
    writer.writeInt64(
      11,
      f
    );
  }
};


/**
 * optional string token = 1;
 * @return {string}
 */
proto.g8e.operator.v1.DeviceLink.prototype.getToken = function() {
  return /** @type {string} */ (jspb.Message.getFieldWithDefault(this, 1, ""));
};


/**
 * @param {string} value
 * @return {!proto.g8e.operator.v1.DeviceLink} returns this
 */
proto.g8e.operator.v1.DeviceLink.prototype.setToken = function(value) {
  return jspb.Message.setProto3StringField(this, 1, value);
};


/**
 * optional string user_id = 2;
 * @return {string}
 */
proto.g8e.operator.v1.DeviceLink.prototype.getUserId = function() {
  return /** @type {string} */ (jspb.Message.getFieldWithDefault(this, 2, ""));
};


/**
 * @param {string} value
 * @return {!proto.g8e.operator.v1.DeviceLink} returns this
 */
proto.g8e.operator.v1.DeviceLink.prototype.setUserId = function(value) {
  return jspb.Message.setProto3StringField(this, 2, value);
};


/**
 * optional string organization_id = 3;
 * @return {string}
 */
proto.g8e.operator.v1.DeviceLink.prototype.getOrganizationId = function() {
  return /** @type {string} */ (jspb.Message.getFieldWithDefault(this, 3, ""));
};


/**
 * @param {string} value
 * @return {!proto.g8e.operator.v1.DeviceLink} returns this
 */
proto.g8e.operator.v1.DeviceLink.prototype.setOrganizationId = function(value) {
  return jspb.Message.setProto3StringField(this, 3, value);
};


/**
 * optional string operator_id = 4;
 * @return {string}
 */
proto.g8e.operator.v1.DeviceLink.prototype.getOperatorId = function() {
  return /** @type {string} */ (jspb.Message.getFieldWithDefault(this, 4, ""));
};


/**
 * @param {string} value
 * @return {!proto.g8e.operator.v1.DeviceLink} returns this
 */
proto.g8e.operator.v1.DeviceLink.prototype.setOperatorId = function(value) {
  return jspb.Message.setProto3StringField(this, 4, value);
};


/**
 * optional string web_session_id = 5;
 * @return {string}
 */
proto.g8e.operator.v1.DeviceLink.prototype.getWebSessionId = function() {
  return /** @type {string} */ (jspb.Message.getFieldWithDefault(this, 5, ""));
};


/**
 * @param {string} value
 * @return {!proto.g8e.operator.v1.DeviceLink} returns this
 */
proto.g8e.operator.v1.DeviceLink.prototype.setWebSessionId = function(value) {
  return jspb.Message.setProto3StringField(this, 5, value);
};


/**
 * optional string name = 6;
 * @return {string}
 */
proto.g8e.operator.v1.DeviceLink.prototype.getName = function() {
  return /** @type {string} */ (jspb.Message.getFieldWithDefault(this, 6, ""));
};


/**
 * @param {string} value
 * @return {!proto.g8e.operator.v1.DeviceLink} returns this
 */
proto.g8e.operator.v1.DeviceLink.prototype.setName = function(value) {
  return jspb.Message.setProto3StringField(this, 6, value);
};


/**
 * optional int32 max_uses = 7;
 * @return {number}
 */
proto.g8e.operator.v1.DeviceLink.prototype.getMaxUses = function() {
  return /** @type {number} */ (jspb.Message.getFieldWithDefault(this, 7, 0));
};


/**
 * @param {number} value
 * @return {!proto.g8e.operator.v1.DeviceLink} returns this
 */
proto.g8e.operator.v1.DeviceLink.prototype.setMaxUses = function(value) {
  return jspb.Message.setProto3IntField(this, 7, value);
};


/**
 * optional int32 uses = 8;
 * @return {number}
 */
proto.g8e.operator.v1.DeviceLink.prototype.getUses = function() {
  return /** @type {number} */ (jspb.Message.getFieldWithDefault(this, 8, 0));
};


/**
 * @param {number} value
 * @return {!proto.g8e.operator.v1.DeviceLink} returns this
 */
proto.g8e.operator.v1.DeviceLink.prototype.setUses = function(value) {
  return jspb.Message.setProto3IntField(this, 8, value);
};


/**
 * optional string status = 9;
 * @return {string}
 */
proto.g8e.operator.v1.DeviceLink.prototype.getStatus = function() {
  return /** @type {string} */ (jspb.Message.getFieldWithDefault(this, 9, ""));
};


/**
 * @param {string} value
 * @return {!proto.g8e.operator.v1.DeviceLink} returns this
 */
proto.g8e.operator.v1.DeviceLink.prototype.setStatus = function(value) {
  return jspb.Message.setProto3StringField(this, 9, value);
};


/**
 * optional int64 created_at_unix_ms = 10;
 * @return {number}
 */
proto.g8e.operator.v1.DeviceLink.prototype.getCreatedAtUnixMs = function() {
  return /** @type {number} */ (jspb.Message.getFieldWithDefault(this, 10, 0));
};


/**
 * @param {number} value
 * @return {!proto.g8e.operator.v1.DeviceLink} returns this
 */
proto.g8e.operator.v1.DeviceLink.prototype.setCreatedAtUnixMs = function(value) {
  return jspb.Message.setProto3IntField(this, 10, value);
};


/**
 * optional int64 expires_at_unix_ms = 11;
 * @return {number}
 */
proto.g8e.operator.v1.DeviceLink.prototype.getExpiresAtUnixMs = function() {
  return /** @type {number} */ (jspb.Message.getFieldWithDefault(this, 11, 0));
};


/**
 * @param {number} value
 * @return {!proto.g8e.operator.v1.DeviceLink} returns this
 */
proto.g8e.operator.v1.DeviceLink.prototype.setExpiresAtUnixMs = function(value) {
  return jspb.Message.setProto3IntField(this, 11, value);
};





if (jspb.Message.GENERATE_TO_OBJECT) {
/**
 * Creates an object representation of this proto.
 * Field names that are reserved in JavaScript and will be renamed to pb_name.
 * Optional fields that are not set will be set to undefined.
 * To access a reserved field use, foo.pb_<name>, eg, foo.pb_default.
 * For the list of reserved names please see:
 *     net/proto2/compiler/js/internal/generator.cc#kKeyword.
 * @param {boolean=} opt_includeInstance Deprecated. whether to include the
 *     JSPB instance for transitional soy proto support:
 *     http://goto/soy-param-migration
 * @return {!Object}
 */
proto.g8e.operator.v1.DeviceLinkResult.prototype.toObject = function(opt_includeInstance) {
  return proto.g8e.operator.v1.DeviceLinkResult.toObject(opt_includeInstance, this);
};


/**
 * Static version of the {@see toObject} method.
 * @param {boolean|undefined} includeInstance Deprecated. Whether to include
 *     the JSPB instance for transitional soy proto support:
 *     http://goto/soy-param-migration
 * @param {!proto.g8e.operator.v1.DeviceLinkResult} msg The msg instance to transform.
 * @return {!Object}
 * @suppress {unusedLocalVariables} f is only used for nested messages
 */
proto.g8e.operator.v1.DeviceLinkResult.toObject = function(includeInstance, msg) {
  var f, obj = {
    success: jspb.Message.getBooleanFieldWithDefault(msg, 1, false),
    link: (f = msg.getLink()) && proto.g8e.operator.v1.DeviceLink.toObject(includeInstance, f),
    operatorCommand: jspb.Message.getFieldWithDefault(msg, 3, ""),
    error: jspb.Message.getFieldWithDefault(msg, 4, "")
  };

  if (includeInstance) {
    obj.$jspbMessageInstance = msg;
  }
  return obj;
};
}


/**
 * Deserializes binary data (in protobuf wire format).
 * @param {jspb.ByteSource} bytes The bytes to deserialize.
 * @return {!proto.g8e.operator.v1.DeviceLinkResult}
 */
proto.g8e.operator.v1.DeviceLinkResult.deserializeBinary = function(bytes) {
  var reader = new jspb.BinaryReader(bytes);
  var msg = new proto.g8e.operator.v1.DeviceLinkResult;
  return proto.g8e.operator.v1.DeviceLinkResult.deserializeBinaryFromReader(msg, reader);
};


/**
 * Deserializes binary data (in protobuf wire format) from the
 * given reader into the given message object.
 * @param {!proto.g8e.operator.v1.DeviceLinkResult} msg The message object to deserialize into.
 * @param {!jspb.BinaryReader} reader The BinaryReader to use.
 * @return {!proto.g8e.operator.v1.DeviceLinkResult}
 */
proto.g8e.operator.v1.DeviceLinkResult.deserializeBinaryFromReader = function(msg, reader) {
  while (reader.nextField()) {
    if (reader.isEndGroup()) {
      break;
    }
    var field = reader.getFieldNumber();
    switch (field) {
    case 1:
      var value = /** @type {boolean} */ (reader.readBool());
      msg.setSuccess(value);
      break;
    case 2:
      var value = new proto.g8e.operator.v1.DeviceLink;
      reader.readMessage(value,proto.g8e.operator.v1.DeviceLink.deserializeBinaryFromReader);
      msg.setLink(value);
      break;
    case 3:
      var value = /** @type {string} */ (reader.readString());
      msg.setOperatorCommand(value);
      break;
    case 4:
      var value = /** @type {string} */ (reader.readString());
      msg.setError(value);
      break;
    default:
      reader.skipField();
      break;
    }
  }
  return msg;
};


/**
 * Serializes the message to binary data (in protobuf wire format).
 * @return {!Uint8Array}
 */
proto.g8e.operator.v1.DeviceLinkResult.prototype.serializeBinary = function() {
  var writer = new jspb.BinaryWriter();
  proto.g8e.operator.v1.DeviceLinkResult.serializeBinaryToWriter(this, writer);
  return writer.getResultBuffer();
};


/**
 * Serializes the given message to binary data (in protobuf wire
 * format), writing to the given BinaryWriter.
 * @param {!proto.g8e.operator.v1.DeviceLinkResult} message
 * @param {!jspb.BinaryWriter} writer
 * @suppress {unusedLocalVariables} f is only used for nested messages
 */
proto.g8e.operator.v1.DeviceLinkResult.serializeBinaryToWriter = function(message, writer) {
  var f = undefined;
  f = message.getSuccess();
  if (f) {
    writer.writeBool(
      1,
      f
    );
  }
  f = message.getLink();
  if (f != null) {
    writer.writeMessage(
      2,
      f,
      proto.g8e.operator.v1.DeviceLink.serializeBinaryToWriter
    );
  }
  f = message.getOperatorCommand();
  if (f.length > 0) {
    writer.writeString(
      3,
      f
    );
  }
  f = message.getError();
  if (f.length > 0) {
    writer.writeString(
      4,
      f
    );
  }
};


/**
 * optional bool success = 1;
 * @return {boolean}
 */
proto.g8e.operator.v1.DeviceLinkResult.prototype.getSuccess = function() {
  return /** @type {boolean} */ (jspb.Message.getBooleanFieldWithDefault(this, 1, false));
};


/**
 * @param {boolean} value
 * @return {!proto.g8e.operator.v1.DeviceLinkResult} returns this
 */
proto.g8e.operator.v1.DeviceLinkResult.prototype.setSuccess = function(value) {
  return jspb.Message.setProto3BooleanField(this, 1, value);
};


/**
 * optional DeviceLink link = 2;
 * @return {?proto.g8e.operator.v1.DeviceLink}
 */
proto.g8e.operator.v1.DeviceLinkResult.prototype.getLink = function() {
  return /** @type{?proto.g8e.operator.v1.DeviceLink} */ (
    jspb.Message.getWrapperField(this, proto.g8e.operator.v1.DeviceLink, 2));
};


/**
 * @param {?proto.g8e.operator.v1.DeviceLink|undefined} value
 * @return {!proto.g8e.operator.v1.DeviceLinkResult} returns this
*/
proto.g8e.operator.v1.DeviceLinkResult.prototype.setLink = function(value) {
  return jspb.Message.setWrapperField(this, 2, value);
};


/**
 * Clears the message field making it undefined.
 * @return {!proto.g8e.operator.v1.DeviceLinkResult} returns this
 */
proto.g8e.operator.v1.DeviceLinkResult.prototype.clearLink = function() {
  return this.setLink(undefined);
};


/**
 * Returns whether this field is set.
 * @return {boolean}
 */
proto.g8e.operator.v1.DeviceLinkResult.prototype.hasLink = function() {
  return jspb.Message.getField(this, 2) != null;
};


/**
 * optional string operator_command = 3;
 * @return {string}
 */
proto.g8e.operator.v1.DeviceLinkResult.prototype.getOperatorCommand = function() {
  return /** @type {string} */ (jspb.Message.getFieldWithDefault(this, 3, ""));
};


/**
 * @param {string} value
 * @return {!proto.g8e.operator.v1.DeviceLinkResult} returns this
 */
proto.g8e.operator.v1.DeviceLinkResult.prototype.setOperatorCommand = function(value) {
  return jspb.Message.setProto3StringField(this, 3, value);
};


/**
 * optional string error = 4;
 * @return {string}
 */
proto.g8e.operator.v1.DeviceLinkResult.prototype.getError = function() {
  return /** @type {string} */ (jspb.Message.getFieldWithDefault(this, 4, ""));
};


/**
 * @param {string} value
 * @return {!proto.g8e.operator.v1.DeviceLinkResult} returns this
 */
proto.g8e.operator.v1.DeviceLinkResult.prototype.setError = function(value) {
  return jspb.Message.setProto3StringField(this, 4, value);
};





if (jspb.Message.GENERATE_TO_OBJECT) {
/**
 * Creates an object representation of this proto.
 * Field names that are reserved in JavaScript and will be renamed to pb_name.
 * Optional fields that are not set will be set to undefined.
 * To access a reserved field use, foo.pb_<name>, eg, foo.pb_default.
 * For the list of reserved names please see:
 *     net/proto2/compiler/js/internal/generator.cc#kKeyword.
 * @param {boolean=} opt_includeInstance Deprecated. whether to include the
 *     JSPB instance for transitional soy proto support:
 *     http://goto/soy-param-migration
 * @return {!Object}
 */
proto.g8e.operator.v1.ListDeviceLinksRequested.prototype.toObject = function(opt_includeInstance) {
  return proto.g8e.operator.v1.ListDeviceLinksRequested.toObject(opt_includeInstance, this);
};


/**
 * Static version of the {@see toObject} method.
 * @param {boolean|undefined} includeInstance Deprecated. Whether to include
 *     the JSPB instance for transitional soy proto support:
 *     http://goto/soy-param-migration
 * @param {!proto.g8e.operator.v1.ListDeviceLinksRequested} msg The msg instance to transform.
 * @return {!Object}
 * @suppress {unusedLocalVariables} f is only used for nested messages
 */
proto.g8e.operator.v1.ListDeviceLinksRequested.toObject = function(includeInstance, msg) {
  var f, obj = {
    userId: jspb.Message.getFieldWithDefault(msg, 1, "")
  };

  if (includeInstance) {
    obj.$jspbMessageInstance = msg;
  }
  return obj;
};
}


/**
 * Deserializes binary data (in protobuf wire format).
 * @param {jspb.ByteSource} bytes The bytes to deserialize.
 * @return {!proto.g8e.operator.v1.ListDeviceLinksRequested}
 */
proto.g8e.operator.v1.ListDeviceLinksRequested.deserializeBinary = function(bytes) {
  var reader = new jspb.BinaryReader(bytes);
  var msg = new proto.g8e.operator.v1.ListDeviceLinksRequested;
  return proto.g8e.operator.v1.ListDeviceLinksRequested.deserializeBinaryFromReader(msg, reader);
};


/**
 * Deserializes binary data (in protobuf wire format) from the
 * given reader into the given message object.
 * @param {!proto.g8e.operator.v1.ListDeviceLinksRequested} msg The message object to deserialize into.
 * @param {!jspb.BinaryReader} reader The BinaryReader to use.
 * @return {!proto.g8e.operator.v1.ListDeviceLinksRequested}
 */
proto.g8e.operator.v1.ListDeviceLinksRequested.deserializeBinaryFromReader = function(msg, reader) {
  while (reader.nextField()) {
    if (reader.isEndGroup()) {
      break;
    }
    var field = reader.getFieldNumber();
    switch (field) {
    case 1:
      var value = /** @type {string} */ (reader.readString());
      msg.setUserId(value);
      break;
    default:
      reader.skipField();
      break;
    }
  }
  return msg;
};


/**
 * Serializes the message to binary data (in protobuf wire format).
 * @return {!Uint8Array}
 */
proto.g8e.operator.v1.ListDeviceLinksRequested.prototype.serializeBinary = function() {
  var writer = new jspb.BinaryWriter();
  proto.g8e.operator.v1.ListDeviceLinksRequested.serializeBinaryToWriter(this, writer);
  return writer.getResultBuffer();
};


/**
 * Serializes the given message to binary data (in protobuf wire
 * format), writing to the given BinaryWriter.
 * @param {!proto.g8e.operator.v1.ListDeviceLinksRequested} message
 * @param {!jspb.BinaryWriter} writer
 * @suppress {unusedLocalVariables} f is only used for nested messages
 */
proto.g8e.operator.v1.ListDeviceLinksRequested.serializeBinaryToWriter = function(message, writer) {
  var f = undefined;
  f = message.getUserId();
  if (f.length > 0) {
    writer.writeString(
      1,
      f
    );
  }
};


/**
 * optional string user_id = 1;
 * @return {string}
 */
proto.g8e.operator.v1.ListDeviceLinksRequested.prototype.getUserId = function() {
  return /** @type {string} */ (jspb.Message.getFieldWithDefault(this, 1, ""));
};


/**
 * @param {string} value
 * @return {!proto.g8e.operator.v1.ListDeviceLinksRequested} returns this
 */
proto.g8e.operator.v1.ListDeviceLinksRequested.prototype.setUserId = function(value) {
  return jspb.Message.setProto3StringField(this, 1, value);
};



/**
 * List of repeated fields within this message type.
 * @private {!Array<number>}
 * @const
 */
proto.g8e.operator.v1.ListDeviceLinksResult.repeatedFields_ = [2];



if (jspb.Message.GENERATE_TO_OBJECT) {
/**
 * Creates an object representation of this proto.
 * Field names that are reserved in JavaScript and will be renamed to pb_name.
 * Optional fields that are not set will be set to undefined.
 * To access a reserved field use, foo.pb_<name>, eg, foo.pb_default.
 * For the list of reserved names please see:
 *     net/proto2/compiler/js/internal/generator.cc#kKeyword.
 * @param {boolean=} opt_includeInstance Deprecated. whether to include the
 *     JSPB instance for transitional soy proto support:
 *     http://goto/soy-param-migration
 * @return {!Object}
 */
proto.g8e.operator.v1.ListDeviceLinksResult.prototype.toObject = function(opt_includeInstance) {
  return proto.g8e.operator.v1.ListDeviceLinksResult.toObject(opt_includeInstance, this);
};


/**
 * Static version of the {@see toObject} method.
 * @param {boolean|undefined} includeInstance Deprecated. Whether to include
 *     the JSPB instance for transitional soy proto support:
 *     http://goto/soy-param-migration
 * @param {!proto.g8e.operator.v1.ListDeviceLinksResult} msg The msg instance to transform.
 * @return {!Object}
 * @suppress {unusedLocalVariables} f is only used for nested messages
 */
proto.g8e.operator.v1.ListDeviceLinksResult.toObject = function(includeInstance, msg) {
  var f, obj = {
    success: jspb.Message.getBooleanFieldWithDefault(msg, 1, false),
    linksList: jspb.Message.toObjectList(msg.getLinksList(),
    proto.g8e.operator.v1.DeviceLink.toObject, includeInstance),
    error: jspb.Message.getFieldWithDefault(msg, 3, "")
  };

  if (includeInstance) {
    obj.$jspbMessageInstance = msg;
  }
  return obj;
};
}


/**
 * Deserializes binary data (in protobuf wire format).
 * @param {jspb.ByteSource} bytes The bytes to deserialize.
 * @return {!proto.g8e.operator.v1.ListDeviceLinksResult}
 */
proto.g8e.operator.v1.ListDeviceLinksResult.deserializeBinary = function(bytes) {
  var reader = new jspb.BinaryReader(bytes);
  var msg = new proto.g8e.operator.v1.ListDeviceLinksResult;
  return proto.g8e.operator.v1.ListDeviceLinksResult.deserializeBinaryFromReader(msg, reader);
};


/**
 * Deserializes binary data (in protobuf wire format) from the
 * given reader into the given message object.
 * @param {!proto.g8e.operator.v1.ListDeviceLinksResult} msg The message object to deserialize into.
 * @param {!jspb.BinaryReader} reader The BinaryReader to use.
 * @return {!proto.g8e.operator.v1.ListDeviceLinksResult}
 */
proto.g8e.operator.v1.ListDeviceLinksResult.deserializeBinaryFromReader = function(msg, reader) {
  while (reader.nextField()) {
    if (reader.isEndGroup()) {
      break;
    }
    var field = reader.getFieldNumber();
    switch (field) {
    case 1:
      var value = /** @type {boolean} */ (reader.readBool());
      msg.setSuccess(value);
      break;
    case 2:
      var value = new proto.g8e.operator.v1.DeviceLink;
      reader.readMessage(value,proto.g8e.operator.v1.DeviceLink.deserializeBinaryFromReader);
      msg.addLinks(value);
      break;
    case 3:
      var value = /** @type {string} */ (reader.readString());
      msg.setError(value);
      break;
    default:
      reader.skipField();
      break;
    }
  }
  return msg;
};


/**
 * Serializes the message to binary data (in protobuf wire format).
 * @return {!Uint8Array}
 */
proto.g8e.operator.v1.ListDeviceLinksResult.prototype.serializeBinary = function() {
  var writer = new jspb.BinaryWriter();
  proto.g8e.operator.v1.ListDeviceLinksResult.serializeBinaryToWriter(this, writer);
  return writer.getResultBuffer();
};


/**
 * Serializes the given message to binary data (in protobuf wire
 * format), writing to the given BinaryWriter.
 * @param {!proto.g8e.operator.v1.ListDeviceLinksResult} message
 * @param {!jspb.BinaryWriter} writer
 * @suppress {unusedLocalVariables} f is only used for nested messages
 */
proto.g8e.operator.v1.ListDeviceLinksResult.serializeBinaryToWriter = function(message, writer) {
  var f = undefined;
  f = message.getSuccess();
  if (f) {
    writer.writeBool(
      1,
      f
    );
  }
  f = message.getLinksList();
  if (f.length > 0) {
    writer.writeRepeatedMessage(
      2,
      f,
      proto.g8e.operator.v1.DeviceLink.serializeBinaryToWriter
    );
  }
  f = message.getError();
  if (f.length > 0) {
    writer.writeString(
      3,
      f
    );
  }
};


/**
 * optional bool success = 1;
 * @return {boolean}
 */
proto.g8e.operator.v1.ListDeviceLinksResult.prototype.getSuccess = function() {
  return /** @type {boolean} */ (jspb.Message.getBooleanFieldWithDefault(this, 1, false));
};


/**
 * @param {boolean} value
 * @return {!proto.g8e.operator.v1.ListDeviceLinksResult} returns this
 */
proto.g8e.operator.v1.ListDeviceLinksResult.prototype.setSuccess = function(value) {
  return jspb.Message.setProto3BooleanField(this, 1, value);
};


/**
 * repeated DeviceLink links = 2;
 * @return {!Array<!proto.g8e.operator.v1.DeviceLink>}
 */
proto.g8e.operator.v1.ListDeviceLinksResult.prototype.getLinksList = function() {
  return /** @type{!Array<!proto.g8e.operator.v1.DeviceLink>} */ (
    jspb.Message.getRepeatedWrapperField(this, proto.g8e.operator.v1.DeviceLink, 2));
};


/**
 * @param {!Array<!proto.g8e.operator.v1.DeviceLink>} value
 * @return {!proto.g8e.operator.v1.ListDeviceLinksResult} returns this
*/
proto.g8e.operator.v1.ListDeviceLinksResult.prototype.setLinksList = function(value) {
  return jspb.Message.setRepeatedWrapperField(this, 2, value);
};


/**
 * @param {!proto.g8e.operator.v1.DeviceLink=} opt_value
 * @param {number=} opt_index
 * @return {!proto.g8e.operator.v1.DeviceLink}
 */
proto.g8e.operator.v1.ListDeviceLinksResult.prototype.addLinks = function(opt_value, opt_index) {
  return jspb.Message.addToRepeatedWrapperField(this, 2, opt_value, proto.g8e.operator.v1.DeviceLink, opt_index);
};


/**
 * Clears the list making it empty but non-null.
 * @return {!proto.g8e.operator.v1.ListDeviceLinksResult} returns this
 */
proto.g8e.operator.v1.ListDeviceLinksResult.prototype.clearLinksList = function() {
  return this.setLinksList([]);
};


/**
 * optional string error = 3;
 * @return {string}
 */
proto.g8e.operator.v1.ListDeviceLinksResult.prototype.getError = function() {
  return /** @type {string} */ (jspb.Message.getFieldWithDefault(this, 3, ""));
};


/**
 * @param {string} value
 * @return {!proto.g8e.operator.v1.ListDeviceLinksResult} returns this
 */
proto.g8e.operator.v1.ListDeviceLinksResult.prototype.setError = function(value) {
  return jspb.Message.setProto3StringField(this, 3, value);
};





if (jspb.Message.GENERATE_TO_OBJECT) {
/**
 * Creates an object representation of this proto.
 * Field names that are reserved in JavaScript and will be renamed to pb_name.
 * Optional fields that are not set will be set to undefined.
 * To access a reserved field use, foo.pb_<name>, eg, foo.pb_default.
 * For the list of reserved names please see:
 *     net/proto2/compiler/js/internal/generator.cc#kKeyword.
 * @param {boolean=} opt_includeInstance Deprecated. whether to include the
 *     JSPB instance for transitional soy proto support:
 *     http://goto/soy-param-migration
 * @return {!Object}
 */
proto.g8e.operator.v1.DeleteDeviceLinkRequested.prototype.toObject = function(opt_includeInstance) {
  return proto.g8e.operator.v1.DeleteDeviceLinkRequested.toObject(opt_includeInstance, this);
};


/**
 * Static version of the {@see toObject} method.
 * @param {boolean|undefined} includeInstance Deprecated. Whether to include
 *     the JSPB instance for transitional soy proto support:
 *     http://goto/soy-param-migration
 * @param {!proto.g8e.operator.v1.DeleteDeviceLinkRequested} msg The msg instance to transform.
 * @return {!Object}
 * @suppress {unusedLocalVariables} f is only used for nested messages
 */
proto.g8e.operator.v1.DeleteDeviceLinkRequested.toObject = function(includeInstance, msg) {
  var f, obj = {
    token: jspb.Message.getFieldWithDefault(msg, 1, ""),
    userId: jspb.Message.getFieldWithDefault(msg, 2, "")
  };

  if (includeInstance) {
    obj.$jspbMessageInstance = msg;
  }
  return obj;
};
}


/**
 * Deserializes binary data (in protobuf wire format).
 * @param {jspb.ByteSource} bytes The bytes to deserialize.
 * @return {!proto.g8e.operator.v1.DeleteDeviceLinkRequested}
 */
proto.g8e.operator.v1.DeleteDeviceLinkRequested.deserializeBinary = function(bytes) {
  var reader = new jspb.BinaryReader(bytes);
  var msg = new proto.g8e.operator.v1.DeleteDeviceLinkRequested;
  return proto.g8e.operator.v1.DeleteDeviceLinkRequested.deserializeBinaryFromReader(msg, reader);
};


/**
 * Deserializes binary data (in protobuf wire format) from the
 * given reader into the given message object.
 * @param {!proto.g8e.operator.v1.DeleteDeviceLinkRequested} msg The message object to deserialize into.
 * @param {!jspb.BinaryReader} reader The BinaryReader to use.
 * @return {!proto.g8e.operator.v1.DeleteDeviceLinkRequested}
 */
proto.g8e.operator.v1.DeleteDeviceLinkRequested.deserializeBinaryFromReader = function(msg, reader) {
  while (reader.nextField()) {
    if (reader.isEndGroup()) {
      break;
    }
    var field = reader.getFieldNumber();
    switch (field) {
    case 1:
      var value = /** @type {string} */ (reader.readString());
      msg.setToken(value);
      break;
    case 2:
      var value = /** @type {string} */ (reader.readString());
      msg.setUserId(value);
      break;
    default:
      reader.skipField();
      break;
    }
  }
  return msg;
};


/**
 * Serializes the message to binary data (in protobuf wire format).
 * @return {!Uint8Array}
 */
proto.g8e.operator.v1.DeleteDeviceLinkRequested.prototype.serializeBinary = function() {
  var writer = new jspb.BinaryWriter();
  proto.g8e.operator.v1.DeleteDeviceLinkRequested.serializeBinaryToWriter(this, writer);
  return writer.getResultBuffer();
};


/**
 * Serializes the given message to binary data (in protobuf wire
 * format), writing to the given BinaryWriter.
 * @param {!proto.g8e.operator.v1.DeleteDeviceLinkRequested} message
 * @param {!jspb.BinaryWriter} writer
 * @suppress {unusedLocalVariables} f is only used for nested messages
 */
proto.g8e.operator.v1.DeleteDeviceLinkRequested.serializeBinaryToWriter = function(message, writer) {
  var f = undefined;
  f = message.getToken();
  if (f.length > 0) {
    writer.writeString(
      1,
      f
    );
  }
  f = message.getUserId();
  if (f.length > 0) {
    writer.writeString(
      2,
      f
    );
  }
};


/**
 * optional string token = 1;
 * @return {string}
 */
proto.g8e.operator.v1.DeleteDeviceLinkRequested.prototype.getToken = function() {
  return /** @type {string} */ (jspb.Message.getFieldWithDefault(this, 1, ""));
};


/**
 * @param {string} value
 * @return {!proto.g8e.operator.v1.DeleteDeviceLinkRequested} returns this
 */
proto.g8e.operator.v1.DeleteDeviceLinkRequested.prototype.setToken = function(value) {
  return jspb.Message.setProto3StringField(this, 1, value);
};


/**
 * optional string user_id = 2;
 * @return {string}
 */
proto.g8e.operator.v1.DeleteDeviceLinkRequested.prototype.getUserId = function() {
  return /** @type {string} */ (jspb.Message.getFieldWithDefault(this, 2, ""));
};


/**
 * @param {string} value
 * @return {!proto.g8e.operator.v1.DeleteDeviceLinkRequested} returns this
 */
proto.g8e.operator.v1.DeleteDeviceLinkRequested.prototype.setUserId = function(value) {
  return jspb.Message.setProto3StringField(this, 2, value);
};





if (jspb.Message.GENERATE_TO_OBJECT) {
/**
 * Creates an object representation of this proto.
 * Field names that are reserved in JavaScript and will be renamed to pb_name.
 * Optional fields that are not set will be set to undefined.
 * To access a reserved field use, foo.pb_<name>, eg, foo.pb_default.
 * For the list of reserved names please see:
 *     net/proto2/compiler/js/internal/generator.cc#kKeyword.
 * @param {boolean=} opt_includeInstance Deprecated. whether to include the
 *     JSPB instance for transitional soy proto support:
 *     http://goto/soy-param-migration
 * @return {!Object}
 */
proto.g8e.operator.v1.TerminateOperatorRequested.prototype.toObject = function(opt_includeInstance) {
  return proto.g8e.operator.v1.TerminateOperatorRequested.toObject(opt_includeInstance, this);
};


/**
 * Static version of the {@see toObject} method.
 * @param {boolean|undefined} includeInstance Deprecated. Whether to include
 *     the JSPB instance for transitional soy proto support:
 *     http://goto/soy-param-migration
 * @param {!proto.g8e.operator.v1.TerminateOperatorRequested} msg The msg instance to transform.
 * @return {!Object}
 * @suppress {unusedLocalVariables} f is only used for nested messages
 */
proto.g8e.operator.v1.TerminateOperatorRequested.toObject = function(includeInstance, msg) {
  var f, obj = {
    operatorId: jspb.Message.getFieldWithDefault(msg, 1, ""),
    userId: jspb.Message.getFieldWithDefault(msg, 2, ""),
    reason: jspb.Message.getFieldWithDefault(msg, 3, "")
  };

  if (includeInstance) {
    obj.$jspbMessageInstance = msg;
  }
  return obj;
};
}


/**
 * Deserializes binary data (in protobuf wire format).
 * @param {jspb.ByteSource} bytes The bytes to deserialize.
 * @return {!proto.g8e.operator.v1.TerminateOperatorRequested}
 */
proto.g8e.operator.v1.TerminateOperatorRequested.deserializeBinary = function(bytes) {
  var reader = new jspb.BinaryReader(bytes);
  var msg = new proto.g8e.operator.v1.TerminateOperatorRequested;
  return proto.g8e.operator.v1.TerminateOperatorRequested.deserializeBinaryFromReader(msg, reader);
};


/**
 * Deserializes binary data (in protobuf wire format) from the
 * given reader into the given message object.
 * @param {!proto.g8e.operator.v1.TerminateOperatorRequested} msg The message object to deserialize into.
 * @param {!jspb.BinaryReader} reader The BinaryReader to use.
 * @return {!proto.g8e.operator.v1.TerminateOperatorRequested}
 */
proto.g8e.operator.v1.TerminateOperatorRequested.deserializeBinaryFromReader = function(msg, reader) {
  while (reader.nextField()) {
    if (reader.isEndGroup()) {
      break;
    }
    var field = reader.getFieldNumber();
    switch (field) {
    case 1:
      var value = /** @type {string} */ (reader.readString());
      msg.setOperatorId(value);
      break;
    case 2:
      var value = /** @type {string} */ (reader.readString());
      msg.setUserId(value);
      break;
    case 3:
      var value = /** @type {string} */ (reader.readString());
      msg.setReason(value);
      break;
    default:
      reader.skipField();
      break;
    }
  }
  return msg;
};


/**
 * Serializes the message to binary data (in protobuf wire format).
 * @return {!Uint8Array}
 */
proto.g8e.operator.v1.TerminateOperatorRequested.prototype.serializeBinary = function() {
  var writer = new jspb.BinaryWriter();
  proto.g8e.operator.v1.TerminateOperatorRequested.serializeBinaryToWriter(this, writer);
  return writer.getResultBuffer();
};


/**
 * Serializes the given message to binary data (in protobuf wire
 * format), writing to the given BinaryWriter.
 * @param {!proto.g8e.operator.v1.TerminateOperatorRequested} message
 * @param {!jspb.BinaryWriter} writer
 * @suppress {unusedLocalVariables} f is only used for nested messages
 */
proto.g8e.operator.v1.TerminateOperatorRequested.serializeBinaryToWriter = function(message, writer) {
  var f = undefined;
  f = message.getOperatorId();
  if (f.length > 0) {
    writer.writeString(
      1,
      f
    );
  }
  f = message.getUserId();
  if (f.length > 0) {
    writer.writeString(
      2,
      f
    );
  }
  f = message.getReason();
  if (f.length > 0) {
    writer.writeString(
      3,
      f
    );
  }
};


/**
 * optional string operator_id = 1;
 * @return {string}
 */
proto.g8e.operator.v1.TerminateOperatorRequested.prototype.getOperatorId = function() {
  return /** @type {string} */ (jspb.Message.getFieldWithDefault(this, 1, ""));
};


/**
 * @param {string} value
 * @return {!proto.g8e.operator.v1.TerminateOperatorRequested} returns this
 */
proto.g8e.operator.v1.TerminateOperatorRequested.prototype.setOperatorId = function(value) {
  return jspb.Message.setProto3StringField(this, 1, value);
};


/**
 * optional string user_id = 2;
 * @return {string}
 */
proto.g8e.operator.v1.TerminateOperatorRequested.prototype.getUserId = function() {
  return /** @type {string} */ (jspb.Message.getFieldWithDefault(this, 2, ""));
};


/**
 * @param {string} value
 * @return {!proto.g8e.operator.v1.TerminateOperatorRequested} returns this
 */
proto.g8e.operator.v1.TerminateOperatorRequested.prototype.setUserId = function(value) {
  return jspb.Message.setProto3StringField(this, 2, value);
};


/**
 * optional string reason = 3;
 * @return {string}
 */
proto.g8e.operator.v1.TerminateOperatorRequested.prototype.getReason = function() {
  return /** @type {string} */ (jspb.Message.getFieldWithDefault(this, 3, ""));
};


/**
 * @param {string} value
 * @return {!proto.g8e.operator.v1.TerminateOperatorRequested} returns this
 */
proto.g8e.operator.v1.TerminateOperatorRequested.prototype.setReason = function(value) {
  return jspb.Message.setProto3StringField(this, 3, value);
};





if (jspb.Message.GENERATE_TO_OBJECT) {
/**
 * Creates an object representation of this proto.
 * Field names that are reserved in JavaScript and will be renamed to pb_name.
 * Optional fields that are not set will be set to undefined.
 * To access a reserved field use, foo.pb_<name>, eg, foo.pb_default.
 * For the list of reserved names please see:
 *     net/proto2/compiler/js/internal/generator.cc#kKeyword.
 * @param {boolean=} opt_includeInstance Deprecated. whether to include the
 *     JSPB instance for transitional soy proto support:
 *     http://goto/soy-param-migration
 * @return {!Object}
 */
proto.g8e.operator.v1.TerminateOperatorResult.prototype.toObject = function(opt_includeInstance) {
  return proto.g8e.operator.v1.TerminateOperatorResult.toObject(opt_includeInstance, this);
};


/**
 * Static version of the {@see toObject} method.
 * @param {boolean|undefined} includeInstance Deprecated. Whether to include
 *     the JSPB instance for transitional soy proto support:
 *     http://goto/soy-param-migration
 * @param {!proto.g8e.operator.v1.TerminateOperatorResult} msg The msg instance to transform.
 * @return {!Object}
 * @suppress {unusedLocalVariables} f is only used for nested messages
 */
proto.g8e.operator.v1.TerminateOperatorResult.toObject = function(includeInstance, msg) {
  var f, obj = {
    success: jspb.Message.getBooleanFieldWithDefault(msg, 1, false),
    message: jspb.Message.getFieldWithDefault(msg, 2, ""),
    error: jspb.Message.getFieldWithDefault(msg, 3, "")
  };

  if (includeInstance) {
    obj.$jspbMessageInstance = msg;
  }
  return obj;
};
}


/**
 * Deserializes binary data (in protobuf wire format).
 * @param {jspb.ByteSource} bytes The bytes to deserialize.
 * @return {!proto.g8e.operator.v1.TerminateOperatorResult}
 */
proto.g8e.operator.v1.TerminateOperatorResult.deserializeBinary = function(bytes) {
  var reader = new jspb.BinaryReader(bytes);
  var msg = new proto.g8e.operator.v1.TerminateOperatorResult;
  return proto.g8e.operator.v1.TerminateOperatorResult.deserializeBinaryFromReader(msg, reader);
};


/**
 * Deserializes binary data (in protobuf wire format) from the
 * given reader into the given message object.
 * @param {!proto.g8e.operator.v1.TerminateOperatorResult} msg The message object to deserialize into.
 * @param {!jspb.BinaryReader} reader The BinaryReader to use.
 * @return {!proto.g8e.operator.v1.TerminateOperatorResult}
 */
proto.g8e.operator.v1.TerminateOperatorResult.deserializeBinaryFromReader = function(msg, reader) {
  while (reader.nextField()) {
    if (reader.isEndGroup()) {
      break;
    }
    var field = reader.getFieldNumber();
    switch (field) {
    case 1:
      var value = /** @type {boolean} */ (reader.readBool());
      msg.setSuccess(value);
      break;
    case 2:
      var value = /** @type {string} */ (reader.readString());
      msg.setMessage(value);
      break;
    case 3:
      var value = /** @type {string} */ (reader.readString());
      msg.setError(value);
      break;
    default:
      reader.skipField();
      break;
    }
  }
  return msg;
};


/**
 * Serializes the message to binary data (in protobuf wire format).
 * @return {!Uint8Array}
 */
proto.g8e.operator.v1.TerminateOperatorResult.prototype.serializeBinary = function() {
  var writer = new jspb.BinaryWriter();
  proto.g8e.operator.v1.TerminateOperatorResult.serializeBinaryToWriter(this, writer);
  return writer.getResultBuffer();
};


/**
 * Serializes the given message to binary data (in protobuf wire
 * format), writing to the given BinaryWriter.
 * @param {!proto.g8e.operator.v1.TerminateOperatorResult} message
 * @param {!jspb.BinaryWriter} writer
 * @suppress {unusedLocalVariables} f is only used for nested messages
 */
proto.g8e.operator.v1.TerminateOperatorResult.serializeBinaryToWriter = function(message, writer) {
  var f = undefined;
  f = message.getSuccess();
  if (f) {
    writer.writeBool(
      1,
      f
    );
  }
  f = message.getMessage();
  if (f.length > 0) {
    writer.writeString(
      2,
      f
    );
  }
  f = message.getError();
  if (f.length > 0) {
    writer.writeString(
      3,
      f
    );
  }
};


/**
 * optional bool success = 1;
 * @return {boolean}
 */
proto.g8e.operator.v1.TerminateOperatorResult.prototype.getSuccess = function() {
  return /** @type {boolean} */ (jspb.Message.getBooleanFieldWithDefault(this, 1, false));
};


/**
 * @param {boolean} value
 * @return {!proto.g8e.operator.v1.TerminateOperatorResult} returns this
 */
proto.g8e.operator.v1.TerminateOperatorResult.prototype.setSuccess = function(value) {
  return jspb.Message.setProto3BooleanField(this, 1, value);
};


/**
 * optional string message = 2;
 * @return {string}
 */
proto.g8e.operator.v1.TerminateOperatorResult.prototype.getMessage = function() {
  return /** @type {string} */ (jspb.Message.getFieldWithDefault(this, 2, ""));
};


/**
 * @param {string} value
 * @return {!proto.g8e.operator.v1.TerminateOperatorResult} returns this
 */
proto.g8e.operator.v1.TerminateOperatorResult.prototype.setMessage = function(value) {
  return jspb.Message.setProto3StringField(this, 2, value);
};


/**
 * optional string error = 3;
 * @return {string}
 */
proto.g8e.operator.v1.TerminateOperatorResult.prototype.getError = function() {
  return /** @type {string} */ (jspb.Message.getFieldWithDefault(this, 3, ""));
};


/**
 * @param {string} value
 * @return {!proto.g8e.operator.v1.TerminateOperatorResult} returns this
 */
proto.g8e.operator.v1.TerminateOperatorResult.prototype.setError = function(value) {
  return jspb.Message.setProto3StringField(this, 3, value);
};





if (jspb.Message.GENERATE_TO_OBJECT) {
/**
 * Creates an object representation of this proto.
 * Field names that are reserved in JavaScript and will be renamed to pb_name.
 * Optional fields that are not set will be set to undefined.
 * To access a reserved field use, foo.pb_<name>, eg, foo.pb_default.
 * For the list of reserved names please see:
 *     net/proto2/compiler/js/internal/generator.cc#kKeyword.
 * @param {boolean=} opt_includeInstance Deprecated. whether to include the
 *     JSPB instance for transitional soy proto support:
 *     http://goto/soy-param-migration
 * @return {!Object}
 */
proto.g8e.operator.v1.RotateAPIKeyRequested.prototype.toObject = function(opt_includeInstance) {
  return proto.g8e.operator.v1.RotateAPIKeyRequested.toObject(opt_includeInstance, this);
};


/**
 * Static version of the {@see toObject} method.
 * @param {boolean|undefined} includeInstance Deprecated. Whether to include
 *     the JSPB instance for transitional soy proto support:
 *     http://goto/soy-param-migration
 * @param {!proto.g8e.operator.v1.RotateAPIKeyRequested} msg The msg instance to transform.
 * @return {!Object}
 * @suppress {unusedLocalVariables} f is only used for nested messages
 */
proto.g8e.operator.v1.RotateAPIKeyRequested.toObject = function(includeInstance, msg) {
  var f, obj = {
    operatorId: jspb.Message.getFieldWithDefault(msg, 1, ""),
    userId: jspb.Message.getFieldWithDefault(msg, 2, "")
  };

  if (includeInstance) {
    obj.$jspbMessageInstance = msg;
  }
  return obj;
};
}


/**
 * Deserializes binary data (in protobuf wire format).
 * @param {jspb.ByteSource} bytes The bytes to deserialize.
 * @return {!proto.g8e.operator.v1.RotateAPIKeyRequested}
 */
proto.g8e.operator.v1.RotateAPIKeyRequested.deserializeBinary = function(bytes) {
  var reader = new jspb.BinaryReader(bytes);
  var msg = new proto.g8e.operator.v1.RotateAPIKeyRequested;
  return proto.g8e.operator.v1.RotateAPIKeyRequested.deserializeBinaryFromReader(msg, reader);
};


/**
 * Deserializes binary data (in protobuf wire format) from the
 * given reader into the given message object.
 * @param {!proto.g8e.operator.v1.RotateAPIKeyRequested} msg The message object to deserialize into.
 * @param {!jspb.BinaryReader} reader The BinaryReader to use.
 * @return {!proto.g8e.operator.v1.RotateAPIKeyRequested}
 */
proto.g8e.operator.v1.RotateAPIKeyRequested.deserializeBinaryFromReader = function(msg, reader) {
  while (reader.nextField()) {
    if (reader.isEndGroup()) {
      break;
    }
    var field = reader.getFieldNumber();
    switch (field) {
    case 1:
      var value = /** @type {string} */ (reader.readString());
      msg.setOperatorId(value);
      break;
    case 2:
      var value = /** @type {string} */ (reader.readString());
      msg.setUserId(value);
      break;
    default:
      reader.skipField();
      break;
    }
  }
  return msg;
};


/**
 * Serializes the message to binary data (in protobuf wire format).
 * @return {!Uint8Array}
 */
proto.g8e.operator.v1.RotateAPIKeyRequested.prototype.serializeBinary = function() {
  var writer = new jspb.BinaryWriter();
  proto.g8e.operator.v1.RotateAPIKeyRequested.serializeBinaryToWriter(this, writer);
  return writer.getResultBuffer();
};


/**
 * Serializes the given message to binary data (in protobuf wire
 * format), writing to the given BinaryWriter.
 * @param {!proto.g8e.operator.v1.RotateAPIKeyRequested} message
 * @param {!jspb.BinaryWriter} writer
 * @suppress {unusedLocalVariables} f is only used for nested messages
 */
proto.g8e.operator.v1.RotateAPIKeyRequested.serializeBinaryToWriter = function(message, writer) {
  var f = undefined;
  f = message.getOperatorId();
  if (f.length > 0) {
    writer.writeString(
      1,
      f
    );
  }
  f = message.getUserId();
  if (f.length > 0) {
    writer.writeString(
      2,
      f
    );
  }
};


/**
 * optional string operator_id = 1;
 * @return {string}
 */
proto.g8e.operator.v1.RotateAPIKeyRequested.prototype.getOperatorId = function() {
  return /** @type {string} */ (jspb.Message.getFieldWithDefault(this, 1, ""));
};


/**
 * @param {string} value
 * @return {!proto.g8e.operator.v1.RotateAPIKeyRequested} returns this
 */
proto.g8e.operator.v1.RotateAPIKeyRequested.prototype.setOperatorId = function(value) {
  return jspb.Message.setProto3StringField(this, 1, value);
};


/**
 * optional string user_id = 2;
 * @return {string}
 */
proto.g8e.operator.v1.RotateAPIKeyRequested.prototype.getUserId = function() {
  return /** @type {string} */ (jspb.Message.getFieldWithDefault(this, 2, ""));
};


/**
 * @param {string} value
 * @return {!proto.g8e.operator.v1.RotateAPIKeyRequested} returns this
 */
proto.g8e.operator.v1.RotateAPIKeyRequested.prototype.setUserId = function(value) {
  return jspb.Message.setProto3StringField(this, 2, value);
};





if (jspb.Message.GENERATE_TO_OBJECT) {
/**
 * Creates an object representation of this proto.
 * Field names that are reserved in JavaScript and will be renamed to pb_name.
 * Optional fields that are not set will be set to undefined.
 * To access a reserved field use, foo.pb_<name>, eg, foo.pb_default.
 * For the list of reserved names please see:
 *     net/proto2/compiler/js/internal/generator.cc#kKeyword.
 * @param {boolean=} opt_includeInstance Deprecated. whether to include the
 *     JSPB instance for transitional soy proto support:
 *     http://goto/soy-param-migration
 * @return {!Object}
 */
proto.g8e.operator.v1.RotateAPIKeyResult.prototype.toObject = function(opt_includeInstance) {
  return proto.g8e.operator.v1.RotateAPIKeyResult.toObject(opt_includeInstance, this);
};


/**
 * Static version of the {@see toObject} method.
 * @param {boolean|undefined} includeInstance Deprecated. Whether to include
 *     the JSPB instance for transitional soy proto support:
 *     http://goto/soy-param-migration
 * @param {!proto.g8e.operator.v1.RotateAPIKeyResult} msg The msg instance to transform.
 * @return {!Object}
 * @suppress {unusedLocalVariables} f is only used for nested messages
 */
proto.g8e.operator.v1.RotateAPIKeyResult.toObject = function(includeInstance, msg) {
  var f, obj = {
    success: jspb.Message.getBooleanFieldWithDefault(msg, 1, false),
    apiKey: jspb.Message.getFieldWithDefault(msg, 2, ""),
    error: jspb.Message.getFieldWithDefault(msg, 3, "")
  };

  if (includeInstance) {
    obj.$jspbMessageInstance = msg;
  }
  return obj;
};
}


/**
 * Deserializes binary data (in protobuf wire format).
 * @param {jspb.ByteSource} bytes The bytes to deserialize.
 * @return {!proto.g8e.operator.v1.RotateAPIKeyResult}
 */
proto.g8e.operator.v1.RotateAPIKeyResult.deserializeBinary = function(bytes) {
  var reader = new jspb.BinaryReader(bytes);
  var msg = new proto.g8e.operator.v1.RotateAPIKeyResult;
  return proto.g8e.operator.v1.RotateAPIKeyResult.deserializeBinaryFromReader(msg, reader);
};


/**
 * Deserializes binary data (in protobuf wire format) from the
 * given reader into the given message object.
 * @param {!proto.g8e.operator.v1.RotateAPIKeyResult} msg The message object to deserialize into.
 * @param {!jspb.BinaryReader} reader The BinaryReader to use.
 * @return {!proto.g8e.operator.v1.RotateAPIKeyResult}
 */
proto.g8e.operator.v1.RotateAPIKeyResult.deserializeBinaryFromReader = function(msg, reader) {
  while (reader.nextField()) {
    if (reader.isEndGroup()) {
      break;
    }
    var field = reader.getFieldNumber();
    switch (field) {
    case 1:
      var value = /** @type {boolean} */ (reader.readBool());
      msg.setSuccess(value);
      break;
    case 2:
      var value = /** @type {string} */ (reader.readString());
      msg.setApiKey(value);
      break;
    case 3:
      var value = /** @type {string} */ (reader.readString());
      msg.setError(value);
      break;
    default:
      reader.skipField();
      break;
    }
  }
  return msg;
};


/**
 * Serializes the message to binary data (in protobuf wire format).
 * @return {!Uint8Array}
 */
proto.g8e.operator.v1.RotateAPIKeyResult.prototype.serializeBinary = function() {
  var writer = new jspb.BinaryWriter();
  proto.g8e.operator.v1.RotateAPIKeyResult.serializeBinaryToWriter(this, writer);
  return writer.getResultBuffer();
};


/**
 * Serializes the given message to binary data (in protobuf wire
 * format), writing to the given BinaryWriter.
 * @param {!proto.g8e.operator.v1.RotateAPIKeyResult} message
 * @param {!jspb.BinaryWriter} writer
 * @suppress {unusedLocalVariables} f is only used for nested messages
 */
proto.g8e.operator.v1.RotateAPIKeyResult.serializeBinaryToWriter = function(message, writer) {
  var f = undefined;
  f = message.getSuccess();
  if (f) {
    writer.writeBool(
      1,
      f
    );
  }
  f = message.getApiKey();
  if (f.length > 0) {
    writer.writeString(
      2,
      f
    );
  }
  f = message.getError();
  if (f.length > 0) {
    writer.writeString(
      3,
      f
    );
  }
};


/**
 * optional bool success = 1;
 * @return {boolean}
 */
proto.g8e.operator.v1.RotateAPIKeyResult.prototype.getSuccess = function() {
  return /** @type {boolean} */ (jspb.Message.getBooleanFieldWithDefault(this, 1, false));
};


/**
 * @param {boolean} value
 * @return {!proto.g8e.operator.v1.RotateAPIKeyResult} returns this
 */
proto.g8e.operator.v1.RotateAPIKeyResult.prototype.setSuccess = function(value) {
  return jspb.Message.setProto3BooleanField(this, 1, value);
};


/**
 * optional string api_key = 2;
 * @return {string}
 */
proto.g8e.operator.v1.RotateAPIKeyResult.prototype.getApiKey = function() {
  return /** @type {string} */ (jspb.Message.getFieldWithDefault(this, 2, ""));
};


/**
 * @param {string} value
 * @return {!proto.g8e.operator.v1.RotateAPIKeyResult} returns this
 */
proto.g8e.operator.v1.RotateAPIKeyResult.prototype.setApiKey = function(value) {
  return jspb.Message.setProto3StringField(this, 2, value);
};


/**
 * optional string error = 3;
 * @return {string}
 */
proto.g8e.operator.v1.RotateAPIKeyResult.prototype.getError = function() {
  return /** @type {string} */ (jspb.Message.getFieldWithDefault(this, 3, ""));
};


/**
 * @param {string} value
 * @return {!proto.g8e.operator.v1.RotateAPIKeyResult} returns this
 */
proto.g8e.operator.v1.RotateAPIKeyResult.prototype.setError = function(value) {
  return jspb.Message.setProto3StringField(this, 3, value);
};





if (jspb.Message.GENERATE_TO_OBJECT) {
/**
 * Creates an object representation of this proto.
 * Field names that are reserved in JavaScript and will be renamed to pb_name.
 * Optional fields that are not set will be set to undefined.
 * To access a reserved field use, foo.pb_<name>, eg, foo.pb_default.
 * For the list of reserved names please see:
 *     net/proto2/compiler/js/internal/generator.cc#kKeyword.
 * @param {boolean=} opt_includeInstance Deprecated. whether to include the
 *     JSPB instance for transitional soy proto support:
 *     http://goto/soy-param-migration
 * @return {!Object}
 */
proto.g8e.operator.v1.ListOperatorSlotsRequested.prototype.toObject = function(opt_includeInstance) {
  return proto.g8e.operator.v1.ListOperatorSlotsRequested.toObject(opt_includeInstance, this);
};


/**
 * Static version of the {@see toObject} method.
 * @param {boolean|undefined} includeInstance Deprecated. Whether to include
 *     the JSPB instance for transitional soy proto support:
 *     http://goto/soy-param-migration
 * @param {!proto.g8e.operator.v1.ListOperatorSlotsRequested} msg The msg instance to transform.
 * @return {!Object}
 * @suppress {unusedLocalVariables} f is only used for nested messages
 */
proto.g8e.operator.v1.ListOperatorSlotsRequested.toObject = function(includeInstance, msg) {
  var f, obj = {
    userId: jspb.Message.getFieldWithDefault(msg, 1, "")
  };

  if (includeInstance) {
    obj.$jspbMessageInstance = msg;
  }
  return obj;
};
}


/**
 * Deserializes binary data (in protobuf wire format).
 * @param {jspb.ByteSource} bytes The bytes to deserialize.
 * @return {!proto.g8e.operator.v1.ListOperatorSlotsRequested}
 */
proto.g8e.operator.v1.ListOperatorSlotsRequested.deserializeBinary = function(bytes) {
  var reader = new jspb.BinaryReader(bytes);
  var msg = new proto.g8e.operator.v1.ListOperatorSlotsRequested;
  return proto.g8e.operator.v1.ListOperatorSlotsRequested.deserializeBinaryFromReader(msg, reader);
};


/**
 * Deserializes binary data (in protobuf wire format) from the
 * given reader into the given message object.
 * @param {!proto.g8e.operator.v1.ListOperatorSlotsRequested} msg The message object to deserialize into.
 * @param {!jspb.BinaryReader} reader The BinaryReader to use.
 * @return {!proto.g8e.operator.v1.ListOperatorSlotsRequested}
 */
proto.g8e.operator.v1.ListOperatorSlotsRequested.deserializeBinaryFromReader = function(msg, reader) {
  while (reader.nextField()) {
    if (reader.isEndGroup()) {
      break;
    }
    var field = reader.getFieldNumber();
    switch (field) {
    case 1:
      var value = /** @type {string} */ (reader.readString());
      msg.setUserId(value);
      break;
    default:
      reader.skipField();
      break;
    }
  }
  return msg;
};


/**
 * Serializes the message to binary data (in protobuf wire format).
 * @return {!Uint8Array}
 */
proto.g8e.operator.v1.ListOperatorSlotsRequested.prototype.serializeBinary = function() {
  var writer = new jspb.BinaryWriter();
  proto.g8e.operator.v1.ListOperatorSlotsRequested.serializeBinaryToWriter(this, writer);
  return writer.getResultBuffer();
};


/**
 * Serializes the given message to binary data (in protobuf wire
 * format), writing to the given BinaryWriter.
 * @param {!proto.g8e.operator.v1.ListOperatorSlotsRequested} message
 * @param {!jspb.BinaryWriter} writer
 * @suppress {unusedLocalVariables} f is only used for nested messages
 */
proto.g8e.operator.v1.ListOperatorSlotsRequested.serializeBinaryToWriter = function(message, writer) {
  var f = undefined;
  f = message.getUserId();
  if (f.length > 0) {
    writer.writeString(
      1,
      f
    );
  }
};


/**
 * optional string user_id = 1;
 * @return {string}
 */
proto.g8e.operator.v1.ListOperatorSlotsRequested.prototype.getUserId = function() {
  return /** @type {string} */ (jspb.Message.getFieldWithDefault(this, 1, ""));
};


/**
 * @param {string} value
 * @return {!proto.g8e.operator.v1.ListOperatorSlotsRequested} returns this
 */
proto.g8e.operator.v1.ListOperatorSlotsRequested.prototype.setUserId = function(value) {
  return jspb.Message.setProto3StringField(this, 1, value);
};



/**
 * List of repeated fields within this message type.
 * @private {!Array<number>}
 * @const
 */
proto.g8e.operator.v1.ListOperatorSlotsResult.repeatedFields_ = [2];



if (jspb.Message.GENERATE_TO_OBJECT) {
/**
 * Creates an object representation of this proto.
 * Field names that are reserved in JavaScript and will be renamed to pb_name.
 * Optional fields that are not set will be set to undefined.
 * To access a reserved field use, foo.pb_<name>, eg, foo.pb_default.
 * For the list of reserved names please see:
 *     net/proto2/compiler/js/internal/generator.cc#kKeyword.
 * @param {boolean=} opt_includeInstance Deprecated. whether to include the
 *     JSPB instance for transitional soy proto support:
 *     http://goto/soy-param-migration
 * @return {!Object}
 */
proto.g8e.operator.v1.ListOperatorSlotsResult.prototype.toObject = function(opt_includeInstance) {
  return proto.g8e.operator.v1.ListOperatorSlotsResult.toObject(opt_includeInstance, this);
};


/**
 * Static version of the {@see toObject} method.
 * @param {boolean|undefined} includeInstance Deprecated. Whether to include
 *     the JSPB instance for transitional soy proto support:
 *     http://goto/soy-param-migration
 * @param {!proto.g8e.operator.v1.ListOperatorSlotsResult} msg The msg instance to transform.
 * @return {!Object}
 * @suppress {unusedLocalVariables} f is only used for nested messages
 */
proto.g8e.operator.v1.ListOperatorSlotsResult.toObject = function(includeInstance, msg) {
  var f, obj = {
    success: jspb.Message.getBooleanFieldWithDefault(msg, 1, false),
    operatorsList: jspb.Message.toObjectList(msg.getOperatorsList(),
    proto.g8e.operator.v1.OperatorDocument.toObject, includeInstance),
    error: jspb.Message.getFieldWithDefault(msg, 3, "")
  };

  if (includeInstance) {
    obj.$jspbMessageInstance = msg;
  }
  return obj;
};
}


/**
 * Deserializes binary data (in protobuf wire format).
 * @param {jspb.ByteSource} bytes The bytes to deserialize.
 * @return {!proto.g8e.operator.v1.ListOperatorSlotsResult}
 */
proto.g8e.operator.v1.ListOperatorSlotsResult.deserializeBinary = function(bytes) {
  var reader = new jspb.BinaryReader(bytes);
  var msg = new proto.g8e.operator.v1.ListOperatorSlotsResult;
  return proto.g8e.operator.v1.ListOperatorSlotsResult.deserializeBinaryFromReader(msg, reader);
};


/**
 * Deserializes binary data (in protobuf wire format) from the
 * given reader into the given message object.
 * @param {!proto.g8e.operator.v1.ListOperatorSlotsResult} msg The message object to deserialize into.
 * @param {!jspb.BinaryReader} reader The BinaryReader to use.
 * @return {!proto.g8e.operator.v1.ListOperatorSlotsResult}
 */
proto.g8e.operator.v1.ListOperatorSlotsResult.deserializeBinaryFromReader = function(msg, reader) {
  while (reader.nextField()) {
    if (reader.isEndGroup()) {
      break;
    }
    var field = reader.getFieldNumber();
    switch (field) {
    case 1:
      var value = /** @type {boolean} */ (reader.readBool());
      msg.setSuccess(value);
      break;
    case 2:
      var value = new proto.g8e.operator.v1.OperatorDocument;
      reader.readMessage(value,proto.g8e.operator.v1.OperatorDocument.deserializeBinaryFromReader);
      msg.addOperators(value);
      break;
    case 3:
      var value = /** @type {string} */ (reader.readString());
      msg.setError(value);
      break;
    default:
      reader.skipField();
      break;
    }
  }
  return msg;
};


/**
 * Serializes the message to binary data (in protobuf wire format).
 * @return {!Uint8Array}
 */
proto.g8e.operator.v1.ListOperatorSlotsResult.prototype.serializeBinary = function() {
  var writer = new jspb.BinaryWriter();
  proto.g8e.operator.v1.ListOperatorSlotsResult.serializeBinaryToWriter(this, writer);
  return writer.getResultBuffer();
};


/**
 * Serializes the given message to binary data (in protobuf wire
 * format), writing to the given BinaryWriter.
 * @param {!proto.g8e.operator.v1.ListOperatorSlotsResult} message
 * @param {!jspb.BinaryWriter} writer
 * @suppress {unusedLocalVariables} f is only used for nested messages
 */
proto.g8e.operator.v1.ListOperatorSlotsResult.serializeBinaryToWriter = function(message, writer) {
  var f = undefined;
  f = message.getSuccess();
  if (f) {
    writer.writeBool(
      1,
      f
    );
  }
  f = message.getOperatorsList();
  if (f.length > 0) {
    writer.writeRepeatedMessage(
      2,
      f,
      proto.g8e.operator.v1.OperatorDocument.serializeBinaryToWriter
    );
  }
  f = message.getError();
  if (f.length > 0) {
    writer.writeString(
      3,
      f
    );
  }
};


/**
 * optional bool success = 1;
 * @return {boolean}
 */
proto.g8e.operator.v1.ListOperatorSlotsResult.prototype.getSuccess = function() {
  return /** @type {boolean} */ (jspb.Message.getBooleanFieldWithDefault(this, 1, false));
};


/**
 * @param {boolean} value
 * @return {!proto.g8e.operator.v1.ListOperatorSlotsResult} returns this
 */
proto.g8e.operator.v1.ListOperatorSlotsResult.prototype.setSuccess = function(value) {
  return jspb.Message.setProto3BooleanField(this, 1, value);
};


/**
 * repeated OperatorDocument operators = 2;
 * @return {!Array<!proto.g8e.operator.v1.OperatorDocument>}
 */
proto.g8e.operator.v1.ListOperatorSlotsResult.prototype.getOperatorsList = function() {
  return /** @type{!Array<!proto.g8e.operator.v1.OperatorDocument>} */ (
    jspb.Message.getRepeatedWrapperField(this, proto.g8e.operator.v1.OperatorDocument, 2));
};


/**
 * @param {!Array<!proto.g8e.operator.v1.OperatorDocument>} value
 * @return {!proto.g8e.operator.v1.ListOperatorSlotsResult} returns this
*/
proto.g8e.operator.v1.ListOperatorSlotsResult.prototype.setOperatorsList = function(value) {
  return jspb.Message.setRepeatedWrapperField(this, 2, value);
};


/**
 * @param {!proto.g8e.operator.v1.OperatorDocument=} opt_value
 * @param {number=} opt_index
 * @return {!proto.g8e.operator.v1.OperatorDocument}
 */
proto.g8e.operator.v1.ListOperatorSlotsResult.prototype.addOperators = function(opt_value, opt_index) {
  return jspb.Message.addToRepeatedWrapperField(this, 2, opt_value, proto.g8e.operator.v1.OperatorDocument, opt_index);
};


/**
 * Clears the list making it empty but non-null.
 * @return {!proto.g8e.operator.v1.ListOperatorSlotsResult} returns this
 */
proto.g8e.operator.v1.ListOperatorSlotsResult.prototype.clearOperatorsList = function() {
  return this.setOperatorsList([]);
};


/**
 * optional string error = 3;
 * @return {string}
 */
proto.g8e.operator.v1.ListOperatorSlotsResult.prototype.getError = function() {
  return /** @type {string} */ (jspb.Message.getFieldWithDefault(this, 3, ""));
};


/**
 * @param {string} value
 * @return {!proto.g8e.operator.v1.ListOperatorSlotsResult} returns this
 */
proto.g8e.operator.v1.ListOperatorSlotsResult.prototype.setError = function(value) {
  return jspb.Message.setProto3StringField(this, 3, value);
};



/**
 * List of repeated fields within this message type.
 * @private {!Array<number>}
 * @const
 */
proto.g8e.operator.v1.BindOperatorsRequested.repeatedFields_ = [1];



if (jspb.Message.GENERATE_TO_OBJECT) {
/**
 * Creates an object representation of this proto.
 * Field names that are reserved in JavaScript and will be renamed to pb_name.
 * Optional fields that are not set will be set to undefined.
 * To access a reserved field use, foo.pb_<name>, eg, foo.pb_default.
 * For the list of reserved names please see:
 *     net/proto2/compiler/js/internal/generator.cc#kKeyword.
 * @param {boolean=} opt_includeInstance Deprecated. whether to include the
 *     JSPB instance for transitional soy proto support:
 *     http://goto/soy-param-migration
 * @return {!Object}
 */
proto.g8e.operator.v1.BindOperatorsRequested.prototype.toObject = function(opt_includeInstance) {
  return proto.g8e.operator.v1.BindOperatorsRequested.toObject(opt_includeInstance, this);
};


/**
 * Static version of the {@see toObject} method.
 * @param {boolean|undefined} includeInstance Deprecated. Whether to include
 *     the JSPB instance for transitional soy proto support:
 *     http://goto/soy-param-migration
 * @param {!proto.g8e.operator.v1.BindOperatorsRequested} msg The msg instance to transform.
 * @return {!Object}
 * @suppress {unusedLocalVariables} f is only used for nested messages
 */
proto.g8e.operator.v1.BindOperatorsRequested.toObject = function(includeInstance, msg) {
  var f, obj = {
    operatorIdsList: (f = jspb.Message.getRepeatedField(msg, 1)) == null ? undefined : f,
    userId: jspb.Message.getFieldWithDefault(msg, 2, ""),
    sessionId: jspb.Message.getFieldWithDefault(msg, 3, "")
  };

  if (includeInstance) {
    obj.$jspbMessageInstance = msg;
  }
  return obj;
};
}


/**
 * Deserializes binary data (in protobuf wire format).
 * @param {jspb.ByteSource} bytes The bytes to deserialize.
 * @return {!proto.g8e.operator.v1.BindOperatorsRequested}
 */
proto.g8e.operator.v1.BindOperatorsRequested.deserializeBinary = function(bytes) {
  var reader = new jspb.BinaryReader(bytes);
  var msg = new proto.g8e.operator.v1.BindOperatorsRequested;
  return proto.g8e.operator.v1.BindOperatorsRequested.deserializeBinaryFromReader(msg, reader);
};


/**
 * Deserializes binary data (in protobuf wire format) from the
 * given reader into the given message object.
 * @param {!proto.g8e.operator.v1.BindOperatorsRequested} msg The message object to deserialize into.
 * @param {!jspb.BinaryReader} reader The BinaryReader to use.
 * @return {!proto.g8e.operator.v1.BindOperatorsRequested}
 */
proto.g8e.operator.v1.BindOperatorsRequested.deserializeBinaryFromReader = function(msg, reader) {
  while (reader.nextField()) {
    if (reader.isEndGroup()) {
      break;
    }
    var field = reader.getFieldNumber();
    switch (field) {
    case 1:
      var value = /** @type {string} */ (reader.readString());
      msg.addOperatorIds(value);
      break;
    case 2:
      var value = /** @type {string} */ (reader.readString());
      msg.setUserId(value);
      break;
    case 3:
      var value = /** @type {string} */ (reader.readString());
      msg.setSessionId(value);
      break;
    default:
      reader.skipField();
      break;
    }
  }
  return msg;
};


/**
 * Serializes the message to binary data (in protobuf wire format).
 * @return {!Uint8Array}
 */
proto.g8e.operator.v1.BindOperatorsRequested.prototype.serializeBinary = function() {
  var writer = new jspb.BinaryWriter();
  proto.g8e.operator.v1.BindOperatorsRequested.serializeBinaryToWriter(this, writer);
  return writer.getResultBuffer();
};


/**
 * Serializes the given message to binary data (in protobuf wire
 * format), writing to the given BinaryWriter.
 * @param {!proto.g8e.operator.v1.BindOperatorsRequested} message
 * @param {!jspb.BinaryWriter} writer
 * @suppress {unusedLocalVariables} f is only used for nested messages
 */
proto.g8e.operator.v1.BindOperatorsRequested.serializeBinaryToWriter = function(message, writer) {
  var f = undefined;
  f = message.getOperatorIdsList();
  if (f.length > 0) {
    writer.writeRepeatedString(
      1,
      f
    );
  }
  f = message.getUserId();
  if (f.length > 0) {
    writer.writeString(
      2,
      f
    );
  }
  f = message.getSessionId();
  if (f.length > 0) {
    writer.writeString(
      3,
      f
    );
  }
};


/**
 * repeated string operator_ids = 1;
 * @return {!Array<string>}
 */
proto.g8e.operator.v1.BindOperatorsRequested.prototype.getOperatorIdsList = function() {
  return /** @type {!Array<string>} */ (jspb.Message.getRepeatedField(this, 1));
};


/**
 * @param {!Array<string>} value
 * @return {!proto.g8e.operator.v1.BindOperatorsRequested} returns this
 */
proto.g8e.operator.v1.BindOperatorsRequested.prototype.setOperatorIdsList = function(value) {
  return jspb.Message.setField(this, 1, value || []);
};


/**
 * @param {string} value
 * @param {number=} opt_index
 * @return {!proto.g8e.operator.v1.BindOperatorsRequested} returns this
 */
proto.g8e.operator.v1.BindOperatorsRequested.prototype.addOperatorIds = function(value, opt_index) {
  return jspb.Message.addToRepeatedField(this, 1, value, opt_index);
};


/**
 * Clears the list making it empty but non-null.
 * @return {!proto.g8e.operator.v1.BindOperatorsRequested} returns this
 */
proto.g8e.operator.v1.BindOperatorsRequested.prototype.clearOperatorIdsList = function() {
  return this.setOperatorIdsList([]);
};


/**
 * optional string user_id = 2;
 * @return {string}
 */
proto.g8e.operator.v1.BindOperatorsRequested.prototype.getUserId = function() {
  return /** @type {string} */ (jspb.Message.getFieldWithDefault(this, 2, ""));
};


/**
 * @param {string} value
 * @return {!proto.g8e.operator.v1.BindOperatorsRequested} returns this
 */
proto.g8e.operator.v1.BindOperatorsRequested.prototype.setUserId = function(value) {
  return jspb.Message.setProto3StringField(this, 2, value);
};


/**
 * optional string session_id = 3;
 * @return {string}
 */
proto.g8e.operator.v1.BindOperatorsRequested.prototype.getSessionId = function() {
  return /** @type {string} */ (jspb.Message.getFieldWithDefault(this, 3, ""));
};


/**
 * @param {string} value
 * @return {!proto.g8e.operator.v1.BindOperatorsRequested} returns this
 */
proto.g8e.operator.v1.BindOperatorsRequested.prototype.setSessionId = function(value) {
  return jspb.Message.setProto3StringField(this, 3, value);
};



/**
 * List of repeated fields within this message type.
 * @private {!Array<number>}
 * @const
 */
proto.g8e.operator.v1.BindOperatorsResult.repeatedFields_ = [4,5];



if (jspb.Message.GENERATE_TO_OBJECT) {
/**
 * Creates an object representation of this proto.
 * Field names that are reserved in JavaScript and will be renamed to pb_name.
 * Optional fields that are not set will be set to undefined.
 * To access a reserved field use, foo.pb_<name>, eg, foo.pb_default.
 * For the list of reserved names please see:
 *     net/proto2/compiler/js/internal/generator.cc#kKeyword.
 * @param {boolean=} opt_includeInstance Deprecated. whether to include the
 *     JSPB instance for transitional soy proto support:
 *     http://goto/soy-param-migration
 * @return {!Object}
 */
proto.g8e.operator.v1.BindOperatorsResult.prototype.toObject = function(opt_includeInstance) {
  return proto.g8e.operator.v1.BindOperatorsResult.toObject(opt_includeInstance, this);
};


/**
 * Static version of the {@see toObject} method.
 * @param {boolean|undefined} includeInstance Deprecated. Whether to include
 *     the JSPB instance for transitional soy proto support:
 *     http://goto/soy-param-migration
 * @param {!proto.g8e.operator.v1.BindOperatorsResult} msg The msg instance to transform.
 * @return {!Object}
 * @suppress {unusedLocalVariables} f is only used for nested messages
 */
proto.g8e.operator.v1.BindOperatorsResult.toObject = function(includeInstance, msg) {
  var f, obj = {
    success: jspb.Message.getBooleanFieldWithDefault(msg, 1, false),
    boundCount: jspb.Message.getFieldWithDefault(msg, 2, 0),
    failedCount: jspb.Message.getFieldWithDefault(msg, 3, 0),
    boundOperatorIdsList: (f = jspb.Message.getRepeatedField(msg, 4)) == null ? undefined : f,
    failedOperatorIdsList: (f = jspb.Message.getRepeatedField(msg, 5)) == null ? undefined : f,
    error: jspb.Message.getFieldWithDefault(msg, 6, "")
  };

  if (includeInstance) {
    obj.$jspbMessageInstance = msg;
  }
  return obj;
};
}


/**
 * Deserializes binary data (in protobuf wire format).
 * @param {jspb.ByteSource} bytes The bytes to deserialize.
 * @return {!proto.g8e.operator.v1.BindOperatorsResult}
 */
proto.g8e.operator.v1.BindOperatorsResult.deserializeBinary = function(bytes) {
  var reader = new jspb.BinaryReader(bytes);
  var msg = new proto.g8e.operator.v1.BindOperatorsResult;
  return proto.g8e.operator.v1.BindOperatorsResult.deserializeBinaryFromReader(msg, reader);
};


/**
 * Deserializes binary data (in protobuf wire format) from the
 * given reader into the given message object.
 * @param {!proto.g8e.operator.v1.BindOperatorsResult} msg The message object to deserialize into.
 * @param {!jspb.BinaryReader} reader The BinaryReader to use.
 * @return {!proto.g8e.operator.v1.BindOperatorsResult}
 */
proto.g8e.operator.v1.BindOperatorsResult.deserializeBinaryFromReader = function(msg, reader) {
  while (reader.nextField()) {
    if (reader.isEndGroup()) {
      break;
    }
    var field = reader.getFieldNumber();
    switch (field) {
    case 1:
      var value = /** @type {boolean} */ (reader.readBool());
      msg.setSuccess(value);
      break;
    case 2:
      var value = /** @type {number} */ (reader.readInt32());
      msg.setBoundCount(value);
      break;
    case 3:
      var value = /** @type {number} */ (reader.readInt32());
      msg.setFailedCount(value);
      break;
    case 4:
      var value = /** @type {string} */ (reader.readString());
      msg.addBoundOperatorIds(value);
      break;
    case 5:
      var value = /** @type {string} */ (reader.readString());
      msg.addFailedOperatorIds(value);
      break;
    case 6:
      var value = /** @type {string} */ (reader.readString());
      msg.setError(value);
      break;
    default:
      reader.skipField();
      break;
    }
  }
  return msg;
};


/**
 * Serializes the message to binary data (in protobuf wire format).
 * @return {!Uint8Array}
 */
proto.g8e.operator.v1.BindOperatorsResult.prototype.serializeBinary = function() {
  var writer = new jspb.BinaryWriter();
  proto.g8e.operator.v1.BindOperatorsResult.serializeBinaryToWriter(this, writer);
  return writer.getResultBuffer();
};


/**
 * Serializes the given message to binary data (in protobuf wire
 * format), writing to the given BinaryWriter.
 * @param {!proto.g8e.operator.v1.BindOperatorsResult} message
 * @param {!jspb.BinaryWriter} writer
 * @suppress {unusedLocalVariables} f is only used for nested messages
 */
proto.g8e.operator.v1.BindOperatorsResult.serializeBinaryToWriter = function(message, writer) {
  var f = undefined;
  f = message.getSuccess();
  if (f) {
    writer.writeBool(
      1,
      f
    );
  }
  f = message.getBoundCount();
  if (f !== 0) {
    writer.writeInt32(
      2,
      f
    );
  }
  f = message.getFailedCount();
  if (f !== 0) {
    writer.writeInt32(
      3,
      f
    );
  }
  f = message.getBoundOperatorIdsList();
  if (f.length > 0) {
    writer.writeRepeatedString(
      4,
      f
    );
  }
  f = message.getFailedOperatorIdsList();
  if (f.length > 0) {
    writer.writeRepeatedString(
      5,
      f
    );
  }
  f = message.getError();
  if (f.length > 0) {
    writer.writeString(
      6,
      f
    );
  }
};


/**
 * optional bool success = 1;
 * @return {boolean}
 */
proto.g8e.operator.v1.BindOperatorsResult.prototype.getSuccess = function() {
  return /** @type {boolean} */ (jspb.Message.getBooleanFieldWithDefault(this, 1, false));
};


/**
 * @param {boolean} value
 * @return {!proto.g8e.operator.v1.BindOperatorsResult} returns this
 */
proto.g8e.operator.v1.BindOperatorsResult.prototype.setSuccess = function(value) {
  return jspb.Message.setProto3BooleanField(this, 1, value);
};


/**
 * optional int32 bound_count = 2;
 * @return {number}
 */
proto.g8e.operator.v1.BindOperatorsResult.prototype.getBoundCount = function() {
  return /** @type {number} */ (jspb.Message.getFieldWithDefault(this, 2, 0));
};


/**
 * @param {number} value
 * @return {!proto.g8e.operator.v1.BindOperatorsResult} returns this
 */
proto.g8e.operator.v1.BindOperatorsResult.prototype.setBoundCount = function(value) {
  return jspb.Message.setProto3IntField(this, 2, value);
};


/**
 * optional int32 failed_count = 3;
 * @return {number}
 */
proto.g8e.operator.v1.BindOperatorsResult.prototype.getFailedCount = function() {
  return /** @type {number} */ (jspb.Message.getFieldWithDefault(this, 3, 0));
};


/**
 * @param {number} value
 * @return {!proto.g8e.operator.v1.BindOperatorsResult} returns this
 */
proto.g8e.operator.v1.BindOperatorsResult.prototype.setFailedCount = function(value) {
  return jspb.Message.setProto3IntField(this, 3, value);
};


/**
 * repeated string bound_operator_ids = 4;
 * @return {!Array<string>}
 */
proto.g8e.operator.v1.BindOperatorsResult.prototype.getBoundOperatorIdsList = function() {
  return /** @type {!Array<string>} */ (jspb.Message.getRepeatedField(this, 4));
};


/**
 * @param {!Array<string>} value
 * @return {!proto.g8e.operator.v1.BindOperatorsResult} returns this
 */
proto.g8e.operator.v1.BindOperatorsResult.prototype.setBoundOperatorIdsList = function(value) {
  return jspb.Message.setField(this, 4, value || []);
};


/**
 * @param {string} value
 * @param {number=} opt_index
 * @return {!proto.g8e.operator.v1.BindOperatorsResult} returns this
 */
proto.g8e.operator.v1.BindOperatorsResult.prototype.addBoundOperatorIds = function(value, opt_index) {
  return jspb.Message.addToRepeatedField(this, 4, value, opt_index);
};


/**
 * Clears the list making it empty but non-null.
 * @return {!proto.g8e.operator.v1.BindOperatorsResult} returns this
 */
proto.g8e.operator.v1.BindOperatorsResult.prototype.clearBoundOperatorIdsList = function() {
  return this.setBoundOperatorIdsList([]);
};


/**
 * repeated string failed_operator_ids = 5;
 * @return {!Array<string>}
 */
proto.g8e.operator.v1.BindOperatorsResult.prototype.getFailedOperatorIdsList = function() {
  return /** @type {!Array<string>} */ (jspb.Message.getRepeatedField(this, 5));
};


/**
 * @param {!Array<string>} value
 * @return {!proto.g8e.operator.v1.BindOperatorsResult} returns this
 */
proto.g8e.operator.v1.BindOperatorsResult.prototype.setFailedOperatorIdsList = function(value) {
  return jspb.Message.setField(this, 5, value || []);
};


/**
 * @param {string} value
 * @param {number=} opt_index
 * @return {!proto.g8e.operator.v1.BindOperatorsResult} returns this
 */
proto.g8e.operator.v1.BindOperatorsResult.prototype.addFailedOperatorIds = function(value, opt_index) {
  return jspb.Message.addToRepeatedField(this, 5, value, opt_index);
};


/**
 * Clears the list making it empty but non-null.
 * @return {!proto.g8e.operator.v1.BindOperatorsResult} returns this
 */
proto.g8e.operator.v1.BindOperatorsResult.prototype.clearFailedOperatorIdsList = function() {
  return this.setFailedOperatorIdsList([]);
};


/**
 * optional string error = 6;
 * @return {string}
 */
proto.g8e.operator.v1.BindOperatorsResult.prototype.getError = function() {
  return /** @type {string} */ (jspb.Message.getFieldWithDefault(this, 6, ""));
};


/**
 * @param {string} value
 * @return {!proto.g8e.operator.v1.BindOperatorsResult} returns this
 */
proto.g8e.operator.v1.BindOperatorsResult.prototype.setError = function(value) {
  return jspb.Message.setProto3StringField(this, 6, value);
};



/**
 * List of repeated fields within this message type.
 * @private {!Array<number>}
 * @const
 */
proto.g8e.operator.v1.UnbindOperatorsRequested.repeatedFields_ = [1];



if (jspb.Message.GENERATE_TO_OBJECT) {
/**
 * Creates an object representation of this proto.
 * Field names that are reserved in JavaScript and will be renamed to pb_name.
 * Optional fields that are not set will be set to undefined.
 * To access a reserved field use, foo.pb_<name>, eg, foo.pb_default.
 * For the list of reserved names please see:
 *     net/proto2/compiler/js/internal/generator.cc#kKeyword.
 * @param {boolean=} opt_includeInstance Deprecated. whether to include the
 *     JSPB instance for transitional soy proto support:
 *     http://goto/soy-param-migration
 * @return {!Object}
 */
proto.g8e.operator.v1.UnbindOperatorsRequested.prototype.toObject = function(opt_includeInstance) {
  return proto.g8e.operator.v1.UnbindOperatorsRequested.toObject(opt_includeInstance, this);
};


/**
 * Static version of the {@see toObject} method.
 * @param {boolean|undefined} includeInstance Deprecated. Whether to include
 *     the JSPB instance for transitional soy proto support:
 *     http://goto/soy-param-migration
 * @param {!proto.g8e.operator.v1.UnbindOperatorsRequested} msg The msg instance to transform.
 * @return {!Object}
 * @suppress {unusedLocalVariables} f is only used for nested messages
 */
proto.g8e.operator.v1.UnbindOperatorsRequested.toObject = function(includeInstance, msg) {
  var f, obj = {
    operatorIdsList: (f = jspb.Message.getRepeatedField(msg, 1)) == null ? undefined : f,
    userId: jspb.Message.getFieldWithDefault(msg, 2, ""),
    sessionId: jspb.Message.getFieldWithDefault(msg, 3, "")
  };

  if (includeInstance) {
    obj.$jspbMessageInstance = msg;
  }
  return obj;
};
}


/**
 * Deserializes binary data (in protobuf wire format).
 * @param {jspb.ByteSource} bytes The bytes to deserialize.
 * @return {!proto.g8e.operator.v1.UnbindOperatorsRequested}
 */
proto.g8e.operator.v1.UnbindOperatorsRequested.deserializeBinary = function(bytes) {
  var reader = new jspb.BinaryReader(bytes);
  var msg = new proto.g8e.operator.v1.UnbindOperatorsRequested;
  return proto.g8e.operator.v1.UnbindOperatorsRequested.deserializeBinaryFromReader(msg, reader);
};


/**
 * Deserializes binary data (in protobuf wire format) from the
 * given reader into the given message object.
 * @param {!proto.g8e.operator.v1.UnbindOperatorsRequested} msg The message object to deserialize into.
 * @param {!jspb.BinaryReader} reader The BinaryReader to use.
 * @return {!proto.g8e.operator.v1.UnbindOperatorsRequested}
 */
proto.g8e.operator.v1.UnbindOperatorsRequested.deserializeBinaryFromReader = function(msg, reader) {
  while (reader.nextField()) {
    if (reader.isEndGroup()) {
      break;
    }
    var field = reader.getFieldNumber();
    switch (field) {
    case 1:
      var value = /** @type {string} */ (reader.readString());
      msg.addOperatorIds(value);
      break;
    case 2:
      var value = /** @type {string} */ (reader.readString());
      msg.setUserId(value);
      break;
    case 3:
      var value = /** @type {string} */ (reader.readString());
      msg.setSessionId(value);
      break;
    default:
      reader.skipField();
      break;
    }
  }
  return msg;
};


/**
 * Serializes the message to binary data (in protobuf wire format).
 * @return {!Uint8Array}
 */
proto.g8e.operator.v1.UnbindOperatorsRequested.prototype.serializeBinary = function() {
  var writer = new jspb.BinaryWriter();
  proto.g8e.operator.v1.UnbindOperatorsRequested.serializeBinaryToWriter(this, writer);
  return writer.getResultBuffer();
};


/**
 * Serializes the given message to binary data (in protobuf wire
 * format), writing to the given BinaryWriter.
 * @param {!proto.g8e.operator.v1.UnbindOperatorsRequested} message
 * @param {!jspb.BinaryWriter} writer
 * @suppress {unusedLocalVariables} f is only used for nested messages
 */
proto.g8e.operator.v1.UnbindOperatorsRequested.serializeBinaryToWriter = function(message, writer) {
  var f = undefined;
  f = message.getOperatorIdsList();
  if (f.length > 0) {
    writer.writeRepeatedString(
      1,
      f
    );
  }
  f = message.getUserId();
  if (f.length > 0) {
    writer.writeString(
      2,
      f
    );
  }
  f = message.getSessionId();
  if (f.length > 0) {
    writer.writeString(
      3,
      f
    );
  }
};


/**
 * repeated string operator_ids = 1;
 * @return {!Array<string>}
 */
proto.g8e.operator.v1.UnbindOperatorsRequested.prototype.getOperatorIdsList = function() {
  return /** @type {!Array<string>} */ (jspb.Message.getRepeatedField(this, 1));
};


/**
 * @param {!Array<string>} value
 * @return {!proto.g8e.operator.v1.UnbindOperatorsRequested} returns this
 */
proto.g8e.operator.v1.UnbindOperatorsRequested.prototype.setOperatorIdsList = function(value) {
  return jspb.Message.setField(this, 1, value || []);
};


/**
 * @param {string} value
 * @param {number=} opt_index
 * @return {!proto.g8e.operator.v1.UnbindOperatorsRequested} returns this
 */
proto.g8e.operator.v1.UnbindOperatorsRequested.prototype.addOperatorIds = function(value, opt_index) {
  return jspb.Message.addToRepeatedField(this, 1, value, opt_index);
};


/**
 * Clears the list making it empty but non-null.
 * @return {!proto.g8e.operator.v1.UnbindOperatorsRequested} returns this
 */
proto.g8e.operator.v1.UnbindOperatorsRequested.prototype.clearOperatorIdsList = function() {
  return this.setOperatorIdsList([]);
};


/**
 * optional string user_id = 2;
 * @return {string}
 */
proto.g8e.operator.v1.UnbindOperatorsRequested.prototype.getUserId = function() {
  return /** @type {string} */ (jspb.Message.getFieldWithDefault(this, 2, ""));
};


/**
 * @param {string} value
 * @return {!proto.g8e.operator.v1.UnbindOperatorsRequested} returns this
 */
proto.g8e.operator.v1.UnbindOperatorsRequested.prototype.setUserId = function(value) {
  return jspb.Message.setProto3StringField(this, 2, value);
};


/**
 * optional string session_id = 3;
 * @return {string}
 */
proto.g8e.operator.v1.UnbindOperatorsRequested.prototype.getSessionId = function() {
  return /** @type {string} */ (jspb.Message.getFieldWithDefault(this, 3, ""));
};


/**
 * @param {string} value
 * @return {!proto.g8e.operator.v1.UnbindOperatorsRequested} returns this
 */
proto.g8e.operator.v1.UnbindOperatorsRequested.prototype.setSessionId = function(value) {
  return jspb.Message.setProto3StringField(this, 3, value);
};



/**
 * List of repeated fields within this message type.
 * @private {!Array<number>}
 * @const
 */
proto.g8e.operator.v1.UnbindOperatorsResult.repeatedFields_ = [4,5];



if (jspb.Message.GENERATE_TO_OBJECT) {
/**
 * Creates an object representation of this proto.
 * Field names that are reserved in JavaScript and will be renamed to pb_name.
 * Optional fields that are not set will be set to undefined.
 * To access a reserved field use, foo.pb_<name>, eg, foo.pb_default.
 * For the list of reserved names please see:
 *     net/proto2/compiler/js/internal/generator.cc#kKeyword.
 * @param {boolean=} opt_includeInstance Deprecated. whether to include the
 *     JSPB instance for transitional soy proto support:
 *     http://goto/soy-param-migration
 * @return {!Object}
 */
proto.g8e.operator.v1.UnbindOperatorsResult.prototype.toObject = function(opt_includeInstance) {
  return proto.g8e.operator.v1.UnbindOperatorsResult.toObject(opt_includeInstance, this);
};


/**
 * Static version of the {@see toObject} method.
 * @param {boolean|undefined} includeInstance Deprecated. Whether to include
 *     the JSPB instance for transitional soy proto support:
 *     http://goto/soy-param-migration
 * @param {!proto.g8e.operator.v1.UnbindOperatorsResult} msg The msg instance to transform.
 * @return {!Object}
 * @suppress {unusedLocalVariables} f is only used for nested messages
 */
proto.g8e.operator.v1.UnbindOperatorsResult.toObject = function(includeInstance, msg) {
  var f, obj = {
    success: jspb.Message.getBooleanFieldWithDefault(msg, 1, false),
    unboundCount: jspb.Message.getFieldWithDefault(msg, 2, 0),
    failedCount: jspb.Message.getFieldWithDefault(msg, 3, 0),
    unboundOperatorIdsList: (f = jspb.Message.getRepeatedField(msg, 4)) == null ? undefined : f,
    failedOperatorIdsList: (f = jspb.Message.getRepeatedField(msg, 5)) == null ? undefined : f,
    error: jspb.Message.getFieldWithDefault(msg, 6, "")
  };

  if (includeInstance) {
    obj.$jspbMessageInstance = msg;
  }
  return obj;
};
}


/**
 * Deserializes binary data (in protobuf wire format).
 * @param {jspb.ByteSource} bytes The bytes to deserialize.
 * @return {!proto.g8e.operator.v1.UnbindOperatorsResult}
 */
proto.g8e.operator.v1.UnbindOperatorsResult.deserializeBinary = function(bytes) {
  var reader = new jspb.BinaryReader(bytes);
  var msg = new proto.g8e.operator.v1.UnbindOperatorsResult;
  return proto.g8e.operator.v1.UnbindOperatorsResult.deserializeBinaryFromReader(msg, reader);
};


/**
 * Deserializes binary data (in protobuf wire format) from the
 * given reader into the given message object.
 * @param {!proto.g8e.operator.v1.UnbindOperatorsResult} msg The message object to deserialize into.
 * @param {!jspb.BinaryReader} reader The BinaryReader to use.
 * @return {!proto.g8e.operator.v1.UnbindOperatorsResult}
 */
proto.g8e.operator.v1.UnbindOperatorsResult.deserializeBinaryFromReader = function(msg, reader) {
  while (reader.nextField()) {
    if (reader.isEndGroup()) {
      break;
    }
    var field = reader.getFieldNumber();
    switch (field) {
    case 1:
      var value = /** @type {boolean} */ (reader.readBool());
      msg.setSuccess(value);
      break;
    case 2:
      var value = /** @type {number} */ (reader.readInt32());
      msg.setUnboundCount(value);
      break;
    case 3:
      var value = /** @type {number} */ (reader.readInt32());
      msg.setFailedCount(value);
      break;
    case 4:
      var value = /** @type {string} */ (reader.readString());
      msg.addUnboundOperatorIds(value);
      break;
    case 5:
      var value = /** @type {string} */ (reader.readString());
      msg.addFailedOperatorIds(value);
      break;
    case 6:
      var value = /** @type {string} */ (reader.readString());
      msg.setError(value);
      break;
    default:
      reader.skipField();
      break;
    }
  }
  return msg;
};


/**
 * Serializes the message to binary data (in protobuf wire format).
 * @return {!Uint8Array}
 */
proto.g8e.operator.v1.UnbindOperatorsResult.prototype.serializeBinary = function() {
  var writer = new jspb.BinaryWriter();
  proto.g8e.operator.v1.UnbindOperatorsResult.serializeBinaryToWriter(this, writer);
  return writer.getResultBuffer();
};


/**
 * Serializes the given message to binary data (in protobuf wire
 * format), writing to the given BinaryWriter.
 * @param {!proto.g8e.operator.v1.UnbindOperatorsResult} message
 * @param {!jspb.BinaryWriter} writer
 * @suppress {unusedLocalVariables} f is only used for nested messages
 */
proto.g8e.operator.v1.UnbindOperatorsResult.serializeBinaryToWriter = function(message, writer) {
  var f = undefined;
  f = message.getSuccess();
  if (f) {
    writer.writeBool(
      1,
      f
    );
  }
  f = message.getUnboundCount();
  if (f !== 0) {
    writer.writeInt32(
      2,
      f
    );
  }
  f = message.getFailedCount();
  if (f !== 0) {
    writer.writeInt32(
      3,
      f
    );
  }
  f = message.getUnboundOperatorIdsList();
  if (f.length > 0) {
    writer.writeRepeatedString(
      4,
      f
    );
  }
  f = message.getFailedOperatorIdsList();
  if (f.length > 0) {
    writer.writeRepeatedString(
      5,
      f
    );
  }
  f = message.getError();
  if (f.length > 0) {
    writer.writeString(
      6,
      f
    );
  }
};


/**
 * optional bool success = 1;
 * @return {boolean}
 */
proto.g8e.operator.v1.UnbindOperatorsResult.prototype.getSuccess = function() {
  return /** @type {boolean} */ (jspb.Message.getBooleanFieldWithDefault(this, 1, false));
};


/**
 * @param {boolean} value
 * @return {!proto.g8e.operator.v1.UnbindOperatorsResult} returns this
 */
proto.g8e.operator.v1.UnbindOperatorsResult.prototype.setSuccess = function(value) {
  return jspb.Message.setProto3BooleanField(this, 1, value);
};


/**
 * optional int32 unbound_count = 2;
 * @return {number}
 */
proto.g8e.operator.v1.UnbindOperatorsResult.prototype.getUnboundCount = function() {
  return /** @type {number} */ (jspb.Message.getFieldWithDefault(this, 2, 0));
};


/**
 * @param {number} value
 * @return {!proto.g8e.operator.v1.UnbindOperatorsResult} returns this
 */
proto.g8e.operator.v1.UnbindOperatorsResult.prototype.setUnboundCount = function(value) {
  return jspb.Message.setProto3IntField(this, 2, value);
};


/**
 * optional int32 failed_count = 3;
 * @return {number}
 */
proto.g8e.operator.v1.UnbindOperatorsResult.prototype.getFailedCount = function() {
  return /** @type {number} */ (jspb.Message.getFieldWithDefault(this, 3, 0));
};


/**
 * @param {number} value
 * @return {!proto.g8e.operator.v1.UnbindOperatorsResult} returns this
 */
proto.g8e.operator.v1.UnbindOperatorsResult.prototype.setFailedCount = function(value) {
  return jspb.Message.setProto3IntField(this, 3, value);
};


/**
 * repeated string unbound_operator_ids = 4;
 * @return {!Array<string>}
 */
proto.g8e.operator.v1.UnbindOperatorsResult.prototype.getUnboundOperatorIdsList = function() {
  return /** @type {!Array<string>} */ (jspb.Message.getRepeatedField(this, 4));
};


/**
 * @param {!Array<string>} value
 * @return {!proto.g8e.operator.v1.UnbindOperatorsResult} returns this
 */
proto.g8e.operator.v1.UnbindOperatorsResult.prototype.setUnboundOperatorIdsList = function(value) {
  return jspb.Message.setField(this, 4, value || []);
};


/**
 * @param {string} value
 * @param {number=} opt_index
 * @return {!proto.g8e.operator.v1.UnbindOperatorsResult} returns this
 */
proto.g8e.operator.v1.UnbindOperatorsResult.prototype.addUnboundOperatorIds = function(value, opt_index) {
  return jspb.Message.addToRepeatedField(this, 4, value, opt_index);
};


/**
 * Clears the list making it empty but non-null.
 * @return {!proto.g8e.operator.v1.UnbindOperatorsResult} returns this
 */
proto.g8e.operator.v1.UnbindOperatorsResult.prototype.clearUnboundOperatorIdsList = function() {
  return this.setUnboundOperatorIdsList([]);
};


/**
 * repeated string failed_operator_ids = 5;
 * @return {!Array<string>}
 */
proto.g8e.operator.v1.UnbindOperatorsResult.prototype.getFailedOperatorIdsList = function() {
  return /** @type {!Array<string>} */ (jspb.Message.getRepeatedField(this, 5));
};


/**
 * @param {!Array<string>} value
 * @return {!proto.g8e.operator.v1.UnbindOperatorsResult} returns this
 */
proto.g8e.operator.v1.UnbindOperatorsResult.prototype.setFailedOperatorIdsList = function(value) {
  return jspb.Message.setField(this, 5, value || []);
};


/**
 * @param {string} value
 * @param {number=} opt_index
 * @return {!proto.g8e.operator.v1.UnbindOperatorsResult} returns this
 */
proto.g8e.operator.v1.UnbindOperatorsResult.prototype.addFailedOperatorIds = function(value, opt_index) {
  return jspb.Message.addToRepeatedField(this, 5, value, opt_index);
};


/**
 * Clears the list making it empty but non-null.
 * @return {!proto.g8e.operator.v1.UnbindOperatorsResult} returns this
 */
proto.g8e.operator.v1.UnbindOperatorsResult.prototype.clearFailedOperatorIdsList = function() {
  return this.setFailedOperatorIdsList([]);
};


/**
 * optional string error = 6;
 * @return {string}
 */
proto.g8e.operator.v1.UnbindOperatorsResult.prototype.getError = function() {
  return /** @type {string} */ (jspb.Message.getFieldWithDefault(this, 6, ""));
};


/**
 * @param {string} value
 * @return {!proto.g8e.operator.v1.UnbindOperatorsResult} returns this
 */
proto.g8e.operator.v1.UnbindOperatorsResult.prototype.setError = function(value) {
  return jspb.Message.setProto3StringField(this, 6, value);
};





if (jspb.Message.GENERATE_TO_OBJECT) {
/**
 * Creates an object representation of this proto.
 * Field names that are reserved in JavaScript and will be renamed to pb_name.
 * Optional fields that are not set will be set to undefined.
 * To access a reserved field use, foo.pb_<name>, eg, foo.pb_default.
 * For the list of reserved names please see:
 *     net/proto2/compiler/js/internal/generator.cc#kKeyword.
 * @param {boolean=} opt_includeInstance Deprecated. whether to include the
 *     JSPB instance for transitional soy proto support:
 *     http://goto/soy-param-migration
 * @return {!Object}
 */
proto.g8e.operator.v1.SetTargetContextRequested.prototype.toObject = function(opt_includeInstance) {
  return proto.g8e.operator.v1.SetTargetContextRequested.toObject(opt_includeInstance, this);
};


/**
 * Static version of the {@see toObject} method.
 * @param {boolean|undefined} includeInstance Deprecated. Whether to include
 *     the JSPB instance for transitional soy proto support:
 *     http://goto/soy-param-migration
 * @param {!proto.g8e.operator.v1.SetTargetContextRequested} msg The msg instance to transform.
 * @return {!Object}
 * @suppress {unusedLocalVariables} f is only used for nested messages
 */
proto.g8e.operator.v1.SetTargetContextRequested.toObject = function(includeInstance, msg) {
  var f, obj = {
    operatorId: jspb.Message.getFieldWithDefault(msg, 1, ""),
    userId: jspb.Message.getFieldWithDefault(msg, 2, ""),
    sessionId: jspb.Message.getFieldWithDefault(msg, 3, "")
  };

  if (includeInstance) {
    obj.$jspbMessageInstance = msg;
  }
  return obj;
};
}


/**
 * Deserializes binary data (in protobuf wire format).
 * @param {jspb.ByteSource} bytes The bytes to deserialize.
 * @return {!proto.g8e.operator.v1.SetTargetContextRequested}
 */
proto.g8e.operator.v1.SetTargetContextRequested.deserializeBinary = function(bytes) {
  var reader = new jspb.BinaryReader(bytes);
  var msg = new proto.g8e.operator.v1.SetTargetContextRequested;
  return proto.g8e.operator.v1.SetTargetContextRequested.deserializeBinaryFromReader(msg, reader);
};


/**
 * Deserializes binary data (in protobuf wire format) from the
 * given reader into the given message object.
 * @param {!proto.g8e.operator.v1.SetTargetContextRequested} msg The message object to deserialize into.
 * @param {!jspb.BinaryReader} reader The BinaryReader to use.
 * @return {!proto.g8e.operator.v1.SetTargetContextRequested}
 */
proto.g8e.operator.v1.SetTargetContextRequested.deserializeBinaryFromReader = function(msg, reader) {
  while (reader.nextField()) {
    if (reader.isEndGroup()) {
      break;
    }
    var field = reader.getFieldNumber();
    switch (field) {
    case 1:
      var value = /** @type {string} */ (reader.readString());
      msg.setOperatorId(value);
      break;
    case 2:
      var value = /** @type {string} */ (reader.readString());
      msg.setUserId(value);
      break;
    case 3:
      var value = /** @type {string} */ (reader.readString());
      msg.setSessionId(value);
      break;
    default:
      reader.skipField();
      break;
    }
  }
  return msg;
};


/**
 * Serializes the message to binary data (in protobuf wire format).
 * @return {!Uint8Array}
 */
proto.g8e.operator.v1.SetTargetContextRequested.prototype.serializeBinary = function() {
  var writer = new jspb.BinaryWriter();
  proto.g8e.operator.v1.SetTargetContextRequested.serializeBinaryToWriter(this, writer);
  return writer.getResultBuffer();
};


/**
 * Serializes the given message to binary data (in protobuf wire
 * format), writing to the given BinaryWriter.
 * @param {!proto.g8e.operator.v1.SetTargetContextRequested} message
 * @param {!jspb.BinaryWriter} writer
 * @suppress {unusedLocalVariables} f is only used for nested messages
 */
proto.g8e.operator.v1.SetTargetContextRequested.serializeBinaryToWriter = function(message, writer) {
  var f = undefined;
  f = message.getOperatorId();
  if (f.length > 0) {
    writer.writeString(
      1,
      f
    );
  }
  f = message.getUserId();
  if (f.length > 0) {
    writer.writeString(
      2,
      f
    );
  }
  f = message.getSessionId();
  if (f.length > 0) {
    writer.writeString(
      3,
      f
    );
  }
};


/**
 * optional string operator_id = 1;
 * @return {string}
 */
proto.g8e.operator.v1.SetTargetContextRequested.prototype.getOperatorId = function() {
  return /** @type {string} */ (jspb.Message.getFieldWithDefault(this, 1, ""));
};


/**
 * @param {string} value
 * @return {!proto.g8e.operator.v1.SetTargetContextRequested} returns this
 */
proto.g8e.operator.v1.SetTargetContextRequested.prototype.setOperatorId = function(value) {
  return jspb.Message.setProto3StringField(this, 1, value);
};


/**
 * optional string user_id = 2;
 * @return {string}
 */
proto.g8e.operator.v1.SetTargetContextRequested.prototype.getUserId = function() {
  return /** @type {string} */ (jspb.Message.getFieldWithDefault(this, 2, ""));
};


/**
 * @param {string} value
 * @return {!proto.g8e.operator.v1.SetTargetContextRequested} returns this
 */
proto.g8e.operator.v1.SetTargetContextRequested.prototype.setUserId = function(value) {
  return jspb.Message.setProto3StringField(this, 2, value);
};


/**
 * optional string session_id = 3;
 * @return {string}
 */
proto.g8e.operator.v1.SetTargetContextRequested.prototype.getSessionId = function() {
  return /** @type {string} */ (jspb.Message.getFieldWithDefault(this, 3, ""));
};


/**
 * @param {string} value
 * @return {!proto.g8e.operator.v1.SetTargetContextRequested} returns this
 */
proto.g8e.operator.v1.SetTargetContextRequested.prototype.setSessionId = function(value) {
  return jspb.Message.setProto3StringField(this, 3, value);
};





if (jspb.Message.GENERATE_TO_OBJECT) {
/**
 * Creates an object representation of this proto.
 * Field names that are reserved in JavaScript and will be renamed to pb_name.
 * Optional fields that are not set will be set to undefined.
 * To access a reserved field use, foo.pb_<name>, eg, foo.pb_default.
 * For the list of reserved names please see:
 *     net/proto2/compiler/js/internal/generator.cc#kKeyword.
 * @param {boolean=} opt_includeInstance Deprecated. whether to include the
 *     JSPB instance for transitional soy proto support:
 *     http://goto/soy-param-migration
 * @return {!Object}
 */
proto.g8e.operator.v1.SetTargetContextResult.prototype.toObject = function(opt_includeInstance) {
  return proto.g8e.operator.v1.SetTargetContextResult.toObject(opt_includeInstance, this);
};


/**
 * Static version of the {@see toObject} method.
 * @param {boolean|undefined} includeInstance Deprecated. Whether to include
 *     the JSPB instance for transitional soy proto support:
 *     http://goto/soy-param-migration
 * @param {!proto.g8e.operator.v1.SetTargetContextResult} msg The msg instance to transform.
 * @return {!Object}
 * @suppress {unusedLocalVariables} f is only used for nested messages
 */
proto.g8e.operator.v1.SetTargetContextResult.toObject = function(includeInstance, msg) {
  var f, obj = {
    success: jspb.Message.getBooleanFieldWithDefault(msg, 1, false),
    operatorId: jspb.Message.getFieldWithDefault(msg, 2, ""),
    error: jspb.Message.getFieldWithDefault(msg, 3, "")
  };

  if (includeInstance) {
    obj.$jspbMessageInstance = msg;
  }
  return obj;
};
}


/**
 * Deserializes binary data (in protobuf wire format).
 * @param {jspb.ByteSource} bytes The bytes to deserialize.
 * @return {!proto.g8e.operator.v1.SetTargetContextResult}
 */
proto.g8e.operator.v1.SetTargetContextResult.deserializeBinary = function(bytes) {
  var reader = new jspb.BinaryReader(bytes);
  var msg = new proto.g8e.operator.v1.SetTargetContextResult;
  return proto.g8e.operator.v1.SetTargetContextResult.deserializeBinaryFromReader(msg, reader);
};


/**
 * Deserializes binary data (in protobuf wire format) from the
 * given reader into the given message object.
 * @param {!proto.g8e.operator.v1.SetTargetContextResult} msg The message object to deserialize into.
 * @param {!jspb.BinaryReader} reader The BinaryReader to use.
 * @return {!proto.g8e.operator.v1.SetTargetContextResult}
 */
proto.g8e.operator.v1.SetTargetContextResult.deserializeBinaryFromReader = function(msg, reader) {
  while (reader.nextField()) {
    if (reader.isEndGroup()) {
      break;
    }
    var field = reader.getFieldNumber();
    switch (field) {
    case 1:
      var value = /** @type {boolean} */ (reader.readBool());
      msg.setSuccess(value);
      break;
    case 2:
      var value = /** @type {string} */ (reader.readString());
      msg.setOperatorId(value);
      break;
    case 3:
      var value = /** @type {string} */ (reader.readString());
      msg.setError(value);
      break;
    default:
      reader.skipField();
      break;
    }
  }
  return msg;
};


/**
 * Serializes the message to binary data (in protobuf wire format).
 * @return {!Uint8Array}
 */
proto.g8e.operator.v1.SetTargetContextResult.prototype.serializeBinary = function() {
  var writer = new jspb.BinaryWriter();
  proto.g8e.operator.v1.SetTargetContextResult.serializeBinaryToWriter(this, writer);
  return writer.getResultBuffer();
};


/**
 * Serializes the given message to binary data (in protobuf wire
 * format), writing to the given BinaryWriter.
 * @param {!proto.g8e.operator.v1.SetTargetContextResult} message
 * @param {!jspb.BinaryWriter} writer
 * @suppress {unusedLocalVariables} f is only used for nested messages
 */
proto.g8e.operator.v1.SetTargetContextResult.serializeBinaryToWriter = function(message, writer) {
  var f = undefined;
  f = message.getSuccess();
  if (f) {
    writer.writeBool(
      1,
      f
    );
  }
  f = message.getOperatorId();
  if (f.length > 0) {
    writer.writeString(
      2,
      f
    );
  }
  f = message.getError();
  if (f.length > 0) {
    writer.writeString(
      3,
      f
    );
  }
};


/**
 * optional bool success = 1;
 * @return {boolean}
 */
proto.g8e.operator.v1.SetTargetContextResult.prototype.getSuccess = function() {
  return /** @type {boolean} */ (jspb.Message.getBooleanFieldWithDefault(this, 1, false));
};


/**
 * @param {boolean} value
 * @return {!proto.g8e.operator.v1.SetTargetContextResult} returns this
 */
proto.g8e.operator.v1.SetTargetContextResult.prototype.setSuccess = function(value) {
  return jspb.Message.setProto3BooleanField(this, 1, value);
};


/**
 * optional string operator_id = 2;
 * @return {string}
 */
proto.g8e.operator.v1.SetTargetContextResult.prototype.getOperatorId = function() {
  return /** @type {string} */ (jspb.Message.getFieldWithDefault(this, 2, ""));
};


/**
 * @param {string} value
 * @return {!proto.g8e.operator.v1.SetTargetContextResult} returns this
 */
proto.g8e.operator.v1.SetTargetContextResult.prototype.setOperatorId = function(value) {
  return jspb.Message.setProto3StringField(this, 2, value);
};


/**
 * optional string error = 3;
 * @return {string}
 */
proto.g8e.operator.v1.SetTargetContextResult.prototype.getError = function() {
  return /** @type {string} */ (jspb.Message.getFieldWithDefault(this, 3, ""));
};


/**
 * @param {string} value
 * @return {!proto.g8e.operator.v1.SetTargetContextResult} returns this
 */
proto.g8e.operator.v1.SetTargetContextResult.prototype.setError = function(value) {
  return jspb.Message.setProto3StringField(this, 3, value);
};





if (jspb.Message.GENERATE_TO_OBJECT) {
/**
 * Creates an object representation of this proto.
 * Field names that are reserved in JavaScript and will be renamed to pb_name.
 * Optional fields that are not set will be set to undefined.
 * To access a reserved field use, foo.pb_<name>, eg, foo.pb_default.
 * For the list of reserved names please see:
 *     net/proto2/compiler/js/internal/generator.cc#kKeyword.
 * @param {boolean=} opt_includeInstance Deprecated. whether to include the
 *     JSPB instance for transitional soy proto support:
 *     http://goto/soy-param-migration
 * @return {!Object}
 */
proto.g8e.operator.v1.OperatorDocument.prototype.toObject = function(opt_includeInstance) {
  return proto.g8e.operator.v1.OperatorDocument.toObject(opt_includeInstance, this);
};


/**
 * Static version of the {@see toObject} method.
 * @param {boolean|undefined} includeInstance Deprecated. Whether to include
 *     the JSPB instance for transitional soy proto support:
 *     http://goto/soy-param-migration
 * @param {!proto.g8e.operator.v1.OperatorDocument} msg The msg instance to transform.
 * @return {!Object}
 * @suppress {unusedLocalVariables} f is only used for nested messages
 */
proto.g8e.operator.v1.OperatorDocument.toObject = function(includeInstance, msg) {
  var f, obj = {
    id: jspb.Message.getFieldWithDefault(msg, 1, ""),
    userId: jspb.Message.getFieldWithDefault(msg, 2, ""),
    organizationId: jspb.Message.getFieldWithDefault(msg, 3, ""),
    component: jspb.Message.getFieldWithDefault(msg, 4, ""),
    name: jspb.Message.getFieldWithDefault(msg, 5, ""),
    status: jspb.Message.getFieldWithDefault(msg, 6, ""),
    operatorSessionId: jspb.Message.getFieldWithDefault(msg, 7, ""),
    boundWebSessionId: jspb.Message.getFieldWithDefault(msg, 8, ""),
    apiKey: jspb.Message.getFieldWithDefault(msg, 9, ""),
    operatorApiKey: jspb.Message.getFieldWithDefault(msg, 10, ""),
    operatorCert: jspb.Message.getFieldWithDefault(msg, 11, ""),
    operatorCertSerial: jspb.Message.getFieldWithDefault(msg, 12, ""),
    slotNumber: jspb.Message.getFieldWithDefault(msg, 13, 0),
    isSlot: jspb.Message.getBooleanFieldWithDefault(msg, 14, false),
    claimed: jspb.Message.getBooleanFieldWithDefault(msg, 15, false),
    operatorType: jspb.Message.getFieldWithDefault(msg, 16, ""),
    cloudSubtype: jspb.Message.getFieldWithDefault(msg, 17, ""),
    systemFingerprint: jspb.Message.getFieldWithDefault(msg, 18, ""),
    createdAtUnixMs: jspb.Message.getFieldWithDefault(msg, 19, 0),
    updatedAtUnixMs: jspb.Message.getFieldWithDefault(msg, 20, 0)
  };

  if (includeInstance) {
    obj.$jspbMessageInstance = msg;
  }
  return obj;
};
}


/**
 * Deserializes binary data (in protobuf wire format).
 * @param {jspb.ByteSource} bytes The bytes to deserialize.
 * @return {!proto.g8e.operator.v1.OperatorDocument}
 */
proto.g8e.operator.v1.OperatorDocument.deserializeBinary = function(bytes) {
  var reader = new jspb.BinaryReader(bytes);
  var msg = new proto.g8e.operator.v1.OperatorDocument;
  return proto.g8e.operator.v1.OperatorDocument.deserializeBinaryFromReader(msg, reader);
};


/**
 * Deserializes binary data (in protobuf wire format) from the
 * given reader into the given message object.
 * @param {!proto.g8e.operator.v1.OperatorDocument} msg The message object to deserialize into.
 * @param {!jspb.BinaryReader} reader The BinaryReader to use.
 * @return {!proto.g8e.operator.v1.OperatorDocument}
 */
proto.g8e.operator.v1.OperatorDocument.deserializeBinaryFromReader = function(msg, reader) {
  while (reader.nextField()) {
    if (reader.isEndGroup()) {
      break;
    }
    var field = reader.getFieldNumber();
    switch (field) {
    case 1:
      var value = /** @type {string} */ (reader.readString());
      msg.setId(value);
      break;
    case 2:
      var value = /** @type {string} */ (reader.readString());
      msg.setUserId(value);
      break;
    case 3:
      var value = /** @type {string} */ (reader.readString());
      msg.setOrganizationId(value);
      break;
    case 4:
      var value = /** @type {string} */ (reader.readString());
      msg.setComponent(value);
      break;
    case 5:
      var value = /** @type {string} */ (reader.readString());
      msg.setName(value);
      break;
    case 6:
      var value = /** @type {string} */ (reader.readString());
      msg.setStatus(value);
      break;
    case 7:
      var value = /** @type {string} */ (reader.readString());
      msg.setOperatorSessionId(value);
      break;
    case 8:
      var value = /** @type {string} */ (reader.readString());
      msg.setBoundWebSessionId(value);
      break;
    case 9:
      var value = /** @type {string} */ (reader.readString());
      msg.setApiKey(value);
      break;
    case 10:
      var value = /** @type {string} */ (reader.readString());
      msg.setOperatorApiKey(value);
      break;
    case 11:
      var value = /** @type {string} */ (reader.readString());
      msg.setOperatorCert(value);
      break;
    case 12:
      var value = /** @type {string} */ (reader.readString());
      msg.setOperatorCertSerial(value);
      break;
    case 13:
      var value = /** @type {number} */ (reader.readInt32());
      msg.setSlotNumber(value);
      break;
    case 14:
      var value = /** @type {boolean} */ (reader.readBool());
      msg.setIsSlot(value);
      break;
    case 15:
      var value = /** @type {boolean} */ (reader.readBool());
      msg.setClaimed(value);
      break;
    case 16:
      var value = /** @type {string} */ (reader.readString());
      msg.setOperatorType(value);
      break;
    case 17:
      var value = /** @type {string} */ (reader.readString());
      msg.setCloudSubtype(value);
      break;
    case 18:
      var value = /** @type {string} */ (reader.readString());
      msg.setSystemFingerprint(value);
      break;
    case 19:
      var value = /** @type {number} */ (reader.readInt64());
      msg.setCreatedAtUnixMs(value);
      break;
    case 20:
      var value = /** @type {number} */ (reader.readInt64());
      msg.setUpdatedAtUnixMs(value);
      break;
    default:
      reader.skipField();
      break;
    }
  }
  return msg;
};


/**
 * Serializes the message to binary data (in protobuf wire format).
 * @return {!Uint8Array}
 */
proto.g8e.operator.v1.OperatorDocument.prototype.serializeBinary = function() {
  var writer = new jspb.BinaryWriter();
  proto.g8e.operator.v1.OperatorDocument.serializeBinaryToWriter(this, writer);
  return writer.getResultBuffer();
};


/**
 * Serializes the given message to binary data (in protobuf wire
 * format), writing to the given BinaryWriter.
 * @param {!proto.g8e.operator.v1.OperatorDocument} message
 * @param {!jspb.BinaryWriter} writer
 * @suppress {unusedLocalVariables} f is only used for nested messages
 */
proto.g8e.operator.v1.OperatorDocument.serializeBinaryToWriter = function(message, writer) {
  var f = undefined;
  f = message.getId();
  if (f.length > 0) {
    writer.writeString(
      1,
      f
    );
  }
  f = message.getUserId();
  if (f.length > 0) {
    writer.writeString(
      2,
      f
    );
  }
  f = message.getOrganizationId();
  if (f.length > 0) {
    writer.writeString(
      3,
      f
    );
  }
  f = message.getComponent();
  if (f.length > 0) {
    writer.writeString(
      4,
      f
    );
  }
  f = message.getName();
  if (f.length > 0) {
    writer.writeString(
      5,
      f
    );
  }
  f = message.getStatus();
  if (f.length > 0) {
    writer.writeString(
      6,
      f
    );
  }
  f = message.getOperatorSessionId();
  if (f.length > 0) {
    writer.writeString(
      7,
      f
    );
  }
  f = message.getBoundWebSessionId();
  if (f.length > 0) {
    writer.writeString(
      8,
      f
    );
  }
  f = message.getApiKey();
  if (f.length > 0) {
    writer.writeString(
      9,
      f
    );
  }
  f = message.getOperatorApiKey();
  if (f.length > 0) {
    writer.writeString(
      10,
      f
    );
  }
  f = message.getOperatorCert();
  if (f.length > 0) {
    writer.writeString(
      11,
      f
    );
  }
  f = message.getOperatorCertSerial();
  if (f.length > 0) {
    writer.writeString(
      12,
      f
    );
  }
  f = message.getSlotNumber();
  if (f !== 0) {
    writer.writeInt32(
      13,
      f
    );
  }
  f = message.getIsSlot();
  if (f) {
    writer.writeBool(
      14,
      f
    );
  }
  f = message.getClaimed();
  if (f) {
    writer.writeBool(
      15,
      f
    );
  }
  f = message.getOperatorType();
  if (f.length > 0) {
    writer.writeString(
      16,
      f
    );
  }
  f = message.getCloudSubtype();
  if (f.length > 0) {
    writer.writeString(
      17,
      f
    );
  }
  f = message.getSystemFingerprint();
  if (f.length > 0) {
    writer.writeString(
      18,
      f
    );
  }
  f = message.getCreatedAtUnixMs();
  if (f !== 0) {
    writer.writeInt64(
      19,
      f
    );
  }
  f = message.getUpdatedAtUnixMs();
  if (f !== 0) {
    writer.writeInt64(
      20,
      f
    );
  }
};


/**
 * optional string id = 1;
 * @return {string}
 */
proto.g8e.operator.v1.OperatorDocument.prototype.getId = function() {
  return /** @type {string} */ (jspb.Message.getFieldWithDefault(this, 1, ""));
};


/**
 * @param {string} value
 * @return {!proto.g8e.operator.v1.OperatorDocument} returns this
 */
proto.g8e.operator.v1.OperatorDocument.prototype.setId = function(value) {
  return jspb.Message.setProto3StringField(this, 1, value);
};


/**
 * optional string user_id = 2;
 * @return {string}
 */
proto.g8e.operator.v1.OperatorDocument.prototype.getUserId = function() {
  return /** @type {string} */ (jspb.Message.getFieldWithDefault(this, 2, ""));
};


/**
 * @param {string} value
 * @return {!proto.g8e.operator.v1.OperatorDocument} returns this
 */
proto.g8e.operator.v1.OperatorDocument.prototype.setUserId = function(value) {
  return jspb.Message.setProto3StringField(this, 2, value);
};


/**
 * optional string organization_id = 3;
 * @return {string}
 */
proto.g8e.operator.v1.OperatorDocument.prototype.getOrganizationId = function() {
  return /** @type {string} */ (jspb.Message.getFieldWithDefault(this, 3, ""));
};


/**
 * @param {string} value
 * @return {!proto.g8e.operator.v1.OperatorDocument} returns this
 */
proto.g8e.operator.v1.OperatorDocument.prototype.setOrganizationId = function(value) {
  return jspb.Message.setProto3StringField(this, 3, value);
};


/**
 * optional string component = 4;
 * @return {string}
 */
proto.g8e.operator.v1.OperatorDocument.prototype.getComponent = function() {
  return /** @type {string} */ (jspb.Message.getFieldWithDefault(this, 4, ""));
};


/**
 * @param {string} value
 * @return {!proto.g8e.operator.v1.OperatorDocument} returns this
 */
proto.g8e.operator.v1.OperatorDocument.prototype.setComponent = function(value) {
  return jspb.Message.setProto3StringField(this, 4, value);
};


/**
 * optional string name = 5;
 * @return {string}
 */
proto.g8e.operator.v1.OperatorDocument.prototype.getName = function() {
  return /** @type {string} */ (jspb.Message.getFieldWithDefault(this, 5, ""));
};


/**
 * @param {string} value
 * @return {!proto.g8e.operator.v1.OperatorDocument} returns this
 */
proto.g8e.operator.v1.OperatorDocument.prototype.setName = function(value) {
  return jspb.Message.setProto3StringField(this, 5, value);
};


/**
 * optional string status = 6;
 * @return {string}
 */
proto.g8e.operator.v1.OperatorDocument.prototype.getStatus = function() {
  return /** @type {string} */ (jspb.Message.getFieldWithDefault(this, 6, ""));
};


/**
 * @param {string} value
 * @return {!proto.g8e.operator.v1.OperatorDocument} returns this
 */
proto.g8e.operator.v1.OperatorDocument.prototype.setStatus = function(value) {
  return jspb.Message.setProto3StringField(this, 6, value);
};


/**
 * optional string operator_session_id = 7;
 * @return {string}
 */
proto.g8e.operator.v1.OperatorDocument.prototype.getOperatorSessionId = function() {
  return /** @type {string} */ (jspb.Message.getFieldWithDefault(this, 7, ""));
};


/**
 * @param {string} value
 * @return {!proto.g8e.operator.v1.OperatorDocument} returns this
 */
proto.g8e.operator.v1.OperatorDocument.prototype.setOperatorSessionId = function(value) {
  return jspb.Message.setProto3StringField(this, 7, value);
};


/**
 * optional string bound_web_session_id = 8;
 * @return {string}
 */
proto.g8e.operator.v1.OperatorDocument.prototype.getBoundWebSessionId = function() {
  return /** @type {string} */ (jspb.Message.getFieldWithDefault(this, 8, ""));
};


/**
 * @param {string} value
 * @return {!proto.g8e.operator.v1.OperatorDocument} returns this
 */
proto.g8e.operator.v1.OperatorDocument.prototype.setBoundWebSessionId = function(value) {
  return jspb.Message.setProto3StringField(this, 8, value);
};


/**
 * optional string api_key = 9;
 * @return {string}
 */
proto.g8e.operator.v1.OperatorDocument.prototype.getApiKey = function() {
  return /** @type {string} */ (jspb.Message.getFieldWithDefault(this, 9, ""));
};


/**
 * @param {string} value
 * @return {!proto.g8e.operator.v1.OperatorDocument} returns this
 */
proto.g8e.operator.v1.OperatorDocument.prototype.setApiKey = function(value) {
  return jspb.Message.setProto3StringField(this, 9, value);
};


/**
 * optional string operator_api_key = 10;
 * @return {string}
 */
proto.g8e.operator.v1.OperatorDocument.prototype.getOperatorApiKey = function() {
  return /** @type {string} */ (jspb.Message.getFieldWithDefault(this, 10, ""));
};


/**
 * @param {string} value
 * @return {!proto.g8e.operator.v1.OperatorDocument} returns this
 */
proto.g8e.operator.v1.OperatorDocument.prototype.setOperatorApiKey = function(value) {
  return jspb.Message.setProto3StringField(this, 10, value);
};


/**
 * optional string operator_cert = 11;
 * @return {string}
 */
proto.g8e.operator.v1.OperatorDocument.prototype.getOperatorCert = function() {
  return /** @type {string} */ (jspb.Message.getFieldWithDefault(this, 11, ""));
};


/**
 * @param {string} value
 * @return {!proto.g8e.operator.v1.OperatorDocument} returns this
 */
proto.g8e.operator.v1.OperatorDocument.prototype.setOperatorCert = function(value) {
  return jspb.Message.setProto3StringField(this, 11, value);
};


/**
 * optional string operator_cert_serial = 12;
 * @return {string}
 */
proto.g8e.operator.v1.OperatorDocument.prototype.getOperatorCertSerial = function() {
  return /** @type {string} */ (jspb.Message.getFieldWithDefault(this, 12, ""));
};


/**
 * @param {string} value
 * @return {!proto.g8e.operator.v1.OperatorDocument} returns this
 */
proto.g8e.operator.v1.OperatorDocument.prototype.setOperatorCertSerial = function(value) {
  return jspb.Message.setProto3StringField(this, 12, value);
};


/**
 * optional int32 slot_number = 13;
 * @return {number}
 */
proto.g8e.operator.v1.OperatorDocument.prototype.getSlotNumber = function() {
  return /** @type {number} */ (jspb.Message.getFieldWithDefault(this, 13, 0));
};


/**
 * @param {number} value
 * @return {!proto.g8e.operator.v1.OperatorDocument} returns this
 */
proto.g8e.operator.v1.OperatorDocument.prototype.setSlotNumber = function(value) {
  return jspb.Message.setProto3IntField(this, 13, value);
};


/**
 * optional bool is_slot = 14;
 * @return {boolean}
 */
proto.g8e.operator.v1.OperatorDocument.prototype.getIsSlot = function() {
  return /** @type {boolean} */ (jspb.Message.getBooleanFieldWithDefault(this, 14, false));
};


/**
 * @param {boolean} value
 * @return {!proto.g8e.operator.v1.OperatorDocument} returns this
 */
proto.g8e.operator.v1.OperatorDocument.prototype.setIsSlot = function(value) {
  return jspb.Message.setProto3BooleanField(this, 14, value);
};


/**
 * optional bool claimed = 15;
 * @return {boolean}
 */
proto.g8e.operator.v1.OperatorDocument.prototype.getClaimed = function() {
  return /** @type {boolean} */ (jspb.Message.getBooleanFieldWithDefault(this, 15, false));
};


/**
 * @param {boolean} value
 * @return {!proto.g8e.operator.v1.OperatorDocument} returns this
 */
proto.g8e.operator.v1.OperatorDocument.prototype.setClaimed = function(value) {
  return jspb.Message.setProto3BooleanField(this, 15, value);
};


/**
 * optional string operator_type = 16;
 * @return {string}
 */
proto.g8e.operator.v1.OperatorDocument.prototype.getOperatorType = function() {
  return /** @type {string} */ (jspb.Message.getFieldWithDefault(this, 16, ""));
};


/**
 * @param {string} value
 * @return {!proto.g8e.operator.v1.OperatorDocument} returns this
 */
proto.g8e.operator.v1.OperatorDocument.prototype.setOperatorType = function(value) {
  return jspb.Message.setProto3StringField(this, 16, value);
};


/**
 * optional string cloud_subtype = 17;
 * @return {string}
 */
proto.g8e.operator.v1.OperatorDocument.prototype.getCloudSubtype = function() {
  return /** @type {string} */ (jspb.Message.getFieldWithDefault(this, 17, ""));
};


/**
 * @param {string} value
 * @return {!proto.g8e.operator.v1.OperatorDocument} returns this
 */
proto.g8e.operator.v1.OperatorDocument.prototype.setCloudSubtype = function(value) {
  return jspb.Message.setProto3StringField(this, 17, value);
};


/**
 * optional string system_fingerprint = 18;
 * @return {string}
 */
proto.g8e.operator.v1.OperatorDocument.prototype.getSystemFingerprint = function() {
  return /** @type {string} */ (jspb.Message.getFieldWithDefault(this, 18, ""));
};


/**
 * @param {string} value
 * @return {!proto.g8e.operator.v1.OperatorDocument} returns this
 */
proto.g8e.operator.v1.OperatorDocument.prototype.setSystemFingerprint = function(value) {
  return jspb.Message.setProto3StringField(this, 18, value);
};


/**
 * optional int64 created_at_unix_ms = 19;
 * @return {number}
 */
proto.g8e.operator.v1.OperatorDocument.prototype.getCreatedAtUnixMs = function() {
  return /** @type {number} */ (jspb.Message.getFieldWithDefault(this, 19, 0));
};


/**
 * @param {number} value
 * @return {!proto.g8e.operator.v1.OperatorDocument} returns this
 */
proto.g8e.operator.v1.OperatorDocument.prototype.setCreatedAtUnixMs = function(value) {
  return jspb.Message.setProto3IntField(this, 19, value);
};


/**
 * optional int64 updated_at_unix_ms = 20;
 * @return {number}
 */
proto.g8e.operator.v1.OperatorDocument.prototype.getUpdatedAtUnixMs = function() {
  return /** @type {number} */ (jspb.Message.getFieldWithDefault(this, 20, 0));
};


/**
 * @param {number} value
 * @return {!proto.g8e.operator.v1.OperatorDocument} returns this
 */
proto.g8e.operator.v1.OperatorDocument.prototype.setUpdatedAtUnixMs = function(value) {
  return jspb.Message.setProto3IntField(this, 20, value);
};





if (jspb.Message.GENERATE_TO_OBJECT) {
/**
 * Creates an object representation of this proto.
 * Field names that are reserved in JavaScript and will be renamed to pb_name.
 * Optional fields that are not set will be set to undefined.
 * To access a reserved field use, foo.pb_<name>, eg, foo.pb_default.
 * For the list of reserved names please see:
 *     net/proto2/compiler/js/internal/generator.cc#kKeyword.
 * @param {boolean=} opt_includeInstance Deprecated. whether to include the
 *     JSPB instance for transitional soy proto support:
 *     http://goto/soy-param-migration
 * @return {!Object}
 */
proto.g8e.operator.v1.ShutdownRequested.prototype.toObject = function(opt_includeInstance) {
  return proto.g8e.operator.v1.ShutdownRequested.toObject(opt_includeInstance, this);
};


/**
 * Static version of the {@see toObject} method.
 * @param {boolean|undefined} includeInstance Deprecated. Whether to include
 *     the JSPB instance for transitional soy proto support:
 *     http://goto/soy-param-migration
 * @param {!proto.g8e.operator.v1.ShutdownRequested} msg The msg instance to transform.
 * @return {!Object}
 * @suppress {unusedLocalVariables} f is only used for nested messages
 */
proto.g8e.operator.v1.ShutdownRequested.toObject = function(includeInstance, msg) {
  var f, obj = {
    reason: jspb.Message.getFieldWithDefault(msg, 1, "")
  };

  if (includeInstance) {
    obj.$jspbMessageInstance = msg;
  }
  return obj;
};
}


/**
 * Deserializes binary data (in protobuf wire format).
 * @param {jspb.ByteSource} bytes The bytes to deserialize.
 * @return {!proto.g8e.operator.v1.ShutdownRequested}
 */
proto.g8e.operator.v1.ShutdownRequested.deserializeBinary = function(bytes) {
  var reader = new jspb.BinaryReader(bytes);
  var msg = new proto.g8e.operator.v1.ShutdownRequested;
  return proto.g8e.operator.v1.ShutdownRequested.deserializeBinaryFromReader(msg, reader);
};


/**
 * Deserializes binary data (in protobuf wire format) from the
 * given reader into the given message object.
 * @param {!proto.g8e.operator.v1.ShutdownRequested} msg The message object to deserialize into.
 * @param {!jspb.BinaryReader} reader The BinaryReader to use.
 * @return {!proto.g8e.operator.v1.ShutdownRequested}
 */
proto.g8e.operator.v1.ShutdownRequested.deserializeBinaryFromReader = function(msg, reader) {
  while (reader.nextField()) {
    if (reader.isEndGroup()) {
      break;
    }
    var field = reader.getFieldNumber();
    switch (field) {
    case 1:
      var value = /** @type {string} */ (reader.readString());
      msg.setReason(value);
      break;
    default:
      reader.skipField();
      break;
    }
  }
  return msg;
};


/**
 * Serializes the message to binary data (in protobuf wire format).
 * @return {!Uint8Array}
 */
proto.g8e.operator.v1.ShutdownRequested.prototype.serializeBinary = function() {
  var writer = new jspb.BinaryWriter();
  proto.g8e.operator.v1.ShutdownRequested.serializeBinaryToWriter(this, writer);
  return writer.getResultBuffer();
};


/**
 * Serializes the given message to binary data (in protobuf wire
 * format), writing to the given BinaryWriter.
 * @param {!proto.g8e.operator.v1.ShutdownRequested} message
 * @param {!jspb.BinaryWriter} writer
 * @suppress {unusedLocalVariables} f is only used for nested messages
 */
proto.g8e.operator.v1.ShutdownRequested.serializeBinaryToWriter = function(message, writer) {
  var f = undefined;
  f = message.getReason();
  if (f.length > 0) {
    writer.writeString(
      1,
      f
    );
  }
};


/**
 * optional string reason = 1;
 * @return {string}
 */
proto.g8e.operator.v1.ShutdownRequested.prototype.getReason = function() {
  return /** @type {string} */ (jspb.Message.getFieldWithDefault(this, 1, ""));
};


/**
 * @param {string} value
 * @return {!proto.g8e.operator.v1.ShutdownRequested} returns this
 */
proto.g8e.operator.v1.ShutdownRequested.prototype.setReason = function(value) {
  return jspb.Message.setProto3StringField(this, 1, value);
};





if (jspb.Message.GENERATE_TO_OBJECT) {
/**
 * Creates an object representation of this proto.
 * Field names that are reserved in JavaScript and will be renamed to pb_name.
 * Optional fields that are not set will be set to undefined.
 * To access a reserved field use, foo.pb_<name>, eg, foo.pb_default.
 * For the list of reserved names please see:
 *     net/proto2/compiler/js/internal/generator.cc#kKeyword.
 * @param {boolean=} opt_includeInstance Deprecated. whether to include the
 *     JSPB instance for transitional soy proto support:
 *     http://goto/soy-param-migration
 * @return {!Object}
 */
proto.g8e.operator.v1.CommandResult.prototype.toObject = function(opt_includeInstance) {
  return proto.g8e.operator.v1.CommandResult.toObject(opt_includeInstance, this);
};


/**
 * Static version of the {@see toObject} method.
 * @param {boolean|undefined} includeInstance Deprecated. Whether to include
 *     the JSPB instance for transitional soy proto support:
 *     http://goto/soy-param-migration
 * @param {!proto.g8e.operator.v1.CommandResult} msg The msg instance to transform.
 * @return {!Object}
 * @suppress {unusedLocalVariables} f is only used for nested messages
 */
proto.g8e.operator.v1.CommandResult.toObject = function(includeInstance, msg) {
  var f, obj = {
    executionId: jspb.Message.getFieldWithDefault(msg, 1, ""),
    status: jspb.Message.getFieldWithDefault(msg, 2, 0),
    output: jspb.Message.getFieldWithDefault(msg, 3, ""),
    error: jspb.Message.getFieldWithDefault(msg, 4, ""),
    stderr: jspb.Message.getFieldWithDefault(msg, 5, ""),
    exitCode: jspb.Message.getFieldWithDefault(msg, 6, 0),
    executionTimeSeconds: jspb.Message.getFloatingPointFieldWithDefault(msg, 7, 0.0),
    startTimeUnixMs: jspb.Message.getFieldWithDefault(msg, 8, 0),
    endTimeUnixMs: jspb.Message.getFieldWithDefault(msg, 9, 0)
  };

  if (includeInstance) {
    obj.$jspbMessageInstance = msg;
  }
  return obj;
};
}


/**
 * Deserializes binary data (in protobuf wire format).
 * @param {jspb.ByteSource} bytes The bytes to deserialize.
 * @return {!proto.g8e.operator.v1.CommandResult}
 */
proto.g8e.operator.v1.CommandResult.deserializeBinary = function(bytes) {
  var reader = new jspb.BinaryReader(bytes);
  var msg = new proto.g8e.operator.v1.CommandResult;
  return proto.g8e.operator.v1.CommandResult.deserializeBinaryFromReader(msg, reader);
};


/**
 * Deserializes binary data (in protobuf wire format) from the
 * given reader into the given message object.
 * @param {!proto.g8e.operator.v1.CommandResult} msg The message object to deserialize into.
 * @param {!jspb.BinaryReader} reader The BinaryReader to use.
 * @return {!proto.g8e.operator.v1.CommandResult}
 */
proto.g8e.operator.v1.CommandResult.deserializeBinaryFromReader = function(msg, reader) {
  while (reader.nextField()) {
    if (reader.isEndGroup()) {
      break;
    }
    var field = reader.getFieldNumber();
    switch (field) {
    case 1:
      var value = /** @type {string} */ (reader.readString());
      msg.setExecutionId(value);
      break;
    case 2:
      var value = /** @type {!proto.g8e.operator.v1.ExecutionStatus} */ (reader.readEnum());
      msg.setStatus(value);
      break;
    case 3:
      var value = /** @type {string} */ (reader.readString());
      msg.setOutput(value);
      break;
    case 4:
      var value = /** @type {string} */ (reader.readString());
      msg.setError(value);
      break;
    case 5:
      var value = /** @type {string} */ (reader.readString());
      msg.setStderr(value);
      break;
    case 6:
      var value = /** @type {number} */ (reader.readInt32());
      msg.setExitCode(value);
      break;
    case 7:
      var value = /** @type {number} */ (reader.readFloat());
      msg.setExecutionTimeSeconds(value);
      break;
    case 8:
      var value = /** @type {number} */ (reader.readInt64());
      msg.setStartTimeUnixMs(value);
      break;
    case 9:
      var value = /** @type {number} */ (reader.readInt64());
      msg.setEndTimeUnixMs(value);
      break;
    default:
      reader.skipField();
      break;
    }
  }
  return msg;
};


/**
 * Serializes the message to binary data (in protobuf wire format).
 * @return {!Uint8Array}
 */
proto.g8e.operator.v1.CommandResult.prototype.serializeBinary = function() {
  var writer = new jspb.BinaryWriter();
  proto.g8e.operator.v1.CommandResult.serializeBinaryToWriter(this, writer);
  return writer.getResultBuffer();
};


/**
 * Serializes the given message to binary data (in protobuf wire
 * format), writing to the given BinaryWriter.
 * @param {!proto.g8e.operator.v1.CommandResult} message
 * @param {!jspb.BinaryWriter} writer
 * @suppress {unusedLocalVariables} f is only used for nested messages
 */
proto.g8e.operator.v1.CommandResult.serializeBinaryToWriter = function(message, writer) {
  var f = undefined;
  f = message.getExecutionId();
  if (f.length > 0) {
    writer.writeString(
      1,
      f
    );
  }
  f = message.getStatus();
  if (f !== 0.0) {
    writer.writeEnum(
      2,
      f
    );
  }
  f = message.getOutput();
  if (f.length > 0) {
    writer.writeString(
      3,
      f
    );
  }
  f = message.getError();
  if (f.length > 0) {
    writer.writeString(
      4,
      f
    );
  }
  f = message.getStderr();
  if (f.length > 0) {
    writer.writeString(
      5,
      f
    );
  }
  f = message.getExitCode();
  if (f !== 0) {
    writer.writeInt32(
      6,
      f
    );
  }
  f = message.getExecutionTimeSeconds();
  if (f !== 0.0) {
    writer.writeFloat(
      7,
      f
    );
  }
  f = message.getStartTimeUnixMs();
  if (f !== 0) {
    writer.writeInt64(
      8,
      f
    );
  }
  f = message.getEndTimeUnixMs();
  if (f !== 0) {
    writer.writeInt64(
      9,
      f
    );
  }
};


/**
 * optional string execution_id = 1;
 * @return {string}
 */
proto.g8e.operator.v1.CommandResult.prototype.getExecutionId = function() {
  return /** @type {string} */ (jspb.Message.getFieldWithDefault(this, 1, ""));
};


/**
 * @param {string} value
 * @return {!proto.g8e.operator.v1.CommandResult} returns this
 */
proto.g8e.operator.v1.CommandResult.prototype.setExecutionId = function(value) {
  return jspb.Message.setProto3StringField(this, 1, value);
};


/**
 * optional ExecutionStatus status = 2;
 * @return {!proto.g8e.operator.v1.ExecutionStatus}
 */
proto.g8e.operator.v1.CommandResult.prototype.getStatus = function() {
  return /** @type {!proto.g8e.operator.v1.ExecutionStatus} */ (jspb.Message.getFieldWithDefault(this, 2, 0));
};


/**
 * @param {!proto.g8e.operator.v1.ExecutionStatus} value
 * @return {!proto.g8e.operator.v1.CommandResult} returns this
 */
proto.g8e.operator.v1.CommandResult.prototype.setStatus = function(value) {
  return jspb.Message.setProto3EnumField(this, 2, value);
};


/**
 * optional string output = 3;
 * @return {string}
 */
proto.g8e.operator.v1.CommandResult.prototype.getOutput = function() {
  return /** @type {string} */ (jspb.Message.getFieldWithDefault(this, 3, ""));
};


/**
 * @param {string} value
 * @return {!proto.g8e.operator.v1.CommandResult} returns this
 */
proto.g8e.operator.v1.CommandResult.prototype.setOutput = function(value) {
  return jspb.Message.setProto3StringField(this, 3, value);
};


/**
 * optional string error = 4;
 * @return {string}
 */
proto.g8e.operator.v1.CommandResult.prototype.getError = function() {
  return /** @type {string} */ (jspb.Message.getFieldWithDefault(this, 4, ""));
};


/**
 * @param {string} value
 * @return {!proto.g8e.operator.v1.CommandResult} returns this
 */
proto.g8e.operator.v1.CommandResult.prototype.setError = function(value) {
  return jspb.Message.setProto3StringField(this, 4, value);
};


/**
 * optional string stderr = 5;
 * @return {string}
 */
proto.g8e.operator.v1.CommandResult.prototype.getStderr = function() {
  return /** @type {string} */ (jspb.Message.getFieldWithDefault(this, 5, ""));
};


/**
 * @param {string} value
 * @return {!proto.g8e.operator.v1.CommandResult} returns this
 */
proto.g8e.operator.v1.CommandResult.prototype.setStderr = function(value) {
  return jspb.Message.setProto3StringField(this, 5, value);
};


/**
 * optional int32 exit_code = 6;
 * @return {number}
 */
proto.g8e.operator.v1.CommandResult.prototype.getExitCode = function() {
  return /** @type {number} */ (jspb.Message.getFieldWithDefault(this, 6, 0));
};


/**
 * @param {number} value
 * @return {!proto.g8e.operator.v1.CommandResult} returns this
 */
proto.g8e.operator.v1.CommandResult.prototype.setExitCode = function(value) {
  return jspb.Message.setProto3IntField(this, 6, value);
};


/**
 * optional float execution_time_seconds = 7;
 * @return {number}
 */
proto.g8e.operator.v1.CommandResult.prototype.getExecutionTimeSeconds = function() {
  return /** @type {number} */ (jspb.Message.getFloatingPointFieldWithDefault(this, 7, 0.0));
};


/**
 * @param {number} value
 * @return {!proto.g8e.operator.v1.CommandResult} returns this
 */
proto.g8e.operator.v1.CommandResult.prototype.setExecutionTimeSeconds = function(value) {
  return jspb.Message.setProto3FloatField(this, 7, value);
};


/**
 * optional int64 start_time_unix_ms = 8;
 * @return {number}
 */
proto.g8e.operator.v1.CommandResult.prototype.getStartTimeUnixMs = function() {
  return /** @type {number} */ (jspb.Message.getFieldWithDefault(this, 8, 0));
};


/**
 * @param {number} value
 * @return {!proto.g8e.operator.v1.CommandResult} returns this
 */
proto.g8e.operator.v1.CommandResult.prototype.setStartTimeUnixMs = function(value) {
  return jspb.Message.setProto3IntField(this, 8, value);
};


/**
 * optional int64 end_time_unix_ms = 9;
 * @return {number}
 */
proto.g8e.operator.v1.CommandResult.prototype.getEndTimeUnixMs = function() {
  return /** @type {number} */ (jspb.Message.getFieldWithDefault(this, 9, 0));
};


/**
 * @param {number} value
 * @return {!proto.g8e.operator.v1.CommandResult} returns this
 */
proto.g8e.operator.v1.CommandResult.prototype.setEndTimeUnixMs = function(value) {
  return jspb.Message.setProto3IntField(this, 9, value);
};





if (jspb.Message.GENERATE_TO_OBJECT) {
/**
 * Creates an object representation of this proto.
 * Field names that are reserved in JavaScript and will be renamed to pb_name.
 * Optional fields that are not set will be set to undefined.
 * To access a reserved field use, foo.pb_<name>, eg, foo.pb_default.
 * For the list of reserved names please see:
 *     net/proto2/compiler/js/internal/generator.cc#kKeyword.
 * @param {boolean=} opt_includeInstance Deprecated. whether to include the
 *     JSPB instance for transitional soy proto support:
 *     http://goto/soy-param-migration
 * @return {!Object}
 */
proto.g8e.operator.v1.FsEntry.prototype.toObject = function(opt_includeInstance) {
  return proto.g8e.operator.v1.FsEntry.toObject(opt_includeInstance, this);
};


/**
 * Static version of the {@see toObject} method.
 * @param {boolean|undefined} includeInstance Deprecated. Whether to include
 *     the JSPB instance for transitional soy proto support:
 *     http://goto/soy-param-migration
 * @param {!proto.g8e.operator.v1.FsEntry} msg The msg instance to transform.
 * @return {!Object}
 * @suppress {unusedLocalVariables} f is only used for nested messages
 */
proto.g8e.operator.v1.FsEntry.toObject = function(includeInstance, msg) {
  var f, obj = {
    name: jspb.Message.getFieldWithDefault(msg, 1, ""),
    isDir: jspb.Message.getBooleanFieldWithDefault(msg, 2, false),
    size: jspb.Message.getFieldWithDefault(msg, 3, 0),
    mode: jspb.Message.getFieldWithDefault(msg, 4, 0),
    modTime: jspb.Message.getFieldWithDefault(msg, 5, 0)
  };

  if (includeInstance) {
    obj.$jspbMessageInstance = msg;
  }
  return obj;
};
}


/**
 * Deserializes binary data (in protobuf wire format).
 * @param {jspb.ByteSource} bytes The bytes to deserialize.
 * @return {!proto.g8e.operator.v1.FsEntry}
 */
proto.g8e.operator.v1.FsEntry.deserializeBinary = function(bytes) {
  var reader = new jspb.BinaryReader(bytes);
  var msg = new proto.g8e.operator.v1.FsEntry;
  return proto.g8e.operator.v1.FsEntry.deserializeBinaryFromReader(msg, reader);
};


/**
 * Deserializes binary data (in protobuf wire format) from the
 * given reader into the given message object.
 * @param {!proto.g8e.operator.v1.FsEntry} msg The message object to deserialize into.
 * @param {!jspb.BinaryReader} reader The BinaryReader to use.
 * @return {!proto.g8e.operator.v1.FsEntry}
 */
proto.g8e.operator.v1.FsEntry.deserializeBinaryFromReader = function(msg, reader) {
  while (reader.nextField()) {
    if (reader.isEndGroup()) {
      break;
    }
    var field = reader.getFieldNumber();
    switch (field) {
    case 1:
      var value = /** @type {string} */ (reader.readString());
      msg.setName(value);
      break;
    case 2:
      var value = /** @type {boolean} */ (reader.readBool());
      msg.setIsDir(value);
      break;
    case 3:
      var value = /** @type {number} */ (reader.readInt64());
      msg.setSize(value);
      break;
    case 4:
      var value = /** @type {number} */ (reader.readInt32());
      msg.setMode(value);
      break;
    case 5:
      var value = /** @type {number} */ (reader.readInt64());
      msg.setModTime(value);
      break;
    default:
      reader.skipField();
      break;
    }
  }
  return msg;
};


/**
 * Serializes the message to binary data (in protobuf wire format).
 * @return {!Uint8Array}
 */
proto.g8e.operator.v1.FsEntry.prototype.serializeBinary = function() {
  var writer = new jspb.BinaryWriter();
  proto.g8e.operator.v1.FsEntry.serializeBinaryToWriter(this, writer);
  return writer.getResultBuffer();
};


/**
 * Serializes the given message to binary data (in protobuf wire
 * format), writing to the given BinaryWriter.
 * @param {!proto.g8e.operator.v1.FsEntry} message
 * @param {!jspb.BinaryWriter} writer
 * @suppress {unusedLocalVariables} f is only used for nested messages
 */
proto.g8e.operator.v1.FsEntry.serializeBinaryToWriter = function(message, writer) {
  var f = undefined;
  f = message.getName();
  if (f.length > 0) {
    writer.writeString(
      1,
      f
    );
  }
  f = message.getIsDir();
  if (f) {
    writer.writeBool(
      2,
      f
    );
  }
  f = message.getSize();
  if (f !== 0) {
    writer.writeInt64(
      3,
      f
    );
  }
  f = message.getMode();
  if (f !== 0) {
    writer.writeInt32(
      4,
      f
    );
  }
  f = message.getModTime();
  if (f !== 0) {
    writer.writeInt64(
      5,
      f
    );
  }
};


/**
 * optional string name = 1;
 * @return {string}
 */
proto.g8e.operator.v1.FsEntry.prototype.getName = function() {
  return /** @type {string} */ (jspb.Message.getFieldWithDefault(this, 1, ""));
};


/**
 * @param {string} value
 * @return {!proto.g8e.operator.v1.FsEntry} returns this
 */
proto.g8e.operator.v1.FsEntry.prototype.setName = function(value) {
  return jspb.Message.setProto3StringField(this, 1, value);
};


/**
 * optional bool is_dir = 2;
 * @return {boolean}
 */
proto.g8e.operator.v1.FsEntry.prototype.getIsDir = function() {
  return /** @type {boolean} */ (jspb.Message.getBooleanFieldWithDefault(this, 2, false));
};


/**
 * @param {boolean} value
 * @return {!proto.g8e.operator.v1.FsEntry} returns this
 */
proto.g8e.operator.v1.FsEntry.prototype.setIsDir = function(value) {
  return jspb.Message.setProto3BooleanField(this, 2, value);
};


/**
 * optional int64 size = 3;
 * @return {number}
 */
proto.g8e.operator.v1.FsEntry.prototype.getSize = function() {
  return /** @type {number} */ (jspb.Message.getFieldWithDefault(this, 3, 0));
};


/**
 * @param {number} value
 * @return {!proto.g8e.operator.v1.FsEntry} returns this
 */
proto.g8e.operator.v1.FsEntry.prototype.setSize = function(value) {
  return jspb.Message.setProto3IntField(this, 3, value);
};


/**
 * optional int32 mode = 4;
 * @return {number}
 */
proto.g8e.operator.v1.FsEntry.prototype.getMode = function() {
  return /** @type {number} */ (jspb.Message.getFieldWithDefault(this, 4, 0));
};


/**
 * @param {number} value
 * @return {!proto.g8e.operator.v1.FsEntry} returns this
 */
proto.g8e.operator.v1.FsEntry.prototype.setMode = function(value) {
  return jspb.Message.setProto3IntField(this, 4, value);
};


/**
 * optional int64 mod_time = 5;
 * @return {number}
 */
proto.g8e.operator.v1.FsEntry.prototype.getModTime = function() {
  return /** @type {number} */ (jspb.Message.getFieldWithDefault(this, 5, 0));
};


/**
 * @param {number} value
 * @return {!proto.g8e.operator.v1.FsEntry} returns this
 */
proto.g8e.operator.v1.FsEntry.prototype.setModTime = function(value) {
  return jspb.Message.setProto3IntField(this, 5, value);
};



/**
 * List of repeated fields within this message type.
 * @private {!Array<number>}
 * @const
 */
proto.g8e.operator.v1.FsListResult.repeatedFields_ = [4];



if (jspb.Message.GENERATE_TO_OBJECT) {
/**
 * Creates an object representation of this proto.
 * Field names that are reserved in JavaScript and will be renamed to pb_name.
 * Optional fields that are not set will be set to undefined.
 * To access a reserved field use, foo.pb_<name>, eg, foo.pb_default.
 * For the list of reserved names please see:
 *     net/proto2/compiler/js/internal/generator.cc#kKeyword.
 * @param {boolean=} opt_includeInstance Deprecated. whether to include the
 *     JSPB instance for transitional soy proto support:
 *     http://goto/soy-param-migration
 * @return {!Object}
 */
proto.g8e.operator.v1.FsListResult.prototype.toObject = function(opt_includeInstance) {
  return proto.g8e.operator.v1.FsListResult.toObject(opt_includeInstance, this);
};


/**
 * Static version of the {@see toObject} method.
 * @param {boolean|undefined} includeInstance Deprecated. Whether to include
 *     the JSPB instance for transitional soy proto support:
 *     http://goto/soy-param-migration
 * @param {!proto.g8e.operator.v1.FsListResult} msg The msg instance to transform.
 * @return {!Object}
 * @suppress {unusedLocalVariables} f is only used for nested messages
 */
proto.g8e.operator.v1.FsListResult.toObject = function(includeInstance, msg) {
  var f, obj = {
    executionId: jspb.Message.getFieldWithDefault(msg, 1, ""),
    status: jspb.Message.getFieldWithDefault(msg, 2, 0),
    path: jspb.Message.getFieldWithDefault(msg, 3, ""),
    entriesList: jspb.Message.toObjectList(msg.getEntriesList(),
    proto.g8e.operator.v1.FsEntry.toObject, includeInstance),
    truncated: jspb.Message.getBooleanFieldWithDefault(msg, 5, false),
    totalCount: jspb.Message.getFieldWithDefault(msg, 6, 0),
    durationSeconds: jspb.Message.getFloatingPointFieldWithDefault(msg, 7, 0.0),
    errorMessage: jspb.Message.getFieldWithDefault(msg, 8, ""),
    errorType: jspb.Message.getFieldWithDefault(msg, 9, "")
  };

  if (includeInstance) {
    obj.$jspbMessageInstance = msg;
  }
  return obj;
};
}


/**
 * Deserializes binary data (in protobuf wire format).
 * @param {jspb.ByteSource} bytes The bytes to deserialize.
 * @return {!proto.g8e.operator.v1.FsListResult}
 */
proto.g8e.operator.v1.FsListResult.deserializeBinary = function(bytes) {
  var reader = new jspb.BinaryReader(bytes);
  var msg = new proto.g8e.operator.v1.FsListResult;
  return proto.g8e.operator.v1.FsListResult.deserializeBinaryFromReader(msg, reader);
};


/**
 * Deserializes binary data (in protobuf wire format) from the
 * given reader into the given message object.
 * @param {!proto.g8e.operator.v1.FsListResult} msg The message object to deserialize into.
 * @param {!jspb.BinaryReader} reader The BinaryReader to use.
 * @return {!proto.g8e.operator.v1.FsListResult}
 */
proto.g8e.operator.v1.FsListResult.deserializeBinaryFromReader = function(msg, reader) {
  while (reader.nextField()) {
    if (reader.isEndGroup()) {
      break;
    }
    var field = reader.getFieldNumber();
    switch (field) {
    case 1:
      var value = /** @type {string} */ (reader.readString());
      msg.setExecutionId(value);
      break;
    case 2:
      var value = /** @type {!proto.g8e.operator.v1.ExecutionStatus} */ (reader.readEnum());
      msg.setStatus(value);
      break;
    case 3:
      var value = /** @type {string} */ (reader.readString());
      msg.setPath(value);
      break;
    case 4:
      var value = new proto.g8e.operator.v1.FsEntry;
      reader.readMessage(value,proto.g8e.operator.v1.FsEntry.deserializeBinaryFromReader);
      msg.addEntries(value);
      break;
    case 5:
      var value = /** @type {boolean} */ (reader.readBool());
      msg.setTruncated(value);
      break;
    case 6:
      var value = /** @type {number} */ (reader.readInt32());
      msg.setTotalCount(value);
      break;
    case 7:
      var value = /** @type {number} */ (reader.readFloat());
      msg.setDurationSeconds(value);
      break;
    case 8:
      var value = /** @type {string} */ (reader.readString());
      msg.setErrorMessage(value);
      break;
    case 9:
      var value = /** @type {string} */ (reader.readString());
      msg.setErrorType(value);
      break;
    default:
      reader.skipField();
      break;
    }
  }
  return msg;
};


/**
 * Serializes the message to binary data (in protobuf wire format).
 * @return {!Uint8Array}
 */
proto.g8e.operator.v1.FsListResult.prototype.serializeBinary = function() {
  var writer = new jspb.BinaryWriter();
  proto.g8e.operator.v1.FsListResult.serializeBinaryToWriter(this, writer);
  return writer.getResultBuffer();
};


/**
 * Serializes the given message to binary data (in protobuf wire
 * format), writing to the given BinaryWriter.
 * @param {!proto.g8e.operator.v1.FsListResult} message
 * @param {!jspb.BinaryWriter} writer
 * @suppress {unusedLocalVariables} f is only used for nested messages
 */
proto.g8e.operator.v1.FsListResult.serializeBinaryToWriter = function(message, writer) {
  var f = undefined;
  f = message.getExecutionId();
  if (f.length > 0) {
    writer.writeString(
      1,
      f
    );
  }
  f = message.getStatus();
  if (f !== 0.0) {
    writer.writeEnum(
      2,
      f
    );
  }
  f = message.getPath();
  if (f.length > 0) {
    writer.writeString(
      3,
      f
    );
  }
  f = message.getEntriesList();
  if (f.length > 0) {
    writer.writeRepeatedMessage(
      4,
      f,
      proto.g8e.operator.v1.FsEntry.serializeBinaryToWriter
    );
  }
  f = message.getTruncated();
  if (f) {
    writer.writeBool(
      5,
      f
    );
  }
  f = message.getTotalCount();
  if (f !== 0) {
    writer.writeInt32(
      6,
      f
    );
  }
  f = message.getDurationSeconds();
  if (f !== 0.0) {
    writer.writeFloat(
      7,
      f
    );
  }
  f = message.getErrorMessage();
  if (f.length > 0) {
    writer.writeString(
      8,
      f
    );
  }
  f = message.getErrorType();
  if (f.length > 0) {
    writer.writeString(
      9,
      f
    );
  }
};


/**
 * optional string execution_id = 1;
 * @return {string}
 */
proto.g8e.operator.v1.FsListResult.prototype.getExecutionId = function() {
  return /** @type {string} */ (jspb.Message.getFieldWithDefault(this, 1, ""));
};


/**
 * @param {string} value
 * @return {!proto.g8e.operator.v1.FsListResult} returns this
 */
proto.g8e.operator.v1.FsListResult.prototype.setExecutionId = function(value) {
  return jspb.Message.setProto3StringField(this, 1, value);
};


/**
 * optional ExecutionStatus status = 2;
 * @return {!proto.g8e.operator.v1.ExecutionStatus}
 */
proto.g8e.operator.v1.FsListResult.prototype.getStatus = function() {
  return /** @type {!proto.g8e.operator.v1.ExecutionStatus} */ (jspb.Message.getFieldWithDefault(this, 2, 0));
};


/**
 * @param {!proto.g8e.operator.v1.ExecutionStatus} value
 * @return {!proto.g8e.operator.v1.FsListResult} returns this
 */
proto.g8e.operator.v1.FsListResult.prototype.setStatus = function(value) {
  return jspb.Message.setProto3EnumField(this, 2, value);
};


/**
 * optional string path = 3;
 * @return {string}
 */
proto.g8e.operator.v1.FsListResult.prototype.getPath = function() {
  return /** @type {string} */ (jspb.Message.getFieldWithDefault(this, 3, ""));
};


/**
 * @param {string} value
 * @return {!proto.g8e.operator.v1.FsListResult} returns this
 */
proto.g8e.operator.v1.FsListResult.prototype.setPath = function(value) {
  return jspb.Message.setProto3StringField(this, 3, value);
};


/**
 * repeated FsEntry entries = 4;
 * @return {!Array<!proto.g8e.operator.v1.FsEntry>}
 */
proto.g8e.operator.v1.FsListResult.prototype.getEntriesList = function() {
  return /** @type{!Array<!proto.g8e.operator.v1.FsEntry>} */ (
    jspb.Message.getRepeatedWrapperField(this, proto.g8e.operator.v1.FsEntry, 4));
};


/**
 * @param {!Array<!proto.g8e.operator.v1.FsEntry>} value
 * @return {!proto.g8e.operator.v1.FsListResult} returns this
*/
proto.g8e.operator.v1.FsListResult.prototype.setEntriesList = function(value) {
  return jspb.Message.setRepeatedWrapperField(this, 4, value);
};


/**
 * @param {!proto.g8e.operator.v1.FsEntry=} opt_value
 * @param {number=} opt_index
 * @return {!proto.g8e.operator.v1.FsEntry}
 */
proto.g8e.operator.v1.FsListResult.prototype.addEntries = function(opt_value, opt_index) {
  return jspb.Message.addToRepeatedWrapperField(this, 4, opt_value, proto.g8e.operator.v1.FsEntry, opt_index);
};


/**
 * Clears the list making it empty but non-null.
 * @return {!proto.g8e.operator.v1.FsListResult} returns this
 */
proto.g8e.operator.v1.FsListResult.prototype.clearEntriesList = function() {
  return this.setEntriesList([]);
};


/**
 * optional bool truncated = 5;
 * @return {boolean}
 */
proto.g8e.operator.v1.FsListResult.prototype.getTruncated = function() {
  return /** @type {boolean} */ (jspb.Message.getBooleanFieldWithDefault(this, 5, false));
};


/**
 * @param {boolean} value
 * @return {!proto.g8e.operator.v1.FsListResult} returns this
 */
proto.g8e.operator.v1.FsListResult.prototype.setTruncated = function(value) {
  return jspb.Message.setProto3BooleanField(this, 5, value);
};


/**
 * optional int32 total_count = 6;
 * @return {number}
 */
proto.g8e.operator.v1.FsListResult.prototype.getTotalCount = function() {
  return /** @type {number} */ (jspb.Message.getFieldWithDefault(this, 6, 0));
};


/**
 * @param {number} value
 * @return {!proto.g8e.operator.v1.FsListResult} returns this
 */
proto.g8e.operator.v1.FsListResult.prototype.setTotalCount = function(value) {
  return jspb.Message.setProto3IntField(this, 6, value);
};


/**
 * optional float duration_seconds = 7;
 * @return {number}
 */
proto.g8e.operator.v1.FsListResult.prototype.getDurationSeconds = function() {
  return /** @type {number} */ (jspb.Message.getFloatingPointFieldWithDefault(this, 7, 0.0));
};


/**
 * @param {number} value
 * @return {!proto.g8e.operator.v1.FsListResult} returns this
 */
proto.g8e.operator.v1.FsListResult.prototype.setDurationSeconds = function(value) {
  return jspb.Message.setProto3FloatField(this, 7, value);
};


/**
 * optional string error_message = 8;
 * @return {string}
 */
proto.g8e.operator.v1.FsListResult.prototype.getErrorMessage = function() {
  return /** @type {string} */ (jspb.Message.getFieldWithDefault(this, 8, ""));
};


/**
 * @param {string} value
 * @return {!proto.g8e.operator.v1.FsListResult} returns this
 */
proto.g8e.operator.v1.FsListResult.prototype.setErrorMessage = function(value) {
  return jspb.Message.setProto3StringField(this, 8, value);
};


/**
 * optional string error_type = 9;
 * @return {string}
 */
proto.g8e.operator.v1.FsListResult.prototype.getErrorType = function() {
  return /** @type {string} */ (jspb.Message.getFieldWithDefault(this, 9, ""));
};


/**
 * @param {string} value
 * @return {!proto.g8e.operator.v1.FsListResult} returns this
 */
proto.g8e.operator.v1.FsListResult.prototype.setErrorType = function(value) {
  return jspb.Message.setProto3StringField(this, 9, value);
};





if (jspb.Message.GENERATE_TO_OBJECT) {
/**
 * Creates an object representation of this proto.
 * Field names that are reserved in JavaScript and will be renamed to pb_name.
 * Optional fields that are not set will be set to undefined.
 * To access a reserved field use, foo.pb_<name>, eg, foo.pb_default.
 * For the list of reserved names please see:
 *     net/proto2/compiler/js/internal/generator.cc#kKeyword.
 * @param {boolean=} opt_includeInstance Deprecated. whether to include the
 *     JSPB instance for transitional soy proto support:
 *     http://goto/soy-param-migration
 * @return {!Object}
 */
proto.g8e.operator.v1.FsReadResult.prototype.toObject = function(opt_includeInstance) {
  return proto.g8e.operator.v1.FsReadResult.toObject(opt_includeInstance, this);
};


/**
 * Static version of the {@see toObject} method.
 * @param {boolean|undefined} includeInstance Deprecated. Whether to include
 *     the JSPB instance for transitional soy proto support:
 *     http://goto/soy-param-migration
 * @param {!proto.g8e.operator.v1.FsReadResult} msg The msg instance to transform.
 * @return {!Object}
 * @suppress {unusedLocalVariables} f is only used for nested messages
 */
proto.g8e.operator.v1.FsReadResult.toObject = function(includeInstance, msg) {
  var f, obj = {
    executionId: jspb.Message.getFieldWithDefault(msg, 1, ""),
    status: jspb.Message.getFieldWithDefault(msg, 2, 0),
    path: jspb.Message.getFieldWithDefault(msg, 3, ""),
    content: jspb.Message.getFieldWithDefault(msg, 4, ""),
    sizeBytes: jspb.Message.getFieldWithDefault(msg, 5, 0),
    truncated: jspb.Message.getBooleanFieldWithDefault(msg, 6, false),
    durationSeconds: jspb.Message.getFloatingPointFieldWithDefault(msg, 7, 0.0),
    errorMessage: jspb.Message.getFieldWithDefault(msg, 8, ""),
    errorType: jspb.Message.getFieldWithDefault(msg, 9, "")
  };

  if (includeInstance) {
    obj.$jspbMessageInstance = msg;
  }
  return obj;
};
}


/**
 * Deserializes binary data (in protobuf wire format).
 * @param {jspb.ByteSource} bytes The bytes to deserialize.
 * @return {!proto.g8e.operator.v1.FsReadResult}
 */
proto.g8e.operator.v1.FsReadResult.deserializeBinary = function(bytes) {
  var reader = new jspb.BinaryReader(bytes);
  var msg = new proto.g8e.operator.v1.FsReadResult;
  return proto.g8e.operator.v1.FsReadResult.deserializeBinaryFromReader(msg, reader);
};


/**
 * Deserializes binary data (in protobuf wire format) from the
 * given reader into the given message object.
 * @param {!proto.g8e.operator.v1.FsReadResult} msg The message object to deserialize into.
 * @param {!jspb.BinaryReader} reader The BinaryReader to use.
 * @return {!proto.g8e.operator.v1.FsReadResult}
 */
proto.g8e.operator.v1.FsReadResult.deserializeBinaryFromReader = function(msg, reader) {
  while (reader.nextField()) {
    if (reader.isEndGroup()) {
      break;
    }
    var field = reader.getFieldNumber();
    switch (field) {
    case 1:
      var value = /** @type {string} */ (reader.readString());
      msg.setExecutionId(value);
      break;
    case 2:
      var value = /** @type {!proto.g8e.operator.v1.ExecutionStatus} */ (reader.readEnum());
      msg.setStatus(value);
      break;
    case 3:
      var value = /** @type {string} */ (reader.readString());
      msg.setPath(value);
      break;
    case 4:
      var value = /** @type {string} */ (reader.readString());
      msg.setContent(value);
      break;
    case 5:
      var value = /** @type {number} */ (reader.readInt64());
      msg.setSizeBytes(value);
      break;
    case 6:
      var value = /** @type {boolean} */ (reader.readBool());
      msg.setTruncated(value);
      break;
    case 7:
      var value = /** @type {number} */ (reader.readFloat());
      msg.setDurationSeconds(value);
      break;
    case 8:
      var value = /** @type {string} */ (reader.readString());
      msg.setErrorMessage(value);
      break;
    case 9:
      var value = /** @type {string} */ (reader.readString());
      msg.setErrorType(value);
      break;
    default:
      reader.skipField();
      break;
    }
  }
  return msg;
};


/**
 * Serializes the message to binary data (in protobuf wire format).
 * @return {!Uint8Array}
 */
proto.g8e.operator.v1.FsReadResult.prototype.serializeBinary = function() {
  var writer = new jspb.BinaryWriter();
  proto.g8e.operator.v1.FsReadResult.serializeBinaryToWriter(this, writer);
  return writer.getResultBuffer();
};


/**
 * Serializes the given message to binary data (in protobuf wire
 * format), writing to the given BinaryWriter.
 * @param {!proto.g8e.operator.v1.FsReadResult} message
 * @param {!jspb.BinaryWriter} writer
 * @suppress {unusedLocalVariables} f is only used for nested messages
 */
proto.g8e.operator.v1.FsReadResult.serializeBinaryToWriter = function(message, writer) {
  var f = undefined;
  f = message.getExecutionId();
  if (f.length > 0) {
    writer.writeString(
      1,
      f
    );
  }
  f = message.getStatus();
  if (f !== 0.0) {
    writer.writeEnum(
      2,
      f
    );
  }
  f = message.getPath();
  if (f.length > 0) {
    writer.writeString(
      3,
      f
    );
  }
  f = message.getContent();
  if (f.length > 0) {
    writer.writeString(
      4,
      f
    );
  }
  f = message.getSizeBytes();
  if (f !== 0) {
    writer.writeInt64(
      5,
      f
    );
  }
  f = message.getTruncated();
  if (f) {
    writer.writeBool(
      6,
      f
    );
  }
  f = message.getDurationSeconds();
  if (f !== 0.0) {
    writer.writeFloat(
      7,
      f
    );
  }
  f = message.getErrorMessage();
  if (f.length > 0) {
    writer.writeString(
      8,
      f
    );
  }
  f = message.getErrorType();
  if (f.length > 0) {
    writer.writeString(
      9,
      f
    );
  }
};


/**
 * optional string execution_id = 1;
 * @return {string}
 */
proto.g8e.operator.v1.FsReadResult.prototype.getExecutionId = function() {
  return /** @type {string} */ (jspb.Message.getFieldWithDefault(this, 1, ""));
};


/**
 * @param {string} value
 * @return {!proto.g8e.operator.v1.FsReadResult} returns this
 */
proto.g8e.operator.v1.FsReadResult.prototype.setExecutionId = function(value) {
  return jspb.Message.setProto3StringField(this, 1, value);
};


/**
 * optional ExecutionStatus status = 2;
 * @return {!proto.g8e.operator.v1.ExecutionStatus}
 */
proto.g8e.operator.v1.FsReadResult.prototype.getStatus = function() {
  return /** @type {!proto.g8e.operator.v1.ExecutionStatus} */ (jspb.Message.getFieldWithDefault(this, 2, 0));
};


/**
 * @param {!proto.g8e.operator.v1.ExecutionStatus} value
 * @return {!proto.g8e.operator.v1.FsReadResult} returns this
 */
proto.g8e.operator.v1.FsReadResult.prototype.setStatus = function(value) {
  return jspb.Message.setProto3EnumField(this, 2, value);
};


/**
 * optional string path = 3;
 * @return {string}
 */
proto.g8e.operator.v1.FsReadResult.prototype.getPath = function() {
  return /** @type {string} */ (jspb.Message.getFieldWithDefault(this, 3, ""));
};


/**
 * @param {string} value
 * @return {!proto.g8e.operator.v1.FsReadResult} returns this
 */
proto.g8e.operator.v1.FsReadResult.prototype.setPath = function(value) {
  return jspb.Message.setProto3StringField(this, 3, value);
};


/**
 * optional string content = 4;
 * @return {string}
 */
proto.g8e.operator.v1.FsReadResult.prototype.getContent = function() {
  return /** @type {string} */ (jspb.Message.getFieldWithDefault(this, 4, ""));
};


/**
 * @param {string} value
 * @return {!proto.g8e.operator.v1.FsReadResult} returns this
 */
proto.g8e.operator.v1.FsReadResult.prototype.setContent = function(value) {
  return jspb.Message.setProto3StringField(this, 4, value);
};


/**
 * optional int64 size_bytes = 5;
 * @return {number}
 */
proto.g8e.operator.v1.FsReadResult.prototype.getSizeBytes = function() {
  return /** @type {number} */ (jspb.Message.getFieldWithDefault(this, 5, 0));
};


/**
 * @param {number} value
 * @return {!proto.g8e.operator.v1.FsReadResult} returns this
 */
proto.g8e.operator.v1.FsReadResult.prototype.setSizeBytes = function(value) {
  return jspb.Message.setProto3IntField(this, 5, value);
};


/**
 * optional bool truncated = 6;
 * @return {boolean}
 */
proto.g8e.operator.v1.FsReadResult.prototype.getTruncated = function() {
  return /** @type {boolean} */ (jspb.Message.getBooleanFieldWithDefault(this, 6, false));
};


/**
 * @param {boolean} value
 * @return {!proto.g8e.operator.v1.FsReadResult} returns this
 */
proto.g8e.operator.v1.FsReadResult.prototype.setTruncated = function(value) {
  return jspb.Message.setProto3BooleanField(this, 6, value);
};


/**
 * optional float duration_seconds = 7;
 * @return {number}
 */
proto.g8e.operator.v1.FsReadResult.prototype.getDurationSeconds = function() {
  return /** @type {number} */ (jspb.Message.getFloatingPointFieldWithDefault(this, 7, 0.0));
};


/**
 * @param {number} value
 * @return {!proto.g8e.operator.v1.FsReadResult} returns this
 */
proto.g8e.operator.v1.FsReadResult.prototype.setDurationSeconds = function(value) {
  return jspb.Message.setProto3FloatField(this, 7, value);
};


/**
 * optional string error_message = 8;
 * @return {string}
 */
proto.g8e.operator.v1.FsReadResult.prototype.getErrorMessage = function() {
  return /** @type {string} */ (jspb.Message.getFieldWithDefault(this, 8, ""));
};


/**
 * @param {string} value
 * @return {!proto.g8e.operator.v1.FsReadResult} returns this
 */
proto.g8e.operator.v1.FsReadResult.prototype.setErrorMessage = function(value) {
  return jspb.Message.setProto3StringField(this, 8, value);
};


/**
 * optional string error_type = 9;
 * @return {string}
 */
proto.g8e.operator.v1.FsReadResult.prototype.getErrorType = function() {
  return /** @type {string} */ (jspb.Message.getFieldWithDefault(this, 9, ""));
};


/**
 * @param {string} value
 * @return {!proto.g8e.operator.v1.FsReadResult} returns this
 */
proto.g8e.operator.v1.FsReadResult.prototype.setErrorType = function(value) {
  return jspb.Message.setProto3StringField(this, 9, value);
};



/**
 * List of repeated fields within this message type.
 * @private {!Array<number>}
 * @const
 */
proto.g8e.operator.v1.FsGrepMatch.repeatedFields_ = [4,5];



if (jspb.Message.GENERATE_TO_OBJECT) {
/**
 * Creates an object representation of this proto.
 * Field names that are reserved in JavaScript and will be renamed to pb_name.
 * Optional fields that are not set will be set to undefined.
 * To access a reserved field use, foo.pb_<name>, eg, foo.pb_default.
 * For the list of reserved names please see:
 *     net/proto2/compiler/js/internal/generator.cc#kKeyword.
 * @param {boolean=} opt_includeInstance Deprecated. whether to include the
 *     JSPB instance for transitional soy proto support:
 *     http://goto/soy-param-migration
 * @return {!Object}
 */
proto.g8e.operator.v1.FsGrepMatch.prototype.toObject = function(opt_includeInstance) {
  return proto.g8e.operator.v1.FsGrepMatch.toObject(opt_includeInstance, this);
};


/**
 * Static version of the {@see toObject} method.
 * @param {boolean|undefined} includeInstance Deprecated. Whether to include
 *     the JSPB instance for transitional soy proto support:
 *     http://goto/soy-param-migration
 * @param {!proto.g8e.operator.v1.FsGrepMatch} msg The msg instance to transform.
 * @return {!Object}
 * @suppress {unusedLocalVariables} f is only used for nested messages
 */
proto.g8e.operator.v1.FsGrepMatch.toObject = function(includeInstance, msg) {
  var f, obj = {
    path: jspb.Message.getFieldWithDefault(msg, 1, ""),
    lineNumber: jspb.Message.getFieldWithDefault(msg, 2, 0),
    content: jspb.Message.getFieldWithDefault(msg, 3, ""),
    beforeList: (f = jspb.Message.getRepeatedField(msg, 4)) == null ? undefined : f,
    afterList: (f = jspb.Message.getRepeatedField(msg, 5)) == null ? undefined : f
  };

  if (includeInstance) {
    obj.$jspbMessageInstance = msg;
  }
  return obj;
};
}


/**
 * Deserializes binary data (in protobuf wire format).
 * @param {jspb.ByteSource} bytes The bytes to deserialize.
 * @return {!proto.g8e.operator.v1.FsGrepMatch}
 */
proto.g8e.operator.v1.FsGrepMatch.deserializeBinary = function(bytes) {
  var reader = new jspb.BinaryReader(bytes);
  var msg = new proto.g8e.operator.v1.FsGrepMatch;
  return proto.g8e.operator.v1.FsGrepMatch.deserializeBinaryFromReader(msg, reader);
};


/**
 * Deserializes binary data (in protobuf wire format) from the
 * given reader into the given message object.
 * @param {!proto.g8e.operator.v1.FsGrepMatch} msg The message object to deserialize into.
 * @param {!jspb.BinaryReader} reader The BinaryReader to use.
 * @return {!proto.g8e.operator.v1.FsGrepMatch}
 */
proto.g8e.operator.v1.FsGrepMatch.deserializeBinaryFromReader = function(msg, reader) {
  while (reader.nextField()) {
    if (reader.isEndGroup()) {
      break;
    }
    var field = reader.getFieldNumber();
    switch (field) {
    case 1:
      var value = /** @type {string} */ (reader.readString());
      msg.setPath(value);
      break;
    case 2:
      var value = /** @type {number} */ (reader.readInt32());
      msg.setLineNumber(value);
      break;
    case 3:
      var value = /** @type {string} */ (reader.readString());
      msg.setContent(value);
      break;
    case 4:
      var value = /** @type {string} */ (reader.readString());
      msg.addBefore(value);
      break;
    case 5:
      var value = /** @type {string} */ (reader.readString());
      msg.addAfter(value);
      break;
    default:
      reader.skipField();
      break;
    }
  }
  return msg;
};


/**
 * Serializes the message to binary data (in protobuf wire format).
 * @return {!Uint8Array}
 */
proto.g8e.operator.v1.FsGrepMatch.prototype.serializeBinary = function() {
  var writer = new jspb.BinaryWriter();
  proto.g8e.operator.v1.FsGrepMatch.serializeBinaryToWriter(this, writer);
  return writer.getResultBuffer();
};


/**
 * Serializes the given message to binary data (in protobuf wire
 * format), writing to the given BinaryWriter.
 * @param {!proto.g8e.operator.v1.FsGrepMatch} message
 * @param {!jspb.BinaryWriter} writer
 * @suppress {unusedLocalVariables} f is only used for nested messages
 */
proto.g8e.operator.v1.FsGrepMatch.serializeBinaryToWriter = function(message, writer) {
  var f = undefined;
  f = message.getPath();
  if (f.length > 0) {
    writer.writeString(
      1,
      f
    );
  }
  f = message.getLineNumber();
  if (f !== 0) {
    writer.writeInt32(
      2,
      f
    );
  }
  f = message.getContent();
  if (f.length > 0) {
    writer.writeString(
      3,
      f
    );
  }
  f = message.getBeforeList();
  if (f.length > 0) {
    writer.writeRepeatedString(
      4,
      f
    );
  }
  f = message.getAfterList();
  if (f.length > 0) {
    writer.writeRepeatedString(
      5,
      f
    );
  }
};


/**
 * optional string path = 1;
 * @return {string}
 */
proto.g8e.operator.v1.FsGrepMatch.prototype.getPath = function() {
  return /** @type {string} */ (jspb.Message.getFieldWithDefault(this, 1, ""));
};


/**
 * @param {string} value
 * @return {!proto.g8e.operator.v1.FsGrepMatch} returns this
 */
proto.g8e.operator.v1.FsGrepMatch.prototype.setPath = function(value) {
  return jspb.Message.setProto3StringField(this, 1, value);
};


/**
 * optional int32 line_number = 2;
 * @return {number}
 */
proto.g8e.operator.v1.FsGrepMatch.prototype.getLineNumber = function() {
  return /** @type {number} */ (jspb.Message.getFieldWithDefault(this, 2, 0));
};


/**
 * @param {number} value
 * @return {!proto.g8e.operator.v1.FsGrepMatch} returns this
 */
proto.g8e.operator.v1.FsGrepMatch.prototype.setLineNumber = function(value) {
  return jspb.Message.setProto3IntField(this, 2, value);
};


/**
 * optional string content = 3;
 * @return {string}
 */
proto.g8e.operator.v1.FsGrepMatch.prototype.getContent = function() {
  return /** @type {string} */ (jspb.Message.getFieldWithDefault(this, 3, ""));
};


/**
 * @param {string} value
 * @return {!proto.g8e.operator.v1.FsGrepMatch} returns this
 */
proto.g8e.operator.v1.FsGrepMatch.prototype.setContent = function(value) {
  return jspb.Message.setProto3StringField(this, 3, value);
};


/**
 * repeated string before = 4;
 * @return {!Array<string>}
 */
proto.g8e.operator.v1.FsGrepMatch.prototype.getBeforeList = function() {
  return /** @type {!Array<string>} */ (jspb.Message.getRepeatedField(this, 4));
};


/**
 * @param {!Array<string>} value
 * @return {!proto.g8e.operator.v1.FsGrepMatch} returns this
 */
proto.g8e.operator.v1.FsGrepMatch.prototype.setBeforeList = function(value) {
  return jspb.Message.setField(this, 4, value || []);
};


/**
 * @param {string} value
 * @param {number=} opt_index
 * @return {!proto.g8e.operator.v1.FsGrepMatch} returns this
 */
proto.g8e.operator.v1.FsGrepMatch.prototype.addBefore = function(value, opt_index) {
  return jspb.Message.addToRepeatedField(this, 4, value, opt_index);
};


/**
 * Clears the list making it empty but non-null.
 * @return {!proto.g8e.operator.v1.FsGrepMatch} returns this
 */
proto.g8e.operator.v1.FsGrepMatch.prototype.clearBeforeList = function() {
  return this.setBeforeList([]);
};


/**
 * repeated string after = 5;
 * @return {!Array<string>}
 */
proto.g8e.operator.v1.FsGrepMatch.prototype.getAfterList = function() {
  return /** @type {!Array<string>} */ (jspb.Message.getRepeatedField(this, 5));
};


/**
 * @param {!Array<string>} value
 * @return {!proto.g8e.operator.v1.FsGrepMatch} returns this
 */
proto.g8e.operator.v1.FsGrepMatch.prototype.setAfterList = function(value) {
  return jspb.Message.setField(this, 5, value || []);
};


/**
 * @param {string} value
 * @param {number=} opt_index
 * @return {!proto.g8e.operator.v1.FsGrepMatch} returns this
 */
proto.g8e.operator.v1.FsGrepMatch.prototype.addAfter = function(value, opt_index) {
  return jspb.Message.addToRepeatedField(this, 5, value, opt_index);
};


/**
 * Clears the list making it empty but non-null.
 * @return {!proto.g8e.operator.v1.FsGrepMatch} returns this
 */
proto.g8e.operator.v1.FsGrepMatch.prototype.clearAfterList = function() {
  return this.setAfterList([]);
};



/**
 * List of repeated fields within this message type.
 * @private {!Array<number>}
 * @const
 */
proto.g8e.operator.v1.FsGrepResult.repeatedFields_ = [4];



if (jspb.Message.GENERATE_TO_OBJECT) {
/**
 * Creates an object representation of this proto.
 * Field names that are reserved in JavaScript and will be renamed to pb_name.
 * Optional fields that are not set will be set to undefined.
 * To access a reserved field use, foo.pb_<name>, eg, foo.pb_default.
 * For the list of reserved names please see:
 *     net/proto2/compiler/js/internal/generator.cc#kKeyword.
 * @param {boolean=} opt_includeInstance Deprecated. whether to include the
 *     JSPB instance for transitional soy proto support:
 *     http://goto/soy-param-migration
 * @return {!Object}
 */
proto.g8e.operator.v1.FsGrepResult.prototype.toObject = function(opt_includeInstance) {
  return proto.g8e.operator.v1.FsGrepResult.toObject(opt_includeInstance, this);
};


/**
 * Static version of the {@see toObject} method.
 * @param {boolean|undefined} includeInstance Deprecated. Whether to include
 *     the JSPB instance for transitional soy proto support:
 *     http://goto/soy-param-migration
 * @param {!proto.g8e.operator.v1.FsGrepResult} msg The msg instance to transform.
 * @return {!Object}
 * @suppress {unusedLocalVariables} f is only used for nested messages
 */
proto.g8e.operator.v1.FsGrepResult.toObject = function(includeInstance, msg) {
  var f, obj = {
    executionId: jspb.Message.getFieldWithDefault(msg, 1, ""),
    status: jspb.Message.getFieldWithDefault(msg, 2, 0),
    path: jspb.Message.getFieldWithDefault(msg, 3, ""),
    matchesList: jspb.Message.toObjectList(msg.getMatchesList(),
    proto.g8e.operator.v1.FsGrepMatch.toObject, includeInstance),
    totalMatches: jspb.Message.getFieldWithDefault(msg, 5, 0),
    truncated: jspb.Message.getBooleanFieldWithDefault(msg, 6, false),
    durationSeconds: jspb.Message.getFloatingPointFieldWithDefault(msg, 7, 0.0),
    errorMessage: jspb.Message.getFieldWithDefault(msg, 8, ""),
    errorType: jspb.Message.getFieldWithDefault(msg, 9, "")
  };

  if (includeInstance) {
    obj.$jspbMessageInstance = msg;
  }
  return obj;
};
}


/**
 * Deserializes binary data (in protobuf wire format).
 * @param {jspb.ByteSource} bytes The bytes to deserialize.
 * @return {!proto.g8e.operator.v1.FsGrepResult}
 */
proto.g8e.operator.v1.FsGrepResult.deserializeBinary = function(bytes) {
  var reader = new jspb.BinaryReader(bytes);
  var msg = new proto.g8e.operator.v1.FsGrepResult;
  return proto.g8e.operator.v1.FsGrepResult.deserializeBinaryFromReader(msg, reader);
};


/**
 * Deserializes binary data (in protobuf wire format) from the
 * given reader into the given message object.
 * @param {!proto.g8e.operator.v1.FsGrepResult} msg The message object to deserialize into.
 * @param {!jspb.BinaryReader} reader The BinaryReader to use.
 * @return {!proto.g8e.operator.v1.FsGrepResult}
 */
proto.g8e.operator.v1.FsGrepResult.deserializeBinaryFromReader = function(msg, reader) {
  while (reader.nextField()) {
    if (reader.isEndGroup()) {
      break;
    }
    var field = reader.getFieldNumber();
    switch (field) {
    case 1:
      var value = /** @type {string} */ (reader.readString());
      msg.setExecutionId(value);
      break;
    case 2:
      var value = /** @type {!proto.g8e.operator.v1.ExecutionStatus} */ (reader.readEnum());
      msg.setStatus(value);
      break;
    case 3:
      var value = /** @type {string} */ (reader.readString());
      msg.setPath(value);
      break;
    case 4:
      var value = new proto.g8e.operator.v1.FsGrepMatch;
      reader.readMessage(value,proto.g8e.operator.v1.FsGrepMatch.deserializeBinaryFromReader);
      msg.addMatches(value);
      break;
    case 5:
      var value = /** @type {number} */ (reader.readInt32());
      msg.setTotalMatches(value);
      break;
    case 6:
      var value = /** @type {boolean} */ (reader.readBool());
      msg.setTruncated(value);
      break;
    case 7:
      var value = /** @type {number} */ (reader.readFloat());
      msg.setDurationSeconds(value);
      break;
    case 8:
      var value = /** @type {string} */ (reader.readString());
      msg.setErrorMessage(value);
      break;
    case 9:
      var value = /** @type {string} */ (reader.readString());
      msg.setErrorType(value);
      break;
    default:
      reader.skipField();
      break;
    }
  }
  return msg;
};


/**
 * Serializes the message to binary data (in protobuf wire format).
 * @return {!Uint8Array}
 */
proto.g8e.operator.v1.FsGrepResult.prototype.serializeBinary = function() {
  var writer = new jspb.BinaryWriter();
  proto.g8e.operator.v1.FsGrepResult.serializeBinaryToWriter(this, writer);
  return writer.getResultBuffer();
};


/**
 * Serializes the given message to binary data (in protobuf wire
 * format), writing to the given BinaryWriter.
 * @param {!proto.g8e.operator.v1.FsGrepResult} message
 * @param {!jspb.BinaryWriter} writer
 * @suppress {unusedLocalVariables} f is only used for nested messages
 */
proto.g8e.operator.v1.FsGrepResult.serializeBinaryToWriter = function(message, writer) {
  var f = undefined;
  f = message.getExecutionId();
  if (f.length > 0) {
    writer.writeString(
      1,
      f
    );
  }
  f = message.getStatus();
  if (f !== 0.0) {
    writer.writeEnum(
      2,
      f
    );
  }
  f = message.getPath();
  if (f.length > 0) {
    writer.writeString(
      3,
      f
    );
  }
  f = message.getMatchesList();
  if (f.length > 0) {
    writer.writeRepeatedMessage(
      4,
      f,
      proto.g8e.operator.v1.FsGrepMatch.serializeBinaryToWriter
    );
  }
  f = message.getTotalMatches();
  if (f !== 0) {
    writer.writeInt32(
      5,
      f
    );
  }
  f = message.getTruncated();
  if (f) {
    writer.writeBool(
      6,
      f
    );
  }
  f = message.getDurationSeconds();
  if (f !== 0.0) {
    writer.writeFloat(
      7,
      f
    );
  }
  f = message.getErrorMessage();
  if (f.length > 0) {
    writer.writeString(
      8,
      f
    );
  }
  f = message.getErrorType();
  if (f.length > 0) {
    writer.writeString(
      9,
      f
    );
  }
};


/**
 * optional string execution_id = 1;
 * @return {string}
 */
proto.g8e.operator.v1.FsGrepResult.prototype.getExecutionId = function() {
  return /** @type {string} */ (jspb.Message.getFieldWithDefault(this, 1, ""));
};


/**
 * @param {string} value
 * @return {!proto.g8e.operator.v1.FsGrepResult} returns this
 */
proto.g8e.operator.v1.FsGrepResult.prototype.setExecutionId = function(value) {
  return jspb.Message.setProto3StringField(this, 1, value);
};


/**
 * optional ExecutionStatus status = 2;
 * @return {!proto.g8e.operator.v1.ExecutionStatus}
 */
proto.g8e.operator.v1.FsGrepResult.prototype.getStatus = function() {
  return /** @type {!proto.g8e.operator.v1.ExecutionStatus} */ (jspb.Message.getFieldWithDefault(this, 2, 0));
};


/**
 * @param {!proto.g8e.operator.v1.ExecutionStatus} value
 * @return {!proto.g8e.operator.v1.FsGrepResult} returns this
 */
proto.g8e.operator.v1.FsGrepResult.prototype.setStatus = function(value) {
  return jspb.Message.setProto3EnumField(this, 2, value);
};


/**
 * optional string path = 3;
 * @return {string}
 */
proto.g8e.operator.v1.FsGrepResult.prototype.getPath = function() {
  return /** @type {string} */ (jspb.Message.getFieldWithDefault(this, 3, ""));
};


/**
 * @param {string} value
 * @return {!proto.g8e.operator.v1.FsGrepResult} returns this
 */
proto.g8e.operator.v1.FsGrepResult.prototype.setPath = function(value) {
  return jspb.Message.setProto3StringField(this, 3, value);
};


/**
 * repeated FsGrepMatch matches = 4;
 * @return {!Array<!proto.g8e.operator.v1.FsGrepMatch>}
 */
proto.g8e.operator.v1.FsGrepResult.prototype.getMatchesList = function() {
  return /** @type{!Array<!proto.g8e.operator.v1.FsGrepMatch>} */ (
    jspb.Message.getRepeatedWrapperField(this, proto.g8e.operator.v1.FsGrepMatch, 4));
};


/**
 * @param {!Array<!proto.g8e.operator.v1.FsGrepMatch>} value
 * @return {!proto.g8e.operator.v1.FsGrepResult} returns this
*/
proto.g8e.operator.v1.FsGrepResult.prototype.setMatchesList = function(value) {
  return jspb.Message.setRepeatedWrapperField(this, 4, value);
};


/**
 * @param {!proto.g8e.operator.v1.FsGrepMatch=} opt_value
 * @param {number=} opt_index
 * @return {!proto.g8e.operator.v1.FsGrepMatch}
 */
proto.g8e.operator.v1.FsGrepResult.prototype.addMatches = function(opt_value, opt_index) {
  return jspb.Message.addToRepeatedWrapperField(this, 4, opt_value, proto.g8e.operator.v1.FsGrepMatch, opt_index);
};


/**
 * Clears the list making it empty but non-null.
 * @return {!proto.g8e.operator.v1.FsGrepResult} returns this
 */
proto.g8e.operator.v1.FsGrepResult.prototype.clearMatchesList = function() {
  return this.setMatchesList([]);
};


/**
 * optional int32 total_matches = 5;
 * @return {number}
 */
proto.g8e.operator.v1.FsGrepResult.prototype.getTotalMatches = function() {
  return /** @type {number} */ (jspb.Message.getFieldWithDefault(this, 5, 0));
};


/**
 * @param {number} value
 * @return {!proto.g8e.operator.v1.FsGrepResult} returns this
 */
proto.g8e.operator.v1.FsGrepResult.prototype.setTotalMatches = function(value) {
  return jspb.Message.setProto3IntField(this, 5, value);
};


/**
 * optional bool truncated = 6;
 * @return {boolean}
 */
proto.g8e.operator.v1.FsGrepResult.prototype.getTruncated = function() {
  return /** @type {boolean} */ (jspb.Message.getBooleanFieldWithDefault(this, 6, false));
};


/**
 * @param {boolean} value
 * @return {!proto.g8e.operator.v1.FsGrepResult} returns this
 */
proto.g8e.operator.v1.FsGrepResult.prototype.setTruncated = function(value) {
  return jspb.Message.setProto3BooleanField(this, 6, value);
};


/**
 * optional float duration_seconds = 7;
 * @return {number}
 */
proto.g8e.operator.v1.FsGrepResult.prototype.getDurationSeconds = function() {
  return /** @type {number} */ (jspb.Message.getFloatingPointFieldWithDefault(this, 7, 0.0));
};


/**
 * @param {number} value
 * @return {!proto.g8e.operator.v1.FsGrepResult} returns this
 */
proto.g8e.operator.v1.FsGrepResult.prototype.setDurationSeconds = function(value) {
  return jspb.Message.setProto3FloatField(this, 7, value);
};


/**
 * optional string error_message = 8;
 * @return {string}
 */
proto.g8e.operator.v1.FsGrepResult.prototype.getErrorMessage = function() {
  return /** @type {string} */ (jspb.Message.getFieldWithDefault(this, 8, ""));
};


/**
 * @param {string} value
 * @return {!proto.g8e.operator.v1.FsGrepResult} returns this
 */
proto.g8e.operator.v1.FsGrepResult.prototype.setErrorMessage = function(value) {
  return jspb.Message.setProto3StringField(this, 8, value);
};


/**
 * optional string error_type = 9;
 * @return {string}
 */
proto.g8e.operator.v1.FsGrepResult.prototype.getErrorType = function() {
  return /** @type {string} */ (jspb.Message.getFieldWithDefault(this, 9, ""));
};


/**
 * @param {string} value
 * @return {!proto.g8e.operator.v1.FsGrepResult} returns this
 */
proto.g8e.operator.v1.FsGrepResult.prototype.setErrorType = function(value) {
  return jspb.Message.setProto3StringField(this, 9, value);
};





if (jspb.Message.GENERATE_TO_OBJECT) {
/**
 * Creates an object representation of this proto.
 * Field names that are reserved in JavaScript and will be renamed to pb_name.
 * Optional fields that are not set will be set to undefined.
 * To access a reserved field use, foo.pb_<name>, eg, foo.pb_default.
 * For the list of reserved names please see:
 *     net/proto2/compiler/js/internal/generator.cc#kKeyword.
 * @param {boolean=} opt_includeInstance Deprecated. whether to include the
 *     JSPB instance for transitional soy proto support:
 *     http://goto/soy-param-migration
 * @return {!Object}
 */
proto.g8e.operator.v1.FileEditResult.prototype.toObject = function(opt_includeInstance) {
  return proto.g8e.operator.v1.FileEditResult.toObject(opt_includeInstance, this);
};


/**
 * Static version of the {@see toObject} method.
 * @param {boolean|undefined} includeInstance Deprecated. Whether to include
 *     the JSPB instance for transitional soy proto support:
 *     http://goto/soy-param-migration
 * @param {!proto.g8e.operator.v1.FileEditResult} msg The msg instance to transform.
 * @return {!Object}
 * @suppress {unusedLocalVariables} f is only used for nested messages
 */
proto.g8e.operator.v1.FileEditResult.toObject = function(includeInstance, msg) {
  var f, obj = {
    executionId: jspb.Message.getFieldWithDefault(msg, 1, ""),
    status: jspb.Message.getFieldWithDefault(msg, 2, 0),
    filePath: jspb.Message.getFieldWithDefault(msg, 3, ""),
    operation: jspb.Message.getFieldWithDefault(msg, 4, ""),
    durationSeconds: jspb.Message.getFloatingPointFieldWithDefault(msg, 5, 0.0),
    bytesWritten: jspb.Message.getFieldWithDefault(msg, 6, 0),
    linesChanged: jspb.Message.getFieldWithDefault(msg, 7, 0),
    backupPath: jspb.Message.getFieldWithDefault(msg, 8, ""),
    errorMessage: jspb.Message.getFieldWithDefault(msg, 9, ""),
    errorType: jspb.Message.getFieldWithDefault(msg, 10, ""),
    content: jspb.Message.getFieldWithDefault(msg, 11, ""),
    stdoutSize: jspb.Message.getFieldWithDefault(msg, 12, 0),
    stderrSize: jspb.Message.getFieldWithDefault(msg, 13, 0)
  };

  if (includeInstance) {
    obj.$jspbMessageInstance = msg;
  }
  return obj;
};
}


/**
 * Deserializes binary data (in protobuf wire format).
 * @param {jspb.ByteSource} bytes The bytes to deserialize.
 * @return {!proto.g8e.operator.v1.FileEditResult}
 */
proto.g8e.operator.v1.FileEditResult.deserializeBinary = function(bytes) {
  var reader = new jspb.BinaryReader(bytes);
  var msg = new proto.g8e.operator.v1.FileEditResult;
  return proto.g8e.operator.v1.FileEditResult.deserializeBinaryFromReader(msg, reader);
};


/**
 * Deserializes binary data (in protobuf wire format) from the
 * given reader into the given message object.
 * @param {!proto.g8e.operator.v1.FileEditResult} msg The message object to deserialize into.
 * @param {!jspb.BinaryReader} reader The BinaryReader to use.
 * @return {!proto.g8e.operator.v1.FileEditResult}
 */
proto.g8e.operator.v1.FileEditResult.deserializeBinaryFromReader = function(msg, reader) {
  while (reader.nextField()) {
    if (reader.isEndGroup()) {
      break;
    }
    var field = reader.getFieldNumber();
    switch (field) {
    case 1:
      var value = /** @type {string} */ (reader.readString());
      msg.setExecutionId(value);
      break;
    case 2:
      var value = /** @type {!proto.g8e.operator.v1.ExecutionStatus} */ (reader.readEnum());
      msg.setStatus(value);
      break;
    case 3:
      var value = /** @type {string} */ (reader.readString());
      msg.setFilePath(value);
      break;
    case 4:
      var value = /** @type {string} */ (reader.readString());
      msg.setOperation(value);
      break;
    case 5:
      var value = /** @type {number} */ (reader.readFloat());
      msg.setDurationSeconds(value);
      break;
    case 6:
      var value = /** @type {number} */ (reader.readInt64());
      msg.setBytesWritten(value);
      break;
    case 7:
      var value = /** @type {number} */ (reader.readInt32());
      msg.setLinesChanged(value);
      break;
    case 8:
      var value = /** @type {string} */ (reader.readString());
      msg.setBackupPath(value);
      break;
    case 9:
      var value = /** @type {string} */ (reader.readString());
      msg.setErrorMessage(value);
      break;
    case 10:
      var value = /** @type {string} */ (reader.readString());
      msg.setErrorType(value);
      break;
    case 11:
      var value = /** @type {string} */ (reader.readString());
      msg.setContent(value);
      break;
    case 12:
      var value = /** @type {number} */ (reader.readInt32());
      msg.setStdoutSize(value);
      break;
    case 13:
      var value = /** @type {number} */ (reader.readInt32());
      msg.setStderrSize(value);
      break;
    default:
      reader.skipField();
      break;
    }
  }
  return msg;
};


/**
 * Serializes the message to binary data (in protobuf wire format).
 * @return {!Uint8Array}
 */
proto.g8e.operator.v1.FileEditResult.prototype.serializeBinary = function() {
  var writer = new jspb.BinaryWriter();
  proto.g8e.operator.v1.FileEditResult.serializeBinaryToWriter(this, writer);
  return writer.getResultBuffer();
};


/**
 * Serializes the given message to binary data (in protobuf wire
 * format), writing to the given BinaryWriter.
 * @param {!proto.g8e.operator.v1.FileEditResult} message
 * @param {!jspb.BinaryWriter} writer
 * @suppress {unusedLocalVariables} f is only used for nested messages
 */
proto.g8e.operator.v1.FileEditResult.serializeBinaryToWriter = function(message, writer) {
  var f = undefined;
  f = message.getExecutionId();
  if (f.length > 0) {
    writer.writeString(
      1,
      f
    );
  }
  f = message.getStatus();
  if (f !== 0.0) {
    writer.writeEnum(
      2,
      f
    );
  }
  f = message.getFilePath();
  if (f.length > 0) {
    writer.writeString(
      3,
      f
    );
  }
  f = message.getOperation();
  if (f.length > 0) {
    writer.writeString(
      4,
      f
    );
  }
  f = message.getDurationSeconds();
  if (f !== 0.0) {
    writer.writeFloat(
      5,
      f
    );
  }
  f = message.getBytesWritten();
  if (f !== 0) {
    writer.writeInt64(
      6,
      f
    );
  }
  f = message.getLinesChanged();
  if (f !== 0) {
    writer.writeInt32(
      7,
      f
    );
  }
  f = message.getBackupPath();
  if (f.length > 0) {
    writer.writeString(
      8,
      f
    );
  }
  f = message.getErrorMessage();
  if (f.length > 0) {
    writer.writeString(
      9,
      f
    );
  }
  f = message.getErrorType();
  if (f.length > 0) {
    writer.writeString(
      10,
      f
    );
  }
  f = message.getContent();
  if (f.length > 0) {
    writer.writeString(
      11,
      f
    );
  }
  f = message.getStdoutSize();
  if (f !== 0) {
    writer.writeInt32(
      12,
      f
    );
  }
  f = message.getStderrSize();
  if (f !== 0) {
    writer.writeInt32(
      13,
      f
    );
  }
};


/**
 * optional string execution_id = 1;
 * @return {string}
 */
proto.g8e.operator.v1.FileEditResult.prototype.getExecutionId = function() {
  return /** @type {string} */ (jspb.Message.getFieldWithDefault(this, 1, ""));
};


/**
 * @param {string} value
 * @return {!proto.g8e.operator.v1.FileEditResult} returns this
 */
proto.g8e.operator.v1.FileEditResult.prototype.setExecutionId = function(value) {
  return jspb.Message.setProto3StringField(this, 1, value);
};


/**
 * optional ExecutionStatus status = 2;
 * @return {!proto.g8e.operator.v1.ExecutionStatus}
 */
proto.g8e.operator.v1.FileEditResult.prototype.getStatus = function() {
  return /** @type {!proto.g8e.operator.v1.ExecutionStatus} */ (jspb.Message.getFieldWithDefault(this, 2, 0));
};


/**
 * @param {!proto.g8e.operator.v1.ExecutionStatus} value
 * @return {!proto.g8e.operator.v1.FileEditResult} returns this
 */
proto.g8e.operator.v1.FileEditResult.prototype.setStatus = function(value) {
  return jspb.Message.setProto3EnumField(this, 2, value);
};


/**
 * optional string file_path = 3;
 * @return {string}
 */
proto.g8e.operator.v1.FileEditResult.prototype.getFilePath = function() {
  return /** @type {string} */ (jspb.Message.getFieldWithDefault(this, 3, ""));
};


/**
 * @param {string} value
 * @return {!proto.g8e.operator.v1.FileEditResult} returns this
 */
proto.g8e.operator.v1.FileEditResult.prototype.setFilePath = function(value) {
  return jspb.Message.setProto3StringField(this, 3, value);
};


/**
 * optional string operation = 4;
 * @return {string}
 */
proto.g8e.operator.v1.FileEditResult.prototype.getOperation = function() {
  return /** @type {string} */ (jspb.Message.getFieldWithDefault(this, 4, ""));
};


/**
 * @param {string} value
 * @return {!proto.g8e.operator.v1.FileEditResult} returns this
 */
proto.g8e.operator.v1.FileEditResult.prototype.setOperation = function(value) {
  return jspb.Message.setProto3StringField(this, 4, value);
};


/**
 * optional float duration_seconds = 5;
 * @return {number}
 */
proto.g8e.operator.v1.FileEditResult.prototype.getDurationSeconds = function() {
  return /** @type {number} */ (jspb.Message.getFloatingPointFieldWithDefault(this, 5, 0.0));
};


/**
 * @param {number} value
 * @return {!proto.g8e.operator.v1.FileEditResult} returns this
 */
proto.g8e.operator.v1.FileEditResult.prototype.setDurationSeconds = function(value) {
  return jspb.Message.setProto3FloatField(this, 5, value);
};


/**
 * optional int64 bytes_written = 6;
 * @return {number}
 */
proto.g8e.operator.v1.FileEditResult.prototype.getBytesWritten = function() {
  return /** @type {number} */ (jspb.Message.getFieldWithDefault(this, 6, 0));
};


/**
 * @param {number} value
 * @return {!proto.g8e.operator.v1.FileEditResult} returns this
 */
proto.g8e.operator.v1.FileEditResult.prototype.setBytesWritten = function(value) {
  return jspb.Message.setProto3IntField(this, 6, value);
};


/**
 * optional int32 lines_changed = 7;
 * @return {number}
 */
proto.g8e.operator.v1.FileEditResult.prototype.getLinesChanged = function() {
  return /** @type {number} */ (jspb.Message.getFieldWithDefault(this, 7, 0));
};


/**
 * @param {number} value
 * @return {!proto.g8e.operator.v1.FileEditResult} returns this
 */
proto.g8e.operator.v1.FileEditResult.prototype.setLinesChanged = function(value) {
  return jspb.Message.setProto3IntField(this, 7, value);
};


/**
 * optional string backup_path = 8;
 * @return {string}
 */
proto.g8e.operator.v1.FileEditResult.prototype.getBackupPath = function() {
  return /** @type {string} */ (jspb.Message.getFieldWithDefault(this, 8, ""));
};


/**
 * @param {string} value
 * @return {!proto.g8e.operator.v1.FileEditResult} returns this
 */
proto.g8e.operator.v1.FileEditResult.prototype.setBackupPath = function(value) {
  return jspb.Message.setProto3StringField(this, 8, value);
};


/**
 * optional string error_message = 9;
 * @return {string}
 */
proto.g8e.operator.v1.FileEditResult.prototype.getErrorMessage = function() {
  return /** @type {string} */ (jspb.Message.getFieldWithDefault(this, 9, ""));
};


/**
 * @param {string} value
 * @return {!proto.g8e.operator.v1.FileEditResult} returns this
 */
proto.g8e.operator.v1.FileEditResult.prototype.setErrorMessage = function(value) {
  return jspb.Message.setProto3StringField(this, 9, value);
};


/**
 * optional string error_type = 10;
 * @return {string}
 */
proto.g8e.operator.v1.FileEditResult.prototype.getErrorType = function() {
  return /** @type {string} */ (jspb.Message.getFieldWithDefault(this, 10, ""));
};


/**
 * @param {string} value
 * @return {!proto.g8e.operator.v1.FileEditResult} returns this
 */
proto.g8e.operator.v1.FileEditResult.prototype.setErrorType = function(value) {
  return jspb.Message.setProto3StringField(this, 10, value);
};


/**
 * optional string content = 11;
 * @return {string}
 */
proto.g8e.operator.v1.FileEditResult.prototype.getContent = function() {
  return /** @type {string} */ (jspb.Message.getFieldWithDefault(this, 11, ""));
};


/**
 * @param {string} value
 * @return {!proto.g8e.operator.v1.FileEditResult} returns this
 */
proto.g8e.operator.v1.FileEditResult.prototype.setContent = function(value) {
  return jspb.Message.setProto3StringField(this, 11, value);
};


/**
 * optional int32 stdout_size = 12;
 * @return {number}
 */
proto.g8e.operator.v1.FileEditResult.prototype.getStdoutSize = function() {
  return /** @type {number} */ (jspb.Message.getFieldWithDefault(this, 12, 0));
};


/**
 * @param {number} value
 * @return {!proto.g8e.operator.v1.FileEditResult} returns this
 */
proto.g8e.operator.v1.FileEditResult.prototype.setStdoutSize = function(value) {
  return jspb.Message.setProto3IntField(this, 12, value);
};


/**
 * optional int32 stderr_size = 13;
 * @return {number}
 */
proto.g8e.operator.v1.FileEditResult.prototype.getStderrSize = function() {
  return /** @type {number} */ (jspb.Message.getFieldWithDefault(this, 13, 0));
};


/**
 * @param {number} value
 * @return {!proto.g8e.operator.v1.FileEditResult} returns this
 */
proto.g8e.operator.v1.FileEditResult.prototype.setStderrSize = function(value) {
  return jspb.Message.setProto3IntField(this, 13, value);
};





if (jspb.Message.GENERATE_TO_OBJECT) {
/**
 * Creates an object representation of this proto.
 * Field names that are reserved in JavaScript and will be renamed to pb_name.
 * Optional fields that are not set will be set to undefined.
 * To access a reserved field use, foo.pb_<name>, eg, foo.pb_default.
 * For the list of reserved names please see:
 *     net/proto2/compiler/js/internal/generator.cc#kKeyword.
 * @param {boolean=} opt_includeInstance Deprecated. whether to include the
 *     JSPB instance for transitional soy proto support:
 *     http://goto/soy-param-migration
 * @return {!Object}
 */
proto.g8e.operator.v1.ExecutionStatusUpdate.prototype.toObject = function(opt_includeInstance) {
  return proto.g8e.operator.v1.ExecutionStatusUpdate.toObject(opt_includeInstance, this);
};


/**
 * Static version of the {@see toObject} method.
 * @param {boolean|undefined} includeInstance Deprecated. Whether to include
 *     the JSPB instance for transitional soy proto support:
 *     http://goto/soy-param-migration
 * @param {!proto.g8e.operator.v1.ExecutionStatusUpdate} msg The msg instance to transform.
 * @return {!Object}
 * @suppress {unusedLocalVariables} f is only used for nested messages
 */
proto.g8e.operator.v1.ExecutionStatusUpdate.toObject = function(includeInstance, msg) {
  var f, obj = {
    executionId: jspb.Message.getFieldWithDefault(msg, 1, ""),
    status: jspb.Message.getFieldWithDefault(msg, 2, 0),
    command: jspb.Message.getFieldWithDefault(msg, 3, ""),
    processAlive: jspb.Message.getBooleanFieldWithDefault(msg, 4, false),
    elapsedSeconds: jspb.Message.getFloatingPointFieldWithDefault(msg, 5, 0.0),
    newOutput: jspb.Message.getFieldWithDefault(msg, 6, ""),
    newStderr: jspb.Message.getFieldWithDefault(msg, 7, ""),
    message: jspb.Message.getFieldWithDefault(msg, 8, "")
  };

  if (includeInstance) {
    obj.$jspbMessageInstance = msg;
  }
  return obj;
};
}


/**
 * Deserializes binary data (in protobuf wire format).
 * @param {jspb.ByteSource} bytes The bytes to deserialize.
 * @return {!proto.g8e.operator.v1.ExecutionStatusUpdate}
 */
proto.g8e.operator.v1.ExecutionStatusUpdate.deserializeBinary = function(bytes) {
  var reader = new jspb.BinaryReader(bytes);
  var msg = new proto.g8e.operator.v1.ExecutionStatusUpdate;
  return proto.g8e.operator.v1.ExecutionStatusUpdate.deserializeBinaryFromReader(msg, reader);
};


/**
 * Deserializes binary data (in protobuf wire format) from the
 * given reader into the given message object.
 * @param {!proto.g8e.operator.v1.ExecutionStatusUpdate} msg The message object to deserialize into.
 * @param {!jspb.BinaryReader} reader The BinaryReader to use.
 * @return {!proto.g8e.operator.v1.ExecutionStatusUpdate}
 */
proto.g8e.operator.v1.ExecutionStatusUpdate.deserializeBinaryFromReader = function(msg, reader) {
  while (reader.nextField()) {
    if (reader.isEndGroup()) {
      break;
    }
    var field = reader.getFieldNumber();
    switch (field) {
    case 1:
      var value = /** @type {string} */ (reader.readString());
      msg.setExecutionId(value);
      break;
    case 2:
      var value = /** @type {!proto.g8e.operator.v1.ExecutionStatus} */ (reader.readEnum());
      msg.setStatus(value);
      break;
    case 3:
      var value = /** @type {string} */ (reader.readString());
      msg.setCommand(value);
      break;
    case 4:
      var value = /** @type {boolean} */ (reader.readBool());
      msg.setProcessAlive(value);
      break;
    case 5:
      var value = /** @type {number} */ (reader.readFloat());
      msg.setElapsedSeconds(value);
      break;
    case 6:
      var value = /** @type {string} */ (reader.readString());
      msg.setNewOutput(value);
      break;
    case 7:
      var value = /** @type {string} */ (reader.readString());
      msg.setNewStderr(value);
      break;
    case 8:
      var value = /** @type {string} */ (reader.readString());
      msg.setMessage(value);
      break;
    default:
      reader.skipField();
      break;
    }
  }
  return msg;
};


/**
 * Serializes the message to binary data (in protobuf wire format).
 * @return {!Uint8Array}
 */
proto.g8e.operator.v1.ExecutionStatusUpdate.prototype.serializeBinary = function() {
  var writer = new jspb.BinaryWriter();
  proto.g8e.operator.v1.ExecutionStatusUpdate.serializeBinaryToWriter(this, writer);
  return writer.getResultBuffer();
};


/**
 * Serializes the given message to binary data (in protobuf wire
 * format), writing to the given BinaryWriter.
 * @param {!proto.g8e.operator.v1.ExecutionStatusUpdate} message
 * @param {!jspb.BinaryWriter} writer
 * @suppress {unusedLocalVariables} f is only used for nested messages
 */
proto.g8e.operator.v1.ExecutionStatusUpdate.serializeBinaryToWriter = function(message, writer) {
  var f = undefined;
  f = message.getExecutionId();
  if (f.length > 0) {
    writer.writeString(
      1,
      f
    );
  }
  f = message.getStatus();
  if (f !== 0.0) {
    writer.writeEnum(
      2,
      f
    );
  }
  f = message.getCommand();
  if (f.length > 0) {
    writer.writeString(
      3,
      f
    );
  }
  f = message.getProcessAlive();
  if (f) {
    writer.writeBool(
      4,
      f
    );
  }
  f = message.getElapsedSeconds();
  if (f !== 0.0) {
    writer.writeFloat(
      5,
      f
    );
  }
  f = message.getNewOutput();
  if (f.length > 0) {
    writer.writeString(
      6,
      f
    );
  }
  f = message.getNewStderr();
  if (f.length > 0) {
    writer.writeString(
      7,
      f
    );
  }
  f = message.getMessage();
  if (f.length > 0) {
    writer.writeString(
      8,
      f
    );
  }
};


/**
 * optional string execution_id = 1;
 * @return {string}
 */
proto.g8e.operator.v1.ExecutionStatusUpdate.prototype.getExecutionId = function() {
  return /** @type {string} */ (jspb.Message.getFieldWithDefault(this, 1, ""));
};


/**
 * @param {string} value
 * @return {!proto.g8e.operator.v1.ExecutionStatusUpdate} returns this
 */
proto.g8e.operator.v1.ExecutionStatusUpdate.prototype.setExecutionId = function(value) {
  return jspb.Message.setProto3StringField(this, 1, value);
};


/**
 * optional ExecutionStatus status = 2;
 * @return {!proto.g8e.operator.v1.ExecutionStatus}
 */
proto.g8e.operator.v1.ExecutionStatusUpdate.prototype.getStatus = function() {
  return /** @type {!proto.g8e.operator.v1.ExecutionStatus} */ (jspb.Message.getFieldWithDefault(this, 2, 0));
};


/**
 * @param {!proto.g8e.operator.v1.ExecutionStatus} value
 * @return {!proto.g8e.operator.v1.ExecutionStatusUpdate} returns this
 */
proto.g8e.operator.v1.ExecutionStatusUpdate.prototype.setStatus = function(value) {
  return jspb.Message.setProto3EnumField(this, 2, value);
};


/**
 * optional string command = 3;
 * @return {string}
 */
proto.g8e.operator.v1.ExecutionStatusUpdate.prototype.getCommand = function() {
  return /** @type {string} */ (jspb.Message.getFieldWithDefault(this, 3, ""));
};


/**
 * @param {string} value
 * @return {!proto.g8e.operator.v1.ExecutionStatusUpdate} returns this
 */
proto.g8e.operator.v1.ExecutionStatusUpdate.prototype.setCommand = function(value) {
  return jspb.Message.setProto3StringField(this, 3, value);
};


/**
 * optional bool process_alive = 4;
 * @return {boolean}
 */
proto.g8e.operator.v1.ExecutionStatusUpdate.prototype.getProcessAlive = function() {
  return /** @type {boolean} */ (jspb.Message.getBooleanFieldWithDefault(this, 4, false));
};


/**
 * @param {boolean} value
 * @return {!proto.g8e.operator.v1.ExecutionStatusUpdate} returns this
 */
proto.g8e.operator.v1.ExecutionStatusUpdate.prototype.setProcessAlive = function(value) {
  return jspb.Message.setProto3BooleanField(this, 4, value);
};


/**
 * optional float elapsed_seconds = 5;
 * @return {number}
 */
proto.g8e.operator.v1.ExecutionStatusUpdate.prototype.getElapsedSeconds = function() {
  return /** @type {number} */ (jspb.Message.getFloatingPointFieldWithDefault(this, 5, 0.0));
};


/**
 * @param {number} value
 * @return {!proto.g8e.operator.v1.ExecutionStatusUpdate} returns this
 */
proto.g8e.operator.v1.ExecutionStatusUpdate.prototype.setElapsedSeconds = function(value) {
  return jspb.Message.setProto3FloatField(this, 5, value);
};


/**
 * optional string new_output = 6;
 * @return {string}
 */
proto.g8e.operator.v1.ExecutionStatusUpdate.prototype.getNewOutput = function() {
  return /** @type {string} */ (jspb.Message.getFieldWithDefault(this, 6, ""));
};


/**
 * @param {string} value
 * @return {!proto.g8e.operator.v1.ExecutionStatusUpdate} returns this
 */
proto.g8e.operator.v1.ExecutionStatusUpdate.prototype.setNewOutput = function(value) {
  return jspb.Message.setProto3StringField(this, 6, value);
};


/**
 * optional string new_stderr = 7;
 * @return {string}
 */
proto.g8e.operator.v1.ExecutionStatusUpdate.prototype.getNewStderr = function() {
  return /** @type {string} */ (jspb.Message.getFieldWithDefault(this, 7, ""));
};


/**
 * @param {string} value
 * @return {!proto.g8e.operator.v1.ExecutionStatusUpdate} returns this
 */
proto.g8e.operator.v1.ExecutionStatusUpdate.prototype.setNewStderr = function(value) {
  return jspb.Message.setProto3StringField(this, 7, value);
};


/**
 * optional string message = 8;
 * @return {string}
 */
proto.g8e.operator.v1.ExecutionStatusUpdate.prototype.getMessage = function() {
  return /** @type {string} */ (jspb.Message.getFieldWithDefault(this, 8, ""));
};


/**
 * @param {string} value
 * @return {!proto.g8e.operator.v1.ExecutionStatusUpdate} returns this
 */
proto.g8e.operator.v1.ExecutionStatusUpdate.prototype.setMessage = function(value) {
  return jspb.Message.setProto3StringField(this, 8, value);
};





if (jspb.Message.GENERATE_TO_OBJECT) {
/**
 * Creates an object representation of this proto.
 * Field names that are reserved in JavaScript and will be renamed to pb_name.
 * Optional fields that are not set will be set to undefined.
 * To access a reserved field use, foo.pb_<name>, eg, foo.pb_default.
 * For the list of reserved names please see:
 *     net/proto2/compiler/js/internal/generator.cc#kKeyword.
 * @param {boolean=} opt_includeInstance Deprecated. whether to include the
 *     JSPB instance for transitional soy proto support:
 *     http://goto/soy-param-migration
 * @return {!Object}
 */
proto.g8e.operator.v1.PortCheckEntry.prototype.toObject = function(opt_includeInstance) {
  return proto.g8e.operator.v1.PortCheckEntry.toObject(opt_includeInstance, this);
};


/**
 * Static version of the {@see toObject} method.
 * @param {boolean|undefined} includeInstance Deprecated. Whether to include
 *     the JSPB instance for transitional soy proto support:
 *     http://goto/soy-param-migration
 * @param {!proto.g8e.operator.v1.PortCheckEntry} msg The msg instance to transform.
 * @return {!Object}
 * @suppress {unusedLocalVariables} f is only used for nested messages
 */
proto.g8e.operator.v1.PortCheckEntry.toObject = function(includeInstance, msg) {
  var f, obj = {
    host: jspb.Message.getFieldWithDefault(msg, 1, ""),
    port: jspb.Message.getFieldWithDefault(msg, 2, 0),
    open: jspb.Message.getBooleanFieldWithDefault(msg, 3, false),
    latencyMs: jspb.Message.getFloatingPointFieldWithDefault(msg, 4, 0.0),
    error: jspb.Message.getFieldWithDefault(msg, 5, "")
  };

  if (includeInstance) {
    obj.$jspbMessageInstance = msg;
  }
  return obj;
};
}


/**
 * Deserializes binary data (in protobuf wire format).
 * @param {jspb.ByteSource} bytes The bytes to deserialize.
 * @return {!proto.g8e.operator.v1.PortCheckEntry}
 */
proto.g8e.operator.v1.PortCheckEntry.deserializeBinary = function(bytes) {
  var reader = new jspb.BinaryReader(bytes);
  var msg = new proto.g8e.operator.v1.PortCheckEntry;
  return proto.g8e.operator.v1.PortCheckEntry.deserializeBinaryFromReader(msg, reader);
};


/**
 * Deserializes binary data (in protobuf wire format) from the
 * given reader into the given message object.
 * @param {!proto.g8e.operator.v1.PortCheckEntry} msg The message object to deserialize into.
 * @param {!jspb.BinaryReader} reader The BinaryReader to use.
 * @return {!proto.g8e.operator.v1.PortCheckEntry}
 */
proto.g8e.operator.v1.PortCheckEntry.deserializeBinaryFromReader = function(msg, reader) {
  while (reader.nextField()) {
    if (reader.isEndGroup()) {
      break;
    }
    var field = reader.getFieldNumber();
    switch (field) {
    case 1:
      var value = /** @type {string} */ (reader.readString());
      msg.setHost(value);
      break;
    case 2:
      var value = /** @type {number} */ (reader.readInt32());
      msg.setPort(value);
      break;
    case 3:
      var value = /** @type {boolean} */ (reader.readBool());
      msg.setOpen(value);
      break;
    case 4:
      var value = /** @type {number} */ (reader.readFloat());
      msg.setLatencyMs(value);
      break;
    case 5:
      var value = /** @type {string} */ (reader.readString());
      msg.setError(value);
      break;
    default:
      reader.skipField();
      break;
    }
  }
  return msg;
};


/**
 * Serializes the message to binary data (in protobuf wire format).
 * @return {!Uint8Array}
 */
proto.g8e.operator.v1.PortCheckEntry.prototype.serializeBinary = function() {
  var writer = new jspb.BinaryWriter();
  proto.g8e.operator.v1.PortCheckEntry.serializeBinaryToWriter(this, writer);
  return writer.getResultBuffer();
};


/**
 * Serializes the given message to binary data (in protobuf wire
 * format), writing to the given BinaryWriter.
 * @param {!proto.g8e.operator.v1.PortCheckEntry} message
 * @param {!jspb.BinaryWriter} writer
 * @suppress {unusedLocalVariables} f is only used for nested messages
 */
proto.g8e.operator.v1.PortCheckEntry.serializeBinaryToWriter = function(message, writer) {
  var f = undefined;
  f = message.getHost();
  if (f.length > 0) {
    writer.writeString(
      1,
      f
    );
  }
  f = message.getPort();
  if (f !== 0) {
    writer.writeInt32(
      2,
      f
    );
  }
  f = message.getOpen();
  if (f) {
    writer.writeBool(
      3,
      f
    );
  }
  f = message.getLatencyMs();
  if (f !== 0.0) {
    writer.writeFloat(
      4,
      f
    );
  }
  f = message.getError();
  if (f.length > 0) {
    writer.writeString(
      5,
      f
    );
  }
};


/**
 * optional string host = 1;
 * @return {string}
 */
proto.g8e.operator.v1.PortCheckEntry.prototype.getHost = function() {
  return /** @type {string} */ (jspb.Message.getFieldWithDefault(this, 1, ""));
};


/**
 * @param {string} value
 * @return {!proto.g8e.operator.v1.PortCheckEntry} returns this
 */
proto.g8e.operator.v1.PortCheckEntry.prototype.setHost = function(value) {
  return jspb.Message.setProto3StringField(this, 1, value);
};


/**
 * optional int32 port = 2;
 * @return {number}
 */
proto.g8e.operator.v1.PortCheckEntry.prototype.getPort = function() {
  return /** @type {number} */ (jspb.Message.getFieldWithDefault(this, 2, 0));
};


/**
 * @param {number} value
 * @return {!proto.g8e.operator.v1.PortCheckEntry} returns this
 */
proto.g8e.operator.v1.PortCheckEntry.prototype.setPort = function(value) {
  return jspb.Message.setProto3IntField(this, 2, value);
};


/**
 * optional bool open = 3;
 * @return {boolean}
 */
proto.g8e.operator.v1.PortCheckEntry.prototype.getOpen = function() {
  return /** @type {boolean} */ (jspb.Message.getBooleanFieldWithDefault(this, 3, false));
};


/**
 * @param {boolean} value
 * @return {!proto.g8e.operator.v1.PortCheckEntry} returns this
 */
proto.g8e.operator.v1.PortCheckEntry.prototype.setOpen = function(value) {
  return jspb.Message.setProto3BooleanField(this, 3, value);
};


/**
 * optional float latency_ms = 4;
 * @return {number}
 */
proto.g8e.operator.v1.PortCheckEntry.prototype.getLatencyMs = function() {
  return /** @type {number} */ (jspb.Message.getFloatingPointFieldWithDefault(this, 4, 0.0));
};


/**
 * @param {number} value
 * @return {!proto.g8e.operator.v1.PortCheckEntry} returns this
 */
proto.g8e.operator.v1.PortCheckEntry.prototype.setLatencyMs = function(value) {
  return jspb.Message.setProto3FloatField(this, 4, value);
};


/**
 * optional string error = 5;
 * @return {string}
 */
proto.g8e.operator.v1.PortCheckEntry.prototype.getError = function() {
  return /** @type {string} */ (jspb.Message.getFieldWithDefault(this, 5, ""));
};


/**
 * @param {string} value
 * @return {!proto.g8e.operator.v1.PortCheckEntry} returns this
 */
proto.g8e.operator.v1.PortCheckEntry.prototype.setError = function(value) {
  return jspb.Message.setProto3StringField(this, 5, value);
};



/**
 * List of repeated fields within this message type.
 * @private {!Array<number>}
 * @const
 */
proto.g8e.operator.v1.PortCheckResult.repeatedFields_ = [3];



if (jspb.Message.GENERATE_TO_OBJECT) {
/**
 * Creates an object representation of this proto.
 * Field names that are reserved in JavaScript and will be renamed to pb_name.
 * Optional fields that are not set will be set to undefined.
 * To access a reserved field use, foo.pb_<name>, eg, foo.pb_default.
 * For the list of reserved names please see:
 *     net/proto2/compiler/js/internal/generator.cc#kKeyword.
 * @param {boolean=} opt_includeInstance Deprecated. whether to include the
 *     JSPB instance for transitional soy proto support:
 *     http://goto/soy-param-migration
 * @return {!Object}
 */
proto.g8e.operator.v1.PortCheckResult.prototype.toObject = function(opt_includeInstance) {
  return proto.g8e.operator.v1.PortCheckResult.toObject(opt_includeInstance, this);
};


/**
 * Static version of the {@see toObject} method.
 * @param {boolean|undefined} includeInstance Deprecated. Whether to include
 *     the JSPB instance for transitional soy proto support:
 *     http://goto/soy-param-migration
 * @param {!proto.g8e.operator.v1.PortCheckResult} msg The msg instance to transform.
 * @return {!Object}
 * @suppress {unusedLocalVariables} f is only used for nested messages
 */
proto.g8e.operator.v1.PortCheckResult.toObject = function(includeInstance, msg) {
  var f, obj = {
    executionId: jspb.Message.getFieldWithDefault(msg, 1, ""),
    status: jspb.Message.getFieldWithDefault(msg, 2, 0),
    resultsList: jspb.Message.toObjectList(msg.getResultsList(),
    proto.g8e.operator.v1.PortCheckEntry.toObject, includeInstance),
    errorMessage: jspb.Message.getFieldWithDefault(msg, 4, ""),
    errorType: jspb.Message.getFieldWithDefault(msg, 5, "")
  };

  if (includeInstance) {
    obj.$jspbMessageInstance = msg;
  }
  return obj;
};
}


/**
 * Deserializes binary data (in protobuf wire format).
 * @param {jspb.ByteSource} bytes The bytes to deserialize.
 * @return {!proto.g8e.operator.v1.PortCheckResult}
 */
proto.g8e.operator.v1.PortCheckResult.deserializeBinary = function(bytes) {
  var reader = new jspb.BinaryReader(bytes);
  var msg = new proto.g8e.operator.v1.PortCheckResult;
  return proto.g8e.operator.v1.PortCheckResult.deserializeBinaryFromReader(msg, reader);
};


/**
 * Deserializes binary data (in protobuf wire format) from the
 * given reader into the given message object.
 * @param {!proto.g8e.operator.v1.PortCheckResult} msg The message object to deserialize into.
 * @param {!jspb.BinaryReader} reader The BinaryReader to use.
 * @return {!proto.g8e.operator.v1.PortCheckResult}
 */
proto.g8e.operator.v1.PortCheckResult.deserializeBinaryFromReader = function(msg, reader) {
  while (reader.nextField()) {
    if (reader.isEndGroup()) {
      break;
    }
    var field = reader.getFieldNumber();
    switch (field) {
    case 1:
      var value = /** @type {string} */ (reader.readString());
      msg.setExecutionId(value);
      break;
    case 2:
      var value = /** @type {!proto.g8e.operator.v1.ExecutionStatus} */ (reader.readEnum());
      msg.setStatus(value);
      break;
    case 3:
      var value = new proto.g8e.operator.v1.PortCheckEntry;
      reader.readMessage(value,proto.g8e.operator.v1.PortCheckEntry.deserializeBinaryFromReader);
      msg.addResults(value);
      break;
    case 4:
      var value = /** @type {string} */ (reader.readString());
      msg.setErrorMessage(value);
      break;
    case 5:
      var value = /** @type {string} */ (reader.readString());
      msg.setErrorType(value);
      break;
    default:
      reader.skipField();
      break;
    }
  }
  return msg;
};


/**
 * Serializes the message to binary data (in protobuf wire format).
 * @return {!Uint8Array}
 */
proto.g8e.operator.v1.PortCheckResult.prototype.serializeBinary = function() {
  var writer = new jspb.BinaryWriter();
  proto.g8e.operator.v1.PortCheckResult.serializeBinaryToWriter(this, writer);
  return writer.getResultBuffer();
};


/**
 * Serializes the given message to binary data (in protobuf wire
 * format), writing to the given BinaryWriter.
 * @param {!proto.g8e.operator.v1.PortCheckResult} message
 * @param {!jspb.BinaryWriter} writer
 * @suppress {unusedLocalVariables} f is only used for nested messages
 */
proto.g8e.operator.v1.PortCheckResult.serializeBinaryToWriter = function(message, writer) {
  var f = undefined;
  f = message.getExecutionId();
  if (f.length > 0) {
    writer.writeString(
      1,
      f
    );
  }
  f = message.getStatus();
  if (f !== 0.0) {
    writer.writeEnum(
      2,
      f
    );
  }
  f = message.getResultsList();
  if (f.length > 0) {
    writer.writeRepeatedMessage(
      3,
      f,
      proto.g8e.operator.v1.PortCheckEntry.serializeBinaryToWriter
    );
  }
  f = message.getErrorMessage();
  if (f.length > 0) {
    writer.writeString(
      4,
      f
    );
  }
  f = message.getErrorType();
  if (f.length > 0) {
    writer.writeString(
      5,
      f
    );
  }
};


/**
 * optional string execution_id = 1;
 * @return {string}
 */
proto.g8e.operator.v1.PortCheckResult.prototype.getExecutionId = function() {
  return /** @type {string} */ (jspb.Message.getFieldWithDefault(this, 1, ""));
};


/**
 * @param {string} value
 * @return {!proto.g8e.operator.v1.PortCheckResult} returns this
 */
proto.g8e.operator.v1.PortCheckResult.prototype.setExecutionId = function(value) {
  return jspb.Message.setProto3StringField(this, 1, value);
};


/**
 * optional ExecutionStatus status = 2;
 * @return {!proto.g8e.operator.v1.ExecutionStatus}
 */
proto.g8e.operator.v1.PortCheckResult.prototype.getStatus = function() {
  return /** @type {!proto.g8e.operator.v1.ExecutionStatus} */ (jspb.Message.getFieldWithDefault(this, 2, 0));
};


/**
 * @param {!proto.g8e.operator.v1.ExecutionStatus} value
 * @return {!proto.g8e.operator.v1.PortCheckResult} returns this
 */
proto.g8e.operator.v1.PortCheckResult.prototype.setStatus = function(value) {
  return jspb.Message.setProto3EnumField(this, 2, value);
};


/**
 * repeated PortCheckEntry results = 3;
 * @return {!Array<!proto.g8e.operator.v1.PortCheckEntry>}
 */
proto.g8e.operator.v1.PortCheckResult.prototype.getResultsList = function() {
  return /** @type{!Array<!proto.g8e.operator.v1.PortCheckEntry>} */ (
    jspb.Message.getRepeatedWrapperField(this, proto.g8e.operator.v1.PortCheckEntry, 3));
};


/**
 * @param {!Array<!proto.g8e.operator.v1.PortCheckEntry>} value
 * @return {!proto.g8e.operator.v1.PortCheckResult} returns this
*/
proto.g8e.operator.v1.PortCheckResult.prototype.setResultsList = function(value) {
  return jspb.Message.setRepeatedWrapperField(this, 3, value);
};


/**
 * @param {!proto.g8e.operator.v1.PortCheckEntry=} opt_value
 * @param {number=} opt_index
 * @return {!proto.g8e.operator.v1.PortCheckEntry}
 */
proto.g8e.operator.v1.PortCheckResult.prototype.addResults = function(opt_value, opt_index) {
  return jspb.Message.addToRepeatedWrapperField(this, 3, opt_value, proto.g8e.operator.v1.PortCheckEntry, opt_index);
};


/**
 * Clears the list making it empty but non-null.
 * @return {!proto.g8e.operator.v1.PortCheckResult} returns this
 */
proto.g8e.operator.v1.PortCheckResult.prototype.clearResultsList = function() {
  return this.setResultsList([]);
};


/**
 * optional string error_message = 4;
 * @return {string}
 */
proto.g8e.operator.v1.PortCheckResult.prototype.getErrorMessage = function() {
  return /** @type {string} */ (jspb.Message.getFieldWithDefault(this, 4, ""));
};


/**
 * @param {string} value
 * @return {!proto.g8e.operator.v1.PortCheckResult} returns this
 */
proto.g8e.operator.v1.PortCheckResult.prototype.setErrorMessage = function(value) {
  return jspb.Message.setProto3StringField(this, 4, value);
};


/**
 * optional string error_type = 5;
 * @return {string}
 */
proto.g8e.operator.v1.PortCheckResult.prototype.getErrorType = function() {
  return /** @type {string} */ (jspb.Message.getFieldWithDefault(this, 5, ""));
};


/**
 * @param {string} value
 * @return {!proto.g8e.operator.v1.PortCheckResult} returns this
 */
proto.g8e.operator.v1.PortCheckResult.prototype.setErrorType = function(value) {
  return jspb.Message.setProto3StringField(this, 5, value);
};





if (jspb.Message.GENERATE_TO_OBJECT) {
/**
 * Creates an object representation of this proto.
 * Field names that are reserved in JavaScript and will be renamed to pb_name.
 * Optional fields that are not set will be set to undefined.
 * To access a reserved field use, foo.pb_<name>, eg, foo.pb_default.
 * For the list of reserved names please see:
 *     net/proto2/compiler/js/internal/generator.cc#kKeyword.
 * @param {boolean=} opt_includeInstance Deprecated. whether to include the
 *     JSPB instance for transitional soy proto support:
 *     http://goto/soy-param-migration
 * @return {!Object}
 */
proto.g8e.operator.v1.FetchLogsResult.prototype.toObject = function(opt_includeInstance) {
  return proto.g8e.operator.v1.FetchLogsResult.toObject(opt_includeInstance, this);
};


/**
 * Static version of the {@see toObject} method.
 * @param {boolean|undefined} includeInstance Deprecated. Whether to include
 *     the JSPB instance for transitional soy proto support:
 *     http://goto/soy-param-migration
 * @param {!proto.g8e.operator.v1.FetchLogsResult} msg The msg instance to transform.
 * @return {!Object}
 * @suppress {unusedLocalVariables} f is only used for nested messages
 */
proto.g8e.operator.v1.FetchLogsResult.toObject = function(includeInstance, msg) {
  var f, obj = {
    executionId: jspb.Message.getFieldWithDefault(msg, 1, ""),
    command: jspb.Message.getFieldWithDefault(msg, 2, ""),
    exitCode: jspb.Message.getFieldWithDefault(msg, 3, 0),
    durationMs: jspb.Message.getFieldWithDefault(msg, 4, 0),
    stdout: jspb.Message.getFieldWithDefault(msg, 5, ""),
    stderr: jspb.Message.getFieldWithDefault(msg, 6, ""),
    stdoutSize: jspb.Message.getFieldWithDefault(msg, 7, 0),
    stderrSize: jspb.Message.getFieldWithDefault(msg, 8, 0),
    timestamp: jspb.Message.getFieldWithDefault(msg, 9, ""),
    sentinelMode: jspb.Message.getFieldWithDefault(msg, 10, ""),
    error: jspb.Message.getFieldWithDefault(msg, 11, "")
  };

  if (includeInstance) {
    obj.$jspbMessageInstance = msg;
  }
  return obj;
};
}


/**
 * Deserializes binary data (in protobuf wire format).
 * @param {jspb.ByteSource} bytes The bytes to deserialize.
 * @return {!proto.g8e.operator.v1.FetchLogsResult}
 */
proto.g8e.operator.v1.FetchLogsResult.deserializeBinary = function(bytes) {
  var reader = new jspb.BinaryReader(bytes);
  var msg = new proto.g8e.operator.v1.FetchLogsResult;
  return proto.g8e.operator.v1.FetchLogsResult.deserializeBinaryFromReader(msg, reader);
};


/**
 * Deserializes binary data (in protobuf wire format) from the
 * given reader into the given message object.
 * @param {!proto.g8e.operator.v1.FetchLogsResult} msg The message object to deserialize into.
 * @param {!jspb.BinaryReader} reader The BinaryReader to use.
 * @return {!proto.g8e.operator.v1.FetchLogsResult}
 */
proto.g8e.operator.v1.FetchLogsResult.deserializeBinaryFromReader = function(msg, reader) {
  while (reader.nextField()) {
    if (reader.isEndGroup()) {
      break;
    }
    var field = reader.getFieldNumber();
    switch (field) {
    case 1:
      var value = /** @type {string} */ (reader.readString());
      msg.setExecutionId(value);
      break;
    case 2:
      var value = /** @type {string} */ (reader.readString());
      msg.setCommand(value);
      break;
    case 3:
      var value = /** @type {number} */ (reader.readInt32());
      msg.setExitCode(value);
      break;
    case 4:
      var value = /** @type {number} */ (reader.readInt64());
      msg.setDurationMs(value);
      break;
    case 5:
      var value = /** @type {string} */ (reader.readString());
      msg.setStdout(value);
      break;
    case 6:
      var value = /** @type {string} */ (reader.readString());
      msg.setStderr(value);
      break;
    case 7:
      var value = /** @type {number} */ (reader.readInt32());
      msg.setStdoutSize(value);
      break;
    case 8:
      var value = /** @type {number} */ (reader.readInt32());
      msg.setStderrSize(value);
      break;
    case 9:
      var value = /** @type {string} */ (reader.readString());
      msg.setTimestamp(value);
      break;
    case 10:
      var value = /** @type {string} */ (reader.readString());
      msg.setSentinelMode(value);
      break;
    case 11:
      var value = /** @type {string} */ (reader.readString());
      msg.setError(value);
      break;
    default:
      reader.skipField();
      break;
    }
  }
  return msg;
};


/**
 * Serializes the message to binary data (in protobuf wire format).
 * @return {!Uint8Array}
 */
proto.g8e.operator.v1.FetchLogsResult.prototype.serializeBinary = function() {
  var writer = new jspb.BinaryWriter();
  proto.g8e.operator.v1.FetchLogsResult.serializeBinaryToWriter(this, writer);
  return writer.getResultBuffer();
};


/**
 * Serializes the given message to binary data (in protobuf wire
 * format), writing to the given BinaryWriter.
 * @param {!proto.g8e.operator.v1.FetchLogsResult} message
 * @param {!jspb.BinaryWriter} writer
 * @suppress {unusedLocalVariables} f is only used for nested messages
 */
proto.g8e.operator.v1.FetchLogsResult.serializeBinaryToWriter = function(message, writer) {
  var f = undefined;
  f = message.getExecutionId();
  if (f.length > 0) {
    writer.writeString(
      1,
      f
    );
  }
  f = message.getCommand();
  if (f.length > 0) {
    writer.writeString(
      2,
      f
    );
  }
  f = message.getExitCode();
  if (f !== 0) {
    writer.writeInt32(
      3,
      f
    );
  }
  f = message.getDurationMs();
  if (f !== 0) {
    writer.writeInt64(
      4,
      f
    );
  }
  f = message.getStdout();
  if (f.length > 0) {
    writer.writeString(
      5,
      f
    );
  }
  f = message.getStderr();
  if (f.length > 0) {
    writer.writeString(
      6,
      f
    );
  }
  f = message.getStdoutSize();
  if (f !== 0) {
    writer.writeInt32(
      7,
      f
    );
  }
  f = message.getStderrSize();
  if (f !== 0) {
    writer.writeInt32(
      8,
      f
    );
  }
  f = message.getTimestamp();
  if (f.length > 0) {
    writer.writeString(
      9,
      f
    );
  }
  f = message.getSentinelMode();
  if (f.length > 0) {
    writer.writeString(
      10,
      f
    );
  }
  f = message.getError();
  if (f.length > 0) {
    writer.writeString(
      11,
      f
    );
  }
};


/**
 * optional string execution_id = 1;
 * @return {string}
 */
proto.g8e.operator.v1.FetchLogsResult.prototype.getExecutionId = function() {
  return /** @type {string} */ (jspb.Message.getFieldWithDefault(this, 1, ""));
};


/**
 * @param {string} value
 * @return {!proto.g8e.operator.v1.FetchLogsResult} returns this
 */
proto.g8e.operator.v1.FetchLogsResult.prototype.setExecutionId = function(value) {
  return jspb.Message.setProto3StringField(this, 1, value);
};


/**
 * optional string command = 2;
 * @return {string}
 */
proto.g8e.operator.v1.FetchLogsResult.prototype.getCommand = function() {
  return /** @type {string} */ (jspb.Message.getFieldWithDefault(this, 2, ""));
};


/**
 * @param {string} value
 * @return {!proto.g8e.operator.v1.FetchLogsResult} returns this
 */
proto.g8e.operator.v1.FetchLogsResult.prototype.setCommand = function(value) {
  return jspb.Message.setProto3StringField(this, 2, value);
};


/**
 * optional int32 exit_code = 3;
 * @return {number}
 */
proto.g8e.operator.v1.FetchLogsResult.prototype.getExitCode = function() {
  return /** @type {number} */ (jspb.Message.getFieldWithDefault(this, 3, 0));
};


/**
 * @param {number} value
 * @return {!proto.g8e.operator.v1.FetchLogsResult} returns this
 */
proto.g8e.operator.v1.FetchLogsResult.prototype.setExitCode = function(value) {
  return jspb.Message.setProto3IntField(this, 3, value);
};


/**
 * optional int64 duration_ms = 4;
 * @return {number}
 */
proto.g8e.operator.v1.FetchLogsResult.prototype.getDurationMs = function() {
  return /** @type {number} */ (jspb.Message.getFieldWithDefault(this, 4, 0));
};


/**
 * @param {number} value
 * @return {!proto.g8e.operator.v1.FetchLogsResult} returns this
 */
proto.g8e.operator.v1.FetchLogsResult.prototype.setDurationMs = function(value) {
  return jspb.Message.setProto3IntField(this, 4, value);
};


/**
 * optional string stdout = 5;
 * @return {string}
 */
proto.g8e.operator.v1.FetchLogsResult.prototype.getStdout = function() {
  return /** @type {string} */ (jspb.Message.getFieldWithDefault(this, 5, ""));
};


/**
 * @param {string} value
 * @return {!proto.g8e.operator.v1.FetchLogsResult} returns this
 */
proto.g8e.operator.v1.FetchLogsResult.prototype.setStdout = function(value) {
  return jspb.Message.setProto3StringField(this, 5, value);
};


/**
 * optional string stderr = 6;
 * @return {string}
 */
proto.g8e.operator.v1.FetchLogsResult.prototype.getStderr = function() {
  return /** @type {string} */ (jspb.Message.getFieldWithDefault(this, 6, ""));
};


/**
 * @param {string} value
 * @return {!proto.g8e.operator.v1.FetchLogsResult} returns this
 */
proto.g8e.operator.v1.FetchLogsResult.prototype.setStderr = function(value) {
  return jspb.Message.setProto3StringField(this, 6, value);
};


/**
 * optional int32 stdout_size = 7;
 * @return {number}
 */
proto.g8e.operator.v1.FetchLogsResult.prototype.getStdoutSize = function() {
  return /** @type {number} */ (jspb.Message.getFieldWithDefault(this, 7, 0));
};


/**
 * @param {number} value
 * @return {!proto.g8e.operator.v1.FetchLogsResult} returns this
 */
proto.g8e.operator.v1.FetchLogsResult.prototype.setStdoutSize = function(value) {
  return jspb.Message.setProto3IntField(this, 7, value);
};


/**
 * optional int32 stderr_size = 8;
 * @return {number}
 */
proto.g8e.operator.v1.FetchLogsResult.prototype.getStderrSize = function() {
  return /** @type {number} */ (jspb.Message.getFieldWithDefault(this, 8, 0));
};


/**
 * @param {number} value
 * @return {!proto.g8e.operator.v1.FetchLogsResult} returns this
 */
proto.g8e.operator.v1.FetchLogsResult.prototype.setStderrSize = function(value) {
  return jspb.Message.setProto3IntField(this, 8, value);
};


/**
 * optional string timestamp = 9;
 * @return {string}
 */
proto.g8e.operator.v1.FetchLogsResult.prototype.getTimestamp = function() {
  return /** @type {string} */ (jspb.Message.getFieldWithDefault(this, 9, ""));
};


/**
 * @param {string} value
 * @return {!proto.g8e.operator.v1.FetchLogsResult} returns this
 */
proto.g8e.operator.v1.FetchLogsResult.prototype.setTimestamp = function(value) {
  return jspb.Message.setProto3StringField(this, 9, value);
};


/**
 * optional string sentinel_mode = 10;
 * @return {string}
 */
proto.g8e.operator.v1.FetchLogsResult.prototype.getSentinelMode = function() {
  return /** @type {string} */ (jspb.Message.getFieldWithDefault(this, 10, ""));
};


/**
 * @param {string} value
 * @return {!proto.g8e.operator.v1.FetchLogsResult} returns this
 */
proto.g8e.operator.v1.FetchLogsResult.prototype.setSentinelMode = function(value) {
  return jspb.Message.setProto3StringField(this, 10, value);
};


/**
 * optional string error = 11;
 * @return {string}
 */
proto.g8e.operator.v1.FetchLogsResult.prototype.getError = function() {
  return /** @type {string} */ (jspb.Message.getFieldWithDefault(this, 11, ""));
};


/**
 * @param {string} value
 * @return {!proto.g8e.operator.v1.FetchLogsResult} returns this
 */
proto.g8e.operator.v1.FetchLogsResult.prototype.setError = function(value) {
  return jspb.Message.setProto3StringField(this, 11, value);
};





if (jspb.Message.GENERATE_TO_OBJECT) {
/**
 * Creates an object representation of this proto.
 * Field names that are reserved in JavaScript and will be renamed to pb_name.
 * Optional fields that are not set will be set to undefined.
 * To access a reserved field use, foo.pb_<name>, eg, foo.pb_default.
 * For the list of reserved names please see:
 *     net/proto2/compiler/js/internal/generator.cc#kKeyword.
 * @param {boolean=} opt_includeInstance Deprecated. whether to include the
 *     JSPB instance for transitional soy proto support:
 *     http://goto/soy-param-migration
 * @return {!Object}
 */
proto.g8e.operator.v1.AuditWebSession.prototype.toObject = function(opt_includeInstance) {
  return proto.g8e.operator.v1.AuditWebSession.toObject(opt_includeInstance, this);
};


/**
 * Static version of the {@see toObject} method.
 * @param {boolean|undefined} includeInstance Deprecated. Whether to include
 *     the JSPB instance for transitional soy proto support:
 *     http://goto/soy-param-migration
 * @param {!proto.g8e.operator.v1.AuditWebSession} msg The msg instance to transform.
 * @return {!Object}
 * @suppress {unusedLocalVariables} f is only used for nested messages
 */
proto.g8e.operator.v1.AuditWebSession.toObject = function(includeInstance, msg) {
  var f, obj = {
    id: jspb.Message.getFieldWithDefault(msg, 1, ""),
    title: jspb.Message.getFieldWithDefault(msg, 2, ""),
    createdAt: jspb.Message.getFieldWithDefault(msg, 3, ""),
    userIdentity: jspb.Message.getFieldWithDefault(msg, 4, "")
  };

  if (includeInstance) {
    obj.$jspbMessageInstance = msg;
  }
  return obj;
};
}


/**
 * Deserializes binary data (in protobuf wire format).
 * @param {jspb.ByteSource} bytes The bytes to deserialize.
 * @return {!proto.g8e.operator.v1.AuditWebSession}
 */
proto.g8e.operator.v1.AuditWebSession.deserializeBinary = function(bytes) {
  var reader = new jspb.BinaryReader(bytes);
  var msg = new proto.g8e.operator.v1.AuditWebSession;
  return proto.g8e.operator.v1.AuditWebSession.deserializeBinaryFromReader(msg, reader);
};


/**
 * Deserializes binary data (in protobuf wire format) from the
 * given reader into the given message object.
 * @param {!proto.g8e.operator.v1.AuditWebSession} msg The message object to deserialize into.
 * @param {!jspb.BinaryReader} reader The BinaryReader to use.
 * @return {!proto.g8e.operator.v1.AuditWebSession}
 */
proto.g8e.operator.v1.AuditWebSession.deserializeBinaryFromReader = function(msg, reader) {
  while (reader.nextField()) {
    if (reader.isEndGroup()) {
      break;
    }
    var field = reader.getFieldNumber();
    switch (field) {
    case 1:
      var value = /** @type {string} */ (reader.readString());
      msg.setId(value);
      break;
    case 2:
      var value = /** @type {string} */ (reader.readString());
      msg.setTitle(value);
      break;
    case 3:
      var value = /** @type {string} */ (reader.readString());
      msg.setCreatedAt(value);
      break;
    case 4:
      var value = /** @type {string} */ (reader.readString());
      msg.setUserIdentity(value);
      break;
    default:
      reader.skipField();
      break;
    }
  }
  return msg;
};


/**
 * Serializes the message to binary data (in protobuf wire format).
 * @return {!Uint8Array}
 */
proto.g8e.operator.v1.AuditWebSession.prototype.serializeBinary = function() {
  var writer = new jspb.BinaryWriter();
  proto.g8e.operator.v1.AuditWebSession.serializeBinaryToWriter(this, writer);
  return writer.getResultBuffer();
};


/**
 * Serializes the given message to binary data (in protobuf wire
 * format), writing to the given BinaryWriter.
 * @param {!proto.g8e.operator.v1.AuditWebSession} message
 * @param {!jspb.BinaryWriter} writer
 * @suppress {unusedLocalVariables} f is only used for nested messages
 */
proto.g8e.operator.v1.AuditWebSession.serializeBinaryToWriter = function(message, writer) {
  var f = undefined;
  f = message.getId();
  if (f.length > 0) {
    writer.writeString(
      1,
      f
    );
  }
  f = message.getTitle();
  if (f.length > 0) {
    writer.writeString(
      2,
      f
    );
  }
  f = message.getCreatedAt();
  if (f.length > 0) {
    writer.writeString(
      3,
      f
    );
  }
  f = message.getUserIdentity();
  if (f.length > 0) {
    writer.writeString(
      4,
      f
    );
  }
};


/**
 * optional string id = 1;
 * @return {string}
 */
proto.g8e.operator.v1.AuditWebSession.prototype.getId = function() {
  return /** @type {string} */ (jspb.Message.getFieldWithDefault(this, 1, ""));
};


/**
 * @param {string} value
 * @return {!proto.g8e.operator.v1.AuditWebSession} returns this
 */
proto.g8e.operator.v1.AuditWebSession.prototype.setId = function(value) {
  return jspb.Message.setProto3StringField(this, 1, value);
};


/**
 * optional string title = 2;
 * @return {string}
 */
proto.g8e.operator.v1.AuditWebSession.prototype.getTitle = function() {
  return /** @type {string} */ (jspb.Message.getFieldWithDefault(this, 2, ""));
};


/**
 * @param {string} value
 * @return {!proto.g8e.operator.v1.AuditWebSession} returns this
 */
proto.g8e.operator.v1.AuditWebSession.prototype.setTitle = function(value) {
  return jspb.Message.setProto3StringField(this, 2, value);
};


/**
 * optional string created_at = 3;
 * @return {string}
 */
proto.g8e.operator.v1.AuditWebSession.prototype.getCreatedAt = function() {
  return /** @type {string} */ (jspb.Message.getFieldWithDefault(this, 3, ""));
};


/**
 * @param {string} value
 * @return {!proto.g8e.operator.v1.AuditWebSession} returns this
 */
proto.g8e.operator.v1.AuditWebSession.prototype.setCreatedAt = function(value) {
  return jspb.Message.setProto3StringField(this, 3, value);
};


/**
 * optional string user_identity = 4;
 * @return {string}
 */
proto.g8e.operator.v1.AuditWebSession.prototype.getUserIdentity = function() {
  return /** @type {string} */ (jspb.Message.getFieldWithDefault(this, 4, ""));
};


/**
 * @param {string} value
 * @return {!proto.g8e.operator.v1.AuditWebSession} returns this
 */
proto.g8e.operator.v1.AuditWebSession.prototype.setUserIdentity = function(value) {
  return jspb.Message.setProto3StringField(this, 4, value);
};





if (jspb.Message.GENERATE_TO_OBJECT) {
/**
 * Creates an object representation of this proto.
 * Field names that are reserved in JavaScript and will be renamed to pb_name.
 * Optional fields that are not set will be set to undefined.
 * To access a reserved field use, foo.pb_<name>, eg, foo.pb_default.
 * For the list of reserved names please see:
 *     net/proto2/compiler/js/internal/generator.cc#kKeyword.
 * @param {boolean=} opt_includeInstance Deprecated. whether to include the
 *     JSPB instance for transitional soy proto support:
 *     http://goto/soy-param-migration
 * @return {!Object}
 */
proto.g8e.operator.v1.AuditFileMutation.prototype.toObject = function(opt_includeInstance) {
  return proto.g8e.operator.v1.AuditFileMutation.toObject(opt_includeInstance, this);
};


/**
 * Static version of the {@see toObject} method.
 * @param {boolean|undefined} includeInstance Deprecated. Whether to include
 *     the JSPB instance for transitional soy proto support:
 *     http://goto/soy-param-migration
 * @param {!proto.g8e.operator.v1.AuditFileMutation} msg The msg instance to transform.
 * @return {!Object}
 * @suppress {unusedLocalVariables} f is only used for nested messages
 */
proto.g8e.operator.v1.AuditFileMutation.toObject = function(includeInstance, msg) {
  var f, obj = {
    id: jspb.Message.getFieldWithDefault(msg, 1, 0),
    filepath: jspb.Message.getFieldWithDefault(msg, 2, ""),
    operation: jspb.Message.getFieldWithDefault(msg, 3, ""),
    ledgerHashBefore: jspb.Message.getFieldWithDefault(msg, 4, ""),
    ledgerHashAfter: jspb.Message.getFieldWithDefault(msg, 5, ""),
    diffStat: jspb.Message.getFieldWithDefault(msg, 6, "")
  };

  if (includeInstance) {
    obj.$jspbMessageInstance = msg;
  }
  return obj;
};
}


/**
 * Deserializes binary data (in protobuf wire format).
 * @param {jspb.ByteSource} bytes The bytes to deserialize.
 * @return {!proto.g8e.operator.v1.AuditFileMutation}
 */
proto.g8e.operator.v1.AuditFileMutation.deserializeBinary = function(bytes) {
  var reader = new jspb.BinaryReader(bytes);
  var msg = new proto.g8e.operator.v1.AuditFileMutation;
  return proto.g8e.operator.v1.AuditFileMutation.deserializeBinaryFromReader(msg, reader);
};


/**
 * Deserializes binary data (in protobuf wire format) from the
 * given reader into the given message object.
 * @param {!proto.g8e.operator.v1.AuditFileMutation} msg The message object to deserialize into.
 * @param {!jspb.BinaryReader} reader The BinaryReader to use.
 * @return {!proto.g8e.operator.v1.AuditFileMutation}
 */
proto.g8e.operator.v1.AuditFileMutation.deserializeBinaryFromReader = function(msg, reader) {
  while (reader.nextField()) {
    if (reader.isEndGroup()) {
      break;
    }
    var field = reader.getFieldNumber();
    switch (field) {
    case 1:
      var value = /** @type {number} */ (reader.readInt64());
      msg.setId(value);
      break;
    case 2:
      var value = /** @type {string} */ (reader.readString());
      msg.setFilepath(value);
      break;
    case 3:
      var value = /** @type {string} */ (reader.readString());
      msg.setOperation(value);
      break;
    case 4:
      var value = /** @type {string} */ (reader.readString());
      msg.setLedgerHashBefore(value);
      break;
    case 5:
      var value = /** @type {string} */ (reader.readString());
      msg.setLedgerHashAfter(value);
      break;
    case 6:
      var value = /** @type {string} */ (reader.readString());
      msg.setDiffStat(value);
      break;
    default:
      reader.skipField();
      break;
    }
  }
  return msg;
};


/**
 * Serializes the message to binary data (in protobuf wire format).
 * @return {!Uint8Array}
 */
proto.g8e.operator.v1.AuditFileMutation.prototype.serializeBinary = function() {
  var writer = new jspb.BinaryWriter();
  proto.g8e.operator.v1.AuditFileMutation.serializeBinaryToWriter(this, writer);
  return writer.getResultBuffer();
};


/**
 * Serializes the given message to binary data (in protobuf wire
 * format), writing to the given BinaryWriter.
 * @param {!proto.g8e.operator.v1.AuditFileMutation} message
 * @param {!jspb.BinaryWriter} writer
 * @suppress {unusedLocalVariables} f is only used for nested messages
 */
proto.g8e.operator.v1.AuditFileMutation.serializeBinaryToWriter = function(message, writer) {
  var f = undefined;
  f = message.getId();
  if (f !== 0) {
    writer.writeInt64(
      1,
      f
    );
  }
  f = message.getFilepath();
  if (f.length > 0) {
    writer.writeString(
      2,
      f
    );
  }
  f = message.getOperation();
  if (f.length > 0) {
    writer.writeString(
      3,
      f
    );
  }
  f = message.getLedgerHashBefore();
  if (f.length > 0) {
    writer.writeString(
      4,
      f
    );
  }
  f = message.getLedgerHashAfter();
  if (f.length > 0) {
    writer.writeString(
      5,
      f
    );
  }
  f = message.getDiffStat();
  if (f.length > 0) {
    writer.writeString(
      6,
      f
    );
  }
};


/**
 * optional int64 id = 1;
 * @return {number}
 */
proto.g8e.operator.v1.AuditFileMutation.prototype.getId = function() {
  return /** @type {number} */ (jspb.Message.getFieldWithDefault(this, 1, 0));
};


/**
 * @param {number} value
 * @return {!proto.g8e.operator.v1.AuditFileMutation} returns this
 */
proto.g8e.operator.v1.AuditFileMutation.prototype.setId = function(value) {
  return jspb.Message.setProto3IntField(this, 1, value);
};


/**
 * optional string filepath = 2;
 * @return {string}
 */
proto.g8e.operator.v1.AuditFileMutation.prototype.getFilepath = function() {
  return /** @type {string} */ (jspb.Message.getFieldWithDefault(this, 2, ""));
};


/**
 * @param {string} value
 * @return {!proto.g8e.operator.v1.AuditFileMutation} returns this
 */
proto.g8e.operator.v1.AuditFileMutation.prototype.setFilepath = function(value) {
  return jspb.Message.setProto3StringField(this, 2, value);
};


/**
 * optional string operation = 3;
 * @return {string}
 */
proto.g8e.operator.v1.AuditFileMutation.prototype.getOperation = function() {
  return /** @type {string} */ (jspb.Message.getFieldWithDefault(this, 3, ""));
};


/**
 * @param {string} value
 * @return {!proto.g8e.operator.v1.AuditFileMutation} returns this
 */
proto.g8e.operator.v1.AuditFileMutation.prototype.setOperation = function(value) {
  return jspb.Message.setProto3StringField(this, 3, value);
};


/**
 * optional string ledger_hash_before = 4;
 * @return {string}
 */
proto.g8e.operator.v1.AuditFileMutation.prototype.getLedgerHashBefore = function() {
  return /** @type {string} */ (jspb.Message.getFieldWithDefault(this, 4, ""));
};


/**
 * @param {string} value
 * @return {!proto.g8e.operator.v1.AuditFileMutation} returns this
 */
proto.g8e.operator.v1.AuditFileMutation.prototype.setLedgerHashBefore = function(value) {
  return jspb.Message.setProto3StringField(this, 4, value);
};


/**
 * optional string ledger_hash_after = 5;
 * @return {string}
 */
proto.g8e.operator.v1.AuditFileMutation.prototype.getLedgerHashAfter = function() {
  return /** @type {string} */ (jspb.Message.getFieldWithDefault(this, 5, ""));
};


/**
 * @param {string} value
 * @return {!proto.g8e.operator.v1.AuditFileMutation} returns this
 */
proto.g8e.operator.v1.AuditFileMutation.prototype.setLedgerHashAfter = function(value) {
  return jspb.Message.setProto3StringField(this, 5, value);
};


/**
 * optional string diff_stat = 6;
 * @return {string}
 */
proto.g8e.operator.v1.AuditFileMutation.prototype.getDiffStat = function() {
  return /** @type {string} */ (jspb.Message.getFieldWithDefault(this, 6, ""));
};


/**
 * @param {string} value
 * @return {!proto.g8e.operator.v1.AuditFileMutation} returns this
 */
proto.g8e.operator.v1.AuditFileMutation.prototype.setDiffStat = function(value) {
  return jspb.Message.setProto3StringField(this, 6, value);
};



/**
 * List of repeated fields within this message type.
 * @private {!Array<number>}
 * @const
 */
proto.g8e.operator.v1.AuditEvent.repeatedFields_ = [14];



if (jspb.Message.GENERATE_TO_OBJECT) {
/**
 * Creates an object representation of this proto.
 * Field names that are reserved in JavaScript and will be renamed to pb_name.
 * Optional fields that are not set will be set to undefined.
 * To access a reserved field use, foo.pb_<name>, eg, foo.pb_default.
 * For the list of reserved names please see:
 *     net/proto2/compiler/js/internal/generator.cc#kKeyword.
 * @param {boolean=} opt_includeInstance Deprecated. whether to include the
 *     JSPB instance for transitional soy proto support:
 *     http://goto/soy-param-migration
 * @return {!Object}
 */
proto.g8e.operator.v1.AuditEvent.prototype.toObject = function(opt_includeInstance) {
  return proto.g8e.operator.v1.AuditEvent.toObject(opt_includeInstance, this);
};


/**
 * Static version of the {@see toObject} method.
 * @param {boolean|undefined} includeInstance Deprecated. Whether to include
 *     the JSPB instance for transitional soy proto support:
 *     http://goto/soy-param-migration
 * @param {!proto.g8e.operator.v1.AuditEvent} msg The msg instance to transform.
 * @return {!Object}
 * @suppress {unusedLocalVariables} f is only used for nested messages
 */
proto.g8e.operator.v1.AuditEvent.toObject = function(includeInstance, msg) {
  var f, obj = {
    id: jspb.Message.getFieldWithDefault(msg, 1, 0),
    operatorSessionId: jspb.Message.getFieldWithDefault(msg, 2, ""),
    timestamp: jspb.Message.getFieldWithDefault(msg, 3, ""),
    type: jspb.Message.getFieldWithDefault(msg, 4, ""),
    contentText: jspb.Message.getFieldWithDefault(msg, 5, ""),
    commandRaw: jspb.Message.getFieldWithDefault(msg, 6, ""),
    commandExitCode: jspb.Message.getFieldWithDefault(msg, 7, 0),
    commandStdout: jspb.Message.getFieldWithDefault(msg, 8, ""),
    commandStderr: jspb.Message.getFieldWithDefault(msg, 9, ""),
    executionDurationMs: jspb.Message.getFieldWithDefault(msg, 10, 0),
    storedLocally: jspb.Message.getBooleanFieldWithDefault(msg, 11, false),
    stdoutTruncated: jspb.Message.getBooleanFieldWithDefault(msg, 12, false),
    stderrTruncated: jspb.Message.getBooleanFieldWithDefault(msg, 13, false),
    fileMutationsList: jspb.Message.toObjectList(msg.getFileMutationsList(),
    proto.g8e.operator.v1.AuditFileMutation.toObject, includeInstance)
  };

  if (includeInstance) {
    obj.$jspbMessageInstance = msg;
  }
  return obj;
};
}


/**
 * Deserializes binary data (in protobuf wire format).
 * @param {jspb.ByteSource} bytes The bytes to deserialize.
 * @return {!proto.g8e.operator.v1.AuditEvent}
 */
proto.g8e.operator.v1.AuditEvent.deserializeBinary = function(bytes) {
  var reader = new jspb.BinaryReader(bytes);
  var msg = new proto.g8e.operator.v1.AuditEvent;
  return proto.g8e.operator.v1.AuditEvent.deserializeBinaryFromReader(msg, reader);
};


/**
 * Deserializes binary data (in protobuf wire format) from the
 * given reader into the given message object.
 * @param {!proto.g8e.operator.v1.AuditEvent} msg The message object to deserialize into.
 * @param {!jspb.BinaryReader} reader The BinaryReader to use.
 * @return {!proto.g8e.operator.v1.AuditEvent}
 */
proto.g8e.operator.v1.AuditEvent.deserializeBinaryFromReader = function(msg, reader) {
  while (reader.nextField()) {
    if (reader.isEndGroup()) {
      break;
    }
    var field = reader.getFieldNumber();
    switch (field) {
    case 1:
      var value = /** @type {number} */ (reader.readInt64());
      msg.setId(value);
      break;
    case 2:
      var value = /** @type {string} */ (reader.readString());
      msg.setOperatorSessionId(value);
      break;
    case 3:
      var value = /** @type {string} */ (reader.readString());
      msg.setTimestamp(value);
      break;
    case 4:
      var value = /** @type {string} */ (reader.readString());
      msg.setType(value);
      break;
    case 5:
      var value = /** @type {string} */ (reader.readString());
      msg.setContentText(value);
      break;
    case 6:
      var value = /** @type {string} */ (reader.readString());
      msg.setCommandRaw(value);
      break;
    case 7:
      var value = /** @type {number} */ (reader.readInt32());
      msg.setCommandExitCode(value);
      break;
    case 8:
      var value = /** @type {string} */ (reader.readString());
      msg.setCommandStdout(value);
      break;
    case 9:
      var value = /** @type {string} */ (reader.readString());
      msg.setCommandStderr(value);
      break;
    case 10:
      var value = /** @type {number} */ (reader.readInt64());
      msg.setExecutionDurationMs(value);
      break;
    case 11:
      var value = /** @type {boolean} */ (reader.readBool());
      msg.setStoredLocally(value);
      break;
    case 12:
      var value = /** @type {boolean} */ (reader.readBool());
      msg.setStdoutTruncated(value);
      break;
    case 13:
      var value = /** @type {boolean} */ (reader.readBool());
      msg.setStderrTruncated(value);
      break;
    case 14:
      var value = new proto.g8e.operator.v1.AuditFileMutation;
      reader.readMessage(value,proto.g8e.operator.v1.AuditFileMutation.deserializeBinaryFromReader);
      msg.addFileMutations(value);
      break;
    default:
      reader.skipField();
      break;
    }
  }
  return msg;
};


/**
 * Serializes the message to binary data (in protobuf wire format).
 * @return {!Uint8Array}
 */
proto.g8e.operator.v1.AuditEvent.prototype.serializeBinary = function() {
  var writer = new jspb.BinaryWriter();
  proto.g8e.operator.v1.AuditEvent.serializeBinaryToWriter(this, writer);
  return writer.getResultBuffer();
};


/**
 * Serializes the given message to binary data (in protobuf wire
 * format), writing to the given BinaryWriter.
 * @param {!proto.g8e.operator.v1.AuditEvent} message
 * @param {!jspb.BinaryWriter} writer
 * @suppress {unusedLocalVariables} f is only used for nested messages
 */
proto.g8e.operator.v1.AuditEvent.serializeBinaryToWriter = function(message, writer) {
  var f = undefined;
  f = message.getId();
  if (f !== 0) {
    writer.writeInt64(
      1,
      f
    );
  }
  f = message.getOperatorSessionId();
  if (f.length > 0) {
    writer.writeString(
      2,
      f
    );
  }
  f = message.getTimestamp();
  if (f.length > 0) {
    writer.writeString(
      3,
      f
    );
  }
  f = message.getType();
  if (f.length > 0) {
    writer.writeString(
      4,
      f
    );
  }
  f = message.getContentText();
  if (f.length > 0) {
    writer.writeString(
      5,
      f
    );
  }
  f = message.getCommandRaw();
  if (f.length > 0) {
    writer.writeString(
      6,
      f
    );
  }
  f = message.getCommandExitCode();
  if (f !== 0) {
    writer.writeInt32(
      7,
      f
    );
  }
  f = message.getCommandStdout();
  if (f.length > 0) {
    writer.writeString(
      8,
      f
    );
  }
  f = message.getCommandStderr();
  if (f.length > 0) {
    writer.writeString(
      9,
      f
    );
  }
  f = message.getExecutionDurationMs();
  if (f !== 0) {
    writer.writeInt64(
      10,
      f
    );
  }
  f = message.getStoredLocally();
  if (f) {
    writer.writeBool(
      11,
      f
    );
  }
  f = message.getStdoutTruncated();
  if (f) {
    writer.writeBool(
      12,
      f
    );
  }
  f = message.getStderrTruncated();
  if (f) {
    writer.writeBool(
      13,
      f
    );
  }
  f = message.getFileMutationsList();
  if (f.length > 0) {
    writer.writeRepeatedMessage(
      14,
      f,
      proto.g8e.operator.v1.AuditFileMutation.serializeBinaryToWriter
    );
  }
};


/**
 * optional int64 id = 1;
 * @return {number}
 */
proto.g8e.operator.v1.AuditEvent.prototype.getId = function() {
  return /** @type {number} */ (jspb.Message.getFieldWithDefault(this, 1, 0));
};


/**
 * @param {number} value
 * @return {!proto.g8e.operator.v1.AuditEvent} returns this
 */
proto.g8e.operator.v1.AuditEvent.prototype.setId = function(value) {
  return jspb.Message.setProto3IntField(this, 1, value);
};


/**
 * optional string operator_session_id = 2;
 * @return {string}
 */
proto.g8e.operator.v1.AuditEvent.prototype.getOperatorSessionId = function() {
  return /** @type {string} */ (jspb.Message.getFieldWithDefault(this, 2, ""));
};


/**
 * @param {string} value
 * @return {!proto.g8e.operator.v1.AuditEvent} returns this
 */
proto.g8e.operator.v1.AuditEvent.prototype.setOperatorSessionId = function(value) {
  return jspb.Message.setProto3StringField(this, 2, value);
};


/**
 * optional string timestamp = 3;
 * @return {string}
 */
proto.g8e.operator.v1.AuditEvent.prototype.getTimestamp = function() {
  return /** @type {string} */ (jspb.Message.getFieldWithDefault(this, 3, ""));
};


/**
 * @param {string} value
 * @return {!proto.g8e.operator.v1.AuditEvent} returns this
 */
proto.g8e.operator.v1.AuditEvent.prototype.setTimestamp = function(value) {
  return jspb.Message.setProto3StringField(this, 3, value);
};


/**
 * optional string type = 4;
 * @return {string}
 */
proto.g8e.operator.v1.AuditEvent.prototype.getType = function() {
  return /** @type {string} */ (jspb.Message.getFieldWithDefault(this, 4, ""));
};


/**
 * @param {string} value
 * @return {!proto.g8e.operator.v1.AuditEvent} returns this
 */
proto.g8e.operator.v1.AuditEvent.prototype.setType = function(value) {
  return jspb.Message.setProto3StringField(this, 4, value);
};


/**
 * optional string content_text = 5;
 * @return {string}
 */
proto.g8e.operator.v1.AuditEvent.prototype.getContentText = function() {
  return /** @type {string} */ (jspb.Message.getFieldWithDefault(this, 5, ""));
};


/**
 * @param {string} value
 * @return {!proto.g8e.operator.v1.AuditEvent} returns this
 */
proto.g8e.operator.v1.AuditEvent.prototype.setContentText = function(value) {
  return jspb.Message.setProto3StringField(this, 5, value);
};


/**
 * optional string command_raw = 6;
 * @return {string}
 */
proto.g8e.operator.v1.AuditEvent.prototype.getCommandRaw = function() {
  return /** @type {string} */ (jspb.Message.getFieldWithDefault(this, 6, ""));
};


/**
 * @param {string} value
 * @return {!proto.g8e.operator.v1.AuditEvent} returns this
 */
proto.g8e.operator.v1.AuditEvent.prototype.setCommandRaw = function(value) {
  return jspb.Message.setProto3StringField(this, 6, value);
};


/**
 * optional int32 command_exit_code = 7;
 * @return {number}
 */
proto.g8e.operator.v1.AuditEvent.prototype.getCommandExitCode = function() {
  return /** @type {number} */ (jspb.Message.getFieldWithDefault(this, 7, 0));
};


/**
 * @param {number} value
 * @return {!proto.g8e.operator.v1.AuditEvent} returns this
 */
proto.g8e.operator.v1.AuditEvent.prototype.setCommandExitCode = function(value) {
  return jspb.Message.setProto3IntField(this, 7, value);
};


/**
 * optional string command_stdout = 8;
 * @return {string}
 */
proto.g8e.operator.v1.AuditEvent.prototype.getCommandStdout = function() {
  return /** @type {string} */ (jspb.Message.getFieldWithDefault(this, 8, ""));
};


/**
 * @param {string} value
 * @return {!proto.g8e.operator.v1.AuditEvent} returns this
 */
proto.g8e.operator.v1.AuditEvent.prototype.setCommandStdout = function(value) {
  return jspb.Message.setProto3StringField(this, 8, value);
};


/**
 * optional string command_stderr = 9;
 * @return {string}
 */
proto.g8e.operator.v1.AuditEvent.prototype.getCommandStderr = function() {
  return /** @type {string} */ (jspb.Message.getFieldWithDefault(this, 9, ""));
};


/**
 * @param {string} value
 * @return {!proto.g8e.operator.v1.AuditEvent} returns this
 */
proto.g8e.operator.v1.AuditEvent.prototype.setCommandStderr = function(value) {
  return jspb.Message.setProto3StringField(this, 9, value);
};


/**
 * optional int64 execution_duration_ms = 10;
 * @return {number}
 */
proto.g8e.operator.v1.AuditEvent.prototype.getExecutionDurationMs = function() {
  return /** @type {number} */ (jspb.Message.getFieldWithDefault(this, 10, 0));
};


/**
 * @param {number} value
 * @return {!proto.g8e.operator.v1.AuditEvent} returns this
 */
proto.g8e.operator.v1.AuditEvent.prototype.setExecutionDurationMs = function(value) {
  return jspb.Message.setProto3IntField(this, 10, value);
};


/**
 * optional bool stored_locally = 11;
 * @return {boolean}
 */
proto.g8e.operator.v1.AuditEvent.prototype.getStoredLocally = function() {
  return /** @type {boolean} */ (jspb.Message.getBooleanFieldWithDefault(this, 11, false));
};


/**
 * @param {boolean} value
 * @return {!proto.g8e.operator.v1.AuditEvent} returns this
 */
proto.g8e.operator.v1.AuditEvent.prototype.setStoredLocally = function(value) {
  return jspb.Message.setProto3BooleanField(this, 11, value);
};


/**
 * optional bool stdout_truncated = 12;
 * @return {boolean}
 */
proto.g8e.operator.v1.AuditEvent.prototype.getStdoutTruncated = function() {
  return /** @type {boolean} */ (jspb.Message.getBooleanFieldWithDefault(this, 12, false));
};


/**
 * @param {boolean} value
 * @return {!proto.g8e.operator.v1.AuditEvent} returns this
 */
proto.g8e.operator.v1.AuditEvent.prototype.setStdoutTruncated = function(value) {
  return jspb.Message.setProto3BooleanField(this, 12, value);
};


/**
 * optional bool stderr_truncated = 13;
 * @return {boolean}
 */
proto.g8e.operator.v1.AuditEvent.prototype.getStderrTruncated = function() {
  return /** @type {boolean} */ (jspb.Message.getBooleanFieldWithDefault(this, 13, false));
};


/**
 * @param {boolean} value
 * @return {!proto.g8e.operator.v1.AuditEvent} returns this
 */
proto.g8e.operator.v1.AuditEvent.prototype.setStderrTruncated = function(value) {
  return jspb.Message.setProto3BooleanField(this, 13, value);
};


/**
 * repeated AuditFileMutation file_mutations = 14;
 * @return {!Array<!proto.g8e.operator.v1.AuditFileMutation>}
 */
proto.g8e.operator.v1.AuditEvent.prototype.getFileMutationsList = function() {
  return /** @type{!Array<!proto.g8e.operator.v1.AuditFileMutation>} */ (
    jspb.Message.getRepeatedWrapperField(this, proto.g8e.operator.v1.AuditFileMutation, 14));
};


/**
 * @param {!Array<!proto.g8e.operator.v1.AuditFileMutation>} value
 * @return {!proto.g8e.operator.v1.AuditEvent} returns this
*/
proto.g8e.operator.v1.AuditEvent.prototype.setFileMutationsList = function(value) {
  return jspb.Message.setRepeatedWrapperField(this, 14, value);
};


/**
 * @param {!proto.g8e.operator.v1.AuditFileMutation=} opt_value
 * @param {number=} opt_index
 * @return {!proto.g8e.operator.v1.AuditFileMutation}
 */
proto.g8e.operator.v1.AuditEvent.prototype.addFileMutations = function(opt_value, opt_index) {
  return jspb.Message.addToRepeatedWrapperField(this, 14, opt_value, proto.g8e.operator.v1.AuditFileMutation, opt_index);
};


/**
 * Clears the list making it empty but non-null.
 * @return {!proto.g8e.operator.v1.AuditEvent} returns this
 */
proto.g8e.operator.v1.AuditEvent.prototype.clearFileMutationsList = function() {
  return this.setFileMutationsList([]);
};



/**
 * List of repeated fields within this message type.
 * @private {!Array<number>}
 * @const
 */
proto.g8e.operator.v1.FetchHistoryResult.repeatedFields_ = [5];



if (jspb.Message.GENERATE_TO_OBJECT) {
/**
 * Creates an object representation of this proto.
 * Field names that are reserved in JavaScript and will be renamed to pb_name.
 * Optional fields that are not set will be set to undefined.
 * To access a reserved field use, foo.pb_<name>, eg, foo.pb_default.
 * For the list of reserved names please see:
 *     net/proto2/compiler/js/internal/generator.cc#kKeyword.
 * @param {boolean=} opt_includeInstance Deprecated. whether to include the
 *     JSPB instance for transitional soy proto support:
 *     http://goto/soy-param-migration
 * @return {!Object}
 */
proto.g8e.operator.v1.FetchHistoryResult.prototype.toObject = function(opt_includeInstance) {
  return proto.g8e.operator.v1.FetchHistoryResult.toObject(opt_includeInstance, this);
};


/**
 * Static version of the {@see toObject} method.
 * @param {boolean|undefined} includeInstance Deprecated. Whether to include
 *     the JSPB instance for transitional soy proto support:
 *     http://goto/soy-param-migration
 * @param {!proto.g8e.operator.v1.FetchHistoryResult} msg The msg instance to transform.
 * @return {!Object}
 * @suppress {unusedLocalVariables} f is only used for nested messages
 */
proto.g8e.operator.v1.FetchHistoryResult.toObject = function(includeInstance, msg) {
  var f, obj = {
    success: jspb.Message.getBooleanFieldWithDefault(msg, 1, false),
    executionId: jspb.Message.getFieldWithDefault(msg, 2, ""),
    operatorSessionId: jspb.Message.getFieldWithDefault(msg, 3, ""),
    webSession: (f = msg.getWebSession()) && proto.g8e.operator.v1.AuditWebSession.toObject(includeInstance, f),
    eventsList: jspb.Message.toObjectList(msg.getEventsList(),
    proto.g8e.operator.v1.AuditEvent.toObject, includeInstance),
    total: jspb.Message.getFieldWithDefault(msg, 6, 0),
    limit: jspb.Message.getFieldWithDefault(msg, 7, 0),
    offset: jspb.Message.getFieldWithDefault(msg, 8, 0),
    error: jspb.Message.getFieldWithDefault(msg, 9, "")
  };

  if (includeInstance) {
    obj.$jspbMessageInstance = msg;
  }
  return obj;
};
}


/**
 * Deserializes binary data (in protobuf wire format).
 * @param {jspb.ByteSource} bytes The bytes to deserialize.
 * @return {!proto.g8e.operator.v1.FetchHistoryResult}
 */
proto.g8e.operator.v1.FetchHistoryResult.deserializeBinary = function(bytes) {
  var reader = new jspb.BinaryReader(bytes);
  var msg = new proto.g8e.operator.v1.FetchHistoryResult;
  return proto.g8e.operator.v1.FetchHistoryResult.deserializeBinaryFromReader(msg, reader);
};


/**
 * Deserializes binary data (in protobuf wire format) from the
 * given reader into the given message object.
 * @param {!proto.g8e.operator.v1.FetchHistoryResult} msg The message object to deserialize into.
 * @param {!jspb.BinaryReader} reader The BinaryReader to use.
 * @return {!proto.g8e.operator.v1.FetchHistoryResult}
 */
proto.g8e.operator.v1.FetchHistoryResult.deserializeBinaryFromReader = function(msg, reader) {
  while (reader.nextField()) {
    if (reader.isEndGroup()) {
      break;
    }
    var field = reader.getFieldNumber();
    switch (field) {
    case 1:
      var value = /** @type {boolean} */ (reader.readBool());
      msg.setSuccess(value);
      break;
    case 2:
      var value = /** @type {string} */ (reader.readString());
      msg.setExecutionId(value);
      break;
    case 3:
      var value = /** @type {string} */ (reader.readString());
      msg.setOperatorSessionId(value);
      break;
    case 4:
      var value = new proto.g8e.operator.v1.AuditWebSession;
      reader.readMessage(value,proto.g8e.operator.v1.AuditWebSession.deserializeBinaryFromReader);
      msg.setWebSession(value);
      break;
    case 5:
      var value = new proto.g8e.operator.v1.AuditEvent;
      reader.readMessage(value,proto.g8e.operator.v1.AuditEvent.deserializeBinaryFromReader);
      msg.addEvents(value);
      break;
    case 6:
      var value = /** @type {number} */ (reader.readInt32());
      msg.setTotal(value);
      break;
    case 7:
      var value = /** @type {number} */ (reader.readInt32());
      msg.setLimit(value);
      break;
    case 8:
      var value = /** @type {number} */ (reader.readInt32());
      msg.setOffset(value);
      break;
    case 9:
      var value = /** @type {string} */ (reader.readString());
      msg.setError(value);
      break;
    default:
      reader.skipField();
      break;
    }
  }
  return msg;
};


/**
 * Serializes the message to binary data (in protobuf wire format).
 * @return {!Uint8Array}
 */
proto.g8e.operator.v1.FetchHistoryResult.prototype.serializeBinary = function() {
  var writer = new jspb.BinaryWriter();
  proto.g8e.operator.v1.FetchHistoryResult.serializeBinaryToWriter(this, writer);
  return writer.getResultBuffer();
};


/**
 * Serializes the given message to binary data (in protobuf wire
 * format), writing to the given BinaryWriter.
 * @param {!proto.g8e.operator.v1.FetchHistoryResult} message
 * @param {!jspb.BinaryWriter} writer
 * @suppress {unusedLocalVariables} f is only used for nested messages
 */
proto.g8e.operator.v1.FetchHistoryResult.serializeBinaryToWriter = function(message, writer) {
  var f = undefined;
  f = message.getSuccess();
  if (f) {
    writer.writeBool(
      1,
      f
    );
  }
  f = message.getExecutionId();
  if (f.length > 0) {
    writer.writeString(
      2,
      f
    );
  }
  f = message.getOperatorSessionId();
  if (f.length > 0) {
    writer.writeString(
      3,
      f
    );
  }
  f = message.getWebSession();
  if (f != null) {
    writer.writeMessage(
      4,
      f,
      proto.g8e.operator.v1.AuditWebSession.serializeBinaryToWriter
    );
  }
  f = message.getEventsList();
  if (f.length > 0) {
    writer.writeRepeatedMessage(
      5,
      f,
      proto.g8e.operator.v1.AuditEvent.serializeBinaryToWriter
    );
  }
  f = message.getTotal();
  if (f !== 0) {
    writer.writeInt32(
      6,
      f
    );
  }
  f = message.getLimit();
  if (f !== 0) {
    writer.writeInt32(
      7,
      f
    );
  }
  f = message.getOffset();
  if (f !== 0) {
    writer.writeInt32(
      8,
      f
    );
  }
  f = message.getError();
  if (f.length > 0) {
    writer.writeString(
      9,
      f
    );
  }
};


/**
 * optional bool success = 1;
 * @return {boolean}
 */
proto.g8e.operator.v1.FetchHistoryResult.prototype.getSuccess = function() {
  return /** @type {boolean} */ (jspb.Message.getBooleanFieldWithDefault(this, 1, false));
};


/**
 * @param {boolean} value
 * @return {!proto.g8e.operator.v1.FetchHistoryResult} returns this
 */
proto.g8e.operator.v1.FetchHistoryResult.prototype.setSuccess = function(value) {
  return jspb.Message.setProto3BooleanField(this, 1, value);
};


/**
 * optional string execution_id = 2;
 * @return {string}
 */
proto.g8e.operator.v1.FetchHistoryResult.prototype.getExecutionId = function() {
  return /** @type {string} */ (jspb.Message.getFieldWithDefault(this, 2, ""));
};


/**
 * @param {string} value
 * @return {!proto.g8e.operator.v1.FetchHistoryResult} returns this
 */
proto.g8e.operator.v1.FetchHistoryResult.prototype.setExecutionId = function(value) {
  return jspb.Message.setProto3StringField(this, 2, value);
};


/**
 * optional string operator_session_id = 3;
 * @return {string}
 */
proto.g8e.operator.v1.FetchHistoryResult.prototype.getOperatorSessionId = function() {
  return /** @type {string} */ (jspb.Message.getFieldWithDefault(this, 3, ""));
};


/**
 * @param {string} value
 * @return {!proto.g8e.operator.v1.FetchHistoryResult} returns this
 */
proto.g8e.operator.v1.FetchHistoryResult.prototype.setOperatorSessionId = function(value) {
  return jspb.Message.setProto3StringField(this, 3, value);
};


/**
 * optional AuditWebSession web_session = 4;
 * @return {?proto.g8e.operator.v1.AuditWebSession}
 */
proto.g8e.operator.v1.FetchHistoryResult.prototype.getWebSession = function() {
  return /** @type{?proto.g8e.operator.v1.AuditWebSession} */ (
    jspb.Message.getWrapperField(this, proto.g8e.operator.v1.AuditWebSession, 4));
};


/**
 * @param {?proto.g8e.operator.v1.AuditWebSession|undefined} value
 * @return {!proto.g8e.operator.v1.FetchHistoryResult} returns this
*/
proto.g8e.operator.v1.FetchHistoryResult.prototype.setWebSession = function(value) {
  return jspb.Message.setWrapperField(this, 4, value);
};


/**
 * Clears the message field making it undefined.
 * @return {!proto.g8e.operator.v1.FetchHistoryResult} returns this
 */
proto.g8e.operator.v1.FetchHistoryResult.prototype.clearWebSession = function() {
  return this.setWebSession(undefined);
};


/**
 * Returns whether this field is set.
 * @return {boolean}
 */
proto.g8e.operator.v1.FetchHistoryResult.prototype.hasWebSession = function() {
  return jspb.Message.getField(this, 4) != null;
};


/**
 * repeated AuditEvent events = 5;
 * @return {!Array<!proto.g8e.operator.v1.AuditEvent>}
 */
proto.g8e.operator.v1.FetchHistoryResult.prototype.getEventsList = function() {
  return /** @type{!Array<!proto.g8e.operator.v1.AuditEvent>} */ (
    jspb.Message.getRepeatedWrapperField(this, proto.g8e.operator.v1.AuditEvent, 5));
};


/**
 * @param {!Array<!proto.g8e.operator.v1.AuditEvent>} value
 * @return {!proto.g8e.operator.v1.FetchHistoryResult} returns this
*/
proto.g8e.operator.v1.FetchHistoryResult.prototype.setEventsList = function(value) {
  return jspb.Message.setRepeatedWrapperField(this, 5, value);
};


/**
 * @param {!proto.g8e.operator.v1.AuditEvent=} opt_value
 * @param {number=} opt_index
 * @return {!proto.g8e.operator.v1.AuditEvent}
 */
proto.g8e.operator.v1.FetchHistoryResult.prototype.addEvents = function(opt_value, opt_index) {
  return jspb.Message.addToRepeatedWrapperField(this, 5, opt_value, proto.g8e.operator.v1.AuditEvent, opt_index);
};


/**
 * Clears the list making it empty but non-null.
 * @return {!proto.g8e.operator.v1.FetchHistoryResult} returns this
 */
proto.g8e.operator.v1.FetchHistoryResult.prototype.clearEventsList = function() {
  return this.setEventsList([]);
};


/**
 * optional int32 total = 6;
 * @return {number}
 */
proto.g8e.operator.v1.FetchHistoryResult.prototype.getTotal = function() {
  return /** @type {number} */ (jspb.Message.getFieldWithDefault(this, 6, 0));
};


/**
 * @param {number} value
 * @return {!proto.g8e.operator.v1.FetchHistoryResult} returns this
 */
proto.g8e.operator.v1.FetchHistoryResult.prototype.setTotal = function(value) {
  return jspb.Message.setProto3IntField(this, 6, value);
};


/**
 * optional int32 limit = 7;
 * @return {number}
 */
proto.g8e.operator.v1.FetchHistoryResult.prototype.getLimit = function() {
  return /** @type {number} */ (jspb.Message.getFieldWithDefault(this, 7, 0));
};


/**
 * @param {number} value
 * @return {!proto.g8e.operator.v1.FetchHistoryResult} returns this
 */
proto.g8e.operator.v1.FetchHistoryResult.prototype.setLimit = function(value) {
  return jspb.Message.setProto3IntField(this, 7, value);
};


/**
 * optional int32 offset = 8;
 * @return {number}
 */
proto.g8e.operator.v1.FetchHistoryResult.prototype.getOffset = function() {
  return /** @type {number} */ (jspb.Message.getFieldWithDefault(this, 8, 0));
};


/**
 * @param {number} value
 * @return {!proto.g8e.operator.v1.FetchHistoryResult} returns this
 */
proto.g8e.operator.v1.FetchHistoryResult.prototype.setOffset = function(value) {
  return jspb.Message.setProto3IntField(this, 8, value);
};


/**
 * optional string error = 9;
 * @return {string}
 */
proto.g8e.operator.v1.FetchHistoryResult.prototype.getError = function() {
  return /** @type {string} */ (jspb.Message.getFieldWithDefault(this, 9, ""));
};


/**
 * @param {string} value
 * @return {!proto.g8e.operator.v1.FetchHistoryResult} returns this
 */
proto.g8e.operator.v1.FetchHistoryResult.prototype.setError = function(value) {
  return jspb.Message.setProto3StringField(this, 9, value);
};





if (jspb.Message.GENERATE_TO_OBJECT) {
/**
 * Creates an object representation of this proto.
 * Field names that are reserved in JavaScript and will be renamed to pb_name.
 * Optional fields that are not set will be set to undefined.
 * To access a reserved field use, foo.pb_<name>, eg, foo.pb_default.
 * For the list of reserved names please see:
 *     net/proto2/compiler/js/internal/generator.cc#kKeyword.
 * @param {boolean=} opt_includeInstance Deprecated. whether to include the
 *     JSPB instance for transitional soy proto support:
 *     http://goto/soy-param-migration
 * @return {!Object}
 */
proto.g8e.operator.v1.FileHistoryEntry.prototype.toObject = function(opt_includeInstance) {
  return proto.g8e.operator.v1.FileHistoryEntry.toObject(opt_includeInstance, this);
};


/**
 * Static version of the {@see toObject} method.
 * @param {boolean|undefined} includeInstance Deprecated. Whether to include
 *     the JSPB instance for transitional soy proto support:
 *     http://goto/soy-param-migration
 * @param {!proto.g8e.operator.v1.FileHistoryEntry} msg The msg instance to transform.
 * @return {!Object}
 * @suppress {unusedLocalVariables} f is only used for nested messages
 */
proto.g8e.operator.v1.FileHistoryEntry.toObject = function(includeInstance, msg) {
  var f, obj = {
    commitHash: jspb.Message.getFieldWithDefault(msg, 1, ""),
    timestamp: jspb.Message.getFieldWithDefault(msg, 2, ""),
    message: jspb.Message.getFieldWithDefault(msg, 3, "")
  };

  if (includeInstance) {
    obj.$jspbMessageInstance = msg;
  }
  return obj;
};
}


/**
 * Deserializes binary data (in protobuf wire format).
 * @param {jspb.ByteSource} bytes The bytes to deserialize.
 * @return {!proto.g8e.operator.v1.FileHistoryEntry}
 */
proto.g8e.operator.v1.FileHistoryEntry.deserializeBinary = function(bytes) {
  var reader = new jspb.BinaryReader(bytes);
  var msg = new proto.g8e.operator.v1.FileHistoryEntry;
  return proto.g8e.operator.v1.FileHistoryEntry.deserializeBinaryFromReader(msg, reader);
};


/**
 * Deserializes binary data (in protobuf wire format) from the
 * given reader into the given message object.
 * @param {!proto.g8e.operator.v1.FileHistoryEntry} msg The message object to deserialize into.
 * @param {!jspb.BinaryReader} reader The BinaryReader to use.
 * @return {!proto.g8e.operator.v1.FileHistoryEntry}
 */
proto.g8e.operator.v1.FileHistoryEntry.deserializeBinaryFromReader = function(msg, reader) {
  while (reader.nextField()) {
    if (reader.isEndGroup()) {
      break;
    }
    var field = reader.getFieldNumber();
    switch (field) {
    case 1:
      var value = /** @type {string} */ (reader.readString());
      msg.setCommitHash(value);
      break;
    case 2:
      var value = /** @type {string} */ (reader.readString());
      msg.setTimestamp(value);
      break;
    case 3:
      var value = /** @type {string} */ (reader.readString());
      msg.setMessage(value);
      break;
    default:
      reader.skipField();
      break;
    }
  }
  return msg;
};


/**
 * Serializes the message to binary data (in protobuf wire format).
 * @return {!Uint8Array}
 */
proto.g8e.operator.v1.FileHistoryEntry.prototype.serializeBinary = function() {
  var writer = new jspb.BinaryWriter();
  proto.g8e.operator.v1.FileHistoryEntry.serializeBinaryToWriter(this, writer);
  return writer.getResultBuffer();
};


/**
 * Serializes the given message to binary data (in protobuf wire
 * format), writing to the given BinaryWriter.
 * @param {!proto.g8e.operator.v1.FileHistoryEntry} message
 * @param {!jspb.BinaryWriter} writer
 * @suppress {unusedLocalVariables} f is only used for nested messages
 */
proto.g8e.operator.v1.FileHistoryEntry.serializeBinaryToWriter = function(message, writer) {
  var f = undefined;
  f = message.getCommitHash();
  if (f.length > 0) {
    writer.writeString(
      1,
      f
    );
  }
  f = message.getTimestamp();
  if (f.length > 0) {
    writer.writeString(
      2,
      f
    );
  }
  f = message.getMessage();
  if (f.length > 0) {
    writer.writeString(
      3,
      f
    );
  }
};


/**
 * optional string commit_hash = 1;
 * @return {string}
 */
proto.g8e.operator.v1.FileHistoryEntry.prototype.getCommitHash = function() {
  return /** @type {string} */ (jspb.Message.getFieldWithDefault(this, 1, ""));
};


/**
 * @param {string} value
 * @return {!proto.g8e.operator.v1.FileHistoryEntry} returns this
 */
proto.g8e.operator.v1.FileHistoryEntry.prototype.setCommitHash = function(value) {
  return jspb.Message.setProto3StringField(this, 1, value);
};


/**
 * optional string timestamp = 2;
 * @return {string}
 */
proto.g8e.operator.v1.FileHistoryEntry.prototype.getTimestamp = function() {
  return /** @type {string} */ (jspb.Message.getFieldWithDefault(this, 2, ""));
};


/**
 * @param {string} value
 * @return {!proto.g8e.operator.v1.FileHistoryEntry} returns this
 */
proto.g8e.operator.v1.FileHistoryEntry.prototype.setTimestamp = function(value) {
  return jspb.Message.setProto3StringField(this, 2, value);
};


/**
 * optional string message = 3;
 * @return {string}
 */
proto.g8e.operator.v1.FileHistoryEntry.prototype.getMessage = function() {
  return /** @type {string} */ (jspb.Message.getFieldWithDefault(this, 3, ""));
};


/**
 * @param {string} value
 * @return {!proto.g8e.operator.v1.FileHistoryEntry} returns this
 */
proto.g8e.operator.v1.FileHistoryEntry.prototype.setMessage = function(value) {
  return jspb.Message.setProto3StringField(this, 3, value);
};



/**
 * List of repeated fields within this message type.
 * @private {!Array<number>}
 * @const
 */
proto.g8e.operator.v1.FetchFileHistoryResult.repeatedFields_ = [4];



if (jspb.Message.GENERATE_TO_OBJECT) {
/**
 * Creates an object representation of this proto.
 * Field names that are reserved in JavaScript and will be renamed to pb_name.
 * Optional fields that are not set will be set to undefined.
 * To access a reserved field use, foo.pb_<name>, eg, foo.pb_default.
 * For the list of reserved names please see:
 *     net/proto2/compiler/js/internal/generator.cc#kKeyword.
 * @param {boolean=} opt_includeInstance Deprecated. whether to include the
 *     JSPB instance for transitional soy proto support:
 *     http://goto/soy-param-migration
 * @return {!Object}
 */
proto.g8e.operator.v1.FetchFileHistoryResult.prototype.toObject = function(opt_includeInstance) {
  return proto.g8e.operator.v1.FetchFileHistoryResult.toObject(opt_includeInstance, this);
};


/**
 * Static version of the {@see toObject} method.
 * @param {boolean|undefined} includeInstance Deprecated. Whether to include
 *     the JSPB instance for transitional soy proto support:
 *     http://goto/soy-param-migration
 * @param {!proto.g8e.operator.v1.FetchFileHistoryResult} msg The msg instance to transform.
 * @return {!Object}
 * @suppress {unusedLocalVariables} f is only used for nested messages
 */
proto.g8e.operator.v1.FetchFileHistoryResult.toObject = function(includeInstance, msg) {
  var f, obj = {
    success: jspb.Message.getBooleanFieldWithDefault(msg, 1, false),
    executionId: jspb.Message.getFieldWithDefault(msg, 2, ""),
    filePath: jspb.Message.getFieldWithDefault(msg, 3, ""),
    historyList: jspb.Message.toObjectList(msg.getHistoryList(),
    proto.g8e.operator.v1.FileHistoryEntry.toObject, includeInstance),
    error: jspb.Message.getFieldWithDefault(msg, 5, "")
  };

  if (includeInstance) {
    obj.$jspbMessageInstance = msg;
  }
  return obj;
};
}


/**
 * Deserializes binary data (in protobuf wire format).
 * @param {jspb.ByteSource} bytes The bytes to deserialize.
 * @return {!proto.g8e.operator.v1.FetchFileHistoryResult}
 */
proto.g8e.operator.v1.FetchFileHistoryResult.deserializeBinary = function(bytes) {
  var reader = new jspb.BinaryReader(bytes);
  var msg = new proto.g8e.operator.v1.FetchFileHistoryResult;
  return proto.g8e.operator.v1.FetchFileHistoryResult.deserializeBinaryFromReader(msg, reader);
};


/**
 * Deserializes binary data (in protobuf wire format) from the
 * given reader into the given message object.
 * @param {!proto.g8e.operator.v1.FetchFileHistoryResult} msg The message object to deserialize into.
 * @param {!jspb.BinaryReader} reader The BinaryReader to use.
 * @return {!proto.g8e.operator.v1.FetchFileHistoryResult}
 */
proto.g8e.operator.v1.FetchFileHistoryResult.deserializeBinaryFromReader = function(msg, reader) {
  while (reader.nextField()) {
    if (reader.isEndGroup()) {
      break;
    }
    var field = reader.getFieldNumber();
    switch (field) {
    case 1:
      var value = /** @type {boolean} */ (reader.readBool());
      msg.setSuccess(value);
      break;
    case 2:
      var value = /** @type {string} */ (reader.readString());
      msg.setExecutionId(value);
      break;
    case 3:
      var value = /** @type {string} */ (reader.readString());
      msg.setFilePath(value);
      break;
    case 4:
      var value = new proto.g8e.operator.v1.FileHistoryEntry;
      reader.readMessage(value,proto.g8e.operator.v1.FileHistoryEntry.deserializeBinaryFromReader);
      msg.addHistory(value);
      break;
    case 5:
      var value = /** @type {string} */ (reader.readString());
      msg.setError(value);
      break;
    default:
      reader.skipField();
      break;
    }
  }
  return msg;
};


/**
 * Serializes the message to binary data (in protobuf wire format).
 * @return {!Uint8Array}
 */
proto.g8e.operator.v1.FetchFileHistoryResult.prototype.serializeBinary = function() {
  var writer = new jspb.BinaryWriter();
  proto.g8e.operator.v1.FetchFileHistoryResult.serializeBinaryToWriter(this, writer);
  return writer.getResultBuffer();
};


/**
 * Serializes the given message to binary data (in protobuf wire
 * format), writing to the given BinaryWriter.
 * @param {!proto.g8e.operator.v1.FetchFileHistoryResult} message
 * @param {!jspb.BinaryWriter} writer
 * @suppress {unusedLocalVariables} f is only used for nested messages
 */
proto.g8e.operator.v1.FetchFileHistoryResult.serializeBinaryToWriter = function(message, writer) {
  var f = undefined;
  f = message.getSuccess();
  if (f) {
    writer.writeBool(
      1,
      f
    );
  }
  f = message.getExecutionId();
  if (f.length > 0) {
    writer.writeString(
      2,
      f
    );
  }
  f = message.getFilePath();
  if (f.length > 0) {
    writer.writeString(
      3,
      f
    );
  }
  f = message.getHistoryList();
  if (f.length > 0) {
    writer.writeRepeatedMessage(
      4,
      f,
      proto.g8e.operator.v1.FileHistoryEntry.serializeBinaryToWriter
    );
  }
  f = message.getError();
  if (f.length > 0) {
    writer.writeString(
      5,
      f
    );
  }
};


/**
 * optional bool success = 1;
 * @return {boolean}
 */
proto.g8e.operator.v1.FetchFileHistoryResult.prototype.getSuccess = function() {
  return /** @type {boolean} */ (jspb.Message.getBooleanFieldWithDefault(this, 1, false));
};


/**
 * @param {boolean} value
 * @return {!proto.g8e.operator.v1.FetchFileHistoryResult} returns this
 */
proto.g8e.operator.v1.FetchFileHistoryResult.prototype.setSuccess = function(value) {
  return jspb.Message.setProto3BooleanField(this, 1, value);
};


/**
 * optional string execution_id = 2;
 * @return {string}
 */
proto.g8e.operator.v1.FetchFileHistoryResult.prototype.getExecutionId = function() {
  return /** @type {string} */ (jspb.Message.getFieldWithDefault(this, 2, ""));
};


/**
 * @param {string} value
 * @return {!proto.g8e.operator.v1.FetchFileHistoryResult} returns this
 */
proto.g8e.operator.v1.FetchFileHistoryResult.prototype.setExecutionId = function(value) {
  return jspb.Message.setProto3StringField(this, 2, value);
};


/**
 * optional string file_path = 3;
 * @return {string}
 */
proto.g8e.operator.v1.FetchFileHistoryResult.prototype.getFilePath = function() {
  return /** @type {string} */ (jspb.Message.getFieldWithDefault(this, 3, ""));
};


/**
 * @param {string} value
 * @return {!proto.g8e.operator.v1.FetchFileHistoryResult} returns this
 */
proto.g8e.operator.v1.FetchFileHistoryResult.prototype.setFilePath = function(value) {
  return jspb.Message.setProto3StringField(this, 3, value);
};


/**
 * repeated FileHistoryEntry history = 4;
 * @return {!Array<!proto.g8e.operator.v1.FileHistoryEntry>}
 */
proto.g8e.operator.v1.FetchFileHistoryResult.prototype.getHistoryList = function() {
  return /** @type{!Array<!proto.g8e.operator.v1.FileHistoryEntry>} */ (
    jspb.Message.getRepeatedWrapperField(this, proto.g8e.operator.v1.FileHistoryEntry, 4));
};


/**
 * @param {!Array<!proto.g8e.operator.v1.FileHistoryEntry>} value
 * @return {!proto.g8e.operator.v1.FetchFileHistoryResult} returns this
*/
proto.g8e.operator.v1.FetchFileHistoryResult.prototype.setHistoryList = function(value) {
  return jspb.Message.setRepeatedWrapperField(this, 4, value);
};


/**
 * @param {!proto.g8e.operator.v1.FileHistoryEntry=} opt_value
 * @param {number=} opt_index
 * @return {!proto.g8e.operator.v1.FileHistoryEntry}
 */
proto.g8e.operator.v1.FetchFileHistoryResult.prototype.addHistory = function(opt_value, opt_index) {
  return jspb.Message.addToRepeatedWrapperField(this, 4, opt_value, proto.g8e.operator.v1.FileHistoryEntry, opt_index);
};


/**
 * Clears the list making it empty but non-null.
 * @return {!proto.g8e.operator.v1.FetchFileHistoryResult} returns this
 */
proto.g8e.operator.v1.FetchFileHistoryResult.prototype.clearHistoryList = function() {
  return this.setHistoryList([]);
};


/**
 * optional string error = 5;
 * @return {string}
 */
proto.g8e.operator.v1.FetchFileHistoryResult.prototype.getError = function() {
  return /** @type {string} */ (jspb.Message.getFieldWithDefault(this, 5, ""));
};


/**
 * @param {string} value
 * @return {!proto.g8e.operator.v1.FetchFileHistoryResult} returns this
 */
proto.g8e.operator.v1.FetchFileHistoryResult.prototype.setError = function(value) {
  return jspb.Message.setProto3StringField(this, 5, value);
};





if (jspb.Message.GENERATE_TO_OBJECT) {
/**
 * Creates an object representation of this proto.
 * Field names that are reserved in JavaScript and will be renamed to pb_name.
 * Optional fields that are not set will be set to undefined.
 * To access a reserved field use, foo.pb_<name>, eg, foo.pb_default.
 * For the list of reserved names please see:
 *     net/proto2/compiler/js/internal/generator.cc#kKeyword.
 * @param {boolean=} opt_includeInstance Deprecated. whether to include the
 *     JSPB instance for transitional soy proto support:
 *     http://goto/soy-param-migration
 * @return {!Object}
 */
proto.g8e.operator.v1.RestoreFileResult.prototype.toObject = function(opt_includeInstance) {
  return proto.g8e.operator.v1.RestoreFileResult.toObject(opt_includeInstance, this);
};


/**
 * Static version of the {@see toObject} method.
 * @param {boolean|undefined} includeInstance Deprecated. Whether to include
 *     the JSPB instance for transitional soy proto support:
 *     http://goto/soy-param-migration
 * @param {!proto.g8e.operator.v1.RestoreFileResult} msg The msg instance to transform.
 * @return {!Object}
 * @suppress {unusedLocalVariables} f is only used for nested messages
 */
proto.g8e.operator.v1.RestoreFileResult.toObject = function(includeInstance, msg) {
  var f, obj = {
    success: jspb.Message.getBooleanFieldWithDefault(msg, 1, false),
    executionId: jspb.Message.getFieldWithDefault(msg, 2, ""),
    filePath: jspb.Message.getFieldWithDefault(msg, 3, ""),
    commitHash: jspb.Message.getFieldWithDefault(msg, 4, ""),
    error: jspb.Message.getFieldWithDefault(msg, 5, "")
  };

  if (includeInstance) {
    obj.$jspbMessageInstance = msg;
  }
  return obj;
};
}


/**
 * Deserializes binary data (in protobuf wire format).
 * @param {jspb.ByteSource} bytes The bytes to deserialize.
 * @return {!proto.g8e.operator.v1.RestoreFileResult}
 */
proto.g8e.operator.v1.RestoreFileResult.deserializeBinary = function(bytes) {
  var reader = new jspb.BinaryReader(bytes);
  var msg = new proto.g8e.operator.v1.RestoreFileResult;
  return proto.g8e.operator.v1.RestoreFileResult.deserializeBinaryFromReader(msg, reader);
};


/**
 * Deserializes binary data (in protobuf wire format) from the
 * given reader into the given message object.
 * @param {!proto.g8e.operator.v1.RestoreFileResult} msg The message object to deserialize into.
 * @param {!jspb.BinaryReader} reader The BinaryReader to use.
 * @return {!proto.g8e.operator.v1.RestoreFileResult}
 */
proto.g8e.operator.v1.RestoreFileResult.deserializeBinaryFromReader = function(msg, reader) {
  while (reader.nextField()) {
    if (reader.isEndGroup()) {
      break;
    }
    var field = reader.getFieldNumber();
    switch (field) {
    case 1:
      var value = /** @type {boolean} */ (reader.readBool());
      msg.setSuccess(value);
      break;
    case 2:
      var value = /** @type {string} */ (reader.readString());
      msg.setExecutionId(value);
      break;
    case 3:
      var value = /** @type {string} */ (reader.readString());
      msg.setFilePath(value);
      break;
    case 4:
      var value = /** @type {string} */ (reader.readString());
      msg.setCommitHash(value);
      break;
    case 5:
      var value = /** @type {string} */ (reader.readString());
      msg.setError(value);
      break;
    default:
      reader.skipField();
      break;
    }
  }
  return msg;
};


/**
 * Serializes the message to binary data (in protobuf wire format).
 * @return {!Uint8Array}
 */
proto.g8e.operator.v1.RestoreFileResult.prototype.serializeBinary = function() {
  var writer = new jspb.BinaryWriter();
  proto.g8e.operator.v1.RestoreFileResult.serializeBinaryToWriter(this, writer);
  return writer.getResultBuffer();
};


/**
 * Serializes the given message to binary data (in protobuf wire
 * format), writing to the given BinaryWriter.
 * @param {!proto.g8e.operator.v1.RestoreFileResult} message
 * @param {!jspb.BinaryWriter} writer
 * @suppress {unusedLocalVariables} f is only used for nested messages
 */
proto.g8e.operator.v1.RestoreFileResult.serializeBinaryToWriter = function(message, writer) {
  var f = undefined;
  f = message.getSuccess();
  if (f) {
    writer.writeBool(
      1,
      f
    );
  }
  f = message.getExecutionId();
  if (f.length > 0) {
    writer.writeString(
      2,
      f
    );
  }
  f = message.getFilePath();
  if (f.length > 0) {
    writer.writeString(
      3,
      f
    );
  }
  f = message.getCommitHash();
  if (f.length > 0) {
    writer.writeString(
      4,
      f
    );
  }
  f = message.getError();
  if (f.length > 0) {
    writer.writeString(
      5,
      f
    );
  }
};


/**
 * optional bool success = 1;
 * @return {boolean}
 */
proto.g8e.operator.v1.RestoreFileResult.prototype.getSuccess = function() {
  return /** @type {boolean} */ (jspb.Message.getBooleanFieldWithDefault(this, 1, false));
};


/**
 * @param {boolean} value
 * @return {!proto.g8e.operator.v1.RestoreFileResult} returns this
 */
proto.g8e.operator.v1.RestoreFileResult.prototype.setSuccess = function(value) {
  return jspb.Message.setProto3BooleanField(this, 1, value);
};


/**
 * optional string execution_id = 2;
 * @return {string}
 */
proto.g8e.operator.v1.RestoreFileResult.prototype.getExecutionId = function() {
  return /** @type {string} */ (jspb.Message.getFieldWithDefault(this, 2, ""));
};


/**
 * @param {string} value
 * @return {!proto.g8e.operator.v1.RestoreFileResult} returns this
 */
proto.g8e.operator.v1.RestoreFileResult.prototype.setExecutionId = function(value) {
  return jspb.Message.setProto3StringField(this, 2, value);
};


/**
 * optional string file_path = 3;
 * @return {string}
 */
proto.g8e.operator.v1.RestoreFileResult.prototype.getFilePath = function() {
  return /** @type {string} */ (jspb.Message.getFieldWithDefault(this, 3, ""));
};


/**
 * @param {string} value
 * @return {!proto.g8e.operator.v1.RestoreFileResult} returns this
 */
proto.g8e.operator.v1.RestoreFileResult.prototype.setFilePath = function(value) {
  return jspb.Message.setProto3StringField(this, 3, value);
};


/**
 * optional string commit_hash = 4;
 * @return {string}
 */
proto.g8e.operator.v1.RestoreFileResult.prototype.getCommitHash = function() {
  return /** @type {string} */ (jspb.Message.getFieldWithDefault(this, 4, ""));
};


/**
 * @param {string} value
 * @return {!proto.g8e.operator.v1.RestoreFileResult} returns this
 */
proto.g8e.operator.v1.RestoreFileResult.prototype.setCommitHash = function(value) {
  return jspb.Message.setProto3StringField(this, 4, value);
};


/**
 * optional string error = 5;
 * @return {string}
 */
proto.g8e.operator.v1.RestoreFileResult.prototype.getError = function() {
  return /** @type {string} */ (jspb.Message.getFieldWithDefault(this, 5, ""));
};


/**
 * @param {string} value
 * @return {!proto.g8e.operator.v1.RestoreFileResult} returns this
 */
proto.g8e.operator.v1.RestoreFileResult.prototype.setError = function(value) {
  return jspb.Message.setProto3StringField(this, 5, value);
};





if (jspb.Message.GENERATE_TO_OBJECT) {
/**
 * Creates an object representation of this proto.
 * Field names that are reserved in JavaScript and will be renamed to pb_name.
 * Optional fields that are not set will be set to undefined.
 * To access a reserved field use, foo.pb_<name>, eg, foo.pb_default.
 * For the list of reserved names please see:
 *     net/proto2/compiler/js/internal/generator.cc#kKeyword.
 * @param {boolean=} opt_includeInstance Deprecated. whether to include the
 *     JSPB instance for transitional soy proto support:
 *     http://goto/soy-param-migration
 * @return {!Object}
 */
proto.g8e.operator.v1.FileDiffEntry.prototype.toObject = function(opt_includeInstance) {
  return proto.g8e.operator.v1.FileDiffEntry.toObject(opt_includeInstance, this);
};


/**
 * Static version of the {@see toObject} method.
 * @param {boolean|undefined} includeInstance Deprecated. Whether to include
 *     the JSPB instance for transitional soy proto support:
 *     http://goto/soy-param-migration
 * @param {!proto.g8e.operator.v1.FileDiffEntry} msg The msg instance to transform.
 * @return {!Object}
 * @suppress {unusedLocalVariables} f is only used for nested messages
 */
proto.g8e.operator.v1.FileDiffEntry.toObject = function(includeInstance, msg) {
  var f, obj = {
    id: jspb.Message.getFieldWithDefault(msg, 1, ""),
    timestamp: jspb.Message.getFieldWithDefault(msg, 2, ""),
    filePath: jspb.Message.getFieldWithDefault(msg, 3, ""),
    operation: jspb.Message.getFieldWithDefault(msg, 4, ""),
    ledgerHashBefore: jspb.Message.getFieldWithDefault(msg, 5, ""),
    ledgerHashAfter: jspb.Message.getFieldWithDefault(msg, 6, ""),
    diffStat: jspb.Message.getFieldWithDefault(msg, 7, ""),
    diffContent: jspb.Message.getFieldWithDefault(msg, 8, ""),
    diffSize: jspb.Message.getFieldWithDefault(msg, 9, 0),
    operatorSessionId: jspb.Message.getFieldWithDefault(msg, 10, "")
  };

  if (includeInstance) {
    obj.$jspbMessageInstance = msg;
  }
  return obj;
};
}


/**
 * Deserializes binary data (in protobuf wire format).
 * @param {jspb.ByteSource} bytes The bytes to deserialize.
 * @return {!proto.g8e.operator.v1.FileDiffEntry}
 */
proto.g8e.operator.v1.FileDiffEntry.deserializeBinary = function(bytes) {
  var reader = new jspb.BinaryReader(bytes);
  var msg = new proto.g8e.operator.v1.FileDiffEntry;
  return proto.g8e.operator.v1.FileDiffEntry.deserializeBinaryFromReader(msg, reader);
};


/**
 * Deserializes binary data (in protobuf wire format) from the
 * given reader into the given message object.
 * @param {!proto.g8e.operator.v1.FileDiffEntry} msg The message object to deserialize into.
 * @param {!jspb.BinaryReader} reader The BinaryReader to use.
 * @return {!proto.g8e.operator.v1.FileDiffEntry}
 */
proto.g8e.operator.v1.FileDiffEntry.deserializeBinaryFromReader = function(msg, reader) {
  while (reader.nextField()) {
    if (reader.isEndGroup()) {
      break;
    }
    var field = reader.getFieldNumber();
    switch (field) {
    case 1:
      var value = /** @type {string} */ (reader.readString());
      msg.setId(value);
      break;
    case 2:
      var value = /** @type {string} */ (reader.readString());
      msg.setTimestamp(value);
      break;
    case 3:
      var value = /** @type {string} */ (reader.readString());
      msg.setFilePath(value);
      break;
    case 4:
      var value = /** @type {string} */ (reader.readString());
      msg.setOperation(value);
      break;
    case 5:
      var value = /** @type {string} */ (reader.readString());
      msg.setLedgerHashBefore(value);
      break;
    case 6:
      var value = /** @type {string} */ (reader.readString());
      msg.setLedgerHashAfter(value);
      break;
    case 7:
      var value = /** @type {string} */ (reader.readString());
      msg.setDiffStat(value);
      break;
    case 8:
      var value = /** @type {string} */ (reader.readString());
      msg.setDiffContent(value);
      break;
    case 9:
      var value = /** @type {number} */ (reader.readInt32());
      msg.setDiffSize(value);
      break;
    case 10:
      var value = /** @type {string} */ (reader.readString());
      msg.setOperatorSessionId(value);
      break;
    default:
      reader.skipField();
      break;
    }
  }
  return msg;
};


/**
 * Serializes the message to binary data (in protobuf wire format).
 * @return {!Uint8Array}
 */
proto.g8e.operator.v1.FileDiffEntry.prototype.serializeBinary = function() {
  var writer = new jspb.BinaryWriter();
  proto.g8e.operator.v1.FileDiffEntry.serializeBinaryToWriter(this, writer);
  return writer.getResultBuffer();
};


/**
 * Serializes the given message to binary data (in protobuf wire
 * format), writing to the given BinaryWriter.
 * @param {!proto.g8e.operator.v1.FileDiffEntry} message
 * @param {!jspb.BinaryWriter} writer
 * @suppress {unusedLocalVariables} f is only used for nested messages
 */
proto.g8e.operator.v1.FileDiffEntry.serializeBinaryToWriter = function(message, writer) {
  var f = undefined;
  f = message.getId();
  if (f.length > 0) {
    writer.writeString(
      1,
      f
    );
  }
  f = message.getTimestamp();
  if (f.length > 0) {
    writer.writeString(
      2,
      f
    );
  }
  f = message.getFilePath();
  if (f.length > 0) {
    writer.writeString(
      3,
      f
    );
  }
  f = message.getOperation();
  if (f.length > 0) {
    writer.writeString(
      4,
      f
    );
  }
  f = message.getLedgerHashBefore();
  if (f.length > 0) {
    writer.writeString(
      5,
      f
    );
  }
  f = message.getLedgerHashAfter();
  if (f.length > 0) {
    writer.writeString(
      6,
      f
    );
  }
  f = message.getDiffStat();
  if (f.length > 0) {
    writer.writeString(
      7,
      f
    );
  }
  f = message.getDiffContent();
  if (f.length > 0) {
    writer.writeString(
      8,
      f
    );
  }
  f = message.getDiffSize();
  if (f !== 0) {
    writer.writeInt32(
      9,
      f
    );
  }
  f = message.getOperatorSessionId();
  if (f.length > 0) {
    writer.writeString(
      10,
      f
    );
  }
};


/**
 * optional string id = 1;
 * @return {string}
 */
proto.g8e.operator.v1.FileDiffEntry.prototype.getId = function() {
  return /** @type {string} */ (jspb.Message.getFieldWithDefault(this, 1, ""));
};


/**
 * @param {string} value
 * @return {!proto.g8e.operator.v1.FileDiffEntry} returns this
 */
proto.g8e.operator.v1.FileDiffEntry.prototype.setId = function(value) {
  return jspb.Message.setProto3StringField(this, 1, value);
};


/**
 * optional string timestamp = 2;
 * @return {string}
 */
proto.g8e.operator.v1.FileDiffEntry.prototype.getTimestamp = function() {
  return /** @type {string} */ (jspb.Message.getFieldWithDefault(this, 2, ""));
};


/**
 * @param {string} value
 * @return {!proto.g8e.operator.v1.FileDiffEntry} returns this
 */
proto.g8e.operator.v1.FileDiffEntry.prototype.setTimestamp = function(value) {
  return jspb.Message.setProto3StringField(this, 2, value);
};


/**
 * optional string file_path = 3;
 * @return {string}
 */
proto.g8e.operator.v1.FileDiffEntry.prototype.getFilePath = function() {
  return /** @type {string} */ (jspb.Message.getFieldWithDefault(this, 3, ""));
};


/**
 * @param {string} value
 * @return {!proto.g8e.operator.v1.FileDiffEntry} returns this
 */
proto.g8e.operator.v1.FileDiffEntry.prototype.setFilePath = function(value) {
  return jspb.Message.setProto3StringField(this, 3, value);
};


/**
 * optional string operation = 4;
 * @return {string}
 */
proto.g8e.operator.v1.FileDiffEntry.prototype.getOperation = function() {
  return /** @type {string} */ (jspb.Message.getFieldWithDefault(this, 4, ""));
};


/**
 * @param {string} value
 * @return {!proto.g8e.operator.v1.FileDiffEntry} returns this
 */
proto.g8e.operator.v1.FileDiffEntry.prototype.setOperation = function(value) {
  return jspb.Message.setProto3StringField(this, 4, value);
};


/**
 * optional string ledger_hash_before = 5;
 * @return {string}
 */
proto.g8e.operator.v1.FileDiffEntry.prototype.getLedgerHashBefore = function() {
  return /** @type {string} */ (jspb.Message.getFieldWithDefault(this, 5, ""));
};


/**
 * @param {string} value
 * @return {!proto.g8e.operator.v1.FileDiffEntry} returns this
 */
proto.g8e.operator.v1.FileDiffEntry.prototype.setLedgerHashBefore = function(value) {
  return jspb.Message.setProto3StringField(this, 5, value);
};


/**
 * optional string ledger_hash_after = 6;
 * @return {string}
 */
proto.g8e.operator.v1.FileDiffEntry.prototype.getLedgerHashAfter = function() {
  return /** @type {string} */ (jspb.Message.getFieldWithDefault(this, 6, ""));
};


/**
 * @param {string} value
 * @return {!proto.g8e.operator.v1.FileDiffEntry} returns this
 */
proto.g8e.operator.v1.FileDiffEntry.prototype.setLedgerHashAfter = function(value) {
  return jspb.Message.setProto3StringField(this, 6, value);
};


/**
 * optional string diff_stat = 7;
 * @return {string}
 */
proto.g8e.operator.v1.FileDiffEntry.prototype.getDiffStat = function() {
  return /** @type {string} */ (jspb.Message.getFieldWithDefault(this, 7, ""));
};


/**
 * @param {string} value
 * @return {!proto.g8e.operator.v1.FileDiffEntry} returns this
 */
proto.g8e.operator.v1.FileDiffEntry.prototype.setDiffStat = function(value) {
  return jspb.Message.setProto3StringField(this, 7, value);
};


/**
 * optional string diff_content = 8;
 * @return {string}
 */
proto.g8e.operator.v1.FileDiffEntry.prototype.getDiffContent = function() {
  return /** @type {string} */ (jspb.Message.getFieldWithDefault(this, 8, ""));
};


/**
 * @param {string} value
 * @return {!proto.g8e.operator.v1.FileDiffEntry} returns this
 */
proto.g8e.operator.v1.FileDiffEntry.prototype.setDiffContent = function(value) {
  return jspb.Message.setProto3StringField(this, 8, value);
};


/**
 * optional int32 diff_size = 9;
 * @return {number}
 */
proto.g8e.operator.v1.FileDiffEntry.prototype.getDiffSize = function() {
  return /** @type {number} */ (jspb.Message.getFieldWithDefault(this, 9, 0));
};


/**
 * @param {number} value
 * @return {!proto.g8e.operator.v1.FileDiffEntry} returns this
 */
proto.g8e.operator.v1.FileDiffEntry.prototype.setDiffSize = function(value) {
  return jspb.Message.setProto3IntField(this, 9, value);
};


/**
 * optional string operator_session_id = 10;
 * @return {string}
 */
proto.g8e.operator.v1.FileDiffEntry.prototype.getOperatorSessionId = function() {
  return /** @type {string} */ (jspb.Message.getFieldWithDefault(this, 10, ""));
};


/**
 * @param {string} value
 * @return {!proto.g8e.operator.v1.FileDiffEntry} returns this
 */
proto.g8e.operator.v1.FileDiffEntry.prototype.setOperatorSessionId = function(value) {
  return jspb.Message.setProto3StringField(this, 10, value);
};



/**
 * List of repeated fields within this message type.
 * @private {!Array<number>}
 * @const
 */
proto.g8e.operator.v1.FetchFileDiffResult.repeatedFields_ = [3];



if (jspb.Message.GENERATE_TO_OBJECT) {
/**
 * Creates an object representation of this proto.
 * Field names that are reserved in JavaScript and will be renamed to pb_name.
 * Optional fields that are not set will be set to undefined.
 * To access a reserved field use, foo.pb_<name>, eg, foo.pb_default.
 * For the list of reserved names please see:
 *     net/proto2/compiler/js/internal/generator.cc#kKeyword.
 * @param {boolean=} opt_includeInstance Deprecated. whether to include the
 *     JSPB instance for transitional soy proto support:
 *     http://goto/soy-param-migration
 * @return {!Object}
 */
proto.g8e.operator.v1.FetchFileDiffResult.prototype.toObject = function(opt_includeInstance) {
  return proto.g8e.operator.v1.FetchFileDiffResult.toObject(opt_includeInstance, this);
};


/**
 * Static version of the {@see toObject} method.
 * @param {boolean|undefined} includeInstance Deprecated. Whether to include
 *     the JSPB instance for transitional soy proto support:
 *     http://goto/soy-param-migration
 * @param {!proto.g8e.operator.v1.FetchFileDiffResult} msg The msg instance to transform.
 * @return {!Object}
 * @suppress {unusedLocalVariables} f is only used for nested messages
 */
proto.g8e.operator.v1.FetchFileDiffResult.toObject = function(includeInstance, msg) {
  var f, obj = {
    success: jspb.Message.getBooleanFieldWithDefault(msg, 1, false),
    executionId: jspb.Message.getFieldWithDefault(msg, 2, ""),
    diffsList: jspb.Message.toObjectList(msg.getDiffsList(),
    proto.g8e.operator.v1.FileDiffEntry.toObject, includeInstance),
    diff: (f = msg.getDiff()) && proto.g8e.operator.v1.FileDiffEntry.toObject(includeInstance, f),
    total: jspb.Message.getFieldWithDefault(msg, 5, 0),
    operatorSessionId: jspb.Message.getFieldWithDefault(msg, 6, ""),
    error: jspb.Message.getFieldWithDefault(msg, 7, "")
  };

  if (includeInstance) {
    obj.$jspbMessageInstance = msg;
  }
  return obj;
};
}


/**
 * Deserializes binary data (in protobuf wire format).
 * @param {jspb.ByteSource} bytes The bytes to deserialize.
 * @return {!proto.g8e.operator.v1.FetchFileDiffResult}
 */
proto.g8e.operator.v1.FetchFileDiffResult.deserializeBinary = function(bytes) {
  var reader = new jspb.BinaryReader(bytes);
  var msg = new proto.g8e.operator.v1.FetchFileDiffResult;
  return proto.g8e.operator.v1.FetchFileDiffResult.deserializeBinaryFromReader(msg, reader);
};


/**
 * Deserializes binary data (in protobuf wire format) from the
 * given reader into the given message object.
 * @param {!proto.g8e.operator.v1.FetchFileDiffResult} msg The message object to deserialize into.
 * @param {!jspb.BinaryReader} reader The BinaryReader to use.
 * @return {!proto.g8e.operator.v1.FetchFileDiffResult}
 */
proto.g8e.operator.v1.FetchFileDiffResult.deserializeBinaryFromReader = function(msg, reader) {
  while (reader.nextField()) {
    if (reader.isEndGroup()) {
      break;
    }
    var field = reader.getFieldNumber();
    switch (field) {
    case 1:
      var value = /** @type {boolean} */ (reader.readBool());
      msg.setSuccess(value);
      break;
    case 2:
      var value = /** @type {string} */ (reader.readString());
      msg.setExecutionId(value);
      break;
    case 3:
      var value = new proto.g8e.operator.v1.FileDiffEntry;
      reader.readMessage(value,proto.g8e.operator.v1.FileDiffEntry.deserializeBinaryFromReader);
      msg.addDiffs(value);
      break;
    case 4:
      var value = new proto.g8e.operator.v1.FileDiffEntry;
      reader.readMessage(value,proto.g8e.operator.v1.FileDiffEntry.deserializeBinaryFromReader);
      msg.setDiff(value);
      break;
    case 5:
      var value = /** @type {number} */ (reader.readInt32());
      msg.setTotal(value);
      break;
    case 6:
      var value = /** @type {string} */ (reader.readString());
      msg.setOperatorSessionId(value);
      break;
    case 7:
      var value = /** @type {string} */ (reader.readString());
      msg.setError(value);
      break;
    default:
      reader.skipField();
      break;
    }
  }
  return msg;
};


/**
 * Serializes the message to binary data (in protobuf wire format).
 * @return {!Uint8Array}
 */
proto.g8e.operator.v1.FetchFileDiffResult.prototype.serializeBinary = function() {
  var writer = new jspb.BinaryWriter();
  proto.g8e.operator.v1.FetchFileDiffResult.serializeBinaryToWriter(this, writer);
  return writer.getResultBuffer();
};


/**
 * Serializes the given message to binary data (in protobuf wire
 * format), writing to the given BinaryWriter.
 * @param {!proto.g8e.operator.v1.FetchFileDiffResult} message
 * @param {!jspb.BinaryWriter} writer
 * @suppress {unusedLocalVariables} f is only used for nested messages
 */
proto.g8e.operator.v1.FetchFileDiffResult.serializeBinaryToWriter = function(message, writer) {
  var f = undefined;
  f = message.getSuccess();
  if (f) {
    writer.writeBool(
      1,
      f
    );
  }
  f = message.getExecutionId();
  if (f.length > 0) {
    writer.writeString(
      2,
      f
    );
  }
  f = message.getDiffsList();
  if (f.length > 0) {
    writer.writeRepeatedMessage(
      3,
      f,
      proto.g8e.operator.v1.FileDiffEntry.serializeBinaryToWriter
    );
  }
  f = message.getDiff();
  if (f != null) {
    writer.writeMessage(
      4,
      f,
      proto.g8e.operator.v1.FileDiffEntry.serializeBinaryToWriter
    );
  }
  f = message.getTotal();
  if (f !== 0) {
    writer.writeInt32(
      5,
      f
    );
  }
  f = message.getOperatorSessionId();
  if (f.length > 0) {
    writer.writeString(
      6,
      f
    );
  }
  f = message.getError();
  if (f.length > 0) {
    writer.writeString(
      7,
      f
    );
  }
};


/**
 * optional bool success = 1;
 * @return {boolean}
 */
proto.g8e.operator.v1.FetchFileDiffResult.prototype.getSuccess = function() {
  return /** @type {boolean} */ (jspb.Message.getBooleanFieldWithDefault(this, 1, false));
};


/**
 * @param {boolean} value
 * @return {!proto.g8e.operator.v1.FetchFileDiffResult} returns this
 */
proto.g8e.operator.v1.FetchFileDiffResult.prototype.setSuccess = function(value) {
  return jspb.Message.setProto3BooleanField(this, 1, value);
};


/**
 * optional string execution_id = 2;
 * @return {string}
 */
proto.g8e.operator.v1.FetchFileDiffResult.prototype.getExecutionId = function() {
  return /** @type {string} */ (jspb.Message.getFieldWithDefault(this, 2, ""));
};


/**
 * @param {string} value
 * @return {!proto.g8e.operator.v1.FetchFileDiffResult} returns this
 */
proto.g8e.operator.v1.FetchFileDiffResult.prototype.setExecutionId = function(value) {
  return jspb.Message.setProto3StringField(this, 2, value);
};


/**
 * repeated FileDiffEntry diffs = 3;
 * @return {!Array<!proto.g8e.operator.v1.FileDiffEntry>}
 */
proto.g8e.operator.v1.FetchFileDiffResult.prototype.getDiffsList = function() {
  return /** @type{!Array<!proto.g8e.operator.v1.FileDiffEntry>} */ (
    jspb.Message.getRepeatedWrapperField(this, proto.g8e.operator.v1.FileDiffEntry, 3));
};


/**
 * @param {!Array<!proto.g8e.operator.v1.FileDiffEntry>} value
 * @return {!proto.g8e.operator.v1.FetchFileDiffResult} returns this
*/
proto.g8e.operator.v1.FetchFileDiffResult.prototype.setDiffsList = function(value) {
  return jspb.Message.setRepeatedWrapperField(this, 3, value);
};


/**
 * @param {!proto.g8e.operator.v1.FileDiffEntry=} opt_value
 * @param {number=} opt_index
 * @return {!proto.g8e.operator.v1.FileDiffEntry}
 */
proto.g8e.operator.v1.FetchFileDiffResult.prototype.addDiffs = function(opt_value, opt_index) {
  return jspb.Message.addToRepeatedWrapperField(this, 3, opt_value, proto.g8e.operator.v1.FileDiffEntry, opt_index);
};


/**
 * Clears the list making it empty but non-null.
 * @return {!proto.g8e.operator.v1.FetchFileDiffResult} returns this
 */
proto.g8e.operator.v1.FetchFileDiffResult.prototype.clearDiffsList = function() {
  return this.setDiffsList([]);
};


/**
 * optional FileDiffEntry diff = 4;
 * @return {?proto.g8e.operator.v1.FileDiffEntry}
 */
proto.g8e.operator.v1.FetchFileDiffResult.prototype.getDiff = function() {
  return /** @type{?proto.g8e.operator.v1.FileDiffEntry} */ (
    jspb.Message.getWrapperField(this, proto.g8e.operator.v1.FileDiffEntry, 4));
};


/**
 * @param {?proto.g8e.operator.v1.FileDiffEntry|undefined} value
 * @return {!proto.g8e.operator.v1.FetchFileDiffResult} returns this
*/
proto.g8e.operator.v1.FetchFileDiffResult.prototype.setDiff = function(value) {
  return jspb.Message.setWrapperField(this, 4, value);
};


/**
 * Clears the message field making it undefined.
 * @return {!proto.g8e.operator.v1.FetchFileDiffResult} returns this
 */
proto.g8e.operator.v1.FetchFileDiffResult.prototype.clearDiff = function() {
  return this.setDiff(undefined);
};


/**
 * Returns whether this field is set.
 * @return {boolean}
 */
proto.g8e.operator.v1.FetchFileDiffResult.prototype.hasDiff = function() {
  return jspb.Message.getField(this, 4) != null;
};


/**
 * optional int32 total = 5;
 * @return {number}
 */
proto.g8e.operator.v1.FetchFileDiffResult.prototype.getTotal = function() {
  return /** @type {number} */ (jspb.Message.getFieldWithDefault(this, 5, 0));
};


/**
 * @param {number} value
 * @return {!proto.g8e.operator.v1.FetchFileDiffResult} returns this
 */
proto.g8e.operator.v1.FetchFileDiffResult.prototype.setTotal = function(value) {
  return jspb.Message.setProto3IntField(this, 5, value);
};


/**
 * optional string operator_session_id = 6;
 * @return {string}
 */
proto.g8e.operator.v1.FetchFileDiffResult.prototype.getOperatorSessionId = function() {
  return /** @type {string} */ (jspb.Message.getFieldWithDefault(this, 6, ""));
};


/**
 * @param {string} value
 * @return {!proto.g8e.operator.v1.FetchFileDiffResult} returns this
 */
proto.g8e.operator.v1.FetchFileDiffResult.prototype.setOperatorSessionId = function(value) {
  return jspb.Message.setProto3StringField(this, 6, value);
};


/**
 * optional string error = 7;
 * @return {string}
 */
proto.g8e.operator.v1.FetchFileDiffResult.prototype.getError = function() {
  return /** @type {string} */ (jspb.Message.getFieldWithDefault(this, 7, ""));
};


/**
 * @param {string} value
 * @return {!proto.g8e.operator.v1.FetchFileDiffResult} returns this
 */
proto.g8e.operator.v1.FetchFileDiffResult.prototype.setError = function(value) {
  return jspb.Message.setProto3StringField(this, 7, value);
};





if (jspb.Message.GENERATE_TO_OBJECT) {
/**
 * Creates an object representation of this proto.
 * Field names that are reserved in JavaScript and will be renamed to pb_name.
 * Optional fields that are not set will be set to undefined.
 * To access a reserved field use, foo.pb_<name>, eg, foo.pb_default.
 * For the list of reserved names please see:
 *     net/proto2/compiler/js/internal/generator.cc#kKeyword.
 * @param {boolean=} opt_includeInstance Deprecated. whether to include the
 *     JSPB instance for transitional soy proto support:
 *     http://goto/soy-param-migration
 * @return {!Object}
 */
proto.g8e.operator.v1.HeartbeatResult.prototype.toObject = function(opt_includeInstance) {
  return proto.g8e.operator.v1.HeartbeatResult.toObject(opt_includeInstance, this);
};


/**
 * Static version of the {@see toObject} method.
 * @param {boolean|undefined} includeInstance Deprecated. Whether to include
 *     the JSPB instance for transitional soy proto support:
 *     http://goto/soy-param-migration
 * @param {!proto.g8e.operator.v1.HeartbeatResult} msg The msg instance to transform.
 * @return {!Object}
 * @suppress {unusedLocalVariables} f is only used for nested messages
 */
proto.g8e.operator.v1.HeartbeatResult.toObject = function(includeInstance, msg) {
  var f, obj = {
    operatorId: jspb.Message.getFieldWithDefault(msg, 1, ""),
    operatorSessionId: jspb.Message.getFieldWithDefault(msg, 2, ""),
    timestamp: jspb.Message.getFieldWithDefault(msg, 3, ""),
    status: jspb.Message.getFieldWithDefault(msg, 4, ""),
    eventType: jspb.Message.getFieldWithDefault(msg, 5, ""),
    sourceComponent: jspb.Message.getFieldWithDefault(msg, 6, ""),
    caseId: jspb.Message.getFieldWithDefault(msg, 7, ""),
    investigationId: jspb.Message.getFieldWithDefault(msg, 8, ""),
    systemIdentity: (f = msg.getSystemIdentity()) && proto.g8e.operator.v1.SystemIdentity.toObject(includeInstance, f),
    networkInfo: (f = msg.getNetworkInfo()) && proto.g8e.operator.v1.NetworkInfo.toObject(includeInstance, f),
    versionInfo: (f = msg.getVersionInfo()) && proto.g8e.operator.v1.VersionInfo.toObject(includeInstance, f),
    uptimeInfo: (f = msg.getUptimeInfo()) && proto.g8e.operator.v1.UptimeInfo.toObject(includeInstance, f),
    performanceMetrics: (f = msg.getPerformanceMetrics()) && proto.g8e.operator.v1.PerformanceMetrics.toObject(includeInstance, f),
    osDetails: (f = msg.getOsDetails()) && proto.g8e.operator.v1.OSDetails.toObject(includeInstance, f),
    userDetails: (f = msg.getUserDetails()) && proto.g8e.operator.v1.UserDetails.toObject(includeInstance, f),
    diskDetails: (f = msg.getDiskDetails()) && proto.g8e.operator.v1.DiskDetails.toObject(includeInstance, f),
    memoryDetails: (f = msg.getMemoryDetails()) && proto.g8e.operator.v1.MemoryDetails.toObject(includeInstance, f),
    environment: (f = msg.getEnvironment()) && proto.g8e.operator.v1.EnvironmentDetails.toObject(includeInstance, f),
    capabilityFlags: (f = msg.getCapabilityFlags()) && proto.g8e.operator.v1.CapabilityFlags.toObject(includeInstance, f),
    fingerprintDetails: (f = msg.getFingerprintDetails()) && proto.g8e.operator.v1.FingerprintDetails.toObject(includeInstance, f),
    systemFingerprint: jspb.Message.getFieldWithDefault(msg, 21, ""),
    apiKey: jspb.Message.getFieldWithDefault(msg, 22, "")
  };

  if (includeInstance) {
    obj.$jspbMessageInstance = msg;
  }
  return obj;
};
}


/**
 * Deserializes binary data (in protobuf wire format).
 * @param {jspb.ByteSource} bytes The bytes to deserialize.
 * @return {!proto.g8e.operator.v1.HeartbeatResult}
 */
proto.g8e.operator.v1.HeartbeatResult.deserializeBinary = function(bytes) {
  var reader = new jspb.BinaryReader(bytes);
  var msg = new proto.g8e.operator.v1.HeartbeatResult;
  return proto.g8e.operator.v1.HeartbeatResult.deserializeBinaryFromReader(msg, reader);
};


/**
 * Deserializes binary data (in protobuf wire format) from the
 * given reader into the given message object.
 * @param {!proto.g8e.operator.v1.HeartbeatResult} msg The message object to deserialize into.
 * @param {!jspb.BinaryReader} reader The BinaryReader to use.
 * @return {!proto.g8e.operator.v1.HeartbeatResult}
 */
proto.g8e.operator.v1.HeartbeatResult.deserializeBinaryFromReader = function(msg, reader) {
  while (reader.nextField()) {
    if (reader.isEndGroup()) {
      break;
    }
    var field = reader.getFieldNumber();
    switch (field) {
    case 1:
      var value = /** @type {string} */ (reader.readString());
      msg.setOperatorId(value);
      break;
    case 2:
      var value = /** @type {string} */ (reader.readString());
      msg.setOperatorSessionId(value);
      break;
    case 3:
      var value = /** @type {string} */ (reader.readString());
      msg.setTimestamp(value);
      break;
    case 4:
      var value = /** @type {string} */ (reader.readString());
      msg.setStatus(value);
      break;
    case 5:
      var value = /** @type {string} */ (reader.readString());
      msg.setEventType(value);
      break;
    case 6:
      var value = /** @type {string} */ (reader.readString());
      msg.setSourceComponent(value);
      break;
    case 7:
      var value = /** @type {string} */ (reader.readString());
      msg.setCaseId(value);
      break;
    case 8:
      var value = /** @type {string} */ (reader.readString());
      msg.setInvestigationId(value);
      break;
    case 9:
      var value = new proto.g8e.operator.v1.SystemIdentity;
      reader.readMessage(value,proto.g8e.operator.v1.SystemIdentity.deserializeBinaryFromReader);
      msg.setSystemIdentity(value);
      break;
    case 10:
      var value = new proto.g8e.operator.v1.NetworkInfo;
      reader.readMessage(value,proto.g8e.operator.v1.NetworkInfo.deserializeBinaryFromReader);
      msg.setNetworkInfo(value);
      break;
    case 11:
      var value = new proto.g8e.operator.v1.VersionInfo;
      reader.readMessage(value,proto.g8e.operator.v1.VersionInfo.deserializeBinaryFromReader);
      msg.setVersionInfo(value);
      break;
    case 12:
      var value = new proto.g8e.operator.v1.UptimeInfo;
      reader.readMessage(value,proto.g8e.operator.v1.UptimeInfo.deserializeBinaryFromReader);
      msg.setUptimeInfo(value);
      break;
    case 13:
      var value = new proto.g8e.operator.v1.PerformanceMetrics;
      reader.readMessage(value,proto.g8e.operator.v1.PerformanceMetrics.deserializeBinaryFromReader);
      msg.setPerformanceMetrics(value);
      break;
    case 14:
      var value = new proto.g8e.operator.v1.OSDetails;
      reader.readMessage(value,proto.g8e.operator.v1.OSDetails.deserializeBinaryFromReader);
      msg.setOsDetails(value);
      break;
    case 15:
      var value = new proto.g8e.operator.v1.UserDetails;
      reader.readMessage(value,proto.g8e.operator.v1.UserDetails.deserializeBinaryFromReader);
      msg.setUserDetails(value);
      break;
    case 16:
      var value = new proto.g8e.operator.v1.DiskDetails;
      reader.readMessage(value,proto.g8e.operator.v1.DiskDetails.deserializeBinaryFromReader);
      msg.setDiskDetails(value);
      break;
    case 17:
      var value = new proto.g8e.operator.v1.MemoryDetails;
      reader.readMessage(value,proto.g8e.operator.v1.MemoryDetails.deserializeBinaryFromReader);
      msg.setMemoryDetails(value);
      break;
    case 18:
      var value = new proto.g8e.operator.v1.EnvironmentDetails;
      reader.readMessage(value,proto.g8e.operator.v1.EnvironmentDetails.deserializeBinaryFromReader);
      msg.setEnvironment(value);
      break;
    case 19:
      var value = new proto.g8e.operator.v1.CapabilityFlags;
      reader.readMessage(value,proto.g8e.operator.v1.CapabilityFlags.deserializeBinaryFromReader);
      msg.setCapabilityFlags(value);
      break;
    case 20:
      var value = new proto.g8e.operator.v1.FingerprintDetails;
      reader.readMessage(value,proto.g8e.operator.v1.FingerprintDetails.deserializeBinaryFromReader);
      msg.setFingerprintDetails(value);
      break;
    case 21:
      var value = /** @type {string} */ (reader.readString());
      msg.setSystemFingerprint(value);
      break;
    case 22:
      var value = /** @type {string} */ (reader.readString());
      msg.setApiKey(value);
      break;
    default:
      reader.skipField();
      break;
    }
  }
  return msg;
};


/**
 * Serializes the message to binary data (in protobuf wire format).
 * @return {!Uint8Array}
 */
proto.g8e.operator.v1.HeartbeatResult.prototype.serializeBinary = function() {
  var writer = new jspb.BinaryWriter();
  proto.g8e.operator.v1.HeartbeatResult.serializeBinaryToWriter(this, writer);
  return writer.getResultBuffer();
};


/**
 * Serializes the given message to binary data (in protobuf wire
 * format), writing to the given BinaryWriter.
 * @param {!proto.g8e.operator.v1.HeartbeatResult} message
 * @param {!jspb.BinaryWriter} writer
 * @suppress {unusedLocalVariables} f is only used for nested messages
 */
proto.g8e.operator.v1.HeartbeatResult.serializeBinaryToWriter = function(message, writer) {
  var f = undefined;
  f = message.getOperatorId();
  if (f.length > 0) {
    writer.writeString(
      1,
      f
    );
  }
  f = message.getOperatorSessionId();
  if (f.length > 0) {
    writer.writeString(
      2,
      f
    );
  }
  f = message.getTimestamp();
  if (f.length > 0) {
    writer.writeString(
      3,
      f
    );
  }
  f = message.getStatus();
  if (f.length > 0) {
    writer.writeString(
      4,
      f
    );
  }
  f = message.getEventType();
  if (f.length > 0) {
    writer.writeString(
      5,
      f
    );
  }
  f = message.getSourceComponent();
  if (f.length > 0) {
    writer.writeString(
      6,
      f
    );
  }
  f = message.getCaseId();
  if (f.length > 0) {
    writer.writeString(
      7,
      f
    );
  }
  f = message.getInvestigationId();
  if (f.length > 0) {
    writer.writeString(
      8,
      f
    );
  }
  f = message.getSystemIdentity();
  if (f != null) {
    writer.writeMessage(
      9,
      f,
      proto.g8e.operator.v1.SystemIdentity.serializeBinaryToWriter
    );
  }
  f = message.getNetworkInfo();
  if (f != null) {
    writer.writeMessage(
      10,
      f,
      proto.g8e.operator.v1.NetworkInfo.serializeBinaryToWriter
    );
  }
  f = message.getVersionInfo();
  if (f != null) {
    writer.writeMessage(
      11,
      f,
      proto.g8e.operator.v1.VersionInfo.serializeBinaryToWriter
    );
  }
  f = message.getUptimeInfo();
  if (f != null) {
    writer.writeMessage(
      12,
      f,
      proto.g8e.operator.v1.UptimeInfo.serializeBinaryToWriter
    );
  }
  f = message.getPerformanceMetrics();
  if (f != null) {
    writer.writeMessage(
      13,
      f,
      proto.g8e.operator.v1.PerformanceMetrics.serializeBinaryToWriter
    );
  }
  f = message.getOsDetails();
  if (f != null) {
    writer.writeMessage(
      14,
      f,
      proto.g8e.operator.v1.OSDetails.serializeBinaryToWriter
    );
  }
  f = message.getUserDetails();
  if (f != null) {
    writer.writeMessage(
      15,
      f,
      proto.g8e.operator.v1.UserDetails.serializeBinaryToWriter
    );
  }
  f = message.getDiskDetails();
  if (f != null) {
    writer.writeMessage(
      16,
      f,
      proto.g8e.operator.v1.DiskDetails.serializeBinaryToWriter
    );
  }
  f = message.getMemoryDetails();
  if (f != null) {
    writer.writeMessage(
      17,
      f,
      proto.g8e.operator.v1.MemoryDetails.serializeBinaryToWriter
    );
  }
  f = message.getEnvironment();
  if (f != null) {
    writer.writeMessage(
      18,
      f,
      proto.g8e.operator.v1.EnvironmentDetails.serializeBinaryToWriter
    );
  }
  f = message.getCapabilityFlags();
  if (f != null) {
    writer.writeMessage(
      19,
      f,
      proto.g8e.operator.v1.CapabilityFlags.serializeBinaryToWriter
    );
  }
  f = message.getFingerprintDetails();
  if (f != null) {
    writer.writeMessage(
      20,
      f,
      proto.g8e.operator.v1.FingerprintDetails.serializeBinaryToWriter
    );
  }
  f = message.getSystemFingerprint();
  if (f.length > 0) {
    writer.writeString(
      21,
      f
    );
  }
  f = message.getApiKey();
  if (f.length > 0) {
    writer.writeString(
      22,
      f
    );
  }
};


/**
 * optional string operator_id = 1;
 * @return {string}
 */
proto.g8e.operator.v1.HeartbeatResult.prototype.getOperatorId = function() {
  return /** @type {string} */ (jspb.Message.getFieldWithDefault(this, 1, ""));
};


/**
 * @param {string} value
 * @return {!proto.g8e.operator.v1.HeartbeatResult} returns this
 */
proto.g8e.operator.v1.HeartbeatResult.prototype.setOperatorId = function(value) {
  return jspb.Message.setProto3StringField(this, 1, value);
};


/**
 * optional string operator_session_id = 2;
 * @return {string}
 */
proto.g8e.operator.v1.HeartbeatResult.prototype.getOperatorSessionId = function() {
  return /** @type {string} */ (jspb.Message.getFieldWithDefault(this, 2, ""));
};


/**
 * @param {string} value
 * @return {!proto.g8e.operator.v1.HeartbeatResult} returns this
 */
proto.g8e.operator.v1.HeartbeatResult.prototype.setOperatorSessionId = function(value) {
  return jspb.Message.setProto3StringField(this, 2, value);
};


/**
 * optional string timestamp = 3;
 * @return {string}
 */
proto.g8e.operator.v1.HeartbeatResult.prototype.getTimestamp = function() {
  return /** @type {string} */ (jspb.Message.getFieldWithDefault(this, 3, ""));
};


/**
 * @param {string} value
 * @return {!proto.g8e.operator.v1.HeartbeatResult} returns this
 */
proto.g8e.operator.v1.HeartbeatResult.prototype.setTimestamp = function(value) {
  return jspb.Message.setProto3StringField(this, 3, value);
};


/**
 * optional string status = 4;
 * @return {string}
 */
proto.g8e.operator.v1.HeartbeatResult.prototype.getStatus = function() {
  return /** @type {string} */ (jspb.Message.getFieldWithDefault(this, 4, ""));
};


/**
 * @param {string} value
 * @return {!proto.g8e.operator.v1.HeartbeatResult} returns this
 */
proto.g8e.operator.v1.HeartbeatResult.prototype.setStatus = function(value) {
  return jspb.Message.setProto3StringField(this, 4, value);
};


/**
 * optional string event_type = 5;
 * @return {string}
 */
proto.g8e.operator.v1.HeartbeatResult.prototype.getEventType = function() {
  return /** @type {string} */ (jspb.Message.getFieldWithDefault(this, 5, ""));
};


/**
 * @param {string} value
 * @return {!proto.g8e.operator.v1.HeartbeatResult} returns this
 */
proto.g8e.operator.v1.HeartbeatResult.prototype.setEventType = function(value) {
  return jspb.Message.setProto3StringField(this, 5, value);
};


/**
 * optional string source_component = 6;
 * @return {string}
 */
proto.g8e.operator.v1.HeartbeatResult.prototype.getSourceComponent = function() {
  return /** @type {string} */ (jspb.Message.getFieldWithDefault(this, 6, ""));
};


/**
 * @param {string} value
 * @return {!proto.g8e.operator.v1.HeartbeatResult} returns this
 */
proto.g8e.operator.v1.HeartbeatResult.prototype.setSourceComponent = function(value) {
  return jspb.Message.setProto3StringField(this, 6, value);
};


/**
 * optional string case_id = 7;
 * @return {string}
 */
proto.g8e.operator.v1.HeartbeatResult.prototype.getCaseId = function() {
  return /** @type {string} */ (jspb.Message.getFieldWithDefault(this, 7, ""));
};


/**
 * @param {string} value
 * @return {!proto.g8e.operator.v1.HeartbeatResult} returns this
 */
proto.g8e.operator.v1.HeartbeatResult.prototype.setCaseId = function(value) {
  return jspb.Message.setProto3StringField(this, 7, value);
};


/**
 * optional string investigation_id = 8;
 * @return {string}
 */
proto.g8e.operator.v1.HeartbeatResult.prototype.getInvestigationId = function() {
  return /** @type {string} */ (jspb.Message.getFieldWithDefault(this, 8, ""));
};


/**
 * @param {string} value
 * @return {!proto.g8e.operator.v1.HeartbeatResult} returns this
 */
proto.g8e.operator.v1.HeartbeatResult.prototype.setInvestigationId = function(value) {
  return jspb.Message.setProto3StringField(this, 8, value);
};


/**
 * optional SystemIdentity system_identity = 9;
 * @return {?proto.g8e.operator.v1.SystemIdentity}
 */
proto.g8e.operator.v1.HeartbeatResult.prototype.getSystemIdentity = function() {
  return /** @type{?proto.g8e.operator.v1.SystemIdentity} */ (
    jspb.Message.getWrapperField(this, proto.g8e.operator.v1.SystemIdentity, 9));
};


/**
 * @param {?proto.g8e.operator.v1.SystemIdentity|undefined} value
 * @return {!proto.g8e.operator.v1.HeartbeatResult} returns this
*/
proto.g8e.operator.v1.HeartbeatResult.prototype.setSystemIdentity = function(value) {
  return jspb.Message.setWrapperField(this, 9, value);
};


/**
 * Clears the message field making it undefined.
 * @return {!proto.g8e.operator.v1.HeartbeatResult} returns this
 */
proto.g8e.operator.v1.HeartbeatResult.prototype.clearSystemIdentity = function() {
  return this.setSystemIdentity(undefined);
};


/**
 * Returns whether this field is set.
 * @return {boolean}
 */
proto.g8e.operator.v1.HeartbeatResult.prototype.hasSystemIdentity = function() {
  return jspb.Message.getField(this, 9) != null;
};


/**
 * optional NetworkInfo network_info = 10;
 * @return {?proto.g8e.operator.v1.NetworkInfo}
 */
proto.g8e.operator.v1.HeartbeatResult.prototype.getNetworkInfo = function() {
  return /** @type{?proto.g8e.operator.v1.NetworkInfo} */ (
    jspb.Message.getWrapperField(this, proto.g8e.operator.v1.NetworkInfo, 10));
};


/**
 * @param {?proto.g8e.operator.v1.NetworkInfo|undefined} value
 * @return {!proto.g8e.operator.v1.HeartbeatResult} returns this
*/
proto.g8e.operator.v1.HeartbeatResult.prototype.setNetworkInfo = function(value) {
  return jspb.Message.setWrapperField(this, 10, value);
};


/**
 * Clears the message field making it undefined.
 * @return {!proto.g8e.operator.v1.HeartbeatResult} returns this
 */
proto.g8e.operator.v1.HeartbeatResult.prototype.clearNetworkInfo = function() {
  return this.setNetworkInfo(undefined);
};


/**
 * Returns whether this field is set.
 * @return {boolean}
 */
proto.g8e.operator.v1.HeartbeatResult.prototype.hasNetworkInfo = function() {
  return jspb.Message.getField(this, 10) != null;
};


/**
 * optional VersionInfo version_info = 11;
 * @return {?proto.g8e.operator.v1.VersionInfo}
 */
proto.g8e.operator.v1.HeartbeatResult.prototype.getVersionInfo = function() {
  return /** @type{?proto.g8e.operator.v1.VersionInfo} */ (
    jspb.Message.getWrapperField(this, proto.g8e.operator.v1.VersionInfo, 11));
};


/**
 * @param {?proto.g8e.operator.v1.VersionInfo|undefined} value
 * @return {!proto.g8e.operator.v1.HeartbeatResult} returns this
*/
proto.g8e.operator.v1.HeartbeatResult.prototype.setVersionInfo = function(value) {
  return jspb.Message.setWrapperField(this, 11, value);
};


/**
 * Clears the message field making it undefined.
 * @return {!proto.g8e.operator.v1.HeartbeatResult} returns this
 */
proto.g8e.operator.v1.HeartbeatResult.prototype.clearVersionInfo = function() {
  return this.setVersionInfo(undefined);
};


/**
 * Returns whether this field is set.
 * @return {boolean}
 */
proto.g8e.operator.v1.HeartbeatResult.prototype.hasVersionInfo = function() {
  return jspb.Message.getField(this, 11) != null;
};


/**
 * optional UptimeInfo uptime_info = 12;
 * @return {?proto.g8e.operator.v1.UptimeInfo}
 */
proto.g8e.operator.v1.HeartbeatResult.prototype.getUptimeInfo = function() {
  return /** @type{?proto.g8e.operator.v1.UptimeInfo} */ (
    jspb.Message.getWrapperField(this, proto.g8e.operator.v1.UptimeInfo, 12));
};


/**
 * @param {?proto.g8e.operator.v1.UptimeInfo|undefined} value
 * @return {!proto.g8e.operator.v1.HeartbeatResult} returns this
*/
proto.g8e.operator.v1.HeartbeatResult.prototype.setUptimeInfo = function(value) {
  return jspb.Message.setWrapperField(this, 12, value);
};


/**
 * Clears the message field making it undefined.
 * @return {!proto.g8e.operator.v1.HeartbeatResult} returns this
 */
proto.g8e.operator.v1.HeartbeatResult.prototype.clearUptimeInfo = function() {
  return this.setUptimeInfo(undefined);
};


/**
 * Returns whether this field is set.
 * @return {boolean}
 */
proto.g8e.operator.v1.HeartbeatResult.prototype.hasUptimeInfo = function() {
  return jspb.Message.getField(this, 12) != null;
};


/**
 * optional PerformanceMetrics performance_metrics = 13;
 * @return {?proto.g8e.operator.v1.PerformanceMetrics}
 */
proto.g8e.operator.v1.HeartbeatResult.prototype.getPerformanceMetrics = function() {
  return /** @type{?proto.g8e.operator.v1.PerformanceMetrics} */ (
    jspb.Message.getWrapperField(this, proto.g8e.operator.v1.PerformanceMetrics, 13));
};


/**
 * @param {?proto.g8e.operator.v1.PerformanceMetrics|undefined} value
 * @return {!proto.g8e.operator.v1.HeartbeatResult} returns this
*/
proto.g8e.operator.v1.HeartbeatResult.prototype.setPerformanceMetrics = function(value) {
  return jspb.Message.setWrapperField(this, 13, value);
};


/**
 * Clears the message field making it undefined.
 * @return {!proto.g8e.operator.v1.HeartbeatResult} returns this
 */
proto.g8e.operator.v1.HeartbeatResult.prototype.clearPerformanceMetrics = function() {
  return this.setPerformanceMetrics(undefined);
};


/**
 * Returns whether this field is set.
 * @return {boolean}
 */
proto.g8e.operator.v1.HeartbeatResult.prototype.hasPerformanceMetrics = function() {
  return jspb.Message.getField(this, 13) != null;
};


/**
 * optional OSDetails os_details = 14;
 * @return {?proto.g8e.operator.v1.OSDetails}
 */
proto.g8e.operator.v1.HeartbeatResult.prototype.getOsDetails = function() {
  return /** @type{?proto.g8e.operator.v1.OSDetails} */ (
    jspb.Message.getWrapperField(this, proto.g8e.operator.v1.OSDetails, 14));
};


/**
 * @param {?proto.g8e.operator.v1.OSDetails|undefined} value
 * @return {!proto.g8e.operator.v1.HeartbeatResult} returns this
*/
proto.g8e.operator.v1.HeartbeatResult.prototype.setOsDetails = function(value) {
  return jspb.Message.setWrapperField(this, 14, value);
};


/**
 * Clears the message field making it undefined.
 * @return {!proto.g8e.operator.v1.HeartbeatResult} returns this
 */
proto.g8e.operator.v1.HeartbeatResult.prototype.clearOsDetails = function() {
  return this.setOsDetails(undefined);
};


/**
 * Returns whether this field is set.
 * @return {boolean}
 */
proto.g8e.operator.v1.HeartbeatResult.prototype.hasOsDetails = function() {
  return jspb.Message.getField(this, 14) != null;
};


/**
 * optional UserDetails user_details = 15;
 * @return {?proto.g8e.operator.v1.UserDetails}
 */
proto.g8e.operator.v1.HeartbeatResult.prototype.getUserDetails = function() {
  return /** @type{?proto.g8e.operator.v1.UserDetails} */ (
    jspb.Message.getWrapperField(this, proto.g8e.operator.v1.UserDetails, 15));
};


/**
 * @param {?proto.g8e.operator.v1.UserDetails|undefined} value
 * @return {!proto.g8e.operator.v1.HeartbeatResult} returns this
*/
proto.g8e.operator.v1.HeartbeatResult.prototype.setUserDetails = function(value) {
  return jspb.Message.setWrapperField(this, 15, value);
};


/**
 * Clears the message field making it undefined.
 * @return {!proto.g8e.operator.v1.HeartbeatResult} returns this
 */
proto.g8e.operator.v1.HeartbeatResult.prototype.clearUserDetails = function() {
  return this.setUserDetails(undefined);
};


/**
 * Returns whether this field is set.
 * @return {boolean}
 */
proto.g8e.operator.v1.HeartbeatResult.prototype.hasUserDetails = function() {
  return jspb.Message.getField(this, 15) != null;
};


/**
 * optional DiskDetails disk_details = 16;
 * @return {?proto.g8e.operator.v1.DiskDetails}
 */
proto.g8e.operator.v1.HeartbeatResult.prototype.getDiskDetails = function() {
  return /** @type{?proto.g8e.operator.v1.DiskDetails} */ (
    jspb.Message.getWrapperField(this, proto.g8e.operator.v1.DiskDetails, 16));
};


/**
 * @param {?proto.g8e.operator.v1.DiskDetails|undefined} value
 * @return {!proto.g8e.operator.v1.HeartbeatResult} returns this
*/
proto.g8e.operator.v1.HeartbeatResult.prototype.setDiskDetails = function(value) {
  return jspb.Message.setWrapperField(this, 16, value);
};


/**
 * Clears the message field making it undefined.
 * @return {!proto.g8e.operator.v1.HeartbeatResult} returns this
 */
proto.g8e.operator.v1.HeartbeatResult.prototype.clearDiskDetails = function() {
  return this.setDiskDetails(undefined);
};


/**
 * Returns whether this field is set.
 * @return {boolean}
 */
proto.g8e.operator.v1.HeartbeatResult.prototype.hasDiskDetails = function() {
  return jspb.Message.getField(this, 16) != null;
};


/**
 * optional MemoryDetails memory_details = 17;
 * @return {?proto.g8e.operator.v1.MemoryDetails}
 */
proto.g8e.operator.v1.HeartbeatResult.prototype.getMemoryDetails = function() {
  return /** @type{?proto.g8e.operator.v1.MemoryDetails} */ (
    jspb.Message.getWrapperField(this, proto.g8e.operator.v1.MemoryDetails, 17));
};


/**
 * @param {?proto.g8e.operator.v1.MemoryDetails|undefined} value
 * @return {!proto.g8e.operator.v1.HeartbeatResult} returns this
*/
proto.g8e.operator.v1.HeartbeatResult.prototype.setMemoryDetails = function(value) {
  return jspb.Message.setWrapperField(this, 17, value);
};


/**
 * Clears the message field making it undefined.
 * @return {!proto.g8e.operator.v1.HeartbeatResult} returns this
 */
proto.g8e.operator.v1.HeartbeatResult.prototype.clearMemoryDetails = function() {
  return this.setMemoryDetails(undefined);
};


/**
 * Returns whether this field is set.
 * @return {boolean}
 */
proto.g8e.operator.v1.HeartbeatResult.prototype.hasMemoryDetails = function() {
  return jspb.Message.getField(this, 17) != null;
};


/**
 * optional EnvironmentDetails environment = 18;
 * @return {?proto.g8e.operator.v1.EnvironmentDetails}
 */
proto.g8e.operator.v1.HeartbeatResult.prototype.getEnvironment = function() {
  return /** @type{?proto.g8e.operator.v1.EnvironmentDetails} */ (
    jspb.Message.getWrapperField(this, proto.g8e.operator.v1.EnvironmentDetails, 18));
};


/**
 * @param {?proto.g8e.operator.v1.EnvironmentDetails|undefined} value
 * @return {!proto.g8e.operator.v1.HeartbeatResult} returns this
*/
proto.g8e.operator.v1.HeartbeatResult.prototype.setEnvironment = function(value) {
  return jspb.Message.setWrapperField(this, 18, value);
};


/**
 * Clears the message field making it undefined.
 * @return {!proto.g8e.operator.v1.HeartbeatResult} returns this
 */
proto.g8e.operator.v1.HeartbeatResult.prototype.clearEnvironment = function() {
  return this.setEnvironment(undefined);
};


/**
 * Returns whether this field is set.
 * @return {boolean}
 */
proto.g8e.operator.v1.HeartbeatResult.prototype.hasEnvironment = function() {
  return jspb.Message.getField(this, 18) != null;
};


/**
 * optional CapabilityFlags capability_flags = 19;
 * @return {?proto.g8e.operator.v1.CapabilityFlags}
 */
proto.g8e.operator.v1.HeartbeatResult.prototype.getCapabilityFlags = function() {
  return /** @type{?proto.g8e.operator.v1.CapabilityFlags} */ (
    jspb.Message.getWrapperField(this, proto.g8e.operator.v1.CapabilityFlags, 19));
};


/**
 * @param {?proto.g8e.operator.v1.CapabilityFlags|undefined} value
 * @return {!proto.g8e.operator.v1.HeartbeatResult} returns this
*/
proto.g8e.operator.v1.HeartbeatResult.prototype.setCapabilityFlags = function(value) {
  return jspb.Message.setWrapperField(this, 19, value);
};


/**
 * Clears the message field making it undefined.
 * @return {!proto.g8e.operator.v1.HeartbeatResult} returns this
 */
proto.g8e.operator.v1.HeartbeatResult.prototype.clearCapabilityFlags = function() {
  return this.setCapabilityFlags(undefined);
};


/**
 * Returns whether this field is set.
 * @return {boolean}
 */
proto.g8e.operator.v1.HeartbeatResult.prototype.hasCapabilityFlags = function() {
  return jspb.Message.getField(this, 19) != null;
};


/**
 * optional FingerprintDetails fingerprint_details = 20;
 * @return {?proto.g8e.operator.v1.FingerprintDetails}
 */
proto.g8e.operator.v1.HeartbeatResult.prototype.getFingerprintDetails = function() {
  return /** @type{?proto.g8e.operator.v1.FingerprintDetails} */ (
    jspb.Message.getWrapperField(this, proto.g8e.operator.v1.FingerprintDetails, 20));
};


/**
 * @param {?proto.g8e.operator.v1.FingerprintDetails|undefined} value
 * @return {!proto.g8e.operator.v1.HeartbeatResult} returns this
*/
proto.g8e.operator.v1.HeartbeatResult.prototype.setFingerprintDetails = function(value) {
  return jspb.Message.setWrapperField(this, 20, value);
};


/**
 * Clears the message field making it undefined.
 * @return {!proto.g8e.operator.v1.HeartbeatResult} returns this
 */
proto.g8e.operator.v1.HeartbeatResult.prototype.clearFingerprintDetails = function() {
  return this.setFingerprintDetails(undefined);
};


/**
 * Returns whether this field is set.
 * @return {boolean}
 */
proto.g8e.operator.v1.HeartbeatResult.prototype.hasFingerprintDetails = function() {
  return jspb.Message.getField(this, 20) != null;
};


/**
 * optional string system_fingerprint = 21;
 * @return {string}
 */
proto.g8e.operator.v1.HeartbeatResult.prototype.getSystemFingerprint = function() {
  return /** @type {string} */ (jspb.Message.getFieldWithDefault(this, 21, ""));
};


/**
 * @param {string} value
 * @return {!proto.g8e.operator.v1.HeartbeatResult} returns this
 */
proto.g8e.operator.v1.HeartbeatResult.prototype.setSystemFingerprint = function(value) {
  return jspb.Message.setProto3StringField(this, 21, value);
};


/**
 * optional string api_key = 22;
 * @return {string}
 */
proto.g8e.operator.v1.HeartbeatResult.prototype.getApiKey = function() {
  return /** @type {string} */ (jspb.Message.getFieldWithDefault(this, 22, ""));
};


/**
 * @param {string} value
 * @return {!proto.g8e.operator.v1.HeartbeatResult} returns this
 */
proto.g8e.operator.v1.HeartbeatResult.prototype.setApiKey = function(value) {
  return jspb.Message.setProto3StringField(this, 22, value);
};





if (jspb.Message.GENERATE_TO_OBJECT) {
/**
 * Creates an object representation of this proto.
 * Field names that are reserved in JavaScript and will be renamed to pb_name.
 * Optional fields that are not set will be set to undefined.
 * To access a reserved field use, foo.pb_<name>, eg, foo.pb_default.
 * For the list of reserved names please see:
 *     net/proto2/compiler/js/internal/generator.cc#kKeyword.
 * @param {boolean=} opt_includeInstance Deprecated. whether to include the
 *     JSPB instance for transitional soy proto support:
 *     http://goto/soy-param-migration
 * @return {!Object}
 */
proto.g8e.operator.v1.SystemIdentity.prototype.toObject = function(opt_includeInstance) {
  return proto.g8e.operator.v1.SystemIdentity.toObject(opt_includeInstance, this);
};


/**
 * Static version of the {@see toObject} method.
 * @param {boolean|undefined} includeInstance Deprecated. Whether to include
 *     the JSPB instance for transitional soy proto support:
 *     http://goto/soy-param-migration
 * @param {!proto.g8e.operator.v1.SystemIdentity} msg The msg instance to transform.
 * @return {!Object}
 * @suppress {unusedLocalVariables} f is only used for nested messages
 */
proto.g8e.operator.v1.SystemIdentity.toObject = function(includeInstance, msg) {
  var f, obj = {
    hostname: jspb.Message.getFieldWithDefault(msg, 1, ""),
    os: jspb.Message.getFieldWithDefault(msg, 2, ""),
    architecture: jspb.Message.getFieldWithDefault(msg, 3, ""),
    pwd: jspb.Message.getFieldWithDefault(msg, 4, ""),
    currentUser: jspb.Message.getFieldWithDefault(msg, 5, ""),
    cpuCount: jspb.Message.getFieldWithDefault(msg, 6, 0),
    memoryMb: jspb.Message.getFieldWithDefault(msg, 7, 0)
  };

  if (includeInstance) {
    obj.$jspbMessageInstance = msg;
  }
  return obj;
};
}


/**
 * Deserializes binary data (in protobuf wire format).
 * @param {jspb.ByteSource} bytes The bytes to deserialize.
 * @return {!proto.g8e.operator.v1.SystemIdentity}
 */
proto.g8e.operator.v1.SystemIdentity.deserializeBinary = function(bytes) {
  var reader = new jspb.BinaryReader(bytes);
  var msg = new proto.g8e.operator.v1.SystemIdentity;
  return proto.g8e.operator.v1.SystemIdentity.deserializeBinaryFromReader(msg, reader);
};


/**
 * Deserializes binary data (in protobuf wire format) from the
 * given reader into the given message object.
 * @param {!proto.g8e.operator.v1.SystemIdentity} msg The message object to deserialize into.
 * @param {!jspb.BinaryReader} reader The BinaryReader to use.
 * @return {!proto.g8e.operator.v1.SystemIdentity}
 */
proto.g8e.operator.v1.SystemIdentity.deserializeBinaryFromReader = function(msg, reader) {
  while (reader.nextField()) {
    if (reader.isEndGroup()) {
      break;
    }
    var field = reader.getFieldNumber();
    switch (field) {
    case 1:
      var value = /** @type {string} */ (reader.readString());
      msg.setHostname(value);
      break;
    case 2:
      var value = /** @type {string} */ (reader.readString());
      msg.setOs(value);
      break;
    case 3:
      var value = /** @type {string} */ (reader.readString());
      msg.setArchitecture(value);
      break;
    case 4:
      var value = /** @type {string} */ (reader.readString());
      msg.setPwd(value);
      break;
    case 5:
      var value = /** @type {string} */ (reader.readString());
      msg.setCurrentUser(value);
      break;
    case 6:
      var value = /** @type {number} */ (reader.readInt32());
      msg.setCpuCount(value);
      break;
    case 7:
      var value = /** @type {number} */ (reader.readInt32());
      msg.setMemoryMb(value);
      break;
    default:
      reader.skipField();
      break;
    }
  }
  return msg;
};


/**
 * Serializes the message to binary data (in protobuf wire format).
 * @return {!Uint8Array}
 */
proto.g8e.operator.v1.SystemIdentity.prototype.serializeBinary = function() {
  var writer = new jspb.BinaryWriter();
  proto.g8e.operator.v1.SystemIdentity.serializeBinaryToWriter(this, writer);
  return writer.getResultBuffer();
};


/**
 * Serializes the given message to binary data (in protobuf wire
 * format), writing to the given BinaryWriter.
 * @param {!proto.g8e.operator.v1.SystemIdentity} message
 * @param {!jspb.BinaryWriter} writer
 * @suppress {unusedLocalVariables} f is only used for nested messages
 */
proto.g8e.operator.v1.SystemIdentity.serializeBinaryToWriter = function(message, writer) {
  var f = undefined;
  f = message.getHostname();
  if (f.length > 0) {
    writer.writeString(
      1,
      f
    );
  }
  f = message.getOs();
  if (f.length > 0) {
    writer.writeString(
      2,
      f
    );
  }
  f = message.getArchitecture();
  if (f.length > 0) {
    writer.writeString(
      3,
      f
    );
  }
  f = message.getPwd();
  if (f.length > 0) {
    writer.writeString(
      4,
      f
    );
  }
  f = message.getCurrentUser();
  if (f.length > 0) {
    writer.writeString(
      5,
      f
    );
  }
  f = message.getCpuCount();
  if (f !== 0) {
    writer.writeInt32(
      6,
      f
    );
  }
  f = message.getMemoryMb();
  if (f !== 0) {
    writer.writeInt32(
      7,
      f
    );
  }
};


/**
 * optional string hostname = 1;
 * @return {string}
 */
proto.g8e.operator.v1.SystemIdentity.prototype.getHostname = function() {
  return /** @type {string} */ (jspb.Message.getFieldWithDefault(this, 1, ""));
};


/**
 * @param {string} value
 * @return {!proto.g8e.operator.v1.SystemIdentity} returns this
 */
proto.g8e.operator.v1.SystemIdentity.prototype.setHostname = function(value) {
  return jspb.Message.setProto3StringField(this, 1, value);
};


/**
 * optional string os = 2;
 * @return {string}
 */
proto.g8e.operator.v1.SystemIdentity.prototype.getOs = function() {
  return /** @type {string} */ (jspb.Message.getFieldWithDefault(this, 2, ""));
};


/**
 * @param {string} value
 * @return {!proto.g8e.operator.v1.SystemIdentity} returns this
 */
proto.g8e.operator.v1.SystemIdentity.prototype.setOs = function(value) {
  return jspb.Message.setProto3StringField(this, 2, value);
};


/**
 * optional string architecture = 3;
 * @return {string}
 */
proto.g8e.operator.v1.SystemIdentity.prototype.getArchitecture = function() {
  return /** @type {string} */ (jspb.Message.getFieldWithDefault(this, 3, ""));
};


/**
 * @param {string} value
 * @return {!proto.g8e.operator.v1.SystemIdentity} returns this
 */
proto.g8e.operator.v1.SystemIdentity.prototype.setArchitecture = function(value) {
  return jspb.Message.setProto3StringField(this, 3, value);
};


/**
 * optional string pwd = 4;
 * @return {string}
 */
proto.g8e.operator.v1.SystemIdentity.prototype.getPwd = function() {
  return /** @type {string} */ (jspb.Message.getFieldWithDefault(this, 4, ""));
};


/**
 * @param {string} value
 * @return {!proto.g8e.operator.v1.SystemIdentity} returns this
 */
proto.g8e.operator.v1.SystemIdentity.prototype.setPwd = function(value) {
  return jspb.Message.setProto3StringField(this, 4, value);
};


/**
 * optional string current_user = 5;
 * @return {string}
 */
proto.g8e.operator.v1.SystemIdentity.prototype.getCurrentUser = function() {
  return /** @type {string} */ (jspb.Message.getFieldWithDefault(this, 5, ""));
};


/**
 * @param {string} value
 * @return {!proto.g8e.operator.v1.SystemIdentity} returns this
 */
proto.g8e.operator.v1.SystemIdentity.prototype.setCurrentUser = function(value) {
  return jspb.Message.setProto3StringField(this, 5, value);
};


/**
 * optional int32 cpu_count = 6;
 * @return {number}
 */
proto.g8e.operator.v1.SystemIdentity.prototype.getCpuCount = function() {
  return /** @type {number} */ (jspb.Message.getFieldWithDefault(this, 6, 0));
};


/**
 * @param {number} value
 * @return {!proto.g8e.operator.v1.SystemIdentity} returns this
 */
proto.g8e.operator.v1.SystemIdentity.prototype.setCpuCount = function(value) {
  return jspb.Message.setProto3IntField(this, 6, value);
};


/**
 * optional int32 memory_mb = 7;
 * @return {number}
 */
proto.g8e.operator.v1.SystemIdentity.prototype.getMemoryMb = function() {
  return /** @type {number} */ (jspb.Message.getFieldWithDefault(this, 7, 0));
};


/**
 * @param {number} value
 * @return {!proto.g8e.operator.v1.SystemIdentity} returns this
 */
proto.g8e.operator.v1.SystemIdentity.prototype.setMemoryMb = function(value) {
  return jspb.Message.setProto3IntField(this, 7, value);
};





if (jspb.Message.GENERATE_TO_OBJECT) {
/**
 * Creates an object representation of this proto.
 * Field names that are reserved in JavaScript and will be renamed to pb_name.
 * Optional fields that are not set will be set to undefined.
 * To access a reserved field use, foo.pb_<name>, eg, foo.pb_default.
 * For the list of reserved names please see:
 *     net/proto2/compiler/js/internal/generator.cc#kKeyword.
 * @param {boolean=} opt_includeInstance Deprecated. whether to include the
 *     JSPB instance for transitional soy proto support:
 *     http://goto/soy-param-migration
 * @return {!Object}
 */
proto.g8e.operator.v1.NetworkInterface.prototype.toObject = function(opt_includeInstance) {
  return proto.g8e.operator.v1.NetworkInterface.toObject(opt_includeInstance, this);
};


/**
 * Static version of the {@see toObject} method.
 * @param {boolean|undefined} includeInstance Deprecated. Whether to include
 *     the JSPB instance for transitional soy proto support:
 *     http://goto/soy-param-migration
 * @param {!proto.g8e.operator.v1.NetworkInterface} msg The msg instance to transform.
 * @return {!Object}
 * @suppress {unusedLocalVariables} f is only used for nested messages
 */
proto.g8e.operator.v1.NetworkInterface.toObject = function(includeInstance, msg) {
  var f, obj = {
    name: jspb.Message.getFieldWithDefault(msg, 1, ""),
    ip: jspb.Message.getFieldWithDefault(msg, 2, ""),
    mtu: jspb.Message.getFieldWithDefault(msg, 3, 0)
  };

  if (includeInstance) {
    obj.$jspbMessageInstance = msg;
  }
  return obj;
};
}


/**
 * Deserializes binary data (in protobuf wire format).
 * @param {jspb.ByteSource} bytes The bytes to deserialize.
 * @return {!proto.g8e.operator.v1.NetworkInterface}
 */
proto.g8e.operator.v1.NetworkInterface.deserializeBinary = function(bytes) {
  var reader = new jspb.BinaryReader(bytes);
  var msg = new proto.g8e.operator.v1.NetworkInterface;
  return proto.g8e.operator.v1.NetworkInterface.deserializeBinaryFromReader(msg, reader);
};


/**
 * Deserializes binary data (in protobuf wire format) from the
 * given reader into the given message object.
 * @param {!proto.g8e.operator.v1.NetworkInterface} msg The message object to deserialize into.
 * @param {!jspb.BinaryReader} reader The BinaryReader to use.
 * @return {!proto.g8e.operator.v1.NetworkInterface}
 */
proto.g8e.operator.v1.NetworkInterface.deserializeBinaryFromReader = function(msg, reader) {
  while (reader.nextField()) {
    if (reader.isEndGroup()) {
      break;
    }
    var field = reader.getFieldNumber();
    switch (field) {
    case 1:
      var value = /** @type {string} */ (reader.readString());
      msg.setName(value);
      break;
    case 2:
      var value = /** @type {string} */ (reader.readString());
      msg.setIp(value);
      break;
    case 3:
      var value = /** @type {number} */ (reader.readInt32());
      msg.setMtu(value);
      break;
    default:
      reader.skipField();
      break;
    }
  }
  return msg;
};


/**
 * Serializes the message to binary data (in protobuf wire format).
 * @return {!Uint8Array}
 */
proto.g8e.operator.v1.NetworkInterface.prototype.serializeBinary = function() {
  var writer = new jspb.BinaryWriter();
  proto.g8e.operator.v1.NetworkInterface.serializeBinaryToWriter(this, writer);
  return writer.getResultBuffer();
};


/**
 * Serializes the given message to binary data (in protobuf wire
 * format), writing to the given BinaryWriter.
 * @param {!proto.g8e.operator.v1.NetworkInterface} message
 * @param {!jspb.BinaryWriter} writer
 * @suppress {unusedLocalVariables} f is only used for nested messages
 */
proto.g8e.operator.v1.NetworkInterface.serializeBinaryToWriter = function(message, writer) {
  var f = undefined;
  f = message.getName();
  if (f.length > 0) {
    writer.writeString(
      1,
      f
    );
  }
  f = message.getIp();
  if (f.length > 0) {
    writer.writeString(
      2,
      f
    );
  }
  f = message.getMtu();
  if (f !== 0) {
    writer.writeInt32(
      3,
      f
    );
  }
};


/**
 * optional string name = 1;
 * @return {string}
 */
proto.g8e.operator.v1.NetworkInterface.prototype.getName = function() {
  return /** @type {string} */ (jspb.Message.getFieldWithDefault(this, 1, ""));
};


/**
 * @param {string} value
 * @return {!proto.g8e.operator.v1.NetworkInterface} returns this
 */
proto.g8e.operator.v1.NetworkInterface.prototype.setName = function(value) {
  return jspb.Message.setProto3StringField(this, 1, value);
};


/**
 * optional string ip = 2;
 * @return {string}
 */
proto.g8e.operator.v1.NetworkInterface.prototype.getIp = function() {
  return /** @type {string} */ (jspb.Message.getFieldWithDefault(this, 2, ""));
};


/**
 * @param {string} value
 * @return {!proto.g8e.operator.v1.NetworkInterface} returns this
 */
proto.g8e.operator.v1.NetworkInterface.prototype.setIp = function(value) {
  return jspb.Message.setProto3StringField(this, 2, value);
};


/**
 * optional int32 mtu = 3;
 * @return {number}
 */
proto.g8e.operator.v1.NetworkInterface.prototype.getMtu = function() {
  return /** @type {number} */ (jspb.Message.getFieldWithDefault(this, 3, 0));
};


/**
 * @param {number} value
 * @return {!proto.g8e.operator.v1.NetworkInterface} returns this
 */
proto.g8e.operator.v1.NetworkInterface.prototype.setMtu = function(value) {
  return jspb.Message.setProto3IntField(this, 3, value);
};



/**
 * List of repeated fields within this message type.
 * @private {!Array<number>}
 * @const
 */
proto.g8e.operator.v1.NetworkInfo.repeatedFields_ = [3,4];



if (jspb.Message.GENERATE_TO_OBJECT) {
/**
 * Creates an object representation of this proto.
 * Field names that are reserved in JavaScript and will be renamed to pb_name.
 * Optional fields that are not set will be set to undefined.
 * To access a reserved field use, foo.pb_<name>, eg, foo.pb_default.
 * For the list of reserved names please see:
 *     net/proto2/compiler/js/internal/generator.cc#kKeyword.
 * @param {boolean=} opt_includeInstance Deprecated. whether to include the
 *     JSPB instance for transitional soy proto support:
 *     http://goto/soy-param-migration
 * @return {!Object}
 */
proto.g8e.operator.v1.NetworkInfo.prototype.toObject = function(opt_includeInstance) {
  return proto.g8e.operator.v1.NetworkInfo.toObject(opt_includeInstance, this);
};


/**
 * Static version of the {@see toObject} method.
 * @param {boolean|undefined} includeInstance Deprecated. Whether to include
 *     the JSPB instance for transitional soy proto support:
 *     http://goto/soy-param-migration
 * @param {!proto.g8e.operator.v1.NetworkInfo} msg The msg instance to transform.
 * @return {!Object}
 * @suppress {unusedLocalVariables} f is only used for nested messages
 */
proto.g8e.operator.v1.NetworkInfo.toObject = function(includeInstance, msg) {
  var f, obj = {
    publicIp: jspb.Message.getFieldWithDefault(msg, 1, ""),
    internalIp: jspb.Message.getFieldWithDefault(msg, 2, ""),
    interfacesList: (f = jspb.Message.getRepeatedField(msg, 3)) == null ? undefined : f,
    connectivityStatusList: jspb.Message.toObjectList(msg.getConnectivityStatusList(),
    proto.g8e.operator.v1.NetworkInterface.toObject, includeInstance)
  };

  if (includeInstance) {
    obj.$jspbMessageInstance = msg;
  }
  return obj;
};
}


/**
 * Deserializes binary data (in protobuf wire format).
 * @param {jspb.ByteSource} bytes The bytes to deserialize.
 * @return {!proto.g8e.operator.v1.NetworkInfo}
 */
proto.g8e.operator.v1.NetworkInfo.deserializeBinary = function(bytes) {
  var reader = new jspb.BinaryReader(bytes);
  var msg = new proto.g8e.operator.v1.NetworkInfo;
  return proto.g8e.operator.v1.NetworkInfo.deserializeBinaryFromReader(msg, reader);
};


/**
 * Deserializes binary data (in protobuf wire format) from the
 * given reader into the given message object.
 * @param {!proto.g8e.operator.v1.NetworkInfo} msg The message object to deserialize into.
 * @param {!jspb.BinaryReader} reader The BinaryReader to use.
 * @return {!proto.g8e.operator.v1.NetworkInfo}
 */
proto.g8e.operator.v1.NetworkInfo.deserializeBinaryFromReader = function(msg, reader) {
  while (reader.nextField()) {
    if (reader.isEndGroup()) {
      break;
    }
    var field = reader.getFieldNumber();
    switch (field) {
    case 1:
      var value = /** @type {string} */ (reader.readString());
      msg.setPublicIp(value);
      break;
    case 2:
      var value = /** @type {string} */ (reader.readString());
      msg.setInternalIp(value);
      break;
    case 3:
      var value = /** @type {string} */ (reader.readString());
      msg.addInterfaces(value);
      break;
    case 4:
      var value = new proto.g8e.operator.v1.NetworkInterface;
      reader.readMessage(value,proto.g8e.operator.v1.NetworkInterface.deserializeBinaryFromReader);
      msg.addConnectivityStatus(value);
      break;
    default:
      reader.skipField();
      break;
    }
  }
  return msg;
};


/**
 * Serializes the message to binary data (in protobuf wire format).
 * @return {!Uint8Array}
 */
proto.g8e.operator.v1.NetworkInfo.prototype.serializeBinary = function() {
  var writer = new jspb.BinaryWriter();
  proto.g8e.operator.v1.NetworkInfo.serializeBinaryToWriter(this, writer);
  return writer.getResultBuffer();
};


/**
 * Serializes the given message to binary data (in protobuf wire
 * format), writing to the given BinaryWriter.
 * @param {!proto.g8e.operator.v1.NetworkInfo} message
 * @param {!jspb.BinaryWriter} writer
 * @suppress {unusedLocalVariables} f is only used for nested messages
 */
proto.g8e.operator.v1.NetworkInfo.serializeBinaryToWriter = function(message, writer) {
  var f = undefined;
  f = message.getPublicIp();
  if (f.length > 0) {
    writer.writeString(
      1,
      f
    );
  }
  f = message.getInternalIp();
  if (f.length > 0) {
    writer.writeString(
      2,
      f
    );
  }
  f = message.getInterfacesList();
  if (f.length > 0) {
    writer.writeRepeatedString(
      3,
      f
    );
  }
  f = message.getConnectivityStatusList();
  if (f.length > 0) {
    writer.writeRepeatedMessage(
      4,
      f,
      proto.g8e.operator.v1.NetworkInterface.serializeBinaryToWriter
    );
  }
};


/**
 * optional string public_ip = 1;
 * @return {string}
 */
proto.g8e.operator.v1.NetworkInfo.prototype.getPublicIp = function() {
  return /** @type {string} */ (jspb.Message.getFieldWithDefault(this, 1, ""));
};


/**
 * @param {string} value
 * @return {!proto.g8e.operator.v1.NetworkInfo} returns this
 */
proto.g8e.operator.v1.NetworkInfo.prototype.setPublicIp = function(value) {
  return jspb.Message.setProto3StringField(this, 1, value);
};


/**
 * optional string internal_ip = 2;
 * @return {string}
 */
proto.g8e.operator.v1.NetworkInfo.prototype.getInternalIp = function() {
  return /** @type {string} */ (jspb.Message.getFieldWithDefault(this, 2, ""));
};


/**
 * @param {string} value
 * @return {!proto.g8e.operator.v1.NetworkInfo} returns this
 */
proto.g8e.operator.v1.NetworkInfo.prototype.setInternalIp = function(value) {
  return jspb.Message.setProto3StringField(this, 2, value);
};


/**
 * repeated string interfaces = 3;
 * @return {!Array<string>}
 */
proto.g8e.operator.v1.NetworkInfo.prototype.getInterfacesList = function() {
  return /** @type {!Array<string>} */ (jspb.Message.getRepeatedField(this, 3));
};


/**
 * @param {!Array<string>} value
 * @return {!proto.g8e.operator.v1.NetworkInfo} returns this
 */
proto.g8e.operator.v1.NetworkInfo.prototype.setInterfacesList = function(value) {
  return jspb.Message.setField(this, 3, value || []);
};


/**
 * @param {string} value
 * @param {number=} opt_index
 * @return {!proto.g8e.operator.v1.NetworkInfo} returns this
 */
proto.g8e.operator.v1.NetworkInfo.prototype.addInterfaces = function(value, opt_index) {
  return jspb.Message.addToRepeatedField(this, 3, value, opt_index);
};


/**
 * Clears the list making it empty but non-null.
 * @return {!proto.g8e.operator.v1.NetworkInfo} returns this
 */
proto.g8e.operator.v1.NetworkInfo.prototype.clearInterfacesList = function() {
  return this.setInterfacesList([]);
};


/**
 * repeated NetworkInterface connectivity_status = 4;
 * @return {!Array<!proto.g8e.operator.v1.NetworkInterface>}
 */
proto.g8e.operator.v1.NetworkInfo.prototype.getConnectivityStatusList = function() {
  return /** @type{!Array<!proto.g8e.operator.v1.NetworkInterface>} */ (
    jspb.Message.getRepeatedWrapperField(this, proto.g8e.operator.v1.NetworkInterface, 4));
};


/**
 * @param {!Array<!proto.g8e.operator.v1.NetworkInterface>} value
 * @return {!proto.g8e.operator.v1.NetworkInfo} returns this
*/
proto.g8e.operator.v1.NetworkInfo.prototype.setConnectivityStatusList = function(value) {
  return jspb.Message.setRepeatedWrapperField(this, 4, value);
};


/**
 * @param {!proto.g8e.operator.v1.NetworkInterface=} opt_value
 * @param {number=} opt_index
 * @return {!proto.g8e.operator.v1.NetworkInterface}
 */
proto.g8e.operator.v1.NetworkInfo.prototype.addConnectivityStatus = function(opt_value, opt_index) {
  return jspb.Message.addToRepeatedWrapperField(this, 4, opt_value, proto.g8e.operator.v1.NetworkInterface, opt_index);
};


/**
 * Clears the list making it empty but non-null.
 * @return {!proto.g8e.operator.v1.NetworkInfo} returns this
 */
proto.g8e.operator.v1.NetworkInfo.prototype.clearConnectivityStatusList = function() {
  return this.setConnectivityStatusList([]);
};





if (jspb.Message.GENERATE_TO_OBJECT) {
/**
 * Creates an object representation of this proto.
 * Field names that are reserved in JavaScript and will be renamed to pb_name.
 * Optional fields that are not set will be set to undefined.
 * To access a reserved field use, foo.pb_<name>, eg, foo.pb_default.
 * For the list of reserved names please see:
 *     net/proto2/compiler/js/internal/generator.cc#kKeyword.
 * @param {boolean=} opt_includeInstance Deprecated. whether to include the
 *     JSPB instance for transitional soy proto support:
 *     http://goto/soy-param-migration
 * @return {!Object}
 */
proto.g8e.operator.v1.CapabilityFlags.prototype.toObject = function(opt_includeInstance) {
  return proto.g8e.operator.v1.CapabilityFlags.toObject(opt_includeInstance, this);
};


/**
 * Static version of the {@see toObject} method.
 * @param {boolean|undefined} includeInstance Deprecated. Whether to include
 *     the JSPB instance for transitional soy proto support:
 *     http://goto/soy-param-migration
 * @param {!proto.g8e.operator.v1.CapabilityFlags} msg The msg instance to transform.
 * @return {!Object}
 * @suppress {unusedLocalVariables} f is only used for nested messages
 */
proto.g8e.operator.v1.CapabilityFlags.toObject = function(includeInstance, msg) {
  var f, obj = {
    localStorageEnabled: jspb.Message.getBooleanFieldWithDefault(msg, 1, false),
    gitAvailable: jspb.Message.getBooleanFieldWithDefault(msg, 2, false),
    ledgerMirrorEnabled: jspb.Message.getBooleanFieldWithDefault(msg, 3, false)
  };

  if (includeInstance) {
    obj.$jspbMessageInstance = msg;
  }
  return obj;
};
}


/**
 * Deserializes binary data (in protobuf wire format).
 * @param {jspb.ByteSource} bytes The bytes to deserialize.
 * @return {!proto.g8e.operator.v1.CapabilityFlags}
 */
proto.g8e.operator.v1.CapabilityFlags.deserializeBinary = function(bytes) {
  var reader = new jspb.BinaryReader(bytes);
  var msg = new proto.g8e.operator.v1.CapabilityFlags;
  return proto.g8e.operator.v1.CapabilityFlags.deserializeBinaryFromReader(msg, reader);
};


/**
 * Deserializes binary data (in protobuf wire format) from the
 * given reader into the given message object.
 * @param {!proto.g8e.operator.v1.CapabilityFlags} msg The message object to deserialize into.
 * @param {!jspb.BinaryReader} reader The BinaryReader to use.
 * @return {!proto.g8e.operator.v1.CapabilityFlags}
 */
proto.g8e.operator.v1.CapabilityFlags.deserializeBinaryFromReader = function(msg, reader) {
  while (reader.nextField()) {
    if (reader.isEndGroup()) {
      break;
    }
    var field = reader.getFieldNumber();
    switch (field) {
    case 1:
      var value = /** @type {boolean} */ (reader.readBool());
      msg.setLocalStorageEnabled(value);
      break;
    case 2:
      var value = /** @type {boolean} */ (reader.readBool());
      msg.setGitAvailable(value);
      break;
    case 3:
      var value = /** @type {boolean} */ (reader.readBool());
      msg.setLedgerMirrorEnabled(value);
      break;
    default:
      reader.skipField();
      break;
    }
  }
  return msg;
};


/**
 * Serializes the message to binary data (in protobuf wire format).
 * @return {!Uint8Array}
 */
proto.g8e.operator.v1.CapabilityFlags.prototype.serializeBinary = function() {
  var writer = new jspb.BinaryWriter();
  proto.g8e.operator.v1.CapabilityFlags.serializeBinaryToWriter(this, writer);
  return writer.getResultBuffer();
};


/**
 * Serializes the given message to binary data (in protobuf wire
 * format), writing to the given BinaryWriter.
 * @param {!proto.g8e.operator.v1.CapabilityFlags} message
 * @param {!jspb.BinaryWriter} writer
 * @suppress {unusedLocalVariables} f is only used for nested messages
 */
proto.g8e.operator.v1.CapabilityFlags.serializeBinaryToWriter = function(message, writer) {
  var f = undefined;
  f = message.getLocalStorageEnabled();
  if (f) {
    writer.writeBool(
      1,
      f
    );
  }
  f = message.getGitAvailable();
  if (f) {
    writer.writeBool(
      2,
      f
    );
  }
  f = message.getLedgerMirrorEnabled();
  if (f) {
    writer.writeBool(
      3,
      f
    );
  }
};


/**
 * optional bool local_storage_enabled = 1;
 * @return {boolean}
 */
proto.g8e.operator.v1.CapabilityFlags.prototype.getLocalStorageEnabled = function() {
  return /** @type {boolean} */ (jspb.Message.getBooleanFieldWithDefault(this, 1, false));
};


/**
 * @param {boolean} value
 * @return {!proto.g8e.operator.v1.CapabilityFlags} returns this
 */
proto.g8e.operator.v1.CapabilityFlags.prototype.setLocalStorageEnabled = function(value) {
  return jspb.Message.setProto3BooleanField(this, 1, value);
};


/**
 * optional bool git_available = 2;
 * @return {boolean}
 */
proto.g8e.operator.v1.CapabilityFlags.prototype.getGitAvailable = function() {
  return /** @type {boolean} */ (jspb.Message.getBooleanFieldWithDefault(this, 2, false));
};


/**
 * @param {boolean} value
 * @return {!proto.g8e.operator.v1.CapabilityFlags} returns this
 */
proto.g8e.operator.v1.CapabilityFlags.prototype.setGitAvailable = function(value) {
  return jspb.Message.setProto3BooleanField(this, 2, value);
};


/**
 * optional bool ledger_mirror_enabled = 3;
 * @return {boolean}
 */
proto.g8e.operator.v1.CapabilityFlags.prototype.getLedgerMirrorEnabled = function() {
  return /** @type {boolean} */ (jspb.Message.getBooleanFieldWithDefault(this, 3, false));
};


/**
 * @param {boolean} value
 * @return {!proto.g8e.operator.v1.CapabilityFlags} returns this
 */
proto.g8e.operator.v1.CapabilityFlags.prototype.setLedgerMirrorEnabled = function(value) {
  return jspb.Message.setProto3BooleanField(this, 3, value);
};





if (jspb.Message.GENERATE_TO_OBJECT) {
/**
 * Creates an object representation of this proto.
 * Field names that are reserved in JavaScript and will be renamed to pb_name.
 * Optional fields that are not set will be set to undefined.
 * To access a reserved field use, foo.pb_<name>, eg, foo.pb_default.
 * For the list of reserved names please see:
 *     net/proto2/compiler/js/internal/generator.cc#kKeyword.
 * @param {boolean=} opt_includeInstance Deprecated. whether to include the
 *     JSPB instance for transitional soy proto support:
 *     http://goto/soy-param-migration
 * @return {!Object}
 */
proto.g8e.operator.v1.VersionInfo.prototype.toObject = function(opt_includeInstance) {
  return proto.g8e.operator.v1.VersionInfo.toObject(opt_includeInstance, this);
};


/**
 * Static version of the {@see toObject} method.
 * @param {boolean|undefined} includeInstance Deprecated. Whether to include
 *     the JSPB instance for transitional soy proto support:
 *     http://goto/soy-param-migration
 * @param {!proto.g8e.operator.v1.VersionInfo} msg The msg instance to transform.
 * @return {!Object}
 * @suppress {unusedLocalVariables} f is only used for nested messages
 */
proto.g8e.operator.v1.VersionInfo.toObject = function(includeInstance, msg) {
  var f, obj = {
    operatorVersion: jspb.Message.getFieldWithDefault(msg, 1, ""),
    status: jspb.Message.getFieldWithDefault(msg, 2, "")
  };

  if (includeInstance) {
    obj.$jspbMessageInstance = msg;
  }
  return obj;
};
}


/**
 * Deserializes binary data (in protobuf wire format).
 * @param {jspb.ByteSource} bytes The bytes to deserialize.
 * @return {!proto.g8e.operator.v1.VersionInfo}
 */
proto.g8e.operator.v1.VersionInfo.deserializeBinary = function(bytes) {
  var reader = new jspb.BinaryReader(bytes);
  var msg = new proto.g8e.operator.v1.VersionInfo;
  return proto.g8e.operator.v1.VersionInfo.deserializeBinaryFromReader(msg, reader);
};


/**
 * Deserializes binary data (in protobuf wire format) from the
 * given reader into the given message object.
 * @param {!proto.g8e.operator.v1.VersionInfo} msg The message object to deserialize into.
 * @param {!jspb.BinaryReader} reader The BinaryReader to use.
 * @return {!proto.g8e.operator.v1.VersionInfo}
 */
proto.g8e.operator.v1.VersionInfo.deserializeBinaryFromReader = function(msg, reader) {
  while (reader.nextField()) {
    if (reader.isEndGroup()) {
      break;
    }
    var field = reader.getFieldNumber();
    switch (field) {
    case 1:
      var value = /** @type {string} */ (reader.readString());
      msg.setOperatorVersion(value);
      break;
    case 2:
      var value = /** @type {string} */ (reader.readString());
      msg.setStatus(value);
      break;
    default:
      reader.skipField();
      break;
    }
  }
  return msg;
};


/**
 * Serializes the message to binary data (in protobuf wire format).
 * @return {!Uint8Array}
 */
proto.g8e.operator.v1.VersionInfo.prototype.serializeBinary = function() {
  var writer = new jspb.BinaryWriter();
  proto.g8e.operator.v1.VersionInfo.serializeBinaryToWriter(this, writer);
  return writer.getResultBuffer();
};


/**
 * Serializes the given message to binary data (in protobuf wire
 * format), writing to the given BinaryWriter.
 * @param {!proto.g8e.operator.v1.VersionInfo} message
 * @param {!jspb.BinaryWriter} writer
 * @suppress {unusedLocalVariables} f is only used for nested messages
 */
proto.g8e.operator.v1.VersionInfo.serializeBinaryToWriter = function(message, writer) {
  var f = undefined;
  f = message.getOperatorVersion();
  if (f.length > 0) {
    writer.writeString(
      1,
      f
    );
  }
  f = message.getStatus();
  if (f.length > 0) {
    writer.writeString(
      2,
      f
    );
  }
};


/**
 * optional string operator_version = 1;
 * @return {string}
 */
proto.g8e.operator.v1.VersionInfo.prototype.getOperatorVersion = function() {
  return /** @type {string} */ (jspb.Message.getFieldWithDefault(this, 1, ""));
};


/**
 * @param {string} value
 * @return {!proto.g8e.operator.v1.VersionInfo} returns this
 */
proto.g8e.operator.v1.VersionInfo.prototype.setOperatorVersion = function(value) {
  return jspb.Message.setProto3StringField(this, 1, value);
};


/**
 * optional string status = 2;
 * @return {string}
 */
proto.g8e.operator.v1.VersionInfo.prototype.getStatus = function() {
  return /** @type {string} */ (jspb.Message.getFieldWithDefault(this, 2, ""));
};


/**
 * @param {string} value
 * @return {!proto.g8e.operator.v1.VersionInfo} returns this
 */
proto.g8e.operator.v1.VersionInfo.prototype.setStatus = function(value) {
  return jspb.Message.setProto3StringField(this, 2, value);
};





if (jspb.Message.GENERATE_TO_OBJECT) {
/**
 * Creates an object representation of this proto.
 * Field names that are reserved in JavaScript and will be renamed to pb_name.
 * Optional fields that are not set will be set to undefined.
 * To access a reserved field use, foo.pb_<name>, eg, foo.pb_default.
 * For the list of reserved names please see:
 *     net/proto2/compiler/js/internal/generator.cc#kKeyword.
 * @param {boolean=} opt_includeInstance Deprecated. whether to include the
 *     JSPB instance for transitional soy proto support:
 *     http://goto/soy-param-migration
 * @return {!Object}
 */
proto.g8e.operator.v1.UptimeInfo.prototype.toObject = function(opt_includeInstance) {
  return proto.g8e.operator.v1.UptimeInfo.toObject(opt_includeInstance, this);
};


/**
 * Static version of the {@see toObject} method.
 * @param {boolean|undefined} includeInstance Deprecated. Whether to include
 *     the JSPB instance for transitional soy proto support:
 *     http://goto/soy-param-migration
 * @param {!proto.g8e.operator.v1.UptimeInfo} msg The msg instance to transform.
 * @return {!Object}
 * @suppress {unusedLocalVariables} f is only used for nested messages
 */
proto.g8e.operator.v1.UptimeInfo.toObject = function(includeInstance, msg) {
  var f, obj = {
    uptime: jspb.Message.getFieldWithDefault(msg, 1, ""),
    uptimeSeconds: jspb.Message.getFieldWithDefault(msg, 2, 0)
  };

  if (includeInstance) {
    obj.$jspbMessageInstance = msg;
  }
  return obj;
};
}


/**
 * Deserializes binary data (in protobuf wire format).
 * @param {jspb.ByteSource} bytes The bytes to deserialize.
 * @return {!proto.g8e.operator.v1.UptimeInfo}
 */
proto.g8e.operator.v1.UptimeInfo.deserializeBinary = function(bytes) {
  var reader = new jspb.BinaryReader(bytes);
  var msg = new proto.g8e.operator.v1.UptimeInfo;
  return proto.g8e.operator.v1.UptimeInfo.deserializeBinaryFromReader(msg, reader);
};


/**
 * Deserializes binary data (in protobuf wire format) from the
 * given reader into the given message object.
 * @param {!proto.g8e.operator.v1.UptimeInfo} msg The message object to deserialize into.
 * @param {!jspb.BinaryReader} reader The BinaryReader to use.
 * @return {!proto.g8e.operator.v1.UptimeInfo}
 */
proto.g8e.operator.v1.UptimeInfo.deserializeBinaryFromReader = function(msg, reader) {
  while (reader.nextField()) {
    if (reader.isEndGroup()) {
      break;
    }
    var field = reader.getFieldNumber();
    switch (field) {
    case 1:
      var value = /** @type {string} */ (reader.readString());
      msg.setUptime(value);
      break;
    case 2:
      var value = /** @type {number} */ (reader.readInt64());
      msg.setUptimeSeconds(value);
      break;
    default:
      reader.skipField();
      break;
    }
  }
  return msg;
};


/**
 * Serializes the message to binary data (in protobuf wire format).
 * @return {!Uint8Array}
 */
proto.g8e.operator.v1.UptimeInfo.prototype.serializeBinary = function() {
  var writer = new jspb.BinaryWriter();
  proto.g8e.operator.v1.UptimeInfo.serializeBinaryToWriter(this, writer);
  return writer.getResultBuffer();
};


/**
 * Serializes the given message to binary data (in protobuf wire
 * format), writing to the given BinaryWriter.
 * @param {!proto.g8e.operator.v1.UptimeInfo} message
 * @param {!jspb.BinaryWriter} writer
 * @suppress {unusedLocalVariables} f is only used for nested messages
 */
proto.g8e.operator.v1.UptimeInfo.serializeBinaryToWriter = function(message, writer) {
  var f = undefined;
  f = message.getUptime();
  if (f.length > 0) {
    writer.writeString(
      1,
      f
    );
  }
  f = message.getUptimeSeconds();
  if (f !== 0) {
    writer.writeInt64(
      2,
      f
    );
  }
};


/**
 * optional string uptime = 1;
 * @return {string}
 */
proto.g8e.operator.v1.UptimeInfo.prototype.getUptime = function() {
  return /** @type {string} */ (jspb.Message.getFieldWithDefault(this, 1, ""));
};


/**
 * @param {string} value
 * @return {!proto.g8e.operator.v1.UptimeInfo} returns this
 */
proto.g8e.operator.v1.UptimeInfo.prototype.setUptime = function(value) {
  return jspb.Message.setProto3StringField(this, 1, value);
};


/**
 * optional int64 uptime_seconds = 2;
 * @return {number}
 */
proto.g8e.operator.v1.UptimeInfo.prototype.getUptimeSeconds = function() {
  return /** @type {number} */ (jspb.Message.getFieldWithDefault(this, 2, 0));
};


/**
 * @param {number} value
 * @return {!proto.g8e.operator.v1.UptimeInfo} returns this
 */
proto.g8e.operator.v1.UptimeInfo.prototype.setUptimeSeconds = function(value) {
  return jspb.Message.setProto3IntField(this, 2, value);
};





if (jspb.Message.GENERATE_TO_OBJECT) {
/**
 * Creates an object representation of this proto.
 * Field names that are reserved in JavaScript and will be renamed to pb_name.
 * Optional fields that are not set will be set to undefined.
 * To access a reserved field use, foo.pb_<name>, eg, foo.pb_default.
 * For the list of reserved names please see:
 *     net/proto2/compiler/js/internal/generator.cc#kKeyword.
 * @param {boolean=} opt_includeInstance Deprecated. whether to include the
 *     JSPB instance for transitional soy proto support:
 *     http://goto/soy-param-migration
 * @return {!Object}
 */
proto.g8e.operator.v1.PerformanceMetrics.prototype.toObject = function(opt_includeInstance) {
  return proto.g8e.operator.v1.PerformanceMetrics.toObject(opt_includeInstance, this);
};


/**
 * Static version of the {@see toObject} method.
 * @param {boolean|undefined} includeInstance Deprecated. Whether to include
 *     the JSPB instance for transitional soy proto support:
 *     http://goto/soy-param-migration
 * @param {!proto.g8e.operator.v1.PerformanceMetrics} msg The msg instance to transform.
 * @return {!Object}
 * @suppress {unusedLocalVariables} f is only used for nested messages
 */
proto.g8e.operator.v1.PerformanceMetrics.toObject = function(includeInstance, msg) {
  var f, obj = {
    cpuPercent: jspb.Message.getFloatingPointFieldWithDefault(msg, 1, 0.0),
    memoryPercent: jspb.Message.getFloatingPointFieldWithDefault(msg, 2, 0.0),
    diskPercent: jspb.Message.getFloatingPointFieldWithDefault(msg, 3, 0.0),
    networkLatency: jspb.Message.getFloatingPointFieldWithDefault(msg, 4, 0.0),
    memoryUsedMb: jspb.Message.getFieldWithDefault(msg, 5, 0),
    memoryTotalMb: jspb.Message.getFieldWithDefault(msg, 6, 0),
    diskUsedGb: jspb.Message.getFloatingPointFieldWithDefault(msg, 7, 0.0),
    diskTotalGb: jspb.Message.getFloatingPointFieldWithDefault(msg, 8, 0.0)
  };

  if (includeInstance) {
    obj.$jspbMessageInstance = msg;
  }
  return obj;
};
}


/**
 * Deserializes binary data (in protobuf wire format).
 * @param {jspb.ByteSource} bytes The bytes to deserialize.
 * @return {!proto.g8e.operator.v1.PerformanceMetrics}
 */
proto.g8e.operator.v1.PerformanceMetrics.deserializeBinary = function(bytes) {
  var reader = new jspb.BinaryReader(bytes);
  var msg = new proto.g8e.operator.v1.PerformanceMetrics;
  return proto.g8e.operator.v1.PerformanceMetrics.deserializeBinaryFromReader(msg, reader);
};


/**
 * Deserializes binary data (in protobuf wire format) from the
 * given reader into the given message object.
 * @param {!proto.g8e.operator.v1.PerformanceMetrics} msg The message object to deserialize into.
 * @param {!jspb.BinaryReader} reader The BinaryReader to use.
 * @return {!proto.g8e.operator.v1.PerformanceMetrics}
 */
proto.g8e.operator.v1.PerformanceMetrics.deserializeBinaryFromReader = function(msg, reader) {
  while (reader.nextField()) {
    if (reader.isEndGroup()) {
      break;
    }
    var field = reader.getFieldNumber();
    switch (field) {
    case 1:
      var value = /** @type {number} */ (reader.readDouble());
      msg.setCpuPercent(value);
      break;
    case 2:
      var value = /** @type {number} */ (reader.readDouble());
      msg.setMemoryPercent(value);
      break;
    case 3:
      var value = /** @type {number} */ (reader.readDouble());
      msg.setDiskPercent(value);
      break;
    case 4:
      var value = /** @type {number} */ (reader.readDouble());
      msg.setNetworkLatency(value);
      break;
    case 5:
      var value = /** @type {number} */ (reader.readInt32());
      msg.setMemoryUsedMb(value);
      break;
    case 6:
      var value = /** @type {number} */ (reader.readInt32());
      msg.setMemoryTotalMb(value);
      break;
    case 7:
      var value = /** @type {number} */ (reader.readDouble());
      msg.setDiskUsedGb(value);
      break;
    case 8:
      var value = /** @type {number} */ (reader.readDouble());
      msg.setDiskTotalGb(value);
      break;
    default:
      reader.skipField();
      break;
    }
  }
  return msg;
};


/**
 * Serializes the message to binary data (in protobuf wire format).
 * @return {!Uint8Array}
 */
proto.g8e.operator.v1.PerformanceMetrics.prototype.serializeBinary = function() {
  var writer = new jspb.BinaryWriter();
  proto.g8e.operator.v1.PerformanceMetrics.serializeBinaryToWriter(this, writer);
  return writer.getResultBuffer();
};


/**
 * Serializes the given message to binary data (in protobuf wire
 * format), writing to the given BinaryWriter.
 * @param {!proto.g8e.operator.v1.PerformanceMetrics} message
 * @param {!jspb.BinaryWriter} writer
 * @suppress {unusedLocalVariables} f is only used for nested messages
 */
proto.g8e.operator.v1.PerformanceMetrics.serializeBinaryToWriter = function(message, writer) {
  var f = undefined;
  f = message.getCpuPercent();
  if (f !== 0.0) {
    writer.writeDouble(
      1,
      f
    );
  }
  f = message.getMemoryPercent();
  if (f !== 0.0) {
    writer.writeDouble(
      2,
      f
    );
  }
  f = message.getDiskPercent();
  if (f !== 0.0) {
    writer.writeDouble(
      3,
      f
    );
  }
  f = message.getNetworkLatency();
  if (f !== 0.0) {
    writer.writeDouble(
      4,
      f
    );
  }
  f = message.getMemoryUsedMb();
  if (f !== 0) {
    writer.writeInt32(
      5,
      f
    );
  }
  f = message.getMemoryTotalMb();
  if (f !== 0) {
    writer.writeInt32(
      6,
      f
    );
  }
  f = message.getDiskUsedGb();
  if (f !== 0.0) {
    writer.writeDouble(
      7,
      f
    );
  }
  f = message.getDiskTotalGb();
  if (f !== 0.0) {
    writer.writeDouble(
      8,
      f
    );
  }
};


/**
 * optional double cpu_percent = 1;
 * @return {number}
 */
proto.g8e.operator.v1.PerformanceMetrics.prototype.getCpuPercent = function() {
  return /** @type {number} */ (jspb.Message.getFloatingPointFieldWithDefault(this, 1, 0.0));
};


/**
 * @param {number} value
 * @return {!proto.g8e.operator.v1.PerformanceMetrics} returns this
 */
proto.g8e.operator.v1.PerformanceMetrics.prototype.setCpuPercent = function(value) {
  return jspb.Message.setProto3FloatField(this, 1, value);
};


/**
 * optional double memory_percent = 2;
 * @return {number}
 */
proto.g8e.operator.v1.PerformanceMetrics.prototype.getMemoryPercent = function() {
  return /** @type {number} */ (jspb.Message.getFloatingPointFieldWithDefault(this, 2, 0.0));
};


/**
 * @param {number} value
 * @return {!proto.g8e.operator.v1.PerformanceMetrics} returns this
 */
proto.g8e.operator.v1.PerformanceMetrics.prototype.setMemoryPercent = function(value) {
  return jspb.Message.setProto3FloatField(this, 2, value);
};


/**
 * optional double disk_percent = 3;
 * @return {number}
 */
proto.g8e.operator.v1.PerformanceMetrics.prototype.getDiskPercent = function() {
  return /** @type {number} */ (jspb.Message.getFloatingPointFieldWithDefault(this, 3, 0.0));
};


/**
 * @param {number} value
 * @return {!proto.g8e.operator.v1.PerformanceMetrics} returns this
 */
proto.g8e.operator.v1.PerformanceMetrics.prototype.setDiskPercent = function(value) {
  return jspb.Message.setProto3FloatField(this, 3, value);
};


/**
 * optional double network_latency = 4;
 * @return {number}
 */
proto.g8e.operator.v1.PerformanceMetrics.prototype.getNetworkLatency = function() {
  return /** @type {number} */ (jspb.Message.getFloatingPointFieldWithDefault(this, 4, 0.0));
};


/**
 * @param {number} value
 * @return {!proto.g8e.operator.v1.PerformanceMetrics} returns this
 */
proto.g8e.operator.v1.PerformanceMetrics.prototype.setNetworkLatency = function(value) {
  return jspb.Message.setProto3FloatField(this, 4, value);
};


/**
 * optional int32 memory_used_mb = 5;
 * @return {number}
 */
proto.g8e.operator.v1.PerformanceMetrics.prototype.getMemoryUsedMb = function() {
  return /** @type {number} */ (jspb.Message.getFieldWithDefault(this, 5, 0));
};


/**
 * @param {number} value
 * @return {!proto.g8e.operator.v1.PerformanceMetrics} returns this
 */
proto.g8e.operator.v1.PerformanceMetrics.prototype.setMemoryUsedMb = function(value) {
  return jspb.Message.setProto3IntField(this, 5, value);
};


/**
 * optional int32 memory_total_mb = 6;
 * @return {number}
 */
proto.g8e.operator.v1.PerformanceMetrics.prototype.getMemoryTotalMb = function() {
  return /** @type {number} */ (jspb.Message.getFieldWithDefault(this, 6, 0));
};


/**
 * @param {number} value
 * @return {!proto.g8e.operator.v1.PerformanceMetrics} returns this
 */
proto.g8e.operator.v1.PerformanceMetrics.prototype.setMemoryTotalMb = function(value) {
  return jspb.Message.setProto3IntField(this, 6, value);
};


/**
 * optional double disk_used_gb = 7;
 * @return {number}
 */
proto.g8e.operator.v1.PerformanceMetrics.prototype.getDiskUsedGb = function() {
  return /** @type {number} */ (jspb.Message.getFloatingPointFieldWithDefault(this, 7, 0.0));
};


/**
 * @param {number} value
 * @return {!proto.g8e.operator.v1.PerformanceMetrics} returns this
 */
proto.g8e.operator.v1.PerformanceMetrics.prototype.setDiskUsedGb = function(value) {
  return jspb.Message.setProto3FloatField(this, 7, value);
};


/**
 * optional double disk_total_gb = 8;
 * @return {number}
 */
proto.g8e.operator.v1.PerformanceMetrics.prototype.getDiskTotalGb = function() {
  return /** @type {number} */ (jspb.Message.getFloatingPointFieldWithDefault(this, 8, 0.0));
};


/**
 * @param {number} value
 * @return {!proto.g8e.operator.v1.PerformanceMetrics} returns this
 */
proto.g8e.operator.v1.PerformanceMetrics.prototype.setDiskTotalGb = function(value) {
  return jspb.Message.setProto3FloatField(this, 8, value);
};





if (jspb.Message.GENERATE_TO_OBJECT) {
/**
 * Creates an object representation of this proto.
 * Field names that are reserved in JavaScript and will be renamed to pb_name.
 * Optional fields that are not set will be set to undefined.
 * To access a reserved field use, foo.pb_<name>, eg, foo.pb_default.
 * For the list of reserved names please see:
 *     net/proto2/compiler/js/internal/generator.cc#kKeyword.
 * @param {boolean=} opt_includeInstance Deprecated. whether to include the
 *     JSPB instance for transitional soy proto support:
 *     http://goto/soy-param-migration
 * @return {!Object}
 */
proto.g8e.operator.v1.OSDetails.prototype.toObject = function(opt_includeInstance) {
  return proto.g8e.operator.v1.OSDetails.toObject(opt_includeInstance, this);
};


/**
 * Static version of the {@see toObject} method.
 * @param {boolean|undefined} includeInstance Deprecated. Whether to include
 *     the JSPB instance for transitional soy proto support:
 *     http://goto/soy-param-migration
 * @param {!proto.g8e.operator.v1.OSDetails} msg The msg instance to transform.
 * @return {!Object}
 * @suppress {unusedLocalVariables} f is only used for nested messages
 */
proto.g8e.operator.v1.OSDetails.toObject = function(includeInstance, msg) {
  var f, obj = {
    kernel: jspb.Message.getFieldWithDefault(msg, 1, ""),
    distro: jspb.Message.getFieldWithDefault(msg, 2, ""),
    version: jspb.Message.getFieldWithDefault(msg, 3, "")
  };

  if (includeInstance) {
    obj.$jspbMessageInstance = msg;
  }
  return obj;
};
}


/**
 * Deserializes binary data (in protobuf wire format).
 * @param {jspb.ByteSource} bytes The bytes to deserialize.
 * @return {!proto.g8e.operator.v1.OSDetails}
 */
proto.g8e.operator.v1.OSDetails.deserializeBinary = function(bytes) {
  var reader = new jspb.BinaryReader(bytes);
  var msg = new proto.g8e.operator.v1.OSDetails;
  return proto.g8e.operator.v1.OSDetails.deserializeBinaryFromReader(msg, reader);
};


/**
 * Deserializes binary data (in protobuf wire format) from the
 * given reader into the given message object.
 * @param {!proto.g8e.operator.v1.OSDetails} msg The message object to deserialize into.
 * @param {!jspb.BinaryReader} reader The BinaryReader to use.
 * @return {!proto.g8e.operator.v1.OSDetails}
 */
proto.g8e.operator.v1.OSDetails.deserializeBinaryFromReader = function(msg, reader) {
  while (reader.nextField()) {
    if (reader.isEndGroup()) {
      break;
    }
    var field = reader.getFieldNumber();
    switch (field) {
    case 1:
      var value = /** @type {string} */ (reader.readString());
      msg.setKernel(value);
      break;
    case 2:
      var value = /** @type {string} */ (reader.readString());
      msg.setDistro(value);
      break;
    case 3:
      var value = /** @type {string} */ (reader.readString());
      msg.setVersion(value);
      break;
    default:
      reader.skipField();
      break;
    }
  }
  return msg;
};


/**
 * Serializes the message to binary data (in protobuf wire format).
 * @return {!Uint8Array}
 */
proto.g8e.operator.v1.OSDetails.prototype.serializeBinary = function() {
  var writer = new jspb.BinaryWriter();
  proto.g8e.operator.v1.OSDetails.serializeBinaryToWriter(this, writer);
  return writer.getResultBuffer();
};


/**
 * Serializes the given message to binary data (in protobuf wire
 * format), writing to the given BinaryWriter.
 * @param {!proto.g8e.operator.v1.OSDetails} message
 * @param {!jspb.BinaryWriter} writer
 * @suppress {unusedLocalVariables} f is only used for nested messages
 */
proto.g8e.operator.v1.OSDetails.serializeBinaryToWriter = function(message, writer) {
  var f = undefined;
  f = message.getKernel();
  if (f.length > 0) {
    writer.writeString(
      1,
      f
    );
  }
  f = message.getDistro();
  if (f.length > 0) {
    writer.writeString(
      2,
      f
    );
  }
  f = message.getVersion();
  if (f.length > 0) {
    writer.writeString(
      3,
      f
    );
  }
};


/**
 * optional string kernel = 1;
 * @return {string}
 */
proto.g8e.operator.v1.OSDetails.prototype.getKernel = function() {
  return /** @type {string} */ (jspb.Message.getFieldWithDefault(this, 1, ""));
};


/**
 * @param {string} value
 * @return {!proto.g8e.operator.v1.OSDetails} returns this
 */
proto.g8e.operator.v1.OSDetails.prototype.setKernel = function(value) {
  return jspb.Message.setProto3StringField(this, 1, value);
};


/**
 * optional string distro = 2;
 * @return {string}
 */
proto.g8e.operator.v1.OSDetails.prototype.getDistro = function() {
  return /** @type {string} */ (jspb.Message.getFieldWithDefault(this, 2, ""));
};


/**
 * @param {string} value
 * @return {!proto.g8e.operator.v1.OSDetails} returns this
 */
proto.g8e.operator.v1.OSDetails.prototype.setDistro = function(value) {
  return jspb.Message.setProto3StringField(this, 2, value);
};


/**
 * optional string version = 3;
 * @return {string}
 */
proto.g8e.operator.v1.OSDetails.prototype.getVersion = function() {
  return /** @type {string} */ (jspb.Message.getFieldWithDefault(this, 3, ""));
};


/**
 * @param {string} value
 * @return {!proto.g8e.operator.v1.OSDetails} returns this
 */
proto.g8e.operator.v1.OSDetails.prototype.setVersion = function(value) {
  return jspb.Message.setProto3StringField(this, 3, value);
};





if (jspb.Message.GENERATE_TO_OBJECT) {
/**
 * Creates an object representation of this proto.
 * Field names that are reserved in JavaScript and will be renamed to pb_name.
 * Optional fields that are not set will be set to undefined.
 * To access a reserved field use, foo.pb_<name>, eg, foo.pb_default.
 * For the list of reserved names please see:
 *     net/proto2/compiler/js/internal/generator.cc#kKeyword.
 * @param {boolean=} opt_includeInstance Deprecated. whether to include the
 *     JSPB instance for transitional soy proto support:
 *     http://goto/soy-param-migration
 * @return {!Object}
 */
proto.g8e.operator.v1.UserDetails.prototype.toObject = function(opt_includeInstance) {
  return proto.g8e.operator.v1.UserDetails.toObject(opt_includeInstance, this);
};


/**
 * Static version of the {@see toObject} method.
 * @param {boolean|undefined} includeInstance Deprecated. Whether to include
 *     the JSPB instance for transitional soy proto support:
 *     http://goto/soy-param-migration
 * @param {!proto.g8e.operator.v1.UserDetails} msg The msg instance to transform.
 * @return {!Object}
 * @suppress {unusedLocalVariables} f is only used for nested messages
 */
proto.g8e.operator.v1.UserDetails.toObject = function(includeInstance, msg) {
  var f, obj = {
    username: jspb.Message.getFieldWithDefault(msg, 1, ""),
    uid: jspb.Message.getFieldWithDefault(msg, 2, 0),
    gid: jspb.Message.getFieldWithDefault(msg, 3, 0),
    home: jspb.Message.getFieldWithDefault(msg, 4, ""),
    name: jspb.Message.getFieldWithDefault(msg, 5, ""),
    shell: jspb.Message.getFieldWithDefault(msg, 6, "")
  };

  if (includeInstance) {
    obj.$jspbMessageInstance = msg;
  }
  return obj;
};
}


/**
 * Deserializes binary data (in protobuf wire format).
 * @param {jspb.ByteSource} bytes The bytes to deserialize.
 * @return {!proto.g8e.operator.v1.UserDetails}
 */
proto.g8e.operator.v1.UserDetails.deserializeBinary = function(bytes) {
  var reader = new jspb.BinaryReader(bytes);
  var msg = new proto.g8e.operator.v1.UserDetails;
  return proto.g8e.operator.v1.UserDetails.deserializeBinaryFromReader(msg, reader);
};


/**
 * Deserializes binary data (in protobuf wire format) from the
 * given reader into the given message object.
 * @param {!proto.g8e.operator.v1.UserDetails} msg The message object to deserialize into.
 * @param {!jspb.BinaryReader} reader The BinaryReader to use.
 * @return {!proto.g8e.operator.v1.UserDetails}
 */
proto.g8e.operator.v1.UserDetails.deserializeBinaryFromReader = function(msg, reader) {
  while (reader.nextField()) {
    if (reader.isEndGroup()) {
      break;
    }
    var field = reader.getFieldNumber();
    switch (field) {
    case 1:
      var value = /** @type {string} */ (reader.readString());
      msg.setUsername(value);
      break;
    case 2:
      var value = /** @type {number} */ (reader.readInt32());
      msg.setUid(value);
      break;
    case 3:
      var value = /** @type {number} */ (reader.readInt32());
      msg.setGid(value);
      break;
    case 4:
      var value = /** @type {string} */ (reader.readString());
      msg.setHome(value);
      break;
    case 5:
      var value = /** @type {string} */ (reader.readString());
      msg.setName(value);
      break;
    case 6:
      var value = /** @type {string} */ (reader.readString());
      msg.setShell(value);
      break;
    default:
      reader.skipField();
      break;
    }
  }
  return msg;
};


/**
 * Serializes the message to binary data (in protobuf wire format).
 * @return {!Uint8Array}
 */
proto.g8e.operator.v1.UserDetails.prototype.serializeBinary = function() {
  var writer = new jspb.BinaryWriter();
  proto.g8e.operator.v1.UserDetails.serializeBinaryToWriter(this, writer);
  return writer.getResultBuffer();
};


/**
 * Serializes the given message to binary data (in protobuf wire
 * format), writing to the given BinaryWriter.
 * @param {!proto.g8e.operator.v1.UserDetails} message
 * @param {!jspb.BinaryWriter} writer
 * @suppress {unusedLocalVariables} f is only used for nested messages
 */
proto.g8e.operator.v1.UserDetails.serializeBinaryToWriter = function(message, writer) {
  var f = undefined;
  f = message.getUsername();
  if (f.length > 0) {
    writer.writeString(
      1,
      f
    );
  }
  f = message.getUid();
  if (f !== 0) {
    writer.writeInt32(
      2,
      f
    );
  }
  f = message.getGid();
  if (f !== 0) {
    writer.writeInt32(
      3,
      f
    );
  }
  f = message.getHome();
  if (f.length > 0) {
    writer.writeString(
      4,
      f
    );
  }
  f = message.getName();
  if (f.length > 0) {
    writer.writeString(
      5,
      f
    );
  }
  f = message.getShell();
  if (f.length > 0) {
    writer.writeString(
      6,
      f
    );
  }
};


/**
 * optional string username = 1;
 * @return {string}
 */
proto.g8e.operator.v1.UserDetails.prototype.getUsername = function() {
  return /** @type {string} */ (jspb.Message.getFieldWithDefault(this, 1, ""));
};


/**
 * @param {string} value
 * @return {!proto.g8e.operator.v1.UserDetails} returns this
 */
proto.g8e.operator.v1.UserDetails.prototype.setUsername = function(value) {
  return jspb.Message.setProto3StringField(this, 1, value);
};


/**
 * optional int32 uid = 2;
 * @return {number}
 */
proto.g8e.operator.v1.UserDetails.prototype.getUid = function() {
  return /** @type {number} */ (jspb.Message.getFieldWithDefault(this, 2, 0));
};


/**
 * @param {number} value
 * @return {!proto.g8e.operator.v1.UserDetails} returns this
 */
proto.g8e.operator.v1.UserDetails.prototype.setUid = function(value) {
  return jspb.Message.setProto3IntField(this, 2, value);
};


/**
 * optional int32 gid = 3;
 * @return {number}
 */
proto.g8e.operator.v1.UserDetails.prototype.getGid = function() {
  return /** @type {number} */ (jspb.Message.getFieldWithDefault(this, 3, 0));
};


/**
 * @param {number} value
 * @return {!proto.g8e.operator.v1.UserDetails} returns this
 */
proto.g8e.operator.v1.UserDetails.prototype.setGid = function(value) {
  return jspb.Message.setProto3IntField(this, 3, value);
};


/**
 * optional string home = 4;
 * @return {string}
 */
proto.g8e.operator.v1.UserDetails.prototype.getHome = function() {
  return /** @type {string} */ (jspb.Message.getFieldWithDefault(this, 4, ""));
};


/**
 * @param {string} value
 * @return {!proto.g8e.operator.v1.UserDetails} returns this
 */
proto.g8e.operator.v1.UserDetails.prototype.setHome = function(value) {
  return jspb.Message.setProto3StringField(this, 4, value);
};


/**
 * optional string name = 5;
 * @return {string}
 */
proto.g8e.operator.v1.UserDetails.prototype.getName = function() {
  return /** @type {string} */ (jspb.Message.getFieldWithDefault(this, 5, ""));
};


/**
 * @param {string} value
 * @return {!proto.g8e.operator.v1.UserDetails} returns this
 */
proto.g8e.operator.v1.UserDetails.prototype.setName = function(value) {
  return jspb.Message.setProto3StringField(this, 5, value);
};


/**
 * optional string shell = 6;
 * @return {string}
 */
proto.g8e.operator.v1.UserDetails.prototype.getShell = function() {
  return /** @type {string} */ (jspb.Message.getFieldWithDefault(this, 6, ""));
};


/**
 * @param {string} value
 * @return {!proto.g8e.operator.v1.UserDetails} returns this
 */
proto.g8e.operator.v1.UserDetails.prototype.setShell = function(value) {
  return jspb.Message.setProto3StringField(this, 6, value);
};





if (jspb.Message.GENERATE_TO_OBJECT) {
/**
 * Creates an object representation of this proto.
 * Field names that are reserved in JavaScript and will be renamed to pb_name.
 * Optional fields that are not set will be set to undefined.
 * To access a reserved field use, foo.pb_<name>, eg, foo.pb_default.
 * For the list of reserved names please see:
 *     net/proto2/compiler/js/internal/generator.cc#kKeyword.
 * @param {boolean=} opt_includeInstance Deprecated. whether to include the
 *     JSPB instance for transitional soy proto support:
 *     http://goto/soy-param-migration
 * @return {!Object}
 */
proto.g8e.operator.v1.DiskDetails.prototype.toObject = function(opt_includeInstance) {
  return proto.g8e.operator.v1.DiskDetails.toObject(opt_includeInstance, this);
};


/**
 * Static version of the {@see toObject} method.
 * @param {boolean|undefined} includeInstance Deprecated. Whether to include
 *     the JSPB instance for transitional soy proto support:
 *     http://goto/soy-param-migration
 * @param {!proto.g8e.operator.v1.DiskDetails} msg The msg instance to transform.
 * @return {!Object}
 * @suppress {unusedLocalVariables} f is only used for nested messages
 */
proto.g8e.operator.v1.DiskDetails.toObject = function(includeInstance, msg) {
  var f, obj = {
    totalGb: jspb.Message.getFloatingPointFieldWithDefault(msg, 1, 0.0),
    usedGb: jspb.Message.getFloatingPointFieldWithDefault(msg, 2, 0.0),
    freeGb: jspb.Message.getFloatingPointFieldWithDefault(msg, 3, 0.0),
    percent: jspb.Message.getFloatingPointFieldWithDefault(msg, 4, 0.0)
  };

  if (includeInstance) {
    obj.$jspbMessageInstance = msg;
  }
  return obj;
};
}


/**
 * Deserializes binary data (in protobuf wire format).
 * @param {jspb.ByteSource} bytes The bytes to deserialize.
 * @return {!proto.g8e.operator.v1.DiskDetails}
 */
proto.g8e.operator.v1.DiskDetails.deserializeBinary = function(bytes) {
  var reader = new jspb.BinaryReader(bytes);
  var msg = new proto.g8e.operator.v1.DiskDetails;
  return proto.g8e.operator.v1.DiskDetails.deserializeBinaryFromReader(msg, reader);
};


/**
 * Deserializes binary data (in protobuf wire format) from the
 * given reader into the given message object.
 * @param {!proto.g8e.operator.v1.DiskDetails} msg The message object to deserialize into.
 * @param {!jspb.BinaryReader} reader The BinaryReader to use.
 * @return {!proto.g8e.operator.v1.DiskDetails}
 */
proto.g8e.operator.v1.DiskDetails.deserializeBinaryFromReader = function(msg, reader) {
  while (reader.nextField()) {
    if (reader.isEndGroup()) {
      break;
    }
    var field = reader.getFieldNumber();
    switch (field) {
    case 1:
      var value = /** @type {number} */ (reader.readDouble());
      msg.setTotalGb(value);
      break;
    case 2:
      var value = /** @type {number} */ (reader.readDouble());
      msg.setUsedGb(value);
      break;
    case 3:
      var value = /** @type {number} */ (reader.readDouble());
      msg.setFreeGb(value);
      break;
    case 4:
      var value = /** @type {number} */ (reader.readDouble());
      msg.setPercent(value);
      break;
    default:
      reader.skipField();
      break;
    }
  }
  return msg;
};


/**
 * Serializes the message to binary data (in protobuf wire format).
 * @return {!Uint8Array}
 */
proto.g8e.operator.v1.DiskDetails.prototype.serializeBinary = function() {
  var writer = new jspb.BinaryWriter();
  proto.g8e.operator.v1.DiskDetails.serializeBinaryToWriter(this, writer);
  return writer.getResultBuffer();
};


/**
 * Serializes the given message to binary data (in protobuf wire
 * format), writing to the given BinaryWriter.
 * @param {!proto.g8e.operator.v1.DiskDetails} message
 * @param {!jspb.BinaryWriter} writer
 * @suppress {unusedLocalVariables} f is only used for nested messages
 */
proto.g8e.operator.v1.DiskDetails.serializeBinaryToWriter = function(message, writer) {
  var f = undefined;
  f = message.getTotalGb();
  if (f !== 0.0) {
    writer.writeDouble(
      1,
      f
    );
  }
  f = message.getUsedGb();
  if (f !== 0.0) {
    writer.writeDouble(
      2,
      f
    );
  }
  f = message.getFreeGb();
  if (f !== 0.0) {
    writer.writeDouble(
      3,
      f
    );
  }
  f = message.getPercent();
  if (f !== 0.0) {
    writer.writeDouble(
      4,
      f
    );
  }
};


/**
 * optional double total_gb = 1;
 * @return {number}
 */
proto.g8e.operator.v1.DiskDetails.prototype.getTotalGb = function() {
  return /** @type {number} */ (jspb.Message.getFloatingPointFieldWithDefault(this, 1, 0.0));
};


/**
 * @param {number} value
 * @return {!proto.g8e.operator.v1.DiskDetails} returns this
 */
proto.g8e.operator.v1.DiskDetails.prototype.setTotalGb = function(value) {
  return jspb.Message.setProto3FloatField(this, 1, value);
};


/**
 * optional double used_gb = 2;
 * @return {number}
 */
proto.g8e.operator.v1.DiskDetails.prototype.getUsedGb = function() {
  return /** @type {number} */ (jspb.Message.getFloatingPointFieldWithDefault(this, 2, 0.0));
};


/**
 * @param {number} value
 * @return {!proto.g8e.operator.v1.DiskDetails} returns this
 */
proto.g8e.operator.v1.DiskDetails.prototype.setUsedGb = function(value) {
  return jspb.Message.setProto3FloatField(this, 2, value);
};


/**
 * optional double free_gb = 3;
 * @return {number}
 */
proto.g8e.operator.v1.DiskDetails.prototype.getFreeGb = function() {
  return /** @type {number} */ (jspb.Message.getFloatingPointFieldWithDefault(this, 3, 0.0));
};


/**
 * @param {number} value
 * @return {!proto.g8e.operator.v1.DiskDetails} returns this
 */
proto.g8e.operator.v1.DiskDetails.prototype.setFreeGb = function(value) {
  return jspb.Message.setProto3FloatField(this, 3, value);
};


/**
 * optional double percent = 4;
 * @return {number}
 */
proto.g8e.operator.v1.DiskDetails.prototype.getPercent = function() {
  return /** @type {number} */ (jspb.Message.getFloatingPointFieldWithDefault(this, 4, 0.0));
};


/**
 * @param {number} value
 * @return {!proto.g8e.operator.v1.DiskDetails} returns this
 */
proto.g8e.operator.v1.DiskDetails.prototype.setPercent = function(value) {
  return jspb.Message.setProto3FloatField(this, 4, value);
};





if (jspb.Message.GENERATE_TO_OBJECT) {
/**
 * Creates an object representation of this proto.
 * Field names that are reserved in JavaScript and will be renamed to pb_name.
 * Optional fields that are not set will be set to undefined.
 * To access a reserved field use, foo.pb_<name>, eg, foo.pb_default.
 * For the list of reserved names please see:
 *     net/proto2/compiler/js/internal/generator.cc#kKeyword.
 * @param {boolean=} opt_includeInstance Deprecated. whether to include the
 *     JSPB instance for transitional soy proto support:
 *     http://goto/soy-param-migration
 * @return {!Object}
 */
proto.g8e.operator.v1.MemoryDetails.prototype.toObject = function(opt_includeInstance) {
  return proto.g8e.operator.v1.MemoryDetails.toObject(opt_includeInstance, this);
};


/**
 * Static version of the {@see toObject} method.
 * @param {boolean|undefined} includeInstance Deprecated. Whether to include
 *     the JSPB instance for transitional soy proto support:
 *     http://goto/soy-param-migration
 * @param {!proto.g8e.operator.v1.MemoryDetails} msg The msg instance to transform.
 * @return {!Object}
 * @suppress {unusedLocalVariables} f is only used for nested messages
 */
proto.g8e.operator.v1.MemoryDetails.toObject = function(includeInstance, msg) {
  var f, obj = {
    totalMb: jspb.Message.getFieldWithDefault(msg, 1, 0),
    availableMb: jspb.Message.getFieldWithDefault(msg, 2, 0),
    usedMb: jspb.Message.getFieldWithDefault(msg, 3, 0),
    percent: jspb.Message.getFloatingPointFieldWithDefault(msg, 4, 0.0)
  };

  if (includeInstance) {
    obj.$jspbMessageInstance = msg;
  }
  return obj;
};
}


/**
 * Deserializes binary data (in protobuf wire format).
 * @param {jspb.ByteSource} bytes The bytes to deserialize.
 * @return {!proto.g8e.operator.v1.MemoryDetails}
 */
proto.g8e.operator.v1.MemoryDetails.deserializeBinary = function(bytes) {
  var reader = new jspb.BinaryReader(bytes);
  var msg = new proto.g8e.operator.v1.MemoryDetails;
  return proto.g8e.operator.v1.MemoryDetails.deserializeBinaryFromReader(msg, reader);
};


/**
 * Deserializes binary data (in protobuf wire format) from the
 * given reader into the given message object.
 * @param {!proto.g8e.operator.v1.MemoryDetails} msg The message object to deserialize into.
 * @param {!jspb.BinaryReader} reader The BinaryReader to use.
 * @return {!proto.g8e.operator.v1.MemoryDetails}
 */
proto.g8e.operator.v1.MemoryDetails.deserializeBinaryFromReader = function(msg, reader) {
  while (reader.nextField()) {
    if (reader.isEndGroup()) {
      break;
    }
    var field = reader.getFieldNumber();
    switch (field) {
    case 1:
      var value = /** @type {number} */ (reader.readInt64());
      msg.setTotalMb(value);
      break;
    case 2:
      var value = /** @type {number} */ (reader.readInt64());
      msg.setAvailableMb(value);
      break;
    case 3:
      var value = /** @type {number} */ (reader.readInt64());
      msg.setUsedMb(value);
      break;
    case 4:
      var value = /** @type {number} */ (reader.readDouble());
      msg.setPercent(value);
      break;
    default:
      reader.skipField();
      break;
    }
  }
  return msg;
};


/**
 * Serializes the message to binary data (in protobuf wire format).
 * @return {!Uint8Array}
 */
proto.g8e.operator.v1.MemoryDetails.prototype.serializeBinary = function() {
  var writer = new jspb.BinaryWriter();
  proto.g8e.operator.v1.MemoryDetails.serializeBinaryToWriter(this, writer);
  return writer.getResultBuffer();
};


/**
 * Serializes the given message to binary data (in protobuf wire
 * format), writing to the given BinaryWriter.
 * @param {!proto.g8e.operator.v1.MemoryDetails} message
 * @param {!jspb.BinaryWriter} writer
 * @suppress {unusedLocalVariables} f is only used for nested messages
 */
proto.g8e.operator.v1.MemoryDetails.serializeBinaryToWriter = function(message, writer) {
  var f = undefined;
  f = message.getTotalMb();
  if (f !== 0) {
    writer.writeInt64(
      1,
      f
    );
  }
  f = message.getAvailableMb();
  if (f !== 0) {
    writer.writeInt64(
      2,
      f
    );
  }
  f = message.getUsedMb();
  if (f !== 0) {
    writer.writeInt64(
      3,
      f
    );
  }
  f = message.getPercent();
  if (f !== 0.0) {
    writer.writeDouble(
      4,
      f
    );
  }
};


/**
 * optional int64 total_mb = 1;
 * @return {number}
 */
proto.g8e.operator.v1.MemoryDetails.prototype.getTotalMb = function() {
  return /** @type {number} */ (jspb.Message.getFieldWithDefault(this, 1, 0));
};


/**
 * @param {number} value
 * @return {!proto.g8e.operator.v1.MemoryDetails} returns this
 */
proto.g8e.operator.v1.MemoryDetails.prototype.setTotalMb = function(value) {
  return jspb.Message.setProto3IntField(this, 1, value);
};


/**
 * optional int64 available_mb = 2;
 * @return {number}
 */
proto.g8e.operator.v1.MemoryDetails.prototype.getAvailableMb = function() {
  return /** @type {number} */ (jspb.Message.getFieldWithDefault(this, 2, 0));
};


/**
 * @param {number} value
 * @return {!proto.g8e.operator.v1.MemoryDetails} returns this
 */
proto.g8e.operator.v1.MemoryDetails.prototype.setAvailableMb = function(value) {
  return jspb.Message.setProto3IntField(this, 2, value);
};


/**
 * optional int64 used_mb = 3;
 * @return {number}
 */
proto.g8e.operator.v1.MemoryDetails.prototype.getUsedMb = function() {
  return /** @type {number} */ (jspb.Message.getFieldWithDefault(this, 3, 0));
};


/**
 * @param {number} value
 * @return {!proto.g8e.operator.v1.MemoryDetails} returns this
 */
proto.g8e.operator.v1.MemoryDetails.prototype.setUsedMb = function(value) {
  return jspb.Message.setProto3IntField(this, 3, value);
};


/**
 * optional double percent = 4;
 * @return {number}
 */
proto.g8e.operator.v1.MemoryDetails.prototype.getPercent = function() {
  return /** @type {number} */ (jspb.Message.getFloatingPointFieldWithDefault(this, 4, 0.0));
};


/**
 * @param {number} value
 * @return {!proto.g8e.operator.v1.MemoryDetails} returns this
 */
proto.g8e.operator.v1.MemoryDetails.prototype.setPercent = function(value) {
  return jspb.Message.setProto3FloatField(this, 4, value);
};



/**
 * List of repeated fields within this message type.
 * @private {!Array<number>}
 * @const
 */
proto.g8e.operator.v1.EnvironmentDetails.repeatedFields_ = [7];



if (jspb.Message.GENERATE_TO_OBJECT) {
/**
 * Creates an object representation of this proto.
 * Field names that are reserved in JavaScript and will be renamed to pb_name.
 * Optional fields that are not set will be set to undefined.
 * To access a reserved field use, foo.pb_<name>, eg, foo.pb_default.
 * For the list of reserved names please see:
 *     net/proto2/compiler/js/internal/generator.cc#kKeyword.
 * @param {boolean=} opt_includeInstance Deprecated. whether to include the
 *     JSPB instance for transitional soy proto support:
 *     http://goto/soy-param-migration
 * @return {!Object}
 */
proto.g8e.operator.v1.EnvironmentDetails.prototype.toObject = function(opt_includeInstance) {
  return proto.g8e.operator.v1.EnvironmentDetails.toObject(opt_includeInstance, this);
};


/**
 * Static version of the {@see toObject} method.
 * @param {boolean|undefined} includeInstance Deprecated. Whether to include
 *     the JSPB instance for transitional soy proto support:
 *     http://goto/soy-param-migration
 * @param {!proto.g8e.operator.v1.EnvironmentDetails} msg The msg instance to transform.
 * @return {!Object}
 * @suppress {unusedLocalVariables} f is only used for nested messages
 */
proto.g8e.operator.v1.EnvironmentDetails.toObject = function(includeInstance, msg) {
  var f, obj = {
    pwd: jspb.Message.getFieldWithDefault(msg, 1, ""),
    lang: jspb.Message.getFieldWithDefault(msg, 2, ""),
    timezone: jspb.Message.getFieldWithDefault(msg, 3, ""),
    term: jspb.Message.getFieldWithDefault(msg, 4, ""),
    isContainer: jspb.Message.getBooleanFieldWithDefault(msg, 5, false),
    containerRuntime: jspb.Message.getFieldWithDefault(msg, 6, ""),
    containerSignalsList: (f = jspb.Message.getRepeatedField(msg, 7)) == null ? undefined : f,
    initSystem: jspb.Message.getFieldWithDefault(msg, 8, "")
  };

  if (includeInstance) {
    obj.$jspbMessageInstance = msg;
  }
  return obj;
};
}


/**
 * Deserializes binary data (in protobuf wire format).
 * @param {jspb.ByteSource} bytes The bytes to deserialize.
 * @return {!proto.g8e.operator.v1.EnvironmentDetails}
 */
proto.g8e.operator.v1.EnvironmentDetails.deserializeBinary = function(bytes) {
  var reader = new jspb.BinaryReader(bytes);
  var msg = new proto.g8e.operator.v1.EnvironmentDetails;
  return proto.g8e.operator.v1.EnvironmentDetails.deserializeBinaryFromReader(msg, reader);
};


/**
 * Deserializes binary data (in protobuf wire format) from the
 * given reader into the given message object.
 * @param {!proto.g8e.operator.v1.EnvironmentDetails} msg The message object to deserialize into.
 * @param {!jspb.BinaryReader} reader The BinaryReader to use.
 * @return {!proto.g8e.operator.v1.EnvironmentDetails}
 */
proto.g8e.operator.v1.EnvironmentDetails.deserializeBinaryFromReader = function(msg, reader) {
  while (reader.nextField()) {
    if (reader.isEndGroup()) {
      break;
    }
    var field = reader.getFieldNumber();
    switch (field) {
    case 1:
      var value = /** @type {string} */ (reader.readString());
      msg.setPwd(value);
      break;
    case 2:
      var value = /** @type {string} */ (reader.readString());
      msg.setLang(value);
      break;
    case 3:
      var value = /** @type {string} */ (reader.readString());
      msg.setTimezone(value);
      break;
    case 4:
      var value = /** @type {string} */ (reader.readString());
      msg.setTerm(value);
      break;
    case 5:
      var value = /** @type {boolean} */ (reader.readBool());
      msg.setIsContainer(value);
      break;
    case 6:
      var value = /** @type {string} */ (reader.readString());
      msg.setContainerRuntime(value);
      break;
    case 7:
      var value = /** @type {string} */ (reader.readString());
      msg.addContainerSignals(value);
      break;
    case 8:
      var value = /** @type {string} */ (reader.readString());
      msg.setInitSystem(value);
      break;
    default:
      reader.skipField();
      break;
    }
  }
  return msg;
};


/**
 * Serializes the message to binary data (in protobuf wire format).
 * @return {!Uint8Array}
 */
proto.g8e.operator.v1.EnvironmentDetails.prototype.serializeBinary = function() {
  var writer = new jspb.BinaryWriter();
  proto.g8e.operator.v1.EnvironmentDetails.serializeBinaryToWriter(this, writer);
  return writer.getResultBuffer();
};


/**
 * Serializes the given message to binary data (in protobuf wire
 * format), writing to the given BinaryWriter.
 * @param {!proto.g8e.operator.v1.EnvironmentDetails} message
 * @param {!jspb.BinaryWriter} writer
 * @suppress {unusedLocalVariables} f is only used for nested messages
 */
proto.g8e.operator.v1.EnvironmentDetails.serializeBinaryToWriter = function(message, writer) {
  var f = undefined;
  f = message.getPwd();
  if (f.length > 0) {
    writer.writeString(
      1,
      f
    );
  }
  f = message.getLang();
  if (f.length > 0) {
    writer.writeString(
      2,
      f
    );
  }
  f = message.getTimezone();
  if (f.length > 0) {
    writer.writeString(
      3,
      f
    );
  }
  f = message.getTerm();
  if (f.length > 0) {
    writer.writeString(
      4,
      f
    );
  }
  f = message.getIsContainer();
  if (f) {
    writer.writeBool(
      5,
      f
    );
  }
  f = message.getContainerRuntime();
  if (f.length > 0) {
    writer.writeString(
      6,
      f
    );
  }
  f = message.getContainerSignalsList();
  if (f.length > 0) {
    writer.writeRepeatedString(
      7,
      f
    );
  }
  f = message.getInitSystem();
  if (f.length > 0) {
    writer.writeString(
      8,
      f
    );
  }
};


/**
 * optional string pwd = 1;
 * @return {string}
 */
proto.g8e.operator.v1.EnvironmentDetails.prototype.getPwd = function() {
  return /** @type {string} */ (jspb.Message.getFieldWithDefault(this, 1, ""));
};


/**
 * @param {string} value
 * @return {!proto.g8e.operator.v1.EnvironmentDetails} returns this
 */
proto.g8e.operator.v1.EnvironmentDetails.prototype.setPwd = function(value) {
  return jspb.Message.setProto3StringField(this, 1, value);
};


/**
 * optional string lang = 2;
 * @return {string}
 */
proto.g8e.operator.v1.EnvironmentDetails.prototype.getLang = function() {
  return /** @type {string} */ (jspb.Message.getFieldWithDefault(this, 2, ""));
};


/**
 * @param {string} value
 * @return {!proto.g8e.operator.v1.EnvironmentDetails} returns this
 */
proto.g8e.operator.v1.EnvironmentDetails.prototype.setLang = function(value) {
  return jspb.Message.setProto3StringField(this, 2, value);
};


/**
 * optional string timezone = 3;
 * @return {string}
 */
proto.g8e.operator.v1.EnvironmentDetails.prototype.getTimezone = function() {
  return /** @type {string} */ (jspb.Message.getFieldWithDefault(this, 3, ""));
};


/**
 * @param {string} value
 * @return {!proto.g8e.operator.v1.EnvironmentDetails} returns this
 */
proto.g8e.operator.v1.EnvironmentDetails.prototype.setTimezone = function(value) {
  return jspb.Message.setProto3StringField(this, 3, value);
};


/**
 * optional string term = 4;
 * @return {string}
 */
proto.g8e.operator.v1.EnvironmentDetails.prototype.getTerm = function() {
  return /** @type {string} */ (jspb.Message.getFieldWithDefault(this, 4, ""));
};


/**
 * @param {string} value
 * @return {!proto.g8e.operator.v1.EnvironmentDetails} returns this
 */
proto.g8e.operator.v1.EnvironmentDetails.prototype.setTerm = function(value) {
  return jspb.Message.setProto3StringField(this, 4, value);
};


/**
 * optional bool is_container = 5;
 * @return {boolean}
 */
proto.g8e.operator.v1.EnvironmentDetails.prototype.getIsContainer = function() {
  return /** @type {boolean} */ (jspb.Message.getBooleanFieldWithDefault(this, 5, false));
};


/**
 * @param {boolean} value
 * @return {!proto.g8e.operator.v1.EnvironmentDetails} returns this
 */
proto.g8e.operator.v1.EnvironmentDetails.prototype.setIsContainer = function(value) {
  return jspb.Message.setProto3BooleanField(this, 5, value);
};


/**
 * optional string container_runtime = 6;
 * @return {string}
 */
proto.g8e.operator.v1.EnvironmentDetails.prototype.getContainerRuntime = function() {
  return /** @type {string} */ (jspb.Message.getFieldWithDefault(this, 6, ""));
};


/**
 * @param {string} value
 * @return {!proto.g8e.operator.v1.EnvironmentDetails} returns this
 */
proto.g8e.operator.v1.EnvironmentDetails.prototype.setContainerRuntime = function(value) {
  return jspb.Message.setProto3StringField(this, 6, value);
};


/**
 * repeated string container_signals = 7;
 * @return {!Array<string>}
 */
proto.g8e.operator.v1.EnvironmentDetails.prototype.getContainerSignalsList = function() {
  return /** @type {!Array<string>} */ (jspb.Message.getRepeatedField(this, 7));
};


/**
 * @param {!Array<string>} value
 * @return {!proto.g8e.operator.v1.EnvironmentDetails} returns this
 */
proto.g8e.operator.v1.EnvironmentDetails.prototype.setContainerSignalsList = function(value) {
  return jspb.Message.setField(this, 7, value || []);
};


/**
 * @param {string} value
 * @param {number=} opt_index
 * @return {!proto.g8e.operator.v1.EnvironmentDetails} returns this
 */
proto.g8e.operator.v1.EnvironmentDetails.prototype.addContainerSignals = function(value, opt_index) {
  return jspb.Message.addToRepeatedField(this, 7, value, opt_index);
};


/**
 * Clears the list making it empty but non-null.
 * @return {!proto.g8e.operator.v1.EnvironmentDetails} returns this
 */
proto.g8e.operator.v1.EnvironmentDetails.prototype.clearContainerSignalsList = function() {
  return this.setContainerSignalsList([]);
};


/**
 * optional string init_system = 8;
 * @return {string}
 */
proto.g8e.operator.v1.EnvironmentDetails.prototype.getInitSystem = function() {
  return /** @type {string} */ (jspb.Message.getFieldWithDefault(this, 8, ""));
};


/**
 * @param {string} value
 * @return {!proto.g8e.operator.v1.EnvironmentDetails} returns this
 */
proto.g8e.operator.v1.EnvironmentDetails.prototype.setInitSystem = function(value) {
  return jspb.Message.setProto3StringField(this, 8, value);
};





if (jspb.Message.GENERATE_TO_OBJECT) {
/**
 * Creates an object representation of this proto.
 * Field names that are reserved in JavaScript and will be renamed to pb_name.
 * Optional fields that are not set will be set to undefined.
 * To access a reserved field use, foo.pb_<name>, eg, foo.pb_default.
 * For the list of reserved names please see:
 *     net/proto2/compiler/js/internal/generator.cc#kKeyword.
 * @param {boolean=} opt_includeInstance Deprecated. whether to include the
 *     JSPB instance for transitional soy proto support:
 *     http://goto/soy-param-migration
 * @return {!Object}
 */
proto.g8e.operator.v1.FingerprintDetails.prototype.toObject = function(opt_includeInstance) {
  return proto.g8e.operator.v1.FingerprintDetails.toObject(opt_includeInstance, this);
};


/**
 * Static version of the {@see toObject} method.
 * @param {boolean|undefined} includeInstance Deprecated. Whether to include
 *     the JSPB instance for transitional soy proto support:
 *     http://goto/soy-param-migration
 * @param {!proto.g8e.operator.v1.FingerprintDetails} msg The msg instance to transform.
 * @return {!Object}
 * @suppress {unusedLocalVariables} f is only used for nested messages
 */
proto.g8e.operator.v1.FingerprintDetails.toObject = function(includeInstance, msg) {
  var f, obj = {
    os: jspb.Message.getFieldWithDefault(msg, 1, ""),
    architecture: jspb.Message.getFieldWithDefault(msg, 2, ""),
    cpuCount: jspb.Message.getFieldWithDefault(msg, 3, 0),
    machineId: jspb.Message.getFieldWithDefault(msg, 4, "")
  };

  if (includeInstance) {
    obj.$jspbMessageInstance = msg;
  }
  return obj;
};
}


/**
 * Deserializes binary data (in protobuf wire format).
 * @param {jspb.ByteSource} bytes The bytes to deserialize.
 * @return {!proto.g8e.operator.v1.FingerprintDetails}
 */
proto.g8e.operator.v1.FingerprintDetails.deserializeBinary = function(bytes) {
  var reader = new jspb.BinaryReader(bytes);
  var msg = new proto.g8e.operator.v1.FingerprintDetails;
  return proto.g8e.operator.v1.FingerprintDetails.deserializeBinaryFromReader(msg, reader);
};


/**
 * Deserializes binary data (in protobuf wire format) from the
 * given reader into the given message object.
 * @param {!proto.g8e.operator.v1.FingerprintDetails} msg The message object to deserialize into.
 * @param {!jspb.BinaryReader} reader The BinaryReader to use.
 * @return {!proto.g8e.operator.v1.FingerprintDetails}
 */
proto.g8e.operator.v1.FingerprintDetails.deserializeBinaryFromReader = function(msg, reader) {
  while (reader.nextField()) {
    if (reader.isEndGroup()) {
      break;
    }
    var field = reader.getFieldNumber();
    switch (field) {
    case 1:
      var value = /** @type {string} */ (reader.readString());
      msg.setOs(value);
      break;
    case 2:
      var value = /** @type {string} */ (reader.readString());
      msg.setArchitecture(value);
      break;
    case 3:
      var value = /** @type {number} */ (reader.readInt32());
      msg.setCpuCount(value);
      break;
    case 4:
      var value = /** @type {string} */ (reader.readString());
      msg.setMachineId(value);
      break;
    default:
      reader.skipField();
      break;
    }
  }
  return msg;
};


/**
 * Serializes the message to binary data (in protobuf wire format).
 * @return {!Uint8Array}
 */
proto.g8e.operator.v1.FingerprintDetails.prototype.serializeBinary = function() {
  var writer = new jspb.BinaryWriter();
  proto.g8e.operator.v1.FingerprintDetails.serializeBinaryToWriter(this, writer);
  return writer.getResultBuffer();
};


/**
 * Serializes the given message to binary data (in protobuf wire
 * format), writing to the given BinaryWriter.
 * @param {!proto.g8e.operator.v1.FingerprintDetails} message
 * @param {!jspb.BinaryWriter} writer
 * @suppress {unusedLocalVariables} f is only used for nested messages
 */
proto.g8e.operator.v1.FingerprintDetails.serializeBinaryToWriter = function(message, writer) {
  var f = undefined;
  f = message.getOs();
  if (f.length > 0) {
    writer.writeString(
      1,
      f
    );
  }
  f = message.getArchitecture();
  if (f.length > 0) {
    writer.writeString(
      2,
      f
    );
  }
  f = message.getCpuCount();
  if (f !== 0) {
    writer.writeInt32(
      3,
      f
    );
  }
  f = message.getMachineId();
  if (f.length > 0) {
    writer.writeString(
      4,
      f
    );
  }
};


/**
 * optional string os = 1;
 * @return {string}
 */
proto.g8e.operator.v1.FingerprintDetails.prototype.getOs = function() {
  return /** @type {string} */ (jspb.Message.getFieldWithDefault(this, 1, ""));
};


/**
 * @param {string} value
 * @return {!proto.g8e.operator.v1.FingerprintDetails} returns this
 */
proto.g8e.operator.v1.FingerprintDetails.prototype.setOs = function(value) {
  return jspb.Message.setProto3StringField(this, 1, value);
};


/**
 * optional string architecture = 2;
 * @return {string}
 */
proto.g8e.operator.v1.FingerprintDetails.prototype.getArchitecture = function() {
  return /** @type {string} */ (jspb.Message.getFieldWithDefault(this, 2, ""));
};


/**
 * @param {string} value
 * @return {!proto.g8e.operator.v1.FingerprintDetails} returns this
 */
proto.g8e.operator.v1.FingerprintDetails.prototype.setArchitecture = function(value) {
  return jspb.Message.setProto3StringField(this, 2, value);
};


/**
 * optional int32 cpu_count = 3;
 * @return {number}
 */
proto.g8e.operator.v1.FingerprintDetails.prototype.getCpuCount = function() {
  return /** @type {number} */ (jspb.Message.getFieldWithDefault(this, 3, 0));
};


/**
 * @param {number} value
 * @return {!proto.g8e.operator.v1.FingerprintDetails} returns this
 */
proto.g8e.operator.v1.FingerprintDetails.prototype.setCpuCount = function(value) {
  return jspb.Message.setProto3IntField(this, 3, value);
};


/**
 * optional string machine_id = 4;
 * @return {string}
 */
proto.g8e.operator.v1.FingerprintDetails.prototype.getMachineId = function() {
  return /** @type {string} */ (jspb.Message.getFieldWithDefault(this, 4, ""));
};


/**
 * @param {string} value
 * @return {!proto.g8e.operator.v1.FingerprintDetails} returns this
 */
proto.g8e.operator.v1.FingerprintDetails.prototype.setMachineId = function(value) {
  return jspb.Message.setProto3StringField(this, 4, value);
};



/**
 * List of repeated fields within this message type.
 * @private {!Array<number>}
 * @const
 */
proto.g8e.operator.v1.PasskeyCredential.repeatedFields_ = [4];



if (jspb.Message.GENERATE_TO_OBJECT) {
/**
 * Creates an object representation of this proto.
 * Field names that are reserved in JavaScript and will be renamed to pb_name.
 * Optional fields that are not set will be set to undefined.
 * To access a reserved field use, foo.pb_<name>, eg, foo.pb_default.
 * For the list of reserved names please see:
 *     net/proto2/compiler/js/internal/generator.cc#kKeyword.
 * @param {boolean=} opt_includeInstance Deprecated. whether to include the
 *     JSPB instance for transitional soy proto support:
 *     http://goto/soy-param-migration
 * @return {!Object}
 */
proto.g8e.operator.v1.PasskeyCredential.prototype.toObject = function(opt_includeInstance) {
  return proto.g8e.operator.v1.PasskeyCredential.toObject(opt_includeInstance, this);
};


/**
 * Static version of the {@see toObject} method.
 * @param {boolean|undefined} includeInstance Deprecated. Whether to include
 *     the JSPB instance for transitional soy proto support:
 *     http://goto/soy-param-migration
 * @param {!proto.g8e.operator.v1.PasskeyCredential} msg The msg instance to transform.
 * @return {!Object}
 * @suppress {unusedLocalVariables} f is only used for nested messages
 */
proto.g8e.operator.v1.PasskeyCredential.toObject = function(includeInstance, msg) {
  var f, obj = {
    id: jspb.Message.getFieldWithDefault(msg, 1, ""),
    publicKey: jspb.Message.getFieldWithDefault(msg, 2, ""),
    counter: jspb.Message.getFieldWithDefault(msg, 3, 0),
    transportsList: (f = jspb.Message.getRepeatedField(msg, 4)) == null ? undefined : f,
    createdAtUnixMs: jspb.Message.getFieldWithDefault(msg, 5, 0),
    lastUsedAtUnixMs: jspb.Message.getFieldWithDefault(msg, 6, 0)
  };

  if (includeInstance) {
    obj.$jspbMessageInstance = msg;
  }
  return obj;
};
}


/**
 * Deserializes binary data (in protobuf wire format).
 * @param {jspb.ByteSource} bytes The bytes to deserialize.
 * @return {!proto.g8e.operator.v1.PasskeyCredential}
 */
proto.g8e.operator.v1.PasskeyCredential.deserializeBinary = function(bytes) {
  var reader = new jspb.BinaryReader(bytes);
  var msg = new proto.g8e.operator.v1.PasskeyCredential;
  return proto.g8e.operator.v1.PasskeyCredential.deserializeBinaryFromReader(msg, reader);
};


/**
 * Deserializes binary data (in protobuf wire format) from the
 * given reader into the given message object.
 * @param {!proto.g8e.operator.v1.PasskeyCredential} msg The message object to deserialize into.
 * @param {!jspb.BinaryReader} reader The BinaryReader to use.
 * @return {!proto.g8e.operator.v1.PasskeyCredential}
 */
proto.g8e.operator.v1.PasskeyCredential.deserializeBinaryFromReader = function(msg, reader) {
  while (reader.nextField()) {
    if (reader.isEndGroup()) {
      break;
    }
    var field = reader.getFieldNumber();
    switch (field) {
    case 1:
      var value = /** @type {string} */ (reader.readString());
      msg.setId(value);
      break;
    case 2:
      var value = /** @type {string} */ (reader.readString());
      msg.setPublicKey(value);
      break;
    case 3:
      var value = /** @type {number} */ (reader.readInt64());
      msg.setCounter(value);
      break;
    case 4:
      var value = /** @type {string} */ (reader.readString());
      msg.addTransports(value);
      break;
    case 5:
      var value = /** @type {number} */ (reader.readInt64());
      msg.setCreatedAtUnixMs(value);
      break;
    case 6:
      var value = /** @type {number} */ (reader.readInt64());
      msg.setLastUsedAtUnixMs(value);
      break;
    default:
      reader.skipField();
      break;
    }
  }
  return msg;
};


/**
 * Serializes the message to binary data (in protobuf wire format).
 * @return {!Uint8Array}
 */
proto.g8e.operator.v1.PasskeyCredential.prototype.serializeBinary = function() {
  var writer = new jspb.BinaryWriter();
  proto.g8e.operator.v1.PasskeyCredential.serializeBinaryToWriter(this, writer);
  return writer.getResultBuffer();
};


/**
 * Serializes the given message to binary data (in protobuf wire
 * format), writing to the given BinaryWriter.
 * @param {!proto.g8e.operator.v1.PasskeyCredential} message
 * @param {!jspb.BinaryWriter} writer
 * @suppress {unusedLocalVariables} f is only used for nested messages
 */
proto.g8e.operator.v1.PasskeyCredential.serializeBinaryToWriter = function(message, writer) {
  var f = undefined;
  f = message.getId();
  if (f.length > 0) {
    writer.writeString(
      1,
      f
    );
  }
  f = message.getPublicKey();
  if (f.length > 0) {
    writer.writeString(
      2,
      f
    );
  }
  f = message.getCounter();
  if (f !== 0) {
    writer.writeInt64(
      3,
      f
    );
  }
  f = message.getTransportsList();
  if (f.length > 0) {
    writer.writeRepeatedString(
      4,
      f
    );
  }
  f = message.getCreatedAtUnixMs();
  if (f !== 0) {
    writer.writeInt64(
      5,
      f
    );
  }
  f = message.getLastUsedAtUnixMs();
  if (f !== 0) {
    writer.writeInt64(
      6,
      f
    );
  }
};


/**
 * optional string id = 1;
 * @return {string}
 */
proto.g8e.operator.v1.PasskeyCredential.prototype.getId = function() {
  return /** @type {string} */ (jspb.Message.getFieldWithDefault(this, 1, ""));
};


/**
 * @param {string} value
 * @return {!proto.g8e.operator.v1.PasskeyCredential} returns this
 */
proto.g8e.operator.v1.PasskeyCredential.prototype.setId = function(value) {
  return jspb.Message.setProto3StringField(this, 1, value);
};


/**
 * optional string public_key = 2;
 * @return {string}
 */
proto.g8e.operator.v1.PasskeyCredential.prototype.getPublicKey = function() {
  return /** @type {string} */ (jspb.Message.getFieldWithDefault(this, 2, ""));
};


/**
 * @param {string} value
 * @return {!proto.g8e.operator.v1.PasskeyCredential} returns this
 */
proto.g8e.operator.v1.PasskeyCredential.prototype.setPublicKey = function(value) {
  return jspb.Message.setProto3StringField(this, 2, value);
};


/**
 * optional int64 counter = 3;
 * @return {number}
 */
proto.g8e.operator.v1.PasskeyCredential.prototype.getCounter = function() {
  return /** @type {number} */ (jspb.Message.getFieldWithDefault(this, 3, 0));
};


/**
 * @param {number} value
 * @return {!proto.g8e.operator.v1.PasskeyCredential} returns this
 */
proto.g8e.operator.v1.PasskeyCredential.prototype.setCounter = function(value) {
  return jspb.Message.setProto3IntField(this, 3, value);
};


/**
 * repeated string transports = 4;
 * @return {!Array<string>}
 */
proto.g8e.operator.v1.PasskeyCredential.prototype.getTransportsList = function() {
  return /** @type {!Array<string>} */ (jspb.Message.getRepeatedField(this, 4));
};


/**
 * @param {!Array<string>} value
 * @return {!proto.g8e.operator.v1.PasskeyCredential} returns this
 */
proto.g8e.operator.v1.PasskeyCredential.prototype.setTransportsList = function(value) {
  return jspb.Message.setField(this, 4, value || []);
};


/**
 * @param {string} value
 * @param {number=} opt_index
 * @return {!proto.g8e.operator.v1.PasskeyCredential} returns this
 */
proto.g8e.operator.v1.PasskeyCredential.prototype.addTransports = function(value, opt_index) {
  return jspb.Message.addToRepeatedField(this, 4, value, opt_index);
};


/**
 * Clears the list making it empty but non-null.
 * @return {!proto.g8e.operator.v1.PasskeyCredential} returns this
 */
proto.g8e.operator.v1.PasskeyCredential.prototype.clearTransportsList = function() {
  return this.setTransportsList([]);
};


/**
 * optional int64 created_at_unix_ms = 5;
 * @return {number}
 */
proto.g8e.operator.v1.PasskeyCredential.prototype.getCreatedAtUnixMs = function() {
  return /** @type {number} */ (jspb.Message.getFieldWithDefault(this, 5, 0));
};


/**
 * @param {number} value
 * @return {!proto.g8e.operator.v1.PasskeyCredential} returns this
 */
proto.g8e.operator.v1.PasskeyCredential.prototype.setCreatedAtUnixMs = function(value) {
  return jspb.Message.setProto3IntField(this, 5, value);
};


/**
 * optional int64 last_used_at_unix_ms = 6;
 * @return {number}
 */
proto.g8e.operator.v1.PasskeyCredential.prototype.getLastUsedAtUnixMs = function() {
  return /** @type {number} */ (jspb.Message.getFieldWithDefault(this, 6, 0));
};


/**
 * @param {number} value
 * @return {!proto.g8e.operator.v1.PasskeyCredential} returns this
 */
proto.g8e.operator.v1.PasskeyCredential.prototype.setLastUsedAtUnixMs = function(value) {
  return jspb.Message.setProto3IntField(this, 6, value);
};





if (jspb.Message.GENERATE_TO_OBJECT) {
/**
 * Creates an object representation of this proto.
 * Field names that are reserved in JavaScript and will be renamed to pb_name.
 * Optional fields that are not set will be set to undefined.
 * To access a reserved field use, foo.pb_<name>, eg, foo.pb_default.
 * For the list of reserved names please see:
 *     net/proto2/compiler/js/internal/generator.cc#kKeyword.
 * @param {boolean=} opt_includeInstance Deprecated. whether to include the
 *     JSPB instance for transitional soy proto support:
 *     http://goto/soy-param-migration
 * @return {!Object}
 */
proto.g8e.operator.v1.PasskeyRegisterChallengeRequested.prototype.toObject = function(opt_includeInstance) {
  return proto.g8e.operator.v1.PasskeyRegisterChallengeRequested.toObject(opt_includeInstance, this);
};


/**
 * Static version of the {@see toObject} method.
 * @param {boolean|undefined} includeInstance Deprecated. Whether to include
 *     the JSPB instance for transitional soy proto support:
 *     http://goto/soy-param-migration
 * @param {!proto.g8e.operator.v1.PasskeyRegisterChallengeRequested} msg The msg instance to transform.
 * @return {!Object}
 * @suppress {unusedLocalVariables} f is only used for nested messages
 */
proto.g8e.operator.v1.PasskeyRegisterChallengeRequested.toObject = function(includeInstance, msg) {
  var f, obj = {
    userId: jspb.Message.getFieldWithDefault(msg, 1, ""),
    email: jspb.Message.getFieldWithDefault(msg, 2, ""),
    userName: jspb.Message.getFieldWithDefault(msg, 3, "")
  };

  if (includeInstance) {
    obj.$jspbMessageInstance = msg;
  }
  return obj;
};
}


/**
 * Deserializes binary data (in protobuf wire format).
 * @param {jspb.ByteSource} bytes The bytes to deserialize.
 * @return {!proto.g8e.operator.v1.PasskeyRegisterChallengeRequested}
 */
proto.g8e.operator.v1.PasskeyRegisterChallengeRequested.deserializeBinary = function(bytes) {
  var reader = new jspb.BinaryReader(bytes);
  var msg = new proto.g8e.operator.v1.PasskeyRegisterChallengeRequested;
  return proto.g8e.operator.v1.PasskeyRegisterChallengeRequested.deserializeBinaryFromReader(msg, reader);
};


/**
 * Deserializes binary data (in protobuf wire format) from the
 * given reader into the given message object.
 * @param {!proto.g8e.operator.v1.PasskeyRegisterChallengeRequested} msg The message object to deserialize into.
 * @param {!jspb.BinaryReader} reader The BinaryReader to use.
 * @return {!proto.g8e.operator.v1.PasskeyRegisterChallengeRequested}
 */
proto.g8e.operator.v1.PasskeyRegisterChallengeRequested.deserializeBinaryFromReader = function(msg, reader) {
  while (reader.nextField()) {
    if (reader.isEndGroup()) {
      break;
    }
    var field = reader.getFieldNumber();
    switch (field) {
    case 1:
      var value = /** @type {string} */ (reader.readString());
      msg.setUserId(value);
      break;
    case 2:
      var value = /** @type {string} */ (reader.readString());
      msg.setEmail(value);
      break;
    case 3:
      var value = /** @type {string} */ (reader.readString());
      msg.setUserName(value);
      break;
    default:
      reader.skipField();
      break;
    }
  }
  return msg;
};


/**
 * Serializes the message to binary data (in protobuf wire format).
 * @return {!Uint8Array}
 */
proto.g8e.operator.v1.PasskeyRegisterChallengeRequested.prototype.serializeBinary = function() {
  var writer = new jspb.BinaryWriter();
  proto.g8e.operator.v1.PasskeyRegisterChallengeRequested.serializeBinaryToWriter(this, writer);
  return writer.getResultBuffer();
};


/**
 * Serializes the given message to binary data (in protobuf wire
 * format), writing to the given BinaryWriter.
 * @param {!proto.g8e.operator.v1.PasskeyRegisterChallengeRequested} message
 * @param {!jspb.BinaryWriter} writer
 * @suppress {unusedLocalVariables} f is only used for nested messages
 */
proto.g8e.operator.v1.PasskeyRegisterChallengeRequested.serializeBinaryToWriter = function(message, writer) {
  var f = undefined;
  f = message.getUserId();
  if (f.length > 0) {
    writer.writeString(
      1,
      f
    );
  }
  f = message.getEmail();
  if (f.length > 0) {
    writer.writeString(
      2,
      f
    );
  }
  f = message.getUserName();
  if (f.length > 0) {
    writer.writeString(
      3,
      f
    );
  }
};


/**
 * optional string user_id = 1;
 * @return {string}
 */
proto.g8e.operator.v1.PasskeyRegisterChallengeRequested.prototype.getUserId = function() {
  return /** @type {string} */ (jspb.Message.getFieldWithDefault(this, 1, ""));
};


/**
 * @param {string} value
 * @return {!proto.g8e.operator.v1.PasskeyRegisterChallengeRequested} returns this
 */
proto.g8e.operator.v1.PasskeyRegisterChallengeRequested.prototype.setUserId = function(value) {
  return jspb.Message.setProto3StringField(this, 1, value);
};


/**
 * optional string email = 2;
 * @return {string}
 */
proto.g8e.operator.v1.PasskeyRegisterChallengeRequested.prototype.getEmail = function() {
  return /** @type {string} */ (jspb.Message.getFieldWithDefault(this, 2, ""));
};


/**
 * @param {string} value
 * @return {!proto.g8e.operator.v1.PasskeyRegisterChallengeRequested} returns this
 */
proto.g8e.operator.v1.PasskeyRegisterChallengeRequested.prototype.setEmail = function(value) {
  return jspb.Message.setProto3StringField(this, 2, value);
};


/**
 * optional string user_name = 3;
 * @return {string}
 */
proto.g8e.operator.v1.PasskeyRegisterChallengeRequested.prototype.getUserName = function() {
  return /** @type {string} */ (jspb.Message.getFieldWithDefault(this, 3, ""));
};


/**
 * @param {string} value
 * @return {!proto.g8e.operator.v1.PasskeyRegisterChallengeRequested} returns this
 */
proto.g8e.operator.v1.PasskeyRegisterChallengeRequested.prototype.setUserName = function(value) {
  return jspb.Message.setProto3StringField(this, 3, value);
};



/**
 * List of repeated fields within this message type.
 * @private {!Array<number>}
 * @const
 */
proto.g8e.operator.v1.PasskeyRegisterChallengeResult.repeatedFields_ = [6,10];



if (jspb.Message.GENERATE_TO_OBJECT) {
/**
 * Creates an object representation of this proto.
 * Field names that are reserved in JavaScript and will be renamed to pb_name.
 * Optional fields that are not set will be set to undefined.
 * To access a reserved field use, foo.pb_<name>, eg, foo.pb_default.
 * For the list of reserved names please see:
 *     net/proto2/compiler/js/internal/generator.cc#kKeyword.
 * @param {boolean=} opt_includeInstance Deprecated. whether to include the
 *     JSPB instance for transitional soy proto support:
 *     http://goto/soy-param-migration
 * @return {!Object}
 */
proto.g8e.operator.v1.PasskeyRegisterChallengeResult.prototype.toObject = function(opt_includeInstance) {
  return proto.g8e.operator.v1.PasskeyRegisterChallengeResult.toObject(opt_includeInstance, this);
};


/**
 * Static version of the {@see toObject} method.
 * @param {boolean|undefined} includeInstance Deprecated. Whether to include
 *     the JSPB instance for transitional soy proto support:
 *     http://goto/soy-param-migration
 * @param {!proto.g8e.operator.v1.PasskeyRegisterChallengeResult} msg The msg instance to transform.
 * @return {!Object}
 * @suppress {unusedLocalVariables} f is only used for nested messages
 */
proto.g8e.operator.v1.PasskeyRegisterChallengeResult.toObject = function(includeInstance, msg) {
  var f, obj = {
    success: jspb.Message.getBooleanFieldWithDefault(msg, 1, false),
    error: jspb.Message.getFieldWithDefault(msg, 2, ""),
    challenge: jspb.Message.getFieldWithDefault(msg, 3, ""),
    rp: (f = msg.getRp()) && proto.g8e.operator.v1.PasskeyRegisterChallengeResult.RelyingParty.toObject(includeInstance, f),
    user: (f = msg.getUser()) && proto.g8e.operator.v1.PasskeyRegisterChallengeResult.UserInfo.toObject(includeInstance, f),
    pubKeyCredParamsList: jspb.Message.toObjectList(msg.getPubKeyCredParamsList(),
    proto.g8e.operator.v1.PasskeyRegisterChallengeResult.PublicKeyCredentialParameters.toObject, includeInstance),
    timeout: jspb.Message.getFieldWithDefault(msg, 7, 0),
    attestation: jspb.Message.getFieldWithDefault(msg, 8, ""),
    authenticatorSelection: (f = msg.getAuthenticatorSelection()) && proto.g8e.operator.v1.PasskeyRegisterChallengeResult.AuthenticatorSelection.toObject(includeInstance, f),
    excludeCredentialsList: (f = jspb.Message.getRepeatedField(msg, 10)) == null ? undefined : f
  };

  if (includeInstance) {
    obj.$jspbMessageInstance = msg;
  }
  return obj;
};
}


/**
 * Deserializes binary data (in protobuf wire format).
 * @param {jspb.ByteSource} bytes The bytes to deserialize.
 * @return {!proto.g8e.operator.v1.PasskeyRegisterChallengeResult}
 */
proto.g8e.operator.v1.PasskeyRegisterChallengeResult.deserializeBinary = function(bytes) {
  var reader = new jspb.BinaryReader(bytes);
  var msg = new proto.g8e.operator.v1.PasskeyRegisterChallengeResult;
  return proto.g8e.operator.v1.PasskeyRegisterChallengeResult.deserializeBinaryFromReader(msg, reader);
};


/**
 * Deserializes binary data (in protobuf wire format) from the
 * given reader into the given message object.
 * @param {!proto.g8e.operator.v1.PasskeyRegisterChallengeResult} msg The message object to deserialize into.
 * @param {!jspb.BinaryReader} reader The BinaryReader to use.
 * @return {!proto.g8e.operator.v1.PasskeyRegisterChallengeResult}
 */
proto.g8e.operator.v1.PasskeyRegisterChallengeResult.deserializeBinaryFromReader = function(msg, reader) {
  while (reader.nextField()) {
    if (reader.isEndGroup()) {
      break;
    }
    var field = reader.getFieldNumber();
    switch (field) {
    case 1:
      var value = /** @type {boolean} */ (reader.readBool());
      msg.setSuccess(value);
      break;
    case 2:
      var value = /** @type {string} */ (reader.readString());
      msg.setError(value);
      break;
    case 3:
      var value = /** @type {string} */ (reader.readString());
      msg.setChallenge(value);
      break;
    case 4:
      var value = new proto.g8e.operator.v1.PasskeyRegisterChallengeResult.RelyingParty;
      reader.readMessage(value,proto.g8e.operator.v1.PasskeyRegisterChallengeResult.RelyingParty.deserializeBinaryFromReader);
      msg.setRp(value);
      break;
    case 5:
      var value = new proto.g8e.operator.v1.PasskeyRegisterChallengeResult.UserInfo;
      reader.readMessage(value,proto.g8e.operator.v1.PasskeyRegisterChallengeResult.UserInfo.deserializeBinaryFromReader);
      msg.setUser(value);
      break;
    case 6:
      var value = new proto.g8e.operator.v1.PasskeyRegisterChallengeResult.PublicKeyCredentialParameters;
      reader.readMessage(value,proto.g8e.operator.v1.PasskeyRegisterChallengeResult.PublicKeyCredentialParameters.deserializeBinaryFromReader);
      msg.addPubKeyCredParams(value);
      break;
    case 7:
      var value = /** @type {number} */ (reader.readInt64());
      msg.setTimeout(value);
      break;
    case 8:
      var value = /** @type {string} */ (reader.readString());
      msg.setAttestation(value);
      break;
    case 9:
      var value = new proto.g8e.operator.v1.PasskeyRegisterChallengeResult.AuthenticatorSelection;
      reader.readMessage(value,proto.g8e.operator.v1.PasskeyRegisterChallengeResult.AuthenticatorSelection.deserializeBinaryFromReader);
      msg.setAuthenticatorSelection(value);
      break;
    case 10:
      var value = /** @type {string} */ (reader.readString());
      msg.addExcludeCredentials(value);
      break;
    default:
      reader.skipField();
      break;
    }
  }
  return msg;
};


/**
 * Serializes the message to binary data (in protobuf wire format).
 * @return {!Uint8Array}
 */
proto.g8e.operator.v1.PasskeyRegisterChallengeResult.prototype.serializeBinary = function() {
  var writer = new jspb.BinaryWriter();
  proto.g8e.operator.v1.PasskeyRegisterChallengeResult.serializeBinaryToWriter(this, writer);
  return writer.getResultBuffer();
};


/**
 * Serializes the given message to binary data (in protobuf wire
 * format), writing to the given BinaryWriter.
 * @param {!proto.g8e.operator.v1.PasskeyRegisterChallengeResult} message
 * @param {!jspb.BinaryWriter} writer
 * @suppress {unusedLocalVariables} f is only used for nested messages
 */
proto.g8e.operator.v1.PasskeyRegisterChallengeResult.serializeBinaryToWriter = function(message, writer) {
  var f = undefined;
  f = message.getSuccess();
  if (f) {
    writer.writeBool(
      1,
      f
    );
  }
  f = message.getError();
  if (f.length > 0) {
    writer.writeString(
      2,
      f
    );
  }
  f = message.getChallenge();
  if (f.length > 0) {
    writer.writeString(
      3,
      f
    );
  }
  f = message.getRp();
  if (f != null) {
    writer.writeMessage(
      4,
      f,
      proto.g8e.operator.v1.PasskeyRegisterChallengeResult.RelyingParty.serializeBinaryToWriter
    );
  }
  f = message.getUser();
  if (f != null) {
    writer.writeMessage(
      5,
      f,
      proto.g8e.operator.v1.PasskeyRegisterChallengeResult.UserInfo.serializeBinaryToWriter
    );
  }
  f = message.getPubKeyCredParamsList();
  if (f.length > 0) {
    writer.writeRepeatedMessage(
      6,
      f,
      proto.g8e.operator.v1.PasskeyRegisterChallengeResult.PublicKeyCredentialParameters.serializeBinaryToWriter
    );
  }
  f = message.getTimeout();
  if (f !== 0) {
    writer.writeInt64(
      7,
      f
    );
  }
  f = message.getAttestation();
  if (f.length > 0) {
    writer.writeString(
      8,
      f
    );
  }
  f = message.getAuthenticatorSelection();
  if (f != null) {
    writer.writeMessage(
      9,
      f,
      proto.g8e.operator.v1.PasskeyRegisterChallengeResult.AuthenticatorSelection.serializeBinaryToWriter
    );
  }
  f = message.getExcludeCredentialsList();
  if (f.length > 0) {
    writer.writeRepeatedString(
      10,
      f
    );
  }
};





if (jspb.Message.GENERATE_TO_OBJECT) {
/**
 * Creates an object representation of this proto.
 * Field names that are reserved in JavaScript and will be renamed to pb_name.
 * Optional fields that are not set will be set to undefined.
 * To access a reserved field use, foo.pb_<name>, eg, foo.pb_default.
 * For the list of reserved names please see:
 *     net/proto2/compiler/js/internal/generator.cc#kKeyword.
 * @param {boolean=} opt_includeInstance Deprecated. whether to include the
 *     JSPB instance for transitional soy proto support:
 *     http://goto/soy-param-migration
 * @return {!Object}
 */
proto.g8e.operator.v1.PasskeyRegisterChallengeResult.RelyingParty.prototype.toObject = function(opt_includeInstance) {
  return proto.g8e.operator.v1.PasskeyRegisterChallengeResult.RelyingParty.toObject(opt_includeInstance, this);
};


/**
 * Static version of the {@see toObject} method.
 * @param {boolean|undefined} includeInstance Deprecated. Whether to include
 *     the JSPB instance for transitional soy proto support:
 *     http://goto/soy-param-migration
 * @param {!proto.g8e.operator.v1.PasskeyRegisterChallengeResult.RelyingParty} msg The msg instance to transform.
 * @return {!Object}
 * @suppress {unusedLocalVariables} f is only used for nested messages
 */
proto.g8e.operator.v1.PasskeyRegisterChallengeResult.RelyingParty.toObject = function(includeInstance, msg) {
  var f, obj = {
    name: jspb.Message.getFieldWithDefault(msg, 1, ""),
    id: jspb.Message.getFieldWithDefault(msg, 2, "")
  };

  if (includeInstance) {
    obj.$jspbMessageInstance = msg;
  }
  return obj;
};
}


/**
 * Deserializes binary data (in protobuf wire format).
 * @param {jspb.ByteSource} bytes The bytes to deserialize.
 * @return {!proto.g8e.operator.v1.PasskeyRegisterChallengeResult.RelyingParty}
 */
proto.g8e.operator.v1.PasskeyRegisterChallengeResult.RelyingParty.deserializeBinary = function(bytes) {
  var reader = new jspb.BinaryReader(bytes);
  var msg = new proto.g8e.operator.v1.PasskeyRegisterChallengeResult.RelyingParty;
  return proto.g8e.operator.v1.PasskeyRegisterChallengeResult.RelyingParty.deserializeBinaryFromReader(msg, reader);
};


/**
 * Deserializes binary data (in protobuf wire format) from the
 * given reader into the given message object.
 * @param {!proto.g8e.operator.v1.PasskeyRegisterChallengeResult.RelyingParty} msg The message object to deserialize into.
 * @param {!jspb.BinaryReader} reader The BinaryReader to use.
 * @return {!proto.g8e.operator.v1.PasskeyRegisterChallengeResult.RelyingParty}
 */
proto.g8e.operator.v1.PasskeyRegisterChallengeResult.RelyingParty.deserializeBinaryFromReader = function(msg, reader) {
  while (reader.nextField()) {
    if (reader.isEndGroup()) {
      break;
    }
    var field = reader.getFieldNumber();
    switch (field) {
    case 1:
      var value = /** @type {string} */ (reader.readString());
      msg.setName(value);
      break;
    case 2:
      var value = /** @type {string} */ (reader.readString());
      msg.setId(value);
      break;
    default:
      reader.skipField();
      break;
    }
  }
  return msg;
};


/**
 * Serializes the message to binary data (in protobuf wire format).
 * @return {!Uint8Array}
 */
proto.g8e.operator.v1.PasskeyRegisterChallengeResult.RelyingParty.prototype.serializeBinary = function() {
  var writer = new jspb.BinaryWriter();
  proto.g8e.operator.v1.PasskeyRegisterChallengeResult.RelyingParty.serializeBinaryToWriter(this, writer);
  return writer.getResultBuffer();
};


/**
 * Serializes the given message to binary data (in protobuf wire
 * format), writing to the given BinaryWriter.
 * @param {!proto.g8e.operator.v1.PasskeyRegisterChallengeResult.RelyingParty} message
 * @param {!jspb.BinaryWriter} writer
 * @suppress {unusedLocalVariables} f is only used for nested messages
 */
proto.g8e.operator.v1.PasskeyRegisterChallengeResult.RelyingParty.serializeBinaryToWriter = function(message, writer) {
  var f = undefined;
  f = message.getName();
  if (f.length > 0) {
    writer.writeString(
      1,
      f
    );
  }
  f = message.getId();
  if (f.length > 0) {
    writer.writeString(
      2,
      f
    );
  }
};


/**
 * optional string name = 1;
 * @return {string}
 */
proto.g8e.operator.v1.PasskeyRegisterChallengeResult.RelyingParty.prototype.getName = function() {
  return /** @type {string} */ (jspb.Message.getFieldWithDefault(this, 1, ""));
};


/**
 * @param {string} value
 * @return {!proto.g8e.operator.v1.PasskeyRegisterChallengeResult.RelyingParty} returns this
 */
proto.g8e.operator.v1.PasskeyRegisterChallengeResult.RelyingParty.prototype.setName = function(value) {
  return jspb.Message.setProto3StringField(this, 1, value);
};


/**
 * optional string id = 2;
 * @return {string}
 */
proto.g8e.operator.v1.PasskeyRegisterChallengeResult.RelyingParty.prototype.getId = function() {
  return /** @type {string} */ (jspb.Message.getFieldWithDefault(this, 2, ""));
};


/**
 * @param {string} value
 * @return {!proto.g8e.operator.v1.PasskeyRegisterChallengeResult.RelyingParty} returns this
 */
proto.g8e.operator.v1.PasskeyRegisterChallengeResult.RelyingParty.prototype.setId = function(value) {
  return jspb.Message.setProto3StringField(this, 2, value);
};





if (jspb.Message.GENERATE_TO_OBJECT) {
/**
 * Creates an object representation of this proto.
 * Field names that are reserved in JavaScript and will be renamed to pb_name.
 * Optional fields that are not set will be set to undefined.
 * To access a reserved field use, foo.pb_<name>, eg, foo.pb_default.
 * For the list of reserved names please see:
 *     net/proto2/compiler/js/internal/generator.cc#kKeyword.
 * @param {boolean=} opt_includeInstance Deprecated. whether to include the
 *     JSPB instance for transitional soy proto support:
 *     http://goto/soy-param-migration
 * @return {!Object}
 */
proto.g8e.operator.v1.PasskeyRegisterChallengeResult.UserInfo.prototype.toObject = function(opt_includeInstance) {
  return proto.g8e.operator.v1.PasskeyRegisterChallengeResult.UserInfo.toObject(opt_includeInstance, this);
};


/**
 * Static version of the {@see toObject} method.
 * @param {boolean|undefined} includeInstance Deprecated. Whether to include
 *     the JSPB instance for transitional soy proto support:
 *     http://goto/soy-param-migration
 * @param {!proto.g8e.operator.v1.PasskeyRegisterChallengeResult.UserInfo} msg The msg instance to transform.
 * @return {!Object}
 * @suppress {unusedLocalVariables} f is only used for nested messages
 */
proto.g8e.operator.v1.PasskeyRegisterChallengeResult.UserInfo.toObject = function(includeInstance, msg) {
  var f, obj = {
    id: jspb.Message.getFieldWithDefault(msg, 1, ""),
    name: jspb.Message.getFieldWithDefault(msg, 2, ""),
    displayName: jspb.Message.getFieldWithDefault(msg, 3, "")
  };

  if (includeInstance) {
    obj.$jspbMessageInstance = msg;
  }
  return obj;
};
}


/**
 * Deserializes binary data (in protobuf wire format).
 * @param {jspb.ByteSource} bytes The bytes to deserialize.
 * @return {!proto.g8e.operator.v1.PasskeyRegisterChallengeResult.UserInfo}
 */
proto.g8e.operator.v1.PasskeyRegisterChallengeResult.UserInfo.deserializeBinary = function(bytes) {
  var reader = new jspb.BinaryReader(bytes);
  var msg = new proto.g8e.operator.v1.PasskeyRegisterChallengeResult.UserInfo;
  return proto.g8e.operator.v1.PasskeyRegisterChallengeResult.UserInfo.deserializeBinaryFromReader(msg, reader);
};


/**
 * Deserializes binary data (in protobuf wire format) from the
 * given reader into the given message object.
 * @param {!proto.g8e.operator.v1.PasskeyRegisterChallengeResult.UserInfo} msg The message object to deserialize into.
 * @param {!jspb.BinaryReader} reader The BinaryReader to use.
 * @return {!proto.g8e.operator.v1.PasskeyRegisterChallengeResult.UserInfo}
 */
proto.g8e.operator.v1.PasskeyRegisterChallengeResult.UserInfo.deserializeBinaryFromReader = function(msg, reader) {
  while (reader.nextField()) {
    if (reader.isEndGroup()) {
      break;
    }
    var field = reader.getFieldNumber();
    switch (field) {
    case 1:
      var value = /** @type {string} */ (reader.readString());
      msg.setId(value);
      break;
    case 2:
      var value = /** @type {string} */ (reader.readString());
      msg.setName(value);
      break;
    case 3:
      var value = /** @type {string} */ (reader.readString());
      msg.setDisplayName(value);
      break;
    default:
      reader.skipField();
      break;
    }
  }
  return msg;
};


/**
 * Serializes the message to binary data (in protobuf wire format).
 * @return {!Uint8Array}
 */
proto.g8e.operator.v1.PasskeyRegisterChallengeResult.UserInfo.prototype.serializeBinary = function() {
  var writer = new jspb.BinaryWriter();
  proto.g8e.operator.v1.PasskeyRegisterChallengeResult.UserInfo.serializeBinaryToWriter(this, writer);
  return writer.getResultBuffer();
};


/**
 * Serializes the given message to binary data (in protobuf wire
 * format), writing to the given BinaryWriter.
 * @param {!proto.g8e.operator.v1.PasskeyRegisterChallengeResult.UserInfo} message
 * @param {!jspb.BinaryWriter} writer
 * @suppress {unusedLocalVariables} f is only used for nested messages
 */
proto.g8e.operator.v1.PasskeyRegisterChallengeResult.UserInfo.serializeBinaryToWriter = function(message, writer) {
  var f = undefined;
  f = message.getId();
  if (f.length > 0) {
    writer.writeString(
      1,
      f
    );
  }
  f = message.getName();
  if (f.length > 0) {
    writer.writeString(
      2,
      f
    );
  }
  f = message.getDisplayName();
  if (f.length > 0) {
    writer.writeString(
      3,
      f
    );
  }
};


/**
 * optional string id = 1;
 * @return {string}
 */
proto.g8e.operator.v1.PasskeyRegisterChallengeResult.UserInfo.prototype.getId = function() {
  return /** @type {string} */ (jspb.Message.getFieldWithDefault(this, 1, ""));
};


/**
 * @param {string} value
 * @return {!proto.g8e.operator.v1.PasskeyRegisterChallengeResult.UserInfo} returns this
 */
proto.g8e.operator.v1.PasskeyRegisterChallengeResult.UserInfo.prototype.setId = function(value) {
  return jspb.Message.setProto3StringField(this, 1, value);
};


/**
 * optional string name = 2;
 * @return {string}
 */
proto.g8e.operator.v1.PasskeyRegisterChallengeResult.UserInfo.prototype.getName = function() {
  return /** @type {string} */ (jspb.Message.getFieldWithDefault(this, 2, ""));
};


/**
 * @param {string} value
 * @return {!proto.g8e.operator.v1.PasskeyRegisterChallengeResult.UserInfo} returns this
 */
proto.g8e.operator.v1.PasskeyRegisterChallengeResult.UserInfo.prototype.setName = function(value) {
  return jspb.Message.setProto3StringField(this, 2, value);
};


/**
 * optional string display_name = 3;
 * @return {string}
 */
proto.g8e.operator.v1.PasskeyRegisterChallengeResult.UserInfo.prototype.getDisplayName = function() {
  return /** @type {string} */ (jspb.Message.getFieldWithDefault(this, 3, ""));
};


/**
 * @param {string} value
 * @return {!proto.g8e.operator.v1.PasskeyRegisterChallengeResult.UserInfo} returns this
 */
proto.g8e.operator.v1.PasskeyRegisterChallengeResult.UserInfo.prototype.setDisplayName = function(value) {
  return jspb.Message.setProto3StringField(this, 3, value);
};





if (jspb.Message.GENERATE_TO_OBJECT) {
/**
 * Creates an object representation of this proto.
 * Field names that are reserved in JavaScript and will be renamed to pb_name.
 * Optional fields that are not set will be set to undefined.
 * To access a reserved field use, foo.pb_<name>, eg, foo.pb_default.
 * For the list of reserved names please see:
 *     net/proto2/compiler/js/internal/generator.cc#kKeyword.
 * @param {boolean=} opt_includeInstance Deprecated. whether to include the
 *     JSPB instance for transitional soy proto support:
 *     http://goto/soy-param-migration
 * @return {!Object}
 */
proto.g8e.operator.v1.PasskeyRegisterChallengeResult.PublicKeyCredentialParameters.prototype.toObject = function(opt_includeInstance) {
  return proto.g8e.operator.v1.PasskeyRegisterChallengeResult.PublicKeyCredentialParameters.toObject(opt_includeInstance, this);
};


/**
 * Static version of the {@see toObject} method.
 * @param {boolean|undefined} includeInstance Deprecated. Whether to include
 *     the JSPB instance for transitional soy proto support:
 *     http://goto/soy-param-migration
 * @param {!proto.g8e.operator.v1.PasskeyRegisterChallengeResult.PublicKeyCredentialParameters} msg The msg instance to transform.
 * @return {!Object}
 * @suppress {unusedLocalVariables} f is only used for nested messages
 */
proto.g8e.operator.v1.PasskeyRegisterChallengeResult.PublicKeyCredentialParameters.toObject = function(includeInstance, msg) {
  var f, obj = {
    type: jspb.Message.getFieldWithDefault(msg, 1, ""),
    alg: jspb.Message.getFieldWithDefault(msg, 2, 0)
  };

  if (includeInstance) {
    obj.$jspbMessageInstance = msg;
  }
  return obj;
};
}


/**
 * Deserializes binary data (in protobuf wire format).
 * @param {jspb.ByteSource} bytes The bytes to deserialize.
 * @return {!proto.g8e.operator.v1.PasskeyRegisterChallengeResult.PublicKeyCredentialParameters}
 */
proto.g8e.operator.v1.PasskeyRegisterChallengeResult.PublicKeyCredentialParameters.deserializeBinary = function(bytes) {
  var reader = new jspb.BinaryReader(bytes);
  var msg = new proto.g8e.operator.v1.PasskeyRegisterChallengeResult.PublicKeyCredentialParameters;
  return proto.g8e.operator.v1.PasskeyRegisterChallengeResult.PublicKeyCredentialParameters.deserializeBinaryFromReader(msg, reader);
};


/**
 * Deserializes binary data (in protobuf wire format) from the
 * given reader into the given message object.
 * @param {!proto.g8e.operator.v1.PasskeyRegisterChallengeResult.PublicKeyCredentialParameters} msg The message object to deserialize into.
 * @param {!jspb.BinaryReader} reader The BinaryReader to use.
 * @return {!proto.g8e.operator.v1.PasskeyRegisterChallengeResult.PublicKeyCredentialParameters}
 */
proto.g8e.operator.v1.PasskeyRegisterChallengeResult.PublicKeyCredentialParameters.deserializeBinaryFromReader = function(msg, reader) {
  while (reader.nextField()) {
    if (reader.isEndGroup()) {
      break;
    }
    var field = reader.getFieldNumber();
    switch (field) {
    case 1:
      var value = /** @type {string} */ (reader.readString());
      msg.setType(value);
      break;
    case 2:
      var value = /** @type {number} */ (reader.readInt32());
      msg.setAlg(value);
      break;
    default:
      reader.skipField();
      break;
    }
  }
  return msg;
};


/**
 * Serializes the message to binary data (in protobuf wire format).
 * @return {!Uint8Array}
 */
proto.g8e.operator.v1.PasskeyRegisterChallengeResult.PublicKeyCredentialParameters.prototype.serializeBinary = function() {
  var writer = new jspb.BinaryWriter();
  proto.g8e.operator.v1.PasskeyRegisterChallengeResult.PublicKeyCredentialParameters.serializeBinaryToWriter(this, writer);
  return writer.getResultBuffer();
};


/**
 * Serializes the given message to binary data (in protobuf wire
 * format), writing to the given BinaryWriter.
 * @param {!proto.g8e.operator.v1.PasskeyRegisterChallengeResult.PublicKeyCredentialParameters} message
 * @param {!jspb.BinaryWriter} writer
 * @suppress {unusedLocalVariables} f is only used for nested messages
 */
proto.g8e.operator.v1.PasskeyRegisterChallengeResult.PublicKeyCredentialParameters.serializeBinaryToWriter = function(message, writer) {
  var f = undefined;
  f = message.getType();
  if (f.length > 0) {
    writer.writeString(
      1,
      f
    );
  }
  f = message.getAlg();
  if (f !== 0) {
    writer.writeInt32(
      2,
      f
    );
  }
};


/**
 * optional string type = 1;
 * @return {string}
 */
proto.g8e.operator.v1.PasskeyRegisterChallengeResult.PublicKeyCredentialParameters.prototype.getType = function() {
  return /** @type {string} */ (jspb.Message.getFieldWithDefault(this, 1, ""));
};


/**
 * @param {string} value
 * @return {!proto.g8e.operator.v1.PasskeyRegisterChallengeResult.PublicKeyCredentialParameters} returns this
 */
proto.g8e.operator.v1.PasskeyRegisterChallengeResult.PublicKeyCredentialParameters.prototype.setType = function(value) {
  return jspb.Message.setProto3StringField(this, 1, value);
};


/**
 * optional int32 alg = 2;
 * @return {number}
 */
proto.g8e.operator.v1.PasskeyRegisterChallengeResult.PublicKeyCredentialParameters.prototype.getAlg = function() {
  return /** @type {number} */ (jspb.Message.getFieldWithDefault(this, 2, 0));
};


/**
 * @param {number} value
 * @return {!proto.g8e.operator.v1.PasskeyRegisterChallengeResult.PublicKeyCredentialParameters} returns this
 */
proto.g8e.operator.v1.PasskeyRegisterChallengeResult.PublicKeyCredentialParameters.prototype.setAlg = function(value) {
  return jspb.Message.setProto3IntField(this, 2, value);
};





if (jspb.Message.GENERATE_TO_OBJECT) {
/**
 * Creates an object representation of this proto.
 * Field names that are reserved in JavaScript and will be renamed to pb_name.
 * Optional fields that are not set will be set to undefined.
 * To access a reserved field use, foo.pb_<name>, eg, foo.pb_default.
 * For the list of reserved names please see:
 *     net/proto2/compiler/js/internal/generator.cc#kKeyword.
 * @param {boolean=} opt_includeInstance Deprecated. whether to include the
 *     JSPB instance for transitional soy proto support:
 *     http://goto/soy-param-migration
 * @return {!Object}
 */
proto.g8e.operator.v1.PasskeyRegisterChallengeResult.AuthenticatorSelection.prototype.toObject = function(opt_includeInstance) {
  return proto.g8e.operator.v1.PasskeyRegisterChallengeResult.AuthenticatorSelection.toObject(opt_includeInstance, this);
};


/**
 * Static version of the {@see toObject} method.
 * @param {boolean|undefined} includeInstance Deprecated. Whether to include
 *     the JSPB instance for transitional soy proto support:
 *     http://goto/soy-param-migration
 * @param {!proto.g8e.operator.v1.PasskeyRegisterChallengeResult.AuthenticatorSelection} msg The msg instance to transform.
 * @return {!Object}
 * @suppress {unusedLocalVariables} f is only used for nested messages
 */
proto.g8e.operator.v1.PasskeyRegisterChallengeResult.AuthenticatorSelection.toObject = function(includeInstance, msg) {
  var f, obj = {
    residentKey: jspb.Message.getFieldWithDefault(msg, 1, ""),
    userVerification: jspb.Message.getFieldWithDefault(msg, 2, "")
  };

  if (includeInstance) {
    obj.$jspbMessageInstance = msg;
  }
  return obj;
};
}


/**
 * Deserializes binary data (in protobuf wire format).
 * @param {jspb.ByteSource} bytes The bytes to deserialize.
 * @return {!proto.g8e.operator.v1.PasskeyRegisterChallengeResult.AuthenticatorSelection}
 */
proto.g8e.operator.v1.PasskeyRegisterChallengeResult.AuthenticatorSelection.deserializeBinary = function(bytes) {
  var reader = new jspb.BinaryReader(bytes);
  var msg = new proto.g8e.operator.v1.PasskeyRegisterChallengeResult.AuthenticatorSelection;
  return proto.g8e.operator.v1.PasskeyRegisterChallengeResult.AuthenticatorSelection.deserializeBinaryFromReader(msg, reader);
};


/**
 * Deserializes binary data (in protobuf wire format) from the
 * given reader into the given message object.
 * @param {!proto.g8e.operator.v1.PasskeyRegisterChallengeResult.AuthenticatorSelection} msg The message object to deserialize into.
 * @param {!jspb.BinaryReader} reader The BinaryReader to use.
 * @return {!proto.g8e.operator.v1.PasskeyRegisterChallengeResult.AuthenticatorSelection}
 */
proto.g8e.operator.v1.PasskeyRegisterChallengeResult.AuthenticatorSelection.deserializeBinaryFromReader = function(msg, reader) {
  while (reader.nextField()) {
    if (reader.isEndGroup()) {
      break;
    }
    var field = reader.getFieldNumber();
    switch (field) {
    case 1:
      var value = /** @type {string} */ (reader.readString());
      msg.setResidentKey(value);
      break;
    case 2:
      var value = /** @type {string} */ (reader.readString());
      msg.setUserVerification(value);
      break;
    default:
      reader.skipField();
      break;
    }
  }
  return msg;
};


/**
 * Serializes the message to binary data (in protobuf wire format).
 * @return {!Uint8Array}
 */
proto.g8e.operator.v1.PasskeyRegisterChallengeResult.AuthenticatorSelection.prototype.serializeBinary = function() {
  var writer = new jspb.BinaryWriter();
  proto.g8e.operator.v1.PasskeyRegisterChallengeResult.AuthenticatorSelection.serializeBinaryToWriter(this, writer);
  return writer.getResultBuffer();
};


/**
 * Serializes the given message to binary data (in protobuf wire
 * format), writing to the given BinaryWriter.
 * @param {!proto.g8e.operator.v1.PasskeyRegisterChallengeResult.AuthenticatorSelection} message
 * @param {!jspb.BinaryWriter} writer
 * @suppress {unusedLocalVariables} f is only used for nested messages
 */
proto.g8e.operator.v1.PasskeyRegisterChallengeResult.AuthenticatorSelection.serializeBinaryToWriter = function(message, writer) {
  var f = undefined;
  f = message.getResidentKey();
  if (f.length > 0) {
    writer.writeString(
      1,
      f
    );
  }
  f = message.getUserVerification();
  if (f.length > 0) {
    writer.writeString(
      2,
      f
    );
  }
};


/**
 * optional string resident_key = 1;
 * @return {string}
 */
proto.g8e.operator.v1.PasskeyRegisterChallengeResult.AuthenticatorSelection.prototype.getResidentKey = function() {
  return /** @type {string} */ (jspb.Message.getFieldWithDefault(this, 1, ""));
};


/**
 * @param {string} value
 * @return {!proto.g8e.operator.v1.PasskeyRegisterChallengeResult.AuthenticatorSelection} returns this
 */
proto.g8e.operator.v1.PasskeyRegisterChallengeResult.AuthenticatorSelection.prototype.setResidentKey = function(value) {
  return jspb.Message.setProto3StringField(this, 1, value);
};


/**
 * optional string user_verification = 2;
 * @return {string}
 */
proto.g8e.operator.v1.PasskeyRegisterChallengeResult.AuthenticatorSelection.prototype.getUserVerification = function() {
  return /** @type {string} */ (jspb.Message.getFieldWithDefault(this, 2, ""));
};


/**
 * @param {string} value
 * @return {!proto.g8e.operator.v1.PasskeyRegisterChallengeResult.AuthenticatorSelection} returns this
 */
proto.g8e.operator.v1.PasskeyRegisterChallengeResult.AuthenticatorSelection.prototype.setUserVerification = function(value) {
  return jspb.Message.setProto3StringField(this, 2, value);
};


/**
 * optional bool success = 1;
 * @return {boolean}
 */
proto.g8e.operator.v1.PasskeyRegisterChallengeResult.prototype.getSuccess = function() {
  return /** @type {boolean} */ (jspb.Message.getBooleanFieldWithDefault(this, 1, false));
};


/**
 * @param {boolean} value
 * @return {!proto.g8e.operator.v1.PasskeyRegisterChallengeResult} returns this
 */
proto.g8e.operator.v1.PasskeyRegisterChallengeResult.prototype.setSuccess = function(value) {
  return jspb.Message.setProto3BooleanField(this, 1, value);
};


/**
 * optional string error = 2;
 * @return {string}
 */
proto.g8e.operator.v1.PasskeyRegisterChallengeResult.prototype.getError = function() {
  return /** @type {string} */ (jspb.Message.getFieldWithDefault(this, 2, ""));
};


/**
 * @param {string} value
 * @return {!proto.g8e.operator.v1.PasskeyRegisterChallengeResult} returns this
 */
proto.g8e.operator.v1.PasskeyRegisterChallengeResult.prototype.setError = function(value) {
  return jspb.Message.setProto3StringField(this, 2, value);
};


/**
 * optional string challenge = 3;
 * @return {string}
 */
proto.g8e.operator.v1.PasskeyRegisterChallengeResult.prototype.getChallenge = function() {
  return /** @type {string} */ (jspb.Message.getFieldWithDefault(this, 3, ""));
};


/**
 * @param {string} value
 * @return {!proto.g8e.operator.v1.PasskeyRegisterChallengeResult} returns this
 */
proto.g8e.operator.v1.PasskeyRegisterChallengeResult.prototype.setChallenge = function(value) {
  return jspb.Message.setProto3StringField(this, 3, value);
};


/**
 * optional RelyingParty rp = 4;
 * @return {?proto.g8e.operator.v1.PasskeyRegisterChallengeResult.RelyingParty}
 */
proto.g8e.operator.v1.PasskeyRegisterChallengeResult.prototype.getRp = function() {
  return /** @type{?proto.g8e.operator.v1.PasskeyRegisterChallengeResult.RelyingParty} */ (
    jspb.Message.getWrapperField(this, proto.g8e.operator.v1.PasskeyRegisterChallengeResult.RelyingParty, 4));
};


/**
 * @param {?proto.g8e.operator.v1.PasskeyRegisterChallengeResult.RelyingParty|undefined} value
 * @return {!proto.g8e.operator.v1.PasskeyRegisterChallengeResult} returns this
*/
proto.g8e.operator.v1.PasskeyRegisterChallengeResult.prototype.setRp = function(value) {
  return jspb.Message.setWrapperField(this, 4, value);
};


/**
 * Clears the message field making it undefined.
 * @return {!proto.g8e.operator.v1.PasskeyRegisterChallengeResult} returns this
 */
proto.g8e.operator.v1.PasskeyRegisterChallengeResult.prototype.clearRp = function() {
  return this.setRp(undefined);
};


/**
 * Returns whether this field is set.
 * @return {boolean}
 */
proto.g8e.operator.v1.PasskeyRegisterChallengeResult.prototype.hasRp = function() {
  return jspb.Message.getField(this, 4) != null;
};


/**
 * optional UserInfo user = 5;
 * @return {?proto.g8e.operator.v1.PasskeyRegisterChallengeResult.UserInfo}
 */
proto.g8e.operator.v1.PasskeyRegisterChallengeResult.prototype.getUser = function() {
  return /** @type{?proto.g8e.operator.v1.PasskeyRegisterChallengeResult.UserInfo} */ (
    jspb.Message.getWrapperField(this, proto.g8e.operator.v1.PasskeyRegisterChallengeResult.UserInfo, 5));
};


/**
 * @param {?proto.g8e.operator.v1.PasskeyRegisterChallengeResult.UserInfo|undefined} value
 * @return {!proto.g8e.operator.v1.PasskeyRegisterChallengeResult} returns this
*/
proto.g8e.operator.v1.PasskeyRegisterChallengeResult.prototype.setUser = function(value) {
  return jspb.Message.setWrapperField(this, 5, value);
};


/**
 * Clears the message field making it undefined.
 * @return {!proto.g8e.operator.v1.PasskeyRegisterChallengeResult} returns this
 */
proto.g8e.operator.v1.PasskeyRegisterChallengeResult.prototype.clearUser = function() {
  return this.setUser(undefined);
};


/**
 * Returns whether this field is set.
 * @return {boolean}
 */
proto.g8e.operator.v1.PasskeyRegisterChallengeResult.prototype.hasUser = function() {
  return jspb.Message.getField(this, 5) != null;
};


/**
 * repeated PublicKeyCredentialParameters pub_key_cred_params = 6;
 * @return {!Array<!proto.g8e.operator.v1.PasskeyRegisterChallengeResult.PublicKeyCredentialParameters>}
 */
proto.g8e.operator.v1.PasskeyRegisterChallengeResult.prototype.getPubKeyCredParamsList = function() {
  return /** @type{!Array<!proto.g8e.operator.v1.PasskeyRegisterChallengeResult.PublicKeyCredentialParameters>} */ (
    jspb.Message.getRepeatedWrapperField(this, proto.g8e.operator.v1.PasskeyRegisterChallengeResult.PublicKeyCredentialParameters, 6));
};


/**
 * @param {!Array<!proto.g8e.operator.v1.PasskeyRegisterChallengeResult.PublicKeyCredentialParameters>} value
 * @return {!proto.g8e.operator.v1.PasskeyRegisterChallengeResult} returns this
*/
proto.g8e.operator.v1.PasskeyRegisterChallengeResult.prototype.setPubKeyCredParamsList = function(value) {
  return jspb.Message.setRepeatedWrapperField(this, 6, value);
};


/**
 * @param {!proto.g8e.operator.v1.PasskeyRegisterChallengeResult.PublicKeyCredentialParameters=} opt_value
 * @param {number=} opt_index
 * @return {!proto.g8e.operator.v1.PasskeyRegisterChallengeResult.PublicKeyCredentialParameters}
 */
proto.g8e.operator.v1.PasskeyRegisterChallengeResult.prototype.addPubKeyCredParams = function(opt_value, opt_index) {
  return jspb.Message.addToRepeatedWrapperField(this, 6, opt_value, proto.g8e.operator.v1.PasskeyRegisterChallengeResult.PublicKeyCredentialParameters, opt_index);
};


/**
 * Clears the list making it empty but non-null.
 * @return {!proto.g8e.operator.v1.PasskeyRegisterChallengeResult} returns this
 */
proto.g8e.operator.v1.PasskeyRegisterChallengeResult.prototype.clearPubKeyCredParamsList = function() {
  return this.setPubKeyCredParamsList([]);
};


/**
 * optional int64 timeout = 7;
 * @return {number}
 */
proto.g8e.operator.v1.PasskeyRegisterChallengeResult.prototype.getTimeout = function() {
  return /** @type {number} */ (jspb.Message.getFieldWithDefault(this, 7, 0));
};


/**
 * @param {number} value
 * @return {!proto.g8e.operator.v1.PasskeyRegisterChallengeResult} returns this
 */
proto.g8e.operator.v1.PasskeyRegisterChallengeResult.prototype.setTimeout = function(value) {
  return jspb.Message.setProto3IntField(this, 7, value);
};


/**
 * optional string attestation = 8;
 * @return {string}
 */
proto.g8e.operator.v1.PasskeyRegisterChallengeResult.prototype.getAttestation = function() {
  return /** @type {string} */ (jspb.Message.getFieldWithDefault(this, 8, ""));
};


/**
 * @param {string} value
 * @return {!proto.g8e.operator.v1.PasskeyRegisterChallengeResult} returns this
 */
proto.g8e.operator.v1.PasskeyRegisterChallengeResult.prototype.setAttestation = function(value) {
  return jspb.Message.setProto3StringField(this, 8, value);
};


/**
 * optional AuthenticatorSelection authenticator_selection = 9;
 * @return {?proto.g8e.operator.v1.PasskeyRegisterChallengeResult.AuthenticatorSelection}
 */
proto.g8e.operator.v1.PasskeyRegisterChallengeResult.prototype.getAuthenticatorSelection = function() {
  return /** @type{?proto.g8e.operator.v1.PasskeyRegisterChallengeResult.AuthenticatorSelection} */ (
    jspb.Message.getWrapperField(this, proto.g8e.operator.v1.PasskeyRegisterChallengeResult.AuthenticatorSelection, 9));
};


/**
 * @param {?proto.g8e.operator.v1.PasskeyRegisterChallengeResult.AuthenticatorSelection|undefined} value
 * @return {!proto.g8e.operator.v1.PasskeyRegisterChallengeResult} returns this
*/
proto.g8e.operator.v1.PasskeyRegisterChallengeResult.prototype.setAuthenticatorSelection = function(value) {
  return jspb.Message.setWrapperField(this, 9, value);
};


/**
 * Clears the message field making it undefined.
 * @return {!proto.g8e.operator.v1.PasskeyRegisterChallengeResult} returns this
 */
proto.g8e.operator.v1.PasskeyRegisterChallengeResult.prototype.clearAuthenticatorSelection = function() {
  return this.setAuthenticatorSelection(undefined);
};


/**
 * Returns whether this field is set.
 * @return {boolean}
 */
proto.g8e.operator.v1.PasskeyRegisterChallengeResult.prototype.hasAuthenticatorSelection = function() {
  return jspb.Message.getField(this, 9) != null;
};


/**
 * repeated string exclude_credentials = 10;
 * @return {!Array<string>}
 */
proto.g8e.operator.v1.PasskeyRegisterChallengeResult.prototype.getExcludeCredentialsList = function() {
  return /** @type {!Array<string>} */ (jspb.Message.getRepeatedField(this, 10));
};


/**
 * @param {!Array<string>} value
 * @return {!proto.g8e.operator.v1.PasskeyRegisterChallengeResult} returns this
 */
proto.g8e.operator.v1.PasskeyRegisterChallengeResult.prototype.setExcludeCredentialsList = function(value) {
  return jspb.Message.setField(this, 10, value || []);
};


/**
 * @param {string} value
 * @param {number=} opt_index
 * @return {!proto.g8e.operator.v1.PasskeyRegisterChallengeResult} returns this
 */
proto.g8e.operator.v1.PasskeyRegisterChallengeResult.prototype.addExcludeCredentials = function(value, opt_index) {
  return jspb.Message.addToRepeatedField(this, 10, value, opt_index);
};


/**
 * Clears the list making it empty but non-null.
 * @return {!proto.g8e.operator.v1.PasskeyRegisterChallengeResult} returns this
 */
proto.g8e.operator.v1.PasskeyRegisterChallengeResult.prototype.clearExcludeCredentialsList = function() {
  return this.setExcludeCredentialsList([]);
};



/**
 * List of repeated fields within this message type.
 * @private {!Array<number>}
 * @const
 */
proto.g8e.operator.v1.AttestationResponse.repeatedFields_ = [5];



if (jspb.Message.GENERATE_TO_OBJECT) {
/**
 * Creates an object representation of this proto.
 * Field names that are reserved in JavaScript and will be renamed to pb_name.
 * Optional fields that are not set will be set to undefined.
 * To access a reserved field use, foo.pb_<name>, eg, foo.pb_default.
 * For the list of reserved names please see:
 *     net/proto2/compiler/js/internal/generator.cc#kKeyword.
 * @param {boolean=} opt_includeInstance Deprecated. whether to include the
 *     JSPB instance for transitional soy proto support:
 *     http://goto/soy-param-migration
 * @return {!Object}
 */
proto.g8e.operator.v1.AttestationResponse.prototype.toObject = function(opt_includeInstance) {
  return proto.g8e.operator.v1.AttestationResponse.toObject(opt_includeInstance, this);
};


/**
 * Static version of the {@see toObject} method.
 * @param {boolean|undefined} includeInstance Deprecated. Whether to include
 *     the JSPB instance for transitional soy proto support:
 *     http://goto/soy-param-migration
 * @param {!proto.g8e.operator.v1.AttestationResponse} msg The msg instance to transform.
 * @return {!Object}
 * @suppress {unusedLocalVariables} f is only used for nested messages
 */
proto.g8e.operator.v1.AttestationResponse.toObject = function(includeInstance, msg) {
  var f, obj = {
    id: jspb.Message.getFieldWithDefault(msg, 1, ""),
    rawId: jspb.Message.getFieldWithDefault(msg, 2, ""),
    clientDataJson: jspb.Message.getFieldWithDefault(msg, 3, ""),
    attestationObject: jspb.Message.getFieldWithDefault(msg, 4, ""),
    transportsList: (f = jspb.Message.getRepeatedField(msg, 5)) == null ? undefined : f
  };

  if (includeInstance) {
    obj.$jspbMessageInstance = msg;
  }
  return obj;
};
}


/**
 * Deserializes binary data (in protobuf wire format).
 * @param {jspb.ByteSource} bytes The bytes to deserialize.
 * @return {!proto.g8e.operator.v1.AttestationResponse}
 */
proto.g8e.operator.v1.AttestationResponse.deserializeBinary = function(bytes) {
  var reader = new jspb.BinaryReader(bytes);
  var msg = new proto.g8e.operator.v1.AttestationResponse;
  return proto.g8e.operator.v1.AttestationResponse.deserializeBinaryFromReader(msg, reader);
};


/**
 * Deserializes binary data (in protobuf wire format) from the
 * given reader into the given message object.
 * @param {!proto.g8e.operator.v1.AttestationResponse} msg The message object to deserialize into.
 * @param {!jspb.BinaryReader} reader The BinaryReader to use.
 * @return {!proto.g8e.operator.v1.AttestationResponse}
 */
proto.g8e.operator.v1.AttestationResponse.deserializeBinaryFromReader = function(msg, reader) {
  while (reader.nextField()) {
    if (reader.isEndGroup()) {
      break;
    }
    var field = reader.getFieldNumber();
    switch (field) {
    case 1:
      var value = /** @type {string} */ (reader.readString());
      msg.setId(value);
      break;
    case 2:
      var value = /** @type {string} */ (reader.readString());
      msg.setRawId(value);
      break;
    case 3:
      var value = /** @type {string} */ (reader.readString());
      msg.setClientDataJson(value);
      break;
    case 4:
      var value = /** @type {string} */ (reader.readString());
      msg.setAttestationObject(value);
      break;
    case 5:
      var value = /** @type {string} */ (reader.readString());
      msg.addTransports(value);
      break;
    default:
      reader.skipField();
      break;
    }
  }
  return msg;
};


/**
 * Serializes the message to binary data (in protobuf wire format).
 * @return {!Uint8Array}
 */
proto.g8e.operator.v1.AttestationResponse.prototype.serializeBinary = function() {
  var writer = new jspb.BinaryWriter();
  proto.g8e.operator.v1.AttestationResponse.serializeBinaryToWriter(this, writer);
  return writer.getResultBuffer();
};


/**
 * Serializes the given message to binary data (in protobuf wire
 * format), writing to the given BinaryWriter.
 * @param {!proto.g8e.operator.v1.AttestationResponse} message
 * @param {!jspb.BinaryWriter} writer
 * @suppress {unusedLocalVariables} f is only used for nested messages
 */
proto.g8e.operator.v1.AttestationResponse.serializeBinaryToWriter = function(message, writer) {
  var f = undefined;
  f = message.getId();
  if (f.length > 0) {
    writer.writeString(
      1,
      f
    );
  }
  f = message.getRawId();
  if (f.length > 0) {
    writer.writeString(
      2,
      f
    );
  }
  f = message.getClientDataJson();
  if (f.length > 0) {
    writer.writeString(
      3,
      f
    );
  }
  f = message.getAttestationObject();
  if (f.length > 0) {
    writer.writeString(
      4,
      f
    );
  }
  f = message.getTransportsList();
  if (f.length > 0) {
    writer.writeRepeatedString(
      5,
      f
    );
  }
};


/**
 * optional string id = 1;
 * @return {string}
 */
proto.g8e.operator.v1.AttestationResponse.prototype.getId = function() {
  return /** @type {string} */ (jspb.Message.getFieldWithDefault(this, 1, ""));
};


/**
 * @param {string} value
 * @return {!proto.g8e.operator.v1.AttestationResponse} returns this
 */
proto.g8e.operator.v1.AttestationResponse.prototype.setId = function(value) {
  return jspb.Message.setProto3StringField(this, 1, value);
};


/**
 * optional string raw_id = 2;
 * @return {string}
 */
proto.g8e.operator.v1.AttestationResponse.prototype.getRawId = function() {
  return /** @type {string} */ (jspb.Message.getFieldWithDefault(this, 2, ""));
};


/**
 * @param {string} value
 * @return {!proto.g8e.operator.v1.AttestationResponse} returns this
 */
proto.g8e.operator.v1.AttestationResponse.prototype.setRawId = function(value) {
  return jspb.Message.setProto3StringField(this, 2, value);
};


/**
 * optional string client_data_json = 3;
 * @return {string}
 */
proto.g8e.operator.v1.AttestationResponse.prototype.getClientDataJson = function() {
  return /** @type {string} */ (jspb.Message.getFieldWithDefault(this, 3, ""));
};


/**
 * @param {string} value
 * @return {!proto.g8e.operator.v1.AttestationResponse} returns this
 */
proto.g8e.operator.v1.AttestationResponse.prototype.setClientDataJson = function(value) {
  return jspb.Message.setProto3StringField(this, 3, value);
};


/**
 * optional string attestation_object = 4;
 * @return {string}
 */
proto.g8e.operator.v1.AttestationResponse.prototype.getAttestationObject = function() {
  return /** @type {string} */ (jspb.Message.getFieldWithDefault(this, 4, ""));
};


/**
 * @param {string} value
 * @return {!proto.g8e.operator.v1.AttestationResponse} returns this
 */
proto.g8e.operator.v1.AttestationResponse.prototype.setAttestationObject = function(value) {
  return jspb.Message.setProto3StringField(this, 4, value);
};


/**
 * repeated string transports = 5;
 * @return {!Array<string>}
 */
proto.g8e.operator.v1.AttestationResponse.prototype.getTransportsList = function() {
  return /** @type {!Array<string>} */ (jspb.Message.getRepeatedField(this, 5));
};


/**
 * @param {!Array<string>} value
 * @return {!proto.g8e.operator.v1.AttestationResponse} returns this
 */
proto.g8e.operator.v1.AttestationResponse.prototype.setTransportsList = function(value) {
  return jspb.Message.setField(this, 5, value || []);
};


/**
 * @param {string} value
 * @param {number=} opt_index
 * @return {!proto.g8e.operator.v1.AttestationResponse} returns this
 */
proto.g8e.operator.v1.AttestationResponse.prototype.addTransports = function(value, opt_index) {
  return jspb.Message.addToRepeatedField(this, 5, value, opt_index);
};


/**
 * Clears the list making it empty but non-null.
 * @return {!proto.g8e.operator.v1.AttestationResponse} returns this
 */
proto.g8e.operator.v1.AttestationResponse.prototype.clearTransportsList = function() {
  return this.setTransportsList([]);
};





if (jspb.Message.GENERATE_TO_OBJECT) {
/**
 * Creates an object representation of this proto.
 * Field names that are reserved in JavaScript and will be renamed to pb_name.
 * Optional fields that are not set will be set to undefined.
 * To access a reserved field use, foo.pb_<name>, eg, foo.pb_default.
 * For the list of reserved names please see:
 *     net/proto2/compiler/js/internal/generator.cc#kKeyword.
 * @param {boolean=} opt_includeInstance Deprecated. whether to include the
 *     JSPB instance for transitional soy proto support:
 *     http://goto/soy-param-migration
 * @return {!Object}
 */
proto.g8e.operator.v1.PasskeyRegisterVerifyRequested.prototype.toObject = function(opt_includeInstance) {
  return proto.g8e.operator.v1.PasskeyRegisterVerifyRequested.toObject(opt_includeInstance, this);
};


/**
 * Static version of the {@see toObject} method.
 * @param {boolean|undefined} includeInstance Deprecated. Whether to include
 *     the JSPB instance for transitional soy proto support:
 *     http://goto/soy-param-migration
 * @param {!proto.g8e.operator.v1.PasskeyRegisterVerifyRequested} msg The msg instance to transform.
 * @return {!Object}
 * @suppress {unusedLocalVariables} f is only used for nested messages
 */
proto.g8e.operator.v1.PasskeyRegisterVerifyRequested.toObject = function(includeInstance, msg) {
  var f, obj = {
    userId: jspb.Message.getFieldWithDefault(msg, 1, ""),
    attestationResponse: (f = msg.getAttestationResponse()) && proto.g8e.operator.v1.AttestationResponse.toObject(includeInstance, f)
  };

  if (includeInstance) {
    obj.$jspbMessageInstance = msg;
  }
  return obj;
};
}


/**
 * Deserializes binary data (in protobuf wire format).
 * @param {jspb.ByteSource} bytes The bytes to deserialize.
 * @return {!proto.g8e.operator.v1.PasskeyRegisterVerifyRequested}
 */
proto.g8e.operator.v1.PasskeyRegisterVerifyRequested.deserializeBinary = function(bytes) {
  var reader = new jspb.BinaryReader(bytes);
  var msg = new proto.g8e.operator.v1.PasskeyRegisterVerifyRequested;
  return proto.g8e.operator.v1.PasskeyRegisterVerifyRequested.deserializeBinaryFromReader(msg, reader);
};


/**
 * Deserializes binary data (in protobuf wire format) from the
 * given reader into the given message object.
 * @param {!proto.g8e.operator.v1.PasskeyRegisterVerifyRequested} msg The message object to deserialize into.
 * @param {!jspb.BinaryReader} reader The BinaryReader to use.
 * @return {!proto.g8e.operator.v1.PasskeyRegisterVerifyRequested}
 */
proto.g8e.operator.v1.PasskeyRegisterVerifyRequested.deserializeBinaryFromReader = function(msg, reader) {
  while (reader.nextField()) {
    if (reader.isEndGroup()) {
      break;
    }
    var field = reader.getFieldNumber();
    switch (field) {
    case 1:
      var value = /** @type {string} */ (reader.readString());
      msg.setUserId(value);
      break;
    case 2:
      var value = new proto.g8e.operator.v1.AttestationResponse;
      reader.readMessage(value,proto.g8e.operator.v1.AttestationResponse.deserializeBinaryFromReader);
      msg.setAttestationResponse(value);
      break;
    default:
      reader.skipField();
      break;
    }
  }
  return msg;
};


/**
 * Serializes the message to binary data (in protobuf wire format).
 * @return {!Uint8Array}
 */
proto.g8e.operator.v1.PasskeyRegisterVerifyRequested.prototype.serializeBinary = function() {
  var writer = new jspb.BinaryWriter();
  proto.g8e.operator.v1.PasskeyRegisterVerifyRequested.serializeBinaryToWriter(this, writer);
  return writer.getResultBuffer();
};


/**
 * Serializes the given message to binary data (in protobuf wire
 * format), writing to the given BinaryWriter.
 * @param {!proto.g8e.operator.v1.PasskeyRegisterVerifyRequested} message
 * @param {!jspb.BinaryWriter} writer
 * @suppress {unusedLocalVariables} f is only used for nested messages
 */
proto.g8e.operator.v1.PasskeyRegisterVerifyRequested.serializeBinaryToWriter = function(message, writer) {
  var f = undefined;
  f = message.getUserId();
  if (f.length > 0) {
    writer.writeString(
      1,
      f
    );
  }
  f = message.getAttestationResponse();
  if (f != null) {
    writer.writeMessage(
      2,
      f,
      proto.g8e.operator.v1.AttestationResponse.serializeBinaryToWriter
    );
  }
};


/**
 * optional string user_id = 1;
 * @return {string}
 */
proto.g8e.operator.v1.PasskeyRegisterVerifyRequested.prototype.getUserId = function() {
  return /** @type {string} */ (jspb.Message.getFieldWithDefault(this, 1, ""));
};


/**
 * @param {string} value
 * @return {!proto.g8e.operator.v1.PasskeyRegisterVerifyRequested} returns this
 */
proto.g8e.operator.v1.PasskeyRegisterVerifyRequested.prototype.setUserId = function(value) {
  return jspb.Message.setProto3StringField(this, 1, value);
};


/**
 * optional AttestationResponse attestation_response = 2;
 * @return {?proto.g8e.operator.v1.AttestationResponse}
 */
proto.g8e.operator.v1.PasskeyRegisterVerifyRequested.prototype.getAttestationResponse = function() {
  return /** @type{?proto.g8e.operator.v1.AttestationResponse} */ (
    jspb.Message.getWrapperField(this, proto.g8e.operator.v1.AttestationResponse, 2));
};


/**
 * @param {?proto.g8e.operator.v1.AttestationResponse|undefined} value
 * @return {!proto.g8e.operator.v1.PasskeyRegisterVerifyRequested} returns this
*/
proto.g8e.operator.v1.PasskeyRegisterVerifyRequested.prototype.setAttestationResponse = function(value) {
  return jspb.Message.setWrapperField(this, 2, value);
};


/**
 * Clears the message field making it undefined.
 * @return {!proto.g8e.operator.v1.PasskeyRegisterVerifyRequested} returns this
 */
proto.g8e.operator.v1.PasskeyRegisterVerifyRequested.prototype.clearAttestationResponse = function() {
  return this.setAttestationResponse(undefined);
};


/**
 * Returns whether this field is set.
 * @return {boolean}
 */
proto.g8e.operator.v1.PasskeyRegisterVerifyRequested.prototype.hasAttestationResponse = function() {
  return jspb.Message.getField(this, 2) != null;
};





if (jspb.Message.GENERATE_TO_OBJECT) {
/**
 * Creates an object representation of this proto.
 * Field names that are reserved in JavaScript and will be renamed to pb_name.
 * Optional fields that are not set will be set to undefined.
 * To access a reserved field use, foo.pb_<name>, eg, foo.pb_default.
 * For the list of reserved names please see:
 *     net/proto2/compiler/js/internal/generator.cc#kKeyword.
 * @param {boolean=} opt_includeInstance Deprecated. whether to include the
 *     JSPB instance for transitional soy proto support:
 *     http://goto/soy-param-migration
 * @return {!Object}
 */
proto.g8e.operator.v1.PasskeyRegisterVerifyResult.prototype.toObject = function(opt_includeInstance) {
  return proto.g8e.operator.v1.PasskeyRegisterVerifyResult.toObject(opt_includeInstance, this);
};


/**
 * Static version of the {@see toObject} method.
 * @param {boolean|undefined} includeInstance Deprecated. Whether to include
 *     the JSPB instance for transitional soy proto support:
 *     http://goto/soy-param-migration
 * @param {!proto.g8e.operator.v1.PasskeyRegisterVerifyResult} msg The msg instance to transform.
 * @return {!Object}
 * @suppress {unusedLocalVariables} f is only used for nested messages
 */
proto.g8e.operator.v1.PasskeyRegisterVerifyResult.toObject = function(includeInstance, msg) {
  var f, obj = {
    success: jspb.Message.getBooleanFieldWithDefault(msg, 1, false),
    error: jspb.Message.getFieldWithDefault(msg, 2, ""),
    credential: (f = msg.getCredential()) && proto.g8e.operator.v1.PasskeyCredential.toObject(includeInstance, f)
  };

  if (includeInstance) {
    obj.$jspbMessageInstance = msg;
  }
  return obj;
};
}


/**
 * Deserializes binary data (in protobuf wire format).
 * @param {jspb.ByteSource} bytes The bytes to deserialize.
 * @return {!proto.g8e.operator.v1.PasskeyRegisterVerifyResult}
 */
proto.g8e.operator.v1.PasskeyRegisterVerifyResult.deserializeBinary = function(bytes) {
  var reader = new jspb.BinaryReader(bytes);
  var msg = new proto.g8e.operator.v1.PasskeyRegisterVerifyResult;
  return proto.g8e.operator.v1.PasskeyRegisterVerifyResult.deserializeBinaryFromReader(msg, reader);
};


/**
 * Deserializes binary data (in protobuf wire format) from the
 * given reader into the given message object.
 * @param {!proto.g8e.operator.v1.PasskeyRegisterVerifyResult} msg The message object to deserialize into.
 * @param {!jspb.BinaryReader} reader The BinaryReader to use.
 * @return {!proto.g8e.operator.v1.PasskeyRegisterVerifyResult}
 */
proto.g8e.operator.v1.PasskeyRegisterVerifyResult.deserializeBinaryFromReader = function(msg, reader) {
  while (reader.nextField()) {
    if (reader.isEndGroup()) {
      break;
    }
    var field = reader.getFieldNumber();
    switch (field) {
    case 1:
      var value = /** @type {boolean} */ (reader.readBool());
      msg.setSuccess(value);
      break;
    case 2:
      var value = /** @type {string} */ (reader.readString());
      msg.setError(value);
      break;
    case 3:
      var value = new proto.g8e.operator.v1.PasskeyCredential;
      reader.readMessage(value,proto.g8e.operator.v1.PasskeyCredential.deserializeBinaryFromReader);
      msg.setCredential(value);
      break;
    default:
      reader.skipField();
      break;
    }
  }
  return msg;
};


/**
 * Serializes the message to binary data (in protobuf wire format).
 * @return {!Uint8Array}
 */
proto.g8e.operator.v1.PasskeyRegisterVerifyResult.prototype.serializeBinary = function() {
  var writer = new jspb.BinaryWriter();
  proto.g8e.operator.v1.PasskeyRegisterVerifyResult.serializeBinaryToWriter(this, writer);
  return writer.getResultBuffer();
};


/**
 * Serializes the given message to binary data (in protobuf wire
 * format), writing to the given BinaryWriter.
 * @param {!proto.g8e.operator.v1.PasskeyRegisterVerifyResult} message
 * @param {!jspb.BinaryWriter} writer
 * @suppress {unusedLocalVariables} f is only used for nested messages
 */
proto.g8e.operator.v1.PasskeyRegisterVerifyResult.serializeBinaryToWriter = function(message, writer) {
  var f = undefined;
  f = message.getSuccess();
  if (f) {
    writer.writeBool(
      1,
      f
    );
  }
  f = message.getError();
  if (f.length > 0) {
    writer.writeString(
      2,
      f
    );
  }
  f = message.getCredential();
  if (f != null) {
    writer.writeMessage(
      3,
      f,
      proto.g8e.operator.v1.PasskeyCredential.serializeBinaryToWriter
    );
  }
};


/**
 * optional bool success = 1;
 * @return {boolean}
 */
proto.g8e.operator.v1.PasskeyRegisterVerifyResult.prototype.getSuccess = function() {
  return /** @type {boolean} */ (jspb.Message.getBooleanFieldWithDefault(this, 1, false));
};


/**
 * @param {boolean} value
 * @return {!proto.g8e.operator.v1.PasskeyRegisterVerifyResult} returns this
 */
proto.g8e.operator.v1.PasskeyRegisterVerifyResult.prototype.setSuccess = function(value) {
  return jspb.Message.setProto3BooleanField(this, 1, value);
};


/**
 * optional string error = 2;
 * @return {string}
 */
proto.g8e.operator.v1.PasskeyRegisterVerifyResult.prototype.getError = function() {
  return /** @type {string} */ (jspb.Message.getFieldWithDefault(this, 2, ""));
};


/**
 * @param {string} value
 * @return {!proto.g8e.operator.v1.PasskeyRegisterVerifyResult} returns this
 */
proto.g8e.operator.v1.PasskeyRegisterVerifyResult.prototype.setError = function(value) {
  return jspb.Message.setProto3StringField(this, 2, value);
};


/**
 * optional PasskeyCredential credential = 3;
 * @return {?proto.g8e.operator.v1.PasskeyCredential}
 */
proto.g8e.operator.v1.PasskeyRegisterVerifyResult.prototype.getCredential = function() {
  return /** @type{?proto.g8e.operator.v1.PasskeyCredential} */ (
    jspb.Message.getWrapperField(this, proto.g8e.operator.v1.PasskeyCredential, 3));
};


/**
 * @param {?proto.g8e.operator.v1.PasskeyCredential|undefined} value
 * @return {!proto.g8e.operator.v1.PasskeyRegisterVerifyResult} returns this
*/
proto.g8e.operator.v1.PasskeyRegisterVerifyResult.prototype.setCredential = function(value) {
  return jspb.Message.setWrapperField(this, 3, value);
};


/**
 * Clears the message field making it undefined.
 * @return {!proto.g8e.operator.v1.PasskeyRegisterVerifyResult} returns this
 */
proto.g8e.operator.v1.PasskeyRegisterVerifyResult.prototype.clearCredential = function() {
  return this.setCredential(undefined);
};


/**
 * Returns whether this field is set.
 * @return {boolean}
 */
proto.g8e.operator.v1.PasskeyRegisterVerifyResult.prototype.hasCredential = function() {
  return jspb.Message.getField(this, 3) != null;
};





if (jspb.Message.GENERATE_TO_OBJECT) {
/**
 * Creates an object representation of this proto.
 * Field names that are reserved in JavaScript and will be renamed to pb_name.
 * Optional fields that are not set will be set to undefined.
 * To access a reserved field use, foo.pb_<name>, eg, foo.pb_default.
 * For the list of reserved names please see:
 *     net/proto2/compiler/js/internal/generator.cc#kKeyword.
 * @param {boolean=} opt_includeInstance Deprecated. whether to include the
 *     JSPB instance for transitional soy proto support:
 *     http://goto/soy-param-migration
 * @return {!Object}
 */
proto.g8e.operator.v1.PasskeyAuthChallengeRequested.prototype.toObject = function(opt_includeInstance) {
  return proto.g8e.operator.v1.PasskeyAuthChallengeRequested.toObject(opt_includeInstance, this);
};


/**
 * Static version of the {@see toObject} method.
 * @param {boolean|undefined} includeInstance Deprecated. Whether to include
 *     the JSPB instance for transitional soy proto support:
 *     http://goto/soy-param-migration
 * @param {!proto.g8e.operator.v1.PasskeyAuthChallengeRequested} msg The msg instance to transform.
 * @return {!Object}
 * @suppress {unusedLocalVariables} f is only used for nested messages
 */
proto.g8e.operator.v1.PasskeyAuthChallengeRequested.toObject = function(includeInstance, msg) {
  var f, obj = {
    email: jspb.Message.getFieldWithDefault(msg, 1, ""),
    userId: jspb.Message.getFieldWithDefault(msg, 2, "")
  };

  if (includeInstance) {
    obj.$jspbMessageInstance = msg;
  }
  return obj;
};
}


/**
 * Deserializes binary data (in protobuf wire format).
 * @param {jspb.ByteSource} bytes The bytes to deserialize.
 * @return {!proto.g8e.operator.v1.PasskeyAuthChallengeRequested}
 */
proto.g8e.operator.v1.PasskeyAuthChallengeRequested.deserializeBinary = function(bytes) {
  var reader = new jspb.BinaryReader(bytes);
  var msg = new proto.g8e.operator.v1.PasskeyAuthChallengeRequested;
  return proto.g8e.operator.v1.PasskeyAuthChallengeRequested.deserializeBinaryFromReader(msg, reader);
};


/**
 * Deserializes binary data (in protobuf wire format) from the
 * given reader into the given message object.
 * @param {!proto.g8e.operator.v1.PasskeyAuthChallengeRequested} msg The message object to deserialize into.
 * @param {!jspb.BinaryReader} reader The BinaryReader to use.
 * @return {!proto.g8e.operator.v1.PasskeyAuthChallengeRequested}
 */
proto.g8e.operator.v1.PasskeyAuthChallengeRequested.deserializeBinaryFromReader = function(msg, reader) {
  while (reader.nextField()) {
    if (reader.isEndGroup()) {
      break;
    }
    var field = reader.getFieldNumber();
    switch (field) {
    case 1:
      var value = /** @type {string} */ (reader.readString());
      msg.setEmail(value);
      break;
    case 2:
      var value = /** @type {string} */ (reader.readString());
      msg.setUserId(value);
      break;
    default:
      reader.skipField();
      break;
    }
  }
  return msg;
};


/**
 * Serializes the message to binary data (in protobuf wire format).
 * @return {!Uint8Array}
 */
proto.g8e.operator.v1.PasskeyAuthChallengeRequested.prototype.serializeBinary = function() {
  var writer = new jspb.BinaryWriter();
  proto.g8e.operator.v1.PasskeyAuthChallengeRequested.serializeBinaryToWriter(this, writer);
  return writer.getResultBuffer();
};


/**
 * Serializes the given message to binary data (in protobuf wire
 * format), writing to the given BinaryWriter.
 * @param {!proto.g8e.operator.v1.PasskeyAuthChallengeRequested} message
 * @param {!jspb.BinaryWriter} writer
 * @suppress {unusedLocalVariables} f is only used for nested messages
 */
proto.g8e.operator.v1.PasskeyAuthChallengeRequested.serializeBinaryToWriter = function(message, writer) {
  var f = undefined;
  f = message.getEmail();
  if (f.length > 0) {
    writer.writeString(
      1,
      f
    );
  }
  f = message.getUserId();
  if (f.length > 0) {
    writer.writeString(
      2,
      f
    );
  }
};


/**
 * optional string email = 1;
 * @return {string}
 */
proto.g8e.operator.v1.PasskeyAuthChallengeRequested.prototype.getEmail = function() {
  return /** @type {string} */ (jspb.Message.getFieldWithDefault(this, 1, ""));
};


/**
 * @param {string} value
 * @return {!proto.g8e.operator.v1.PasskeyAuthChallengeRequested} returns this
 */
proto.g8e.operator.v1.PasskeyAuthChallengeRequested.prototype.setEmail = function(value) {
  return jspb.Message.setProto3StringField(this, 1, value);
};


/**
 * optional string user_id = 2;
 * @return {string}
 */
proto.g8e.operator.v1.PasskeyAuthChallengeRequested.prototype.getUserId = function() {
  return /** @type {string} */ (jspb.Message.getFieldWithDefault(this, 2, ""));
};


/**
 * @param {string} value
 * @return {!proto.g8e.operator.v1.PasskeyAuthChallengeRequested} returns this
 */
proto.g8e.operator.v1.PasskeyAuthChallengeRequested.prototype.setUserId = function(value) {
  return jspb.Message.setProto3StringField(this, 2, value);
};



/**
 * List of repeated fields within this message type.
 * @private {!Array<number>}
 * @const
 */
proto.g8e.operator.v1.PasskeyAuthChallengeResult.repeatedFields_ = [7];



if (jspb.Message.GENERATE_TO_OBJECT) {
/**
 * Creates an object representation of this proto.
 * Field names that are reserved in JavaScript and will be renamed to pb_name.
 * Optional fields that are not set will be set to undefined.
 * To access a reserved field use, foo.pb_<name>, eg, foo.pb_default.
 * For the list of reserved names please see:
 *     net/proto2/compiler/js/internal/generator.cc#kKeyword.
 * @param {boolean=} opt_includeInstance Deprecated. whether to include the
 *     JSPB instance for transitional soy proto support:
 *     http://goto/soy-param-migration
 * @return {!Object}
 */
proto.g8e.operator.v1.PasskeyAuthChallengeResult.prototype.toObject = function(opt_includeInstance) {
  return proto.g8e.operator.v1.PasskeyAuthChallengeResult.toObject(opt_includeInstance, this);
};


/**
 * Static version of the {@see toObject} method.
 * @param {boolean|undefined} includeInstance Deprecated. Whether to include
 *     the JSPB instance for transitional soy proto support:
 *     http://goto/soy-param-migration
 * @param {!proto.g8e.operator.v1.PasskeyAuthChallengeResult} msg The msg instance to transform.
 * @return {!Object}
 * @suppress {unusedLocalVariables} f is only used for nested messages
 */
proto.g8e.operator.v1.PasskeyAuthChallengeResult.toObject = function(includeInstance, msg) {
  var f, obj = {
    success: jspb.Message.getBooleanFieldWithDefault(msg, 1, false),
    error: jspb.Message.getFieldWithDefault(msg, 2, ""),
    needsSetup: jspb.Message.getBooleanFieldWithDefault(msg, 3, false),
    challenge: jspb.Message.getFieldWithDefault(msg, 4, ""),
    timeout: jspb.Message.getFieldWithDefault(msg, 5, 0),
    rpId: jspb.Message.getFieldWithDefault(msg, 6, ""),
    allowCredentialsList: (f = jspb.Message.getRepeatedField(msg, 7)) == null ? undefined : f,
    userVerification: jspb.Message.getFieldWithDefault(msg, 8, "")
  };

  if (includeInstance) {
    obj.$jspbMessageInstance = msg;
  }
  return obj;
};
}


/**
 * Deserializes binary data (in protobuf wire format).
 * @param {jspb.ByteSource} bytes The bytes to deserialize.
 * @return {!proto.g8e.operator.v1.PasskeyAuthChallengeResult}
 */
proto.g8e.operator.v1.PasskeyAuthChallengeResult.deserializeBinary = function(bytes) {
  var reader = new jspb.BinaryReader(bytes);
  var msg = new proto.g8e.operator.v1.PasskeyAuthChallengeResult;
  return proto.g8e.operator.v1.PasskeyAuthChallengeResult.deserializeBinaryFromReader(msg, reader);
};


/**
 * Deserializes binary data (in protobuf wire format) from the
 * given reader into the given message object.
 * @param {!proto.g8e.operator.v1.PasskeyAuthChallengeResult} msg The message object to deserialize into.
 * @param {!jspb.BinaryReader} reader The BinaryReader to use.
 * @return {!proto.g8e.operator.v1.PasskeyAuthChallengeResult}
 */
proto.g8e.operator.v1.PasskeyAuthChallengeResult.deserializeBinaryFromReader = function(msg, reader) {
  while (reader.nextField()) {
    if (reader.isEndGroup()) {
      break;
    }
    var field = reader.getFieldNumber();
    switch (field) {
    case 1:
      var value = /** @type {boolean} */ (reader.readBool());
      msg.setSuccess(value);
      break;
    case 2:
      var value = /** @type {string} */ (reader.readString());
      msg.setError(value);
      break;
    case 3:
      var value = /** @type {boolean} */ (reader.readBool());
      msg.setNeedsSetup(value);
      break;
    case 4:
      var value = /** @type {string} */ (reader.readString());
      msg.setChallenge(value);
      break;
    case 5:
      var value = /** @type {number} */ (reader.readInt64());
      msg.setTimeout(value);
      break;
    case 6:
      var value = /** @type {string} */ (reader.readString());
      msg.setRpId(value);
      break;
    case 7:
      var value = /** @type {string} */ (reader.readString());
      msg.addAllowCredentials(value);
      break;
    case 8:
      var value = /** @type {string} */ (reader.readString());
      msg.setUserVerification(value);
      break;
    default:
      reader.skipField();
      break;
    }
  }
  return msg;
};


/**
 * Serializes the message to binary data (in protobuf wire format).
 * @return {!Uint8Array}
 */
proto.g8e.operator.v1.PasskeyAuthChallengeResult.prototype.serializeBinary = function() {
  var writer = new jspb.BinaryWriter();
  proto.g8e.operator.v1.PasskeyAuthChallengeResult.serializeBinaryToWriter(this, writer);
  return writer.getResultBuffer();
};


/**
 * Serializes the given message to binary data (in protobuf wire
 * format), writing to the given BinaryWriter.
 * @param {!proto.g8e.operator.v1.PasskeyAuthChallengeResult} message
 * @param {!jspb.BinaryWriter} writer
 * @suppress {unusedLocalVariables} f is only used for nested messages
 */
proto.g8e.operator.v1.PasskeyAuthChallengeResult.serializeBinaryToWriter = function(message, writer) {
  var f = undefined;
  f = message.getSuccess();
  if (f) {
    writer.writeBool(
      1,
      f
    );
  }
  f = message.getError();
  if (f.length > 0) {
    writer.writeString(
      2,
      f
    );
  }
  f = message.getNeedsSetup();
  if (f) {
    writer.writeBool(
      3,
      f
    );
  }
  f = message.getChallenge();
  if (f.length > 0) {
    writer.writeString(
      4,
      f
    );
  }
  f = message.getTimeout();
  if (f !== 0) {
    writer.writeInt64(
      5,
      f
    );
  }
  f = message.getRpId();
  if (f.length > 0) {
    writer.writeString(
      6,
      f
    );
  }
  f = message.getAllowCredentialsList();
  if (f.length > 0) {
    writer.writeRepeatedString(
      7,
      f
    );
  }
  f = message.getUserVerification();
  if (f.length > 0) {
    writer.writeString(
      8,
      f
    );
  }
};


/**
 * optional bool success = 1;
 * @return {boolean}
 */
proto.g8e.operator.v1.PasskeyAuthChallengeResult.prototype.getSuccess = function() {
  return /** @type {boolean} */ (jspb.Message.getBooleanFieldWithDefault(this, 1, false));
};


/**
 * @param {boolean} value
 * @return {!proto.g8e.operator.v1.PasskeyAuthChallengeResult} returns this
 */
proto.g8e.operator.v1.PasskeyAuthChallengeResult.prototype.setSuccess = function(value) {
  return jspb.Message.setProto3BooleanField(this, 1, value);
};


/**
 * optional string error = 2;
 * @return {string}
 */
proto.g8e.operator.v1.PasskeyAuthChallengeResult.prototype.getError = function() {
  return /** @type {string} */ (jspb.Message.getFieldWithDefault(this, 2, ""));
};


/**
 * @param {string} value
 * @return {!proto.g8e.operator.v1.PasskeyAuthChallengeResult} returns this
 */
proto.g8e.operator.v1.PasskeyAuthChallengeResult.prototype.setError = function(value) {
  return jspb.Message.setProto3StringField(this, 2, value);
};


/**
 * optional bool needs_setup = 3;
 * @return {boolean}
 */
proto.g8e.operator.v1.PasskeyAuthChallengeResult.prototype.getNeedsSetup = function() {
  return /** @type {boolean} */ (jspb.Message.getBooleanFieldWithDefault(this, 3, false));
};


/**
 * @param {boolean} value
 * @return {!proto.g8e.operator.v1.PasskeyAuthChallengeResult} returns this
 */
proto.g8e.operator.v1.PasskeyAuthChallengeResult.prototype.setNeedsSetup = function(value) {
  return jspb.Message.setProto3BooleanField(this, 3, value);
};


/**
 * optional string challenge = 4;
 * @return {string}
 */
proto.g8e.operator.v1.PasskeyAuthChallengeResult.prototype.getChallenge = function() {
  return /** @type {string} */ (jspb.Message.getFieldWithDefault(this, 4, ""));
};


/**
 * @param {string} value
 * @return {!proto.g8e.operator.v1.PasskeyAuthChallengeResult} returns this
 */
proto.g8e.operator.v1.PasskeyAuthChallengeResult.prototype.setChallenge = function(value) {
  return jspb.Message.setProto3StringField(this, 4, value);
};


/**
 * optional int64 timeout = 5;
 * @return {number}
 */
proto.g8e.operator.v1.PasskeyAuthChallengeResult.prototype.getTimeout = function() {
  return /** @type {number} */ (jspb.Message.getFieldWithDefault(this, 5, 0));
};


/**
 * @param {number} value
 * @return {!proto.g8e.operator.v1.PasskeyAuthChallengeResult} returns this
 */
proto.g8e.operator.v1.PasskeyAuthChallengeResult.prototype.setTimeout = function(value) {
  return jspb.Message.setProto3IntField(this, 5, value);
};


/**
 * optional string rp_id = 6;
 * @return {string}
 */
proto.g8e.operator.v1.PasskeyAuthChallengeResult.prototype.getRpId = function() {
  return /** @type {string} */ (jspb.Message.getFieldWithDefault(this, 6, ""));
};


/**
 * @param {string} value
 * @return {!proto.g8e.operator.v1.PasskeyAuthChallengeResult} returns this
 */
proto.g8e.operator.v1.PasskeyAuthChallengeResult.prototype.setRpId = function(value) {
  return jspb.Message.setProto3StringField(this, 6, value);
};


/**
 * repeated string allow_credentials = 7;
 * @return {!Array<string>}
 */
proto.g8e.operator.v1.PasskeyAuthChallengeResult.prototype.getAllowCredentialsList = function() {
  return /** @type {!Array<string>} */ (jspb.Message.getRepeatedField(this, 7));
};


/**
 * @param {!Array<string>} value
 * @return {!proto.g8e.operator.v1.PasskeyAuthChallengeResult} returns this
 */
proto.g8e.operator.v1.PasskeyAuthChallengeResult.prototype.setAllowCredentialsList = function(value) {
  return jspb.Message.setField(this, 7, value || []);
};


/**
 * @param {string} value
 * @param {number=} opt_index
 * @return {!proto.g8e.operator.v1.PasskeyAuthChallengeResult} returns this
 */
proto.g8e.operator.v1.PasskeyAuthChallengeResult.prototype.addAllowCredentials = function(value, opt_index) {
  return jspb.Message.addToRepeatedField(this, 7, value, opt_index);
};


/**
 * Clears the list making it empty but non-null.
 * @return {!proto.g8e.operator.v1.PasskeyAuthChallengeResult} returns this
 */
proto.g8e.operator.v1.PasskeyAuthChallengeResult.prototype.clearAllowCredentialsList = function() {
  return this.setAllowCredentialsList([]);
};


/**
 * optional string user_verification = 8;
 * @return {string}
 */
proto.g8e.operator.v1.PasskeyAuthChallengeResult.prototype.getUserVerification = function() {
  return /** @type {string} */ (jspb.Message.getFieldWithDefault(this, 8, ""));
};


/**
 * @param {string} value
 * @return {!proto.g8e.operator.v1.PasskeyAuthChallengeResult} returns this
 */
proto.g8e.operator.v1.PasskeyAuthChallengeResult.prototype.setUserVerification = function(value) {
  return jspb.Message.setProto3StringField(this, 8, value);
};





if (jspb.Message.GENERATE_TO_OBJECT) {
/**
 * Creates an object representation of this proto.
 * Field names that are reserved in JavaScript and will be renamed to pb_name.
 * Optional fields that are not set will be set to undefined.
 * To access a reserved field use, foo.pb_<name>, eg, foo.pb_default.
 * For the list of reserved names please see:
 *     net/proto2/compiler/js/internal/generator.cc#kKeyword.
 * @param {boolean=} opt_includeInstance Deprecated. whether to include the
 *     JSPB instance for transitional soy proto support:
 *     http://goto/soy-param-migration
 * @return {!Object}
 */
proto.g8e.operator.v1.AssertionResponse.prototype.toObject = function(opt_includeInstance) {
  return proto.g8e.operator.v1.AssertionResponse.toObject(opt_includeInstance, this);
};


/**
 * Static version of the {@see toObject} method.
 * @param {boolean|undefined} includeInstance Deprecated. Whether to include
 *     the JSPB instance for transitional soy proto support:
 *     http://goto/soy-param-migration
 * @param {!proto.g8e.operator.v1.AssertionResponse} msg The msg instance to transform.
 * @return {!Object}
 * @suppress {unusedLocalVariables} f is only used for nested messages
 */
proto.g8e.operator.v1.AssertionResponse.toObject = function(includeInstance, msg) {
  var f, obj = {
    id: jspb.Message.getFieldWithDefault(msg, 1, ""),
    rawId: jspb.Message.getFieldWithDefault(msg, 2, ""),
    clientDataJson: jspb.Message.getFieldWithDefault(msg, 3, ""),
    authenticatorData: jspb.Message.getFieldWithDefault(msg, 4, ""),
    signature: jspb.Message.getFieldWithDefault(msg, 5, ""),
    userHandle: jspb.Message.getFieldWithDefault(msg, 6, "")
  };

  if (includeInstance) {
    obj.$jspbMessageInstance = msg;
  }
  return obj;
};
}


/**
 * Deserializes binary data (in protobuf wire format).
 * @param {jspb.ByteSource} bytes The bytes to deserialize.
 * @return {!proto.g8e.operator.v1.AssertionResponse}
 */
proto.g8e.operator.v1.AssertionResponse.deserializeBinary = function(bytes) {
  var reader = new jspb.BinaryReader(bytes);
  var msg = new proto.g8e.operator.v1.AssertionResponse;
  return proto.g8e.operator.v1.AssertionResponse.deserializeBinaryFromReader(msg, reader);
};


/**
 * Deserializes binary data (in protobuf wire format) from the
 * given reader into the given message object.
 * @param {!proto.g8e.operator.v1.AssertionResponse} msg The message object to deserialize into.
 * @param {!jspb.BinaryReader} reader The BinaryReader to use.
 * @return {!proto.g8e.operator.v1.AssertionResponse}
 */
proto.g8e.operator.v1.AssertionResponse.deserializeBinaryFromReader = function(msg, reader) {
  while (reader.nextField()) {
    if (reader.isEndGroup()) {
      break;
    }
    var field = reader.getFieldNumber();
    switch (field) {
    case 1:
      var value = /** @type {string} */ (reader.readString());
      msg.setId(value);
      break;
    case 2:
      var value = /** @type {string} */ (reader.readString());
      msg.setRawId(value);
      break;
    case 3:
      var value = /** @type {string} */ (reader.readString());
      msg.setClientDataJson(value);
      break;
    case 4:
      var value = /** @type {string} */ (reader.readString());
      msg.setAuthenticatorData(value);
      break;
    case 5:
      var value = /** @type {string} */ (reader.readString());
      msg.setSignature(value);
      break;
    case 6:
      var value = /** @type {string} */ (reader.readString());
      msg.setUserHandle(value);
      break;
    default:
      reader.skipField();
      break;
    }
  }
  return msg;
};


/**
 * Serializes the message to binary data (in protobuf wire format).
 * @return {!Uint8Array}
 */
proto.g8e.operator.v1.AssertionResponse.prototype.serializeBinary = function() {
  var writer = new jspb.BinaryWriter();
  proto.g8e.operator.v1.AssertionResponse.serializeBinaryToWriter(this, writer);
  return writer.getResultBuffer();
};


/**
 * Serializes the given message to binary data (in protobuf wire
 * format), writing to the given BinaryWriter.
 * @param {!proto.g8e.operator.v1.AssertionResponse} message
 * @param {!jspb.BinaryWriter} writer
 * @suppress {unusedLocalVariables} f is only used for nested messages
 */
proto.g8e.operator.v1.AssertionResponse.serializeBinaryToWriter = function(message, writer) {
  var f = undefined;
  f = message.getId();
  if (f.length > 0) {
    writer.writeString(
      1,
      f
    );
  }
  f = message.getRawId();
  if (f.length > 0) {
    writer.writeString(
      2,
      f
    );
  }
  f = message.getClientDataJson();
  if (f.length > 0) {
    writer.writeString(
      3,
      f
    );
  }
  f = message.getAuthenticatorData();
  if (f.length > 0) {
    writer.writeString(
      4,
      f
    );
  }
  f = message.getSignature();
  if (f.length > 0) {
    writer.writeString(
      5,
      f
    );
  }
  f = message.getUserHandle();
  if (f.length > 0) {
    writer.writeString(
      6,
      f
    );
  }
};


/**
 * optional string id = 1;
 * @return {string}
 */
proto.g8e.operator.v1.AssertionResponse.prototype.getId = function() {
  return /** @type {string} */ (jspb.Message.getFieldWithDefault(this, 1, ""));
};


/**
 * @param {string} value
 * @return {!proto.g8e.operator.v1.AssertionResponse} returns this
 */
proto.g8e.operator.v1.AssertionResponse.prototype.setId = function(value) {
  return jspb.Message.setProto3StringField(this, 1, value);
};


/**
 * optional string raw_id = 2;
 * @return {string}
 */
proto.g8e.operator.v1.AssertionResponse.prototype.getRawId = function() {
  return /** @type {string} */ (jspb.Message.getFieldWithDefault(this, 2, ""));
};


/**
 * @param {string} value
 * @return {!proto.g8e.operator.v1.AssertionResponse} returns this
 */
proto.g8e.operator.v1.AssertionResponse.prototype.setRawId = function(value) {
  return jspb.Message.setProto3StringField(this, 2, value);
};


/**
 * optional string client_data_json = 3;
 * @return {string}
 */
proto.g8e.operator.v1.AssertionResponse.prototype.getClientDataJson = function() {
  return /** @type {string} */ (jspb.Message.getFieldWithDefault(this, 3, ""));
};


/**
 * @param {string} value
 * @return {!proto.g8e.operator.v1.AssertionResponse} returns this
 */
proto.g8e.operator.v1.AssertionResponse.prototype.setClientDataJson = function(value) {
  return jspb.Message.setProto3StringField(this, 3, value);
};


/**
 * optional string authenticator_data = 4;
 * @return {string}
 */
proto.g8e.operator.v1.AssertionResponse.prototype.getAuthenticatorData = function() {
  return /** @type {string} */ (jspb.Message.getFieldWithDefault(this, 4, ""));
};


/**
 * @param {string} value
 * @return {!proto.g8e.operator.v1.AssertionResponse} returns this
 */
proto.g8e.operator.v1.AssertionResponse.prototype.setAuthenticatorData = function(value) {
  return jspb.Message.setProto3StringField(this, 4, value);
};


/**
 * optional string signature = 5;
 * @return {string}
 */
proto.g8e.operator.v1.AssertionResponse.prototype.getSignature = function() {
  return /** @type {string} */ (jspb.Message.getFieldWithDefault(this, 5, ""));
};


/**
 * @param {string} value
 * @return {!proto.g8e.operator.v1.AssertionResponse} returns this
 */
proto.g8e.operator.v1.AssertionResponse.prototype.setSignature = function(value) {
  return jspb.Message.setProto3StringField(this, 5, value);
};


/**
 * optional string user_handle = 6;
 * @return {string}
 */
proto.g8e.operator.v1.AssertionResponse.prototype.getUserHandle = function() {
  return /** @type {string} */ (jspb.Message.getFieldWithDefault(this, 6, ""));
};


/**
 * @param {string} value
 * @return {!proto.g8e.operator.v1.AssertionResponse} returns this
 */
proto.g8e.operator.v1.AssertionResponse.prototype.setUserHandle = function(value) {
  return jspb.Message.setProto3StringField(this, 6, value);
};





if (jspb.Message.GENERATE_TO_OBJECT) {
/**
 * Creates an object representation of this proto.
 * Field names that are reserved in JavaScript and will be renamed to pb_name.
 * Optional fields that are not set will be set to undefined.
 * To access a reserved field use, foo.pb_<name>, eg, foo.pb_default.
 * For the list of reserved names please see:
 *     net/proto2/compiler/js/internal/generator.cc#kKeyword.
 * @param {boolean=} opt_includeInstance Deprecated. whether to include the
 *     JSPB instance for transitional soy proto support:
 *     http://goto/soy-param-migration
 * @return {!Object}
 */
proto.g8e.operator.v1.PasskeyAuthVerifyRequested.prototype.toObject = function(opt_includeInstance) {
  return proto.g8e.operator.v1.PasskeyAuthVerifyRequested.toObject(opt_includeInstance, this);
};


/**
 * Static version of the {@see toObject} method.
 * @param {boolean|undefined} includeInstance Deprecated. Whether to include
 *     the JSPB instance for transitional soy proto support:
 *     http://goto/soy-param-migration
 * @param {!proto.g8e.operator.v1.PasskeyAuthVerifyRequested} msg The msg instance to transform.
 * @return {!Object}
 * @suppress {unusedLocalVariables} f is only used for nested messages
 */
proto.g8e.operator.v1.PasskeyAuthVerifyRequested.toObject = function(includeInstance, msg) {
  var f, obj = {
    email: jspb.Message.getFieldWithDefault(msg, 1, ""),
    userId: jspb.Message.getFieldWithDefault(msg, 2, ""),
    assertionResponse: (f = msg.getAssertionResponse()) && proto.g8e.operator.v1.AssertionResponse.toObject(includeInstance, f)
  };

  if (includeInstance) {
    obj.$jspbMessageInstance = msg;
  }
  return obj;
};
}


/**
 * Deserializes binary data (in protobuf wire format).
 * @param {jspb.ByteSource} bytes The bytes to deserialize.
 * @return {!proto.g8e.operator.v1.PasskeyAuthVerifyRequested}
 */
proto.g8e.operator.v1.PasskeyAuthVerifyRequested.deserializeBinary = function(bytes) {
  var reader = new jspb.BinaryReader(bytes);
  var msg = new proto.g8e.operator.v1.PasskeyAuthVerifyRequested;
  return proto.g8e.operator.v1.PasskeyAuthVerifyRequested.deserializeBinaryFromReader(msg, reader);
};


/**
 * Deserializes binary data (in protobuf wire format) from the
 * given reader into the given message object.
 * @param {!proto.g8e.operator.v1.PasskeyAuthVerifyRequested} msg The message object to deserialize into.
 * @param {!jspb.BinaryReader} reader The BinaryReader to use.
 * @return {!proto.g8e.operator.v1.PasskeyAuthVerifyRequested}
 */
proto.g8e.operator.v1.PasskeyAuthVerifyRequested.deserializeBinaryFromReader = function(msg, reader) {
  while (reader.nextField()) {
    if (reader.isEndGroup()) {
      break;
    }
    var field = reader.getFieldNumber();
    switch (field) {
    case 1:
      var value = /** @type {string} */ (reader.readString());
      msg.setEmail(value);
      break;
    case 2:
      var value = /** @type {string} */ (reader.readString());
      msg.setUserId(value);
      break;
    case 3:
      var value = new proto.g8e.operator.v1.AssertionResponse;
      reader.readMessage(value,proto.g8e.operator.v1.AssertionResponse.deserializeBinaryFromReader);
      msg.setAssertionResponse(value);
      break;
    default:
      reader.skipField();
      break;
    }
  }
  return msg;
};


/**
 * Serializes the message to binary data (in protobuf wire format).
 * @return {!Uint8Array}
 */
proto.g8e.operator.v1.PasskeyAuthVerifyRequested.prototype.serializeBinary = function() {
  var writer = new jspb.BinaryWriter();
  proto.g8e.operator.v1.PasskeyAuthVerifyRequested.serializeBinaryToWriter(this, writer);
  return writer.getResultBuffer();
};


/**
 * Serializes the given message to binary data (in protobuf wire
 * format), writing to the given BinaryWriter.
 * @param {!proto.g8e.operator.v1.PasskeyAuthVerifyRequested} message
 * @param {!jspb.BinaryWriter} writer
 * @suppress {unusedLocalVariables} f is only used for nested messages
 */
proto.g8e.operator.v1.PasskeyAuthVerifyRequested.serializeBinaryToWriter = function(message, writer) {
  var f = undefined;
  f = message.getEmail();
  if (f.length > 0) {
    writer.writeString(
      1,
      f
    );
  }
  f = message.getUserId();
  if (f.length > 0) {
    writer.writeString(
      2,
      f
    );
  }
  f = message.getAssertionResponse();
  if (f != null) {
    writer.writeMessage(
      3,
      f,
      proto.g8e.operator.v1.AssertionResponse.serializeBinaryToWriter
    );
  }
};


/**
 * optional string email = 1;
 * @return {string}
 */
proto.g8e.operator.v1.PasskeyAuthVerifyRequested.prototype.getEmail = function() {
  return /** @type {string} */ (jspb.Message.getFieldWithDefault(this, 1, ""));
};


/**
 * @param {string} value
 * @return {!proto.g8e.operator.v1.PasskeyAuthVerifyRequested} returns this
 */
proto.g8e.operator.v1.PasskeyAuthVerifyRequested.prototype.setEmail = function(value) {
  return jspb.Message.setProto3StringField(this, 1, value);
};


/**
 * optional string user_id = 2;
 * @return {string}
 */
proto.g8e.operator.v1.PasskeyAuthVerifyRequested.prototype.getUserId = function() {
  return /** @type {string} */ (jspb.Message.getFieldWithDefault(this, 2, ""));
};


/**
 * @param {string} value
 * @return {!proto.g8e.operator.v1.PasskeyAuthVerifyRequested} returns this
 */
proto.g8e.operator.v1.PasskeyAuthVerifyRequested.prototype.setUserId = function(value) {
  return jspb.Message.setProto3StringField(this, 2, value);
};


/**
 * optional AssertionResponse assertion_response = 3;
 * @return {?proto.g8e.operator.v1.AssertionResponse}
 */
proto.g8e.operator.v1.PasskeyAuthVerifyRequested.prototype.getAssertionResponse = function() {
  return /** @type{?proto.g8e.operator.v1.AssertionResponse} */ (
    jspb.Message.getWrapperField(this, proto.g8e.operator.v1.AssertionResponse, 3));
};


/**
 * @param {?proto.g8e.operator.v1.AssertionResponse|undefined} value
 * @return {!proto.g8e.operator.v1.PasskeyAuthVerifyRequested} returns this
*/
proto.g8e.operator.v1.PasskeyAuthVerifyRequested.prototype.setAssertionResponse = function(value) {
  return jspb.Message.setWrapperField(this, 3, value);
};


/**
 * Clears the message field making it undefined.
 * @return {!proto.g8e.operator.v1.PasskeyAuthVerifyRequested} returns this
 */
proto.g8e.operator.v1.PasskeyAuthVerifyRequested.prototype.clearAssertionResponse = function() {
  return this.setAssertionResponse(undefined);
};


/**
 * Returns whether this field is set.
 * @return {boolean}
 */
proto.g8e.operator.v1.PasskeyAuthVerifyRequested.prototype.hasAssertionResponse = function() {
  return jspb.Message.getField(this, 3) != null;
};





if (jspb.Message.GENERATE_TO_OBJECT) {
/**
 * Creates an object representation of this proto.
 * Field names that are reserved in JavaScript and will be renamed to pb_name.
 * Optional fields that are not set will be set to undefined.
 * To access a reserved field use, foo.pb_<name>, eg, foo.pb_default.
 * For the list of reserved names please see:
 *     net/proto2/compiler/js/internal/generator.cc#kKeyword.
 * @param {boolean=} opt_includeInstance Deprecated. whether to include the
 *     JSPB instance for transitional soy proto support:
 *     http://goto/soy-param-migration
 * @return {!Object}
 */
proto.g8e.operator.v1.PasskeyAuthVerifyResult.prototype.toObject = function(opt_includeInstance) {
  return proto.g8e.operator.v1.PasskeyAuthVerifyResult.toObject(opt_includeInstance, this);
};


/**
 * Static version of the {@see toObject} method.
 * @param {boolean|undefined} includeInstance Deprecated. Whether to include
 *     the JSPB instance for transitional soy proto support:
 *     http://goto/soy-param-migration
 * @param {!proto.g8e.operator.v1.PasskeyAuthVerifyResult} msg The msg instance to transform.
 * @return {!Object}
 * @suppress {unusedLocalVariables} f is only used for nested messages
 */
proto.g8e.operator.v1.PasskeyAuthVerifyResult.toObject = function(includeInstance, msg) {
  var f, obj = {
    success: jspb.Message.getBooleanFieldWithDefault(msg, 1, false),
    error: jspb.Message.getFieldWithDefault(msg, 2, ""),
    userId: jspb.Message.getFieldWithDefault(msg, 3, ""),
    sessionId: jspb.Message.getFieldWithDefault(msg, 4, ""),
    sessionExpiresAtUnixMs: jspb.Message.getFieldWithDefault(msg, 5, 0)
  };

  if (includeInstance) {
    obj.$jspbMessageInstance = msg;
  }
  return obj;
};
}


/**
 * Deserializes binary data (in protobuf wire format).
 * @param {jspb.ByteSource} bytes The bytes to deserialize.
 * @return {!proto.g8e.operator.v1.PasskeyAuthVerifyResult}
 */
proto.g8e.operator.v1.PasskeyAuthVerifyResult.deserializeBinary = function(bytes) {
  var reader = new jspb.BinaryReader(bytes);
  var msg = new proto.g8e.operator.v1.PasskeyAuthVerifyResult;
  return proto.g8e.operator.v1.PasskeyAuthVerifyResult.deserializeBinaryFromReader(msg, reader);
};


/**
 * Deserializes binary data (in protobuf wire format) from the
 * given reader into the given message object.
 * @param {!proto.g8e.operator.v1.PasskeyAuthVerifyResult} msg The message object to deserialize into.
 * @param {!jspb.BinaryReader} reader The BinaryReader to use.
 * @return {!proto.g8e.operator.v1.PasskeyAuthVerifyResult}
 */
proto.g8e.operator.v1.PasskeyAuthVerifyResult.deserializeBinaryFromReader = function(msg, reader) {
  while (reader.nextField()) {
    if (reader.isEndGroup()) {
      break;
    }
    var field = reader.getFieldNumber();
    switch (field) {
    case 1:
      var value = /** @type {boolean} */ (reader.readBool());
      msg.setSuccess(value);
      break;
    case 2:
      var value = /** @type {string} */ (reader.readString());
      msg.setError(value);
      break;
    case 3:
      var value = /** @type {string} */ (reader.readString());
      msg.setUserId(value);
      break;
    case 4:
      var value = /** @type {string} */ (reader.readString());
      msg.setSessionId(value);
      break;
    case 5:
      var value = /** @type {number} */ (reader.readInt64());
      msg.setSessionExpiresAtUnixMs(value);
      break;
    default:
      reader.skipField();
      break;
    }
  }
  return msg;
};


/**
 * Serializes the message to binary data (in protobuf wire format).
 * @return {!Uint8Array}
 */
proto.g8e.operator.v1.PasskeyAuthVerifyResult.prototype.serializeBinary = function() {
  var writer = new jspb.BinaryWriter();
  proto.g8e.operator.v1.PasskeyAuthVerifyResult.serializeBinaryToWriter(this, writer);
  return writer.getResultBuffer();
};


/**
 * Serializes the given message to binary data (in protobuf wire
 * format), writing to the given BinaryWriter.
 * @param {!proto.g8e.operator.v1.PasskeyAuthVerifyResult} message
 * @param {!jspb.BinaryWriter} writer
 * @suppress {unusedLocalVariables} f is only used for nested messages
 */
proto.g8e.operator.v1.PasskeyAuthVerifyResult.serializeBinaryToWriter = function(message, writer) {
  var f = undefined;
  f = message.getSuccess();
  if (f) {
    writer.writeBool(
      1,
      f
    );
  }
  f = message.getError();
  if (f.length > 0) {
    writer.writeString(
      2,
      f
    );
  }
  f = message.getUserId();
  if (f.length > 0) {
    writer.writeString(
      3,
      f
    );
  }
  f = message.getSessionId();
  if (f.length > 0) {
    writer.writeString(
      4,
      f
    );
  }
  f = message.getSessionExpiresAtUnixMs();
  if (f !== 0) {
    writer.writeInt64(
      5,
      f
    );
  }
};


/**
 * optional bool success = 1;
 * @return {boolean}
 */
proto.g8e.operator.v1.PasskeyAuthVerifyResult.prototype.getSuccess = function() {
  return /** @type {boolean} */ (jspb.Message.getBooleanFieldWithDefault(this, 1, false));
};


/**
 * @param {boolean} value
 * @return {!proto.g8e.operator.v1.PasskeyAuthVerifyResult} returns this
 */
proto.g8e.operator.v1.PasskeyAuthVerifyResult.prototype.setSuccess = function(value) {
  return jspb.Message.setProto3BooleanField(this, 1, value);
};


/**
 * optional string error = 2;
 * @return {string}
 */
proto.g8e.operator.v1.PasskeyAuthVerifyResult.prototype.getError = function() {
  return /** @type {string} */ (jspb.Message.getFieldWithDefault(this, 2, ""));
};


/**
 * @param {string} value
 * @return {!proto.g8e.operator.v1.PasskeyAuthVerifyResult} returns this
 */
proto.g8e.operator.v1.PasskeyAuthVerifyResult.prototype.setError = function(value) {
  return jspb.Message.setProto3StringField(this, 2, value);
};


/**
 * optional string user_id = 3;
 * @return {string}
 */
proto.g8e.operator.v1.PasskeyAuthVerifyResult.prototype.getUserId = function() {
  return /** @type {string} */ (jspb.Message.getFieldWithDefault(this, 3, ""));
};


/**
 * @param {string} value
 * @return {!proto.g8e.operator.v1.PasskeyAuthVerifyResult} returns this
 */
proto.g8e.operator.v1.PasskeyAuthVerifyResult.prototype.setUserId = function(value) {
  return jspb.Message.setProto3StringField(this, 3, value);
};


/**
 * optional string session_id = 4;
 * @return {string}
 */
proto.g8e.operator.v1.PasskeyAuthVerifyResult.prototype.getSessionId = function() {
  return /** @type {string} */ (jspb.Message.getFieldWithDefault(this, 4, ""));
};


/**
 * @param {string} value
 * @return {!proto.g8e.operator.v1.PasskeyAuthVerifyResult} returns this
 */
proto.g8e.operator.v1.PasskeyAuthVerifyResult.prototype.setSessionId = function(value) {
  return jspb.Message.setProto3StringField(this, 4, value);
};


/**
 * optional int64 session_expires_at_unix_ms = 5;
 * @return {number}
 */
proto.g8e.operator.v1.PasskeyAuthVerifyResult.prototype.getSessionExpiresAtUnixMs = function() {
  return /** @type {number} */ (jspb.Message.getFieldWithDefault(this, 5, 0));
};


/**
 * @param {number} value
 * @return {!proto.g8e.operator.v1.PasskeyAuthVerifyResult} returns this
 */
proto.g8e.operator.v1.PasskeyAuthVerifyResult.prototype.setSessionExpiresAtUnixMs = function(value) {
  return jspb.Message.setProto3IntField(this, 5, value);
};





if (jspb.Message.GENERATE_TO_OBJECT) {
/**
 * Creates an object representation of this proto.
 * Field names that are reserved in JavaScript and will be renamed to pb_name.
 * Optional fields that are not set will be set to undefined.
 * To access a reserved field use, foo.pb_<name>, eg, foo.pb_default.
 * For the list of reserved names please see:
 *     net/proto2/compiler/js/internal/generator.cc#kKeyword.
 * @param {boolean=} opt_includeInstance Deprecated. whether to include the
 *     JSPB instance for transitional soy proto support:
 *     http://goto/soy-param-migration
 * @return {!Object}
 */
proto.g8e.operator.v1.ListPasskeyCredentialsRequested.prototype.toObject = function(opt_includeInstance) {
  return proto.g8e.operator.v1.ListPasskeyCredentialsRequested.toObject(opt_includeInstance, this);
};


/**
 * Static version of the {@see toObject} method.
 * @param {boolean|undefined} includeInstance Deprecated. Whether to include
 *     the JSPB instance for transitional soy proto support:
 *     http://goto/soy-param-migration
 * @param {!proto.g8e.operator.v1.ListPasskeyCredentialsRequested} msg The msg instance to transform.
 * @return {!Object}
 * @suppress {unusedLocalVariables} f is only used for nested messages
 */
proto.g8e.operator.v1.ListPasskeyCredentialsRequested.toObject = function(includeInstance, msg) {
  var f, obj = {
    userId: jspb.Message.getFieldWithDefault(msg, 1, "")
  };

  if (includeInstance) {
    obj.$jspbMessageInstance = msg;
  }
  return obj;
};
}


/**
 * Deserializes binary data (in protobuf wire format).
 * @param {jspb.ByteSource} bytes The bytes to deserialize.
 * @return {!proto.g8e.operator.v1.ListPasskeyCredentialsRequested}
 */
proto.g8e.operator.v1.ListPasskeyCredentialsRequested.deserializeBinary = function(bytes) {
  var reader = new jspb.BinaryReader(bytes);
  var msg = new proto.g8e.operator.v1.ListPasskeyCredentialsRequested;
  return proto.g8e.operator.v1.ListPasskeyCredentialsRequested.deserializeBinaryFromReader(msg, reader);
};


/**
 * Deserializes binary data (in protobuf wire format) from the
 * given reader into the given message object.
 * @param {!proto.g8e.operator.v1.ListPasskeyCredentialsRequested} msg The message object to deserialize into.
 * @param {!jspb.BinaryReader} reader The BinaryReader to use.
 * @return {!proto.g8e.operator.v1.ListPasskeyCredentialsRequested}
 */
proto.g8e.operator.v1.ListPasskeyCredentialsRequested.deserializeBinaryFromReader = function(msg, reader) {
  while (reader.nextField()) {
    if (reader.isEndGroup()) {
      break;
    }
    var field = reader.getFieldNumber();
    switch (field) {
    case 1:
      var value = /** @type {string} */ (reader.readString());
      msg.setUserId(value);
      break;
    default:
      reader.skipField();
      break;
    }
  }
  return msg;
};


/**
 * Serializes the message to binary data (in protobuf wire format).
 * @return {!Uint8Array}
 */
proto.g8e.operator.v1.ListPasskeyCredentialsRequested.prototype.serializeBinary = function() {
  var writer = new jspb.BinaryWriter();
  proto.g8e.operator.v1.ListPasskeyCredentialsRequested.serializeBinaryToWriter(this, writer);
  return writer.getResultBuffer();
};


/**
 * Serializes the given message to binary data (in protobuf wire
 * format), writing to the given BinaryWriter.
 * @param {!proto.g8e.operator.v1.ListPasskeyCredentialsRequested} message
 * @param {!jspb.BinaryWriter} writer
 * @suppress {unusedLocalVariables} f is only used for nested messages
 */
proto.g8e.operator.v1.ListPasskeyCredentialsRequested.serializeBinaryToWriter = function(message, writer) {
  var f = undefined;
  f = message.getUserId();
  if (f.length > 0) {
    writer.writeString(
      1,
      f
    );
  }
};


/**
 * optional string user_id = 1;
 * @return {string}
 */
proto.g8e.operator.v1.ListPasskeyCredentialsRequested.prototype.getUserId = function() {
  return /** @type {string} */ (jspb.Message.getFieldWithDefault(this, 1, ""));
};


/**
 * @param {string} value
 * @return {!proto.g8e.operator.v1.ListPasskeyCredentialsRequested} returns this
 */
proto.g8e.operator.v1.ListPasskeyCredentialsRequested.prototype.setUserId = function(value) {
  return jspb.Message.setProto3StringField(this, 1, value);
};



/**
 * List of repeated fields within this message type.
 * @private {!Array<number>}
 * @const
 */
proto.g8e.operator.v1.ListPasskeyCredentialsResult.repeatedFields_ = [3];



if (jspb.Message.GENERATE_TO_OBJECT) {
/**
 * Creates an object representation of this proto.
 * Field names that are reserved in JavaScript and will be renamed to pb_name.
 * Optional fields that are not set will be set to undefined.
 * To access a reserved field use, foo.pb_<name>, eg, foo.pb_default.
 * For the list of reserved names please see:
 *     net/proto2/compiler/js/internal/generator.cc#kKeyword.
 * @param {boolean=} opt_includeInstance Deprecated. whether to include the
 *     JSPB instance for transitional soy proto support:
 *     http://goto/soy-param-migration
 * @return {!Object}
 */
proto.g8e.operator.v1.ListPasskeyCredentialsResult.prototype.toObject = function(opt_includeInstance) {
  return proto.g8e.operator.v1.ListPasskeyCredentialsResult.toObject(opt_includeInstance, this);
};


/**
 * Static version of the {@see toObject} method.
 * @param {boolean|undefined} includeInstance Deprecated. Whether to include
 *     the JSPB instance for transitional soy proto support:
 *     http://goto/soy-param-migration
 * @param {!proto.g8e.operator.v1.ListPasskeyCredentialsResult} msg The msg instance to transform.
 * @return {!Object}
 * @suppress {unusedLocalVariables} f is only used for nested messages
 */
proto.g8e.operator.v1.ListPasskeyCredentialsResult.toObject = function(includeInstance, msg) {
  var f, obj = {
    success: jspb.Message.getBooleanFieldWithDefault(msg, 1, false),
    error: jspb.Message.getFieldWithDefault(msg, 2, ""),
    credentialsList: jspb.Message.toObjectList(msg.getCredentialsList(),
    proto.g8e.operator.v1.PasskeyCredential.toObject, includeInstance)
  };

  if (includeInstance) {
    obj.$jspbMessageInstance = msg;
  }
  return obj;
};
}


/**
 * Deserializes binary data (in protobuf wire format).
 * @param {jspb.ByteSource} bytes The bytes to deserialize.
 * @return {!proto.g8e.operator.v1.ListPasskeyCredentialsResult}
 */
proto.g8e.operator.v1.ListPasskeyCredentialsResult.deserializeBinary = function(bytes) {
  var reader = new jspb.BinaryReader(bytes);
  var msg = new proto.g8e.operator.v1.ListPasskeyCredentialsResult;
  return proto.g8e.operator.v1.ListPasskeyCredentialsResult.deserializeBinaryFromReader(msg, reader);
};


/**
 * Deserializes binary data (in protobuf wire format) from the
 * given reader into the given message object.
 * @param {!proto.g8e.operator.v1.ListPasskeyCredentialsResult} msg The message object to deserialize into.
 * @param {!jspb.BinaryReader} reader The BinaryReader to use.
 * @return {!proto.g8e.operator.v1.ListPasskeyCredentialsResult}
 */
proto.g8e.operator.v1.ListPasskeyCredentialsResult.deserializeBinaryFromReader = function(msg, reader) {
  while (reader.nextField()) {
    if (reader.isEndGroup()) {
      break;
    }
    var field = reader.getFieldNumber();
    switch (field) {
    case 1:
      var value = /** @type {boolean} */ (reader.readBool());
      msg.setSuccess(value);
      break;
    case 2:
      var value = /** @type {string} */ (reader.readString());
      msg.setError(value);
      break;
    case 3:
      var value = new proto.g8e.operator.v1.PasskeyCredential;
      reader.readMessage(value,proto.g8e.operator.v1.PasskeyCredential.deserializeBinaryFromReader);
      msg.addCredentials(value);
      break;
    default:
      reader.skipField();
      break;
    }
  }
  return msg;
};


/**
 * Serializes the message to binary data (in protobuf wire format).
 * @return {!Uint8Array}
 */
proto.g8e.operator.v1.ListPasskeyCredentialsResult.prototype.serializeBinary = function() {
  var writer = new jspb.BinaryWriter();
  proto.g8e.operator.v1.ListPasskeyCredentialsResult.serializeBinaryToWriter(this, writer);
  return writer.getResultBuffer();
};


/**
 * Serializes the given message to binary data (in protobuf wire
 * format), writing to the given BinaryWriter.
 * @param {!proto.g8e.operator.v1.ListPasskeyCredentialsResult} message
 * @param {!jspb.BinaryWriter} writer
 * @suppress {unusedLocalVariables} f is only used for nested messages
 */
proto.g8e.operator.v1.ListPasskeyCredentialsResult.serializeBinaryToWriter = function(message, writer) {
  var f = undefined;
  f = message.getSuccess();
  if (f) {
    writer.writeBool(
      1,
      f
    );
  }
  f = message.getError();
  if (f.length > 0) {
    writer.writeString(
      2,
      f
    );
  }
  f = message.getCredentialsList();
  if (f.length > 0) {
    writer.writeRepeatedMessage(
      3,
      f,
      proto.g8e.operator.v1.PasskeyCredential.serializeBinaryToWriter
    );
  }
};


/**
 * optional bool success = 1;
 * @return {boolean}
 */
proto.g8e.operator.v1.ListPasskeyCredentialsResult.prototype.getSuccess = function() {
  return /** @type {boolean} */ (jspb.Message.getBooleanFieldWithDefault(this, 1, false));
};


/**
 * @param {boolean} value
 * @return {!proto.g8e.operator.v1.ListPasskeyCredentialsResult} returns this
 */
proto.g8e.operator.v1.ListPasskeyCredentialsResult.prototype.setSuccess = function(value) {
  return jspb.Message.setProto3BooleanField(this, 1, value);
};


/**
 * optional string error = 2;
 * @return {string}
 */
proto.g8e.operator.v1.ListPasskeyCredentialsResult.prototype.getError = function() {
  return /** @type {string} */ (jspb.Message.getFieldWithDefault(this, 2, ""));
};


/**
 * @param {string} value
 * @return {!proto.g8e.operator.v1.ListPasskeyCredentialsResult} returns this
 */
proto.g8e.operator.v1.ListPasskeyCredentialsResult.prototype.setError = function(value) {
  return jspb.Message.setProto3StringField(this, 2, value);
};


/**
 * repeated PasskeyCredential credentials = 3;
 * @return {!Array<!proto.g8e.operator.v1.PasskeyCredential>}
 */
proto.g8e.operator.v1.ListPasskeyCredentialsResult.prototype.getCredentialsList = function() {
  return /** @type{!Array<!proto.g8e.operator.v1.PasskeyCredential>} */ (
    jspb.Message.getRepeatedWrapperField(this, proto.g8e.operator.v1.PasskeyCredential, 3));
};


/**
 * @param {!Array<!proto.g8e.operator.v1.PasskeyCredential>} value
 * @return {!proto.g8e.operator.v1.ListPasskeyCredentialsResult} returns this
*/
proto.g8e.operator.v1.ListPasskeyCredentialsResult.prototype.setCredentialsList = function(value) {
  return jspb.Message.setRepeatedWrapperField(this, 3, value);
};


/**
 * @param {!proto.g8e.operator.v1.PasskeyCredential=} opt_value
 * @param {number=} opt_index
 * @return {!proto.g8e.operator.v1.PasskeyCredential}
 */
proto.g8e.operator.v1.ListPasskeyCredentialsResult.prototype.addCredentials = function(opt_value, opt_index) {
  return jspb.Message.addToRepeatedWrapperField(this, 3, opt_value, proto.g8e.operator.v1.PasskeyCredential, opt_index);
};


/**
 * Clears the list making it empty but non-null.
 * @return {!proto.g8e.operator.v1.ListPasskeyCredentialsResult} returns this
 */
proto.g8e.operator.v1.ListPasskeyCredentialsResult.prototype.clearCredentialsList = function() {
  return this.setCredentialsList([]);
};





if (jspb.Message.GENERATE_TO_OBJECT) {
/**
 * Creates an object representation of this proto.
 * Field names that are reserved in JavaScript and will be renamed to pb_name.
 * Optional fields that are not set will be set to undefined.
 * To access a reserved field use, foo.pb_<name>, eg, foo.pb_default.
 * For the list of reserved names please see:
 *     net/proto2/compiler/js/internal/generator.cc#kKeyword.
 * @param {boolean=} opt_includeInstance Deprecated. whether to include the
 *     JSPB instance for transitional soy proto support:
 *     http://goto/soy-param-migration
 * @return {!Object}
 */
proto.g8e.operator.v1.RevokePasskeyCredentialRequested.prototype.toObject = function(opt_includeInstance) {
  return proto.g8e.operator.v1.RevokePasskeyCredentialRequested.toObject(opt_includeInstance, this);
};


/**
 * Static version of the {@see toObject} method.
 * @param {boolean|undefined} includeInstance Deprecated. Whether to include
 *     the JSPB instance for transitional soy proto support:
 *     http://goto/soy-param-migration
 * @param {!proto.g8e.operator.v1.RevokePasskeyCredentialRequested} msg The msg instance to transform.
 * @return {!Object}
 * @suppress {unusedLocalVariables} f is only used for nested messages
 */
proto.g8e.operator.v1.RevokePasskeyCredentialRequested.toObject = function(includeInstance, msg) {
  var f, obj = {
    userId: jspb.Message.getFieldWithDefault(msg, 1, ""),
    credentialId: jspb.Message.getFieldWithDefault(msg, 2, "")
  };

  if (includeInstance) {
    obj.$jspbMessageInstance = msg;
  }
  return obj;
};
}


/**
 * Deserializes binary data (in protobuf wire format).
 * @param {jspb.ByteSource} bytes The bytes to deserialize.
 * @return {!proto.g8e.operator.v1.RevokePasskeyCredentialRequested}
 */
proto.g8e.operator.v1.RevokePasskeyCredentialRequested.deserializeBinary = function(bytes) {
  var reader = new jspb.BinaryReader(bytes);
  var msg = new proto.g8e.operator.v1.RevokePasskeyCredentialRequested;
  return proto.g8e.operator.v1.RevokePasskeyCredentialRequested.deserializeBinaryFromReader(msg, reader);
};


/**
 * Deserializes binary data (in protobuf wire format) from the
 * given reader into the given message object.
 * @param {!proto.g8e.operator.v1.RevokePasskeyCredentialRequested} msg The message object to deserialize into.
 * @param {!jspb.BinaryReader} reader The BinaryReader to use.
 * @return {!proto.g8e.operator.v1.RevokePasskeyCredentialRequested}
 */
proto.g8e.operator.v1.RevokePasskeyCredentialRequested.deserializeBinaryFromReader = function(msg, reader) {
  while (reader.nextField()) {
    if (reader.isEndGroup()) {
      break;
    }
    var field = reader.getFieldNumber();
    switch (field) {
    case 1:
      var value = /** @type {string} */ (reader.readString());
      msg.setUserId(value);
      break;
    case 2:
      var value = /** @type {string} */ (reader.readString());
      msg.setCredentialId(value);
      break;
    default:
      reader.skipField();
      break;
    }
  }
  return msg;
};


/**
 * Serializes the message to binary data (in protobuf wire format).
 * @return {!Uint8Array}
 */
proto.g8e.operator.v1.RevokePasskeyCredentialRequested.prototype.serializeBinary = function() {
  var writer = new jspb.BinaryWriter();
  proto.g8e.operator.v1.RevokePasskeyCredentialRequested.serializeBinaryToWriter(this, writer);
  return writer.getResultBuffer();
};


/**
 * Serializes the given message to binary data (in protobuf wire
 * format), writing to the given BinaryWriter.
 * @param {!proto.g8e.operator.v1.RevokePasskeyCredentialRequested} message
 * @param {!jspb.BinaryWriter} writer
 * @suppress {unusedLocalVariables} f is only used for nested messages
 */
proto.g8e.operator.v1.RevokePasskeyCredentialRequested.serializeBinaryToWriter = function(message, writer) {
  var f = undefined;
  f = message.getUserId();
  if (f.length > 0) {
    writer.writeString(
      1,
      f
    );
  }
  f = message.getCredentialId();
  if (f.length > 0) {
    writer.writeString(
      2,
      f
    );
  }
};


/**
 * optional string user_id = 1;
 * @return {string}
 */
proto.g8e.operator.v1.RevokePasskeyCredentialRequested.prototype.getUserId = function() {
  return /** @type {string} */ (jspb.Message.getFieldWithDefault(this, 1, ""));
};


/**
 * @param {string} value
 * @return {!proto.g8e.operator.v1.RevokePasskeyCredentialRequested} returns this
 */
proto.g8e.operator.v1.RevokePasskeyCredentialRequested.prototype.setUserId = function(value) {
  return jspb.Message.setProto3StringField(this, 1, value);
};


/**
 * optional string credential_id = 2;
 * @return {string}
 */
proto.g8e.operator.v1.RevokePasskeyCredentialRequested.prototype.getCredentialId = function() {
  return /** @type {string} */ (jspb.Message.getFieldWithDefault(this, 2, ""));
};


/**
 * @param {string} value
 * @return {!proto.g8e.operator.v1.RevokePasskeyCredentialRequested} returns this
 */
proto.g8e.operator.v1.RevokePasskeyCredentialRequested.prototype.setCredentialId = function(value) {
  return jspb.Message.setProto3StringField(this, 2, value);
};





if (jspb.Message.GENERATE_TO_OBJECT) {
/**
 * Creates an object representation of this proto.
 * Field names that are reserved in JavaScript and will be renamed to pb_name.
 * Optional fields that are not set will be set to undefined.
 * To access a reserved field use, foo.pb_<name>, eg, foo.pb_default.
 * For the list of reserved names please see:
 *     net/proto2/compiler/js/internal/generator.cc#kKeyword.
 * @param {boolean=} opt_includeInstance Deprecated. whether to include the
 *     JSPB instance for transitional soy proto support:
 *     http://goto/soy-param-migration
 * @return {!Object}
 */
proto.g8e.operator.v1.RevokePasskeyCredentialResult.prototype.toObject = function(opt_includeInstance) {
  return proto.g8e.operator.v1.RevokePasskeyCredentialResult.toObject(opt_includeInstance, this);
};


/**
 * Static version of the {@see toObject} method.
 * @param {boolean|undefined} includeInstance Deprecated. Whether to include
 *     the JSPB instance for transitional soy proto support:
 *     http://goto/soy-param-migration
 * @param {!proto.g8e.operator.v1.RevokePasskeyCredentialResult} msg The msg instance to transform.
 * @return {!Object}
 * @suppress {unusedLocalVariables} f is only used for nested messages
 */
proto.g8e.operator.v1.RevokePasskeyCredentialResult.toObject = function(includeInstance, msg) {
  var f, obj = {
    success: jspb.Message.getBooleanFieldWithDefault(msg, 1, false),
    error: jspb.Message.getFieldWithDefault(msg, 2, ""),
    found: jspb.Message.getBooleanFieldWithDefault(msg, 3, false),
    remaining: jspb.Message.getFieldWithDefault(msg, 4, 0)
  };

  if (includeInstance) {
    obj.$jspbMessageInstance = msg;
  }
  return obj;
};
}


/**
 * Deserializes binary data (in protobuf wire format).
 * @param {jspb.ByteSource} bytes The bytes to deserialize.
 * @return {!proto.g8e.operator.v1.RevokePasskeyCredentialResult}
 */
proto.g8e.operator.v1.RevokePasskeyCredentialResult.deserializeBinary = function(bytes) {
  var reader = new jspb.BinaryReader(bytes);
  var msg = new proto.g8e.operator.v1.RevokePasskeyCredentialResult;
  return proto.g8e.operator.v1.RevokePasskeyCredentialResult.deserializeBinaryFromReader(msg, reader);
};


/**
 * Deserializes binary data (in protobuf wire format) from the
 * given reader into the given message object.
 * @param {!proto.g8e.operator.v1.RevokePasskeyCredentialResult} msg The message object to deserialize into.
 * @param {!jspb.BinaryReader} reader The BinaryReader to use.
 * @return {!proto.g8e.operator.v1.RevokePasskeyCredentialResult}
 */
proto.g8e.operator.v1.RevokePasskeyCredentialResult.deserializeBinaryFromReader = function(msg, reader) {
  while (reader.nextField()) {
    if (reader.isEndGroup()) {
      break;
    }
    var field = reader.getFieldNumber();
    switch (field) {
    case 1:
      var value = /** @type {boolean} */ (reader.readBool());
      msg.setSuccess(value);
      break;
    case 2:
      var value = /** @type {string} */ (reader.readString());
      msg.setError(value);
      break;
    case 3:
      var value = /** @type {boolean} */ (reader.readBool());
      msg.setFound(value);
      break;
    case 4:
      var value = /** @type {number} */ (reader.readInt32());
      msg.setRemaining(value);
      break;
    default:
      reader.skipField();
      break;
    }
  }
  return msg;
};


/**
 * Serializes the message to binary data (in protobuf wire format).
 * @return {!Uint8Array}
 */
proto.g8e.operator.v1.RevokePasskeyCredentialResult.prototype.serializeBinary = function() {
  var writer = new jspb.BinaryWriter();
  proto.g8e.operator.v1.RevokePasskeyCredentialResult.serializeBinaryToWriter(this, writer);
  return writer.getResultBuffer();
};


/**
 * Serializes the given message to binary data (in protobuf wire
 * format), writing to the given BinaryWriter.
 * @param {!proto.g8e.operator.v1.RevokePasskeyCredentialResult} message
 * @param {!jspb.BinaryWriter} writer
 * @suppress {unusedLocalVariables} f is only used for nested messages
 */
proto.g8e.operator.v1.RevokePasskeyCredentialResult.serializeBinaryToWriter = function(message, writer) {
  var f = undefined;
  f = message.getSuccess();
  if (f) {
    writer.writeBool(
      1,
      f
    );
  }
  f = message.getError();
  if (f.length > 0) {
    writer.writeString(
      2,
      f
    );
  }
  f = message.getFound();
  if (f) {
    writer.writeBool(
      3,
      f
    );
  }
  f = message.getRemaining();
  if (f !== 0) {
    writer.writeInt32(
      4,
      f
    );
  }
};


/**
 * optional bool success = 1;
 * @return {boolean}
 */
proto.g8e.operator.v1.RevokePasskeyCredentialResult.prototype.getSuccess = function() {
  return /** @type {boolean} */ (jspb.Message.getBooleanFieldWithDefault(this, 1, false));
};


/**
 * @param {boolean} value
 * @return {!proto.g8e.operator.v1.RevokePasskeyCredentialResult} returns this
 */
proto.g8e.operator.v1.RevokePasskeyCredentialResult.prototype.setSuccess = function(value) {
  return jspb.Message.setProto3BooleanField(this, 1, value);
};


/**
 * optional string error = 2;
 * @return {string}
 */
proto.g8e.operator.v1.RevokePasskeyCredentialResult.prototype.getError = function() {
  return /** @type {string} */ (jspb.Message.getFieldWithDefault(this, 2, ""));
};


/**
 * @param {string} value
 * @return {!proto.g8e.operator.v1.RevokePasskeyCredentialResult} returns this
 */
proto.g8e.operator.v1.RevokePasskeyCredentialResult.prototype.setError = function(value) {
  return jspb.Message.setProto3StringField(this, 2, value);
};


/**
 * optional bool found = 3;
 * @return {boolean}
 */
proto.g8e.operator.v1.RevokePasskeyCredentialResult.prototype.getFound = function() {
  return /** @type {boolean} */ (jspb.Message.getBooleanFieldWithDefault(this, 3, false));
};


/**
 * @param {boolean} value
 * @return {!proto.g8e.operator.v1.RevokePasskeyCredentialResult} returns this
 */
proto.g8e.operator.v1.RevokePasskeyCredentialResult.prototype.setFound = function(value) {
  return jspb.Message.setProto3BooleanField(this, 3, value);
};


/**
 * optional int32 remaining = 4;
 * @return {number}
 */
proto.g8e.operator.v1.RevokePasskeyCredentialResult.prototype.getRemaining = function() {
  return /** @type {number} */ (jspb.Message.getFieldWithDefault(this, 4, 0));
};


/**
 * @param {number} value
 * @return {!proto.g8e.operator.v1.RevokePasskeyCredentialResult} returns this
 */
proto.g8e.operator.v1.RevokePasskeyCredentialResult.prototype.setRemaining = function(value) {
  return jspb.Message.setProto3IntField(this, 4, value);
};


/**
 * @enum {number}
 */
proto.g8e.operator.v1.ExecutionStatus = {
  EXECUTION_STATUS_UNSPECIFIED: 0,
  EXECUTION_STATUS_EXECUTING: 1,
  EXECUTION_STATUS_COMPLETED: 2,
  EXECUTION_STATUS_FAILED: 3,
  EXECUTION_STATUS_CANCELLED: 4,
  EXECUTION_STATUS_TIMEOUT: 5
};

/**
 * @enum {number}
 */
proto.g8e.operator.v1.HeartbeatType = {
  HEARTBEAT_TYPE_UNSPECIFIED: 0,
  HEARTBEAT_TYPE_AUTOMATIC: 1,
  HEARTBEAT_TYPE_MANUAL: 2
};

goog.object.extend(exports, proto.g8e.operator.v1);
