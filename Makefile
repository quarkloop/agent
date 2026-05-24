BINARY_DIR := bin
PLUGIN_DIR := plugins
VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
LDFLAGS := -X github.com/quarkloop/cli/pkg/buildinfo.Version=$(VERSION)

# Tool plugins
TOOLS :=

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
		services/citation \
		services/core \
		services/devops \
		services/document \
		services/ingestion \
		services/indexer \
		services/gateway \
		services/secrets \
		services/space \
		services/system \
		services/workflow \
		services/io

.PHONY: all build clean test test-e2e test-e2e-local vet fmt fmt-check tidy proto arch-check boundary-check service-inventory service-inventory-check dead-code-check check release-check \
		build-supervisor build-runtime build-cli \
		build-plugins build-tools build-tools-lib build-services

all: build

## Build all binaries
build: build-supervisor build-runtime build-cli build-tools build-services

## Build all plugins
build-plugins: build-tools build-tools-lib

## Build tool plugins as binaries
build-tools:
		@if [ -z "$(strip $(TOOLS))" ]; then \
			echo "--- No tool plugins configured ---"; \
			exit 0; \
		fi
		@for tool in $(TOOLS); do \
			echo "--- Building tool (binary): $$tool ---"; \
			go build -o $(BINARY_DIR)/$$tool ./$(PLUGIN_DIR)/tools/$$tool/cmd/$$tool; \
		done

build-services:
		@echo "--- Building service: indexer ---"
		go build -o $(BINARY_DIR)/indexer-service ./services/indexer/cmd/indexer
		@echo "--- Building service: ingestion ---"
		go build -o $(BINARY_DIR)/ingestion-service ./services/ingestion/cmd/ingestion
		@echo "--- Building service: citation ---"
		go build -o $(BINARY_DIR)/citation-service ./services/citation/cmd/citation
		@echo "--- Building service: core ---"
		go build -o $(BINARY_DIR)/core-service ./services/core/cmd/core
		@echo "--- Building service: devops ---"
		go build -o $(BINARY_DIR)/devops-service ./services/devops/cmd/devops
		@echo "--- Building service: document ---"
		go build -o $(BINARY_DIR)/document-service ./services/document/cmd/document
		@echo "--- Building service: gateway ---"
		go build -o $(BINARY_DIR)/gateway-service ./services/gateway/cmd/gateway
		@echo "--- Building service: space ---"
		go build -o $(BINARY_DIR)/space-service ./services/space/cmd/space
		@echo "--- Building service: secrets ---"
		go build -o $(BINARY_DIR)/secrets-service ./services/secrets/cmd/secrets
		@echo "--- Building service: system ---"
		go build -o $(BINARY_DIR)/system-service ./services/system/cmd/system
		@echo "--- Building service: workflow ---"
		go build -o $(BINARY_DIR)/workflow-service ./services/workflow/cmd/workflow
		@echo "--- Building service: io ---"
		go build -o $(BINARY_DIR)/io-service ./services/io/cmd/io

## Build tool plugins as .so files (lib mode, requires CGO)
build-tools-lib:
		@if [ -z "$(strip $(TOOLS))" ]; then \
			echo "--- No tool plugins configured ---"; \
			exit 0; \
		fi
		@for tool in $(TOOLS); do \
			if [ -f $(PLUGIN_DIR)/tools/$$tool/plugin.go ]; then \
				echo "--- Building tool (lib): $$tool ---"; \
				CGO_ENABLED=1 go build -buildmode=plugin -tags plugin \
					-o $(PLUGIN_DIR)/tools/$$tool/plugin.so \
					./$(PLUGIN_DIR)/tools/$$tool; \
			fi; \
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

## Run tests across all modules.
test:
		@set -e; for mod in $(MODULES); do \
			echo "--- Testing $$mod ---"; \
			(cd $$mod && go test ./...); \
		done

## Run contract and service E2E tests that do not require model providers
test-e2e-local:
		go test -tags e2e -v -timeout 12m -run '^(TestLongE2EPromptsAreOwnedByBuilders|TestPDFPromptBuildersExposeAgentWorkflowContract|TestMarkdownPromptBuildersExposeAgentWorkflowContract|TestDevOpsReleasePromptBuilderUsesServiceFunctionContract|TestSupervisorSessionEventReachesAgent|TestIndexerServiceWithRealDgraph)$$' ./e2e

E2E_TIMEOUT ?= 20m
E2E_PDF_TIMEOUT ?= 20m

## Run E2E tests (requires OPENROUTER_API_KEY or ZHIPU_API_KEY; loads quark/.env when present)
test-e2e:
		go test -tags e2e -v -timeout $(E2E_PDF_TIMEOUT) -run '^TestAgentIndexesUploadedPDFDataset$$' ./e2e
		go test -tags e2e -v -timeout $(E2E_TIMEOUT) -skip '^TestAgentIndexesUploadedPDFDataset$$' ./e2e

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
			(cd $$mod && staticcheck -checks=U1000 ./...); \
		done
		@echo "--- Dead-code check e2e ---"
		@(cd e2e && staticcheck -tags e2e -checks=U1000 ./...)

check: fmt-check vet test arch-check dead-code-check

## Run the release readiness gate, including provider-free E2E coverage
release-check:
		@go version | grep -q 'go1\.26' || { echo "Go 1.26 is required"; go version; exit 1; }
		$(MAKE) check
		$(MAKE) proto
		@git diff --exit-code -- proto pkg/serviceapi >/dev/null || { echo "generated protobuf/service API files are out of date"; exit 1; }
		$(MAKE) build
		$(MAKE) build-plugins
		$(MAKE) test-e2e-local
		@echo "release-check complete. Run provider-backed make test-e2e before release candidates when credentials/quota are available."

## Run vet across all modules.
vet:
		@set -e; for mod in $(MODULES); do \
			echo "--- Vetting $$mod ---"; \
			(cd $$mod && go vet ./...); \
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

## Run staticcheck across all modules.
lint:
		@issues=0; for mod in $(MODULES); do \
			echo "--- Linting $$mod ---"; \
			out=$$(cd $$mod && staticcheck ./... 2>&1 | grep -v "^-"); \
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
