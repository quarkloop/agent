package natshub

import (
	"errors"

	natsserver "github.com/nats-io/nats-server/v2/server"
)

func BuildOptions(cfg Config) (*natsserver.Options, error) {
	opts, _, _, err := buildOptionsAndRegistry(cfg)
	return opts, err
}

func buildOptionsAndRegistry(cfg Config) (*natsserver.Options, map[string]*natsserver.Account, *credentialRegistry, error) {
	normalized, err := Normalize(cfg)
	if err != nil {
		return nil, nil, nil, err
	}
	if normalized.Mode != ModeEmbedded {
		return nil, nil, nil, errors.New("embedded nats options requested for external mode")
	}
	accounts, accountMap, credentials, err := buildAccounts(normalized.Accounts)
	if err != nil {
		return nil, nil, nil, err
	}
	opts := &natsserver.Options{
		ServerName:                 normalized.ServerName,
		Host:                       normalized.Client.Host,
		Port:                       normalized.Client.Port,
		NoSigs:                     true,
		NoLog:                      normalized.NoLog,
		Accounts:                   accounts,
		SystemAccount:              normalized.SystemAccount,
		CustomClientAuthentication: registryAuthenticator{registry: credentials},
		JetStream:                  normalized.JetStream.Enabled,
		StoreDir:                   normalized.JetStream.StoreDir,
		JetStreamDomain:            normalized.JetStream.Domain,
		JetStreamMaxMemory:         normalized.JetStream.MaxMemory,
		JetStreamMaxStore:          normalized.JetStream.MaxStore,
	}
	if normalized.Monitoring.Enabled {
		opts.HTTPHost = normalized.Monitoring.Host
		opts.HTTPPort = normalized.Monitoring.Port
	}
	if normalized.WebSocket.Enabled {
		opts.Websocket = natsserver.WebsocketOpts{
			Host:  normalized.WebSocket.Host,
			Port:  normalized.WebSocket.Port,
			NoTLS: true,
		}
	}
	return opts, accountMap, credentials, nil
}
