# Bosho (বসো): Feature List

<!-- presentation-sync: last synced at commit 08f42e3 on 2026-11-22 -->

This list is the source of truth for the presentation (`docs/presentation/DECK.md`).
Each feature has a stable ID (`G-03`, `K-01`, ...). Slides refer to features by ID, so a change here shows which slides to update.
Keep the IDs: a removed feature moves to "Removed" with its ID, and a new feature gets the next free number in its group.

**One line:** A guest scans the QR on the table, orders from the phone browser, watches the order cook, asks for the bill and leaves a review. No app, no signup.

## Guest (phone browser, `/t/<token>`)

| ID | Feature | Where in code |
| --- | --- | --- |
| G-01 | Scan the table QR and the restaurant's menu opens, already knowing the table. No app, no signup. | `internal/server/guest.go` (`gLanding`, `gEnter`) |
| G-02 | Restaurant branding on every guest screen: logo or initial mark, brand colour, tagline, address, phone. | `web/templates/guest.html` (`ghead`, `rmark`), `internal/server/theme.go` |
| G-03 | Bangla and English, switchable at any time. | `internal/i18n/i18n.go`, `gLang` |
| G-04 | Menu by category with photos, prices in taka, options with extra price (for example "extra egg +৳20"). Sold-out dishes are greyed out. | `internal/server/queries.go` (`menu`), `guest_menu` template |
| G-05 | One cart shared by the whole group. Every item shows who added it. | `internal/server/guest_cart.go` |
| G-06 | Warning when someone else already added the same dish ("Rafi already added 1 biryani. Add another?"). | `gCartAdd`, i18n `dup_warn` |
| G-07 | Notes per item ("less spicy", "no onion") and quantity changes before placing. | `gCartQty`, `gCartNote` |
| G-08 | Groups at a shared table: a newcomer either joins the existing group (someone in the group must tap Allow) or starts a separate order (Group B, own bill). | `guest.go` (`openGroups`, `nextLabel`, `gWait`), `gJoinDecide` |
| G-09 | Place the order; order again later as an add-on round on the same bill. | `gPlaceOrder` |
| G-10 | Live order status (Received, Cooking, Ready, Served) with an estimated ready time. | `guest_orders.go` (`gOrdersLive`), `queries.go` (`estimateMinutes`) |
| G-11 | Change or cancel an item freely until the kitchen accepts; after that, send a change request that staff approve or decline. | `gItemChange` |
| G-12 | Call a waiter, ask for water or tissue, without shouting across the room. | `gCall` |
| G-13 | Ask for the bill: itemised, with VAT and service charge. Can be withdrawn. | `gBill`, `queries.go` (`bill`) |
| G-14 | Paid screen, then a review: 1 to 5 stars for the visit with a comment, plus optional stars for each dish eaten. One review per group. | `gReview` |
| G-15 | Off-site screen when the restaurant allows ordering only on its Wi-Fi and the guest is elsewhere. | `guest_offsite` template, `network.go` |

## Kitchen and floor staff (`/staff/...`)

| ID | Feature | Where in code |
| --- | --- | --- |
| K-01 | Kitchen board with New, Cooking and Ready columns, refreshing every 3 seconds. | `internal/server/kitchen.go` (`kitchenBoard`), `k_board` template |
| K-02 | Accept an order with a ready time, or reject it with a reason. | `orderAct` |
| K-03 | Approve or decline guest change requests on accepted items. | `itemAct` |
| K-04 | Waiter calls (waiter, water, tissue) shown on the board; mark done. | `callDone` |
| K-05 | Sound alert on the kitchen board (new orders, calls, change requests) and the counter (bill requests). Turned on with one tap. | `web/static/app.js`, `data-alert` |
| K-06 | Sold out: any staff member switches a dish off and on; guests see it greyed out at once. | `stock`, `stockToggle`, `stock_row` template |
| K-07 | Take order: a waiter opens a table and orders for guests who do not scan. These orders skip the Accept step. | `internal/server/waiter.go` |
| K-08 | Counter: all open bills as cards, bill detail, remove an item with a recorded reason. | `internal/server/counter.go` |
| K-09 | Mark a bill paid by cash, bKash, Nagad or card (recorded, not processed), or close it without payment. | `sessionPaid`, `sessionClose` |
| K-10 | Left sidebar for staff; it slides in on phones. Each role starts on its own page. | `shead` in `web/templates/staff.html`, `staffHome` in `auth.go` |

