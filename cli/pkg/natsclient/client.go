package natsclient

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/nats-io/nats.go"
	"github.com/quarkloop/pkg/boundary"
	"github.com/quarkloop/pkg/serviceapi/clientcontract"
)

const (
	EnvURL      = "QUARK_NATS_URL"
	EnvUser     = "QUARK_NATS_USER"
	EnvPassword = "QUARK_NATS_PASSWORD"

	DefaultURL      = "nats://127.0.0.1:4222"
	DefaultUser     = "quark-control"
	DefaultPassword = "quark-control-dev"
)

type Config struct {
	URL           string
	Username      string
	Password      string
	Name          string
	Timeout       time.Duration
	ReconnectWait time.Duration
	MaxReconnects int
}

func ConfigFromEnv() Config {
	return Config{
		URL:           firstNonEmpty(os.Getenv(EnvURL), DefaultURL),
		Username:      firstNonEmpty(os.Getenv(EnvUser), DefaultUser),
		Password:      firstNonEmpty(os.Getenv(EnvPassword), DefaultPassword),
		Name:          "quark-cli",
		Timeout:       5 * time.Second,
		ReconnectWait: 250 * time.Millisecond,
		MaxReconnects: 10,
	}
}

type Client struct {
	conn *nats.Conn
}

type ResponseError struct {
	Category boundary.Category
	Message  string
}

func (e *ResponseError) Error() string {
	if e == nil {
		return ""
	}
	if e.Category == "" {
		return e.Message
	}
	return string(e.Category) + ": " + e.Message
}

func IsNotFound(err error) bool {
	return responseHasCategory(err, boundary.NotFound)
}

func IsConflict(err error) bool {
	return responseHasCategory(err, boundary.Conflict)
}

func Connect(ctx context.Context, cfg Config, opts ...nats.Option) (*Client, error) {
	normalized := normalizeConfig(cfg)
	options := []nats.Option{
		nats.Name(normalized.Name),
		nats.Timeout(normalized.Timeout),
		nats.ReconnectWait(normalized.ReconnectWait),
		nats.MaxReconnects(normalized.MaxReconnects),
		nats.RetryOnFailedConnect(true),
	}
	if normalized.Username != "" || normalized.Password != "" {
		options = append(options, nats.UserInfo(normalized.Username, normalized.Password))
	}
	options = append(options, opts...)

	if err := ctx.Err(); err != nil {
		return nil, ctx.Err()
	}
	conn, err := nats.Connect(normalized.URL, options...)
	if err != nil {
		return nil, fmt.Errorf("connect nats: %w", err)
	}
	if err := ctx.Err(); err != nil {
		conn.Close()
		return nil, err
	}
	if err := conn.FlushTimeout(normalized.Timeout); err != nil {
		conn.Close()
		return nil, fmt.Errorf("verify nats connection: %w", err)
	}
	return &Client{conn: conn}, nil
}

func (c *Client) Close() {
	if c != nil && c.conn != nil {
		c.conn.Close()
	}
}

func (c *Client) Request(ctx context.Context, subject string, req clientcontract.RequestEnvelope) (clientcontract.ResponseEnvelope, error) {
	if c == nil || c.conn == nil {
		return clientcontract.ResponseEnvelope{}, errors.New("nats client is not connected")
	}
	if strings.TrimSpace(subject) == "" {
		return clientcontract.ResponseEnvelope{}, errors.New("subject is required")
	}
	if err := req.Validate(); err != nil {
		return clientcontract.ResponseEnvelope{}, err
	}
	data, err := json.Marshal(req)
	if err != nil {
		return clientcontract.ResponseEnvelope{}, fmt.Errorf("marshal request envelope: %w", err)
	}
	msg, err := c.conn.RequestWithContext(ctx, subject, data)
	if err != nil {
		return clientcontract.ResponseEnvelope{}, fmt.Errorf("request %s: %w", subject, err)
	}
	var resp clientcontract.ResponseEnvelope
	if err := json.Unmarshal(msg.Data, &resp); err != nil {
		return clientcontract.ResponseEnvelope{}, fmt.Errorf("decode response envelope: %w", err)
	}
	if err := resp.Validate(); err != nil {
		return clientcontract.ResponseEnvelope{}, err
	}
	return resp, nil
}

