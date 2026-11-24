# Missing Features

First written 2026-11-22 against commit 08f42e3. Status updated 2026-11-22.

These are the features Bosho needs to be reliable and stable in a real restaurant. "Must" means it should exist before restaurants pay. "Option" means a restaurant can switch it on in Settings.

Each item is marked:

- **Done**: built and tested.
- **Partly**: the code part is built; something is left for you, explained on the line.
- **Your step**: needs your account, money or hardware, not code.
- **Later**: not built yet, on purpose (usually waits for online payment).

## 1. Reliability and stability (must)

1. **Check every database write for errors.** Done. Every failed write is now logged. Accept, reject, ready, served, change requests, waiter calls, the bill, payment, closing, joining, placing orders and reviews answer "Something went wrong" instead of looking as if they worked.
2. **Use transactions for every multi-step change.** Done. Mark paid, close without payment, served, approving a cancellation, removing an item, starting a group, cancelling an item and reviews each save in one transaction. Placing an order already used one; its writes are now checked too.
3. **Make repeated taps safe (idempotency).** Done. Buttons can't be tapped twice while sending. Every status change only applies once (for example "paid" only from open). A second "Mark paid" records no second payment; the database test proves this. Orders from one group need 10 seconds between them, and calls are de-duplicated.
4. **Keep the server always on during opening hours.** Done in `fly.toml`: one machine always runs (`min_machines_running = 1`, auto-stop off). This costs a little more on Fly than stopping when idle.
5. **Keep the database awake.** Partly. While the app runs, its cleanup job queries the database every 10 minutes, so Supabase never sees a week without activity. Your step: a paid Supabase plan also adds daily backups.
6. **Daily database backups with a tested restore.** Partly. `make backup` saves the database to `backups/` and checks the file reads back. `make restore-check` restores the newest one into a throwaway local PostgreSQL and counts the rows. Your step: run `make backup` every night (a cron line is in `scripts/backup.sh`) and copy `backups/` somewhere safe.
7. **Survive Redis and photo storage failures.** Done. Redis was already optional. The app now starts even when photo storage can't be reached and keeps checking in the background. A photo that can't load shows a drawing instead.
8. **Reconnect and catch up on weak internet.** Done (frontend work): a "No internet, reconnecting" banner, and screens refresh by themselves when the connection is back.
9. **Kitchen alarm for waiting orders.** Done. The sound repeats every 15 seconds until an order is accepted, and tickets turn amber, then red, after the minutes set in Settings.
10. **Automatic cleanup of stale sessions.** Done. Every 10 minutes the app closes idle groups that never ordered (at every table, not only the next one scanned), empties carts of closed groups, ends waiter calls older than 3 hours and deletes expired logins. Groups with orders are never closed by it, because they hold money.
11. **Health check that tests the database.** Done. `/healthz` already queried PostgreSQL. `fly.toml` now has Fly check it every 30 seconds and restart a machine that fails.
12. **Error logging and alerts.** Partly. Logs are structured (`log/slog`). Every request gets an id (`X-Request-Id`). Server errors, refused requests, slow requests (over 2 seconds), crashes (with stack trace), login lockouts and cleanup steps are logged. Your step: alerts need a service; Fly can send log alerts, or use an uptime monitor (13).
13. **Uptime monitoring.** Your step. Add `https://<your-app>/healthz` to a free monitor such as UptimeRobot or Better Stack.
14. **Timeouts on every request and query.** Done. Each request gets at most 20 seconds, then "The server is busy. Please try again." Photo uploads are exempt. The server also has read, write and idle timeouts. Queries don't have their own time limit; the request limit covers them.
15. **Load test a busy evening.** Done. `make loadtest` (`cmd/loadtest`) runs 60 guests polling and ordering, plus a kitchen screen. Against a local database there were no failures; the 95th-percentile answer was under 0.6 seconds for guests and 0.73 seconds for the kitchen. The kitchen board and counter used one query per order or bill; they now batch them.
16. **More automated tests for the money and order rules.** Done. `internal/server/flow_test.go` runs the real app against PostgreSQL in its own throwaway schema. It covers cart totals with VAT and service, placing an order, accept, a change request approved, mark paid twice (one payment), the receipt, joining with Allow, the login lockout, cross-site refusal, opening hours and CSV export. `.github/workflows/test.yml` runs it with `go vet` and the format check on every push.
17. **Staging copy of the app.** Your step. Create a second Fly app and a second Supabase project, and deploy there first. No code change is needed.

## 2. Security (must)

