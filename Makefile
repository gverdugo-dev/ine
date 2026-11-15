# Operational commands for ine.

.DEFAULT_GOAL := help

.PHONY: help
help: ## Show this help
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) \
		| awk 'BEGIN {FS = ":.*?## "}; {printf "  \033[36m%-10s\033[0m %s\n", $$1, $$2}'

.PHONY: build
build: ## Compile the CLI into bin/ine
	go build -o bin/ine ./cmd/ine

.PHONY: install
install: ## Install the CLI into GOBIN
	go install ./cmd/ine

# gofmt -l only lists files, so the check fails when the list is not empty.
.PHONY: test
test: ## Run vet, the gofmt check and the tests with the race detector
	go vet ./...
	@files=$$(gofmt -l .); if [ -n "$$files" ]; then echo "gofmt needed on:"; echo "$$files"; exit 1; fi
	go test ./... -race
