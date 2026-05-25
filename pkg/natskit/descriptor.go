package natskit

import (
	"fmt"
	"strings"

	servicev1 "github.com/quarkloop/pkg/serviceapi/gen/quark/service/v1"
)

// MustServiceRPC builds a static protobuf-backed service function descriptor
// whose concrete NATS subject is derived by the same operation authority used
// during dispatch. Service descriptors are product configuration; an invalid
// literal is a programming error and fails during service construction.
func MustServiceRPC(owner, functionName, service, method, request, response, description string) *servicev1.RpcDescriptor {
	operation, err := ServiceOperationFromFunctionName(owner, functionName)
	if err != nil {
		panic(fmt.Sprintf("invalid service RPC %q: %v", functionName, err))
	}
	return &servicev1.RpcDescriptor{
		Service:      strings.TrimSpace(service),
		Method:       strings.TrimSpace(method),
		Request:      strings.TrimSpace(request),
		Response:     strings.TrimSpace(response),
		Description:  strings.TrimSpace(description),
		Owner:        operation.Owner,
		FunctionName: strings.TrimSpace(functionName),
		Subject:      operation.Subject,
	}
}

// MustStreamingServiceRPC is MustServiceRPC for server-streaming service
// functions transported over the NATS reply stream.
func MustStreamingServiceRPC(owner, functionName, service, method, request, response, description string) *servicev1.RpcDescriptor {
	rpc := MustServiceRPC(owner, functionName, service, method, request, response, description)
	rpc.Streaming = true
	return rpc
}
