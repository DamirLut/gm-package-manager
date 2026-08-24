DATABASE_PATH ?= ./storage/metadata.db
GOOSE_DIR := internal/database/migrations

.PHONY: run build test vet fmt tidy clean migrate-new migrate-up migrate-down migrate-status db-shell

run:
	go run ./cmd/main

build:
	go build -o bin/server ./cmd/main

test:
	go test ./...

vet:
	go vet ./...

fmt:
	gofmt -w .

tidy:
	go mod tidy

clean:
	rm -rf bin

migrate-new:
	go tool goose -s -dir $(GOOSE_DIR) create $(name) sql

migrate-up:
	go tool goose -dir $(GOOSE_DIR) sqlite3 $(DATABASE_PATH) up

migrate-down:
	go tool goose -dir $(GOOSE_DIR) sqlite3 $(DATABASE_PATH) down

migrate-status:
	go tool goose -dir $(GOOSE_DIR) sqlite3 $(DATABASE_PATH) status

db-shell:
	sqlite3 $(DATABASE_PATH)
