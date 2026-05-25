package app

import (
	"context"
	"fmt"

	"github.com/quarkloop/pkg/natskit"
	gatewayv1 "github.com/quarkloop/pkg/serviceapi/gen/quark/gateway/v1"
	"github.com/quarkloop/services/gateway/internal/gatewaysvc"
	"google.golang.org/protobuf/proto"
)

func gatewayStreamGenerate(server *gatewaysvc.Server) natskit.RPCStreamHandler {
	return func(ctx context.Context, input proto.Message, publish func(proto.Message) error) (proto.Message, error) {
		request, ok := input.(*gatewayv1.StreamGenerateRequest)
		if !ok {
			return nil, fmt.Errorf("gateway StreamGenerate request type %T is invalid", input)
		}
		events, err := server.StreamGenerateEvents(ctx, request)
		if err != nil {
			return nil, err
		}
		for event := range events {
			if event.Err != nil {
				return nil, event.Err
			}
			if event.Response.GetDone() {
				return event.Response, nil
			}
			if err := publish(event.Response); err != nil {
				return nil, err
			}
		}
		return nil, fmt.Errorf("gateway stream closed without completion")
	}
}
