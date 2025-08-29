DATABASE_URL ?= postgres://postgres:postgres@localhost:5432/ops?sslmode=disable

migrate:
	migrate -path db/migrations -database $(DATABASE_URL) up

sqlc:
	sqlc generate
