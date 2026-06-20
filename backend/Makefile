SHELL := /usr/bin/env bash

BINARY := transx
COVERPROFILE ?= /tmp/modules.noquery.cover
COVERAGE_MIN ?= 90.0

.PHONY: help sqlc mock format vet lint test test-integration coverage build check openapi migrate seed run-wallet run-replayer run-consumer run-stub-provider clean

help:
	@printf "Available targets:\n"
	@printf "  sqlc         Generate sqlc code\n"
	@printf "  mock         Generate mockery mocks into internal/testmocks\n"
	@printf "  format       Format Go code\n"
	@printf "  vet          Run go vet\n"
	@printf "  lint         Run golangci-lint (enforces module boundaries via depguard)\n"
	@printf "  test         Run go test ./...\n"
	@printf "  test-integration Run tagged integration tests (requires Docker)\n"
	@printf "  coverage     Run internal/modules tests excluding generated gen and sqlc query packages; require >= $(COVERAGE_MIN)%%\n"
	@printf "  build        Build transx binary\n"
	@printf "  check        Run sqlc, format, vet, lint, test, coverage\n"
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

mock:
	mockery --config .mockery.yaml --log-level=error

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

test:
	go test ./...

test-integration:
	go test -count=1 -tags=integration ./... -timeout 120s

coverage:
	@go list ./internal/modules/... ./cmd/consumer/... ./cmd/replayer/... | grep -Ev '(/query|/gen)' | xargs go test -coverprofile=$(COVERPROFILE)
	@coverage="$$(go tool cover -func=$(COVERPROFILE) | awk '/^total:/ {gsub(/%/,"",$$3); print $$3}')"; \
		echo "modules + workers coverage: $$coverage%"; \
		awk -v coverage="$$coverage" -v min="$(COVERAGE_MIN)" 'BEGIN { if (coverage + 0 < min + 0) exit 1 }'

build:
	go build -o bin/$(BINARY) .

check: sqlc format vet lint test coverage

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
