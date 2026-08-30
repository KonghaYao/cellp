.PHONY: test build openapi-check tidy

test:
	go test ./...

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
