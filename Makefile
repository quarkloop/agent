BINARY_DIR := bin
PLUGIN_DIR := plugins
VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
LDFLAGS := -X github.com/quarkloop/cli/pkg/buildinfo.Version=$(VERSION)

# Tool plugins
TOOLS := bash fs web-search build-release

# Provider plugins
PROVIDERS := openrouter openai anthropic

# All modules for testing/vetting (18 modules; e2e tested separately)
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
		services/embedding \
		services/indexer \
		services/space \
		plugins/tools/bash \
		plugins/tools/fs \
		plugins/tools/web-search \
		plugins/tools/build-release \
		plugins/providers/openrouter \
		plugins/providers/openai \
		plugins/providers/anthropic

.PHONY: all build clean test test-e2e vet fmt fmt-check tidy proto arch-check \
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
		@echo "--- Building service: build-release ---"
		go build -o $(BINARY_DIR)/build-release-service ./services/build-release/cmd/build-release
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

## Run E2E tests (requires OPENROUTER_API_KEY or ZHIPU_API_KEY; loads quark/.env when present)
test-e2e:
		go test -tags e2e -v -timeout 20m ./e2e

## Check coarse package ownership and import boundaries
arch-check:
		python3 scripts/check-architecture.py

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
