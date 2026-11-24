# Bosho (বসো)

QR table ordering for restaurants. A guest scans the QR on the table, orders from the phone browser, watches the order status, asks for the bill, and leaves a review.

Go + htmx. Data, photos and Redis are free online services, so only the app runs locally (or in Docker).

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

- Owner login: phone `01700000000`, password `demo1234` (the demo has two branches, Dhanmondi and Gulshan; the owner starts in "All branches")
- Guest link for table 1: http://localhost:8081/t/demo-table-1 (Dhanmondi), http://localhost:8081/t/demo-gulshan-1 (Gulshan)
- Gulshan's cashier: phone `01700000004`, password `demo1234`

Set `DEMO=0` and use `/signup` for a real restaurant.

`make run` and `make up` find the laptop's Wi-Fi IP on every start and put it into the QR codes, as long as `BASE_URL` is empty in `.env`, so phones on the same Wi-Fi can scan them.

Tables are created by the migrations on the first start. Any S3-compatible store works for photos (Cloudflare R2, AWS S3, a self-hosted MinIO); only the `S3_*` settings change.

## Settings

`make run`, `make up` and Compose read `.env` (see `.env.example`). The app itself reads these variables:

| Variable | Default | Meaning |
| --- | --- | --- |
| `ADDR` | `:8081` | Listen address |
| `BASE_URL` | request host | Public address printed into QR codes, for example `https://bosho.food` |
| `DATABASE_URL` | required | PostgreSQL connection string (Supabase session pooler, `sslmode=require`) |
| `S3_ENDPOINT` | required | S3-compatible host, for example `s3.us-east-005.backblazeb2.com` |
| `S3_ACCESS_KEY`, `S3_SECRET_KEY` | required | Object storage credentials |
| `S3_BUCKET` | `bosho-photos` | Bucket for photos; create it yourself on B2 (a bucket-limited key cannot create one) |
| `S3_REGION` | empty | Region from the B2 endpoint, for example `us-east-005` |
| `S3_USE_SSL` | on | `0` only for a plain-HTTP store such as a local MinIO |
| `REDIS_URL` | empty | Upstash `rediss://` URL. Optional; without it every refresh queries PostgreSQL |
| `DEMO` | off | `1` seeds the sample restaurant, with two branches |
| `BREVO_API_KEY` | empty | Brevo API key for owner emails. Sends over HTTPS, so it works on Render's free plan, which blocks SMTP. The `EMAIL_FROM` address must be a sender verified in Brevo (a Gmail address is fine) |
| `SMTP_HOST`, `SMTP_PORT`, `SMTP_USER`, `SMTP_PASSWORD` | empty, `587` | SMTP server for owner emails (confirm email, forgot password). Gmail: `smtp.gmail.com`, your Gmail address, and an [app password](https://myaccount.google.com/apppasswords) (needs 2-Step Verification) |
| `RESEND_API_KEY` | empty | Resend instead of SMTP, once you have your own domain. With neither set, emails are printed in the log |
| `EMAIL_FROM` | `Bosho <SMTP_USER>` | Sender name and address. With Gmail it must be your Gmail address |
| `EMAIL_VERIFY` | off | `1`: guests can't order until the owner confirms their email; owners can still set up. The platform admin can override it per business at `/admin` (`global`, `required`, `off`). Off: no confirmation link is sent and the per-restaurant override is ignored |
| `PASSWORD_KEY` | stored in the database | 32 bytes in base64 (`openssl rand -base64 32`); encrypts the staff passwords the owner can look up. Don't change it once set |
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
  mail/               sending emails through Brevo, SMTP (Gmail) or Resend, or into the log when none is set
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
    perm.go             what each role may do, checked on every route
    email.go            owner email: confirmation link, forgot password
    secret.go           encryption of the staff passwords the owner can look up
    kitchen.go          kitchen board, order and item actions, sold-out list
    counter.go          open bills, mark paid, close
    waiter.go           ordering on behalf of guests
    owner_*.go          dashboard, menu, tables and QR, staff, settings, profile, branches
    admin.go            platform admin panel (/admin): login with an emailed code, businesses, branch limit
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
| `make create-admin EMAIL=you@example.com` | Add a platform admin for `/admin`, or set a new password for one (asks for it, or reads `ADMIN_PASSWORD`; 12 or more characters) |

To change the schema, add a new file such as `internal/database/migrations/0002_add_tips.sql`. It runs once at the next start. Never edit a migration that has already run.

Queries are written with `?` placeholders; `internal/database` rewrites them to PostgreSQL's `$1` form.

## Screens

| Who | Where | What |
| --- | --- | --- |
| Guest | `/t/<token>` (from the QR) | Menu with search, tags and a recommended row; dish sheet with options, quantity and live total; shared cart with full total and suggestions; order status with progress and a ready alert; bill, call waiter, receipt, review. Bangla and English. |
| Kitchen / waiter | `/staff/kitchen` | New, Cooking, Ready columns with amber and red waiting times; kitchen accepts with a ready time, rejects and marks ready; waiter marks served and answers calls; each can undo their own tap; change requests, sound alert, keeps the screen on |
| Waiter | `/staff/tables` | Order on behalf of guests who do not scan |
| Kitchen / waiter | `/staff/stock` | Mark dishes sold out |
| Cashier | `/staff/counter`, `/owner/closing` | Open bills, mark paid, printed bill and receipt with BIN and the branch's invoice number (GUL-0042), closing report (the owner or a manager also gives discounts with a reason, removes items with a reason and closes without payment) |
| Manager | `/owner` | Runs one branch like the owner, except VAT and service charge, branding, other branches, and managers or the owner |
| Owner | `/owner` | Sales today, best sellers, reviews, menu (descriptions, tags, featured, option groups), tables and printable QR cards, staff (manager, kitchen, waiter, cashier; the owner sets and can look up their passwords, and moves people between branches), settings (charges, hours, orders only in hours, ticket colours), reports with CSV export, closing report, activity log |
| Owner | `/owner/branches`, `/owner/business` | Branches: add (copying menu, settings and branding as ticked), edit, close for now, archive, bring back; the sidebar switcher with "All branches" for combined dashboard, reports, closing report and activity log; the business's name and branding, which a branch can override |
| Platform admin | `/admin` | Separate login with email, password and an emailed 6-digit code; every business with its owner and branches; branch limit and email confirmation per business, with an admin log |
| Any staff | `/staff/account` | Log out on all devices; the owner also changes their own password and email here |
| Owner | `/forgot`, `/reset`, `/verify-email` | Forgot password by email link, and email confirmation |

## Rules built in

- The bill belongs to a group, not a table. A second scan on a busy table chooses "join" (needs an Allow tap from the group) or "separate order" (Group B with its own bill).
- Each guest order waits for a staff Accept by default (Settings can turn this off). Orders entered by a waiter skip the wait.
- A guest can change or cancel an item freely until the kitchen accepts. After that it is a request the staff approve or decline. After serving, only staff can remove it, with a recorded reason.
- Prices, VAT and service charge are always computed on the server. Money is stored in poisha.
- A business groups its branches, and every query is filtered by branch (or by business in the owner's "All branches" views), so one installation serves many restaurants. A branch's menu is copied when it is created and separate after that; each branch counts its own invoice numbers.
- A dish with options is added from its sheet only; required option groups must be chosen, and the server checks the rules and prices again.
- Staff service screens (sidebar, kitchen, counter, take order, sold out) switch between English and Bangla; owner pages are English.
- A discount (percent or taka, with a reason, by the owner or a manager) comes off the subtotal before service charge and VAT. Each paid bill gets the branch's next invoice number.
- Money and access changes are recorded in the activity log: price changes, removed items, discounts, payments, groups closed without payment, staff and password changes.
- Each role does only its own tasks, and the server checks it on every request (`internal/server/perm.go`). One owner per business: nobody can add, remove or reset another owner. A manager runs one branch and can't touch managers, the owner, VAT or service charge.
- A business may have as many branches (not archived) as the platform admin allows, 3 by default. An archived branch's QR codes and staff logins stop working; its data stays.
- Login locks for 15 minutes after 5 wrong passwords; guest phones are limited to 60 taps a minute; posts from other websites are refused.
- Every write is checked; multi-step changes (paying, closing, removing items) run in one transaction. A background job tidies idle groups, old carts and calls every 10 minutes.

## Not built yet

Online payment, split bill, tips, kitchen ticket printing, subscription billing, one person working at several branches, a shared menu across branches, the rest of the admin panel (other limits, feature switches, log in as owner), allergen information, dark mode, kiosk mode for table tablets, built-in HTTPS (Fly provides it; elsewhere put a reverse proxy such as Caddy in front). Screens refresh by polling every 3 to 4 seconds. See `docs/missing-features.md` for the full list and status.
# restaurant-menu
# restaurant-menu
