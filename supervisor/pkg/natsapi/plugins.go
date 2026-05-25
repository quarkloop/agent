package natsapi

import (
	"context"

	"github.com/quarkloop/pkg/boundary"
	"github.com/quarkloop/pkg/serviceapi/clientcontract"
)

func (s *Server) listPlugins(req clientcontract.RequestEnvelope) (any, error) {
	var payload clientcontract.ListPluginsRequest
	if err := req.DecodePayload(&payload); err != nil {
		return nil, err
	}
	if s.pluginController == nil {
		return nil, boundary.New(boundary.Supervisor, boundary.Unavailable, clientcontract.SubjectPluginList, "plugin catalog is not configured")
	}
	installed, err := s.pluginController.ListSpacePlugins(context.Background(), payload.SpaceID, payload.TypeFilter)
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
	if s.pluginController == nil {
		return nil, boundary.New(boundary.Supervisor, boundary.Unavailable, clientcontract.SubjectPluginGet, "plugin catalog is not configured")
	}
	item, err := s.pluginController.GetSpacePlugin(context.Background(), payload.SpaceID, payload.Plugin)
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
	if s.pluginController == nil {
		return nil, boundary.New(boundary.Supervisor, boundary.Unavailable, clientcontract.SubjectPluginInstall, "plugin catalog is not configured")
	}
	installed, err := s.pluginController.InstallSpacePlugin(context.Background(), payload.SpaceID, payload.Ref)
	if err != nil {
		return nil, err
	}
	if err := s.publishCatalogEvent(payload.SpaceID, "plugin_installed"); err != nil {
		return nil, err
	}
	return clientcontract.InstallPluginResponse{Plugin: toContractPlugin(installed)}, nil
}

func (s *Server) uninstallPlugin(req clientcontract.RequestEnvelope) (any, error) {
	var payload clientcontract.PluginRefRequest
	if err := req.DecodePayload(&payload); err != nil {
		return nil, err
	}
	if s.pluginController == nil {
		return nil, boundary.New(boundary.Supervisor, boundary.Unavailable, clientcontract.SubjectPluginUninstall, "plugin catalog is not configured")
	}
	if err := s.pluginController.UninstallSpacePlugin(context.Background(), payload.SpaceID, payload.Plugin); err != nil {
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
	if s.pluginController == nil {
		return nil, boundary.New(boundary.Supervisor, boundary.Unavailable, clientcontract.SubjectPluginSearch, "plugin catalog is not configured")
	}
	results, err := s.pluginController.SearchPlugins(context.Background(), payload.Query)
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
	if s.pluginController == nil {
		return nil, boundary.New(boundary.Supervisor, boundary.Unavailable, clientcontract.SubjectPluginHubInfo, "plugin catalog is not configured")
	}
	info, err := s.pluginController.HubPluginInfo(context.Background(), payload.Plugin)
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
