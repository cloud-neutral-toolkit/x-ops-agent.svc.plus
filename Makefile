.PHONY: run test migrate deps tidy build clean-mod

# Run the API
run:
	go run ./cmd/api

# Run all tests
test:
	go test ./...

# Run database migrations
migrate:
	migrate -path migrations -database $$DATABASE_URL up

# Install dependencies and build
deps: tidy build

# Tidy go.mod / go.sum
tidy:
	@rm -f go.sum
	@go mod tidy

# Build project
build:
	@go build ./...

# Clean and re-download modules
clean-mod:
	@go clean -modcache
	@go mod download all
	@go mod verify
