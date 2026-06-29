module github.com/quarkloop/services/citation

go 1.26.4

require (
	github.com/quarkloop/pkg/natskit v0.0.0
	github.com/quarkloop/pkg/serviceapi v0.0.0
)

require (
	github.com/klauspost/compress v1.18.6 // indirect
	github.com/nats-io/nats.go v1.52.0 // indirect
	github.com/nats-io/nkeys v0.4.15 // indirect
	github.com/nats-io/nuid v1.0.1 // indirect
	github.com/quarkloop/pkg/boundary v0.0.0 // indirect
	github.com/quarkloop/pkg/plugin v0.0.0 // indirect
	golang.org/x/crypto v0.52.0 // indirect
	golang.org/x/sys v0.45.0 // indirect
	google.golang.org/protobuf v1.36.12-0.20260120151049-f2248ac996af // indirect
	gopkg.in/yaml.v3 v3.0.1 // indirect
)

replace (
	github.com/quarkloop/pkg/boundary v0.0.0 => ../../pkg/boundary
	github.com/quarkloop/pkg/natskit v0.0.0 => ../../pkg/natskit
	github.com/quarkloop/pkg/plugin v0.0.0 => ../../pkg/plugin
	github.com/quarkloop/pkg/serviceapi v0.0.0 => ../../pkg/serviceapi
)
