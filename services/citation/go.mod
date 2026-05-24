module github.com/quarkloop/services/citation

go 1.26

require (
	github.com/quarkloop/pkg/serviceapi v0.0.0
)

require (
	golang.org/x/net v0.51.0 // indirect
	golang.org/x/sys v0.41.0 // indirect
	golang.org/x/text v0.34.0 // indirect
	google.golang.org/genproto/googleapis/rpc v0.0.0-20250804133106-a7a43d27e69b // indirect
	google.golang.org/protobuf v1.36.10 // indirect
)

replace github.com/quarkloop/pkg/serviceapi v0.0.0 => ../../pkg/serviceapi
