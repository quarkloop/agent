module github.com/quarkloop/pkg/boundary

go 1.26.4

require github.com/quarkloop/pkg/plugin v0.0.0

require (
	github.com/kr/text v0.2.0 // indirect
	gopkg.in/yaml.v3 v3.0.1 // indirect
)

replace github.com/quarkloop/pkg/plugin v0.0.0 => ../plugin
