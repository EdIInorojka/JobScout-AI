GO ?= go

.PHONY: fmt lint test build migrate serve test-integration test-hh

fmt:
	$(GO) fmt ./...

lint:
	$(GO) vet ./...

test:
	$(GO) test ./...

test-integration:
	@if [ -z "$(TEST_DATABASE_URL)" ]; then echo "TEST_DATABASE_URL is required"; exit 1; fi
	$(GO) test -count=1 -p=1 -tags=integration ./internal/store/postgres ./internal/app

test-hh:
	$(GO) test -count=1 -tags=integration ./internal/integrations/hh

build:
	$(GO) build ./...

migrate:
	$(GO) run ./cmd/jobscout migrate

serve:
	$(GO) run ./cmd/jobscout serve
