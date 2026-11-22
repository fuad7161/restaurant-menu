# Missing Features

As of 2026-11-22, checked against commit 08f42e3

These are the features Bosho does not have yet that it needs to be reliable and stable in a real restaurant. Nothing here is built yet. "Must" means it should exist before restaurants pay. "Option" means a restaurant can switch it on in Settings.

## 1. Reliability and stability (must)

1. **Check every database write for errors.** About 60 `a.db.Exec(...)` calls in `internal/server` ignore the error. A failed write (for example accepting an order or marking a bill paid) can look like success to staff.
2. **Use transactions for every multi-step change.** Placing an order already locks the session (`FOR UPDATE`). Do the same for accept and reject, mark paid, close the table, join approval and item change requests, so two staff tapping at once cannot leave a half-done state.
3. **Make repeated taps safe (idempotency).** A double tap or a retry on weak internet must not create two orders, two payments or two waiter calls. Give each action a request id and ignore repeats.
4. **Keep the server always on during opening hours.** `fly.toml` has `min_machines_running = 0` and `auto_stop_machines = 'stop'`, so the first guest after a quiet spell waits for a cold start. Keep at least one machine running.
5. **Keep the database awake.** Free Supabase projects pause after a period of no activity. Use a paid plan, or a scheduled health ping, before going live.
6. **Daily database backups with a tested restore.** Free plans have no point-in-time recovery. Run a nightly `pg_dump` to object storage and test a restore once.
7. **Survive Redis and photo storage failures.** If Upstash or B2 is down, ordering must keep working (fall back to the database, show a placeholder photo). Add a test that runs the app with Redis off.
8. **Reconnect and catch up on weak internet.** Guest and kitchen screens should show "Offline, reconnecting" and reload missed orders by themselves when the connection returns.
9. **Kitchen alarm for waiting orders.** Repeat the sound until accepted, and alert the manager if an order waits longer than a set number of minutes.
10. **Automatic cleanup of stale sessions.** Close or flag sessions left open overnight, old carts and old waiter calls, so the counter screen never fills with dead bills.
11. **Health check that tests the database.** `/healthz` exists; make it also check PostgreSQL so the host restarts a broken machine.
12. **Error logging and alerts.** Structured logs (Go `log/slog`), a request id per request, and an alert (email or Telegram) when errors spike or the app restarts.
13. **Uptime monitoring.** An outside check every minute (for example UptimeRobot or Better Stack) that tells you before the restaurant does.
14. **Timeouts on every request and query.** A slow query must not hang the kitchen screen. Set server read and write timeouts and a context timeout on database calls.
15. **Load test a busy evening.** Simulate 30 tables polling and ordering at once and check response times and database connections (the pool is capped at 20).
16. **More automated tests for the money and order rules.** Totals, VAT, service charge, change requests, join approval, mark paid and close. Run them in CI on every push.
17. **Staging copy of the app.** A second deployment with its own database for testing changes before they reach restaurants.

## 2. Security (must)

1. **Login rate limiting and lockout** after repeated wrong passwords.
2. **Rate limiting on guest orders, cart changes and waiter calls**, per device and per table.
3. **CSRF protection on staff and owner forms.** Cookies use `SameSite=Lax`; add a CSRF token as well.
4. **Security headers:** Content-Security-Policy, `X-Frame-Options`, `X-Content-Type-Options`, `Secure` flag on cookies in production.
5. **Password reset for staff and owner**, for example by the owner resetting staff passwords, and an OTP or support flow for the owner.
6. **Log out everywhere** and a list of active staff sessions.
7. **Audit log** of sensitive actions: price changes, removed items, discounts, mark paid, close without payment.
8. **Check uploaded files are real images**, then resize and convert them (also improves speed).

## 3. Payment and bill (must for version 2)

1. **Online payment** (bKash, Nagad, cards) through one aggregator such as SSLCommerz, aamarPay or ShurjoPay. Mark paid only on the server-to-server confirmation, never on the browser redirect.
2. **Refunds and failed-payment handling**, including a payment that succeeds after the guest closes the page.
3. **Split bill:** split evenly, pay for my items, or pay a custom amount.
4. **Partial and mixed payments**, for example part cash and part bKash.
5. **Discounts and complimentary items** by staff, with a reason, recorded in the audit log.
6. **Tips**, recorded per staff member.
7. **Digital receipt** the guest can download or share.
8. **VAT invoice format** with the restaurant's BIN, following current NBR rules. Check whether an EFD or SDC device link is required.
9. **End-of-day closing report:** totals by payment method, removed items, discounts, open bills left.

## 4. Kitchen and staff (must)

1. **Kitchen ticket printing (KOT)** on an 80 mm thermal printer, and printed bills at the counter.
2. **Live updates (Server-Sent Events)** instead of polling every 3 to 4 seconds, with polling kept as the fallback.
3. **Kitchen sections:** send each item to its station (grill, drinks, bakery).
4. **Item-level status** so the kitchen can mark single items ready, not only the whole order.
5. **Open and close the restaurant for orders** on a schedule, not only by hand.

## 5. Optional modes (Settings switches)

1. **Pay first.** The guest pays online before the order goes to the kitchen. Suits food courts, cafes and fast food. A paid order can skip the staff Accept step.
2. **Table tablet.** A restaurant-owned tablet stays on the table and runs the guest screen in a locked kiosk mode, tied to that table without scanning. Too costly for most restaurants in Bangladesh, so keep it as an option for those who want it.
3. **Card or deposit before ordering** (pre-authorised tab), as a stronger fake-order defence once online payment exists.
4. **Table PIN.** The waiter gives guests a short code when seating them; stricter than Wi-Fi-only ordering.
5. **Auto-accept** for trusted cases, for example a paid order or a waiter-entered order (waiter orders already skip the wait).
6. **Takeaway pickup** from a counter QR, using the same menu and kitchen screen.

## 6. Guest experience (should)

1. **Allergen and spice tags** on dishes.
2. **Search on the menu** for restaurants with long menus.
3. **Accessibility basics:** good contrast, large text, tap targets of at least 44 px, labels on icon buttons.
4. **Fast first load on slow 4G:** compressed photos (WebP), cached menu, small pages.
5. **Privacy note** on the guest screen saying only a display name is stored.
6. **"Review us on Google" link** after a 4 or 5 star rating.

## 7. Owner and business (should)

1. **Sales reports** by day, week and month, by dish and by hour.
2. **Multiple branches** under one owner login.
3. **Data export** (CSV) of orders, payments and reviews.
4. **Subscription billing** for your own monthly fee.
5. **POS export or API** for restaurants that already use HisabPlus, Restora or similar.
