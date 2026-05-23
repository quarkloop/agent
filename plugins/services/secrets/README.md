# Secrets Service Plugin

Secrets declares OpenBao-backed service functions for secret references, scoped
tokens, leases, rotation, and redacted access audit.

## Service Functions

| Function | NATS subject | RPC method | Risk | Approval | Purpose |
| --- | --- | --- | --- | --- | --- |
| `secrets_ResolveRef` | `svc.secrets.v1.resolve_ref` | `quark.secrets.v1.SecretsService/ResolveRef` | admin | yes | Resolve an OpenBao secret reference. |
| `secrets_IssueScopedSecret` | `svc.secrets.v1.issue_scoped_secret` | `quark.secrets.v1.SecretsService/IssueScopedSecret` | admin | yes | Issue a scoped secret token. |
| `secrets_RenewLease` | `svc.secrets.v1.renew_lease` | `quark.secrets.v1.SecretsService/RenewLease` | write | no | Renew an OpenBao lease. |
| `secrets_RevokeLease` | `svc.secrets.v1.revoke_lease` | `quark.secrets.v1.SecretsService/RevokeLease` | write | yes | Revoke an OpenBao lease. |
| `secrets_RotateSecret` | `svc.secrets.v1.rotate_secret` | `quark.secrets.v1.SecretsService/RotateSecret` | admin | yes | Rotate a KV v2 secret value. |
| `secrets_AuditAccess` | `svc.secrets.v1.audit_access` | `quark.secrets.v1.SecretsService/AuditAccess` | write | no | Record a redacted secret access event. |

## Reference Format

Secrets uses `bao://<mount>/<path>#<field>` references. The default local mount
is `secret`; OpenBao KV v2 paths are resolved under `/<mount>/data/<path>`.

## Redaction

Secret values, scoped tokens, accessors, and lease IDs are sensitive. Service
logs and service-function diagnostics must use the standard redaction boundary
before persistence or user display.
