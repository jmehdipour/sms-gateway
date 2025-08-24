APP := sms-gateway
CONFIG ?= config.yaml

.PHONY: help run-server test build run-worker run-worker-normal run-worker-express migrate seed up down

help:
	@echo "Targets:"
	@echo "  make run-server         - Run HTTP server locally"
	@echo "  make test               - Run all tests"
	@echo "  make build              - Build binary into ./bin/$(APP)"
	@echo "  make run-sender-normal  - Run sender worker (normal)"
	@echo "  make run-sender-express - Run sender worker (express)"
	@echo "  make migrate            - Run MySQL migrations"
	@echo "  make seed               - Seed demo customers"
	@echo "  make up                 - Start docker-compose services"
	@echo "  make down               - Stop docker-compose services"

run-server:
	@echo ">> Running $(APP) HTTP server..."
	go run . serve --config=$(CONFIG)

test:
	@echo ">> Running tests..."
	go test ./... -v

build:
	@echo ">> Building $(APP)..."
	mkdir -p bin
	go build -o bin/$(APP) .

run-sender-normal:
	@echo ">> Sender (normal)"
	go run . worker sender normal --config=$(CONFIG)

run-sender-express:
	@echo ">> Sender (express)"
	go run . worker sender express --config=$(CONFIG)

migrate:
	@echo ">> Running migrations..."
	go run . migrate --config=$(CONFIG)

seed:
	@echo ">> Seeding demo customers..."
	go run . seed --config=$(CONFIG)

# Docker compose helpers
up:
	@echo ">> Starting docker-compose services..."
	docker-compose up -d

down:
	@echo ">> Stopping docker-compose services..."
	docker-compose down