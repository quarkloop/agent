module github.com/quarkloop/e2e

go 1.26.4

require (
	github.com/quarkloop/pkg/boundary v0.0.0
	github.com/quarkloop/pkg/natskit v0.0.0
	github.com/quarkloop/pkg/plugin v0.0.0
	github.com/quarkloop/pkg/serviceapi v0.0.0
	github.com/quarkloop/pkg/space v0.0.0
	github.com/quarkloop/supervisor v0.0.0
	google.golang.org/protobuf v1.36.12-0.20260120151049-f2248ac996af
)

require (
	github.com/antithesishq/antithesis-sdk-go v0.7.0-default-no-op // indirect
	github.com/google/go-tpm v0.9.8 // indirect
	github.com/klauspost/compress v1.18.6 // indirect
	github.com/minio/highwayhash v1.0.4 // indirect
	github.com/nats-io/jwt/v2 v2.8.1 // indirect
	github.com/nats-io/nats-server/v2 v2.14.1 // indirect
	github.com/nats-io/nats.go v1.52.0 // indirect
	github.com/nats-io/nkeys v0.4.15 // indirect
	github.com/nats-io/nuid v1.0.1 // indirect
	golang.org/x/crypto v0.52.0 // indirect
	golang.org/x/sys v0.45.0 // indirect
	golang.org/x/time v0.15.0 // indirect
	gopkg.in/yaml.v3 v3.0.1 // indirect
)

replace (
	github.com/quarkloop/cli v0.0.0 => ../cli
	github.com/quarkloop/pkg/boundary => ../pkg/boundary
	github.com/quarkloop/pkg/event => ../pkg/event
	github.com/quarkloop/pkg/natskit => ../pkg/natskit
	github.com/quarkloop/pkg/plugin => ../pkg/plugin
	github.com/quarkloop/pkg/serviceapi v0.0.0 => ../pkg/serviceapi
	github.com/quarkloop/pkg/space => ../pkg/space
	github.com/quarkloop/runtime v0.0.0 => ../runtime
	github.com/quarkloop/supervisor v0.0.0 => ../supervisor
)
