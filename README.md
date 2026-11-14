# Bosho (বসো)

QR table ordering for restaurants. A guest scans the QR on the table, orders from the phone browser, watches the order status, asks for the bill, and leaves a review.

Go + htmx. Data, photos and Redis are free online services, so only the app runs locally (or in Docker). The product plan is in `qr-table-ordering-plan.md`.

## Start

1. Create the online services (all have free plans):
   - **PostgreSQL**: [Supabase](https://supabase.com) -> New project -> **Connect** -> copy the **Session pooler** string and fill in the password.
   - **Photos**: [Backblaze B2](https://www.backblaze.com/sign-up/cloud-storage) (10 GB free, no card) -> **Buckets** -> **Create a Bucket** (unique name, **Private**) and note its **Endpoint**, for example `s3.us-east-005.backblazeb2.com`. Then **Application Keys** -> **Add a New Application Key** with **Read and Write** on that bucket. Copy the **keyID** and **applicationKey** (shown only once).
   - **Redis**: [Upstash](https://upstash.com) -> create database -> copy the `rediss://` URL. Not used by the app yet.
2. Fill in `.env`:

```sh
cp .env.example .env        # then paste the URLs and keys
make run                    # run with Go on this machine
# or: make up               # run in Docker
```

Open http://localhost:8081. With `DEMO=1` in `.env`, a sample restaurant is created the first time:

- Owner login: phone `01700000000`, password `demo1234`
- Guest link for table 1: http://localhost:8081/t/demo-table-1

Set `DEMO=0` and use `/signup` for a real restaurant.

`make run` and `make up` find the laptop's Wi-Fi IP on every start and put it into the QR codes, as long as `BASE_URL` is empty in `.env`, so phones on the same Wi-Fi can scan them.

Tables are created by the migrations on the first start. Any S3-compatible store works for photos (Cloudflare R2, AWS S3, a self-hosted MinIO); only the `S3_*` settings change.

## Settings

`make run`, `make up` and Compose read `.env` (see `.env.example`). The app itself reads these variables:

| Variable | Default | Meaning |
| --- | --- | --- |
| `ADDR` | `:8081` | Listen address |
| `BASE_URL` | request host | Public address printed into QR codes, for example `https://bosho.app` |
| `DATABASE_URL` | required | PostgreSQL connection string (Supabase session pooler, `sslmode=require`) |
| `S3_ENDPOINT` | required | S3-compatible host, for example `s3.us-east-005.backblazeb2.com` |
| `S3_ACCESS_KEY`, `S3_SECRET_KEY` | required | Object storage credentials |
| `S3_BUCKET` | `bosho-photos` | Bucket for photos; create it yourself on B2 (a bucket-limited key cannot create one) |
| `S3_REGION` | empty | Region from the B2 endpoint, for example `us-east-005` |
| `S3_USE_SSL` | on | `0` only for a plain-HTTP store such as a local MinIO |
| `REDIS_URL` | empty | Upstash `rediss://` URL; not used by the app yet |
| `DEMO` | off | `1` seeds the sample restaurant |

## Project layout

```
cmd/bosho/            entry point: load config, connect, start the server
internal/
  config/             settings from environment variables
  database/           PostgreSQL connection, query helpers, migration runner
    migrations/       SQL files, applied in name order at startup
  storage/            photo upload and serving through S3 (Backblaze B2)
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
| `make run` | Run the app with Go against the online services |
| `make up` / `make down` | Start or stop the app in Docker |
| `make logs` | Follow the app log |
| `make test` | `go vet` and unit tests |
| `make psql` | Open a shell on the online database (needs `psql`) |

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
# restaurant-menu
