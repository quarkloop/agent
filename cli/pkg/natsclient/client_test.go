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

func TestTypedControlMethodsUseNATSContracts(t *testing.T) {
	hub := startHub(t)
	responder := connectRaw(t, hub, natshub.DefaultControlUser, natshub.DefaultControlPassword)
	registerTypedControlResponders(t, responder)

	client, err := Connect(context.Background(), Config{
		URL:      hub.Endpoints().ClientURL,
		Username: natshub.DefaultControlUser,
		Password: natshub.DefaultControlPassword,
	})
	if err != nil {
		t.Fatalf("connect client: %v", err)
	}
	defer client.Close()

	space, err := client.CreateSpace(context.Background(), clientcontract.CreateSpaceRequest{
		Name:       "docs",
		Quarkfile:  []byte("meta:\n  name: docs\n"),
		WorkingDir: filepath.Join(t.TempDir(), "workspace"),
	})
	if err != nil {
		t.Fatalf("create space: %v", err)
	}
	if space.Name != "docs" {
		t.Fatalf("space = %#v", space)
	}

	updated, err := client.UpdateSpace(context.Background(), "docs", []byte("meta:\n  name: docs\n"))
	if err != nil {
		t.Fatalf("update space: %v", err)
	}
	if updated.Name != "docs" {
		t.Fatalf("updated space = %#v", updated)
	}

	session, err := client.CreateSession(context.Background(), clientcontract.CreateSessionRequest{
		SpaceID: "docs",
		Type:    clientcontract.SessionTypeChat,
		Title:   "nats",
	})
	if err != nil {
		t.Fatalf("create session: %v", err)
	}
	if session.ID == "" {
		t.Fatal("session id is empty")
	}

	list, err := client.ListSessions(context.Background(), "docs")
	if err != nil {
		t.Fatalf("list sessions: %v", err)
	}
	if len(list.Sessions) != 1 || list.Sessions[0].ID != session.ID {
		t.Fatalf("sessions = %#v", list.Sessions)
	}

	if err := client.DeleteSession(context.Background(), "docs", session.ID); err != nil {
		t.Fatalf("delete session: %v", err)
	}

	if err := client.KBSet(context.Background(), "docs", "config", "model", []byte("openrouter")); err != nil {
		t.Fatalf("kb set: %v", err)
	}
	value, err := client.KBGet(context.Background(), "docs", "config", "model")
	if err != nil {
		t.Fatalf("kb get: %v", err)
	}
	if string(value) != "openrouter" {
		t.Fatalf("kb value = %q", value)
	}
	keys, err := client.KBList(context.Background(), "docs", "config")
	if err != nil {
		t.Fatalf("kb list: %v", err)
	}
	if len(keys) != 1 || keys[0] != "model" {
		t.Fatalf("kb keys = %#v", keys)
	}
	if err := client.KBDelete(context.Background(), "docs", "config", "model"); err != nil {
		t.Fatalf("kb delete: %v", err)
	}

	doctor, err := client.Doctor(context.Background(), "docs")
	if err != nil {
		t.Fatalf("doctor: %v", err)
	}
	if !doctor.OK {
		t.Fatalf("doctor = %#v", doctor)
	}

	plugins, err := client.ListPlugins(context.Background(), "docs", "service")
	if err != nil {
		t.Fatalf("plugin list: %v", err)
	}
	if len(plugins) != 1 || plugins[0].Name != "io" {
		t.Fatalf("plugins = %#v", plugins)
	}
	installed, err := client.InstallPlugin(context.Background(), "docs", "plugins/services/io")
	if err != nil {
		t.Fatalf("plugin install: %v", err)
	}
	if installed.Name != "io" {
		t.Fatalf("installed = %#v", installed)
	}

	services, err := client.ListServices(context.Background(), "docs")
	if err != nil {
		t.Fatalf("service list: %v", err)
	}
	if len(services) != 1 || services[0].Name != "indexer" {
		t.Fatalf("services = %#v", services)
	}
	service, err := client.InspectService(context.Background(), "docs", "indexer")
	if err != nil {
		t.Fatalf("service inspect: %v", err)
	}
	if service.Status != clientcontract.ServiceStatusReady {
		t.Fatalf("service = %#v", service)
	}
	serviceDoctor, err := client.ServiceDoctor(context.Background(), "docs")
	if err != nil {
		t.Fatalf("service doctor: %v", err)
	}
	if len(serviceDoctor.Services) != 1 {
		t.Fatalf("service doctor = %#v", serviceDoctor)
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

func registerTypedControlResponders(t *testing.T, responder *nats.Conn) {
	t.Helper()
	sessionID := "session-1"
	responders := map[string]func(clientcontract.RequestEnvelope) any{
		clientcontract.SubjectSpaceCreate: func(req clientcontract.RequestEnvelope) any {
			var payload clientcontract.CreateSpaceRequest
			if err := req.DecodePayload(&payload); err != nil {
				t.Errorf("decode create space: %v", err)
			}
			return clientcontract.SpaceInfo{Name: payload.Name}
		},
		clientcontract.SubjectSpaceUpdate: func(req clientcontract.RequestEnvelope) any {
			var payload clientcontract.UpdateSpaceRequest
			if err := req.DecodePayload(&payload); err != nil {
				t.Errorf("decode update space: %v", err)
			}
			return clientcontract.SpaceInfo{Name: payload.Name}
		},
		clientcontract.SubjectSessionCreate: func(req clientcontract.RequestEnvelope) any {
			var payload clientcontract.CreateSessionRequest
			if err := req.DecodePayload(&payload); err != nil {
				t.Errorf("decode create session: %v", err)
			}
			return clientcontract.SessionInfo{ID: sessionID, Type: payload.Type, Title: payload.Title}
		},
		clientcontract.SubjectSessionList: func(clientcontract.RequestEnvelope) any {
			return clientcontract.ListSessionsResponse{Sessions: []clientcontract.SessionInfo{{ID: sessionID, Type: clientcontract.SessionTypeChat}}}
		},
		clientcontract.SubjectSessionDelete: func(clientcontract.RequestEnvelope) any {
			return struct{}{}
		},
		clientcontract.SubjectKBSet: func(req clientcontract.RequestEnvelope) any {
			var payload clientcontract.KBSetRequest
			if err := req.DecodePayload(&payload); err != nil {
				t.Errorf("decode kb set: %v", err)
			}
			return struct{}{}
		},
		clientcontract.SubjectKBGet: func(req clientcontract.RequestEnvelope) any {
			var payload clientcontract.KBRefRequest
			if err := req.DecodePayload(&payload); err != nil {
				t.Errorf("decode kb get: %v", err)
			}
			return clientcontract.KBValueResponse{Value: []byte("openrouter")}
		},
		clientcontract.SubjectKBList: func(clientcontract.RequestEnvelope) any {
			return clientcontract.KBListResponse{Keys: []string{"model"}}
		},
		clientcontract.SubjectKBDelete: func(clientcontract.RequestEnvelope) any {
			return struct{}{}
		},
		clientcontract.SubjectSpaceDoctor: func(clientcontract.RequestEnvelope) any {
			return clientcontract.DoctorResponse{OK: true}
		},
		clientcontract.SubjectPluginList: func(clientcontract.RequestEnvelope) any {
			return clientcontract.ListPluginsResponse{Plugins: []clientcontract.PluginInfo{{
				Name: "io", Type: "service", Version: "1.0.0",
			}}}
		},
		clientcontract.SubjectPluginInstall: func(req clientcontract.RequestEnvelope) any {
			var payload clientcontract.InstallPluginRequest
			if err := req.DecodePayload(&payload); err != nil {
				t.Errorf("decode plugin install: %v", err)
			}
			return clientcontract.InstallPluginResponse{Plugin: clientcontract.PluginInfo{
				Name: "io", Type: "service", Version: "1.0.0",
			}}
		},
		clientcontract.SubjectServiceList: func(clientcontract.RequestEnvelope) any {
			return clientcontract.ListServicesResponse{Services: []clientcontract.ServiceInfo{{
				Name: "indexer", Status: clientcontract.ServiceStatusReady, Version: "1.0.0",
			}}}
		},
		clientcontract.SubjectServiceInspect: func(req clientcontract.RequestEnvelope) any {
			var payload clientcontract.InspectServiceRequest
			if err := req.DecodePayload(&payload); err != nil {
				t.Errorf("decode service inspect: %v", err)
			}
			return clientcontract.ServiceInfo{Name: payload.Service, Status: clientcontract.ServiceStatusReady, Version: "1.0.0"}
		},
		clientcontract.SubjectServiceDoctor: func(clientcontract.RequestEnvelope) any {
			return clientcontract.ServiceDoctorResponse{Services: []clientcontract.ServiceInfo{{
				Name: "indexer", Status: clientcontract.ServiceStatusReady, Version: "1.0.0",
			}}}
		},
	}
	for subject, buildPayload := range responders {
		subject := subject
		buildPayload := buildPayload
		if _, err := responder.Subscribe(subject, func(msg *nats.Msg) {
			req := decodeRequest(t, msg.Data)
			resp, err := clientcontract.OK(req.RequestID, buildPayload(req))
			if err != nil {
				t.Errorf("build response: %v", err)
				return
			}
			data, _ := json.Marshal(resp)
			_ = msg.Respond(data)
		}); err != nil {
			t.Fatalf("subscribe %s: %v", subject, err)
		}
	}
	responder.Flush()
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