func ConnectFromEnv(ctx context.Context) (*Client, error) {
	return Connect(ctx, ConfigFromEnv())
}

func (c *Client) CreateSpace(ctx context.Context, req clientcontract.CreateSpaceRequest) (clientcontract.SpaceInfo, error) {
	return requestPayload[clientcontract.SpaceInfo](ctx, c, clientcontract.SubjectSpaceCreate, "", req)
}

func (c *Client) ListSpaces(ctx context.Context) (clientcontract.ListSpacesResponse, error) {
	return requestPayload[clientcontract.ListSpacesResponse](ctx, c, clientcontract.SubjectSpaceList, "", struct{}{})
}

func (c *Client) GetSpace(ctx context.Context, name string) (clientcontract.SpaceInfo, error) {
	return requestPayload[clientcontract.SpaceInfo](ctx, c, clientcontract.SubjectSpaceGet, "", clientcontract.GetSpaceRequest{Name: name})
}

func (c *Client) UpdateSpace(ctx context.Context, name string, quarkfile []byte) (clientcontract.SpaceInfo, error) {
	return requestPayload[clientcontract.SpaceInfo](ctx, c, clientcontract.SubjectSpaceUpdate, name, clientcontract.UpdateSpaceRequest{
		Name:      name,
		Quarkfile: append([]byte(nil), quarkfile...),
	})
}

func (c *Client) DeleteSpace(ctx context.Context, name string) error {
	_, err := requestPayload[struct{}](ctx, c, clientcontract.SubjectSpaceDelete, name, clientcontract.DeleteSpaceRequest{Name: name})
	return err
}

func (c *Client) Quarkfile(ctx context.Context, name string) (clientcontract.QuarkfileResponse, error) {
	return requestPayload[clientcontract.QuarkfileResponse](ctx, c, clientcontract.SubjectSpaceQuarkfile, name, clientcontract.QuarkfileRequest{Name: name})
}

func (c *Client) Doctor(ctx context.Context, name string) (clientcontract.DoctorResponse, error) {
	return requestPayload[clientcontract.DoctorResponse](ctx, c, clientcontract.SubjectSpaceDoctor, name, clientcontract.DoctorRequest{Name: name})
}

func (c *Client) CreateSession(ctx context.Context, req clientcontract.CreateSessionRequest) (clientcontract.SessionInfo, error) {
	return requestPayload[clientcontract.SessionInfo](ctx, c, clientcontract.SubjectSessionCreate, req.SpaceID, req)
}

func (c *Client) ListSessions(ctx context.Context, spaceID string) (clientcontract.ListSessionsResponse, error) {
	return requestPayload[clientcontract.ListSessionsResponse](ctx, c, clientcontract.SubjectSessionList, spaceID, clientcontract.ListSessionsRequest{SpaceID: spaceID})
}

func (c *Client) GetSession(ctx context.Context, spaceID, sessionID string) (clientcontract.SessionInfo, error) {
	return requestPayload[clientcontract.SessionInfo](ctx, c, clientcontract.SubjectSessionGet, spaceID, clientcontract.SessionRefRequest{
		SpaceID:   spaceID,
		SessionID: sessionID,
	})
}

func (c *Client) DeleteSession(ctx context.Context, spaceID, sessionID string) error {
	_, err := requestPayload[struct{}](ctx, c, clientcontract.SubjectSessionDelete, spaceID, clientcontract.SessionRefRequest{
		SpaceID:   spaceID,
		SessionID: sessionID,
	})
	return err
}

