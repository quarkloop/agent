module github.com/quarkloop/pkg/serviceapi

go 1.26

require (
	github.com/quarkloop/pkg/boundary v0.0.0
	google.golang.org/protobuf v1.36.10
)

require (
	github.com/kr/text v0.2.0 // indirect
	github.com/quarkloop/pkg/plugin v0.0.0 // indirect
	gopkg.in/yaml.v3 v3.0.1 // indirect
)

replace (
	github.com/quarkloop/pkg/boundary v0.0.0 => ../boundary
	github.com/quarkloop/pkg/plugin v0.0.0 => ../plugin
)
