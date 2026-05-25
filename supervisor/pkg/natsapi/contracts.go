package natsapi

import (
	"github.com/quarkloop/pkg/serviceapi/clientcontract"
	"github.com/quarkloop/supervisor/pkg/natshub"
	"github.com/quarkloop/supervisor/pkg/pluginmanager"
	"github.com/quarkloop/supervisor/pkg/sessions"
	"github.com/quarkloop/supervisor/pkg/space"
)

func toContractSpace(sp *space.Space) clientcontract.SpaceInfo {
	if sp == nil {
		return clientcontract.SpaceInfo{}
	}
	return clientcontract.SpaceInfo{
		Name:       sp.Name,
		Version:    sp.Version,
		WorkingDir: sp.WorkingDir,
		CreatedAt:  sp.CreatedAt,
		UpdatedAt:  sp.UpdatedAt,
	}
}

func toContractPlugin(item pluginmanager.InstalledPlugin) clientcontract.PluginInfo {
	return clientcontract.PluginInfo{
		Name:        item.Manifest.Name,
		Version:     item.Manifest.Version,
		Type:        string(item.Manifest.Type),
		Mode:        string(item.Manifest.Mode),
		Description: item.Manifest.Description,
	}
}

func toContractSession(sess *sessions.Session) clientcontract.SessionInfo {
	if sess == nil {
		return clientcontract.SessionInfo{}
	}
	return clientcontract.SessionInfo{
		ID:        sess.ID,
		Type:      clientcontract.SessionType(sess.Type),
		Title:     sess.Title,
		CreatedAt: sess.CreatedAt,
		UpdatedAt: sess.UpdatedAt,
	}
}

func toContractCredential(url string, credential natshub.Credential) clientcontract.NATSCredential {
	return clientcontract.NATSCredential{
		URL:       url,
		Username:  credential.Username,
		Password:  credential.Password,
		Account:   credential.Account,
		Role:      string(credential.Role),
		SpaceID:   credential.SpaceID,
		SessionID: credential.SessionID,
		AgentID:   credential.AgentID,
	}
}

func toContractAuditRecord(record natshub.StoredAuditRecord) clientcontract.AuditRecord {
	return clientcontract.AuditRecord{
		Sequence:           record.Sequence,
		ServiceCallID:      record.ServiceCallID,
		ReferenceID:        record.ReferenceID,
		AuditRef:           record.AuditRef,
		SpaceID:            record.SpaceID,
		SessionID:          record.SessionID,
		RunID:              record.RunID,
		WorkflowID:         record.WorkflowID,
		AgentID:            record.AgentID,
		Service:            record.Service,
		Function:           record.Function,
		Subject:            record.Subject,
		Status:             record.Status,
		ErrorCategory:      record.ErrorCategory,
		DurationMillis:     record.DurationMillis,
		TraceID:            record.TraceID,
		RetentionExpiresAt: record.RetentionExpiresAt,
		RecordedAt:         record.RecordedAt,
	}
}

func toContractAuditPage(page natshub.AuditPage) clientcontract.AuditListResponse {
	out := clientcontract.AuditListResponse{
		Records:    make([]clientcontract.AuditRecord, 0, len(page.Records)),
		NextCursor: page.NextCursor,
	}
	for _, record := range page.Records {
		out.Records = append(out.Records, toContractAuditRecord(record))
	}
	return out
}
