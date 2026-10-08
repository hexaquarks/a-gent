GO ?= go
BIN_DIR ?= $(HOME)/.local/bin
BINARY_NAME ?= a-gent
BINARY_PATH := $(BIN_DIR)/$(BINARY_NAME)
GO_FILES := $(shell find . -name '*.go' -not -path './vendor/*')

.PHONY: build install uninstall run test test-integration vet format check help

build: ## Compile the application without creating an output binary.
	$(GO) build ./cmd/a-gent

install: ## Install the application; override BIN_DIR to choose a destination.
	mkdir -p "$(BIN_DIR)"
	$(GO) build -o "$(BINARY_PATH)" ./cmd/a-gent

uninstall: ## Remove the binary installed by the install target.
	rm -f "$(BINARY_PATH)"

run: ## Run the application from the source checkout.
	$(GO) run ./cmd/a-gent

test: ## Run the Go test suite.
	$(GO) test ./...

test-integration: ## Run Go integration tests in isolated tmux terminals.
	$(GO) test -tags=integration -count=1 -timeout=90s -v ./internal/ui ./internal/tmux -run TestIntegration

vet: ## Run Go's static analysis.
	$(GO) vet ./...

format: ## Format all Go source files.
	gofmt -w $(GO_FILES)

check: ## Verify formatting, tests, and static analysis.
	@unformatted_files="$$(gofmt -l $(GO_FILES))"; \
	if [ -n "$$unformatted_files" ]; then \
		echo "Go files need formatting:"; \
		echo "$$unformatted_files"; \
		exit 1; \
	fi
	$(MAKE) test
	$(MAKE) vet

help: ## Display the available targets.
	@awk 'BEGIN {FS = ":.*##"}; /^[a-zA-Z_-]+:.*##/ {printf "%-10s %s\n", $$1, $$2}' $(MAKEFILE_LIST)
