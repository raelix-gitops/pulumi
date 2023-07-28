# Generated by the gRPC Python protocol compiler plugin. DO NOT EDIT!
"""Client and server classes corresponding to protobuf-defined services."""
import grpc

from pulumi.codegen import loader_pb2 as pulumi_dot_codegen_dot_loader__pb2


class LoaderStub(object):
    """Loader is a service for getting schemas from the Pulumi engine for use in code generators and other tools.
    This is currently unstable and experimental.
    """

    def __init__(self, channel):
        """Constructor.

        Args:
            channel: A grpc.Channel.
        """
        self.GetSchema = channel.unary_unary(
                '/codegen.Loader/GetSchema',
                request_serializer=pulumi_dot_codegen_dot_loader__pb2.GetSchemaRequest.SerializeToString,
                response_deserializer=pulumi_dot_codegen_dot_loader__pb2.GetSchemaResponse.FromString,
                )


class LoaderServicer(object):
    """Loader is a service for getting schemas from the Pulumi engine for use in code generators and other tools.
    This is currently unstable and experimental.
    """

    def GetSchema(self, request, context):
        """GetSchema tries to find a schema for the given package and version.
        """
        context.set_code(grpc.StatusCode.UNIMPLEMENTED)
        context.set_details('Method not implemented!')
        raise NotImplementedError('Method not implemented!')


def add_LoaderServicer_to_server(servicer, server):
    rpc_method_handlers = {
            'GetSchema': grpc.unary_unary_rpc_method_handler(
                    servicer.GetSchema,
                    request_deserializer=pulumi_dot_codegen_dot_loader__pb2.GetSchemaRequest.FromString,
                    response_serializer=pulumi_dot_codegen_dot_loader__pb2.GetSchemaResponse.SerializeToString,
            ),
    }
    generic_handler = grpc.method_handlers_generic_handler(
            'codegen.Loader', rpc_method_handlers)
    server.add_generic_rpc_handlers((generic_handler,))


 # This class is part of an EXPERIMENTAL API.
class Loader(object):
    """Loader is a service for getting schemas from the Pulumi engine for use in code generators and other tools.
    This is currently unstable and experimental.
    """

    @staticmethod
    def GetSchema(request,
            target,
            options=(),
            channel_credentials=None,
            call_credentials=None,
            insecure=False,
            compression=None,
            wait_for_ready=None,
            timeout=None,
            metadata=None):
        return grpc.experimental.unary_unary(request, target, '/codegen.Loader/GetSchema',
            pulumi_dot_codegen_dot_loader__pb2.GetSchemaRequest.SerializeToString,
            pulumi_dot_codegen_dot_loader__pb2.GetSchemaResponse.FromString,
            options, channel_credentials,
            insecure, call_credentials, compression, wait_for_ready, timeout, metadata)
