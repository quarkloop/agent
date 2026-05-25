package natshub

import (
	"fmt"

	natsserver "github.com/nats-io/nats-server/v2/server"
)

func (h *Hub) ImportServiceFunctions(spaceID string, routes []ServiceFunctionRoute) error {
	h.mu.Lock()
	defer h.mu.Unlock()
	normalized, err := NormalizeServiceFunctionRoutes(routes)
	if err != nil {
		return err
	}
	space, err := h.provisionSpaceLocked(spaceID)
	if err != nil {
		return err
	}
	h.imports[spaceID] = cloneServiceFunctionRoutes(normalized)
	if !h.started {
		return nil
	}
	return h.applyServiceFunctionImportsLocked(space.Account, normalized)
}

func (h *Hub) applyServiceFunctionImportsLocked(spaceAccountName string, routes []ServiceFunctionRoute) error {
	controlAccount, err := h.accountLocked(ControlAccountName)
	if err != nil {
		return err
	}
	spaceAccount, err := h.accountLocked(spaceAccountName)
	if err != nil {
		return err
	}
	for _, route := range routes {
		key := route.key()
		if _, ok := h.applied[spaceAccountName]; !ok {
			h.applied[spaceAccountName] = make(map[string]struct{})
		}
		if _, ok := h.applied[spaceAccountName][key]; ok {
			continue
		}
		if route.Streaming {
			if err := controlAccount.AddServiceExportWithResponse(route.ExportSubject, natsserver.Streamed, []*natsserver.Account{spaceAccount}); err != nil {
				return fmt.Errorf("export streaming service function %q: %w", route.ExportSubject, err)
			}
		} else if err := controlAccount.AddServiceExport(route.ExportSubject, []*natsserver.Account{spaceAccount}); err != nil {
			return fmt.Errorf("export service function %q: %w", route.ExportSubject, err)
		}
		if err := spaceAccount.AddServiceImport(controlAccount, route.ImportSubject, route.ExportSubject); err != nil {
			return fmt.Errorf("import service function %q into %q: %w", route.ImportSubject, spaceAccountName, err)
		}
		h.applied[spaceAccountName][key] = struct{}{}
	}
	return nil
}

func (h *Hub) applyCatalogImportsLocked(spaceAccountName string) error {
	controlAccount, err := h.accountLocked(ControlAccountName)
	if err != nil {
		return err
	}
	spaceAccount, err := h.accountLocked(spaceAccountName)
	if err != nil {
		return err
	}
	key := catalogRuntimeGetSubject + "\x00" + catalogRuntimeGetSubject
	if _, ok := h.applied[spaceAccountName]; !ok {
		h.applied[spaceAccountName] = make(map[string]struct{})
	}
	if _, ok := h.applied[spaceAccountName][key]; ok {
		return nil
	}
	if err := controlAccount.AddServiceExport(catalogRuntimeGetSubject, []*natsserver.Account{spaceAccount}); err != nil {
		return fmt.Errorf("export catalog subject %q: %w", catalogRuntimeGetSubject, err)
	}
	if err := spaceAccount.AddServiceImport(controlAccount, catalogRuntimeGetSubject, catalogRuntimeGetSubject); err != nil {
		return fmt.Errorf("import catalog subject %q into %q: %w", catalogRuntimeGetSubject, spaceAccountName, err)
	}
	if err := controlAccount.AddStreamExport(catalogRuntimeEventsSubject, []*natsserver.Account{spaceAccount}); err != nil {
		return fmt.Errorf("export catalog events %q: %w", catalogRuntimeEventsSubject, err)
	}
	if err := spaceAccount.AddStreamImport(controlAccount, catalogRuntimeEventsSubject, ""); err != nil {
		return fmt.Errorf("import catalog events %q into %q: %w", catalogRuntimeEventsSubject, spaceAccountName, err)
	}
	h.applied[spaceAccountName][key] = struct{}{}
	return nil
}
