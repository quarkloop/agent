module github.com/quarkloop/supervisor

go 1.26.4

require (
	github.com/go-git/go-git/v5 v5.17.2
	github.com/spf13/cobra v1.10.2
	google.golang.org/protobuf v1.36.12-0.20260120151049-f2248ac996af
)

require (
	dario.cat/mergo v1.0.0 // indirect
	github.com/Microsoft/go-winio v0.6.2 // indirect
	github.com/ProtonMail/go-crypto v1.1.6 // indirect
	github.com/antithesishq/antithesis-sdk-go v0.7.0-default-no-op // indirect
	github.com/cloudflare/circl v1.6.3 // indirect
	github.com/cyphar/filepath-securejoin v0.4.1 // indirect
	github.com/davecgh/go-spew v1.1.2-0.20180830191138-d8f796af33cc // indirect
	github.com/emirpasic/gods v1.18.1 // indirect
	github.com/go-git/gcfg v1.5.1-0.20230307220236-3a3c6141e376 // indirect
	github.com/go-git/go-billy/v5 v5.8.0 // indirect
	github.com/golang/groupcache v0.0.0-20241129210726-2c02b8208cf8 // indirect
	github.com/google/go-tpm v0.9.8 // indirect
	github.com/inconshreveable/mousetrap v1.1.0 // indirect
	github.com/jbenet/go-context v0.0.0-20150711004518-d14ea06fba99 // indirect
	github.com/kevinburke/ssh_config v1.2.0 // indirect
	github.com/klauspost/compress v1.18.6 // indirect
	github.com/minio/highwayhash v1.0.4 // indirect
	github.com/nats-io/jwt/v2 v2.8.1 // indirect
	github.com/nats-io/nkeys v0.4.15 // indirect
	github.com/nats-io/nuid v1.0.1 // indirect
	github.com/pjbgf/sha1cd v0.3.2 // indirect
	github.com/pmezard/go-difflib v1.0.1-0.20181226105442-5d4384ee4fb2 // indirect
	github.com/sergi/go-diff v1.3.2-0.20230802210424-5b0b94c5c0d3 // indirect
	github.com/skeema/knownhosts v1.3.1 // indirect
	github.com/spf13/pflag v1.0.9 // indirect
	github.com/stretchr/testify v1.11.1 // indirect
	github.com/xanzy/ssh-agent v0.3.3 // indirect
	golang.org/x/crypto v0.52.0 // indirect
	golang.org/x/net v0.55.0 // indirect
	golang.org/x/sys v0.45.0 // indirect
	golang.org/x/time v0.15.0 // indirect
	gopkg.in/warnings.v0 v0.1.2 // indirect
)

require (
	github.com/nats-io/nats-server/v2 v2.14.1
	github.com/nats-io/nats.go v1.52.0
	github.com/quarkloop/pkg/boundary v0.0.0
	github.com/quarkloop/pkg/event v0.0.0
	github.com/quarkloop/pkg/natskit v0.0.0
	github.com/quarkloop/pkg/plugin v0.0.0
	github.com/quarkloop/pkg/serviceapi v0.0.0
	github.com/quarkloop/pkg/space v0.0.0
	gopkg.in/yaml.v3 v3.0.1
)

replace (
	github.com/quarkloop/pkg/boundary => ../pkg/boundary
	github.com/quarkloop/pkg/event => ../pkg/event
	github.com/quarkloop/pkg/natskit => ../pkg/natskit
	github.com/quarkloop/pkg/plugin => ../pkg/plugin
	github.com/quarkloop/pkg/serviceapi => ../pkg/serviceapi
	github.com/quarkloop/pkg/space => ../pkg/space
)
