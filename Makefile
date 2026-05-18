BINARY_DIR := bin
PLUGIN_DIR := plugins
VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
LDFLAGS := -X github.com/quarkloop/cli/pkg/buildinfo.Version=$(VERSION)

# Tool plugins
TOOLS := bash fs web-search build-release

# Provider plugins
PROVIDERS := openrouter openai anthropic

# All modules for testing/vetting; e2e is tested separately.
MODULES := \
		supervisor \
		runtime \
		cli \
		pkg/boundary \
		pkg/plugin \
		pkg/serviceapi \
		pkg/space \
		pkg/toolkit \
		services/build-release \
		services/citation \
		services/core \
		services/devops \
		services/document \
		services/embedding \
		services/ingestion \
		services/indexer \
		services/model \
		services/space \
		plugins/tools/bash \
		plugins/tools/fs \
		plugins/tools/web-search \
		plugins/tools/build-release \
		plugins/providers/openrouter \
		plugins/providers/openai \
		plugins/providers/anthropic

.PHONY: all build clean test test-e2e test-e2e-local vet fmt fmt-check tidy proto arch-check boundary-check service-inventory service-inventory-check dead-code-check check release-check \
		build-supervisor build-runtime build-cli \
		build-plugins build-tools build-tools-lib build-providers build-services

all: build

## Build all binaries
build: build-supervisor build-runtime build-cli build-tools build-services

## Build all plugins (tools as binary + lib, providers as lib)
build-plugins: build-tools build-tools-lib build-providers

## Build tool plugins as binaries
build-tools:
		@for tool in $(TOOLS); do \
			echo "--- Building tool (binary): $$tool ---"; \
			go build -o $(BINARY_DIR)/$$tool ./$(PLUGIN_DIR)/tools/$$tool/cmd/$$tool; \
		done

build-services:
		@echo "--- Building service: indexer ---"
		go build -o $(BINARY_DIR)/indexer-service ./services/indexer/cmd/indexer
		@echo "--- Building service: embedding ---"
		go build -o $(BINARY_DIR)/embedding-service ./services/embedding/cmd/embedding
		@echo "--- Building service: ingestion ---"
		go build -o $(BINARY_DIR)/ingestion-service ./services/ingestion/cmd/ingestion
		@echo "--- Building service: build-release ---"
		go build -o $(BINARY_DIR)/build-release-service ./services/build-release/cmd/build-release
		@echo "--- Building service: citation ---"
		go build -o $(BINARY_DIR)/citation-service ./services/citation/cmd/citation
		@echo "--- Building service: core ---"
		go build -o $(BINARY_DIR)/core-service ./services/core/cmd/core
		@echo "--- Building service: devops ---"
		go build -o $(BINARY_DIR)/devops-service ./services/devops/cmd/devops
		@echo "--- Building service: document ---"
		go build -o $(BINARY_DIR)/document-service ./services/document/cmd/document
		@echo "--- Building service: model ---"
		go build -o $(BINARY_DIR)/model-service ./services/model/cmd/model
		@echo "--- Building service: space ---"
		go build -o $(BINARY_DIR)/space-service ./services/space/cmd/space

## Build tool plugins as .so files (lib mode, requires CGO)
build-tools-lib:
		@for tool in $(TOOLS); do \
			if [ -f $(PLUGIN_DIR)/tools/$$tool/plugin.go ]; then \
				echo "--- Building tool (lib): $$tool ---"; \
				CGO_ENABLED=1 go build -buildmode=plugin -tags plugin \
					-o $(PLUGIN_DIR)/tools/$$tool/plugin.so \
					./$(PLUGIN_DIR)/tools/$$tool; \
			fi; \
		done

## Build provider plugins as .so files (requires CGO)
build-providers:
		@for provider in $(PROVIDERS); do \
			echo "--- Building provider: $$provider ---"; \
			CGO_ENABLED=1 go build -buildmode=plugin -tags plugin \
				-o $(PLUGIN_DIR)/providers/$$provider/plugin.so \
				./$(PLUGIN_DIR)/providers/$$provider; \
		done

build-supervisor:
		go build -o $(BINARY_DIR)/supervisor ./supervisor/cmd/supervisor

build-runtime:
		go build -o $(BINARY_DIR)/runtime ./runtime/cmd/runtime

build-cli:
		go build -ldflags "$(LDFLAGS)" -o $(BINARY_DIR)/quark ./cli/cmd/quark

## Regenerate protobuf/gRPC Go stubs
proto:
		buf generate

