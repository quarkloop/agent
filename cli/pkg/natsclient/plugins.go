package natsclient

import (
	"context"

	"github.com/quarkloop/pkg/serviceapi/clientcontract"
)

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