## Owner (`/owner/...`)

| ID | Feature | Where in code |
| --- | --- | --- |
| O-01 | Dashboard for today: sales, bills paid, open bills, average rating. | `internal/server/owner_dashboard.go`, `dashboard` template |
| O-02 | Today's takings by payment method, to check against the cash drawer. | `dashboard` |
| O-03 | Best sellers over the last 30 days and the latest reviews as cards. | `dashboard` |
| O-04 | Get-started checklist for a new restaurant (profile, menu, tables, VAT, kitchen screen). | `dashboard` template |
| O-05 | Menu editor: categories, dishes in Bangla and English, price, photo, prep minutes, options with extra price. | `internal/server/owner_menu.go` |
| O-06 | Tables: add tables ("New table" is the first card), switch a table off (hidden from guests). | `internal/server/owner_tables.go` |
| O-07 | Branded QR codes: round dots, the restaurant's logo or initial in the middle, highest error correction so they still scan. | `internal/qrmark/qrmark.go`, `internal/server/qr.go` |
| O-08 | Print QR cards for all tables or one table; renew a table's QR so old photos of it stop working. | `tablePrint`, `tableNewQR` |
| O-09 | Staff accounts with roles: owner, kitchen, waiter, cashier. | `internal/server/owner_staff.go` |
| O-10 | Profile: name, logo upload, brand colour from 12 ready swatches (all readable), tagline, address, phone. | `internal/server/owner_profile.go`, `theme.go` |
| O-11 | Settings: VAT %, service charge %, open or closed for orders, staff must accept each guest order (stops fake orders). | `settings`, `settingsSave` |
| O-12 | Ordering network: guests can order only on the restaurant's Wi-Fi. One tap allows the current network; ranges can be typed. | `internal/server/network.go`, `owner_networks` template |

## Accounts and access

| ID | Feature | Where in code |
| --- | --- | --- |
| A-01 | Sign up creates a restaurant and its owner account. | `internal/server/auth.go` (`signup`) |
| A-02 | Staff log in with phone and password (bcrypt). Already logged in skips the login page; after login, staff return to the page they asked for. | `login`, `?next=` |

## Platform and rules

| ID | Feature | Where in code |
| --- | --- | --- |
| P-01 | Web app: Go server, HTML templates and htmx. Works on any phone browser. | `cmd/bosho`, `web/` |
| P-02 | Multi-tenant: every query is filtered by restaurant, so one installation serves many restaurants. | all handlers |
| P-03 | Money in poisha; prices, VAT and service charge always computed on the server. Name and price are copied onto each order line so old bills never change. | `internal/money`, `order_items` table |
| P-04 | The bill belongs to a group, not a table. | `table_sessions` table |
| P-05 | Redis change counter: self-refreshing screens get an empty 204 reply when nothing changed, without touching PostgreSQL. Works without Redis too. | `internal/cache`, `internal/server/poll.go` |
| P-06 | Runs on free online services: Supabase PostgreSQL, Backblaze B2 photos (any S3 store), Upstash Redis. | `internal/config`, `internal/storage` |
| P-07 | Docker and Fly.io deploy (Singapore region); schema migrations run at startup. | `Dockerfile`, `fly.toml`, `internal/database/migrations` |
| P-08 | Demo mode seeds a sample restaurant; local runs put the laptop's Wi-Fi IP into the QR codes so phones can scan them. | `internal/server/demo.go`, `Makefile` |

## Not built yet

Online payment (bKash, Nagad, card gateway), split bill, tips, receipt and kitchen printing, subscription billing, image compression, login rate limiting, built-in HTTPS, live push (screens poll every 3 to 4 seconds).

## Removed

None yet.
