module github.com/quarkloop/runtime

go 1.26.2

require (
	github.com/gofiber/fiber/v2 v2.52.13
	github.com/google/uuid v1.6.0
	github.com/nats-io/nats-server/v2 v2.14.0
	github.com/nats-io/nats.go v1.51.0
	github.com/quarkloop/pkg/boundary v0.0.0
	github.com/quarkloop/pkg/event v0.0.0
	github.com/quarkloop/pkg/plugin v0.0.0
	github.com/quarkloop/pkg/serviceapi v0.0.0
	github.com/quarkloop/supervisor v0.0.0
	github.com/spf13/cobra v1.8.0
	google.golang.org/grpc v1.76.0
	google.golang.org/protobuf v1.36.10
)

require (
	github.com/andybalholm/brotli v1.1.0 // indirect
	github.com/antithesishq/antithesis-sdk-go v0.7.0-default-no-op // indirect
	github.com/google/go-tpm v0.9.8 // indirect
	github.com/inconshreveable/mousetrap v1.1.0 // indirect
	github.com/klauspost/compress v1.18.5 // indirect
	github.com/mattn/go-colorable v0.1.13 // indirect
	github.com/mattn/go-isatty v0.0.20 // indirect
	github.com/mattn/go-runewidth v0.0.16 // indirect
	github.com/minio/highwayhash v1.0.4 // indirect
	github.com/nats-io/jwt/v2 v2.8.1 // indirect
	github.com/nats-io/nkeys v0.4.15 // indirect
	github.com/nats-io/nuid v1.0.1 // indirect
	github.com/rivo/uniseg v0.2.0 // indirect
	github.com/spf13/pflag v1.0.5 // indirect
	github.com/valyala/bytebufferpool v1.0.0 // indirect
	github.com/valyala/fasthttp v1.51.0 // indirect
	github.com/valyala/tcplisten v1.0.0 // indirect
	golang.org/x/crypto v0.50.0 // indirect
	golang.org/x/net v0.52.0 // indirect
	golang.org/x/sys v0.43.0 // indirect
	golang.org/x/text v0.36.0 // indirect
	golang.org/x/time v0.15.0 // indirect
	google.golang.org/genproto/googleapis/rpc v0.0.0-20250804133106-a7a43d27e69b // indirect
	gopkg.in/yaml.v3 v3.0.1 // indirect
)

replace (
	github.com/quarkloop/cli v0.0.0 => ../cli
	github.com/quarkloop/pkg/boundary v0.0.0 => ../pkg/boundary
	github.com/quarkloop/supervisor v0.0.0 => ../supervisor
)

replace gopkg.in/kr/pretty.v0 => github.com/kr/pretty v0.3.1

replace github.com/quarkloop/pkg/plugin v0.0.0 => ../pkg/plugin

replace github.com/quarkloop/pkg/serviceapi v0.0.0 => ../pkg/serviceapi

replace github.com/quarkloop/pkg/event => ../pkg/event

replace github.com/quarkloop/pkg/space v0.0.0 => ../pkg/space