1. **Login rate limiting and lockout.** Done. 5 wrong passwords for one phone number, or 30 from one network, lock login for 15 minutes.
2. **Rate limiting on guest actions.** Done. 60 taps a minute per phone and 600 per network, then "Please wait a minute".
3. **CSRF protection.** Done. Form posts and taps that come from another website are refused (checked with `Sec-Fetch-Site` and `Origin`), on top of `SameSite=Lax` cookies.
4. **Security headers.** Done. Content-Security-Policy (this site's files only), `X-Frame-Options: DENY`, `X-Content-Type-Options: nosniff`, `Referrer-Policy`. Cookies are `Secure` on HTTPS.
5. **Password reset.** Done. The owner sets every staff password, can look it up on the Staff page, and can change it at any time (the person is logged out everywhere). Staff can't change their own. An owner who forgets their password gets a reset link by email ("Forgot password?" on the login page). Owners also confirm their email by a link; requiring it before guests can order is off by default (`EMAIL_VERIFY`).
6. **Log out everywhere.** Done: on "My account". Changing a password also logs out every other device. Later: a list of active logins.
7. **Audit log.** Done. The "Activity log" page records price changes, deleted dishes, items removed from bills (with reason and amount), rejected orders, payments, groups closed without payment, staff added, removed or with a reset password, changed charges and password changes.
8. **Check uploaded files are real images.** Done (frontend work): dish photos must decode as an image, and are resized and turned upright.

## 3. Payment and bill (must for version 2)

1. **Online payment.** Later. Needs a merchant account with an aggregator (SSLCommerz, aamarPay or ShurjoPay) and a trade licence.
2. **Refunds and failed-payment handling.** Later, with 1.
3. **Split bill.** Later, with 1.
4. **Partial and mixed payments.** Later.
5. **Discounts and complimentary items.** Done. The cashier or owner gives a percent or taka discount with a reason on the bill page; it is in the activity log, the closing report and the payments CSV. A free item is still an item removed with a reason.
6. **Tips.** Later, with 1.
7. **Digital receipt.** Done (frontend work).
8. **VAT invoice format.** Partly. The BIN (on Profile) and a running invoice number per restaurant are on the guest receipt and the counter's printed bill. Your step: check the current NBR rules (Mushak forms) with an accountant for any other required fields.
9. **End-of-day closing report.** Done. On "Reports", then "Closing report", for any day: money by payment method, bills paid, items removed by staff, orders rejected, groups closed without payment, bills still open. It can be printed.

## 4. Kitchen and staff (must)

1. **Kitchen ticket printing (KOT).** Later. Needs a thermal printer to test with.
2. **Live updates (Server-Sent Events).** Later. Polling with the Redis shortcut handled the load test well.
3. **Kitchen sections.** Later.
4. **Item-level status.** Later.
5. **Open and close the restaurant on a schedule.** Done. A setting, "Take guest orders only during these hours", uses the opening and last-order times. Waiters can still order for a table.

## 5. Optional modes (Settings switches)

1. **Pay first.** Later: needs online payment.
2. **Table tablet.** Later.
3. **Card or deposit before ordering.** Later: needs online payment.
4. **Table PIN.** Later. Wi-Fi-only ordering and staff Accept already stop orders from outside.
5. **Auto-accept.** Already there: Settings can turn off "staff must accept", and waiter orders skip it.
6. **Takeaway pickup.** Later.

## 6. Guest experience (should)

1. **Allergen and spice tags.** Partly: spice level, vegetarian and other tags were done in the frontend work. Allergens are later.
2. **Search on the menu.** Done (frontend work).
3. **Accessibility basics.** Done (frontend work).
4. **Fast first load on slow 4G.** Done (frontend work). Photos are JPEG in two sizes, not WebP, because Go has no WebP encoder.
5. **Privacy note.** Done: under the name box on the welcome screen.
6. **"Review us on Google" link.** Done. The owner adds the link on the Profile page. Guests who give 4 or 5 stars see a "Rate us on Google" button.

## 7. Owner and business (should)

1. **Sales reports.** Done. The "Reports" page shows the last 7, 30 or 90 days: sales and bills per day, the average bill, sales per dish and orders per hour.
2. **Multiple branches.** Later.
3. **Data export (CSV).** Done. Orders (one line per dish), payments and reviews for any date range, from the Reports page. Files open in Excel with Bangla intact.
4. **Subscription billing.** Later. Needs online payment.
5. **POS export or API.** Later. The CSV export covers accountants for now.
