package app

import (
	"github.com/quarkloop/pkg/natskit"
	servicev1 "github.com/quarkloop/pkg/serviceapi/gen/quark/service/v1"
	"github.com/quarkloop/services/gateway/internal/gatewaysvc"
)

func gatewayBinding(address string, skill *servicev1.SkillDescriptor, server *gatewaysvc.Server) natskit.Binding {
	return natskit.Binding{
		Descriptor: gatewaysvc.Descriptor(address, skill),
		Services: []natskit.RPCService{{
			Service:        "quark.gateway.v1.GatewayService",
			Implementation: server,
		}},
		Streams: []natskit.RPCStream{{
			Service: "quark.gateway.v1.GatewayService",
			Method:  "StreamGenerate",
			Handle:  gatewayStreamGenerate(server),
		}},
		Usage: gatewayUsageFromResponse,
	}
}
