# Disable all the default make stuff
MAKEFLAGS += --no-builtin-rules
.SUFFIXES:

## Display help menu
.PHONY: help
help:
	@echo Documented Make targets:
	@perl -e 'undef $$/; while (<>) { while ($$_ =~ /## (.*?)(?:\n# .*)*\n.PHONY:\s+(\S+).*/mg) { printf "\033[36m%-30s\033[0m %s\n", $$2, $$1 } }' $(MAKEFILE_LIST) | sort

## Install dependencies
.PHONY: install
install:
	go mod download

## Generate mocks
.PHONY: generate
generate:
	go generate -v ./...

## Build binary
.PHONY: build
build:
	go build -o octl

TEST_PACKAGES = $$(go list ./... | grep -v -E "(mocks|generated|clients/platform-orchestrator-cp|clients/platform-orchestrator-dp|clients/platform-orchestrator-iam)")

## Run tests with coverage
.PHONY: test
test:
	go tool gotestsum --format testname -- -coverprofile=cover.out $(TEST_PACKAGES) && go tool cover -func=cover.out

## Generate coverage badge
.PHONY: coverage-badge
coverage-badge: test
	@COVERAGE=$$(go tool cover -func=cover.out | tail -1 | awk '{print $$3}' | sed 's/%//' | cut -d'.' -f1); \
		if [ $$COVERAGE -ge 80 ]; then COLOR="brightgreen"; \
		elif [ $$COVERAGE -ge 60 ]; then COLOR="yellow"; \
		elif [ $$COVERAGE -ge 40 ]; then COLOR="orange"; \
		else COLOR="red"; fi; \
	curl -s "https://img.shields.io/badge/coverage-$$COVERAGE%25-$$COLOR" -o badge.svg

## Run linter
.PHONY: lint
lint:
	golangci-lint run
