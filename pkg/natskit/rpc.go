package natskit

import (
	"context"
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
	"time"

	servicev1 "github.com/quarkloop/pkg/serviceapi/gen/quark/service/v1"
	"github.com/quarkloop/pkg/serviceapi/servicekit"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/reflect/protoregistry"
)

type Binding struct {
	Descriptor *servicev1.ServiceDescriptor
	Services   []RPCService
}

type RPCService struct {
	Service        string
	Implementation any
}

func RunRPCService(ctx context.Context, cfg Config, queue string, bindings ...Binding) error {
	host, err := StartRPCService(ctx, cfg, queue, bindings...)
	if err != nil {
		return err
	}
	defer host.Close()
	<-ctx.Done()
	return nil
}

func StartRPCService(ctx context.Context, cfg Config, queue string, bindings ...Binding) (*Host, error) {
	if strings.TrimSpace(cfg.URL) == "" {
		return nil, fmt.Errorf("nats url is required")
	}
	host, err := NewHost(ctx, cfg, queue)
	if err != nil {
		return nil, err
	}
	for _, binding := range bindings {
		if err := registerBinding(host, binding, cfg.Timeout); err != nil {
			host.Close()
			return nil, err
		}
	}
	if err := host.Ready(ctx); err != nil {
		host.Close()
		return nil, err
	}
	cfg = normalizeConfig(cfg)
	cfg.Logger.Info("nats service operations ready", "url", cfg.URL, "queue", queue)
	return host, nil
}

func registerBinding(host *Host, binding Binding, fallback time.Duration) error {
	if binding.Descriptor == nil {
		return fmt.Errorf("service descriptor is required")
	}
	descriptor := servicekit.CloneDescriptor(binding.Descriptor)
	methods := serviceMethods(binding.Services)
	for _, rpc := range descriptor.GetRpcs() {
		if rpc.GetStreaming() {
			continue
		}
		method, err := methodBindingFor(methods, rpc)
		if err != nil {
			return err
		}
		operation, err := operationForRPC(descriptor, rpc)
		if err != nil {
			return err
		}
		boundMethod := method
		if err := host.RegisterUnary(operation, serviceTimeout(rpc, fallback), func(ctx context.Context, req RequestEnvelope) (ResponseEnvelope, error) {
			request, err := newRequestMessage(boundMethod.Request, req.Payload)
			if err != nil {
				return ResponseEnvelope{}, err
			}
			response, err := callUnaryMethod(ctx, boundMethod.Implementation, boundMethod.Method, request)
			if err != nil {
				return ResponseEnvelope{}, err
			}
			payload, err := protojson.MarshalOptions{UseProtoNames: false}.Marshal(response)
			if err != nil {
				return ResponseEnvelope{}, err
			}
			return OKResponse(req.ServiceCallID, payload), nil
		}); err != nil {
			return err
		}
	}
	return nil
}

type methodBinding struct {
	Service        string
	Method         string
	Request        protoreflect.FullName
	Implementation any
}

func serviceMethods(services []RPCService) map[string]methodBinding {
	out := make(map[string]methodBinding)
	for _, service := range services {
		if strings.TrimSpace(service.Service) != "" && service.Implementation != nil {
			out[serviceMethodKey(service.Service, "*")] = methodBinding{Service: strings.TrimSpace(service.Service), Implementation: service.Implementation}
		}
	}
	return out
}

func methodBindingFor(methods map[string]methodBinding, rpc *servicev1.RpcDescriptor) (methodBinding, error) {
	if rpc == nil {
		return methodBinding{}, fmt.Errorf("rpc descriptor is required")
	}
	method, ok := methods[serviceMethodKey(rpc.GetService(), rpc.GetMethod())]
	if !ok {
		method, ok = methods[serviceMethodKey(rpc.GetService(), "*")]
	}
	if !ok || method.Implementation == nil {
		return methodBinding{}, fmt.Errorf("missing implementation for %s/%s", rpc.GetService(), rpc.GetMethod())
	}
	method.Method = strings.TrimSpace(rpc.GetMethod())
	method.Request = protoreflect.FullName(strings.TrimSpace(rpc.GetRequest()))
	if method.Method == "" || method.Request == "" {
		return methodBinding{}, fmt.Errorf("rpc method and request are required for %s", rpc.GetService())
	}
	return method, nil
}

func operationForRPC(desc *servicev1.ServiceDescriptor, rpc *servicev1.RpcDescriptor) (Operation, error) {
	owner := strings.TrimSpace(rpc.GetOwner())
	if owner == "" && desc != nil {
		owner = desc.GetName()
	}
	function := strings.TrimSpace(rpc.GetFunctionName())
	if function == "" {
		function = rpc.GetMethod()
	}
	return ServiceOperationFromFunctionName(owner, function)
}

func newRequestMessage(name protoreflect.FullName, payload json.RawMessage) (proto.Message, error) {
	messageType, err := protoregistry.GlobalTypes.FindMessageByName(name)
	if err != nil {
		return nil, fmt.Errorf("resolve protobuf request %s: %w", name, err)
	}
	message := messageType.New().Interface()
	if err := (protojson.UnmarshalOptions{DiscardUnknown: true}).Unmarshal(payload, message); err != nil {
		return nil, fmt.Errorf("decode protobuf request %s: %w", name, err)
	}
	return message, nil
}

func callUnaryMethod(ctx context.Context, implementation any, methodName string, req proto.Message) (proto.Message, error) {
	method := reflect.ValueOf(implementation).MethodByName(methodName)
	if !method.IsValid() {
		return nil, fmt.Errorf("service implementation does not expose %s", methodName)
	}
	methodType := method.Type()
	reqValue := reflect.ValueOf(req)
	if methodType.NumIn() != 2 || methodType.NumOut() != 2 || !reqValue.Type().AssignableTo(methodType.In(1)) {
		return nil, fmt.Errorf("service method %s has incompatible unary signature", methodName)
	}
	results := method.Call([]reflect.Value{reflect.ValueOf(ctx), reqValue})
	if !results[1].IsNil() {
		return nil, results[1].Interface().(error)
	}
	response, ok := results[0].Interface().(proto.Message)
	if !ok {
		return nil, fmt.Errorf("service method %s response is not protobuf message", methodName)
	}
	return response, nil
}

func serviceMethodKey(service, method string) string {
	return strings.TrimSpace(service) + "/" + strings.TrimSpace(method)
}

func serviceTimeout(rpc *servicev1.RpcDescriptor, fallback time.Duration) time.Duration {
	if rpc != nil && rpc.GetTimeoutMillis() > 0 {
		return time.Duration(rpc.GetTimeoutMillis()) * time.Millisecond
	}
	if fallback > 0 {
		return fallback
	}
	return DefaultTimeout
}
