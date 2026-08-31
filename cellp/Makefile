.PHONY: test build openapi-check tidy test-cover

# Default gate matches COVER_MIN=80 target (excludes internal/branch).
COVER_MIN ?= 80

test:
	go test ./...

COVER_PACKAGES := $(shell go list ./internal/... | grep -v '/branch$$')

test-cover:
	go test $(COVER_PACKAGES) -coverprofile=coverage.out -covermode=set
	@go tool cover -func=coverage.out | tail -1
	@pct=$$(go tool cover -func=coverage.out | awk '/^total:/ {print $$3}' | tr -d '%'); \
	if [ "$${pct%%.*}" -lt $(COVER_MIN) ]; then echo "coverage $${pct}% < $(COVER_MIN)% (see Makefile COVER_MIN)"; exit 1; fi
	@echo "coverage OK (>= $(COVER_MIN)%)"

build:
	go build -o cellpd ./cmd/cellpd
	go build -o cellp ./cmd/cellp

tidy:
	go mod tidy

openapi-check:
	@test -f api/openapi.yaml
	@grep -q 'createVersion' api/openapi.yaml
	@grep -q 'deployToken' api/openapi.yaml
	@grep -q '202' api/openapi.yaml
	@echo "openapi-check: OK"