func (c *Client) KBGet(ctx context.Context, spaceID, namespace, key string) ([]byte, error) {
	resp, err := requestPayload[clientcontract.KBValueResponse](ctx, c, clientcontract.SubjectKBGet, spaceID, clientcontract.KBRefRequest{
		SpaceID:   spaceID,
		Namespace: namespace,
		Key:       key,
	})
	if err != nil {
		return nil, err
	}
	return append([]byte(nil), resp.Value...), nil
}

func (c *Client) KBSet(ctx context.Context, spaceID, namespace, key string, value []byte) error {
	_, err := requestPayload[struct{}](ctx, c, clientcontract.SubjectKBSet, spaceID, clientcontract.KBSetRequest{
		SpaceID:   spaceID,
		Namespace: namespace,
		Key:       key,
		Value:     append([]byte(nil), value...),
	})
	return err
}

func (c *Client) KBDelete(ctx context.Context, spaceID, namespace, key string) error {
	_, err := requestPayload[struct{}](ctx, c, clientcontract.SubjectKBDelete, spaceID, clientcontract.KBRefRequest{
		SpaceID:   spaceID,
		Namespace: namespace,
		Key:       key,
	})
	return err
}

func (c *Client) KBList(ctx context.Context, spaceID, namespace string) ([]string, error) {
	resp, err := requestPayload[clientcontract.KBListResponse](ctx, c, clientcontract.SubjectKBList, spaceID, clientcontract.KBListRequest{
		SpaceID:   spaceID,
		Namespace: namespace,
	})
	if err != nil {
		return nil, err
	}
	return append([]string(nil), resp.Keys...), nil
}

func (c *Client) ListPlugins(ctx context.Context, spaceID, typeFilter string) ([]clientcontract.PluginInfo, error) {
	resp, err := requestPayload[clientcontract.ListPluginsResponse](ctx, c, clientcontract.SubjectPluginList, spaceID, clientcontract.ListPluginsRequest{
		SpaceID:    spaceID,
		TypeFilter: typeFilter,
	})
	if err != nil {
		return nil, err
	}
	return append([]clientcontract.PluginInfo(nil), resp.Plugins...), nil
}

func (c *Client) GetPlugin(ctx context.Context, spaceID, plugin string) (clientcontract.PluginInfo, error) {
	return requestPayload[clientcontract.PluginInfo](ctx, c, clientcontract.SubjectPluginGet, spaceID, clientcontract.PluginRefRequest{
		SpaceID: spaceID,
		Plugin:  plugin,
	})
}

func (c *Client) InstallPlugin(ctx context.Context, spaceID, ref string) (clientcontract.PluginInfo, error) {
	resp, err := requestPayload[clientcontract.InstallPluginResponse](ctx, c, clientcontract.SubjectPluginInstall, spaceID, clientcontract.InstallPluginRequest{
		SpaceID: spaceID,
		Ref:     ref,
	})
	if err != nil {
		return clientcontract.PluginInfo{}, err
	}
	return resp.Plugin, nil
}

func (c *Client) UninstallPlugin(ctx context.Context, spaceID, plugin string) error {
	_, err := requestPayload[struct{}](ctx, c, clientcontract.SubjectPluginUninstall, spaceID, clientcontract.PluginRefRequest{
		SpaceID: spaceID,
		Plugin:  plugin,
	})
	return err
}

func (c *Client) SearchPlugins(ctx context.Context, spaceID, query string) ([]clientcontract.PluginSearchResult, error) {
	resp, err := requestPayload[clientcontract.SearchPluginsResponse](ctx, c, clientcontract.SubjectPluginSearch, spaceID, clientcontract.SearchPluginsRequest{
		SpaceID: spaceID,
		Query:   query,
	})
	if err != nil {
		return nil, err
	}
	return append([]clientcontract.PluginSearchResult(nil), resp.Results...), nil
}

