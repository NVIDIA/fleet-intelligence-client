# SPDX-FileCopyrightText: Copyright (c) 2026, NVIDIA CORPORATION & AFFILIATES. All rights reserved.
# SPDX-License-Identifier: Apache-2.0

SHELL := /bin/bash

BINARY_NAME := nvfleetint
BINARY_PATH := ./cmd/nvfleetint
BIN_DIR := bin
GO_FILES := $(shell find cmd internal nvfleetint -type f -name '*.go' | sort)
VERSION ?= dev
COMMIT ?= $(shell git rev-parse --short HEAD 2>/dev/null || echo unknown)
BUILD_DATE ?= $(shell date -u +%Y-%m-%dT%H:%M:%SZ)
OPENAPI_SPEC := api/openapi/openapi.yaml
OPENAPI_CODEGEN_CONFIG := api/openapi/oapi-codegen.yaml
OPENAPI_CODEGEN := go run github.com/oapi-codegen/oapi-codegen/v2/cmd/oapi-codegen@v2.8.0
COVERAGE_FILE := coverage.out
COVERAGE_THRESHOLD := 80
# Packages excluded from coverage: generated code is not hand-tested, and
# cmdtest is fixtures for the other packages' tests rather than shipped code.
COVERAGE_EXCLUDE_PATTERN := internal/generated|internal/cmdtest

LDFLAGS := -s -w \
	-X 'main.version=$(VERSION)' \
	-X 'main.commit=$(COMMIT)' \
	-X 'main.buildDate=$(BUILD_DATE)'

.PHONY: build
build: ## Build nvfleetint
	@mkdir -p $(BIN_DIR)
	go build -ldflags "$(LDFLAGS)" -o $(BIN_DIR)/$(BINARY_NAME) $(BINARY_PATH)

.PHONY: test
test: ## Run unit tests
	go test ./...

.PHONY: test-coverage
test-coverage: ## Run tests + enforce $(COVERAGE_THRESHOLD)% coverage gate
	@packages=$$(go list ./... | grep -v -E "$(COVERAGE_EXCLUDE_PATTERN)"); \
	go test $$packages -coverprofile=$(COVERAGE_FILE) -covermode=atomic
	@go tool cover -func=$(COVERAGE_FILE) | tail -1
	@total=$$(go tool cover -func=$(COVERAGE_FILE) | grep '^total:' | grep -Eo '[0-9]+\.[0-9]+'); \
	echo "Total coverage: $$total% (threshold: $(COVERAGE_THRESHOLD)%)"; \
	if awk "BEGIN {exit !($$total < $(COVERAGE_THRESHOLD))}"; then \
		echo "FAIL: coverage $$total% is below the $(COVERAGE_THRESHOLD)% threshold"; \
		exit 1; \
	fi

.PHONY: lint
lint: ## Run formatting check, go vet, and golangci-lint
	@unformatted="$$(gofmt -l $(GO_FILES))"; \
	if [ -n "$$unformatted" ]; then \
		echo "$$unformatted"; \
		exit 1; \
	fi
	go vet ./...
	golangci-lint run ./...

.PHONY: check
check: lint test-coverage build skills-check ## Run all validation (enforces the coverage threshold)

.PHONY: skills-check
skills-check: ## Validate portable Agent Skills metadata and bundled references
	./scripts/validate-skills.sh

.PHONY: setup-git-hooks
setup-git-hooks: ## Configure local git hooks
	@if [ ! -d ".git" ]; then \
		echo "This is not a git repository. Run from the project root."; \
		exit 1; \
	fi
	git config core.hooksPath .git-hooks
	chmod +x .git-hooks/pre-commit .git-hooks/commit-msg
	@echo "Git hooks configured."

.PHONY: test-git-hooks
test-git-hooks: ## Run git hooks manually
	./.git-hooks/pre-commit
	@tmpfile=$$(mktemp); \
	printf '%s\n' 'chore: validate commit message hook' > "$$tmpfile"; \
	./.git-hooks/commit-msg "$$tmpfile"; \
	rm -f "$$tmpfile"

.PHONY: fmt
fmt: ## Format Go files
	gofmt -w $(GO_FILES)

.PHONY: generate
generate: ## Generate OpenAPI client code
	$(OPENAPI_CODEGEN) -config $(OPENAPI_CODEGEN_CONFIG) $(OPENAPI_SPEC)

.PHONY: clean
clean: ## Remove local build artifacts
	rm -rf $(BIN_DIR) dist coverage.out

.PHONY: help
help: ## Show available targets
	@awk 'BEGIN {FS = ":.*##"} /^[a-zA-Z_-]+:.*##/ {printf "%-12s %s\n", $$1, $$2}' $(MAKEFILE_LIST)
