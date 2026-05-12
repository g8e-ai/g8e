// GENERATED CODE -- DO NOT EDIT!

'use strict';
var grpc = require('@grpc/grpc-js');
var operator_pb = require('./operator_pb.js');
var common_pb = require('./common_pb.js');

function serialize_g8e_operator_v1_CommandCancelRequested(arg) {
  if (!(arg instanceof operator_pb.CommandCancelRequested)) {
    throw new Error('Expected argument of type g8e.operator.v1.CommandCancelRequested');
  }
  return Buffer.from(arg.serializeBinary());
}

function deserialize_g8e_operator_v1_CommandCancelRequested(buffer_arg) {
  return operator_pb.CommandCancelRequested.deserializeBinary(new Uint8Array(buffer_arg));
}

function serialize_g8e_operator_v1_CommandRequested(arg) {
  if (!(arg instanceof operator_pb.CommandRequested)) {
    throw new Error('Expected argument of type g8e.operator.v1.CommandRequested');
  }
  return Buffer.from(arg.serializeBinary());
}

function deserialize_g8e_operator_v1_CommandRequested(buffer_arg) {
  return operator_pb.CommandRequested.deserializeBinary(new Uint8Array(buffer_arg));
}

function serialize_g8e_operator_v1_CommandResult(arg) {
  if (!(arg instanceof operator_pb.CommandResult)) {
    throw new Error('Expected argument of type g8e.operator.v1.CommandResult');
  }
  return Buffer.from(arg.serializeBinary());
}

function deserialize_g8e_operator_v1_CommandResult(buffer_arg) {
  return operator_pb.CommandResult.deserializeBinary(new Uint8Array(buffer_arg));
}

function serialize_g8e_operator_v1_FileEditRequested(arg) {
  if (!(arg instanceof operator_pb.FileEditRequested)) {
    throw new Error('Expected argument of type g8e.operator.v1.FileEditRequested');
  }
  return Buffer.from(arg.serializeBinary());
}

function deserialize_g8e_operator_v1_FileEditRequested(buffer_arg) {
  return operator_pb.FileEditRequested.deserializeBinary(new Uint8Array(buffer_arg));
}

function serialize_g8e_operator_v1_FsListRequested(arg) {
  if (!(arg instanceof operator_pb.FsListRequested)) {
    throw new Error('Expected argument of type g8e.operator.v1.FsListRequested');
  }
  return Buffer.from(arg.serializeBinary());
}

function deserialize_g8e_operator_v1_FsListRequested(buffer_arg) {
  return operator_pb.FsListRequested.deserializeBinary(new Uint8Array(buffer_arg));
}

function serialize_g8e_operator_v1_FsReadRequested(arg) {
  if (!(arg instanceof operator_pb.FsReadRequested)) {
    throw new Error('Expected argument of type g8e.operator.v1.FsReadRequested');
  }
  return Buffer.from(arg.serializeBinary());
}

function deserialize_g8e_operator_v1_FsReadRequested(buffer_arg) {
  return operator_pb.FsReadRequested.deserializeBinary(new Uint8Array(buffer_arg));
}


var OperatorServiceService = exports.OperatorServiceService = {
  // Execute a shell command
executeCommand: {
    path: '/g8e.operator.v1.OperatorService/ExecuteCommand',
    requestStream: false,
    responseStream: false,
    requestType: operator_pb.CommandRequested,
    responseType: operator_pb.CommandResult,
    requestSerialize: serialize_g8e_operator_v1_CommandRequested,
    requestDeserialize: deserialize_g8e_operator_v1_CommandRequested,
    responseSerialize: serialize_g8e_operator_v1_CommandResult,
    responseDeserialize: deserialize_g8e_operator_v1_CommandResult,
  },
  // Cancel a running command
cancelCommand: {
    path: '/g8e.operator.v1.OperatorService/CancelCommand',
    requestStream: false,
    responseStream: false,
    requestType: operator_pb.CommandCancelRequested,
    responseType: operator_pb.CommandResult,
    requestSerialize: serialize_g8e_operator_v1_CommandCancelRequested,
    requestDeserialize: deserialize_g8e_operator_v1_CommandCancelRequested,
    responseSerialize: serialize_g8e_operator_v1_CommandResult,
    responseDeserialize: deserialize_g8e_operator_v1_CommandResult,
  },
  // Edit a file
editFile: {
    path: '/g8e.operator.v1.OperatorService/EditFile',
    requestStream: false,
    responseStream: false,
    requestType: operator_pb.FileEditRequested,
    responseType: operator_pb.CommandResult,
    requestSerialize: serialize_g8e_operator_v1_FileEditRequested,
    requestDeserialize: deserialize_g8e_operator_v1_FileEditRequested,
    responseSerialize: serialize_g8e_operator_v1_CommandResult,
    responseDeserialize: deserialize_g8e_operator_v1_CommandResult,
  },
  // List directory contents
listFileSystem: {
    path: '/g8e.operator.v1.OperatorService/ListFileSystem',
    requestStream: false,
    responseStream: false,
    requestType: operator_pb.FsListRequested,
    responseType: operator_pb.CommandResult,
    requestSerialize: serialize_g8e_operator_v1_FsListRequested,
    requestDeserialize: deserialize_g8e_operator_v1_FsListRequested,
    responseSerialize: serialize_g8e_operator_v1_CommandResult,
    responseDeserialize: deserialize_g8e_operator_v1_CommandResult,
  },
  // Read file contents
readFileSystem: {
    path: '/g8e.operator.v1.OperatorService/ReadFileSystem',
    requestStream: false,
    responseStream: false,
    requestType: operator_pb.FsReadRequested,
    responseType: operator_pb.CommandResult,
    requestSerialize: serialize_g8e_operator_v1_FsReadRequested,
    requestDeserialize: deserialize_g8e_operator_v1_FsReadRequested,
    responseSerialize: serialize_g8e_operator_v1_CommandResult,
    responseDeserialize: deserialize_g8e_operator_v1_CommandResult,
  },
};

exports.OperatorServiceClient = grpc.makeGenericClientConstructor(OperatorServiceService);
