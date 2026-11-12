# Bosho (বসো)

QR table ordering for restaurants. A guest scans the QR on the table, orders from the phone browser, watches the order status, asks for the bill, and leaves a review.

Go + htmx, PostgreSQL for data, MinIO for dish photos, all started with Docker Compose. The product plan is in `qr-table-ordering-plan.md`.

## Start

```sh
cp .env.example .env        # then change the passwords
docker compose up --build   # or: make up
```

Open http://localhost:8080. With `DEMO=1` in `.env`, a sample restaurant is created the first time:

- Owner login: phone `01700000000`, password `demo1234`
- Guest link for table 1: http://localhost:8080/t/demo-table-1

Set `DEMO=0` and use `/signup` for a real restaurant.

Start with `make up` to use the QR codes from a phone on the same Wi-Fi. It finds the laptop's Wi-Fi IP on every start and puts it into the QR codes, as long as `BASE_URL` is empty in `.env`. Plain `docker compose up` does not do this.

## Services

| Service | Image | Purpose | Host address |
| --- | --- | --- | --- |
| `app` | built from `Dockerfile` | The web server | `localhost:8080` |
| `postgres` | `postgres:17-alpine` | All data | `127.0.0.1:5432` |
| `minio` | `cgr.dev/chainguard/minio` | Dish photos (S3 API) | API `127.0.0.1:9000`, console `127.0.0.1:9001` |

Data lives in the Docker volumes `postgres-data` and `minio-data`, so it survives `docker compose down`. `docker compose down -v` deletes it.

MinIO stopped publishing images on Docker Hub, so the Compose file uses Chainguard's build of the same server. Any S3-compatible store works; only the `S3_*` settings change.

## Settings

Compose reads `.env` (see `.env.example`). The app itself reads these variables:

| Variable | Default | Meaning |
| --- | --- | --- |
| `ADDR` | `:8080` | Listen address |
| `BASE_URL` | request host | Public address printed into QR codes, for example `https://bosho.app` |
| `DATABASE_URL` | local Postgres | PostgreSQL connection string |
| `S3_ENDPOINT` | `localhost:9000` | MinIO or S3 host and port |
| `S3_ACCESS_KEY`, `S3_SECRET_KEY` | `bosho`, `bosho-secret` | Object storage credentials |
| `S3_BUCKET` | `bosho-photos` | Bucket for photos; created when missing |
| `S3_USE_SSL` | off | `1` when the object store uses HTTPS |
| `DEMO` | off | `1` seeds the sample restaurant |

## Project layout

```
cmd/bosho/            entry point: load config, connect, start the server
internal/
  config/             settings from environment variables
  database/           PostgreSQL connection, query helpers, migration runner
    migrations/       SQL files, applied in name order at startup
  storage/            photo upload and serving through MinIO
  money/              taka and poisha parsing, formatting, percentages
  i18n/               Bangla and English texts for guest screens
  server/             HTTP handlers
    server.go           app wiring, routes, rendering
    models.go           shared types
    queries.go          shared queries: menu, orders, bill, ready-time estimate
    guest.go            scanning, groups, joining a table
    guest_cart.go       shared cart and placing an order
    guest_orders.go     order status, changes, bill, review
    auth.go             staff login, signup, sessions
    kitchen.go          kitchen board, order and item actions, sold-out list
    counter.go          open bills, mark paid, close
    waiter.go           ordering on behalf of guests
    owner_*.go          dashboard, menu, tables and QR, staff, settings
    demo.go             sample data
web/
  templates/          HTML pages and htmx fragments
  static/             CSS, htmx, polling and alert-sound script
Dockerfile, docker-compose.yml, Makefile, .env.example
```

Handlers keep their SQL next to the code that uses it; queries shared by several screens are in `queries.go`.

## Development

| Command | Does |
| --- | --- |
| `make up` / `make down` | Start or stop the whole stack |
| `make logs` | Follow the app log |
| `make run` | Run the app on the host against the Docker Postgres and MinIO |
| `make test` | `go vet` and unit tests |
| `make psql` | Open a database shell |

To change the schema, add a new file such as `internal/database/migrations/0002_add_tips.sql`. It runs once at the next start. Never edit a migration that has already run.

Queries are written with `?` placeholders; `internal/database` rewrites them to PostgreSQL's `$1` form.

## Screens

| Who | Where | What |
| --- | --- | --- |
| Guest | `/t/<token>` (from the QR) | Menu, shared cart, order status, bill, call waiter, review. Bangla and English. |
| Kitchen / waiter | `/staff/kitchen` | New, Cooking, Ready columns; accept with a ready time, reject, change requests, waiter calls, sound alert |
| Waiter | `/staff/tables` | Order on behalf of guests who do not scan |
| Any staff | `/staff/stock` | Mark dishes sold out |
| Cashier | `/staff/counter` | Open bills, remove items with a reason, mark paid, close without payment |
| Owner | `/owner` | Sales today, best sellers, reviews, menu, tables and printable QR cards, staff, settings |

## Rules built in

- The bill belongs to a group, not a table. A second scan on a busy table chooses "join" (needs an Allow tap from the group) or "separate order" (Group B with its own bill).
- Each guest order waits for a staff Accept by default (Settings can turn this off). Orders entered by a waiter skip the wait.
- A guest can change or cancel an item freely until the kitchen accepts. After that it is a request the staff approve or decline. After serving, only staff can remove it, with a recorded reason.
- Prices, VAT and service charge are always computed on the server. Money is stored in poisha.
- Every query is filtered by restaurant, so one installation serves many restaurants.

## Not built yet

Online payment, split bill, tips, printing, subscription billing, image compression, login rate limiting, HTTPS (put a reverse proxy such as Caddy in front for production). Screens refresh by polling every 3 to 4 seconds.
# restaurant-menu
