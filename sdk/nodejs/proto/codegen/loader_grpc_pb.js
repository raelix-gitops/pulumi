// GENERATED CODE -- DO NOT EDIT!

// Original file comments:
// Copyright 2016-2022, Pulumi Corporation.
//
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
//
'use strict';
var grpc = require('grpc');
var pulumi_codegen_loader_pb = require('../../pulumi/codegen/loader_pb.js');
var google_protobuf_empty_pb = require('google-protobuf/google/protobuf/empty_pb.js');

function serialize_pulumirpc_codegen_GetSchemaRequest(arg) {
  if (!(arg instanceof pulumi_codegen_loader_pb.GetSchemaRequest)) {
    throw new Error('Expected argument of type pulumirpc.codegen.GetSchemaRequest');
  }
  return Buffer.from(arg.serializeBinary());
}

function deserialize_pulumirpc_codegen_GetSchemaRequest(buffer_arg) {
  return pulumi_codegen_loader_pb.GetSchemaRequest.deserializeBinary(new Uint8Array(buffer_arg));
}

function serialize_pulumirpc_codegen_GetSchemaResponse(arg) {
  if (!(arg instanceof pulumi_codegen_loader_pb.GetSchemaResponse)) {
    throw new Error('Expected argument of type pulumirpc.codegen.GetSchemaResponse');
  }
  return Buffer.from(arg.serializeBinary());
}

function deserialize_pulumirpc_codegen_GetSchemaResponse(buffer_arg) {
  return pulumi_codegen_loader_pb.GetSchemaResponse.deserializeBinary(new Uint8Array(buffer_arg));
}


var LoaderService = exports.LoaderService = {
  getSchema: {
    path: '/pulumirpc.codegen.Loader/GetSchema',
    requestStream: false,
    responseStream: false,
    requestType: pulumi_codegen_loader_pb.GetSchemaRequest,
    responseType: pulumi_codegen_loader_pb.GetSchemaResponse,
    requestSerialize: serialize_pulumirpc_codegen_GetSchemaRequest,
    requestDeserialize: deserialize_pulumirpc_codegen_GetSchemaRequest,
    responseSerialize: serialize_pulumirpc_codegen_GetSchemaResponse,
    responseDeserialize: deserialize_pulumirpc_codegen_GetSchemaResponse,
  },
};

exports.LoaderClient = grpc.makeGenericClientConstructor(LoaderService);
