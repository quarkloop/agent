package secretssvc

import (
	"github.com/quarkloop/pkg/natskit"
	servicev1 "github.com/quarkloop/pkg/serviceapi/gen/quark/service/v1"
)

func Descriptor(skill *servicev1.SkillDescriptor) *servicev1.ServiceDescriptor {
	var skills []*servicev1.SkillDescriptor
	if skill != nil {
		skills = append(skills, skill)
	}
	serviceName := "quark.secrets.v1.SecretsService"
	return &servicev1.ServiceDescriptor{
		Name:    "secrets",
		Type:    "secrets",
		Version: "1.0.0",
		Rpcs: []*servicev1.RpcDescriptor{
			rpc(serviceName, "ResolveRef", "quark.secrets.v1.ResolveRefRequest", "quark.secrets.v1.ResolveRefResponse", "Resolve an OpenBao secret reference."),
			rpc(serviceName, "IssueScopedSecret", "quark.secrets.v1.IssueScopedSecretRequest", "quark.secrets.v1.IssueScopedSecretResponse", "Issue a scoped secret token."),
			rpc(serviceName, "RenewLease", "quark.secrets.v1.RenewLeaseRequest", "quark.secrets.v1.RenewLeaseResponse", "Renew a secret lease."),
			rpc(serviceName, "RevokeLease", "quark.secrets.v1.RevokeLeaseRequest", "quark.secrets.v1.RevokeLeaseResponse", "Revoke a secret lease."),
			rpc(serviceName, "RotateSecret", "quark.secrets.v1.RotateSecretRequest", "quark.secrets.v1.RotateSecretResponse", "Rotate a stored secret value."),
			rpc(serviceName, "AuditAccess", "quark.secrets.v1.AuditAccessRequest", "quark.secrets.v1.AuditAccessResponse", "Record a redacted secret access audit event."),
		},
		Skills: skills,
	}
}

func rpc(service, method, request, response, description string) *servicev1.RpcDescriptor {
	return natskit.MustServiceRPC("secrets", "secrets_"+method, service, method, request, response, description)
}
