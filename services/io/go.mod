module github.com/quarkloop/services/io

go 1.26.2

require (
	github.com/quarkloop/pkg/natskit v0.0.0
	github.com/quarkloop/pkg/serviceapi v0.0.0
)

require (
	github.com/klauspost/compress v1.18.5 // indirect
	github.com/nats-io/nats.go v1.51.0 // indirect
	github.com/nats-io/nkeys v0.4.15 // indirect
	github.com/nats-io/nuid v1.0.1 // indirect
	github.com/quarkloop/pkg/boundary v0.0.0 // indirect
	github.com/quarkloop/pkg/plugin v0.0.0 // indirect
	golang.org/x/crypto v0.50.0 // indirect
	golang.org/x/sys v0.43.0 // indirect
	google.golang.org/protobuf v1.36.10 // indirect
	gopkg.in/yaml.v3 v3.0.1 // indirect
)

replace (
	github.com/quarkloop/pkg/boundary v0.0.0 => ../../pkg/boundary
	github.com/quarkloop/pkg/natskit v0.0.0 => ../../pkg/natskit
	github.com/quarkloop/pkg/plugin v0.0.0 => ../../pkg/plugin
	github.com/quarkloop/pkg/serviceapi v0.0.0 => ../../pkg/serviceapi
)
