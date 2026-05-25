module github.com/quarkloop/services/workflow

go 1.26.2

require (
	github.com/nats-io/nats-server/v2 v2.14.1
	github.com/quarkloop/pkg/boundary v0.0.0
	github.com/quarkloop/pkg/natskit v0.0.0
	github.com/quarkloop/pkg/serviceapi v0.0.0
	github.com/stretchr/testify v1.10.0
	go.temporal.io/sdk v1.38.0
	google.golang.org/protobuf v1.36.10
)

require (
	github.com/nats-io/nats.go v1.51.0 // indirect
	google.golang.org/grpc v1.71.0 // indirect
)

require (
	github.com/antithesishq/antithesis-sdk-go v0.7.0-default-no-op // indirect
	github.com/davecgh/go-spew v1.1.1 // indirect
	github.com/facebookgo/clock v0.0.0-20150410010913-600d898af40a // indirect
	github.com/gogo/protobuf v1.3.2 // indirect
	github.com/golang/mock v1.6.0 // indirect
	github.com/google/go-tpm v0.9.8 // indirect
	github.com/google/uuid v1.6.0 // indirect
	github.com/grpc-ecosystem/go-grpc-middleware/v2 v2.3.2 // indirect
	github.com/grpc-ecosystem/grpc-gateway/v2 v2.22.0 // indirect
	github.com/klauspost/compress v1.18.6 // indirect
	github.com/minio/highwayhash v1.0.4 // indirect
	github.com/nats-io/jwt/v2 v2.8.1 // indirect
	github.com/nats-io/nkeys v0.4.15 // indirect
	github.com/nats-io/nuid v1.0.1 // indirect
	github.com/nexus-rpc/sdk-go v0.5.1 // indirect
	github.com/pmezard/go-difflib v1.0.0 // indirect
	github.com/quarkloop/pkg/plugin v0.0.0 // indirect
	github.com/robfig/cron v1.2.0 // indirect
	github.com/stretchr/objx v0.5.2 // indirect
	go.temporal.io/api v1.54.0
	golang.org/x/crypto v0.51.0 // indirect
	golang.org/x/net v0.53.0 // indirect
	golang.org/x/sync v0.20.0 // indirect
	golang.org/x/sys v0.44.0 // indirect
	golang.org/x/text v0.37.0 // indirect
	golang.org/x/time v0.15.0 // indirect
	google.golang.org/genproto/googleapis/api v0.0.0-20250804133106-a7a43d27e69b // indirect
	google.golang.org/genproto/googleapis/rpc v0.0.0-20250804133106-a7a43d27e69b // indirect
	gopkg.in/yaml.v3 v3.0.1 // indirect
)

replace (
	github.com/quarkloop/pkg/boundary v0.0.0 => ../../pkg/boundary
	github.com/quarkloop/pkg/natskit v0.0.0 => ../../pkg/natskit
	github.com/quarkloop/pkg/plugin v0.0.0 => ../../pkg/plugin
	github.com/quarkloop/pkg/serviceapi v0.0.0 => ../../pkg/serviceapi
)
