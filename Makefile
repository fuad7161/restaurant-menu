.PHONY: up down logs build run test fmt psql backup restore-check loadtest create-admin

# The laptop's Wi-Fi address, found fresh on every start, so QR codes keep working when the router gives a new IP.
LAN_IP := $(shell ip -4 route get 1.1.1.1 2>/dev/null | sed -n 's/.* src \([0-9.]*\).*/\1/p')

up:        ## start the app in Docker; QR codes point at this laptop's Wi-Fi address unless BASE_URL is set in .env
	@set -a; [ -f .env ] && . ./.env; set +a; \
	if [ -z "$$BASE_URL" ] && [ -n "$(LAN_IP)" ]; then export BASE_URL="http://$(LAN_IP):$${APP_PORT:-8081}"; fi; \
	echo "QR codes and phone address: $${BASE_URL:-http://localhost:$${APP_PORT:-8081}}"; \
	docker compose up --build -d

down:      ## stop the app (data lives online, nothing is lost)
	docker compose down

logs:      ## follow the app log
	docker compose logs -f app

build:     ## compile the binary to bin/bosho
	go build -o bin/bosho ./cmd/bosho

run:       ## run the app on the host, without Docker, against the online services in .env
	@set -a; [ -f .env ] && . ./.env; set +a; \
	if [ -z "$$BASE_URL" ] && [ -n "$(LAN_IP)" ]; then export BASE_URL="http://$(LAN_IP):$${APP_PORT:-8081}"; fi; \
	ADDR=":$${APP_PORT:-8081}" go run ./cmd/bosho

test:
	go vet ./... && go test ./...

fmt:
	gofmt -w cmd internal web

create-admin: ## add a platform admin, or set a new password for one: make create-admin EMAIL=you@example.com
	@set -a; [ -f .env ] && . ./.env; set +a; go run ./cmd/bosho create-admin "$(EMAIL)"

psql:      ## open a shell on the online database (needs psql installed)
	@set -a; . ./.env; set +a; psql "$$DATABASE_URL"

backup:    ## save the online database to backups/ and check the file (needs pg_dump 17+); run it nightly
	./scripts/backup.sh

restore-check: ## restore the newest backup into a throwaway local PostgreSQL in Docker and count the rows
	./scripts/restore-check.sh

loadtest:  ## simulate a busy evening against a running app: make loadtest URL=http://localhost:8081
	go run ./cmd/loadtest -url $${URL:-http://localhost:8081}
