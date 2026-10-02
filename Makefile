DATABASE_URL ?= postgres://postgres:dev@localhost:5432/booking?sslmode=disable
MIGRATIONS_DIR ?= migrations

.PHONY: build test vet run docker-up docker-down migrate-up migrate-down migrate-status migrate-reset migrate-create

build:
	go build ./...

test:
	go test ./...

vet:
	go vet ./...

run:
	go run ./cmd/server

# Migrasi via runner standalone (Goose)
migrate-up:
	go run ./cmd/migrate -dir=$(MIGRATIONS_DIR) -dsn="$(DATABASE_URL)" up

migrate-down:
	go run ./cmd/migrate -dir=$(MIGRATIONS_DIR) -dsn="$(DATABASE_URL)" down

migrate-status:
	go run ./cmd/migrate -dir=$(MIGRATIONS_DIR) -dsn="$(DATABASE_URL)" status

migrate-reset:
	go run ./cmd/migrate -dir=$(MIGRATIONS_DIR) -dsn="$(DATABASE_URL)" reset

migrate-create:
	@if [ -z "$(name)" ]; then echo "Usage: make migrate-create name=nama_migrasi"; exit 1; fi
	go run ./cmd/migrate -dir=$(MIGRATIONS_DIR) create $(name) sql

docker-up:
	docker compose up --build -d

docker-down:
	docker compose down -v