func (c *Client) HubPluginInfo(ctx context.Context, spaceID, plugin string) (clientcontract.HubPluginInfo, error) {
	return requestPayload[clientcontract.HubPluginInfo](ctx, c, clientcontract.SubjectPluginHubInfo, spaceID, clientcontract.PluginRefRequest{
		SpaceID: spaceID,
		Plugin:  plugin,
	})
}

func (c *Client) ListServices(ctx context.Context, spaceID string) ([]clientcontract.ServiceInfo, error) {
	resp, err := requestPayload[clientcontract.ListServicesResponse](ctx, c, clientcontract.SubjectServiceList, spaceID, clientcontract.ListServicesRequest{SpaceID: spaceID})
	if err != nil {
		return nil, err
	}
	return cloneServices(resp.Services), nil
}

func (c *Client) InspectService(ctx context.Context, spaceID, service string) (clientcontract.ServiceInfo, error) {
	return requestPayload[clientcontract.ServiceInfo](ctx, c, clientcontract.SubjectServiceInspect, spaceID, clientcontract.InspectServiceRequest{
		SpaceID: spaceID,
		Service: service,
	})
}

func (c *Client) ServiceDoctor(ctx context.Context, spaceID string) (clientcontract.ServiceDoctorResponse, error) {
	resp, err := requestPayload[clientcontract.ServiceDoctorResponse](ctx, c, clientcontract.SubjectServiceDoctor, spaceID, clientcontract.ListServicesRequest{SpaceID: spaceID})
	if err != nil {
		return clientcontract.ServiceDoctorResponse{}, err
	}
	resp.Services = cloneServices(resp.Services)
	resp.Issues = append([]string(nil), resp.Issues...)
	return resp, nil
}

func requestPayload[T any](ctx context.Context, c *Client, subject, spaceID string, payload any) (T, error) {
	var out T
	req, err := clientcontract.NewRequest(newRequestID(), spaceID, payload)
	if err != nil {
		return out, err
	}
	resp, err := c.Request(ctx, subject, req)
	if err != nil {
		return out, err
	}
	if resp.Status == "error" {
		return out, responseError(resp)
	}
	if err := resp.DecodePayload(&out); err != nil {
		return out, err
	}
	return out, nil
}

func cloneServices(in []clientcontract.ServiceInfo) []clientcontract.ServiceInfo {
	out := make([]clientcontract.ServiceInfo, 0, len(in))
	for _, service := range in {
		copyService := service
		copyService.Functions = append([]clientcontract.ServiceFunctionInfo(nil), service.Functions...)
		copyService.Diagnostics = append([]string(nil), service.Diagnostics...)
		out = append(out, copyService)
	}
	return out
}

func responseError(resp clientcontract.ResponseEnvelope) error {
	if resp.Error == nil {
		return &ResponseError{Category: boundary.Internal, Message: "missing response error"}
	}
	return &ResponseError{
		Category: boundary.Category(resp.Error.Category),
		Message:  resp.Error.Message,
	}
}

func responseHasCategory(err error, category boundary.Category) bool {
	var responseErr *ResponseError
	if errors.As(err, &responseErr) {
		return responseErr.Category == category
	}
	return boundary.IsCategory(err, category)
}

func newRequestID() string {
	var buf [12]byte
	if _, err := rand.Read(buf[:]); err != nil {
		return fmt.Sprintf("req-%d", time.Now().UnixNano())
	}
	return "req-" + hex.EncodeToString(buf[:])
}

func normalizeConfig(cfg Config) Config {
	if strings.TrimSpace(cfg.URL) == "" {
		cfg.URL = DefaultURL
	}
	if strings.TrimSpace(cfg.Name) == "" {
		cfg.Name = "quark-cli"
	}
	if cfg.Timeout <= 0 {
		cfg.Timeout = 5 * time.Second
	}
	if cfg.ReconnectWait <= 0 {
		cfg.ReconnectWait = 250 * time.Millisecond
	}
	if cfg.MaxReconnects == 0 {
		cfg.MaxReconnects = 10
	}
	return cfg
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}
