.DEFAULT_GOAL := help

GO ?= go
BIN_DIR ?= bin
GOCACHE ?= /tmp/bridgekeeper-go-cache
GO_TEST = GOCACHE=$(GOCACHE) $(GO) test
GO_BUILD = GOCACHE=$(GOCACHE) $(GO) build
GO_VET = GOCACHE=$(GOCACHE) $(GO) vet

BRIDGEKEEPER_BIN := $(BIN_DIR)/bridgekeeper
POLICYCHECK_BIN := $(BIN_DIR)/policycheck

## Show this help message
.PHONY: help
help:
	@awk 'BEGIN {FS = ":.*$$"; printf "\nUsage:\n  make \033[36m<target>\033[0m\n\nTargets:\n"} /^## /{desc = substr($$0, 4)} /^[a-zA-Z0-9_.-]+:.*$$/ && $$1 !~ /^\./ && desc {printf "  \033[36m%-14s\033[0m %s\n", $$1, desc; desc=""}' $(MAKEFILE_LIST)

## Create local build and cache directories
.PHONY: dirs
dirs:
	@mkdir -p $(BIN_DIR) $(GOCACHE)

## Format source code
.PHONY: format
format:
	gofmt -w .

## Check formatting without changing files
.PHONY: fmtcheck
fmtcheck:
	@test -z "$$(gofmt -l .)" || (echo "The following files need gofmt:" && gofmt -l . && exit 1)

## Run go vet
.PHONY: lint
lint:
	$(GO_VET) ./...

## Run all tests
.PHONY: test
test: dirs
	$(GO_TEST) ./...

## Run tests without cache reuse
.PHONY: test-clean
test-clean: dirs
	$(GO_TEST) -count=1 ./...

## Build all binaries into ./bin
.PHONY: build
build: $(BRIDGEKEEPER_BIN) $(POLICYCHECK_BIN)

$(BRIDGEKEEPER_BIN): dirs
	$(GO_BUILD) -o $@ ./cmd/bridgekeeper

$(POLICYCHECK_BIN): dirs
	$(GO_BUILD) -o $@ ./cmd/policycheck

## Run all local verification checks
.PHONY: verify
verify: fmtcheck lint test

## Run the same checks as verify and then build binaries
.PHONY: ci
ci: verify build

## Remove local build artifacts
.PHONY: clean
clean:
	rm -rf $(BIN_DIR)

## Remove local build artifacts and Go test cache used by this Makefile
.PHONY: distclean
distclean: clean
	rm -rf $(GOCACHE)
