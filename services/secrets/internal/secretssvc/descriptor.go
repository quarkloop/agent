package secretssvc

import (
	secretsv1 "github.com/quarkloop/pkg/serviceapi/gen/quark/secrets/v1"
	servicev1 "github.com/quarkloop/pkg/serviceapi/gen/quark/service/v1"
)

func Descriptor(address string, skill *servicev1.SkillDescriptor) *servicev1.ServiceDescriptor {
	var skills []*servicev1.SkillDescriptor
	if skill != nil {
		skills = append(skills, skill)
	}
	serviceName := secretsv1.SecretsService_ServiceDesc.ServiceName
	return &servicev1.ServiceDescriptor{
		Name:    "secrets",
		Type:    "secrets",
		Version: "1.0.0",
		Address: address,
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
	return &servicev1.RpcDescriptor{
		Service:     service,
		Method:      method,
		Request:     request,
		Response:    response,
		Description: description,
	}
}
