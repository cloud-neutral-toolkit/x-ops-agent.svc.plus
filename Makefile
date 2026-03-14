.PHONY: run run-ops run-mcp register-openclaw test migrate deps tidy build clean-mod

# Run the API via unified binary
run:
	go run ./cmd/agent --mode api

# Run the OPS agent HTTP surface
run-ops:
	go run ./cmd/agent --mode ops --env-file .env

# Run the MCP server over stdio
run-mcp:
	go run ./cmd/agent --mode mcp --env-file .env

# Register or update the OPS agent in OpenClaw gateway
register-openclaw:
	go run ./cmd/agent --mode register --env-file .env

# Run all tests
test:
	go test ./...

# Run database migrations
migrate:
	migrate -path internal/migrations -database $$DATABASE_URL up

# Install dependencies and build
deps: tidy build

# Tidy go.mod / go.sum
tidy:
	@rm -f go.sum
	@go mod tidy

# Build project
build:
	@mkdir -p bin
	@go build -o bin/xopsagent ./cmd/agent

# Clean and re-download modules
clean-mod:
	@go clean -modcache
	@go mod download all
	@go mod verify
