# Bosho (বসো): Feature List

<!-- presentation-sync: last synced at commit 7d77861 (plus uncommitted owner email and staff passwords) on 2026-11-24 -->

This list is the source of truth for the presentation (`docs/presentation/DECK.md`).
Each feature has a stable ID (`G-03`, `K-01`, ...). Slides refer to features by ID, so a change here shows which slides to update.
Keep the IDs: a removed feature moves to "Removed" with its ID, and a new feature gets the next free number in its group.

**One line:** A guest scans the QR on the table, orders from the phone browser, watches the order cook, asks for the bill and leaves a review. No app, no signup.

## Guest (phone browser, `/t/<token>`)

| ID | Feature | Where in code |
| --- | --- | --- |
| G-01 | Scan the table QR and the restaurant's menu opens, already knowing the table. No app, no signup. | `internal/server/guest.go` (`gLanding`, `gEnter`) |
| G-02 | Restaurant branding on every guest screen: logo or initial mark, brand colour as the accent on a clean white page, tagline, address, phone. | `web/templates/guest.html` (`ghead`, `rmark`), `internal/server/theme.go` |
| G-03 | Bangla and English, switchable at any time. | `internal/i18n/i18n.go`, `gLang` |
| G-04 | Menu by category with photos, short descriptions, prices in taka and tags (Popular, Chef's pick, New, Vegetarian, spice level). A "Recommended" row shows the owner's featured dishes. A line says which charges (VAT, service) come on top. Sold-out dishes are greyed out. | `internal/server/queries.go` (`menu`, `bestSellers`), `guest.go` (`infoData`), `guest_menu` template |
| G-05 | One cart shared by the whole group. Every item shows who added it. | `internal/server/guest_cart.go` |
| G-06 | Warning when someone else already added the same dish ("Rafi already added 1 biryani. Add another?"). | `gCartAdd`, i18n `dup_warn` |
| G-07 | Notes per item ("less spicy", "no onion") and quantity changes before placing. Taking a line to zero shows "Removed · Undo". The guest can change the name they order under. | `gCartQty`, `gCartUndo`, `gCartNote`, `gName` |
| G-08 | Groups at a shared table: a newcomer either joins the existing group (someone in the group must tap Allow) or starts a separate order (Group B, own bill). | `guest.go` (`openGroups`, `nextLabel`, `gWait`), `gJoinDecide` |
| G-09 | Place the order after a confirmation that lists the items and the estimated total (with VAT and service charge); an "Order sent" message follows. Order again later as an add-on round on the same bill. | `gPlaceOrder`, `cartTotals`, `g_cartlist` template |
| G-10 | Live order status (Received, Cooking, Ready, Served) with an estimated ready time as a range and a progress bar. When food is ready the phone vibrates, beeps if it can and shows "Ready" in the tab title. | `guest_orders.go` (`liveData`), `queries.go` (`estimateMinutes`), `web/static/app.js` (`checkReady`) |
| G-11 | Change or cancel an item freely until the kitchen accepts; after that, send a change request that staff approve or decline. | `gItemChange` |
| G-12 | Call a waiter, ask for water or tissue, without shouting across the room. The button shows when it was called and when staff answered, and pauses for a minute against repeated taps. | `gCall`, `callButtons` |
| G-13 | Ask for the bill: itemised, with VAT and service charge. Can be withdrawn. | `gBill`, `queries.go` (`bill`) |
| G-14 | Paid screen, then a review: 1 to 5 stars for the visit with a comment, plus optional stars for each dish eaten. One review per group. Guests who give 4 or 5 stars are offered the restaurant's Google review page (when the owner set its link). | `gReview`, `liveData` |
| G-15 | Off-site screen when the restaurant allows ordering only on its Wi-Fi and the guest is elsewhere. | `guest_offsite` template, `network.go` |
| G-16 | Dish sheet: tapping a dish opens a large photo, the full description, option groups with rules ("Size: required, choose 1", "up to 3"), a note, a quantity picker and an Add button showing the live total. The server checks the choices and prices again. | `gDish`, `chosenOptions`, `g_sheet` template, `app.js` |
| G-17 | Search the menu in Bangla or English, and a category bar that highlights the section on screen. | `guest_menu` template, `app.js` |
| G-18 | "Goes well with" suggestions under the cart: dishes often ordered with what is in it, then best sellers. | `guest_cart.go` (`suggestions`) |
| G-19 | Opening hours on the welcome and menu screens, and a "last orders at …" notice in the 30 minutes before. | `guest.go` (`hoursInfo`) |
| G-20 | Receipt after paying, to print or save as a PDF: items, VAT, service charge, total, payment method and time. | `gReceipt`, `guest_receipt` template |
| G-21 | A privacy note on the welcome screen: only the typed name is kept, no phone number, no account. | `guest_enter` template |

## Kitchen and floor staff (`/staff/...`)

| ID | Feature | Where in code |
| --- | --- | --- |
| K-01 | Kitchen board with New, Cooking and Ready columns, refreshing every 3 seconds. Large text and buttons; each ticket shows how long it has waited and turns amber, then red, after the limits set in Settings. | `internal/server/kitchen.go` (`boardData`, `ageClass`), `k_board` template |
| K-02 | Kitchen accepts an order with a ready time, or rejects it with a reason typed into an in-page dialog. | `orderAct`, `app.js` (`ask`) |
| K-03 | Approve or decline guest change requests on accepted items. | `itemAct` |
| K-04 | Waiter calls (waiter, water, tissue) shown on the board for waiters; mark done. | `callDone` |
| K-05 | Sound alert on the kitchen board (new orders, calls, change requests) and the counter (bill requests). Turned on with one tap. | `web/static/app.js`, `data-alert` |
| K-06 | Sold out: kitchen and waiters switch a dish off and on; guests see it greyed out at once. | `stock`, `stockToggle`, `stock_row` template |
| K-07 | Take order: a waiter opens a table and orders for guests who do not scan. These orders skip the Accept step. | `internal/server/waiter.go` |
| K-08 | Counter: all open bills as cards and bill detail. Only the owner removes an item from a bill, with a recorded reason. | `internal/server/counter.go` |
| K-09 | Cashier marks a bill paid by cash, bKash, Nagad or card (recorded, not processed). Only the owner closes a bill without payment. | `sessionPaid`, `sessionClose` |
| K-10 | Light left sidebar for staff; it slides in on phones. It shows only the screens that role can open, and each role starts on its own page: kitchen on the Kitchen board, waiter on Take order, cashier on the Counter, owner on the Dashboard. | `shead` in `web/templates/staff.html`, `staffHome` in `perm.go` |
| K-11 | Undo "Mark ready" (kitchen) or "Mark served" (waiter) within a minute, from the message that appears. Each undo is in the activity log. | `orderAct` (`undo`) |
| K-12 | Kitchen and counter screens keep the tablet awake where the browser allows it (otherwise a note says how), and staff screens can be installed on the home screen. | `app.js`, `web/static/manifest.webmanifest` |
| K-13 | Staff service screens (sidebar, kitchen, counter, take order, sold out) in Bangla or English, switched in the sidebar. Owner pages stay English. | `staffLang` in `auth.go`, `internal/i18n` |
| K-14 | My account: log out on all devices. The owner also changes their own password there (other devices are logged out) and sets their email. Staff can't change their password; the owner sets it. | `account`, `accountPassword`, `logoutAll` in `auth.go`, `accountEmail` in `email.go` |

## Owner (`/owner/...`)

| ID | Feature | Where in code |
| --- | --- | --- |
| O-01 | Dashboard for today: sales, bills paid, open bills, average rating. | `internal/server/owner_dashboard.go`, `dashboard` template |
| O-02 | Today's takings by payment method, to check against the cash drawer. | `dashboard` |
| O-03 | Best sellers over the last 30 days and the latest reviews as cards. | `dashboard` |
| O-04 | Get-started checklist for a new restaurant (profile, menu, tables, VAT, kitchen screen). | `dashboard` template |
| O-05 | Menu editor: categories, dishes in Bangla and English with short descriptions, price, photo (resized for phones on upload), prep minutes, tags, "featured", and options with extra price, optionally in groups with a rule (required choose 1, up to N, any). | `internal/server/owner_menu.go`, `internal/storage/resize.go` |
| O-06 | Tables: add tables ("New table" is the first card), switch a table off (hidden from guests). | `internal/server/owner_tables.go` |
| O-07 | Branded QR codes: round dots, the restaurant's logo or initial in the middle, highest error correction so they still scan. | `internal/qrmark/qrmark.go`, `internal/server/qr.go` |
| O-08 | Print QR cards for all tables or one table; renew a table's QR so old photos of it stop working. | `tablePrint`, `tableNewQR` |
| O-09 | Staff accounts with the roles kitchen, waiter and cashier. The owner sets each staff password, can look it up again on the Staff page (stored encrypted), and can change it at any time, which logs that person out everywhere. A restaurant has one owner: the owner can't add, remove or reset another owner. | `internal/server/owner_staff.go` (`staffPassword`), `secret.go` |
| O-10 | Profile: name, logo upload, brand colour from 12 ready swatches (all readable), tagline, address, phone, Google review link. | `internal/server/owner_profile.go`, `theme.go` |
| O-11 | Settings: VAT %, service charge %, open or closed for orders, staff must accept each guest order (stops fake orders), opening hours shown to guests, an option to take guest orders only during those hours, and the minutes after which kitchen tickets turn amber and red. | `settings`, `settingsSave`, `withinHours` |
| O-12 | Ordering network: guests can order only on the restaurant's Wi-Fi. One tap allows the current network; ranges can be typed. | `internal/server/network.go`, `owner_networks` template |
| O-13 | Reports for the last 7, 30 or 90 days: sales and bills per day, average bill, sales per dish, orders per hour. | `internal/server/reports.go` (`reports`) |
| O-14 | Download orders, payments and reviews as CSV for any dates (opens in Excel with Bangla intact). | `reports.go` (`export`) |
| O-15 | Closing report for any day: money by payment method, bills paid, items removed by staff, rejected orders, groups closed without payment, bills still open. Printable. The cashier can open it too. | `reports.go` (`closing`) |
| O-16 | Activity log: who changed prices, deleted dishes, removed items from bills, took payments, closed bills without payment, rejected orders, or changed staff, passwords or charges. | `internal/server/audit.go` |

## Accounts and access

| ID | Feature | Where in code |
| --- | --- | --- |
| A-01 | Sign up creates a restaurant and its owner account, with the owner's email. | `internal/server/auth.go` (`signup`) |
| A-02 | Staff log in with phone and password (bcrypt). Already logged in skips the login page; after login, staff return to the page they asked for. Five wrong passwords lock that number for 15 minutes. | `login`, `?next=` |
| A-03 | Each role does only its own tasks, checked by the server on every request, not just hidden on the page. Kitchen accepts, rejects and marks ready; waiter takes orders, marks served and answers calls; cashier takes payment. Removing an item from a bill and closing without payment are owner only. Refused attempts are logged. | `internal/server/perm.go` (`rolePerms`, `need`), `perm_test.go` |
| A-04 | Forgot password for owners: a link by email that works once for 1 hour and logs out every device. The page never says whether an account exists. Staff ask their owner instead. | `internal/server/email.go` (`forgot`, `resetSave`), `internal/mail` |
| A-05 | Email confirmation: the owner gets a link at signup (valid 24 hours; send again once a minute, 5 a day). Off by default; when switched on, an unconfirmed owner can set up but guests see "Not taking orders yet". Emails go through Gmail (SMTP) or Resend, or into the log when neither is set. | `email.go` (`verifyEmail`, `awaitingEmail`), `EMAIL_VERIFY` |

## Platform and rules

| ID | Feature | Where in code |
| --- | --- | --- |
| P-01 | Web app: Go server, HTML templates and htmx. Works on any phone browser. | `cmd/bosho`, `web/` |
| P-02 | Multi-tenant: every query is filtered by restaurant, so one installation serves many restaurants. | all handlers |
| P-03 | Money in poisha; prices, VAT and service charge always computed on the server. Name and price are copied onto each order line so old bills never change. | `internal/money`, `order_items` table |
| P-04 | The bill belongs to a group, not a table. | `table_sessions` table |
| P-05 | Redis change counter: self-refreshing screens get an empty 204 reply when nothing changed, without touching PostgreSQL. Works without Redis too. | `internal/cache`, `internal/server/poll.go` |
| P-06 | Runs on free online services: Supabase PostgreSQL, Backblaze B2 photos (any S3 store), Upstash Redis. | `internal/config`, `internal/storage` |
| P-07 | Docker and Fly.io deploy (Singapore region) with one machine always running and a health check every 30 seconds; schema migrations run at startup. | `Dockerfile`, `fly.toml`, `internal/database/migrations` |
| P-08 | Demo mode seeds a sample restaurant; local runs put the laptop's Wi-Fi IP into the QR codes so phones can scan them. | `internal/server/demo.go`, `Makefile` |
| P-09 | Made for weak internet and busy hands: a "No internet, reconnecting" banner, buttons that show they are working and cannot send twice, short messages at the bottom of the screen, and questions asked in the page's language instead of browser pop-ups. | `web/static/app.js`, `ui` template |
| P-10 | Fast on slow phones: fonts served by the app itself, static files cached and versioned, dish photos in two sizes. | `web/static/fonts`, `asset` in `helpers.go`, `internal/storage/resize.go` |
| P-11 | Safe writes: every database write is checked, money changes (paying, closing, removing items) save in one transaction, a second tap changes nothing, and a background job tidies idle groups, old carts and calls every 10 minutes. The app keeps working when photo storage or Redis is down. | `helpers.go` (`inTx`), `maintenance.go`, `internal/storage` |
| P-12 | Security: guest phones limited to 60 taps a minute, posts from other websites refused, strict security headers, Secure cookies on HTTPS. | `internal/server/middleware.go` |
| P-13 | Operations: request ids and structured logs, a time limit on every request, nightly backup and restore-check commands, a busy-evening load test, and tests of the money and order rules run on every push. | `middleware.go`, `scripts/`, `cmd/loadtest`, `flow_test.go`, `.github/workflows/test.yml` |

## Not built yet

Online payment (bKash, Nagad, card gateway), split bill, tips, discounts, kitchen ticket printing, subscription billing, live push (screens poll every 3 to 4 seconds), allergen information, dark mode, kiosk mode for table tablets, multiple branches. Full list with status: `docs/missing-features.md`.

## Removed

None yet.
