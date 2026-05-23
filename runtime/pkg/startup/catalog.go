package startup

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"

	"github.com/quarkloop/pkg/plugin"
	"github.com/quarkloop/pkg/serviceapi/clientcontract"
	"github.com/quarkloop/pkg/serviceapi/servicekit"
	"github.com/quarkloop/runtime/pkg/catalogclient"
	"github.com/quarkloop/runtime/pkg/gatewayclient"
	"github.com/quarkloop/runtime/pkg/pluginmanager"
	runtimeservices "github.com/quarkloop/runtime/pkg/services"
)

func LoadRuntimeCatalogSnapshotForSpace(ctx context.Context, cfg catalogclient.Config) (*clientcontract.RuntimeCatalogResponse, error) {
	if !cfg.Available() {
		return nil, nil
	}
	snapshot, err := catalogclient.FetchRuntimeCatalog(ctx, cfg)
	if err != nil {
		return nil, err
	}
	if len(snapshot.PluginCatalog) == 0 {
		return nil, fmt.Errorf("runtime catalog snapshot missing plugin catalog")
	}
	slog.Info("runtime catalog snapshot loaded", "space", snapshot.SpaceID, "generated_at", snapshot.GeneratedAt)
	return &snapshot, nil
}

func LoadServiceCatalog(snapshot *clientcontract.RuntimeCatalogResponse) (*runtimeservices.Catalog, error) {
	return LoadServiceCatalogForSpace(snapshot, clientcontract.NATSCredential{})
}

func LoadServiceCatalogForSpace(snapshot *clientcontract.RuntimeCatalogResponse, credential clientcontract.NATSCredential) (*runtimeservices.Catalog, error) {
	if snapshot != nil && len(snapshot.ServiceCatalog) > 0 {
		descriptors, err := servicekit.UnmarshalRuntimeServiceCatalog(snapshot.ServiceCatalog)
		if err != nil {
			return nil, fmt.Errorf("parse nats runtime service catalog: %w", err)
		}
		return runtimeservices.NewCatalogWithCaller(descriptors, runtimeservices.NewNATSCaller(NATSCallerConfig(credential))), nil
	}
	slog.Info("no supervisor-resolved service catalog provided")
	return nil, nil
}

func LoadPluginCatalog(snapshot *clientcontract.RuntimeCatalogResponse) (*pluginmanager.Catalog, error) {
	raw := ""
	if snapshot != nil && len(snapshot.PluginCatalog) > 0 {
		raw = string(snapshot.PluginCatalog)
	}
	if raw == "" {
		slog.Info("no supervisor-resolved plugin catalog provided; using empty catalog")
		catalog := plugin.NewRuntimeCatalog(nil)
		return &catalog, nil
	}
	var catalog pluginmanager.Catalog
	if err := json.Unmarshal([]byte(raw), &catalog); err != nil {
		return nil, fmt.Errorf("parse runtime plugin catalog: %w", err)
	}
	if err := catalog.Validate(); err != nil {
		return nil, fmt.Errorf("invalid runtime plugin catalog: %w", err)
	}
	if catalog.Empty() {
		slog.Info("supervisor-resolved plugin catalog is empty")
		return &catalog, nil
	}
	for _, item := range catalog.Plugins {
		slog.Info("plugin catalog entry loaded", "name", item.Name, "type", item.Type, "path", item.Path)
	}
	return &catalog, nil
}

func CatalogConfig(credential clientcontract.NATSCredential) catalogclient.Config {
	cfg := catalogclient.ConfigFromEnv()
	cfg.URL = FirstNonEmpty(credential.URL, cfg.URL)
	cfg.Username = FirstNonEmpty(credential.Username, cfg.Username)
	cfg.Password = FirstNonEmpty(credential.Password, cfg.Password)
	cfg.SpaceID = FirstNonEmpty(credential.SpaceID, cfg.SpaceID)
	return cfg
}

func NATSCallerConfig(credential clientcontract.NATSCredential) runtimeservices.NATSCallerConfig {
	cfg := runtimeservices.NATSCallerConfigFromEnv()
	cfg.URL = FirstNonEmpty(credential.URL, cfg.URL)
	cfg.Username = FirstNonEmpty(credential.Username, cfg.Username)
	cfg.Password = FirstNonEmpty(credential.Password, cfg.Password)
	cfg.SpaceID = FirstNonEmpty(credential.SpaceID, cfg.SpaceID)
	if credential.SpaceID != "" {
		cfg.Name = "quark-runtime-service-functions-" + SpaceToken(credential.SpaceID)
	}
	return cfg
}

func GatewayConfig(credential clientcontract.NATSCredential) gatewayclient.Config {
	cfg := gatewayclient.ConfigFromEnv()
	cfg.URL = FirstNonEmpty(credential.URL, cfg.URL)
	cfg.Username = FirstNonEmpty(credential.Username, cfg.Username)
	cfg.Password = FirstNonEmpty(credential.Password, cfg.Password)
	return cfg
}
