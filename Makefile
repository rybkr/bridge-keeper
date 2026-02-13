.DEFAULT_GOAL := help

## Show this help message
.PHONY: help
help:
	@awk 'BEGIN {FS = ":.*$$"; printf "\nUsage:\n  make \033[36m<target>\033[0m\n\nTargets:\n"} /^## /{desc = substr($$0, 4)} /^[a-zA-Z_-]+:.*$$/ && desc {printf "  \033[36m%-12s\033[0m %s\n", $$1, desc; desc=""}' $(MAKEFILE_LIST)

## Run linters
.PHONY: lint
lint:
	go vet ./...

## Format source code
.PHONY: format
format:
	gofmt -w .

## Run all tests
.PHONY: test
test:
	go test ./...

## Run all CI checks (format, lint)
.PHONY: ci
ci: format lint test
