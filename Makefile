.PHONY: run test migrate

run:
	go run ./cmd/api

test:
	go test ./...

migrate:
	migrate -path migrations -database $$DATABASE_URL up