## Run tests across all modules (providers are tested under the `plugin` build
## tag since all their sources are gated on it)
test:
		@set -e; for mod in $(MODULES); do \
			echo "--- Testing $$mod ---"; \
			case $$mod in \
				plugins/providers/*) (cd $$mod && go test -tags plugin ./...);; \
				*) (cd $$mod && go test ./...);; \
			esac; \
		done

## Run local deterministic E2E tests that do not require provider credentials
test-e2e-local:
		go test -tags e2e -v -timeout 12m -run '^(TestLongE2EPromptsAreOwnedByBuilders|TestPDFPromptBuildersExposeAgentWorkflowContract|TestMarkdownPromptBuildersExposeAgentWorkflowContract|TestBuildReleasePromptBuilderUsesServiceFunctionContract|TestSupervisorSessionEventReachesAgent|TestLocalDeterministicSupervisorRuntimeAndServices|TestIndexerServiceWithRealDgraph)$$' ./e2e

## Run E2E tests (requires OPENROUTER_API_KEY or ZHIPU_API_KEY; loads quark/.env when present)
test-e2e:
		go test -tags e2e -v -timeout 20m ./e2e

## Refresh the code-owned service implementation map
service-inventory:
		python3 scripts/service_inventory.py --write

## Verify the code-owned service implementation map is current
service-inventory-check:
		python3 scripts/service_inventory.py --check

## Check coarse package ownership and import boundaries
arch-check: service-inventory-check
		python3 scripts/check-architecture.py

boundary-check: arch-check

## Run focused dead-code/import hygiene checks across modules and E2E build tags
dead-code-check:
		@command -v staticcheck >/dev/null || { echo "staticcheck is required for dead-code-check"; exit 1; }
		@set -e; for mod in $(MODULES); do \
			echo "--- Dead-code check $$mod ---"; \
			case $$mod in \
				plugins/providers/*) (cd $$mod && staticcheck -tags plugin -checks=U1000 ./...);; \
				*) (cd $$mod && staticcheck -checks=U1000 ./...);; \
			esac; \
		done
		@echo "--- Dead-code check e2e ---"
		@(cd e2e && staticcheck -tags e2e -checks=U1000 ./...)

check: fmt-check vet test arch-check dead-code-check

## Run the release readiness gate, including local deterministic E2E
release-check:
		@go version | grep -q 'go1\.26' || { echo "Go 1.26 is required"; go version; exit 1; }
		$(MAKE) check
		$(MAKE) proto
		@git diff --exit-code -- proto pkg/serviceapi >/dev/null || { echo "generated protobuf/service API files are out of date"; exit 1; }
		$(MAKE) build
		$(MAKE) build-plugins
		$(MAKE) test-e2e-local
		@echo "release-check complete. Run provider-backed make test-e2e before release candidates when credentials/quota are available."

## Run vet across all modules (providers are vetted under the `plugin` build
## tag since all their sources are gated on it)
vet:
		@set -e; for mod in $(MODULES); do \
			echo "--- Vetting $$mod ---"; \
			case $$mod in \
				plugins/providers/*) (cd $$mod && go vet -tags plugin ./...);; \
				*) (cd $$mod && go vet ./...);; \
			esac; \
		done

## Run gofmt across all modules
fmt:
		@for mod in $(MODULES); do \
			echo "--- Formatting $$mod ---"; \
			(cd $$mod && gofmt -w .); \
		done

## Check formatting without modifying files (exits non-zero if any file is unformatted)
fmt-check:
		@unformatted=$$(for mod in $(MODULES); do (cd $$mod && gofmt -l .); done); \
		if [ -n "$$unformatted" ]; then \
			echo "Unformatted files:"; echo "$$unformatted"; exit 1; \
		fi

## Run go mod tidy across all modules
tidy:
		@set -e; for mod in $(MODULES); do \
			echo "--- Tidying $$mod ---"; \
			(cd $$mod && go mod tidy); \
		done

## Run staticcheck across all modules (providers linted under the `plugin` tag)
lint:
		@issues=0; for mod in $(MODULES); do \
			echo "--- Linting $$mod ---"; \
			case $$mod in \
				plugins/providers/*) out=$$(cd $$mod && staticcheck -tags plugin ./... 2>&1 | grep -v "^-");; \
				*) out=$$(cd $$mod && staticcheck ./... 2>&1 | grep -v "^-");; \
			esac; \
			if [ -n "$$out" ]; then echo "$$out"; issues=1; fi; \
		done; exit $$issues

## Remove built binaries and plugin .so files
clean:
		rm -rf $(BINARY_DIR)
		find $(PLUGIN_DIR) -name "*.so" -delete 2>/dev/null || true

$(BINARY_DIR):
		mkdir -p $(BINARY_DIR)

build-supervisor build-runtime build-cli build-tools build-services: | $(BINARY_DIR)

## Show available targets
help:
		@grep -E '^##' Makefile | sed 's/## //'
