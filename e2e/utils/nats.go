//go:build e2e

package utils

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"github.com/nats-io/nats.go"
	"github.com/quarkloop/pkg/serviceapi/clientcontract"
	"github.com/quarkloop/supervisor/pkg/natshub"
)

func issueRuntimeCredential(t *testing.T, endpoints NATSEndpoints, spaceID string) clientcontract.NATSCredential {
	t.Helper()
	return issueSpaceScopedCredential(t, endpoints, clientcontract.SubjectRuntimeCredential, spaceID)
}

func issueSessionCredential(t *testing.T, endpoints NATSEndpoints, spaceID, sessionID string) clientcontract.NATSCredential {
	t.Helper()
	control := connectControlNATS(t, endpoints)
	defer control.Close()
	resp := requestNATSPayload[clientcontract.SessionCredentialResponse](t, control, clientcontract.SubjectSessionCredential, spaceID, clientcontract.SessionCredentialRequest{
		SpaceID:   spaceID,
		SessionID: sessionID,
	})
	credential := resp.Credential
	if credential.URL == "" {
		credential.URL = endpoints.ClientURL
	}
	return credential
}

func issueSpaceScopedCredential(t *testing.T, endpoints NATSEndpoints, subject, spaceID string) clientcontract.NATSCredential {
	t.Helper()
	control := connectControlNATS(t, endpoints)
	defer control.Close()
	resp := requestNATSPayload[clientcontract.SpaceCredentialResponse](t, control, subject, spaceID, clientcontract.SpaceCredentialRequest{SpaceID: spaceID})
	credential := resp.Credential
	if credential.URL == "" {
		credential.URL = endpoints.ClientURL
	}
	return credential
}

func connectControlNATS(t *testing.T, endpoints NATSEndpoints) *nats.Conn {
	t.Helper()
	conn, err := nats.Connect(
		endpoints.ClientURL,
		nats.UserInfo(natshub.DefaultControlUser, natshub.DefaultControlPassword),
		nats.Name("quark-e2e-control"),
		nats.Timeout(5*time.Second),
	)
	if err != nil {
		t.Fatalf("connect control nats: %v", err)
	}
	return conn
}

func connectNATSCredential(t *testing.T, credential clientcontract.NATSCredential) *nats.Conn {
	t.Helper()
	conn, err := nats.Connect(
		credential.URL,
		nats.UserInfo(credential.Username, credential.Password),
		nats.Name("quark-e2e-"+credential.Role),
		nats.Timeout(5*time.Second),
		nats.ReconnectWait(250*time.Millisecond),
		nats.MaxReconnects(10),
	)
	if err != nil {
		t.Fatalf("connect scoped nats credential: %v", err)
	}
	return conn
}

func requestNATSPayload[T any](t *testing.T, conn *nats.Conn, subject, spaceID string, payload any) T {
	t.Helper()
	out, err := tryRequestNATSPayload[T](conn, subject, spaceID, payload, 10*time.Second)
	if err != nil {
		t.Fatalf("%v", err)
	}
	return out
}

func tryRequestNATSPayload[T any](conn *nats.Conn, subject, spaceID string, payload any, timeout time.Duration) (T, error) {
	var out T
	req, err := clientcontract.NewRequest("e2e-"+subject, spaceID, payload)
	if err != nil {
		return out, fmt.Errorf("new nats request: %w", err)
	}
	data, err := json.Marshal(req)
	if err != nil {
		return out, fmt.Errorf("marshal nats request: %w", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	reply, err := conn.RequestWithContext(ctx, subject, data)
	if err != nil {
		return out, fmt.Errorf("nats request %s: %w", subject, err)
	}
	var resp clientcontract.ResponseEnvelope
	if err := json.Unmarshal(reply.Data, &resp); err != nil {
		return out, fmt.Errorf("decode nats response: %w", err)
	}
	if resp.Status != "ok" {
		if resp.Error != nil {
			return out, fmt.Errorf("nats response %s failed: %s: %s", subject, resp.Error.Category, resp.Error.Message)
		}
		return out, fmt.Errorf("nats response %s failed: %#v", subject, resp)
	}
	if err := resp.DecodePayload(&out); err != nil {
		return out, fmt.Errorf("decode nats payload %s: %w", subject, err)
	}
	return out, nil
}

func requestNATSMessage(ctx context.Context, conn *nats.Conn, subject string, req clientcontract.RequestEnvelope) error {
	data, err := json.Marshal(req)
	if err != nil {
		return fmt.Errorf("marshal nats request: %w", err)
	}
	reply, err := conn.RequestWithContext(ctx, subject, data)
	if err != nil {
		return fmt.Errorf("nats request %s: %w", subject, err)
	}
	var resp clientcontract.ResponseEnvelope
	if err := json.Unmarshal(reply.Data, &resp); err != nil {
		return fmt.Errorf("decode nats response: %w", err)
	}
	if resp.Status == "error" {
		if resp.Error == nil {
			return fmt.Errorf("nats response %s failed without error payload", subject)
		}
		return fmt.Errorf("nats response %s failed: %s: %s", subject, resp.Error.Category, resp.Error.Message)
	}
	return resp.Validate()
}
