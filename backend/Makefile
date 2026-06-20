SHELL := /usr/bin/env bash

BINARY := transx
COVERPROFILE ?= /tmp/modules.noquery.cover
COVERAGE_MIN ?= 90.0

.PHONY: help sqlc format vet lint build check openapi migrate seed run-wallet run-replayer run-consumer run-stub-provider clean

help:
	@printf "Available targets:\n"
	@printf "  sqlc         Generate sqlc code\n"
	@printf "  format       Format Go code\n"
	@printf "  vet          Run go vet\n"
	@printf "  lint         Run golangci-lint (enforces module boundaries via depguard)\n"
	@printf "  build        Build transx binary\n"
	@printf "  check        Run sqlc, fmt, vet, lint\n"
	@printf "  openapi      Generate openapi.yaml\n"
	@printf "  migrate      Apply database migrations\n"
	@printf "  seed         Insert development users\n"
	@printf "  run-wallet   Run wallet HTTP API (API only)\n"
	@printf "  run-replayer Run outbox-replayer (drain outbox to Kafka)\n"
	@printf "  run-consumer Run transfer consumer (processor + provider + retries)\n"
	@printf "  run-stub-provider Run stub payment provider HTTP service\n"
	@printf "  clean        Remove local build artifacts\n"

sqlc:
	sqlc generate

format:
	@echo "🎨 Formatting all Go files..."
	@gofmt -w .
	@echo "📦 Organizing imports..."
	@goimports -w .
	@echo "📏 Formatting line lengths..."
	@golines -w -m 120 .
	@echo "✨ Applying gofumpt formatting..."
	@gofumpt -extra -w .
	@echo "✅ Go files formatted successfully"


vet:
	go vet ./...

lint:
	golangci-lint run ./...

build:
	go build -o bin/$(BINARY) .

check: sqlc format vet lint

openapi:
	go run . openapi-export --output openapi.yaml

migrate:
	go run . --config config.yaml migrate up

seed:
	go run . --config config.yaml seed

run-wallet:
	go run . --config config.yaml wallet

run-replayer:
	HTTP__ADDRESS=:4001 go run . --config config.yaml outbox-replayer

run-consumer:
	HTTP__ADDRESS=:4002 go run . --config config.yaml consumer

run-stub-provider:
	go run . --config config.yaml stub-provider

clean:
	rm -rf bin/
