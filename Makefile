GO ?= go

.PHONY: fmt lint test build migrate serve

fmt:
	$(GO) fmt ./...

lint:
	$(GO) vet ./...

test:
	$(GO) test ./...

build:
	$(GO) build ./...

migrate:
	$(GO) run ./cmd/jobscout migrate

serve:
	$(GO) run ./cmd/jobscout serve

