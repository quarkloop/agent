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
	})
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
		AsyncError: func(err error) {
			permissionErrors <- err
		},
	})
	if err != nil {
		t.Fatalf("connect session client: %v", err)
	}
	defer client.Close()

	req, err := clientcontract.NewRequest("req-1", "docs", clientcontract.AuditListRequest{SpaceID: "docs"})
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()
	if _, err := client.Request(ctx, clientcontract.SubjectAuditList, req); err == nil {
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
	registerTypedControlResponders(t, responder, hub)

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
	credential, err := client.IssueSessionCredential(context.Background(), "docs", session.ID)
	if err != nil {
		t.Fatalf("issue session credential: %v", err)
	}
	if credential.Username == "" || credential.Password == "" || credential.SessionID != session.ID {
		t.Fatalf("credential = %#v", credential)
	}

	sessionClient, err := ConnectWithCredential(context.Background(), credential)
	if err != nil {
		t.Fatalf("connect with session credential: %v", err)
	}
	defer sessionClient.Close()

	spaceCredential, err := client.IssueSpaceCredential(context.Background(), "docs")
	if err != nil {
		t.Fatalf("issue space credential: %v", err)
	}
	if spaceCredential.Username == "" || spaceCredential.SpaceID != "docs" {
		t.Fatalf("space credential = %#v", spaceCredential)
	}
	runtimeCredential, err := client.IssueRuntimeCredential(context.Background(), "docs")
	if err != nil {
		t.Fatalf("issue runtime credential: %v", err)
	}
	if runtimeCredential.Username == "" || runtimeCredential.Role != "runtime" {
		t.Fatalf("runtime credential = %#v", runtimeCredential)
	}
	spaceClient, err := ConnectWithCredential(context.Background(), spaceCredential)
	if err != nil {
		t.Fatalf("connect with space credential: %v", err)
	}
	defer spaceClient.Close()

	spaceCredentials, err := hub.ProvisionSpace("docs")
	if err != nil {
		t.Fatalf("provision space: %v", err)
	}
	runtimeResponder := connectRaw(t, hub, spaceCredentials.Runtime.Username, spaceCredentials.Runtime.Password)
	registerRuntimeResponders(t, runtimeResponder)

	plan, err := spaceClient.RuntimePlan(context.Background(), "docs")
	if err != nil {
		t.Fatalf("runtime plan: %v", err)
	}
	if plan.Status != "idle" {
		t.Fatalf("plan = %#v", plan)
	}
	activity, err := spaceClient.RuntimeActivity(context.Background(), "docs", 10)
	if err != nil {
		t.Fatalf("runtime activity: %v", err)
	}
	if len(activity) != 1 || activity[0].Type != "message.user" {
		t.Fatalf("activity = %#v", activity)
	}

	events, errs, stop, err := sessionClient.SubscribeSessionEvents(context.Background(), session.ID)
	if err != nil {
		t.Fatalf("subscribe session events: %v", err)
	}
	defer stop()
	eventSubject, err := clientcontract.SessionEventsSubject(session.ID)
	if err != nil {
		t.Fatalf("session events subject: %v", err)
	}
	publishSessionEvent(t, runtimeResponder, eventSubject, clientcontract.SessionEvent{
		Type:      "token",
		SessionID: session.ID,
		Payload:   json.RawMessage(`"hello"`),
	})
	select {
	case event := <-events:
		if event.Type != "token" || string(event.Payload) != `"hello"` {
			t.Fatalf("session event = %#v", event)
		}
	case err := <-errs:
		t.Fatalf("session event error: %v", err)
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for session event")
	}

	registerSessionInputResponder(t, runtimeResponder, session.ID)
	ack, err := sessionClient.SendSessionMessage(context.Background(), clientcontract.SendMessageRequest{
		SpaceID:   "docs",
		SessionID: session.ID,
		Content:   "hello",
	})
	if err != nil {
		t.Fatalf("send session message: %v", err)
	}
	if !ack.Accepted || ack.SessionID != session.ID {
		t.Fatalf("message ack = %#v", ack)
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
	auditRecord, err := client.GetAuditRecord(context.Background(), "docs", "ref-1")
	if err != nil {
		t.Fatalf("audit get: %v", err)
	}
	if auditRecord.ReferenceID != "ref-1" || auditRecord.Service != "indexer" {
		t.Fatalf("audit record = %#v", auditRecord)
	}
	auditPage, err := client.ListAuditRecords(context.Background(), clientcontract.AuditListRequest{SpaceID: "docs", RunID: "run-1", Limit: 10})
	if err != nil {
		t.Fatalf("audit list: %v", err)
	}
	if len(auditPage.Records) != 1 || auditPage.NextCursor != 7 {
		t.Fatalf("audit page = %#v", auditPage)
	}
	retention, err := client.AuditRetention(context.Background())
	if err != nil {
		t.Fatalf("audit retention: %v", err)
	}
	if retention.MaxMessages != 1000 {
		t.Fatalf("audit retention = %#v", retention)
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

func registerTypedControlResponders(t *testing.T, responder *nats.Conn, hub *natshub.Hub) {
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
		clientcontract.SubjectSessionCredential: func(req clientcontract.RequestEnvelope) any {
			var payload clientcontract.SessionCredentialRequest
			if err := req.DecodePayload(&payload); err != nil {
				t.Errorf("decode session credential: %v", err)
			}
			credential, err := hub.IssueSessionCredential(payload.SpaceID, payload.SessionID)
			if err != nil {
				t.Errorf("issue session credential: %v", err)
			}
			return clientcontract.SessionCredentialResponse{Credential: clientcontract.NATSCredential{
				URL:       hub.Endpoints().ClientURL,
				Username:  credential.Username,
				Password:  credential.Password,
				Account:   credential.Account,
				Role:      string(credential.Role),
				SpaceID:   credential.SpaceID,
				SessionID: credential.SessionID,
			}}
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
		clientcontract.SubjectSpaceCredential: func(req clientcontract.RequestEnvelope) any {
			var payload clientcontract.SpaceCredentialRequest
			if err := req.DecodePayload(&payload); err != nil {
				t.Errorf("decode space credential: %v", err)
			}
			credential, err := hub.IssueUserCredential(payload.SpaceID)
			if err != nil {
				t.Errorf("issue space credential: %v", err)
			}
			return clientcontract.SpaceCredentialResponse{Credential: clientcontract.NATSCredential{
				URL:      hub.Endpoints().ClientURL,
				Username: credential.Username,
				Password: credential.Password,
				Account:  credential.Account,
				Role:     string(credential.Role),
				SpaceID:  credential.SpaceID,
			}}
		},
		clientcontract.SubjectRuntimeCredential: func(req clientcontract.RequestEnvelope) any {
			var payload clientcontract.SpaceCredentialRequest
			if err := req.DecodePayload(&payload); err != nil {
				t.Errorf("decode runtime credential: %v", err)
			}
			space, err := hub.ProvisionSpace(payload.SpaceID)
			if err != nil {
				t.Errorf("issue runtime credential: %v", err)
			}
			return clientcontract.SpaceCredentialResponse{Credential: clientcontract.NATSCredential{
				URL:      hub.Endpoints().ClientURL,
				Username: space.Runtime.Username,
				Password: space.Runtime.Password,
				Account:  space.Runtime.Account,
				Role:     string(space.Runtime.Role),
				SpaceID:  space.Runtime.SpaceID,
			}}
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
		clientcontract.SubjectAuditGet: func(req clientcontract.RequestEnvelope) any {
			var payload clientcontract.AuditGetRequest
			if err := req.DecodePayload(&payload); err != nil {
				t.Errorf("decode audit get: %v", err)
			}
			return clientcontract.AuditRecord{ReferenceID: payload.ReferenceID, SpaceID: payload.SpaceID, Service: "indexer", Function: "get_context", Status: "ok"}
		},
		clientcontract.SubjectAuditList: func(req clientcontract.RequestEnvelope) any {
			var payload clientcontract.AuditListRequest
			if err := req.DecodePayload(&payload); err != nil {
				t.Errorf("decode audit list: %v", err)
			}
			return clientcontract.AuditListResponse{Records: []clientcontract.AuditRecord{{Sequence: 7, SpaceID: payload.SpaceID, RunID: payload.RunID, ReferenceID: "ref-1"}}, NextCursor: 7}
		},
		clientcontract.SubjectAuditRetention: func(clientcontract.RequestEnvelope) any {
			return clientcontract.AuditRetentionResponse{MaxAgeSeconds: 3600, MaxMessages: 1000}
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

func registerRuntimeResponders(t *testing.T, responder *nats.Conn) {
	t.Helper()
	responders := map[string]func(clientcontract.RequestEnvelope) any{
		clientcontract.SubjectRuntimePlanGet: func(clientcontract.RequestEnvelope) any {
			return clientcontract.RuntimePlanResponse{Status: "idle", Complete: true}
		},
		clientcontract.SubjectRuntimeActivityList: func(clientcontract.RequestEnvelope) any {
			return clientcontract.RuntimeActivityListResponse{Records: []clientcontract.RuntimeActivityRecord{{
				ID:   "activity-1",
				Type: "message.user",
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
				t.Errorf("build runtime response: %v", err)
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

func registerSessionInputResponder(t *testing.T, responder *nats.Conn, sessionID string) {
	t.Helper()
	inputSubject, err := clientcontract.SessionInputSubject(sessionID)
	if err != nil {
		t.Fatalf("session input subject: %v", err)
	}
	if _, err := responder.Subscribe(inputSubject, func(msg *nats.Msg) {
		req := decodeRequest(t, msg.Data)
		var payload clientcontract.SendMessageRequest
		if err := req.DecodePayload(&payload); err != nil {
			t.Errorf("decode send message: %v", err)
		}
		resp, err := clientcontract.OK(req.RequestID, clientcontract.SendMessageResponse{
			SessionID: payload.SessionID,
			Accepted:  true,
		})
		if err != nil {
			t.Errorf("build send response: %v", err)
			return
		}
		data, _ := json.Marshal(resp)
		_ = msg.Respond(data)
	}); err != nil {
		t.Fatalf("subscribe %s: %v", inputSubject, err)
	}
	responder.Flush()
}

func publishSessionEvent(t *testing.T, conn *nats.Conn, subject string, event clientcontract.SessionEvent) {
	t.Helper()
	data, err := json.Marshal(event)
	if err != nil {
		t.Fatalf("marshal session event: %v", err)
	}
	if err := conn.Publish(subject, data); err != nil {
		t.Fatalf("publish session event: %v", err)
	}
	if err := conn.FlushTimeout(time.Second); err != nil {
		t.Fatalf("flush session event: %v", err)
	}
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
