package secretssvc

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"path"
	"strconv"
	"strings"
	"time"

	secretsv1 "github.com/quarkloop/pkg/serviceapi/gen/quark/secrets/v1"
)

type OpenBaoConfig struct {
	Address string
	Token   string
	Mount   string
	Client  *http.Client
}

type OpenBaoClient struct {
	address string
	token   string
	mount   string
	client  *http.Client
}

func NewOpenBaoClient(cfg OpenBaoConfig) (*OpenBaoClient, error) {
	address := strings.TrimRight(strings.TrimSpace(cfg.Address), "/")
	if address == "" {
		return nil, fmt.Errorf("openbao address is required")
	}
	if _, err := url.ParseRequestURI(address); err != nil {
		return nil, fmt.Errorf("openbao address %q is invalid: %w", address, err)
	}
	mount := strings.Trim(strings.TrimSpace(cfg.Mount), "/")
	if mount == "" {
		mount = "secret"
	}
	client := cfg.Client
	if client == nil {
		client = &http.Client{Timeout: 10 * time.Second}
	}
	return &OpenBaoClient{address: address, token: strings.TrimSpace(cfg.Token), mount: mount, client: client}, nil
}

func (c *OpenBaoClient) Resolve(ctx context.Context, ref SecretRef, includeValue bool) (*secretsv1.SecretMaterial, error) {
	if c.token == "" {
		return nil, fmt.Errorf("openbao token is required")
	}
	var resp baoReadResponse
	if err := c.do(ctx, http.MethodGet, c.kvDataPath(ref), nil, &resp); err != nil {
		return nil, err
	}
	value, ok := resp.Data.Data[ref.Field]
	if !ok {
		return nil, fmt.Errorf("secret field %q not found", ref.Field)
	}
	material := &secretsv1.SecretMaterial{
		Ref:           ref.String(),
		Field:         ref.Field,
		ValueRedacted: !includeValue,
		Lease:         leaseFromBao(resp.LeaseID, resp.LeaseDuration, resp.Renewable),
	}
	if includeValue {
		material.Value = fmt.Sprint(value)
	}
	return material, nil
}

func (c *OpenBaoClient) IssueToken(ctx context.Context, req *secretsv1.IssueScopedSecretRequest) (*secretsv1.ScopedSecret, error) {
	if c.token == "" {
		return nil, fmt.Errorf("openbao token is required")
	}
	body := map[string]any{
		"policies":  append([]string(nil), req.GetPolicies()...),
		"renewable": req.GetRenewable(),
		"meta":      cloneStringMap(req.GetMetadata()),
	}
	if req.GetTtlSeconds() > 0 {
		body["ttl"] = strconv.FormatInt(req.GetTtlSeconds(), 10) + "s"
	}
	var resp baoTokenResponse
	if err := c.do(ctx, http.MethodPost, "auth/token/create", body, &resp); err != nil {
		return nil, err
	}
	return &secretsv1.ScopedSecret{
		Scope:    req.GetScope(),
		Token:    resp.Auth.ClientToken,
		Accessor: resp.Auth.Accessor,
		Policies: append([]string(nil), resp.Auth.Policies...),
		Lease:    leaseFromBao("", resp.Auth.LeaseDuration, resp.Auth.Renewable),
	}, nil
}

func (c *OpenBaoClient) RenewLease(ctx context.Context, leaseID string, incrementSeconds int64) (*secretsv1.Lease, error) {
	if c.token == "" {
		return nil, fmt.Errorf("openbao token is required")
	}
	body := map[string]any{"lease_id": leaseID}
	if incrementSeconds > 0 {
		body["increment"] = incrementSeconds
	}
	var resp baoLeaseResponse
	if err := c.do(ctx, http.MethodPost, "sys/leases/renew", body, &resp); err != nil {
		return nil, err
	}
	return leaseFromBao(resp.LeaseID, resp.LeaseDuration, resp.Renewable), nil
}

func (c *OpenBaoClient) RevokeLease(ctx context.Context, leaseID string, sync bool) error {
	if c.token == "" {
		return fmt.Errorf("openbao token is required")
	}
	return c.do(ctx, http.MethodPost, "sys/leases/revoke", map[string]any{
		"lease_id": leaseID,
		"sync":     sync,
	}, nil)
}

func (c *OpenBaoClient) Rotate(ctx context.Context, ref SecretRef, value string, cas int64) (int64, error) {
	if c.token == "" {
		return 0, fmt.Errorf("openbao token is required")
	}
	body := map[string]any{
		"data": map[string]any{ref.Field: value},
	}
	if cas > 0 {
		body["options"] = map[string]any{"cas": cas}
	}
	var resp baoWriteResponse
	if err := c.do(ctx, http.MethodPost, c.kvDataPath(ref), body, &resp); err != nil {
		return 0, err
	}
	return resp.Data.Version, nil
}

func (c *OpenBaoClient) kvDataPath(ref SecretRef) string {
	mount := ref.Mount
	if mount == "" {
		mount = c.mount
	}
	return path.Join(mount, "data", ref.Path)
}

func (c *OpenBaoClient) do(ctx context.Context, method, requestPath string, body any, out any) error {
	var reader io.Reader
	if body != nil {
		data, err := json.Marshal(body)
		if err != nil {
			return err
		}
		reader = bytes.NewReader(data)
	}
	req, err := http.NewRequestWithContext(ctx, method, c.address+"/v1/"+strings.TrimLeft(requestPath, "/"), reader)
	if err != nil {
		return err
	}
	req.Header.Set("X-Vault-Token", c.token)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := c.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("openbao %s %s returned %s: %s", method, requestPath, resp.Status, strings.TrimSpace(string(data)))
	}
	if out == nil || len(data) == 0 {
		return nil
	}
	if err := json.Unmarshal(data, out); err != nil {
		return fmt.Errorf("decode openbao response: %w", err)
	}
	return nil
}

type baoReadResponse struct {
	LeaseID       string `json:"lease_id"`
	LeaseDuration int64  `json:"lease_duration"`
	Renewable     bool   `json:"renewable"`
	Data          struct {
		Data map[string]any `json:"data"`
	} `json:"data"`
}

type baoTokenResponse struct {
	Auth struct {
		ClientToken   string   `json:"client_token"`
		Accessor      string   `json:"accessor"`
		Policies      []string `json:"policies"`
		LeaseDuration int64    `json:"lease_duration"`
		Renewable     bool     `json:"renewable"`
	} `json:"auth"`
}

type baoLeaseResponse struct {
	LeaseID       string `json:"lease_id"`
	LeaseDuration int64  `json:"lease_duration"`
	Renewable     bool   `json:"renewable"`
}

type baoWriteResponse struct {
	Data struct {
		Version int64 `json:"version"`
	} `json:"data"`
}

func leaseFromBao(leaseID string, durationSeconds int64, renewable bool) *secretsv1.Lease {
	if leaseID == "" && durationSeconds == 0 && !renewable {
		return nil
	}
	return &secretsv1.Lease{LeaseId: leaseID, DurationSeconds: durationSeconds, Renewable: renewable}
}
