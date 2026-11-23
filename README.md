# Bosho (বসো)

QR table ordering for restaurants. A guest scans the QR on the table, orders from the phone browser, watches the order status, asks for the bill, and leaves a review.

Go + htmx. Data, photos and Redis are free online services, so only the app runs locally (or in Docker). The product plan is in `qr-table-ordering-plan.md`.

## Start

1. Create the online services (all have free plans):
   - **PostgreSQL**: [Supabase](https://supabase.com) -> New project -> **Connect** -> copy the **Session pooler** string and fill in the password.
   - **Photos**: [Backblaze B2](https://www.backblaze.com/sign-up/cloud-storage) (10 GB free, no card) -> **Buckets** -> **Create a Bucket** (unique name, **Private**) and note its **Endpoint**, for example `s3.us-east-005.backblazeb2.com`. Then **Application Keys** -> **Add a New Application Key** with **Read and Write** on that bucket. Copy the **keyID** and **applicationKey** (shown only once).
   - **Redis**: [Upstash](https://upstash.com) -> create database -> copy the `rediss://` URL. Optional: it lets the screens that refresh themselves skip the database when nothing changed (see `internal/cache`).
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
| `REDIS_URL` | empty | Upstash `rediss://` URL. Optional; without it every refresh queries PostgreSQL |
| `DEMO` | off | `1` seeds the sample restaurant |
| `TRUST_PROXY` | off | `1` when a reverse proxy (Cloudflare, Fly, Render, nginx) sits in front: guest addresses are then read from `X-Forwarded-For`. Needed for **Ordering network**; leave off otherwise, or anyone could fake their address |

## Project layout

```
cmd/bosho/            entry point: load config, connect, start the server and the cleanup job
cmd/loadtest/         busy-evening load test (make loadtest)
scripts/              database backup and restore check
internal/
  config/             settings from environment variables
  database/           PostgreSQL connection, query helpers, migration runner
    migrations/       SQL files, applied in name order at startup
  storage/            photo upload (dish photos resized to two sizes) and serving through S3 (Backblaze B2)
  cache/              Redis change counter that lets unchanged screens skip the database
  money/              taka and poisha parsing, formatting, percentages
  i18n/               Bangla and English texts for guest and staff service screens
  server/             HTTP handlers
    server.go           app wiring, routes, rendering
    middleware.go       request ids and logging, crash recovery, security headers, cross-site check, rate limits, time limit
    maintenance.go      background cleanup every 10 minutes
    audit.go            activity log
    reports.go          reports, closing report, CSV export
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
  static/             CSS, htmx, app.js (polling, alerts, dialogs, dish sheet), fonts, icons, staff app manifest
Dockerfile, docker-compose.yml, Makefile, .env.example
```

Handlers keep their SQL next to the code that uses it; queries shared by several screens are in `queries.go`.

## Development

| Command | Does |
| --- | --- |
| `make run` | Run the app with Go against the online services |
| `make up` / `make down` | Start or stop the app in Docker |
| `make logs` | Follow the app log |
| `make test` | `go vet` and tests. Set `TEST_DATABASE_URL` (any PostgreSQL; the tests use a throwaway schema) to also run the order and money tests; CI does |
| `make psql` | Open a shell on the online database (needs `psql`) |
| `make backup` | Save the online database to `backups/` and check the file (needs `pg_dump` 17+). Run it nightly |
| `make restore-check` | Restore the newest backup into a throwaway local PostgreSQL in Docker and count the rows |
| `make loadtest` | Simulate a busy evening against a running app (`URL=...`); places real orders, so only on test data |

To change the schema, add a new file such as `internal/database/migrations/0002_add_tips.sql`. It runs once at the next start. Never edit a migration that has already run.

Queries are written with `?` placeholders; `internal/database` rewrites them to PostgreSQL's `$1` form.

## Screens

| Who | Where | What |
| --- | --- | --- |
| Guest | `/t/<token>` (from the QR) | Menu with search, tags and a recommended row; dish sheet with options, quantity and live total; shared cart with full total and suggestions; order status with progress and a ready alert; bill, call waiter, receipt, review. Bangla and English. |
| Kitchen / waiter | `/staff/kitchen` | New, Cooking, Ready columns with amber and red waiting times; kitchen accepts with a ready time, rejects and marks ready; waiter marks served and answers calls; each can undo their own tap; change requests, sound alert, keeps the screen on |
| Waiter | `/staff/tables` | Order on behalf of guests who do not scan |
| Kitchen / waiter | `/staff/stock` | Mark dishes sold out |
| Cashier | `/staff/counter`, `/owner/closing` | Open bills, mark paid, closing report (the owner also removes items with a reason and closes without payment) |
| Owner | `/owner` | Sales today, best sellers, reviews, menu (descriptions, tags, featured, option groups), tables and printable QR cards, staff (kitchen, waiter, cashier; password reset), settings (charges, hours, orders only in hours, ticket colours), reports with CSV export, closing report, activity log |
| Any staff | `/staff/account` | Change own password, log out on all devices |

## Rules built in

- The bill belongs to a group, not a table. A second scan on a busy table chooses "join" (needs an Allow tap from the group) or "separate order" (Group B with its own bill).
- Each guest order waits for a staff Accept by default (Settings can turn this off). Orders entered by a waiter skip the wait.
- A guest can change or cancel an item freely until the kitchen accepts. After that it is a request the staff approve or decline. After serving, only staff can remove it, with a recorded reason.
- Prices, VAT and service charge are always computed on the server. Money is stored in poisha.
- Every query is filtered by restaurant, so one installation serves many restaurants.
- A dish with options is added from its sheet only; required option groups must be chosen, and the server checks the rules and prices again.
- Staff service screens (sidebar, kitchen, counter, take order, sold out) switch between English and Bangla; owner pages are English.
- Money and access changes are recorded in the activity log: price changes, removed items, payments, groups closed without payment, staff and password changes.
- Each role does only its own tasks, and the server checks it on every request (`internal/server/perm.go`). One owner per restaurant: an owner can't add, remove or reset another owner.
- Login locks for 15 minutes after 5 wrong passwords; guest phones are limited to 60 taps a minute; posts from other websites are refused.
- Every write is checked; multi-step changes (paying, closing, removing items) run in one transaction. A background job tidies idle groups, old carts and calls every 10 minutes.

## Not built yet

Online payment, split bill, tips, discounts, kitchen ticket printing, subscription billing, owner password reset by SMS, allergen information, dark mode, kiosk mode for table tablets, built-in HTTPS (Fly provides it; elsewhere put a reverse proxy such as Caddy in front). Screens refresh by polling every 3 to 4 seconds. See `docs/missing-features.md` for the full list and status.
# restaurant-menu
# restaurant-menu
