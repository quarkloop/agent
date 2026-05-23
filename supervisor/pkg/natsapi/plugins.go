package natsapi

import (
	"context"

	plugin "github.com/quarkloop/pkg/plugin"
	"github.com/quarkloop/pkg/serviceapi/clientcontract"
	"github.com/quarkloop/supervisor/pkg/pluginmanager"
)

func (s *Server) listPlugins(req clientcontract.RequestEnvelope) (any, error) {
	var payload clientcontract.ListPluginsRequest
	if err := req.DecodePayload(&payload); err != nil {
		return nil, err
	}
	mgr, err := s.store.Plugins(payload.SpaceID)
	if err != nil {
		return nil, err
	}
	var installed []pluginmanager.InstalledPlugin
	if payload.TypeFilter != "" {
		installed, err = mgr.ListByType(plugin.PluginType(payload.TypeFilter))
	} else {
		installed, err = mgr.List()
	}
	if err != nil {
		return nil, err
	}
	out := make([]clientcontract.PluginInfo, 0, len(installed))
	for _, item := range installed {
		out = append(out, toContractPlugin(item))
	}
	return clientcontract.ListPluginsResponse{Plugins: out}, nil
}

func (s *Server) getPlugin(req clientcontract.RequestEnvelope) (any, error) {
	var payload clientcontract.PluginRefRequest
	if err := req.DecodePayload(&payload); err != nil {
		return nil, err
	}
	mgr, err := s.store.Plugins(payload.SpaceID)
	if err != nil {
		return nil, err
	}
	item, err := mgr.Get(payload.Plugin)
	if err != nil {
		return nil, err
	}
	return toContractPlugin(item), nil
}

func (s *Server) installPlugin(req clientcontract.RequestEnvelope) (any, error) {
	var payload clientcontract.InstallPluginRequest
	if err := req.DecodePayload(&payload); err != nil {
		return nil, err
	}
	mgr, err := s.store.Plugins(payload.SpaceID)
	if err != nil {
		return nil, err
	}
	installed, err := mgr.Install(context.Background(), payload.Ref)
	if err != nil {
		return nil, err
	}
	if err := s.publishCatalogEvent(payload.SpaceID, "plugin_installed"); err != nil {
		return nil, err
	}
	return clientcontract.InstallPluginResponse{Plugin: toContractPlugin(*installed)}, nil
}

func (s *Server) uninstallPlugin(req clientcontract.RequestEnvelope) (any, error) {
	var payload clientcontract.PluginRefRequest
	if err := req.DecodePayload(&payload); err != nil {
		return nil, err
	}
	mgr, err := s.store.Plugins(payload.SpaceID)
	if err != nil {
		return nil, err
	}
	if err := mgr.Uninstall(payload.Plugin); err != nil {
		return nil, err
	}
	if err := s.publishCatalogEvent(payload.SpaceID, "plugin_uninstalled"); err != nil {
		return nil, err
	}
	return struct{}{}, nil
}

func (s *Server) searchPlugins(req clientcontract.RequestEnvelope) (any, error) {
	var payload clientcontract.SearchPluginsRequest
	if err := req.DecodePayload(&payload); err != nil {
		return nil, err
	}
	mgr, err := s.store.Plugins(payload.SpaceID)
	if err != nil {
		return nil, err
	}
	results, err := mgr.Search(payload.Query)
	if err != nil {
		return nil, err
	}
	out := make([]clientcontract.PluginSearchResult, 0, len(results))
	for _, result := range results {
		out = append(out, clientcontract.PluginSearchResult{
			Name:        result.Name,
			Version:     result.Version,
			Type:        result.Type,
			Description: result.Description,
			Author:      result.Author,
		})
	}
	return clientcontract.SearchPluginsResponse{Results: out}, nil
}

func (s *Server) hubPluginInfo(req clientcontract.RequestEnvelope) (any, error) {
	var payload clientcontract.PluginRefRequest
	if err := req.DecodePayload(&payload); err != nil {
		return nil, err
	}
	mgr, err := s.store.Plugins(payload.SpaceID)
	if err != nil {
		return nil, err
	}
	info, err := mgr.GetHubInfo(payload.Plugin)
	if err != nil {
		return nil, err
	}
	return clientcontract.HubPluginInfo{
		Name:        info.Name,
		Version:     info.Version,
		Type:        info.Type,
		Description: info.Description,
		Author:      info.Author,
		License:     info.License,
		Repository:  info.Repository,
		Downloads:   info.Downloads,
		Versions:    append([]string(nil), info.Versions...),
	}, nil
}
