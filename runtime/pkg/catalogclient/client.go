package catalogclient

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/nats-io/nats.go"
	"github.com/quarkloop/pkg/serviceapi/clientcontract"
)

const (
	EnvNATSURL      = "QUARK_NATS_URL"
	EnvNATSUser     = "QUARK_NATS_USER"
	EnvNATSPassword = "QUARK_NATS_PASSWORD"
	EnvSpace        = "QUARK_SPACE"
)

type Config struct {
	URL      string
	Username string
	Password string
	SpaceID  string
	Timeout  time.Duration
}

func ConfigFromEnv() Config {
	return Config{
		URL:      strings.TrimSpace(os.Getenv(EnvNATSURL)),
		Username: strings.TrimSpace(os.Getenv(EnvNATSUser)),
		Password: strings.TrimSpace(os.Getenv(EnvNATSPassword)),
		SpaceID:  strings.TrimSpace(os.Getenv(EnvSpace)),
		Timeout:  5 * time.Second,
	}
}

func (c Config) Available() bool {
	return strings.TrimSpace(c.URL) != "" &&
		strings.TrimSpace(c.Username) != "" &&
		strings.TrimSpace(c.Password) != "" &&
		strings.TrimSpace(c.SpaceID) != ""
}

func FetchRuntimeCatalog(ctx context.Context, cfg Config) (clientcontract.RuntimeCatalogResponse, error) {
	if !cfg.Available() {
		return clientcontract.RuntimeCatalogResponse{}, errors.New("nats catalog client config is incomplete")
	}
	timeout := cfg.Timeout
	if timeout <= 0 {
		timeout = 5 * time.Second
	}
	conn, err := nats.Connect(
		cfg.URL,
		nats.UserInfo(cfg.Username, cfg.Password),
		nats.Name("quark-runtime-catalog-client"),
		nats.Timeout(timeout),
		nats.RetryOnFailedConnect(true),
		nats.MaxReconnects(10),
		nats.ReconnectWait(250*time.Millisecond),
	)
	if err != nil {
		return clientcontract.RuntimeCatalogResponse{}, fmt.Errorf("connect nats catalog client: %w", err)
	}
	defer conn.Close()
	if err := conn.FlushTimeout(timeout); err != nil {
		return clientcontract.RuntimeCatalogResponse{}, fmt.Errorf("verify nats catalog client: %w", err)
	}
	req, err := clientcontract.NewRequest("runtime-catalog", cfg.SpaceID, clientcontract.RuntimeCatalogRequest{SpaceID: cfg.SpaceID})
	if err != nil {
		return clientcontract.RuntimeCatalogResponse{}, err
	}
	data, err := json.Marshal(req)
	if err != nil {
		return clientcontract.RuntimeCatalogResponse{}, fmt.Errorf("marshal runtime catalog request: %w", err)
	}
	requestCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	msg := nats.NewMsg(clientcontract.SubjectCatalogRuntimeGet)
	msg.Data = data
	for key, value := range req.CorrelationHeaders() {
		msg.Header.Set(key, value)
	}
	reply, err := conn.RequestMsgWithContext(requestCtx, msg)
	if err != nil {
		return clientcontract.RuntimeCatalogResponse{}, fmt.Errorf("request runtime catalog: %w", err)
	}
	var resp clientcontract.ResponseEnvelope
	if err := json.Unmarshal(reply.Data, &resp); err != nil {
		return clientcontract.RuntimeCatalogResponse{}, fmt.Errorf("decode runtime catalog response: %w", err)
	}
	if resp.Status != "ok" {
		if resp.Error != nil {
			return clientcontract.RuntimeCatalogResponse{}, fmt.Errorf("runtime catalog error: %s: %s", resp.Error.Category, resp.Error.Message)
		}
		return clientcontract.RuntimeCatalogResponse{}, fmt.Errorf("runtime catalog request failed: %#v", resp)
	}
	var out clientcontract.RuntimeCatalogResponse
	if err := resp.DecodePayload(&out); err != nil {
		return clientcontract.RuntimeCatalogResponse{}, fmt.Errorf("decode runtime catalog payload: %w", err)
	}
	return out, nil
}
