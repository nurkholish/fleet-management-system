COMPOSE_FILE := deploy/docker-compose.yml
BACKEND_BIN  := bin/backend
PUBLISHER_BIN:= bin/publisher
WORKER_BIN   := bin/worker

.PHONY: help up down restart ps logs logs-backend logs-worker logs-publisher \
        db-count db-latest mqtt-sub rabbit-queues rabbit-dlq \
        build run-backend run-publisher run-worker \
        test test-cover tidy clean urls dev fresh

help:
	@echo ""
	@echo "Fleet — Makefile"
	@echo "============================="
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | \
		awk 'BEGIN{FS=":.*?## "}{printf "  \033[36m%-22s\033[0m %s\n", $$1, $$2}'
	@echo ""

up: ## Start full stack
	docker compose -f $(COMPOSE_FILE) up --build -d
	@$(MAKE) --no-print-directory urls

down: ## Stop and remove containers + volumes
	docker compose -f $(COMPOSE_FILE) down -v

restart: down up

ps:
	@docker compose -f $(COMPOSE_FILE) ps

logs:
	docker compose -f $(COMPOSE_FILE) logs -f

logs-backend:
	docker compose -f $(COMPOSE_FILE) logs backend --tail 50 -f

logs-worker:
	docker compose -f $(COMPOSE_FILE) logs worker --tail 50 -f

logs-publisher:
	docker compose -f $(COMPOSE_FILE) logs publisher --tail 50 -f

db-count:
	@docker exec -it fleet-postgres psql -U fleet -d fleet_db -t -c \
		"SELECT COUNT(*) FROM vehicle_locations;"

db-latest:
	@docker exec -it fleet-postgres psql -U fleet -d fleet_db -c \
		"SELECT vehicle_id, latitude, longitude, timestamp \
		 FROM vehicle_locations ORDER BY timestamp DESC LIMIT 10;"

mqtt-sub:
	docker exec -it fleet-mosquitto \
		mosquitto_sub -h localhost -t '/fleet/vehicle/+/location' -v

rabbit-queues:
	docker exec -it fleet-rabbitmq rabbitmqctl list_queues name messages

rabbit-dlq:
	docker exec -it fleet-rabbitmq rabbitmqctl list_queues name messages | grep dlq

build: ## Build all binaries
	@mkdir -p bin
	go build -o $(BACKEND_BIN)   ./cmd/backend
	go build -o $(PUBLISHER_BIN) ./cmd/publisher
	go build -o $(WORKER_BIN)    ./cmd/worker

run-backend:   ; go run ./cmd/backend
run-publisher: ; go run ./cmd/publisher
run-worker:    ; go run ./cmd/worker

test: ## Run tests with race detector
	go test ./... -race -cover

test-cover: ## HTML coverage report
	go test ./... -race -coverprofile=coverage.out
	go tool cover -html=coverage.out

tidy:
	go mod tidy

clean:
	rm -rf bin/ tmp/ coverage.out

urls:
	@echo ""
	@echo "Service URLs"
	@echo "============"
	@echo "  Backend API      →  http://localhost:8080"
	@echo "  Health           →  http://localhost:8080/healthz"
	@echo "  Ready            →  http://localhost:8080/readyz"
	@echo "  Geofences        →  http://localhost:8080/geofences"
	@echo "  RabbitMQ UI      →  http://localhost:15672  (fleet/fleet_secret)"
	@echo "  Adminer          →  http://localhost:8081"
	@echo "  Dozzle           →  http://localhost:8083"
	@echo ""

dev: ## Start infra + backend + worker + publisher
	docker compose -f $(COMPOSE_FILE) up -d postgres rabbitmq mosquitto backend worker publisher
	@$(MAKE) --no-print-directory urls

fresh: down up