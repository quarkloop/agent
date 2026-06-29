module github.com/quarkloop/pkg/serviceapi

go 1.26.4

require (
	github.com/quarkloop/pkg/boundary v0.0.0
	google.golang.org/protobuf v1.36.12-0.20260120151049-f2248ac996af
)

require (
	github.com/quarkloop/pkg/plugin v0.0.0 // indirect
	gopkg.in/yaml.v3 v3.0.1 // indirect
)

replace (
	github.com/quarkloop/pkg/boundary v0.0.0 => ../boundary
	github.com/quarkloop/pkg/plugin v0.0.0 => ../plugin
)
