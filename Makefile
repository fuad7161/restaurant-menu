.PHONY: up down logs build run test fmt psql

# The laptop's Wi-Fi address, found fresh on every start, so QR codes keep working when the router gives a new IP.
LAN_IP := $(shell ip -4 route get 1.1.1.1 2>/dev/null | sed -n 's/.* src \([0-9.]*\).*/\1/p')

up:        ## start everything in Docker; QR codes point at this laptop's Wi-Fi address unless BASE_URL is set in .env
	@set -a; [ -f .env ] && . ./.env; set +a; \
	if [ -z "$$BASE_URL" ] && [ -n "$(LAN_IP)" ]; then export BASE_URL="http://$(LAN_IP):$${APP_PORT:-8080}"; fi; \
	echo "QR codes and phone address: $${BASE_URL:-http://localhost:$${APP_PORT:-8080}}"; \
	docker compose up --build -d

down:      ## stop everything (data is kept in volumes)
	docker compose down

logs:      ## follow the app log
	docker compose logs -f app

build:     ## compile the binary to bin/bosho
	go build -o bin/bosho ./cmd/bosho

run:       ## run the app on the host against the Docker Postgres and MinIO
	docker compose up -d postgres minio
	set -a; [ -f .env ] && . ./.env; set +a; \
	DATABASE_URL="postgres://$${POSTGRES_USER:-bosho}:$${POSTGRES_PASSWORD:-bosho}@localhost:$${POSTGRES_PORT:-5432}/$${POSTGRES_DB:-bosho}?sslmode=disable" \
	S3_ENDPOINT="localhost:$${MINIO_PORT:-9000}" S3_ACCESS_KEY="$${MINIO_ROOT_USER:-bosho}" S3_SECRET_KEY="$${MINIO_ROOT_PASSWORD:-bosho-secret}" \
	go run ./cmd/bosho

test:
	go vet ./... && go test ./...

fmt:
	gofmt -w cmd internal web

psql:      ## open a database shell
	docker compose exec postgres psql -U $${POSTGRES_USER:-bosho} $${POSTGRES_DB:-bosho}
