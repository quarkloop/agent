package natsclient

import (
	"context"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/nats-io/nats.go"
	"github.com/quarkloop/pkg/serviceapi/clientcontract"
	"github.com/quarkloop/supervisor/pkg/natshub"
)

func TestClientRequestReply(t *testing.T) {
	hub := startHub(t)
	responder := connectRaw(t, hub, natshub.DefaultControlUser, natshub.DefaultControlPassword)
	if _, err := responder.Subscribe(clientcontract.SubjectSpaceList, func(msg *nats.Msg) {
		req := decodeRequest(t, msg.Data)
		resp, err := clientcontract.OK(req.RequestID, clientcontract.ListSpacesResponse{})
		if err != nil {
			t.Errorf("build response: %v", err)
			return
		}
		data, _ := json.Marshal(resp)
		_ = msg.Respond(data)
	}); err != nil {
		t.Fatalf("subscribe: %v", err)
	}
	responder.Flush()

	client, err := Connect(context.Background(), Config{
		URL:      hub.Endpoints().ClientURL,
		Username: natshub.DefaultControlUser,
		Password: natshub.DefaultControlPassword,
	})
	if err != nil {
		t.Fatalf("connect client: %v", err)
	}
	defer client.Close()

	req, err := clientcontract.NewRequest("req-1", "", struct{}{})
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	resp, err := client.Request(ctx, clientcontract.SubjectSpaceList, req)
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	var out clientcontract.ListSpacesResponse
	if err := resp.DecodePayload(&out); err != nil {
		t.Fatalf("decode response: %v", err)
	}
}

func TestClientRejectsInvalidCredentials(t *testing.T) {
	hub := startHub(t)
	_, err := Connect(context.Background(), Config{
		URL:           hub.Endpoints().ClientURL,
		Username:      "missing",
		Password:      "wrong",
		MaxReconnects: -1,
	}, nats.NoReconnect())
	if err == nil {
		t.Fatal("expected invalid credentials error")
	}
}

func TestClientReportsPermissionDeniedRequest(t *testing.T) {
	hub := startHub(t)
	sessionCredential, err := hub.IssueSessionCredential("docs", "chat")
	if err != nil {
		t.Fatalf("issue session credential: %v", err)
	}
	permissionErrors := make(chan error, 1)
	client, err := Connect(context.Background(), Config{
		URL:      hub.Endpoints().ClientURL,
		Username: sessionCredential.Username,
		Password: sessionCredential.Password,
		Timeout:  time.Second,
	}, nats.ErrorHandler(func(_ *nats.Conn, _ *nats.Subscription, err error) {
		permissionErrors <- err
	}))
	if err != nil {
		t.Fatalf("connect session client: %v", err)
	}
	defer client.Close()

	req, err := clientcontract.NewRequest("req-1", "docs", struct{}{})
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()
	if _, err := client.Request(ctx, clientcontract.SubjectSpaceList, req); err == nil {
		t.Fatal("expected denied request error")
	}
	select {
	case err := <-permissionErrors:
		if !strings.Contains(strings.ToLower(err.Error()), "permissions violation") {
			t.Fatalf("permission error = %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for permission error")
	}
}

func startHub(t *testing.T) *natshub.Hub {
	t.Helper()
	cfg := natshub.DefaultConfig(filepath.Join(t.TempDir(), "nats"))
	cfg.Client.Port = 0
	cfg.WebSocket.Enabled = false
	cfg.Monitoring.Enabled = false
	cfg.NoLog = true
	hub, err := natshub.New(cfg)
	if err != nil {
		t.Fatalf("new hub: %v", err)
	}
	if err := hub.Start(context.Background()); err != nil {
		t.Fatalf("start hub: %v", err)
	}
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = hub.Stop(ctx)
	})
	return hub
}

func connectRaw(t *testing.T, hub *natshub.Hub, user, password string) *nats.Conn {
	t.Helper()
	nc, err := nats.Connect(hub.Endpoints().ClientURL, nats.UserInfo(user, password), nats.Timeout(time.Second))
	if err != nil {
		t.Fatalf("connect raw nats: %v", err)
	}
	t.Cleanup(nc.Close)
	return nc
}

func decodeRequest(t *testing.T, data []byte) clientcontract.RequestEnvelope {
	t.Helper()
	var req clientcontract.RequestEnvelope
	if err := json.Unmarshal(data, &req); err != nil {
		t.Fatalf("decode request: %v", err)
	}
	if err := req.Validate(); err != nil {
		t.Fatalf("validate request: %v", err)
	}
	return req
}
