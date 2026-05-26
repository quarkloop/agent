//go:build e2e

package e2e

import (
	"context"
	"testing"
	"time"

	"github.com/quarkloop/e2e/utils"
	"github.com/quarkloop/pkg/natskit"
	secretsv1 "github.com/quarkloop/pkg/serviceapi/gen/quark/secrets/v1"
	"github.com/quarkloop/supervisor/pkg/natshub"
)

func TestSecretsServiceNATSContract(t *testing.T) {
	env := utils.StartE2E(t, false, utils.StartOptions{
		DisableKnowledgeServices: true,
		Services:                 localServicePlugins("secrets"),
	})
	conn, err := natskit.Connect(context.Background(), natskit.Config{
		URL: env.NATS.ClientURL, Username: natshub.DefaultControlUser,
		Password: natshub.DefaultControlPassword, Name: "quark-e2e-secrets", Timeout: 5 * time.Second,
	})
	if err != nil {
		t.Fatalf("connect NATS: %v", err)
	}
	defer conn.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	const secretRef = "bao://secret/e2e/openrouter#api_key"
	var rotated secretsv1.RotateSecretResponse
	requestServiceFunction(t, ctx, conn, env.Space, "secrets", "rotate_secret", &secretsv1.RotateSecretRequest{
		SecretRef: secretRef,
		Field:     "api_key",
		Value:     "e2e-secret-material",
		Reason:    "NATS E2E contract verification",
	}, &rotated)
	if rotated.GetVersion() == 0 || rotated.GetAuditId() == "" {
		t.Fatalf("rotate response missing version/audit identity: %+v", &rotated)
	}
	var resolved secretsv1.ResolveRefResponse
	requestServiceFunction(t, ctx, conn, env.Space, "secrets", "resolve_ref", &secretsv1.ResolveRefRequest{
		SecretRef: secretRef,
		Field:     "api_key",
		Purpose:   "verify redacted service response",
		ActorId:   "e2e-test",
	}, &resolved)
	if resolved.GetSecret().GetValue() != "" || !resolved.GetSecret().GetValueRedacted() || resolved.GetAuditId() == "" {
		t.Fatalf("secret response was not redacted and audited: %+v", &resolved)
	}
}
