package spaceauth

import (
	"testing"
)

func TestResolverFromEnvUsesSingleRuntimeCredential(t *testing.T) {
	t.Setenv(EnvNATSURL, "nats://127.0.0.1:4222")
	t.Setenv(EnvNATSUser, "runtime")
	t.Setenv(EnvNATSPassword, "secret")

	resolver, err := ResolverFromEnv()
	if err != nil {
		t.Fatalf("resolver from env: %v", err)
	}
	credential, err := resolver.Resolve("space-a")
	if err != nil {
		t.Fatalf("resolve credential: %v", err)
	}
	if credential.SpaceID != "space-a" || credential.Username != "runtime" || credential.Password != "secret" {
		t.Fatalf("credential = %+v", credential)
	}
}

func TestResolverFromEnvUsesPerSpaceCredentialMap(t *testing.T) {
	t.Setenv(EnvNATSURL, "nats://127.0.0.1:4222")
	t.Setenv(EnvNATSUser, "fallback")
	t.Setenv(EnvRuntimeSpaceCredentials, `{
		"space-a": {"username":"runtime-a","password":"secret-a"},
		"space-b": {"url":"nats://example:4222","username":"runtime-b","password":"secret-b","space_id":"space-b"}
	}`)

	resolver, err := ResolverFromEnv()
	if err != nil {
		t.Fatalf("resolver from env: %v", err)
	}
	if !resolver.HasExplicitCredentials() {
		t.Fatal("expected explicit credentials")
	}
	first, err := resolver.Resolve("space-a")
	if err != nil {
		t.Fatalf("resolve space-a: %v", err)
	}
	if first.URL != "nats://127.0.0.1:4222" || first.Username != "runtime-a" || first.Password != "secret-a" {
		t.Fatalf("space-a credential = %+v", first)
	}
	second, err := resolver.Resolve("space-b")
	if err != nil {
		t.Fatalf("resolve space-b: %v", err)
	}
	if second.URL != "nats://example:4222" || second.Username != "runtime-b" {
		t.Fatalf("space-b credential = %+v", second)
	}
}

func TestResolverFromEnvUsesPerSpaceCredentialList(t *testing.T) {
	t.Setenv(EnvRuntimeSpaceCredentials, `[
		{"url":"nats://127.0.0.1:4222","username":"runtime-a","space_id":"space-a"}
	]`)

	resolver, err := ResolverFromEnv()
	if err != nil {
		t.Fatalf("resolver from env: %v", err)
	}
	credential, err := resolver.Resolve("space-a")
	if err != nil {
		t.Fatalf("resolve credential: %v", err)
	}
	if credential.Username != "runtime-a" {
		t.Fatalf("credential = %+v", credential)
	}
}

func TestResolverRejectsMissingCredentialFields(t *testing.T) {
	t.Setenv(EnvNATSURL, "nats://127.0.0.1:4222")

	resolver, err := ResolverFromEnv()
	if err != nil {
		t.Fatalf("resolver from env: %v", err)
	}
	if _, err := resolver.Resolve("space-a"); err == nil {
		t.Fatal("expected missing username error")
	}
}
