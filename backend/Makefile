SHELL := /usr/bin/env bash

BINARY := transx
COVERPROFILE ?= /tmp/modules.noquery.cover
COVERAGE_MIN ?= 90.0
SQL_FORMATTER ?= pg_format
SQL_FILES := $(shell find db internal -type f -name '*.sql' 2>/dev/null)

.PHONY: help sqlc proto mock format format-sql vet lint test test-integration coverage build check openapi migrate seed run-wallet run-replayer run-consumer run-notification run-stub-provider run-fx clean

help:
	@printf "Available targets:\n"
	@printf "  sqlc         Generate sqlc code\n"
	@printf "  proto        Generate gRPC code from proto/ with buf\n"
	@printf "  mock         Generate mockery mocks into internal/testmocks\n"
	@printf "  format       Format Go and SQL files\n"
	@printf "  format-sql   Format SQL files with $(SQL_FORMATTER)\n"
	@printf "  vet          Run go vet\n"
	@printf "  lint         Run golangci-lint (enforces module boundaries via depguard)\n"
	@printf "  test         Run unit tests with go test -short -p 1 ./...\n"
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
	@printf "  run-notification Run notification consumer for terminal transfer events\n"
	@printf "  run-stub-provider Run stub payment provider HTTP service\n"
	@printf "  run-fx       Run the FX gRPC service\n"
	@printf "  clean        Remove local build artifacts\n"

sqlc:
	sqlc generate

proto:
	buf generate

mock:
	mockery --config .mockery.yaml --log-level=error

format: format-sql
	@echo "🎨 Formatting all Go files..."
	@gofmt -w .
	@echo "📦 Organizing imports..."
	@goimports -w .
	@echo "📏 Formatting line lengths..."
	@golines -w -m 120 .
	@echo "✨ Applying gofumpt formatting..."
	@gofumpt -extra -w .
	@echo "✅ Go files formatted successfully"

format-sql:
	@if command -v $(SQL_FORMATTER) >/dev/null 2>&1; then \
		if [ -n "$(SQL_FILES)" ]; then \
			echo "🧹 Formatting SQL files..."; \
			$(SQL_FORMATTER) -i $(SQL_FILES); \
		fi; \
		echo "✅ SQL files formatted successfully"; \
	else \
		echo "⚠️  $(SQL_FORMATTER) not found; skipping SQL formatting (install pgFormatter / pg_format to enable)"; \
	fi

vet:
	go vet ./...

lint:
	golangci-lint run ./...

test:
	go test -short -p 1 ./...

test-integration:
	go test -count=1 -tags=integration -p 1 ./... -timeout 120s

coverage:
	@go list ./internal/modules/... ./cmd/consumer/... ./cmd/notification/... ./cmd/replayer/... ./internal/common/consumerretry/... | grep -Ev '(/query|/gen)' | xargs go test -coverprofile=$(COVERPROFILE)
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

run-notification:
	HTTP__ADDRESS=:4003 go run . --config config.yaml notification

run-stub-provider:
	go run . --config config.yaml stub-provider

run-fx:
	go run . --config config.yaml fx

clean:
	rm -rf bin/
